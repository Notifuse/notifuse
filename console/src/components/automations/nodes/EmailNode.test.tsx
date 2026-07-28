import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { NodeProps } from '@xyflow/react'
import { EmailNode } from './EmailNode'
import type { AutomationNodeData } from '../utils/flowConverter'
import type { Template } from '../../../services/api/types'

// EmailNode only needs Handle/Position/useConnection from @xyflow/react; render
// them as inert so the node can mount without a ReactFlow provider.
vi.mock('@xyflow/react', () => ({
  Handle: () => null,
  Position: { Top: 'top', Bottom: 'bottom' },
  useConnection: () => ({ inProgress: false })
}))

// The reference list of templates comes from the automation context. It is
// populated by AutomationsPage's templates query; EmailNode resolves the
// selected template_id against it to display the template name on the canvas.
let mockTemplates: Template[] = []
vi.mock('../context', () => ({
  useAutomation: () => ({ templates: mockTemplates })
}))

const makeTemplate = (id: string, name: string, category: string): Template =>
  ({ id, name, category, channel: 'email' } as Template)

const renderNode = (config: Record<string, unknown>) => {
  const props = {
    data: { nodeType: 'email', config } as unknown as AutomationNodeData,
    selected: false
  } as unknown as NodeProps<AutomationNodeData>
  return render(
    <I18nProvider i18n={i18n}>
      <EmailNode {...props} />
    </I18nProvider>
  )
}

describe('EmailNode', () => {
  it('shows the template name for a selected template of any category', () => {
    // Regression guard for the "Template set" bug: a welcome-category template is
    // a valid selection for an automation email node (welcome/unsubscribe emails
    // are sent via automations since migration v20), so as long as the reference
    // list contains it, the canvas must show its name — not the "Template set"
    // fallback. This only holds because AutomationsPage no longer restricts the
    // reference list to category:'marketing'.
    mockTemplates = [makeTemplate('welcome-pdf', 'Send Checklist PDF', 'welcome')]
    renderNode({ template_id: 'welcome-pdf' })

    expect(screen.getByText('Send Checklist PDF')).toBeInTheDocument()
    expect(screen.queryByText('Template set')).not.toBeInTheDocument()
  })

  it('falls back to "Template set" when a template is selected but absent from the list', () => {
    mockTemplates = []
    renderNode({ template_id: 'missing-id' })

    expect(screen.getByText('Template set')).toBeInTheDocument()
  })

  it('prompts to select when no template is configured', () => {
    mockTemplates = []
    renderNode({})

    expect(screen.getByText('Select')).toBeInTheDocument()
  })
})
