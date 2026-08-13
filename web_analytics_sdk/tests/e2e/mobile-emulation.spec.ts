/**
 * Mobile Behavior E2E Tests - Mobile Emulation
 *
 * Tests mobile-specific behavior using Pixel 5 emulation (Chromium compatible).
 * Updated for V3 SessionPayload format.
 * Now uses request interception instead of mock server.
 */

import { test, expect, devices, SessionPayload, hasGoal, truncateEvents } from './fixtures';

// Mobile device configuration - must be at top level
// Using Pixel 5 (Android) instead of iPhone to work with Chromium
const pixel5 = devices['Pixel 5'];
test.use({ ...pixel5 });

test.describe('Mobile Emulation', () => {
  test.beforeEach(async () => {
    await truncateEvents();
  });

  test('detects mobile device correctly', async ({ page }) => {
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

    // V3: Device info is in attributes
    const payload = payloads[0];
    expect(payload.attributes?.device).toBe('mobile');
  });

  test('handles touch scroll events', async ({ page }) => {
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

    await page.touchscreen.tap(200, 300);

    await page.evaluate(() => {
      window.scrollTo(0, 500);
      window.dispatchEvent(new Event('scroll'));
    });

    await page.waitForTimeout(500);

    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'scroll_check', type: 'other' }));
    await page.waitForTimeout(500);

    // Find payload with the goal
    const payloadWithGoal = payloads.find((p) => hasGoal(p, 'scroll_check'));
    expect(payloadWithGoal).toBeTruthy();

    // V3: Current page is in actions[] (no more current_page field)
    const pageviewActions = payloadWithGoal?.actions.filter((a) => a.type === 'pageview') || [];
    expect(pageviewActions.length).toBeGreaterThan(0);
    expect(typeof pageviewActions[0].scroll).toBe('number');
  });

  test('uses shorter heartbeat interval on mobile (7s)', async ({ page }) => {
    // Capture payloads with timestamps
    const captures: { payload: SessionPayload; timestamp: number }[] = [];
    await page.route('**/track', async (route) => {
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          captures.push({ payload: JSON.parse(postData), timestamp: Date.now() });
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Wait for mobile heartbeat (7s) + buffer
    await page.waitForTimeout(8000);

    // Should have received at least 2 payloads (initial + 1 heartbeat)
    expect(captures.length).toBeGreaterThanOrEqual(2);
  });

  test('handles freeze event (background app)', async ({ page }) => {
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

    // Wait some time focused
    await page.waitForTimeout(1000);

    // Freeze the app (should pause duration tracking)
    await page.evaluate(() => {
      document.dispatchEvent(new Event('freeze'));
    });

    // Time passes while frozen (should NOT count toward duration)
    await page.waitForTimeout(2000);

    // Resume the app
    await page.evaluate(() => {
      document.dispatchEvent(new Event('resume'));
    });

    // Wait a bit more focused
    await page.waitForTimeout(500);

    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'freeze_check', type: 'other' }));
    await page.waitForTimeout(500);

    // SDK should have handled freeze/resume events
    expect(payloads.length).toBeGreaterThanOrEqual(1);
  });

  test('reports correct viewport size', async ({ page }) => {
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

    const payload = payloads[0];

    // Pixel 5 viewport is 393x851 in portrait
    expect(payload.attributes?.viewport_width).toBeLessThan(500);
    expect(payload.attributes?.viewport_height).toBeGreaterThan(
      payload.attributes?.viewport_width || 0
    );
  });

  test('handles orientation change', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    const initialViewport = await page.evaluate(() => ({
      width: window.innerWidth,
      height: window.innerHeight,
    }));

    await page.setViewportSize({ width: 844, height: 390 });

    await page.evaluate(() => {
      window.dispatchEvent(new Event('resize'));
      window.dispatchEvent(new Event('orientationchange'));
    });

    await page.waitForTimeout(500);

    const newViewport = await page.evaluate(() => ({
      width: window.innerWidth,
      height: window.innerHeight,
    }));

    expect(newViewport.width).toBeGreaterThan(initialViewport.width);
  });

  test('tracks touch-based clicks', async ({ page }) => {
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

    await page.tap('#btn-track');
    await page.waitForTimeout(500);

    // Should have payload with button_click goal
    const payloadWithGoal = payloads.find((p) => hasGoal(p, 'button_click'));
    expect(payloadWithGoal).toBeTruthy();
  });

  test('touch scroll updates scroll in current_page', async ({ page }) => {
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

    await page.evaluate(() => {
      window.scrollTo(0, 500);
      window.dispatchEvent(new Event('scroll'));
    });
    await page.waitForTimeout(200);

    await page.evaluate(() => {
      window.scrollTo(0, 1000);
      window.dispatchEvent(new Event('scroll'));
    });
    await page.waitForTimeout(200);

    await page.evaluate(() => NotifuseAnalytics.trackGoal({ action: 'scroll_final', type: 'other' }));
    await page.waitForTimeout(500);

    // Find payload with the goal
    const payloadWithGoal = payloads.find((p) => hasGoal(p, 'scroll_final'));
    expect(payloadWithGoal).toBeTruthy();

    // V3: Current page is in actions[] (no more current_page field)
    const pageviewActions = payloadWithGoal?.actions.filter((a) => a.type === 'pageview') || [];
    expect(pageviewActions.length).toBeGreaterThan(0);
    expect(typeof pageviewActions[0].scroll).toBe('number');
  });

  test('mobile user agent is detected', async ({ page }) => {
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

    const attrs = payloads[0].attributes;
    expect(attrs).toBeTruthy();

    // User agent should contain mobile indicators
    expect(attrs?.user_agent).toBeTruthy();
    // Pixel 5 is Android
    expect(attrs?.os?.toLowerCase()).toContain('android');
  });

  test('screen dimensions reflect mobile device', async ({ page }) => {
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

    const attrs = payloads[0].attributes;
    expect(attrs).toBeTruthy();

    // Mobile screen dimensions
    expect(attrs?.screen_width).toBeGreaterThan(0);
    expect(attrs?.screen_height).toBeGreaterThan(0);

    // Viewport should be smaller than desktop
    expect(attrs?.viewport_width).toBeLessThan(500);
  });
});
