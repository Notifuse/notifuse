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
        !text.includes('net::ERR') &&
        !text.includes('CatchBoundaryImpl') &&
        !text.includes('error boundary') &&
        !text.includes('recreate this component tree') &&
        !text.includes('The above error occurred')
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
  // Wait for any Ant Design spinners to disappear. A page that never shows one
  // matches nothing and satisfies this straight away.
  await expect(page.locator('.ant-spin-spinning')).toHaveCount(0)
}

test.describe('Protected Pages Load', () => {
  // Note: config.js and API mocks are set up in the auth fixture (authenticatedPage)

  test('DashboardPage loads and renders workspace selector', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto('/console/')
    await waitForPageLoad(page)

    // The selector lists the workspaces the mocked user belongs to
    await expect(page.getByRole('heading', { name: 'Select workspace' })).toBeVisible()
    await expect(page.getByText('Test Workspace')).toBeVisible()
    await expect(page.getByText('ID: test-workspace')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('AnalyticsPage loads and renders analytics content', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/`)
    await waitForPageLoad(page)

    // The KPI strip, the chart of sends and the cards make up the dashboard
    await expect(page.getByText('Emails sent', { exact: true })).toBeVisible()
    await expect(page.getByText('Emails sent per day')).toBeVisible()
    await expect(page.getByText('Recent failures')).toBeVisible()
    await expect(page.getByText('Sending', { exact: true })).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('BroadcastsPage loads and renders broadcasts list', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
    await waitForPageLoad(page)

    // The fixture serves no broadcasts, so the list shows its empty state
    await expect(page.getByText('No broadcasts yet')).toBeVisible()
    await expect(page.getByRole('button', { name: 'New broadcast' })).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('ContactsPage loads and renders contacts table', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
    await waitForPageLoad(page)

    // The table renders with its columns even when the fixture serves no contacts
    const header = page.locator('.ant-table-thead')
    await expect(header.getByText('Contact', { exact: true })).toBeVisible()
    await expect(header.getByText('Lists', { exact: true })).toBeVisible()
    await expect(header.getByText('Segments', { exact: true })).toBeVisible()
    await expect(page.getByText('No contacts yet')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('the former Lists page opens Contacts, with the lists in its rail', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
    await waitForPageLoad(page)

    await expect(page).toHaveURL(new RegExp(`/console/workspace/${WORKSPACE_ID}/contacts`))
    await expect(page.getByRole('button', { name: 'Create your first list' })).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('TemplatesPage loads and renders templates content', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
    await waitForPageLoad(page)

    await expect(page.getByText('No templates found', { exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: 'New template' }).first()).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('TransactionalNotificationsPage loads correctly', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
    await waitForPageLoad(page)

    await expect(page.getByText('No transactional notifications found')).toBeVisible()
    await expect(page.getByRole('button', { name: 'New notification' })).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('AutomationsPage loads correctly', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/automations`)
    await waitForPageLoad(page)

    await expect(page.getByRole('heading', { name: 'Automations' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Create your first automation' })).toBeVisible()
    await expect(page.getByText('No automations yet')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('WorkspaceSettingsPage loads correctly', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
    await waitForPageLoad(page)

    // Settings rail plus the team table, with the signed-in owner in it
    await expect(page.getByRole('navigation', { name: 'Settings' })).toBeVisible()
    await expect(page.getByText('Who can do what in this workspace')).toBeVisible()
    const memberRow = page.locator('.ant-table-row').filter({ hasText: 'test@example.com' })
    await expect(memberRow).toContainText('Owner')

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('LogsPage loads and renders logs content', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/logs`)
    await waitForPageLoad(page)

    await expect(page.getByText('Everything sent, received and changed in this workspace')).toBeVisible()
    await expect(page.getByText('No messages found')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('FileManagerPage loads correctly', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/file-manager`)
    await waitForPageLoad(page)

    // The mocked workspace has no storage settings, so it asks to configure them
    await expect(page.getByText('File storage is not configured.')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Configure now' })).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('DebugSegmentPage loads correctly', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/debug-segment`)
    await waitForPageLoad(page)

    await expect(page.getByRole('heading', { name: 'Debug Segment Builder' })).toBeVisible()
    await expect(page.getByText('Segment Conditions')).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('BlogPage loads correctly', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
    await waitForPageLoad(page)

    // Category rail on the left, posts on the right
    await expect(page.getByText('Categories', { exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'All posts' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'New post' })).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })

  test('CreateWorkspacePage loads correctly', async ({ authenticatedPage }) => {
    const page = authenticatedPage
    const errors = setupConsoleErrorTracking(page)

    await page.goto('/console/workspace/create')
    await waitForPageLoad(page)

    await expect(page.getByRole('heading', { name: 'New workspace' })).toBeVisible()
    await expect(page.getByText('Workspace Name')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Create Workspace' })).toBeVisible()

    // Check for critical console errors
    expect(errors).toHaveLength(0)
  })
})
