import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App } from 'antd'
import type { EmailBlock, MJMLComponentType } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { EmailBlockFactory } from './EmailBlockFactory'
import { inheritedAttributes } from '../utils/inheritedAttributes'
import { blur, fresh, openSection, renderPanel, setRows } from './settingsPanelTestUtils'

// FileSrc needs the file manager; its own tests cover it. Here it is a plain URL input that writes on blur.
vi.mock('../ui/FileSrc', () => ({
  default: ({ value, onChange }: { value?: string; onChange: (value: string | undefined) => void }) => (
    <input aria-label="Image URL" defaultValue={value} onBlur={(e) => onChange(e.target.value || undefined)} />
  )
}))

const block = (type: MJMLComponentType, attributes: Record<string, unknown> = {}) =>
  ({ id: `${type}-1`, type, attributes, children: [] }) as unknown as EmailBlock

// Opens every closed section, so that every row of the panel is drawn.
const openAll = () => {
  for (const header of screen.getAllByRole('button', { expanded: false })) fireEvent.click(header)
}

// The settings row holding the element labelled `label`.
const rowOf = (label: string) => screen.getByLabelText(label).closest('[data-testid="settings-row"]') as HTMLElement
const rowTitled = (title: string) =>
  screen.getByText(title, { selector: 'span' }).closest('[data-testid="settings-row"]') as HTMLElement

const type = (input: HTMLElement, value: string) => {
  fireEvent.focus(input)
  fireEvent.change(input, { target: { value } })
}

const typed = expect.objectContaining({ coalesceKey: expect.any(String) })

// Picks a colour of the palette in the colour field labelled `label`.
const pickColor = async (label: string) => {
  fireEvent.click(screen.getByRole('button', { name: label }))
  const palette = await screen.findByRole('group', { name: 'Palette' })
  fireEvent.click(within(palette).getByRole('button', { name: '#E02424' }))
}

// Picks an option of the antd select labelled `label`.
const chooseOption = async (label: string, option: string) => {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: label }))
  await waitFor(() => expect(document.querySelector('.ant-select-item-option')).not.toBeNull())
  const item = [...document.querySelectorAll('.ant-select-item-option')].find((el) => el.textContent === option)
  fireEvent.click(item as HTMLElement)
}

// A visible border on the four sides (a transparent colour would disable the width).
const borders = (prefix: 'border' | 'innerBorder') =>
  Object.fromEntries(['Top', 'Right', 'Bottom', 'Left'].map((side) => [`${prefix}${side}`, '1px solid #000000']))

const lastWrite = (onUpdate: ReturnType<typeof vi.fn>) => onUpdate.mock.calls.at(-1)!

describe('structure panels: no « set » mark straight out of createBlock', () => {
  it.each(['mj-body', 'mj-wrapper', 'mj-section', 'mj-column', 'mj-group'] as MJMLComponentType[])(
    '%s shows no dot and no set count, every section open',
    (blockType) => {
      renderPanel(fresh(blockType))
      openAll()
      expect(setRows()).toHaveLength(0)
      expect(screen.queryByLabelText(/\d+ set/)).toBeNull()
    }
  )
})

describe('body panel', () => {
  it('writes the width typed, in px', () => {
    const { onUpdate } = renderPanel(block('mj-body'))
    type(screen.getByLabelText('Email width'), '640')
    expect(onUpdate).toHaveBeenLastCalledWith({ width: '640px' }, typed)
  })

  it('writes the background colour', async () => {
    const { onUpdate } = renderPanel(block('mj-body'))
    await pickColor('Email background')
    expect(onUpdate).toHaveBeenLastCalledWith({ backgroundColor: '#e02424' }, undefined)
  })

  it('writes the CSS class', () => {
    const { onUpdate } = renderPanel(block('mj-body'))
    openSection(/Advanced/)
    blur('CSS class', 'email-body')
    expect(onUpdate).toHaveBeenLastCalledWith({ cssClass: 'email-body' }, undefined)
  })

  it('warns above 650 px', () => {
    renderPanel(block('mj-body', { width: '700px' }))
    expect(screen.getByText(/Email widths above 650px/)).toBeInTheDocument()
  })

  it('warns on an inherited width above 650 px', () => {
    renderPanel(block('mj-body'), { width: '720px' })
    expect(screen.getByText(/Email widths above 650px/)).toBeInTheDocument()
  })

  it('says nothing at 650 px', () => {
    renderPanel(block('mj-body', { width: '650px' }))
    expect(screen.queryByText(/Email widths above 650px/)).toBeNull()
  })
})

