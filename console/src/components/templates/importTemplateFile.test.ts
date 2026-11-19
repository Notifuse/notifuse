import { describe, it, expect } from 'vitest'
import { i18n } from '@lingui/core'
import { importTemplateFile, syntaxErrorLine } from './importTemplateFile'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const validTree = {
  id: 'root',
  type: 'mjml',
  children: [
    {
      id: 'body',
      type: 'mj-body',
      children: [
        {
          id: 'sec',
          type: 'mj-section',
          children: [{ id: 'col', type: 'mj-column', children: [] }]
        }
      ]
    }
  ]
}

const file = (content: string, name: string, type = 'text/plain') =>
  new File([content], name, { type })

describe('importTemplateFile, JSON', () => {
  it('reads a full export with its test data', async () => {
    const result = await importTemplateFile(
      file(JSON.stringify({ emailTree: validTree, testData: { a: 1 } }), 'welcome.json'),
      'json',
      i18n
    )
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.tree.id).toBe('root')
    expect(result.testData).toEqual({ a: 1 })
    expect(result.warnings).toEqual([])
  })

  it('reads a bare tree', async () => {
    const result = await importTemplateFile(
      file(JSON.stringify(validTree), 'tree.json'),
      'json',
      i18n
    )
    expect(result.ok && result.tree.type).toBe('mjml')
  })

  it('refuses a file that is not JSON, without a list of errors', async () => {
    const result = await importTemplateFile(file('{not json', 'broken.json'), 'json', i18n)
    expect(result).toEqual({
      ok: false,
      errors: ['Failed to import template. Please check the file format.']
    })
  })

  it('refuses a root that is not mjml with the current message', async () => {
    const result = await importTemplateFile(
      file(JSON.stringify({ id: 'x', type: 'mj-body' }), 'x.json'),
      'json',
      i18n
    )
    expect(result).toEqual({
      ok: false,
      title: 'Import Validation Failed',
      errors: ['Invalid tree structure: root must be mjml type']
    })
  })

  it('refuses a condition the server would refuse', async () => {
    const bad = structuredClone(validTree)
    Object.assign(bad.children[0].children[0].children[0], {
      display_condition: { match: 'all', rules: [{ variable: 'a', operator: 'is_true' }] }
    })
    const result = await importTemplateFile(file(JSON.stringify(bad), 'bad.json'), 'json', i18n)
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.errors.join('\n')).toMatch(/col \(mj-column\) cannot have a display condition/)
  })
})

