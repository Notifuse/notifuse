import { test, expect, requestCapture } from '../fixtures/auth'
import { waitForLoading } from '../fixtures/test-utils'
import { API_PATTERNS } from '../fixtures/request-capture'
import { testBroadcastData } from '../fixtures/form-data'
import { logCapturedRequests } from '../fixtures/payload-assertions'
import type { Locator, Page } from '@playwright/test'

const WORKSPACE_ID = 'test-workspace'

// antd 6 puts role="dialog" on the drawer section itself and names it after the
// drawer title, so the title is what the dialog is looked up by.
const CREATE_DRAWER = 'Create a broadcast'
const EDIT_DRAWER = 'Edit broadcast'

// The card header of a given broadcast: it carries the status badge and the
// icon-only action buttons.
const cardHead = (page: Page, broadcastName: string): Locator =>
  page.locator('.ant-card-head').filter({ hasText: broadcastName })

// The whole card of a given broadcast, header plus body.
const card = (page: Page, broadcastName: string): Locator =>
  page.locator('.ant-card').filter({ hasText: broadcastName })

// Opens the creation drawer and returns it.
const openCreateDrawer = async (page: Page): Promise<Locator> => {
  await page.getByRole('button', { name: 'Create Broadcast', exact: true }).click()
  const drawer = page.getByRole('dialog', { name: CREATE_DRAWER })
  await expect(drawer).toBeVisible()
  return drawer
}

// Opens the edit drawer of an existing broadcast and returns it. The edit action is
// an icon-only button, and FontAwesome stamps the icon name onto the svg, which is
// the only stable handle it exposes.
const openEditDrawer = async (page: Page, broadcastName: string): Promise<Locator> => {
  await cardHead(page, broadcastName).locator('button:has(svg[data-icon="pen-to-square"])').click()
  const drawer = page.getByRole('dialog', { name: EDIT_DRAWER })
  await expect(drawer).toBeVisible()
  return drawer
}

// Picks an option in an antd Select rendered inside `drawer`, identified by the
// label of its form item.
const selectOption = async (
  page: Page,
  drawer: Locator,
  fieldLabel: string,
  optionText: string
): Promise<void> => {
  await drawer.getByLabel(fieldLabel, { exact: true }).click()
  await page.locator('.ant-select-dropdown').waitFor({ state: 'visible' })
  await page.locator('.ant-select-item-option').filter({ hasText: optionText }).click()
}

// The text antd 6 shows for the current value of a Select (v5 named it
// .ant-select-selection-item).
const selectValue = (drawer: Locator, fieldLabel: string): Locator =>
  drawer.locator('.ant-form-item').filter({ hasText: fieldLabel }).locator('.ant-select-content')

