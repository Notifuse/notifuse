import { useEffect } from 'react'
import { createRootRoute, createRoute, redirect, useParams, useNavigate } from '@tanstack/react-router'
import { RootLayout } from './layouts/RootLayout'
import { WorkspaceLayout } from './layouts/WorkspaceLayout'
import { SignInPage } from './pages/SignInPage'
import { LogoutPage } from './pages/LogoutPage'
import { AcceptInvitationPage } from './pages/AcceptInvitationPage'
import { CreateWorkspacePage } from './pages/CreateWorkspacePage'
import { DashboardPage } from './pages/DashboardPage'
import { WorkspaceSettingsPage } from './pages/WorkspaceSettingsPage'
import { ContactsPage } from './pages/ContactsPage'
import { ListsPage } from './pages/ListsPage'
import { FileManagerPage } from './pages/FileManagerPage'
import { TemplatesPage } from './pages/TemplatesPage'
import { BroadcastsPage } from './pages/BroadcastsPage'
import { AutomationsPage } from './pages/AutomationsPage'
import { TransactionalNotificationsPage } from './pages/TransactionalNotificationsPage'
import { LogsPage } from './pages/LogsPage'
import { AnalyticsPage } from './pages/AnalyticsPage'
import { WebAnalyticsPage } from './pages/WebAnalyticsPage'
import { WebAnalyticsLivePage } from './pages/WebAnalyticsLivePage'
import {
  DATE_PRESETS,
  type ComparisonMode,
  type DatePreset
} from './components/web_analytics/lib/types'
import { WEB_ANALYTICS_TABS, type WebAnalyticsSearch } from './components/web_analytics/context'
import { DebugSegmentPage } from './pages/DebugSegmentPage'
import { BlogPage } from './pages/BlogPage'
import SetupWizard from './pages/SetupWizard'
import { createRouter } from '@tanstack/react-router'

export interface ContactsSearch {
  email?: string
  external_id?: string
  first_name?: string
  last_name?: string
  full_name?: string
  phone?: string
  country?: string
  language?: string
  list_id?: string
  contact_list_status?: string
  segments?: string[]
  limit?: number
}

export interface SignInSearch {
  email?: string
  // OIDC failure flag (non-secret enum). The success one-time code arrives in the
  // URL fragment (#oidc_code=…), which is NOT a search param, so it is not listed here.
  oidc_error?: string
}

export interface AcceptInvitationSearch {
  token?: string
}

export interface BlogSearch {
  status?: string
  category_id?: string
}

export interface FileManagerSearch {
  path?: string
}

export interface BroadcastsSearch {
  status?: string
  q?: string
}

export interface TemplatesSearch {
  category?: string
  q?: string
}

// Create the root route
const rootRoute = createRootRoute({
  component: RootLayout
})

// Create the index route
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/console/',
  component: DashboardPage
})

// Create the signin route
const signinRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/console/signin',
  component: SignInPage,
  validateSearch: (search: Record<string, unknown>): SignInSearch => ({
    email: search.email as string | undefined,
    oidc_error: typeof search.oidc_error === 'string' ? search.oidc_error : undefined
  })
})

// Create the logout route
const logoutRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/console/logout',
  component: LogoutPage
})

// Create the setup wizard route
const setupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/console/setup',
  component: SetupWizard
})

// Create the accept invitation route
const acceptInvitationRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/console/accept-invitation',
  component: AcceptInvitationPage,
  validateSearch: (search: Record<string, unknown>): AcceptInvitationSearch => ({
    token: search.token as string | undefined
  })
})

// Create the workspace create route
const workspaceCreateRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/console/workspace/create',
  component: CreateWorkspacePage
})

// Create the workspace route
const workspaceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/console/workspace/$workspaceId',
  component: WorkspaceLayout
})

// Create the default workspace route (redirects to analytics/dashboard)
const workspaceIndexRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/',
  component: AnalyticsPage
})

// Create workspace child routes
const workspaceBroadcastsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/broadcasts',
  component: BroadcastsPage,
  validateSearch: (search: Record<string, unknown>): BroadcastsSearch => {
    // Repeated query keys (?status=a&status=b) parse to arrays; coerce to a
    // single value, trim, and drop empties so the page always sees a clean
    // string or undefined.
    const normalize = (value: unknown): string | undefined => {
      const single = Array.isArray(value) ? value[0] : value
      if (typeof single !== 'string') return undefined
      const trimmed = single.trim()
      return trimmed === '' ? undefined : trimmed
    }
    return { status: normalize(search.status), q: normalize(search.q) }
  }
})

const workspaceAutomationsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/automations',
  component: AutomationsPage
})

const workspaceListsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/lists',
  component: ListsPage
})

export const workspaceFileManagerRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/file-manager',
  component: FileManagerPage,
  validateSearch: (search: Record<string, unknown>): FileManagerSearch => ({
    path: search.path as string | undefined
  })
})

const workspaceTransactionalNotificationsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/transactional-notifications',
  component: TransactionalNotificationsPage
})

const workspaceLogsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/logs',
  component: LogsPage
})

export const workspaceContactsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/contacts',
  component: ContactsPage,
  validateSearch: (search: Record<string, unknown>): ContactsSearch => ({
    email: search.email as string | undefined,
    external_id: search.external_id as string | undefined,
    first_name: search.first_name as string | undefined,
    last_name: search.last_name as string | undefined,
    full_name: search.full_name as string | undefined,
    phone: search.phone as string | undefined,
    country: search.country as string | undefined,
    language: search.language as string | undefined,
    list_id: search.list_id as string | undefined,
    contact_list_status: search.contact_list_status as string | undefined,
    segments: Array.isArray(search.segments)
      ? (search.segments as string[])
      : search.segments
        ? [search.segments as string]
        : undefined,
    limit: search.limit ? Number(search.limit) : 10
  })
})

