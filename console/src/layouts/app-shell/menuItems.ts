import { createElement, type ReactNode } from 'react'
import type { I18n, MessageDescriptor } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import type { MenuProps } from 'antd'
import {
  AlignJustify,
  ChartLine,
  Files,
  LayoutGrid,
  Send,
  Settings,
  SquareTerminal,
  SquareUserRound,
  Workflow,
  type LucideIcon
} from 'lucide-react'
import type { UserPermissions } from '../../services/api/types'
import { MenuLink, type MenuTarget } from './MenuLink'

/** Web analytics sub-entries, mirroring the routes under /web-analytics. */
export const WEB_ANALYTICS_SECTIONS = [
  'dashboard',
  'live',
  'explore',
  'goals',
  'filters',
  'annotations'
] as const

export type MenuGroup = 'web-analytics' | 'content'

/** Collapsible sidebar groups, and the path fragments living inside each. */
const MENU_GROUPS: Record<MenuGroup, string[]> = {
  'web-analytics': ['/web-analytics'],
  content: ['/templates', '/blog', '/file-manager']
}

export type MenuItem = NonNullable<MenuProps['items']>[number]
export type HasAccess = (resource: keyof UserPermissions) => boolean

/**
 * The group the path belongs to. The web analytics settings live at /settings/web-analytics and
 * belong to the settings entry, so a settings path belongs to no group.
 */
export function activeGroup(pathname: string): MenuGroup | null {
  if (pathname.includes('/settings')) return null
  const entry = (Object.entries(MENU_GROUPS) as [MenuGroup, string[]][]).find(([, fragments]) =>
    fragments.some((fragment) => pathname.includes(fragment))
  )
  return entry?.[0] ?? null
}

/** The menu entry the path highlights. */
export function selectedMenuKey(pathname: string): string {
  // Must be checked before '/web-analytics': the web analytics settings live at
  // /settings/web-analytics and belong to the settings entry.
  if (pathname.includes('/settings')) return 'settings'
  if (activeGroup(pathname) === 'web-analytics') {
    // /web-analytics alone redirects to the dashboard, so an unrecognized trailing segment lands
    // on the same entry the user ends up looking at.
    const section = pathname.split('/web-analytics/')[1]?.split('/')[0] ?? ''
    return (WEB_ANALYTICS_SECTIONS as readonly string[]).includes(section)
      ? `web-analytics-${section}`
      : 'web-analytics-dashboard'
  }
  const cascade: [string, string][] = [
    // The Lists page redirects to Contacts, whose rail holds the lists.
    ['/lists', 'contacts'],
    ['/templates', 'templates'],
    ['/blog', 'blog'],
    ['/contacts', 'contacts'],
    ['/file-manager', 'file-manager'],
    ['/transactional-notifications', 'transactional-notifications'],
    ['/logs', 'logs'],
    ['/broadcasts', 'broadcasts'],
    ['/automations', 'automations']
  ]
  return cascade.find(([fragment]) => pathname.includes(fragment))?.[1] ?? 'analytics'
}

export type GroupPage =
  | {
      to: '/console/workspace/$workspaceId/web-analytics/$tab'
      params: { workspaceId: string; tab: 'dashboard' }
    }
  | {
      to: '/console/workspace/$workspaceId/templates'
      params: { workspaceId: string }
    }
  | {
      to: '/console/workspace/$workspaceId/blog'
      params: { workspaceId: string }
    }

/** Where opening a group lands: the first child the group renders. */
export function firstPageOfGroup(group: string, hasAccess: HasAccess, workspaceId: string): GroupPage | null {
  if (group === 'web-analytics') {
    return {
      to: '/console/workspace/$workspaceId/web-analytics/$tab',
      params: { workspaceId, tab: 'dashboard' }
    }
  }
  if (group === 'content') {
    if (hasAccess('templates'))
      return {
        to: '/console/workspace/$workspaceId/templates',
        params: { workspaceId }
      }
    if (hasAccess('workspace'))
      return {
        to: '/console/workspace/$workspaceId/blog',
        params: { workspaceId }
      }
  }
  return null
}

