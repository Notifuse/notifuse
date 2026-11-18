import { describe, it, expect } from 'vitest'
import type { EmailBlock } from '../types'
import {
  defaultHeadingStyles,
  ensureHeadingStyles,
  readHeadingStyles,
  serializeHeadingStyles,
  validateHeadingStyle,
  writeHeadingStyles,
  colorFromPicker,
  headingFieldDiffers,
  headingSetCount
} from './headingStyles'

const block = (id: string, type: string, extra: Record<string, unknown> = {}): EmailBlock =>
  ({ id, type, attributes: {}, ...extra }) as unknown as EmailBlock

const text = (id: string, content: string) => block(id, 'mj-text', { content })

const email = (head: EmailBlock[] | null, ...body: EmailBlock[]): EmailBlock =>
  block('root', 'mjml', {
    children: [
      ...(head ? [block('head', 'mj-head', { children: head })] : []),
      block('body', 'mj-body', { children: body })
    ]
  })

const attributes = (fontFamily = 'Georgia, serif', color = '#222222') =>
  block('attrs', 'mj-attributes', {
    children: [block('attrs-text', 'mj-text', { attributes: { fontFamily, color } })]
  })

const userStyle = block('user-style', 'mj-style', { attributes: { inline: 'inline' }, content: 'h2 { font-size: 12px }' })

const styleBlocks = (tree: EmailBlock) =>
  (tree.children?.find((child) => child.type === 'mj-head')?.children ?? []).filter(
    (child) => child.type === 'mj-style'
  )

describe('serializeHeadingStyles and readHeadingStyles', () => {
  it('reads back the styles it wrote', () => {
    const styles = defaultHeadingStyles({ fontFamily: 'Georgia, serif', color: '#222222' })
    const tree = writeHeadingStyles(email([attributes()], text('t', '<h2>x</h2>')), styles)
    expect(readHeadingStyles(tree)).toEqual(styles)
  })

  it('writes CSS the converter and the Liquid pass leave alone', () => {
    const css = serializeHeadingStyles(defaultHeadingStyles({ fontFamily: '"Helvetica Neue", Arial', color: '#000' }))
    for (const forbidden of ['&', '<', '>', '{{', '{%']) expect(css).not.toContain(forbidden)
    expect(css).toContain('h2 {')
    expect(css).toContain('font-size: 22px')
  })

  it('gives null without a managed block', () => {
    expect(readHeadingStyles(email([userStyle], text('t', '<h2>x</h2>')))).toBeNull()
  })

  it('reads a block whose CSS was edited by hand, or that is no longer inline, as hand-edited', () => {
    const tree = writeHeadingStyles(email([]), defaultHeadingStyles({}))
    const [managed] = styleBlocks(tree)
    const content = (managed as unknown as { content: string }).content
    const edited = (change: Record<string, unknown>) =>
      email([{ ...managed, ...change } as EmailBlock])
    expect(readHeadingStyles(edited({ content: content.replace('22px', '40px') }))).toBe('hand-edited')
    expect(readHeadingStyles(edited({ attributes: {} }))).toBe('hand-edited')
    expect(readHeadingStyles(edited({ content: content.replace('{"', '{x"') }))).toBe('hand-edited')
  })

  it('reads the last of two managed blocks, and writing keeps only one', () => {
    const first = writeHeadingStyles(email([]), defaultHeadingStyles({ color: '#111111' }))
    const second = writeHeadingStyles(email([]), defaultHeadingStyles({ color: '#999999' }))
    const both = email([...styleBlocks(first), ...styleBlocks(second).map((b) => ({ ...b, id: 'other' }))])
    expect((readHeadingStyles(both) as ReturnType<typeof defaultHeadingStyles>)[1].color).toBe('#999999')
    expect(styleBlocks(writeHeadingStyles(both, defaultHeadingStyles({})))).toHaveLength(1)
  })
})

describe('validateHeadingStyle', () => {
  it('accepts the defaults and refuses values that could break the CSS', () => {
    expect(validateHeadingStyle(defaultHeadingStyles({ fontFamily: 'Arial', color: '#fff' })[2])).toEqual([])
    const base = defaultHeadingStyles({})[2]
    expect(validateHeadingStyle({ ...base, fontSize: '300px' })).not.toEqual([])
    expect(validateHeadingStyle({ ...base, color: 'red; } body { x' })).not.toEqual([])
    expect(validateHeadingStyle({ ...base, fontFamily: 'Arial */' })).not.toEqual([])
    expect(validateHeadingStyle({ ...base, color: '{{ brand }}' })).not.toEqual([])
    expect(validateHeadingStyle({ ...base, lineHeight: 'tall' })).not.toEqual([])
  })
})

