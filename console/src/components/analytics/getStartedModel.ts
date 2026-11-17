import type { Integration, Workspace, WorkspaceMember } from '../../services/api/workspace'

/**
 * The "Get started" checklist of the dashboard: six required steps from an empty workspace to its
 * first email, and two optional ones. Every step ticks from the workspace's own data; nothing is
 * stored on the server.
 */

export type StepId = 'workspace' | 'provider' | 'sender' | 'contacts' | 'template' | 'send' | 'website' | 'team'

/** What a done step says in its pill. */
export interface StepDetail {
  /** provider: the integration connected first. */
  integration?: Integration
  /** sender: the default sender of each role. */
  senders?: { marketing?: string; transactional?: string }
  /** contacts and templates: how many there are. */
  count?: number
  /** team: people beyond the platform admins, and API keys. */
  teammates?: number
  apiKeys?: number
}

export interface StepState {
  id: StepId
  required: boolean
  done: boolean
  detail?: StepDetail
}

export interface GetStartedModel {
  steps: StepState[]
  /** Required steps done; the optional ones never count. */
  done: number
  total: number
  /** The first required step not done; undefined once they all are. */
  next?: StepId
  complete: boolean
}

/** A workspace that already sends keeps the checklist this many days after its creation. */
export const GET_STARTED_GRACE_DAYS = 30
/** From this many required steps done, the done steps fold into pills. */
export const COLLAPSE_AFTER = 2

const REQUIRED: StepId[] = ['workspace', 'provider', 'sender', 'contacts', 'template', 'send']

const lower = (email: string) => email.trim().toLowerCase()

/** The workspace's email integrations: a Zapier or LLM integration sends nothing. */
export function emailIntegrations(workspace: Workspace): Integration[] {
  return (workspace.integrations ?? []).filter(
    (integration) => integration.type === 'email' && Boolean(integration.email_provider)
  )
}

/**
 * The default sender of the provider a role uses, if it can send: the role's provider must still
 * exist, and have a default sender, since the server sends from the default one when no sender is
 * chosen and fails without it.
 */
export function roleSender(workspace: Workspace, role: 'marketing' | 'transactional'): string | undefined {
  const id =
    role === 'marketing'
      ? workspace.settings?.marketing_email_provider_id
      : workspace.settings?.transactional_email_provider_id
  if (!id) return undefined
  const integration = emailIntegrations(workspace).find((candidate) => candidate.id === id)
  return integration?.email_provider?.senders?.find((sender) => sender.is_default)?.email
}

/** Whether a role has a provider that can send. */
export function roleReady(workspace: Workspace, role: 'marketing' | 'transactional'): boolean {
  return roleSender(workspace, role) !== undefined
}

/**
 * The emails of the workspace's owner, lower-cased. Only a platform admin (ROOT_EMAIL) can create a
 * workspace, and its creation adds the creator as the first contact: that contact is a root email.
 */
export function ownerEmails(rootEmails: string[]): Set<string> {
  return new Set(rootEmails.map(lower).filter(Boolean))
}

/** The latest contacts hold only the owner's own contact, added when the workspace was created. */
export function hasOnlyOwnerContact(rows: { email: string }[], owners: Set<string>): boolean {
  return rows.length === 1 && owners.has(lower(rows[0].email))
}

/** People beyond the platform admins (members and invitations), and API keys. */
function team(members: WorkspaceMember[] | undefined, me: { id: string; email: string }, rootEmails: string[]) {
  const roots = ownerEmails(rootEmails)
  const viewerIsRoot = roots.has(lower(me.email))
  const people = new Set<string>()
  // A viewer who is not a platform admin is a teammate of the admin who created the workspace.
  if (!viewerIsRoot) people.add(lower(me.email))
  let apiKeys = 0
  for (const member of members ?? []) {
    if (member.type === 'api_key') {
      apiKeys += 1
      continue
    }
    if (roots.has(lower(member.email))) continue
    if (viewerIsRoot && member.user_id && member.user_id === me.id) continue
    people.add(lower(member.email))
  }
  return { teammates: people.size, apiKeys }
}

/**
 * Whether someone other than the platform admins works in the workspace: an API key, a member or an
 * invitation that is not a root email, or a viewer who is not one.
 */
