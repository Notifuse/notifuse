import { describe, expect, it, vi } from 'vitest'
import { createEvent, render, screen, fireEvent, within } from '@testing-library/react'
import type { MouseEvent, ReactNode } from 'react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { TransactionalNotification } from '../../services/api/transactional_notifications'
import type { UserPermissions, Workspace } from '../../services/api/types'
// The router's Link as an anchor: its search resolved against a current `?q=pass`, its onClick kept.
vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  Link: ({
    search,
    children,
    onClick,
    className
  }: {
    search?: (prev: Record<string, unknown>) => Record<string, string>
    children: ReactNode
    onClick?: (event: MouseEvent<HTMLAnchorElement>) => void
    className?: string
  }) => (
    <a href={`?${new URLSearchParams(search?.({ q: 'pass' })).toString()}`} onClick={onClick} className={className}>
      {children}
    </a>
  )
}))
// Another window than the shipped one, so the tooltip is seen to follow the constant.
vi.mock('./useTransactionalStats', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./useTransactionalStats')>()),
  LAST_SENT_DAYS: 45
}))

import { TransactionalTable, type TransactionalRowActions } from './TransactionalTable'
import type { TemplateState } from './PreviewPanel'

const workspace = {
  id: 'ws1',
  name: 'Acme',
  settings: {},
  integrations: [{ id: 'supabase', name: 'Supabase', type: 'supabase' }]
} as unknown as Workspace

const notification = (id: string, extra: Partial<TransactionalNotification> = {}): TransactionalNotification =>
  ({
    id,
    name: `Name ${id}`,
    description: '',
    channels: { email: { template_id: 'tpl' } },
    tracking_settings: {},
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...extra
  }) as TransactionalNotification

const full = {
  templates: { read: true, write: false },
  transactional: { read: true, write: true },
  message_history: { read: true, write: false }
} as unknown as UserPermissions

const handlers = (): TransactionalRowActions => ({
  onOpen: vi.fn(),
  onTest: vi.fn(),
  onApi: vi.fn(),
  onEdit: vi.fn(),
  onPreview: vi.fn(),
  onCopyId: vi.fn(),
  onLogs: vi.fn(),
  onDelete: vi.fn()
})

const ok: TemplateState = { status: 'ok', template: { id: 'tpl', name: 'Tpl' } as never }
const gate = { forbidden: false, loading: false, error: false }

function renderTable(
  notifications: TransactionalNotification[],
  permissions: UserPermissions = full,
  actions = handlers(),
  templates = new Map<string, TemplateState>([['tpl', ok]])
) {
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <TransactionalTable
          {...actions}
          workspace={workspace}
          permissions={permissions}
          notifications={notifications}
          statsGate={gate}
          lastSentGate={gate}
          templates={templates}
          smtp={false}
          timezone="Europe/Paris"
          now={new Date('2026-11-12T09:00:00Z')}
          footer="footer"
        />
      </App>
    </I18nProvider>
  )
  return actions
}

/** Opens the « … » menu of a row and returns its entries. */
async function openMenu(name: string) {
  const row = screen.getByText(name).closest('tr') as HTMLElement
  fireEvent.click(within(row).getByRole('button', { name: 'More actions' }))
  return await screen.findByRole('menu')
}

const entry = (menu: HTMLElement, label: string) =>
  within(menu).getByText(label).closest('[role="menuitem"]') as HTMLElement

