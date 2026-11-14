import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { ExportContactsModal } from './ExportContactsModal'
import { contactsApi } from '../../services/api/contacts'

vi.mock('../../services/api/contacts', () => ({
  contactsApi: { list: vi.fn() }
}))

vi.mock('papaparse', () => ({ default: { unparse: vi.fn(() => '') } }))

describe('ExportContactsModal tracking consent filters', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(contactsApi.list).mockResolvedValue({ contacts: [], next_cursor: undefined } as never)
  })

  it('exports only the contacts the consent filters keep, and says so', async () => {
    render(
      <I18nProvider i18n={i18n}>
        <ExportContactsModal
          visible
          onCancel={vi.fn()}
          workspaceId="ws1"
          workspace={undefined}
          filters={{ tracking_opens_consent: 'denied', tracking_clicks_consent: 'unknown' }}
          segmentsData={[]}
          listsData={[]}
        />
      </I18nProvider>
    )

    expect(screen.getByText('Opens tracking consent:', { exact: false })).toBeInTheDocument()
    expect(screen.getByText('Clicks tracking consent:', { exact: false })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /export/i }))

    await waitFor(() => expect(contactsApi.list).toHaveBeenCalled())
    expect(vi.mocked(contactsApi.list).mock.calls[0][0]).toMatchObject({
      tracking_opens_consent: 'denied',
      tracking_clicks_consent: 'unknown'
    })
  })
})

describe('ExportContactsModal view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(contactsApi.list).mockResolvedValue({ contacts: [], next_cursor: undefined } as never)
  })

  it('exports the view on screen: search, segment view and full name', async () => {
    render(
      <I18nProvider i18n={i18n}>
        <ExportContactsModal
          visible
          onCancel={vi.fn()}
          workspaceId="ws1"
          workspace={undefined}
          filters={{ search: 'ann', segment_id: 'pro', full_name: 'Ann Lee' }}
          segmentsData={[{ id: 'pro', name: 'Plan Pro' } as never]}
          listsData={[]}
        />
      </I18nProvider>
    )

    expect(screen.getByText('Segment: Plan Pro', { exact: false })).toBeInTheDocument()
    expect(screen.getByText('Search: ann', { exact: false })).toBeInTheDocument()
    expect(screen.getByText('Full Name: Ann Lee', { exact: false })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /export/i }))

    await waitFor(() => expect(contactsApi.list).toHaveBeenCalled())
    expect(vi.mocked(contactsApi.list).mock.calls[0][0]).toMatchObject({
      search: 'ann',
      segments: ['pro'],
      full_name: 'Ann Lee'
    })
  })
})

describe('ExportContactsModal in French', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    i18n.load('fr', {})
    i18n.activate('fr')
  })

  afterEach(() => {
    i18n.activate('en')
  })

  it('counts the fetched contacts with the French thousands separator', async () => {
    // A first page of 12,345 contacts, then a page that never answers, so the count stays on screen
    const page = Array.from({ length: 12345 }, (_, i) => ({ email: `c${i}@example.com` }))
    vi.mocked(contactsApi.list)
      .mockResolvedValueOnce({ contacts: page, next_cursor: 'next' } as never)
      .mockReturnValueOnce(new Promise(() => {}) as never)

    render(
      <I18nProvider i18n={i18n}>
        <ExportContactsModal
          visible
          onCancel={vi.fn()}
          workspaceId="ws1"
          workspace={undefined}
          filters={{}}
          segmentsData={[]}
          listsData={[]}
        />
      </I18nProvider>
    )

    await userEvent.click(screen.getByRole('button', { name: /export/i }))

    // French groups thousands with a narrow no-break space
    expect(await screen.findByText(/^Fetched 12\s345 contacts\.\.\.$/)).toBeInTheDocument()
  })
})
