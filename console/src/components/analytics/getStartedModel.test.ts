import { describe, expect, it } from 'vitest'
import type { Integration, Workspace, WorkspaceMember } from '../../services/api/workspace'
import {
  COLLAPSE_AFTER,
  GET_STARTED_GRACE_DAYS,
  deriveGetStarted,
  emailIntegrations,
  getStartedVisibility,
  hasOnlyOwnerContact,
  hasTeammate,
  ownerEmails,
  roleReady,
  shouldCollapse,
  type GetStartedInput,
  type GetStartedModel,
  type StepId
} from './getStartedModel'

const ROOT = 'Root@Acme.com'
const me = { id: 'u-root', email: ROOT }

const ses = (id: string, senders: { email: string; is_default: boolean }[] = []): Integration =>
  ({
    id,
    name: `SES ${id}`,
    type: 'email',
    email_provider: {
      kind: 'ses',
      senders: senders.map((sender, index) => ({ id: `s${index}`, name: '', ...sender })),
      rate_limit_per_minute: 0
    },
    created_at: '',
    updated_at: ''
  }) as Integration

const workspace = (
  overrides: Omit<Partial<Workspace>, 'settings'> & { settings?: Partial<Workspace['settings']> } = {}
): Workspace =>
  ({
    id: 'ws1',
    name: 'Acme',
    created_at: '2026-11-15T10:00:00Z',
    updated_at: '2026-11-15T10:00:00Z',
    integrations: [],
    ...overrides,
    settings: { timezone: 'UTC', ...overrides.settings }
  }) as Workspace

const member = (fields: Partial<WorkspaceMember>): WorkspaceMember => ({
  user_id: '',
  workspace_id: 'ws1',
  role: 'member',
  email: '',
  type: 'user',
  created_at: '',
  updated_at: '',
  permissions: undefined,
  ...fields
})

const input = (overrides: Partial<GetStartedInput> = {}): GetStartedInput => ({
  workspace: workspace(),
  rootEmails: [ROOT],
  me,
  latestContacts: [],
  contactsTotal: 0,
  templates: 0,
  latestAt: null,
  members: [],
  ...overrides
})

const step = (model: GetStartedModel, id: StepId) => model.steps.find((candidate) => candidate.id === id)!
const done = (overrides: Partial<GetStartedInput>, id: StepId) => step(deriveGetStarted(input(overrides)), id).done

const ready = workspace({
  integrations: [ses('i1', [{ email: 'hello@acme.com', is_default: true }])],
  settings: { marketing_email_provider_id: 'i1', transactional_email_provider_id: 'i1' }
})