/** The page each top entry links to, for the entries that are plain links. */
const ENTRY_PAGES: Record<string, (workspaceId: string) => MenuTarget> = {
  contacts: (workspaceId) => ({ to: '/console/workspace/$workspaceId/contacts', params: { workspaceId } }),
  lists: (workspaceId) => ({ to: '/console/workspace/$workspaceId/lists', params: { workspaceId } }),
  broadcasts: (workspaceId) => ({ to: '/console/workspace/$workspaceId/broadcasts', params: { workspaceId } }),
  automations: (workspaceId) => ({ to: '/console/workspace/$workspaceId/automations', params: { workspaceId } }),
  'transactional-notifications': (workspaceId) => ({
    to: '/console/workspace/$workspaceId/transactional-notifications',
    params: { workspaceId }
  }),
  logs: (workspaceId) => ({ to: '/console/workspace/$workspaceId/logs', params: { workspaceId } }),
  settings: (workspaceId) => ({ to: '/console/workspace/$workspaceId/settings', params: { workspaceId } })
}

/** The sidebar's top entries after the dashboard, in its order, with the right each needs. */
const ENTRIES_AFTER_DASHBOARD: { key: string; reachable: (hasAccess: HasAccess) => boolean }[] = [
  { key: 'web-analytics', reachable: (hasAccess) => hasAccess('web_analytics') },
  { key: 'contacts', reachable: (hasAccess) => hasAccess('contacts') },
  { key: 'lists', reachable: (hasAccess) => hasAccess('lists') },
  { key: 'broadcasts', reachable: (hasAccess) => hasAccess('broadcasts') },
  { key: 'automations', reachable: (hasAccess) => hasAccess('automations') },
  { key: 'transactional-notifications', reachable: (hasAccess) => hasAccess('transactional') },
  { key: 'content', reachable: (hasAccess) => hasAccess('templates') || hasAccess('workspace') },
  { key: 'logs', reachable: (hasAccess) => hasAccess('message_history') },
  { key: 'settings', reachable: (hasAccess) => hasAccess('workspace') }
]

export interface MenuPage {
  key: string
  target: MenuTarget
}

/**
 * Where a member who cannot read the dashboard lands: the first sidebar entry after it they can
 * reach, Settings when none. Never the dashboard itself, so a redirect from it cannot loop.
 */
export function firstAccessiblePage(hasAccess: HasAccess, workspaceId: string): MenuPage {
  for (const entry of ENTRIES_AFTER_DASHBOARD) {
    if (!entry.reachable(hasAccess)) continue
    const group = firstPageOfGroup(entry.key, hasAccess, workspaceId)
    if (group) return { key: entry.key, target: group }
    const page = ENTRY_PAGES[entry.key]
    if (page) return { key: entry.key, target: page(workspaceId) }
  }
  return { key: 'settings', target: ENTRY_PAGES.settings(workspaceId) }
}

const LABELS = {
  dashboard: msg`Dashboard`,
  webAnalytics: msg`Web Analytics`,
  live: msg`Live`,
  explore: msg`Explore`,
  goals: msg`Goals`,
  filters: msg`Filters`,
  annotations: msg`Annotations`,
  contacts: msg`Contacts`,
  broadcasts: msg`Broadcasts`,
  automations: msg`Automations`,
  transactional: msg`Transactional`,
  content: msg`Content`,
  templates: msg`Templates`,
  blog: msg`Blog`,
  fileManager: msg`File Manager`,
  logs: msg`Logs`,
  settings: msg`Settings`
} satisfies Record<string, MessageDescriptor>

// Written with createElement so this module stays a .ts file of plain functions (fast refresh
// wants a .tsx file to export components only).
function icon(Icon: LucideIcon): ReactNode {
  return createElement(Icon, {
    size: 15,
    strokeWidth: 1.75,
    'aria-hidden': true
  })
}

function link(target: MenuTarget, text: string): ReactNode {
  return createElement(MenuLink, { target, children: text })
}

interface BuildMenuItemsOptions {
  hasAccess: HasAccess
  workspaceId: string
  /** From useLingui(): the macro's `t` cannot leave the component that calls it. */
  i18n: I18n
}

