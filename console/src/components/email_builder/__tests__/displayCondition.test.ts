import { readFileSync } from 'fs'
import { resolve } from 'path'
import { describe, expect, it } from 'vitest'
import {
  collectVariableSuggestions,
  conditionErrorsInTree,
  countHiddenBlocks,
  evaluateDisplayCondition,
  formatGoFloat,
  isConditionEligible,
  resolveVariable,
  trimGoSpace,
  validateCondition
} from '../displayCondition'
import type { DisplayCondition, EmailBlock } from '../types'

// The cases sending is checked against with real liquidgo in
// pkg/notifuse_mjml/display_condition_test.go: the editor must decide each one alike.
interface Fixture {
  evaluate: {
    name: string
    data: unknown
    condition: DisplayCondition
    expected: boolean
  }[]
  validate: { name: string; condition: DisplayCondition; valid: boolean }[]
}

const fixture: Fixture = JSON.parse(
  readFileSync(
    resolve(__dirname, '../../../../../pkg/notifuse_mjml/testdata/display_conditions.json'),
    'utf8'
  )
)

describe('display condition fixture', () => {
  it('holds evaluation and validation cases', () => {
    expect(fixture.evaluate.length).toBeGreaterThan(0)
    expect(fixture.validate.length).toBeGreaterThan(0)
  })

  it('pins every operator both ways', () => {
    const outcomes = new Map<string, Set<boolean>>()
    for (const c of fixture.evaluate) {
      if (c.condition.rules.length !== 1) continue
      const op = c.condition.rules[0].operator
      if (!outcomes.has(op)) outcomes.set(op, new Set())
      outcomes.get(op)!.add(c.expected)
    }
    for (const op of [
      'is_true',
      'is_blank',
      'is_present',
      'is_absent',
      'eq',
      'neq',
      'contains',
      'not_contains',
      'gt',
      'gte',
      'lt',
      'lte'
    ]) {
      expect(outcomes.get(op), op).toEqual(new Set([true, false]))
    }
  })

  it.each(fixture.evaluate.map((c) => [c.name, c] as const))('evaluates %s', (_name, c) => {
    expect(evaluateDisplayCondition(c.condition, c.data)).toBe(c.expected)
  })

  it.each(fixture.validate.map((c) => [c.name, c] as const))('validates %s', (_name, c) => {
    expect(validateCondition(c.condition) === null).toBe(c.valid)
  })
})

describe('Go helpers', () => {
  it('formats floats as Go does', () => {
    expect(formatGoFloat(1e-7)).toBe('0.0000001')
    expect(formatGoFloat(1e21)).toBe('1000000000000000000000')
    expect(formatGoFloat(-1.5)).toBe('-1.5')
    expect(formatGoFloat(100)).toBe('100')
    expect(formatGoFloat(0.30000000000000004)).toBe('0.30000000000000004')
  })
  it('trims the spaces Go trims', () => {
    expect(trimGoSpace('\u0085  x　')).toBe('x')
    expect(trimGoSpace('﻿x​')).toBe('﻿x​')
  })
  it('resolves only own properties', () => {
    expect(resolveVariable({ a: {} }, 'a.constructor')).toBeUndefined()
    expect(resolveVariable({ a: [{ b: 1 }] }, 'a.0.b')).toBe(1)
  })
})

const tree = (head: EmailBlock[], body: EmailBlock[]): EmailBlock =>
  ({
    id: 'root',
    type: 'mjml',
    children: [
      { id: 'head', type: 'mj-head', children: head },
      { id: 'body', type: 'mj-body', children: body }
    ]
  }) as EmailBlock

const vip: DisplayCondition = {
  match: 'all',
  rules: [{ variable: 'contact.vip', operator: 'is_true' }]
}

describe('eligibility', () => {
  const defaults = { id: 'defaults-text', type: 'mj-text' } as EmailBlock
  const image = { id: 'img', type: 'mj-image' } as EmailBlock
  const column = {
    id: 'col',
    type: 'mj-column',
    children: [image]
  } as EmailBlock
  const t = tree(
    [
      {
        id: 'attrs',
        type: 'mj-attributes',
        children: [defaults]
      } as EmailBlock
    ],
    [{ id: 'sec', type: 'mj-section', children: [column] } as EmailBlock]
  )

  it('accepts content blocks under mj-body only', () => {
    expect(isConditionEligible(t, image)).toBe(true)
    expect(isConditionEligible(t, column)).toBe(false)
    expect(isConditionEligible(t, defaults)).toBe(false)
  })

  it('lists the conditions the server would refuse', () => {
    const bad = tree(
      [{ id: 'raw-1', type: 'mj-raw', display_condition: vip } as EmailBlock],
      [
        {
          id: 'col-1',
          type: 'mj-column',
          display_condition: vip
        } as EmailBlock,
        {
          id: 'txt-1',
          type: 'mj-text',
          display_condition: { match: 'all', rules: [] }
        } as EmailBlock,
        { id: 'ok', type: 'mj-text', display_condition: vip } as EmailBlock
      ]
    )
    const errors = conditionErrorsInTree(bad)
    expect(errors).toHaveLength(3)
    expect(errors.join('\n')).toContain('raw-1')
    expect(errors.join('\n')).toContain('col-1')
    expect(errors.join('\n')).toContain('txt-1')
  })
})

describe('countHiddenBlocks', () => {
  it('counts a hidden section once', () => {
    const t = tree(
      [],
      [
        {
          id: 'sec',
          type: 'mj-section',
          display_condition: vip,
          children: [
            {
              id: 'col',
              type: 'mj-column',
              children: [{ id: 't', type: 'mj-text', display_condition: vip }]
            }
          ]
        } as EmailBlock,
        { id: 'img', type: 'mj-image', display_condition: vip } as EmailBlock
      ]
    )
    expect(countHiddenBlocks(t, { contact: { vip: false } })).toBe(2)
    expect(countHiddenBlocks(t, { contact: { vip: true } })).toBe(0)
  })
})

describe('collectVariableSuggestions', () => {
  it('lists nested paths with samples, then the contact variables', () => {
    const suggestions = collectVariableSuggestions({
      order: { total: 42, items: [{ sku: 'A1' }], customer: { vip: true } }
    })
    const byName = Object.fromEntries(suggestions.map((s) => [s.variable, s.sample]))
    expect(byName['order.total']).toBe('42')
    expect(byName['order.customer.vip']).toBe('true')
    expect(byName['order.items.size']).toBe('1')
    expect(byName['order.items.first.sku']).toBe('A1')
    expect(Object.keys(byName)).toContain('contact.first_name')
  })
})