describe('deriveGetStarted', () => {
  it('always ticks the workspace step', () => {
    expect(done({}, 'workspace')).toBe(true)
  })

  describe('provider step', () => {
    it('is open without an integration', () => {
      expect(done({}, 'provider')).toBe(false)
    })
    it('does not count an integration that sends no email', () => {
      const other = [
        { id: 'z', name: 'Zapier', type: 'zapier' },
        { id: 'l', name: 'LLM', type: 'llm' }
      ] as Integration[]
      expect(done({ workspace: workspace({ integrations: other }) }, 'provider')).toBe(false)
      expect(emailIntegrations(workspace({ integrations: other }))).toEqual([])
    })
    it('ticks with an email integration, which names its pill', () => {
      const model = deriveGetStarted(input({ workspace: workspace({ integrations: [ses('i1')] }) }))
      expect(step(model, 'provider').done).toBe(true)
      expect(step(model, 'provider').detail?.integration?.id).toBe('i1')
    })
  })

  describe('sender step', () => {
    const withRoles = (settings: Partial<Workspace['settings']>, integrations = [ses('i1', [{ email: 'a@acme.com', is_default: true }])]) =>
      workspace({ integrations, settings })

    it('is open without roles', () => {
      expect(done({ workspace: withRoles({}) }, 'sender')).toBe(false)
    })
    it('is open with one role only', () => {
      const ws = withRoles({ marketing_email_provider_id: 'i1' })
      expect(done({ workspace: ws }, 'sender')).toBe(false)
      expect(roleReady(ws, 'marketing')).toBe(true)
      expect(roleReady(ws, 'transactional')).toBe(false)
    })
    it('is open when a role points to a deleted integration', () => {
      expect(
        done({ workspace: withRoles({ marketing_email_provider_id: 'i1', transactional_email_provider_id: 'gone' }) }, 'sender')
      ).toBe(false)
    })
    it('is open when the providers have senders but none is the default', () => {
      const ws = withRoles(
        { marketing_email_provider_id: 'i1', transactional_email_provider_id: 'i1' },
        [ses('i1', [{ email: 'a@acme.com', is_default: false }])]
      )
      expect(done({ workspace: ws }, 'sender')).toBe(false)
    })
    it('ticks when both roles have a default sender, and names them', () => {
      const model = deriveGetStarted(input({ workspace: ready }))
      expect(step(model, 'sender').done).toBe(true)
      expect(step(model, 'sender').detail?.senders).toEqual({
        marketing: 'hello@acme.com',
        transactional: 'hello@acme.com'
      })
    })
  })

  describe('contacts step', () => {
    const contacts = (...emails: string[]) => emails.map((email) => ({ email }))
    it('is open with no contact', () => {
      expect(done({ latestContacts: [] }, 'contacts')).toBe(false)
    })
    it('is open with only the owner, whatever the case of the email', () => {
      expect(done({ latestContacts: contacts('ROOT@acme.COM') }, 'contacts')).toBe(false)
      expect(hasOnlyOwnerContact(contacts('ROOT@acme.COM'), ownerEmails([ROOT]))).toBe(true)
    })
    it('ticks with one contact who is not the owner', () => {
      expect(done({ latestContacts: contacts('jane@example.com') }, 'contacts')).toBe(true)
    })
    it('ticks with two contacts, the owner included', () => {
      expect(done({ latestContacts: contacts(ROOT, 'jane@example.com') }, 'contacts')).toBe(true)
    })
    it('is open while the contacts load', () => {
      expect(done({ latestContacts: undefined }, 'contacts')).toBe(false)
    })
    it('counts every contact for its pill', () => {
      const model = deriveGetStarted(input({ latestContacts: contacts('a@x.com', 'b@x.com'), contactsTotal: 1240 }))
      expect(step(model, 'contacts').detail?.count).toBe(1240)
    })
  })

  describe('template step', () => {
    it('is open with no template, and while they load', () => {
      expect(done({ templates: 0 }, 'template')).toBe(false)
      expect(done({ templates: undefined }, 'template')).toBe(false)
    })
    it('ticks with one template', () => {
      expect(done({ templates: 1 }, 'template')).toBe(true)
    })
  })

  describe('send step', () => {
    it('is open with no message', () => {
      expect(done({ latestAt: null }, 'send')).toBe(false)
    })
    it('ticks with a message', () => {
      expect(done({ latestAt: '2026-11-16T09:00:00Z' }, 'send')).toBe(true)
    })
  })

  describe('website step', () => {
    it('follows web analytics', () => {
      expect(done({ workspace: workspace({ settings: { web_analytics: { enabled: true } } as Partial<Workspace['settings']> }) }, 'website')).toBe(true)
      expect(done({ workspace: workspace({ settings: { web_analytics: { enabled: false } } as Partial<Workspace['settings']> }) }, 'website')).toBe(false)
      expect(done({ workspace: workspace() }, 'website')).toBe(false)
    })
  })

  describe('team step', () => {
    it('is open with only me', () => {
      const members = [member({ user_id: me.id, email: ROOT, role: 'owner' })]
      expect(done({ members }, 'team')).toBe(false)
      expect(hasTeammate(members, me, [ROOT])).toBe(false)
    })
    it('is open with only another platform admin, listed as a virtual owner', () => {
      const members = [
        member({ user_id: me.id, email: ROOT, role: 'owner' }),
        member({ user_id: 'u2', email: 'ops@acme.com', role: 'owner' })
      ]
      expect(hasTeammate(members, me, [ROOT, 'OPS@acme.com'])).toBe(false)
    })
    it('ticks with a pending invitation', () => {
      const members = [member({ user_id: me.id, email: ROOT }), member({ user_id: '', email: 'jane@example.com', invitation_id: 'inv' })]
      expect(done({ members }, 'team')).toBe(true)
    })
    it('ticks with an API key, counted apart', () => {
      const members = [member({ user_id: me.id, email: ROOT }), member({ user_id: 'k1', email: 'key@api', type: 'api_key' })]
      const model = deriveGetStarted(input({ members }))
      expect(step(model, 'team').done).toBe(true)
      expect(step(model, 'team').detail).toMatchObject({ teammates: 0, apiKeys: 1 })
    })
    it('ticks for a viewer who is not a platform admin', () => {
      const viewer = { id: 'u9', email: 'jane@example.com' }
      expect(hasTeammate(undefined, viewer, [ROOT])).toBe(true)
    })
    it('stays open while the members load', () => {
      expect(done({ members: undefined }, 'team')).toBe(false)
    })
  })

  describe('next step and totals', () => {
    it('is the first step not done, in order', () => {
      expect(deriveGetStarted(input()).next).toBe('provider')
      expect(deriveGetStarted(input({ workspace: workspace({ integrations: [ses('i1')] }) })).next).toBe('sender')
    })
    it('skips the steps already done', () => {
      expect(deriveGetStarted(input({ workspace: ready, templates: 2 })).next).toBe('contacts')
      expect(deriveGetStarted(input({ workspace: ready, templates: 2, latestContacts: [{ email: 'a@x.com' }] })).next).toBe('send')
    })
    it('is undefined once every required step is done, and never an optional one', () => {
      const model = deriveGetStarted(
        input({ workspace: ready, templates: 1, latestContacts: [{ email: 'a@x.com' }], latestAt: '2026-11-16T09:00:00Z' })
      )
      expect(step(model, 'website').done).toBe(false)
      expect(step(model, 'team').done).toBe(false)
      expect(model.next).toBeUndefined()
      expect(model).toMatchObject({ done: 6, total: 6, complete: true })
    })
    it('leaves the optional steps out of the progress', () => {
      const model = deriveGetStarted(
        input({ workspace: workspace({ settings: { web_analytics: { enabled: true } } as Partial<Workspace['settings']> }), members: [member({ email: 'jane@example.com', user_id: 'u2' })] })
      )
      expect(step(model, 'website').done && step(model, 'team').done).toBe(true)
      expect(model).toMatchObject({ done: 1, total: 6, complete: false })
    })
  })
})

