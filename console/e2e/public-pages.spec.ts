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

    // The logout page should redirect to signin (or setup if not configured)
    // After logout, user is typically redirected to sign in
    await expect(page).toHaveURL(/signin|logout|setup/, { timeout: 10000 })

    // Page should load without crashing
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors
    const criticalErrors = errors.filter(
      (e) => !e.includes('401') && !e.includes('Unauthorized')
    )
    expect(criticalErrors).toHaveLength(0)
  })

  test('AcceptInvitationPage loads correctly', async ({ page }) => {
    const errors = setupConsoleErrorTracking(page)

    await page.goto('/console/accept-invitation?token=test-token')

    // Wait for page to be fully loaded
    await page.waitForLoadState('networkidle')

    // The page should load without crashing
    // It may show an error for invalid token, but that's expected
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors - ignore React boundary errors as the page may error without a valid token
    const criticalErrors = errors.filter(
      (e) =>
        !e.includes('401') &&
        !e.includes('Unauthorized') &&
        !e.includes('Invalid') &&
        !e.includes('error boundary') &&
        !e.includes('AcceptInvitationPage')
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

    // The page should load - either showing setup wizard or redirecting
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors
    const criticalErrors = errors.filter(
      (e) => !e.includes('401') && !e.includes('Unauthorized')
    )
    expect(criticalErrors).toHaveLength(0)
  })
})
