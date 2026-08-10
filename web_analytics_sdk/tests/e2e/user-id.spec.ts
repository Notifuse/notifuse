/**
 * User ID E2E Tests
 *
 * Tests the user identification feature:
 * - Setting user ID via NotifuseAnalytics.setUserId()
 * - Getting user ID via NotifuseAnalytics.getUserId()
 * - User ID persisted across page navigations
 * - Clearing user ID
 */

import { test, expect, truncateWorkspaceTables } from './fixtures';

test.describe('User ID', () => {
  test.beforeEach(async () => {
    await truncateWorkspaceTables();
  });

  test('getUserId() returns null initially', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    const userId = await page.evaluate(() => NotifuseAnalytics.getUserId());
    expect(userId).toBeNull();
  });

  test('setUserId() stores and retrieves user ID', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Set user ID
    await page.evaluate(() => NotifuseAnalytics.setUserId('user_123'));

    // Get user ID
    const userId = await page.evaluate(() => NotifuseAnalytics.getUserId());
    expect(userId).toBe('user_123');
  });

  test('setUserId(null) clears user ID', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Set user ID
    await page.evaluate(() => NotifuseAnalytics.setUserId('temp_user'));

    let userId = await page.evaluate(() => NotifuseAnalytics.getUserId());
    expect(userId).toBe('temp_user');

    // Clear user ID
    await page.evaluate(() => NotifuseAnalytics.setUserId(null));

    userId = await page.evaluate(() => NotifuseAnalytics.getUserId());
    expect(userId).toBeNull();
  });

  test('user_id persists after page reload', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Set user ID
    await page.evaluate(() => NotifuseAnalytics.setUserId('reload_user'));

    // Reload the page
    await page.reload();
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // User ID should be restored from storage
    const userId = await page.evaluate(() => NotifuseAnalytics.getUserId());
    expect(userId).toBe('reload_user');
  });

  test('user_id is included in debug info', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Set user ID
    await page.evaluate(() => NotifuseAnalytics.setUserId('debug_user'));

    // Check debug info includes user ID
    const debugInfo = await page.evaluate(() => NotifuseAnalytics.debug());
    expect((debugInfo as { session?: { userId?: string } }).session?.userId).toBe('debug_user');
  });

  test('reset() clears user ID', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Set user ID
    await page.evaluate(() => NotifuseAnalytics.setUserId('will_reset'));

    let userId = await page.evaluate(() => NotifuseAnalytics.getUserId());
    expect(userId).toBe('will_reset');

    // Reset SDK (should clear user ID)
    await page.evaluate(() => NotifuseAnalytics.reset());

    // Re-init after reset
    await page.evaluate(() => NotifuseAnalytics.init({
      workspace_id: 'test_workspace',
      endpoint: 'http://localhost:4555',
    }));

    userId = await page.evaluate(() => NotifuseAnalytics.getUserId());
    expect(userId).toBeNull();
  });
});
