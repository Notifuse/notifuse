import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App as AntApp } from 'antd'
import { useEffect, useState, type ReactNode } from 'react'
import { newDraft, type ComposeValues } from './composeValues'
import type { ComposeUpdate } from './types'
import { AudienceCard } from './AudienceCard'
import { ContentCard } from './ContentCard'
import { SendingCard } from './SendingCard'
import { TrackingCard } from './TrackingCard'
import { AdvancedRow, feedState } from './AdvancedRow'
import type { Template } from '../../../services/api/template'
import type { Workspace } from '../../../services/api/types'

vi.mock('../../templates/TemplateSelectorInput', () => ({
  default: ({ value, onChange, canReadTemplates }: { value?: string | null; onChange?: (v: string | null) => void; canReadTemplates?: boolean }) => (
    <select
      aria-label="Template"
      data-can-read-templates={String(canReadTemplates)}
      value={value ?? ''}
      onChange={(e) => onChange?.(e.target.value || null)}
    >
      <option value="">none</option>
      <option value="tpl-a">Alpha</option>
      <option value="tpl-b">Beta</option>
    </select>
  )
}))
vi.mock('../VariationSenderSelect', () => ({
  VariationSenderSelect: ({ value, onChange, canReadTemplates }: { value?: string; onChange?: (v?: string) => void; canReadTemplates?: boolean }) => (
    <select
      aria-label="Sender"
      data-can-read-templates={String(canReadTemplates)}
      value={value ?? ''}
      onChange={(e) => onChange?.(e.target.value || undefined)}
    >
      <option value="">template</option>
      <option value="s1">Acme</option>
      <option value="gone">Gone</option>
    </select>
  )
}))
vi.mock('../GlobalFeedSettings', () => ({ GlobalFeedSettings: ({ broadcastId }: { broadcastId?: string }) => <div>global feed {broadcastId ?? 'none'}</div> }))
vi.mock('../RecipientFeedSettings', () => ({ RecipientFeedSettings: () => <div>recipient feed</div> }))

const tracked = { email_tracking: { opens_enabled: true, clicks_enabled: true, unknown_consent: 'track' } }
const workspaceWith = (settings: Record<string, unknown> = tracked) =>
  ({
    id: 'ws1',
    settings: { timezone: 'UTC', marketing_email_provider_id: 'int1', ...settings },
    integrations: [{ id: 'int1', email_provider: { senders: [{ id: 's1', name: 'Acme', email: 'a@acme.test', is_default: true }] } }]
  }) as unknown as Workspace

/** Renders a card with live values, and exposes the last ones. */
function Harness({ initial, children }: { initial: ComposeValues; children: (v: ComposeValues, u: ComposeUpdate) => ReactNode }) {
  const [values, setValues] = useState(initial)
  useEffect(() => {
    latest = values
  }, [values])
  return <AntApp>{children(values, (fn) => setValues((v) => fn(v)))}</AntApp>
}
let latest: ComposeValues
const base = (over: Partial<ComposeValues> = {}) => ({ ...newDraft('News', workspaceWith()), ...over })

const templates: Record<string, Template> = {
  'tpl-a': { id: 'tpl-a', name: 'Alpha', email: { subject: 'Hello', subject_preview: 'Read me' } } as unknown as Template,
  'tpl-b': { id: 'tpl-b', name: 'Beta', email: { subject: 'Hi' } } as unknown as Template
}

