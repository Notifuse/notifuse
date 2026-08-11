/**
 * Identity E2E Tests
 *
 * Tests the verified-contact identification feature end to end:
 * - identify(email, hmac) attaches an identity, getIdentity() returns the object
 * - The identity survives a reload because it lives under its own storage key
 * - clearIdentity() and reset() drop it
 *
 * The address is never truncated or normalized client-side: the customer's
 * server signed the raw string, so the SDK must hand back exactly what it was
 * given for the hmac to still verify.
 */

import { test, expect, truncateWorkspaceTables } from './fixtures';

// Same key the SDK writes through Storage (nf_ prefix + STORAGE_KEYS.IDENTITY).
const IDENTITY_KEY = 'nf_identity';
const SESSION_KEY = 'nf_session';

test.describe('Identity', () => {
  test.beforeEach(async () => {
    await truncateWorkspaceTables();
  });

  test('getIdentity() returns null initially', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    const identity = await page.evaluate(() => NotifuseAnalytics.getIdentity());
    expect(identity).toBeNull();
  });

  test('identify() stores the email and its hmac verbatim', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // Mixed case on purpose: lowercasing here would invalidate the signature.
    await page.evaluate(() => NotifuseAnalytics.identify('User_123@Example.com', 'sig_abc'));

    const identity = await page.evaluate(() => NotifuseAnalytics.getIdentity());
    expect(identity).toEqual({ email: 'User_123@Example.com', hmac: 'sig_abc' });
  });

  test('identify() rejects an unsigned address', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    // /track is public, so an address without an hmac is worthless — it must
    // fail loudly rather than look like a successful identification.
    const error = await page.evaluate(async () => {
      try {
        // @ts-expect-error - deliberately calling with a missing hmac
        await NotifuseAnalytics.identify('no_hmac@example.com');
        return null;
      } catch (err) {
        return (err as Error).message;
      }
    });
    expect(error).toContain('identify(email, hmac)');

    const identity = await page.evaluate(() => NotifuseAnalytics.getIdentity());
    expect(identity).toBeNull();
  });

  test('identity persists after page reload', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    await page.evaluate(() => NotifuseAnalytics.identify('reload_user@example.com', 'sig_reload'));

    await page.reload();
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    const identity = await page.evaluate(() => NotifuseAnalytics.getIdentity());
    expect(identity).toEqual({ email: 'reload_user@example.com', hmac: 'sig_reload' });
  });

  test('identity is read back from its own key, not from the session blob', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    await page.evaluate(() => NotifuseAnalytics.identify('durable@example.com', 'sig_durable'));

    // The credential gets its own durable key.
    const stored = await page.evaluate((key) => localStorage.getItem(key), IDENTITY_KEY);
    expect(stored).not.toBeNull();
    expect(JSON.parse(stored as string)).toEqual({
      email: 'durable@example.com',
      hmac: 'sig_durable',
    });

    // Simulate a second tab that still holds a pre-identification copy of the
    // session and writes it back on its next beat: the blob loses the identity
    // while the dedicated key keeps it. Resuming must trust the key, otherwise
    // every later page load goes anonymous with the credential intact in storage.
    // Stripping it from an init script rather than from this page leaves no
    // window in which the still-running SDK could save the blob back.
    await page.addInitScript((key) => {
      const raw = localStorage.getItem(key);
      if (!raw) return;
      const session = JSON.parse(raw) as { identity: unknown };
      session.identity = null;
      localStorage.setItem(key, JSON.stringify(session));
    }, SESSION_KEY);

    await page.reload();
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    const identity = await page.evaluate(() => NotifuseAnalytics.getIdentity());
    expect(identity).toEqual({ email: 'durable@example.com', hmac: 'sig_durable' });
  });

  test('identity is exposed on the debug session', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    await page.evaluate(() => NotifuseAnalytics.identify('debug_user@example.com', 'sig_debug'));

    // debug() carries the whole Session; the identity rides on it, there is no
    // top-level user id any more.
    const session = await page.evaluate(() => NotifuseAnalytics.debug().session);
    expect(session?.identity).toEqual({ email: 'debug_user@example.com', hmac: 'sig_debug' });
  });

  test('clearIdentity() drops the identity and its storage key', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    await page.evaluate(() => NotifuseAnalytics.identify('temp_user@example.com', 'sig_temp'));
    expect(await page.evaluate(() => NotifuseAnalytics.getIdentity())).toEqual({
      email: 'temp_user@example.com',
      hmac: 'sig_temp',
    });

    await page.evaluate(() => NotifuseAnalytics.clearIdentity());

    expect(await page.evaluate(() => NotifuseAnalytics.getIdentity())).toBeNull();
    // Removing the key is what makes it stick: a leftover key would be reloaded
    // on the next page load.
    expect(await page.evaluate((key) => localStorage.getItem(key), IDENTITY_KEY)).toBeNull();
  });

  test('identity does not survive a reload after clearIdentity()', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    await page.evaluate(() => NotifuseAnalytics.identify('logout@example.com', 'sig_logout'));
    await page.evaluate(() => NotifuseAnalytics.clearIdentity());

    await page.reload();
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    const identity = await page.evaluate(() => NotifuseAnalytics.getIdentity());
    expect(identity).toBeNull();
  });

  test('reset() clears identity', async ({ page }) => {
    await page.goto('/test-page.html');
    await page.waitForFunction(() => window.SDK_INITIALIZED);
    await page.evaluate(() => window.SDK_READY);

    await page.evaluate(() => NotifuseAnalytics.identify('will_reset@example.com', 'sig_reset'));
    expect(await page.evaluate(() => NotifuseAnalytics.getIdentity())).toEqual({
      email: 'will_reset@example.com',
      hmac: 'sig_reset',
    });

    // reset() starts a brand new session in place — no re-init needed.
    await page.evaluate(() => NotifuseAnalytics.reset());

    expect(await page.evaluate(() => NotifuseAnalytics.getIdentity())).toBeNull();
    expect(await page.evaluate((key) => localStorage.getItem(key), IDENTITY_KEY)).toBeNull();
  });
});
