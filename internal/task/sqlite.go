package task

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/desico/chapter-overview/internal/model"
	_ "modernc.org/sqlite"
)

const createTableSQL = `
CREATE TABLE IF NOT EXISTS tasks (
    id            TEXT PRIMARY KEY,
    status        TEXT NOT NULL,
    progress      REAL,
    message       TEXT,
    pdf_name      TEXT,
    pdf_path      TEXT,
    total_pages   INTEGER,
    chapters_json TEXT,
    metrics_json  TEXT,
    error         TEXT,
    created_at    DATETIME,
    updated_at    DATETIME
)`

// sqliteStore is a SQLite-backed Store.
type sqliteStore struct {
	db  *sql.DB
	mu  sync.Mutex
}

// NewSQLiteStore opens (or creates) a SQLite database at the given path and
// returns a Store backed by it. Pass ":memory:" for an in-memory database.
func NewSQLiteStore(path string) (Store, error) {
	var dsn string
	switch {
	case path == ":memory:":
		dsn = "file::memory:?mode=memory&cache=shared"
	case strings.HasPrefix(path, "file:"):
		dsn = path // caller-provided full URI (e.g., tests with unique names)
	default:
		dsn = fmt.Sprintf("file:%s?_journal=WAL&_busy_timeout=5000", path)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}
	return &sqliteStore{db: db}, nil
}

// Create inserts a new task row.
func (s *sqliteStore) Create(task *Task) error {
	chapJSON, err := marshalChapters(task.Chapters)
	if err != nil {
		return err
	}
	metrJSON, err := marshalStats(task.Stats)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO tasks
		    (id, status, progress, message, pdf_name, pdf_path, total_pages,
		     chapters_json, metrics_json, error, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, string(task.Status), task.Progress, task.Message,
		task.PDFName, task.PDFPath, task.TotalPages,
		chapJSON, metrJSON, task.Error,
		task.CreatedAt.UTC().Format(time.RFC3339Nano),
		task.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

// Get retrieves a task by ID.
func (s *sqliteStore) Get(id string) (*Task, error) {
	row := s.db.QueryRow(
		`SELECT id, status, progress, message, pdf_name, pdf_path, total_pages,
		        chapters_json, metrics_json, error, created_at, updated_at
		 FROM tasks WHERE id = ?`, id)
	return scanTask(row)
}

// Update retrieves the task, applies fn, then writes it back.
// It uses a mutex to serialize writes.
func (s *sqliteStore) Update(id string, fn func(*Task)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, err := s.Get(id)
	if err != nil {
		return err
	}
	fn(task)
	task.UpdatedAt = time.Now().UTC()

	chapJSON, err := marshalChapters(task.Chapters)
	if err != nil {
		return err
	}
	metrJSON, err := marshalStats(task.Stats)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`UPDATE tasks
		 SET status=?, progress=?, message=?, pdf_name=?, pdf_path=?,
		     total_pages=?, chapters_json=?, metrics_json=?, error=?, updated_at=?
		 WHERE id=?`,
		string(task.Status), task.Progress, task.Message, task.PDFName, task.PDFPath,
		task.TotalPages, chapJSON, metrJSON, task.Error,
		task.UpdatedAt.UTC().Format(time.RFC3339Nano),
		id,
	)
	return err
}

// List returns tasks matching opts, ordered by created_at DESC.
func (s *sqliteStore) List(opts ListOptions) ([]*Task, error) {
	var sb strings.Builder
	sb.WriteString(
		`SELECT id, status, progress, message, pdf_name, pdf_path, total_pages,
		        chapters_json, metrics_json, error, created_at, updated_at
		 FROM tasks`)

	var args []interface{}
	if len(opts.Statuses) > 0 {
		placeholders := make([]string, len(opts.Statuses))
		for i, st := range opts.Statuses {
			placeholders[i] = "?"
			args = append(args, string(st))
		}
		sb.WriteString(" WHERE status IN (")
		sb.WriteString(strings.Join(placeholders, ","))
		sb.WriteString(")")
	}
	sb.WriteString(" ORDER BY created_at DESC")

	if opts.Page > 0 && opts.PageSize > 0 {
		sb.WriteString(" LIMIT ? OFFSET ?")
		args = append(args, opts.PageSize, (opts.Page-1)*opts.PageSize)
	}

	rows, err := s.db.Query(sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// Delete removes a task by ID.
func (s *sqliteStore) Delete(id string) error {
	res, err := s.db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task %q not found", id)
	}
	return nil
}

// ---- helpers ----------------------------------------------------------------

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanTask(row scanner) (*Task, error) {
	var t Task
	var statusStr string
	var chapJSON, metrJSON sql.NullString
	var createdStr, updatedStr string

	err := row.Scan(
		&t.ID, &statusStr, &t.Progress, &t.Message,
		&t.PDFName, &t.PDFPath, &t.TotalPages,
		&chapJSON, &metrJSON, &t.Error,
		&createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	t.Status = Status(statusStr)

	if chapJSON.Valid && chapJSON.String != "" {
		if err := json.Unmarshal([]byte(chapJSON.String), &t.Chapters); err != nil {
			return nil, fmt.Errorf("unmarshal chapters: %w", err)
		}
	}
	if metrJSON.Valid && metrJSON.String != "" {
		var stats model.Stats
		if err := json.Unmarshal([]byte(metrJSON.String), &stats); err != nil {
			return nil, fmt.Errorf("unmarshal metrics: %w", err)
		}
		t.Stats = &stats
	}

	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
	return &t, nil
}

func marshalChapters(chapters []model.Chapter) (sql.NullString, error) {
	if chapters == nil {
		return sql.NullString{}, nil
	}
	b, err := json.Marshal(chapters)
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: string(b), Valid: true}, nil
}

func marshalStats(stats *model.Stats) (sql.NullString, error) {
	if stats == nil {
		return sql.NullString{}, nil
	}
	b, err := json.Marshal(stats)
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: string(b), Valid: true}, nil
}
