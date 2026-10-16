import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { NodeProps } from '@xyflow/react'
import { WebhookNode } from './WebhookNode'
import type { AutomationFlowNode, AutomationNodeData } from '../utils/flowConverter'

// WebhookNode only needs Handle/Position/useConnection from @xyflow/react; render
// them as inert so the node can mount without a ReactFlow provider.
vi.mock('@xyflow/react', () => ({
  Handle: () => null,
  Position: { Top: 'top', Bottom: 'bottom' },
  useConnection: () => ({ inProgress: false })
}))

const renderNode = (url: string) => {
  const data: AutomationNodeData = { nodeType: 'webhook', config: { url }, label: 'Webhook' }
  const props = { data, selected: false } as NodeProps<AutomationFlowNode>
  return render(
    <I18nProvider i18n={i18n}>
      <WebhookNode {...props} />
    </I18nProvider>
  )
}

describe('WebhookNode', () => {
  const original = window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS
  afterEach(() => {
    window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS = original
  })

  // The form only renders for the selected node. The canvas is what shows an
  // automation saved before the server refused its URL, the moment it is opened.
  it('marks a node aimed at an address the server refuses', () => {
    window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS = false
    renderNode('http://10.0.0.5/hook')

    expect(screen.getByText('Refused: private address')).toBeInTheDocument()
  })

  it('does not mark it on a server that delivers to its own network', () => {
    window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS = true
    renderNode('http://10.0.0.5/hook')

    expect(screen.queryByText('Refused: private address')).not.toBeInTheDocument()
  })

  it('does not mark a public receiver', () => {
    window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS = false
    renderNode('https://hooks.example.com/hook')

    expect(screen.queryByText('Refused: private address')).not.toBeInTheDocument()
    expect(screen.getByText('https://hooks.example.com/hook')).toBeInTheDocument()
  })
})