export function hasTeammate(
  members: WorkspaceMember[] | undefined,
  me: { id: string; email: string },
  rootEmails: string[]
): boolean {
  const { teammates, apiKeys } = team(members, me, rootEmails)
  return teammates > 0 || apiKeys > 0
}

export interface GetStartedInput {
  workspace: Workspace
  rootEmails: string[]
  me: { id: string; email: string }
  /** The latest contacts (at most five); undefined while they load. */
  latestContacts?: { email: string }[]
  /** Every contact; undefined while it loads. */
  contactsTotal?: number
  /** Email templates; undefined while they load. */
  templates?: number
  /** The latest message's creation; null when there is none, undefined while it loads. */
  latestAt: string | null | undefined
  /** The members list; undefined while it loads or when it cannot be read. */
  members?: WorkspaceMember[]
}

/** Every step's state, the progress and the next step. */
export function deriveGetStarted(input: GetStartedInput): GetStartedModel {
  const { workspace, latestContacts, templates, latestAt } = input
  const owners = ownerEmails(input.rootEmails)

  const integrations = emailIntegrations(workspace)
  const marketing = roleSender(workspace, 'marketing')
  const transactional = roleSender(workspace, 'transactional')
  const contactsDone =
    latestContacts !== undefined &&
    (latestContacts.length >= 2 || (latestContacts.length === 1 && !hasOnlyOwnerContact(latestContacts, owners)))
  const members = team(input.members, input.me, input.rootEmails)

  const steps: StepState[] = [
    { id: 'workspace', required: true, done: true },
    {
      id: 'provider',
      required: true,
      done: integrations.length > 0,
      detail: integrations.length > 0 ? { integration: integrations[0] } : undefined
    },
    {
      id: 'sender',
      required: true,
      done: marketing !== undefined && transactional !== undefined,
      detail: { senders: { marketing, transactional } }
    },
    {
      id: 'contacts',
      required: true,
      done: contactsDone,
      detail: { count: input.contactsTotal ?? latestContacts?.length }
    },
    { id: 'template', required: true, done: (templates ?? 0) >= 1, detail: { count: templates } },
    { id: 'send', required: true, done: typeof latestAt === 'string' },
    { id: 'website', required: false, done: Boolean(workspace.settings?.web_analytics?.enabled) },
    {
      id: 'team',
      required: false,
      done: members.teammates > 0 || members.apiKeys > 0,
      detail: members
    }
  ]

  const required = steps.filter((step) => step.required)
  const done = required.filter((step) => step.done).length
  const next = REQUIRED.find((id) => !steps.find((step) => step.id === id)?.done)
  return { steps, done, total: required.length, next, complete: done === required.length }
}

export type GetStartedVisibility = 'hidden' | 'loading' | 'shown'

export interface VisibilityInput {
  /** The member may act on the steps: an owner (or a platform admin) who can read what they count. */
  canAct: boolean
  /** Hidden by this member in this browser. */
  hidden: boolean
  /** Undefined while the steps' data loads. */
  model?: GetStartedModel
  /** The latest message: null when none, undefined while it loads. */
  latestAt: string | null | undefined
  /** The workspace's creation (ISO). */
  createdAt: string
  now: number
}

/**
 * Whether the checklist shows. A workspace that already sends sees it only within its first
 * GET_STARTED_GRACE_DAYS days, so an established workspace never meets it; a new one keeps it after
 * its first send until every required step is done or it is hidden.
 *
 * `loading` (a skeleton) only when the checklist is certain to show: with no message yet. A workspace
 * that sends shows nothing until its data is in, so one with every step done never flashes it.
 */
export function getStartedVisibility({ canAct, hidden, model, latestAt, createdAt, now }: VisibilityInput): GetStartedVisibility {
  if (!canAct || hidden) return 'hidden'
  if (latestAt === undefined) return 'hidden'
  if (latestAt !== null) {
    const created = Date.parse(createdAt)
    // An unreadable date counts as old: never show the checklist to a workspace that sends on a guess.
    const recent = Number.isFinite(created) && now - created < GET_STARTED_GRACE_DAYS * 86_400_000
    if (!recent) return 'hidden'
    if (!model) return 'hidden'
  }
  if (!model) return 'loading'
  return model.complete ? 'hidden' : 'shown'
}

/** Whether the done steps fold into pills. */
export function shouldCollapse(model: GetStartedModel): boolean {
  return model.done >= COLLAPSE_AFTER
}
