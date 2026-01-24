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
HTMLCanvasElement.prototype.getContext = function(contextId: string, options?: unknown) {
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
