import { describe, it, expect } from 'vitest'
import {
  searchString,
  parseSettingsSearch,
  parseTemplatesSearch,
  parseBlogSearch,
  parseFileManagerSearch
} from './searchParams'

// The router's default search parser runs JSON.parse on each value: "?webhook=2024" arrives as the
// number 2024 and "?webhook=true" as a boolean.
describe('searchString', () => {
  it('keeps a string, trimmed', () => {
    expect(searchString(' abc ')).toBe('abc')
  })

  it('turns a number or a boolean the parser produced back into its text', () => {
    expect(searchString(2024)).toBe('2024')
    expect(searchString(true)).toBe('true')
  })

  it('drops an empty value, an object, null and undefined', () => {
    expect(searchString('')).toBeUndefined()
    expect(searchString('   ')).toBeUndefined()
    expect(searchString({ a: 1 })).toBeUndefined()
    expect(searchString(null)).toBeUndefined()
    expect(searchString(undefined)).toBeUndefined()
  })

  it('reads the first value of a repeated parameter', () => {
    expect(searchString(['a', 'b'])).toBe('a')
  })
})

describe('parseSettingsSearch', () => {
  it('keeps the webhook to open, as a string', () => {
    expect(parseSettingsSearch({ webhook: 'wh_1' })).toEqual({ webhook: 'wh_1' })
    expect(parseSettingsSearch({ webhook: 42 })).toEqual({ webhook: '42' })
  })

  it('leaves the webhook out when absent or empty', () => {
    expect(parseSettingsSearch({})).toEqual({})
    expect(parseSettingsSearch({ webhook: '' })).toEqual({})
  })
})

describe('parseTemplatesSearch', () => {
  it('keeps a known category', () => {
    expect(parseTemplatesSearch({ category: 'opt_in' })).toEqual({ category: 'opt_in' })
    expect(parseTemplatesSearch({ category: 'blog' })).toEqual({ category: 'blog' })
  })

  it('drops a category the server does not have', () => {
    expect(parseTemplatesSearch({ category: 'automation' })).toEqual({})
    expect(parseTemplatesSearch({ category: 1 })).toEqual({})
    expect(parseTemplatesSearch({ category: true })).toEqual({})
  })

  it('keeps the search and the template to preview as strings', () => {
    expect(parseTemplatesSearch({ q: 2024, preview: 'welcome' })).toEqual({
      q: '2024',
      preview: 'welcome'
    })
    expect(parseTemplatesSearch({ preview: false })).toEqual({ preview: 'false' })
  })

  it('leaves out empty values and other types', () => {
    expect(parseTemplatesSearch({ q: '', preview: { id: 1 } })).toEqual({})
  })
})

describe('parseBlogSearch', () => {
  it('keeps a known status, scheduled included', () => {
    expect(parseBlogSearch({ status: 'scheduled' })).toEqual({ status: 'scheduled' })
    expect(parseBlogSearch({ status: 'draft' })).toEqual({ status: 'draft' })
  })

  it('drops a status the server would refuse', () => {
    expect(parseBlogSearch({ status: 'archived' })).toEqual({})
    expect(parseBlogSearch({ status: 1 })).toEqual({})
  })

  it('keeps the category and the search as text', () => {
    expect(parseBlogSearch({ category_id: 2024, q: ' launch ' })).toEqual({
      category_id: '2024',
      q: 'launch'
    })
  })

  it('drops an empty category or search', () => {
    expect(parseBlogSearch({ category_id: '', q: '  ' })).toEqual({})
  })
})

describe('parseFileManagerSearch', () => {
  it('keeps the folder and the file as text, a number or a boolean included', () => {
    expect(parseFileManagerSearch({ path: 'docs/', file: 2024 })).toEqual({
      path: 'docs/',
      file: '2024'
    })
    expect(parseFileManagerSearch({ path: true })).toEqual({ path: 'true/' })
  })

  it('turns a folder into a prefix: a trailing slash, no leading one', () => {
    expect(parseFileManagerSearch({ path: 'docs' })).toEqual({ path: 'docs/' })
    expect(parseFileManagerSearch({ path: '/docs/2026/' })).toEqual({ path: 'docs/2026/' })
  })

  it('drops an empty folder or file, and other types', () => {
    expect(parseFileManagerSearch({ path: '', file: '  ' })).toEqual({})
    expect(parseFileManagerSearch({ path: '/', file: { key: 'a' } })).toEqual({})
  })
})
