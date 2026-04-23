import { render, screen } from '@testing-library/react'
import MetricsPanel from '../MetricsPanel'
import type { Stats } from '../../types'

const mockStats: Stats = {
  records: [],
  total_input_tokens: 12500,
  total_output_tokens: 3200,
  avg_tokens_per_page: 320,
  max_tokens_per_call: 4096,
  total_duration_ms: 75000,
  p90_duration_ms: 62000,
  p99_duration_ms: 74000,
}

describe('MetricsPanel', () => {
  it('renders "—" for all values when stats is undefined', () => {
    const { container } = render(<MetricsPanel stats={undefined} chapterCount={5} />)
    // All value cells should show —
    const dashes = container.querySelectorAll('.tabular-nums')
    const texts = Array.from(dashes).map((el) => el.textContent)
    expect(texts.every((t) => t?.includes('—'))).toBe(true)
  })

  it('renders formatted token counts when stats provided', () => {
    render(<MetricsPanel stats={mockStats} chapterCount={5} />)
    // Input tokens: 12,500
    expect(screen.getByText('12,500')).toBeTruthy()
    // Output tokens: 3,200
    expect(screen.getByText('3,200')).toBeTruthy()
    // Avg/page: 320
    expect(screen.getByText('320')).toBeTruthy()
  })

  it('formats duration > 60s as "Xm Ys"', () => {
    // chapterCount=3 → avg/ch = 75000/3 = 25000ms → "25.0s" (different from total)
    render(<MetricsPanel stats={mockStats} chapterCount={3} />)
    // total_duration_ms = 75000 → "1m 15s"
    expect(screen.getAllByText('1m 15s').length).toBeGreaterThan(0)
    // p90_duration_ms = 62000 → "1m 2s"
    expect(screen.getAllByText('1m 2s').length).toBeGreaterThan(0)
  })

  it('formats duration < 60s as "X.Xs"', () => {
    const shortStats: Stats = { ...mockStats, total_duration_ms: 12500, p90_duration_ms: 8000, p99_duration_ms: 11000 }
    render(<MetricsPanel stats={shortStats} chapterCount={2} />)
    // 12500ms / 2 chapters = 6250ms → "6.3s"
    expect(screen.getByText('12.5s')).toBeTruthy()
  })
})
