import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'

const { copyToClipboard } = vi.hoisted(() => ({ copyToClipboard: vi.fn() }))
vi.mock('../../lib/copyToClipboard', () => ({ copyToClipboard }))

import { ApiCallPanel } from './ApiCallPanel'
import { curlSnippet, pythonSnippet, typeScriptSnippet } from '../../lib/transactionalApiSnippets'

const notification = { id: 'password_reset' }

const renderPanel = (onOpenAll = vi.fn()) => {
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <ApiCallPanel notification={notification} workspaceId="ws_123" onOpenAll={onOpenAll} />
      </App>
    </I18nProvider>
  )
  return onOpenAll
}

// One div per line, so line breaks do not survive in the text: compared without whitespace.
const shownCode = () => (screen.getByTestId('api-call-code').textContent ?? '').replace(/\s+/g, '')
const flat = (code: string) => code.replace(/\s+/g, '')

beforeEach(() => {
  window.API_ENDPOINT = 'https://api.example.test'
  copyToClipboard.mockReset()
  copyToClipboard.mockResolvedValue(true)
})

describe('ApiCallPanel', () => {
  it('shows cURL, then TypeScript and Python, from the shared samples', () => {
    renderPanel()
    expect(shownCode()).toBe(flat(curlSnippet(notification, 'ws_123')))

    fireEvent.click(screen.getByText('TypeScript'))
    expect(shownCode()).toBe(flat(typeScriptSnippet(notification, 'ws_123')))

    fireEvent.click(screen.getByText('Python'))
    expect(shownCode()).toBe(flat(pythonSnippet(notification, 'ws_123')))
  })

  it('copies the code shown', async () => {
    renderPanel()
    fireEvent.click(screen.getByText('Python'))
    fireEvent.click(screen.getByRole('button', { name: 'Copy' }))

    await waitFor(() => expect(copyToClipboard).toHaveBeenCalledWith(pythonSnippet(notification, 'ws_123')))
    expect(await screen.findByText('Code copied')).toBeInTheDocument()
  })

  it('opens every language', () => {
    const onOpenAll = renderPanel()
    fireEvent.click(screen.getByRole('button', { name: 'All languages' }))
    expect(onOpenAll).toHaveBeenCalled()
  })
})
