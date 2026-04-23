package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/desico/chapter-overview/internal/model"
	"github.com/desico/chapter-overview/internal/pdf"
	"github.com/desico/chapter-overview/internal/provider"
)

const (
	maxChapters     = 15
	pagesBatchSize  = 15    // pages per LLM batch; safe for moonshot-v1-8k with dense/CJK content
	maxChapterChars = 80000 // ~20k tokens; fits comfortably in moonshot-v1-32k
)

// EventType classifies a pipeline progress event.
type EventType string

const (
	EventDetecting       EventType = "detecting"
	EventChapterDetected EventType = "chapter_detected"
	EventSummarizing     EventType = "summarizing"
	EventChapterDone     EventType = "chapter_done"
)

// ProgressEvent carries progress information from the pipeline.
type ProgressEvent struct {
	Type         EventType      `json:"type"`
	Progress     float64        `json:"progress,omitempty"`
	Message      string         `json:"message,omitempty"`
	Chapter      *model.Chapter `json:"chapter,omitempty"`
	ChapterIndex int            `json:"chapter_index,omitempty"`
	Usage        provider.Usage `json:"usage"`
	DurationMs   int64          `json:"duration_ms,omitempty"`
	BatchStart   int            `json:"batch_start,omitempty"`
	BatchEnd     int            `json:"batch_end,omitempty"`
}

// Options controls pipeline behavior.
type Options struct {
	MaxConcurrent int
	OnProgress    func(ProgressEvent)
	// PageLoader overrides the default pdf.ExtractPagesRange for page content retrieval.
	// Set to a *pdf.PageCache.GetRange to share extracted pages across detect and summarize.
	PageLoader func(pdfPath string, start, end int) ([]model.PageContent, error)
}

func (o Options) concurrency() int {
	if o.MaxConcurrent <= 0 {
		return 50
	}
	return o.MaxConcurrent
}

func (o Options) emit(e ProgressEvent) {
	if o.OnProgress != nil {
		o.OnProgress(e)
	}
}

func (o Options) loadPages(pdfPath string, start, end int) ([]model.PageContent, error) {
	if o.PageLoader != nil {
		return o.PageLoader(pdfPath, start, end)
	}
	return pdf.ExtractPagesRange(pdfPath, start, end)
}

// DetectChapters finds chapter boundaries in the PDF.
// It tries the PDF's embedded TOC first; if that fails or is too sparse,
// it falls back to asking the LLM to analyze each page.
func DetectChapters(ctx context.Context, pdfPath string, prov provider.Provider, opts Options) ([]model.Chapter, error) {
	totalPages, err := pdf.GetPageCount(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("getting page count: %w", err)
	}

	// Try TOC first — require at least 3 entries to be worth using
	toc, err := pdf.ExtractTOC(pdfPath)
	if err == nil && len(toc) >= 3 {
		fmt.Printf("[detect] Found %d TOC entries — validating with LLM\n", len(toc))
		return validateTOC(ctx, pdfPath, toc, totalPages, prov, opts)
	}

	// No usable TOC — scan pages with LLM
	if len(toc) > 0 {
		fmt.Printf("[detect] TOC too sparse (%d entries), falling back to page scan\n", len(toc))
	}
	fmt.Printf("[detect] Scanning %d pages with LLM\n", totalPages)
	return scanPagesForChapters(ctx, pdfPath, totalPages, prov, opts)
}

// SummarizeChapters fills in the Summary field for each chapter.
func SummarizeChapters(ctx context.Context, pdfPath string, chapters []model.Chapter, prov provider.Provider, opts Options) ([]model.Chapter, error) {
	sem := make(chan struct{}, opts.concurrency())
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	for i := range chapters {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()

			ch := &chapters[idx]
			summary, status, err := summarizeChapter(ctx, pdfPath, *ch, prov, opts)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("chapter %d %q: %w", ch.Index, ch.Title, err)
				}
				ch.Summary = fallbackWarning
				ch.SummaryStatus = model.SummaryFailed
				return
			}
			ch.Summary = summary
			ch.SummaryStatus = status
		}(i)
	}
	wg.Wait()

	if firstErr != nil {
		fmt.Printf("[warn] summarization had errors: %v\n", firstErr)
	}
	return chapters, nil
}

