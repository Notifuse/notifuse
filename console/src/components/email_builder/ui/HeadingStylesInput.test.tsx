import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App } from 'antd'
import type { EmailBlock } from '../types'
import HeadingStylesInput from './HeadingStylesInput'
import { setRows } from '../blocks/settingsPanelTestUtils'
import {
  defaultHeadingStyles,
  readHeadingStyles,
  writeHeadingStyles,
  type HeadingLevel,
  type HeadingStyles
} from '../utils/headingStyles'

const email = () =>
  ({
    id: 'root',
    type: 'mjml',
    children: [
      { id: 'head', type: 'mj-head', children: [{ id: 'attrs', type: 'mj-attributes', attributes: {}, children: [] }] },
      { id: 'body', type: 'mj-body', attributes: {}, children: [] }
    ]
  }) as unknown as EmailBlock

// An email whose heading styles differ from the defaults at H1 and H2
const customised = () => {
  const styles = defaultHeadingStyles({})
  styles[1] = { ...styles[1], fontSize: '40px', marginTop: '12px' }
  styles[2] = { ...styles[2], fontSize: '30px', fontWeight: '600', lineHeight: '1.5', marginBottom: '20px' }
  return writeHeadingStyles(email(), styles)
}

const renderInput = (tree: EmailBlock) => {
  const onUpdateBlock = vi.fn()
  render(
    <App>
      <HeadingStylesInput emailTree={tree} onUpdateBlock={onUpdateBlock} />
    </App>
  )
  // The heading styles of the tree the last edit gives, with the head it rewrote
  const saved = (): HeadingStyles => {
    const [headId, head] = onUpdateBlock.mock.calls.at(-1) as [string, EmailBlock]
    expect(headId).toBe('head')
    return readHeadingStyles({ ...tree, children: [head, ...(tree.children ?? []).slice(1)] } as EmailBlock) as HeadingStyles
  }
  return { onUpdateBlock, saved }
}

const pickLevel = (level: HeadingLevel) => fireEvent.click(screen.getByRole('radio', { name: `H${level}` }))

const typeNumber = (label: string, value: string) => {
  const input = screen.getByRole('spinbutton', { name: label })
  fireEvent.focus(input)
  fireEvent.change(input, { target: { value } })
}

const typeText = (label: string, value: string) => {
  const input = screen.getByRole('textbox', { name: label })
  fireEvent.focus(input)
  fireEvent.change(input, { target: { value } })
  fireEvent.blur(input)
}

