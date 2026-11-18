import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App } from 'antd'
import type { EmailBlock, MJMLComponentType } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { EmailBlockFactory } from './EmailBlockFactory'
import { inheritedAttributes } from '../utils/inheritedAttributes'
import { blur, emailWith, fresh, renderPanel, setRows } from './settingsPanelTestUtils'
import { EditorActionsContext } from '../ui/fields/editorActions'

// Monaco, replaced by a plain text area
vi.mock('@monaco-editor/react', () => ({
  Editor: ({ value, onChange }: { value?: string; onChange?: (value: string) => void }) => (
    <textarea aria-label="Code editor" value={value} onChange={(event) => onChange?.(event.target.value)} />
  )
}))

const blockOf = (type: MJMLComponentType, attributes: Record<string, unknown> = {}, content?: string) =>
  ({ id: `${type}-1`, type, attributes, ...(content !== undefined ? { content } : {}) }) as unknown as EmailBlock

// A social element inside a new mj-social, as the editor holds it: it inherits what its block passes
const renderElement = (element: EmailBlock, social: Record<string, unknown> = {}) => {
  const block = EmailBlockClass.createBlock('mj-social', 'social-1')
  const parent = { ...block, attributes: { ...block.attributes, ...social }, children: [element] } as EmailBlock
  const tree = emailWith(parent)
  const onUpdate = vi.fn()
  const inherited = inheritedAttributes(tree, element, EmailBlockClass.extractAttributeDefaults(tree))
  render(<App>{EmailBlockFactory.createBlock(element).renderSettingsPanel(onUpdate, inherited as never, tree)}</App>)
  return { onUpdate }
}

const elementOf = (attributes: Record<string, unknown> = {}, content?: string) =>
  ({
    id: 'element-1',
    type: 'mj-social-element',
    attributes,
    ...(content !== undefined ? { content } : {})
  }) as unknown as EmailBlock

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
  const palettes = await screen.findAllByRole('group', { name: 'Palette' })
  fireEvent.click(within(palettes[palettes.length - 1]).getByRole('button', { name: color }))
}

// The options of the antd Select named `label`, its list opened
const openOptions = async (label: string) => {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: label }))
  await waitFor(() => expect(document.querySelector('.ant-select-item-option')).not.toBeNull())
  return [...document.querySelectorAll('.ant-select-item-option')]
}

const pickOption = async (label: string, option: string) => {
  const match = (await openOptions(label)).find((el) => el.textContent === option)
  if (!match) throw new Error(`No option ${option} in ${label}`)
  fireEvent.click(match)
}

const typeNumber = (label: string, value: string) => {
  const input = screen.getByRole('spinbutton', { name: label })
  fireEvent.focus(input)
  fireEvent.change(input, { target: { value } })
}

const sideOf = (box: string, side = 'Top') => within(screen.getByRole('group', { name: box })).getByLabelText(side)

const coalesced = { coalesceKey: expect.any(String) }
const lastWrite = (onUpdate: ReturnType<typeof vi.fn>) => onUpdate.mock.calls.at(-1)?.[0]

describe('Social and head settings panels: nothing marked set on a new block', () => {
  // Every field compares the block's value with the inherited one; createBlock copies the MJML
  // defaults (and the type's mj-attributes), which are the inherited values, so no row differs.
  it.each([
    'mj-social',
    'mj-title',
    'mj-preview',
    'mj-font',
    'mj-style',
    'mj-breakpoint',
    'mj-attributes',
    'mj-head'
  ] as MJMLComponentType[])('shows no « set » mark on a %s straight out of createBlock, every section open', (type) => {
    renderPanel(fresh(type))
    openAll()
    expect(setRows()).toHaveLength(0)
  })

  it('shows none on a social element created inside its mj-social', () => {
    // createBlock leaves out what the mj-social passes, which the element then inherits
    renderElement(EmailBlockClass.createBlock('mj-social-element', 'element-1'))
    openAll()
    expect(setRows()).toHaveLength(0)
  })

  it("marks only what a new mj-social's starter icons write by hand: network, link and background", () => {
    // The three starter icons of createBlock('mj-social') carry a name, a link and a brand colour
    // written by hand, which differ from the inherited values: those rows are set, as expected.
    const starter = EmailBlockClass.createBlock('mj-social').children![0]
    renderElement(starter)
    openAll()
    const marked = setRows().map((row) => row.querySelector('span > span.min-w-0')?.textContent)
    expect(marked).toEqual(['Network', 'Link', 'Background'])
  })
})

