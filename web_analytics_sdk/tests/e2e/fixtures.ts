/**
 * Custom Playwright fixtures for E2E tests
 *
 * Includes stealth mode to bypass SDK bot detection.
 * Updated for V3 SessionPayload format.
 * Beats are read back from the in-memory collector (helpers/collector.ts).
 */

import { test as base, expect, devices } from '@playwright/test';

// Re-export collector helpers for use in tests
export {
  truncateEvents,
  truncateWorkspaceTables,
  queryEvents,
  countEvents,
  waitForEvents,
  collectorStats,
  getTestWorkspaceId,
  type EventRecord,
} from './helpers/collector';

// V3 SessionPayload types (matches SDK types/session-state.ts)
export interface PageviewAction {
  type: 'pageview';
  path: string;
  page_number: number;
  duration: number;
  scroll: number;
  entered_at: number;
  exited_at: number;
}

export interface GoalAction {
  type: 'goal';
  name: string;
  path: string;
  page_number: number;
  timestamp: number;
  value?: number;
  properties?: Record<string, string>;
}

export type Action = PageviewAction | GoalAction;

export interface CurrentPage {
  path: string;
  page_number: number;
  entered_at: number;
  scroll: number;
}

export interface SessionAttributes {
  referrer?: string;
  landing_page: string;
  utm_source?: string;
  utm_medium?: string;
  utm_campaign?: string;
  utm_term?: string;
  utm_content?: string;
  utm_id?: string;
  utm_id_from?: string;
  screen_width?: number;
  screen_height?: number;
  viewport_width?: number;
  viewport_height?: number;
  device?: string;
  browser?: string;
  browser_type?: string;
  os?: string;
  user_agent?: string;
  connection_type?: string;
  language?: string;
  timezone?: string;
}

export interface SessionPayload {
  workspace_id: string;
  session_id: string;
  actions: Action[];
  current_page?: CurrentPage;
  checkpoint?: number;
  attributes?: SessionAttributes;
  created_at: number;
  updated_at: number;
  sdk_version: string;
  sent_at?: number; // Set at HTTP send time for clock skew detection
}

export interface CapturedPayload {
  payload: SessionPayload;
  _received_at: number;
  _raw_body: string;
}

// The window global is typed with the SDK's own public interface rather than a
// copy of it. A hand-maintained duplicate silently kept compiling after the API
// changed, so the specs only failed once a browser ran them.
export type { NotifuseAnalyticsAPI } from '../../src/types';

// Declare global types for browser context.
// Window.NotifuseAnalyticsConfig and Window.chrome are already augmented by the
// SDK sources, so only the fixture pages' own globals are declared here.
declare global {
  interface Window {
    SDK_READY: Promise<void>;
    SDK_INITIALIZED: boolean;
  }
  const NotifuseAnalytics: import('../../src/types').NotifuseAnalyticsAPI;
}

const stealthScript = `
  // Delete webdriver property
  Object.defineProperty(navigator, 'webdriver', {
    get: () => undefined,
  });

  // Ensure plugins exist (Chrome-like)
  if (!navigator.plugins || navigator.plugins.length === 0) {
    Object.defineProperty(navigator, 'plugins', {
      get: () => [1, 2, 3, 4, 5],
    });
  }

  // Ensure languages exist
  if (!navigator.languages || navigator.languages.length === 0) {
    Object.defineProperty(navigator, 'languages', {
      get: () => ['en-US', 'en'],
    });
  }

  // Add chrome object if missing
  if (!window.chrome) {
    window.chrome = {
      runtime: {},
    };
  }
`;

// Extend base test to auto-apply stealth script
export const test = base.extend({
  page: async ({ page }, use) => {
    // Add stealth script before each navigation
    await page.addInitScript(stealthScript);
    await use(page);
  },
});

export { expect, devices };

// Helper functions for V3 payload assertions

/**
 * Get the latest goal action from a payload
 */
export function getLatestGoal(payload: SessionPayload): GoalAction | undefined {
  const goals = payload.actions.filter((a): a is GoalAction => a.type === 'goal');
  return goals[goals.length - 1];
}

/**
 * Get all goals from a payload
 */
export function getGoals(payload: SessionPayload): GoalAction[] {
  return payload.actions.filter((a): a is GoalAction => a.type === 'goal');
}

/**
 * Get all pageviews from a payload
 */
export function getPageviews(payload: SessionPayload): PageviewAction[] {
  return payload.actions.filter((a): a is PageviewAction => a.type === 'pageview');
}

/**
 * Get total duration from completed pageviews
 */
export function getTotalPageviewDuration(payload: SessionPayload): number {
  return getPageviews(payload).reduce((sum, pv) => sum + pv.duration, 0);
}

/**
 * Check if payload has a goal with given name
 */
export function hasGoal(payload: SessionPayload, name: string): boolean {
  return payload.actions.some((a) => a.type === 'goal' && a.name === name);
}

/**
 * Check if payload has a pageview with given path
 */
export function hasPageview(payload: SessionPayload, path: string): boolean {
  return payload.actions.some((a) => a.type === 'pageview' && a.path === path);
}