// validateTOC classifies and finalizes a raw TOC into ≤maxChapters chapters.
// Front matter collapses to one boundary; back matter collapses to one boundary;
// remaining content titles are LLM-merged into the remaining budget if needed.
func validateTOC(ctx context.Context, _ string, toc []model.ChapterBoundary, totalPages int, prov provider.Provider, opts Options) ([]model.Chapter, error) {
	return finalizeStructure(ctx, toc, totalPages, prov, opts), nil
}

// scanPagesForChapters analyzes pages in batches to detect chapter starts.
// Uses pagesBatchSize pages per LLM call to minimize API round-trips while
// fitting within the detection model's context window.
func scanPagesForChapters(ctx context.Context, pdfPath string, totalPages int, prov provider.Provider, opts Options) ([]model.Chapter, error) {
	type batchResult struct {
		boundaries []model.ChapterBoundary
	}

	type batch struct{ start, end int }
	var batches []batch
	for start := 1; start <= totalPages; start += pagesBatchSize {
		end := start + pagesBatchSize - 1
		if end > totalPages {
			end = totalPages
		}
		batches = append(batches, batch{start, end})
	}

	fmt.Printf("[detect] Scanning %d pages in %d batches of %d\n", totalPages, len(batches), pagesBatchSize)

	batchResults := make([]batchResult, len(batches))
	sem := make(chan struct{}, opts.concurrency())
	var wg sync.WaitGroup

	for i, b := range batches {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, b batch) {
			defer wg.Done()
			defer func() { <-sem }()

			pages, err := opts.loadPages(pdfPath, b.start, b.end)
			if err != nil {
				fmt.Printf("[warn] batch %d-%d extraction failed: %v\n", b.start, b.end, err)
				return
			}

			prompt := buildBatchDetectionPrompt(pages)
			var batchImages [][]byte
			for _, p := range pages {
				batchImages = append(batchImages, p.Images...)
			}

			var llmResult provider.Response
			start := time.Now()
			if len(batchImages) > 0 {
				llmResult, err = prov.CompleteMultimodal(ctx, prompt, batchImages)
			} else {
				llmResult, err = prov.Complete(ctx, prompt)
			}
			durationMs := time.Since(start).Milliseconds()
			if err != nil {
				fmt.Printf("[warn] batch %d-%d LLM failed: %v\n", b.start, b.end, err)
				return
			}

			opts.emit(ProgressEvent{
				Type:       EventDetecting,
				Usage:      llmResult.Usage,
				DurationMs: durationMs,
				BatchStart: b.start,
				BatchEnd:   b.end,
			})

			boundaries := parseBatchDetectionResponse(llmResult.Content)
			batchResults[idx] = batchResult{boundaries: boundaries}
		}(i, b)
	}
	wg.Wait()

	var boundaries []model.ChapterBoundary
	for _, br := range batchResults {
		boundaries = append(boundaries, br.boundaries...)
	}

	if len(boundaries) == 0 {
		boundaries = []model.ChapterBoundary{{StartPage: 1, Title: filepath.Base(pdfPath)}}
	}

	return finalizeStructure(ctx, boundaries, totalPages, prov, opts), nil
}

