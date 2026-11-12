import { describe, it, expect } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { NodeEmailStats, NodeStats, NodeType } from '../../../services/api/automation'
import { EmailStepStats } from './EmailStepStats'
import { StepStats } from './StepStats'

const EMAIL: NodeEmailStats = {
  executions: 100,
  queued: 0,
  not_sent: 0,
  sent: 100,
  delivered: 90,
  opened: 30,
  opens_tracked: 60,
  clicked: 5,
  clicks_tracked: 50,
  failed: 1,
  bounced: 2,
  complained: 0,
  unsubscribed: 1
}

const renderEmail = (email: Partial<NodeEmailStats> | undefined, { smtp = false, canReadHistory = true } = {}) =>
  render(
    <I18nProvider i18n={i18n}>
      <EmailStepStats email={email ? { ...EMAIL, ...email } : undefined} canReadHistory={canReadHistory} smtp={smtp} messagesHref="/logs?x=1" />
    </I18nProvider>
  )

const row = (key: string) => document.querySelector(`[data-email-row="${key}"]`)!

describe('EmailStepStats', () => {
  it('shows the four rates, then what went wrong', () => {
    renderEmail({})
    expect(row('sent')).toHaveTextContent('100')
    expect(row('delivered')).toHaveTextContent('90.0%')
    expect(row('opened')).toHaveTextContent('50.0%')
    expect(row('clicked')).toHaveTextContent('10.0%')
    expect(screen.getByText('1 failed · 1 unsubscribe · 2 bounces')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'View messages' })).toHaveAttribute('href', '/logs?x=1')
  })

  it('shows queued and not sent only when there are any', () => {
    const { unmount } = renderEmail({})
    expect(screen.queryByText(/queued/)).not.toBeInTheDocument()
    expect(screen.queryByText(/not sent/)).not.toBeInTheDocument()
    unmount()
    renderEmail({ queued: 2, not_sent: 1 })
    expect(screen.getByText('2 queued')).toBeInTheDocument()
    expect(screen.getByText('1 not sent')).toBeInTheDocument()
  })

  it('reads no delivery from SMTP, and says why', async () => {
    renderEmail({}, { smtp: true })
    expect(row('delivered')).toHaveTextContent('—')
    fireEvent.mouseOver(row('delivered').querySelector('span:last-child')!)
    expect(await screen.findByText('SMTP does not report deliveries')).toBeInTheDocument()
  })

  it('reads zeros when the step sent nothing', () => {
    renderEmail(undefined)
    expect(row('sent')).toHaveTextContent('0')
    expect(screen.getByText('0 failed · 0 unsubscribes · 0 bounces')).toBeInTheDocument()
  })

  it('says the emails need the message history', () => {
    renderEmail({}, { canReadHistory: false })
    expect(screen.getByText("You need access to the message history to see this step's emails.")).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'View messages' })).not.toBeInTheDocument()
  })
})

const stats = (over: Partial<NodeStats> = {}): NodeStats => ({
  node_type: 'delay',
  visits: 10,
  completed: 6,
  failed_attempts: 0,
  skipped: 0,
  skip_reasons: {},
  errors: 0,
  failed_journeys: 0,
  in_progress: 4,
  waiting: 3,
  retrying: 0,
  branches: {},
  ...over
})

const renderStep = (nodeType: NodeType, s: NodeStats, config: Record<string, unknown> = {}) =>
  render(
    <I18nProvider i18n={i18n}>
      <StepStats nodeType={nodeType} config={config} stats={s} />
    </I18nProvider>
  )

describe('StepStats', () => {
  it('shows the journeys of a delay with its waiting ones', () => {
    renderStep('delay', stats())
    expect(screen.getByText('Reached')).toBeInTheDocument()
    expect(screen.getByText('Waiting')).toBeInTheDocument()
    expect(screen.queryByText('Retrying')).not.toBeInTheDocument()
    expect(screen.queryByText('Errors')).not.toBeInTheDocument()
  })

  it('shows retrying, failed in danger and errors when there are any', () => {
    renderStep('add_to_list', stats({ retrying: 2, failed_journeys: 1, errors: 3 }))
    expect(screen.queryByText('Waiting')).not.toBeInTheDocument()
    expect(screen.getByText('Retrying')).toBeInTheDocument()
    expect(screen.getByText('1')).toHaveClass('text-danger-fg')
    expect(screen.getByText('Errors')).toBeInTheDocument()
  })

  it('splits the passes of an A/B test by variant', () => {
    const config = { variants: [{ id: 'a', name: 'A', weight: 50 }, { id: 'b', name: 'B', weight: 50 }] }
    renderStep('ab_test', stats({ branches: { a: 3, b: 1 } }), config)
    const splits = document.querySelector('[data-step-splits]')!
    expect(splits).toHaveTextContent('A (50 %)')
    expect(splits).toHaveTextContent('75.0%')
    expect(splits).toHaveTextContent('25.0%')
  })

  it('has no splits for a single-exit step', () => {
    renderStep('delay', stats())
    expect(document.querySelector('[data-step-splits]')).toBeNull()
  })
})

describe('StepStats of a branch', () => {
  it('shows the passes that took no path, so the shares add up', () => {
    const config = { paths: [{ id: 'p1', name: 'Pro' }] }
    renderStep('branch', stats({ branches: { p1: 3, none: 1 } }), config)
    const splits = document.querySelector('[data-step-splits]')!
    expect(splits).toHaveTextContent('Pro')
    expect(splits).toHaveTextContent('75.0%')
    expect(splits).toHaveTextContent('No path')
    expect(splits).toHaveTextContent('25.0%')
  })
})
