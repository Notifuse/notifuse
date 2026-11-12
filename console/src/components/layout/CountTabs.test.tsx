import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { CountTabs } from './CountTabs'

describe('CountTabs', () => {
  const options = [
    { value: 'all', label: 'All', count: 1204 },
    { value: 'draft', label: 'Drafts', count: 2 },
    { value: 'sent', label: 'Sent' }
  ]

  it('labels each option with its formatted count', () => {
    render(<CountTabs options={options} value="all" onChange={() => {}} />)
    expect(screen.getByText('1,204')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText('Sent')).toBeInTheDocument()
  })

  it('reports the option chosen', () => {
    const onChange = vi.fn()
    render(<CountTabs options={options} value="all" onChange={onChange} />)
    fireEvent.click(screen.getByText('Drafts'))
    expect(onChange).toHaveBeenCalledWith('draft')
  })

  it('darkens the count of the chosen option', () => {
    render(<CountTabs options={options} value="draft" onChange={() => {}} />)
    expect(screen.getByText('2').className).toContain('text-ink-2')
    expect(screen.getByText('1,204').className).toContain('text-ink-3')
  })
})
