import { describe, it, expect, vi, afterEach } from 'vitest';
import { monotonicNow } from './clock';

describe('monotonicNow (W0.4)', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('reads the monotonic clock, not the wall clock', () => {
    // Every elapsed-time measurement in the SDK was a Date.now() delta. One
    // backward wall-clock step — NTP correction after suspend, a VM restore, a
    // phone re-syncing after airplane mode — made those deltas negative, and a
    // negative duration frozen into the cumulative actions[] is rejected by the
    // server on every subsequent beat of that session, forever.
    vi.spyOn(performance, 'now').mockReturnValue(1234);
    expect(monotonicNow()).toBe(1234);
  });

  it('is unaffected by the wall clock jumping backwards', () => {
    const before = monotonicNow();
    vi.spyOn(Date, 'now').mockReturnValue(0);
    expect(monotonicNow()).toBeGreaterThanOrEqual(before);
  });

  it('falls back to a usable number when performance is unavailable', () => {
    vi.stubGlobal('performance', undefined);
    expect(typeof monotonicNow()).toBe('number');
  });
});
