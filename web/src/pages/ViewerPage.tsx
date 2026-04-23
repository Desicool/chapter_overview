import { useParams, Link } from 'react-router-dom'
import PDFViewer from '../components/PDFViewer'

export default function ViewerPage() {
  const { id, page } = useParams<{ id: string; page: string }>()
  const initialPage = parseInt(page ?? '1', 10)

  if (!id) {
    return (
      <div className="min-h-screen bg-bg flex items-center justify-center text-muted font-mono text-sm">
        Invalid task ID.
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-bg">
      {/* Top bar */}
      <div className="px-6 py-3 border-b border-border flex items-center gap-4">
        <Link
          to={`/tasks/${id}`}
          className="font-mono text-xs text-muted hover:text-amber transition-colors"
        >
          ← Back to chapters
        </Link>
        <span className="font-mono text-xs text-amber tabular-nums">
          Starting at page {isNaN(initialPage) ? 1 : initialPage}
        </span>
      </div>

      {/* PDF Viewer */}
      <PDFViewer taskId={id} initialPage={isNaN(initialPage) ? 1 : initialPage} />
    </div>
  )
}