describe('Social block panel: every field writes its key', () => {
  const social = (attributes: Record<string, unknown> = {}) => blockOf('mj-social', attributes)

  it("shows the template's mj-social-element defaults as the icons' inherited values", () => {
    const block = social()
    const tree = emailWith(block)
    const attrs = EmailBlockClass.findBlockById(tree, 'attrs') as EmailBlock
    ;(attrs.children as EmailBlock[]).push({
      id: 'def-element',
      type: 'mj-social-element',
      attributes: { iconSize: '30px' }
    } as unknown as EmailBlock)
    const inherited = inheritedAttributes(tree, block, EmailBlockClass.extractAttributeDefaults(tree))
    const panel = EmailBlockFactory.createBlock(block).renderSettingsPanel(vi.fn(), inherited as never, tree)
    render(<App>{panel}</App>)
    const header = document.querySelector('[aria-controls="settings-section-mj-social-icons"]') as HTMLElement
    if (header.getAttribute('aria-expanded') === 'false') fireEvent.click(header)
    expect(screen.getByLabelText('Icon size')).toHaveAttribute('placeholder', '30')
  })

  it('Layout writes mode', () => {
    const { onUpdate } = renderPanel(social())
    clickSegment('Layout', 'In a column')
    expect(onUpdate).toHaveBeenLastCalledWith({ mode: 'vertical' }, undefined)
  })

  it('Alignment writes align', () => {
    const { onUpdate } = renderPanel(social())
    clickSegment('Alignment', 'Left')
    expect(onUpdate).toHaveBeenLastCalledWith({ align: 'left' }, undefined)
  })

  it('Font writes fontFamily', async () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    await pickOption('Font', 'Georgia, serif')
    expect(lastWrite(onUpdate)).toEqual({ fontFamily: 'Georgia, serif' })
  })

  it('Font size writes fontSize', () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    typeNumber('Font size', '18')
    expect(onUpdate).toHaveBeenLastCalledWith({ fontSize: '18px' }, coalesced)
  })

  it('Font weight writes fontWeight', async () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    await pickOption('Font weight', 'Bold')
    expect(lastWrite(onUpdate)).toEqual({ fontWeight: 'bold' })
  })

  it('Style writes fontStyle', () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    clickSegment('Style', 'Oblique')
    expect(lastWrite(onUpdate)).toEqual({ fontStyle: 'oblique' })
  })

  it('Decoration writes textDecoration', () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    clickSegment('Decoration', 'Overline')
    expect(lastWrite(onUpdate)).toEqual({ textDecoration: 'overline' })
  })

  it('Line height writes lineHeight', () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    typeNumber('Line height', '2')
    expect(Object.keys(lastWrite(onUpdate))).toEqual(['lineHeight'])
  })

  it('Icon size writes iconSize', () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    typeNumber('Icon size', '30')
    expect(onUpdate).toHaveBeenLastCalledWith({ iconSize: '30px' }, coalesced)
  })

  it('Icon height writes iconHeight', () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    typeNumber('Icon height', '24')
    expect(onUpdate).toHaveBeenLastCalledWith({ iconHeight: '24px' }, coalesced)
  })

  it('Corners writes borderRadius', () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    typeNumber('Corners', '8')
    expect(onUpdate).toHaveBeenLastCalledWith({ borderRadius: '8px' }, coalesced)
  })

  it('Text color writes color', async () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    await pickColor('Text color')
    expect(lastWrite(onUpdate)).toEqual({ color: '#e02424' })
  })

  it('Icon padding writes iconPadding, once on blur', () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    blur('Icon padding', '2px 4px')
    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect(onUpdate).toHaveBeenLastCalledWith({ iconPadding: '2px 4px' }, undefined)
  })

  it('Text padding writes textPadding, once on blur', () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    blur('Text padding', '4px 8px')
    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect(onUpdate).toHaveBeenLastCalledWith({ textPadding: '4px 8px' }, undefined)
  })

  it('Container background writes containerBackgroundColor', async () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    await pickColor('Container background')
    expect(lastWrite(onUpdate)).toEqual({ containerBackgroundColor: '#e02424' })
  })

  it('Icon border writes border, one line', () => {
    const { onUpdate } = renderPanel(social({ border: '1px solid #000000' }))
    openAll()
    typeNumber('Border width', '2')
    expect(lastWrite(onUpdate)).toEqual({ border: '2px solid #000000' })
  })

  it('Around the block writes paddingTop', () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    const top = sideOf('Around the block')
    fireEvent.focus(top)
    fireEvent.change(top, { target: { value: '12' } })
    expect(onUpdate).toHaveBeenLastCalledWith({ paddingTop: '12px' }, coalesced)
  })

  it('Icon spacing writes innerPadding, a shorthand string', () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    const top = sideOf('Icon spacing')
    fireEvent.focus(top)
    fireEvent.change(top, { target: { value: '6' } })
    const [updates] = onUpdate.mock.calls.at(-1)!
    expect(Object.keys(updates)).toEqual(['innerPadding'])
    expect(updates.innerPadding).toMatch(/^6px/)
  })

  it('CSS class writes cssClass', () => {
    const { onUpdate } = renderPanel(social())
    openAll()
    blur('CSS class', 'social')
    expect(onUpdate).toHaveBeenLastCalledWith({ cssClass: 'social' }, undefined)
  })
})

