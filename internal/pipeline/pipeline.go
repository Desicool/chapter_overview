package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

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
			summary, err := summarizeChapter(ctx, pdfPath, *ch, prov, opts)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("chapter %d %q: %w", ch.Index, ch.Title, err)
				}
				ch.Summary = "[summarization failed]"
				return
			}
			ch.Summary = summary
		}(i)
	}
	wg.Wait()

	if firstErr != nil {
		fmt.Printf("[warn] summarization had errors: %v\n", firstErr)
	}
	return chapters, nil
}

// validateTOC sends the TOC to the LLM to validate, correct, and merge to ≤15 chapters.
func validateTOC(ctx context.Context, _ string, toc []model.ChapterBoundary, totalPages int, prov provider.Provider, opts Options) ([]model.Chapter, error) {
	var sb strings.Builder
	sb.WriteString("Here is the table of contents extracted from a PDF:\n\n")
	for _, b := range toc {
		sb.WriteString(fmt.Sprintf("Page %d: %s\n", b.StartPage, b.Title))
	}
	sb.WriteString(fmt.Sprintf("\nTotal pages: %d\n", totalPages))
	sb.WriteString(fmt.Sprintf(`
Your task: validate and clean up these TOC entries into logical chapters.
Rules:
1. Merge only numbered sub-sections (e.g. "1.1 Introduction", "2.3 Examples") into their parent chapter.
2. If there are still more than %d chapters, reduce by merging in this priority order:
   a. First: merge front matter (preface, acknowledgements, title pages) into the first content chapter.
   b. Second: merge back matter (index, bibliography, appendix entries) together.
   c. Last resort only: merge the two shortest consecutive numbered chapters.
   Never merge two substantial numbered content chapters unless unavoidable.
3. Every chapter must have a start_page and end_page.
4. The last chapter's end_page should be the total page count.

Respond ONLY with a JSON array, no extra text:
[{"title":"Chapter Title","start_page":1,"end_page":50}, ...]`, maxChapters))

	start := time.Now()
	result, err := prov.Complete(ctx, sb.String())
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("LLM TOC validation: %w", err)
	}

	opts.emit(ProgressEvent{
		Type:       EventDetecting,
		Usage:      result.Usage,
		DurationMs: durationMs,
	})

	chapters, err := parseChapterJSON(result.Content, totalPages)
	if err != nil {
		return tocToChapters(toc, totalPages), nil
	}
	return chapters, nil
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

	if len(boundaries) > maxChapters {
		boundaries, _ = mergeChaptersViaLLM(ctx, boundaries, totalPages, prov)
	}

	return tocToChapters(boundaries, totalPages), nil
}

// summarizeChapter sends all pages of a chapter to the LLM for summarization.
func summarizeChapter(ctx context.Context, pdfPath string, ch model.Chapter, prov provider.Provider, opts Options) (string, error) {
	pages, err := opts.loadPages(pdfPath, ch.StartPage, ch.EndPage)
	if err != nil {
		return "", err
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

	prompt := fmt.Sprintf(
		"Summarize the following chapter from a PDF document.\n\nChapter: %s (pages %d–%d)\n\n%s\n\nProvide a concise, informative summary in 3–5 sentences.",
		ch.Title, ch.StartPage, ch.EndPage, fullText,
	)

	var llmResult provider.Response
	start := time.Now()
	if hasImages && len(allImages) > 0 {
		if len(allImages) > 5 {
			allImages = allImages[:5]
		}
		llmResult, err = prov.CompleteMultimodal(ctx, prompt, allImages)
	} else {
		llmResult, err = prov.Complete(ctx, prompt)
	}
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", err
	}

	opts.emit(ProgressEvent{
		Type:         EventChapterDone,
		ChapterIndex: ch.Index,
		Chapter: &model.Chapter{
			Index:     ch.Index,
			Title:     ch.Title,
			StartPage: ch.StartPage,
			EndPage:   ch.EndPage,
			Summary:   llmResult.Content,
		},
		Usage:      llmResult.Usage,
		DurationMs: durationMs,
	})

	return llmResult.Content, nil
}

// mergeChaptersViaLLM asks the LLM to merge a long list of chapters into ≤15.
func mergeChaptersViaLLM(ctx context.Context, boundaries []model.ChapterBoundary, totalPages int, prov provider.Provider) ([]model.ChapterBoundary, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("A PDF with %d pages has %d detected chapter starts:\n\n", totalPages, len(boundaries)))
	for _, b := range boundaries {
		sb.WriteString(fmt.Sprintf("Page %d: %s\n", b.StartPage, b.Title))
	}
	sb.WriteString(fmt.Sprintf(`
Reduce these chapter starts to at most %d by merging in priority order:
1. Merge front matter (preface, acknowledgements, title pages) into the first content chapter.
2. Merge back matter (index, bibliography, appendix entries) together.
3. Last resort: merge the two shortest consecutive chapters.
Never merge two substantial numbered content chapters unless unavoidable.
Respond ONLY with a JSON array:
[{"title":"Group Title","start_page":1}, ...]`, maxChapters))

	result, err := prov.Complete(ctx, sb.String())
	if err != nil {
		return sampleBoundaries(boundaries, maxChapters), nil
	}

	var raw []struct {
		Title     string `json:"title"`
		StartPage int    `json:"start_page"`
	}
	if err := parseJSON(result.Content, &raw); err != nil {
		return sampleBoundaries(boundaries, maxChapters), nil
	}

	merged := make([]model.ChapterBoundary, 0, len(raw))
	for _, r := range raw {
		merged = append(merged, model.ChapterBoundary{StartPage: r.StartPage, Title: r.Title})
	}
	return merged, nil
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
