import { describe, it, expect, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { generateMessageId } from '@lingui/message-utils/generateMessageId'
import { useLingui } from '@lingui/react/macro'

// The shared mock of the Lingui macro (setup.tsx), which every component test goes through.
describe('the Lingui macro mock', () => {
  afterEach(() => {
    i18n.activate('en')
  })

  it('keeps the same t between renders, as the real one does', () => {
    const { result, rerender } = renderHook(() => useLingui().t)
    const first = result.current
    rerender()
    expect(result.current).toBe(first)
  })

  it('resolves a descriptor with its context and its values', () => {
    i18n.load('fr', {
      [generateMessageId('Open', 'action')]: 'Ouvrir',
      Open: 'Ouvertures',
      'Hello {name}': 'Bonjour {name}'
    })
    i18n.activate('fr')
    const { result } = renderHook(() => useLingui().t)
    const t = result.current
    expect(t({ message: 'Open', context: 'action' })).toBe('Ouvrir')
    expect(t({ message: 'Open' })).toBe('Ouvertures')
    expect(t({ message: 'Hello {name}', values: { name: 'Ada' } } as never)).toBe('Bonjour Ada')
  })
})
