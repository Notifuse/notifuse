import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import { ChangeVariationSenderModal } from './ChangeVariationSenderModal'
import { broadcastApi, type Broadcast } from '../../services/api/broadcast'
import type { Workspace } from '../../services/api/types'

vi.mock('../../services/api/template', () => ({
  templatesApi: {
    list: vi.fn().mockResolvedValue({ templates: [{ id: 'tmpl-a', email: { sender_id: 's1' } }] })
  }
}))

vi.mock('../../services/api/broadcast', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../services/api/broadcast')>()
  return { ...actual, broadcastApi: { update: vi.fn() } }
})

const workspace = {
  id: 'ws1',
  settings: { marketing_email_provider_id: 'mkt' },
  integrations: [
    {
      id: 'mkt',
      email_provider: {
        senders: [
          { id: 's1', name: 'Default', email: 'hello@example.com', is_default: true },
          { id: 's2', name: 'Promos', email: 'promos@example.com', is_default: false }
        ]
      }
    }
  ]
} as unknown as Workspace

const makeBroadcast = (overrides: Partial<Broadcast> = {}): Broadcast =>
  ({
    id: 'bc1',
    workspace_id: 'ws1',
    name: 'Spring sale',
    status: 'paused',
    // Every server pause sets it.
    paused_at: '2026-11-10T09:00:00Z',
    audience: { list: 'list1', segments: [], exclude_unsubscribed: true },
    test_settings: {
      enabled: true,
      sample_percentage: 20,
      auto_send_winner: false,
      variations: [
        {
          variation_name: 'A',
          template_id: 'tmpl-a',
          sender_id: 'gone',
          template: { id: 'tmpl-a', name: 'Joined copy' }
        },
        { variation_name: 'B', template_id: 'tmpl-b', sender_id: 's1' }
      ]
    },
    ...overrides
  }) as unknown as Broadcast

const renderModal = (broadcast: Broadcast, disabled = false) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AntApp>
        <ChangeVariationSenderModal workspace={workspace} broadcast={broadcast} disabled={disabled} />
      </AntApp>
    </QueryClientProvider>
  )
}

describe('ChangeVariationSenderModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(broadcastApi.update as Mock).mockResolvedValue({})
  })

  it('offers nothing when every chosen sender is still in the integration', () => {
    const broadcast = makeBroadcast()
    broadcast.test_settings.variations[0].sender_id = 's2'
    renderModal(broadcast)
    expect(screen.queryByRole('button', { name: 'Change sender' })).not.toBeInTheDocument()
  })

  it('offers nothing on a broadcast that is not paused', () => {
    renderModal(makeBroadcast({ status: 'scheduled' }))
    expect(screen.queryByRole('button', { name: 'Change sender' })).not.toBeInTheDocument()
  })

  it('offers nothing once every recipient is queued: the queued emails keep their sender', () => {
    renderModal(makeBroadcast({ completed_at: '2026-11-10T10:00:00Z' }))
    expect(screen.queryByRole('button', { name: 'Change sender' })).not.toBeInTheDocument()
  })

  it('is disabled without write permission', () => {
    renderModal(makeBroadcast(), true)
    expect(screen.getByRole('button', { name: 'Change sender' })).toBeDisabled()
  })

  it('replaces the missing sender and keeps the other variations as stored', async () => {
    renderModal(makeBroadcast())
    await userEvent.click(screen.getByRole('button', { name: 'Change sender' }))

    // Only the variation whose sender left the integration is asked for.
    expect(await screen.findByText('Sender of variation A')).toBeInTheDocument()
    expect(screen.queryByText('Sender of variation B')).not.toBeInTheDocument()
    expect(screen.getByText('gone (deleted)')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('combobox'))
    await userEvent.click(await screen.findByText('Promos <promos@example.com>'))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(broadcastApi.update).toHaveBeenCalledTimes(1))
    const request = (broadcastApi.update as Mock).mock.calls[0][0]
    expect(request).toMatchObject({ workspace_id: 'ws1', id: 'bc1', name: 'Spring sale' })
    // The A/B settings are sent back as stored; what the request leaves out is kept.
    expect(request.test_settings).toMatchObject({
      enabled: true,
      sample_percentage: 20,
      auto_send_winner: false
    })
    expect(request).not.toHaveProperty('schedule')
    expect(request).not.toHaveProperty('data_feed')
    expect(request.test_settings.variations).toEqual([
      { variation_name: 'A', template_id: 'tmpl-a', sender_id: 's2', metrics: undefined },
      { variation_name: 'B', template_id: 'tmpl-b', sender_id: 's1', metrics: undefined }
    ])
  })

  it('numbers unnamed variations, as the console saves them', async () => {
    const broadcast = makeBroadcast()
    broadcast.test_settings.variations[0].variation_name = ''
    broadcast.test_settings.variations[1].variation_name = ''
    broadcast.test_settings.variations[1].sender_id = 'also-gone'
    renderModal(broadcast)
    await userEvent.click(screen.getByRole('button', { name: 'Change sender' }))

    expect(await screen.findByText('Sender of variation 1')).toBeInTheDocument()
    expect(screen.getByText('Sender of variation 2')).toBeInTheDocument()
  })

  it("offers nothing when only a losing variation's sender is gone", () => {
    renderModal(makeBroadcast({ winning_template: 'tmpl-b' }))
    expect(screen.queryByRole('button', { name: 'Change sender' })).not.toBeInTheDocument()
  })

  it('asks for every missing sender once a sent variation needs one', async () => {
    const broadcast = makeBroadcast({ winning_template: 'tmpl-b' })
    broadcast.test_settings.variations[1].sender_id = 'also-gone'
    renderModal(broadcast)
    await userEvent.click(screen.getByRole('button', { name: 'Change sender' }))

    expect(await screen.findByText('Sender of variation A')).toBeInTheDocument()
    expect(screen.getByText('Sender of variation B')).toBeInTheDocument()
  })

  it('refuses to save while the deleted sender is still selected', async () => {
    renderModal(makeBroadcast())
    await userEvent.click(screen.getByRole('button', { name: 'Change sender' }))
    await userEvent.click(await screen.findByRole('button', { name: 'Save' }))

    expect(
      await screen.findByText(
        "This sender is no longer one of the marketing email integration's. Choose another one or clear the field to use the template's."
      )
    ).toBeInTheDocument()
    expect(broadcastApi.update).not.toHaveBeenCalled()
  })
})
