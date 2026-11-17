import { describe, it, expect, afterEach, beforeAll, afterAll } from 'vitest'
import { Editor } from '@tiptap/core'
import { createRichExtensions } from './shared/extensions'
import {
  getEditorHTML,
  sanitizeRichContent,
  handleBackgroundColorChange,
  handleTextColorChange,
  getEffectiveTextColor,
  getEffectiveBackgroundColor
} from './shared/utils'
import type { Editor as ReactEditor } from '@tiptap/react'

// The Text block's editor, loaded with HTML as a template stores it. What it gives
// back with getEditorHTML() is what the block saves and the email sends.

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

const roundTrip = (content: string) => getEditorHTML(load(content))

// The document range of the first occurrence of `text`.
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

const select = (editor: Editor, text: string) => {
  editor.commands.setTextSelection(rangeOf(editor, text))
}

const placeCursorIn = (editor: Editor, text: string) => {
  editor.commands.setTextSelection(rangeOf(editor, text).from + 1)
}

const asReactEditor = (editor: Editor) => editor as unknown as ReactEditor

afterEach(() => {
  editors.splice(0).forEach((editor) => editor.destroy())
  mounted.splice(0).forEach((element) => element.remove())
})

describe('loading and saving Text block HTML', () => {
  it('keeps styled paragraphs apart, with their style', () => {
    expect(
      roundTrip('<p style="text-align: center">First</p><p style="text-align: center; margin: 0">Second</p>')
    ).toBe('<p style="text-align: center">First</p><p style="text-align: center; margin: 0">Second</p>')
  })

  it('keeps bold, italic and underline that carry a style', () => {
    const html = roundTrip(
      '<p><strong style="color: #ff0000">bold</strong> <em style="color: #00ff00">italic</em> <u style="font-size: 20px">under</u></p>'
    )
    const root = document.createElement('div')
    root.innerHTML = html
    expect(root.querySelector('strong')?.textContent).toBe('bold')
    expect(root.querySelector('em')?.textContent).toBe('italic')
    expect(root.querySelector('u')?.textContent).toBe('under')
    expect(html).toContain('color: #ff0000')
    expect(html).toContain('color: #00ff00')
    expect(html).toContain('font-size: 20px')
  })

  it('keeps colours written in hex, and every declaration of a span as written', () => {
    expect(
      roundTrip('<p><span style="color: #31c48d; font-size: 20px; border: 1px solid #000">text</span></p>')
    ).toBe('<p><span style="color: #31c48d; font-size: 20px; border: 1px solid #000">text</span></p>')
  })

  it('keeps a style whose value is Liquid', () => {
    expect(roundTrip('<p><span style="color: {{ brand_color }}">Brand</span></p>')).toBe(
      '<p><span style="color: {{ brand_color }}">Brand</span></p>'
    )
  })

  it('gives the same HTML when loaded again', () => {
    const content =
      '<p style="text-align: right"><span style="color: #123456">a</span> <a href="https://x.example" style="color: #ff0000; text-decoration: none">link</a></p><ul style="padding-left: 20px"><li><p>item</p></li></ul>'
    const once = roundTrip(content)
    expect(roundTrip(once)).toBe(once)
  })

  it('keeps the style of lists and quotes', () => {
    expect(roundTrip('<ul style="padding-left: 20px"><li style="color: #333"><p>item</p></li></ul>')).toBe(
      '<ul style="padding-left: 20px"><li style="color: #333"><p>item</p></li></ul>'
    )
    expect(roundTrip('<blockquote style="border-left: 2px solid #ccc"><p>quote</p></blockquote>')).toBe(
      '<blockquote style="border-left: 2px solid #ccc"><p>quote</p></blockquote>'
    )
  })

  it('keeps images placed in the text', () => {
    expect(
      roundTrip('<p>before <img src="https://x.example/i.png" alt="Logo" width="40" style="vertical-align: middle"> after</p>')
    ).toBe(
      '<p>before <img src="https://x.example/i.png" alt="Logo" width="40" style="vertical-align: middle"> after</p>'
    )
  })

  it('keeps the colour of a <font> tag', () => {
    expect(roundTrip('<p><font color="#ff0000" face="Georgia" size="5">old</font></p>')).toBe(
      '<p><span style="color: #ff0000; font-family: Georgia; font-size: 24px">old</span></p>'
    )
  })

  it('keeps headings of every level as headings', () => {
    const html = '<h1>One</h1><h2>Two</h2><h3>Three</h3><h4>Four</h4><h5>Five</h5><h6>Six</h6>'
    expect(roundTrip(html)).toBe(html)
  })

  it('keeps a heading’s style on the heading, as written', () => {
    // Opening a Text block used to turn its headings into paragraphs
    expect(roundTrip('<h2 style="color:red; text-align:center">Titre</h2><p>Corps</p>')).toBe(
      '<h2 style="color:red; text-align:center">Titre</h2><p>Corps</p>'
    )
    expect(roundTrip('<h2 style="font-size: 36px; {% if vip %}color: gold{% endif %}">Title</h2>')).toBe(
      '<h2 style="font-size: 36px; {% if vip %}color: gold{% endif %}">Title</h2>'
    )
  })

  it('keeps styled text inside a heading', () => {
    expect(roundTrip('<h1><span style="color: red">Title</span> rest</h1>')).toBe(
      '<h1><span style="color: red">Title</span> rest</h1>'
    )
  })

  it('keeps a heading that opens a list item', () => {
    expect(roundTrip('<ul><li><h3>Step 1</h3><p>Do this</p></li></ul>')).toBe(
      '<ul><li><h3>Step 1</h3><p>Do this</p></li></ul>'
    )
  })

  it('saves a heading-only text without an empty paragraph after it', () => {
    expect(roundTrip('<h2>Only</h2>')).toBe('<h2>Only</h2>')
  })

  it('splits a styled <div> into a paragraph with its box and text with its text style', () => {
    expect(roundTrip('<div style="color: #ff0000; margin: 10px">a</div><div>b</div>')).toBe(
      '<p style="margin: 10px"><span style="color: #ff0000">a</span></p><p>b</p>'
    )
  })

  it('keeps the alignment of a <div>, a <center> and a heading', () => {
    expect(roundTrip('<div style="text-align: center; color: red">centered</div>')).toBe(
      '<p style="text-align: center"><span style="color: red">centered</span></p>'
    )
    expect(roundTrip('<center>middle</center>')).toBe('<p style="text-align: center">middle</p>')
  })

  it('combines the styles of nested elements', () => {
    expect(roundTrip('<div style="color: red"><span style="font-size: 20px">big</span> small</div>')).toBe(
      '<p><span style="color: red; font-size: 20px">big</span><span style="color: red"> small</span></p>'
    )
    expect(
      roundTrip('<p><span style="color: red; font-size: 20px">a <span style="color: blue">b</span></span></p>')
    ).toBe(
      '<p><span style="color: red; font-size: 20px">a </span><span style="color: blue; font-size: 20px">b</span></p>'
    )
  })

  it('lets an inner element override the style of an outer one, as CSS does', () => {
    // The paragraph's colour is repeated on its text, over the <div>'s
    expect(roundTrip('<div style="color: red"><p style="color: blue">blue</p></div>')).toBe(
      '<p style="color: blue"><span style="color: blue">blue</span></p>'
    )
    expect(
      roundTrip(
        '<p><span style="color: #333; font-size: 20px">Read <a href="https://x.example" style="color: #06c">here</a></span></p>'
      )
    ).toBe(
      '<p><span style="color: #333; font-size: 20px">Read </span><a href="https://x.example" style="color: #06c"><span style="font-size: 20px">here</span></a></p>'
    )
  })

  it('keeps an element whose style cancels its own formatting plain', () => {
    expect(roundTrip('<p><em style="font-style: normal">upright</em></p>')).toBe(
      '<p><span style="font-style: normal">upright</span></p>'
    )
    expect(roundTrip('<p><strong style="font-weight: normal">light</strong></p>')).toBe(
      '<p><span style="font-weight: normal">light</span></p>'
    )
    expect(roundTrip('<p><u style="text-decoration: none">plain</u></p>')).toBe(
      '<p><span style="text-decoration: none">plain</span></p>'
    )
    expect(roundTrip('<p><strong style="font-weight: 700; color: red">bold</strong></p>')).toBe(
      '<p><span style="font-weight: 700; color: red"><strong>bold</strong></span></p>'
    )
  })

  it('reads a relative <font> size from the default size', () => {
    expect(roundTrip('<p><font size="+1">up</font> <font size="-1">down</font></p>')).toBe(
      '<p><span style="font-size: 18px">up</span> <span style="font-size: 13px">down</span></p>'
    )
  })

  it('saves links without editor-only attributes', () => {
    expect(
      roundTrip('<p><a class="editor-link" href="https://x.example" color="red" style="color: red">l</a></p>')
    ).toBe('<p><a href="https://x.example" style="color: red">l</a></p>')
  })

  it('keeps a link target written in Liquid and its other attributes', () => {
    expect(
      roundTrip('<p><a href="{{ unsubscribe_url }}" target="_blank" rel="noopener" class="cta">Unsubscribe</a></p>')
    ).toBe('<p><a href="{{ unsubscribe_url }}" target="_blank" rel="noopener" class="cta">Unsubscribe</a></p>')
  })
})

