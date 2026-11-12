import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { AbTestPanel, type AbTestPanelProps } from './AbTestPanel'
import type { Broadcast } from '../../../services/api/broadcast'
import type { Workspace } from '../../../services/api/types'
import type { UserPermissions } from '../../../services/api/workspace'

vi.mock('../BroadcastAnalyticsLinks', () => ({
  BroadcastAnalyticsLinks: ({ variationTemplateId }: { variationTemplateId?: string }) => (
    <span data-testid="analytics-links">{variationTemplateId}</span>
  )
}))
vi.mock('../../templates/TemplatePreviewDrawer', () => ({
  default: ({ children }: { children: ReactNode }) => <div data-testid="preview-drawer">{children}</div>
}))

const workspace = {
  id: 'ws1',
  name: 'Acme',
  settings: { marketing_email_provider_id: 'int1', timezone: 'UTC' },
  integrations: [
    {
      id: 'int1',
      email_provider: {
        senders: [
          { id: 's-default', name: 'Acme', email: 'hello@acme.test', is_default: true },
          { id: 's-news', name: 'Acme News', email: 'news@acme.test', is_default: false }
        ]
      }
    }
  ]
} as unknown as Workspace

const permissions = {
  broadcasts: { read: true, write: true },
  templates: { read: true, write: true },
  transactional: { read: true, write: true }
} as unknown as UserPermissions

const template = (id: string, name: string, extra: Record<string, unknown> = {}) => ({
  id,
  name,
  email: { subject: `Subject ${name}`, subject_preview: `Preview ${name}`, ...extra }
})

const broadcast = (overrides: Partial<Broadcast> = {}): Broadcast =>
  ({
    id: 'bc1',
    workspace_id: 'ws1',
    name: 'November news',
    status: 'test_completed',
    audience: { list: 'l1', exclude_unsubscribed: true },
    schedule: { is_scheduled: false, use_recipient_timezone: false },
    test_settings: {
      enabled: true,
      sample_percentage: 20,
      auto_send_winner: true,
      auto_send_winner_metric: 'open_rate',
      test_duration_hours: 4,
      variations: [
        { variation_name: 'Short subject', template_id: 'tpl-a', template: template('tpl-a', 'Alpha') },
        {
          variation_name: 'Long subject',
          template_id: 'tpl-b',
          sender_id: 's-news',
          template: template('tpl-b', 'Beta', { reply_to: 'support@acme.test' })
        }
      ]
    },
    test_phase_recipient_count: 200,
    winner_phase_recipient_count: 800,
    created_at: '2026-11-01T10:00:00Z',
    updated_at: '2026-11-01T10:00:00Z',
    started_at: '2026-11-02T10:00:00Z',
    ...overrides
  }) as Broadcast

const stats = (sent: number, opened: number, clicked: number, opensTracked = sent, clicksTracked = sent) => ({
  total_sent: sent,
  total_delivered: sent,
  total_opened: opened,
  total_clicked: clicked,
  total_opens_tracked: opensTracked,
  total_clicks_tracked: clicksTracked,
  total_bounced: 0,
  total_complained: 0,
  total_failed: 0,
  total_unsubscribed: 0
})

const renderPanel = (props: Partial<AbTestPanelProps> = {}) => {
  const onPickWinner = vi.fn()
  const onSendTest = vi.fn()
  render(
    <AbTestPanel
      workspace={workspace}
      broadcast={broadcast()}
      variationStats={{ 'tpl-a': stats(100, 40, 10), 'tpl-b': stats(100, 30, 5) } as AbTestPanelProps['variationStats']}
      permissions={permissions}
      onPickWinner={onPickWinner}
      onSendTest={onSendTest}
      {...props}
    />
  )
  return { onPickWinner, onSendTest }
}

const rowOf = (text: string) => screen.getByText(text).closest('tr') as HTMLElement

