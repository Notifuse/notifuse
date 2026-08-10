/**
 * Test collector — stands in for Notifuse's POST /track during E2E runs.
 *
 * These specs verify SDK behaviour across real browsers and devices: what the
 * SDK sends, when it sends it, and how it survives reloads, offline periods and
 * clock skew. They do not verify the server pipeline — the Go suite does that
 * against real Postgres (tests/integration/web_analytics_browser_test.go).
 *
 * So this collector only records beats and exposes them back. It deliberately
 * does NOT re-implement server-side enrichment (attribution, GeoIP, URL
 * parsing): asserting on values invented here would be coverage of this file
 * rather than of the SDK.
 *
 * It does mirror one server rule, because the SDK depends on it: a beat is only
 * applied when its `seq` is strictly greater than the stored one.
 */

import express from 'express';

// 4555, not 4000: a local Notifuse dev server commonly holds 4000, and
// Playwright would silently reuse it instead of this collector.
const PORT = Number(process.env.COLLECTOR_PORT || 4555);

interface StoredBeat {
  payload: Record<string, unknown>;
  receivedAt: number;
  beats: number;
}

/** Latest accepted beat per session, exactly like the server's upsert. */
const sessions = new Map<string, StoredBeat>();
let rejectedBySeq = 0;

const app = express();

// The SDK sends text/plain to stay a CORS "simple request" and avoid a
// preflight on every beat, so the body must be parsed regardless of type.
app.use(express.text({ type: '*/*', limit: '1mb' }));

app.use((req, res, next) => {
  const origin = req.headers.origin;
  res.setHeader('Access-Control-Allow-Origin', origin || '*');
  res.setHeader('Access-Control-Allow-Methods', 'POST, GET, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type');
  if (req.method === 'OPTIONS') {
    res.sendStatus(204);
    return;
  }
  next();
});

app.post('/track', (req, res) => {
  let payload: Record<string, unknown>;
  try {
    payload = JSON.parse(typeof req.body === 'string' ? req.body : String(req.body));
  } catch {
    // The real endpoint answers 200 to anything so a broken beat never
    // surfaces in a customer's console.
    res.json({ success: true });
    return;
  }

  const sessionId = String(payload.session_id ?? '');
  const seq = Number(payload.seq ?? 0);

  if (sessionId) {
    const existing = sessions.get(sessionId);
    if (existing && seq <= Number(existing.payload.seq ?? 0)) {
      rejectedBySeq++;
    } else {
      sessions.set(sessionId, {
        payload,
        receivedAt: Date.now(),
        beats: (existing?.beats ?? 0) + 1,
      });
    }
  }

  res.json({ success: true });
});

/** Flatten a stored session into one row per action. */
function flatten(stored: StoredBeat): Record<string, unknown>[] {
  const p = stored.payload;
  const attributes = (p.attributes as Record<string, unknown>) ?? {};
  const actions = (p.actions as Record<string, unknown>[]) ?? [];

  const base = {
    ...attributes,
    session_id: p.session_id,
    workspace_id: p.workspace_id,
    sdk_version: p.sdk_version,
    beat_seq: p.seq,
    beats_received: stored.beats,
    created_at: p.created_at,
    updated_at: p.updated_at,
    received_at: stored.receivedAt,
    user_id: p.user_id ?? null,
    dimensions: p.dimensions ?? {},
  };

  return actions.map((action) =>
    action.type === 'goal'
      ? {
          ...base,
          name: 'goal',
          goal_name: action.name,
          goal_value: action.value ?? 0,
          goal_timestamp: action.timestamp,
          properties: action.properties ?? {},
          path: action.path,
          page_number: action.page_number,
          duration: 0,
          max_scroll: 0,
        }
      : {
          ...base,
          name: 'pageview',
          path: action.path,
          page_number: action.page_number,
          duration: action.duration,
          max_scroll: action.scroll,
          entered_at: action.entered_at,
          exited_at: action.exited_at,
          goal_name: '',
          goal_value: 0,
        }
  );
}

app.get('/__events', (req, res) => {
  const sessionId = req.query.session_id as string | undefined;

  const stored = sessionId
    ? [sessions.get(sessionId)].filter((s): s is StoredBeat => Boolean(s))
    : [...sessions.values()].sort((a, b) => a.receivedAt - b.receivedAt);

  const rows = stored.flatMap(flatten);
  rows.sort((a, b) => {
    const byPage = Number(a.page_number ?? 0) - Number(b.page_number ?? 0);
    return byPage !== 0 ? byPage : String(a.name).localeCompare(String(b.name));
  });

  res.json(rows);
});

app.get('/__stats', (_req, res) => {
  res.json({
    sessions: sessions.size,
    rejectedBySeq,
    beats: [...sessions.values()].reduce((n, s) => n + s.beats, 0),
  });
});

app.post('/__reset', (_req, res) => {
  sessions.clear();
  rejectedBySeq = 0;
  res.json({ success: true });
});

app.get('/health', (_req, res) => {
  res.json({ status: 'ok' });
});

app.listen(PORT, () => {
  console.log(`Track collector running on http://localhost:${PORT}`);
});
