import { createRootRoute, createRoute } from '@tanstack/react-router'
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
import { TransactionalNotificationsPage } from './pages/TransactionalNotificationsPage'
import { LogsPage } from './pages/LogsPage'
import { AnalyticsPage } from './pages/AnalyticsPage'
import { createRouter } from '@tanstack/react-router'

export interface ContactsSearch {
  cursor?: string
  email?: string
  external_id?: string
  first_name?: string
  last_name?: string
  phone?: string
  country?: string
  language?: string
  list_id?: string
  contact_list_status?: string
  limit?: number
}

export interface SignInSearch {
  email?: string
}

export interface AcceptInvitationSearch {
  token?: string
}

// Create the root route
const rootRoute = createRootRoute({
  component: RootLayout
})

// Create the index route
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: DashboardPage
})

// Create the signin route
const signinRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/signin',
  component: SignInPage,
  validateSearch: (search: Record<string, unknown>): SignInSearch => ({
    email: search.email as string | undefined
  })
})

// Create the logout route
const logoutRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/logout',
  component: LogoutPage
})

// Create the accept invitation route
const acceptInvitationRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/accept-invitation',
  component: AcceptInvitationPage,
  validateSearch: (search: Record<string, unknown>): AcceptInvitationSearch => ({
    token: search.token as string | undefined
  })
})

// Create the workspace create route
const workspaceCreateRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/workspace/create',
  component: CreateWorkspacePage
})

// Create the workspace route
const workspaceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/workspace/$workspaceId',
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
  component: BroadcastsPage
})

const workspaceListsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/lists',
  component: ListsPage
})

const workspaceFileManagerRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/file-manager',
  component: FileManagerPage
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
    cursor: search.cursor as string | undefined,
    email: search.email as string | undefined,
    external_id: search.external_id as string | undefined,
    first_name: search.first_name as string | undefined,
    last_name: search.last_name as string | undefined,
    phone: search.phone as string | undefined,
    country: search.country as string | undefined,
    language: search.language as string | undefined,
    list_id: search.list_id as string | undefined,
    contact_list_status: search.contact_list_status as string | undefined,
    limit: search.limit ? Number(search.limit) : 10
  })
})

const workspaceSettingsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/settings',
  component: WorkspaceSettingsPage
})

const workspaceTemplatesRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/templates',
  component: TemplatesPage
})

const workspaceAnalyticsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: '/analytics',
  component: AnalyticsPage
})

// Create the router
const routeTree = rootRoute.addChildren([
  indexRoute,
  signinRoute,
  logoutRoute,
  acceptInvitationRoute,
  workspaceCreateRoute,
  workspaceRoute.addChildren([
    workspaceIndexRoute,
    workspaceBroadcastsRoute,
    workspaceContactsRoute,
    workspaceListsRoute,
    workspaceTransactionalNotificationsRoute,
    workspaceLogsRoute,
    workspaceFileManagerRoute,
    workspaceSettingsRoute,
    workspaceTemplatesRoute,
    workspaceAnalyticsRoute
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
