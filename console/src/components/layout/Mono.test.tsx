import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { Mono } from './Mono'

const setClipboard = (writeText: ((text: string) => Promise<void>) | null) =>
  Object.defineProperty(navigator, 'clipboard', {
    value: writeText ? { writeText } : undefined,
    configurable: true,
    writable: true
  })
const setExecCommand = (impl: (() => boolean) | null) =>
  Object.defineProperty(document, 'execCommand', { value: impl ?? undefined, configurable: true, writable: true })

const renderMono = (copy?: boolean) =>
  render(
    <App>
      <Mono copy={copy}>utm_source=newsletter</Mono>
    </App>
  )

describe('Mono', () => {
  afterEach(() => {
    setClipboard(null)
    setExecCommand(null)
  })

  it('shows an identifier in code, with its full text as a title', () => {
    renderMono()
    const code = screen.getByText('utm_source=newsletter')
    expect(code.tagName).toBe('CODE')
    expect(code).toHaveAttribute('title', 'utm_source=newsletter')
    expect(screen.queryByRole('button', { name: 'Copy' })).toBeNull()
  })

  it('copies through the clipboard API', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    setClipboard(writeText)
    renderMono(true)
    fireEvent.click(screen.getByRole('button', { name: 'Copy' }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('utm_source=newsletter'))
    expect(await screen.findByText('Copied')).toBeInTheDocument()
  })

  it('falls back to a selection copy without the clipboard API', async () => {
    const execCommand = vi.fn().mockReturnValue(true)
    setExecCommand(execCommand)
    renderMono(true)
    fireEvent.click(screen.getByRole('button', { name: 'Copy' }))
    await waitFor(() => expect(execCommand).toHaveBeenCalledWith('copy'))
    expect(await screen.findByText('Copied')).toBeInTheDocument()
  })
})
