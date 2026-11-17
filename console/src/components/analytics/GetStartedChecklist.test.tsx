import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { ComponentType, ReactNode } from 'react'
import type { StoredPermissions } from '../../services/api/permissions'
import type { Integration, Workspace, WorkspaceMember } from '../../services/api/workspace'

const api = vi.hoisted(() => ({
  listTemplates: vi.fn(),
  listContacts: vi.fn(),
  query: vi.fn(),
  getMembers: vi.fn(),
  listLists: vi.fn(),
  openDrawer: vi.fn(),
  navigate: vi.fn()
}))

vi.mock('../../services/api/client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))
vi.mock('../../services/api/template', () => ({ templatesApi: { list: api.listTemplates } }))
vi.mock('../../services/api/contacts', () => ({ contactsApi: { list: api.listContacts } }))
vi.mock('../../services/api/workspace', () => ({ workspaceService: { getMembers: api.getMembers } }))
vi.mock('../../services/api/list', () => ({ listsApi: { list: api.listLists } }))
vi.mock('./dashboardClient', () => ({ dashboardAnalytics: { query: api.query }, DASHBOARD_REFRESH_MS: 60_000 }))
vi.mock('../contacts/ContactsCsvUploadProvider', () => ({
  useContactsCsvUpload: () => ({ openDrawer: api.openDrawer })
}))

/** The href the router would build: the params filled into the path, then the search. */
const hrefOf = (to: string, params?: Record<string, string>, search?: Record<string, unknown>) => {
  let href = to
  for (const [key, value] of Object.entries(params ?? {})) href = href.replace(`$${key}`, value)
  const qs = new URLSearchParams(
    Object.fromEntries(Object.entries(search ?? {}).map(([key, value]) => [key, String(value)]))
  ).toString()
  return qs ? `${href}?${qs}` : href
}
interface LinkProps {
  to: string
  params?: Record<string, string>
  search?: Record<string, unknown>
  children?: ReactNode
}
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => api.navigate,
  Link: ({ to, params, search, children }: LinkProps) => <a href={hrefOf(to, params, search)}>{children}</a>,
  createLink:
    (Comp: ComponentType<Record<string, unknown>>) =>
    // As the router does: a createLink host never receives `type`.
    ({ to, params, search, type: _type, ...rest }: LinkProps & Record<string, unknown>) => (
      <Comp href={hrefOf(to, params, search)} {...rest} />
    )
}))

import { GetStartedChecklist } from './GetStartedChecklist'
import { getStartedHiddenKey } from './useGetStartedHidden'

const ROOT = 'root@acme.com'
const root = { id: 'u-root', email: ROOT }

const all = { read: true, write: true }
const owner: StoredPermissions = {
  contacts: all,
  lists: all,
  templates: all,
  broadcasts: all,
  transactional: all,
  workspace: all,
  message_history: all,
  web_analytics: all,
  blog: all
}

const daysAgo = (days: number) => new Date(Date.now() - days * 86_400_000).toISOString()

const ses = (senders: { email: string; is_default: boolean }[] = []): Integration =>
  ({
    id: 'ses1',
    name: 'Main',
    type: 'email',
    email_provider: {
      kind: 'ses',
      senders: senders.map((sender, index) => ({ id: `s${index}`, name: '', ...sender })),
      rate_limit_per_minute: 0
    }
  }) as Integration

const workspaceOf = (
  fields: { integrations?: Integration[]; roles?: boolean; created?: string; web?: boolean } = {}
): Workspace =>
  ({
    id: 'ws1',
    name: 'Acme',
    created_at: fields.created ?? daysAgo(2),
    updated_at: daysAgo(2),
    integrations: fields.integrations ?? [],
    settings: {
      timezone: 'Europe/Paris',
      ...(fields.roles ? { marketing_email_provider_id: 'ses1', transactional_email_provider_id: 'ses1' } : {}),
      ...(fields.web ? { web_analytics: { enabled: true } } : {})
    }
  }) as unknown as Workspace

const ready = () => workspaceOf({ integrations: [ses([{ email: 'hello@acme.com', is_default: true }])], roles: true })

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