describe('AudienceCard', () => {
  const estimate = { available: true, count: 1234, counting: false, error: false }
  const lists = [{ id: 'l1', name: 'Newsletter', active_count: 1500 }]
  const segments = [{ id: 'seg1', name: 'VIP', color: 'gold', users_count: 30 }]

  it('offers each list with its active contacts and shows the estimate', () => {
    render(
      <Harness initial={base({ list: 'l1' })}>
        {(v, u) => <AudienceCard values={v} onChange={u} lists={lists} segments={segments} canReadLists canReadSegments estimate={estimate} />}
      </Harness>
    )
    expect(screen.getByText('Newsletter · 1500 active')).toBeInTheDocument()
    expect(screen.getByTestId('audience-estimate')).toHaveTextContent('≈ 1234 recipients')
  })

  it('says Counting… while counting and — without the right to count', () => {
    const { rerender } = render(
      <Harness initial={base()}>
        {(v, u) => <AudienceCard values={v} onChange={u} lists={lists} segments={segments} canReadLists canReadSegments estimate={{ ...estimate, counting: true }} />}
      </Harness>
    )
    expect(screen.getByTestId('audience-estimate')).toHaveTextContent('Counting…')
    rerender(
      <Harness initial={base()}>
        {(v, u) => <AudienceCard values={v} onChange={u} lists={lists} segments={segments} canReadLists={false} canReadSegments={false} estimate={{ ...estimate, available: false }} />}
      </Harness>
    )
    expect(screen.getByTestId('audience-estimate')).toHaveTextContent('—')
  })

  it('shows the stored list, unchangeable, without the right to read lists', () => {
    render(
      <Harness initial={base({ list: 'l9' })}>
        {(v, u) => <AudienceCard values={v} onChange={u} lists={[]} segments={[]} canReadLists={false} canReadSegments={false} estimate={{ ...estimate, available: false }} />}
      </Harness>
    )
    expect(screen.getByText('l9')).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'List' })).toBeDisabled()
  })

  it('explains that including unsubscribed contacts drops the List-Unsubscribe header, and toggles it', async () => {
    render(
      <Harness initial={base()}>
        {(v, u) => <AudienceCard values={v} onChange={u} lists={lists} segments={segments} canReadLists canReadSegments estimate={estimate} />}
      </Harness>
    )
    expect(screen.getByText(/carry no List-Unsubscribe header/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('switch', { name: 'Exclude unsubscribed recipients' }))
    expect(latest.excludeUnsubscribed).toBe(false)
  })
})

