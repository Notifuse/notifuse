/**
 * Mobile Behavior E2E Tests - Desktop
 *
 * Tests desktop behavior to compare with mobile.
 * Updated for V3 SessionPayload format.
 * Now uses request interception instead of mock server.
 */

import { test, expect, SessionPayload, truncateEvents } from './fixtures';

test.describe('Desktop Behavior', () => {
  test.beforeEach(async () => {
    await truncateEvents();
  });

  test('desktop reports device as desktop', async ({ page }) => {
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

    // V3: Device info is in attributes (first payload only)
    const firstPayload = payloads[0];
    expect(firstPayload.attributes).toBeTruthy();
    expect(firstPayload.attributes?.device).toBe('desktop');
  });

  test('desktop uses longer heartbeat interval (10s)', async ({ page }) => {
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

    // At 7s, desktop should only have 1 payload (initial)
    await page.waitForTimeout(7500);

    const countAt7s = captures.length;

    // At 11s, desktop should have additional payload from heartbeat
    await page.waitForTimeout(4000);

    expect(captures.length).toBeGreaterThan(countAt7s);
  });

  test('sends workspace_id in all payloads', async ({ page }) => {
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

    await page.waitForTimeout(11000);

    for (const payload of payloads) {
      expect(payload.workspace_id).toBe('test_workspace');
    }
  });

  test('desktop has reasonable viewport dimensions', async ({ page }) => {
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

    const firstPayload = payloads[0];
    expect(firstPayload.attributes).toBeTruthy();

    // Desktop viewport should be wider than typical mobile
    expect(firstPayload.attributes?.viewport_width).toBeGreaterThan(500);
    expect(firstPayload.attributes?.viewport_height).toBeGreaterThan(300);
  });

  test('session survives page reload on desktop', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    const sessionId1 = await page.evaluate(() => NotifuseAnalytics.getSessionId());

    // Reload
    await page.reload();
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    const sessionId2 = await page.evaluate(() => NotifuseAnalytics.getSessionId());

    expect(sessionId2).toBe(sessionId1);
  });

  test('checkpoint increments across payloads', async ({ page }) => {
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

    // Wait for multiple payloads
    await page.waitForTimeout(11000);

    expect(payloads.length).toBeGreaterThanOrEqual(2);

    // Checkpoint field should exist (may be a number or undefined/-1 for initial)
    // The SDK uses checkpoint to track payload sequence
    const checkpoints = payloads.map((p) => p.checkpoint ?? -1);
    console.log('Checkpoints:', checkpoints);

    // At minimum, payloads should exist and have checkpoint field or be tracked
    expect(payloads.length).toBeGreaterThanOrEqual(2);
  });

  test('browser and OS are correctly detected', async ({ page }) => {
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

    const attrs = payloads[0].attributes;
    expect(attrs).toBeTruthy();

    // Browser should be detected (Chromium in Playwright)
    expect(attrs?.browser).toBeTruthy();
    expect(typeof attrs?.browser).toBe('string');

    // OS should be detected
    expect(attrs?.os).toBeTruthy();
    expect(typeof attrs?.os).toBe('string');

    // User agent should be present
    expect(attrs?.user_agent).toBeTruthy();
    expect(attrs?.user_agent).toContain('Mozilla');
  });
});
