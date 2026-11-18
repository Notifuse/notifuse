import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { StepTrail } from './StepTrail'

const steps = [
  { key: 'file', label: 'File' },
  { key: 'columns', label: 'Columns' },
  { key: 'import', label: 'Import' }
]

const stateOf = (label: string) =>
  screen.getByText(label).closest('[data-step-state]')?.getAttribute('data-step-state')

describe('StepTrail', () => {
  it('marks the steps before the current one done and the ones after it to do', () => {
    render(<StepTrail steps={steps} current={1} ariaLabel="Import steps" />)
    expect(screen.getByRole('list', { name: 'Import steps' })).toBeInTheDocument()
    expect(stateOf('File')).toBe('done')
    expect(stateOf('Columns')).toBe('current')
    expect(stateOf('Import')).toBe('todo')
  })

  it('names the current step for assistive technology, and only that one', () => {
    render(<StepTrail steps={steps} current={1} ariaLabel="Import steps" />)
    const items = screen.getAllByRole('listitem')
    expect(items.map((item) => item.getAttribute('aria-current'))).toEqual([null, 'step', null])
  })

  it('marks every step done once the flow is complete', () => {
    render(<StepTrail steps={steps} current={2} complete ariaLabel="Import steps" />)
    expect([stateOf('File'), stateOf('Columns'), stateOf('Import')]).toEqual(['done', 'done', 'done'])
    expect(screen.getAllByRole('listitem').some((item) => item.hasAttribute('aria-current'))).toBe(false)
  })
})