// summarizeChapter sends all pages of a chapter to the LLM for summarization,
// validates the response, and falls back to text extraction if the LLM refuses.
func summarizeChapter(ctx context.Context, pdfPath string, ch model.Chapter, prov provider.Provider, opts Options) (string, model.SummaryStatus, error) {
	pages, err := opts.loadPages(pdfPath, ch.StartPage, ch.EndPage)
	if err != nil {
		return "", model.SummaryFailed, err
	}

	var textParts []string
	var allImages [][]byte
	hasImages := false

	for _, p := range pages {
		if p.HasMeaningfulText {
			textParts = append(textParts, p.Text)
		}
		if len(p.Images) > 0 {
			hasImages = true
			allImages = append(allImages, p.Images...)
		}
	}

	fullText := strings.Join(textParts, "\n")
	if len(fullText) > maxChapterChars {
		fullText = fullText[:maxChapterChars] + "\n[...content truncated for length...]"
	}

	basePrompt := buildSummaryPrompt(ch, fullText)

	// First attempt
	summary, totalUsage, totalMs, ok := tryLLMSummary(ctx, prov, basePrompt, allImages, hasImages)
	if !ok {
		// One corrective re-prompt: may catch format errors or soft refusals.
		retryPrompt := basePrompt + "\n\nYour previous response was not valid JSON or was a refusal. Respond again with ONLY the JSON object. Do not include any other text."
		var retryUsage provider.Usage
		var retryMs int64
		summary, retryUsage, retryMs, ok = tryLLMSummary(ctx, prov, retryPrompt, allImages, hasImages)
		totalUsage.InputTokens += retryUsage.InputTokens
		totalUsage.OutputTokens += retryUsage.OutputTokens
		totalMs += retryMs
	}

	status := model.SummaryOK
	if !ok {
		summary, status = buildFallbackSummary(fullText)
	}

	opts.emit(ProgressEvent{
		Type:         EventChapterDone,
		ChapterIndex: ch.Index,
		Chapter: &model.Chapter{
			Index:         ch.Index,
			Title:         ch.Title,
			StartPage:     ch.StartPage,
			EndPage:       ch.EndPage,
			Summary:       summary,
			SummaryStatus: status,
		},
		Usage:      totalUsage,
		DurationMs: totalMs,
	})

	return summary, status, nil
}

// buildSummaryPrompt builds the structured-JSON summary prompt.
func buildSummaryPrompt(ch model.Chapter, text string) string {
	return fmt.Sprintf(`Summarize the following chapter from a PDF document.

Chapter: %s (pages %d–%d)

%s

Respond ONLY with a JSON object, no extra text:
{
  "summarized": true,
  "summary": "3-5 sentence informative summary",
  "refusal_reason": ""
}

If you cannot summarize (content unreadable, access issues, policy refusal),
set "summarized": false, leave "summary": "", and put the reason in "refusal_reason".`,
		ch.Title, ch.StartPage, ch.EndPage, text,
	)
}

// tryLLMSummary makes one LLM call and validates the response.
// Returns (summary, usage, durationMs, ok). ok=false means refusal/invalid/heuristic-tripped.
func tryLLMSummary(ctx context.Context, prov provider.Provider, prompt string, images [][]byte, hasImages bool) (string, provider.Usage, int64, bool) {
	var llmResult provider.Response
	var err error
	start := time.Now()
	if hasImages && len(images) > 0 {
		capped := images
		if len(capped) > 5 {
			capped = capped[:5]
		}
		llmResult, err = prov.CompleteMultimodal(ctx, prompt, capped)
	} else {
		llmResult, err = prov.Complete(ctx, prompt)
	}
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", provider.Usage{}, durationMs, false
	}

	var parsed struct {
		Summarized    bool   `json:"summarized"`
		Summary       string `json:"summary"`
		RefusalReason string `json:"refusal_reason"`
	}
	if err := parseJSON(llmResult.Content, &parsed); err != nil {
		return "", llmResult.Usage, durationMs, false
	}
	if !parsed.Summarized {
		return "", llmResult.Usage, durationMs, false
	}
	if looksLikeRefusal(parsed.Summary) {
		return "", llmResult.Usage, durationMs, false
	}
	return strings.TrimSpace(parsed.Summary), llmResult.Usage, durationMs, true
}

// refusalPhrases are substrings that indicate an LLM refusal masquerading as a summary.
var refusalPhrases = []string{
	"cannot access",
	"can't access",
	"unable to access",
	"i don't have access",
	"i do not have access",
	"no content provided",
	"as an ai",
	"i'm sorry",
	"i am sorry",
	"i cannot",
}

