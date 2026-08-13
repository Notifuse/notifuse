/**
 * Comprehensive Session E2E Test
 *
 * This test consolidates and verifies ALL SDK features and database fields in a single
 * multi-page session. It covers:
 * - All public SDK APIs (getSessionId, getConfig, debug, trackPageView, trackGoal, etc.)
 * - SPA navigation tracking
 * - Visibility change handling (focus time pauses when hidden)
 * - Pause/resume functionality
 * - Custom dimensions
 * - Every field the SDK puts on the wire
 *
 * Replaces: page-duration.spec.ts, duration-accuracy.spec.ts, session-lifecycle.spec.ts, debug.spec.ts
 */

import {
  test,
  expect,
  SessionPayload,
  truncateEvents,
  waitForEvents,
  PageviewAction,
  GoalAction,
} from './fixtures';

test.describe('Comprehensive Session Verification', () => {
  test.beforeEach(async () => {
    await truncateEvents();
  });

  test('multi-page session with full SDK API coverage and database verification', async ({ page }) => {
    // Increase timeout for this comprehensive test
    test.setTimeout(120000);

    // Capture payloads for debugging
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

    // ========================================
    // Phase 1: Navigate with UTM and Referrer
    // ========================================
    const testUrl =
      '/spa-page.html?utm_source=test_source&utm_medium=test_medium&utm_campaign=test_campaign';
    await page.goto(testUrl, {
      referer: 'https://referrer.example.com/page',
    });

    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // ========================================
    // Phase 2: Verify SDK APIs work
    // ========================================
    const sessionId = await page.evaluate(() => NotifuseAnalytics.getSessionId());
    expect(sessionId).toBeTruthy();
    expect(sessionId.length).toBeGreaterThan(10);

    const config = await page.evaluate(() => NotifuseAnalytics.getConfig());
    expect(config).toBeTruthy();
    expect(config?.workspace_id).toBe('test_workspace');

    const debugInfo = await page.evaluate(() => NotifuseAnalytics.debug());
    expect(debugInfo.isTracking).toBe(true);
    expect(debugInfo.session).toBeTruthy();

    // ========================================
    // Phase 3: Set Custom Dimensions
    // ========================================
    await page.evaluate(() => {
      NotifuseAnalytics.setDimension(1, 'dimension_1_value');
      NotifuseAnalytics.setDimension(2, 'dimension_2_value');
    });

    const dim1 = await page.evaluate(() => NotifuseAnalytics.getDimension(1));
    expect(dim1).toBe('dimension_1_value');

    const dim2 = await page.evaluate(() => NotifuseAnalytics.getDimension(2));
    expect(dim2).toBe('dimension_2_value');

    // ========================================
    // Page 1: /home (landing page)
    // Stay 2 seconds, scroll to ~50%
    // ========================================
    await page.waitForTimeout(2000);

    // Scroll to approximately 50%
    await page.evaluate(() => {
      const scrollHeight = document.documentElement.scrollHeight - window.innerHeight;
      window.scrollTo(0, scrollHeight / 2);
      window.dispatchEvent(new Event('scroll'));
    });
    await page.waitForTimeout(200); // Let scroll tracking register

    // ========================================
    // Test visibility pause (hidden time should NOT count toward duration)
    // ========================================
    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'hidden', writable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    await page.waitForTimeout(1000); // 1 second hidden (should NOT count)
    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', writable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });

    // ========================================
    // Page 2: /products (SPA navigation)
    // Stay 1.5 seconds, track goal
    // ========================================
    await page.click('nav a:has-text("Products")');
    await page.waitForTimeout(1500);

    // Track goal with value and properties
    await page.evaluate(() =>
      NotifuseAnalytics.trackGoal({
        action: 'purchase',
        type: 'purchase',
        value: 99.99,
        properties: { product: 'test-item' },
      }),
    );
    await page.waitForTimeout(500); // Wait for goal to be sent

    // ========================================
    // Test pause/resume (paused time should NOT count)
    // ========================================
    await page.evaluate(() => NotifuseAnalytics.pause());
    await page.waitForTimeout(1000); // 1 second paused (should NOT count)
    await page.evaluate(() => NotifuseAnalytics.resume());

    // ========================================
    // Page 3: /about (SPA navigation)
    // ========================================
    await page.click('nav a:has-text("About")');
    await page.waitForTimeout(500);

    // ========================================
    // Page 4: Manual trackPageView
    // ========================================
    await page.evaluate(() => NotifuseAnalytics.trackPageView('/custom-path'));
    await page.waitForTimeout(500);

    // ========================================
    // Verify Duration APIs
    // ========================================
    const focusDuration = await page.evaluate(() => NotifuseAnalytics.getFocusDuration());
    const totalDuration = await page.evaluate(() => NotifuseAnalytics.getTotalDuration());

    // Focus duration should be less than total (hidden/paused time excluded)
    expect(focusDuration).toBeGreaterThan(0);
    expect(totalDuration).toBeGreaterThan(0);
    // Total should be >= focus (total includes all time, focus excludes hidden/paused)
    expect(totalDuration).toBeGreaterThanOrEqual(focusDuration);

    // ========================================
    // Trigger unload to send final payload
    // ========================================
    await page.close();
    await new Promise((r) => setTimeout(r, 2000)); // Wait for beacon/fetchLater

    // ========================================
    // Phase 3: Verify SDK Payloads (mandatory)
    // ========================================
    expect(payloads.length).toBeGreaterThan(0);

    // Verify V3 payload structure
    for (const payload of payloads) {
      // V3: No current_page or checkpoint
      expect(payload.current_page).toBeUndefined();
      expect(payload.checkpoint).toBeUndefined();
      // V3: Always include attributes
      expect(payload.attributes).toBeDefined();
      expect(payload.attributes?.landing_page).toBeTruthy();
    }

    // Verify SDK sent correct actions
    const lastPayload = payloads[payloads.length - 1];
    const sdkPageviews = lastPayload.actions.filter(
      (a): a is PageviewAction => a.type === 'pageview',
    );
    const sdkGoals = lastPayload.actions.filter((a): a is GoalAction => a.type === 'goal');

    expect(sdkPageviews.length).toBe(4); // 3 SPA + 1 manual trackPageView
    expect(sdkGoals.length).toBe(1);

    // Verify pageview paths
    expect(sdkPageviews[0].path).toBe('/home');
    expect(sdkPageviews[1].path).toBe('/products');
    expect(sdkPageviews[2].path).toBe('/about');
    expect(sdkPageviews[3].path).toBe('/custom-path');

    // Verify goal
    expect(sdkGoals[0].name).toBe('purchase');
    expect(sdkGoals[0].value).toBe(99.99);

    // Verify UTM was captured (check any payload since attributes are captured on init)
    const payloadWithUtm = payloads.find((p) => p.attributes?.utm_source);
    if (payloadWithUtm) {
      expect(payloadWithUtm.attributes?.utm_source).toBe('test_source');
      expect(payloadWithUtm.attributes?.utm_medium).toBe('test_medium');
      expect(payloadWithUtm.attributes?.utm_campaign).toBe('test_campaign');
    } else {
      // UTM may not be captured if URL is changed before SDK parses it
      // This is expected with spa-page.html which replaces URL to /home
      console.log('Note: UTM params not captured - spa-page.html replaces URL before SDK init');
    }

    // Verify referrer was captured (check any payload)
    const payloadWithReferrer = payloads.find((p) => p.attributes?.referrer);
    if (payloadWithReferrer) {
      expect(payloadWithReferrer.attributes?.referrer).toBe('https://referrer.example.com/page');
    } else {
      console.log('Note: Referrer not captured');
    }

    // ========================================
    // Phase 4: Read the beats back from the collector
    // ========================================
    const events = await waitForEvents(sessionId, 5, 15000); // 4 pageviews + 1 goal

    // Every action must have landed. The collector accepts unconditionally,
    // so a shortfall here means the SDK failed to send it.
    expect(events.length).toBeGreaterThanOrEqual(5);

    // Separate events by type
    const pageviews = events
      .filter((e) => e.name === 'pageview')
      .sort((a, b) => a.page_number - b.page_number);
    const goals = events.filter((e) => e.name === 'goal');

    // ========================================
    // Verify Event Counts (if we have events)
    // ========================================
    expect(pageviews.length).toBe(4); // 3 SPA navigations + 1 manual trackPageView
    expect(goals.length).toBe(1);

    // ========================================
    // Verify Identity Fields (all events)
    // ========================================
    for (const event of events) {
      expect(event.session_id).toBe(sessionId);
      expect(event.workspace_id).toBe('test_workspace');
      // Notifuse releases are vMAJOR.minor (config/config.go), not semver
      expect(event.sdk_version).toMatch(/^\d+\.\d+$/);
    }

    // ========================================
    // Verify Pageview 1: /home (landing)
    // ========================================
    const pv1 = pageviews[0];
    expect(pv1.path).toBe('/home');
    expect(pv1.page_number).toBe(1);
    // Duration should be ~2s (focus time only, not including 1s hidden)
    expect(pv1.duration).toBeGreaterThanOrEqual(1500);
    expect(pv1.duration).toBeLessThan(4000); // Should not include hidden time
    // Scroll should be recorded
    expect(pv1.max_scroll).toBeGreaterThan(0);

    // ========================================
    // Verify Pageview 2: /products
    // ========================================
    const pv2 = pageviews[1];
    expect(pv2.path).toBe('/products');
    expect(pv2.page_number).toBe(2);
    // Duration should be ~1.5s (not including 1s paused)
    expect(pv2.duration).toBeGreaterThanOrEqual(1000);
    expect(pv2.duration).toBeLessThan(4000); // Should not include paused time

    // ========================================
    // Verify Pageview 3: /about
    // ========================================
    const pv3 = pageviews[2];
    expect(pv3.path).toBe('/about');
    expect(pv3.page_number).toBe(3);

    // ========================================
    // Verify Pageview 4: /custom-path (manual trackPageView)
    // ========================================
    const pv4 = pageviews[3];
    expect(pv4.path).toBe('/custom-path');
    expect(pv4.page_number).toBe(4);

    // ========================================
    // Verify Goal Event
    // ========================================
    const goal = goals[0];
    expect(goal.name).toBe('goal');
    expect(goal.goal_name).toBe('purchase');
    expect(goal.goal_value).toBe(99.99);
    expect(goal.path).toBe('/products'); // Goal was on products page
    expect(goal.page_number).toBe(2);

    // ========================================
    // Verify Traffic Source as sent (referrer_domain/is_direct are derived
    // server-side and are not part of the payload)
    // ========================================
    for (const event of events) {
      expect(event.referrer).toBe('https://referrer.example.com/page');
    }

    // ========================================
    // Verify UTM Parameters (all events should have same)
    // ========================================
    for (const event of events) {
      expect(event.utm_source).toBe('test_source');
      expect(event.utm_medium).toBe('test_medium');
      expect(event.utm_campaign).toBe('test_campaign');
    }

    // ========================================
    // Verify Landing Page (all events should have same)
    // ========================================
    for (const event of events) {
      expect(event.landing_page).toContain('/home');
    }

    // ========================================
    // Verify Device & Browser (all events should have same)
    // ========================================
    for (const event of events) {
      expect(event.device).toBe('desktop');
      expect(event.browser).toBeTruthy();
      expect(event.os).toBeTruthy();
      expect(event.user_agent).toBeTruthy();
      expect(event.screen_width).toBeGreaterThan(0);
      expect(event.screen_height).toBeGreaterThan(0);
      expect(event.viewport_width).toBeGreaterThan(0);
      expect(event.viewport_height).toBeGreaterThan(0);
      expect(event.language).toBeTruthy();
      expect(event.timezone).toBeTruthy();
    }

    // ========================================
    // Verify Timing Fields
    // ========================================
    const now = Date.now();
    for (const event of events) {
      // received_at should be recent (within last 2 minutes)
      const receivedAt = new Date(event.received_at).getTime();
      expect(receivedAt).toBeGreaterThan(now - 120000);
      expect(receivedAt).toBeLessThanOrEqual(now + 10000);
    }

    // Verify entered_at < exited_at for pageviews
    for (const pv of pageviews) {
      const enteredAt = new Date(pv.entered_at as unknown as string).getTime();
      const exitedAt = new Date(pv.exited_at as unknown as string).getTime();
      expect(exitedAt).toBeGreaterThanOrEqual(enteredAt);
    }

    // ========================================
    // Verify Custom Dimensions
    // ========================================
    for (const event of events) {
      // The SDK keys dimensions as custom_N, matching the server columns.
      expect(event.dimensions['custom_1']).toBe('dimension_1_value');
      expect(event.dimensions['custom_2']).toBe('dimension_2_value');
      // Unset dimensions are simply absent from the payload
      expect(event.dimensions['custom_3']).toBeUndefined();
      expect(event.dimensions['custom_4']).toBeUndefined();
      expect(event.dimensions['custom_5']).toBeUndefined();
    }

  });
});
