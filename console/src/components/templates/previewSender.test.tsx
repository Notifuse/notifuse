import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { BroadcastSenderLine, SenderLine } from './previewSender'
import { broadcastApi } from '../../services/api/broadcast'
import type { Sender } from '../../services/api/workspace'

const state = vi.hoisted(() => ({ permissions: null as null | Record<string, Record<string, boolean>> }))
vi.mock('../../contexts/AuthContext', () => ({
  useWorkspacePermissions: () => ({ permissions: state.permissions, loading: false })
}))
vi.mock('../../services/api/broadcast', () => ({ broadcastApi: { get: vi.fn() } }))

const senders: Sender[] = [
  { id: 'default', name: 'News', email: 'news@example.com', is_default: true },
  { id: 'tpl', name: 'Template', email: 'template@example.com', is_default: false },
  { id: 'var', name: 'Variation', email: 'variation@example.com', is_default: false }
]

const withClient = (ui: ReactNode) =>
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>{ui}</QueryClientProvider>)

describe('SenderLine', () => {
  it("shows the email's sender, else the integration's default", () => {
    const { unmount } = render(<SenderLine senders={senders} senderId="tpl" />)
    expect(screen.getByText(/template@example.com/)).toBeInTheDocument()
    unmount()
    render(<SenderLine senders={senders} senderId="gone" />)
    expect(screen.getByText(/news@example.com/)).toBeInTheDocument()
  })

  it('shows the From name the message was sent with, and the original name', () => {
    render(<SenderLine senders={senders} senderId="tpl" fromName="Ann" />)
    expect(screen.getByText(/Ann <template@example.com>/)).toBeInTheDocument()
    expect(screen.getByText('(original: Template)')).toBeInTheDocument()
  })

  it('says when there is no sender at all', () => {
    render(<SenderLine senders={[]} />)
    expect(screen.getByText('No default sender configured')).toBeInTheDocument()
  })
})

describe('BroadcastSenderLine', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(broadcastApi.get).mockResolvedValue({
      broadcast: {
        test_settings: {
          variations: [
            { variation_name: 'A', template_id: 'tpl-1', sender_id: 'var' },
            { variation_name: 'B', template_id: 'tpl-2' }
          ]
        }
      }
    } as never)
  })

  const line = (templateId = 'tpl-1') =>
    withClient(
      <BroadcastSenderLine
        workspaceId="ws1"
        broadcastId="bc1"
        templateId={templateId}
        senders={senders}
        templateSenderId="tpl"
      />
    )

  it("shows the sender of the broadcast's variation first", async () => {
    state.permissions = { broadcasts: { read: true } }
    line()
    expect(await screen.findByText(/variation@example.com/)).toBeInTheDocument()
    expect(broadcastApi.get).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'bc1' })
  })

  it("shows the template's sender for a variation without one", async () => {
    state.permissions = { broadcasts: { read: true } }
    line('tpl-2')
    await waitFor(() => expect(broadcastApi.get).toHaveBeenCalled())
    expect(screen.getByText(/template@example.com/)).toBeInTheDocument()
  })

  it("reads no broadcast without Broadcasts read, and shows the template's sender", () => {
    state.permissions = { broadcasts: { read: false } }
    line()
    expect(screen.getByText(/template@example.com/)).toBeInTheDocument()
    expect(broadcastApi.get).not.toHaveBeenCalled()
  })
})
