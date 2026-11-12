import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, waitFor } from '@testing-library/react'
import { EditPanel, type CanvasHandle } from './EditPanel'
import type { EmailBlock } from '../types'

vi.mock('overlayscrollbars-react', () => ({
  OverlayScrollbarsComponent: React.forwardRef<unknown, { children?: React.ReactNode }>(
    ({ children }, _ref) => (
      <div data-testid="scroll" data-overlayscrollbars-viewport="">
        {children}
      </div>
    )
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
      onOpenSaveDialog={vi.fn()}
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

  it("opens the editor's save dialog from the toolbar of the selected block", async () => {
    const onOpenSaveDialog = vi.fn()
    renderPanel({ selectedBlockId: 'column-1', onOpenSaveDialog })

    const save = await waitFor(() => {
      const icon = document.querySelector('svg[data-icon="floppy-disk"]')
      expect(icon).not.toBeNull()
      return icon!.closest('button')!
    })
    fireEvent.click(save)

    expect(onOpenSaveDialog).toHaveBeenCalledWith('column-1')
    expect(document.querySelector('.ant-modal')).toBeNull()
  })

  it('renders customer content under the canvas rules, which the console no longer applies globally', () => {
    const canvasRef: { current: CanvasHandle | null } = { current: null }
    const { unmount } = renderPanel({ canvasRef })
    expect(canvasRef.current!.root()!.classList.contains('email-canvas')).toBe(true)
    unmount()
  })

  it('hands over its canvas: the rendered email and the element that scrolls it', () => {
    const canvasRef: { current: CanvasHandle | null } = { current: null }
    const { unmount } = renderPanel({ canvasRef })

    const root = canvasRef.current!.root()
    expect(root).not.toBeNull()
    expect(root!.querySelector('[data-block-id="text-1"]')).not.toBeNull()
    // The mocked scroll area gives no instance: its viewport is found from the root
    const viewport = canvasRef.current!.viewport()
    expect(viewport).not.toBeNull()
    expect(viewport!.getAttribute('data-testid')).toBe('scroll')
    expect(viewport!.contains(root)).toBe(true)

    unmount()
    expect(canvasRef.current).toBeNull()
  })
})
