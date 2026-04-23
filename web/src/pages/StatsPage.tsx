import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import { listTasks, getTaskMetrics, estimatePerPage } from '../api'
import type { Task } from '../types'

type ActiveTab = 'page' | 'task'
type MetricMode = 'tokens' | 'time'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

function formatStatus(status: string): string {
  return status.charAt(0).toUpperCase() + status.slice(1)
}

function fmtNum(n: number | undefined | null): string {
  if (n == null) return '—'
  return n.toLocaleString()
}

function fmtDuration(ms: number | undefined | null): string {
  if (ms == null) return '—'
  if (ms >= 60000) {
    const mins = Math.floor(ms / 60000)
    const secs = Math.round((ms % 60000) / 1000)
    return `${mins}m ${secs}s`
  }
  return `${(ms / 1000).toFixed(1)}s`
}

export default function StatsPage() {
  const navigate = useNavigate()
  const [activeTab, setActiveTab] = useState<ActiveTab>('page')

  // Page view state
  const [doneTasks, setDoneTasks] = useState<Task[]>([])
  const [selectedTaskId, setSelectedTaskId] = useState<string>('')
  const [metricMode, setMetricMode] = useState<MetricMode>('tokens')
  const [chartData, setChartData] = useState<{ page: number; value: number }[]>([])
  const [loadingChart, setLoadingChart] = useState(false)

  // Task view state
  const [allTasks, setAllTasks] = useState<Task[]>([])
  const [loadingTasks, setLoadingTasks] = useState(false)

  // Load done tasks for selector on mount
  useEffect(() => {
    listTasks({ status: 'done' }).then(setDoneTasks).catch(console.error)
  }, [])

  // Load all tasks when switching to task tab
  useEffect(() => {
    if (activeTab === 'task' && allTasks.length === 0) {
      setLoadingTasks(true)
      listTasks().then((tasks) => {
        setAllTasks(tasks)
        setLoadingTasks(false)
      }).catch((err: Error) => {
        console.error(err)
        setLoadingTasks(false)
      })
    }
  }, [activeTab, allTasks.length])

  // Load metrics when task is selected
  useEffect(() => {
    if (!selectedTaskId) {
      setChartData([])
      return
    }
    setLoadingChart(true)
    getTaskMetrics(selectedTaskId).then((records) => {
      const perPage = estimatePerPage(records)
      const data: { page: number; value: number }[] = []
      perPage.forEach((val, page) => {
        data.push({
          page,
          value: metricMode === 'tokens' ? val.tokens : val.duration,
        })
      })
      data.sort((a, b) => a.page - b.page)
      setChartData(data)
      setLoadingChart(false)
    }).catch((err: Error) => {
      console.error(err)
      setLoadingChart(false)
    })
  }, [selectedTaskId, metricMode])

  return (
    <div className="min-h-screen bg-bg">
      {/* Top bar */}
      <div className="px-8 py-4 border-b border-border flex items-center justify-between">
        <div className="flex gap-6">
          {/* Tab: Page View */}
          <button
            onClick={() => setActiveTab('page')}
            className={`font-mono text-sm pb-1 transition-colors ${
              activeTab === 'page'
                ? 'text-text border-b-2 border-amber'
                : 'text-muted hover:text-text'
            }`}
          >
            Page View
          </button>

          {/* Tab: Task View */}
          <button
            onClick={() => setActiveTab('task')}
            className={`font-mono text-sm pb-1 transition-colors ${
              activeTab === 'task'
                ? 'text-text border-b-2 border-amber'
                : 'text-muted hover:text-text'
            }`}
          >
            Task View
          </button>
        </div>

        {/* Grafana link */}
        <a
          href="http://localhost:3000"
          target="_blank"
          rel="noreferrer"
          className="font-mono text-xs text-muted hover:text-amber transition-colors"
        >
          Open Grafana ↗
        </a>
      </div>

      {/* Content */}
      <div className="px-8 py-8 max-w-6xl mx-auto">
        {activeTab === 'page' && (
          <PageViewTab
            doneTasks={doneTasks}
            selectedTaskId={selectedTaskId}
            setSelectedTaskId={setSelectedTaskId}
            metricMode={metricMode}
            setMetricMode={setMetricMode}
            chartData={chartData}
            loadingChart={loadingChart}
          />
        )}

        {activeTab === 'task' && (
          <TaskViewTab
            tasks={allTasks}
            loading={loadingTasks}
            onRowClick={(id) => navigate(`/tasks/${id}`)}
          />
        )}
      </div>
    </div>
  )
}

/* ── Page View Tab ──────────────────────────────────────────── */

interface PageViewTabProps {
  doneTasks: Task[]
  selectedTaskId: string
  setSelectedTaskId: (id: string) => void
  metricMode: MetricMode
  setMetricMode: (m: MetricMode) => void
  chartData: { page: number; value: number }[]
  loadingChart: boolean
}

