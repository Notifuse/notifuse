import { describe, it, expect } from 'vitest'
import { emailsFromColumn, parseBulkCsv } from './bulkCsv'

describe('parseBulkCsv', () => {
  it('reads the header and rows, and points at the columns named like an address', () => {
    const csv = parseBulkCsv('Name,Work E-mail,Email address\nAnna,a@work.co,a@home.co\n\n', 'u.csv')
    expect(csv).toEqual({
      fileName: 'u.csv',
      headers: ['Name', 'Work E-mail', 'Email address'],
      rows: [['Anna', 'a@work.co', 'a@home.co']],
      emailColumnCandidates: [1, 2]
    })
  })

  it('points at no column when none is named like an address', () => {
    expect(parseBulkCsv('who,where\na@b.co,Paris\n', 'u.csv')?.emailColumnCandidates).toEqual([])
  })

  it('refuses a file without a data row', () => {
    expect(parseBulkCsv('email\n', 'u.csv')).toBeNull()
    expect(parseBulkCsv('', 'u.csv')).toBeNull()
  })

  it('keeps a quoted value whole', () => {
    expect(parseBulkCsv('email,name\na@b.co,"Ortiz, Javier"\n', 'u.csv')?.rows).toEqual([['a@b.co', 'Ortiz, Javier']])
  })
})

describe('emailsFromColumn', () => {
  const rows = [
    ['A@B.co ', 'x'],
    ['a@b.co', 'y'],
    ['', 'z'],
    ['not-an-email', 'w'],
    ['c@d.io', 'v']
  ]

  it('lowercases and trims, removes duplicates, and counts what it left out', () => {
    expect(emailsFromColumn(rows, 0)).toEqual({ emails: ['a@b.co', 'c@d.io'], rows: 5, duplicates: 1, invalid: 2 })
  })

  it('reads another column when asked', () => {
    expect(emailsFromColumn([['x', 'e@f.co']], 1).emails).toEqual(['e@f.co'])
  })

  it('counts an address the server refuses as invalid', () => {
    expect(emailsFromColumn([['a@b']], 0)).toMatchObject({ emails: [], invalid: 1 })
  })
})