describe('wrapper panel', () => {
  it('writes the background colour', async () => {
    const { onUpdate } = renderPanel(block('mj-wrapper'))
    await pickColor('Background color')
    expect(onUpdate).toHaveBeenLastCalledWith({ backgroundColor: '#e02424' }, undefined)
  })

  it('writes the background image URL', () => {
    const { onUpdate } = renderPanel(block('mj-wrapper'))
    fireEvent.click(within(screen.getByLabelText('Background type', { selector: '.ant-segmented' })).getByText('Image'))
    blur('Image URL', 'https://cdn.example.com/hero.jpg')
    expect(onUpdate).toHaveBeenLastCalledWith({ backgroundUrl: 'https://cdn.example.com/hero.jpg' }, undefined)
  })

  it('writes the background position', () => {
    const { onUpdate } = renderPanel(block('mj-wrapper', { backgroundUrl: 'https://cdn.example.com/hero.jpg' }))
    fireEvent.click(within(screen.getByRole('group', { name: 'Background position' })).getByRole('button', { name: 'Center' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ backgroundPosition: 'center center' }, undefined)
  })

  it('writes full width as full-width', () => {
    const { onUpdate } = renderPanel(block('mj-wrapper'))
    openSection(/Layout/)
    fireEvent.click(screen.getByRole('switch', { name: 'Full width' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ fullWidth: 'full-width' }, undefined)
  })

  it('writes full width off as an empty value', () => {
    const { onUpdate } = renderPanel(block('mj-wrapper', { fullWidth: 'full-width' }))
    openSection(/Layout/)
    fireEvent.click(screen.getByRole('switch', { name: 'Full width' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ fullWidth: '' }, undefined)
  })

  it('writes the text alignment', () => {
    const { onUpdate } = renderPanel(block('mj-wrapper'))
    openSection(/Layout/)
    fireEvent.click(screen.getByRole('radio', { name: 'Left' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ textAlign: 'left' }, undefined)
  })

  it('writes the four border keys', () => {
    const { onUpdate } = renderPanel(block('mj-wrapper', borders('border')))
    openSection(/Border and corners/)
    type(screen.getByRole('spinbutton', { name: 'Border width' }), '2')
    const [updates, options] = lastWrite(onUpdate)
    expect(Object.keys(updates)).toEqual(['borderTop', 'borderRight', 'borderBottom', 'borderLeft'])
    expect(updates.borderTop).toBe('2px solid #000000')
    expect(options).toEqual(typed)
  })

  it('writes the corners', () => {
    const { onUpdate } = renderPanel(block('mj-wrapper'))
    openSection(/Border and corners/)
    type(screen.getByRole('spinbutton', { name: 'Corners' }), '8')
    expect(onUpdate).toHaveBeenLastCalledWith({ borderRadius: '8px' }, typed)
  })

  it('writes the padding side typed, alone', () => {
    const { onUpdate } = renderPanel(block('mj-wrapper'))
    openSection(/Spacing/)
    type(within(screen.getByRole('group', { name: 'Around the sections' })).getByLabelText('Top'), '24')
    expect(onUpdate).toHaveBeenLastCalledWith({ paddingTop: '24px' }, typed)
  })

  it('writes the CSS class', () => {
    const { onUpdate } = renderPanel(block('mj-wrapper'))
    openSection(/Advanced/)
    blur('CSS class', 'hero')
    expect(onUpdate).toHaveBeenLastCalledWith({ cssClass: 'hero' }, undefined)
  })
})

describe('section panel', () => {
  it('writes the background colour', async () => {
    const { onUpdate } = renderPanel(block('mj-section'))
    await pickColor('Background color')
    expect(onUpdate).toHaveBeenLastCalledWith({ backgroundColor: '#e02424' }, undefined)
  })

  it('keeps the colour when the background switches to Image, as its fallback', () => {
    const { onUpdate } = renderPanel(block('mj-section', { backgroundColor: '#123456' }))
    fireEvent.click(within(screen.getByLabelText('Background type', { selector: '.ant-segmented' })).getByText('Image'))
    expect(onUpdate).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Fallback color' })).toHaveTextContent('#123456')
    blur('Image URL', 'https://cdn.example.com/hero.jpg')
    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect(onUpdate).toHaveBeenLastCalledWith({ backgroundUrl: 'https://cdn.example.com/hero.jpg' }, undefined)
  })

  it('writes the background size', async () => {
    const { onUpdate } = renderPanel(block('mj-section', { backgroundUrl: 'https://cdn.example.com/hero.jpg' }))
    await chooseOption('Background size', 'Cover')
    expect(onUpdate).toHaveBeenLastCalledWith({ backgroundSize: 'cover' }, undefined)
  })

  it('writes the background repeat', async () => {
    const { onUpdate } = renderPanel(block('mj-section', { backgroundUrl: 'https://cdn.example.com/hero.jpg' }))
    await chooseOption('Background repeat', 'Repeat X')
    expect(onUpdate).toHaveBeenLastCalledWith({ backgroundRepeat: 'repeat-x' }, undefined)
  })

  it('writes a custom background position', () => {
    const { onUpdate } = renderPanel(block('mj-section', { backgroundUrl: 'https://cdn.example.com/hero.jpg' }))
    fireEvent.click(screen.getByRole('button', { name: 'Custom' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ backgroundPosition: '50% 50%' }, undefined)
  })

  it('writes full width as full-width', () => {
    const { onUpdate } = renderPanel(block('mj-section'))
    openSection(/Layout/)
    fireEvent.click(screen.getByRole('switch', { name: 'Full width' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ fullWidth: 'full-width' }, undefined)
  })

  it('writes full width off as an empty value', () => {
    const { onUpdate } = renderPanel(block('mj-section', { fullWidth: 'full-width' }))
    openSection(/Layout/)
    fireEvent.click(screen.getByRole('switch', { name: 'Full width' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ fullWidth: '' }, undefined)
  })

  it('writes the text alignment', () => {
    const { onUpdate } = renderPanel(block('mj-section'))
    openSection(/Layout/)
    fireEvent.click(screen.getByRole('radio', { name: 'Right' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ textAlign: 'right' }, undefined)
  })

  it('writes the direction', () => {
    const { onUpdate } = renderPanel(block('mj-section'))
    openSection(/Layout/)
    fireEvent.click(screen.getByRole('radio', { name: 'Right to left' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ direction: 'rtl' }, undefined)
  })

  it('writes the four border keys', () => {
    const { onUpdate } = renderPanel(block('mj-section', borders('border')))
    openSection(/Border and corners/)
    type(screen.getByRole('spinbutton', { name: 'Border width' }), '3')
    const [updates] = lastWrite(onUpdate)
    expect(Object.keys(updates)).toEqual(['borderTop', 'borderRight', 'borderBottom', 'borderLeft'])
  })

  it('writes the corners', () => {
    const { onUpdate } = renderPanel(block('mj-section'))
    openSection(/Border and corners/)
    type(screen.getByRole('spinbutton', { name: 'Corners' }), '6')
    expect(onUpdate).toHaveBeenLastCalledWith({ borderRadius: '6px' }, typed)
  })

  it('writes the padding side typed, alone', () => {
    const { onUpdate } = renderPanel(block('mj-section'))
    openSection(/Spacing/)
    type(within(screen.getByRole('group', { name: 'Around the columns' })).getByLabelText('Bottom'), '40')
    expect(onUpdate).toHaveBeenLastCalledWith({ paddingBottom: '40px' }, typed)
  })

  it('shows the inherited padding as placeholders, not 0', () => {
    renderPanel(block('mj-section'), { paddingTop: '32px' })
    openSection(/Spacing/)
    const box = screen.getByRole('group', { name: 'Around the columns' })
    expect(within(box).getByLabelText('Top')).toHaveAttribute('placeholder', '32')
    expect(within(box).getByLabelText('Bottom')).toHaveAttribute('placeholder', '20')
  })

  it('writes the column gap in px by default', () => {
    const { onUpdate } = renderPanel(block('mj-section'))
    openSection(/Spacing/)
    const gap = screen.getByRole('spinbutton', { name: 'Column gap' })
    expect(gap).toHaveAttribute('placeholder', 'None')
    type(gap, '24')
    expect(onUpdate).toHaveBeenLastCalledWith({ gutter: '24px' }, typed)
  })

  it('writes the column gap in %, by its unit switch and typed', () => {
    const { onUpdate } = renderPanel(block('mj-section', { gutter: '24px' }))
    openSection(/Spacing/)
    fireEvent.click(within(rowOf('Column gap')).getByRole('radio', { name: '%' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ gutter: '24%' }, undefined)
  })

  it('writes a typed column gap in the % it holds', () => {
    const { onUpdate } = renderPanel(block('mj-section', { gutter: '5%' }))
    type(screen.getByRole('spinbutton', { name: 'Column gap' }), '6')
    expect(onUpdate).toHaveBeenLastCalledWith({ gutter: '6%' }, typed)
  })

  it('writes the CSS class', () => {
    const { onUpdate } = renderPanel(block('mj-section'))
    openSection(/Advanced/)
    blur('CSS class', 'hero')
    expect(onUpdate).toHaveBeenLastCalledWith({ cssClass: 'hero' }, undefined)
  })

  it("takes its placeholders from the type's mj-attributes, not from the block defaults", () => {
    renderPanel(block('mj-section'), { borderRadius: '12px' })
    openSection(/Border and corners/)
    expect(screen.getByRole('spinbutton', { name: 'Corners' })).toHaveAttribute('placeholder', '12')
  })
})

describe('column panel', () => {
  it('writes the width typed, in the % it holds', () => {
    const { onUpdate } = renderPanel(block('mj-column', { width: '100%' }))
    type(screen.getByLabelText('Column width'), '40')
    expect(onUpdate).toHaveBeenLastCalledWith({ width: '40%' }, typed)
  })

  it('writes the vertical alignment', () => {
    const { onUpdate } = renderPanel(block('mj-column'))
    fireEvent.click(screen.getByRole('radio', { name: 'Middle' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ verticalAlign: 'middle' }, undefined)
  })

  it('writes the background colour', async () => {
    const { onUpdate } = renderPanel(block('mj-column'))
    openSection(/Colors/)
    await pickColor('Column background')
    expect(onUpdate).toHaveBeenLastCalledWith({ backgroundColor: '#e02424' }, undefined)
  })

  it('writes the inner background colour, its help saying it needs a padding', async () => {
    const { onUpdate } = renderPanel(block('mj-column'))
    openSection(/Colors/)
    expect(screen.getByRole('button', { name: 'About Inner background' })).toBeInTheDocument()
    await pickColor('Inner background')
    expect(onUpdate).toHaveBeenLastCalledWith({ innerBackgroundColor: '#e02424' }, undefined)
  })

  it('writes the four border keys', () => {
    const { onUpdate } = renderPanel(block('mj-column', borders('border')))
    openSection(/Border and corners/)
    type(within(rowTitled('Border')).getByRole('spinbutton', { name: 'Border width' }), '2')
    const [updates] = lastWrite(onUpdate)
    expect(Object.keys(updates)).toEqual(['borderTop', 'borderRight', 'borderBottom', 'borderLeft'])
  })

  it('writes the corners', () => {
    const { onUpdate } = renderPanel(block('mj-column'))
    openSection(/Border and corners/)
    type(screen.getByRole('spinbutton', { name: 'Corners' }), '4')
    expect(onUpdate).toHaveBeenLastCalledWith({ borderRadius: '4px' }, typed)
  })

  it('writes the inner border under the innerBorder keys', () => {
    const { onUpdate } = renderPanel(block('mj-column', borders('innerBorder')))
    openSection(/Border and corners/)
    type(within(rowTitled('Inner border')).getByRole('spinbutton', { name: 'Border width' }), '3')
    const [updates, options] = lastWrite(onUpdate)
    expect(Object.keys(updates)).toEqual(['innerBorderTop', 'innerBorderRight', 'innerBorderBottom', 'innerBorderLeft'])
    expect(updates.innerBorderTop).toBe('3px solid #000000')
    expect(options).toEqual(typed)
  })

  it('shows the inner border it holds, and marks it set', () => {
    renderPanel(block('mj-column', borders('innerBorder')))
    const row = rowTitled('Inner border')
    expect(within(row).getByRole('spinbutton', { name: 'Border width' })).toHaveValue('1')
    expect(row).toHaveAttribute('data-set', 'true')
  })

  it('writes the inner corners', () => {
    const { onUpdate } = renderPanel(block('mj-column'))
    openSection(/Border and corners/)
    type(screen.getByRole('spinbutton', { name: 'Inner corners' }), '8')
    expect(onUpdate).toHaveBeenLastCalledWith({ innerBorderRadius: '8px' }, typed)
  })

  it('writes the padding side typed, alone', () => {
    const { onUpdate } = renderPanel(block('mj-column'))
    openSection(/Spacing/)
    type(within(screen.getByRole('group', { name: 'Inside the column' })).getByLabelText('Left'), '16')
    expect(onUpdate).toHaveBeenLastCalledWith({ paddingLeft: '16px' }, typed)
  })

  it('writes the CSS class', () => {
    const { onUpdate } = renderPanel(block('mj-column'))
    openSection(/Advanced/)
    blur('CSS class', 'col-left')
    expect(onUpdate).toHaveBeenLastCalledWith({ cssClass: 'col-left' }, undefined)
  })
})

// A group in a section, beside a column: the share and the overflow need the section.
const renderGroupInSection = (group: EmailBlock, sibling: Record<string, unknown> = {}) => {
  const tree = {
    id: 'root',
    type: 'mjml',
    children: [
      { id: 'head', type: 'mj-head', children: [] },
      {
        id: 'body',
        type: 'mj-body',
        attributes: {},
        children: [
          {
            id: 'section',
            type: 'mj-section',
            attributes: {},
            children: [group, { id: 'sibling', type: 'mj-column', attributes: sibling, children: [] }]
          }
        ]
      }
    ]
  } as unknown as EmailBlock
  const onUpdate = vi.fn()
  const inherited = inheritedAttributes(tree, group, EmailBlockClass.extractAttributeDefaults(tree))
  render(<App>{EmailBlockFactory.createBlock(group).renderSettingsPanel(onUpdate, inherited as never, tree)}</App>)
  return { onUpdate }
}

describe('group panel', () => {
  it('writes the width typed, in % by default', () => {
    const { onUpdate } = renderPanel(block('mj-group'))
    type(screen.getByLabelText('Group width'), '40')
    expect(onUpdate).toHaveBeenLastCalledWith({ width: '40%' }, typed)
  })

  it('shows its share of the section as the width placeholder', () => {
    renderGroupInSection(block('mj-group'))
    expect(screen.getByLabelText('Group width')).toHaveAttribute('placeholder', '50')
  })

  it('still warns when the section overflows', () => {
    renderGroupInSection(block('mj-group', { width: '70%' }), { width: '60%' })
    expect(screen.getByText(/go 30% past its width/)).toBeInTheDocument()
  })

  it('writes the vertical alignment', () => {
    const { onUpdate } = renderPanel(block('mj-group'))
    fireEvent.click(screen.getByRole('radio', { name: 'Bottom' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ verticalAlign: 'bottom' }, undefined)
  })

  it('writes the height: px starts at 100px', () => {
    const { onUpdate } = renderPanel(block('mj-group'))
    fireEvent.click(within(rowOf('Group height')).getByRole('radio', { name: 'px' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ height: '100px' }, undefined)
  })

  it('writes the height typed, in px', () => {
    const { onUpdate } = renderPanel(block('mj-group', { height: '200px' }))
    type(screen.getByLabelText('Group height'), '250')
    expect(onUpdate).toHaveBeenLastCalledWith({ height: '250px' }, typed)
  })

  it('writes the direction', () => {
    const { onUpdate } = renderPanel(block('mj-group'))
    fireEvent.click(screen.getByRole('radio', { name: 'Right to left' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ direction: 'rtl' }, undefined)
  })

  it('writes the background colour', async () => {
    const { onUpdate } = renderPanel(block('mj-group'))
    openSection(/Background/)
    await pickColor('Background color')
    expect(onUpdate).toHaveBeenLastCalledWith({ backgroundColor: '#e02424' }, undefined)
  })

  it('writes the background image URL', () => {
    const { onUpdate } = renderPanel(block('mj-group'))
    openSection(/Background/)
    fireEvent.click(within(screen.getByLabelText('Background type', { selector: '.ant-segmented' })).getByText('Image'))
    blur('Image URL', 'https://cdn.example.com/a.jpg')
    expect(onUpdate).toHaveBeenLastCalledWith({ backgroundUrl: 'https://cdn.example.com/a.jpg' }, undefined)
  })

  it('writes the CSS class', () => {
    const { onUpdate } = renderPanel(block('mj-group'))
    openSection(/Advanced/)
    blur('CSS class', 'group')
    expect(onUpdate).toHaveBeenLastCalledWith({ cssClass: 'group' }, undefined)
  })
})

describe('Padding read from an imported shorthand', () => {
  it('shows the sides of a padding="10px" shorthand the block holds', () => {
    renderPanel({
      id: 'wrapper-1',
      type: 'mj-wrapper',
      attributes: { padding: '10px 20px' }
    } as unknown as EmailBlock)
    openSection(/Spacing/)
    const box = screen.getByRole('group', { name: 'Around the sections' })
    expect(within(box).getByLabelText('Top')).toHaveValue('10')
    expect(within(box).getByLabelText('Right')).toHaveValue('20')
  })
})
