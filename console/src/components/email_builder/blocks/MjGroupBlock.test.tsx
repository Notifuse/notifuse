import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { MjGroupBlock } from './MjGroupBlock'
import { EmailBlockClass } from '../EmailBlockClass'
import type { EmailBlock, MergedBlockAttributes } from '../types'

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
  it.each([
    [1, '100%'],
    [2, '50%'],
    [3, '33.33%']
  ])('shows the share of the section with %i layout children', (count, share) => {
    const g = groupBlock()
    const siblings = Array.from({ length: count - 1 }, (_, i) => column(`c${i}`))
    const tree = treeWith([g, { id: 'raw', type: 'mj-raw', content: '' }, ...siblings])
    expect(widthPlaceholder(g, tree)).toBe(share)
  })

  it("shows the template's default group width first", () => {
    const g = groupBlock()
    const tree = treeWith([g, column('c')])
    expect(widthPlaceholder(g, tree, { width: '300px' })).toBe('300px')
  })

  it('shows no share for the group of the default attributes', () => {
    const defaults = groupBlock('defaults-group')
    const tree = treeWith([groupBlock(), column('c')], [defaults])
    expect(widthPlaceholder(defaults, tree)).toBe('Enter width')
  })
})
