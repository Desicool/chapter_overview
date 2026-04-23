package pipeline

import (
	"testing"

	"github.com/desico/chapter-overview/internal/model"
)

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
