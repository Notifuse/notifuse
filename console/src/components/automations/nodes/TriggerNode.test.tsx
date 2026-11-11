import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
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
  useAutomation: () => ({ lists: [], segments: [] })
}))

const renderNode = (config: Record<string, unknown>) => {
  const data: AutomationNodeData = { nodeType: 'trigger', config, label: 'Trigger' }
  const props = { data, selected: false } as NodeProps<AutomationFlowNode>
  return render(
    <I18nProvider i18n={i18n}>
      <TriggerNode {...props} />
    </I18nProvider>
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
})
