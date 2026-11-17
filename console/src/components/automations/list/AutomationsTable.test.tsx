import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Automation } from '../../../services/api/automation'
import { AutomationsTable, type AutomationsTableProps } from './AutomationsTable'

vi.stubGlobal('matchMedia', () => ({
  matches: false,
  addListener() {},
  removeListener() {},
  addEventListener() {},
  removeEventListener() {}
}))

const withEmail: Automation = {
  id: 'auto-1',
  workspace_id: 'ws1',
  name: 'Welcome',
  status: 'live',
  list_id: 'list1',
  trigger: { event_kind: 'contact.created', frequency: 'once' },
  root_node_id: 't1',
  nodes: [
    { id: 't1', automation_id: 'auto-1', type: 'trigger', config: {}, next_node_id: 'e1', position: { x: 0, y: 0 }, created_at: '2026-01-01T00:00:00Z' },
    { id: 'e1', automation_id: 'auto-1', type: 'email', config: { template_id: 'tpl' }, position: { x: 0, y: 150 }, created_at: '2026-01-01T00:00:00Z' }
  ],
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
}

const renderTable = (openRates: AutomationsTableProps['openRates']) =>
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <AutomationsTable
          rows={[withEmail]}
          counts={undefined}
          openRates={openRates}
          names={{ lists: [{ id: 'list1', name: 'Newsletter' }] }}
          canWrite
          canReadHistory
          onOpen={vi.fn()}
          onActivate={vi.fn()}
          onPause={vi.fn()}
          onDelete={vi.fn()}
          onViewMessages={vi.fn()}
        />
      </App>
    </I18nProvider>
  )

const openRateCell = () => {
  const headers = Array.from(document.querySelectorAll('thead th'))
  const column = headers.findIndex((th) => th.textContent === 'Open rate')
  return document.querySelectorAll('tr[data-row-key="auto-1"] td')[column]
}

describe('AutomationsTable open rate', () => {
  it('shows a skeleton while the rates load, not a dash', () => {
    renderTable({ rates: new Map(), loading: true, error: false })
    expect(document.querySelector('[data-open-rate-loading]')).not.toBeNull()
    expect(openRateCell()?.textContent).not.toContain('—')
  })

  it('shows the rate once loaded, and a dash when nothing tracked was sent', () => {
    const { unmount } = renderTable({ rates: new Map([['auto-1', 0.25]]), loading: false, error: false })
    expect(document.querySelector('[data-open-rate-loading]')).toBeNull()
    expect(openRateCell()?.textContent).toMatch(/25/)
    unmount()
    renderTable({ rates: new Map([['auto-1', null]]), loading: false, error: false })
    expect(openRateCell()?.textContent).toContain('—')
  })
})

describe('AutomationsTable width', () => {
  it('fits the 1170 px a 1440 px screen leaves it, actions included', () => {
    // The table used to scroll from 1250 px, hiding Activate and "…" on a 1440 px screen.
    renderTable({ rates: new Map(), loading: false, error: false })
    const width = parseInt(document.querySelector('table')?.style.width ?? '', 10)
    expect(width).toBeGreaterThan(0)
    expect(width).toBeLessThanOrEqual(1170)
  })
})
