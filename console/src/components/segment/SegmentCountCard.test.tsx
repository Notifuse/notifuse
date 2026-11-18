import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { SegmentCountCard, type SegmentCountCardProps } from './SegmentCountCard'

const response = { emails: [], total_count: 1288, limit: 100, generated_sql: 'SELECT 1', sql_args: ['%FR%'] }

const renderCard = (props: Partial<SegmentCountCardProps> = {}) => {
  const onCount = vi.fn()
  const result = render(
    <I18nProvider i18n={i18n}>
      <SegmentCountCard refreshing={false} stale={false} blocked={false} canCount={true} onCount={onCount} {...props} />
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
    expect(screen.getByText('7 % of 18,342 contacts')).toBeTruthy()
  })

  it('never says more than the whole workspace, nor 0 % of a few', () => {
    const { rerender } = renderCard({ response: { ...response, total_count: 150 }, totalContacts: 100 })
    expect(screen.getByText('100 % of 100 contacts')).toBeTruthy()
    rerender(
      <I18nProvider i18n={i18n}>
        <SegmentCountCard
          response={{ ...response, total_count: 3 }}
          totalContacts={100000}
          refreshing={false}
          stale={false}
          blocked={false}
          canCount={true}
          onCount={vi.fn()}
        />
      </I18nProvider>
    )
    expect(screen.getByText('< 0.1 % of 100,000 contacts')).toBeTruthy()
  })

  it('opens the query on a click, and keeps it open', async () => {
    renderCard({ response })
    const link = screen.getByText('See the query')
    // A mouse reaches the link before it clicks: a popover that also opened on hover was toggled
    // shut by that click.
    fireEvent.mouseEnter(link)
    await new Promise((resolve) => setTimeout(resolve, 250))
    await userEvent.click(link)
    const sql = await screen.findByText('SELECT 1')
    await waitFor(() => expect(sql.closest('.ant-popover')?.className).not.toContain('ant-popover-hidden'))
  })

  it('dims a count that no longer answers the form, and says why', () => {
    const { container } = renderCard({ response, stale: true, blocked: true })
    expect(container.querySelector('.opacity-40')).toBeTruthy()
    expect(screen.getByText('Complete the condition to refresh the count')).toBeTruthy()
  })

  it('offers no retry while the tree cannot be counted', () => {
    renderCard({ response, error: 'boom', canCount: false })
    expect(screen.queryByText('Retry')).toBeNull()
    expect(screen.getByText('Complete the condition to refresh the count')).toBeTruthy()
  })

  it('reports a failed count with a retry', () => {
    const { onCount } = renderCard({ response, error: 'invalid timeframe_values' })
    fireEvent.click(screen.getByText('Retry'))
    expect(onCount).toHaveBeenCalled()
    expect(screen.getByText('1288 contacts')).toBeTruthy()
  })
})

describe('SegmentCountCard — a contact value that cannot be read', () => {
  it('says so in one line, with the contact and the value on hover, and offers no retry', async () => {
    renderCard({
      error: 'x',
      valueError: {
        column: 'custom_json_1',
        path: ['renewed_at'],
        type: 'date',
        email: 'jane@example.com',
        value: '2026-02-30'
      },
      customFieldLabels: { custom_json_1: 'Billing' }
    })
    expect(screen.getByText("A contact's value can't be read.")).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
    expect(screen.queryByText('The count failed.')).toBeNull()
    await userEvent.hover(screen.getByText("A contact's value can't be read."))
    expect(
      await screen.findByText('jane@example.com has "2026-02-30" in Billing › renewed_at, which is not a date.')
    ).toBeInTheDocument()
  })

  it('words a number, and a value it could not pin on a contact', async () => {
    renderCard({ error: 'x', valueError: { column: 'custom_json_2', path: ['score'], type: 'number' } })
    await userEvent.hover(screen.getByText("A contact's value can't be read."))
    expect(await screen.findByText("A contact's value in Custom JSON 2 › score is not a number.")).toBeInTheDocument()
  })
})
