import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { WidgetTabs } from './WidgetTabs'

const TABS = [
  { key: 'landing', label: 'Landing pages' },
  { key: 'exits', label: 'Exits' }
]

describe('WidgetTabs', () => {
  it('renders nothing with a single tab', () => {
    const { container } = render(
      <WidgetTabs tabs={[TABS[0]]} activeKey="landing" onChange={() => {}} />
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('calls onChange with the key of the clicked tab', () => {
    const onChange = vi.fn()
    render(<WidgetTabs tabs={TABS} activeKey="landing" onChange={onChange} />)
    fireEvent.click(screen.getByRole('tab', { name: 'Exits' }))
    expect(onChange).toHaveBeenCalledWith('exits')
  })

  it('marks only the active tab as selected, inside a tablist', () => {
    render(<WidgetTabs tabs={TABS} activeKey="exits" onChange={() => {}} />)
    expect(screen.getByRole('tablist')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Exits' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: 'Landing pages' })).toHaveAttribute(
      'aria-selected',
      'false'
    )
  })

  it('draws the active tab in ink over the primary bar, the others in ink-3', () => {
    render(<WidgetTabs tabs={TABS} activeKey="exits" onChange={() => {}} />)
    expect(screen.getByRole('tab', { name: 'Exits' })).toHaveClass('text-ink', 'border-primary-500')
    expect(screen.getByRole('tab', { name: 'Landing pages' })).toHaveClass('text-ink-3')
  })
})