test.describe('Broadcasts Feature', () => {
  test.describe('Page Load & Empty State', () => {
    test('loads broadcasts page and shows empty state', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      await expect(page.getByText('No broadcasts found')).toBeVisible()
      await expect(page.getByRole('button', { name: 'Create Broadcast', exact: true })).toBeVisible()
    })

    test('loads broadcasts page with data', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      await expect(page).toHaveURL(/broadcasts/)

      // One card per broadcast returned by the API
      await expect(cardHead(page, 'January Newsletter')).toBeVisible()
      await expect(cardHead(page, 'Product Launch')).toBeVisible()
      await expect(cardHead(page, 'A/B Test Campaign')).toBeVisible()
    })
  })

  test.describe('CRUD Operations', () => {
    test('opens create broadcast form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await expect(drawer.getByLabel('Broadcast name')).toBeVisible()
      await expect(drawer.getByLabel('List', { exact: true })).toBeVisible()
      await expect(drawer.getByRole('button', { name: 'Next', exact: true })).toBeVisible()
    })

    test('fills broadcast form', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // Tab 1: Audience - the two required fields
      const nameInput = drawer.getByLabel('Broadcast name')
      await nameInput.fill('Test Marketing Broadcast')
      await expect(nameInput).toHaveValue('Test Marketing Broadcast')

      await selectOption(page, drawer, 'List', 'Newsletter')
      await expect(selectValue(drawer, 'List')).toHaveText('Newsletter')

      await expect(drawer.getByRole('button', { name: 'Next', exact: true })).toBeVisible()
    })

    test('views broadcast details', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // Only the first card is expanded by default
      const productLaunch = card(page, 'Product Launch')
      await productLaunch.getByRole('button', { name: 'Show Details' }).click()

      // The audience the broadcast targets
      await expect(productLaunch.getByText('Marketing Updates')).toBeVisible()
      await expect(productLaunch.getByText('Active Users')).toBeVisible()
    })
  })

  test.describe('Audience Selection', () => {
    test('shows audience selection options', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await expect(drawer.getByLabel('List', { exact: true })).toBeVisible()
      await expect(drawer.getByText('Belonging to at least one of the following segments')).toBeVisible()
      await expect(
        drawer.getByText('Exclude unsubscribed, bounced & complained recipients')
      ).toBeVisible()

      // The list options are the workspace lists
      await drawer.getByLabel('List', { exact: true }).click()
      await page.locator('.ant-select-dropdown').waitFor({ state: 'visible' })
      await expect(page.locator('.ant-select-item-option').filter({ hasText: 'Newsletter' })).toBeVisible()
      await expect(
        page.locator('.ant-select-item-option').filter({ hasText: 'Marketing Updates' })
      ).toBeVisible()
      await expect(
        page.locator('.ant-select-item-option').filter({ hasText: 'Beta Testers' })
      ).toBeVisible()
    })
  })

  test.describe('Scheduling', () => {
    test('shows the scheduled delivery of a scheduled broadcast', async ({
      authenticatedPageWithData
    }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const abTest = card(page, 'A/B Test Campaign')
      await abTest.getByRole('button', { name: 'Show Details' }).click()

      // schedule.scheduled_date 2024-02-01 at 09:00 in UTC
      await expect(abTest.getByText('Feb 1, 2024 9:00 AM in UTC')).toBeVisible()
    })
  })

  test.describe('Status Display', () => {
    test('displays broadcast status', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      await expect(cardHead(page, 'January Newsletter').getByText('Draft')).toBeVisible()
      await expect(cardHead(page, 'Product Launch').getByText('Complete')).toBeVisible()
      await expect(cardHead(page, 'A/B Test Campaign').getByText('Scheduled')).toBeVisible()
    })

    test('shows draft broadcasts', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // A draft is the only status that offers both editing and sending
      const draft = cardHead(page, 'January Newsletter')
      await expect(draft.locator('button:has(svg[data-icon="pen-to-square"])')).toBeVisible()
      await expect(draft.getByRole('button', { name: 'Send or Schedule' })).toBeVisible()

      // A completed broadcast offers neither
      const sent = cardHead(page, 'Product Launch')
      await expect(sent.locator('button:has(svg[data-icon="pen-to-square"])')).toHaveCount(0)
      await expect(sent.getByRole('button', { name: 'Send or Schedule' })).toHaveCount(0)
    })
  })

  test.describe('Statistics', () => {
    test('displays broadcast statistics', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const stats = card(page, 'January Newsletter')
      for (const label of [
        'Sent',
        'Delivered',
        'Opens',
        'Clicks',
        'Failed',
        'Bounced',
        'Complaints',
        'Unsub.'
      ]) {
        await expect(stats.getByText(label, { exact: true })).toBeVisible()
      }
    })
  })

  test.describe('Edit Form Prefill', () => {
    test('edit broadcast drawer shows existing broadcast name', async ({
      authenticatedPageWithData
    }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page, 'January Newsletter')

      await expect(drawer.getByLabel('Broadcast name')).toHaveValue('January Newsletter')
    })

    test('edit broadcast preserves list selection', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page, 'January Newsletter')

      // audience.list is list-1, whose name the select displays
      await expect(selectValue(drawer, 'List')).toHaveText('Newsletter')
    })

    test('edit broadcast preserves template selection', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page, 'January Newsletter')

      // The template lives on the last tab
      await drawer.getByRole('tab', { name: '4. Content' }).click()

      // test_settings.variations[0].template_id is tpl-2
      await expect(drawer.getByPlaceholder('Select template')).toHaveValue('Monthly Newsletter')
    })

    test('edit draft broadcast shows correct status', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page, 'January Newsletter')

      // A draft stays fully editable
      await expect(drawer.getByLabel('Broadcast name')).toBeEnabled()
      await expect(drawer.getByLabel('List', { exact: true })).toBeEnabled()
      await expect(drawer.getByRole('button', { name: 'Next', exact: true })).toBeVisible()
    })
  })

  test.describe('Form Validation', () => {
    test('requires broadcast name', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByRole('button', { name: 'Next', exact: true }).click()

      await expect(drawer.getByText('Please enter a broadcast name')).toBeVisible()
      // The step must not advance while the audience tab is invalid
      await expect(drawer.getByLabel('Broadcast name')).toBeVisible()
    })

    test('requires list selection', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByLabel('Broadcast name').fill('Test Broadcast')

      await drawer.getByRole('button', { name: 'Next', exact: true }).click()

      await expect(drawer.getByText('Please select a list')).toBeVisible()
      await expect(drawer.getByText('Please enter a broadcast name')).toHaveCount(0)
    })
  })

  test.describe('Navigation', () => {
    test('navigates to broadcasts from sidebar', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      // Start at dashboard
      await page.goto(`/console/workspace/${WORKSPACE_ID}/`)
      await waitForLoading(page)

      // Click broadcasts link in sidebar
      const broadcastsLink = page.locator('a[href*="broadcasts"], [data-menu-id*="broadcasts"]').first()
      await broadcastsLink.click()

      // Should be on broadcasts page
      await expect(page).toHaveURL(/broadcasts/)
    })

    test('can close create form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // An untouched form closes without the unsaved-changes confirmation
      await drawer.getByRole('button', { name: 'Close' }).click()

      await expect(drawer).toBeHidden()
    })
  })

  test.describe('Full Form Submission with Payload Verification', () => {
    test('creates broadcast with all fields and verifies payload', async ({
      authenticatedPageWithData
    }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // Tab 1: Audience
      await drawer.getByLabel('Broadcast name').fill(testBroadcastData.name)
      await selectOption(page, drawer, 'List', 'Newsletter')
      await drawer.getByRole('button', { name: 'Next', exact: true }).click()

      // Tab 2: Web Analytics
      await drawer.getByLabel('utm_source').fill(testBroadcastData.utm_source!)
      await drawer.getByLabel('utm_medium').fill(testBroadcastData.utm_medium!)
      await drawer.getByLabel('utm_campaign').fill(testBroadcastData.utm_campaign!)
      await drawer.getByRole('button', { name: 'Next', exact: true }).click()

      // Tab 3: Data Feeds - left at its defaults
      await drawer.getByRole('button', { name: 'Next', exact: true }).click()

      // Tab 4: Content
      await drawer.getByPlaceholder('Select template').click()
      const picker = page.getByRole('dialog', { name: 'Select Template' })
      await picker
        .getByRole('listitem')
        .filter({ hasText: 'Monthly Newsletter' })
        .getByRole('button', { name: 'Select', exact: true })
        .click()
      await expect(drawer.getByPlaceholder('Select template')).toHaveValue('Monthly Newsletter')

      await drawer.getByRole('button', { name: 'Save', exact: true }).click()

      await expect
        .poll(() => requestCapture.getRequestCount(API_PATTERNS.BROADCAST_CREATE))
        .toBeGreaterThan(0)
      logCapturedRequests(requestCapture)

      const request = requestCapture.getLastRequest(API_PATTERNS.BROADCAST_CREATE)
      expect(request?.body, 'Broadcast create body should not be empty').toBeTruthy()

      const body = request!.body as {
        workspace_id?: string
        name?: string
        audience?: { list?: string; exclude_unsubscribed?: boolean }
        utm_parameters?: Record<string, unknown>
        test_settings?: { enabled?: boolean; variations?: Array<{ template_id?: string }> }
      }

      expect(body.workspace_id).toBe(WORKSPACE_ID)
      expect(body.name).toBe(testBroadcastData.name)
      expect(body.audience?.list).toBe('list-1')
      expect(body.audience?.exclude_unsubscribed).toBe(testBroadcastData.exclude_unsubscribed)
      expect(body.utm_parameters?.source).toBe(testBroadcastData.utm_source)
      expect(body.utm_parameters?.medium).toBe(testBroadcastData.utm_medium)
      expect(body.utm_parameters?.campaign).toBe(testBroadcastData.utm_campaign)
      expect(body.test_settings?.enabled).toBe(testBroadcastData.test_enabled)
      expect(body.test_settings?.variations?.[0]?.template_id).toBe('tpl-2')
    })
  })
})
