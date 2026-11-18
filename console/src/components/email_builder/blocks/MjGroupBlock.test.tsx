import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { MjGroupBlock } from './MjGroupBlock'
import { EmailBlockClass } from '../EmailBlockClass'
import type { EmailBlock, MergedBlockAttributes } from '../types'
import type { PreviewProps } from './BaseEmailBlock'

const column = (id: string) => ({ id, type: 'mj-column', attributes: {}, children: [] })
const groupBlock = (id = 'g', attributes: Record<string, unknown> = {}) => ({
  id,
  type: 'mj-group',
  attributes,
  children: [column(`${id}-c`)]
})

const treeWith = (sectionChildren: unknown[], attributeChildren: unknown[] = []) =>
  ({
    id: 'root',
    type: 'mjml',
    children: [
      {
        id: 'head',
        type: 'mj-head',
        children: [{ id: 'attrs', type: 'mj-attributes', children: attributeChildren }]
      },
      {
        id: 'body',
        type: 'mj-body',
        children: [{ id: 'sec', type: 'mj-section', attributes: {}, children: sectionChildren }]
      }
    ]
  }) as unknown as EmailBlock

const widthPlaceholder = (
  group: unknown,
  tree?: EmailBlock,
  blockDefaults: Record<string, unknown> = {}
) => {
  const { container } = render(
    <>
      {new MjGroupBlock(group as EmailBlock).renderSettingsPanel(
        vi.fn(),
        blockDefaults as MergedBlockAttributes,
        tree
      )}
    </>
  )
  return container.querySelector('input')?.getAttribute('placeholder')
}

describe('new groups', () => {
  it('get no width, so they take their share of the section as in MJML', () => {
    expect(EmailBlockClass.createBlock('mj-group').attributes).not.toHaveProperty('width')
  })
})

describe('group Width placeholder', () => {
  // The field shows the number; the % unit is the share's.
  it.each([
    [1, '100'],
    [2, '50'],
    [3, '33.33']
  ])('shows the share of the section with %i layout children', (count, share) => {
    const g = groupBlock()
    const siblings = Array.from({ length: count - 1 }, (_, i) => column(`c${i}`))
    const tree = treeWith([g, { id: 'raw', type: 'mj-raw', content: '' }, ...siblings])
    expect(widthPlaceholder(g, tree)).toBe(share)
  })

  it("shows the template's default group width first", () => {
    const g = groupBlock()
    const tree = treeWith([g, column('c')])
    expect(widthPlaceholder(g, tree, { width: '300px' })).toBe('300')
    const unit = screen.getByLabelText('Group width unit', { selector: '.ant-segmented' })
    expect(within(unit).getByRole('radio', { name: 'px' })).toBeChecked()
  })

  it('shows no share for the group of the default attributes', () => {
    const defaults = groupBlock('defaults-group')
    const tree = treeWith([groupBlock(), column('c')], [defaults])
    expect(widthPlaceholder(defaults, tree)).toBeNull()
  })
})

describe('group Width warning', () => {
  const warning = /go .*% past its width/
  const renderPanel = (group: unknown, tree: EmailBlock) =>
    render(<>{new MjGroupBlock(group as EmailBlock).renderSettingsPanel(vi.fn(), {} as MergedBlockAttributes, tree)}</>)

  it('warns when the section goes past 100%', () => {
    const g = groupBlock('g', { width: '70%' })
    renderPanel(g, treeWith([g, { ...column('c'), attributes: { width: '60%' } }]))
    expect(screen.getByText(/go 30% past its width/)).toBeInTheDocument()
  })

  it('says nothing when the section fits', () => {
    const g = groupBlock('g', { width: '40%' })
    renderPanel(g, treeWith([g, { ...column('c'), attributes: { width: '60%' } }]))
    expect(screen.queryByText(warning)).toBeNull()
  })
})

describe('group canvas', () => {
  const props = () =>
    ({
      selectedBlockId: null,
      onSelectBlock: vi.fn(),
      onUpdateBlock: vi.fn(),
      onCloneBlock: vi.fn(),
      onDeleteBlock: vi.fn(),
      attributeDefaults: {},
      emailTree: {} as EmailBlock,
      onSaveBlock: vi.fn()
    }) as unknown as PreviewProps

  const cellWidths = (group: unknown) => {
    const { container } = render(<>{new MjGroupBlock(group as EmailBlock).getEdit(props())}</>)
    return Array.from(container.querySelectorAll<HTMLElement>('[data-group-column-width]')).map(
      (cell) => [cell.style.flexBasis, cell.style.maxWidth]
    )
  }

  it('gives each column its own width, as in the email', () => {
    const group = {
      id: 'g',
      type: 'mj-group',
      attributes: {},
      children: [
        { ...column('a'), attributes: { width: '30%' } },
        { ...column('b'), attributes: { width: '70%' } }
      ]
    }
    expect(cellWidths(group)).toEqual([
      ['30%', '30%'],
      ['70%', '70%']
    ])
  })

  it('gives a column without a width its share of the group', () => {
    const group = { id: 'g', type: 'mj-group', attributes: {}, children: [column('a'), column('b'), column('c')] }
    expect(cellWidths(group)).toEqual([
      ['33.33%', '33.33%'],
      ['33.33%', '33.33%'],
      ['33.33%', '33.33%']
    ])
  })
})
