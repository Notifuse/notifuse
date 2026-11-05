import { describe, it, expect, vi, beforeEach } from 'vitest'
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
