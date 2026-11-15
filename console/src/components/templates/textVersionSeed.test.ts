import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { customTextOrUndefined, missingRequiredLink, seedTextVersion } from './textVersionSeed'
import { templatesApi } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'

vi.mock('../../services/api/template', () => ({ templatesApi: { compile: vi.fn() } }))

describe('customTextOrUndefined', () => {
  it('keeps a text as written', () => {
    expect(customTextOrUndefined('  Hi\n')).toBe('  Hi\n')
  })

  it('treats an absent or blank text as the automatic version', () => {
    expect(customTextOrUndefined(undefined)).toBeUndefined()
    expect(customTextOrUndefined(null)).toBeUndefined()
    expect(customTextOrUndefined(' \n\t')).toBeUndefined()
  })
})

describe('missingRequiredLink', () => {
  it('names the unsubscribe link a marketing text misses', () => {
    expect(missingRequiredLink('marketing', 'Hi')).toBe('unsubscribe_url')
    expect(missingRequiredLink('marketing', 'Bye: {{ unsubscribe_url }}')).toBeUndefined()
  })

  it('names the confirmation link an opt-in text misses', () => {
    expect(missingRequiredLink('opt_in', 'Hi')).toBe('confirm_subscription_url')
    expect(missingRequiredLink('opt_in', '{{confirm_subscription_url}}')).toBeUndefined()
  })

  it('asks nothing of other categories, nor of the automatic version', () => {
    expect(missingRequiredLink('transactional', 'Hi')).toBeUndefined()
    expect(missingRequiredLink(undefined, 'Hi')).toBeUndefined()
    expect(missingRequiredLink('marketing', undefined)).toBeUndefined()
    expect(missingRequiredLink('marketing', '  ')).toBeUndefined()
  })
})

describe('seedTextVersion', () => {
  const workspace = { id: 'ws1' } as Workspace
  const seed = (response: Record<string, unknown>) => {
    ;(templatesApi.compile as Mock).mockResolvedValue(response)
    return seedTextVersion(workspace, { mjmlSource: '<mjml></mjml>' })
  }
  const body = (content: string) => `<mjml><mj-body>${content}</mj-body></mjml>`

  beforeEach(() => vi.clearAllMocks())

  it('says Liquid was lost when a display condition wraps a block', async () => {
    // As pkg/notifuse_mjml/display_condition.go writes it: the tags sit between the sections
    const mjml = body(
      '{% assign c_ok = false %}{% if vip %}{% assign c_ok = true %}{% endif %}{% if c_ok %}<mj-section><mj-column><mj-text>VIP</mj-text></mj-column></mj-section>{% endif %}'
    )
    expect((await seed({ mjml, text: 'VIP\n' })).lostLiquid).toBe(true)
  })

  it('says nothing was lost when the tags are kept in the text', async () => {
    const mjml = body('<mj-section><mj-column><mj-text>{% if vip %}VIP{% endif %}</mj-text></mj-column></mj-section>')
    expect((await seed({ mjml, text: '{% if vip %}VIP{% endif %}\n' })).lostLiquid).toBe(false)
  })

  it('does not count Liquid in attributes, in the head or in comments', async () => {
    const mjml =
      '<mjml><mj-head><mj-preview>{% if vip %}Hi VIP{% endif %}</mj-preview><mj-title>{% if a %}A{% endif %}</mj-title></mj-head>' +
      '<mj-body><!-- {% if old %} --><mj-section css-class="{% if vip %}vip{% endif %}"><mj-column>' +
      '<mj-button href="https://x.test/?a={% if b > 1 %}b{% endif %}">Go</mj-button></mj-column></mj-section></mj-body></mjml>'
    expect((await seed({ mjml, text: 'Go\n' })).lostLiquid).toBe(false)
  })

  it('tells the placement notice from markup that would be refused', async () => {
    const placement = await seed({
      mjml: body(''),
      text: '',
      text_error:
        'Liquid placed between the blocks of a section or a column may not land where it applies in the text version: check it before saving'
    })
    expect(placement.liquidPlacement).toBe(true)
    expect(placement.textError).toBeUndefined()

    const refused = await seed({ mjml: body(''), text: '{% if vip %}', text_error: "'if' tag was never closed" })
    expect(refused.liquidPlacement).toBe(false)
    expect(refused.textError).toBe("'if' tag was never closed")
  })
})