describe('text and background colours', () => {
  it('changes only the colour of the selected text', () => {
    const editor = load('<p><span style="color: #ff0000; font-size: 20px">word</span> rest</p>')
    select(editor, 'word')
    handleTextColorChange(asReactEditor(editor), '#0000ff')
    expect(getEditorHTML(editor)).toBe('<p><span style="color: #0000ff; font-size: 20px">word</span> rest</p>')
  })

  it('removes only the colour when the colour is cleared', () => {
    const editor = load(
      '<p><span style="color: #ff0000; background-color: #ffff00; font-size: 20px">word</span> rest</p>'
    )
    select(editor, 'word')
    handleTextColorChange(asReactEditor(editor), '')
    expect(getEditorHTML(editor)).toBe(
      '<p><span style="background-color: #ffff00; font-size: 20px">word</span> rest</p>'
    )
  })

  it('adds a background without touching the colour', () => {
    const editor = load('<p><span style="color: #ff0000">word</span> rest</p>')
    select(editor, 'word')
    handleBackgroundColorChange(asReactEditor(editor), '#ffff00')
    expect(getEditorHTML(editor)).toBe(
      '<p><span style="color: #ff0000; background-color: #ffff00">word</span> rest</p>'
    )
  })

  it('keeps each run its own other properties when a selection spans several', () => {
    const editor = load(
      '<p><span style="font-size: 20px">big</span><span style="font-size: 10px">small</span></p>'
    )
    editor.commands.setTextSelection({ from: rangeOf(editor, 'big').from, to: rangeOf(editor, 'small').to })
    handleTextColorChange(asReactEditor(editor), '#ff0000')
    expect(getEditorHTML(editor)).toBe(
      '<p><span style="font-size: 20px; color: #ff0000">big</span><span style="font-size: 10px; color: #ff0000">small</span></p>'
    )
  })

  it('colours the whole link when the cursor is inside it', () => {
    const editor = load('<p><a href="https://x.example">link</a> after</p>')
    placeCursorIn(editor, 'link')
    handleTextColorChange(asReactEditor(editor), '#ff0000')
    expect(getEditorHTML(editor)).toBe('<p><a href="https://x.example" style="color: #ff0000">link</a> after</p>')
  })

  it('colours part of a link inside it, keeping one link', () => {
    const editor = load('<p><a href="https://x.example">abcdef</a></p>')
    select(editor, 'bcd')
    handleTextColorChange(asReactEditor(editor), '#ff0000')
    expect(getEditorHTML(editor)).toBe(
      '<p><a href="https://x.example">a<span style="color: #ff0000">bcd</span>ef</a></p>'
    )
  })

  it('colours a whole selected link on the link', () => {
    const editor = load('<p><a href="https://x.example">abc</a> after</p>')
    select(editor, 'abc')
    handleTextColorChange(asReactEditor(editor), '#ff0000')
    expect(getEditorHTML(editor)).toBe('<p><a href="https://x.example" style="color: #ff0000">abc</a> after</p>')
  })

  it('puts the colour on the link and on plain text when a selection spans both', () => {
    const editor = load('<p><a href="https://x.example">link</a> after</p>')
    editor.commands.setTextSelection({ from: rangeOf(editor, 'link').from, to: rangeOf(editor, 'after').to })
    handleTextColorChange(asReactEditor(editor), '#ff0000')
    expect(getEditorHTML(editor)).toBe(
      '<p><a href="https://x.example" style="color: #ff0000">link</a><span style="color: #ff0000"> after</span></p>'
    )
  })

  it('colours the text typed next when nothing is selected', () => {
    const editor = load('<p>start</p>')
    editor.commands.setTextSelection(rangeOf(editor, 'start').to)
    handleTextColorChange(asReactEditor(editor), '#ff0000')
    editor.commands.insertContent('red')
    expect(getEditorHTML(editor)).toBe('<p>start<span style="color: #ff0000">red</span></p>')
  })

  it('reports the colours of the selection, the link first', () => {
    const editor = load(
      '<p><span style="color: #ff0000; background-color: #ffff00">word</span> <a href="https://x.example" style="color: #00ff00">link</a></p>'
    )
    select(editor, 'word')
    expect(getEffectiveTextColor(asReactEditor(editor))).toBe('#ff0000')
    expect(getEffectiveBackgroundColor(asReactEditor(editor))).toBe('#ffff00')
    placeCursorIn(editor, 'link')
    expect(getEffectiveTextColor(asReactEditor(editor))).toBe('#00ff00')
  })
})

