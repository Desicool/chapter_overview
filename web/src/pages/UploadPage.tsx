import { useState, useRef, DragEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { uploadPDF } from '../api'

export default function UploadPage() {
  const navigate = useNavigate()
  const inputRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [dragging, setDragging] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const LARGE_FILE_BYTES = 100 * 1024 * 1024 // 100 MB

  function handleFile(f: File) {
    if (!f.name.endsWith('.pdf')) {
      setError('Please select a PDF file.')
      return
    }
    setError(null)
    setFile(f)
  }

  function handleDrop(e: DragEvent<HTMLDivElement>) {
    e.preventDefault()
    setDragging(false)
    const f = e.dataTransfer.files[0]
    if (f) handleFile(f)
  }

  function handleDragOver(e: DragEvent<HTMLDivElement>) {
    e.preventDefault()
    setDragging(true)
  }

  function handleDragLeave() {
    setDragging(false)
  }

  function handleInputChange(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0]
    if (f) handleFile(f)
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!file) return
    setUploading(true)
    setError(null)
    try {
      const task = await uploadPDF(file)
      navigate(`/tasks/${task.id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Upload failed')
      setUploading(false)
    }
  }

  function formatSize(bytes: number): string {
    if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
    return `${(bytes / 1024).toFixed(0)} KB`
  }

  return (
    <div className="min-h-screen bg-bg flex flex-col">
      {/* Top bar */}
      <div className="flex justify-end px-8 py-4">
        <Link
          to="/stats"
          className="font-mono text-xs text-muted hover:text-amber transition-colors"
        >
          Statistics →
        </Link>
      </div>

      {/* Main content */}
      <div className="flex-1 flex flex-col items-center justify-center px-6 pb-16">
        <h1 className="font-serif text-5xl text-text mb-3 text-center leading-tight">
          Reveal the Structure
        </h1>
        <p className="font-sans text-muted text-base mb-10 text-center max-w-md">
          Upload a PDF to detect chapters and generate summaries
        </p>

        <form onSubmit={handleSubmit} className="w-full max-w-lg">
          {/* Drop zone */}
          <div
            role="button"
            tabIndex={0}
            onClick={() => inputRef.current?.click()}
            onKeyDown={(e) => e.key === 'Enter' && inputRef.current?.click()}
            onDrop={handleDrop}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            className={`
              border-2 border-dashed rounded-lg p-12 flex flex-col items-center justify-center cursor-pointer
              transition-all duration-200
              ${dragging
                ? 'border-amber animate-pulse bg-amber/5'
                : 'border-border hover:border-amber/50 bg-surface'}
            `}
          >
            <input
              ref={inputRef}
              type="file"
              accept=".pdf"
              className="hidden"
              onChange={handleInputChange}
            />
            <svg
              className={`w-12 h-12 mb-4 ${dragging ? 'text-amber' : 'text-muted'}`}
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={1.5}
                d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"
              />
            </svg>

            {file ? (
              <div className="text-center">
                <p className="font-mono text-sm text-text">{file.name}</p>
                <p className="font-mono text-xs text-muted mt-1">{formatSize(file.size)}</p>
                {file.size > LARGE_FILE_BYTES && (
                  <p className="font-sans text-xs text-amber mt-2">
                    Large file — processing may take several minutes
                  </p>
                )}
              </div>
            ) : (
              <div className="text-center">
                <p className="font-sans text-sm text-muted">
                  Drop a PDF here, or{' '}
                  <span className="text-amber underline">browse</span>
                </p>
              </div>
            )}
          </div>

          {/* Error */}
          {error && (
            <p className="mt-3 font-mono text-xs text-red-400">{error}</p>
          )}

          {/* Submit */}
          <button
            type="submit"
            disabled={!file || uploading}
            className={`
              mt-6 w-full py-3 rounded font-mono text-sm font-medium transition-all duration-200
              ${!file || uploading
                ? 'bg-surface text-muted cursor-not-allowed border border-border'
                : 'bg-amber text-bg hover:bg-amber/90 active:scale-[0.98]'}
            `}
          >
            {uploading ? 'Uploading…' : 'Process PDF'}
          </button>
        </form>
      </div>
    </div>
  )
}
