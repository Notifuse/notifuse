import type { TrackingMode, TrackingSettings } from '../../services/api/template'
import type {
  CreateTransactionalNotificationRequest,
  TrackingSettingsInput,
  TransactionalNotification,
  UpdateTransactionalNotificationRequest
} from '../../services/api/transactional_notifications'
import type { Workspace } from '../../services/api/types'
import type { EmailTrackingSettings } from '../../services/api/workspace'
import extractTLD from '../../lib/tld'
import { truncateIdentifier } from '../../utils/identifiers'
import { TRANSACTIONAL_ID_MAX_LENGTH } from '../../constants/limits'
import { SEND_VARIABLES } from '../templates/liquidVariablePaths'

/** What the console accepts as an API identifier; the API itself only limits its length. */
export const API_ID_PATTERN = /^[a-z0-9_]+$/

/** The API identifier a name gives: lowercase, punctuation dropped, spaces as underscores, 32 at most. */
export function generateApiId(name: string): string {
  if (!name) return ''
  const id = name
    .toLowerCase()
    .replace(/[^\w\s]/g, '')
    .replace(/\s+/g, '_')
    .replace(/_+/g, '_')
  return truncateIdentifier(id, TRANSACTIONAL_ID_MAX_LENGTH)
}

/** The UTM parameters the form edits, in the order it shows them. */
export const UTM_FIELDS = ['utm_source', 'utm_medium', 'utm_campaign', 'utm_content', 'utm_term'] as const
export type UtmField = (typeof UTM_FIELDS)[number]

export type TrackingDraft = { tracking_mode: TrackingMode } & Record<UtmField, string>

/** A notification as the form holds it, every field present so a reset clears what was typed. */
export interface NotificationDraft {
  name: string
  id: string
  description: string
  channels: { email: { template_id: string } }
  tracking_settings: TrackingDraft
}

/**
 * The form's starting point: the notification's values, or a new notification's defaults (the
 * workspace's domain as utm_source, email, transactional; utm_content follows the identifier).
 */
export function draftOf(notification: TransactionalNotification | undefined, workspace: Workspace): NotificationDraft {
  if (notification) {
    const stored = notification.tracking_settings
    return {
      name: notification.name ?? '',
      id: notification.id,
      description: notification.description ?? '',
      channels: { email: { template_id: notification.channels?.email?.template_id ?? '' } },
      tracking_settings: {
        // A stored absent mode means inherit
        tracking_mode: stored?.tracking_mode || 'inherit',
        utm_source: stored?.utm_source ?? '',
        utm_medium: stored?.utm_medium ?? '',
        utm_campaign: stored?.utm_campaign ?? '',
        utm_content: stored?.utm_content ?? '',
        utm_term: stored?.utm_term ?? ''
      }
    }
  }
  return {
    name: '',
    id: '',
    description: '',
    channels: { email: { template_id: '' } },
    tracking_settings: {
      tracking_mode: 'inherit',
      utm_source: extractTLD(workspace.settings?.website_url || '') || '',
      utm_medium: 'email',
      utm_campaign: 'transactional',
      utm_content: '',
      utm_term: ''
    }
  }
}

/**
 * The draft with the form's watched values laid over it. The watch answers {} until the form is
 * connected, and a field the selector clears comes through as null.
 */
export function mergeDraft(base: NotificationDraft, watched: Partial<NotificationDraft> | undefined): NotificationDraft {
  const tracking = { ...base.tracking_settings, ...watched?.tracking_settings }
  // Absent from the watch: the starting template. Cleared (null): no template, not the starting one.
  const templateId: string | null | undefined = watched?.channels?.email?.template_id
  return {
    name: watched?.name ?? base.name,
    id: watched?.id ?? base.id,
    description: watched?.description ?? base.description,
    channels: { email: { template_id: templateId === undefined ? base.channels.email.template_id : (templateId ?? '') } },
    tracking_settings: {
      tracking_mode: tracking.tracking_mode || 'inherit',
      utm_source: tracking.utm_source ?? '',
      utm_medium: tracking.utm_medium ?? '',
      utm_campaign: tracking.utm_campaign ?? '',
      utm_content: tracking.utm_content ?? '',
      utm_term: tracking.utm_term ?? ''
    }
  }
}

export function sameTracking(a: TrackingDraft, b: TrackingDraft): boolean {
  return a.tracking_mode === b.tracking_mode && UTM_FIELDS.every((field) => a[field] === b[field])
}

export function sameDraft(a: NotificationDraft, b: NotificationDraft): boolean {
  return (
    a.name === b.name &&
    a.id === b.id &&
    a.description === b.description &&
    a.channels.email.template_id === b.channels.email.template_id &&
    sameTracking(a.tracking_settings, b.tracking_settings)
  )
}

/**
 * The tracking block a save sends. The server replaces the stored block with it, so it starts
 * from what is stored: a field the form has no input for (set through the API) is kept.
 */
export function trackingToSend(draft: TrackingDraft, stored?: TrackingSettings): TrackingSettingsInput {
  return { ...stored, ...draft }
}

export function createRequest(draft: NotificationDraft, workspaceId: string): CreateTransactionalNotificationRequest {
  return {
    workspace_id: workspaceId,
    notification: {
      id: draft.id,
      name: draft.name,
      description: draft.description,
      channels: { email: { template_id: draft.channels.email.template_id } },
      tracking_settings: trackingToSend(draft.tracking_settings)
    }
  }
}

/**
 * The update a save sends. metadata is never edited here, and the server keeps it when the key is
 * absent. An empty description clears it. The server refuses any channels on a notification managed
 * by an integration, so they are left out for it; its unchanged name is accepted.
 */
export function updateRequest(
  draft: NotificationDraft,
  notification: TransactionalNotification,
  workspaceId: string
): UpdateTransactionalNotificationRequest {
  return {
    workspace_id: workspaceId,
    id: notification.id,
    updates: {
      name: draft.name,
      description: draft.description,
      channels: notification.integration_id
        ? undefined
        : { ...notification.channels, email: { ...notification.channels?.email, template_id: draft.channels.email.template_id } },
      tracking_settings: trackingToSend(draft.tracking_settings, notification.tracking_settings)
    }
  }
}

/** The template variables a send has to provide: its test data, without what every send fills in. */
export function dataToSend(testData: Record<string, unknown> | null | undefined): Record<string, unknown> {
  return Object.fromEntries(Object.entries(testData ?? {}).filter(([key]) => !SEND_VARIABLES.includes(key)))
}

export interface TrackingEffect {
  opens: 'on' | 'off' | 'workspaceOff'
  clicks: 'on' | 'off' | 'workspaceOff'
  utm: boolean
}

/**
 * What a tracking mode does with the workspace's policy, as the sheet reads it: following the
 * workspace turns on what it turns on, no mode turns on what it turned off, and only disabled
 * leaves the links without UTM parameters.
 */
export function trackingEffect(mode: TrackingMode, policy: EmailTrackingSettings): TrackingEffect {
  if (mode === 'inherit') {
    return {
      opens: policy.opens_enabled ? 'on' : 'workspaceOff',
      clicks: policy.clicks_enabled ? 'on' : 'workspaceOff',
      utm: true
    }
  }
  return { opens: 'off', clicks: 'off', utm: mode !== 'disabled' }
}
