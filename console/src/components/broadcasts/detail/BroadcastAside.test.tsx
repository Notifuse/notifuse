import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { App as AntApp } from 'antd'
import { BroadcastAside, previewVariationOf, type BroadcastAsideProps } from './BroadcastAside'
import type { Broadcast } from '../../../services/api/broadcast'
import type { Task } from '../../../services/api/task'
import type { Workspace } from '../../../services/api/types'

vi.mock('./TemplatePreviewFrame', () => ({
  TemplatePreviewFrame: ({ template }: { template: { id: string } }) => <div data-testid="frame">{template.id}</div>
}))
vi.mock('../../templates/TemplatePreviewDrawer', () => ({
  default: ({ children }: { children: ReactNode }) => <div data-testid="preview-drawer">{children}</div>
}))

const workspace = {
  id: 'ws1',
  name: 'Acme',
  settings: { marketing_email_provider_id: 'int1', timezone: 'Europe/Paris' },
  integrations: [
    { id: 'int1', email_provider: { senders: [{ id: 's1', name: 'Acme', email: 'hello@acme.test', is_default: true }] } }
  ]
} as unknown as Workspace

const tpl = (id: string, extra: Record<string, unknown> = {}) => ({
  id,
  name: `Template ${id}`,
  email: { subject: `Subject ${id}`, subject_preview: `Preview ${id}`, ...extra }
})

const broadcast = (overrides: Partial<Broadcast> = {}): Broadcast =>
  ({
    id: 'bc1',
    name: 'November news',
    status: 'processed',
    audience: { list: 'l1', segments: ['seg1', 'seg-gone'], exclude_unsubscribed: true },
    schedule: { is_scheduled: false, use_recipient_timezone: false },
    test_settings: {
      enabled: false,
      sample_percentage: 0,
      auto_send_winner: false,
      variations: [{ variation_name: 'A', template_id: 'tpl-a', template: tpl('tpl-a', { reply_to: 'support@acme.test' }) }]
    },
    utm_parameters: { source: 'acme.test', medium: 'email', campaign: 'november_news' },
    created_at: '2026-11-01T10:00:00Z',
    updated_at: '2026-11-01T10:00:00Z',
    started_at: '2026-11-02T10:00:00Z',
    completed_at: '2026-11-02T11:00:00Z',
    test_phase_recipient_count: 0,
    winner_phase_recipient_count: 0,
    ...overrides
  }) as Broadcast

const renderAside = (props: Partial<BroadcastAsideProps> = {}) => {
  const b = props.broadcast ?? broadcast()
  return render(
    <AntApp>
      <BroadcastAside
        workspace={workspace}
        broadcast={b}
        lists={[{ id: 'l1', name: 'Newsletter' }]}
        segments={[{ id: 'seg1', name: 'VIP', color: 'gold' }]}
        previewVariation={previewVariationOf(b)}
        {...props}
      />
    </AntApp>
  )
}

const block = (title: string) => screen.getByRole('heading', { name: title }).closest('section') as HTMLElement

describe('previewVariationOf', () => {
  const ab = broadcast({
    test_settings: {
      enabled: true,
      sample_percentage: 20,
      auto_send_winner: false,
      variations: [
        { variation_name: 'A', template_id: 'tpl-a' },
        { variation_name: 'B', template_id: 'tpl-b' }
      ]
    }
  })
  it('is the winner once there is one', () => {
    expect(previewVariationOf({ ...ab, winning_template: 'tpl-b' })?.template_id).toBe('tpl-b')
    expect(previewVariationOf(ab, 'tpl-b')?.template_id).toBe('tpl-b')
  })
  it('is the first variant otherwise', () => {
    expect(previewVariationOf(ab)?.template_id).toBe('tpl-a')
  })
})

