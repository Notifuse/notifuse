import { describe, it, expect, afterEach, vi } from 'vitest';
import { ScrollTracker } from './scroll';

/**
 * These tests reproduce a snippet installed without `async`: the SDK then runs
 * from <head> while the parser has not created <body> yet.
 */
function setReadyState(state: DocumentReadyState): void {
  Object.defineProperty(document, 'readyState', { value: state, configurable: true });
}

function removeBody(): void {
  Object.defineProperty(document, 'body', { get: () => null, configurable: true });
}

function restoreDocument(): void {
  delete (document as unknown as Record<string, unknown>).readyState;
  delete (document as unknown as Record<string, unknown>).body;
}

describe('ScrollTracker', () => {
  afterEach(() => {
    restoreDocument();
  });

  it('starts without throwing while the document is still parsing', () => {
    setReadyState('loading');
    removeBody();

    const tracker = new ScrollTracker();
    expect(() => tracker.start()).not.toThrow();
    tracker.stop();
  });

  it('defers the initial measurement until the document is parsed', () => {
    setReadyState('loading');
    removeBody();
    const onMilestone = vi.fn();

    const tracker = new ScrollTracker();
    tracker.setMilestoneCallback(onMilestone);
    tracker.start();

    // A page shorter than the viewport counts as fully seen, but that can only
    // be concluded once it is parsed — deciding it from <head> would credit
    // every single visit with a 100% read and a completed milestone.
    expect(tracker.getMaxScrollPercent()).toBe(0);
    expect(onMilestone).not.toHaveBeenCalled();

    restoreDocument();
    document.dispatchEvent(new Event('DOMContentLoaded'));

    expect(tracker.getMaxScrollPercent()).toBe(100);
    tracker.stop();
  });

  it('measures immediately when the document is already parsed', () => {
    setReadyState('complete');

    const tracker = new ScrollTracker();
    tracker.start();

    expect(tracker.getMaxScrollPercent()).toBe(100);
    tracker.stop();
  });

  it('ignores a measurement taken while the body is missing', () => {
    setReadyState('complete');
    removeBody();

    const tracker = new ScrollTracker();
    tracker.start();

    expect(tracker.getMaxScrollPercent()).toBe(0);
    tracker.stop();
  });

  it('drops the pending initial measurement when stopped early', () => {
    setReadyState('loading');
    removeBody();

    const tracker = new ScrollTracker();
    tracker.start();
    tracker.stop();

    restoreDocument();
    document.dispatchEvent(new Event('DOMContentLoaded'));

    expect(tracker.getMaxScrollPercent()).toBe(0);
  });
});
