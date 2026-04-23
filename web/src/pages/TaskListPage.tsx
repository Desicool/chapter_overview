import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { listTasks } from '../api'
import type { Task, Status } from '../types'

const TERMINAL: Status[] = ['done', 'failed']

const STATUS_STYLES: Record<Status, string> = {
  pending:    'text-muted',
  detecting:  'text-amber',
  summarizing:'text-amber',
  done:       'text-green-400',
  failed:     'text-red-400',
}

function formatDate(iso: string) {
  return new Date(iso).toLocaleString(undefined, {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}

export default function TaskListPage() {
  const [tasks, setTasks] = useState<Task[]>([])
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  async function fetchTasks() {
    try {
      const data = await listTasks()
      setTasks(data)
    } catch {
      // ignore fetch errors silently
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchTasks()
  }, [])

  useEffect(() => {
    const hasActive = tasks.some((t) => !TERMINAL.includes(t.status))
    if (hasActive && !intervalRef.current) {
      intervalRef.current = setInterval(fetchTasks, 5000)
    } else if (!hasActive && intervalRef.current) {
      clearInterval(intervalRef.current)
      intervalRef.current = null
    }
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current)
        intervalRef.current = null
      }
    }
  }, [tasks])

  if (loading) {
    return (
      <div className="flex items-center justify-center py-32">
        <div className="shimmer w-64 h-8 rounded" />
      </div>
    )
  }

  if (tasks.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-32 gap-4">
        <p className="font-serif text-2xl text-text">No tasks yet</p>
        <p className="font-mono text-sm text-muted">Upload a PDF to get started</p>
        <Link
          to="/"
          className="mt-2 font-mono text-xs text-amber border border-amber/30 rounded px-4 py-2 hover:bg-amber/10 transition-colors"
        >
          Upload →
        </Link>
      </div>
    )
  }

  return (
    <div className="max-w-5xl mx-auto px-8 py-8">
      <div className="flex items-baseline justify-between mb-6">
        <h1 className="font-serif text-2xl text-text">Tasks</h1>
        <span className="font-mono text-xs text-muted">{tasks.length} total</span>
      </div>

      {/* Header row */}
      <div className="grid grid-cols-[1fr_auto_auto_auto_auto] gap-6 px-4 mb-2">
        {['File', 'Created', 'Pages', 'Chapters', 'Status'].map((h) => (
          <span key={h} className="font-mono text-xs text-muted uppercase tracking-widest">{h}</span>
        ))}
      </div>

      <div className="space-y-1">
        {tasks.map((t) => (
          <button
            key={t.id}
            onClick={() => navigate(`/tasks/${t.id}`)}
            className="w-full text-left bg-surface border border-border rounded px-4 py-3 hover:border-amber/40 transition-colors grid grid-cols-[1fr_auto_auto_auto_auto] gap-6 items-center"
          >
            <span className="font-serif text-text text-sm truncate">{t.pdf_name}</span>
            <span className="font-mono text-xs text-muted tabular-nums whitespace-nowrap">
              {formatDate(t.created_at)}
            </span>
            <span className="font-mono text-xs text-muted tabular-nums text-right">
              {t.total_pages || '—'}
            </span>
            <span className="font-mono text-xs text-muted tabular-nums text-right">
              {t.chapters?.length ?? 0}
            </span>
            <span className={`font-mono text-xs tabular-nums ${STATUS_STYLES[t.status]}`}>
              {t.status}
            </span>
          </button>
        ))}
      </div>
    </div>
  )
}