describe('styles holding Liquid control tags', () => {
  it('keeps a {% if %} spanning several declarations whole when loading', () => {
    expect(
      roundTrip('<div style="{% if vip %}margin: 10px; color: gold{% endif %}"><b>Hello</b> world</div>')
    ).toBe('<p style="{% if vip %}margin: 10px; color: gold{% endif %}"><strong>Hello</strong> world</p>')
  })

  it('adds a colour after a {% if %} instead of cutting it', () => {
    const editor = load('<p><span style="{% if vip %}font-size: 20px; color: gold{% endif %}">Hello</span></p>')
    select(editor, 'Hello')
    handleTextColorChange(asReactEditor(editor), '#ff0000')
    expect(getEditorHTML(editor)).toBe(
      '<p><span style="{% if vip %}font-size: 20px; color: gold{% endif %}; color: #ff0000">Hello</span></p>'
    )
  })

  it('leaves a {% if %} untouched when a colour is cleared', () => {
    const content = '<p><span style="{% if vip %}font-size: 20px; color: gold{% endif %}">Hello</span></p>'
    const editor = load(content)
    select(editor, 'Hello')
    handleTextColorChange(asReactEditor(editor), '')
    expect(getEditorHTML(editor)).toBe(content)
  })
})

describe('line breaks and images inside styled text', () => {
  it('colours a link holding a line break as one link', () => {
    const editor = load('<p><a href="https://x.example">line 1<br>line 2</a></p>')
    placeCursorIn(editor, 'line 1')
    handleTextColorChange(asReactEditor(editor), '#ff0000')
    expect(getEditorHTML(editor)).toBe('<p><a href="https://x.example" style="color: #ff0000">line 1<br>line 2</a></p>')
  })

  it('clears a colour across a line break', () => {
    const editor = load('<p><span style="color: #ff0000">line 1<br>line 2</span></p>')
    editor.commands.selectAll()
    handleTextColorChange(asReactEditor(editor), '')
    expect(getEditorHTML(editor)).toBe('<p>line 1<br>line 2</p>')
  })

  it('keeps an empty alt, which marks an image as decorative', () => {
    expect(roundTrip('<p><img src="https://x.example/i.png" alt=""></p>')).toBe(
      '<p><img src="https://x.example/i.png" alt=""></p>'
    )
  })
})

