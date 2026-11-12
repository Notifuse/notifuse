import { describe, it, expect, vi, afterEach } from 'vitest'
import { copyToClipboard } from './copyToClipboard'

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

describe('copyToClipboard', () => {
  afterEach(() => {
    setClipboard(null)
    setExecCommand(null)
  })

  it('writes through the clipboard API when there is one', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    setClipboard(writeText)
    const execCommand = vi.fn().mockReturnValue(true)
    setExecCommand(execCommand)

    await expect(copyToClipboard('abc')).resolves.toBe(true)
    expect(writeText).toHaveBeenCalledWith('abc')
    expect(execCommand).not.toHaveBeenCalled()
  })

  it('falls back to a selection copy without the clipboard API (a console served over http)', async () => {
    const execCommand = vi.fn().mockReturnValue(true)
    setExecCommand(execCommand)

    await expect(copyToClipboard('abc')).resolves.toBe(true)
    expect(execCommand).toHaveBeenCalledWith('copy')
    expect(document.querySelectorAll('textarea')).toHaveLength(0)
  })

  it('falls back when the clipboard API rejects', async () => {
    setClipboard(vi.fn().mockRejectedValue(new DOMException('denied', 'NotAllowedError')))
    const execCommand = vi.fn().mockReturnValue(true)
    setExecCommand(execCommand)

    await expect(copyToClipboard('abc')).resolves.toBe(true)
    expect(execCommand).toHaveBeenCalledWith('copy')
  })

  it('reports a failure when neither path copies', async () => {
    setExecCommand(vi.fn().mockReturnValue(false))
    await expect(copyToClipboard('abc')).resolves.toBe(false)
  })

  it('reports a failure when the fallback throws', async () => {
    setExecCommand(
      vi.fn(() => {
        throw new Error('nope')
      })
    )
    await expect(copyToClipboard('abc')).resolves.toBe(false)
  })
})
