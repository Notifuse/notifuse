import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { openSection } from './settingsPanelTestUtils'
import { MjSocialBlock, socialInheritedAttributes } from './MjSocialBlock'
import type { EmailBlock, MergedBlockAttributes } from '../types'
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

const element = (id: string, attributes: Record<string, unknown> = {}) => ({
  id,
  type: 'mj-social-element',
  attributes: { name: 'facebook', href: 'https://facebook.com/notifuse', ...attributes },
  content: `Follow ${id}`,
  children: []
})

const social = (attributes: Record<string, unknown>, ...children: unknown[]) =>
  ({ id: 'social', type: 'mj-social', attributes, children }) as unknown as EmailBlock

const renderCanvas = (block: EmailBlock, attributeDefaults?: Record<string, Record<string, unknown>>) =>
  render(<>{new MjSocialBlock(block).getEdit(previewProps(attributeDefaults))}</>)

const iconOf = (container: HTMLElement, id: string) =>
  container.querySelector(`[data-block-id="${id}"] img`) as HTMLImageElement

describe('socialInheritedAttributes', () => {
  it('passes the attributes MJML passes, inner-padding as the padding, and nothing unset', () => {
    expect(
      socialInheritedAttributes(
        {
          border: '1px solid #000000',
          fontSize: '18px',
          innerPadding: '8px',
          align: 'left',
          mode: 'vertical',
          color: ''
        },
        { iconSize: '30px' }
      )
    ).toEqual({ border: '1px solid #000000', fontSize: '18px', padding: '8px', iconSize: '30px' })
  })

  it("lets the block's attributes win over the template's default attributes", () => {
    expect(socialInheritedAttributes({ fontSize: '18px' }, { fontSize: '10px' })).toEqual({ fontSize: '18px' })
  })
})

describe('social canvas', () => {
  it("draws the block's icon border and font on every icon", () => {
    const { container } = renderCanvas(
      social({ border: '2px solid #ff0000', fontSize: '18px' }, element('a'), element('b'))
    )
    expect(iconOf(container, 'a').style.border).toBe('2px solid rgb(255, 0, 0)')
    expect(iconOf(container, 'b').style.border).toBe('2px solid rgb(255, 0, 0)')
    expect(screen.getByText('Follow a').style.fontSize).toBe('18px')
  })

  it("lets an icon's own border and font win over the block's", () => {
    const { container } = renderCanvas(
      social(
        { border: '2px solid #ff0000', fontSize: '18px' },
        element('a', { border: '1px dashed #000000', fontSize: '11px' })
      )
    )
    expect(iconOf(container, 'a').style.border).toBe('1px dashed rgb(0, 0, 0)')
    expect(screen.getByText('Follow a').style.fontSize).toBe('11px')
  })

  it("takes what the template's default mj-social attributes set", () => {
    const { container } = renderCanvas(social({}, element('a')), { 'mj-social': { border: '3px solid #00ff00' } })
    expect(iconOf(container, 'a').style.border).toBe('3px solid rgb(0, 255, 0)')
  })

  it('draws no icon border when nothing sets one', () => {
    const { container } = renderCanvas(social({}, element('a')))
    expect(iconOf(container, 'a').style.border).toMatch(/^(0|0px|)$/)
  })

  it("pads each icon with the block's icon spacing", () => {
    const { container } = renderCanvas(social({ innerPadding: '9px' }, element('a')))
    const cell = iconOf(container, 'a').closest('table')?.closest('td') as HTMLTableCellElement
    expect(cell.style.padding).toBe('9px')
  })
})

describe('social settings', () => {
  const renderPanel = (attributes: Record<string, unknown> = {}) => {
    const onUpdate = vi.fn()
    render(
      <>
        {new MjSocialBlock(social(attributes, element('a'))).renderSettingsPanel(onUpdate, {} as MergedBlockAttributes)}
      </>
    )
    return onUpdate
  }

  it('writes mode="vertical" for a column and removes the mode for a row', () => {
    const onUpdate = renderPanel()
    fireEvent.click(screen.getByText('In a column'))
    expect(onUpdate).toHaveBeenLastCalledWith({ mode: 'vertical' }, undefined)
  })

  it('removes the mode when going back to a row', () => {
    const onUpdate = renderPanel({ mode: 'vertical' })
    fireEvent.click(screen.getByText('In a row'))
    expect(onUpdate).toHaveBeenLastCalledWith({ mode: undefined }, undefined)
  })

  it('writes every font field it shows, not only the line height and the alignment', () => {
    const onUpdate = renderPanel({ fontFamily: 'Georgia, serif', lineHeight: '30px' })
    openSection(/^Font$/)
    fireEvent.change(screen.getByPlaceholderText('13'), { target: { value: '18' } })
    expect(onUpdate).toHaveBeenLastCalledWith({ fontSize: '18px' }, expect.objectContaining({ coalesceKey: expect.any(String) }))
    fireEvent.click(screen.getByRole('radio', { name: 'Italic' }))
    expect(onUpdate).toHaveBeenLastCalledWith({ fontStyle: 'italic' }, undefined)
    fireEvent.click(screen.getByTitle('Underline'))
    expect(onUpdate).toHaveBeenLastCalledWith({ textDecoration: 'underline' }, undefined)
  })

  it('offers no text transform nor letter spacing, which mj-social does not have', () => {
    renderPanel()
    openSection(/^Font$/)
    expect(screen.getByText('Line height')).toBeTruthy()
    expect(screen.queryByText('Case')).toBeNull()
    expect(screen.queryByText('Letter spacing')).toBeNull()
  })
})

