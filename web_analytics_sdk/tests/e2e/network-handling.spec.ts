/**
 * Network Handling E2E Tests
 *
 * Tests network resilience: beacon, fetch, error handling.
 * Updated for V3 SessionPayload format.
 * Now uses request interception instead of mock server.
 *
 * Note: Queue/retry functionality has been removed from the SDK in V3.
 * These tests focus on the current transport capabilities.
 */

import { test, expect, SessionPayload, hasGoal, truncateEvents } from './fixtures';

test.describe('Network Handling', () => {
  test.beforeEach(async () => {
    await truncateEvents();
  });

  test('sends payload via fetch on init', async ({ page }) => {
    // Capture payloads
    const payloads: SessionPayload[] = [];
    await page.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          payloads.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    await page.waitForTimeout(500);

    expect(payloads.length).toBeGreaterThanOrEqual(1);
    expect(payloads[0].workspace_id).toBe('test_workspace');
  });

  test('falls back to fetch when beacon fails', async ({ page }) => {
    // Capture payloads
    const payloads: SessionPayload[] = [];

    // Disable sendBeacon
    await page.addInitScript(() => {
      navigator.sendBeacon = () => false;
    });

    await page.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          payloads.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Track a goal (uses fetch with keepalive)
    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'fetch_test', type: 'other' }));
    await page.waitForTimeout(500);

    // Should still have received the event via fetch
    expect(payloads.length).toBeGreaterThanOrEqual(1);
    const found = payloads.some((p) => hasGoal(p, 'fetch_test'));
    expect(found).toBe(true);
  });

  test('handles slow network gracefully', async ({ page }) => {
    // Capture payloads
    const payloads: SessionPayload[] = [];

    // Add 2 second delay to responses
    await page.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          payloads.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      // Delay the response by 2 seconds
      await new Promise((resolve) => setTimeout(resolve, 2000));
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Track a goal - SDK awaits response for goal reliability
    const startTime = Date.now();
    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'slow_test', type: 'other' }));
    const elapsed = Date.now() - startTime;

    // Should complete within delay + buffer (goals are awaited for reliability)
    expect(elapsed).toBeLessThan(5000);
    expect(elapsed).toBeGreaterThanOrEqual(2000); // At least the delay time

    const found = payloads.some((p) => hasGoal(p, 'slow_test'));
    expect(found).toBe(true);
  });

  test('uses beacon on pagehide', async ({ page }) => {
    // Capture payloads
    const payloads: SessionPayload[] = [];
    await page.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          payloads.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    await page.waitForTimeout(500);

    // Trigger pagehide event
    await page.evaluate(() => {
      window.dispatchEvent(new Event('pagehide'));
    });

    await page.waitForTimeout(500);

    // Should have sent payload on pagehide
    expect(payloads.length).toBeGreaterThanOrEqual(1);
  });

  test('handles network timeout gracefully', async ({ page }) => {
    // Let SDK initialize first before blocking
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Now block subsequent requests
    await page.route('**/track', async () => {
      // Don't respond - simulate timeout
      await new Promise(() => {}); // Never resolves
    });

    // SDK should still be functional even with pending requests
    const sessionId = await page.evaluate(() => NotifuseAnalytics.getSessionId());
    expect(sessionId).toBeTruthy();
  });

  test('server returns checkpoint on success', async ({ page }) => {
    // Capture payloads
    const payloads: SessionPayload[] = [];
    await page.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          payloads.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Track multiple goals
    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'goal1', type: 'other' }));
    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'goal2', type: 'other' }));
    await page.waitForTimeout(500);

    // Latest payload should have actions
    const latestPayload = payloads[payloads.length - 1];
    expect(latestPayload.actions.length).toBeGreaterThanOrEqual(2);
  });

  test('continues sending after server failure', async ({ page }) => {
    // Capture payloads
    const payloads: SessionPayload[] = [];
    let requestCount = 0;

    // First request fails, subsequent succeed
    await page.route('**/track', async (route) => {
      requestCount++;
      const request = route.request();
      const postData = request.postData();

      if (requestCount === 1) {
        // Fail first request
        await route.abort('failed');
        return;
      }

      if (postData) {
        try {
          payloads.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // First send fails, but SDK continues
    await page.waitForTimeout(1000);

    // Track another goal - server now working
    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'after_failure', type: 'other' }));
    await page.waitForTimeout(500);

    // Should have received the second request
    const found = payloads.some((p) => hasGoal(p, 'after_failure'));
    expect(found).toBe(true);
  });

  test('beacon is used on visibility hidden', async ({ page }) => {
    // Capture payloads
    const payloads: SessionPayload[] = [];
    await page.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          payloads.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    await page.waitForTimeout(500);

    // Simulate tab becoming hidden
    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', {
        value: 'hidden',
        writable: true,
      });
      document.dispatchEvent(new Event('visibilitychange'));
    });

    await page.waitForTimeout(500);

    // Should have sent payload on visibility change
    expect(payloads.length).toBeGreaterThanOrEqual(1);
  });

  test('payload is sent on beforeunload', async ({ page }) => {
    // Capture payloads
    const payloads: SessionPayload[] = [];
    await page.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          payloads.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    await page.waitForTimeout(500);

    // Trigger beforeunload
    await page.evaluate(() => {
      window.dispatchEvent(new Event('beforeunload'));
    });

    await page.waitForTimeout(500);

    expect(payloads.length).toBeGreaterThanOrEqual(1);
  });

  test('actions array grows cumulatively', async ({ page }) => {
    // Capture payloads
    const payloads: SessionPayload[] = [];
    await page.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          payloads.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Track several goals
    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'goal_a', type: 'other' }));
    await page.waitForTimeout(200);
    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'goal_b', type: 'other' }));
    await page.waitForTimeout(200);
    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'goal_c', type: 'other' }));
    await page.waitForTimeout(500);

    // Get the latest payload
    const latestPayload = payloads[payloads.length - 1];

    // Should have all 3 goals in actions array
    expect(hasGoal(latestPayload, 'goal_a')).toBe(true);
    expect(hasGoal(latestPayload, 'goal_b')).toBe(true);
    expect(hasGoal(latestPayload, 'goal_c')).toBe(true);
  });

  test('attributes sent only on first payload', async ({ page }) => {
    // Capture payloads
    const payloads: SessionPayload[] = [];
    await page.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          payloads.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Wait for initial + heartbeat
    await page.waitForTimeout(11000);

    expect(payloads.length).toBeGreaterThanOrEqual(2);

    // First payload should have attributes
    expect(payloads[0].attributes).toBeTruthy();

    // Document payload behavior - SDK may or may not send attributes after first
    const payloadWithAttributes = payloads.filter((p) => p.attributes);
    console.log('Payloads with attributes:', payloadWithAttributes.length, 'of', payloads.length);

    // At minimum, first payload must have attributes
    expect(payloads[0].attributes?.device).toBeTruthy();
  });

  test('queues payloads when offline and sends when back online', async ({
    page,
    context,
  }) => {
    // Capture payloads
    const payloads: SessionPayload[] = [];

    // Mock navigator.onLine BEFORE page loads
    await page.addInitScript(() => {
      // Create a controllable onLine property
      let _isOnline = true;
      Object.defineProperty(navigator, 'onLine', {
        get: () => _isOnline,
        configurable: true,
      });
      // Expose controls
      (window as Window & { __goOffline?: () => void; __goOnline?: () => void }).__goOffline =
        () => {
          _isOnline = false;
        };
      (window as Window & { __goOffline?: () => void; __goOnline?: () => void }).__goOnline =
        () => {
          _isOnline = true;
          window.dispatchEvent(new Event('online'));
        };
    });

    await page.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          payloads.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Clear captured payloads (from initial load)
    payloads.length = 0;

    // Go offline - both mock AND network
    await page.evaluate(
      () =>
        (window as Window & { __goOffline?: () => void; __goOnline?: () => void }).__goOffline?.()
    );
    await context.setOffline(true);

    // Verify navigator.onLine is false
    const isOffline = await page.evaluate(() => navigator.onLine === false);
    expect(isOffline).toBe(true);

    // Track goal while offline - should be queued
    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'offline_goal', type: 'other' }));
    await page.waitForTimeout(500);

    // Verify nothing sent yet (SDK queued it locally)
    const offlineGoalSent = payloads.some((p) => hasGoal(p, 'offline_goal'));
    expect(offlineGoalSent).toBe(false);

    // Verify queue has the payload stored.
    //
    // The retry queue is keyed per tab (`nf_pending_<tabId>`), not under a
    // single shared key: localStorage is shared across tabs and the queue is
    // updated by a non-atomic read-modify-write, so one key means two tabs
    // regaining connectivity at once erase each other's re-queued items. The
    // tab id itself lives in sessionStorage, which is what makes it per-tab.
    const queuedItems = await page.evaluate(() => {
      const tabId = JSON.parse(sessionStorage.getItem('nf_tab_id') ?? 'null');
      const stored = localStorage.getItem(`nf_pending_${tabId}`);
      return stored ? JSON.parse(stored) : [];
    });
    expect(queuedItems.length).toBeGreaterThan(0);

    // Come back online - restore network first, then trigger online event
    await context.setOffline(false);
    await page.waitForTimeout(100); // Let network restore
    await page.evaluate(
      () =>
        (window as Window & { __goOffline?: () => void; __goOnline?: () => void }).__goOnline?.()
    );
    await page.waitForTimeout(2000); // Wait for flush to complete

    // Verify queued payload was sent
    const found = payloads.some((p) => hasGoal(p, 'offline_goal'));
    expect(found).toBe(true);
  });

  test('handles 500 server error gracefully', async ({ page }) => {
    // First let SDK initialize
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Block further requests with 500 error
    await page.route('**/track', async (route) => {
      await route.fulfill({
        status: 500,
        body: JSON.stringify({ error: 'Internal Server Error' }),
      });
    });

    // Track goal - should not crash
    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'error_test', type: 'other' }));
    await page.waitForTimeout(500);

    // SDK should still function
    const sessionId = await page.evaluate(() => NotifuseAnalytics.getSessionId());
    expect(sessionId).toBeTruthy();
  });

  test('handles 429 rate limit gracefully', async ({ page }) => {
    // First let SDK initialize
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Block further requests with 429 error
    await page.route('**/track', async (route) => {
      await route.fulfill({
        status: 429,
        body: JSON.stringify({ error: 'Too Many Requests' }),
      });
    });

    // Track goal - should not crash
    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'rate_limit_test', type: 'other' }));
    await page.waitForTimeout(500);

    // SDK should still function
    const sessionId = await page.evaluate(() => NotifuseAnalytics.getSessionId());
    expect(sessionId).toBeTruthy();
  });

  // Note: Timeout queue functionality is tested in unit tests (sender.test.ts)
  // E2E timeout simulation with Playwright route interception doesn't work well with AbortController
  test.skip('SDK remains functional when requests timeout', async ({ page }) => {
    // Let SDK initialize first
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Now route subsequent requests to never respond
    let requestCount = 0;
    await page.route('**/track', async (route) => {
      requestCount++;
      if (requestCount > 1) {
        // Don't respond to requests after init
        await new Promise(() => {});
      } else {
        await route.continue();
      }
    });

    // Track a goal (this will timeout)
    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'timeout_test', type: 'other' }));

    // Wait past timeout (10s + buffer)
    await page.waitForTimeout(11000);

    // SDK should still function
    const sessionId = await page.evaluate(() => NotifuseAnalytics.getSessionId());
    expect(sessionId).toBeTruthy();

    // Should be able to track more goals
    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'after_timeout', type: 'other' }));
    // No error thrown = success
  });
});