describe('colours declared on paragraphs', () => {
  it('reports and clears the colour of a paragraph', () => {
    const editor = load('<p style="color: #ff0000; margin: 0">word</p>')
    select(editor, 'word')
    expect(getEffectiveTextColor(asReactEditor(editor))).toBe('#ff0000')
    handleTextColorChange(asReactEditor(editor), '')
    expect(getEditorHTML(editor)).toBe('<p style="margin: 0">word</p>')
  })

  it('clears a paragraph colour repeated on its text', () => {
    const editor = load('<div style="color: red"><p style="color: blue">blue</p></div>')
    select(editor, 'blue')
    handleTextColorChange(asReactEditor(editor), '')
    expect(getEditorHTML(editor)).toBe('<p>blue</p>')
  })

  it('reports the colour of text inside a link before the link’s', () => {
    const editor = load(
      '<p><a href="https://x.example" style="color: #ff0000">a<span style="color: #0000ff">bcd</span>ef</a></p>'
    )
    select(editor, 'bcd')
    expect(getEffectiveTextColor(asReactEditor(editor))).toBe('#0000ff')
  })
})

describe('formatting written as CSS', () => {
  it('loads CSS bold, italic and underline without adding tags', () => {
    for (const html of [
      '<p style="font-weight: bold">Hello</p>',
      '<p><span style="font-weight: bold">b</span></p>',
      '<p><span style="font-style: italic">i</span></p>',
      '<p><a href="https://x.example" style="color: #06c; text-decoration: underline">Click</a></p>'
    ]) {
      expect(roundTrip(html)).toBe(html)
    }
  })

  it('gives the same HTML when a heading is opened twice', () => {
    const once = roundTrip('<h2>One</h2>')
    expect(roundTrip(once)).toBe(once)
  })

  it('lets a light span inside bold text stay light', () => {
    expect(roundTrip('<p><strong>bold <span style="font-weight: normal">light</span></strong></p>')).toBe(
      '<p><strong>bold </strong><span style="font-weight: normal">light</span></p>'
    )
  })

  it('passes the alignment of a wrapper to the paragraphs it holds', () => {
    expect(roundTrip('<div style="text-align: center"><p>First</p><p>Second</p></div>')).toBe(
      '<p style="text-align: center">First</p><p style="text-align: center">Second</p>'
    )
    expect(roundTrip('<center><p>A</p></center>')).toBe('<p style="text-align: center">A</p>')
  })
})

