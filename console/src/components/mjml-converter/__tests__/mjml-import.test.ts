import { readFileSync } from 'fs'
import { resolve } from 'path'
import { describe, expect, test } from 'vitest'
import type { EmailBlock } from '../../email_builder/types'
import { convertMjmlToJsonBrowser, importMjmlSource } from '../mjml-to-json-browser'
import { syntaxErrorLine } from '../../templates/importTemplateFile'

type Block = EmailBlock & { content?: string; display_condition?: unknown }

const inColumn = (blocks: string) =>
  `<mjml><mj-body><mj-section><mj-column>${blocks}</mj-column></mj-section></mj-body></mjml>`
const inBody = (blocks: string) => `<mjml><mj-body>${blocks}</mj-body></mjml>`
const SECTION = '<mj-section><mj-column><mj-text>x</mj-text></mj-column></mj-section>'

const bodyOf = (tree: EmailBlock) => tree.children?.find((c) => c.type === 'mj-body') as Block
const headOf = (tree: EmailBlock) => tree.children?.find((c) => c.type === 'mj-head') as Block
const columnOf = (tree: EmailBlock) => bodyOf(tree).children?.[0]?.children?.[0] as Block
const firstInColumn = (mjml: string) =>
  columnOf(convertMjmlToJsonBrowser(mjml)).children?.[0] as Block
const types = (block: EmailBlock | undefined) => block?.children?.map((c) => c.type)

// A JSON unicode escape (backslash, u, hex), built so no tool rewrites it.
const esc = (hex: string) => String.fromCharCode(92) + 'u' + hex

describe('MJML import reads what MJML reads', () => {
  test.each([
    ['a bare & in a text', '<p>Tom & Jerry</p>'],
    ['&& in Liquid', '<p>{% if a && b %}x{% endif %}</p>'],
    ['< in a Liquid condition', '<p>{% if cart.items.size < 3 %}few{% endif %}</p>'],
    ['a quoted < in Liquid output', '<p>{{ "a<b" }}</p>'],
    ['a < in prose', '<p>1 < 2</p>'],
    ['an unquoted HTML attribute', '<table border=0><tr><td>x</td></tr></table>'],
    ['an HTML attribute without value', '<table><tr><td nowrap>x</td></tr></table>'],
    ['unclosed paragraphs', '<p>one<p>two'],
    ['unclosed list items', '<ul><li>one<li>two</ul>'],
    ['an entity without semicolon', '<p>a&nbsp b &#39 c</p>'],
    ['an HTML5 entity', '<p>&minus;10% &frac12; &Eacute;</p>']
  ])('keeps %s exactly in mj-text', (_, content) => {
    const text = firstInColumn(inColumn(`<mj-text>${content}</mj-text>`))
    expect(text.type).toBe('mj-text')
    expect(text.content).toBe(content)
  })

  test('keeps the content of an mj-button and an mj-raw exactly', () => {
    const column = columnOf(
      convertMjmlToJsonBrowser(
        inColumn(
          '<mj-button href="#">Save & go <b>now</mj-button><mj-raw><div style="a">open</mj-raw>'
        )
      )
    )
    expect(column.children?.map((c) => (c as Block).content)).toEqual([
      'Save & go <b>now',
      '<div style="a">open'
    ])
  })

  test('keeps HTML opened in one mj-raw and closed in another', () => {
    const body = bodyOf(
      convertMjmlToJsonBrowser(
        inBody(`<mj-raw><div class="w"></mj-raw>${SECTION}<mj-raw></div></mj-raw>`)
      )
    )
    expect(types(body)).toEqual(['mj-raw', 'mj-section', 'mj-raw'])
    expect((body.children?.[2] as Block).content).toBe('</div>')
  })

  test('reads Liquid with quotes inside an attribute value', () => {
    const image = firstInColumn(inColumn(`<mj-image src="{{ item.image | default: "x.jpg" }}" />`))
    expect(image.attributes).toEqual({ src: '{{ item.image | default: "x.jpg" }}' })
  })

  test('reads a single-quoted attribute holding an &', () => {
    const button = firstInColumn(
      inColumn(`<mj-button href='https://x.com/?a=1&b=2'>Go</mj-button>`)
    )
    expect(button.attributes).toEqual({ href: 'https://x.com/?a=1&b=2' })
  })

  test('decodes the entities of attribute values', () => {
    const image = firstInColumn(
      inColumn('<mj-image src="https://x.com/?a=1&amp;b=2" alt="&#39;Q&#39; &minus;" />')
    )
    expect(image.attributes).toEqual({ src: 'https://x.com/?a=1&b=2', alt: "'Q' −" })
  })

  test('keeps the last value of a repeated attribute', () => {
    const tree = convertMjmlToJsonBrowser(
      inBody(
        '<mj-section background-color="#fff" background-color="#000"><mj-column /></mj-section>'
      )
    )
    expect(bodyOf(tree).children?.[0].attributes).toEqual({ backgroundColor: '#000' })
  })

  test('decodes what the export escapes in mj-title, mj-preview and mj-style', () => {
    const tree = convertMjmlToJsonBrowser(
      `<mjml><mj-head><mj-title>R&amp;D &#60;news&#62;</mj-title><mj-preview>{{ contact.first_name | default: 'Hi' }} &amp; welcome</mj-preview>` +
        `<mj-style>.a &#62; .b { color: red } .c::after { content: "&amp;" }</mj-style></mj-head><mj-body>${SECTION}</mj-body></mjml>`
    )
    expect(headOf(tree).children?.map((c) => (c as Block).content)).toEqual([
      'R&D <news>',
      "{{ contact.first_name | default: 'Hi' }} & welcome",
      '.a > .b { color: red } .c::after { content: "&" }'
    ])
  })

  test('keeps a CSS < in mj-style', () => {
    const tree = convertMjmlToJsonBrowser(
      `<mjml><mj-head><mj-style>/* a < b */ .x{color:red}</mj-style></mj-head><mj-body>${SECTION}</mj-body></mjml>`
    )
    expect((headOf(tree).children?.[0] as Block).content).toBe('/* a < b */ .x{color:red}')
  })

  test('still wraps a bare mj-text in a paragraph', () => {
    expect(firstInColumn(inColumn('<mj-text>Hello</mj-text>')).content).toBe('<p>Hello</p>')
  })

  test('reads self-closing blocks, mj-attributes and a leading declaration', () => {
    const tree = convertMjmlToJsonBrowser(
      `<?xml version="1.0"?><!-- c --><mjml><mj-head><mj-attributes><mj-text font-size="15px" /><mj-all font-family="Arial" /></mj-attributes></mj-head>` +
        `<mj-body><mj-section><mj-column><mj-spacer height="10px" /><mj-divider /><mj-text>Hi</mj-text></mj-column></mj-section></mj-body></mjml>`
    )
    expect(types(headOf(tree).children?.[0])).toEqual(['mj-text', 'mj-all'])
    expect(types(columnOf(tree))).toEqual(['mj-spacer', 'mj-divider', 'mj-text'])
  })
})

