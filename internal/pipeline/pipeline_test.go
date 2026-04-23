package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/desico/chapter-overview/internal/model"
	"github.com/desico/chapter-overview/internal/provider"
)

// ---------------------------------------------------------------------------
// scriptedProvider: returns queued responses in order
// ---------------------------------------------------------------------------

type scriptedProvider struct {
	responses []provider.Response
	errs      []error
	calls     int
}

func (s *scriptedProvider) Complete(_ context.Context, _ string) (provider.Response, error) {
	idx := s.calls
	s.calls++
	if idx < len(s.errs) && s.errs[idx] != nil {
		return provider.Response{}, s.errs[idx]
	}
	if idx < len(s.responses) {
		return s.responses[idx], nil
	}
	return provider.Response{}, nil
}

func (s *scriptedProvider) CompleteJSON(ctx context.Context, p string) (provider.Response, error) {
	return s.Complete(ctx, p)
}

func (s *scriptedProvider) CompleteMultimodal(ctx context.Context, p string, _ [][]byte) (provider.Response, error) {
	return s.Complete(ctx, p)
}

// ---------------------------------------------------------------------------
// parseBatchDetectionResponse
// ---------------------------------------------------------------------------

func TestParseBatchDetectionResponse_ValidJSON(t *testing.T) {
	resp := `[{"page": 5, "title": "Introduction"}, {"page": 15, "title": "Chapter 2"}]`
	boundaries := parseBatchDetectionResponse(resp)
	if len(boundaries) != 2 {
		t.Fatalf("expected 2 boundaries, got %d", len(boundaries))
	}
	if boundaries[0].StartPage != 5 || boundaries[0].Title != "Introduction" {
		t.Errorf("boundaries[0] = %+v; want {5, Introduction}", boundaries[0])
	}
	if boundaries[1].StartPage != 15 || boundaries[1].Title != "Chapter 2" {
		t.Errorf("boundaries[1] = %+v; want {15, Chapter 2}", boundaries[1])
	}
}

func TestParseBatchDetectionResponse_InvalidJSON(t *testing.T) {
	boundaries := parseBatchDetectionResponse("not json at all")
	if boundaries != nil {
		t.Errorf("expected nil for invalid JSON, got %v", boundaries)
	}
}

func TestParseBatchDetectionResponse_EmptyArray(t *testing.T) {
	boundaries := parseBatchDetectionResponse("[]")
	if len(boundaries) != 0 {
		t.Errorf("expected empty boundaries, got %v", boundaries)
	}
}

func TestParseBatchDetectionResponse_FiltersInvalidEntries(t *testing.T) {
	// page=0 or empty title should be filtered
	resp := `[{"page": 0, "title": "Bad"}, {"page": 5, "title": ""}, {"page": 10, "title": "Good"}]`
	boundaries := parseBatchDetectionResponse(resp)
	if len(boundaries) != 1 {
		t.Fatalf("expected 1 valid boundary, got %d", len(boundaries))
	}
	if boundaries[0].StartPage != 10 || boundaries[0].Title != "Good" {
		t.Errorf("unexpected boundary: %+v", boundaries[0])
	}
}

// ---------------------------------------------------------------------------
// parseChapterJSON
// ---------------------------------------------------------------------------

func TestParseChapterJSON_ValidJSON(t *testing.T) {
	resp := `[{"title":"Intro","start_page":1,"end_page":10},{"title":"Body","start_page":11,"end_page":50}]`
	chapters, err := parseChapterJSON(resp, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(chapters))
	}
	if chapters[0].Title != "Intro" || chapters[0].StartPage != 1 || chapters[0].EndPage != 10 {
		t.Errorf("chapters[0] = %+v", chapters[0])
	}
	if chapters[1].Title != "Body" || chapters[1].StartPage != 11 || chapters[1].EndPage != 50 {
		t.Errorf("chapters[1] = %+v", chapters[1])
	}
	// Index should be assigned 1-based
	if chapters[0].Index != 1 {
		t.Errorf("chapters[0].Index = %d; want 1", chapters[0].Index)
	}
}

