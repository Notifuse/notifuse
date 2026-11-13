import type {
  GlobalFeedSettings as GlobalFeed,
  RecipientFeedSettings as RecipientFeed
} from '../../../services/api/broadcast'

/** "Off", or the host of the feed's URL. */
export function feedState(feed: GlobalFeed | RecipientFeed, off: string): string {
  if (!feed.enabled) return off
  try {
    return new URL(feed.url ?? '').host || feed.url || off
  } catch {
    return feed.url || off
  }
}