// looksLikeRefusal returns true if the text is too short or contains refusal phrases.
func looksLikeRefusal(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 40 {
		return true
	}
	lower := strings.ToLower(s)
	for _, p := range refusalPhrases {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

const fallbackWarning = "[warning: LLM could not summarize this chapter]"
const fallbackGarbledWarning = "[warning: LLM could not summarize — text extraction produced unreadable output]"
const fallbackTextLimit = 150

// readabilityThreshold is the minimum ratio of recognizable runes required to
// show an excerpt; below this the extracted text is considered garbled.
const readabilityThreshold = 0.6

// buildFallbackSummary returns a warning-prefixed excerpt of the extracted text
// when readable, or a garbled-text warning when the extraction looks like noise.
func buildFallbackSummary(fullText string) (string, model.SummaryStatus) {
	if strings.TrimSpace(fullText) == "" {
		return fallbackWarning, model.SummaryFailed
	}
	// Score readability on the raw text to catch cases where a few ASCII chars
	// are drowned in a sea of PUA/garbled runes.
	if !isReadable(fullText) {
		return fallbackGarbledWarning, model.SummaryFailed
	}
	// Text is readable; sanitize control/PUA chars before displaying the excerpt.
	sanitized := sanitizeExtractedText(fullText)
	if sanitized == "" {
		return fallbackGarbledWarning, model.SummaryFailed
	}
	excerpt := sanitized
	if len(excerpt) > fallbackTextLimit {
		excerpt = excerpt[:fallbackTextLimit]
		if idx := strings.LastIndexAny(excerpt, " \n\t"); idx > fallbackTextLimit/2 {
			excerpt = excerpt[:idx]
		}
		excerpt = strings.TrimRight(excerpt, " \n\t.,;:") + "..."
	}
	return fallbackWarning + "\n\n" + excerpt, model.SummaryFallback
}

// sanitizeExtractedText strips control characters, Unicode replacement runes
// (U+FFFD), and private-use-area runes (U+E000–U+F8FF) that ledongthuc/pdf
// emits when CID-to-Unicode mapping fails.
func sanitizeExtractedText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20:
			// skip other control chars
		case r == '�':
			// replacement rune — unmapped glyph
		case r >= 0xE000 && r <= 0xF8FF:
			// private-use area — ledongthuc CID fallback
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// isReadable returns true when at least readabilityThreshold of the
// non-whitespace runes in s are ASCII printable, CJK ideographs, or CJK
// punctuation. A single-pass check; designed to run on short excerpts (<1ms).
func isReadable(s string) bool {
	var total, recognizable int
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		switch {
		case r >= 0x20 && r <= 0x7E: // ASCII printable
			recognizable++
		case r >= 0x4E00 && r <= 0x9FFF: // CJK unified ideographs
			recognizable++
		case r >= 0x3000 && r <= 0x303F: // CJK symbols and punctuation
			recognizable++
		case r >= 0xFF00 && r <= 0xFFEF: // halfwidth/fullwidth forms
			recognizable++
		case unicode.IsLetter(r) || unicode.IsNumber(r): // other scripts (Latin ext, Cyrillic, etc.)
			recognizable++
		}
	}
	if total == 0 {
		return false
	}
	return float64(recognizable)/float64(total) >= readabilityThreshold
}

// kind labels a chapter boundary as front matter, content, or back matter.
type boundaryKind string

const (
	kindFront   boundaryKind = "front"
	kindContent boundaryKind = "content"
	kindBack    boundaryKind = "back"
)

var (
	// Front matter: non-content sections before chapter 1.
	// Patterns are prefixes; no trailing \b so e.g. "Acknowledgements" matches "acknowledg".
	frontMatterRE = regexp.MustCompile(`(?i)^\s*(preface|foreword|acknowledg|title\s*page|copyright|dedication|(table\s*of\s*)?contents|about\s+(the|this)\s+book|prologue|abstract)`)
	// Back matter: non-content sections after the last chapter.
	backMatterRE = regexp.MustCompile(`(?i)^\s*(index|bibliograph|references|appendi|glossar|about\s+the\s+author|endnotes|notes|colophon|epilogue|afterword)`)
	// Numbered-chapter pattern used to promote a "Chapter 1"/"1 " title as the first content chapter.
	numberedChapterRE = regexp.MustCompile(`(?i)^\s*(chapter\s+\d+|part\s+[IVX0-9]+|\d+[\.\s])`)
	// Subsection pattern: "1.1", "2.3.1", "1.1 Intro", "1.1.1.1"… Anything with two or more dotted numbers at the start.
	subsectionRE = regexp.MustCompile(`^\s*\d+\.\d+`)
)

// stripSubsections drops TOC entries that look like numbered subsections
// ("1.1 Foo", "2.3.1 Bar"). Their parent chapter's page range naturally
// covers them since the next top-level boundary marks the end.
func stripSubsections(boundaries []model.ChapterBoundary) []model.ChapterBoundary {
	out := make([]model.ChapterBoundary, 0, len(boundaries))
	for _, b := range boundaries {
		if subsectionRE.MatchString(b.Title) {
			continue
		}
		out = append(out, b)
	}
	return out
}

// classifyBoundary classifies a single title by heuristic.
func classifyBoundary(title string) boundaryKind {
	switch {
	case frontMatterRE.MatchString(title):
		return kindFront
	case backMatterRE.MatchString(title):
		return kindBack
	default:
		return kindContent
	}
}

// classifyBoundaries tags every boundary as front/content/back using the heuristic.
// Ordering matters: once we hit the first "content" boundary, later front-matter-looking
// titles are treated as content (e.g., a chapter titled "Dedication" mid-book).
// Once we've seen the last numbered content chapter, back-matter-looking titles are honored;
// before then they're treated as content.
func classifyBoundaries(boundaries []model.ChapterBoundary) []boundaryKind {
	kinds := make([]boundaryKind, len(boundaries))
	firstContentIdx := -1
	lastNumberedIdx := -1
	for i, b := range boundaries {
		if numberedChapterRE.MatchString(b.Title) {
			if firstContentIdx == -1 {
				firstContentIdx = i
			}
			lastNumberedIdx = i
		}
	}
	for i, b := range boundaries {
		k := classifyBoundary(b.Title)
		switch k {
		case kindFront:
			if firstContentIdx != -1 && i > firstContentIdx {
				k = kindContent
			}
		case kindBack:
			if lastNumberedIdx != -1 && i < lastNumberedIdx {
				k = kindContent
			}
		}
		kinds[i] = k
	}
	return kinds
}

// classifyBoundariesWithLLM falls back to the LLM when the heuristic produces
// zero front and zero back labels for a sufficiently long list (likely non-English).
// Returns the original kinds unchanged if the LLM call fails or returns garbage.
func classifyBoundariesWithLLM(ctx context.Context, boundaries []model.ChapterBoundary, kinds []boundaryKind, prov provider.Provider, opts Options) []boundaryKind {
	var sb strings.Builder
	sb.WriteString("For each of the following PDF table-of-contents entries, classify as front matter, content, or back matter.\n")
	sb.WriteString("Front matter = preface, dedication, acknowledgements, contents, copyright, etc. Back matter = index, bibliography, appendix, glossary, references, etc. Everything else = content.\n\n")
	for i, b := range boundaries {
		fmt.Fprintf(&sb, "%d. (page %d) %s\n", i+1, b.StartPage, b.Title)
	}
	sb.WriteString(`
Respond ONLY with a JSON array with one object per entry, in the same order:
[{"kind":"front"},{"kind":"content"},{"kind":"back"}, ...]
Valid values for "kind" are exactly: "front", "content", "back".`)

	start := time.Now()
	result, err := prov.Complete(ctx, sb.String())
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return kinds
	}
	opts.emit(ProgressEvent{
		Type:       EventDetecting,
		Usage:      result.Usage,
		DurationMs: durationMs,
	})

	var raw []struct {
		Kind string `json:"kind"`
	}
	if err := parseJSON(result.Content, &raw); err != nil {
		return kinds
	}
	if len(raw) != len(boundaries) {
		return kinds
	}
	out := make([]boundaryKind, len(boundaries))
	for i, r := range raw {
		switch strings.ToLower(strings.TrimSpace(r.Kind)) {
		case "front":
			out[i] = kindFront
		case "back":
			out[i] = kindBack
		default:
			out[i] = kindContent
		}
	}
	return out
}

