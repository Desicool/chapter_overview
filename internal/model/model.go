package model

// PageContent holds the extracted content of a single PDF page.
type PageContent struct {
	PageNumber        int    // 1-indexed
	Text              string // extracted text (empty for scanned pages)
	Images            [][]byte
	HasMeaningfulText bool
}

// ChapterBoundary marks the start of a chapter.
type ChapterBoundary struct {
	StartPage int
	Title     string
}

// SummaryStatus describes the quality/origin of a chapter summary.
type SummaryStatus string

const (
	SummaryOK       SummaryStatus = "ok"       // LLM produced a valid summary
	SummaryFallback SummaryStatus = "fallback" // LLM refused/failed; summary contains warning + raw text
	SummaryFailed   SummaryStatus = "failed"   // LLM refused AND text extraction yielded nothing
	SummaryPending  SummaryStatus = ""         // not yet summarized
)

// Chapter is a detected chapter with its summary.
type Chapter struct {
	Index         int           `json:"index"`
	Title         string        `json:"title"`
	StartPage     int           `json:"start_page"`
	EndPage       int           `json:"end_page"`
	Summary       string        `json:"summary"`
	SummaryStatus SummaryStatus `json:"summary_status,omitempty"`
}

// Result is the final JSON output.
type Result struct {
	Source     string    `json:"source"`
	TotalPages int       `json:"total_pages"`
	Chapters   []Chapter `json:"chapters"`
}

// LLMRecord captures token usage and timing for one LLM API call.
type LLMRecord struct {
	Phase        string `json:"phase"`
	ChapterIndex int    `json:"chapter_index"`
	BatchStart   int    `json:"batch_start"`
	BatchEnd     int    `json:"batch_end"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	DurationMs   int64  `json:"duration_ms"`
}

// Stats aggregates LLM usage metrics across a task.
type Stats struct {
	Records           []LLMRecord `json:"records"`
	TotalInputTokens  int         `json:"total_input_tokens"`
	TotalOutputTokens int         `json:"total_output_tokens"`
	AvgTokensPerPage  float64     `json:"avg_tokens_per_page"`
	MaxTokensPerCall  int         `json:"max_tokens_per_call"`
	TotalDurationMs   int64       `json:"total_duration_ms"` // sum of all LLM call durations (inflated by parallelism)
	P90DurationMs     int64       `json:"p90_duration_ms"`
	P99DurationMs     int64       `json:"p99_duration_ms"`
	ElapsedMs         int64       `json:"elapsed_ms"` // wall-clock time for the full task
}

// ComputeStats derives Stats from raw LLM records.
func ComputeStats(records []LLMRecord, totalPages int) Stats {
	s := Stats{
		Records: make([]LLMRecord, len(records)),
	}
	copy(s.Records, records)

	if len(records) == 0 {
		return s
	}

	durations := make([]int64, 0, len(records))
	for _, r := range records {
		s.TotalInputTokens += r.InputTokens
		s.TotalOutputTokens += r.OutputTokens
		s.TotalDurationMs += r.DurationMs
		total := r.InputTokens + r.OutputTokens
		if total > s.MaxTokensPerCall {
			s.MaxTokensPerCall = total
		}
		durations = append(durations, r.DurationMs)
	}

	if totalPages > 0 {
		s.AvgTokensPerPage = float64(s.TotalInputTokens+s.TotalOutputTokens) / float64(totalPages)
	}

	// Sort durations for percentile calculation
	sortInt64(durations)
	n := len(durations)
	s.P90DurationMs = durations[int(float64(n)*0.90)]
	s.P99DurationMs = durations[int(float64(n)*0.99)]

	return s
}

// sortInt64 sorts a slice of int64 in ascending order.
func sortInt64(s []int64) {
	// Simple insertion sort — records are small in practice
	for i := 1; i < len(s); i++ {
		key := s[i]
		j := i - 1
		for j >= 0 && s[j] > key {
			s[j+1] = s[j]
			j--
		}
		s[j+1] = key
	}
}