describe('Social element panel: every field writes its key', () => {
  it('Network writes name, -noshare for a network that shares', async () => {
    const { onUpdate } = renderElement(elementOf())
    await pickOption('Social network', 'Instagram')
    expect(lastWrite(onUpdate)).toEqual({ name: 'instagram' })
    await pickOption('Social network', 'Facebook')
    expect(lastWrite(onUpdate)).toEqual({ name: 'facebook-noshare' })
  })

  it('Network « Custom » removes name', async () => {
    const { onUpdate } = renderElement(elementOf({ name: 'instagram' }))
    await pickOption('Social network', 'Custom')
    expect(lastWrite(onUpdate)).toEqual({ name: undefined })
  })

  it('Share link writes name without -noshare', () => {
    const { onUpdate } = renderElement(elementOf({ name: 'facebook-noshare' }))
    fireEvent.click(screen.getByRole('switch', { name: 'Share link' }))
    expect(lastWrite(onUpdate)).toEqual({ name: 'facebook' })
  })

  it('Link writes href', () => {
    const { onUpdate } = renderElement(elementOf())
    blur('Link URL', 'https://instagram.com/acme')
    expect(onUpdate).toHaveBeenLastCalledWith({ href: 'https://instagram.com/acme' }, undefined)
  })

  it('Icon writes src', () => {
    const { onUpdate } = renderElement(elementOf())
    blur('Custom icon URL', 'https://cdn.test/icon.png')
    expect(onUpdate).toHaveBeenLastCalledWith({ src: 'https://cdn.test/icon.png' }, undefined)
  })

  it('Alt text writes alt', () => {
    const { onUpdate } = renderElement(elementOf())
    blur('Alt text', 'Our Instagram')
    expect(onUpdate).toHaveBeenLastCalledWith({ alt: 'Our Instagram' }, undefined)
  })

  it('Label writes content (H16)', () => {
    const { onUpdate } = renderElement(elementOf({ name: 'instagram' }))
    blur('Label', 'Follow us')
    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect(onUpdate).toHaveBeenLastCalledWith({ content: 'Follow us' })
  })

  it('Icon size writes iconSize', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    typeNumber('Icon size', '30')
    expect(onUpdate).toHaveBeenLastCalledWith({ iconSize: '30px' }, coalesced)
  })

  it('Icon height writes iconHeight', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    typeNumber('Icon height', '24')
    expect(onUpdate).toHaveBeenLastCalledWith({ iconHeight: '24px' }, coalesced)
  })

  it('Background writes backgroundColor', async () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    await pickColor('Icon background')
    expect(lastWrite(onUpdate)).toEqual({ backgroundColor: '#e02424' })
  })

  it('Corners writes borderRadius', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    typeNumber('Corners', '6')
    expect(onUpdate).toHaveBeenLastCalledWith({ borderRadius: '6px' }, coalesced)
  })

  it('Border writes border, one line, and none for 0 against its block (noneValue)', () => {
    const { onUpdate } = renderElement(elementOf({ border: '1px solid #000000' }))
    openAll()
    typeNumber('Border width', '2')
    expect(lastWrite(onUpdate)).toEqual({ border: '2px solid #000000' })
    typeNumber('Border width', '0')
    expect(lastWrite(onUpdate)).toEqual({ border: 'none' })
  })

  it('Vertical align writes verticalAlign', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    clickSegment('Vertical align', 'Bottom')
    expect(onUpdate).toHaveBeenLastCalledWith({ verticalAlign: 'bottom' }, undefined)
  })

  it('Alignment writes align', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    clickSegment('Alignment', 'Right')
    expect(onUpdate).toHaveBeenLastCalledWith({ align: 'right' }, undefined)
  })

  it('Text color writes color', async () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    await pickColor('Text color')
    expect(lastWrite(onUpdate)).toEqual({ color: '#e02424' })
  })

  it('Font size writes fontSize, 8 to 48 px', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    typeNumber('Font size', '16')
    expect(onUpdate).toHaveBeenLastCalledWith({ fontSize: '16px' }, coalesced)
    const size = screen.getByRole('spinbutton', { name: 'Font size' })
    expect(size).toHaveAttribute('aria-valuemin', '8')
    expect(size).toHaveAttribute('aria-valuemax', '48')
  })

  it('Font writes fontFamily from the font list (H8)', async () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    await pickOption('Font', 'Arial, sans-serif')
    expect(lastWrite(onUpdate)).toEqual({ fontFamily: 'Arial, sans-serif' })
  })

  it('Weight writes fontWeight, and a saved « lighter » stays shown (H8)', async () => {
    const { onUpdate } = renderElement(elementOf({ fontWeight: 'lighter' }))
    openAll()
    const options = (await openOptions('Font weight')).map((el) => el.textContent)
    expect(options).toContain('lighter')
    await pickOption('Font weight', '700 - Bold')
    expect(lastWrite(onUpdate)).toEqual({ fontWeight: '700' })
  })

  it('a new element is not offered « lighter »', async () => {
    renderElement(elementOf())
    openAll()
    const options = (await openOptions('Font weight')).map((el) => el.textContent)
    expect(options).not.toContain('lighter')
  })

  it('Style writes fontStyle', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    clickSegment('Style', 'Italic')
    expect(lastWrite(onUpdate)).toEqual({ fontStyle: 'italic' })
  })

  it('Decoration writes textDecoration', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    clickSegment('Decoration', 'Line through')
    expect(lastWrite(onUpdate)).toEqual({ textDecoration: 'line-through' })
  })

  it('Line height writes lineHeight, with the kit (H8)', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    typeNumber('Line height', '2')
    expect(Object.keys(lastWrite(onUpdate))).toEqual(['lineHeight'])
  })

  it('Open in writes target', async () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    await pickOption('Open in', 'Same window')
    expect(lastWrite(onUpdate)).toEqual({ target: '_self' })
  })

  it('Rel writes rel, once on blur', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    blur('Rel', 'noopener')
    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect(onUpdate).toHaveBeenLastCalledWith({ rel: 'noopener' }, undefined)
  })

  it('Tooltip writes title, once on blur', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    blur('Tooltip', 'Our page')
    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect(onUpdate).toHaveBeenLastCalledWith({ title: 'Our page' }, undefined)
  })

  it('Icon padding writes iconPadding', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    blur('Icon padding', '2px')
    expect(onUpdate).toHaveBeenLastCalledWith({ iconPadding: '2px' }, undefined)
  })

  it('Text padding writes textPadding', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    blur('Text padding', '4px')
    expect(onUpdate).toHaveBeenLastCalledWith({ textPadding: '4px' }, undefined)
  })

  it('Around the element writes paddingLeft', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    const left = sideOf('Around the element', 'Left')
    fireEvent.focus(left)
    fireEvent.change(left, { target: { value: '9' } })
    expect(onUpdate).toHaveBeenLastCalledWith({ paddingLeft: '9px' }, coalesced)
  })

  it('CSS class writes cssClass', () => {
    const { onUpdate } = renderElement(elementOf())
    openAll()
    blur('CSS class', 'insta')
    expect(onUpdate).toHaveBeenLastCalledWith({ cssClass: 'insta' }, undefined)
  })
})