describe('MJML import errors', () => {
  const errorOf = (mjml: string) => {
    try {
      convertMjmlToJsonBrowser(mjml)
    } catch (error) {
      return (error as Error).message
    }
    throw new Error('expected an error')
  }

  test('names the line of an unclosed block', () => {
    const message = errorOf(
      '<mjml>\n<mj-body>\n<mj-section>\n<mj-column>\n<mj-text>x</mj-text>\n</mj-section>\n</mj-body>\n</mjml>'
    )
    expect(message).toContain('Invalid MJML syntax')
    expect(message).toContain('mj-column')
    expect(syntaxErrorLine(message)).toBe(4)
  })

  test('names the line of a closing tag that closes nothing', () => {
    const message = errorOf('<mjml>\n<mj-body>\n</mj-column>\n</mj-body>\n</mjml>')
    expect(syntaxErrorLine(message)).toBe(3)
  })

  test('refuses Liquid written inside a tag, with its line', () => {
    const message = errorOf(
      '<mjml>\n<mj-body>\n<mj-section {% if vip %}background-color="red"{% endif %}><mj-column /></mj-section>\n</mj-body>\n</mjml>'
    )
    expect(message).toContain('Liquid inside a tag')
    expect(syntaxErrorLine(message)).toBe(3)
  })

  test('refuses a file whose root is not mjml', () => {
    expect(errorOf('<html><body></body></html>')).toContain('Root element must be')
  })
})

