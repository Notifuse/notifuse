import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from 'antd'
import type { EmailBlock } from '../types'
import { MjAttributesBlock } from './MjAttributesBlock'
import { defaultHeadingStyles, readHeadingStyles, writeHeadingStyles, type HeadingStyles } from '../utils/headingStyles'

const attributesBlock = {
  id: 'attrs',
  type: 'mj-attributes',
  attributes: {},
  children: [{ id: 'attrs-text', type: 'mj-text', attributes: { fontFamily: 'Georgia, serif', color: '#222222' } }]
} as unknown as EmailBlock

const userStyle = { id: 'user-style', type: 'mj-style', attributes: { inline: 'inline' }, content: 'h2 { color: red }' }

const email = (headExtra: unknown[] = []) =>
  ({
    id: 'root',
    type: 'mjml',
    children: [
      { id: 'head', type: 'mj-head', children: [attributesBlock, ...headExtra] },
      { id: 'body', type: 'mj-body', attributes: {}, children: [] }
    ]
  }) as unknown as EmailBlock

const renderPanel = (tree: EmailBlock) => {
  const onUpdateBlock = vi.fn()
  render(
    <App>{new MjAttributesBlock(attributesBlock).renderSettingsPanel(vi.fn(), {} as never, tree, onUpdateBlock)}</App>
  )
  // The tree the panel's last edit gives, with the head it rewrote
  const savedTree = () => {
    const [headId, head] = onUpdateBlock.mock.calls.at(-1) as [string, EmailBlock]
    expect(headId).toBe('head')
    return { ...tree, children: [head, ...(tree.children ?? []).slice(1)] } as EmailBlock
  }
  return { onUpdateBlock, savedTree }
}

const setNumber = async (label: string, value: string) => {
  const input = screen.getByRole('spinbutton', { name: label })
  await userEvent.clear(input)
  await userEvent.type(input, value)
  await userEvent.tab()
}

describe('Default attributes > Headings', () => {
  it('offers a tab for each heading level', () => {
    renderPanel(email())
    for (const level of [1, 2, 3, 4, 5, 6]) {
      expect(screen.getByRole('radio', { name: `H${level}` })).toBeTruthy()
    }
  })

  it('writes the size of the chosen level, before the template’s own styles', async () => {
    const { savedTree } = renderPanel(email([userStyle]))
    await userEvent.click(screen.getByRole('radio', { name: 'H2' }))
    await setNumber('Size', '30')

    const tree = savedTree()
    const styles = readHeadingStyles(tree) as HeadingStyles
    expect(styles[2].fontSize).toBe('30px')
    expect(styles[1].fontSize).toBe('28px')
    // No font or colour of its own: headings take their block's
    expect(styles[2].fontFamily).toBeUndefined()
    expect(styles[2].color).toBeUndefined()
    const head = tree.children?.[0]
    expect(head?.children?.map((child) => child.id).slice(1)).toEqual([expect.any(String), 'user-style'])
  })

  it('resets the chosen level only', async () => {
    const custom = defaultHeadingStyles({})
    custom[1] = { ...custom[1], fontSize: '40px' }
    custom[2] = { ...custom[2], fontSize: '30px' }
    const tree = writeHeadingStyles(email(), custom)
    const { savedTree } = renderPanel(tree)
    await userEvent.click(screen.getByRole('radio', { name: 'H2' }))
    await userEvent.click(screen.getByRole('button', { name: 'Reset H2' }))

    const styles = readHeadingStyles(savedTree()) as HeadingStyles
    expect(styles[2].fontSize).toBe('22px')
    expect(styles[1].fontSize).toBe('40px')
  })

  it('shows no field over styles edited by hand', () => {
    const tree = writeHeadingStyles(email(), defaultHeadingStyles({}))
    const head = tree.children![0]
    const managed = head.children!.find((child) => child.type === 'mj-style') as unknown as { content: string }
    managed.content = managed.content.replace('22px', '44px')
    const { onUpdateBlock } = renderPanel(tree)
    expect(screen.getByText(/edited by hand/)).toBeTruthy()
    expect(screen.queryByRole('spinbutton', { name: 'Size' })).toBeNull()
    expect(onUpdateBlock).not.toHaveBeenCalled()
  })

  it('says when a typed value cannot be used, and saves nothing', async () => {
    const { onUpdateBlock } = renderPanel(email())
    const font = screen.getByRole('textbox', { name: 'Font' })
    await userEvent.type(font, 'Open_Sans')
    await userEvent.tab()
    expect(font).toHaveAttribute('aria-invalid', 'true')
    const lineHeight = screen.getByRole('textbox', { name: 'Line height' })
    await userEvent.clear(lineHeight)
    await userEvent.type(lineHeight, '150%')
    await userEvent.tab()
    expect(lineHeight).toHaveAttribute('aria-invalid', 'true')
    expect(onUpdateBlock).not.toHaveBeenCalled()
  })

  it('counts the heading fields that differ from the defaults on the closed Headings section', () => {
    const styles = defaultHeadingStyles()
    styles[1] = { ...styles[1], fontSize: '40px', marginTop: '12px' }
    styles[3] = { ...styles[3], fontWeight: '600' }
    renderPanel(writeHeadingStyles(email(), styles))
    fireEvent.click(screen.getByRole('button', { name: /^Headings/ }))
    expect(screen.getByLabelText('3 set')).toHaveTextContent('3')
  })
})

