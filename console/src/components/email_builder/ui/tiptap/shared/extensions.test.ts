import { describe, it, expect, afterEach } from 'vitest'
import { Editor } from '@tiptap/core'
import type { Extensions } from '@tiptap/core'
import { createInlineExtensions, createRichExtensions, isInsideLiquidMarkup } from './extensions'

const mounted: HTMLElement[] = []

const mountEditor = (extensions: Extensions, content: string) => {
  const element = document.createElement('div')
  document.body.appendChild(element)
  mounted.push(element)
  return new Editor({ element, extensions, content })
}

afterEach(() => {
  mounted.splice(0).forEach((element) => element.remove())
})

/**
 * Regression tests for the inline (button) editor schema.
 *
 * Issue #352: typing into a button produced one character per line
 * ("Example", then "h", "o", "l" each on a new line). Root cause: StarterKit
 * enables the TrailingNode extension by default. In the inline schema there is
 * no paragraph, so TrailingNode resolved its node to the schema's default inline
 * type (hardBreak) and appended a <br> after every transaction — i.e. after every
 * keystroke. Disabling trailingNode in createInlineExtensions fixes it.
 */
describe('createInlineExtensions', () => {
  let editor: Editor | undefined

  afterEach(() => {
    editor?.destroy()
    editor = undefined
  })

  const makeEditor = (content: string) => mountEditor(createInlineExtensions(), content)

  it('does not insert hard breaks when typing characters (issue #352)', () => {
    editor = makeEditor('<span data-inline-doc="">Example</span>')

    // Type "hol" character-by-character at the end of the content,
    // exactly like the user did in the bug report.
    editor.commands.setTextSelection(editor.state.doc.content.size)
    for (const char of 'hol') {
      editor.commands.insertContent(char)
    }

    const html = editor.getHTML()
    expect(html).not.toContain('<br')
    expect(editor.getText()).toBe('Examplehol')
  })

  it('keeps the inline schema free of block/break trailing nodes', () => {
    editor = makeEditor('<span data-inline-doc="">Example</span>')

    // The trailing-node behaviour appends a node after the content on every
    // transaction. A no-op selection change must not grow the document.
    const sizeBefore = editor.state.doc.content.size
    editor.commands.setTextSelection(1)
    editor.commands.setTextSelection(editor.state.doc.content.size)
    expect(editor.state.doc.content.size).toBe(sizeBefore)
    expect(editor.getHTML()).not.toContain('<br')
  })
})

/**
 * Types one character at a time through ProseMirror's handleTextInput, the path a
 * keystroke takes, so input rules run exactly as they do in the browser.
 */
const typeText = (editor: Editor, text: string) => {
  const { view } = editor
  for (const char of text) {
    const { from, to } = view.state.selection
    const handled = view.someProp('handleTextInput', (f) =>
      f.call(view, view, from, to, char, () => view.state.tr.insertText(char, from, to))
    )
    if (!handled) view.dispatch(view.state.tr.insertText(char, from, to))
  }
}

/**
 * Typography curls typed quotes and turns != into ≠, and StarterKit's Markdown
 * shortcuts turn ` _x_ ` into <em>x</em>. Inside Liquid markup that breaks the
 * template: `{{ first_name | default: ‘Valued User’ }}` renders "Hello !" because
 * Liquid reads the curly-quoted argument as an unknown variable.
 */
describe('input rules inside Liquid markup', () => {
  let editor: Editor | undefined

  afterEach(() => {
    editor?.destroy()
    editor = undefined
  })

  const editors = {
    'rich text block': () => mountEditor(createRichExtensions(), '<p></p>'),
    'inline button text': () => mountEditor(createInlineExtensions(), '<span data-inline-doc=""></span>')
  }

  describe.each(Object.entries(editors))('%s', (_name, makeEditor) => {
    const typed = (text: string) => {
      editor = makeEditor()
      typeText(editor, text)
      return editor
    }

    it('keeps the quotes of a default filter typed by the user', () => {
      const text = "Hello {{ first_name | default: 'Valued User' }}!"
      expect(typed(text).getText()).toBe(text)
    })

    it('keeps a quote typed right after the opening braces', () => {
      const text = `{{'2024-03-05' | date: '%Y'}} {{"x" | upcase}}`
      expect(typed(text).getText()).toBe(text)
    })

    it('keeps double quotes and the != operator in a tag', () => {
      const text = '{% if contact.country != "FR" %}abroad{% endif %}'
      expect(typed(text).getText()).toBe(text)
    })

    it('keeps quotes after a closing brace inside a string of a tag', () => {
      const text = `{% assign x = "}}" | append: 'y' %}`
      expect(typed(text).getText()).toBe(text)
    })

    it('keeps Markdown shortcuts out of Liquid markup', () => {
      const html = typed('{{ a | append: " _x_ " }} {% assign _a = "1" %}{{ contact.first_name }}').getHTML()
      expect(html).not.toContain('<em>')
    })

    it('does not apply a Markdown shortcut across a tag delimiter', () => {
      const html = typed('{% assign _a = 1 %} snake_case').getHTML()
      expect(html).not.toContain('<em>')
    })

    it('still curls quotes in prose, including after a closed tag', () => {
      editor = typed(`{{ contact.first_name }} said 'hi' != "bye"`)
      expect(editor.getText()).toBe('{{ contact.first_name }} said \u2018hi\u2019 \u2260 \u201cbye\u201d')
    })

    it('still applies Markdown shortcuts in prose', () => {
      expect(typed('some _italic_ text').getHTML()).toContain('<em>italic</em>')
    })

    it('still applies a Markdown shortcut around a whole tag', () => {
      expect(typed('**{{ contact.first_name }}**').getHTML()).toContain('<strong>{{ contact.first_name }}</strong>')
    })
  })

  it('sees a tag opened far before the cursor, behind formatted text', () => {
    // Tiptap's own text-before-cursor only covers the last 500 characters of a block
    const filler = 'x'.repeat(600)
    editor = mountEditor(createRichExtensions(), `<p>{% if a == <strong>${filler}</strong> or b ==</p>`)
    editor.commands.setTextSelection(editor.state.doc.content.size - 1)
    typeText(editor, " 'y' %}")
    expect(editor.getText()).toBe(`{% if a == ${filler} or b == 'y' %}`)
  })
})

describe('isInsideLiquidMarkup', () => {
  it.each([
    ['Hello {{ first_name | default: ', true],
    ['{% if contact.age ', true],
    ['{{ a }} and {% if b ', true],
    ['{% assign x = "}}" | append: ', true],
    ['Hello {{ first_name }} ', false],
    ['{% if a %}yes ', false],
    ['{{ a } ', false],
    ['plain text ', false],
    ['', false]
  ])('%j → %s', (textBefore, expected) => {
    expect(isInsideLiquidMarkup(textBefore)).toBe(expected)
  })
})