describe('ensureHeadingStyles', () => {
  const before = email([attributes(), userStyle], text('t', '<p>plain</p>'))
  const after = email([attributes(), userStyle], text('t', '<h2>Now a heading</h2>'))

  it('adds the managed styles when an edit makes the first heading, before the user’s own styles', () => {
    const result = ensureHeadingStyles(before, after)
    const styles = styleBlocks(result)
    expect(styles).toHaveLength(2)
    expect(styles[1]).toEqual(userStyle)
    expect((styles[0] as unknown as { attributes: Record<string, unknown> }).attributes.inline).toBe('inline')
    const read = readHeadingStyles(result) as ReturnType<typeof defaultHeadingStyles>
    expect(read[2]).toMatchObject({ fontSize: '22px' })
    // A heading takes its block's font and colour: a white text block keeps white headings
    expect(read[2].fontFamily).toBeUndefined()
    expect(read[2].color).toBeUndefined()
  })

  it('adds nothing when the email already had headings, or still has none', () => {
    expect(ensureHeadingStyles(after, after)).toBe(after)
    expect(ensureHeadingStyles(before, before)).toBe(before)
  })

  it('adds nothing a second time', () => {
    const once = ensureHeadingStyles(before, after)
    expect(ensureHeadingStyles(before, once)).toBe(once)
  })

  it('counts only Text blocks: a heading in Raw HTML adds nothing', () => {
    const raw = email([], block('r', 'mj-raw', { content: '<h2>raw</h2>' }))
    expect(ensureHeadingStyles(email([]), raw)).toBe(raw)
  })

  it('creates the head and the default attributes when the email has none', () => {
    const result = ensureHeadingStyles(email(null, text('t', '<p>x</p>')), email(null, text('t', '<h1>x</h1>')))
    const head = result.children?.find((child) => child.type === 'mj-head')
    expect(head?.children?.map((child) => child.type)).toEqual(['mj-attributes', 'mj-style'])
  })
})

describe('colorFromPicker', () => {
  it('drops the colour when the picker is cleared', () => {
    expect(colorFromPicker('transparent')).toBeUndefined()
    expect(colorFromPicker(undefined)).toBeUndefined()
    expect(colorFromPicker('#123456')).toBe('#123456')
  })
})

describe('headingFieldDiffers and headingSetCount', () => {
  const base = defaultHeadingStyles()[1]

  it('reads numbers and pixel lengths by their value, and a colour by its hex', () => {
    expect(headingFieldDiffers({ ...base, marginTop: '0' }, base, 'marginTop')).toBe(false)
    expect(headingFieldDiffers({ ...base, color: '#FFF' }, { ...base, color: '#ffffff' }, 'color')).toBe(false)
    expect(headingFieldDiffers({ ...base, fontWeight: 'BOLD' }, base, 'fontWeight')).toBe(false)
    expect(headingFieldDiffers({ ...base, lineHeight: '1.30' }, base, 'lineHeight')).toBe(false)
    expect(headingFieldDiffers({ ...base, fontSize: '28.0px' }, base, 'fontSize')).toBe(false)
    expect(headingFieldDiffers({ ...base, lineHeight: '1.3px' }, base, 'lineHeight')).toBe(true)
  })

  it('sees an own colour or font as a difference, the defaults having none', () => {
    expect(headingFieldDiffers({ ...base, color: '#000000' }, base, 'color')).toBe(true)
    expect(headingFieldDiffers({ ...base, fontFamily: 'Georgia' }, base, 'fontFamily')).toBe(true)
    expect(headingFieldDiffers({ ...base, fontSize: '29px' }, base, 'fontSize')).toBe(true)
  })

  it('counts the differing fields of every level', () => {
    const styles = defaultHeadingStyles()
    styles[1] = { ...styles[1], fontSize: '40px', marginTop: '12px' }
    styles[6] = { ...styles[6], color: '#000000' }
    expect(headingSetCount(writeHeadingStyles(email([]), styles))).toBe(3)
  })

  it('counts none for an email without heading styles, or with styles edited by hand', () => {
    expect(headingSetCount(email([]))).toBe(0)
    const tree = writeHeadingStyles(email([]), defaultHeadingStyles())
    const managed = tree.children![0].children!.find((child) => child.type === 'mj-style') as unknown as { content: string }
    managed.content = managed.content.replace('22px', '44px')
    expect(headingSetCount(tree)).toBe(0)
  })
})
