import { useEffect } from "react";
import {
  createRootRoute,
  createRoute,
  redirect,
  useParams,
  useNavigate,
} from "@tanstack/react-router";
import { RootLayout } from "./layouts/RootLayout";
import { WorkspaceLayout } from "./layouts/WorkspaceLayout";
import { SignInPage } from "./pages/SignInPage";
import { LogoutPage } from "./pages/LogoutPage";
import { AcceptInvitationPage } from "./pages/AcceptInvitationPage";
import { CreateWorkspacePage } from "./pages/CreateWorkspacePage";
import { DashboardPage } from "./pages/DashboardPage";
import { WorkspaceSettingsPage } from "./pages/WorkspaceSettingsPage";
import { ContactsPage } from "./pages/ContactsPage";
import { FileManagerPage } from "./pages/FileManagerPage";
import { TemplatesPage } from "./pages/TemplatesPage";
import { BroadcastsPage } from "./pages/BroadcastsPage";
import { AutomationsPage } from "./pages/AutomationsPage";
import { TransactionalNotificationsPage } from "./pages/TransactionalNotificationsPage";
import { validateTransactionalSearch } from "./components/transactional/transactionalSearch";
import { LogsPage } from "./pages/LogsPage";
import { AnalyticsPage } from "./pages/AnalyticsPage";
import { validateDashboardSearch } from "./components/analytics/dashboardSearch";
import { WebAnalyticsPage } from "./pages/WebAnalyticsPage";
import { WebAnalyticsLivePage } from "./pages/WebAnalyticsLivePage";
import {
  DATE_PRESETS,
  type ComparisonMode,
  type DatePreset,
} from "./components/web_analytics/lib/types";
import {
  WEB_ANALYTICS_TABS,
  type WebAnalyticsSearch,
} from "./components/web_analytics/context";
import { DebugSegmentPage } from "./pages/DebugSegmentPage";
import { BlogPage } from "./pages/BlogPage";
import SetupWizard from "./pages/SetupWizard";
import { createRouter } from "@tanstack/react-router";
import type { TrackingConsentFilter } from "./services/api/contacts";
import {
  parseBlogSearch,
  parseSettingsSearch,
  parseTemplatesSearch,
} from "./lib/searchParams";
import { DEFAULT_SETTINGS_SECTION } from "./components/settings/SettingsSidebar";

export interface ContactsSearch {
  email?: string;
  external_id?: string;
  first_name?: string;
  last_name?: string;
  full_name?: string;
  phone?: string;
  country?: string;
  language?: string;
  list_id?: string;
  contact_list_status?: string;
  tracking_opens_consent?: TrackingConsentFilter;
  tracking_clicks_consent?: TrackingConsentFilter;
  /** Legacy filter: contacts in any of these segments (a chip). */
  segments?: string[];
  /** The segment view; sent to the API as segments=[segment_id]. */
  segment_id?: string;
  /** Unified search on email, names and external ID. */
  search?: string;
  /** Email of the contact whose sheet is open. */
  contact?: string;
  limit?: number;
}

const CONTACTS_DEFAULT_LIMIT = 50;
const CONTACTS_MAX_LIMIT = 10000;

// The default search parser JSON.parses each value, so ?search=123 arrives as a number and
// ?x=a&x=b as an array: every contacts parameter is read back as one string.
function searchText(value: unknown): string | undefined {
  const single = Array.isArray(value) ? value[0] : value;
  if (single === undefined || single === null) return undefined;
  const text = String(single);
  return text.trim() === "" ? undefined : text;
}

function contactsLimit(value: unknown): number {
  const limit = Math.floor(Number(value));
  if (!Number.isFinite(limit) || limit < 1) return CONTACTS_DEFAULT_LIMIT;
  return Math.min(limit, CONTACTS_MAX_LIMIT);
}

const trackingConsentFilters: readonly TrackingConsentFilter[] = ["granted", "denied", "unknown"];

// An unknown value would make the API refuse the whole list request.
function trackingConsentFilter(value: unknown): TrackingConsentFilter | undefined {
  return trackingConsentFilters.find((v) => v === value);
}

export interface SignInSearch {
  email?: string;
  // OIDC failure flag (non-secret enum). The success one-time code arrives in the
  // URL fragment (#oidc_code=…), which is NOT a search param, so it is not listed here.
  oidc_error?: string;
}

export interface AcceptInvitationSearch {
  token?: string;
}

export interface FileManagerSearch {
  path?: string;
}

export interface BroadcastsSearch {
  status?: string;
  q?: string;
  /** Opens the detail page of this broadcast. */
  broadcast?: string;
  /** Opens the composer: `new`, or the id of a draft or scheduled broadcast. */
  compose?: string;
}

