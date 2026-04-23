import { useEffect, useRef, useState } from 'react'
import { GlobalWorkerOptions, getDocument } from 'pdfjs-dist'
import workerSrc from 'pdfjs-dist/build/pdf.worker.min.mjs?url'

GlobalWorkerOptions.workerSrc = workerSrc

interface Props {
  taskId: string
  initialPage: number
}

export default function PDFViewer({ taskId, initialPage }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [currentPage, setCurrentPage] = useState(initialPage)
  const [totalPages, setTotalPages] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const pdfRef = useRef<any>(null)
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const renderTaskRef = useRef<any>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)

    getDocument(`/api/tasks/${taskId}/pdf`).promise.then((pdf) => {
      if (cancelled) return
      pdfRef.current = pdf
      setTotalPages(pdf.numPages)
      setLoading(false)
    }).catch((err: Error) => {
      if (cancelled) return
      setError(err.message)
      setLoading(false)
    })

    return () => { cancelled = true }
  }, [taskId])

  useEffect(() => {
    const pdf = pdfRef.current
    const canvas = canvasRef.current
    if (!pdf || !canvas || loading) return

    const pageNum = Math.min(Math.max(1, currentPage), totalPages || 1)

    // Cancel any in-progress render
    if (renderTaskRef.current) {
      renderTaskRef.current.cancel()
      renderTaskRef.current = null
    }

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    pdf.getPage(pageNum).then((page: any) => {
      const viewport = page.getViewport({ scale: 1.5 })
      canvas.width = viewport.width
      canvas.height = viewport.height
      const ctx = canvas.getContext('2d')!
      const renderTask = page.render({ canvasContext: ctx, viewport })
      renderTaskRef.current = renderTask
      return renderTask.promise
    }).catch(() => { /* render cancelled or failed */ })
  }, [currentPage, totalPages, loading])

  function handlePageInput(e: React.ChangeEvent<HTMLInputElement>) {
    const v = parseInt(e.target.value, 10)
    if (!isNaN(v)) setCurrentPage(v)
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-64 text-red-400 font-mono text-sm">
        Failed to load PDF: {error}
      </div>
    )
  }

  return (
    <div className="flex flex-col items-center gap-4 py-6 bg-bg min-h-screen">
      {/* Controls */}
      <div className="flex items-center gap-3 bg-surface border border-border rounded px-4 py-2">
        <button
          onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
          disabled={currentPage <= 1}
          className="text-amber hover:text-amber/70 disabled:text-muted font-mono text-sm transition-colors"
        >
          ← Prev
        </button>

        <span className="font-mono text-muted text-xs">Page</span>
        <input
          type="number"
          min={1}
          max={totalPages || 1}
          value={currentPage}
          onChange={handlePageInput}
          className="w-12 bg-bg border border-border rounded px-1.5 py-0.5 font-mono text-sm text-amber text-center"
        />
        <span className="font-mono text-muted text-xs">/ {totalPages || '—'}</span>

        <button
          onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
          disabled={currentPage >= totalPages}
          className="text-amber hover:text-amber/70 disabled:text-muted font-mono text-sm transition-colors"
        >
          Next →
        </button>
      </div>

      {/* Canvas */}
      {loading ? (
        <div className="shimmer w-[595px] h-[841px] rounded" />
      ) : (
        <canvas
          ref={canvasRef}
          className="rounded shadow-xl border border-border max-w-full"
        />
      )}
    </div>
  )
}
