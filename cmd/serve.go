package cmd

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/desico/chapter-overview/internal/metrics"
	"github.com/desico/chapter-overview/internal/pipeline"
	"github.com/desico/chapter-overview/internal/provider"
	"github.com/desico/chapter-overview/internal/server"
	"github.com/desico/chapter-overview/internal/task"
)

// WebFS holds the embedded web/dist filesystem. Set by web_embed.go in main.
var WebFS fs.FS

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web server",
	RunE:  runServe,
}

var (
	flagPort            int
	flagDataDir         string
	flagDB              string
	flagServeConcurrent int
	flagServeProvider   string
	flagServeText       string
	flagServeVision     string
)

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().IntVar(&flagPort, "port", 8080, "HTTP port")
	serveCmd.Flags().StringVar(&flagDataDir, "data-dir", "./data", "Directory for uploaded PDFs")
	serveCmd.Flags().StringVar(&flagDB, "db", "./tasks.db", "SQLite database path")
	serveCmd.Flags().IntVar(&flagServeConcurrent, "max-concurrent", 3, "Max parallel pipeline workers")
	serveCmd.Flags().StringVar(&flagServeProvider, "provider", "kimi", "LLM provider")
	serveCmd.Flags().StringVar(&flagServeText, "text-model", "", "Text model override")
	serveCmd.Flags().StringVar(&flagServeVision, "vision-model", "", "Vision model override")
}

func runServe(_ *cobra.Command, _ []string) error {
	if err := os.MkdirAll(flagDataDir, 0755); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	store, err := task.NewSQLiteStore(flagDB)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	// Startup recovery: mark any tasks interrupted by a previous crash as failed
	inProgress, _ := store.List(task.ListOptions{
		Statuses: []task.Status{task.StatusPending, task.StatusDetecting, task.StatusSummarizing},
	})
	for _, t := range inProgress {
		_ = store.Update(t.ID, func(t *task.Task) {
			t.Status = task.StatusFailed
			t.Error = "server restarted during processing"
		})
	}

	hub := task.NewHub()
	metricsReg := metrics.New()

	prov, err := provider.Get(flagServeProvider, provider.Config{
		TextModel:   flagServeText,
		VisionModel: flagServeVision,
	})
	if err != nil {
		return fmt.Errorf("initializing provider: %w", err)
	}

	pipelineOpts := pipeline.Options{MaxConcurrent: flagServeConcurrent}
	_ = pipelineOpts // options are threaded through by worker

	worker := task.NewWorker(store, hub, prov, flagServeConcurrent, metricsReg, flagDataDir)
	engine := server.New(store, hub, worker, metricsReg.Handler(), flagDataDir, WebFS)

	fmt.Printf("Listening on :%d\n", flagPort)
	return engine.Run(fmt.Sprintf(":%d", flagPort))
}