interface Data {
  contacts?: { email: string }[]
  total?: number
  templates?: number
  members?: WorkspaceMember[]
}
const answer = (data: Data = {}) => {
  api.listContacts.mockResolvedValue({ contacts: (data.contacts ?? [{ email: ROOT }]).map((c) => ({ ...c, created_at: daysAgo(1) })) })
  api.query.mockResolvedValue({ data: [{ count: data.total ?? (data.contacts ?? [{ email: ROOT }]).length }] })
  api.listTemplates.mockResolvedValue({ templates: Array.from({ length: data.templates ?? 0 }, (_, i) => ({ id: `t${i}` })) })
  api.getMembers.mockResolvedValue({ members: data.members ?? [member({ user_id: root.id, email: ROOT, role: 'owner' })] })
  api.listLists.mockResolvedValue({ lists: [{ id: 'test', name: 'Test List' }] })
}

interface Props {
  workspace?: Workspace
  permissions?: StoredPermissions
  role?: 'root' | 'owner' | 'member'
  user?: { id: string; email: string }
  latestAt?: string | null
}
const tree = (client: QueryClient, props: Props = {}) => (
  <QueryClientProvider client={client}>
    <I18nProvider i18n={i18n}>
      <GetStartedChecklist
        workspace={props.workspace ?? workspaceOf()}
        permissions={props.permissions ?? owner}
        role={props.role ?? 'root'}
        user={props.user ?? root}
        latestAt={props.latestAt === undefined ? null : props.latestAt}
      />
    </I18nProvider>
  </QueryClientProvider>
)
const newClient = () => new QueryClient({ defaultOptions: { queries: { retry: false } } })
const renderList = (props: Props = {}) => render(tree(newClient(), props))
const settle = () => new Promise((resolve) => setTimeout(resolve, 30))
const step = (id: string) => document.querySelector(`[data-step="${id}"]`) as HTMLElement
const primaries = () => document.querySelectorAll('.ant-btn-primary')

beforeEach(() => {
  window.ROOT_EMAIL = ROOT
  window.localStorage.clear()
  for (const mock of Object.values(api)) mock.mockReset()
  answer()
})
afterEach(() => {
  window.ROOT_EMAIL = ''
})

