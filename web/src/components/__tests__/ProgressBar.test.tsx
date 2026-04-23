import { render, screen } from '@testing-library/react'
import ProgressBar from '../ProgressBar'

describe('ProgressBar', () => {
  it('renders amber fill at 50% width', () => {
    const { container } = render(<ProgressBar value={0.5} status="detecting" />)
    const fill = container.querySelector('.bg-amber') as HTMLElement
    expect(fill).toBeTruthy()
    expect(fill.style.width).toBe('50%')
  })

  it('shows correct status pill text for summarizing', () => {
    render(<ProgressBar value={0.7} status="summarizing" />)
    expect(screen.getByText('Summarizing')).toBeTruthy()
  })

  it('shows correct status pill text for pending', () => {
    render(<ProgressBar value={0} status="pending" />)
    expect(screen.getByText('Pending')).toBeTruthy()
  })

  it('done status has green pill class', () => {
    const { container } = render(<ProgressBar value={1} status="done" />)
    const pill = container.querySelector('.bg-green-800')
    expect(pill).toBeTruthy()
  })

  it('clamps fill to 100% when value > 1', () => {
    const { container } = render(<ProgressBar value={1.5} status="done" />)
    const fill = container.querySelector('.bg-amber') as HTMLElement
    expect(fill.style.width).toBe('100%')
  })
})
