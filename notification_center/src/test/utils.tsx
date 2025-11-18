import { render, RenderOptions } from '@testing-library/react'
import { ReactElement } from 'react'

/**
 * Custom render function that wraps components with necessary providers
 */
export function renderWithProviders(
  ui: ReactElement,
  options?: Omit<RenderOptions, 'wrapper'>
) {
  return render(ui, { ...options })
}

/**
 * Mock URL search parameters
 */
export function mockURLSearchParams(params: Record<string, string>) {
  const searchParams = new URLSearchParams(params)
  delete (window as any).location
  ;(window as any).location = {
    search: `?${searchParams.toString()}`,
    href: `http://localhost:3001/?${searchParams.toString()}`,
    origin: 'http://localhost:3001',
    pathname: '/',
  }
}

/**
 * Common test data fixtures
 */
export const testData = {
  validParams: {
    wid: 'workspace-123',
    email: 'test@example.com',
    email_hmac: 'valid-hmac-123',
    lid: 'newsletter',
    lname: 'Newsletter',
    mid: 'message-123',
  },
  previewParams: {
    wid: 'workspace-123',
    email: 'john.doe@example.com',
    email_hmac: 'abc123',
    mid: 'preview',
  },
  contact: {
    id: 'contact-123',
    email: 'test@example.com',
    first_name: 'John',
    last_name: 'Doe',
  },
}

/**
 * Wait for async operations to complete
 */
export const waitForAsync = () => new Promise(resolve => setTimeout(resolve, 0))

