export type Status = 'pending' | 'detecting' | 'summarizing' | 'done' | 'failed'

export type SummaryStatus = 'ok' | 'fallback' | 'failed'

export interface Chapter {
  index: number
  title: string
  start_page: number
  end_page: number
  summary: string
  summary_status?: SummaryStatus
}

export interface LLMRecord {
  phase: string
  chapter_index: number
  batch_start: number
  batch_end: number
  input_tokens: number
  output_tokens: number
  duration_ms: number
}

export interface Stats {
  records: LLMRecord[]
  total_input_tokens: number
  total_output_tokens: number
  avg_tokens_per_page: number
  max_tokens_per_call: number
  total_duration_ms: number
  p90_duration_ms: number
  p99_duration_ms: number
  elapsed_ms: number
}

export interface Task {
  id: string
  status: Status
  progress: number
  message: string
  pdf_name: string
  total_pages: number
  chapters?: Chapter[]
  metrics?: Stats
  error?: string
  created_at: string
  updated_at: string
}

export interface SSEEvent {
  type: string
  data: unknown
}
