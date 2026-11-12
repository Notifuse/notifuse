import { describe, it, expect } from 'vitest'
import { router } from '../router'

describe('route page layouts', () => {
  it('declares the rail layout on Settings, its sections and the Blog, and on nothing else', () => {
    const declared = Object.values(router.routesById)
      .filter((route) => route.options.staticData?.layout !== undefined)
      .map((route) => [route.id, route.options.staticData?.layout])
      .sort(([a], [b]) => String(a).localeCompare(String(b)))

    expect(declared).toEqual([
      ['/console/workspace/$workspaceId/blog', 'rail'],
      ['/console/workspace/$workspaceId/settings', 'rail'],
      ['/console/workspace/$workspaceId/settings/$section', 'rail']
    ])
  })
})
