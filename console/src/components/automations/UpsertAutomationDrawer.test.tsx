import type { ReactNode } from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { UpsertAutomationDrawer } from './UpsertAutomationDrawer'
import type { Workspace } from '../../services/api/types'

// jsdom lacks matchMedia, which antd's Drawer touches.
vi.stubGlobal('matchMedia', () => ({
  matches: false,
  addListener() {},
  removeListener() {},
  addEventListener() {},
  removeEventListener() {}
}))

// The provider exposes only what this file checks: the permission it gets. The editor inside it
// is exercised by its own tests.
vi.mock('./context', () => ({
  AutomationProvider: ({
    canReadTemplates,
    children
  }: {
    canReadTemplates: boolean
    children: ReactNode
  }) => (
    <div data-testid="automation-provider" data-can-read-templates={String(canReadTemplates)}>
      {children}
    </div>
  ),
  // What the drawer's header reads, inert.
  useAutomation: () => ({
    isEditing: false,
    name: '',
    setName: () => {},
    listId: undefined,
    setListId: () => {},
    exitOnReply: false,
    setExitOnReply: () => {},
    lists: [],
    workspace: { id: 'ws1', integrations: [] },
    hasUnsavedChanges: false,
    isSaving: false,
    save: () => Promise.resolve(),
    validate: () => [],
    canUndo: false,
    canRedo: false,
    undo: () => {},
    redo: () => {}
  })
}))

const workspace = { id: 'ws1', name: 'Workspace' } as unknown as Workspace

vi.mock('./AutomationFlowEditor', () => ({ AutomationFlowEditor: () => null }))

describe('UpsertAutomationDrawer', () => {
  it.each([false, true])('gives the editor Templates read %s', (canRead) => {
    render(
      <I18nProvider i18n={i18n}>
        <App>
          <UpsertAutomationDrawer workspace={workspace} canReadTemplates={canRead} />
        </App>
      </I18nProvider>
    )

    // The provider mounts when the drawer opens.
    fireEvent.click(screen.getByRole('button', { name: 'Create Automation' }))
    expect(screen.getByTestId('automation-provider').getAttribute('data-can-read-templates')).toBe(
      String(canRead)
    )
  })
})
