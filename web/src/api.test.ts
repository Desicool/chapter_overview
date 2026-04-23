import { estimatePerPage } from './api'
import type { LLMRecord } from './types'

function makeRecord(overrides: Partial<LLMRecord> = {}): LLMRecord {
  return {
    phase: 'summarize',
    chapter_index: 0,
    batch_start: 1,
    batch_end: 1,
    input_tokens: 100,
    output_tokens: 0,
    duration_ms: 1000,
    ...overrides,
  }
}

describe('estimatePerPage', () => {
  it('returns empty map for empty records', () => {
    const result = estimatePerPage([])
    expect(result.size).toBe(0)
  })

  it('distributes tokens evenly across pages 1-10 with 100 total tokens', () => {
    const record = makeRecord({
      batch_start: 1,
      batch_end: 10,
      input_tokens: 100,
      output_tokens: 0,
      duration_ms: 1000,
    })
    const result = estimatePerPage([record])
    expect(result.size).toBe(10)
    for (let p = 1; p <= 10; p++) {
      const entry = result.get(p)
      expect(entry).toBeDefined()
      expect(entry!.tokens).toBeCloseTo(10)
      expect(entry!.duration).toBeCloseTo(100)
    }
  })

  it('sums tokens across multiple non-overlapping records', () => {
    const r1 = makeRecord({ batch_start: 1, batch_end: 5, input_tokens: 50, output_tokens: 0, duration_ms: 500 })
    const r2 = makeRecord({ batch_start: 6, batch_end: 10, input_tokens: 100, output_tokens: 0, duration_ms: 1000 })
    const result = estimatePerPage([r1, r2])
    expect(result.size).toBe(10)
    // r1: 50 tokens over 5 pages = 10 per page
    expect(result.get(1)!.tokens).toBeCloseTo(10)
    // r2: 100 tokens over 5 pages = 20 per page
    expect(result.get(6)!.tokens).toBeCloseTo(20)
  })

  it('accumulates tokens from overlapping records', () => {
    // both cover page 5
    const r1 = makeRecord({ batch_start: 1, batch_end: 5, input_tokens: 50, output_tokens: 0, duration_ms: 500 })
    const r2 = makeRecord({ batch_start: 3, batch_end: 7, input_tokens: 50, output_tokens: 0, duration_ms: 500 })
    const result = estimatePerPage([r1, r2])
    // page 5: from r1 = 50/5 = 10, from r2 = 50/5 = 10 → total 20
    expect(result.get(5)!.tokens).toBeCloseTo(20)
    // page 1: only from r1 = 10
    expect(result.get(1)!.tokens).toBeCloseTo(10)
    // page 7: only from r2 = 10
    expect(result.get(7)!.tokens).toBeCloseTo(10)
  })

  it('counts both input and output tokens', () => {
    const record = makeRecord({
      batch_start: 1,
      batch_end: 4,
      input_tokens: 60,
      output_tokens: 40,
      duration_ms: 400,
    })
    const result = estimatePerPage([record])
    // (60 + 40) / 4 = 25 per page
    expect(result.get(1)!.tokens).toBeCloseTo(25)
    // duration: 400 / 4 = 100 per page
    expect(result.get(1)!.duration).toBeCloseTo(100)
  })
})
