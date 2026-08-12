import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { CodeSnippet } from './CodeSnippet'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const CODE = '<script src="https://example.com/na.js"></script>'

const renderSnippet = () =>
  render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <CodeSnippet code={CODE} language="markup" />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )

/** jsdom ships neither, so both halves of the copy path have to be installed. */
const setClipboard = (writeText: ((text: string) => Promise<void>) | null) => {
  Object.defineProperty(navigator, 'clipboard', {
    value: writeText ? { writeText } : undefined,
    configurable: true,
    writable: true
  })
}

const setExecCommand = (impl: (() => boolean) | null) => {
  Object.defineProperty(document, 'execCommand', {
    value: impl ?? undefined,
    configurable: true,
    writable: true
  })
}

describe('CodeSnippet', () => {
  beforeEach(() => {
    setClipboard(null)
    setExecCommand(null)
  })

  afterEach(() => {
    Reflect.deleteProperty(navigator, 'clipboard')
    Reflect.deleteProperty(document, 'execCommand')
  })

  it('copies through the async clipboard when the origin is secure', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    setClipboard(writeText)
    renderSnippet()

    fireEvent.click(screen.getByRole('button', { name: 'Copy' }))

    await waitFor(() => expect(screen.getByRole('button', { name: 'Copied' })).toBeInTheDocument())
    expect(writeText).toHaveBeenCalledWith(CODE)
  })

  it('falls back to a selection copy when the clipboard API is absent', async () => {
    // An insecure origin — a self-hosted console on a bare LAN address.
    const execCommand = vi.fn().mockReturnValue(true)
    setExecCommand(execCommand)
    renderSnippet()

    fireEvent.click(screen.getByRole('button', { name: 'Copy' }))

    await waitFor(() => expect(screen.getByRole('button', { name: 'Copied' })).toBeInTheDocument())
    expect(execCommand).toHaveBeenCalledWith('copy')
    expect(screen.queryByText('Failed to copy to clipboard')).not.toBeInTheDocument()
  })

  it('falls back when the clipboard API rejects', async () => {
    setClipboard(vi.fn().mockRejectedValue(new DOMException('denied', 'NotAllowedError')))
    const execCommand = vi.fn().mockReturnValue(true)
    setExecCommand(execCommand)
    renderSnippet()

    fireEvent.click(screen.getByRole('button', { name: 'Copy' }))

    await waitFor(() => expect(screen.getByRole('button', { name: 'Copied' })).toBeInTheDocument())
    expect(execCommand).toHaveBeenCalledWith('copy')
  })

  it('leaves the textarea it borrows out of the document', async () => {
    setExecCommand(vi.fn().mockReturnValue(true))
    renderSnippet()

    fireEvent.click(screen.getByRole('button', { name: 'Copy' }))

    await waitFor(() => expect(screen.getByRole('button', { name: 'Copied' })).toBeInTheDocument())
    expect(document.querySelectorAll('textarea')).toHaveLength(0)
  })

  it('reports failure only when both paths are unavailable', async () => {
    setExecCommand(vi.fn().mockReturnValue(false))
    renderSnippet()

    fireEvent.click(screen.getByRole('button', { name: 'Copy' }))

    await waitFor(() =>
      expect(screen.getByText('Failed to copy to clipboard')).toBeInTheDocument()
    )
    expect(screen.getByRole('button', { name: 'Copy' })).toBeInTheDocument()
  })
})