// The marks the export writes (pkg/notifuse_mjml/liquid_markers.go), as it lays them out.
const LIQUID_OPEN = '{% comment %}notifuse:liquid{% endcomment %}'
const LIQUID_CLOSE = '{% comment %}/notifuse:liquid{% endcomment %}'
const conditionOpen = (json: string) => `{% comment %}notifuse:condition ${json}{% endcomment %}`
const CONDITION_CLOSE = '{% comment %}/notifuse:condition{% endcomment %}'
const COMPILED_OPEN =
  '{% assign __dc_img_1 = true %}{% assign __dc_ok_img_1 = false %}{% if order.total > 1 %}{% assign __dc_ok_img_1 = true %}{% endif %}{% if __dc_img_1 %}'

describe('MJML import with the export marks', () => {
  test('rebuilds an mj-liquid block with its exact content', () => {
    const content =
      '{% for item in items %}\n<mj-column><mj-text>{{ item.name }} < {{ item.max }}</mj-text></mj-column>\n{% endfor %}'
    const { tree, liquidBlocksKept } = importMjmlSource(
      inBody(`<mj-section>\n${LIQUID_OPEN}${content}${LIQUID_CLOSE}\n</mj-section>`)
    )
    const section = bodyOf(tree).children?.[0] as Block
    expect(types(section)).toEqual(['mj-liquid'])
    expect((section.children?.[0] as Block).content).toBe(content)
    expect(liquidBlocksKept).toBe(0)
  })

  test('gives a block back its display condition and drops the compiled Liquid', () => {
    const condition = {
      match: 'all',
      rules: [{ variable: 'order.total', operator: 'eq', value: `50% {%} </mj-text> R&D` }]
    }
    const json = JSON.stringify(condition)
      .replace(/%/g, esc('0025'))
      .replace(/</g, esc('003c'))
      .replace(/>/g, esc('003e'))
      .replace(/&/g, esc('0026'))
    const mjml = inColumn(
      `\n        <mj-spacer />\n        ${conditionOpen(json)}${COMPILED_OPEN}\n        <mj-image src="a.png" />\n        {% endif %}${CONDITION_CLOSE}\n      `
    )
    const { tree, liquidBlocksKept } = importMjmlSource(mjml)
    const column = columnOf(tree)
    expect(types(column)).toEqual(['mj-spacer', 'mj-image'])
    expect((column.children?.[1] as Block).display_condition).toEqual(condition)
    expect(liquidBlocksKept).toBe(0)
  })

  test('refuses an mj-liquid mark that is not closed', () => {
    expect(() => convertMjmlToJsonBrowser(inBody(`${LIQUID_OPEN}{% if a %}${SECTION}`))).toThrow(
      /notifuse:liquid/
    )
  })

  test('refuses a condition mark with no block after it', () => {
    expect(() =>
      convertMjmlToJsonBrowser(
        inBody(
          `${conditionOpen('{"match":"all","rules":[]}')}{% if x %}{% endif %}${CONDITION_CLOSE}`
        )
      )
    ).toThrow(/notifuse:condition/)
  })
})

