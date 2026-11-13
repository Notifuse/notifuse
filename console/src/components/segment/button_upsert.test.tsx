import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual('@tanstack/react-router')
  return {
    ...actual,
    useParams: () => ({ workspaceId: 'ws1' }),
    useNavigate: () => vi.fn(),
    useMatch: () => false
  }
})
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', settings: { timezone: 'UTC', custom_field_labels: {} } }] })
}))
vi.mock('../../services/api/list', () => ({ listsApi: { list: vi.fn().mockResolvedValue({ lists: [] }) } }))
vi.mock('../../services/api/template', () => ({
  templatesApi: { list: vi.fn().mockResolvedValue({ templates: [] }) }
}))
vi.mock('../../services/api/segment', async () => {
  const actual = await vi.importActual('../../services/api/segment')
  return { ...actual, previewSegment: vi.fn(), getSegment: vi.fn().mockRejectedValue(new Error('not found')) }
})

import { DrawerSegment } from './button_upsert'

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>
)

describe('DrawerSegment', () => {
  it('opens without a trigger and asks its caller to close', async () => {
    const setVisible = vi.fn()
    render(<DrawerSegment canReadTemplates setDrawserVisible={setVisible} />, { wrapper })
    expect(await screen.findByText('New segment')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(setVisible).toHaveBeenCalledWith(false)
  })
})