describe('pasting Liquid', () => {
  // jsdom has no ClipboardEvent, which ProseMirror builds for a paste
  const clipboardEvent = (globalThis as { ClipboardEvent?: unknown }).ClipboardEvent
  beforeAll(() => {
    if (!clipboardEvent) {
      ;(globalThis as { ClipboardEvent?: unknown }).ClipboardEvent = class extends Event {
        clipboardData = null
      }
    }
  })
  afterAll(() => {
    if (!clipboardEvent) delete (globalThis as { ClipboardEvent?: unknown }).ClipboardEvent
  })

  it('keeps underscores and stars of pasted Liquid as written', () => {
    const editor = load('<p></p>')
    editor.view.pasteText('{{ a | append: " _x_ " }} {% assign _a = 1 %} {{ _b }} {{ "*y*" }}')
    expect(getEditorHTML(editor)).toBe(
      '<p>{{ a | append: " _x_ " }} {% assign _a = 1 %} {{ _b }} {{ "*y*" }}</p>'
    )
  })
})

describe('fourth review', () => {
  it('keeps declarations after a Liquid block editable', () => {
    const editor = load('<p><span style="{% if vip %}font-size: 20px{% endif %}">Hello</span></p>')
    select(editor, 'Hello')
    handleTextColorChange(asReactEditor(editor), '#ff0000')
    expect(getEffectiveTextColor(asReactEditor(editor))).toBe('#ff0000')
    handleTextColorChange(asReactEditor(editor), '#0000ff')
    expect(getEditorHTML(editor)).toBe(
      '<p><span style="{% if vip %}font-size: 20px{% endif %}; color: #0000ff">Hello</span></p>'
    )
    handleTextColorChange(asReactEditor(editor), '')
    expect(getEditorHTML(editor)).toBe('<p><span style="{% if vip %}font-size: 20px{% endif %}">Hello</span></p>')
  })

  it('keeps a Liquid block of a formatting element or a wrapper on the text', () => {
    expect(roundTrip('<p><strong style="color: #ff0000; {% if vip %}font-size: 20px{% endif %}">Hello</strong></p>')).toBe(
      '<p><span style="color: #ff0000; {% if vip %}font-size: 20px{% endif %}"><strong>Hello</strong></span></p>'
    )
    expect(roundTrip('<div style="{% if vip %}color: gold{% endif %}"><p>Hello</p></div>')).toBe(
      '<p><span style="{% if vip %}color: gold{% endif %}">Hello</span></p>'
    )
  })

  it('clears a paragraph colour from the selected part only', () => {
    const editor = load('<p style="color: #ff0000">first second</p>')
    select(editor, 'first')
    handleTextColorChange(asReactEditor(editor), '')
    expect(getEditorHTML(editor)).toBe('<p>first<span style="color: #ff0000"> second</span></p>')
  })

  it('keeps the style of an underline cancelled with text-decoration-line', () => {
    expect(roundTrip('<p><u style="text-decoration-line: none">plain</u></p>')).toBe(
      '<p><span style="text-decoration-line: none">plain</span></p>'
    )
  })

  it('formats pasted prose and leaves pasted Liquid alone in the same paste', () => {
    const clipboardEvent = (globalThis as { ClipboardEvent?: unknown }).ClipboardEvent
    if (!clipboardEvent) {
      ;(globalThis as { ClipboardEvent?: unknown }).ClipboardEvent = class extends Event {
        clipboardData = null
      }
    }
    try {
      const editor = load('<p></p>')
      editor.view.pasteText('_hello_ {{ a | append: " _x_ " }}')
      expect(getEditorHTML(editor)).toBe('<p><em>hello</em> {{ a | append: " _x_ " }}</p>')
    } finally {
      if (!clipboardEvent) delete (globalThis as { ClipboardEvent?: unknown }).ClipboardEvent
    }
  })

  it('removes CSS bold with the keyboard shortcut', () => {
    const editor = load('<p><span style="font-weight: bold">bold</span></p>')
    select(editor, 'bold')
    editor.commands.keyboardShortcut('Mod-b')
    expect(getEditorHTML(editor)).toBe('<p>bold</p>')
  })
})

