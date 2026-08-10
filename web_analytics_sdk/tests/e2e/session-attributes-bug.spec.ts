/**
 * Session Attributes Bug E2E Test
 *
 * This test reproduces the bug where device/browser/OS information is not
 * stored in the events table because:
 *
 * 1. SDK sends first payload with `actions: []` (user still on first page)
 *    and `attributes: { device: 'desktop', browser: 'Chrome', ... }`
 * 2. API sees `actions.length === 0` and returns early WITHOUT creating events
 * 3. Attributes are DISCARDED
 * 4. Later payloads have completed actions but NO attributes
 * 5. Events are created with empty device/browser/OS fields
 *
 * Full browser E2E: Playwright drives the real bundle and the beats are read
 * back from the collector.
 */

import { test, expect, truncateEvents, waitForEvents } from './fixtures';

test.describe('Session Attributes Bug', () => {
  test.beforeEach(async () => {
    // Clear events table before each test
    await truncateEvents();
  });

  test('V3 FIX: first payload has landing page in actions, attributes are preserved', async ({ page }) => {
    // Navigate to test page - SDK initializes and sends first payload
    // V3: actions=[{type:'pageview', path:'/test-page.html', ...}], attributes={device:'desktop',...}
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Get session ID for querying
    const sessionId = await page.evaluate(() => NotifuseAnalytics.getSessionId());

    // Navigate away to trigger pageview completion
    await page.goto('/spa-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Wait a bit for the SDK to send the pageview
    await page.waitForTimeout(2000);

    // Wait for the beats to reach the collector
    const events = await waitForEvents(sessionId, 1, 15000);

    expect(events.length).toBeGreaterThan(0);

    const firstEvent = events[0];

    // V3 FIX: Device info should be present (not empty anymore)
    // In V3, the landing page is in actions[] from the start, so attributes are always included
    expect(firstEvent.device).toBeTruthy(); // FIXED: should be 'desktop'
    expect(firstEvent.browser).toBeTruthy(); // FIXED: should have browser
    expect(firstEvent.os).toBeTruthy(); // FIXED: should have OS
    expect(firstEvent.user_agent).toBeTruthy(); // FIXED: should have user agent
    expect(firstEvent.language).toBeTruthy(); // FIXED: should have language
    expect(firstEvent.timezone).toBeTruthy(); // FIXED: should have timezone

    console.log('Event data (V3 - bug fixed):', {
      session_id: firstEvent.session_id,
      path: firstEvent.path,
      device: firstEvent.device,
      browser: firstEvent.browser,
      os: firstEvent.os,
      landing_page: firstEvent.landing_page,
    });
  });

  test('V3 FIX: all events from session have device info', async ({ page }) => {
    // Navigate to test page
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    const sessionId = await page.evaluate(() => NotifuseAnalytics.getSessionId());

    // Track a goal (should create a goal event)
    await page.click('#btn-goal');
    await page.waitForTimeout(500);

    // Navigate to SPA page (completes first pageview, starts second)
    await page.goto('/spa-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.waitForTimeout(1000);

    // Navigate within SPA (creates more pageviews)
    await page.click('nav a:has-text("Products")');
    await page.waitForTimeout(1000);

    await page.click('nav a:has-text("About")');
    await page.waitForTimeout(2000);

    // Wait for events
    const events = await waitForEvents(sessionId, 2, 15000);

    expect(events.length).toBeGreaterThan(0);

    console.log(`Found ${events.length} events for session ${sessionId}`);

    // V3 FIX: ALL events should have device info (not empty anymore)
    for (const event of events) {
      console.log(`Event: name=${event.name}, path=${event.path}, device=${event.device}, browser=${event.browser}`);

      // V3 Fix: device fields should be present
      expect(event.device).toBeTruthy();
      expect(event.browser).toBeTruthy();
      expect(event.os).toBeTruthy();
    }
  });

  test('V3: verify SDK sends attributes AND landing page in first payload', async ({ page }) => {
    // This test verifies V3 behavior: first payload has both
    // attributes AND the landing page in actions[]

    // Set up request interception to capture the payload
    const payloads: unknown[] = [];
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
      // Continue the request to the real API
      await route.continue();
    });

    // Navigate to test page
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Wait for first payload to be sent
    await page.waitForTimeout(2000);

    // Check that first payload has attributes
    expect(payloads.length).toBeGreaterThan(0);

    const firstPayload = payloads[0] as {
      attributes?: {
        device?: string;
        browser?: string;
        os?: string;
      };
      actions?: { type: string; path?: string }[];
      current_page?: unknown;
      checkpoint?: unknown;
    };

    console.log('First payload:', JSON.stringify(firstPayload, null, 2));

    // V3: SDK sends attributes in first payload
    expect(firstPayload.attributes).toBeDefined();
    expect(firstPayload.attributes?.device).toBeDefined();
    expect(firstPayload.attributes?.browser).toBeDefined();

    // V3: Landing page is in actions[] from the start (not empty anymore)
    expect(firstPayload.actions?.length).toBeGreaterThan(0);
    expect(firstPayload.actions?.[0].type).toBe('pageview');

    // V3: No current_page or checkpoint fields
    expect(firstPayload.current_page).toBeUndefined();
    expect(firstPayload.checkpoint).toBeUndefined();
  });
});
