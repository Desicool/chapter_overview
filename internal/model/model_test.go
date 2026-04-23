package model

import (
	"testing"
)

func TestComputeStats_Empty(t *testing.T) {
	s := ComputeStats(nil, 10)
	if s.TotalInputTokens != 0 {
		t.Errorf("TotalInputTokens = %d; want 0", s.TotalInputTokens)
	}
	if s.TotalOutputTokens != 0 {
		t.Errorf("TotalOutputTokens = %d; want 0", s.TotalOutputTokens)
	}
	if s.AvgTokensPerPage != 0 {
		t.Errorf("AvgTokensPerPage = %f; want 0", s.AvgTokensPerPage)
	}
	if s.MaxTokensPerCall != 0 {
		t.Errorf("MaxTokensPerCall = %d; want 0", s.MaxTokensPerCall)
	}
	if s.TotalDurationMs != 0 {
		t.Errorf("TotalDurationMs = %d; want 0", s.TotalDurationMs)
	}
	if s.P90DurationMs != 0 {
		t.Errorf("P90DurationMs = %d; want 0", s.P90DurationMs)
	}
	if s.P99DurationMs != 0 {
		t.Errorf("P99DurationMs = %d; want 0", s.P99DurationMs)
	}
	if len(s.Records) != 0 {
		t.Errorf("Records len = %d; want 0", len(s.Records))
	}
}

func TestComputeStats_SingleRecord(t *testing.T) {
	records := []LLMRecord{
		{
			Phase:        "detect",
			ChapterIndex: -1,
			InputTokens:  100,
			OutputTokens: 50,
			DurationMs:   200,
		},
	}
	s := ComputeStats(records, 10)

	if s.TotalInputTokens != 100 {
		t.Errorf("TotalInputTokens = %d; want 100", s.TotalInputTokens)
	}
	if s.TotalOutputTokens != 50 {
		t.Errorf("TotalOutputTokens = %d; want 50", s.TotalOutputTokens)
	}
	// AvgTokensPerPage = (100+50)/10 = 15
	if s.AvgTokensPerPage != 15.0 {
		t.Errorf("AvgTokensPerPage = %f; want 15.0", s.AvgTokensPerPage)
	}
	// MaxTokensPerCall = 100+50 = 150
	if s.MaxTokensPerCall != 150 {
		t.Errorf("MaxTokensPerCall = %d; want 150", s.MaxTokensPerCall)
	}
	if s.TotalDurationMs != 200 {
		t.Errorf("TotalDurationMs = %d; want 200", s.TotalDurationMs)
	}
	// Single record: floor(0.90*1)=0, floor(0.99*1)=0 → sorted[0] = 200
	if s.P90DurationMs != 200 {
		t.Errorf("P90DurationMs = %d; want 200", s.P90DurationMs)
	}
	if s.P99DurationMs != 200 {
		t.Errorf("P99DurationMs = %d; want 200", s.P99DurationMs)
	}
}

func TestComputeStats_MultipleRecords(t *testing.T) {
	// 10 records with DurationMs 10, 20, ..., 100
	records := make([]LLMRecord, 10)
	for i := 0; i < 10; i++ {
		records[i] = LLMRecord{
			Phase:        "detect",
			ChapterIndex: -1,
			InputTokens:  50,
			OutputTokens: 25,
			DurationMs:   int64((i + 1) * 10),
		}
	}

	s := ComputeStats(records, 100)

	// TotalInputTokens = 10 * 50 = 500
	if s.TotalInputTokens != 500 {
		t.Errorf("TotalInputTokens = %d; want 500", s.TotalInputTokens)
	}
	// TotalOutputTokens = 10 * 25 = 250
	if s.TotalOutputTokens != 250 {
		t.Errorf("TotalOutputTokens = %d; want 250", s.TotalOutputTokens)
	}
	// AvgTokensPerPage = (500+250)/100 = 7.5
	if s.AvgTokensPerPage != 7.5 {
		t.Errorf("AvgTokensPerPage = %f; want 7.5", s.AvgTokensPerPage)
	}
	// MaxTokensPerCall = 50+25 = 75 (same for all records)
	if s.MaxTokensPerCall != 75 {
		t.Errorf("MaxTokensPerCall = %d; want 75", s.MaxTokensPerCall)
	}
	// TotalDurationMs = 10+20+...+100 = 550
	if s.TotalDurationMs != 550 {
		t.Errorf("TotalDurationMs = %d; want 550", s.TotalDurationMs)
	}
	// P90: floor(0.90 * 10) = 9 → sorted[9] = 100
	if s.P90DurationMs != 100 {
		t.Errorf("P90DurationMs = %d; want 100", s.P90DurationMs)
	}
	// P99: floor(0.99 * 10) = 9 → sorted[9] = 100
	if s.P99DurationMs != 100 {
		t.Errorf("P99DurationMs = %d; want 100", s.P99DurationMs)
	}
	// Records should be copied
	if len(s.Records) != 10 {
		t.Errorf("Records len = %d; want 10", len(s.Records))
	}
}

func TestComputeStats_ZeroTotalPages(t *testing.T) {
	records := []LLMRecord{
		{InputTokens: 100, OutputTokens: 50, DurationMs: 100},
	}
	s := ComputeStats(records, 0)
	if s.AvgTokensPerPage != 0 {
		t.Errorf("AvgTokensPerPage with totalPages=0 = %f; want 0", s.AvgTokensPerPage)
	}
}
