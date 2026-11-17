import { describe, expect, it } from 'vitest'
import type { WebFilter, WebFilterOperation } from '../../../services/api/web_analytics'
import { draftOf, type FilterDraft } from './ruleDraft'
import { DRAFT_RULE_ID, ruleNeighbours, ruleOverrides } from './ruleOrder'

const rule = (id: string, priority: number, operations: WebFilter['operations'], enabled = true): WebFilter => ({
  id,
  name: id,
  priority,
  order: 0,
  conditions: [],
  operations,
  enabled
})
const sets = (dimension: string): WebFilter['operations'] => [{ dimension, action: 'set_value', value: 'x' }]

const draft = (priority: number, operations: WebFilter['operations']): FilterDraft => ({
  ...draftOf(),
  name: 'This',
  priority,
  operations
})

/** The neighbours of a draft that has a priority. */
const placed = (...args: Parameters<typeof ruleNeighbours>) => {
  const order = ruleNeighbours(...args)
  if (!order) throw new Error('the draft has no priority')
  return order
}

describe('ruleNeighbours', () => {
  it('orders by priority, keeps list order between equals, and sorts a new rule after its equals', () => {
    const rules = [rule('a', 500, sets('channel')), rule('b', 900, sets('channel')), rule('c', 500, sets('channel'))]
    const order = placed(rules, draft(500, sets('channel')))
    expect(order.all.map((r) => r.id)).toEqual(['b', 'a', 'c', DRAFT_RULE_ID])
    expect(order.thisId).toBe(DRAFT_RULE_ID)
  })

  it('places an edited rule where its new priority puts it', () => {
    const rules = [rule('a', 900, sets('channel')), rule('b', 500, sets('channel')), rule('c', 100, sets('channel'))]
    expect(placed(rules, draft(500, sets('channel')), 'b').all.map((r) => r.id)).toEqual(['a', 'b', 'c'])
    expect(placed(rules, draft(950, sets('channel')), 'b').all.map((r) => r.id)).toEqual(['b', 'a', 'c'])
  })

  it('keeps only the rules writing a dimension this one writes, disabled ones included', () => {
    const rules = [
      rule('channel', 900, sets('channel')),
      rule('custom', 800, sets('custom_1')),
      rule('paused', 700, sets('channel_group'), false)
    ]
    const order = placed(rules, draft(500, [...sets('channel'), ...sets('channel_group')]))
    expect(order.all.map((r) => r.id)).toEqual(['channel', 'paused', DRAFT_RULE_ID])
  })

  it('shows two rules either side, and counts every rule above and below', () => {
    const rules = [900, 800, 700, 600, 400, 300, 200].map((priority) => rule(`p${priority}`, priority, sets('channel')))
    const order = placed(rules, draft(500, sets('channel')))
    expect(order.rows.map((r) => r.id)).toEqual(['p700', 'p600', DRAFT_RULE_ID, 'p400', 'p300'])
    expect(order.above).toBe(4)
    expect(order.below).toBe(3)
  })
})

type Action = WebFilterOperation['action']
type Place = 'higher' | 'equalBefore' | 'equalAfter' | 'lower'

const op = (action: Action, value = 'x'): WebFilterOperation =>
  action === 'unset_value' ? { dimension: 'channel', action } : { dimension: 'channel', action, value }

/**
 * Two rules writing one dimension, both matching a session: what EvaluateWebFilters
 * (internal/domain/web_analytics_filters.go) leaves there, run as a Go table over every action
 * pair and place. `before`: the other rule runs first and this rule's value is never written;
 * `after`: it runs later and replaces this rule's value; null: this rule's value stands. This rule
 * has priority 500; the other has 900 (higher), 500 above it in the list (equalBefore), 500 below
 * it (equalAfter) or 100 (lower).
 */
