import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { MjSectionBlock } from './MjSectionBlock'
import type { EmailBlock } from '../types'
import type { PreviewProps } from './BaseEmailBlock'

const previewProps = (attributeDefaults: Record<string, Record<string, unknown>> = {}): PreviewProps =>
  ({
    selectedBlockId: null,
    onSelectBlock: vi.fn(),
    onUpdateBlock: vi.fn(),
    onCloneBlock: vi.fn(),
    onDeleteBlock: vi.fn(),
    attributeDefaults,
    emailTree: {} as EmailBlock,
    onSaveBlock: vi.fn()
  }) as unknown as PreviewProps

const column = (id: string, attributes: Record<string, unknown> = {}) => ({
  id,
  type: 'mj-column',
  attributes,
  children: []
})

const group = (attributes: Record<string, unknown> = {}) => ({
  id: 'g',
  type: 'mj-group',
  attributes,
  children: [column('g-c1'), column('g-c2')]
})

const sectionOf = (...children: unknown[]) =>
  ({ id: 'sec', type: 'mj-section', attributes: {}, children }) as unknown as EmailBlock

const cellOf = (container: HTMLElement, id: string) =>
  container.querySelector(`[data-block-id="${id}"]`)?.closest('td') as HTMLTableCellElement

describe('section canvas widths', () => {
  it('shares the section between a group without a width and a column', () => {
    const block = sectionOf(group(), column('c'))
    const { container } = render(<>{new MjSectionBlock(block).getEdit(previewProps())}</>)

    expect(cellOf(container, 'g').style.width).toBe('50%')
    expect(cellOf(container, 'c').style.width).toBe('50%')
  })

  it('gives a group its width once: on its cell, the group filling the cell', () => {
    const block = sectionOf(group({ width: '60%' }), column('c', { width: '40%' }))
    const { container } = render(<>{new MjSectionBlock(block).getEdit(previewProps())}</>)

    expect(cellOf(container, 'g').style.width).toBe('60%')
    const groupDiv = container.querySelector('[data-block-id="g"]') as HTMLElement
    expect(groupDiv.style.width).toBe('100%')
    expect(cellOf(container, 'c').style.width).toBe('40%')
  })

  it("takes a group's width from the template's default attributes", () => {
    const block = sectionOf(group(), column('c'))
    const { container } = render(
      <>{new MjSectionBlock(block).getEdit(previewProps({ 'mj-group': { width: '70%' } }))}</>
    )

    expect(cellOf(container, 'g').style.width).toBe('70%')
    expect(cellOf(container, 'c').style.width).toBe('50%')
  })
})
