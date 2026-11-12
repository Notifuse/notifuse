import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { TransactionalNotification } from '../../services/api/transactional_notifications'
import type { UserPermissions, Workspace } from '../../services/api/types'
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
    fireEvent.mouseEnter(within(entry(menu, 'Delete')).getByText('Delete'))
    expect(await screen.findByText('You need Transactional write permission')).toBeInTheDocument()
  })

  it('refuses to delete a notification an integration manages, and names it', async () => {
    renderTable([notification('managed', { integration_id: 'supabase' })])
    const menu = await openMenu('Name managed')

    expect(entry(menu, 'Delete')).toHaveAttribute('aria-disabled', 'true')
    fireEvent.mouseEnter(within(entry(menu, 'Delete')).getByText('Delete'))
    expect(await screen.findByText("Managed by Supabase: it can't be deleted")).toBeInTheDocument()
    // The label of the integration stands next to the name.
    expect(screen.getAllByText('Supabase').length).toBeGreaterThan(0)
  })

  it('keeps the template preview disabled until the template is read', async () => {
    renderTable([notification('welcome')], full, handlers(), new Map([['tpl', { status: 'loading' }]]))
    const menu = await openMenu('Name welcome')
    expect(entry(menu, 'Preview template')).toHaveAttribute('aria-disabled', 'true')
  })

  it('does not open the sheet when Enter is pressed on a button of the row', () => {
    const actions = renderTable([notification('welcome')])
    const row = screen.getByText('Name welcome').closest('tr') as HTMLElement

    fireEvent.keyDown(within(row).getByRole('button', { name: 'API command' }), { key: 'Enter' })
    expect(actions.onOpen).not.toHaveBeenCalled()
    fireEvent.keyDown(row, { key: 'Enter' })
    expect(actions.onOpen).toHaveBeenCalledTimes(1)
  })
})
