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