describe('Social element panel: inherited values and decisions', () => {
  it('takes its placeholders from its mj-social, the height showing the icon size', () => {
    renderElement(elementOf(), { iconSize: '32px', fontSize: '18px' })
    openAll()
    expect(screen.getByRole('spinbutton', { name: 'Icon size' })).toHaveAttribute('placeholder', '32')
    expect(screen.getByRole('spinbutton', { name: 'Icon height' })).toHaveAttribute('placeholder', '32')
    expect(screen.getByRole('spinbutton', { name: 'Font size' })).toHaveAttribute('placeholder', '18')
  })

  it('keeps the custom icon URL enabled on a network, and an emptied one writes undefined (H10)', () => {
    const { onUpdate } = renderElement(elementOf({ name: 'instagram', src: 'https://cdn.test/icon.png' }))
    const src = screen.getByLabelText('Custom icon URL')
    expect(src).not.toBeDisabled()
    blur('Custom icon URL', '')
    expect(onUpdate).toHaveBeenLastCalledWith({ src: undefined }, undefined)
  })

  it('no longer offers Twitter for a new choice (H10)', async () => {
    renderElement(elementOf())
    const options = (await openOptions('Social network')).map((el) => el.textContent)
    expect(options).toContain('X')
    expect(options).not.toContain('Twitter')
  })

  it('still shows Twitter for an element saved as twitter (H10)', async () => {
    renderElement(elementOf({ name: 'twitter' }))
    const select = screen.getByRole('combobox', { name: 'Social network' }).closest('.ant-select') as HTMLElement
    expect(within(select).getByText('Twitter')).toBeInTheDocument()
    const options = (await openOptions('Social network')).map((el) => el.textContent)
    expect(options).toContain('Twitter')
  })

  it('shows a saved label and resets it by removing the content (H16)', () => {
    const { onUpdate } = renderElement(elementOf({ name: 'instagram' }, 'Follow us'))
    expect(screen.getByLabelText('Label')).toHaveValue('Follow us')
    fireEvent.click(within(rowOf('Label')).getByRole('button', { name: /Reset Label/ }))
    expect(onUpdate).toHaveBeenLastCalledWith({ content: undefined })
  })
})

