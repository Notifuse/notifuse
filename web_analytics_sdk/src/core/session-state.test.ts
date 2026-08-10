import { describe, it, expect, vi, afterEach } from 'vitest';
import { SessionState } from './session-state';
import type { SessionAttributes } from '../types/session-state';

const attrs = { landing_page: 'https://example.com/' } as SessionAttributes;

const newState = (tabId = 42) =>
  new SessionState({ workspace_id: 'ws', session_id: 'sid', created_at: Date.now(), tab_id: tabId });

describe('SessionState - writer identity (W0.1)', () => {
  it('stamps every payload with the writing tab', () => {
    // Without this the server cannot tell two tabs apart: they share a session
    // id, so both number their pages from 1 and overwrite each other's rows.
    const st = newState(987654321);
    st.addPageview('/a');
    expect(st.buildPayload(attrs).tab_id).toBe(987654321);
  });
});

describe('SessionState - clamping a stepped clock (W0.4)', () => {
  afterEach(() => vi.restoreAllMocks());

  it('never freezes a negative duration into a completed page', () => {
    // Completed pages are never recomputed — only the current one is refreshed
    // at build time — so a negative value written here poisons the beat forever.
    const st = newState();
    st.setFocusTimeGetter(() => -5000);
    st.addPageview('/a');
    st.addPageview('/b');

    expect(payloadPage(st, 0).duration).toBe(0);
  });

  it('never refreshes the current page to a negative duration', () => {
    const st = newState();
    st.setFocusTimeGetter(() => -1);
    st.addPageview('/only');

    expect(payloadPage(st, 0).duration).toBe(0);
  });

  it('never writes exited_at before entered_at', () => {
    const st = newState();
    st.addPageview('/a');

    const real = Date.now();
    vi.spyOn(Date, 'now').mockReturnValue(real - 60_000);
    st.addPageview('/b');

    const first = payloadPage(st, 0);
    expect(first.exited_at).toBeGreaterThanOrEqual(first.entered_at);
  });

  it('clamps exited_at on the unload path too', () => {
    const st = newState();
    st.addPageview('/a');

    const real = Date.now();
    vi.spyOn(Date, 'now').mockReturnValue(real - 60_000);
    st.finalizeForUnload();

    const first = payloadPage(st, 0);
    expect(first.exited_at).toBeGreaterThanOrEqual(first.entered_at);
  });
});

function payloadPage(st: SessionState, i: number) {
  const action = st.buildPayload(attrs).actions[i];
  if (action.type !== 'pageview') throw new Error('expected a pageview');
  return action;
}