/**
 * Repeated query keys (?status=a&status=b) parse to arrays; coerce to a single value, trim, and
 * drop empties so the page always sees a clean string or undefined. An identifier made of digits
 * is parsed as a number by the router, and read back as text.
 */
export function validateBroadcastsSearch(search: Record<string, unknown>): BroadcastsSearch {
  const normalize = (value: unknown, numbers = false): string | undefined => {
    const single = Array.isArray(value) ? value[0] : value;
    const text = numbers && typeof single === "number" && Number.isFinite(single) ? String(single) : single;
    if (typeof text !== "string") return undefined;
    const trimmed = text.trim();
    return trimmed === "" ? undefined : trimmed;
  };
  return {
    status: normalize(search.status),
    q: normalize(search.q),
    broadcast: normalize(search.broadcast, true),
    compose: normalize(search.compose, true),
  };
}

export type { BlogSearch, TemplatesSearch } from "./lib/searchParams";

// Create the root route
const rootRoute = createRootRoute({
  component: RootLayout,
});

// Create the index route
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/console/",
  component: DashboardPage,
});

// Create the signin route
const signinRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/console/signin",
  component: SignInPage,
  validateSearch: (search: Record<string, unknown>): SignInSearch => ({
    email: search.email as string | undefined,
    oidc_error:
      typeof search.oidc_error === "string" ? search.oidc_error : undefined,
  }),
});

// Create the logout route
const logoutRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/console/logout",
  component: LogoutPage,
});

// Create the setup wizard route
const setupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/console/setup",
  component: SetupWizard,
});

// Create the accept invitation route
const acceptInvitationRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/console/accept-invitation",
  component: AcceptInvitationPage,
  validateSearch: (
    search: Record<string, unknown>,
  ): AcceptInvitationSearch => ({
    token: search.token as string | undefined,
  }),
});

// Create the workspace create route
const workspaceCreateRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/console/workspace/create",
  component: CreateWorkspacePage,
});

// Create the workspace route
const workspaceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/console/workspace/$workspaceId",
  component: WorkspaceLayout,
});

// Create the default workspace route (redirects to analytics/dashboard)
const workspaceIndexRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/",
  component: AnalyticsPage,
  validateSearch: validateDashboardSearch,
});

// Create workspace child routes
const workspaceBroadcastsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/broadcasts",
  component: BroadcastsPage,
  validateSearch: validateBroadcastsSearch,
});

const workspaceAutomationsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/automations",
  component: AutomationsPage,
});

// The lists live in the Contacts page's rail now.
const workspaceListsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/lists",
  staticData: { layout: "rail" },
  beforeLoad: ({ params }) => {
    throw redirect({
      to: "/console/workspace/$workspaceId/contacts",
      params: { workspaceId: params.workspaceId },
      replace: true,
    });
  },
});

export const workspaceFileManagerRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/file-manager",
  component: FileManagerPage,
  validateSearch: (search: Record<string, unknown>): FileManagerSearch => ({
    path: search.path as string | undefined,
  }),
});

const workspaceTransactionalNotificationsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/transactional-notifications",
  component: TransactionalNotificationsPage,
  validateSearch: validateTransactionalSearch,
});

// The tab is typed; the message tab's own filters (is_failed and friends) pass
// through untouched, since that tab reads them straight off the location.
export type LogsSearch = { tab?: string } & Record<string, unknown>;

const workspaceLogsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/logs",
  component: LogsPage,
  validateSearch: (search: Record<string, unknown>): LogsSearch => ({
    ...search,
    tab: typeof search.tab === "string" ? search.tab : undefined,
  }),
});

export const workspaceContactsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/contacts",
  component: ContactsPage,
  staticData: { layout: "rail" },
  validateSearch: (search: Record<string, unknown>): ContactsSearch => {
    const listId = searchText(search.list_id);
    const segments = (Array.isArray(search.segments) ? search.segments : [search.segments])
      .map(searchText)
      .filter((id): id is string => id !== undefined);
    return {
      email: searchText(search.email),
      external_id: searchText(search.external_id),
      first_name: searchText(search.first_name),
      last_name: searchText(search.last_name),
      full_name: searchText(search.full_name),
      phone: searchText(search.phone),
      country: searchText(search.country),
      language: searchText(search.language),
      list_id: listId,
      contact_list_status: searchText(search.contact_list_status),
      tracking_opens_consent: trackingConsentFilter(search.tracking_opens_consent),
      tracking_clicks_consent: trackingConsentFilter(search.tracking_clicks_consent),
      segments: segments.length > 0 ? segments : undefined,
      // A list and a segment are both views: the list wins.
      segment_id: listId ? undefined : searchText(search.segment_id),
      search: searchText(search.search),
      contact: searchText(search.contact),
      limit: contactsLimit(search.limit),
    };
  },
  // A link with one segment and no other view (the former segment filter) opens that
  // segment's view.
  beforeLoad: ({ params, search }) => {
    if (search.segments?.length === 1 && !search.list_id && !search.segment_id) {
      throw redirect({
        to: "/console/workspace/$workspaceId/contacts",
        params: { workspaceId: params.workspaceId },
        search: { ...search, segments: undefined, segment_id: search.segments[0] },
        replace: true,
      });
    }
  },
});

