import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { SegmentCountCard, type SegmentCountCardProps } from './SegmentCountCard'

const response = { emails: [], total_count: 1288, limit: 100, generated_sql: 'SELECT 1', sql_args: ['%FR%'] }

const renderCard = (props: Partial<SegmentCountCardProps> = {}) => {
  const onCount = vi.fn()
  const result = render(
    <I18nProvider i18n={i18n}>
      <SegmentCountCard
        refreshing={false}
        stale={false}
        blocked={false}
        canCount={true}
        onCount={onCount}
        {...props}
      />
    </I18nProvider>
  )
  return { onCount, ...result }
}

describe('SegmentCountCard', () => {
  it('offers to count before anything is counted', () => {
    const { onCount } = renderCard()
    expect(screen.getByText('—')).toBeTruthy()
    // Its own message: the tree's sentence starts with « Contacts matching », translated as a fragment
    expect(screen.getByText('Matching contacts')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Preview' }))
    expect(onCount).toHaveBeenCalled()
  })

  it('cannot count a tree that does not compile', () => {
    renderCard({ canCount: false })
    expect(screen.getByRole('button', { name: 'Preview' })).toBeDisabled()
  })

  it('shows the count and its share of the workspace', () => {
    renderCard({ response, totalContacts: 18342 })
    expect(screen.getByText('1288 contacts')).toBeTruthy()
    expect(screen.getByText('7 % of the 18,342 contacts')).toBeTruthy()
  })

  it('puts the query behind « See the query »', async () => {
    renderCard({ response })
    fireEvent.mouseEnter(screen.getByText('See the query'))
    expect(await screen.findByText('SELECT 1')).toBeTruthy()
  })

  it('dims a count that no longer answers the form, and says why', () => {
    const { container } = renderCard({ response, stale: true, blocked: true })
    expect(container.querySelector('.opacity-40')).toBeTruthy()
    expect(screen.getByText('Complete the condition to refresh the count')).toBeTruthy()
  })

  it('reports a failed count with a retry', () => {
    const { onCount } = renderCard({ response, error: 'invalid timeframe_values' })
    fireEvent.click(screen.getByText('Retry'))
    expect(onCount).toHaveBeenCalled()
    expect(screen.getByText('1288 contacts')).toBeTruthy()
  })
})
