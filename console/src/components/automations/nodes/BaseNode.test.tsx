import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { BaseNode } from './BaseNode'

const renderNode = (props: { description?: string }) =>
  render(
    <I18nProvider i18n={i18n}>
      <BaseNode type="delay" label="Delay" icon={<span />} description={props.description}>
        <div>2 days</div>
      </BaseNode>
    </I18nProvider>
  )

describe('BaseNode description', () => {
  it('shows the description under the label, above the type summary', () => {
    renderNode({ description: 'Let them read it first' })

    const description = screen.getByText('Let them read it first')
    const summary = screen.getByText('2 days')
    expect(description).toBeInTheDocument()

    // The order is the point: the description is the node's own subtitle, not a footnote under
    // whatever the node type happens to summarise.
    expect(description.compareDocumentPosition(summary)).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
  })

  it('keeps the full text reachable when it is truncated on the card', () => {
    const long =
      'Wait two days so the welcome email has landed before the product tour arrives on top of it'
    renderNode({ description: long })

    expect(screen.getByText(long)).toHaveAttribute('title', long)
  })

  it('renders nothing extra when there is no description', () => {
    const { container } = renderNode({})

    expect(container.querySelector('[title]')).toBeNull()
    expect(screen.getByText('Delay')).toBeInTheDocument()
    expect(screen.getByText('2 days')).toBeInTheDocument()
  })

  it('renders nothing extra for a blank description', () => {
    // BaseNode is handed getNodeDescription's output, but a caller passing '' straight through
    // must not open a gap in the card either.
    const { container } = renderNode({ description: '' })

    expect(container.querySelector('[title]')).toBeNull()
  })
})
