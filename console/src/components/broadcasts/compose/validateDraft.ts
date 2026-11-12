import type { DataFeedHeader } from '../../../services/api/broadcast'
import { MAX_VARIANTS, type ComposeValues } from './composeValues'

/**
 * Why a draft cannot be saved yet. The rules of Broadcast.Validate (broadcast.go) and of the data
 * feed validation (data_feed.go, data_feed_url_validation.go), plus the console's own rules on
 * templates and senders, so the server never receives a request it would refuse for them.
 */
export type DraftIssue =
  | { code: 'name_missing' }
  | { code: 'name_too_long' }
  | { code: 'list_missing' }
  | { code: 'ab_too_few' }
  | { code: 'ab_too_many' }
  | { code: 'variant_no_template'; variant: string }
  | { code: 'template_reused'; variant: string }
  | { code: 'sender_gone'; variant: string }
  | { code: 'sample_out_of_range' }
  | { code: 'duration_out_of_range' }
  | { code: 'metric_missing' }
  | { code: 'auto_winner_untracked' }
  | { code: 'feed_url_missing'; feed: FeedKind }
  | { code: 'feed_url_invalid'; feed: FeedKind }
  | { code: 'feed_header_incomplete'; feed: FeedKind }

export type FeedKind = 'global' | 'recipient'

export interface DraftContext {
  /** Ids of the marketing integration's senders. */
  senderIds: string[]
}

/** The server counts the name in bytes (len() of a Go string). */
export const MAX_NAME_BYTES = 255

const byteLength = (text: string) => new TextEncoder().encode(text).length

/** Mirrors ValidateFeedURL without the SSRF checks, which stay on the server. */
export function feedUrlIsValid(value: string): boolean {
  let url: URL
  try {
    url = new URL(value)
  } catch {
    return false
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') return false
  // WHATWG reads "https:///x" as host "x"; Go's url.Parse, which the server uses, as no host.
  if (!url.host || !/^https?:\/\/[^/?#]/i.test(value)) return false
  if (url.search !== '' || value.includes('?')) return false
  if (url.hash !== '' || value.includes('#')) return false
  return true
}

function feedIssue(
  feed: FeedKind,
  settings: { enabled: boolean; url?: string; headers?: DataFeedHeader[] }
): DraftIssue | null {
  if (!settings.enabled) return null
  if (!settings.url?.trim()) return { code: 'feed_url_missing', feed }
  if (!feedUrlIsValid(settings.url.trim())) return { code: 'feed_url_invalid', feed }
  if ((settings.headers ?? []).some((h) => !h.name || !h.value)) return { code: 'feed_header_incomplete', feed }
  return null
}

/** The first reason the draft cannot be saved, or null. */
export function validateDraft(values: ComposeValues, { senderIds }: DraftContext): DraftIssue | null {
  if (!values.name.trim()) return { code: 'name_missing' }
  if (byteLength(values.name) > MAX_NAME_BYTES) return { code: 'name_too_long' }
  if (!values.list) return { code: 'list_missing' }

  const variants = values.abEnabled ? values.variants : values.variants.slice(0, 1)
  if (values.abEnabled) {
    if (variants.length < 2) return { code: 'ab_too_few' }
    if (variants.length > MAX_VARIANTS) return { code: 'ab_too_many' }
    const seen = new Set<string>()
    for (const variant of variants) {
      if (!variant.template_id) return { code: 'variant_no_template', variant: variant.variation_name }
      if (seen.has(variant.template_id)) return { code: 'template_reused', variant: variant.variation_name }
      seen.add(variant.template_id)
    }
  }
  for (const variant of variants) {
    if (variant.sender_id && !senderIds.includes(variant.sender_id)) {
      return { code: 'sender_gone', variant: variant.variation_name }
    }
  }

  if (values.abEnabled) {
    if (!(values.samplePercentage >= 1 && values.samplePercentage <= 100)) return { code: 'sample_out_of_range' }
    if (values.autoSendWinner) {
      if (values.noTracking) return { code: 'auto_winner_untracked' }
      const hours = values.testDurationHours ?? 0
      if (!(hours >= 1 && hours <= 168)) return { code: 'duration_out_of_range' }
      if (!values.winnerMetric) return { code: 'metric_missing' }
    }
  }

  return feedIssue('global', values.globalFeed) ?? feedIssue('recipient', values.recipientFeed)
}
