import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MjSectionBlock } from './MjSectionBlock'
import type { EmailBlock, MergedBlockAttributes } from '../types'
import { MJ_SECTION_DEFAULTS } from '../mjml-defaults'
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

describe('section gutter', () => {
  const sectionWithGutter = (gutter: string | undefined, ...children: unknown[]) =>
    ({
      id: 'sec',
      type: 'mj-section',
      attributes: gutter === undefined ? {} : { gutter },
      children
    }) as unknown as EmailBlock

  const treeOf = (section: EmailBlock) =>
    ({ id: 'body', type: 'mj-body', attributes: {}, children: [section] }) as unknown as EmailBlock

  const renderSection = (section: EmailBlock) =>
    render(<>{new MjSectionBlock(section).getEdit({ ...previewProps(), emailTree: treeOf(section) })}</>)

  it('narrows the column cells and pads the sides that face another column', () => {
    // safe-mjml 1.1.0: .mj-column-per-48-333333, padding 0 1.666667% / 0 0 0 1.666667%
    const { container } = renderSection(sectionWithGutter('20px', column('a'), column('b')))

    const first = cellOf(container, 'a')
    const second = cellOf(container, 'b')
    expect([first.style.width, first.style.paddingLeft, first.style.paddingRight]).toEqual(['50%', '0%', '1.666667%'])
    expect([second.style.width, second.style.paddingLeft, second.style.paddingRight]).toEqual([
      '50%',
      '1.666667%',
      '0%'
    ])
    expect(first.style.boxSizing).toBe('border-box')
  })

  it('takes the gutter of the default section attributes', () => {
    const section = sectionWithGutter(undefined, column('a'), column('b'))
    const { container } = render(
      <>
        {new MjSectionBlock(section).getEdit({
          ...previewProps({ 'mj-section': { gutter: '20px' } }),
          emailTree: treeOf(section)
        })}
      </>
    )
    expect(cellOf(container, 'a').style.paddingRight).toBe('1.666667%')
  })

  it('leaves the cells as they were without a gutter', () => {
    const { container } = renderSection(sectionWithGutter(undefined, column('a'), column('b')))
    const first = cellOf(container, 'a')
    expect(first.style.width).toBe('50%')
    expect(first.style.paddingLeft).toBe('')
    expect(first.style.paddingRight).toBe('')
  })

  it('pads the columns of a group, not the group', () => {
    // safe-mjml 1.1.0, 600px body: the group stays 50%, its columns get padding 3.333333% on their inner side
    const section = sectionWithGutter('20px', group(), column('c'))
    const { container } = renderSection(section)

    expect(cellOf(container, 'g').style.paddingLeft).toBe('')
    expect(cellOf(container, 'c').style.paddingLeft).toBe('1.666667%')
    const groupColumns = Array.from(container.querySelectorAll<HTMLElement>('[data-group-column-width]'))
    expect(groupColumns.map((cell) => [cell.style.paddingLeft, cell.style.paddingRight])).toEqual([
      ['0%', '3.333333%'],
      ['3.333333%', '0%']
    ])
  })

  it('writes the gap the settings field gets, in px by default, and removes it when cleared', () => {
    const onUpdate = vi.fn()
    const section = sectionWithGutter(undefined, column('a'))
    render(
      <>
        {new MjSectionBlock(section).renderSettingsPanel(
          onUpdate,
          MJ_SECTION_DEFAULTS as unknown as MergedBlockAttributes,
          treeOf(section)
        )}
      </>
    )
    fireEvent.click(screen.getByRole('button', { name: /Spacing/ }))

    const input = screen.getByPlaceholderText('None')
    const typed = expect.objectContaining({ coalesceKey: expect.any(String) })
    fireEvent.change(input, { target: { value: '20' } })
    expect(onUpdate).toHaveBeenLastCalledWith({ gutter: '20px' }, typed)
    fireEvent.change(input, { target: { value: '' } })
    expect(onUpdate).toHaveBeenLastCalledWith({ gutter: undefined }, typed)
  })
})