describe('MJML import without marks keeps Liquid placed between blocks', () => {
  test('turns a Liquid if around a section into one Liquid block', () => {
    const source = `{% if n < 3 %}${SECTION}{% endif %}`
    const { tree, liquidBlocksKept } = importMjmlSource(inBody(`\n  ${source}\n  ${SECTION}\n`))
    const body = bodyOf(tree)
    expect(types(body)).toEqual(['mj-liquid', 'mj-section'])
    expect((body.children?.[0] as Block).content).toBe(source)
    expect(liquidBlocksKept).toBe(1)
  })

  test('keeps Liquid output placed between blocks', () => {
    const body = bodyOf(convertMjmlToJsonBrowser(inBody(`${SECTION}{{ footer_html }}`)))
    expect(types(body)).toEqual(['mj-section', 'mj-liquid'])
    expect((body.children?.[1] as Block).content).toBe('{{ footer_html }}')
  })

  test('takes in the section that reads a variable the Liquid assigns', () => {
    const totals =
      '<mj-section><mj-column><mj-text>{{ total | divided_by: 100 }}</mj-text></mj-column></mj-section>'
    const source = `{% assign total = 0 %}{% for i in items %}{% assign total = total | plus: i.price %}{% endfor %}\n${totals}`
    const body = bodyOf(convertMjmlToJsonBrowser(inBody(`${source}\n${SECTION}`)))
    expect(types(body)).toEqual(['mj-liquid', 'mj-section'])
    expect((body.children?.[0] as Block).content).toBe(source)
  })

  test('does not take in a section that only reads a longer name', () => {
    const other =
      '<mj-section><mj-column><mj-text>{{ cart.total }} {{ total_items }}</mj-text></mj-column></mj-section>'
    const body = bodyOf(convertMjmlToJsonBrowser(inBody(`{% assign total = 1 %}${other}`)))
    expect(types(body)).toEqual(['mj-liquid', 'mj-section'])
  })

  test('takes in the whole wrapper when a later block reads what it assigns', () => {
    const later = '<mj-section><mj-column><mj-text>{{ count }}</mj-text></mj-column></mj-section>'
    const wrapper = `<mj-wrapper>{% assign count = 2 %}${SECTION}</mj-wrapper>`
    const body = bodyOf(convertMjmlToJsonBrowser(inBody(`${wrapper}\n${later}\n${SECTION}`)))
    expect(types(body)).toEqual(['mj-liquid', 'mj-section'])
    expect((body.children?.[0] as Block).content).toBe(`${wrapper}\n${later}`)
  })

  test('takes in the group when Liquid sits between its columns, which mj-group cannot hold', () => {
    const group =
      '<mj-group>{% for c in cols %}<mj-column><mj-text>{{ c }}</mj-text></mj-column>{% endfor %}</mj-group>'
    const section = bodyOf(convertMjmlToJsonBrowser(inBody(`<mj-section>${group}</mj-section>`)))
      .children?.[0] as Block
    expect(types(section)).toEqual(['mj-liquid'])
    expect((section.children?.[0] as Block).content).toBe(group)
  })

  test('imports the abandoned cart exported by mjml_builder', () => {
    const source = readFileSync(resolve(__dirname, 'fixtures/cart-with-liquid.mjml'), 'utf8')
    const { tree, liquidBlocksKept } = importMjmlSource(source)
    expect(liquidBlocksKept).toBe(2)
    const wrapper = bodyOf(tree).children?.[1] as Block
    expect(types(bodyOf(tree))).toEqual(['mj-section', 'mj-wrapper', 'mj-section'])
    expect(types(wrapper)).toEqual([
      'mj-section',
      'mj-section',
      'mj-section',
      'mj-liquid',
      'mj-section',
      'mj-liquid',
      'mj-section',
      'mj-section'
    ])
    const discount = (wrapper.children?.[3] as Block).content ?? ''
    expect(discount.startsWith('{% assign __dc_cart_discount = true %}')).toBe(true)
    expect(discount.endsWith('{% endif %}')).toBe(true)
    const cart = (wrapper.children?.[5] as Block).content ?? ''
    expect(cart.startsWith('{% assign subtotal_cents = 0 %}')).toBe(true)
    expect(cart).toContain('{% for item in cart.items %}')
    expect(cart).toContain('(&minus;{{ cart.discount_percent }}%)')
    expect(cart.trimEnd().endsWith('</mj-section>')).toBe(true)
    expect(cart).toContain('Total</td>')
    expect(source).toContain(cart)
    const preview = headOf(tree).children?.find((c) => c.type === 'mj-preview') as Block
    expect(preview.content).toBe(
      "{{ contact.first_name | default: 'Hi' }}, we saved your cart until {{ cart.expires_at | date: '%A' }}. Pick up right where you left off."
    )
  })
})

