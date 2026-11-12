import type {
  BroadcastStatus,
  CreateBroadcastRequest,
  ScheduleSettings,
  UpdateBroadcastRequest,
  UTMParameters
} from '../../../services/api/broadcast'
import { VARIANT_LETTERS, type ComposeValues } from './composeValues'

export interface PayloadContext {
  workspaceId: string
  /** The broadcast once created. */
  id?: string
  /** The status of the last server answer: the schedule choice is only written on a draft. */
  status?: BroadcastStatus
}

const definedOnly = (utm: UTMParameters): UTMParameters | undefined => {
  const entries = Object.entries(utm).filter(([, v]) => v !== undefined && v !== '')
  return entries.length ? (Object.fromEntries(entries) as UTMParameters) : undefined
}

/**
 * The schedule choice of a draft, stored with is_scheduled false: nothing reads it until
 * broadcasts.schedule books the broadcast, from its own request.
 */
export function draftSchedule(values: ComposeValues): ScheduleSettings {
  const base: ScheduleSettings = { is_scheduled: false, use_recipient_timezone: false }
  if (values.sendChoice !== 'schedule') return base
  return {
    ...base,
    ...(values.scheduledDate ? { scheduled_date: values.scheduledDate } : {}),
    scheduled_time: values.scheduledTime,
    timezone: values.timezone
  }
}

/** Whether the values carry a schedule choice an update has to write after the create. */
export const hasScheduleChoice = (values: ComposeValues) => values.sendChoice === 'schedule'

/**
 * The broadcasts.create or broadcasts.update request of the composer. Each variation carries only
 * variation_name, template_id and sender_id: a joined template sent back would be stored in the
 * test_settings JSONB. metadata is never sent, so the server keeps the stored value.
 */
export function buildBroadcastPayload(
  values: ComposeValues,
  { workspaceId, id, status }: PayloadContext
): CreateBroadcastRequest | UpdateBroadcastRequest {
  const variants = values.abEnabled ? values.variants : values.variants.slice(0, 1)
  const autoSendWinner = values.abEnabled && values.autoSendWinner && !values.noTracking
  const common = {
    workspace_id: workspaceId,
    name: values.name.trim(),
    audience: {
      list: values.list,
      segments: values.segments,
      exclude_unsubscribed: values.excludeUnsubscribed
    },
    test_settings: {
      enabled: values.abEnabled,
      sample_percentage: values.samplePercentage,
      auto_send_winner: autoSendWinner,
      ...(autoSendWinner && values.winnerMetric ? { auto_send_winner_metric: values.winnerMetric } : {}),
      ...(autoSendWinner && values.testDurationHours ? { test_duration_hours: values.testDurationHours } : {}),
      variations: variants.map((variant, index) => ({
        variation_name: values.abEnabled ? variant.variation_name || VARIANT_LETTERS[index] : 'A',
        template_id: variant.template_id,
        ...(variant.sender_id ? { sender_id: variant.sender_id } : {})
      }))
    },
    utm_parameters: definedOnly({
      source: values.utmSource,
      medium: values.utmMedium,
      campaign: values.utmCampaign,
      term: values.utmTerm,
      content: values.utmContent
    }),
    tracking_mode: values.noTracking ? ('no_tracking' as const) : ('inherit' as const),
    // Both feeds always: the server reads an absent feed as "keep what is stored", so a feed
    // just switched off would keep being called.
    data_feed: { global_feed: values.globalFeed, recipient_feed: values.recipientFeed }
  }

  if (!id) {
    // broadcasts.create stores an empty schedule whatever it receives; the choice follows in an
    // update.
    return { ...common, schedule: { is_scheduled: false, use_recipient_timezone: false } }
  }
  // A scheduled broadcast keeps its booking: without the key the server keeps the stored schedule.
  if (status !== 'draft') return { ...common, id }
  return { ...common, id, schedule: draftSchedule(values) }
}
