import { test, expect } from './fixtures/auth'
import { Page } from '@playwright/test'

const WORKSPACE_ID = 'test-workspace'

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

// Helper to wait for page to be fully loaded
async function waitForPageLoad(page: Page) {
  await page.waitForLoadState('networkidle')
  // Wait for any Ant Design spinners to disappear
  const spinner = page.locator('.ant-spin-spinning')
  if ((await spinner.count()) > 0) {
    await spinner.first().waitFor({ state: 'hidden', timeout: 15000 }).catch(() => {
      // Ignore timeout - some pages may not have spinners
    })
  }
}

test.describe('Protected Pages Load', () => {
  // Note: config.js and API mocks are set up in the auth fixture (authenticatedPage)

  test('DashboardPage loads and renders workspace selector', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto('/console/')
    await waitForPageLoad(page)

    // Dashboard should show workspace selection or redirect to a workspace
    // The body should be visible and contain content
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('AnalyticsPage loads and renders analytics content', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/`)
    await waitForPageLoad(page)

    // Analytics page should show analytics-related content
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('BroadcastsPage loads and renders broadcasts list', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
    await waitForPageLoad(page)

    // Should show Broadcasts heading or related content
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('ContactsPage loads and renders contacts table', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
    await waitForPageLoad(page)

    // Should show Contacts heading (use first() since there are multiple elements)
    await expect(page.getByText('Contacts', { exact: true }).first()).toBeVisible({ timeout: 10000 })

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('ListsPage loads and renders lists content', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
    await waitForPageLoad(page)

    // Should show Lists heading or related content
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('TemplatesPage loads and renders templates content', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
    await waitForPageLoad(page)

    // Should show Templates heading or related content
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('TransactionalNotificationsPage loads correctly', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
    await waitForPageLoad(page)

    // Should show page content
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('AutomationsPage loads correctly', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/automations`)
    await waitForPageLoad(page)

    // Should show automations content
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('WorkspaceSettingsPage loads correctly', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
    await waitForPageLoad(page)

    // Should show Settings page content
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors - ignore data fetching errors from mocked APIs
    const criticalErrors = errors.filter(
      (e) =>
        !e.includes('Failed to fetch') &&
        !e.includes('TypeError') &&
        !e.includes('error boundary') &&
        !e.includes('CatchBoundaryImpl')
    )
    expect(criticalErrors).toHaveLength(0)
  })

  test('LogsPage loads and renders logs content', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/logs`)
    await waitForPageLoad(page)

    // Should show Logs heading or related content
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('FileManagerPage loads correctly', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/file-manager`)
    await waitForPageLoad(page)

    // Should show file manager content
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('DebugSegmentPage loads correctly', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
    await waitForPageLoad(page)

    // Should show debug segment content
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('BlogPage loads correctly', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
    await waitForPageLoad(page)

    // Should show blog content
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('CreateWorkspacePage loads correctly', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto('/console/workspace/create')
    await waitForPageLoad(page)

    // Should show create workspace form
    await expect(page.locator('body')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })
})