describe('TransactionalTable', () => {
  it('runs each entry of the row menu on its notification, without opening the sheet', async () => {
    const actions = renderTable([notification('welcome')])
    for (const [label, handler] of [
      ['Edit', actions.onEdit],
      ['Preview template', actions.onPreview],
      ['Copy ID', actions.onCopyId],
      ['View in logs', actions.onLogs],
      ['Delete', actions.onDelete]
    ] as const) {
      fireEvent.click(entry(await openMenu('Name welcome'), label))
      expect(handler).toHaveBeenCalledWith(expect.objectContaining({ id: 'welcome' }))
    }
    expect(actions.onOpen).not.toHaveBeenCalled()
  })

  it('disables what the rights do not allow, and says why', async () => {
    renderTable([notification('welcome')], {
      templates: { read: false, write: false },
      transactional: { read: true, write: false },
      message_history: { read: false, write: false }
    } as unknown as UserPermissions)
    const menu = await openMenu('Name welcome')

    for (const label of ['Edit', 'Preview template', 'View in logs', 'Delete']) {
      expect(entry(menu, label)).toHaveAttribute('aria-disabled', 'true')
    }
    expect(entry(menu, 'Copy ID')).not.toHaveAttribute('aria-disabled', 'true')
    // Written in the entry, without hover.
    expect(within(entry(menu, 'Delete')).getByText('You need Transactional write permission')).toBeInTheDocument()
    expect(within(entry(menu, 'View in logs')).getByText('You need access to message history')).toBeInTheDocument()
    expect(within(entry(menu, 'Preview template')).getByText('You need access to templates')).toBeInTheDocument()
  })

  it('refuses to delete a notification an integration manages, and names it', async () => {
    renderTable([notification('managed', { integration_id: 'supabase' })])
    const menu = await openMenu('Name managed')

    expect(entry(menu, 'Delete')).toHaveAttribute('aria-disabled', 'true')
    expect(within(entry(menu, 'Delete')).getByText("Managed by Supabase: it can't be deleted")).toBeInTheDocument()
    // The label of the integration stands next to the name.
    expect(screen.getAllByText('Supabase').length).toBeGreaterThan(0)
  })

  it('keeps the template preview disabled until the template is read', async () => {
    renderTable([notification('welcome')], full, handlers(), new Map([['tpl', { status: 'loading' }]]))
    const menu = await openMenu('Name welcome')
    expect(entry(menu, 'Preview template')).toHaveAttribute('aria-disabled', 'true')
    // Nothing to say while it loads.
    expect(entry(menu, 'Preview template').textContent).toBe('Preview template')
  })

  it('says why the template preview is disabled when its template failed to load', async () => {
    renderTable(
      [notification('welcome')],
      full,
      handlers(),
      new Map([['tpl', { status: 'error', error: new Error('boom'), retry: () => {} }]])
    )
    const menu = await openMenu('Name welcome')
    expect(entry(menu, 'Preview template')).toHaveAttribute('aria-disabled', 'true')
    expect(within(entry(menu, 'Preview template')).getByText("Couldn't load the template")).toBeInTheDocument()
  })

  it('says how far back « Last sent » looked when a notification has no send', async () => {
    renderTable([notification('welcome')])
    const row = screen.getByText('Name welcome').closest('tr') as HTMLElement
    const cells = row.querySelectorAll('td')
    // The column before the actions.
    const dash = within(cells[cells.length - 2] as HTMLElement).getByText('—')
    fireEvent.mouseEnter(dash)
    expect(await screen.findByText('No successful send in the last 45 days')).toBeInTheDocument()
  })

  it('gives each notification a link to its sheet, which opens it once', () => {
    const actions = renderTable([notification('welcome')])
    const link = screen.getByRole('link', { name: 'Name welcome' })
    expect(link.getAttribute('href')).toBe('?q=pass&notification=welcome')

    // The row opens the sheet; the link's own navigation is held back, so one history entry.
    const click = createEvent.click(link)
    fireEvent(link, click)
    expect(click.defaultPrevented).toBe(true)
    expect(actions.onOpen).toHaveBeenCalledTimes(1)
    expect(actions.onOpen).toHaveBeenCalledWith(expect.objectContaining({ id: 'welcome' }))
  })

  it('leaves a modified click on the link to the browser, without opening the sheet here', () => {
    const actions = renderTable([notification('welcome')])
    const link = screen.getByRole('link', { name: 'Name welcome' })
    // jsdom cannot open a new tab: the test holds the navigation back, out of the spy's sight.
    const holdBack = (event: Event) => Event.prototype.preventDefault.call(event)
    window.addEventListener('click', holdBack, { capture: true })

    for (const modifier of [{ metaKey: true }, { ctrlKey: true }, { shiftKey: true }]) {
      const click = createEvent.click(link, modifier)
      const preventDefault = vi.spyOn(click, 'preventDefault')
      fireEvent(link, click)
      expect(preventDefault).not.toHaveBeenCalled()
    }
    expect(actions.onOpen).not.toHaveBeenCalled()
    window.removeEventListener('click', holdBack, { capture: true })
  })

  it('opens the sheet on a click anywhere on the row, but not from its buttons', () => {
    const actions = renderTable([notification('welcome')])
    const row = screen.getByText('Name welcome').closest('tr') as HTMLElement

    fireEvent.click(within(row).getByRole('button', { name: 'API command' }))
    expect(actions.onOpen).not.toHaveBeenCalled()
    fireEvent.click(within(row).getByText('Tpl'))
    expect(actions.onOpen).toHaveBeenCalledTimes(1)
    // The link is the row's keyboard stop, not the row itself.
    expect(row).not.toHaveAttribute('tabindex')
  })
})
