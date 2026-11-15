import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { StatsControl } from './StatsControl'
import type { StatsPeriod } from './nodeStatsState'

const renderControl = (props: Partial<Parameters<typeof StatsControl>[0]> = {}) => {
  const onToggle = vi.fn()
  const onPeriodChange = vi.fn()
  render(
    <I18nProvider i18n={i18n}>
      <StatsControl
        on={false}
        onToggle={onToggle}
        period={'last30' as StatsPeriod}
        onPeriodChange={onPeriodChange}
        {...props}
      />
    </I18nProvider>
  )
  return { onToggle, onPeriodChange }
}

describe('StatsControl', () => {
  it('turns the stats on, without a period while off', () => {
    const { onToggle } = renderControl()
    expect(screen.queryByRole('radio')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('switch', { name: 'Show stats' }))
    expect(onToggle).toHaveBeenCalledWith(true, expect.anything())
  })

  it('offers 7 days, 30 days and all time while on, and reports the choice', () => {
    const { onPeriodChange } = renderControl({ on: true })
    expect(screen.getByRole('switch', { name: 'Show stats' })).toBeChecked()
    expect(screen.getAllByRole('radio').map((r) => r.closest('label')?.textContent)).toEqual([
      '7 d',
      '30 d',
      'All time'
    ])
    expect(screen.getByRole('radio', { name: '30 d' })).toBeChecked()
    fireEvent.click(screen.getByRole('radio', { name: '7 d' }))
    expect(onPeriodChange).toHaveBeenCalledWith('last7')
  })

  it.each(['Save the automation first', 'No journeys yet'])('is locked with its reason: %s', async (reason) => {
    renderControl({ disabledReason: reason })
    const toggle = screen.getByRole('switch', { name: 'Show stats' })
    expect(toggle).toBeDisabled()
    fireEvent.mouseEnter(toggle.parentElement!)
    expect(await screen.findByText(reason)).toBeInTheDocument()
  })
})
