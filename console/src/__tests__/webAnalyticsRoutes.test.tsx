import { describe, it, expect } from 'vitest'
import { router } from '../router'

// The web analytics sections live behind a single /$tab route, so an unknown
// section used to render the dashboard under whatever URL was typed.
describe('web analytics route guard', () => {
  it('rewrites an unknown section to the dashboard', async () => {
    window.history.pushState({}, '', '/console/workspace/ws1/web-analytics/nope')
    await router.load()
    expect(router.state.location.pathname).toBe('/console/workspace/ws1/web-analytics/dashboard')
  })

  it('leaves a known section alone', async () => {
    window.history.pushState({}, '', '/console/workspace/ws1/web-analytics/explore')
    await router.load()
    expect(router.state.location.pathname).toBe('/console/workspace/ws1/web-analytics/explore')
  })
})

describe('web analytics minSessions search param', () => {
  /** The minSessions the route hands the page, for a raw query string. */
  const parsed = async (query: string) => {
    window.history.pushState({}, '', `/console/workspace/ws1/web-analytics/explore?${query}`)
    await router.load()
    const match = router.state.matches.find((candidate) => candidate.routeId.endsWith('/web-analytics/$tab'))
    return (match?.search as { minSessions?: unknown } | undefined)?.minSessions
  }

  it('keeps a floor of 1, whether it arrives as a number or a string', async () => {
    // The router JSON-parses search values: `1` is the number, `"1"` the string.
    expect(await parsed('minSessions=1')).toBe(1)
    expect(await parsed('minSessions=%221%22')).toBe(1)
    expect(await parsed('minSessions=25')).toBe(25)
  })

  it('drops zero, negatives and garbage, which the page then reads as the default', async () => {
    expect(await parsed('minSessions=0')).toBeUndefined()
    expect(await parsed('minSessions=-5')).toBeUndefined()
    expect(await parsed('minSessions=abc')).toBeUndefined()
    expect(await parsed('minSessions=true')).toBeUndefined()
  })
})
