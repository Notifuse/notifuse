import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { EditPanel } from './EditPanel'
import type { EmailBlock } from '../types'

vi.mock('overlayscrollbars-react', () => ({
  OverlayScrollbarsComponent: React.forwardRef<unknown, { children?: React.ReactNode }>(
    ({ children }, _ref) => <div data-testid="scroll">{children}</div>
  )
}))

const tree = {
  id: 'root',
  type: 'mjml',
  attributes: {},
  children: [
    { id: 'head', type: 'mj-head', attributes: {}, children: [] },
    {
      id: 'body',
      type: 'mj-body',
      attributes: { width: '600px' },
      children: [
        {
          id: 'section-1',
          type: 'mj-section',
          attributes: {},
          children: [
            {
              id: 'column-1',
              type: 'mj-column',
              attributes: {},
              children: [{ id: 'text-1', type: 'mj-text', attributes: {}, content: '<p>Hello</p>' }]
            }
          ]
        }
      ]
    }
  ]
} as unknown as EmailBlock

const renderPanel = (overrides: Partial<React.ComponentProps<typeof EditPanel>> = {}) =>
  render(
    <EditPanel
      emailTree={tree}
      selectedBlockId={null}
      onSelectBlock={vi.fn()}
      onUpdateBlock={vi.fn()}
      onCloneBlock={vi.fn()}
      onDeleteBlock={vi.fn()}
      onSaveBlock={vi.fn()}
      {...overrides}
    />
  )

describe('EditPanel', () => {
  it('takes the height of its parent, not one computed from the window', () => {
    const { container } = renderPanel()
    const panel = container.firstElementChild as HTMLElement

    expect(panel.style.height).toBe('')
    expect(panel.getAttribute('style') ?? '').not.toContain('100vh')
    expect(panel.className).toContain('h-full')
    expect(panel.className).toContain('min-h-0')
  })
})
