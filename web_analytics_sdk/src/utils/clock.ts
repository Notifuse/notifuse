/**
 * Monotonic time source for measuring elapsed intervals.
 *
 * Date.now() is a wall clock and steps backwards: an NTP correction after
 * suspend, a VM restore, a phone re-syncing after airplane mode. Any interval
 * computed as a Date.now() delta can therefore come out negative — and because
 * actions[] is cumulative and completed pages are never recomputed, a single
 * negative duration is re-sent on every later beat and rejected by the server
 * every time, turning one clock glitch into permanent loss for that session.
 *
 * performance.now() is monotonic by specification. Use this for every elapsed
 * measurement, and keep Date.now() for wall-clock stamps (entered_at,
 * exited_at, goal timestamps) that genuinely mean "what time was it".
 *
 * Never mix the two in one subtraction: their epochs are unrelated.
 */
export function monotonicNow(): number {
  if (typeof performance !== 'undefined' && typeof performance.now === 'function') {
    return performance.now();
  }
  return Date.now();
}