describe('Head block panels: every field writes its key', () => {
  it.each(['mj-title', 'mj-preview'] as MJMLComponentType[])(
    'has no panel for %s: the server writes it from the template settings on save',
    (type) => {
      const block = blockOf(type, {}, 'Saved text')
      expect(EmailBlockFactory.createBlock(block).renderSettingsPanel(vi.fn(), {} as never, emailWith(block))).toBeNull()
    }
  )

  it('Font: picking a popular Google Font writes its name and stylesheet in one write', async () => {
    const { onUpdate } = renderPanel(blockOf('mj-font', { name: 'Inter', href: 'https://fonts.googleapis.com/css2?family=Inter' }))
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Popular Google Font' }))
    fireEvent.click(await screen.findByTitle('Lato'))
    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect(onUpdate).toHaveBeenLastCalledWith({
      name: 'Lato',
      href: 'https://fonts.googleapis.com/css2?family=Lato:wght@300;400;700&display=swap'
    })
  })

  it('Font: the picker shows the popular font the block holds, and none for another', () => {
    renderPanel(
      blockOf('mj-font', { name: 'Roboto', href: 'https://fonts.googleapis.com/css2?family=Roboto:wght@300;400;500;700&display=swap' })
    )
    expect(screen.getByTitle('Roboto')).toBeInTheDocument()
  })

  it('Font: the name is written once, on blur; an emptied name keeps the saved one', () => {
    const { onUpdate } = renderPanel(blockOf('mj-font', { name: 'Inter', href: 'https://a.test/inter.css' }))
    blur('Font name', 'Raleway')
    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect(onUpdate).toHaveBeenLastCalledWith({ name: 'Raleway' })
    onUpdate.mockClear()
    blur('Font name', '  ')
    expect(onUpdate).not.toHaveBeenCalled()
  })

  it.each([
    '{{ font_url }}',
    '//fonts.googleapis.com/css?family=Lato',
    'https://example.com/{% if dark %}dark{% endif %}.css',
    'https://{{ cdn_host }}/fonts.css'
  ])('Font: writes the stylesheet %s', (href) => {
    const { onUpdate } = renderPanel(blockOf('mj-font', { name: 'Inter', href: 'https://a.test/inter.css' }))
    blur('Stylesheet URL', href)
    expect(onUpdate).toHaveBeenLastCalledWith({ href })
  })

  it('Font: writes a stylesheet pasted after a no-break space without it', () => {
    const { onUpdate } = renderPanel(blockOf('mj-font', { name: 'Inter', href: 'https://a.test/inter.css' }))
    blur('Stylesheet URL', '\u00a0https://fonts.googleapis.com/css?family=Lato')
    expect(onUpdate).toHaveBeenLastCalledWith({ href: 'https://fonts.googleapis.com/css?family=Lato' })
  })

  it("Font: writes a stylesheet on the console's own origin", () => {
    const href = `${window.location.origin}/fonts/brand.css`
    const { onUpdate } = renderPanel(blockOf('mj-font', { name: 'Inter', href: 'https://a.test/inter.css' }))
    blur('Stylesheet URL', href)
    expect(onUpdate).toHaveBeenLastCalledWith({ href })
  })

  it('Font: warns while the name or the stylesheet is missing', () => {
    renderPanel(blockOf('mj-font', { name: '', href: 'https://a.test/inter.css' }))
    expect(screen.getByRole('alert')).toHaveTextContent('The font is imported once it has a name and a stylesheet.')
  })

  it.each([
    '/fonts/inter.css',
    'fonts/inter.css',
    'https//example.com/{% if dark %}dark{% endif %}.css',
    'javascript:alert(1)',
    'data:text/css,body{}',
    'mailto:fonts@example.com'
  ])(
    'Font: refuses the stylesheet %s, which stays in the field in red',
    (href) => {
      const { onUpdate } = renderPanel(blockOf('mj-font', { name: 'Inter', href: 'https://a.test/inter.css' }))
      blur('Stylesheet URL', href)
      expect(onUpdate).not.toHaveBeenCalled()
      expect(screen.getByLabelText('Stylesheet URL')).toHaveAttribute('aria-invalid', 'true')
    }
  )

  it.each([
    ['https://fonts.googleapis.com/css2?family=Inter', 'https://fonts.googleapis.com/css2?family=Inter'],
    ['//fonts.googleapis.com/css?family=Lato', 'https://fonts.googleapis.com/css?family=Lato']
  ])('Font: links the stylesheet %s', (href, link) => {
    renderPanel(blockOf('mj-font', { name: 'Inter', href }))
    expect(screen.getByRole('link', { name: 'Open the link' })).toHaveAttribute('href', link)
  })

  it.each(['javascript:window.__probe=1', 'https://{{ cdn_host }}/fonts.css', '{{ font_url }}'])(
    'Font: does not link the stylesheet %s',
    (href) => {
      renderPanel(blockOf('mj-font', { name: 'Inter', href }))
      expect(screen.queryByRole('link')).toBeNull()
    }
  )

  it('Font: the help is in tooltips, not an alert', () => {
    renderPanel(blockOf('mj-font', { name: 'Inter', href: 'https://a.test/inter.css' }))
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Import Font' })).toBeNull()
    expect(screen.getByRole('button', { name: 'About Name' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'About Stylesheet' })).toBeInTheDocument()
  })

  it('Inline Styles writes inline', () => {
    const { onUpdate } = renderPanel(blockOf('mj-style', {}, '.a { color: red }'))
    fireEvent.click(screen.getByRole('switch', { name: 'Inline styles' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ inline: 'inline' })
  })

  it('Inline Styles, when on, has a reset that turns it off', () => {
    const { onUpdate } = renderPanel(blockOf('mj-style', { inline: 'inline' }, '.a { color: red }'))
    fireEvent.click(screen.getByRole('button', { name: 'Reset Inline styles to the inherited value' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ inline: undefined })
  })

  it('CSS Content writes content from the drawer', async () => {
    const { onUpdate } = renderPanel(blockOf('mj-style', {}, ''))
    fireEvent.click(screen.getByRole('button', { name: 'Add CSS' }))
    fireEvent.change(await screen.findByLabelText('Code editor'), { target: { value: '.b { margin: 0 }' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ content: '.b { margin: 0 }' })
  })

  it('Breakpoint width writes width, its explanations in a tooltip (H13)', () => {
    const { onUpdate } = renderPanel(blockOf('mj-breakpoint', {}))
    expect(screen.queryByText('Preview mode required')).toBeNull()
    expect(screen.getByRole('button', { name: 'About Breakpoint width' })).toBeInTheDocument()
    const width = screen.getByRole('spinbutton', { name: 'Breakpoint width' })
    expect(width).toHaveAttribute('placeholder', '480')
    typeNumber('Breakpoint width', '600')
    expect(onUpdate).toHaveBeenLastCalledWith({ width: '600px' }, coalesced)
  })

  it('offers « See in preview » only when the editor can switch to it', () => {
    const block = blockOf('mj-breakpoint', {})
    const tree = emailWith(block)
    const inherited = inheritedAttributes(tree, block, EmailBlockClass.extractAttributeDefaults(tree))
    const showPreview = vi.fn()
    const panel = EmailBlockFactory.createBlock(block).renderSettingsPanel(vi.fn(), inherited as never, tree)
    const { unmount } = render(
      <App>
        <EditorActionsContext.Provider value={{ showPreview }}>{panel}</EditorActionsContext.Provider>
      </App>
    )
    fireEvent.click(screen.getByRole('button', { name: 'See in preview' }))
    expect(showPreview).toHaveBeenCalledTimes(1)
    unmount()
    renderPanel(blockOf('mj-breakpoint', {}))
    expect(screen.queryByRole('button', { name: 'See in preview' })).toBeNull()
  })

  it('mj-head keeps its message', () => {
    renderPanel(blockOf('mj-head'))
    expect(screen.getByText('No settings available for the head element.')).toBeInTheDocument()
  })
})
