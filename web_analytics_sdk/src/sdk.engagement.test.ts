import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { NotifuseAnalyticsSDK } from './sdk';

/**
 * Engagement time is the product's core metric: it feeds session duration,
 * bounce rate and the per-page focus time the server sums. These tests pin the
 * rule that every one of those numbers depends on — time on a page keeps
 * counting whenever the visitor is actually looking at it, and never resets.
 */

function setVisibility(state: 'visible' | 'hidden'): void {
  Object.defineProperty(document, 'visibilityState', { value: state, configurable: true });
  document.dispatchEvent(new Event('visibilitychange'));
}

async function initSdk(): Promise<NotifuseAnalyticsSDK> {
  const sdk = new NotifuseAnalyticsSDK();
  await sdk.init({
    workspace_id: 'ws_engagement',
    endpoint: 'https://collector.example.com',
  });
  return sdk;
}

describe('engagement time', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    setVisibility('visible');
  });

  afterEach(() => {
    vi.useRealTimers();
    delete (document as unknown as Record<string, unknown>).visibilityState;
  });

  it('keeps counting the current page after the visitor comes back to the tab', async () => {
    const sdk = await initSdk();

    await vi.advanceTimersByTimeAsync(1000);
    expect(await sdk.getFocusDuration()).toBeGreaterThanOrEqual(1000);

    // Visitor switches to another tab for half a second...
    setVisibility('hidden');
    await vi.advanceTimersByTimeAsync(500);

    // ...and comes back. This is the single most common thing a visitor does,
    // so the page they return to must resume counting.
    setVisibility('visible');
    await vi.advanceTimersByTimeAsync(1500);

    expect(sdk.debug().currentPage).not.toBeNull();

    const focus = await sdk.getFocusDuration();
    expect(focus).toBeGreaterThanOrEqual(2400); // 1000 before + 1500 after
    expect(focus).toBeLessThan(3000); // the 500ms spent hidden must not count
  });

  it('resume() keeps the time accumulated before pause()', async () => {
    const sdk = await initSdk();

    await vi.advanceTimersByTimeAsync(2000);

    await sdk.pause();
    await vi.advanceTimersByTimeAsync(1000); // paused time must not count
    await sdk.resume();

    await vi.advanceTimersByTimeAsync(500);

    const focus = await sdk.getFocusDuration();
    expect(focus).toBeGreaterThanOrEqual(2400); // 2000 before pause + 500 after
    expect(focus).toBeLessThan(3000);
  });

  it('resume() re-arms tracking once the heartbeat cap has been reached', async () => {
    const sdk = await initSdk();

    // Past heartbeatMaxDuration (10 min): the SDK stops pinging on purpose.
    await vi.advanceTimersByTimeAsync(11 * 60 * 1000);

    const cappedCalls = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls.length;
    await vi.advanceTimersByTimeAsync(2 * 60 * 1000);
    expect((globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls.length).toBe(
      cappedCalls
    );

    // An explicit resume() is the documented way to start tracking again.
    await sdk.resume();
    await vi.advanceTimersByTimeAsync(60 * 1000);

    expect(
      (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls.length
    ).toBeGreaterThan(cappedCalls);
  });
});
