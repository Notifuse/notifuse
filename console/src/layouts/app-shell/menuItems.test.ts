import { describe, it, expect, vi } from 'vitest'
import { i18n } from '@lingui/core'
import type { ReactNode } from 'react'
import type { UserPermissions } from '../../services/api/types'
import { activeGroup, buildMenuItems, firstPageOfGroup, selectedMenuKey, type MenuItem } from './menuItems'

vi.mock('./MenuLink', () => ({
  MenuLink: ({ children }: { children: ReactNode }) => children
}))

const ws = '/console/workspace/ws1'

describe('selectedMenuKey', () => {
  it.each([
    [ws, 'analytics'],
    [`${ws}/settings/team`, 'settings'],
    [`${ws}/settings/web-analytics`, 'settings'],
    [`${ws}/web-analytics/live`, 'web-analytics-live'],
    [`${ws}/web-analytics/annotations`, 'web-analytics-annotations'],
    [`${ws}/web-analytics/unknown`, 'web-analytics-dashboard'],
    [`${ws}/web-analytics`, 'web-analytics-dashboard'],
    [`${ws}/lists`, 'lists'],
    [`${ws}/templates`, 'templates'],
    [`${ws}/blog`, 'blog'],
    [`${ws}/contacts`, 'contacts'],
    [`${ws}/file-manager`, 'file-manager'],
    [`${ws}/transactional-notifications`, 'transactional-notifications'],
    [`${ws}/logs`, 'logs'],
    [`${ws}/broadcasts`, 'broadcasts'],
    [`${ws}/automations`, 'automations'],
    [`${ws}/debug-segment`, 'analytics']
  ])('%s selects %s', (pathname, key) => {
    expect(selectedMenuKey(pathname)).toBe(key)
  })
})

describe('activeGroup', () => {
  it.each([
    [`${ws}/web-analytics/goals`, 'web-analytics'],
    [`${ws}/templates`, 'content'],
    [`${ws}/blog`, 'content'],
    [`${ws}/file-manager`, 'content'],
    [`${ws}/settings/web-analytics`, null],
    [`${ws}/contacts`, null]
  ])('%s is in %s', (pathname, group) => {
    expect(activeGroup(pathname)).toBe(group)
  })
})

const allowAll = () => true
const allowAllBut =
  (...denied: (keyof UserPermissions)[]) =>
  (resource: keyof UserPermissions) =>
    !denied.includes(resource)

describe('firstPageOfGroup', () => {
  it('opens the web analytics dashboard', () => {
    expect(firstPageOfGroup('web-analytics', allowAll, 'ws1')).toEqual({
      to: '/console/workspace/$workspaceId/web-analytics/$tab',
      params: { workspaceId: 'ws1', tab: 'dashboard' }
    })
  })

  it('opens templates, else the blog, else nothing', () => {
    expect(firstPageOfGroup('content', allowAll, 'ws1')).toEqual({
      to: '/console/workspace/$workspaceId/templates',
      params: { workspaceId: 'ws1' }
    })
    expect(firstPageOfGroup('content', allowAllBut('templates'), 'ws1')).toEqual({
      to: '/console/workspace/$workspaceId/blog',
      params: { workspaceId: 'ws1' }
    })
    expect(firstPageOfGroup('content', allowAllBut('templates', 'workspace'), 'ws1')).toBeNull()
  })

  it('has no page for an unknown group', () => {
    expect(firstPageOfGroup('other', allowAll, 'ws1')).toBeNull()
  })
})

type WithChildren = { key: string; children?: MenuItem[] }
const keys = (items: MenuItem[]) => items.map((item) => (item as WithChildren).key)
const childKeys = (items: MenuItem[], key: string) =>
  keys((items.find((item) => (item as WithChildren).key === key) as WithChildren).children ?? [])

describe('buildMenuItems', () => {
  it('lists every entry in the menu order for a full access', () => {
    const items = buildMenuItems({ hasAccess: allowAll, workspaceId: 'ws1', i18n })
    expect(keys(items)).toEqual([
      'analytics',
      'web-analytics',
      'contacts',
      'lists',
      'broadcasts',
      'automations',
      'transactional-notifications',
      'content',
      'logs',
      'settings'
    ])
    expect(childKeys(items, 'web-analytics')).toEqual([
      'web-analytics-dashboard',
      'web-analytics-live',
      'web-analytics-explore',
      'web-analytics-goals',
      'web-analytics-filters',
      'web-analytics-annotations'
    ])
    expect(childKeys(items, 'content')).toEqual(['templates', 'blog', 'file-manager'])
  })

  it.each([
    ['message_history', ['analytics', 'logs']],
    ['web_analytics', ['web-analytics']],
    ['contacts', ['contacts']],
    ['lists', ['lists']],
    ['broadcasts', ['broadcasts']],
    ['automations', ['automations']],
    ['transactional', ['transactional-notifications']],
    ['workspace', ['settings']]
  ] as const)('drops the entries a member without %s cannot reach', (resource, gone) => {
    const items = keys(buildMenuItems({ hasAccess: allowAllBut(resource), workspaceId: 'ws1', i18n }))
    gone.forEach((key) => expect(items).not.toContain(key))
  })

  it('keeps Blog and File Manager without templates, and drops Content without any child', () => {
    const noTemplates = buildMenuItems({ hasAccess: allowAllBut('templates'), workspaceId: 'ws1', i18n })
    expect(childKeys(noTemplates, 'content')).toEqual(['blog', 'file-manager'])
    const none = buildMenuItems({
      hasAccess: allowAllBut('templates', 'workspace'),
      workspaceId: 'ws1',
      i18n
    })
    expect(keys(none)).not.toContain('content')
  })

  it('gives each top entry an icon and no child an icon', () => {
    const items = buildMenuItems({ hasAccess: allowAll, workspaceId: 'ws1', i18n }) as {
      icon?: unknown
      children?: { icon?: unknown }[]
    }[]
    items.forEach((item) => expect(item.icon).toBeTruthy())
    items.flatMap((item) => item.children ?? []).forEach((child) => expect(child.icon).toBeUndefined())
  })
})