// classifyWithFallback runs heuristic classification; if the heuristic finds
// zero front and zero back labels among a long list (likely non-English), it
// consults the LLM.
func classifyWithFallback(ctx context.Context, boundaries []model.ChapterBoundary, prov provider.Provider, opts Options) []boundaryKind {
	kinds := classifyBoundaries(boundaries)
	if len(boundaries) <= 10 {
		return kinds
	}
	hasFront, hasBack := false, false
	for _, k := range kinds {
		if k == kindFront {
			hasFront = true
		}
		if k == kindBack {
			hasBack = true
		}
	}
	if hasFront || hasBack {
		return kinds
	}
	return classifyBoundariesWithLLM(ctx, boundaries, kinds, prov, opts)
}

// finalizeStructure classifies boundaries, collapses front/back matter into one
// bucket each, and budget-merges content titles via LLM if they exceed the remaining
// slots. Always returns re-indexed chapters with start/end pages computed.
func finalizeStructure(ctx context.Context, boundaries []model.ChapterBoundary, totalPages int, prov provider.Provider, opts Options) []model.Chapter {
	if len(boundaries) == 0 {
		return []model.Chapter{{Index: 1, Title: "Document", StartPage: 1, EndPage: totalPages}}
	}

	// Sort by page so classification + collapsing are deterministic.
	sort.Slice(boundaries, func(i, j int) bool {
		return boundaries[i].StartPage < boundaries[j].StartPage
	})

	// Drop numbered subsections ("1.1", "2.3.1", …); parent chapter page ranges cover them.
	boundaries = stripSubsections(boundaries)
	if len(boundaries) == 0 {
		return []model.Chapter{{Index: 1, Title: "Document", StartPage: 1, EndPage: totalPages}}
	}

	kinds := classifyWithFallback(ctx, boundaries, prov, opts)

	var front, content, back []model.ChapterBoundary
	for i, b := range boundaries {
		switch kinds[i] {
		case kindFront:
			front = append(front, b)
		case kindBack:
			back = append(back, b)
		default:
			content = append(content, b)
		}
	}

	// Budget: the remaining slots for content titles after reserving front/back.
	budget := maxChapters
	if len(front) > 0 {
		budget--
	}
	if len(back) > 0 {
		budget--
	}
	if budget < 1 {
		budget = 1
	}

	if len(content) > budget {
		merged := consolidateContentViaLLM(ctx, content, budget, prov, opts)
		if len(merged) > 0 {
			content = merged
		} else {
			content = sampleBoundaries(content, budget)
		}
	}

	// Stitch: [front-bucket] + content + [back-bucket]
	stitched := make([]model.ChapterBoundary, 0, len(content)+2)
	if len(front) > 0 {
		stitched = append(stitched, model.ChapterBoundary{
			StartPage: front[0].StartPage,
			Title:     "Front Matter",
		})
	}
	stitched = append(stitched, content...)
	if len(back) > 0 {
		stitched = append(stitched, model.ChapterBoundary{
			StartPage: back[0].StartPage,
			Title:     "Back Matter",
		})
	}

	return tocToChapters(stitched, totalPages)
}

