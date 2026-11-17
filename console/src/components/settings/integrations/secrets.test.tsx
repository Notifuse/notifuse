import { describe, it, expect } from 'vitest'
import { renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { credentialHint, placeholderHint, useSecretPlaceholder } from './secrets'
import type { Integration } from '../../../services/api/types'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const wrapper = ({ children }: { children: ReactNode }) => <I18nProvider i18n={i18n}>{children}</I18nProvider>

const integration = { credential_hints: { 'anthropic.api_key': 'x9Qe', 'firecrawl.api_key': '' } } as unknown as Integration

describe('credentialHint', () => {
  it('reads the end of a secret the server revealed, and nothing for an empty one', () => {
    expect(credentialHint(integration, 'anthropic.api_key')).toBe('x9Qe')
    expect(credentialHint(integration, 'firecrawl.api_key')).toBeUndefined()
    expect(credentialHint(undefined, 'anthropic.api_key')).toBeUndefined()
  })
})

describe('useSecretPlaceholder', () => {
  it('shows the example on a new integration', () => {
    const { result } = renderHook(() => useSecretPlaceholder(undefined), { wrapper })
    expect(result.current('anthropic.api_key', false, 'sk-ant-...')).toBe('sk-ant-...')
  })

  it('shows the end of the stored secret on an edit', () => {
    const { result } = renderHook(() => useSecretPlaceholder(integration), { wrapper })
    expect(result.current('anthropic.api_key', true, 'sk-ant-...')).toBe(placeholderHint('x9Qe'))
    expect(placeholderHint('x9Qe')).toBe('••••••••x9Qe')
  })

  it('says a stored secret without a hint is kept, and never claims one that is not stored', () => {
    const { result } = renderHook(() => useSecretPlaceholder(integration), { wrapper })
    expect(result.current('firecrawl.api_key', true, 'fc-...')).toBe('Leave blank to keep the current value')
    expect(result.current('firecrawl.api_key', false, 'fc-...')).toBe('fc-...')
  })
})
