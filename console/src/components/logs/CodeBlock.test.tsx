import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { CodeBlock } from './CodeBlock'
import { formatCode } from './formatCode'

const copy = vi.hoisted(() => vi.fn().mockResolvedValue(true))
vi.mock('../../lib/copyToClipboard', () => ({ copyToClipboard: copy }))

describe('formatCode', () => {
  it('indents JSON text and values', () => {
    expect(formatCode('{"a":1}')).toBe('{\n  "a": 1\n}')
    expect(formatCode({ a: 1 })).toBe('{\n  "a": 1\n}')
  })

  it('keeps text that is not JSON', () => {
    expect(formatCode('<xml/>')).toBe('<xml/>')
  })
})

describe('CodeBlock', () => {
  it('copies the formatted text', async () => {
    render(
      <App>
        <CodeBlock value='{"a":1}' label="payload" />
      </App>
    )
    fireEvent.click(screen.getByRole('button', { name: 'Copy payload' }))
    await waitFor(() => expect(copy).toHaveBeenCalledWith('{\n  "a": 1\n}'))
  })
})