describe('tables in a Text block', () => {
  const table =
    '<table style="width: 100%" cellpadding="4"><tbody><tr>' +
    '<td style="color: red" colspan="2">Total: {{ order.total | money }}</td>' +
    '</tr></tbody></table>'

  it('keeps a table as written, between paragraphs', () => {
    expect(roundTrip(`<p>before</p>${table}<p>after</p>`)).toBe(`<p>before</p>${table}<p>after</p>`)
  })

  it('leaves a table untouched when the colour of everything is changed', () => {
    const editor = load(`<p>before</p>${table}<p>after</p>`)
    editor.commands.selectAll()
    handleTextColorChange(asReactEditor(editor), '#0000ff')
    expect(getEditorHTML(editor)).toContain(table)
  })

  it('drops handlers from a table it loads', () => {
    const html = roundTrip('<table><tbody><tr><td><img src="x" onerror="window.__probe = 1">x</td></tr></tbody></table>')
    expect(html).not.toContain('onerror')
    expect(html).toContain('<table>')
  })
})

describe('attributes the editor keeps', () => {
  it('keeps id, class and data-* on paragraphs and spans', () => {
    // Attributes come out in the editor's order, which HTML gives no meaning
    expect(
      roundTrip(
        '<p id="intro" class="lead" data-track="hero">A <span class="brand" data-x="1">brand</span> ' +
          '<span id="note" style="color: red">note</span></p>'
      )
    ).toBe(
      '<p id="intro" class="lead" data-track="hero">A <span class="brand" data-x="1">brand</span> ' +
        '<span style="color: red" id="note">note</span></p>'
    )
  })

  it('keeps a link’s id and title', () => {
    const html = '<p><a href="#top" id="back" title="Back to the top">Top</a></p>'
    expect(roundTrip(html)).toBe(html)
  })

  it('keeps an empty anchor that links point to', () => {
    const html = '<p><a name="top"></a>Title <a id="details"></a></p>'
    expect(roundTrip(html)).toBe(html)
  })

  it('keeps srcset, class, border and align on images', () => {
    expect(
      roundTrip(
        '<p><img src="https://x.example/a.png" srcset="https://x.example/a@2x.png 2x" class="logo" border="0" align="left" alt="Logo"></p>'
      )
    ).toBe(
      '<p><img src="https://x.example/a.png" alt="Logo" srcset="https://x.example/a@2x.png 2x" class="logo" border="0" align="left"></p>'
    )
  })

  it('keeps a span’s class when its colour changes', () => {
    const editor = load('<p><span class="brand" style="color: red">brand</span> x</p>')
    select(editor, 'brand')
    handleTextColorChange(asReactEditor(editor), '')
    expect(getEditorHTML(editor)).toBe('<p><span class="brand">brand</span> x</p>')
  })
})

