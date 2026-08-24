import { afterAll, afterEach, vi } from 'vitest'
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
      const valuesObj = values.reduce<Record<string, unknown>>((acc, val, idx) => {
        acc[idx] = val
        return acc
      }, {})
      return i18n._(messageId, valuesObj)
    },
    i18n,
  }),
  Trans: ({ children }: { children: ReactNode }) => children,
  // Plural macro fallback: pick `one` for 1, `other` otherwise, and replace `#`
  // with the value (matches real macro behavior well enough for tests).
  Plural: ({ value, one, other }: { value: number; one?: string; other: string }) => {
    const template = value === 1 && one ? one : other
    return template.replace(/#/g, String(value))
  },
}))

// Mock @lingui/core/macro for the same reason as the JSX macro above: nothing in the vitest
// pipeline runs the Babel plugin, so the package resolves to its runtime stub, which throws on
// any call. The real macro turns msg`…` into a message descriptor, naming each placeholder after
// the label in `${{ name: value }}`; reproduce enough of one that i18n._() renders it.
vi.mock('@lingui/core/macro', () => ({
  msg: (strings: TemplateStringsArray, ...placeholders: unknown[]) => {
    const names = placeholders.map((placeholder, idx) =>
      placeholder && typeof placeholder === 'object'
        ? Object.keys(placeholder)[0]
        : String(idx)
    )
    const message = strings.reduce(
      (result, str, idx) => result + str + (idx < names.length ? `{${names[idx]}}` : ''),
      ''
    )
    const values = placeholders.reduce<Record<string, unknown>>((acc, placeholder, idx) => {
      acc[names[idx]] =
        placeholder && typeof placeholder === 'object'
          ? Object.values(placeholder)[0]
          : placeholder
      return acc
    }, {})
    return { id: message, message, values }
  },
}))

// Test wrapper with i18n support
export function TestI18nWrapper({ children }: { children: ReactNode }) {
  return <I18nProvider i18n={i18n}>{children}</I18nProvider>
}

// Mock localStorage (jsdom doesn't provide full implementation)
const localStorageMock = (() => {
  let store: Record<string, string> = {}
  return {
    getItem: vi.fn((key: string) => store[key] || null),
    setItem: vi.fn((key: string, value: string) => {
      store[key] = value
    }),
    removeItem: vi.fn((key: string) => {
      delete store[key]
    }),
    clear: vi.fn(() => {
      store = {}
    })
  }
})()

Object.defineProperty(window, 'localStorage', { value: localStorageMock })

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

// Mock ResizeObserver for Ant Design components that measure themselves
// (Input.TextArea with showCount/autoSize, Select dropdowns); jsdom has none.
globalThis.ResizeObserver =
  globalThis.ResizeObserver ??
  class {
    observe() {}
    unobserve() {}
    disconnect() {}
  }

// Mock window.getComputedStyle for Ant Design components
window.getComputedStyle = () => {
  return {
    getPropertyValue: () => '',
    width: '0px',
    height: '0px',
    display: 'block',
    overflow: 'hidden',
    overflowY: 'hidden',
    overflowX: 'hidden',
    position: 'static',
    margin: '0px',
    padding: '0px'
  } as unknown as CSSStyleDeclaration
}

// Mock HTMLCanvasElement.getContext for emoji-related packages
const originalGetContext = HTMLCanvasElement.prototype.getContext
HTMLCanvasElement.prototype.getContext = function(
  this: HTMLCanvasElement,
  contextId: string,
  options?: unknown
) {
  if (contextId === '2d') {
    return {
      canvas: this,
      fillRect: vi.fn(),
      clearRect: vi.fn(),
      getImageData: vi.fn().mockReturnValue({ data: new Uint8ClampedArray(4), width: 1, height: 1 }),
      putImageData: vi.fn(),
      createImageData: vi.fn().mockReturnValue({ data: new Uint8ClampedArray(4), width: 1, height: 1 }),
      setTransform: vi.fn(),
      drawImage: vi.fn(),
      save: vi.fn(),
      fillText: vi.fn(),
      restore: vi.fn(),
      beginPath: vi.fn(),
      moveTo: vi.fn(),
      lineTo: vi.fn(),
      closePath: vi.fn(),
      stroke: vi.fn(),
      translate: vi.fn(),
      scale: vi.fn(),
      rotate: vi.fn(),
      arc: vi.fn(),
      fill: vi.fn(),
      measureText: vi.fn().mockReturnValue({ width: 0 }),
      transform: vi.fn(),
      rect: vi.fn(),
      clip: vi.fn(),
      font: '',
      textAlign: 'start',
      textBaseline: 'alphabetic',
      fillStyle: '',
      strokeStyle: ''
    } as unknown as CanvasRenderingContext2D
  }
  return originalGetContext?.call(this, contextId, options) ?? null
} as typeof HTMLCanvasElement.prototype.getContext

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

// Let anything still scheduled fire before vitest tears the environment down.
//
// vitest.config.ts sets no `pool`, so `isolate: true` disposes the jsdom
// environment after EVERY file, and antd's Form.Item debounce arms a 10ms timer
// in @rc-component/util's useDelayState that the hook never cancels on unmount —
// it exposes cancelPending and calls it from nowhere. cleanup() cannot help: it
// unmounts the tree, and the timer was never registered against it. When that
// timer lands after teardown it throws "window is not defined" as an unhandled
// error, and vitest fails the whole run regardless of assertions.
//
// A real-time wait, not fake timers: the point is to let the real 10ms elapse.
// It runs after the file's last test and last cleanup(), so no test can observe
// it, and 63 files x 50ms is inside run-to-run noise.
//
// This neutralises the leak rather than removing it. Removing it means patching
// useDelayState to cancel on unmount, upstream.
afterAll(async () => {
  await new Promise((resolve) => setTimeout(resolve, 50))
})
