import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { NodeProps } from '@xyflow/react'
import { DelayNode } from './DelayNode'
import type { AutomationFlowNode, AutomationNodeData } from '../utils/flowConverter'

vi.mock('@xyflow/react', () => ({
  Handle: () => null,
  Position: { Top: 'top', Bottom: 'bottom' },
  useConnection: () => ({ inProgress: false })
}))

const renderNode = (config: Record<string, unknown>) => {
  const data: AutomationNodeData = { nodeType: 'delay', config, label: 'Delay' }
  const props = { data, selected: false } as NodeProps<AutomationFlowNode>
  return render(
    <I18nProvider i18n={i18n}>
      <DelayNode {...props} />
    </I18nProvider>
  )
}

describe('DelayNode', () => {
  it.each([
    [1, 'days', '1 day'],
    [3, 'days', '3 days'],
    [1, 'hours', '1 hour'],
    [2, 'hours', '2 hours'],
    [1, 'minutes', '1 minute'],
    [15, 'minutes', '15 minutes']
  ])('shows %i %s as "%s"', (duration, unit, text) => {
    renderNode({ duration, unit })
    expect(screen.getByText(text)).toBeInTheDocument()
  })

  it('asks for a duration when none is set', () => {
    renderNode({})
    expect(screen.getByText('Configure')).toBeInTheDocument()
  })
})