describe('social settings for every icon', () => {
  const renderPanel = (attributes: Record<string, unknown> = {}) => {
    const onUpdate = vi.fn()
    render(
      <>
        {new MjSocialBlock(social(attributes, element('a'))).renderSettingsPanel(onUpdate, {} as MergedBlockAttributes)}
      </>
    )
    return onUpdate
  }

  it('writes the icon size and height in px', () => {
    const onUpdate = renderPanel()
    openSection(/^Icons$/)
    const [size, height] = screen.getAllByPlaceholderText('20')
    fireEvent.change(size, { target: { value: '30' } })
    expect(onUpdate).toHaveBeenLastCalledWith({ iconSize: '30px' }, expect.anything())
    fireEvent.change(height, { target: { value: '24' } })
    expect(onUpdate).toHaveBeenLastCalledWith({ iconHeight: '24px' }, expect.anything())
  })

  it('writes the paddings as typed once the field is left, and removes an empty one', () => {
    const onUpdate = renderPanel({ iconPadding: '2px' })
    const textPadding = screen.getByPlaceholderText('4px 4px 4px 0')
    fireEvent.focus(textPadding)
    fireEvent.change(textPadding, { target: { value: '4px 8px' } })
    expect(onUpdate).not.toHaveBeenCalled()
    fireEvent.blur(textPadding)
    expect(onUpdate).toHaveBeenLastCalledWith({ textPadding: '4px 8px' }, undefined)
    const iconPadding = screen.getByLabelText('Icon padding')
    fireEvent.focus(iconPadding)
    fireEvent.change(iconPadding, { target: { value: ' ' } })
    fireEvent.blur(iconPadding)
    expect(onUpdate).toHaveBeenLastCalledWith({ iconPadding: undefined }, undefined)
  })

  it("shows the icons the block's size and text color, an icon's own winning", () => {
    const { container } = renderCanvas(
      social({ iconSize: '32px', color: '#ff0000' }, element('a'), element('b', { iconSize: '16px' }))
    )
    expect(iconOf(container, 'a').getAttribute('width')).toBe('32')
    expect(iconOf(container, 'b').getAttribute('width')).toBe('16')
    expect(screen.getByText('Follow a').style.color).toBe('rgb(255, 0, 0)')
  })
})

describe('social review fixes', () => {
  it("lets the template's default mj-social-element attributes win over what mj-social passes, as MJML", () => {
    const { container } = renderCanvas(social({ fontSize: '18px', border: '2px solid #ff0000' }, element('a')), {
      'mj-social-element': { fontSize: '10px' }
    })
    expect(screen.getByText('Follow a').style.fontSize).toBe('10px')
    expect(iconOf(container, 'a').style.border).toBe('2px solid rgb(255, 0, 0)')
  })

  it('lets an icon keep no border against the border of its block', () => {
    const { container } = renderCanvas(social({ border: '2px solid #ff0000' }, element('a', { border: 'none' })))
    expect(iconOf(container, 'a').style.border).toMatch(/none|^$/)
  })

  it("shows the template's default mode and writes a row explicitly against a default column", () => {
    const onUpdate = vi.fn()
    render(
      <>
        {new MjSocialBlock(social({}, element('a'))).renderSettingsPanel(onUpdate, {
          mode: 'vertical'
        } as MergedBlockAttributes)}
      </>
    )
    expect(screen.getByText('In a column').closest('.ant-segmented-item')?.className).toContain('selected')
    fireEvent.click(screen.getByText('In a row'))
    expect(onUpdate).toHaveBeenLastCalledWith({ mode: 'horizontal' }, undefined)
  })

  it("shows the font the icons take from the template's default attributes", () => {
    const tree = {
      id: 'root',
      type: 'mjml',
      children: [
        {
          id: 'head',
          type: 'mj-head',
          children: [
            {
              id: 'attrs',
              type: 'mj-attributes',
              children: [{ id: 'el', type: 'mj-social-element', attributes: { fontFamily: 'Georgia, serif' } }]
            }
          ]
        }
      ]
    } as unknown as EmailBlock
    render(
      <>{new MjSocialBlock(social({}, element('a'))).renderSettingsPanel(vi.fn(), {} as MergedBlockAttributes, tree)}</>
    )
    openSection(/^Font$/)
    expect(screen.getByText('Georgia, serif')).toBeTruthy()
  })
})

describe('social icon size on the canvas', () => {
  it('makes an icon without a height as high as it is wide, with its border around it, as the email', () => {
    const { container } = renderCanvas(social({ iconSize: '32px', border: '2px solid #ff0000' }, element('a')))
    const icon = iconOf(container, 'a')
    expect([icon.style.width, icon.style.height]).toEqual(['32px', '32px'])
    expect(icon.style.boxSizing).toBe('content-box')
  })

  it('keeps a height set apart from the size', () => {
    const { container } = renderCanvas(social({ iconSize: '32px', iconHeight: '24px' }, element('a')))
    expect(iconOf(container, 'a').style.height).toBe('24px')
  })
})

describe('social icon height placeholder', () => {
  it("shows the icon size, which an icon without a height takes", () => {
    render(
      <>
        {new MjSocialBlock(social({ iconSize: '32px' }, element('a'))).renderSettingsPanel(
          vi.fn(),
          {} as MergedBlockAttributes
        )}
      </>
    )
    openSection(/^Icons$/)
    expect(screen.getAllByPlaceholderText('32')).toHaveLength(1)
  })
})