function PageViewTab({
  doneTasks,
  selectedTaskId,
  setSelectedTaskId,
  metricMode,
  setMetricMode,
  chartData,
  loadingChart,
}: PageViewTabProps) {
  return (
    <div>
      {/* Selectors row */}
      <div className="flex items-center gap-4 mb-6">
        {/* Task selector */}
        <select
          value={selectedTaskId}
          onChange={(e) => setSelectedTaskId(e.target.value)}
          className="bg-surface border border-border rounded px-3 py-1.5 font-mono text-sm text-text flex-1 max-w-sm"
        >
          <option value="">— Select a task —</option>
          {doneTasks.map((t) => (
            <option key={t.id} value={t.id}>
              {t.pdf_name} ({formatDate(t.created_at)})
            </option>
          ))}
        </select>

        {/* Metric toggle */}
        <div className="flex gap-1 bg-surface border border-border rounded overflow-hidden">
          <button
            onClick={() => setMetricMode('tokens')}
            className={`px-3 py-1.5 font-mono text-xs transition-colors ${
              metricMode === 'tokens' ? 'bg-amber text-bg' : 'text-muted hover:text-text'
            }`}
          >
            Tokens per page
          </button>
          <button
            onClick={() => setMetricMode('time')}
            className={`px-3 py-1.5 font-mono text-xs transition-colors ${
              metricMode === 'time' ? 'bg-amber text-bg' : 'text-muted hover:text-text'
            }`}
          >
            Time (ms) per page
          </button>
        </div>
      </div>

      {/* Chart */}
      {!selectedTaskId ? (
        <div className="flex items-center justify-center h-64 border border-dashed border-border rounded text-muted font-mono text-sm">
          Select a task to view page metrics
        </div>
      ) : loadingChart ? (
        <div className="shimmer h-64 rounded" />
      ) : chartData.length === 0 ? (
        <div className="flex items-center justify-center h-64 border border-dashed border-border rounded text-muted font-mono text-sm">
          No metrics available
        </div>
      ) : (
        <div className="bg-surface border border-border rounded p-4">
          <ResponsiveContainer width="100%" height={320}>
            <BarChart data={chartData} margin={{ top: 8, right: 8, bottom: 8, left: 8 }}>
              <XAxis
                dataKey="page"
                tick={{ fill: '#6b6b70', fontSize: 11, fontFamily: 'JetBrains Mono, monospace' }}
                label={{ value: 'Page', position: 'insideBottom', offset: -4, fill: '#6b6b70', fontSize: 11 }}
              />
              <YAxis
                tick={{ fill: '#6b6b70', fontSize: 11, fontFamily: 'JetBrains Mono, monospace' }}
                width={60}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: '#141418',
                  border: '1px solid #2a2a30',
                  borderRadius: 4,
                  fontFamily: 'JetBrains Mono, monospace',
                  fontSize: 12,
                  color: '#f0ede8',
                }}
                labelFormatter={(label) => `Page ${label}`}
                formatter={(value: number) => [
                  metricMode === 'tokens'
                    ? `${Math.round(value).toLocaleString()} tokens`
                    : `${Math.round(value).toLocaleString()} ms`,
                  metricMode === 'tokens' ? 'Tokens' : 'Duration',
                ]}
              />
              <Bar dataKey="value" fill="#f59e0b" radius={[2, 2, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        </div>
      )}

      {/* Disclaimer */}
      <p className="mt-4 font-mono text-xs text-muted">
        ⚠ Values are estimates — tokens/time distributed evenly within each LLM batch
      </p>
    </div>
  )
}

/* ── Task View Tab ──────────────────────────────────────────── */

interface TaskViewTabProps {
  tasks: Task[]
  loading: boolean
  onRowClick: (id: string) => void
}

function TaskViewTab({ tasks, loading, onRowClick }: TaskViewTabProps) {
  if (loading) {
    return <div className="shimmer h-64 rounded" />
  }

  if (tasks.length === 0) {
    return (
      <div className="flex items-center justify-center h-64 border border-dashed border-border rounded text-muted font-mono text-sm">
        No tasks found
      </div>
    )
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full font-mono text-sm border-collapse">
        <thead>
          <tr className="border-b border-border">
            {['PDF Name', 'Created', 'Pages', 'Input Tokens', 'Output Tokens', 'Avg Tokens/Page', 'P90 Duration', 'Status'].map((h) => (
              <th key={h} className="text-left py-2 pr-4 text-muted text-xs tracking-wide">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {tasks.map((t) => (
            <tr
              key={t.id}
              onClick={() => onRowClick(t.id)}
              className="border-b border-border/50 cursor-pointer hover:bg-surface transition-colors"
            >
              <td className="py-2 pr-4 text-text truncate max-w-[200px]">{t.pdf_name}</td>
              <td className="py-2 pr-4 text-muted text-xs">{formatDate(t.created_at)}</td>
              <td className="py-2 pr-4 tabular-nums">{t.total_pages || '—'}</td>
              <td className="py-2 pr-4 tabular-nums">{fmtNum(t.metrics?.total_input_tokens)}</td>
              <td className="py-2 pr-4 tabular-nums">{fmtNum(t.metrics?.total_output_tokens)}</td>
              <td className="py-2 pr-4 tabular-nums">{fmtNum(t.metrics?.avg_tokens_per_page)}</td>
              <td className="py-2 pr-4 tabular-nums">{fmtDuration(t.metrics?.p90_duration_ms)}</td>
              <td className="py-2 pr-4">
                <StatusBadge status={t.status} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    pending: 'text-muted',
    detecting: 'text-blue-400',
    summarizing: 'text-yellow-400',
    done: 'text-green-400',
    failed: 'text-red-400',
  }
  return (
    <span className={`text-xs ${colors[status] ?? 'text-muted'}`}>
      {formatStatus(status)}
    </span>
  )
}
