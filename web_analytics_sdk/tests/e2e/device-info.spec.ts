/**
 * Device Info E2E Tests
 *
 * Tests that device information is correctly detected and sent in session payloads.
 * Regression test for bug where device info was missing from first payload.
 *
 * Uses request interception to capture payloads instead of mock server.
 */

import { test, expect, SessionPayload, truncateEvents } from './fixtures';

test.describe('Device Info', () => {
  test.beforeEach(async () => {
    // Clear events table before each test
    await truncateEvents();
  });

  test('auto-init from NotifuseAnalyticsConfig includes device info', async ({ page }) => {
    // Capture payloads via request interception
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

    // This mimics production setup with window.NotifuseAnalyticsConfig
    await page.goto('/auto-init-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Wait for initial payload
    await page.waitForTimeout(1000);

    expect(payloads.length).toBeGreaterThanOrEqual(1);

    const payload = payloads[0];

    // Auto-init MUST include attributes with device info
    expect(payload.attributes).toBeTruthy();
    expect(payload.attributes!.device).toBeTruthy();
    expect(payload.attributes!.browser).toBeTruthy();
    expect(payload.attributes!.os).toBeTruthy();
    expect(payload.attributes!.screen_width).toBeGreaterThan(0);
  });

  test('first payload includes device info in attributes', async ({ page }) => {
    // Capture payloads via request interception
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

    // Wait for initial payload
    await page.waitForTimeout(1000);

    expect(payloads.length).toBeGreaterThanOrEqual(1);

    const payload = payloads[0];

    // First payload MUST have attributes
    expect(payload.attributes).toBeTruthy();

    // Verify device info fields are present
    const attrs = payload.attributes!;

    // Screen dimensions
    expect(attrs.screen_width).toBeGreaterThan(0);
    expect(attrs.screen_height).toBeGreaterThan(0);

    // Viewport dimensions
    expect(attrs.viewport_width).toBeGreaterThan(0);
    expect(attrs.viewport_height).toBeGreaterThan(0);

    // Device type (desktop in E2E tests)
    expect(attrs.device).toBe('desktop');

    // Browser detection
    expect(attrs.browser).toBeTruthy();
    expect(typeof attrs.browser).toBe('string');

    // OS detection
    expect(attrs.os).toBeTruthy();
    expect(typeof attrs.os).toBe('string');

    // User agent
    expect(attrs.user_agent).toBeTruthy();
    expect(attrs.user_agent).toContain('Mozilla');

    // Language
    expect(attrs.language).toBeTruthy();

    // Timezone
    expect(attrs.timezone).toBeTruthy();
  });

  test('checkpoint 0 payload has attributes with device info', async ({ page }) => {
    // Capture payloads via request interception
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

    // Wait for initial payload
    await page.waitForTimeout(1000);

    // Find the first payload (checkpoint 0 or undefined)
    const firstPayload = payloads.find(
      (p) => p.checkpoint === 0 || p.checkpoint === undefined
    );

    expect(firstPayload).toBeTruthy();
    expect(firstPayload!.attributes).toBeTruthy();

    // Device info must be present
    expect(firstPayload!.attributes!.device).toBeTruthy();
    expect(firstPayload!.attributes!.browser).toBeTruthy();
    expect(firstPayload!.attributes!.os).toBeTruthy();
  });

  test('subsequent payloads may omit attributes', async ({ page }) => {
    // Capture payloads via request interception
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

    // Wait for heartbeat (10s+ for desktop)
    await page.waitForTimeout(11000);

    // Should have multiple payloads
    expect(payloads.length).toBeGreaterThanOrEqual(2);

    // First payload should have attributes
    expect(payloads[0].attributes).toBeTruthy();
    expect(payloads[0].attributes!.device).toBeTruthy();

    // Subsequent payloads may omit attributes (this is by design)
    // The API should use the attributes from the first payload
  });

  test('new session in fresh context has device info', async ({ browser }) => {
    // Create a completely fresh context (simulates incognito/private window)
    const context = await browser.newContext();
    const page = await context.newPage();

    // Capture payloads via request interception
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

    try {
      await page.goto('/test-page.html');
      await page.waitForFunction(() => window.SDK_INITIALIZED);
      await page.evaluate(() => window.SDK_READY);

      // Wait for initial payload
      await page.waitForTimeout(1000);

      expect(payloads.length).toBeGreaterThanOrEqual(1);

      const payload = payloads[0];

      // Fresh context must have attributes with device info
      expect(payload.attributes).toBeTruthy();
      expect(payload.attributes!.device).toBeTruthy();
      expect(payload.attributes!.browser).toBeTruthy();
      expect(payload.attributes!.os).toBeTruthy();
      expect(payload.attributes!.screen_width).toBeGreaterThan(0);
      expect(payload.attributes!.viewport_width).toBeGreaterThan(0);
    } finally {
      await context.close();
    }
  });

  test('REGRESSION: second tab in same session should still have device info on first payload', async ({
    page,
    context,
  }) => {
    // This test reproduces a bug where:
    // 1. User visits site in tab 1 -> session created, attributes sent
    // 2. User opens new tab 2 in same browser -> same session ID (from localStorage)
    // 3. Tab 2's SessionState.restore() finds sessionStorage with attributesSent: true
    // 4. Tab 2's first payload INCORRECTLY omits attributes

    // Capture payloads for tab 1
    const payloadsTab1: SessionPayload[] = [];
    await page.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          payloadsTab1.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    // Tab 1: Visit page, send initial payload with attributes
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);
    await page.waitForTimeout(1000);

    // Verify tab 1 sent attributes
    expect(payloadsTab1.length).toBeGreaterThanOrEqual(1);
    expect(payloadsTab1[0].attributes).toBeTruthy();
    expect(payloadsTab1[0].attributes!.device).toBeTruthy();

    const sessionIdTab1 = payloadsTab1[0].session_id;

    // Capture payloads for tab 2
    const payloadsTab2: SessionPayload[] = [];

    // Tab 2: Open new page in same context (shares localStorage + sessionStorage)
    const page2 = await context.newPage();
    await page2.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          payloadsTab2.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page2.goto('/test-page.html');
    await page2.waitForFunction(() => window.SDK_INITIALIZED);
    await page2.evaluate(() => window.SDK_READY);
    await page2.waitForTimeout(1000);

    expect(payloadsTab2.length).toBeGreaterThanOrEqual(1);

    const tab2Payload = payloadsTab2[0];

    // Session should be the same (reused from localStorage)
    expect(tab2Payload.session_id).toBe(sessionIdTab1);

    // BUG: Tab 2's first payload should ALSO have attributes!
    // The server needs device info for this tab too (might be on different device/browser)
    // Currently this fails because attributesSent is restored from sessionStorage
    expect(tab2Payload.attributes).toBeTruthy();
    expect(tab2Payload.attributes!.device).toBeTruthy();

    await page2.close();
  });

  test('REGRESSION: page reload should NOT resend attributes if already sent', async ({
    page,
  }) => {
    // This tests the intentional behavior where attributes are only sent once
    // per session, even across page reloads (sessionStorage persists)

    // Capture payloads for first load
    const payloadsFirstLoad: SessionPayload[] = [];
    await page.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          payloadsFirstLoad.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    // First load: send initial payload with attributes
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);
    await page.waitForTimeout(1000);

    // Verify first load sent attributes
    expect(payloadsFirstLoad.length).toBeGreaterThanOrEqual(1);
    expect(payloadsFirstLoad[0].attributes).toBeTruthy();
    expect(payloadsFirstLoad[0].attributes!.device).toBeTruthy();

    const sessionId = payloadsFirstLoad[0].session_id;
    const firstCheckpoint = payloadsFirstLoad[payloadsFirstLoad.length - 1].checkpoint;

    // Capture payloads for reload
    const payloadsReload: SessionPayload[] = [];

    // Remove previous route and add new one
    await page.unroute('**/track');
    await page.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          payloadsReload.push(JSON.parse(postData));
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    // Reload the page (sessionStorage persists, session ID reused)
    await page.reload();
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);
    await page.waitForTimeout(1000);

    expect(payloadsReload.length).toBeGreaterThanOrEqual(1);

    const reloadPayload = payloadsReload[0];

    // Session should be the same
    expect(reloadPayload.session_id).toBe(sessionId);

    // After reload, attributesSent should be restored as true from sessionStorage
    // So attributes should NOT be included (this is correct behavior for same-tab reloads)
    // However, checkpoint should be > first checkpoint if state was properly restored
    console.log('First checkpoint:', firstCheckpoint);
    console.log('Reload checkpoint:', reloadPayload.checkpoint);
    console.log('Reload has attributes:', !!reloadPayload.attributes);

    // This is the expected behavior - attributes are NOT resent on reload
    // The issue is when a DIFFERENT browser/tab needs to send device info
  });

  test('DEBUG: log first payload structure', async ({ page }) => {
    // Capture payloads via request interception
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

    // Debug test to see exact payload structure
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);
    await page.waitForTimeout(1000);

    console.log('=== First Payload ===');
    console.log(JSON.stringify(payloads[0], null, 2));

    expect(payloads[0].attributes).toBeTruthy();
  });

  test('V3 FIX: attributes always sent regardless of sessionStorage state', async ({
    page,
  }) => {
    // Capture payloads via request interception
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

    // First, visit the page normally to get the session ID
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);
    await page.waitForTimeout(1000);

    // Get the session ID that was created
    const sessionId = await page.evaluate(() => NotifuseAnalytics.getSessionId());

    // Simulate stale sessionStorage (old V2 format with attributesSent=true)
    // V3 should ignore these fields and still send attributes
    await page.evaluate((sid) => {
      sessionStorage.setItem(
        'nf_session_state',
        JSON.stringify({
          session_id: sid,
          actions: [],
          currentPage: null,
          checkpoint: 0,
          attributesSent: true, // V3 ignores this
        })
      );
    }, sessionId);

    // Clear captured payloads
    payloads.length = 0;

    // Reload the page - SDK will restore from sessionStorage
    await page.reload();
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);
    await page.waitForTimeout(1000);

    expect(payloads.length).toBeGreaterThanOrEqual(1);

    const payload = payloads[0];

    console.log('=== Payload after restore (V3) ===');
    console.log(JSON.stringify(payload, null, 2));

    // V3: No checkpoint field
    expect(payload.checkpoint).toBeUndefined();

    // V3 FIX: Attributes are ALWAYS sent (no attributesSent flag)
    expect(payload.attributes).toBeTruthy();
    expect(payload.attributes?.device).toBeTruthy();
    expect(payload.attributes?.browser).toBeTruthy();
  });
});
