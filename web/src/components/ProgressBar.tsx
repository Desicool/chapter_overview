import type { Status } from '../types'

interface Props {
  value: number // 0-1
  status: Status
}

const statusConfig: Record<Status, { label: string; color: string }> = {
  pending: { label: 'Pending', color: 'bg-muted text-text' },
  detecting: { label: 'Detecting chapters', color: 'bg-blue-800 text-blue-200' },
  summarizing: { label: 'Summarizing', color: 'bg-yellow-800 text-yellow-200' },
  done: { label: 'Done', color: 'bg-green-800 text-green-200' },
  failed: { label: 'Failed', color: 'bg-red-800 text-red-200' },
}

export default function ProgressBar({ value, status }: Props) {
  const pct = Math.min(100, Math.max(0, value * 100))
  const { label, color } = statusConfig[status]

  return (
    <div>
      {/* Amber fill bar */}
      <div className="w-full h-1 bg-surface">
        <div
          className="h-1 bg-amber transition-all duration-500"
          style={{ width: `${pct}%` }}
        />
      </div>

      {/* Status pill */}
      <div className="flex items-center gap-2 mt-2">
        <span
          className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-mono ${color}`}
        >
          {label}
        </span>
        <span className="font-mono text-xs text-muted">{Math.round(pct)}%</span>
      </div>
    </div>
  )
}
