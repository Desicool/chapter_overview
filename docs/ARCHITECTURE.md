# Architecture

## End-to-end flow

```
User uploads PDF
       │
       ▼
POST /api/tasks  ──►  task.Worker (goroutine pool)
                              │
                    ┌─────────▼──────────┐
                    │  pipeline.Run()    │
                    │                   │
                    │ 1. DetectChapters  │
                    │    ├─ ExtractTOC   │  pdfcpu bookmarks
                    │    └─ LLM batch   │  15 pages/call → JSON
                    │                   │
                    │ 2. SummarizeChaps  │
                    │    ├─ text-only    │  HasMeaningfulText=true
                    │    ├─ multimodal   │  image pages / no text
                    │    └─ fallback     │  readability guard → warning
                    └─────────┬──────────┘
                              │ ProgressEvents
                    ┌─────────▼──────────┐
                    │   task.Hub (SSE)   │  fan-out to all open tabs
                    └─────────┬──────────┘
                              │ persisted to SQLite
                    ┌─────────▼──────────┐
                    │  task.SQLiteStore  │
                    └────────────────────┘
```

## Package map

| Package | Responsibility |
|---|---|
| `cmd/` | Cobra CLI — `root` (one-shot) and `serve` (HTTP server) entry points |
| `internal/pdf` | PDF I/O: TOC extraction (pdfcpu), page text (ledongthuc/pdf), image extraction (pdfcpu), page cache |
| `internal/pipeline` | Orchestrates DetectChapters + SummarizeChapters; owns all LLM prompt logic |
| `internal/provider` | `Provider` interface + Kimi implementation (OpenAI-compatible client) |
| `internal/task` | Background worker, SQLite persistence, SSE Hub for live updates |
| `internal/server` | Gin HTTP handlers — upload, task status, SSE endpoint, embedded web UI |
| `internal/model` | Shared data types: Chapter, PageContent, SummaryStatus, ChapterBoundary |
| `internal/metrics` | Prometheus counters/histograms registered at startup |
| `web/` | React + TypeScript frontend; built to `web/dist/` and embedded via `web_embed.go` |

## Key design decisions

### 1. Dual PDF libraries

`pdfcpu` handles bookmarks (TOC), image extraction, and PDF splitting.
`ledongthuc/pdf` handles per-page text extraction.
`pdfcpu`'s `ExtractContent` returns raw PDF content-stream operators, not human-readable text — it cannot be used for text extraction.

### 2. Text-first, multimodal only when needed

`summarizeChapter` (`pipeline/pipeline.go`) classifies each page: if `HasMeaningfulText` (≥ 20 trimmed chars from ledongthuc), the chapter uses a text-only LLM call; if images are present or no text was extracted, a vision model call is made. This keeps costs and latency low for text-heavy PDFs.

### 3. PageCache — the 11× speedup

`pdf.PageCache` (`internal/pdf/pdf.go`) memoizes `ExtractPagesRange` results across both the detect and summarize phases. Before this optimization the same PDF pages were re-read from disk twice per chapter; the cache eliminates that redundancy and was responsible for an 11× wall-clock speedup on a 688-page PDF.

### 4. LLM-batched chapter detection

When no embedded TOC is present, `scanPagesForChapters` sends pages in batches of 15 to the LLM (`pagesBatchSize = 15`, `pipeline.go`). Each call returns a JSON array of `{"page": N, "title": "…"}`. Boundaries are merged and capped at 15 chapters via `finalizeStructure`.

### 5. Fallback readability guard

When the LLM refuses or produces invalid JSON after two attempts, `buildFallbackSummary` is called. It:
1. Scores **readability** of the raw extracted text — ratio of recognizable runes (ASCII, CJK ideographs, common scripts) to total non-whitespace runes.
2. If ratio < 0.60, strips the garbled excerpt and emits `[warning: text extraction produced unreadable output]` with `SummaryFailed`.
3. Otherwise, sanitizes (removes control chars, U+FFFD, PUA runes U+E000–U+F8FF) and shows a ≤ 150-char excerpt with a `SummaryFallback` warning.

This prevents garbled bytes (common with CID-mapped CJK fonts) from reaching the UI.

### 6. SSE + SQLite hub pattern

The HTTP server exposes a `GET /api/tasks/:id/events` SSE endpoint. `task.Hub` is an in-process fan-out multiplexer: each subscriber gets its own buffered channel; the worker publishes `ProgressEvent`s as chapters complete. Results are also persisted to SQLite so reconnecting tabs or the CLI can reconstruct full state.

## Deployment

```bash
# One-time local build
make frontend    # npm install + build → web/dist/
make backend     # go build → ./chapter-overview (embeds web/dist/)

# Run with optional observability stack
docker compose up -d          # Prometheus :9090, Grafana :3000
MOONSHOT_API_KEY=<key> ./chapter-overview serve --port 8080

# Or skip Docker
MOONSHOT_API_KEY=<key> ./chapter-overview serve
```

Prometheus scrape target is configured in `prometheus.yml` (scrapes `:8080/metrics`).
Grafana dashboards are provisioned from `grafana/`.

SQLite database defaults to `tasks.db` in the working directory.