// consolidateContentViaLLM asks the LLM to reduce a list of content-only chapter
// titles to a target count by merging the shortest consecutive pairs.
// Returns an empty slice on error so the caller can fall back to sampling.
func consolidateContentViaLLM(ctx context.Context, content []model.ChapterBoundary, budget int, prov provider.Provider, opts Options) []model.ChapterBoundary {
	var sb strings.Builder
	fmt.Fprintf(&sb, "A book has %d content chapters (front/back matter already handled):\n\n", len(content))
	for _, b := range content {
		fmt.Fprintf(&sb, "Page %d: %s\n", b.StartPage, b.Title)
	}
	fmt.Fprintf(&sb, `
Reduce these content chapters to exactly %d by merging consecutive chapters.
Rules:
- Merge the two shortest consecutive chapters first; repeat until count = %d.
- Never merge two substantial numbered chapters unless unavoidable.
- Preserve original chapter order.
- The merged title should describe both original chapters (e.g., "Chapters 3-4: Foundations").

Respond ONLY with a JSON array, no extra text:
[{"title":"Merged Title","start_page":1}, ...]`, budget, budget)

	start := time.Now()
	result, err := prov.Complete(ctx, sb.String())
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return nil
	}
	opts.emit(ProgressEvent{
		Type:       EventDetecting,
		Usage:      result.Usage,
		DurationMs: durationMs,
	})

	var raw []struct {
		Title     string `json:"title"`
		StartPage int    `json:"start_page"`
	}
	if err := parseJSON(result.Content, &raw); err != nil {
		return nil
	}
	merged := make([]model.ChapterBoundary, 0, len(raw))
	for _, r := range raw {
		if r.StartPage > 0 && r.Title != "" {
			merged = append(merged, model.ChapterBoundary{StartPage: r.StartPage, Title: r.Title})
		}
	}
	return merged
}

