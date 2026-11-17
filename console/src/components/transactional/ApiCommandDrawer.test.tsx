import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { ApiCommandDrawer } from './ApiCommandDrawer'
import {
  curlSnippet,
  goSnippet,
  javaSnippet,
  pythonSnippet,
  smtpPayloadSnippet,
  typeScriptSnippet
} from '../../lib/transactionalApiSnippets'
import type { TransactionalNotification } from '../../services/api/transactional_notifications'

const notification = { id: 'password_reset', name: 'Password reset' } as TransactionalNotification

const renderDrawer = (onClose = vi.fn()) => {
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <ApiCommandDrawer open onClose={onClose} notification={notification} workspaceId="ws_123" />
      </App>
    </I18nProvider>
  )
  return { onClose }
}

/** The sample shown in the open tab, without its line numbers. */
function shownCode(): string {
  const pre = screen.getByTestId('api-command-code').cloneNode(true) as HTMLElement
  pre.querySelectorAll('[data-line-number]').forEach((number) => number.remove())
  // An empty line holds its newline as its only token.
  return Array.from(pre.children, (line) => (line.textContent ?? '').replace(/\n$/, '')).join('\n')
}

const tabNames = () => screen.getAllByRole('tab').map((tab) => tab.textContent)

let writeText: ReturnType<typeof vi.fn>

beforeEach(() => {
  window.API_ENDPOINT = 'https://api.example.test'
  window.SMTP_BRIDGE_ENABLED = false
  writeText = vi.fn().mockResolvedValue(undefined)
  Object.assign(navigator, { clipboard: { writeText } })
})

afterEach(() => {
  window.SMTP_BRIDGE_ENABLED = false
})

describe('ApiCommandDrawer', () => {
  it('opens as a 520px side panel, not a modal, named after the notification', () => {
    renderDrawer()
    expect(document.querySelector('.ant-modal')).toBeNull()
    expect(document.querySelector<HTMLElement>('.ant-drawer-content-wrapper')!.style.width).toBe('520px')
    expect(screen.getByRole('heading', { level: 2, name: 'API command' })).toBeInTheDocument()
    const header = document.querySelector('[data-sidepanel-header]') as HTMLElement
    expect(header).toHaveTextContent('Password reset')
    expect(header).toHaveTextContent('password_reset')
    // The panel has no address of its own to copy.
    expect(screen.queryByRole('button', { name: 'Copy link' })).toBeNull()
  })

  it('offers the SMTP bridge only when it is enabled', () => {
    renderDrawer()
    expect(tabNames()).toEqual(['cURL', 'TypeScript', 'Python', 'Go', 'Java'])
  })

  it('adds the SMTP tab when the bridge is enabled', () => {
    window.SMTP_BRIDGE_ENABLED = true
    renderDrawer()
    expect(tabNames()).toEqual(['cURL', 'TypeScript', 'Python', 'Go', 'Java', 'SMTP'])
  })

  it.each([
    ['cURL', curlSnippet, 'Use this curl command to send a transactional notification via API:'],
    ['TypeScript', typeScriptSnippet, 'Send a transactional notification using TypeScript/JavaScript:'],
    ['Python', pythonSnippet, 'Send a transactional notification using Python:'],
    ['Go', goSnippet, 'Send a transactional notification using Go:'],
    ['Java', javaSnippet, 'Send a transactional notification using Java:']
  ])('shows the shared %s sample unchanged, with its help', (tab, build, intro) => {
    renderDrawer()
    fireEvent.click(screen.getByRole('tab', { name: tab }))
    expect(shownCode()).toBe(build(notification, 'ws_123'))
    expect(screen.getByText(intro)).toBeInTheDocument()
    expect(
      screen.getByText("If the contact email doesn't exist in your workspace, it will be automatically created.")
    ).toBeInTheDocument()
    expect(screen.getByText('All contact fields are optional except email.')).toBeInTheDocument()
  })

  it('shows the SMTP connection details and payload', () => {
    window.SMTP_BRIDGE_ENABLED = true
    window.SMTP_BRIDGE_DOMAIN = 'smtp.example.test'
    window.SMTP_BRIDGE_PORT = 2525
    window.SMTP_BRIDGE_TLS_MODE = 'implicit'
    renderDrawer()
    fireEvent.click(screen.getByRole('tab', { name: 'SMTP' }))

    expect(shownCode()).toBe(smtpPayloadSnippet(notification, 'ws_123'))
    expect(screen.getByText('smtp.example.test')).toBeInTheDocument()
    expect(screen.getByText('2525')).toBeInTheDocument()
    expect(screen.getByText('Implicit TLS (SMTPS)')).toBeInTheDocument()
    expect(screen.getByText('Your workspace API key')).toBeInTheDocument()
    expect(screen.getByText('Supports reply_to, cc, bcc, and attachments')).toBeInTheDocument()
  })

  it('copies the shared sample of the open tab', async () => {
    renderDrawer()

    fireEvent.click(screen.getByRole('tab', { name: 'Python' }))
    fireEvent.click(screen.getByRole('button', { name: /Copy Python Code/ }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(pythonSnippet(notification, 'ws_123')))
    expect(await screen.findByText('Python code copied to clipboard!')).toBeInTheDocument()
  })

  it('copies the SMTP JSON payload from the SMTP tab', async () => {
    window.SMTP_BRIDGE_ENABLED = true
    renderDrawer()

    fireEvent.click(screen.getByRole('tab', { name: 'SMTP' }))
    fireEvent.click(screen.getByRole('button', { name: /Copy JSON Payload Code/ }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(smtpPayloadSnippet(notification, 'ws_123')))
  })

  it('says when the copy fails', async () => {
    // jsdom has no execCommand either, so the selection fallback fails too.
    writeText.mockRejectedValue(new Error('denied'))
    renderDrawer()

    fireEvent.click(screen.getByRole('button', { name: /Copy cURL Code/ }))
    expect(await screen.findByText('Failed to copy to clipboard')).toBeInTheDocument()
  })

  it('closes from the Close button, the close icon and Escape', () => {
    const { onClose } = renderDrawer()
    const footer = document.querySelector('[data-sidepanel-footer]') as HTMLElement
    const closes = screen.getAllByRole('button', { name: 'Close' })
    fireEvent.click(closes.find((button) => footer.contains(button)) as HTMLElement)
    expect(onClose).toHaveBeenCalledTimes(1)
    fireEvent.click(closes.find((button) => !footer.contains(button)) as HTMLElement)
    expect(onClose).toHaveBeenCalledTimes(2)
    fireEvent.keyDown(document.querySelector('.ant-drawer-section') ?? document.activeElement ?? document.body, {
      key: 'Escape'
    })
    expect(onClose).toHaveBeenCalledTimes(3)
  })
})

describe('ApiCommandDrawer for a notification not created yet', () => {
  it('shows the identifier typed and the data given', () => {
    const draft = { id: 'order_shipped', name: 'Order shipped', data: { order: { number: 'A-1042' } } }
    render(
      <I18nProvider i18n={i18n}>
        <App>
          <ApiCommandDrawer open onClose={vi.fn()} notification={draft} workspaceId="ws_123" />
        </App>
      </I18nProvider>
    )
    expect(shownCode()).toBe(curlSnippet(draft, 'ws_123'))
    expect(shownCode()).toContain('"number": "A-1042"')
    expect(screen.getByText('Order shipped')).toBeInTheDocument()
  })
})
