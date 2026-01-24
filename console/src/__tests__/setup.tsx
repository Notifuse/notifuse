import { afterEach, vi } from 'vitest'
import { cleanup } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { ReactNode } from 'react'

// Initialize i18n for tests
i18n.load('en', {})
i18n.activate('en')

// Mock @lingui/react/macro since it requires Babel transformation
// The macro transforms t`text` into i18n._('text') at build time
// This mock simulates the real behavior by using i18n._ for translations
vi.mock('@lingui/react/macro', () => ({
  useLingui: () => ({
    t: (strings: TemplateStringsArray, ...values: unknown[]) => {
      // Reconstruct the template literal to get the message ID
      const messageId = strings.reduce((result, str, idx) => {
        return result + str + (values[idx] !== undefined ? `{${idx}}` : '')
      }, '')
      // Use i18n._ to get the translation (simulates real macro behavior)
      // The values are passed as an object with numeric keys
      const valuesObj = values.reduce((acc, val, idx) => {
        acc[idx] = val
        return acc
      }, {} as Record<number, unknown>)
      return i18n._(messageId, valuesObj)
    },
    i18n,
  }),
  Trans: ({ children }: { children: ReactNode }) => children,
}))

// Test wrapper with i18n support
export function TestI18nWrapper({ children }: { children: ReactNode }) {
  return <I18nProvider i18n={i18n}>{children}</I18nProvider>
}

// Mock window.matchMedia for Ant Design
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn()
  }))
})

// Mock Ant Design message component
vi.mock('antd', async () => {
  const actual = await vi.importActual('antd')
  return {
    ...actual,
    message: {
      success: vi.fn(),
      error: vi.fn(),
      info: vi.fn(),
      warning: vi.fn(),
      loading: vi.fn()
    }
  }
})

// Setup mocks
vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual('@tanstack/react-router')
  return {
    ...actual,
    useNavigate: () => vi.fn(),
    useMatch: () => false
  }
})

// Clean up after each test
afterEach(() => {
  cleanup()
})
