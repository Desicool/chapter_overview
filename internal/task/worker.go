package task

import (
	"context"
	"fmt"
	"time"

	"github.com/desico/chapter-overview/internal/metrics"
	"github.com/desico/chapter-overview/internal/model"
	"github.com/desico/chapter-overview/internal/pdf"
	"github.com/desico/chapter-overview/internal/pipeline"
	"github.com/desico/chapter-overview/internal/provider"
)

// Worker runs pipeline tasks in background goroutines with a concurrency cap.
type Worker struct {
	store   Store
	hub     *Hub
	prov    provider.Provider
	opts    pipeline.Options
	metrx   *metrics.Registry
	sem     chan struct{}
	dataDir string
}

// NewWorker creates a Worker. maxConcurrent caps simultaneous pipeline runs.
func NewWorker(store Store, hub *Hub, prov provider.Provider, maxConcurrent int, metricsReg *metrics.Registry, dataDir string) *Worker {
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}
	return &Worker{
		store:   store,
		hub:     hub,
		prov:    prov,
		opts:    pipeline.Options{MaxConcurrent: maxConcurrent},
		metrx:   metricsReg,
		sem:     make(chan struct{}, maxConcurrent),
		dataDir: dataDir,
	}
}

// Submit enqueues a task for background processing.
// Returns ErrTooManyTasks immediately if the concurrency cap is reached.
func (w *Worker) Submit(task *Task) error {
	select {
	case w.sem <- struct{}{}:
		go w.run(task)
		return nil
	default:
		return ErrTooManyTasks
	}
}

func (w *Worker) run(task *Task) {
	defer func() { <-w.sem }()

	taskStart := time.Now()
	ctx := context.Background()

	if w.metrx != nil {
		w.metrx.ActiveTasks.Inc()
		defer w.metrx.ActiveTasks.Dec()
	}

	// Resolve total page count before any status updates
	if n, err := pdf.GetPageCount(task.PDFPath); err == nil {
		task.TotalPages = n
		w.update(task.ID, func(t *Task) { t.TotalPages = n })
	}

	// Mark as detecting
	w.update(task.ID, func(t *Task) {
		t.Status = StatusDetecting
		t.Message = "Detecting chapters..."
	})
	if updated, err := w.store.Get(task.ID); err == nil {
		w.publish(task.ID, "progress", updated)
	}

	// Accumulate LLM records from every progress event
	var records []model.LLMRecord

	pipelineOpts := pipeline.Options{
		MaxConcurrent: w.opts.MaxConcurrent,
		OnProgress: func(e pipeline.ProgressEvent) {
			// Accumulate LLM records for metrics
			if e.Usage.InputTokens > 0 || e.Usage.OutputTokens > 0 {
				phase := "detect"
				if e.Type == pipeline.EventChapterDone || e.Type == pipeline.EventSummarizing {
					phase = "summarize"
				}
				records = append(records, model.LLMRecord{
					Phase:        phase,
					ChapterIndex: e.ChapterIndex,
					BatchStart:   e.BatchStart,
					BatchEnd:     e.BatchEnd,
					InputTokens:  e.Usage.InputTokens,
					OutputTokens: e.Usage.OutputTokens,
					DurationMs:   e.DurationMs,
				})
				if w.metrx != nil {
					w.metrx.LLMCallDurationMs.Observe(float64(e.DurationMs))
				}
			}
			// chapter_done: update summary in store + publish just the chapter
			if e.Type == pipeline.EventChapterDone && e.Chapter != nil {
				w.update(task.ID, func(t *Task) {
					for i := range t.Chapters {
						if t.Chapters[i].Index == e.ChapterIndex {
							t.Chapters[i].Summary = e.Chapter.Summary
						}
					}
				})
				w.publish(task.ID, "chapter_done", e.Chapter)
			}
		},
	}

	// Phase 1: detect chapters
	chapters, err := pipeline.DetectChapters(ctx, task.PDFPath, w.prov, pipelineOpts)
	if err != nil {
		w.fail(task, fmt.Errorf("chapter detection: %w", err))
		return
	}

	// Store detected chapters + transition to summarizing
	w.update(task.ID, func(t *Task) {
		t.Chapters = chapters
		t.Status = StatusSummarizing
		t.Message = fmt.Sprintf("Summarizing %d chapters...", len(chapters))
	})
	// Emit each chapter individually so UI renders them before summaries arrive
	for _, ch := range chapters {
		chCopy := ch
		w.publish(task.ID, "chapter_detected", &chCopy)
	}
	// Read back updated task for progress event
	if updatedTask, err := w.store.Get(task.ID); err == nil {
		w.publish(task.ID, "progress", updatedTask)
	}

	summarized, err := pipeline.SummarizeChapters(ctx, task.PDFPath, chapters, w.prov, pipelineOpts)
	if err != nil {
		// Non-fatal: partial results with warning message
		w.update(task.ID, func(t *Task) { t.Message = "Summarization had errors: " + err.Error() })
	}

	stats := model.ComputeStats(records, task.TotalPages)

	w.update(task.ID, func(t *Task) {
		t.Status = StatusDone
		t.Progress = 1.0
		t.Message = "Done"
		t.Chapters = summarized
		t.Stats = &stats
	})

	// Read back the final task state for the done event
	finalTask, _ := w.store.Get(task.ID)
	w.publish(task.ID, "done", finalTask)
	w.hub.Close(task.ID)

	// Update Prometheus
	if w.metrx != nil {
		w.metrx.TasksTotal.WithLabelValues("done").Inc()
		w.metrx.TokensTotal.WithLabelValues("input").Add(float64(stats.TotalInputTokens))
		w.metrx.TokensTotal.WithLabelValues("output").Add(float64(stats.TotalOutputTokens))
		w.metrx.PagesProcessedTotal.Add(float64(task.TotalPages))
		w.metrx.TaskDurationSeconds.Observe(time.Since(taskStart).Seconds())
	}
}

func (w *Worker) fail(task *Task, err error) {
	w.update(task.ID, func(t *Task) {
		t.Status = StatusFailed
		t.Error = err.Error()
	})
	failedTask, _ := w.store.Get(task.ID)
	w.publish(task.ID, "error", failedTask)
	w.hub.Close(task.ID)
	if w.metrx != nil {
		w.metrx.TasksTotal.WithLabelValues("failed").Inc()
	}
}

func (w *Worker) update(id string, fn func(*Task)) {
	_ = w.store.Update(id, fn)
}

func (w *Worker) publish(taskID string, eventType string, data any) {
	w.hub.Publish(taskID, SSEEvent{Type: eventType, Data: data})
}
