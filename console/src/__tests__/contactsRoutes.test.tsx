import { describe, it, expect } from 'vitest'
import { router } from '../router'

const ws = '/console/workspace/ws1'

/** The pathname after redirects, and the contacts route's validated search. */
async function go(url: string) {
  window.history.pushState({}, '', url)
  await router.load()
  const match = router.state.matches[router.state.matches.length - 1]
  return { pathname: router.state.location.pathname, search: match.search as Record<string, unknown> }
}

describe('contacts routes', () => {
  it('sends the Lists page to Contacts', async () => {
    const location = await go(`${ws}/lists`)
    expect(location.pathname).toBe(`${ws}/contacts`)
  })

  it('opens a single legacy segment as the segment view', async () => {
    const location = await go(`${ws}/contacts?segments=pro`)
    expect(location.pathname).toBe(`${ws}/contacts`)
    expect(location.search).toMatchObject({ segment_id: 'pro' })
    expect(location.search.segments).toBeUndefined()
  })

  it('keeps a single legacy segment as a filter of a list', async () => {
    const location = await go(`${ws}/contacts?list_id=news&segments=pro`)
    expect(location.search).toMatchObject({ list_id: 'news', segments: ['pro'] })
    expect(location.search.segment_id).toBeUndefined()
  })

  it('keeps several legacy segments as a filter', async () => {
    const location = await go(`${ws}/contacts?segments=a&segments=b`)
    expect(location.search.segments).toEqual(['a', 'b'])
    expect(location.search.segment_id).toBeUndefined()
  })

  it('loads 50 rows by default, and never more than 10 000', async () => {
    expect((await go(`${ws}/contacts`)).search.limit).toBe(50)
    expect((await go(`${ws}/contacts?limit=abc`)).search.limit).toBe(50)
    expect((await go(`${ws}/contacts?limit=0`)).search.limit).toBe(50)
    expect((await go(`${ws}/contacts?limit=150`)).search.limit).toBe(150)
    expect((await go(`${ws}/contacts?limit=99999`)).search.limit).toBe(10000)
  })

  it('keeps a list over a segment when both are given', async () => {
    const location = await go(`${ws}/contacts?list_id=news&segment_id=pro`)
    expect(location.search.list_id).toBe('news')
    expect(location.search.segment_id).toBeUndefined()
  })

  it('reads numeric-looking values as text', async () => {
    const location = await go(`${ws}/contacts?search=123&contact=42&list_id=7`)
    expect(location.search.search).toBe('123')
    expect(location.search.contact).toBe('42')
    expect(location.search.list_id).toBe('7')
  })
})
