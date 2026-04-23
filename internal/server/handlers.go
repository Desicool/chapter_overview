package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/desico/chapter-overview/internal/task"
)

type handlers struct {
	store   task.Store
	hub     *task.Hub
	worker  *task.Worker
	dataDir string
}

func (h *handlers) upload(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing file field"})
		return
	}

	id := uuid.New().String()
	dir := filepath.Join(h.dataDir, id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload dir"})
		return
	}
	pdfPath := filepath.Join(dir, "original.pdf")
	if err := c.SaveUploadedFile(fh, pdfPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}

	now := time.Now().UTC()
	t := &task.Task{
		ID:        id,
		Status:    task.StatusPending,
		PDFName:   fh.Filename,
		PDFPath:   pdfPath,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.store.Create(t); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}
	if err := h.worker.Submit(t); err != nil {
		if err == task.ErrTooManyTasks {
			c.Header("Retry-After", "30")
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "server busy, retry later"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to submit task"})
		return
	}
	c.JSON(http.StatusCreated, t)
}

func (h *handlers) list(c *gin.Context) {
	opts := task.ListOptions{}

	if s := c.Query("status"); s != "" {
		opts.Statuses = []task.Status{task.Status(s)}
	}
	if p, err := strconv.Atoi(c.Query("page")); err == nil {
		opts.Page = p
	}
	if ps, err := strconv.Atoi(c.Query("page_size")); err == nil {
		opts.PageSize = ps
	}

	tasks, err := h.store.List(opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tasks == nil {
		tasks = []*task.Task{}
	}
	c.JSON(http.StatusOK, tasks)
}

func (h *handlers) get(c *gin.Context) {
	t, err := h.store.Get(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *handlers) delete(c *gin.Context) {
	id := c.Param("id")
	t, err := h.store.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if t.Status == task.StatusPending || t.Status == task.StatusDetecting || t.Status == task.StatusSummarizing {
		c.JSON(http.StatusConflict, gin.H{"error": "task is in progress"})
		return
	}
	if err := h.store.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = os.RemoveAll(filepath.Join(h.dataDir, id))
	c.Status(http.StatusNoContent)
}

func (h *handlers) sseEvents(c *gin.Context) {
	id := c.Param("id")
	t, err := h.store.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")

	// If already terminal, send final event and close immediately
	if t.Status == task.StatusDone || t.Status == task.StatusFailed {
		eventType := "done"
		if t.Status == task.StatusFailed {
			eventType = "error"
		}
		data, _ := json.Marshal(task.SSEEvent{Type: eventType, Data: t})
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
		return
	}

	ch, unsub := h.hub.Subscribe(id)
	defer unsub()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	ctx := c.Request.Context()
	c.Stream(func(w io.Writer) bool {
		select {
		case <-ctx.Done():
			return false
		case <-heartbeat.C:
			fmt.Fprint(w, ": keepalive\n\n")
			c.Writer.Flush()
			return true
		case event, ok := <-ch:
			if !ok {
				return false
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			c.Writer.Flush()
			// Close SSE stream on terminal events
			if event.Type == "done" || event.Type == "error" {
				return false
			}
			return true
		}
	})
}

func (h *handlers) servePDF(c *gin.Context) {
	id := c.Param("id")
	t, err := h.store.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	http.ServeFile(c.Writer, c.Request, t.PDFPath)
}

func (h *handlers) taskMetrics(c *gin.Context) {
	id := c.Param("id")
	t, err := h.store.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if t.Stats == nil {
		c.JSON(http.StatusOK, []struct{}{})
		return
	}
	c.JSON(http.StatusOK, t.Stats.Records)
}