// eslint-disable-next-line react-refresh/only-export-components -- Internal redirect component
const WorkspaceSettingsRedirect = () => {
  const { workspaceId } = useParams({
    from: "/console/workspace/$workspaceId/settings",
  });
  const navigate = useNavigate();

  useEffect(() => {
    navigate({
      to: "/console/workspace/$workspaceId/settings/$section",
      params: { workspaceId, section: DEFAULT_SETTINGS_SECTION },
      replace: true,
    });
  }, [workspaceId, navigate]);

  return null;
};

const workspaceSettingsRedirectRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/settings",
  component: WorkspaceSettingsRedirect,
  // Declared on the redirect too, so the expanded sidebar and the page margin do not flash
  // before the first section takes over.
  staticData: { layout: "rail" },
});

const workspaceSettingsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/settings/$section",
  component: WorkspaceSettingsPage,
  validateSearch: parseSettingsSearch,
  staticData: { layout: "rail" },
});

const workspaceTemplatesRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/templates",
  component: TemplatesPage,
  validateSearch: parseTemplatesSearch,
});

const workspaceAnalyticsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/analytics",
  component: AnalyticsPage,
  validateSearch: validateDashboardSearch,
});

// eslint-disable-next-line react-refresh/only-export-components -- Internal redirect component
const WebAnalyticsRedirect = () => {
  const { workspaceId } = useParams({
    from: "/console/workspace/$workspaceId/web-analytics",
  });
  const navigate = useNavigate();

  useEffect(() => {
    navigate({
      to: "/console/workspace/$workspaceId/web-analytics/$tab",
      params: { workspaceId, tab: "dashboard" },
      replace: true,
    });
  }, [workspaceId, navigate]);

  return null;
};

const workspaceWebAnalyticsRedirectRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/web-analytics",
  component: WebAnalyticsRedirect,
});

const workspaceWebAnalyticsLiveRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/web-analytics/live",
  component: WebAnalyticsLivePage,
});

const workspaceWebAnalyticsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/web-analytics/$tab",
  component: WebAnalyticsPage,
  // An unknown section used to render the dashboard under whatever URL was
  // typed, so the address bar and the highlighted sidebar entry disagreed.
  // Rewrite it instead, and replace so Back leaves the section entirely.
  beforeLoad: ({ params }) => {
    if (!(WEB_ANALYTICS_TABS as readonly string[]).includes(params.tab)) {
      throw redirect({
        to: "/console/workspace/$workspaceId/web-analytics/$tab",
        params: { workspaceId: params.workspaceId, tab: "dashboard" },
        replace: true,
      });
    }
  },
  // Without coercion a numeric-looking value arrives as a number and the
  // string handling downstream (JSON.parse, split) throws on it.
  validateSearch: (search: Record<string, unknown>): WebAnalyticsSearch => {
    const text = (value: unknown): string | undefined => {
      if (value === undefined || value === null) return undefined;
      const trimmed = String(value).trim();
      return trimmed === "" ? undefined : trimmed;
    };
    const period = text(search.period);
    const comparison = text(search.comparison);
    const minSessions = Number(search.minSessions);

    return {
      period:
        period && DATE_PRESETS.includes(period as DatePreset)
          ? (period as DatePreset)
          : undefined,
      timezone: text(search.timezone),
      comparison:
        comparison === "previous_period" ||
        comparison === "previous_year" ||
        comparison === "none"
          ? (comparison as ComparisonMode)
          : undefined,
      customStart: text(search.customStart),
      customEnd: text(search.customEnd),
      filters: text(search.filters),
      metricFilters: text(search.metricFilters),
      minSessions:
        Number.isFinite(minSessions) && minSessions > 1
          ? minSessions
          : undefined,
      dimensions: text(search.dimensions),
      tag: text(search.tag),
    };
  },
});

const workspaceNewSegmentRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/debug-segment",
  component: DebugSegmentPage,
});

const workspaceBlogRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/blog",
  component: BlogPage,
  staticData: { layout: "rail" },
  validateSearch: parseBlogSearch,
});

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
    workspaceBlogRoute,
  ]),
]);

// Create and export the router with explicit type
export const router = createRouter({
  routeTree,
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
  interface StaticDataRouteOption {
    /**
     * The page template WorkspaceLayout lays the route out with: `page` (the default) gets the
     * page padding and the stored sidebar; `rail`, a page with its own left column (Settings,
     * Blog), gets no padding and a folded sidebar.
     */
    layout?: "page" | "rail";
  }
}
