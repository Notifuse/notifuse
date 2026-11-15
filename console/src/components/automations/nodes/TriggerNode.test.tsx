import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { NodeProps } from '@xyflow/react'
import { TriggerNode } from './TriggerNode'
import type { AutomationFlowNode, AutomationNodeData } from '../utils/flowConverter'

vi.mock('@xyflow/react', () => ({
  Handle: () => null,
  Position: { Top: 'top', Bottom: 'bottom' },
  useConnection: () => ({ inProgress: false })
}))

vi.mock('../context', () => ({
  useAutomation: () => ({
    workspace: { id: 'ws1' },
    lists: [],
    segments: [],
    templates: [{ id: 'tpl-welcome', name: 'Welcome email' }]
  })
}))

const broadcastGet = vi.hoisted(() => vi.fn())
vi.mock('../../../services/api/broadcast', () => ({ broadcastApi: { get: broadcastGet } }))

const renderNode = (config: Record<string, unknown>) => {
  const data: AutomationNodeData = { nodeType: 'trigger', config, label: 'Trigger' }
  const props = { data, selected: false } as NodeProps<AutomationFlowNode>
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>
        <TriggerNode {...props} />
      </I18nProvider>
    </QueryClientProvider>
  )
}

describe('TriggerNode', () => {
  it('names a link click trigger and shows the link it waits for', () => {
    renderNode({ event_kind: 'email.link_clicked', link_url: '/pricing', frequency: 'every_time' })

    expect(screen.getByText('Email Link Clicked')).toBeInTheDocument()
    expect(screen.getByText('Link contains /pricing')).toBeInTheDocument()
  })

  it('says any link when the trigger has no link filter', () => {
    renderNode({ event_kind: 'email.link_clicked', frequency: 'once' })

    expect(screen.getByText('Any link')).toBeInTheDocument()
  })

  it('keeps the generic display of the other kinds', () => {
    renderNode({ event_kind: 'email.clicked', frequency: 'once' })

    expect(screen.getByText('Email Clicked')).toBeInTheDocument()
    expect(screen.queryByText('Any link')).not.toBeInTheDocument()
  })

  it('names the broadcast and template filters once they are read', async () => {
    broadcastGet.mockResolvedValue({ broadcast: { id: 'bc-1', name: 'November newsletter' } })
    renderNode({ event_kind: 'email.link_clicked', broadcast_id: 'bc-1', template_id: 'tpl-welcome', frequency: 'once' })

    expect(await screen.findByText('Broadcast: November newsletter')).toBeInTheDocument()
    expect(screen.getByText('Template: Welcome email')).toBeInTheDocument()
    expect(broadcastGet).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'bc-1' })
  })

  it('shows the IDs of filters it cannot name', async () => {
    broadcastGet.mockRejectedValue(new Error('forbidden'))
    renderNode({ event_kind: 'email.link_clicked', broadcast_id: 'bc-gone', template_id: 'tpl-gone', frequency: 'once' })

    expect(screen.getByText('Broadcast: bc-gone')).toBeInTheDocument()
    expect(screen.getByText('Template: tpl-gone')).toBeInTheDocument()
    await vi.waitFor(() => expect(broadcastGet).toHaveBeenCalled())
    expect(screen.getByText('Broadcast: bc-gone')).toBeInTheDocument()
  })

  it('reads no broadcast for a trigger without that filter', () => {
    broadcastGet.mockClear()
    renderNode({ event_kind: 'email.link_clicked', frequency: 'once' })

    expect(document.querySelector('[data-trigger-broadcast]')).toBeNull()
    expect(document.querySelector('[data-trigger-template]')).toBeNull()
    expect(broadcastGet).not.toHaveBeenCalled()
  })
})
