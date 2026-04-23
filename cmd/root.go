package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/desico/chapter-overview/internal/model"
	"github.com/desico/chapter-overview/internal/pdf"
	"github.com/desico/chapter-overview/internal/pipeline"
	"github.com/desico/chapter-overview/internal/provider"
)

var (
	flagProvider      string
	flagTextModel     string
	flagVisionModel   string
	flagDetectModel   string
	flagOutputDir     string
	flagMaxConcurrent int
	flagSplitPDF      bool
	flagMaxChapters   int
)

var rootCmd = &cobra.Command{
	Use:   "chapter-overview <pdf-path>",
	Short: "Split a PDF into chapters and summarize each one",
	Args:  cobra.ExactArgs(1),
	RunE:  run,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVar(&flagProvider, "provider", "kimi", "LLM provider (kimi)")
	rootCmd.Flags().StringVar(&flagTextModel, "text-model", "", "Summarization model override (default: moonshot-v1-32k)")
	rootCmd.Flags().StringVar(&flagVisionModel, "vision-model", "", "Vision model override (default: moonshot-v1-32k-vision-preview)")
	rootCmd.Flags().StringVar(&flagOutputDir, "output-dir", "./output", "Output directory")
	rootCmd.Flags().IntVar(&flagMaxConcurrent, "max-concurrent", 50, "Max parallel LLM calls")
	rootCmd.Flags().StringVar(&flagDetectModel, "detect-model", "", "Detection model override (default: moonshot-v1-8k)")
	rootCmd.Flags().BoolVar(&flagSplitPDF, "split-pdf", false, "Also output per-chapter PDF files")
	rootCmd.Flags().IntVar(&flagMaxChapters, "max-chapters", 0, "Limit summarization to first N chapters (0 = all)")
}

func run(cmd *cobra.Command, args []string) error {
	pdfPath := args[0]

	if _, err := os.Stat(pdfPath); err != nil {
		return fmt.Errorf("cannot open PDF: %w", err)
	}

	if err := os.MkdirAll(flagOutputDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	prov, err := provider.Get(flagProvider, provider.Config{
		TextModel:   flagTextModel,
		VisionModel: flagVisionModel,
		DetectModel: flagDetectModel,
	})
	if err != nil {
		return fmt.Errorf("initializing provider: %w", err)
	}
	prov = provider.WithRetry(prov, 4)

	ctx := context.Background()
	opts := pipeline.Options{MaxConcurrent: flagMaxConcurrent}

	totalPages, err := pdf.GetPageCount(pdfPath)
	if err != nil {
		return fmt.Errorf("reading PDF: %w", err)
	}
	fmt.Printf("PDF: %s (%d pages)\n", filepath.Base(pdfPath), totalPages)

	fmt.Println("\n[Phase 1] Detecting chapters...")
	chapters, err := pipeline.DetectChapters(ctx, pdfPath, prov, opts)
	if err != nil {
		return fmt.Errorf("chapter detection: %w", err)
	}
	fmt.Printf("[Phase 1] Detected %d chapters\n", len(chapters))

	toSummarize := chapters
	if flagMaxChapters > 0 && flagMaxChapters < len(chapters) {
		toSummarize = chapters[:flagMaxChapters]
		fmt.Printf("[Phase 2] Summarizing first %d of %d chapters...\n", flagMaxChapters, len(chapters))
	} else {
		fmt.Println("\n[Phase 2] Summarizing chapters...")
	}
	summarized, err := pipeline.SummarizeChapters(ctx, pdfPath, toSummarize, prov, opts)
	if err != nil {
		return fmt.Errorf("summarization: %w", err)
	}
	// Write summaries back; chapters beyond the limit retain empty summary
	copy(chapters, summarized)

	result := model.Result{
		Source:     filepath.Base(pdfPath),
		TotalPages: totalPages,
		Chapters:   chapters,
	}

	outJSON := filepath.Join(flagOutputDir, "result.json")
	if err := writeJSON(outJSON, result); err != nil {
		return fmt.Errorf("writing result: %w", err)
	}
	fmt.Printf("\nResult written to %s\n", outJSON)

	if flagSplitPDF {
		fmt.Println("\n[Phase 3] Splitting PDF by chapter...")
		if err := splitChapterPDFs(pdfPath, chapters, flagOutputDir); err != nil {
			fmt.Printf("[warn] PDF split had errors: %v\n", err)
		}
	}

	printSummary(chapters)
	return nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func splitChapterPDFs(pdfPath string, chapters []model.Chapter, outDir string) error {
	for _, ch := range chapters {
		safeName := sanitizeFilename(ch.Title)
		outPath := filepath.Join(outDir, fmt.Sprintf("chapter_%02d_%s.pdf", ch.Index, safeName))
		if err := pdf.SplitPDF(pdfPath, ch.StartPage, ch.EndPage, outPath); err != nil {
			fmt.Printf("[warn] chapter %d split failed: %v\n", ch.Index, err)
		} else {
			fmt.Printf("  chapter_%02d_%s.pdf (pages %d–%d)\n", ch.Index, safeName, ch.StartPage, ch.EndPage)
		}
	}
	return nil
}

func sanitizeFilename(name string) string {
	name = strings.ToLower(name)
	var sb strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			sb.WriteRune(r)
		} else {
			sb.WriteByte('_')
		}
	}
	s := sb.String()
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

func printSummary(chapters []model.Chapter) {
	fmt.Printf("\n=== Summary (%d chapters) ===\n\n", len(chapters))
	for _, ch := range chapters {
		fmt.Printf("Chapter %d: %s (pages %d–%d)\n", ch.Index, ch.Title, ch.StartPage, ch.EndPage)
		if ch.Summary != "" {
			fmt.Printf("  %s\n", ch.Summary)
		}
		fmt.Println()
	}
}
