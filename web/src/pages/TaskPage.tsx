import { useEffect, useRef, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { getTask, subscribeToTask } from '../api'
import type { Task, Chapter, Status } from '../types'
import ProgressBar from '../components/ProgressBar'
import MetricsPanel from '../components/MetricsPanel'

const TERMINAL: Status[] = ['done', 'failed']

export default function TaskPage() {
  const { id } = useParams<{ id: string }>()
  const [task, setTask] = useState<Task | null>(null)
  const [chapters, setChapters] = useState<Chapter[]>([])
  const [progress, setProgress] = useState(0)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const unsubRef = useRef<(() => void) | null>(null)

  useEffect(() => {
    if (!id) return

    getTask(id).then((t) => {
      setTask(t)
      setChapters(t.chapters ?? [])
      setProgress(t.progress)
      setLoading(false)

      if (!TERMINAL.includes(t.status)) {
        openSSE(id)
      }
    }).catch((err: Error) => {
      setError(err.message)
      setLoading(false)
    })

    return () => {
      unsubRef.current?.()
    }
  }, [id])

  function openSSE(taskId: string) {
    const unsub = subscribeToTask(taskId, (event) => {
      switch (event.type) {
        case 'progress': {
          const t = event.data as Task
          setProgress(t.progress ?? 0)
          setTask((prev) => prev
            ? { ...prev, progress: t.progress ?? prev.progress, message: t.message ?? prev.message }
            : prev)
          break
        }

        case 'chapter_detected': {
          const ch = event.data as Chapter
          if (ch?.index == null) break
          setChapters((prev) => {
            if (prev.find((c) => c.index === ch.index)) return prev
            return [...prev, ch].sort((a, b) => a.index - b.index)
          })
          break
        }

        case 'chapter_done': {
          const ch = event.data as Chapter
          if (ch?.index == null) break
          setChapters((prev) =>
            prev.map((c) => c.index === ch.index ? { ...c, summary: ch.summary } : c)
          )
          break
        }

        case 'done': {
          unsubRef.current?.()
          unsubRef.current = null
          const t = event.data as Task
          setTask(t)
          setProgress(1)
          if (t.chapters?.length) {
            setChapters(t.chapters)
          }
          break
        }

        case 'error': {
          unsubRef.current?.()
          unsubRef.current = null
          const t = event.data as Task
          setTask((prev) => prev ? { ...prev, status: 'failed', error: t.error } : prev)
          setError(t.error ?? 'An error occurred')
          break
        }
      }
    })
    unsubRef.current = unsub
  }

  if (loading) {
    return (
      <div className="min-h-screen bg-bg flex items-center justify-center">
        <div className="shimmer w-64 h-8 rounded" />
      </div>
    )
  }

  if (!task) {
    return (
      <div className="min-h-screen bg-bg flex items-center justify-center text-muted font-mono text-sm">
        Task not found.
      </div>
    )
  }

  const isDone = task.status === 'done'

  return (
    <div className="min-h-screen bg-bg">
      {/* Thin amber progress bar */}
      <div className="w-full h-1 bg-surface">
        <div
          className="h-1 bg-amber transition-all duration-500"
          style={{ width: `${Math.round(progress * 100)}%` }}
        />
      </div>

      {/* Top bar */}
      <div className="px-8 py-4 border-b border-border flex items-center gap-4">
        <Link to="/tasks" className="font-mono text-xs text-muted hover:text-amber transition-colors">
          ← Tasks
        </Link>
        <span className="text-border">|</span>
        <h2 className="font-serif text-lg text-text truncate flex-1">{task.pdf_name}</h2>
        <ProgressBar value={progress} status={task.status} />
      </div>

      {/* Error banner */}
      {error && (
        <div className="mx-8 mt-4 p-3 bg-red-900/30 border border-red-700 rounded font-mono text-xs text-red-300">
          {error}
        </div>
      )}

      {/* Two-column layout */}
      <div className="flex gap-0 max-w-7xl mx-auto px-8 pt-8 pb-16">
        {/* Left — 65% chapters table */}
        <div className="w-[65%] pr-8">
          {chapters.length === 0 && !isDone && (
            <div className="text-muted font-mono text-sm">
              {task.status === 'detecting' ? 'Detecting chapters…' : 'Waiting to start…'}
            </div>
          )}

          {chapters.length === 0 && task.status === 'summarizing' && (
            <div className="text-muted font-mono text-sm">Summarizing chapters…</div>
          )}

          {chapters.length > 0 && (
            <table className="w-full border-collapse">
              <thead>
                <tr className="border-b border-border">
                  <th className="font-mono text-xs text-muted uppercase tracking-widest text-left pb-2 pr-4 w-14">Ch.</th>
                  <th className="font-mono text-xs text-muted uppercase tracking-widest text-left pb-2 pr-4 w-40">Title</th>
                  <th className="font-mono text-xs text-muted uppercase tracking-widest text-left pb-2 pr-4 w-24">Pages</th>
                  <th className="font-mono text-xs text-muted uppercase tracking-widest text-left pb-2 pr-4">Summary</th>
                  <th className="font-mono text-xs text-muted uppercase tracking-widest text-left pb-2 w-12"></th>
                </tr>
              </thead>
              <tbody>
                {chapters.map((ch, i) => (
                  <ChapterRow key={ch.index} chapter={ch} taskId={task.id} index={i} />
                ))}
              </tbody>
            </table>
          )}

          {chapters.length > 0 && !isDone && (
            <p className="mt-4 font-mono text-xs text-muted">
              {task.status === 'summarizing' ? 'Summarizing chapters…' : ''}
            </p>
          )}
        </div>

        {/* Right — 35% sticky metrics */}
        <div className="w-[35%]">
          <div className="sticky top-8">
            <h3 className="font-mono text-xs text-muted uppercase tracking-widest mb-3">
              Metrics
            </h3>
            <MetricsPanel
              stats={isDone ? task.metrics : undefined}
              chapterCount={chapters.length}
            />

            {!isDone && (
              <p className="mt-3 font-mono text-xs text-muted text-center">
                Metrics available after processing completes
              </p>
            )}

            {task.total_pages > 0 && (
              <div className="mt-4 font-mono text-xs text-muted">
                <span className="text-text">{task.total_pages}</span> pages total
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

interface ChapterRowProps {
  chapter: Chapter
  taskId: string
  index: number
}

function ChapterRow({ chapter, taskId, index }: ChapterRowProps) {
  const hasSummary = chapter.summary && chapter.summary.trim().length > 0

  return (
    <tr
      className="border-b border-border/50 opacity-0 animate-[fadeSlideIn_0.4s_ease_forwards] align-top"
      style={
        {
          '--index': index,
          animationDelay: `calc(var(--index) * 60ms)`,
        } as React.CSSProperties
      }
    >
      {/* Ch. N badge */}
      <td className="py-3 pr-4">
        <span className="font-mono text-xs text-amber tracking-widest">
          Ch.&nbsp;{chapter.index}
        </span>
      </td>

      {/* Title */}
      <td className="py-3 pr-4 max-w-[10rem]">
        <span className="font-serif text-sm text-text leading-snug line-clamp-2">
          {chapter.title || `Chapter ${chapter.index}`}
        </span>
      </td>

      {/* Page range */}
      <td className="py-3 pr-4 whitespace-nowrap">
        <span className="font-mono text-xs text-muted">
          pp.&nbsp;{chapter.start_page}–{chapter.end_page}
        </span>
      </td>

      {/* Summary */}
      <td className="py-3 pr-4">
        {hasSummary ? (
          chapter.summary_status === 'fallback' || chapter.summary_status === 'failed' ? (
            <div
              role="note"
              className="text-sm text-amber/90 leading-relaxed border-l-2 border-amber/60 pl-3 animate-[fadeIn_0.4s_ease] whitespace-pre-line"
            >
              <span aria-hidden="true" className="mr-1">⚠</span>
              {chapter.summary}
            </div>
          ) : (
            <p className="text-sm text-text/80 leading-relaxed animate-[fadeIn_0.4s_ease]">
              {chapter.summary}
            </p>
          )
        ) : (
          <div className="space-y-2 py-1">
            <div className="shimmer h-3 rounded w-full" />
            <div className="shimmer h-3 rounded w-4/5" />
            <div className="shimmer h-3 rounded w-3/5" />
          </div>
        )}
      </td>

      {/* View link */}
      <td className="py-3 whitespace-nowrap">
        <Link
          to={`/tasks/${taskId}/view/${chapter.start_page}`}
          className="text-xs text-amber hover:text-amber/80 transition-colors font-mono"
        >
          View&nbsp;→
        </Link>
      </td>
    </tr>
  )
}