describe('MJML import, review cases', () => {
  test('ignores blocks commented out with HTML comments', () => {
    const commented = '<!-- <mj-section><mj-column></mj-column></mj-section> -->'
    const { tree, liquidBlocksKept } = importMjmlSource(inBody(`${commented}${SECTION}`))
    expect(types(bodyOf(tree))).toEqual(['mj-section'])
    expect(liquidBlocksKept).toBe(0)
  })

  test('keeps an HTML comment inside a text as written', () => {
    expect(firstInColumn(inColumn('<mj-text><!-- </mj-text> -->A</mj-text>')).content).toBe(
      '<!-- </mj-text> -->A'
    )
  })

  test('keeps an Outlook conditional comment in an mj-raw', () => {
    const raw = '<!--[if mso]><table><tr><td><![endif]-->'
    const body = bodyOf(convertMjmlToJsonBrowser(inBody(`<mj-raw>${raw}</mj-raw>${SECTION}`)))
    expect((body.children?.[0] as Block).content).toBe(raw)
  })

  test('does not count the tags of a Liquid comment as open', () => {
    const column = columnOf(
      convertMjmlToJsonBrowser(
        inColumn(
          '{% comment %}{% if old %}{% endcomment %}<mj-text>A</mj-text><mj-text>B</mj-text>'
        )
      )
    )
    expect(types(column)).toEqual(['mj-liquid', 'mj-text', 'mj-text'])
  })

  test('keeps a block whose children have Liquid between them in a Liquid block', () => {
    const social =
      '<mj-social>{% if contact.fb %}<mj-social-element name="facebook">FB</mj-social-element>{% endif %}</mj-social>'
    const { tree, liquidBlocksKept } = importMjmlSource(inColumn(`${social}<mj-text>A</mj-text>`))
    const column = columnOf(tree)
    expect(types(column)).toEqual(['mj-liquid', 'mj-text'])
    expect((column.children?.[0] as Block).content).toBe(social)
    expect(liquidBlocksKept).toBe(1)
  })

  test('refuses Liquid between the blocks of mj-head, which no Liquid block can hold', () => {
    expect(() =>
      convertMjmlToJsonBrowser(
        `<mjml><mj-head>{% if a %}<mj-title>T</mj-title>{% endif %}</mj-head><mj-body>${SECTION}</mj-body></mjml>`
      )
    ).toThrow(/mj-head/)
  })

  test('drops plain text between blocks, as it holds no Liquid', () => {
    const { tree, liquidBlocksKept } = importMjmlSource(inColumn('hello<mj-text>A</mj-text>'))
    expect(types(columnOf(tree))).toEqual(['mj-text'])
    expect(liquidBlocksKept).toBe(0)
  })

  test('does not wrap a text starting with Liquid around a paragraph in another paragraph', () => {
    expect(
      firstInColumn(inColumn('<mj-text>{% if a %}<p>x</p>{% endif %}</mj-text>')).content
    ).toBe('{% if a %}<p>x</p>{% endif %}')
  })
})

describe('MJML import, second review cases', () => {
  test('decodes the Liquid of an attribute, which the export escapes with the value', () => {
    const button = firstInColumn(
      inColumn('<mj-button href="{{ url | append: &#39;?a=1&amp;b=2&#39; }}">Go</mj-button>')
    )
    expect(button.attributes).toEqual({ href: "{{ url | append: '?a=1&b=2' }}" })
  })

  test('decodes the Liquid of an mj-preview, which the export escapes with the content', () => {
    const tree = convertMjmlToJsonBrowser(
      `<mjml><mj-head><mj-preview>{% if n &#62; 1 %}Many{% endif %} &amp; more</mj-preview></mj-head><mj-body>${SECTION}</mj-body></mjml>`
    )
    expect((headOf(tree).children?.[0] as Block).content).toBe(
      '{% if n > 1 %}Many{% endif %} & more'
    )
  })

  test('refuses Liquid written outside the mjml root', () => {
    expect(() => convertMjmlToJsonBrowser(`{% assign brand = "ACME" %}${inBody(SECTION)}`)).toThrow(
      /outside <mjml>/
    )
  })

  test('refuses a second root', () => {
    expect(() => convertMjmlToJsonBrowser(`${inBody(SECTION)}<mjml></mjml>`)).toThrow(
      /outside <mjml>/
    )
  })

  test('does not parse the content of a marked Liquid block', () => {
    const content =
      '<mj-section {% if vip %}background-color="#fc0"{% endif %}><mj-column><mj-text>Hi</mj-text></mj-column>'
    const body = bodyOf(
      convertMjmlToJsonBrowser(
        inBody(
          `${LIQUID_OPEN}${content}${LIQUID_CLOSE}${SECTION}${LIQUID_OPEN}</mj-section>${LIQUID_CLOSE}`
        )
      )
    )
    expect(types(body)).toEqual(['mj-liquid', 'mj-section', 'mj-liquid'])
    expect((body.children?.[0] as Block).content).toBe(content)
    expect((body.children?.[2] as Block).content).toBe('</mj-section>')
  })

  test('drops the control characters the import sets aside with, rather than hang', () => {
    const stx = String.fromCharCode(2)
    const soh = String.fromCharCode(1)
    const text = firstInColumn(inColumn(`<mj-text>a${stx}b${soh}1${soh}c</mj-text>`))
    expect(text.content).toBe('<p>ab1c</p>')
  })
})