const ENGINE_CASES: Array<[Action, Action, Place, 'before' | 'after' | null]> = [
  // A set claims the dimension: only a set or unset of higher priority run first blocks it, and
  // only one of equal priority run later replaces it. A default never touches it.
  ['set_value', 'set_value', 'higher', 'before'],
  ['set_value', 'set_value', 'equalBefore', null],
  ['set_value', 'set_value', 'equalAfter', 'after'],
  ['set_value', 'set_value', 'lower', null],
  ['set_value', 'unset_value', 'higher', 'before'],
  ['set_value', 'unset_value', 'equalBefore', null],
  ['set_value', 'unset_value', 'equalAfter', 'after'],
  ['set_value', 'unset_value', 'lower', null],
  ['set_value', 'set_default_value', 'higher', null],
  ['set_value', 'set_default_value', 'equalBefore', null],
  ['set_value', 'set_default_value', 'equalAfter', null],
  ['set_value', 'set_default_value', 'lower', null],
  // An unset is a set to null: a later default fills it back ("Explicit unset (null) → default
  // fills it back"), and another unset leaves the same null.
  ['unset_value', 'set_value', 'higher', 'before'],
  ['unset_value', 'set_value', 'equalBefore', null],
  ['unset_value', 'set_value', 'equalAfter', 'after'],
  ['unset_value', 'set_value', 'lower', null],
  ['unset_value', 'unset_value', 'higher', null],
  ['unset_value', 'unset_value', 'equalBefore', null],
  ['unset_value', 'unset_value', 'equalAfter', null],
  ['unset_value', 'unset_value', 'lower', null],
  ['unset_value', 'set_default_value', 'higher', null],
  ['unset_value', 'set_default_value', 'equalBefore', null],
  ['unset_value', 'set_default_value', 'equalAfter', 'after'],
  ['unset_value', 'set_default_value', 'lower', 'after'],
  // A default claims nothing: anything run first that leaves a value pre-empts it ("Higher-priority
  // set_value ran first → default does not overwrite"), an unset run first does not, and any set or
  // unset run later replaces it ("later set_value still overrides").
  ['set_default_value', 'set_value', 'higher', 'before'],
  ['set_default_value', 'set_value', 'equalBefore', 'before'],
  ['set_default_value', 'set_value', 'equalAfter', 'after'],
  ['set_default_value', 'set_value', 'lower', 'after'],
  ['set_default_value', 'unset_value', 'higher', null],
  ['set_default_value', 'unset_value', 'equalBefore', null],
  ['set_default_value', 'unset_value', 'equalAfter', 'after'],
  ['set_default_value', 'unset_value', 'lower', 'after'],
  ['set_default_value', 'set_default_value', 'higher', 'before'],
  ['set_default_value', 'set_default_value', 'equalBefore', 'before'],
  ['set_default_value', 'set_default_value', 'equalAfter', null],
  ['set_default_value', 'set_default_value', 'lower', null]
]

describe('ruleOverrides', () => {
  it.each(ENGINE_CASES)('this rule %s, the other %s, %s: %s', (mine, theirs, place, expected) => {
    const other = rule('other', place === 'higher' ? 900 : place === 'lower' ? 100 : 500, [op(theirs)])
    const edited = rule('edited', 500, [op(mine)])
    const rules = place === 'equalBefore' ? [other, edited] : [edited, other]
    const result = ruleOverrides(rules, draft(500, [op(mine)]), 'edited')
    if (result.kind !== 'runs') throw new Error(`unexpected ${result.kind}`)
    expect({
      before: result.before.map((r) => r.id),
      after: result.after.map((r) => r.id)
    }).toEqual({
      before: expected === 'before' ? ['other'] : [],
      after: expected === 'after' ? ['other'] : []
    })
    expect(result.beforeDimensions).toEqual(expected === 'before' ? ['channel'] : [])
    expect(result.afterDimensions).toEqual(expected === 'after' ? ['channel'] : [])
  })

  it('lists each group in engine order, and ignores a disabled rule and another dimension', () => {
    const rules = [
      rule('higher', 900, sets('channel')),
      rule('higherUnset', 800, [{ dimension: 'channel', action: 'unset_value' }]),
      rule('paused', 950, sets('channel'), false),
      rule('elsewhere', 900, sets('custom_1')),
      rule('lower', 100, sets('channel'))
    ]
    const result = ruleOverrides(rules, draft(500, sets('channel')))
    expect(result).toMatchObject({ kind: 'runs', after: [], beforeDimensions: ['channel'] })
    if (result.kind === 'runs') expect(result.before.map((r) => r.id)).toEqual(['higher', 'higherUnset'])
  })

  it('places a new rule after its equals, so an equal set already saved runs first and loses', () => {
    const result = ruleOverrides([rule('saved', 500, sets('channel'))], draft(500, sets('channel')))
    expect(result).toMatchObject({ kind: 'runs', before: [], after: [] })
  })

  it('says a disabled draft does not run, rather than what would override it', () => {
    const rules = [rule('higher', 900, sets('channel'))]
    expect(ruleOverrides(rules, { ...draft(500, sets('channel')), enabled: false })).toEqual({ kind: 'disabled' })
  })

  it('computes nothing while the priority is cleared', () => {
    const cleared = { ...draft(500, sets('channel')), priority: null as unknown as number }
    expect(ruleOverrides([rule('higher', 900, sets('channel'))], cleared)).toEqual({ kind: 'no_priority' })
    expect(ruleNeighbours([rule('higher', 900, sets('channel'))], cleared)).toBeNull()
  })
})
