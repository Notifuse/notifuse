import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { TestEmailModal } from './TestEmailModal'
import { emailService } from '../../../services/api/email'
import type { EmailProvider } from '../../../services/api/types'

vi.mock('../../../services/api/email', () => ({ emailService: { testProvider: vi.fn() } }))

i18n.loadAndActivate({ locale: 'en', messages: {} })

const provider = { kind: 'sendgrid', senders: [], rate_limit_per_minute: 25 } as EmailProvider

const show = (onClose = vi.fn()) => {
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <TestEmailModal workspaceId="ws1" target={{ provider }} onClose={onClose} />
      </App>
    </I18nProvider>
  )
  return onClose
}

beforeEach(() => vi.clearAllMocks())

describe('TestEmailModal', () => {
  it('sends nothing without an address', () => {
    show()
    expect(screen.getByRole('button', { name: 'Send test email' })).toBeDisabled()
  })

  it('sends a provider not saved yet without an integration id, and closes', async () => {
    vi.mocked(emailService.testProvider).mockResolvedValue({ success: true })
    const onClose = show()
    fireEvent.change(screen.getByLabelText('Send to'), { target: { value: 'me@acme.com' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send test email' }))

    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(emailService.testProvider).toHaveBeenCalledWith('ws1', provider, 'me@acme.com', undefined)
  })

  it('stays open with the provider error', async () => {
    vi.mocked(emailService.testProvider).mockResolvedValue({ success: false, error: 'bad key' })
    const onClose = show()
    fireEvent.change(screen.getByLabelText('Send to'), { target: { value: 'me@acme.com' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send test email' }))

    expect(await screen.findByText('Failed to send test email: bad key')).toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()
  })
})