describe('getStartedVisibility', () => {
  const now = Date.parse('2026-11-17T12:00:00Z')
  const open = deriveGetStarted(input())
  const complete = deriveGetStarted(
    input({ workspace: ready, templates: 1, latestContacts: [{ email: 'a@x.com' }], latestAt: '2026-11-16T09:00:00Z' })
  )
  const daysAgo = (days: number) => new Date(now - days * 86_400_000).toISOString()
  const base = { canAct: true, hidden: false, model: open, latestAt: null, createdAt: daysAgo(2), now }

  it('hides it from a member who cannot act or read the steps', () => {
    expect(getStartedVisibility({ ...base, canAct: false })).toBe('hidden')
  })
  it('hides it once hidden', () => {
    expect(getStartedVisibility({ ...base, hidden: true })).toBe('hidden')
  })
  it('hides it at 6 of 6', () => {
    expect(getStartedVisibility({ ...base, model: complete, latestAt: '2026-11-16T09:00:00Z' })).toBe('hidden')
  })
  it('hides it from a workspace that sends, past the grace days', () => {
    expect(GET_STARTED_GRACE_DAYS).toBe(30)
    expect(getStartedVisibility({ ...base, latestAt: '2026-11-16T09:00:00Z', createdAt: daysAgo(31) })).toBe('hidden')
  })
  it('shows it to a recent workspace that sends with steps open', () => {
    expect(getStartedVisibility({ ...base, latestAt: '2026-11-16T09:00:00Z', createdAt: daysAgo(2) })).toBe('shown')
  })
  it('shows it to an old workspace that never sent', () => {
    expect(getStartedVisibility({ ...base, createdAt: daysAgo(730) })).toBe('shown')
  })
  it('counts an invalid creation date as old', () => {
    expect(getStartedVisibility({ ...base, latestAt: '2026-11-16T09:00:00Z', createdAt: 'not a date' })).toBe('hidden')
  })
  it('loads with no message, but shows nothing while a sending workspace loads', () => {
    expect(getStartedVisibility({ ...base, model: undefined })).toBe('loading')
    expect(getStartedVisibility({ ...base, model: undefined, latestAt: '2026-11-16T09:00:00Z' })).toBe('hidden')
    expect(getStartedVisibility({ ...base, latestAt: undefined })).toBe('hidden')
  })
})

describe('shouldCollapse', () => {
  it('folds the done steps from the threshold', () => {
    const at = (count: number) => ({ ...deriveGetStarted(input()), done: count })
    expect(COLLAPSE_AFTER).toBe(2)
    expect(shouldCollapse(at(COLLAPSE_AFTER - 1))).toBe(false)
    expect(shouldCollapse(at(COLLAPSE_AFTER))).toBe(true)
    // A real model: workspace and provider done
    expect(shouldCollapse(deriveGetStarted(input({ workspace: workspace({ integrations: [ses('i1')] }) })))).toBe(true)
  })
})