// eslint-disable-next-line react-refresh/only-export-components -- Internal redirect component
const WorkspaceSettingsRedirect = () => {
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/settings' })
  const navigate = useNavigate()

  useEffect(() => {
    navigate({
      to: '/console/workspace/$workspaceId/settings/$section',
      params: { workspaceId, section: 'team' },
      replace: true
    })
  }, [workspaceId, navigate])

  return null
}

const workspaceSettingsRedirectRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/settings',
  component: WorkspaceSettingsRedirect
})

const workspaceSettingsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/settings/$section',
  component: WorkspaceSettingsPage
})

const workspaceTemplatesRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/templates',
  component: TemplatesPage,
  validateSearch: (search: Record<string, unknown>): TemplatesSearch => {
    // The default JSON.parse-based search parser coerces number/boolean-looking
    // values (?q=123 -> number, ?q=a&q=b -> array), and the search box calls
    // .trim() on q. Coerce every param to a clean string or undefined so a
    // hand-written or shared URL can never feed a non-string into the page.
    const normalize = (value: unknown): string | undefined => {
      const single = Array.isArray(value) ? value[0] : value
      if (typeof single !== 'string') return undefined
      const trimmed = single.trim()
      return trimmed === '' ? undefined : trimmed
    }
    return { category: normalize(search.category), q: normalize(search.q) }
  }
})

const workspaceAnalyticsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/analytics',
  component: AnalyticsPage
})

// eslint-disable-next-line react-refresh/only-export-components -- Internal redirect component
const WebAnalyticsRedirect = () => {
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/web-analytics' })
  const navigate = useNavigate()

  useEffect(() => {
    navigate({
      to: '/console/workspace/$workspaceId/web-analytics/$tab',
      params: { workspaceId, tab: 'dashboard' },
      replace: true
    })
  }, [workspaceId, navigate])

  return null
}

const workspaceWebAnalyticsRedirectRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/web-analytics',
  component: WebAnalyticsRedirect
})

const workspaceWebAnalyticsLiveRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/web-analytics/live',
  component: WebAnalyticsLivePage
})

const workspaceWebAnalyticsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/web-analytics/$tab',
  component: WebAnalyticsPage,
  // An unknown section used to render the dashboard under whatever URL was
  // typed, so the address bar and the highlighted sidebar entry disagreed.
  // Rewrite it instead, and replace so Back leaves the section entirely.
  beforeLoad: ({ params }) => {
    if (!(WEB_ANALYTICS_TABS as readonly string[]).includes(params.tab)) {
      throw redirect({
        to: '/console/workspace/$workspaceId/web-analytics/$tab',
        params: { workspaceId: params.workspaceId, tab: 'dashboard' },
        replace: true
      })
    }
  },
  // Without coercion a numeric-looking value arrives as a number and the
  // string handling downstream (JSON.parse, split) throws on it.
  validateSearch: (search: Record<string, unknown>): WebAnalyticsSearch => {
    const text = (value: unknown): string | undefined => {
      if (value === undefined || value === null) return undefined
      const trimmed = String(value).trim()
      return trimmed === '' ? undefined : trimmed
    }
    const period = text(search.period)
    const comparison = text(search.comparison)
    const minSessions = Number(search.minSessions)

    return {
      period:
        period && DATE_PRESETS.includes(period as DatePreset) ? (period as DatePreset) : undefined,
      timezone: text(search.timezone),
      comparison:
        comparison === 'previous_period' || comparison === 'previous_year' || comparison === 'none'
          ? (comparison as ComparisonMode)
          : undefined,
      customStart: text(search.customStart),
      customEnd: text(search.customEnd),
      filters: text(search.filters),
      metricFilters: text(search.metricFilters),
      minSessions: Number.isFinite(minSessions) && minSessions > 1 ? minSessions : undefined,
      dimensions: text(search.dimensions),
      tag: text(search.tag)
    }
  }
})

const workspaceNewSegmentRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/debug-segment',
  component: DebugSegmentPage
})

const workspaceBlogRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/blog',
  component: BlogPage,
  validateSearch: (search: Record<string, unknown>): BlogSearch => ({
    status: search.status as string | undefined,
    category_id: search.category_id as string | undefined
  })
})

// Create the router
const routeTree = rootRoute.addChildren([
  indexRoute,
  signinRoute,
  logoutRoute,
  setupRoute,
  acceptInvitationRoute,
  workspaceCreateRoute,
  workspaceRoute.addChildren([
    workspaceIndexRoute,
    workspaceBroadcastsRoute,
    workspaceAutomationsRoute,
    workspaceContactsRoute,
    workspaceListsRoute,
    workspaceTransactionalNotificationsRoute,
    workspaceLogsRoute,
    workspaceFileManagerRoute,
    workspaceSettingsRedirectRoute,
    workspaceSettingsRoute,
    workspaceTemplatesRoute,
    workspaceAnalyticsRoute,
    workspaceWebAnalyticsRedirectRoute,
    workspaceWebAnalyticsLiveRoute,
    workspaceWebAnalyticsRoute,
    workspaceNewSegmentRoute,
    workspaceBlogRoute
  ])
])

// Create and export the router with explicit type
export const router = createRouter({
  routeTree
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