describe('GetStartedChecklist', () => {
  it('starts a new workspace at 1 of 6, with one primary action: connect a provider', async () => {
    renderList()
    expect(await screen.findByText('1 of 6 done')).toBeInTheDocument()
    expect(screen.getByText('Get started')).toBeInTheDocument()
    expect(within(step('workspace')).getByRole('img', { name: 'Done' })).toBeInTheDocument()
    expect(primaries()).toHaveLength(1)
    const primary = primaries()[0]
    expect(primary).toHaveTextContent('Connect an email provider')
    expect(primary.closest('a')).toHaveAttribute('href', '/console/workspace/ws1/settings/integrations')
    expect(within(step('provider')).getByRole('img', { name: 'Next step' })).toBeInTheDocument()
  })

  it('shows a skeleton while the steps load, before any send', async () => {
    api.listTemplates.mockImplementation(() => new Promise(() => {}))
    renderList()
    expect(await screen.findByTestId('get-started-loading')).toBeInTheDocument()
  })

  it('makes the sender the next step once a provider is connected without roles', async () => {
    renderList({ workspace: workspaceOf({ integrations: [ses([{ email: 'hello@acme.com', is_default: true }])] }) })
    expect(await screen.findByText('2 of 6 done')).toBeInTheDocument()
    expect(primaries()).toHaveLength(1)
    expect(primaries()[0]).toHaveTextContent('Open integrations')
    expect(within(step('sender')).getByRole('img', { name: 'Next step' })).toBeInTheDocument()
  })

  it('keeps the contacts step open with only the owner, and imports from the Contacts page with the lists', async () => {
    answer({ contacts: [{ email: 'Root@Acme.com' }] })
    renderList({ workspace: ready(), latestAt: null })
    expect(await screen.findByText('3 of 6 done')).toBeInTheDocument()
    const contacts = step('contacts')
    expect(within(contacts).getByRole('img', { name: 'Next step' })).toBeInTheDocument()
    const button = within(contacts).getByRole('button', { name: /Import contacts/ })
    expect(button).toHaveClass('ant-btn-primary')
    await waitFor(() => expect(button).not.toHaveClass('ant-btn-loading'))
    fireEvent.click(button)
    expect(api.navigate).toHaveBeenCalledWith({ to: '/console/workspace/$workspaceId/contacts', params: { workspaceId: 'ws1' } })
    expect(api.openDrawer).toHaveBeenCalledWith('ws1', [{ id: 'test', name: 'Test List' }], true)
  })

  it('offers a new broadcast and a new notification for the first send', async () => {
    answer({ contacts: [{ email: 'a@x.com' }, { email: ROOT }], templates: 1 })
    renderList({ workspace: ready() })
    expect(await screen.findByText('5 of 6 done')).toBeInTheDocument()
    // Folded: the done steps are pills, the send step is the one row left.
    const send = step('send')
    const broadcast = within(send).getByText('New broadcast').closest('a') as HTMLElement
    expect(broadcast).toHaveAttribute('href', '/console/workspace/ws1/broadcasts?compose=new')
    expect(broadcast).toHaveClass('ant-btn-primary')
    const notification = within(send).getByText('New notification').closest('a') as HTMLElement
    expect(notification).toHaveAttribute('href', '/console/workspace/ws1/transactional-notifications?new=true')
    expect(notification).not.toHaveClass('ant-btn-primary')
    expect(primaries()).toHaveLength(1)
  })

  it('folds the done steps into pills saying what was done, and expands them on demand', async () => {
    answer({ contacts: [{ email: 'a@x.com' }, { email: 'b@x.com' }], total: 1240, templates: 3 })
    renderList({ workspace: ready() })
    expect(await screen.findByText('Amazon SES connected')).toBeInTheDocument()
    expect(screen.getByText('Workspace created')).toBeInTheDocument()
    expect(screen.getByText('Sends as hello@acme.com')).toBeInTheDocument()
    expect(await screen.findByText('1,240 contacts')).toBeInTheDocument()
    expect(screen.getByText('3 templates')).toBeInTheDocument()
    expect(step('provider')).toBeNull()

    const toggle = screen.getByRole('button', { name: /Show done steps/ })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(toggle)
    expect(within(step('provider')).getByRole('img', { name: 'Done' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Hide done steps/ })).toHaveAttribute('aria-expanded', 'true')
  })

  it('names each role in the sender pill when their senders differ', async () => {
    const two = {
      ...ready(),
      integrations: [
        ses([{ email: 'news@acme.com', is_default: true }]),
        { ...ses([{ email: 'app@acme.com', is_default: true }]), id: 'pm1' }
      ],
      settings: { timezone: 'UTC', marketing_email_provider_id: 'ses1', transactional_email_provider_id: 'pm1' }
    } as unknown as Workspace
    renderList({ workspace: two })
    expect(await screen.findByText('Marketing sends as news@acme.com')).toBeInTheDocument()
    expect(screen.getByText('Transactional sends as app@acme.com')).toBeInTheDocument()
  })

  it('counts teammates and API keys apart in their pills', async () => {
    answer({
      members: [
        member({ user_id: root.id, email: ROOT, role: 'owner' }),
        member({ user_id: 'u2', email: 'jane@example.com' }),
        member({ email: 'joe@example.com', invitation_id: 'inv' }),
        member({ user_id: 'k1', email: 'key@api.notifuse', type: 'api_key' })
      ]
    })
    renderList({ workspace: workspaceOf({ integrations: [ses()] }) })
    expect(await screen.findByText('2 teammates')).toBeInTheDocument()
    expect(screen.getByText('1 API key')).toBeInTheDocument()
  })

  it('renders nothing at 6 of 6, even with the optional steps open', async () => {
    answer({ contacts: [{ email: 'a@x.com' }], templates: 1 })
    const { container } = renderList({ workspace: ready(), latestAt: daysAgo(1) })
    await waitFor(() => expect(api.listTemplates).toHaveBeenCalled())
    await settle()
    expect(container).toBeEmptyDOMElement()
  })

  it('hides for good once hidden', async () => {
    const client = newClient()
    const { unmount } = render(tree(client))
    fireEvent.click(await screen.findByRole('button', { name: 'Hide' }))
    expect(screen.queryByText('Get started')).not.toBeInTheDocument()
    expect(window.localStorage.getItem(getStartedHiddenKey(root.id, 'ws1'))).toBe('true')
    unmount()

    const again = render(tree(newClient()))
    await settle()
    expect(again.container).toBeEmptyDOMElement()
  })

  it('renders and asks nothing without Templates read', async () => {
    const { container } = renderList({ permissions: { ...owner, templates: { read: false, write: false } } })
    await settle()
    expect(container).toBeEmptyDOMElement()
    expect(api.listTemplates).not.toHaveBeenCalled()
  })

  it('renders and asks nothing for a member who is not an owner, even one who writes the workspace', async () => {
    const { container } = renderList({ role: 'member', user: { id: 'u2', email: 'jane@example.com' } })
    await settle()
    expect(container).toBeEmptyDOMElement()
    expect(api.listTemplates).not.toHaveBeenCalled()
    expect(api.listContacts).not.toHaveBeenCalled()
    expect(api.getMembers).not.toHaveBeenCalled()
  })

  it('renders and asks nothing for a workspace past its first days that already sends', async () => {
    const { container } = renderList({ workspace: workspaceOf({ created: daysAgo(31) }), latestAt: daysAgo(1) })
    await settle()
    expect(container).toBeEmptyDOMElement()
    expect(api.listTemplates).not.toHaveBeenCalled()
    expect(api.getMembers).not.toHaveBeenCalled()
  })

  it('stays for a recent workspace that sends with steps still open', async () => {
    renderList({ workspace: workspaceOf({ created: daysAgo(3) }), latestAt: daysAgo(1) })
    expect(await screen.findByText('2 of 6 done')).toBeInTheDocument()
    expect(screen.getByText('First email sent')).toBeInTheDocument()
  })

  it('shows no template button without Templates write', async () => {
    renderList({ permissions: { ...owner, templates: { read: true, write: false } }, role: 'owner' })
    await screen.findByText('1 of 6 done')
    expect(within(step('template')).queryByText('Browse the gallery')).not.toBeInTheDocument()
    expect(within(step('team')).getByText('Open Team & API keys').closest('a')).toHaveAttribute(
      'href',
      '/console/workspace/ws1/settings/team'
    )
  })

  it('links the template step to the gallery of a new template', async () => {
    renderList()
    await screen.findByText('1 of 6 done')
    expect(within(step('template')).getByText('Browse the gallery').closest('a')).toHaveAttribute(
      'href',
      '/console/workspace/ws1/templates?new=true'
    )
  })

  it('mentions the blog, linked to its page, to a member who can read it', async () => {
    renderList()
    expect((await screen.findByText('a blog')).closest('a')).toHaveAttribute('href', '/console/workspace/ws1/blog')
  })

  it('leaves the blog out without Blog read', async () => {
    renderList({ permissions: { ...owner, blog: { read: false, write: false } } })
    await screen.findByText('1 of 6 done')
    expect(screen.queryByText('a blog')).not.toBeInTheDocument()
  })

  it('reads the members itself for a platform admin, and ignores the admins listed as owners', async () => {
    answer({
      members: [
        member({ user_id: root.id, email: ROOT, role: 'owner' }),
        member({ user_id: 'u-ops', email: 'ops@acme.com', role: 'owner' })
      ]
    })
    window.ROOT_EMAIL = `${ROOT}, ops@acme.com`
    renderList()
    await screen.findByText('1 of 6 done')
    await waitFor(() => expect(api.getMembers).toHaveBeenCalledWith('ws1'))
    expect(within(step('team')).queryByRole('img', { name: 'Done' })).not.toBeInTheDocument()
  })

  it('does not ask the members for an owner, whose rights already hold them', async () => {
    renderList({ role: 'owner', user: { id: 'u9', email: 'jane@example.com' } })
    await screen.findByText('1 of 6 done')
    expect(api.getMembers).not.toHaveBeenCalled()
    // An owner who is not a platform admin is a teammate of the admin who created the workspace.
    expect(within(step('team')).getByRole('img', { name: 'Done' })).toBeInTheDocument()
  })

  it('leaves the team step unticked while the members load, without holding the checklist', async () => {
    api.getMembers.mockImplementation(() => new Promise(() => {}))
    renderList()
    expect(await screen.findByText('1 of 6 done')).toBeInTheDocument()
    expect(within(step('team')).queryByRole('img', { name: 'Done' })).not.toBeInTheDocument()
  })
})