describe('importTemplateFile, MJML', () => {
  const mjml = (before: string, after = '') =>
    `<mjml><mj-body>${before}<mj-section><mj-column><mj-text>Hi</mj-text></mj-column></mj-section>${after}</mj-body></mjml>`

  it('converts MJML to a visual tree', async () => {
    const result = await importTemplateFile(file(mjml(''), 'news.mjml'), 'mjml', i18n)
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.tree.type).toBe('mjml')
    expect(result.testData).toBeUndefined()
    expect(result.warnings).toEqual([])
  })

  it('keeps Liquid written between blocks, and says how many passages became Liquid blocks', async () => {
    const result = await importTemplateFile(
      file(mjml('{% if contact.vip %}', '{% endif %}{{ footer }}'), 'export.mjml'),
      'mjml',
      i18n
    )
    expect(result.ok).toBe(true)
    if (!result.ok) return
    const body = result.tree.children?.[0]
    expect(body?.children?.map((child) => child.type)).toEqual(['mj-liquid'])
    expect(result.warnings).toEqual([
      {
        title: 'Liquid kept in Liquid blocks',
        content:
          '1 passage written between blocks was kept as a Liquid block: its Liquid is unchanged and it is edited as code.'
      }
    ])
  })

  it('names the count of passages in the plural', async () => {
    const result = await importTemplateFile(
      file(mjml('{% if a %}', '{% endif %}<mj-section><mj-column /></mj-section>{{ footer }}'), 'export.mjml'),
      'mjml',
      i18n
    )
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.warnings[0].content).toBe(
      '2 passages written between blocks were kept as Liquid blocks: their Liquid is unchanged and they are edited as code.'
    )
  })

  it('gives back the blocks and display conditions the export marked, with no warning', async () => {
    const condition = '{"match":"all","rules":[{"variable":"contact.vip","operator":"is_true"}]}'
    const marked =
      '<mjml><mj-body><mj-section><mj-column>' +
      `{% comment %}notifuse:condition ${condition}{% endcomment %}{% assign __dc_t = true %}{% if __dc_t %}<mj-text>VIP</mj-text>{% endif %}{% comment %}/notifuse:condition{% endcomment %}` +
      '{% comment %}notifuse:liquid{% endcomment %}{% if a %}<mj-text>A</mj-text>{% endif %}{% comment %}/notifuse:liquid{% endcomment %}' +
      '</mj-column></mj-section></mj-body></mjml>'
    const result = await importTemplateFile(file(marked, 'export.mjml'), 'mjml', i18n)
    expect(result.ok).toBe(true)
    if (!result.ok) return
    const column = result.tree.children?.[0]?.children?.[0]?.children?.[0]
    expect(column?.children?.map((child) => child.type)).toEqual(['mj-text', 'mj-liquid'])
    expect((column?.children?.[0] as { display_condition?: unknown }).display_condition).toEqual(JSON.parse(condition))
    expect(result.warnings).toEqual([])
  })

  it('refuses markup that is not MJML', async () => {
    const result = await importTemplateFile(file('<html></html>', 'page.mjml'), 'mjml', i18n)
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.title).toBe('MJML Syntax Error')
  })

  it('returns the file and the line of an MJML syntax error, for the excerpt', async () => {
    const broken = [
      '<mjml>',
      '  <mj-body>',
      '    <mj-section>',
      '      <mj-column>',
      '        <mj-text>Hi',
      '      </mj-column>',
      '    </mj-section>',
      '  </mj-body>',
      '</mjml>',
      ''
    ].join('\n')
    const result = await importTemplateFile(file(broken, 'broken.mjml'), 'mjml', i18n)
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.title).toBe('MJML Syntax Error')
    expect(result.source).toBe(broken)
    // The line where the unclosed <mj-text> opens
    expect(result.line).toBe(5)
    expect(result.errors).toEqual(['<mj-text> is not closed.'])
  })
})

describe('importTemplateFile, MJML import errors', () => {
  it.each([
    ['<mjml><mj-body><mj-section {% if a %}x="1"{% endif %}></mj-section></mj-body></mjml>', 'Liquid inside a tag cannot be imported into the visual editor.'],
    ['<mjml><mj-body></mj-column></mj-body></mjml>', '</mj-column> closes no open tag.'],
    ['{% assign a = 1 %}<mjml><mj-body></mj-body></mjml>', 'Liquid or markup outside <mjml> cannot be imported.'],
    ['<mjml><mj-head>{% if a %}<mj-title>T</mj-title>{% endif %}</mj-head><mj-body></mj-body></mjml>', 'Liquid between the blocks of <mj-head> cannot be imported into the visual editor.'],
    ['<mjml><mj-body>{% comment %}notifuse:liquid{% endcomment %}{% if a %}</mj-body></mjml>', 'This notifuse:liquid mark of a Notifuse export is not closed.']
  ])('says what is wrong with %s, on its line', async (content, message) => {
    const result = await importTemplateFile(file(content, 'x.mjml'), 'mjml', i18n)
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.title).toBe('MJML Syntax Error')
    expect(result.errors).toEqual([message])
    expect(result.line).toBe(1)
  })
})

describe('syntaxErrorLine', () => {
  it('reads the first line libxml2 names, not the opening tag it names after it', () => {
    expect(
      syntaxErrorLine(
        'Invalid MJML syntax: This page contains the following errors:error on line 6 at column 19: Opening and ending tag mismatch: mj-text line 5 and mj-column'
      )
    ).toBe(6)
  })

  it('reads the line Firefox names', () => {
    expect(syntaxErrorLine('Invalid MJML syntax: XML Parsing Error: mismatched tag\nLine Number 6, Column 3:')).toBe(6)
  })

  it('reads the line jsdom puts first', () => {
    expect(
      syntaxErrorLine('Failed to convert MJML to JSON: Error: Invalid MJML syntax: 6:12: unexpected close tag.')
    ).toBe(6)
  })

  it('finds no line in an error that names none', () => {
    expect(syntaxErrorLine('Invalid MJML syntax: Root element must be <mjml>')).toBeUndefined()
  })
})
