import { describe, it, expect, vi, beforeAll, afterAll } from 'vitest'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Editor } from '@tiptap/react'
import { TiptapRichEditor } from '../TiptapRichEditor'
import { TiptapInlineEditor } from '../TiptapInlineEditor'

// jsdom lays nothing out, and ranges have no rectangles there. ProseMirror reads
// them to scroll the selection into view after a command that focuses the editor.
const rangeLayout = Range.prototype as Partial<Pick<Range, 'getClientRects' | 'getBoundingClientRect'>>
const originalRangeLayout = {
  getClientRects: Object.getOwnPropertyDescriptor(Range.prototype, 'getClientRects'),
  getBoundingClientRect: Object.getOwnPropertyDescriptor(Range.prototype, 'getBoundingClientRect')
}
beforeAll(() => {
  rangeLayout.getClientRects = () => [] as unknown as DOMRectList
  rangeLayout.getBoundingClientRect = () => new DOMRect(0, 0, 0, 0)
})
afterAll(() => {
  for (const [name, descriptor] of Object.entries(originalRangeLayout)) {
    if (descriptor) Object.defineProperty(Range.prototype, name, descriptor)
    else delete (rangeLayout as Record<string, unknown>)[name]
  }
})

// The toolbar on the Text block's real editor: what a user sees and what the block
// saves through onChange.

const renderEditor = async (content: string, headingLevelBeforeBlock?: number | null) => {
  const onChange = vi.fn()
  const { container } = render(
    <TiptapRichEditor content={content} onChange={onChange} headingLevelBeforeBlock={headingLevelBeforeBlock} />
  )
  const root = await waitFor(() => {
    const element = container.querySelector('.ProseMirror')
    expect(element).not.toBeNull()
    return element as HTMLElement & { editor: Editor }
  })
  const editor = root.editor
  const lastSaved = () => onChange.mock.calls.at(-1)?.[0] as string | undefined
  return { editor, onChange, lastSaved }
}

const rangeOf = (editor: Editor, text: string) => {
  let range: { from: number; to: number } | undefined
  editor.state.doc.descendants((node, pos) => {
    if (range || !node.isText || !node.text?.includes(text)) return
    const offset = node.text.indexOf(text)
    range = { from: pos + offset, to: pos + offset + text.length }
  })
  if (!range) throw new Error(`"${text}" is not in the document`)
  return range
}

const moveTo = (editor: Editor, position: number | { from: number; to: number }) =>
  act(() => {
    editor.commands.setTextSelection(position)
  })

// The popover's own buttons, apart from the toolbar's link button of the same name
const popover = () => {
  const root = document.querySelector<HTMLElement>('.tiptap-link-popover')
  expect(root).not.toBeNull()
  return within(root as HTMLElement)
}

const openLinkPopover = async () => {
  await userEvent.click(screen.getByRole('button', { name: 'Insert Link' }))
  await screen.findByText('Link Type')
}

