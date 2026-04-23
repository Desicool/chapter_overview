import type { Stats } from '../types'

interface Props {
  stats: Stats | undefined
  chapterCount: number
}

function formatDuration(ms: number | undefined): string {
  if (ms === undefined) return '—'
  if (ms >= 60000) {
    const mins = Math.floor(ms / 60000)
    const secs = Math.round((ms % 60000) / 1000)
    return `${mins}m ${secs}s`
  }
  return `${(ms / 1000).toFixed(1)}s`
}

function fmtNum(n: number | undefined | null): string {
  if (n == null) return '—'
  return Math.round(n).toLocaleString()
}

function fmtAvgTokens(n: number | undefined | null): string {
  if (n == null || n === 0) return '—'
  return Math.round(n).toLocaleString()
}

export default function MetricsPanel({ stats, chapterCount }: Props) {
  const avgDurationPerChapter =
    stats && chapterCount > 0
      ? stats.total_duration_ms / chapterCount
      : undefined

  return (
    <div className="bg-surface border border-border rounded p-5 font-mono text-sm">
      {/* Header row */}
      <div className="grid grid-cols-2 gap-x-8 mb-2">
        <span className="text-amber font-semibold tracking-widest text-xs uppercase">Tokens</span>
        <span className="text-amber font-semibold tracking-widest text-xs uppercase">Timing</span>
      </div>

      {/* Divider */}
      <div className="border-t border-border mb-3" />

      {/* Rows */}
      <div className="grid grid-cols-2 gap-x-8 gap-y-1.5">
        {/* Tokens column */}
        <div className="space-y-1.5">
          <MetricRow label="Input" value={fmtNum(stats?.total_input_tokens)} />
          <MetricRow label="Output" value={fmtNum(stats?.total_output_tokens)} />
          <MetricRow label="Avg/page" value={fmtAvgTokens(stats?.avg_tokens_per_page)} />
          <MetricRow label="Max/call" value={fmtNum(stats?.max_tokens_per_call)} />
        </div>

        {/* Timing column */}
        <div className="space-y-1.5">
          <MetricRow label="Total" value={formatDuration(stats?.total_duration_ms)} />
          <MetricRow label="Avg/ch" value={formatDuration(avgDurationPerChapter)} />
          <MetricRow label="P90" value={formatDuration(stats?.p90_duration_ms)} />
          <MetricRow
            label="P99"
            value={formatDuration(stats?.p99_duration_ms)}
            footnote="①"
          />
        </div>
      </div>

      {/* Divider */}
      <div className="border-t border-border mt-3 mb-2" />

      {/* Footnote */}
      <p className="text-muted text-xs">
        ① P99 ≈ max when N &lt; 20 LLM calls
      </p>
    </div>
  )
}

function MetricRow({
  label,
  value,
  footnote,
}: {
  label: string
  value: string
  footnote?: string
}) {
  return (
    <div className="flex justify-between gap-2">
      <span className="text-muted">{label}</span>
      <span className="text-text tabular-nums">
        {value}
        {footnote && <sup className="ml-0.5 text-muted">{footnote}</sup>}
      </span>
    </div>
  )
}