describe('AbTestPanel', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders nothing without an A/B test', () => {
    const { container } = render(
      <AbTestPanel
        workspace={workspace}
        broadcast={broadcast({ test_settings: { enabled: false, sample_percentage: 0, auto_send_winner: false, variations: [] } })}
        variationStats={{}}
        permissions={permissions}
        onPickWinner={vi.fn()}
        onSendTest={vi.fn()}
      />
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('letters the variants in order, with their names, templates and subjects', () => {
    renderPanel()
    const a = rowOf('Short subject')
    expect(within(a).getByText('A')).toBeInTheDocument()
    expect(within(a).getByText('Alpha')).toBeInTheDocument()
    expect(within(a).getByText('Subject Alpha')).toBeInTheDocument()
    expect(within(a).getByText('Preview Alpha')).toBeInTheDocument()
    expect(within(rowOf('Long subject')).getByText('B')).toBeInTheDocument()
  })

  it('shows each variant’s sender: default, chosen on the broadcast with its Reply-To, or deleted', () => {
    renderPanel({
      broadcast: broadcast({
        test_settings: {
          ...broadcast().test_settings,
          variations: [
            ...broadcast().test_settings.variations,
            { variation_name: 'Gone', template_id: 'tpl-c', sender_id: 's-old', template: template('tpl-c', 'Gamma') as never }
          ]
        }
      })
    })
    expect(within(rowOf('Short subject')).getByText('Acme <hello@acme.test>')).toBeInTheDocument()
    expect(within(rowOf('Short subject')).queryByText('chosen on the broadcast')).not.toBeInTheDocument()
    const b = rowOf('Long subject')
    expect(within(b).getByText('Acme News <news@acme.test>')).toBeInTheDocument()
    expect(within(b).getByText('chosen on the broadcast')).toBeInTheDocument()
    expect(within(b).getByText('Reply-To: support@acme.test')).toBeInTheDocument()
    expect(within(rowOf('Gone')).getByText('s-old (deleted)')).toBeInTheDocument()
  })

  it('names a template by its ID without Templates read, and keeps Untitled with it', () => {
    const noTemplates = broadcast({
      test_settings: {
        ...broadcast().test_settings,
        variations: [
          { variation_name: 'Short subject', template_id: 'tpl-a' },
          { variation_name: 'Long subject', template_id: 'tpl-b' }
        ]
      }
    })
    const { unmount } = render(
      <AbTestPanel
        workspace={workspace}
        broadcast={noTemplates}
        variationStats={{}}
        permissions={{ ...permissions, templates: { read: false, write: false } } as unknown as UserPermissions}
        onPickWinner={vi.fn()}
        onSendTest={vi.fn()}
      />
    )
    expect(within(rowOf('Short subject')).getByRole('cell', { name: 'tpl-a' })).toBeInTheDocument()
    expect(screen.queryByText('Untitled')).not.toBeInTheDocument()
    unmount()
    renderPanel({ broadcast: noTemplates })
    expect(screen.getAllByText('Untitled')).toHaveLength(2)
  })

  it('says Default sender when the integration has none for the variant', () => {
    renderPanel({ workspace: { ...workspace, integrations: [] } as unknown as Workspace })
    expect(within(rowOf('Short subject')).getByText('Default sender')).toBeInTheDocument()
  })

  it('shows sent count and rates from the variation stats', () => {
    renderPanel()
    const a = rowOf('Short subject')
    expect(within(a).getByText('100')).toBeInTheDocument()
    expect(within(a).getByText('40.0%')).toBeInTheDocument()
    expect(within(a).getByText('10.0%')).toBeInTheDocument()
  })

  it('prefers live test results over the variation stats', () => {
    renderPanel({
      testResults: {
        broadcast_id: 'bc1',
        status: 'test_completed',
        is_auto_send_winner: true,
        variation_results: {
          'tpl-a': { template_id: 'tpl-a', template_name: 'Alpha', recipients: 50, delivered: 50, opens: 25, clicks: 5, open_rate: 0.5, click_rate: 0.1 }
        }
      }
    })
    const a = rowOf('Short subject')
    expect(within(a).getByText('50')).toBeInTheDocument()
    expect(within(a).getByText('50.0%')).toBeInTheDocument()
  })

  it('flags a rate computed over under half the recipients', () => {
    renderPanel({ variationStats: { 'tpl-a': stats(100, 10, 2, 40, 40) } as AbTestPanelProps['variationStats'] })
    expect(within(rowOf('Short subject')).getAllByText('40% tracked')).toHaveLength(2)
  })

  it('hides the result columns before anything is sent', () => {
    renderPanel({ broadcast: broadcast({ status: 'draft' }) })
    expect(screen.queryByRole('columnheader', { name: 'Sent' })).not.toBeInTheDocument()
    expect(screen.queryByRole('columnheader', { name: 'Opens' })).not.toBeInTheDocument()
  })

  it('marks the winner, and the recommended variant while none is picked', () => {
    renderPanel({
      testResults: { broadcast_id: 'bc1', status: 'test_completed', is_auto_send_winner: false, variation_results: {}, recommended_winner: 'tpl-b' }
    })
    expect(within(rowOf('Long subject')).getByText('Recommended')).toBeInTheDocument()
    expect(screen.queryByText('Winner')).not.toBeInTheDocument()
  })

  it('shows the winner pill on the winning template, not the recommendation', () => {
    renderPanel({
      broadcast: broadcast({ status: 'winner_selected', winning_template: 'tpl-a' }),
      testResults: { broadcast_id: 'bc1', status: 'winner_selected', is_auto_send_winner: false, variation_results: {}, recommended_winner: 'tpl-b' }
    })
    expect(within(rowOf('Short subject')).getByText('Winner')).toBeInTheDocument()
    expect(screen.queryByText('Recommended')).not.toBeInTheDocument()
  })

  it('offers Pick as winner when the test is completed and passes the variant to the caller', async () => {
    const { onPickWinner } = renderPanel()
    await userEvent.click(within(rowOf('Long subject')).getByRole('button', { name: 'Pick as winner' }))
    expect(onPickWinner).toHaveBeenCalledWith('tpl-b', 'Beta')
  })

  it('disables Pick as winner without write permission', () => {
    renderPanel({ permissions: { ...permissions, broadcasts: { read: true, write: false } } as unknown as UserPermissions })
    for (const button of screen.getAllByRole('button', { name: 'Pick as winner' })) expect(button).toBeDisabled()
  })

  it('offers no Pick as winner outside test_completed', () => {
    renderPanel({ broadcast: broadcast({ status: 'testing' }) })
    expect(screen.queryByRole('button', { name: 'Pick as winner' })).not.toBeInTheDocument()
  })

  it('sends a test of the variant’s template', async () => {
    const { onSendTest } = renderPanel()
    const button = rowOf('Long subject').querySelector('.fa-paper-plane')?.closest('button') as HTMLElement
    await userEvent.click(button)
    expect(onSendTest).toHaveBeenCalledWith(expect.objectContaining({ id: 'tpl-b' }))
  })

  it('scopes the web analytics links to each variant', () => {
    renderPanel()
    expect(screen.getAllByTestId('analytics-links').map((n) => n.textContent)).toEqual(['tpl-a', 'tpl-b'])
  })

  it('summarises the settings: automatic winner metric and duration', () => {
    renderPanel()
    expect(screen.getByText('Sample 20 % · winner on open rate after 4 h')).toBeInTheDocument()
  })

  it('summarises a manual pick and the winner phase once sent', () => {
    renderPanel({
      broadcast: broadcast({
        status: 'processed',
        winning_template: 'tpl-a',
        winner_sent_at: '2026-11-02T15:30:00Z',
        test_settings: { ...broadcast().test_settings, auto_send_winner: false }
      })
    })
    expect(screen.getByText(/^Sample 20 % · winner picked manually · sent to the remaining 800 at /)).toBeInTheDocument()
  })

  it('says when a running test ends', () => {
    const started = new Date(Date.now() - 60 * 60 * 1000).toISOString()
    renderPanel({ broadcast: broadcast({ status: 'testing', test_sent_at: started }) })
    expect(screen.getByText(/· Test ends in /)).toBeInTheDocument()
  })
})
