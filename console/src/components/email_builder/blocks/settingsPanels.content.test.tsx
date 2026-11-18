import { describe, it, expect, vi } from 'vitest'
import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import type { EmailBlock, MJMLComponentType } from '../types'
import { blur, fresh, openSection, renderPanel, setRows } from './settingsPanelTestUtils'

// The file manager's button, replaced by one that picks a fixed file
vi.mock('../../file_manager/context', () => ({
  useFileManager: () => ({
    SelectFileButton: ({ onSelect, buttonText }: { onSelect: (url: string) => void; buttonText: string }) => (
      <button type="button" onClick={() => onSelect('https://cdn.test/picked.png')}>
        {buttonText}
      </button>
    )
  })
}))

// Monaco, replaced by a plain text area
vi.mock('@monaco-editor/react', () => ({
  Editor: ({ value, onChange }: { value?: string; onChange?: (value: string) => void }) => (
    <textarea aria-label="Code editor" value={value} onChange={(event) => onChange?.(event.target.value)} />
  )
}))

const blockOf = (type: MJMLComponentType, attributes: Record<string, unknown> = {}, content?: string) =>
  ({ id: `${type}-1`, type, attributes, ...(content !== undefined ? { content } : {}) }) as unknown as EmailBlock

// Opens every closed section of the panel
const openAll = () => {
  for (;;) {
    const closed = screen
      .queryAllByRole('button')
      .find((button) => button.getAttribute('aria-expanded') === 'false' && button.hasAttribute('aria-controls'))
    if (!closed) return
    fireEvent.click(closed)
  }
}

const segmented = (label: string) => screen.getByLabelText(label, { selector: '.ant-segmented' })
const clickSegment = (label: string, option: string) => fireEvent.click(within(segmented(label)).getByTitle(option))

const rowOf = (label: string) =>
  screen.getByText(label, { selector: 'span' }).closest('[data-testid="settings-row"]') as HTMLElement

// Picks a palette colour in the colour field named `label`
const pickColor = async (label: string, color = '#E02424') => {
  fireEvent.click(screen.getByRole('button', { name: label }))
  const palette = await screen.findByRole('group', { name: 'Palette' })
  fireEvent.click(within(palette).getByRole('button', { name: color }))
}

// Picks an option of the antd Select named `label`
const pickOption = async (label: string, option: string) => {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: label }))
  await waitFor(() => expect(document.querySelector('.ant-select-item-option')).not.toBeNull())
  const match = [...document.querySelectorAll('.ant-select-item-option')].find((el) => el.textContent === option)
  if (!match) throw new Error(`No option ${option} in ${label}`)
  fireEvent.click(match)
}

const optionsOf = async (label: string) => {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: label }))
  await waitFor(() => expect(document.querySelector('.ant-select-item-option')).not.toBeNull())
  return [...document.querySelectorAll('.ant-select-item-option')].map((el) => el.textContent)
}

const typeNumber = (input: HTMLElement, value: string) => {
  fireEvent.focus(input)
  fireEvent.change(input, { target: { value } })
}

const sideOf = (box: string, side = 'Top') => within(screen.getByRole('group', { name: box })).getByLabelText(side)

const coalesced = { coalesceKey: expect.any(String) }

describe('Content block settings panels: nothing marked set on a new block', () => {
  // Every field of these panels compares the block's value with the inherited one; createBlock
  // copies the MJML defaults, which are also the inherited values, so no row differs.
  it.each(['mj-text', 'mj-image', 'mj-divider', 'mj-spacer', 'mj-raw', 'mj-liquid'] as MJMLComponentType[])(
    'shows no « set » mark on a %s straight out of createBlock, every section open',
    (type) => {
      renderPanel(fresh(type))
      openAll()
      expect(setRows()).toHaveLength(0)
    }
  )
})