describe('BroadcastAside', () => {
  it('previews the single template with its sender, Reply-To, subject and preview text', () => {
    renderAside()
    const preview = block('Preview')
    expect(within(preview).getByText('Acme <hello@acme.test>')).toBeInTheDocument()
    expect(within(preview).getByText('support@acme.test')).toBeInTheDocument()
    expect(within(preview).getByText('Subject tpl-a')).toBeInTheDocument()
    expect(within(preview).getByText('Preview tpl-a')).toBeInTheDocument()
    expect(within(preview).getByTestId('frame')).toHaveTextContent('tpl-a')
    expect(within(preview).getByRole('button', { name: 'Open' })).toBeInTheDocument()
  })

  it('leaves the sender lines to the A/B panel when a test runs', () => {
    const b = broadcast({
      winning_template: 'tpl-b',
      test_settings: {
        enabled: true,
        sample_percentage: 20,
        auto_send_winner: false,
        variations: [
          { variation_name: 'A', template_id: 'tpl-a', template: tpl('tpl-a') as never },
          { variation_name: 'B', template_id: 'tpl-b', template: tpl('tpl-b') as never }
        ]
      }
    })
    renderAside({ broadcast: b, previewVariation: previewVariationOf(b) })
    const preview = block('Preview')
    expect(within(preview).queryByText('From')).not.toBeInTheDocument()
    expect(within(preview).getByTestId('frame')).toHaveTextContent('tpl-b')
  })

  it('shows the template ID, and why, without Templates read', () => {
    const b = broadcast({
      test_settings: { enabled: false, sample_percentage: 0, auto_send_winner: false, variations: [{ variation_name: 'A', template_id: 'tpl-a' }] }
    })
    renderAside({ broadcast: b, previewVariation: previewVariationOf(b), canReadTemplates: false })
    const preview = block('Preview')
    expect(within(preview).getByText('tpl-a')).toBeInTheDocument()
    expect(within(preview).getByText("You need Templates read permission to see this template's name and preview")).toBeInTheDocument()
  })

  it('says a template missing from the join no longer exists, with Templates read', () => {
    const b = broadcast({
      test_settings: { enabled: false, sample_percentage: 0, auto_send_winner: false, variations: [{ variation_name: 'A', template_id: 'tpl-gone' }] }
    })
    renderAside({ broadcast: b, previewVariation: previewVariationOf(b) })
    const preview = block('Preview')
    expect(within(preview).getByText('tpl-gone')).toBeInTheDocument()
    expect(within(preview).getByText('This template no longer exists')).toBeInTheDocument()
    expect(within(preview).queryByText('No template yet')).toBeNull()
  })

  it('says when there is no template to preview', () => {
    const b = broadcast({ test_settings: { enabled: false, sample_percentage: 0, auto_send_winner: false, variations: [] } })
    renderAside({ broadcast: b, previewVariation: undefined })
    expect(within(block('Preview')).getByText('No template yet')).toBeInTheDocument()
  })

  it('shows the audience: list, segments (unknown ones by id), unsubscribed and schedule timezone', () => {
    renderAside()
    const audience = block('Audience')
    expect(within(audience).getByText('Newsletter')).toBeInTheDocument()
    expect(within(audience).getByText('VIP')).toBeInTheDocument()
    expect(within(audience).getByText('Unknown segment (seg-gone)')).toBeInTheDocument()
    expect(within(audience).getByText('Excluded')).toBeInTheDocument()
    expect(within(audience).getByText('Europe/Paris')).toBeInTheDocument()
  })

  it('says None without segments, Included and the recipient timezone when set', () => {
    renderAside({
      broadcast: broadcast({
        audience: { list: 'l1', exclude_unsubscribed: false },
        schedule: { is_scheduled: true, use_recipient_timezone: true }
      })
    })
    const audience = block('Audience')
    expect(within(audience).getByText('None')).toBeInTheDocument()
    expect(within(audience).getByText('Included')).toBeInTheDocument()
    expect(within(audience).getByText("Each recipient's timezone")).toBeInTheDocument()
  })

  it('shows the UTM parameters, the per-variant utm_content and the tracking mode', () => {
    renderAside()
    const tracking = block('Web tracking')
    expect(within(tracking).getByText('november_news')).toBeInTheDocument()
    expect(within(tracking).getByText('Template ID of each variant')).toBeInTheDocument()
    expect(within(tracking).getByText('Per contact consent')).toBeInTheDocument()
  })

  it('says Not tracked for a broadcast sent without tracking', () => {
    renderAside({ broadcast: broadcast({ tracking_mode: 'no_tracking' }) })
    expect(within(block('Web tracking')).getByText('Not tracked')).toBeInTheDocument()
  })

  it('shows the sending task when one exists', () => {
    const task = {
      id: 't1',
      type: 'send_broadcast',
      status: 'paused',
      progress: 0,
      next_run_after: '2026-11-02T12:00:00Z',
      error_message: 'Provider refused',
      state: {
        message: 'Paused by circuit breaker',
        send_broadcast: { broadcast_id: 'bc1', total_recipients: 200, enqueued_count: 50, failed_count: 3, channel_type: 'email', recipient_offset: 50 }
      }
    } as unknown as Task
    renderAside({ task })
    const block_ = block('Sending task')
    expect(within(block_).getByText('Paused')).toBeInTheDocument()
    expect(within(block_).getByText('25.0%')).toBeInTheDocument()
    expect(within(block_).getByText('Paused by circuit breaker')).toBeInTheDocument()
    expect(within(block_).getByText('3')).toHaveClass('text-danger-fg')
    expect(within(block_).getByText('Provider refused')).toBeInTheDocument()
    expect(within(block_).getByText('send_broadcast')).toBeInTheDocument()
  })

  it('has no task block without a task', () => {
    renderAside({ task: null })
    expect(screen.queryByRole('heading', { name: 'Sending task' })).not.toBeInTheDocument()
  })

  it('lists only the dates the broadcast has, with the pause reason', () => {
    renderAside({ broadcast: broadcast({ paused_at: '2026-11-02T10:30:00Z', pause_reason: 'Bounce rate too high' }) })
    const history = block('History')
    for (const label of ['Created', 'Started', 'Completed', 'Paused']) expect(within(history).getByText(label)).toBeInTheDocument()
    expect(within(history).queryByText('Test sent')).not.toBeInTheDocument()
    expect(within(history).queryByText('Cancelled')).not.toBeInTheDocument()
    expect(within(history).getByText('Bounce rate too high')).toBeInTheDocument()
  })
})
