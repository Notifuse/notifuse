/**
 * Client for the E2E track collector (helpers/collector-server.ts).
 *
 * Replaces the ClickHouse helper the SDK carried over from Staminads: Notifuse
 * stores no raw event rows — sessions, pages and goals go into three separate
 * Postgres tables — so there is nothing equivalent to query, and the server
 * pipeline is covered by the Go integration suite instead.
 */

const COLLECTOR_URL = process.env.COLLECTOR_URL || 'http://localhost:4555';
const WORKSPACE_ID = 'test_workspace';

/**
 * One row per action the SDK sent. Every field comes from the payload itself:
 * nothing here is server-derived, so an assertion on this record is always an
 * assertion about the SDK.
 */
export interface EventRecord {
  /** Action type as the SDK labels it: 'pageview' or 'goal'. */
  name: string;
  session_id: string;
  workspace_id: string;
  sdk_version: string;
  beat_seq: number;
  /** How many beats the collector accepted for this session. */
  beats_received: number;
  path: string;
  page_number: number;
  duration: number;
  max_scroll: number;
  goal_name: string;
  goal_value: number;
  user_id: string | null;
  dimensions: Record<string, string>;
  created_at: number;
  updated_at: number;
  received_at: number;
  // Session attributes, as sent (device/browser/os are parsed in the browser).
  device?: string;
  browser?: string;
  browser_type?: string;
  os?: string;
  user_agent?: string;
  language?: string;
  timezone?: string;
  connection_type?: string;
  referrer?: string;
  landing_page?: string;
  utm_source?: string;
  utm_medium?: string;
  utm_campaign?: string;
  screen_width?: number;
  screen_height?: number;
  viewport_width?: number;
  viewport_height?: number;
  [key: string]: unknown;
}

/** Drop everything the collector holds. */
export async function truncateEvents(): Promise<void> {
  await fetch(`${COLLECTOR_URL}/__reset`, { method: 'POST' });
}

/** Kept for parity with the specs' vocabulary: the collector has one store. */
export const truncateWorkspaceTables = truncateEvents;

export async function queryEvents(sessionId?: string): Promise<EventRecord[]> {
  const url = sessionId
    ? `${COLLECTOR_URL}/__events?session_id=${encodeURIComponent(sessionId)}`
    : `${COLLECTOR_URL}/__events`;
  const res = await fetch(url);
  return (await res.json()) as EventRecord[];
}

export async function countEvents(sessionId?: string): Promise<number> {
  return (await queryEvents(sessionId)).length;
}

/** Beats accepted vs rejected by the collector's `seq` guard. */
export async function collectorStats(): Promise<{
  sessions: number;
  beats: number;
  rejectedBySeq: number;
}> {
  const res = await fetch(`${COLLECTOR_URL}/__stats`);
  return res.json() as Promise<{ sessions: number; beats: number; rejectedBySeq: number }>;
}

/**
 * Poll until the session has at least `expectedCount` actions.
 *
 * Returns whatever arrived on timeout; the caller asserts on the count, so a
 * silent shortfall still fails the test rather than being swallowed here.
 */
export async function waitForEvents(
  sessionId: string,
  expectedCount: number,
  timeoutMs: number = 10000,
  intervalMs: number = 200
): Promise<EventRecord[]> {
  const deadline = Date.now() + timeoutMs;

  while (Date.now() < deadline) {
    const events = await queryEvents(sessionId);
    if (events.length >= expectedCount) {
      return events;
    }
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
  }

  return queryEvents(sessionId);
}

export function getTestWorkspaceId(): string {
  return WORKSPACE_ID;
}
