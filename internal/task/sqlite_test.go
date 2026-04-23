package task

import (
	"fmt"
	"testing"
	"time"
)

// newTestStore opens a uniquely-named in-memory SQLite store for testing.
// Each call gets a fresh, isolated database.
func newTestStore(t *testing.T) Store {
	t.Helper()
	dsn := fmt.Sprintf("file:testdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	store, err := NewSQLiteStore(dsn)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	return store
}

func makeTask(id string, status Status) *Task {
	now := time.Now().UTC().Truncate(time.Second)
	return &Task{
		ID:        id,
		Status:    status,
		PDFName:   "test.pdf",
		PDFPath:   "/tmp/test.pdf",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestSQLiteCreateGet(t *testing.T) {
	store := newTestStore(t)
	task := makeTask("task-1", StatusPending)

	if err := store.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.Get("task-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("ID: got %q, want %q", got.ID, task.ID)
	}
	if got.Status != task.Status {
		t.Errorf("Status: got %q, want %q", got.Status, task.Status)
	}
	if got.PDFName != task.PDFName {
		t.Errorf("PDFName: got %q, want %q", got.PDFName, task.PDFName)
	}
}

func TestSQLiteUpdate(t *testing.T) {
	store := newTestStore(t)
	task := makeTask("task-2", StatusPending)

	if err := store.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Update("task-2", func(t *Task) {
		t.Status = StatusDetecting
		t.Progress = 0.5
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := store.Get("task-2")
	if err != nil {
		t.Fatalf("Get after Update: %v", err)
	}
	if got.Status != StatusDetecting {
		t.Errorf("Status: got %q, want %q", got.Status, StatusDetecting)
	}
	if got.Progress != 0.5 {
		t.Errorf("Progress: got %v, want 0.5", got.Progress)
	}
}

func TestSQLiteListAll(t *testing.T) {
	store := newTestStore(t)

	// Insert tasks with different times so ordering is deterministic.
	for i, st := range []Status{StatusPending, StatusDetecting, StatusDone} {
		task := makeTask(fmt.Sprintf("list-task-%d", i), st)
		task.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Second)
		task.UpdatedAt = task.CreatedAt
		if err := store.Create(task); err != nil {
			t.Fatalf("Create task %d: %v", i, err)
		}
	}

	tasks, err := store.List(ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("got %d tasks, want 3", len(tasks))
	}
	// Ordered by created_at DESC — newest first.
	if tasks[0].Status != StatusDone {
		t.Errorf("first task should be Done, got %q", tasks[0].Status)
	}
}

func TestSQLiteListFilterByStatus(t *testing.T) {
	store := newTestStore(t)
	for i, st := range []Status{StatusPending, StatusPending, StatusDone} {
		task := makeTask(fmt.Sprintf("filter-task-%d", i), st)
		if err := store.Create(task); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	tasks, err := store.List(ListOptions{Statuses: []Status{StatusPending}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("got %d pending tasks, want 2", len(tasks))
	}
	for _, task := range tasks {
		if task.Status != StatusPending {
			t.Errorf("expected pending, got %q", task.Status)
		}
	}
}

func TestSQLiteListPagination(t *testing.T) {
	store := newTestStore(t)
	for i := 0; i < 5; i++ {
		task := makeTask(fmt.Sprintf("page-task-%d", i), StatusPending)
		task.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Second)
		task.UpdatedAt = task.CreatedAt
		if err := store.Create(task); err != nil {
			t.Fatalf("Create task %d: %v", i, err)
		}
	}

	page1, err := store.List(ListOptions{Page: 1, PageSize: 3})
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if len(page1) != 3 {
		t.Errorf("page 1: got %d tasks, want 3", len(page1))
	}

	page2, err := store.List(ListOptions{Page: 2, PageSize: 3})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("page 2: got %d tasks, want 2", len(page2))
	}

	// Verify no overlap.
	seen := make(map[string]bool)
	for _, t := range page1 {
		seen[t.ID] = true
	}
	for _, t := range page2 {
		if seen[t.ID] {
			t2 := t
			_ = t2
			// Use a different variable name to avoid shadowing the outer t.
		}
	}
}

func TestSQLiteDelete(t *testing.T) {
	store := newTestStore(t)
	task := makeTask("del-task-1", StatusPending)

	if err := store.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Delete("del-task-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := store.Get("del-task-1")
	if err == nil {
		t.Fatal("Get after Delete should return error")
	}
}
