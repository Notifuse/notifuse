import type { EmailTrackingSettings, WorkspaceSettings } from '../services/api/workspace'

/**
 * The workspace's tracking policy, read the way the server reads it: the
 * detailed settings when they exist, otherwise the single switch for both
 * opens and clicks, with unknown consent tracked.
 */
export function getEmailTrackingPolicy(
  settings: Pick<WorkspaceSettings, 'email_tracking_enabled' | 'email_tracking'> | undefined | null
): EmailTrackingSettings {
  if (settings?.email_tracking) {
    return {
      opens_enabled: settings.email_tracking.opens_enabled,
      clicks_enabled: settings.email_tracking.clicks_enabled,
      unknown_consent: settings.email_tracking.unknown_consent || 'track'
    }
  }
  const enabled = settings?.email_tracking_enabled || false
  return { opens_enabled: enabled, clicks_enabled: enabled, unknown_consent: 'track' }
}

export type TrackingConsentValue = 'granted' | 'denied'

export const TRACKING_DOCS_URL = 'https://docs.notifuse.com/features/email-tracking'

/** The values a consent column holds; unknown consent is NULL. */
export const TRACKING_CONSENT_FIELD_OPTIONS = [
  { value: 'granted', label: 'Granted' },
  { value: 'denied', label: 'Denied' }
]

/** Where a consent change can come from, as tracking_consent_source stores it. */
export const TRACKING_CONSENT_SOURCE_FIELD_OPTIONS = [
  { value: 'api', label: 'API' },
  { value: 'console', label: 'Console' },
  { value: 'import', label: 'Import' },
  { value: 'notification_center', label: 'Notification center' },
  { value: 'email_link', label: 'Email link' },
  { value: 'subscribe_form', label: 'Subscription form' }
]

/**
 * The emails an open or click rate is computed over: those that carried the
 * pixel (opens) or tracked links (clicks). A response without the tracked
 * total predates per-recipient tracking, and then every sent email counts.
 */
export function trackedBase(sent: number, tracked: number | undefined): number {
  return tracked ?? sent
}

/** True when some sent emails were not tracked, so the rate covers fewer recipients. */
export function isPartiallyTracked(sent: number, tracked: number | undefined): boolean {
  return tracked !== undefined && tracked < sent
}
