package task

import (
	"errors"
	"time"

	"github.com/desico/chapter-overview/internal/model"
)

// Status represents the processing state of a task.
type Status string

const (
	StatusPending     Status = "pending"
	StatusDetecting   Status = "detecting"
	StatusSummarizing Status = "summarizing"
	StatusDone        Status = "done"
	StatusFailed      Status = "failed"
)

var (
	// ErrTooManyTasks is returned when the worker semaphore is full.
	ErrTooManyTasks = errors.New("too many concurrent tasks")
	// ErrNotFound is returned when a task ID does not exist in the store.
	ErrNotFound = errors.New("task not found")
)

// Task represents a PDF processing job.
type Task struct {
	ID         string          `json:"id"`
	Status     Status          `json:"status"`
	Progress   float64         `json:"progress"`
	Message    string          `json:"message"`
	PDFName    string          `json:"pdf_name"`
	PDFPath    string          `json:"-"`
	TotalPages int             `json:"total_pages"`
	Chapters   []model.Chapter `json:"chapters,omitempty"`
	Stats      *model.Stats    `json:"metrics,omitempty"`
	Error      string          `json:"error,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// ListOptions filters and paginates List results.
type ListOptions struct {
	Statuses []Status // nil/empty = all statuses
	Page     int      // 1-based; 0 = no pagination
	PageSize int      // 0 = no limit
}

// Store is a pluggable persistence layer.
type Store interface {
	Create(task *Task) error
	Get(id string) (*Task, error)
	Update(id string, fn func(*Task)) error
	List(opts ListOptions) ([]*Task, error)
	Delete(id string) error
}