describe('Text settings panel', () => {
  const text = (attributes: Record<string, unknown> = {}) => blockOf('mj-text', attributes, '<p>Hi</p>')

  it('writes the text colour', async () => {
    const { onUpdate } = renderPanel(text())
    await pickColor('Text color')
    expect(onUpdate).toHaveBeenLastCalledWith({ color: '#e02424' }, undefined)
  })

  it('writes the container background', async () => {
    const { onUpdate } = renderPanel(text())
    await pickColor('Container background')
    expect(onUpdate).toHaveBeenLastCalledWith({ containerBackgroundColor: '#e02424' }, undefined)
  })

  it('writes the font family', async () => {
    const { onUpdate } = renderPanel(text())
    openSection(/Font/)
    await pickOption('Font', 'Georgia, serif')
    expect(onUpdate).toHaveBeenLastCalledWith({ fontFamily: 'Georgia, serif' }, undefined)
  })

  it('writes the font size, in px, with a coalesce key', () => {
    const { onUpdate } = renderPanel(text())
    openSection(/Font/)
    typeNumber(screen.getByRole('spinbutton', { name: 'Font size' }), '16')
    expect(onUpdate).toHaveBeenLastCalledWith({ fontSize: '16px' }, coalesced)
  })

  it('writes the font weight', async () => {
    const { onUpdate } = renderPanel(text())
    openSection(/Font/)
    await pickOption('Font weight', 'Bold')
    expect(onUpdate).toHaveBeenLastCalledWith({ fontWeight: 'bold' }, undefined)
  })

  it('writes the font style', () => {
    const { onUpdate } = renderPanel(text())
    openSection(/Font/)
    clickSegment('Style', 'Italic')
    expect(onUpdate).toHaveBeenLastCalledWith({ fontStyle: 'italic' }, undefined)
  })

  it('writes the text transform', () => {
    const { onUpdate } = renderPanel(text())
    openSection(/Font/)
    clickSegment('Case', 'Uppercase')
    expect(onUpdate).toHaveBeenLastCalledWith({ textTransform: 'uppercase' }, undefined)
  })

  it('writes the text decoration', () => {
    const { onUpdate } = renderPanel(text())
    openSection(/Font/)
    clickSegment('Decoration', 'Underline')
    expect(onUpdate).toHaveBeenLastCalledWith({ textDecoration: 'underline' }, undefined)
  })

  it('writes the line height', () => {
    const { onUpdate } = renderPanel(text())
    openSection(/Font/)
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Line height' }), { target: { value: '1.6' } })
    expect(onUpdate).toHaveBeenLastCalledWith({ lineHeight: '1.6' }, coalesced)
  })

  it('writes the letter spacing', () => {
    const { onUpdate } = renderPanel(text())
    openSection(/Font/)
    fireEvent.change(within(rowOf('Letter spacing')).getByRole('spinbutton'), { target: { value: '2' } })
    expect(onUpdate).toHaveBeenLastCalledWith({ letterSpacing: '2px' }, coalesced)
  })

  it('writes the alignment under mj-text\'s own key, align, never textAlign', () => {
    const { onUpdate } = renderPanel(text())
    openSection(/Font/)
    clickSegment('Align', 'Center')
    expect(onUpdate).toHaveBeenLastCalledWith({ align: 'center' }, undefined)
    for (const [updates] of onUpdate.mock.calls) expect(updates).not.toHaveProperty('textAlign')
  })

  it('shows the block\'s align as the selected alignment, and resets align', () => {
    const { onUpdate } = renderPanel(text({ align: 'right' }))
    openSection(/Font/)
    expect(segmented('Align').querySelector('.ant-segmented-item-selected .ant-segmented-item-label')?.getAttribute('title')).toBe('Right')
    fireEvent.click(screen.getByRole('button', { name: 'Reset Align to the inherited value' }))
    // The reset writes the inherited alignment explicitly
    expect(onUpdate).toHaveBeenLastCalledWith({ align: 'left' }, undefined)
  })

  it('writes the height: px starts at 100px, Auto writes auto', () => {
    const { onUpdate } = renderPanel(text())
    openSection(/Size/)
    fireEvent.click(within(segmented('Text height mode')).getByText('px'))
    expect(onUpdate).toHaveBeenLastCalledWith({ height: '100px' }, undefined)
  })

  it('writes the height as auto from a px height', () => {
    const { onUpdate } = renderPanel(text({ height: '120px' }))
    openSection(/Size/)
    fireEvent.click(within(segmented('Text height mode')).getByText('Auto'))
    expect(onUpdate).toHaveBeenLastCalledWith({ height: 'auto' }, undefined)
  })

  it('writes the padding side by side', () => {
    const { onUpdate } = renderPanel(text())
    openSection(/Spacing/)
    typeNumber(sideOf('Around the text'), '14')
    expect(onUpdate).toHaveBeenLastCalledWith({ paddingTop: '14px' }, coalesced)
  })

  it('writes the CSS class', () => {
    const { onUpdate } = renderPanel(text())
    openSection(/Advanced/)
    blur('CSS class', 'intro')
    expect(onUpdate).toHaveBeenLastCalledWith({ cssClass: 'intro' }, undefined)
  })
})

