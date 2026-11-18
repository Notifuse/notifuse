import { describe, expect, it } from 'vitest'
import { generateListId, LIST_ID_PATTERN } from './listId'

describe('generateListId', () => {
  it('lowercases and drops spaces', () => {
    expect(generateListId('Newsletter produit')).toBe('newsletterproduit')
  })

  it('keeps accented letters as their letter', () => {
    expect(generateListId('Clients été')).toBe('clientsete')
    expect(generateListId('Ça déménage')).toBe('cademenage')
  })

  it('drops punctuation and keeps digits', () => {
    expect(generateListId("Beta testers (2026) - l'équipe!")).toBe('betatesters2026lequipe')
  })

  it('gives no ID for a name without a Latin letter or digit', () => {
    expect(generateListId('会員ニュース')).toBe('')
    expect(generateListId('')).toBe('')
  })

  it('cuts at 32 characters', () => {
    const id = generateListId('a'.repeat(40))
    expect(id).toHaveLength(32)
  })

  it('always gives an ID the server accepts, or none', () => {
    for (const name of ['Été 2026', 'Ünïcödé', 'Smörgåsbord', 'Straße']) {
      const id = generateListId(name)
      expect(id === '' || LIST_ID_PATTERN.test(id)).toBe(true)
    }
  })
})