describe('HeadingStylesInput', () => {
  it("shows the chosen level's values", () => {
    renderInput(customised())
    expect(screen.getByRole('spinbutton', { name: 'Size' })).toHaveValue('40')
    expect(screen.getByRole('spinbutton', { name: 'Space above' })).toHaveValue('12')

    pickLevel(2)
    expect(screen.getByRole('spinbutton', { name: 'Size' })).toHaveValue('30')
    expect(screen.getByRole('textbox', { name: 'Line height' })).toHaveValue('1.5')
    expect(screen.getByRole('spinbutton', { name: 'Space below' })).toHaveValue('20')
    expect(screen.getByText('600')).toBeInTheDocument()
    expect(screen.getByText('Heading 2')).toBeInTheDocument()
  })

  it('writes the font of the chosen level', () => {
    const { saved } = renderInput(email())
    pickLevel(3)
    typeText('Font', 'Georgia, serif')
    const styles = saved()
    expect(styles[3].fontFamily).toBe('Georgia, serif')
    expect(styles[1].fontFamily).toBeUndefined()
  })

  it('writes the size', () => {
    const { saved } = renderInput(email())
    pickLevel(2)
    typeNumber('Size', '26')
    expect(saved()[2].fontSize).toBe('26px')
  })

  it('writes a typed size as one undo step, in whole pixels', () => {
    const { onUpdateBlock, saved } = renderInput(email())
    pickLevel(2)
    typeNumber('Size', '26.6')
    expect(saved()[2].fontSize).toBe('27px')
    const options = onUpdateBlock.mock.calls.at(-1)?.[2] as { coalesceKey?: string } | undefined
    expect(options?.coalesceKey).toMatch(/^headings:2:fontSize:/)
  })

  it('writes the weight', async () => {
    const { saved } = renderInput(email())
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Weight' }))
    await waitFor(() => expect(document.querySelector('.ant-select-item-option')).not.toBeNull())
    const option = [...document.querySelectorAll('.ant-select-item-option')].find((el) => el.textContent === '800')
    fireEvent.click(option as Element)
    expect(saved()[1].fontWeight).toBe('800')
  })

  it('writes the line height', () => {
    const { saved } = renderInput(email())
    typeText('Line height', '1.6')
    expect(saved()[1].lineHeight).toBe('1.6')
  })

  it('writes the colour', async () => {
    const { saved } = renderInput(email())
    fireEvent.click(screen.getByRole('button', { name: 'Color' }))
    const palette = await screen.findByRole('group', { name: 'Palette' })
    fireEvent.click(within(palette).getByRole('button', { name: '#E02424' }))
    expect(saved()[1].color).toBe('#e02424')
  })

  it('removes a level colour when cleared', () => {
    const styles = defaultHeadingStyles({})
    styles[1] = { ...styles[1], color: '#112233' }
    const { saved } = renderInput(writeHeadingStyles(email(), styles))
    fireEvent.click(screen.getByRole('button', { name: 'Clear Color' }))
    expect(saved()[1].color).toBeUndefined()
  })

  it('writes the space above and below', () => {
    const { saved } = renderInput(email())
    typeNumber('Space above', '16')
    expect(saved()[1].marginTop).toBe('16px')
    typeNumber('Space below', '4')
    expect(saved()[1].marginBottom).toBe('4px')
  })

  it('keeps a text it cannot use in its field, in red, and saves nothing', () => {
    const { onUpdateBlock } = renderInput(email())
    typeText('Font', 'Open_Sans')
    expect(screen.getByRole('textbox', { name: 'Font' })).toHaveAttribute('aria-invalid', 'true')
    typeText('Line height', '150%')
    expect(screen.getByRole('textbox', { name: 'Line height' })).toHaveAttribute('aria-invalid', 'true')
    expect(onUpdateBlock).not.toHaveBeenCalled()
  })

  it('resets the chosen level to its defaults, and only it', () => {
    const { saved } = renderInput(customised())
    pickLevel(2)
    fireEvent.click(screen.getByRole('button', { name: 'Reset H2' }))
    const styles = saved()
    expect(styles[2]).toEqual(defaultHeadingStyles()[2])
    expect(styles[1].fontSize).toBe('40px')
  })

  it('shows the message and no field over styles edited by hand', () => {
    const tree = writeHeadingStyles(email(), defaultHeadingStyles({}))
    const managed = tree.children![0].children!.find((child) => child.type === 'mj-style') as unknown as {
      content: string
    }
    managed.content = managed.content.replace('22px', '44px')
    const { onUpdateBlock } = renderInput(tree)
    expect(screen.getByText('These styles were edited by hand. Edit them in the mj-style block.')).toBeInTheDocument()
    expect(screen.queryByRole('spinbutton')).toBeNull()
    expect(screen.queryByRole('textbox')).toBeNull()
    expect(screen.queryByRole('radio')).toBeNull()
    expect(onUpdateBlock).not.toHaveBeenCalled()
  })

  it('marks no row, no level and offers no level reset on default styles', () => {
    renderInput(writeHeadingStyles(email(), defaultHeadingStyles()))
    expect(setRows()).toHaveLength(0)
    expect(screen.queryAllByTitle(/changed/)).toHaveLength(0)
    expect(screen.queryByRole('button', { name: 'Reset H1' })).toBeNull()
  })

  it('marks the rows of the chosen level that differ from its defaults', () => {
    renderInput(customised())
    const marked = () =>
      // The label of each row marked « set »
      setRows().map((row) => row.querySelector('span > span.min-w-0')?.textContent)
    expect(marked()).toEqual(['Size, weight', 'Space above, below'])
    pickLevel(2)
    expect(marked()).toEqual(['Size, weight', 'Line height', 'Space above, below'])
  })

  it('names the levels that differ from their defaults on hover, with no dot', () => {
    renderInput(customised())
    expect(screen.getAllByTitle(/, changed$/).map((level) => level.getAttribute('title'))).toEqual([
      'H1, changed',
      'H2, changed'
    ])
    // No dot drawn in the level picker, as on the rows
    const picker = screen.getByRole('radio', { name: 'H1' }).closest('.ant-segmented') as HTMLElement
    expect(picker.querySelector('.rounded-full')).toBeNull()
  })

  it("resets a row to the level's defaults, leaving the other fields", () => {
    const { saved } = renderInput(customised())
    pickLevel(2)
    fireEvent.click(screen.getByRole('button', { name: 'Reset Size, weight to the default value' }))
    const styles = saved()
    expect(styles[2]).toEqual({ ...defaultHeadingStyles()[2], lineHeight: '1.5', marginBottom: '20px' })
    expect(styles[1].fontSize).toBe('40px')
  })

  it('resets a level colour and font by removing them', () => {
    const styles = defaultHeadingStyles({})
    styles[1] = { ...styles[1], color: '#123456', fontFamily: 'Georgia' }
    const { saved } = renderInput(writeHeadingStyles(email(), styles))
    fireEvent.click(screen.getByRole('button', { name: 'Reset Color to the default value' }))
    expect(saved()[1]).toEqual({ ...defaultHeadingStyles()[1], fontFamily: 'Georgia' })
  })
})
