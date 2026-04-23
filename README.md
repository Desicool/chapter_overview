# Chapter Overview

Upload a PDF and get an AI-generated summary of each chapter. Works via a web UI or the CLI.

## Quick start

```bash
# 1. Build frontend + backend
make all

# 2. Start (Prometheus + Grafana sidecars optional — comment out docker-compose.yml services if not needed)
MOONSHOT_API_KEY=<your-key> make start
# → http://localhost:8080
```

Or CLI-only (no server, no Docker):

```bash
MOONSHOT_API_KEY=<your-key> ./chapter-overview path/to/book.pdf
# Writes JSON to output/result.json
```

## Environment variables

| Variable | Required | Description |
|---|---|---|
| `MOONSHOT_API_KEY` | Yes | [Moonshot (Kimi) API key](https://platform.moonshot.cn/) |

## CLI flags

```
./chapter-overview [pdf-path]           # one-shot summarise to output/
./chapter-overview serve [--port 8080]  # start HTTP server
```

Common optional flags (both modes):

```
--provider kimi          # only provider today
--text-model <id>        # override Kimi text model (default: moonshot-v1-32k)
--vision-model <id>      # override vision model (default: moonshot-v1-32k-vision-preview)
--concurrency 4          # parallel chapter workers
--out ./output           # output directory for CLI mode
```

## Server API

| Endpoint | Method | Description |
|---|---|---|
| `POST /api/tasks` | multipart `file` | Upload PDF; returns `{"task_id":"…"}` |
| `GET /api/tasks/:id` | — | Task status + chapters JSON |
| `GET /api/tasks/:id/events` | SSE | Real-time chapter-done events |

## Observability

Docker Compose starts Prometheus (`:9090`) and Grafana (`:3000`). Dashboards are in `grafana/`.

## Known limitations / TODO

- **LLM summarize "failures" may be false positives.** The fallback (⚠ amber cards in the UI) fires when the LLM refuses, returns invalid JSON, or the response trips a short-text heuristic. On borderline content a better-aligned or domain-fine-tuned model will recover many of these. The current readability guard only fixes the *display* of garbled extraction; improving the upstream refusal-detection (smarter retry, self-trained model) is future work.
- Kimi is the only supported provider today. The `provider.Provider` interface is small — adding OpenAI / Anthropic is straightforward.
- CLI mode does not stream progress; server mode does (SSE).
