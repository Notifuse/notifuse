import type {
  Broadcast,
  BroadcastVariation,
  GlobalFeedSettings,
  RecipientFeedSettings
} from '../../../services/api/broadcast'
import type { Workspace } from '../../../services/api/types'
import extractTLD from '../../../lib/tld'

export interface ComposeVariant {
  variation_name: string
  template_id: string
  sender_id?: string
}

export type SendChoice = 'now' | 'schedule'
export type WinnerMetric = 'open_rate' | 'click_rate'

/** Everything the composer edits, as one value: each change is a new value the autosave reads. */
export interface ComposeValues {
  name: string
  list?: string
  segments: string[]
  excludeUnsubscribed: boolean
  abEnabled: boolean
  variants: ComposeVariant[]
  samplePercentage: number
  autoSendWinner: boolean
  winnerMetric?: WinnerMetric
  testDurationHours?: number
  sendChoice: SendChoice
  /** YYYY-MM-DD */
  scheduledDate?: string
  /** HH:mm */
  scheduledTime: string
  timezone: string
  utmSource?: string
  utmMedium?: string
  utmCampaign?: string
  /** Kept as stored: no field edits them. */
  utmTerm?: string
  utmContent?: string
  /** False while utm_campaign follows the name. */
  utmCampaignEdited: boolean
  noTracking: boolean
  globalFeed: GlobalFeedSettings
  recipientFeed: RecipientFeedSettings
}

export const DEFAULT_SAMPLE_PERCENTAGE = 50
export const DEFAULT_SCHEDULE_TIME = '11:00'
export const MAX_VARIANTS = 5
export const VARIANT_LETTERS = ['A', 'B', 'C', 'D', 'E']

const emptyFeed = (): GlobalFeedSettings => ({ enabled: false, url: '', headers: [] })

/** The current transformation of a name into utm_campaign: lower case, `_` for the rest. */
export function campaignFromName(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^\w\s]/g, '_')
    .replace(/\s+/g, '_')
    .replace(/_+/g, '_')
}

/** A new broadcast. `name` is "Broadcast {date}", worded by the caller in the person's language. */
export function newDraft(name: string, workspace: Workspace): ComposeValues {
  const source = extractTLD(workspace.settings?.website_url || '')
  return {
    name,
    list: undefined,
    segments: [],
    excludeUnsubscribed: true,
    abEnabled: false,
    variants: [{ variation_name: 'A', template_id: '' }],
    samplePercentage: DEFAULT_SAMPLE_PERCENTAGE,
    autoSendWinner: false,
    sendChoice: 'now',
    scheduledTime: DEFAULT_SCHEDULE_TIME,
    timezone: workspace.settings?.timezone || 'UTC',
    utmSource: source || undefined,
    utmMedium: 'email',
    utmCampaign: campaignFromName(name),
    utmCampaignEdited: false,
    noTracking: false,
    globalFeed: emptyFeed(),
    recipientFeed: emptyFeed()
  }
}

const toVariant = (v: BroadcastVariation, index: number): ComposeVariant => ({
  variation_name: v.variation_name || VARIANT_LETTERS[index] || String(index + 1),
  template_id: v.template_id || '',
  ...(v.sender_id ? { sender_id: v.sender_id } : {})
})

/** The values of a stored broadcast. */
export function fromBroadcast(broadcast: Broadcast, workspace: Workspace): ComposeValues {
  const test = broadcast.test_settings
  const variations = test?.variations?.length ? test.variations : [{ variation_name: 'A', template_id: '' }]
  const utm = broadcast.utm_parameters ?? {}
  const schedule = broadcast.schedule
  return {
    name: broadcast.name,
    list: broadcast.audience?.list || undefined,
    segments: broadcast.audience?.segments ?? [],
    excludeUnsubscribed: broadcast.audience?.exclude_unsubscribed ?? true,
    abEnabled: Boolean(test?.enabled),
    variants: (test?.enabled ? variations : variations.slice(0, 1)).map(toVariant),
    samplePercentage: test?.sample_percentage || DEFAULT_SAMPLE_PERCENTAGE,
    autoSendWinner: Boolean(test?.auto_send_winner),
    winnerMetric: test?.auto_send_winner_metric,
    testDurationHours: test?.test_duration_hours || undefined,
    // A draft keeps its schedule choice with is_scheduled false; a scheduled one has it booked.
    sendChoice: schedule?.scheduled_date ? 'schedule' : 'now',
    scheduledDate: schedule?.scheduled_date || undefined,
    scheduledTime: schedule?.scheduled_time || DEFAULT_SCHEDULE_TIME,
    timezone: schedule?.timezone || workspace.settings?.timezone || 'UTC',
    utmSource: utm.source,
    utmMedium: utm.medium,
    utmCampaign: utm.campaign,
    utmTerm: utm.term,
    utmContent: utm.content,
    // A reopened broadcast whose stored value differs from its name's transformation was edited.
    utmCampaignEdited: (utm.campaign ?? '') !== campaignFromName(broadcast.name),
    noTracking: broadcast.tracking_mode === 'no_tracking',
    globalFeed: broadcast.data_feed?.global_feed ?? emptyFeed(),
    recipientFeed: broadcast.data_feed?.recipient_feed ?? emptyFeed()
  }
}

/** Applies a change of name: utm_campaign follows it until edited. */
export function withName(values: ComposeValues, name: string): ComposeValues {
  return values.utmCampaignEdited ? { ...values, name } : { ...values, name, utmCampaign: campaignFromName(name) }
}

/** Normalises a typed time to HH:mm ("9:5" → "09:05"); null when it is no time. */
export function normalizeTime(input: string): string | null {
  const match = /^\s*(\d{1,2})(?:[:h.]?(\d{1,2}))?\s*$/i.exec(input)
  if (!match) return null
  const hours = Number(match[1])
  const minutes = Number(match[2] ?? '0')
  if (hours > 23 || minutes > 59) return null
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}`
}
