import type { Task, LLMRecord } from './types'

const BASE = '/api'

export async function uploadPDF(file: File): Promise<Task> {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch(`${BASE}/tasks`, { method: 'POST', body: form })
  if (!res.ok) throw new Error(`Upload failed: ${res.status}`)
  return res.json()
}

export async function getTask(id: string): Promise<Task> {
  const res = await fetch(`${BASE}/tasks/${id}`)
  if (!res.ok) throw new Error(`Task not found: ${res.status}`)
  return res.json()
}

export async function listTasks(opts?: { status?: string; page?: number; pageSize?: number }): Promise<Task[]> {
  const params = new URLSearchParams()
  if (opts?.status) params.set('status', opts.status)
  if (opts?.page) params.set('page', String(opts.page))
  if (opts?.pageSize) params.set('page_size', String(opts.pageSize))
  const res = await fetch(`${BASE}/tasks?${params}`)
  if (!res.ok) throw new Error(`List failed: ${res.status}`)
  return res.json()
}

export async function deleteTask(id: string): Promise<void> {
  const res = await fetch(`${BASE}/tasks/${id}`, { method: 'DELETE' })
  if (!res.ok) throw new Error(`Delete failed: ${res.status}`)
}

export async function getTaskMetrics(id: string): Promise<LLMRecord[]> {
  const res = await fetch(`${BASE}/tasks/${id}/metrics`)
  if (!res.ok) throw new Error(`Metrics failed: ${res.status}`)
  return res.json()
}

export function subscribeToTask(id: string, onEvent: (e: { type: string; data: unknown }) => void): () => void {
  const es = new EventSource(`${BASE}/tasks/${id}/events`)
  es.onmessage = (e) => {
    try { onEvent(JSON.parse(e.data)) } catch { /* ignore parse errors */ }
  }
  es.onerror = () => es.close()
  return () => es.close()
}

/**
 * Distribute tokens and duration evenly across each page in a record's batch range.
 * Accumulates across overlapping records.
 */
export function estimatePerPage(records: LLMRecord[]): Map<number, { tokens: number; duration: number }> {
  const result = new Map<number, { tokens: number; duration: number }>()

  for (const rec of records) {
    const start = rec.batch_start
    const end = rec.batch_end
    const pageCount = end - start + 1
    const tokensPerPage = (rec.input_tokens + rec.output_tokens) / pageCount
    const durationPerPage = rec.duration_ms / pageCount

    for (let p = start; p <= end; p++) {
      const existing = result.get(p) ?? { tokens: 0, duration: 0 }
      result.set(p, {
        tokens: existing.tokens + tokensPerPage,
        duration: existing.duration + durationPerPage,
      })
    }
  }

  return result
}
