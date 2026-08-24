import { test, expect, Page, Route } from '@playwright/test'

// Helper to create JSON response
const jsonResponse = (data: unknown) => ({
  status: 200,
  contentType: 'application/json',
  body: JSON.stringify(data)
})

// Track console errors for each test
function setupConsoleErrorTracking(page: Page): string[] {
  const errors: string[] = []
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      const text = msg.text()
      // Ignore known benign errors
      if (
        !text.includes('favicon') &&
        !text.includes('Failed to load resource') &&
        !text.includes('net::ERR')
      ) {
        errors.push(text)
      }
    }
  })
  return errors
}

test.describe('Public Pages Load', () => {
  test.beforeEach(async ({ page }) => {
    // Mock config.js - system is installed so we can access signin
    await page.route('**/config.js', (route: Route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/javascript',
        body: `
          window.API_URL = "https://localapi.notifuse.com:4000";
          window.ROOT_EMAIL = "test@example.com";
          window.IS_INSTALLED = true;
        `
      })
    )

    // Intercept all fetch/xhr requests to the API backend
    await page.route('https://localapi.notifuse.com:4000/**', (route: Route) => {
      const url = route.request().url()
      const resourceType = route.request().resourceType()

      // Only mock fetch/xhr requests
      if (resourceType !== 'fetch' && resourceType !== 'xhr') {
        return route.continue()
      }

      // Return unauthorized for user.me (not logged in)
      if (url.includes('/api/user.me')) {
        return route.fulfill({
          status: 401,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'Unauthorized' })
        })
      }

      // Default: return empty success
      return route.fulfill(jsonResponse({}))
    })
  })

  test('SignInPage loads and renders email form', async ({ page }) => {
    const errors = setupConsoleErrorTracking(page)

    await page.goto('/console/signin')

    // Wait for page to be fully loaded
    await page.waitForLoadState('networkidle')

    // Verify the Sign In card is visible
    await expect(page.locator('.ant-card-head-title').filter({ hasText: 'Sign In' })).toBeVisible({
      timeout: 10000
    })

    // Verify email input is present
    await expect(page.locator('input[type="email"]')).toBeVisible()

    // Verify submit button is present
    await expect(page.locator('button[type="submit"]').filter({ hasText: 'Send Magic Code' })).toBeVisible()

    // Check for critical console errors
    const criticalErrors = errors.filter(
      (e) => !e.includes('401') && !e.includes('Unauthorized')
    )
    expect(criticalErrors).toHaveLength(0)
  })

  test('LogoutPage loads correctly', async ({ page }) => {
    const errors = setupConsoleErrorTracking(page)

    await page.goto('/console/logout')

    // Wait for page to be fully loaded
    await page.waitForLoadState('networkidle')

    // Logging out drops the session and lands on the sign in form
    await expect(page).toHaveURL(/\/console\/signin/, { timeout: 10000 })
    await expect(page.locator('.ant-card-head-title').filter({ hasText: 'Sign In' })).toBeVisible()
    await expect(page.locator('input[type="email"]')).toBeVisible()

    // Check for critical console errors
    const criticalErrors = errors.filter(
      (e) => !e.includes('401') && !e.includes('Unauthorized')
    )
    expect(criticalErrors).toHaveLength(0)
  })

  test('AcceptInvitationPage loads correctly', async ({ page }) => {
    const errors = setupConsoleErrorTracking(page)

    // A token the backend refuses, which is what an expired invitation link looks like.
    // Registered after the catch-all in beforeEach, so it wins.
    await page.route('**/api/workspaces.verifyInvitationToken', (route: Route) =>
      route.fulfill({
        status: 400,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'invitation token is invalid or has expired' })
      })
    )

    await page.goto('/console/accept-invitation?token=test-token')

    // Wait for page to be fully loaded
    await page.waitForLoadState('networkidle')

    // The page reports the rejection instead of the invitation, and offers a way out.
    // Scoped to the card because the same message also pops up as a toast.
    const card = page.locator('.ant-card')
    await expect(card.getByRole('heading', { name: 'Invalid Invitation' })).toBeVisible()
    await expect(card.getByText('invitation token is invalid or has expired')).toBeVisible()
    await expect(card.getByRole('button', { name: 'Go to Sign In' })).toBeVisible()

    // Check for critical console errors
    const criticalErrors = errors.filter(
      (e) => !e.includes('401') && !e.includes('Unauthorized') && !e.includes('invitation token')
    )
    expect(criticalErrors).toHaveLength(0)
  })

  test('SetupWizard loads correctly', async ({ page }) => {
    const errors = setupConsoleErrorTracking(page)

    // Mock setup status endpoint
    await page.route('**/api/setup.status*', (route: Route) =>
      route.fulfill(jsonResponse({ completed: false, step: 1 }))
    )

    await page.goto('/console/setup')

    // Wait for page to be fully loaded
    await page.waitForLoadState('networkidle')

    // The wizard asks for the root account and the SMTP settings it sends mail with
    await expect(page.getByRole('heading', { name: 'Setup' })).toBeVisible()
    await expect(page.getByText('Root Email')).toBeVisible()
    await expect(page.getByText('SMTP Configuration')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Complete Setup' })).toBeVisible()

    // Check for critical console errors
    const criticalErrors = errors.filter(
      (e) => !e.includes('401') && !e.includes('Unauthorized')
    )
    expect(criticalErrors).toHaveLength(0)
  })
})