// buildBatchDetectionPrompt creates a prompt for detecting chapter starts across multiple pages.
func buildBatchDetectionPrompt(pages []model.PageContent) string {
	var sb strings.Builder
	sb.WriteString("Below are excerpts from consecutive PDF pages. Identify which pages start a new major chapter or part.\n\n")
	for _, p := range pages {
		sb.WriteString(fmt.Sprintf("--- Page %d ---\n", p.PageNumber))
		if p.HasMeaningfulText {
			preview := p.Text
			if len(preview) > 500 {
				preview = preview[:500]
			}
			sb.WriteString(preview)
		} else {
			sb.WriteString("[no extractable text]")
		}
		sb.WriteByte('\n')
	}
	sb.WriteString(`
Respond ONLY with a JSON array of chapter starts found in these pages (empty array if none):
[{"page": 12, "title": "Chapter Title"}, ...]`)
	return sb.String()
}

// parseBatchDetectionResponse parses the LLM's JSON response for batch page detection.
func parseBatchDetectionResponse(resp string) []model.ChapterBoundary {
	var raw []struct {
		Page  int    `json:"page"`
		Title string `json:"title"`
	}
	if err := parseJSON(resp, &raw); err != nil {
		return nil
	}
	result := make([]model.ChapterBoundary, 0, len(raw))
	for _, r := range raw {
		if r.Page > 0 && r.Title != "" {
			result = append(result, model.ChapterBoundary{StartPage: r.Page, Title: r.Title})
		}
	}
	return result
}

// parseChapterJSON parses a JSON chapter array from the LLM.
func parseChapterJSON(resp string, totalPages int) ([]model.Chapter, error) {
	resp = strings.TrimSpace(resp)
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")

	var raw []struct {
		Title     string `json:"title"`
		StartPage int    `json:"start_page"`
		EndPage   int    `json:"end_page"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp)), &raw); err != nil {
		return nil, err
	}

	chapters := make([]model.Chapter, 0, len(raw))
	for i, r := range raw {
		end := r.EndPage
		if end == 0 && i+1 < len(raw) {
			end = raw[i+1].StartPage - 1
		}
		if end == 0 {
			end = totalPages
		}
		chapters = append(chapters, model.Chapter{
			Index:     i + 1,
			Title:     r.Title,
			StartPage: r.StartPage,
			EndPage:   end,
		})
	}
	return chapters, nil
}

// tocToChapters converts boundary list to Chapter slice, computing end pages.
func tocToChapters(boundaries []model.ChapterBoundary, totalPages int) []model.Chapter {
	sort.Slice(boundaries, func(i, j int) bool {
		return boundaries[i].StartPage < boundaries[j].StartPage
	})

	chapters := make([]model.Chapter, len(boundaries))
	for i, b := range boundaries {
		end := totalPages
		if i+1 < len(boundaries) {
			end = boundaries[i+1].StartPage - 1
		}
		chapters[i] = model.Chapter{
			Index:     i + 1,
			Title:     b.Title,
			StartPage: b.StartPage,
			EndPage:   end,
		}
	}
	return chapters
}

// sampleBoundaries picks evenly-spaced boundaries up to n.
func sampleBoundaries(bs []model.ChapterBoundary, n int) []model.ChapterBoundary {
	if len(bs) <= n {
		return bs
	}
	step := float64(len(bs)) / float64(n)
	result := make([]model.ChapterBoundary, n)
	for i := range result {
		result[i] = bs[int(float64(i)*step)]
	}
	return result
}

// parseJSON tries to extract and parse JSON from an LLM response that may have extra text.
func parseJSON(s string, v any) error {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	for i, c := range s {
		if c == '[' || c == '{' {
			s = s[i:]
			break
		}
	}
	return json.Unmarshal([]byte(strings.TrimSpace(s)), v)
}