describe('loose text in a wrapper that also holds blocks', () => {
  // As the Text block loads content: sanitized and prepared, then parsed
  const loaded = (html: string) => getEditorHTML(load(sanitizeRichContent(html)))

  it('keeps the wrapper’s alignment on its loose text', () => {
    expect(loaded('<div style="text-align: center">First<div>Second</div></div>')).toBe(
      '<p style="text-align: center">First</p><p style="text-align: center">Second</p>'
    )
  })

  it('makes one paragraph of each run of loose text, between blocks', () => {
    expect(loaded('<div style="text-align: right"><p>A</p>loose <b>bold</b><p>B</p></div>')).toBe(
      '<p style="text-align: right">A</p><p style="text-align: right">loose <strong>bold</strong></p>' +
        '<p style="text-align: right">B</p>'
    )
  })

  it('changes nothing for a wrapper without alignment', () => {
    expect(loaded('<div>First<div>Second</div></div>')).toBe('<p>First</p><p>Second</p>')
  })
})

describe('fourth pass on kept attributes', () => {
  // ProseMirror builds a ClipboardEvent for a paste, which jsdom lacks
  const withClipboardEvent = (run: () => void) => {
    const original = (globalThis as { ClipboardEvent?: unknown }).ClipboardEvent
    if (!original) {
      ;(globalThis as { ClipboardEvent?: unknown }).ClipboardEvent = class extends Event {
        clipboardData = null
      }
    }
    try {
      run()
    } finally {
      if (!original) delete (globalThis as { ClipboardEvent?: unknown }).ClipboardEvent
    }
  }

  it('keeps the text of a named anchor that holds some', () => {
    expect(roundTrip('<p><a name="sec1">Section 1</a> text</p>')).toBe('<p>Section 1 text</p>')
  })

  it('does not copy an id or data-* attributes onto the second half of a split paragraph', () => {
    const editor = load('<p id="intro" class="lead" data-track="x">Hello world</p>')
    editor.commands.setTextSelection(rangeOf(editor, 'Hello').to)
    editor.commands.splitBlock()
    expect(getEditorHTML(editor)).toBe(
      '<p id="intro" class="lead" data-track="x">Hello</p><p class="lead"> world</p>'
    )
  })

  it('drops the classes, ids and data-* of HTML pasted from elsewhere, and unfolds its tables', () => {
    withClipboardEvent(() => {
      const editor = load('<p></p>')
      editor.view.pasteHTML(
        '<p class="MsoNormal" id="x1" data-start="0">Hi <span class="SpellE">there</span></p>' +
          '<table class="MsoTableGrid"><tbody><tr><td>Cell 1</td><td>Cell 2</td></tr></tbody></table>'
      )
      expect(getEditorHTML(editor)).toBe('<p>Hi there</p><p>Cell 1</p><p>Cell 2</p>')
    })
  })

  it('keeps everything of HTML copied from the editor itself, but its paste marker', () => {
    withClipboardEvent(() => {
      const editor = load('<p></p>')
      editor.view.pasteHTML(
        '<table data-pm-slice="0 0 []"><tbody><tr><td class="c">x</td></tr></tbody></table>' +
          '<p class="lead">y</p>'
      )
      const html = getEditorHTML(editor)
      expect(html).toContain('<table><tbody><tr><td class="c">x</td></tr></tbody></table>')
      expect(html).toContain('<p class="lead">y</p>')
      expect(html).not.toContain('data-pm-slice')
    })
  })

  it('keeps the class of the outer span on text in a nested styled span', () => {
    expect(roundTrip('<p><span class="brand">a <span style="color:red">b</span></span></p>')).toBe(
      '<p><span class="brand">a </span><span style="color: red" class="brand">b</span></p>'
    )
  })

  it('keeps a span’s class when its text is linked or unlinked, and on an inserted link', () => {
    const editor = load('<p><span class="brand" style="color: red">brand</span> x</p>')
    select(editor, 'brand')
    editor.commands.setLink({ href: 'https://x.example' })
    expect(getEditorHTML(editor)).toBe(
      '<p><a href="https://x.example" style="color: red"><span class="brand">brand</span></a> x</p>'
    )
    editor.commands.unsetLink()
    expect(getEditorHTML(editor)).toBe('<p><span style="color: red" class="brand">brand</span> x</p>')
  })
})
