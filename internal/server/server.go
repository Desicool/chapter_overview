package server

import (
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/desico/chapter-overview/internal/task"
)

// New creates the Gin engine with all routes wired up.
// webFS must be the root of the built frontend (i.e., the web/dist directory).
func New(store task.Store, hub *task.Hub, worker *task.Worker, metricsHandler http.Handler, dataDir string, webFS fs.FS) *gin.Engine {
	r := gin.Default()
	r.MaxMultipartMemory = 200 << 20 // 200 MB; Gin default is 32 MB and silently truncates

	h := &handlers{store: store, hub: hub, worker: worker, dataDir: dataDir}

	api := r.Group("/api")
	{
		api.POST("/tasks", h.upload)
		api.GET("/tasks", h.list)
		api.GET("/tasks/:id", h.get)
		api.DELETE("/tasks/:id", h.delete)
		api.GET("/tasks/:id/events", h.sseEvents)
		api.GET("/tasks/:id/pdf", h.servePDF)
		api.GET("/tasks/:id/metrics", h.taskMetrics)
	}

	if metricsHandler != nil {
		r.GET("/metrics", gin.WrapH(metricsHandler))
	}

	if webFS != nil {
		r.StaticFS("/assets", http.FS(mustSub(webFS, "assets")))
		r.NoRoute(func(c *gin.Context) {
			// Serve SPA index.html for all non-API paths
			f, err := webFS.Open("index.html")
			if err != nil {
				c.Status(http.StatusNotFound)
				return
			}
			defer f.Close()
			stat, _ := f.Stat()
			c.DataFromReader(http.StatusOK, stat.Size(), "text/html; charset=utf-8", f, nil)
		})
	}

	return r
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		// assets dir might not exist before first frontend build
		return fsys
	}
	return sub
}