describe('formatting buttons', () => {
  it('show the formatting where the cursor is, as it moves', async () => {
    const { editor } = await renderEditor('<p><strong>bold</strong> plain <em>slanted</em></p>')
    const bold = screen.getByRole('button', { name: 'Bold' })
    const italic = screen.getByRole('button', { name: 'Italic' })

    moveTo(editor, rangeOf(editor, 'bold').from + 1)
    expect(bold).toHaveAttribute('aria-pressed', 'true')
    expect(italic).toHaveAttribute('aria-pressed', 'false')

    moveTo(editor, rangeOf(editor, 'plain').from + 1)
    expect(bold).toHaveAttribute('aria-pressed', 'false')

    moveTo(editor, rangeOf(editor, 'slanted').from + 1)
    expect(italic).toHaveAttribute('aria-pressed', 'true')
  })

  it('show the colour where the cursor is, as it moves', async () => {
    const { editor } = await renderEditor('<p><span style="color: #ff0000">red</span> plain</p>')
    const textColor = screen.getByRole('button', { name: 'Text Color' })

    moveTo(editor, rangeOf(editor, 'red').from + 1)
    expect(textColor).toHaveAttribute('aria-pressed', 'true')
    moveTo(editor, rangeOf(editor, 'plain').from + 1)
    expect(textColor).toHaveAttribute('aria-pressed', 'false')
  })

  it('bold the selected text and save it', async () => {
    const { editor, lastSaved } = await renderEditor('<p>make this bold</p>')
    moveTo(editor, rangeOf(editor, 'this'))
    await userEvent.click(screen.getByRole('button', { name: 'Bold' }))
    expect(lastSaved()).toBe('<p>make <strong>this</strong> bold</p>')
    expect(screen.getByRole('button', { name: 'Bold' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('with nothing selected, bold the text typed next and leave the paragraph alone', async () => {
    const { editor, lastSaved } = await renderEditor('<p>hello world</p>')
    moveTo(editor, rangeOf(editor, 'world').from)
    await userEvent.click(screen.getByRole('button', { name: 'Bold' }))
    act(() => {
      editor.commands.insertContent('new ')
    })
    expect(lastSaved()).toBe('<p>hello <strong>new </strong>world</p>')
  })

  it('show and remove bold written as CSS', async () => {
    const { editor, lastSaved } = await renderEditor('<p><span style="font-weight: bold; color: red">bold</span> x</p>')
    moveTo(editor, rangeOf(editor, 'bold'))
    const bold = screen.getByRole('button', { name: 'Bold' })
    expect(bold).toHaveAttribute('aria-pressed', 'true')
    await userEvent.click(bold)
    expect(lastSaved()).toBe('<p><span style="color: red">bold</span> x</p>')
    expect(bold).toHaveAttribute('aria-pressed', 'false')
  })

  it('remove bold from a <strong> that also carries it as CSS', async () => {
    const { editor, lastSaved } = await renderEditor('<p><strong style="font-weight: 700; color: red">bold</strong></p>')
    moveTo(editor, rangeOf(editor, 'bold'))
    await userEvent.click(screen.getByRole('button', { name: 'Bold' }))
    expect(lastSaved()).toBe('<p><span style="color: red">bold</span></p>')
  })

  it('remove italic written as CSS', async () => {
    const { editor, lastSaved } = await renderEditor('<p><span style="font-style: italic">slanted</span></p>')
    moveTo(editor, rangeOf(editor, 'slanted'))
    await userEvent.click(screen.getByRole('button', { name: 'Italic' }))
    expect(lastSaved()).toBe('<p>slanted</p>')
  })

  it('show no picker colour for a colour written in Liquid, without failing', async () => {
    const { editor } = await renderEditor('<p><span style="color: {{ brand_color }}">brand</span></p>')
    moveTo(editor, rangeOf(editor, 'brand').from + 1)
    expect(screen.getByRole('button', { name: 'Text Color' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('stop bold at the cursor in a paragraph styled bold', async () => {
    const { editor, lastSaved } = await renderEditor('<p style="font-weight: bold">Hello</p>')
    moveTo(editor, rangeOf(editor, 'Hello').to)
    await userEvent.click(screen.getByRole('button', { name: 'Bold' }))
    act(() => {
      editor.commands.insertContent(' world')
    })
    expect(lastSaved()).toBe('<p style="font-weight: bold">Hello<span style="font-weight: normal"> world</span></p>')
  })

  it('stop bold at the cursor inside a link styled bold', async () => {
    const { editor, lastSaved } = await renderEditor('<p><a href="https://x.example" style="font-weight: bold">Click</a></p>')
    moveTo(editor, rangeOf(editor, 'Click').from + 2)
    await userEvent.click(screen.getByRole('button', { name: 'Bold' }))
    act(() => {
      editor.commands.insertContent('X')
    })
    expect(lastSaved()).toBe(
      '<p><a href="https://x.example" style="font-weight: bold">Cl<span style="font-weight: normal">X</span>ick</a></p>'
    )
  })

  it('remove the bold of a styled paragraph from the selected part only', async () => {
    const { editor, lastSaved } = await renderEditor('<p style="font-weight: bold">first second</p>')
    moveTo(editor, rangeOf(editor, 'first'))
    await userEvent.click(screen.getByRole('button', { name: 'Bold' }))
    expect(lastSaved()).toBe('<p>first<span style="font-weight: bold"> second</span></p>')
  })

  it('bold a selection that is only partly bold', async () => {
    const { editor, lastSaved } = await renderEditor('<p>plain <span style="font-weight: bold">bold</span></p>')
    moveTo(editor, { from: rangeOf(editor, 'plain').from, to: rangeOf(editor, 'bold').to })
    const bold = screen.getByRole('button', { name: 'Bold' })
    expect(bold).toHaveAttribute('aria-pressed', 'false')
    await userEvent.click(bold)
    expect(lastSaved()).toBe('<p><strong>plain </strong><span style="font-weight: bold"><strong>bold</strong></span></p>')
  })

  it('remove bold from a selection bold by tag and by CSS', async () => {
    const { editor, lastSaved } = await renderEditor(
      '<p><strong><em>first</em> <span style="font-weight: bold">second</span></strong></p>'
    )
    moveTo(editor, { from: rangeOf(editor, 'first').from, to: rangeOf(editor, 'second').to })
    await userEvent.click(screen.getByRole('button', { name: 'Bold' }))
    expect(lastSaved()).toBe('<p><em>first</em> second</p>')
  })

  it('show and remove bold and underline written on a link', async () => {
    const { editor, lastSaved } = await renderEditor(
      '<p><a href="https://x.example" style="color: #06c; text-decoration: underline; font-weight: bold">Click</a></p>'
    )
    moveTo(editor, rangeOf(editor, 'Click'))
    expect(screen.getByRole('button', { name: 'Bold' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: 'Underline' })).toHaveAttribute('aria-pressed', 'true')
    await userEvent.click(screen.getByRole('button', { name: 'Bold' }))
    expect(lastSaved()).toBe('<p><a href="https://x.example" style="color: #06c; text-decoration: underline">Click</a></p>')
  })

  it('enable undo once there is something to undo', async () => {
    const { editor } = await renderEditor('<p>text</p>')
    const undo = screen.getByRole('button', { name: 'Undo' })
    expect(undo).toBeDisabled()
    act(() => {
      editor.commands.insertContent('more')
    })
    expect(undo).toBeEnabled()
  })
})

describe('link popover', () => {
  it('inserts a link with its target as text when nothing is selected', async () => {
    const { editor, lastSaved } = await renderEditor('<p>Visit now</p>')
    moveTo(editor, rangeOf(editor, 'now').from)
    await openLinkPopover()

    await userEvent.type(screen.getByPlaceholderText(/https:\/\/example\.com/), 'https://x.example')
    await userEvent.click(popover().getByRole('button', { name: 'Insert Link' }))

    expect(lastSaved()).toBe('<p>Visit <a href="https://x.example">https://x.example</a>now</p>')
  })

  it('inserts the web version variable without asking for a value', async () => {
    const { editor, lastSaved } = await renderEditor('<p>Read online</p>')
    moveTo(editor, rangeOf(editor, 'online').from)
    await openLinkPopover()

    await userEvent.click(screen.getByRole('combobox'))
    await userEvent.click(await screen.findByText('Web version of the email'))
    expect(screen.getByDisplayValue('{{ view_in_browser_url }}')).toBeDisabled()
    expect(screen.getByText(/broadcasts and automations/)).toBeTruthy()

    await userEvent.click(popover().getByRole('button', { name: 'Insert Link' }))
    expect(lastSaved()).toBe(
      '<p>Read <a href="{{ view_in_browser_url }}">View in browser</a>online</p>'
    )
  })

  it('reopens an existing web version link as one', async () => {
    const { editor } = await renderEditor('<p><a href="{{view_in_browser_url}}">web</a></p>')
    moveTo(editor, rangeOf(editor, 'web').from + 1)
    await openLinkPopover()

    await waitFor(() => expect(screen.getByDisplayValue('{{ view_in_browser_url }}')).toBeDisabled())
    expect(popover().getByRole('button', { name: 'Update Link' })).toBeTruthy()
  })

  it('updates the target of the link around the cursor', async () => {
    const { editor, lastSaved } = await renderEditor(
      '<p><a href="https://old.example" style="color: #ff0000">the link</a> after</p>'
    )
    moveTo(editor, rangeOf(editor, 'the link').from + 2)
    await openLinkPopover()

    const input = await screen.findByDisplayValue('https://old.example')
    await userEvent.clear(input)
    await userEvent.type(input, 'https://new.example')
    await userEvent.click(popover().getByRole('button', { name: 'Update Link' }))

    expect(lastSaved()).toBe('<p><a href="https://new.example" style="color: #ff0000">the link</a> after</p>')
  })

  it('edits the link when the cursor is right after it', async () => {
    const { editor, lastSaved } = await renderEditor('<p>see <a href="https://old.example">the link</a> after</p>')
    moveTo(editor, rangeOf(editor, 'the link').to)
    await openLinkPopover()

    const input = await screen.findByDisplayValue('https://old.example')
    await userEvent.clear(input)
    await userEvent.type(input, 'https://new.example')
    await userEvent.click(popover().getByRole('button', { name: 'Update Link' }))

    expect(lastSaved()).toBe('<p>see <a href="https://new.example">the link</a> after</p>')
  })

  it('removes the link around the cursor and keeps its text', async () => {
    const { editor, lastSaved } = await renderEditor('<p><a href="https://x.example">the link</a> after</p>')
    moveTo(editor, rangeOf(editor, 'the link').from + 2)
    await openLinkPopover()

    await userEvent.click(popover().getByRole('button', { name: 'Remove Link' }))
    expect(lastSaved()).toBe('<p>the link after</p>')
  })

  it('removes a link whose target is emptied', async () => {
    const { editor, lastSaved } = await renderEditor('<p><a href="https://x.example">the link</a></p>')
    moveTo(editor, rangeOf(editor, 'the link').from + 2)
    await openLinkPopover()

    await userEvent.clear(await screen.findByDisplayValue('https://x.example'))
    await userEvent.click(popover().getByRole('button', { name: 'Update Link' }))
    expect(lastSaved()).toBe('<p>the link</p>')
  })
})

describe('text style menu', () => {
  const styleButton = () => screen.getByRole('button', { name: /^Text style/ })
  const openMenu = async () => {
    await userEvent.click(styleButton())
    return screen.findByRole('menu')
  }
  const option = (menu: HTMLElement, name: RegExp) => within(menu).getByRole('menuitem', { name })

  it('shows the current style, and follows the cursor', async () => {
    const { editor } = await renderEditor('<p>para</p><h2>title</h2>')
    moveTo(editor, rangeOf(editor, 'para').from + 1)
    expect(styleButton()).toHaveTextContent('Paragraph')
    moveTo(editor, rangeOf(editor, 'title').from + 1)
    expect(styleButton()).toHaveTextContent('Heading 2')
  })

  it('turns a paragraph into a heading and back', async () => {
    const { editor, lastSaved } = await renderEditor('<p>words</p>')
    moveTo(editor, rangeOf(editor, 'words').from + 1)
    await userEvent.click(option(await openMenu(), /^Heading 3/))
    expect(lastSaved()).toBe('<h3>words</h3>')
    expect(styleButton()).toHaveTextContent('Heading 3')
    await userEvent.click(option(await openMenu(), /^Paragraph/))
    expect(lastSaved()).toBe('<p>words</p>')
  })

  it('marks the levels that skip one after the heading above the block, and keeps them usable', async () => {
    const { editor, lastSaved } = await renderEditor('<p>words</p>', 2)
    moveTo(editor, rangeOf(editor, 'words').from + 1)
    const menu = await openMenu()
    expect(option(menu, /^Heading 3/)).not.toHaveTextContent('skip')
    for (const level of [4, 5, 6]) {
      expect(option(menu, new RegExp(`^Heading ${level}`))).toHaveTextContent('skip')
    }
    await userEvent.click(option(menu, /^Heading 5/))
    expect(lastSaved()).toBe('<h5>words</h5>')
  })

  it('counts a heading earlier in the same block as the one above', async () => {
    const { editor } = await renderEditor('<h1>top</h1><p>words</p>', null)
    moveTo(editor, rangeOf(editor, 'words').from + 1)
    const menu = await openMenu()
    expect(option(menu, /^Heading 2/)).not.toHaveTextContent('skip')
    expect(option(menu, /^Heading 3/)).toHaveTextContent('skip')
  })

  it('marks nothing when no heading comes before', async () => {
    const { editor } = await renderEditor('<p>words</p>', null)
    moveTo(editor, rangeOf(editor, 'words').from + 1)
    const menu = await openMenu()
    expect(within(menu).queryByText('skip')).toBeNull()
  })

  it('is not offered in the button editor', async () => {
    const { container } = render(<TiptapInlineEditor content="Buy" onChange={vi.fn()} />)
    await waitFor(() => expect(container.querySelector('.ProseMirror')).not.toBeNull())
    expect(screen.queryByRole('button', { name: /^Text style/ })).toBeNull()
  })
})
