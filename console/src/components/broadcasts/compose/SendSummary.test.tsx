import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import dayjs from '../../../lib/dayjs'
import { SendSummary } from './SendSummary'
import { errorCount, summaryChecks, type SummaryInput } from './sendSummaryChecks'
import { newDraft, type ComposeValues } from './composeValues'
import type { Template } from '../../../services/api/template'
import type { Workspace } from '../../../services/api/types'

const workspace = { id: 'ws1', settings: { timezone: 'UTC' } } as unknown as Workspace
const now = dayjs('2026-11-12T10:00:00Z')

const withLink = { id: 'tpl-a', name: 'Alpha', email: { editor_mode: 'code', mjml_source: '<a href="{{ unsubscribe_url }}">x</a>' } } as unknown as Template
const withoutLink = {
  id: 'tpl-b',
  name: 'Beta',
  email: { editor_mode: 'code', mjml_source: '<p>no link</p>' },
  translations: { fr: { email: { editor_mode: 'code', mjml_source: '<p>non</p>' } } }
} as unknown as Template

const values = (over: Partial<ComposeValues> = {}): ComposeValues => ({
  ...newDraft('News', workspace),
  list: 'l1',
  variants: [{ variation_name: 'A', template_id: 'tpl-a' }],
  ...over
})

const input = (over: Partial<SummaryInput> = {}): SummaryInput => ({
  values: values(),
  estimate: { available: true, count: 1200, counting: false, error: false },
  templates: { 'tpl-a': withLink, 'tpl-b': withoutLink },
  hasMarketingProvider: true,
  senderIds: ['s1'],
  issue: null,
  booked: false,
  now,
  ...over
})

const level = (checks: ReturnType<typeof summaryChecks>, key: string) => checks.find((c) => c.key === key)?.level

describe('summaryChecks', () => {
  it('is all ok for a complete draft sent now', () => {
    const checks = summaryChecks(input())
    expect(checks.map((c) => [c.key, c.level])).toEqual([
      ['audience', 'ok'],
      ['content', 'ok'],
      ['when', 'ok']
    ])
    expect(errorCount(checks)).toBe(0)
  })

  it('errs without a list, and warns when the audience counts nobody', () => {
    expect(level(summaryChecks(input({ values: values({ list: undefined }) })), 'audience')).toBe('error')
    expect(level(summaryChecks(input({ estimate: { available: true, count: 0, counting: false, error: false } })), 'audience')).toBe('warning')
  })

  it('errs when a variant has no template', () => {
    const checks = summaryChecks(
      input({ values: values({ abEnabled: true, variants: [{ variation_name: 'A', template_id: 'tpl-a' }, { variation_name: 'B', template_id: '' }] }) })
    )
    expect(level(checks, 'content')).toBe('error')
  })

  it('errs on a past schedule or a schedule without date', () => {
    const past = values({ sendChoice: 'schedule', scheduledDate: '2026-11-11', scheduledTime: '09:00' })
    expect(level(summaryChecks(input({ values: past })), 'when')).toBe('error')
    const future = values({ sendChoice: 'schedule', scheduledDate: '2026-11-13', scheduledTime: '09:00' })
    expect(level(summaryChecks(input({ values: future })), 'when')).toBe('ok')
    const noDate = values({ sendChoice: 'schedule', scheduledDate: undefined })
    expect(summaryChecks(input({ values: noDate })).find((c) => c.key === 'when')).toMatchObject({ level: 'error', missingDate: true })
  })

  it('checks no date on a scheduled broadcast: its booking stands', () => {
    expect(summaryChecks(input({ booked: true })).find((c) => c.key === 'when')).toBeUndefined()
  })

  it('warns, without blocking, for each variant whose template lacks an unsubscribe link, with its languages', () => {
    const checks = summaryChecks(
      input({ values: values({ abEnabled: true, variants: [{ variation_name: 'A', template_id: 'tpl-a' }, { variation_name: 'B', template_id: 'tpl-b' }] }) })
    )
    expect(checks.find((c) => c.key === 'unsubscribe-1')).toMatchObject({ level: 'warning', variant: 'B', languages: ['default', 'fr'] })
    expect(checks.find((c) => c.key === 'unsubscribe-0')).toBeUndefined()
    expect(errorCount(checks)).toBe(0)
  })

  it('errs without a marketing provider and for a sender that left the integration', () => {
    const checks = summaryChecks(
      input({ hasMarketingProvider: false, values: values({ variants: [{ variation_name: 'A', template_id: 'tpl-a', sender_id: 'gone' }] }) })
    )
    expect(level(checks, 'provider')).toBe('error')
    expect(checks.find((c) => c.key === 'sender-0')).toMatchObject({ level: 'error', senderId: 'gone' })
  })

  it('adds the reason validateDraft gives, unless a line already says it', () => {
    expect(summaryChecks(input({ issue: { code: 'feed_url_missing', feed: 'global' } })).find((c) => c.key === 'issue')).toBeDefined()
    expect(summaryChecks(input({ issue: { code: 'list_missing' } })).find((c) => c.key === 'issue')).toBeUndefined()
  })

  it('notes an untracked broadcast as information', () => {
    expect(level(summaryChecks(input({ values: values({ noTracking: true }) })), 'untracked')).toBe('ok')
  })
})

describe('SendSummary', () => {
  const renderSummary = (over: Partial<SummaryInput> = {}, extra: { listName?: string; segmentNames?: string[] } = {}) => {
    const i = input(over)
    render(
      <SendSummary
        checks={summaryChecks(i)}
        values={i.values}
        listName={extra.listName ?? 'Newsletter'}
        segmentNames={extra.segmentNames ?? []}
        workspaceId="ws1"
      />
    )
  }

  it('words the audience, the content and the moment', () => {
    renderSummary({}, { segmentNames: ['VIP'] })
    expect(screen.getByText('Newsletter, VIP · ≈ 1200 recipients')).toBeInTheDocument()
    expect(screen.getByText('1 template')).toBeInTheDocument()
    expect(screen.getByText('Now')).toBeInTheDocument()
  })

  it('says no template is chosen yet', () => {
    renderSummary({ values: values({ variants: [{ variation_name: 'A', template_id: '' }] }) })
    expect(screen.getByText('No template chosen')).toBeInTheDocument()
    expect(screen.queryByText('1 template')).toBeNull()
  })

  it('words an A/B test with its sample and duration', () => {
    renderSummary({
      values: values({
        abEnabled: true,
        samplePercentage: 20,
        autoSendWinner: true,
        winnerMetric: 'open_rate',
        testDurationHours: 4,
        variants: [{ variation_name: 'A', template_id: 'tpl-a' }, { variation_name: 'B', template_id: 'tpl-b' }]
      })
    })
    expect(screen.getByText('2 variants, test on 20 % for 4 h')).toBeInTheDocument()
    expect(screen.getByText('Variant B has no unsubscribe link (default, fr)')).toBeInTheDocument()
  })

  it('words the single template without unsubscribe link, the provider and the sender', () => {
    renderSummary({
      hasMarketingProvider: false,
      values: values({ variants: [{ variation_name: 'A', template_id: 'tpl-b', sender_id: 'gone' }] })
    })
    expect(screen.getByText('The template has no unsubscribe link (default, fr)')).toBeInTheDocument()
    expect(screen.getByText('No marketing email provider')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Configure Provider' })).toHaveAttribute('href', '/console/workspace/ws1/settings/integrations')
    expect(screen.getByText('Variant A: sender gone no longer exists')).toBeInTheDocument()
  })
})
