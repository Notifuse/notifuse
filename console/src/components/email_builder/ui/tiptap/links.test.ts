import { describe, it, expect, afterEach } from 'vitest'
import { Editor } from '@tiptap/core'
import type { Editor as ReactEditor } from '@tiptap/react'
import { createRichExtensions } from './shared/extensions'
import { createLinkWithStyleMerging, getEditorHTML } from './shared/utils'

// Links made through the link popover's helper, on the Text block's real editor.

const editors: Editor[] = []
const mounted: HTMLElement[] = []

const load = (content: string) => {
  const element = document.createElement('div')
  document.body.appendChild(element)
  mounted.push(element)
  const editor = new Editor({ element, extensions: createRichExtensions(), content })
  editors.push(editor)
  return editor
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

const link = (editor: Editor, value: string, type = 'url', text?: string) =>
  createLinkWithStyleMerging(editor as unknown as ReactEditor, value, type, text)

afterEach(() => {
  editors.splice(0).forEach((editor) => editor.destroy())
  mounted.splice(0).forEach((element) => element.remove())
})

describe('making a link', () => {
  it.each([
    ['url', 'https://example.com', 'https://example.com'],
    ['url', '{{ cta_url }}', '{{ cta_url }}'],
    ['email', 'a@example.com', 'mailto:a@example.com'],
    ['phone', '+331234', 'tel:+331234'],
    ['anchor', 'top', '#top'],
    ['web_version', 'ignored', '{{ view_in_browser_url }}']
  ])('links the selection to a %s target', (type, value, href) => {
    const editor = load('<p>click here</p>')
    editor.commands.setTextSelection(rangeOf(editor, 'here'))
    link(editor, value, type)
    expect(getEditorHTML(editor)).toBe(`<p>click <a href="${href}">here</a></p>`)
  })

  it('moves the colour of the text it links onto the link, and keeps its other style', () => {
    const editor = load('<p><span style="font-size: 20px; color: #ff0000">big</span> rest</p>')
    editor.commands.setTextSelection(rangeOf(editor, 'big'))
    link(editor, 'https://x.example')
    expect(getEditorHTML(editor)).toBe(
      '<p><a href="https://x.example" style="color: #ff0000"><span style="font-size: 20px">big</span></a> rest</p>'
    )
  })

  it('keeps one link over text of several colours, each on its text', () => {
    const editor = load('<p><span style="color: red">red</span><span style="color: blue">blue</span></p>')
    editor.commands.setTextSelection({ from: rangeOf(editor, 'red').from, to: rangeOf(editor, 'blue').to })
    link(editor, 'https://x.example')
    expect(getEditorHTML(editor)).toBe(
      '<p><a href="https://x.example"><span style="color: red">red</span><span style="color: blue">blue</span></a></p>'
    )
  })

  it('gives an inserted link the formatting of the text around the cursor', () => {
    const editor = load('<p><strong><span style="font-size: 20px">Hello world</span></strong></p>')
    editor.commands.setTextSelection(rangeOf(editor, 'world').from)
    link(editor, 'https://x.example')
    const root = document.createElement('div')
    root.innerHTML = getEditorHTML(editor)
    const inserted = root.querySelector('a')
    expect(inserted?.textContent).toBe('https://x.example')
    expect(inserted?.querySelector('strong')).not.toBeNull()
    expect(inserted?.querySelector('span')?.getAttribute('style')).toBe('font-size: 20px')
  })

  it('inserts the link with its text when nothing is selected', () => {
    const editor = load('<p>Visit now</p>')
    editor.commands.setTextSelection(rangeOf(editor, 'now').from)
    link(editor, 'https://x.example')
    expect(getEditorHTML(editor)).toBe('<p>Visit <a href="https://x.example">https://x.example</a>now</p>')
  })

  it('inserts the web version link with the text it is given', () => {
    const editor = load('<p>Read online</p>')
    editor.commands.setTextSelection(rangeOf(editor, 'online').from)
    link(editor, '', 'web_version', 'View in browser')
    expect(getEditorHTML(editor)).toBe(
      '<p>Read <a href="{{ view_in_browser_url }}">View in browser</a>online</p>'
    )
  })

  it('lets text typed after an inserted link stay outside it', () => {
    const editor = load('<p>Visit now</p>')
    editor.commands.setTextSelection(rangeOf(editor, 'now').from)
    link(editor, 'https://x.example')
    editor.commands.insertContent('!')
    expect(getEditorHTML(editor)).toBe('<p>Visit <a href="https://x.example">https://x.example</a>!now</p>')
  })

  it('moves the colour onto the link when the selection also holds an image', () => {
    const editor = load('<p><img src="https://x.example/i.png"><span style="color: #ff0000"> label</span></p>')
    editor.commands.selectAll()
    link(editor, 'https://x.example')
    expect(getEditorHTML(editor)).toBe(
      '<p><a href="https://x.example" style="color: #ff0000"><img src="https://x.example/i.png"> label</a></p>'
    )
  })

  it('extends a styled link over the text next to it as one link', () => {
    const editor = load('<p><a href="https://old.example" style="color: #ff0000">old</a><span style="color: #ff0000">new</span></p>')
    editor.commands.selectAll()
    link(editor, 'https://new.example')
    expect(getEditorHTML(editor)).toBe('<p><a href="https://new.example" style="color: #ff0000">oldnew</a></p>')
  })

  it('changes the target of the whole link, keeping its style', () => {
    const editor = load('<p><a href="https://old.example" style="color: #ff0000">the link</a></p>')
    editor.commands.setTextSelection(rangeOf(editor, 'the link'))
    link(editor, 'https://new.example')
    expect(getEditorHTML(editor)).toBe('<p><a href="https://new.example" style="color: #ff0000">the link</a></p>')
  })
})

describe('removing a link', () => {
  it('removes the whole link around the cursor and keeps its colours on the text', () => {
    const editor = load(
      '<p><a href="https://x.example" style="color: #ff0000; text-decoration: none">the link</a> after</p>'
    )
    editor.commands.setTextSelection(rangeOf(editor, 'the link').from + 2)
    editor.commands.unsetLink()
    expect(getEditorHTML(editor)).toBe('<p><span style="color: #ff0000">the link</span> after</p>')
  })

  it('removes the link from an image it holds as well as from its text', () => {
    const editor = load('<p><a href="https://x.example"><img src="https://x.example/i.png"> label</a></p>')
    editor.commands.setTextSelection(rangeOf(editor, 'label').from + 1)
    editor.commands.unsetLink()
    expect(getEditorHTML(editor)).toBe('<p><img src="https://x.example/i.png"> label</p>')
  })

  it('keeps every text property of the link on its text', () => {
    const editor = load('<p><a href="https://x.example" style="color: #ff0000; font-size: 20px; padding: 4px">big</a></p>')
    editor.commands.setTextSelection(rangeOf(editor, 'big').from + 1)
    editor.commands.unsetLink()
    expect(getEditorHTML(editor)).toBe('<p><span style="color: #ff0000; font-size: 20px">big</span></p>')
  })

  it('links coloured text holding a line break as one link', () => {
    const editor = load('<p><span style="color: #ff0000">line 1<br>line 2</span></p>')
    editor.commands.selectAll()
    link(editor, 'https://x.example')
    expect(getEditorHTML(editor)).toBe('<p><a href="https://x.example" style="color: #ff0000">line 1<br>line 2</a></p>')
  })

  it('keeps the text style inside the link', () => {
    const editor = load('<p><a href="https://x.example"><span style="font-size: 20px">big</span></a></p>')
    editor.commands.setTextSelection(rangeOf(editor, 'big'))
    editor.commands.unsetLink()
    expect(getEditorHTML(editor)).toBe('<p><span style="font-size: 20px">big</span></p>')
  })

  it('does nothing with the cursor outside any link', () => {
    const editor = load('<p>plain <a href="https://x.example">link</a></p>')
    editor.commands.setTextSelection(rangeOf(editor, 'plain').from + 1)
    expect(editor.commands.unsetLink()).toBe(false)
    expect(getEditorHTML(editor)).toBe('<p>plain <a href="https://x.example">link</a></p>')
  })
})
