import { describe, it, expect } from 'vitest'
import { i18n } from '@lingui/core'
import { importTemplateFile } from './importTemplateFile'

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

  it('warns that display conditions are not imported', async () => {
    const result = await importTemplateFile(
      file(
        mjml(
          '{% assign __dc_s1 = true %}{% if contact.vip == true %}{% endif %}{% if __dc_s1 %}',
          '{% endif %}'
        ),
        'export.mjml'
      ),
      'mjml',
      i18n
    )
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.warnings).toHaveLength(1)
    expect(result.warnings[0].title).toBe('Display conditions not imported')
  })

  it('refuses markup that is not MJML', async () => {
    const result = await importTemplateFile(file('<html></html>', 'page.mjml'), 'mjml', i18n)
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.title).toBe('MJML Syntax Error')
  })
})
