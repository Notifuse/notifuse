import { describe, it, expect } from 'vitest'
import { generateSegmentId, segmentIdProblem } from './segmentId'

describe('generateSegmentId', () => {
  it('writes the name in snake_case', () => {
    expect(generateSegmentId('Big Spenders')).toBe('big_spenders')
    expect(generateSegmentId('  VIP - 2026  ')).toBe('vip_2026')
  })

  it('folds accents to their base letter', () => {
    expect(generateSegmentId('Clients actifs à forte valeur')).toBe('clients_actifs_a_forte_valeur')
    expect(generateSegmentId('Été')).toBe('ete')
  })

  it('drops punctuation', () => {
    expect(generateSegmentId('Opened (30 d)!')).toBe('opened_30_d')
  })

  it('gives nothing for a name without a Latin letter or digit', () => {
    expect(generateSegmentId('会員')).toBe('')
    expect(segmentIdProblem('会員')).toBe('empty')
    expect(segmentIdProblem('会員 2026')).toBeUndefined()
  })

  it('cuts the ID at the 32 characters the server takes, without a trailing underscore', () => {
    const id = generateSegmentId('Subscribed to the newsletter in the last 30 days')
    expect(id).toBe('subscribed_to_the_newsletter_in')
    expect(id.length).toBeLessThanOrEqual(32)
  })
})
