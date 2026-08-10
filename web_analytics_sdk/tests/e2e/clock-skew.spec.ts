/**
 * Clock Skew E2E Tests
 *
 * Tests that sent_at is properly included in all payloads
 * and can be used by the server for clock skew detection.
 *
 * Updated for V3 SessionPayload format.
 * Now uses request interception instead of mock server.
 *
 * Clock skew = received_at - sent_at
 *   - Positive skew: client clock is behind server (or network latency)
 *   - Negative skew: client clock is ahead of server (always wrong)
 *
 * Note: sent_at is set at HTTP send time (not when payload is built/queued)
 */

import { test, expect, SessionPayload, truncateEvents } from './fixtures';

test.describe('Clock Skew Detection', () => {
  test.beforeEach(async () => {
    await truncateEvents();
  });

  test('all payloads include sent_at timestamp', async ({ page }) => {
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

    // All payloads should have sent_at (set at HTTP send time)
    for (const payload of payloads) {
      expect(payload.sent_at).toBeDefined();
      expect(typeof payload.sent_at).toBe('number');
      expect(payload.sent_at).toBeGreaterThan(0);

      // Also verify created_at and updated_at still exist
      expect(payload.created_at).toBeDefined();
      expect(payload.updated_at).toBeDefined();
    }
  });

  test('sent_at is close to actual send time (< 100ms diff)', async ({ page }) => {
    // Capture payloads with receive time
    const captures: { payload: SessionPayload; receivedAt: number }[] = [];
    await page.route('**/track', async (route) => {
      const receivedAt = Date.now();
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          captures.push({ payload: JSON.parse(postData), receivedAt });
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    await page.waitForTimeout(1000);

    expect(captures.length).toBeGreaterThanOrEqual(1);

    // sent_at should be very close to when we received it (accounting for processing)
    for (const capture of captures) {
      const diff = capture.receivedAt - capture.payload.sent_at!;
      // Should be < 100ms difference (same machine, no real network latency)
      expect(Math.abs(diff)).toBeLessThan(100);
    }
  });

  test('detects client clock 1 hour ahead (negative skew)', async ({ page }) => {
    // Capture payloads with receive time
    const captures: { payload: SessionPayload; receivedAt: number }[] = [];

    // Simulate client clock 1 hour ahead
    await page.addInitScript(() => {
      const realDateNow = Date.now;
      const offset = 60 * 60 * 1000; // 1 hour ahead
      Date.now = () => realDateNow() + offset;
    });

    await page.route('**/track', async (route) => {
      const receivedAt = Date.now(); // Real server time
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          captures.push({ payload: JSON.parse(postData), receivedAt });
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    await page.waitForTimeout(1000);

    expect(captures.length).toBeGreaterThanOrEqual(1);

    // Check first payload - use sent_at for accurate skew calculation
    const capture = captures[0];
    expect(capture.payload.sent_at).toBeDefined();
    const skewMs = capture.receivedAt - capture.payload.sent_at!;
    const skewMinutes = skewMs / 1000 / 60;

    // Skew should be approximately -60 minutes (client ahead)
    expect(skewMinutes).toBeLessThan(-55);
    expect(skewMinutes).toBeGreaterThan(-65);

    console.log(`Client 1h ahead: skew = ${skewMinutes.toFixed(1)} minutes`);
  });

  test('detects client clock 30 minutes behind (positive skew)', async ({ page }) => {
    // Capture payloads with receive time
    const captures: { payload: SessionPayload; receivedAt: number }[] = [];

    // Simulate client clock 30 minutes behind
    await page.addInitScript(() => {
      const realDateNow = Date.now;
      const offset = -30 * 60 * 1000; // 30 minutes behind
      Date.now = () => realDateNow() + offset;
    });

    await page.route('**/track', async (route) => {
      const receivedAt = Date.now(); // Real server time
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          captures.push({ payload: JSON.parse(postData), receivedAt });
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    await page.waitForTimeout(1000);

    expect(captures.length).toBeGreaterThanOrEqual(1);

    // Check first payload - use sent_at for accurate skew calculation
    const capture = captures[0];
    expect(capture.payload.sent_at).toBeDefined();
    const skewMs = capture.receivedAt - capture.payload.sent_at!;
    const skewMinutes = skewMs / 1000 / 60;

    // Skew should be approximately +30 minutes (client behind)
    expect(skewMinutes).toBeGreaterThan(25);
    expect(skewMinutes).toBeLessThan(35);

    console.log(`Client 30m behind: skew = ${skewMinutes.toFixed(1)} minutes`);
  });

  test('skew calculation demonstrates correction threshold', async ({ page }) => {
    // Capture payloads with receive time
    const captures: { payload: SessionPayload; receivedAt: number }[] = [];
    await page.route('**/track', async (route) => {
      const receivedAt = Date.now();
      const request = route.request();
      const postData = request.postData();
      if (postData) {
        try {
          captures.push({ payload: JSON.parse(postData), receivedAt });
        } catch {
          // ignore
        }
      }
      await route.continue();
    });

    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    await page.waitForTimeout(1000);

    expect(captures.length).toBeGreaterThanOrEqual(1);

    // Verify server can calculate skew using sent_at and apply threshold logic
    const capture = captures[0];
    expect(capture.payload.sent_at).toBeDefined();
    const skewMs = capture.receivedAt - capture.payload.sent_at!;

    // In normal conditions, skew should be minimal (<5s)
    // Server-side logic checks: |skew| > 5000ms to determine if correction needed
    const needsCorrection = Math.abs(skewMs) > 5000;

    // Under normal conditions, this should be false
    expect(needsCorrection).toBe(false);

    console.log(`Normal skew: ${skewMs}ms, needsCorrection = ${needsCorrection}`);
  });

  test('heartbeat payloads also include sent_at', async ({ page }) => {
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

    // Wait for heartbeat (10s on desktop)
    await page.waitForTimeout(11000);

    expect(payloads.length).toBeGreaterThanOrEqual(2);

    // All payloads including heartbeat should have sent_at
    for (const payload of payloads) {
      expect(payload.sent_at).toBeDefined();
      expect(typeof payload.sent_at).toBe('number');
      expect(payload.sent_at).toBeGreaterThan(0);
    }

    // Heartbeat payload's sent_at should be > first payload's sent_at
    const firstSentAt = payloads[0].sent_at!;
    const lastSentAt = payloads[payloads.length - 1].sent_at!;
    expect(lastSentAt).toBeGreaterThan(firstSentAt);

    // Time difference should be approximately heartbeat interval (10s)
    const timeDiff = lastSentAt - firstSentAt;
    expect(timeDiff).toBeGreaterThan(9000);
  });
});