describe('ContentCard', () => {
  const renderCard = (initial: ComposeValues, workspace = workspaceWith(), onPreview = vi.fn(), canReadTemplates = true) =>
    render(
      <Harness initial={initial}>
        {(v, u) => (
          <ContentCard
            workspace={workspace}
            values={v}
            onChange={u}
            templates={templates}
            previewIndex={0}
            onPreview={onPreview}
            estimateCount={1000}
            canReadTemplates={canReadTemplates}
          />
        )}
      </Harness>
    )

  it.each([true, false])('passes Templates read %s to the template and sender fields', (canRead) => {
    renderCard(base({ variants: [{ variation_name: 'A', template_id: 'tpl-x' }] }), workspaceWith(), vi.fn(), canRead)
    expect(screen.getByRole('combobox', { name: 'Template' })).toHaveAttribute('data-can-read-templates', String(canRead))
    expect(screen.getByRole('combobox', { name: 'Sender' })).toHaveAttribute('data-can-read-templates', String(canRead))
  })

  it('shows the subject and preview text of the chosen template, read only, with a link to edit it', () => {
    renderCard(base({ variants: [{ variation_name: 'A', template_id: 'tpl-a' }] }))
    expect(screen.getByText('Hello')).toBeInTheDocument()
    expect(screen.getByText('Read me')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Edit the template' })).toHaveAttribute('href', '/console/workspace/ws1/templates?q=Alpha')
  })

  it('adds an empty variant B when the A/B test is turned on', async () => {
    renderCard(base({ variants: [{ variation_name: 'A', template_id: 'tpl-a' }] }))
    await userEvent.click(screen.getByRole('switch', { name: 'A/B test' }))
    expect(latest.abEnabled).toBe(true)
    expect(latest.variants).toEqual([
      { variation_name: 'A', template_id: 'tpl-a' },
      { variation_name: 'B', template_id: '' }
    ])
  })

  it('disables the A/B switch when the workspace tracks neither opens nor clicks', () => {
    renderCard(base(), workspaceWith({ email_tracking: { opens_enabled: false, clicks_enabled: false } }))
    expect(screen.getByRole('switch', { name: 'A/B test' })).toBeDisabled()
  })

  it('confirms before turning the test off when other variants have a template', async () => {
    renderCard(base({ abEnabled: true, variants: [{ variation_name: 'A', template_id: 'tpl-a' }, { variation_name: 'B', template_id: 'tpl-b' }] }))
    await userEvent.click(screen.getByRole('switch', { name: 'A/B test' }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Turn off' }))
    expect(latest.abEnabled).toBe(false)
    expect(latest.variants).toEqual([{ variation_name: 'A', template_id: 'tpl-a' }])
  })

  it('stops at 5 variants and removes one only above 2', async () => {
    renderCard(base({ abEnabled: true, variants: [{ variation_name: 'A', template_id: '' }, { variation_name: 'B', template_id: '' }] }))
    expect(screen.queryByRole('button', { name: 'Remove variant A' })).not.toBeInTheDocument()
    for (let i = 0; i < 3; i++) await userEvent.click(screen.getByRole('button', { name: /Add variant/ }))
    expect(latest.variants.map((v) => v.variation_name)).toEqual(['A', 'B', 'C', 'D', 'E'])
    expect(screen.queryByRole('button', { name: /Add variant/ })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Remove variant C' }))
    expect(latest.variants.map((v) => v.variation_name)).toEqual(['A', 'B', 'D', 'E'])
  })

  it('refuses a template already used by another variant', () => {
    renderCard(base({ abEnabled: true, variants: [{ variation_name: 'A', template_id: 'tpl-a' }, { variation_name: 'B', template_id: '' }] }))
    fireEvent.change(screen.getAllByRole('combobox', { name: 'Template' })[1], { target: { value: 'tpl-a' } })
    expect(screen.getAllByText('This template is already used in another variation').length).toBeGreaterThan(0)
  })

  it('keeps the chosen sender, clears it, and refuses one that left the integration', () => {
    renderCard(base({ variants: [{ variation_name: 'A', template_id: 'tpl-a' }] }))
    const sender = screen.getByRole('combobox', { name: 'Sender' })
    fireEvent.change(sender, { target: { value: 's1' } })
    expect(latest.variants[0]).toEqual({ variation_name: 'A', template_id: 'tpl-a', sender_id: 's1' })
    fireEvent.change(sender, { target: { value: '' } })
    expect(latest.variants[0]).toEqual({ variation_name: 'A', template_id: 'tpl-a' })
    fireEvent.change(sender, { target: { value: 'gone' } })
    expect(screen.getByText(/This sender is no longer one of the marketing email integration's/)).toBeInTheDocument()
  })

  it('sizes the test sample from the estimate', () => {
    renderCard(base({ abEnabled: true, samplePercentage: 20, variants: [{ variation_name: 'A', template_id: '' }, { variation_name: 'B', template_id: '' }] }))
    expect(screen.getByTestId('sample-size')).toHaveTextContent('20 % · 200 contacts')
  })

  it('fills the metric and duration when the automatic winner is turned on', async () => {
    renderCard(base({ abEnabled: true, variants: [{ variation_name: 'A', template_id: '' }, { variation_name: 'B', template_id: '' }] }))
    await userEvent.click(screen.getByRole('switch', { name: 'Send the winner automatically' }))
    expect(latest).toMatchObject({ autoSendWinner: true, winnerMetric: 'open_rate', testDurationHours: 4 })
    expect(screen.getByRole('spinbutton', { name: 'Test duration (hours)' })).toBeInTheDocument()
  })

  it('locks the automatic winner off when the broadcast is not tracked, and says why', () => {
    renderCard(base({ abEnabled: true, noTracking: true, autoSendWinner: true, variants: [{ variation_name: 'A', template_id: '' }, { variation_name: 'B', template_id: '' }] }))
    const toggle = screen.getByRole('switch', { name: 'Send the winner automatically' })
    expect(toggle).toBeDisabled()
    expect(toggle).not.toBeChecked()
    expect(screen.getByText('Opens and clicks are not tracked for this broadcast: pick the winning variation yourself.')).toBeInTheDocument()
  })

  it('warns that only consenting contacts count when unknown contacts are not tracked, and only then', () => {
    const ab = base({ abEnabled: true, variants: [{ variation_name: 'A', template_id: '' }, { variation_name: 'B', template_id: '' }] })
    const warning = /only recipients who agreed to tracking count/
    const { unmount } = renderCard(ab, workspaceWith({ email_tracking: { opens_enabled: true, clicks_enabled: true, unknown_consent: 'no_track' } }))
    expect(screen.getByText(warning)).toBeInTheDocument()
    unmount()
    renderCard(ab)
    expect(screen.queryByText(warning)).not.toBeInTheDocument()
  })
})

describe('SendingCard', () => {
  it('chooses Schedule, with 11:00 by default, and normalises a typed time', async () => {
    render(<Harness initial={base()}>{(v, u) => <SendingCard values={v} onChange={u} workspaceTimezone="UTC" />}</Harness>)
    await userEvent.click(screen.getByRole('radio', { name: /Schedule/ }))
    expect(latest.sendChoice).toBe('schedule')
    const time = screen.getByRole('combobox', { name: 'Time' })
    expect(time).toHaveValue('11:00')
    await userEvent.clear(time)
    await userEvent.type(time, '9:5')
    fireEvent.blur(time)
    expect(latest.scheduledTime).toBe('09:05')
  })

  it('keeps the last valid time when the typed one is no time', async () => {
    render(<Harness initial={base({ sendChoice: 'schedule' })}>{(v, u) => <SendingCard values={v} onChange={u} workspaceTimezone="UTC" />}</Harness>)
    const time = screen.getByRole('combobox', { name: 'Time' })
    await userEvent.clear(time)
    await userEvent.type(time, '25:99')
    fireEvent.blur(time)
    expect(latest.scheduledTime).toBe('11:00')
  })

  it('shows the booking of a scheduled broadcast, read only', () => {
    render(
      <Harness initial={base()}>
        {(v, u) => (
          <SendingCard
            values={v}
            onChange={u}
            workspaceTimezone="UTC"
            booked={{ is_scheduled: true, scheduled_date: '2030-01-02', scheduled_time: '09:00', timezone: 'UTC', use_recipient_timezone: false }}
          />
        )}
      </Harness>
    )
    expect(screen.getByText(/^Scheduled for .* at /)).toBeInTheDocument()
    expect(screen.queryByRole('radio')).not.toBeInTheDocument()
  })
})

describe('TrackingCard', () => {
  it('keeps the campaign generated from the name until it is edited', async () => {
    render(<Harness initial={base()}>{(v, u) => <TrackingCard values={v} onChange={u} />}</Harness>)
    expect(screen.getByRole('textbox', { name: 'utm_campaign' })).toHaveValue('news')
    expect(screen.getByText('Generated from the name until you edit it')).toBeInTheDocument()
    await userEvent.type(screen.getByRole('textbox', { name: 'utm_campaign' }), '_x')
    expect(latest).toMatchObject({ utmCampaign: 'news_x', utmCampaignEdited: true })
  })

  it("turns tracking off for the broadcast, and the automatic winner with it", async () => {
    render(<Harness initial={base({ autoSendWinner: true })}>{(v, u) => <TrackingCard values={v} onChange={u} />}</Harness>)
    await userEvent.click(screen.getByRole('switch', { name: "Don't track opens and clicks for this broadcast" }))
    expect(latest).toMatchObject({ noTracking: true, autoSendWinner: false })
  })
})

describe('AdvancedRow', () => {
  beforeEach(() => vi.clearAllMocks())

  it('says Off or the host of each feed, and unfolds one on click', async () => {
    render(
      <Harness initial={base({ globalFeed: { enabled: true, url: 'https://api.acme.test/feed', headers: [] } })}>
        {(v, u) => <AdvancedRow workspaceId="ws1" values={v} onChange={u} />}
      </Harness>
    )
    expect(screen.getByText('api.acme.test')).toBeInTheDocument()
    expect(screen.getByText('Off')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /Global data feed/ }))
    expect(screen.getByText('global feed none')).toBeInTheDocument()
  })

  it('feedState reads an invalid URL as typed', () => {
    expect(feedState({ enabled: true, url: 'not a url', headers: [] }, 'Off')).toBe('not a url')
    expect(feedState({ enabled: false, url: 'https://x.test', headers: [] }, 'Off')).toBe('Off')
  })
})
