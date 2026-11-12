import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Panel } from './Panel'

describe('Panel', () => {
  it('draws a header only when it has a title, with its extra on the right', () => {
    const { container, rerender } = render(<Panel>body</Panel>)
    expect(container.querySelector('[data-panel-head]')).toBeNull()
    rerender(
      <Panel title="Recent failures" icon={<span>i</span>} extra={<a href="#logs">See in logs</a>}>
        body
      </Panel>
    )
    const head = container.querySelector('[data-panel-head]')!
    expect(head).toHaveTextContent('Recent failures')
    expect(head).toHaveTextContent('See in logs')
  })

  it('marks its title data-unreset against the margin of the antd reset', () => {
    render(<Panel title="Sources">x</Panel>)
    expect(screen.getByRole('heading', { name: 'Sources' })).toHaveAttribute('data-unreset')
  })

  it('pads its body, except when flush for a table or a chart', () => {
    const { container, rerender } = render(<Panel>body</Panel>)
    expect(container.querySelector('[data-panel-body]')!.className).toMatch(/\bpx-3\.5\b/)
    rerender(<Panel flush>body</Panel>)
    expect(container.querySelector('[data-panel-body]')!.className).not.toMatch(/\bpx-/)
  })

  it('renders its footer and passes its className', () => {
    const { container } = render(
      <Panel footer={<span>10 of 12 broadcasts</span>} className="col-span-2">
        body
      </Panel>
    )
    expect(screen.getByText('10 of 12 broadcasts').closest('[data-panel-footer]')).not.toBeNull()
    expect((container.firstChild as HTMLElement).className).toMatch(/\bcol-span-2\b/)
  })

  it('turns its border and title red for a danger zone', () => {
    const { container, rerender } = render(<Panel title="Settings">x</Panel>)
    const section = container.firstChild as HTMLElement
    expect(section.className).toMatch(/\bborder-line\b/)
    expect(screen.getByRole('heading').className).toMatch(/\btext-ink\b/)

    rerender(
      <Panel title="Delete this workspace" tone="danger">
        x
      </Panel>
    )
    expect(section.className).toMatch(/border-danger-fg\/25/)
    expect(section.className).not.toMatch(/\bborder-line\b/)
    expect(screen.getByRole('heading').className).toMatch(/\btext-danger-fg\b/)
  })
})
