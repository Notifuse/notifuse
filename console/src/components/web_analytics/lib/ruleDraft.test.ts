import { describe, expect, it } from 'vitest'
import type { WebFilter } from '../../../services/api/web_analytics'
import { draftOf, regexProblems, ruleDraftProblems, sameRuleDraft, type FilterDraft } from './ruleDraft'

const SAVED: WebFilter = {
  id: 'r1',
  name: 'Newsletter traffic',
  priority: 10,
  order: 1,
  conditions: [
    { field: 'utm_source', operator: 'equals', value: 'newsletter' },
    { field: 'utm_term', operator: 'is_empty' }
  ],
  operations: [
    { dimension: 'channel', action: 'set_value', value: 'Email' },
    { dimension: 'utm_content', action: 'unset_value' }
  ],
  enabled: true
}

describe('draftOf', () => {
  it('reads a saved rule, a missing tag list as []', () => {
    expect(draftOf(SAVED)).toEqual({
      name: 'Newsletter traffic',
      priority: 10,
      tags: [],
      enabled: true,
      conditions: SAVED.conditions,
      operations: SAVED.operations
    })
  })

  it('starts a new rule with one empty condition and one empty operation', () => {
    const draft = draftOf()
    expect(draft.priority).toBe(500)
    expect(draft.conditions).toEqual([{ field: 'utm_source', operator: 'equals', value: '' }])
    expect(draft.operations).toEqual([{ dimension: 'channel', action: 'set_value', value: '' }])
  })
})

describe('sameRuleDraft', () => {
  const base = draftOf(SAVED)

  it('treats an absent and an empty value as the same where the operator or action takes none', () => {
    const edited: FilterDraft = {
      ...base,
      conditions: [base.conditions[0], { field: 'utm_term', operator: 'is_empty', value: '' }],
      operations: [base.operations[0], { dimension: 'utm_content', action: 'unset_value', value: '' }]
    }
    expect(sameRuleDraft(edited, base)).toBe(true)
  })

  it('ignores spaces around the name and an absent tag list', () => {
    expect(sameRuleDraft({ ...base, name: '  Newsletter traffic ' }, base)).toBe(true)
    expect(sameRuleDraft({ ...base, tags: undefined } as unknown as FilterDraft, base)).toBe(true)
  })

  it('sees a changed tag, priority, value or a cleared priority', () => {
    expect(sameRuleDraft({ ...base, tags: ['email'] }, base)).toBe(false)
    expect(sameRuleDraft({ ...base, priority: 11 }, base)).toBe(false)
    expect(sameRuleDraft({ ...base, priority: null } as unknown as FilterDraft, base)).toBe(false)
    expect(
      sameRuleDraft({ ...base, conditions: [{ ...base.conditions[0], value: 'news' }, base.conditions[1]] }, base)
    ).toBe(false)
  })
})

describe('ruleDraftProblems', () => {
  it('accepts a rule with no condition, which always matches', () => {
    expect(ruleDraftProblems({ conditions: [], operations: SAVED.operations })).toEqual({
      conditions: null,
      operations: null
    })
  })

  it('flags a condition without a value, but not a valueless operator', () => {
    expect(
      ruleDraftProblems({
        conditions: [{ field: 'utm_source', operator: 'equals', value: '  ' }],
        operations: SAVED.operations
      }).conditions
    ).toBe('missing_value')
    expect(
      ruleDraftProblems({ conditions: [{ field: 'utm_term', operator: 'is_not_empty' }], operations: SAVED.operations })
        .conditions
    ).toBeNull()
  })

  it('needs one operation, and a value on each set or default', () => {
    expect(ruleDraftProblems({ conditions: [], operations: [] }).operations).toBe('none')
    expect(
      ruleDraftProblems({ conditions: [], operations: [{ dimension: 'channel', action: 'set_default_value', value: '' }] })
        .operations
    ).toBe('missing_value')
    expect(
      ruleDraftProblems({ conditions: [], operations: [{ dimension: 'channel', action: 'unset_value' }] }).operations
    ).toBeNull()
  })
})

describe('regexProblems', () => {
  const regex = (value: string) => ({ field: 'utm_source', operator: 'regex' as const, value })

  it('flags a pattern that does not compile', () => {
    expect(regexProblems([regex('^google$'), regex('([')])).toEqual([{ index: 1, kind: 'invalid' }])
  })

  it('flags lookarounds and backreferences, which RE2 refuses', () => {
    expect(regexProblems([regex('^(?!bing)'), regex('(?<=x)y'), regex('(a)\\1')]).map((p) => p.kind)).toEqual([
      'refused',
      'refused',
      'refused'
    ])
  })

  it('reads an escaped backslash before a digit as a backslash, not a backreference', () => {
    // \\1 in the pattern: a literal backslash, then 1.
    expect(regexProblems([regex('^a\\\\1$')])).toEqual([])
    // \12 is an octal code in RE2, \8 an invalid escape.
    expect(regexProblems([regex('a\\12')])).toEqual([])
    expect(regexProblems([regex('a\\8')]).map((p) => p.kind)).toEqual(['refused'])
  })

  it('accepts Go named groups', () => {
    expect(regexProblems([regex('^(?P<host>[a-z]+)\\.com$')])).toEqual([])
  })

  it('says RE2 syntax JavaScript cannot express cannot be tested, rather than invalid', () => {
    const patterns = ['(?U)a+', 'google\\z', '\\Qa.b\\E', '^[[:alpha:]]+$', 'a(?i)b', '\\Agoogle']
    expect(regexProblems(patterns.map(regex))).toEqual(patterns.map((_, index) => ({ index, kind: 'untestable' })))
  })

  it('lists the most serious problem first', () => {
    expect(regexProblems([regex('(?U)a'), regex('(['), regex('(a)\\1')])).toEqual([
      { index: 2, kind: 'refused' },
      { index: 1, kind: 'invalid' },
      { index: 0, kind: 'untestable' }
    ])
  })

  it('accepts the inline flags RE2 understands, and ignores other operators', () => {
    expect(regexProblems([regex('(?i)^google$'), { field: 'utm_source', operator: 'equals', value: '([' }])).toEqual(
      []
    )
  })
})