func TestParseChapterJSON_InferredEndPage(t *testing.T) {
	// No end_page on first item; second item's start_page-1 should be used
	resp := `[{"title":"Intro","start_page":1},{"title":"Body","start_page":20}]`
	chapters, err := parseChapterJSON(resp, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chapters[0].EndPage != 19 {
		t.Errorf("chapters[0].EndPage = %d; want 19 (inferred)", chapters[0].EndPage)
	}
	// Last chapter should get totalPages
	if chapters[1].EndPage != 100 {
		t.Errorf("chapters[1].EndPage = %d; want 100 (totalPages)", chapters[1].EndPage)
	}
}

func TestParseChapterJSON_MarkdownCodeFence(t *testing.T) {
	resp := "```json\n[{\"title\":\"Ch1\",\"start_page\":1,\"end_page\":10}]\n```"
	chapters, err := parseChapterJSON(resp, 50)
	if err != nil {
		t.Fatalf("unexpected error with code fence: %v", err)
	}
	if len(chapters) != 1 {
		t.Fatalf("expected 1 chapter, got %d", len(chapters))
	}
	if chapters[0].Title != "Ch1" {
		t.Errorf("chapters[0].Title = %q; want \"Ch1\"", chapters[0].Title)
	}
}

func TestParseChapterJSON_InvalidJSON(t *testing.T) {
	_, err := parseChapterJSON("not json", 100)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// ---------------------------------------------------------------------------
// tocToChapters
// ---------------------------------------------------------------------------

func TestTocToChapters(t *testing.T) {
	boundaries := []model.ChapterBoundary{
		{StartPage: 1, Title: "Intro"},
		{StartPage: 10, Title: "Methods"},
		{StartPage: 25, Title: "Results"},
	}
	chapters := tocToChapters(boundaries, 50)

	if len(chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(chapters))
	}
	// First chapter ends at second's start-1
	if chapters[0].EndPage != 9 {
		t.Errorf("chapters[0].EndPage = %d; want 9", chapters[0].EndPage)
	}
	if chapters[1].EndPage != 24 {
		t.Errorf("chapters[1].EndPage = %d; want 24", chapters[1].EndPage)
	}
	// Last chapter gets totalPages
	if chapters[2].EndPage != 50 {
		t.Errorf("chapters[2].EndPage = %d; want 50", chapters[2].EndPage)
	}
	// Indexes should be 1-based
	for i, ch := range chapters {
		if ch.Index != i+1 {
			t.Errorf("chapters[%d].Index = %d; want %d", i, ch.Index, i+1)
		}
	}
}

func TestTocToChapters_SortsInput(t *testing.T) {
	// Input is out of order — should be sorted
	boundaries := []model.ChapterBoundary{
		{StartPage: 25, Title: "Results"},
		{StartPage: 1, Title: "Intro"},
		{StartPage: 10, Title: "Methods"},
	}
	chapters := tocToChapters(boundaries, 40)
	if chapters[0].StartPage != 1 {
		t.Errorf("first chapter should start at page 1, got %d", chapters[0].StartPage)
	}
	if chapters[1].StartPage != 10 {
		t.Errorf("second chapter should start at page 10, got %d", chapters[1].StartPage)
	}
}

func TestTocToChapters_LastChapterGetsTotal(t *testing.T) {
	boundaries := []model.ChapterBoundary{
		{StartPage: 1, Title: "Only"},
	}
	chapters := tocToChapters(boundaries, 77)
	if chapters[0].EndPage != 77 {
		t.Errorf("single chapter EndPage = %d; want 77", chapters[0].EndPage)
	}
}

// ---------------------------------------------------------------------------
// sampleBoundaries
// ---------------------------------------------------------------------------

func TestSampleBoundaries_20Items15Max(t *testing.T) {
	bs := make([]model.ChapterBoundary, 20)
	for i := range bs {
		bs[i] = model.ChapterBoundary{StartPage: i + 1, Title: "Ch"}
	}
	sampled := sampleBoundaries(bs, 15)
	if len(sampled) != 15 {
		t.Errorf("sampleBoundaries(20, 15) returned %d; want 15", len(sampled))
	}
}

func TestSampleBoundaries_FewerThanMax(t *testing.T) {
	bs := make([]model.ChapterBoundary, 5)
	sampled := sampleBoundaries(bs, 15)
	if len(sampled) != 5 {
		t.Errorf("sampleBoundaries(5, 15) returned %d; want 5 (no truncation)", len(sampled))
	}
}

func TestSampleBoundaries_ExactMax(t *testing.T) {
	bs := make([]model.ChapterBoundary, 15)
	sampled := sampleBoundaries(bs, 15)
	if len(sampled) != 15 {
		t.Errorf("sampleBoundaries(15, 15) returned %d; want 15", len(sampled))
	}
}

// ---------------------------------------------------------------------------
// parseJSON
// ---------------------------------------------------------------------------

func TestParseJSON_ValidArrayWithPrefix(t *testing.T) {
	input := "Sure, here it is: [{\"page\": 1, \"title\": \"Intro\"}]"
	var raw []struct {
		Page  int    `json:"page"`
		Title string `json:"title"`
	}
	if err := parseJSON(input, &raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(raw) != 1 || raw[0].Page != 1 || raw[0].Title != "Intro" {
		t.Errorf("unexpected result: %+v", raw)
	}
}

func TestParseJSON_ValidArrayWithSuffix(t *testing.T) {
	input := "```json\n[{\"page\": 5, \"title\": \"Ch\"}]\n```"
	var raw []struct {
		Page  int    `json:"page"`
		Title string `json:"title"`
	}
	if err := parseJSON(input, &raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(raw) != 1 || raw[0].Page != 5 {
		t.Errorf("unexpected result: %+v", raw)
	}
}

func TestParseJSON_CodeFence(t *testing.T) {
	input := "```json\n[{\"page\": 3, \"title\": \"Results\"}]\n```"
	var raw []struct {
		Page  int    `json:"page"`
		Title string `json:"title"`
	}
	if err := parseJSON(input, &raw); err != nil {
		t.Fatalf("unexpected error with code fence: %v", err)
	}
	if len(raw) != 1 || raw[0].Page != 3 {
		t.Errorf("unexpected result: %+v", raw)
	}
}

func TestParseJSON_InvalidJSON(t *testing.T) {
	var raw []struct{ Page int }
	err := parseJSON("completely invalid", &raw)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// ---------------------------------------------------------------------------
// classifyBoundaries
// ---------------------------------------------------------------------------

func TestClassifyBoundaries_Heuristic(t *testing.T) {
	bs := []model.ChapterBoundary{
		{StartPage: 1, Title: "Preface"},
		{StartPage: 3, Title: "Acknowledgements"},
		{StartPage: 5, Title: "Chapter 1: Introduction"},
		{StartPage: 20, Title: "Chapter 2: Methods"},
		{StartPage: 40, Title: "Chapter 3: Results"},
		{StartPage: 60, Title: "Bibliography"},
		{StartPage: 65, Title: "Index"},
	}
	kinds := classifyBoundaries(bs)
	want := []boundaryKind{kindFront, kindFront, kindContent, kindContent, kindContent, kindBack, kindBack}
	for i, k := range kinds {
		if k != want[i] {
			t.Errorf("kinds[%d] = %q; want %q (title=%q)", i, k, want[i], bs[i].Title)
		}
	}
}

func TestClassifyBoundaries_PromotesFrontMatterAfterFirstChapter(t *testing.T) {
	// "Dedication" after the first numbered chapter should be treated as content,
	// not as front matter (unusual placement).
	bs := []model.ChapterBoundary{
		{StartPage: 1, Title: "Chapter 1"},
		{StartPage: 10, Title: "Dedication"}, // late → content
	}
	kinds := classifyBoundaries(bs)
	if kinds[1] != kindContent {
		t.Errorf("late 'Dedication' kind = %q; want %q", kinds[1], kindContent)
	}
}

func TestClassifyBoundaries_BackMatterBeforeLastChapterIsContent(t *testing.T) {
	// "Appendix" appearing *before* the last numbered chapter is content, not back matter.
	bs := []model.ChapterBoundary{
		{StartPage: 1, Title: "Chapter 1"},
		{StartPage: 10, Title: "Appendix"}, // mid-book → content
		{StartPage: 20, Title: "Chapter 2"},
	}
	kinds := classifyBoundaries(bs)
	if kinds[1] != kindContent {
		t.Errorf("mid-book 'Appendix' kind = %q; want %q", kinds[1], kindContent)
	}
}

// ---------------------------------------------------------------------------
// finalizeStructure
// ---------------------------------------------------------------------------

func TestFinalizeStructure_CollapsesFrontAndBack(t *testing.T) {
	// 2 front + 3 content + 2 back; no budget pressure (content fits within budget).
	bs := []model.ChapterBoundary{
		{StartPage: 1, Title: "Preface"},
		{StartPage: 3, Title: "Acknowledgements"},
		{StartPage: 5, Title: "Chapter 1"},
		{StartPage: 20, Title: "Chapter 2"},
		{StartPage: 40, Title: "Chapter 3"},
		{StartPage: 60, Title: "Bibliography"},
		{StartPage: 65, Title: "Index"},
	}
	// Provider shouldn't be called when content count ≤ budget.
	prov := &scriptedProvider{}
	chapters := finalizeStructure(context.Background(), bs, 80, prov, Options{})

	if len(chapters) != 5 { // 1 front + 3 content + 1 back
		t.Fatalf("got %d chapters; want 5", len(chapters))
	}
	if chapters[0].Title != "Front Matter" {
		t.Errorf("chapters[0].Title = %q; want Front Matter", chapters[0].Title)
	}
	if chapters[0].StartPage != 1 {
		t.Errorf("Front Matter StartPage = %d; want 1", chapters[0].StartPage)
	}
	if chapters[len(chapters)-1].Title != "Back Matter" {
		t.Errorf("last chapter title = %q; want Back Matter", chapters[len(chapters)-1].Title)
	}
	// Indexes 1-based & contiguous.
	for i, ch := range chapters {
		if ch.Index != i+1 {
			t.Errorf("chapters[%d].Index = %d; want %d", i, ch.Index, i+1)
		}
	}
	if prov.calls != 0 {
		t.Errorf("provider calls = %d; want 0 when budget not exceeded", prov.calls)
	}
}

func TestFinalizeStructure_BudgetMath(t *testing.T) {
	// 2 front + 18 content + 3 back → budget = 15 - 2 (front+back) = 13 content.
	bs := []model.ChapterBoundary{
		{StartPage: 1, Title: "Preface"},
		{StartPage: 3, Title: "Foreword"},
	}
	for i := 1; i <= 18; i++ {
		bs = append(bs, model.ChapterBoundary{StartPage: 10 + i*5, Title: "Chapter X"})
	}
	bs = append(bs,
		model.ChapterBoundary{StartPage: 200, Title: "Appendix"},
		model.ChapterBoundary{StartPage: 210, Title: "Bibliography"},
		model.ChapterBoundary{StartPage: 220, Title: "Index"},
	)

	// Scripted LLM returns 13 merged content boundaries.
	mergedJSON := `[`
	for i := 0; i < 13; i++ {
		if i > 0 {
			mergedJSON += ","
		}
		mergedJSON += `{"title":"Merged","start_page":` + itoa(15+i*10) + `}`
	}
	mergedJSON += `]`

	prov := &scriptedProvider{
		responses: []provider.Response{{Content: mergedJSON}},
	}
	chapters := finalizeStructure(context.Background(), bs, 230, prov, Options{})

	if len(chapters) != 15 {
		t.Fatalf("got %d chapters; want 15 (1 front + 13 content + 1 back)", len(chapters))
	}
	if chapters[0].Title != "Front Matter" {
		t.Errorf("first = %q; want Front Matter", chapters[0].Title)
	}
	if chapters[14].Title != "Back Matter" {
		t.Errorf("last = %q; want Back Matter", chapters[14].Title)
	}
	if prov.calls != 1 {
		t.Errorf("provider calls = %d; want 1 (consolidate)", prov.calls)
	}
}

func TestFinalizeStructure_EmptyBoundaries(t *testing.T) {
	prov := &scriptedProvider{}
	chapters := finalizeStructure(context.Background(), nil, 100, prov, Options{})
	if len(chapters) != 1 {
		t.Fatalf("got %d chapters; want 1 (fallback)", len(chapters))
	}
	if chapters[0].EndPage != 100 {
		t.Errorf("fallback EndPage = %d; want 100", chapters[0].EndPage)
	}
}

func TestFinalizeStructure_StripsSubsections(t *testing.T) {
	// Numbered subsections should be dropped so parent chapter page ranges cover them.
	bs := []model.ChapterBoundary{
		{StartPage: 1, Title: "Chapter 1: Intro"},
		{StartPage: 3, Title: "1.1 Background"},
		{StartPage: 5, Title: "1.2 Motivation"},
		{StartPage: 10, Title: "Chapter 2: Methods"},
		{StartPage: 12, Title: "2.1 Setup"},
		{StartPage: 20, Title: "Chapter 3: Results"},
	}
	chapters := finalizeStructure(context.Background(), bs, 30, &scriptedProvider{}, Options{})
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters; want 3 (subsections stripped)", len(chapters))
	}
	if chapters[0].EndPage != 9 {
		t.Errorf("Chapter 1 EndPage = %d; want 9 (covers 1.1, 1.2)", chapters[0].EndPage)
	}
}

// ---------------------------------------------------------------------------
// classifyWithFallback: non-English uses LLM
// ---------------------------------------------------------------------------

func TestClassifyWithFallback_LLMUsedWhenHeuristicEmpty(t *testing.T) {
	// Chinese titles — heuristic matches nothing → should call LLM.
	bs := []model.ChapterBoundary{
		{StartPage: 1, Title: "序言"},       // preface
		{StartPage: 5, Title: "第一章 引言"},   // Chapter 1: Introduction
		{StartPage: 20, Title: "第二章 方法"},  // Chapter 2: Methods
		{StartPage: 40, Title: "第三章 结果"},  // Chapter 3: Results
		{StartPage: 60, Title: "第四章 讨论"},  // Chapter 4: Discussion
		{StartPage: 80, Title: "第五章 总结"},  // Chapter 5: Conclusion
		{StartPage: 100, Title: "第六章 附录"}, // Chapter 6: Appendix
		{StartPage: 120, Title: "第七章 X"},
		{StartPage: 140, Title: "第八章 Y"},
		{StartPage: 160, Title: "第九章 Z"},
		{StartPage: 180, Title: "参考文献"}, // references
		{StartPage: 190, Title: "索引"},   // index
	}
	llmResp := `[
		{"kind":"front"},
		{"kind":"content"},{"kind":"content"},{"kind":"content"},{"kind":"content"},{"kind":"content"},{"kind":"content"},{"kind":"content"},{"kind":"content"},{"kind":"content"},
		{"kind":"back"},{"kind":"back"}
	]`
	prov := &scriptedProvider{responses: []provider.Response{{Content: llmResp}}}
	kinds := classifyWithFallback(context.Background(), bs, prov, Options{})

	if prov.calls != 1 {
		t.Errorf("provider calls = %d; want 1 (LLM fallback)", prov.calls)
	}
	if kinds[0] != kindFront {
		t.Errorf("kinds[0] = %q; want front", kinds[0])
	}
	if kinds[len(kinds)-1] != kindBack {
		t.Errorf("last kind = %q; want back", kinds[len(kinds)-1])
	}
}

func TestClassifyWithFallback_SkipLLMWhenHeuristicHits(t *testing.T) {
	// English titles — heuristic finds front and back → no LLM call.
	bs := []model.ChapterBoundary{
		{StartPage: 1, Title: "Preface"},
		{StartPage: 5, Title: "Chapter 1"},
		{StartPage: 20, Title: "Chapter 2"},
		{StartPage: 40, Title: "Chapter 3"},
		{StartPage: 60, Title: "Chapter 4"},
		{StartPage: 80, Title: "Chapter 5"},
		{StartPage: 100, Title: "Chapter 6"},
		{StartPage: 120, Title: "Chapter 7"},
		{StartPage: 140, Title: "Chapter 8"},
		{StartPage: 160, Title: "Chapter 9"},
		{StartPage: 180, Title: "Index"},
	}
	prov := &scriptedProvider{}
	_ = classifyWithFallback(context.Background(), bs, prov, Options{})
	if prov.calls != 0 {
		t.Errorf("provider calls = %d; want 0 (heuristic sufficient)", prov.calls)
	}
}

func TestClassifyWithFallback_SkipLLMWhenListShort(t *testing.T) {
	// Short non-English list — skip LLM (not worth the call).
	bs := []model.ChapterBoundary{
		{StartPage: 1, Title: "第一章"},
		{StartPage: 20, Title: "第二章"},
		{StartPage: 40, Title: "第三章"},
	}
	prov := &scriptedProvider{}
	_ = classifyWithFallback(context.Background(), bs, prov, Options{})
	if prov.calls != 0 {
		t.Errorf("provider calls = %d; want 0 (short list)", prov.calls)
	}
}

// itoa: small helper to avoid importing strconv inside test-only builder.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

// ---------------------------------------------------------------------------
// looksLikeRefusal
// ---------------------------------------------------------------------------

func TestLooksLikeRefusal(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"I cannot access this document, please provide the content.", true},
		{"I'm sorry, but I am unable to summarize.", true},
		{"As an AI, I cannot...", true},
		{"short", true},
		{"This chapter introduces the algorithm and describes its complexity on input graphs of size N. The authors present two variants and prove correctness.", false},
	}
	for _, c := range cases {
		got := looksLikeRefusal(c.s)
		if got != c.want {
			t.Errorf("looksLikeRefusal(%q) = %v; want %v", c.s, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// buildFallbackSummary
// ---------------------------------------------------------------------------

func TestBuildFallbackSummary_WithText(t *testing.T) {
	text := "This is the extracted text of a chapter. It contains multiple sentences that discuss various topics in depth. The content is long enough to be truncated beyond 150 chars total, making sure the word-boundary trim kicks in properly here."
	summary, status := buildFallbackSummary(text)
	if status != model.SummaryFallback {
		t.Errorf("status = %q; want %q", status, model.SummaryFallback)
	}
	if !strings.Contains(summary, fallbackWarning) {
		t.Error("summary missing warning prefix")
	}
	if !strings.HasSuffix(summary, "...") {
		t.Errorf("truncated summary should end with ...; got tail %q", summary[len(summary)-10:])
	}
}

func TestBuildFallbackSummary_EmptyText(t *testing.T) {
	summary, status := buildFallbackSummary("   ")
	if status != model.SummaryFailed {
		t.Errorf("status = %q; want %q", status, model.SummaryFailed)
	}
	if summary != fallbackWarning {
		t.Errorf("summary = %q; want warning only", summary)
	}
}

func TestBuildFallbackSummary_GarbledPUA(t *testing.T) {
	// Simulate ledongthuc/pdf private-use-area output (unmapped CID glyphs).
	garbled := ""
	summary, status := buildFallbackSummary(garbled)
	if status != model.SummaryFailed {
		t.Errorf("status = %q; want %q", status, model.SummaryFailed)
	}
	if summary != fallbackGarbledWarning {
		t.Errorf("expected garbled warning; got %q", summary)
	}
}

func TestBuildFallbackSummary_GarbledMixed(t *testing.T) {
	// A few ASCII chars drowned in PUA noise → ratio below threshold.
	garbled := "ab" + strings.Repeat("", 20)
	summary, status := buildFallbackSummary(garbled)
	if status != model.SummaryFailed {
		t.Errorf("status = %q; want %q", status, model.SummaryFailed)
	}
	if strings.Contains(summary, "") {
		t.Error("garbled runes must not appear in fallback output")
	}
}

func TestBuildFallbackSummary_CleanCJK(t *testing.T) {
	// Readable Chinese text — should produce SummaryFallback with excerpt.
	cjk := "本章节主要介绍了税务管理的基本原则和操作流程，包括申报要求和缴纳期限的详细说明。"
	summary, status := buildFallbackSummary(cjk)
	if status != model.SummaryFallback {
		t.Errorf("status = %q; want %q", status, model.SummaryFallback)
	}
	if !strings.Contains(summary, "本章") {
		t.Error("CJK excerpt should appear in fallback summary")
	}
}

func TestBuildFallbackSummary_ControlCharsStripped(t *testing.T) {
	// Control chars mixed with readable ASCII.
	mixed := "Good text\x01\x02\x03 more good text here to exceed the threshold clearly."
	summary, status := buildFallbackSummary(mixed)
	if status != model.SummaryFallback {
		t.Errorf("status = %q; want %q for readable text", status, model.SummaryFallback)
	}
	if strings.ContainsAny(summary, "\x01\x02\x03") {
		t.Error("control chars must be stripped from fallback output")
	}
}

// ---------------------------------------------------------------------------
// tryLLMSummary: refusal JSON + heuristic
// ---------------------------------------------------------------------------

func TestTryLLMSummary_RefusalJSON(t *testing.T) {
	resp := provider.Response{
		Content: `{"summarized": false, "summary": "", "refusal_reason": "cannot access"}`,
	}
	prov := &scriptedProvider{responses: []provider.Response{resp}}
	_, _, _, ok := tryLLMSummary(context.Background(), prov, "p", nil, false)
	if ok {
		t.Error("expected ok=false for refusal JSON")
	}
}

func TestTryLLMSummary_HeuristicCatchesRefusal(t *testing.T) {
	// JSON says summarized=true but content is clearly a refusal.
	resp := provider.Response{
		Content: `{"summarized": true, "summary": "I cannot access the document, please provide it.", "refusal_reason": ""}`,
	}
	prov := &scriptedProvider{responses: []provider.Response{resp}}
	_, _, _, ok := tryLLMSummary(context.Background(), prov, "p", nil, false)
	if ok {
		t.Error("expected ok=false when heuristic catches refusal phrasing")
	}
}

func TestTryLLMSummary_ValidSummary(t *testing.T) {
	good := "This chapter introduces the algorithm and describes its complexity on input graphs of size N. The authors present two variants and prove correctness."
	resp := provider.Response{
		Content: `{"summarized": true, "summary": "` + good + `", "refusal_reason": ""}`,
	}
	prov := &scriptedProvider{responses: []provider.Response{resp}}
	out, _, _, ok := tryLLMSummary(context.Background(), prov, "p", nil, false)
	if !ok {
		t.Fatal("expected ok=true for valid summary")
	}
	if out != good {
		t.Errorf("summary = %q; want %q", out, good)
	}
}
