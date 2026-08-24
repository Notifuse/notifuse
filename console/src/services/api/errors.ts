/**
 * API error types.
 *
 * These live apart from client.ts because that module imports the router, and the router
 * builds routes at module scope. A component that only wants to inspect an error's status
 * would otherwise drag the whole router graph into its test environment, where a shallow
 * router mock cannot satisfy it.
 */
import { msg } from '@lingui/core/macro'
import type { MessageDescriptor } from '@lingui/core'
import { i18n } from '../../i18n'
import { ALL_PERMISSION_RESOURCES } from './permissions'
import type { PermissionResource, ResourcePermissions } from './permissions'

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public data?: unknown
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

/**
 * The query client's retry policy.
 *
 * An expired session and a missing permission are not transient: the second attempt is refused
 * exactly like the first, and repeating it only holds the error back by one retryDelay — long
 * enough that a scoped API key's denial looks like a hung page rather than an answer. Everything
 * else keeps the single retry the client had before.
 */
export function shouldRetryQuery(failureCount: number, error: Error): boolean {
  if (error instanceof ApiError && (error.status === 401 || error.status === 403)) {
    return false
  }
  return failureCount < 1
}

// The two verbs a grant is made of, taken from the grant itself so the pair cannot drift.
export type PermissionVerb = keyof ResourcePermissions

export interface PermissionDenial {
  resource: PermissionResource
  permission: PermissionVerb
}

// Wording follows the Read/Write matrix on the Team page, so an owner reading a denial knows
// which switch to flip. Lazy descriptors, not `t`: these are built at module scope, before any
// catalog is active, and are rendered through i18n._() at the moment the message is needed.
export const RESOURCE_LABELS: Record<PermissionResource, MessageDescriptor> = {
  contacts: msg`Contacts`,
  lists: msg`Lists`,
  templates: msg`Templates`,
  broadcasts: msg`Broadcasts`,
  transactional: msg`Transactional`,
  workspace: msg`Workspace`,
  message_history: msg`Message History`,
  blog: msg`Blog`,
  automations: msg`Automations`,
  llm: msg`LLM`,
  web_analytics: msg`Web Analytics`,
  segments: msg`Segments`,
  webhook_subscriptions: msg`Webhook Subscriptions`,
  webhook_events: msg`Webhook Events`
}

const PERMISSION_VERBS: PermissionVerb[] = ['read', 'write']

/**
 * Reads the denial fields off a parsed error body.
 *
 * Detection is by the presence of `resource` and `permission`, never by status: 403 is also the
 * answer to a workspace-access failure and to the quota errors (workspace limit, team member
 * limit) that two call sites string-match to raise an upgrade prompt. Those bodies carry the
 * `error` key alone, so they fall through here and keep their own message.
 *
 * A resource the console does not know — a backend newer than this bundle — also returns null,
 * which leaves the server's own English sentence in place rather than inventing a label for it.
 */
export function permissionDenialFromBody(body: unknown): PermissionDenial | null {
  if (!body || typeof body !== 'object') return null

  const { resource, permission } = body as { resource?: unknown; permission?: unknown }
  if (typeof resource !== 'string' || typeof permission !== 'string') return null
  if (!ALL_PERMISSION_RESOURCES.includes(resource as PermissionResource)) return null
  if (!PERMISSION_VERBS.includes(permission as PermissionVerb)) return null

  return {
    resource: resource as PermissionResource,
    permission: permission as PermissionVerb
  }
}

/**
 * Reads the denial fields off a thrown error, for call sites that want the missing grant rather
 * than the sentence — ApiError keeps the parsed body on `data`.
 */
export function permissionDenial(err: unknown): PermissionDenial | null {
  return err instanceof ApiError ? permissionDenialFromBody(err.data) : null
}

/**
 * The console's own sentence for a denial, built from the resource and verb rather than by
 * decorating the server's English string — which is not translatable and would read twice.
 */
export function permissionDeniedMessage({ resource, permission }: PermissionDenial): string {
  const label = i18n._(RESOURCE_LABELS[resource])
  return permission === 'read'
    ? i18n._(msg`You do not have read access to ${{ resource: label }}.`)
    : i18n._(msg`You do not have write access to ${{ resource: label }}.`)
}
