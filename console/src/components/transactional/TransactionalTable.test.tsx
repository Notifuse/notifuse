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

// A refused entry is drawn disabled and stays on the keyboard's path (antd's disabled one has no tabindex)
const expectRefused = (item: HTMLElement) => {
  expect(item).toHaveClass('ant-dropdown-menu-item-disabled')
  expect(item).toHaveAttribute('tabindex', '-1')
}

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
      expectRefused(entry(menu, label))
    }
    expect(entry(menu, 'Copy ID')).not.toHaveClass('ant-dropdown-menu-item-disabled')
    // Written in the entry, without hover.
    expect(within(entry(menu, 'Delete')).getByText('You need Transactional write permission')).toBeInTheDocument()
    expect(within(entry(menu, 'View in logs')).getByText('You need access to message history')).toBeInTheDocument()
    expect(within(entry(menu, 'Preview template')).getByText('You need access to templates')).toBeInTheDocument()
  })

  it('does nothing on a click on a refused entry', async () => {
    const actions = handlers()
    renderTable(
      [notification('welcome')],
      {
        templates: { read: false, write: false },
        transactional: { read: true, write: false },
        message_history: { read: false, write: false }
      } as unknown as UserPermissions,
      actions
    )
    for (const label of ['Edit', 'Preview template', 'View in logs', 'Delete']) {
      fireEvent.click(entry(await openMenu('Name welcome'), label))
    }
    expect(actions.onEdit).not.toHaveBeenCalled()
    expect(actions.onPreview).not.toHaveBeenCalled()
    expect(actions.onLogs).not.toHaveBeenCalled()
    expect(actions.onDelete).not.toHaveBeenCalled()
  })

  it('keeps a refused Send test on the keyboard path and says why', () => {
    const actions = handlers()
    renderTable(
      [notification('welcome')],
      { ...full, transactional: { read: true, write: false } } as unknown as UserPermissions,
      actions
    )
    const test = screen.getByRole('button', { name: 'Send test' })
    expect(test).not.toBeDisabled()
    expect(test).toHaveAttribute('aria-disabled', 'true')
    expect(test).toHaveAccessibleDescription(
      'You need Templates read and Transactional write permissions to send test emails'
    )
    fireEvent.click(test)
    expect(actions.onTest).not.toHaveBeenCalled()
    expect(actions.onOpen).not.toHaveBeenCalled()
  })

  it('refuses to delete a notification an integration manages, and names it', async () => {
    renderTable([notification('managed', { integration_id: 'supabase' })])
    const menu = await openMenu('Name managed')

    expectRefused(entry(menu, 'Delete'))
    expect(within(entry(menu, 'Delete')).getByText("Managed by Supabase: it can't be deleted")).toBeInTheDocument()
    // The integration's icon stands next to the name, in place of a label
    const row = screen.getByText('Name managed').closest('tr') as HTMLElement
    const icon = within(row).getByRole('img', { name: 'Managed by Supabase (supabase integration)' })
    expect(icon.querySelector('img')).toHaveAttribute('src', '/console/supabase.png')
    expect(within(row).queryByText('Supabase')).not.toBeInTheDocument()
  })

  it('keeps the template preview disabled until the template is read', async () => {
    renderTable([notification('welcome')], full, handlers(), new Map([['tpl', { status: 'loading' }]]))
    const menu = await openMenu('Name welcome')
    expectRefused(entry(menu, 'Preview template'))
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
    expectRefused(entry(menu, 'Preview template'))
    expect(within(entry(menu, 'Preview template')).getByText("Couldn't load the template")).toBeInTheDocument()
  })

  it('says why the template preview is disabled for a notification without template', async () => {
    renderTable([notification('bare', { channels: {} })])
    const menu = await openMenu('Name bare')
    expectRefused(entry(menu, 'Preview template'))
    expect(within(entry(menu, 'Preview template')).getByText('This notification has no email template')).toBeInTheDocument()
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
    // Nor from the padding around the buttons
    fireEvent.click(within(row).getByRole('button', { name: 'API command' }).closest('td') as HTMLElement)
    expect(actions.onOpen).not.toHaveBeenCalled()
    fireEvent.click(within(row).getByText('Tpl'))
    expect(actions.onOpen).toHaveBeenCalledTimes(1)
    // The link is the row's keyboard stop, not the row itself.
    expect(row).not.toHaveAttribute('tabindex')
  })

  it('shows the row buttons on every row, hidden by no class of their own', () => {
    renderTable([notification('welcome'), notification('reset')])
    for (const name of ['Name welcome', 'Name reset']) {
      const row = screen.getByText(name).closest('tr') as HTMLElement
      const buttons = ['Send test', 'API command', 'More actions'].map((label) =>
        within(row).getByRole('button', { name: label })
      )
      // jsdom applies no Tailwind: the classes and styles, from each button up to its row, are what would
      // hide them, a variant such as max-sm:hidden included
      for (const button of buttons) {
        for (let node: HTMLElement | null = button; node && node !== row.parentElement; node = node.parentElement) {
          expect(node.getAttribute('class') ?? '').not.toMatch(/opacity-|invisible|(^|\s|:)hidden/)
          expect(node.style.opacity).not.toBe('0')
          expect(node.style.visibility).not.toBe('hidden')
          expect(node.style.display).not.toBe('none')
        }
      }
    }
  })

  it('puts the pill of a deleted template under its name, which keeps the column to itself', () => {
    const deleted: TemplateState = {
      status: 'ok',
      template: { id: 'tpl', name: 'Tpl', deleted_at: '2026-11-01T00:00:00Z' } as never
    }
    renderTable([notification('welcome')], full, handlers(), new Map([['tpl', deleted]]))
    const name = screen.getByText('Tpl')
    const stack = name.parentElement as HTMLElement
    expect(stack).toHaveClass('flex-col')
    // Short in the list, « Deleted template » in its tooltip and for screen readers
    expect(within(stack).getByText('Deleted')).toHaveAttribute('aria-hidden', 'true')
    expect(within(stack).getByText('Deleted template. This notification still sends with it')).toHaveClass('sr-only')
    // The name first, the pill after it
    expect(stack.firstElementChild).toBe(name)
  })

  it('fixes the widths of the template and figure columns and leaves the rest to the name', () => {
    renderTable([notification('welcome')])
    const widths = [...document.querySelectorAll('table colgroup col')].map((col) => (col as HTMLElement).style.width)
    expect(widths).toEqual(['', '120px', '84px', '110px', '106px', '106px', '62px', '70px', '120px', '108px'])
    // Fixed, so a long name truncates rather than widen its column
    const table = document.querySelector('table') as HTMLElement
    expect(table.style.tableLayout).toBe('fixed')
    // Scrolls rather than squeeze the name: the fixed widths above plus the name's 120 px minimum
    expect(table.style.width).toBe('1006px')
    // The actions stay pinned to the right edge while it scrolls
    const actions = within(screen.getByText('Name welcome').closest('tr') as HTMLElement)
      .getByRole('button', { name: 'More actions' })
      .closest('td')
    expect(actions).toHaveClass('ant-table-cell-fix-end')
  })
})