describe('Image settings panel', () => {
  const image = (attributes: Record<string, unknown> = {}) => blockOf('mj-image', attributes)

  it('writes a typed image URL once, on blur', () => {
    const { onUpdate } = renderPanel(image())
    const input = screen.getByLabelText('Image URL')
    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: 'https://cdn.test/h' } })
    fireEvent.change(input, { target: { value: 'https://cdn.test/hero.png' } })
    expect(onUpdate).not.toHaveBeenCalled()
    fireEvent.blur(input)
    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect(onUpdate).toHaveBeenCalledWith({ src: 'https://cdn.test/hero.png' }, undefined)
  })

  it('writes the URL of an image picked in the file manager', () => {
    const { onUpdate } = renderPanel(image())
    fireEvent.click(screen.getByRole('button', { name: 'Browse images' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ src: 'https://cdn.test/picked.png' }, undefined)
  })

  it('previews an http(s) image, and none for a Liquid URL', () => {
    renderPanel(image({ src: 'https://cdn.test/hero.png', alt: 'Hero' }))
    expect(screen.getByRole('img', { name: 'Hero' })).toHaveAttribute('src', 'https://cdn.test/hero.png')
  })

  it('draws no preview for a Liquid URL', () => {
    renderPanel(image({ src: '{{ product.image }}', alt: 'Product' }))
    expect(screen.queryByRole('img', { name: 'Product' })).toBeNull()
  })

  it('writes the alt text', () => {
    const { onUpdate } = renderPanel(image())
    blur('Alt text', 'ACME cart')
    expect(onUpdate).toHaveBeenLastCalledWith({ alt: 'ACME cart' }, undefined)
  })

  it('writes the tooltip as title', () => {
    const { onUpdate } = renderPanel(image())
    blur('Tooltip', 'Our cart')
    expect(onUpdate).toHaveBeenLastCalledWith({ title: 'Our cart' }, undefined)
  })

  it('writes the link as href, in its own Link section', () => {
    const { onUpdate } = renderPanel(image())
    openSection(/Link/)
    blur('Link URL', 'https://acme.test')
    expect(onUpdate).toHaveBeenLastCalledWith({ href: 'https://acme.test' }, undefined)
  })

  it('refuses an invalid link without writing it', () => {
    const { onUpdate } = renderPanel(image())
    openSection(/Link/)
    blur('Link URL', 'acme cart')
    expect(onUpdate).not.toHaveBeenCalled()
    expect(screen.getByText(/Invalid URL format/)).toBeInTheDocument()
  })

  it('writes where the link opens as target', async () => {
    const { onUpdate } = renderPanel(image())
    openSection(/Link/)
    await pickOption('Open in', 'Same window')
    expect(onUpdate).toHaveBeenLastCalledWith({ target: '_self' }, undefined)
  })

  it('writes the link rel', () => {
    const { onUpdate } = renderPanel(image())
    openSection(/Link/)
    blur('Rel', 'noopener')
    expect(onUpdate).toHaveBeenLastCalledWith({ rel: 'noopener' }, undefined)
  })

  it('writes the width in px, with a coalesce key', () => {
    const { onUpdate } = renderPanel(image())
    openSection(/Size/)
    typeNumber(screen.getByLabelText('Image width'), '300')
    expect(onUpdate).toHaveBeenLastCalledWith({ width: '300px' }, coalesced)
  })

  it('writes the height: px starts at 100px', () => {
    const { onUpdate } = renderPanel(image())
    openSection(/Size/)
    fireEvent.click(within(segmented('Image height mode')).getByText('px'))
    expect(onUpdate).toHaveBeenLastCalledWith({ height: '100px' }, undefined)
  })

  it("writes full width on mobile as the strings 'true' and 'false'", () => {
    const { onUpdate } = renderPanel(image())
    openSection(/Size/)
    fireEvent.click(screen.getByRole('switch', { name: 'Full width on mobile' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ fluidOnMobile: 'true' }, undefined)
  })

  it("writes 'false' when full width on mobile is switched off", () => {
    const { onUpdate } = renderPanel(image({ fluidOnMobile: 'true' }))
    openSection(/Size/)
    fireEvent.click(screen.getByRole('switch', { name: 'Full width on mobile' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ fluidOnMobile: 'false' }, undefined)
  })

  it('writes the alignment', () => {
    const { onUpdate } = renderPanel(image())
    openSection(/Size/)
    clickSegment('Image alignment', 'Left')
    expect(onUpdate).toHaveBeenLastCalledWith({ align: 'left' }, undefined)
  })

  it('shows the inherited alignment, not center, when the block sets none', () => {
    renderPanel(image(), { align: 'right' })
    openSection(/Size/)
    const selected = segmented('Image alignment').querySelector('.ant-segmented-item-selected .ant-segmented-item-label')
    expect(selected?.getAttribute('title')).toBe('Right')
    expect(selected?.querySelector('[data-inherited="true"]')).not.toBeNull()
  })

  it('writes the border on its four sides', () => {
    const line = '1px solid #000000'
    const { onUpdate } = renderPanel(image({ borderTop: line, borderRight: line, borderBottom: line, borderLeft: line }))
    openSection(/Border and corners/)
    typeNumber(screen.getByRole('spinbutton', { name: 'Border width' }), '2')
    const [updates, options] = onUpdate.mock.calls.at(-1)!
    expect(Object.keys(updates).sort()).toEqual(['borderBottom', 'borderLeft', 'borderRight', 'borderTop'])
    expect(updates.borderTop).toBe('2px solid #000000')
    expect(options).toEqual(coalesced)
  })

  it('writes the corners as border-radius', () => {
    const { onUpdate } = renderPanel(image())
    openSection(/Border and corners/)
    typeNumber(screen.getByRole('spinbutton', { name: 'Corners' }), '8')
    expect(onUpdate).toHaveBeenLastCalledWith({ borderRadius: '8px' }, coalesced)
  })

  it('writes the container background', async () => {
    const { onUpdate } = renderPanel(image())
    openSection(/Colors/)
    await pickColor('Container background')
    expect(onUpdate).toHaveBeenLastCalledWith({ containerBackgroundColor: '#e02424' }, undefined)
  })

  it('writes the padding side by side', () => {
    const { onUpdate } = renderPanel(image())
    openSection(/Spacing/)
    typeNumber(sideOf('Around the image', 'Left'), '0')
    expect(onUpdate).toHaveBeenLastCalledWith({ paddingLeft: '0px' }, coalesced)
  })

  it('writes the CSS class', () => {
    const { onUpdate } = renderPanel(image())
    openSection(/Advanced/)
    blur('CSS class', 'hero')
    expect(onUpdate).toHaveBeenLastCalledWith({ cssClass: 'hero' }, undefined)
  })
})

describe('Divider settings panel', () => {
  const divider = (attributes: Record<string, unknown> = {}) => blockOf('mj-divider', attributes)

  it('writes the line colour as border-color', async () => {
    const { onUpdate } = renderPanel(divider())
    await pickColor('Line color')
    expect(onUpdate).toHaveBeenLastCalledWith({ borderColor: '#e02424' }, undefined)
  })

  it('writes the line thickness as border-width, in px', () => {
    const { onUpdate } = renderPanel(divider())
    typeNumber(screen.getByLabelText('Line thickness'), '2')
    expect(onUpdate).toHaveBeenLastCalledWith({ borderWidth: '2px' }, coalesced)
  })

  it('clamps the line thickness to 50 px on blur', () => {
    const { onUpdate } = renderPanel(divider())
    const input = screen.getByLabelText('Line thickness')
    act(() => input.focus())
    fireEvent.change(input, { target: { value: '80' } })
    act(() => input.blur())
    expect(onUpdate).toHaveBeenLastCalledWith({ borderWidth: '50px' }, coalesced)
  })

  it('offers the three styles mj-divider accepts', async () => {
    renderPanel(divider())
    expect(await optionsOf('Line style')).toEqual(['Solid', 'Dashed', 'Dotted'])
  })

  it('writes the line style as border-style', async () => {
    const { onUpdate } = renderPanel(divider())
    await pickOption('Line style', 'Dashed')
    expect(onUpdate).toHaveBeenLastCalledWith({ borderStyle: 'dashed' }, undefined)
  })

  it('writes the width in % by default', () => {
    const { onUpdate } = renderPanel(divider())
    typeNumber(screen.getByLabelText('Divider width'), '60')
    expect(onUpdate).toHaveBeenLastCalledWith({ width: '60%' }, coalesced)
  })

  it('switches the width to px, keeping the number', () => {
    const { onUpdate } = renderPanel(divider({ width: '60%' }))
    fireEvent.click(within(segmented('Divider width unit')).getByText('px'))
    expect(onUpdate).toHaveBeenLastCalledWith({ width: '60px' }, undefined)
  })

  it('writes the alignment', () => {
    const { onUpdate } = renderPanel(divider())
    clickSegment('Divider alignment', 'Right')
    expect(onUpdate).toHaveBeenLastCalledWith({ align: 'right' }, undefined)
  })

  it('shows the inherited alignment, not center, when the block sets none', () => {
    renderPanel(divider(), { align: 'left' })
    const selected = segmented('Divider alignment').querySelector('.ant-segmented-item-selected .ant-segmented-item-label')
    expect(selected?.getAttribute('title')).toBe('Left')
  })

  it('writes the container background', async () => {
    const { onUpdate } = renderPanel(divider())
    openSection(/Colors/)
    await pickColor('Container background')
    expect(onUpdate).toHaveBeenLastCalledWith({ containerBackgroundColor: '#e02424' }, undefined)
  })

  it('writes the padding side by side', () => {
    const { onUpdate } = renderPanel(divider())
    openSection(/Spacing/)
    typeNumber(sideOf('Around the line', 'Bottom'), '20')
    expect(onUpdate).toHaveBeenLastCalledWith({ paddingBottom: '20px' }, coalesced)
  })

  it('writes the CSS class', () => {
    const { onUpdate } = renderPanel(divider())
    openSection(/Advanced/)
    blur('CSS class', 'sep')
    expect(onUpdate).toHaveBeenLastCalledWith({ cssClass: 'sep' }, undefined)
  })
})

describe('Spacer settings panel', () => {
  const spacer = (attributes: Record<string, unknown> = {}) => blockOf('mj-spacer', attributes)

  it('writes the height in px, with a coalesce key', () => {
    const { onUpdate } = renderPanel(spacer())
    typeNumber(screen.getByLabelText('Spacer height'), '32')
    expect(onUpdate).toHaveBeenLastCalledWith({ height: '32px' }, coalesced)
  })

  it('clamps the height to 500 px on blur', () => {
    const { onUpdate } = renderPanel(spacer())
    const input = screen.getByLabelText('Spacer height')
    act(() => input.focus())
    fireEvent.change(input, { target: { value: '800' } })
    act(() => input.blur())
    expect(onUpdate).toHaveBeenLastCalledWith({ height: '500px' }, coalesced)
  })

  it('clamps a negative height to 0 px on blur', () => {
    const { onUpdate } = renderPanel(spacer())
    const input = screen.getByLabelText('Spacer height')
    act(() => input.focus())
    fireEvent.change(input, { target: { value: '-5' } })
    act(() => input.blur())
    expect(onUpdate).toHaveBeenLastCalledWith({ height: '0px' }, coalesced)
  })

  it('shows the inherited height as the placeholder', () => {
    renderPanel(spacer())
    expect(screen.getByLabelText('Spacer height')).toHaveAttribute('placeholder', '20')
  })

  it('writes the container background', async () => {
    const { onUpdate } = renderPanel(spacer())
    openSection(/Colors/)
    await pickColor('Container background')
    expect(onUpdate).toHaveBeenLastCalledWith({ containerBackgroundColor: '#e02424' }, undefined)
  })

  it('writes the padding side by side', () => {
    const { onUpdate } = renderPanel(spacer())
    openSection(/Spacing/)
    typeNumber(sideOf('Around the spacer', 'Right'), '8')
    expect(onUpdate).toHaveBeenLastCalledWith({ paddingRight: '8px' }, coalesced)
  })

  it('writes the CSS class', () => {
    const { onUpdate } = renderPanel(spacer())
    openSection(/Advanced/)
    blur('CSS class', 'gap')
    expect(onUpdate).toHaveBeenLastCalledWith({ cssClass: 'gap' }, undefined)
  })
})

describe('Raw HTML settings panel', () => {
  it('writes the HTML content from the code drawer', async () => {
    const { onUpdate } = renderPanel(blockOf('mj-raw', {}, ''))
    fireEvent.click(screen.getByRole('button', { name: 'Set HTML Content' }))
    fireEvent.change(await screen.findByLabelText('Code editor'), { target: { value: '<img src="p.gif">' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ content: '<img src="p.gif">' })
  })

  it('keeps its help in the section tooltip, not inline', () => {
    renderPanel(blockOf('mj-raw', {}, ''))
    expect(screen.getByRole('button', { name: 'About HTML' })).toBeInTheDocument()
    expect(screen.queryByText(/tracking pixels/)).toBeNull()
  })

  it('previews the saved HTML', () => {
    renderPanel(blockOf('mj-raw', {}, '<p>Tracking</p>'))
    expect(screen.getByText(/Tracking/)).toBeInTheDocument()
  })

  it('writes the CSS class', () => {
    const { onUpdate } = renderPanel(blockOf('mj-raw', {}, ''))
    openSection(/Advanced/)
    blur('CSS class', 'tracking')
    expect(onUpdate).toHaveBeenLastCalledWith({ cssClass: 'tracking' }, undefined)
  })
})

describe('Liquid settings panel', () => {
  it('writes the MJML and Liquid content from the code drawer', async () => {
    const { onUpdate } = renderPanel(blockOf('mj-liquid', {}, ''))
    fireEvent.click(screen.getByRole('button', { name: 'Set Liquid content' }))
    fireEvent.change(await screen.findByLabelText('Code editor'), { target: { value: '{% for item in cart %}{% endfor %}' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ content: '{% for item in cart %}{% endfor %}' })
  })

  it('keeps its help in the section tooltip, not inline', () => {
    renderPanel(blockOf('mj-liquid', {}, ''))
    expect(screen.getByRole('button', { name: 'About MJML and Liquid' })).toBeInTheDocument()
    expect(screen.queryByText(/Liquid engine/)).toBeNull()
  })
})
