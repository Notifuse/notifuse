import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { ApiCommandModal } from './ApiCommandModal'
import { pythonSnippet } from '../../lib/transactionalApiSnippets'
import type { TransactionalNotification } from '../../services/api/transactional_notifications'

const notification = { id: 'password_reset', name: 'Password reset' } as TransactionalNotification

const renderModal = () =>
  render(
    <I18nProvider i18n={i18n}>
      <ApiCommandModal open onClose={() => {}} notification={notification} workspaceId="ws_123" />
    </I18nProvider>
  )

beforeEach(() => {
  window.API_ENDPOINT = 'https://api.example.test'
  window.SMTP_BRIDGE_ENABLED = false
})

describe('ApiCommandModal', () => {
  it('offers the SMTP bridge only when it is enabled', () => {
    renderModal()
    expect(screen.getAllByRole('tab').map((tab) => tab.textContent)).toEqual([
      'cURL',
      'TypeScript',
      'Python',
      'Go',
      'Java'
    ])
  })

  it('adds the SMTP tab when the bridge is enabled', () => {
    window.SMTP_BRIDGE_ENABLED = true
    renderModal()
    expect(screen.getAllByRole('tab').map((tab) => tab.textContent)).toEqual([
      'cURL',
      'TypeScript',
      'Python',
      'Go',
      'Java',
      'SMTP'
    ])
  })

  it('copies the shared sample of the open tab', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })
    renderModal()

    fireEvent.click(screen.getByRole('tab', { name: 'Python' }))
    fireEvent.click(screen.getByRole('button', { name: /Copy Python Code/ }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(pythonSnippet(notification, 'ws_123')))
  })
})
