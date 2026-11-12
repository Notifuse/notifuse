import { describe, it, expect } from 'vitest'
import { i18n } from '@lingui/core'
import { summarizeUsage, groupReferences, referenceStatusLabel } from './templateUsage'
import type { TemplateUsageReference } from '../../services/api/template'

i18n.loadAndActivate({ locale: 'en', messages: {} })

describe('summarizeUsage', () => {
  it('is not used without counts', () => {
    expect(summarizeUsage(undefined)).toEqual({ total: 0, label: null, details: [] })
  })

  it('is not used when every count is zero', () => {
    expect(summarizeUsage({ broadcasts: 0, lists: 0 }).label).toBeNull()
  })

  it.each([
    [{ broadcasts: 1 }, '1 newsletter'],
    [{ broadcasts: 3 }, '3 newsletters'],
    [{ transactional_notifications: 1 }, '1 notification'],
    [{ transactional_notifications: 2 }, '2 notifications'],
    [{ automations: 1 }, '1 automation'],
    [{ automations: 4 }, '4 automations'],
    [{ lists: 1 }, '1 list'],
    [{ lists: 2 }, '2 lists']
  ])('names the single type of %j', (counts, label) => {
    const summary = summarizeUsage(counts)
    expect(summary.label).toBe(label)
    expect(summary.details).toEqual([label])
  })

  it('sums several types and details each one, in the order of the response', () => {
    const summary = summarizeUsage({ lists: 1, broadcasts: 2, automations: 0 })
    expect(summary.total).toBe(3)
    expect(summary.label).toBe('3 uses')
    expect(summary.details).toEqual(['2 newsletters', '1 list'])
  })
})

describe('groupReferences', () => {
  const refs = (type: TemplateUsageReference['type'], n: number): TemplateUsageReference[] =>
    Array.from({ length: n }, (_, i) => ({ type, id: `${type}${i}`, name: `${type} ${i}` }))

  it('groups by type in the response order and keeps at most five per type', () => {
    const groups = groupReferences(
      [...refs('list', 1), ...refs('broadcast', 7)],
      { broadcasts: 7, lists: 1 },
      5
    )
    expect(groups.map((g) => g.type)).toEqual(['broadcasts', 'lists'])
    expect(groups[0].items).toHaveLength(5)
    expect(groups[0].more).toBe(2)
    expect(groups[1].items).toHaveLength(1)
    expect(groups[1].more).toBe(0)
  })

  it('counts the more from the exact usage count, beyond the references returned', () => {
    // The server returns at most 50 references per type while its count stays exact.
    const groups = groupReferences(refs('automation', 50), { automations: 120 }, 5)
    expect(groups[0].more).toBe(115)
  })

  it('leaves out a type without references', () => {
    expect(groupReferences([], { broadcasts: 0 }, 5)).toEqual([])
  })
})

describe('referenceStatusLabel', () => {
  it.each([
    ['draft', 'Draft'],
    ['scheduled', 'Scheduled'],
    ['processing', 'Sending'],
    ['paused', 'Paused'],
    ['testing', 'Testing'],
    ['test_completed', 'Test completed'],
    ['winner_selected', 'Winner selected'],
    ['live', 'Live']
  ])('labels the status %s', (status, label) => {
    expect(referenceStatusLabel(status, i18n)).toBe(label)
  })

  it('shows a status it does not know as the server sent it', () => {
    expect(referenceStatusLabel('archived', i18n)).toBe('archived')
  })
})