/** The sidebar entries the member's rights allow, in the menu's order. */
export function buildMenuItems({ hasAccess, workspaceId, i18n }: BuildMenuItemsOptions): MenuItem[] {
  const label = (descriptor: MessageDescriptor) => i18n._(descriptor)
  const tab = (key: (typeof WEB_ANALYTICS_SECTIONS)[number], text: MessageDescriptor): MenuItem =>
    key === 'live'
      ? {
          key: `web-analytics-${key}`,
          label: link(
            {
              to: '/console/workspace/$workspaceId/web-analytics/live',
              params: { workspaceId }
            },
            label(text)
          )
        }
      : {
          key: `web-analytics-${key}`,
          label: link(
            {
              to: '/console/workspace/$workspaceId/web-analytics/$tab',
              params: { workspaceId, tab: key }
            },
            label(text)
          )
        }

  // Templates, Blog and File Manager are the material you author and reuse. The group is dropped
  // when a member can reach none of it, instead of showing an empty expandable row. Children
  // carry no icons, as in the Web Analytics group.
  const contentChildren: MenuItem[] = []
  if (hasAccess('templates')) {
    contentChildren.push({
      key: 'templates',
      label: link(
        {
          to: '/console/workspace/$workspaceId/templates',
          params: { workspaceId }
        },
        label(LABELS.templates)
      )
    })
  }
  if (hasAccess('workspace')) {
    contentChildren.push(
      {
        key: 'blog',
        label: link(
          {
            to: '/console/workspace/$workspaceId/blog',
            params: { workspaceId }
          },
          label(LABELS.blog)
        )
      },
      {
        key: 'file-manager',
        label: link(
          {
            to: '/console/workspace/$workspaceId/file-manager',
            params: { workspaceId }
          },
          label(LABELS.fileManager)
        )
      }
    )
  }

  const items: (MenuItem | false)[] = [
    hasAccess('message_history') && {
      key: 'analytics',
      icon: icon(LayoutGrid),
      label: link({ to: '/console/workspace/$workspaceId', params: { workspaceId } }, label(LABELS.dashboard))
    },
    hasAccess('web_analytics') && {
      key: 'web-analytics',
      icon: icon(ChartLine),
      // A submenu rather than a link: the caret toggles the section, and opening it lands on the
      // dashboard (WorkspaceLayout's handleOpenChange).
      label: label(LABELS.webAnalytics),
      children: [
        tab('dashboard', LABELS.dashboard),
        tab('live', LABELS.live),
        tab('explore', LABELS.explore),
        tab('goals', LABELS.goals),
        tab('filters', LABELS.filters),
        tab('annotations', LABELS.annotations)
      ]
    },
    // Contacts holds the lists too: a member who may only read lists lands there.
    (hasAccess('contacts') || hasAccess('lists')) && {
      key: 'contacts',
      icon: icon(SquareUserRound),
      label: link(
        {
          to: '/console/workspace/$workspaceId/contacts',
          params: { workspaceId }
        },
        label(LABELS.contacts)
      )
    },
    hasAccess('broadcasts') && {
      key: 'broadcasts',
      icon: icon(Send),
      label: link(
        {
          to: '/console/workspace/$workspaceId/broadcasts',
          params: { workspaceId }
        },
        label(LABELS.broadcasts)
      )
    },
    hasAccess('automations') && {
      key: 'automations',
      icon: icon(Workflow),
      label: link(
        {
          to: '/console/workspace/$workspaceId/automations',
          params: { workspaceId }
        },
        label(LABELS.automations)
      )
    },
    hasAccess('transactional') && {
      key: 'transactional-notifications',
      icon: icon(SquareTerminal),
      label: link(
        {
          to: '/console/workspace/$workspaceId/transactional-notifications',
          params: { workspaceId }
        },
        label(LABELS.transactional)
      )
    },
    contentChildren.length > 0 && {
      key: 'content',
      icon: icon(Files),
      label: label(LABELS.content),
      children: contentChildren
    },
    hasAccess('message_history') && {
      key: 'logs',
      icon: icon(AlignJustify),
      label: link({ to: '/console/workspace/$workspaceId/logs', params: { workspaceId } }, label(LABELS.logs))
    },
    hasAccess('workspace') && {
      key: 'settings',
      icon: icon(Settings),
      label: link(
        {
          to: '/console/workspace/$workspaceId/settings',
          params: { workspaceId }
        },
        label(LABELS.settings)
      )
    }
  ]
  return items.filter((item): item is MenuItem => Boolean(item))
}
