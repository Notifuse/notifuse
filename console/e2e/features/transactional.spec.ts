import { test, expect, requestCapture } from '../fixtures/auth'
import { waitForLoading } from '../fixtures/test-utils'
import { API_PATTERNS } from '../fixtures/request-capture'
import { testTransactionalData } from '../fixtures/form-data'
import { logCapturedRequests } from '../fixtures/payload-assertions'
import type { Locator, Page } from '@playwright/test'

const WORKSPACE_ID = 'test-workspace'

// antd 6 puts role="dialog" on the drawer section itself and names it after the
// drawer title, so the title is what the dialog is looked up by.
const CREATE_DRAWER = 'Create a notification'
const EDIT_DRAWER = 'Edit notification'

// The table row of a notification, found by its name.
const notificationRow = (page: Page, notificationName: string): Locator =>
  page.locator('tr').filter({ has: page.getByText(notificationName, { exact: true }) })

// A button of a row, shown when the row is hovered.
const rowAction = async (page: Page, notificationName: string, label: string): Promise<Locator> => {
  const row = notificationRow(page, notificationName)
  await row.hover()
  return row.getByRole('button', { name: label, exact: true })
}

// Opens the edit drawer of an existing notification from its row menu and returns it.
const openEditDrawer = async (page: Page, notificationName: string): Promise<Locator> => {
  await (await rowAction(page, notificationName, 'More actions')).click()
  await page.getByRole('menuitem', { name: 'Edit', exact: true }).click()
  const drawer = page.getByRole('dialog', { name: EDIT_DRAWER })
  await expect(drawer).toBeVisible()
  return drawer
}

// Opens the creation drawer and returns it.
const openCreateDrawer = async (page: Page): Promise<Locator> => {
  await page.getByRole('button', { name: 'New notification', exact: true }).click()
  const drawer = page.getByRole('dialog', { name: CREATE_DRAWER })
  await expect(drawer).toBeVisible()
  return drawer
}

test.describe('Transactional Notifications Feature', () => {
  test.describe('Page Load & Empty State', () => {
    test('loads transactional page and shows empty state', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      await expect(page.getByText('No transactional notifications found')).toBeVisible()
      await expect(
        page.getByRole('button', { name: 'New notification', exact: true })
      ).toBeVisible()
    })

    test('loads transactional page with data', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      await expect(page).toHaveURL(/transactional/)

      // One row per notification returned by the API
      await expect(notificationRow(page, 'Password Reset')).toBeVisible()
      await expect(notificationRow(page, 'Order Confirmation')).toBeVisible()
      await expect(notificationRow(page, 'Account Verification')).toBeVisible()
      await expect(page.getByText('3 notifications', { exact: true })).toBeVisible()
    })
  })

  test.describe('CRUD Operations', () => {
    test('opens create notification form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await expect(drawer.getByLabel('Notification name')).toBeVisible()
      await expect(drawer.getByLabel('API Identifier')).toBeVisible()
      await expect(drawer.getByPlaceholder('Select email template')).toBeVisible()
    })

    test('fills transactional notification form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      const nameInput = drawer.getByLabel('Notification name')
      await nameInput.fill('Password Reset Email')
      await expect(nameInput).toHaveValue('Password Reset Email')

      // The API identifier is derived from the name for a new notification
      await expect(drawer.getByLabel('API Identifier')).toHaveValue('password_reset_email')

      const descriptionInput = drawer.getByLabel('Description')
      await descriptionInput.fill('Sends a password reset email to users')
      await expect(descriptionInput).toHaveValue('Sends a password reset email to users')

      await expect(drawer.getByRole('button', { name: 'Save', exact: true })).toBeVisible()
    })

    test('views notification details', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      // Each row carries the notification's API id; its sheet, the description
      await expect(notificationRow(page, 'Password Reset').getByText('transactional-1', { exact: true })).toBeVisible()
      await notificationRow(page, 'Password Reset').click()
      await expect(page).toHaveURL(/notification=transactional-1/)
      const sheet = page.locator('.app-full-drawer')
      await expect(sheet.getByText('Sent when user requests password reset')).toBeVisible()
      await expect(sheet.getByText('1 of 3')).toBeVisible()
    })

    test('opens the sheet of a notification from its URL', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications?notification=transactional-2`)
      await waitForLoading(page)

      const sheet = page.locator('.app-full-drawer')
      await expect(sheet.getByRole('heading', { name: 'Order Confirmation' })).toBeVisible()
      await expect(sheet.getByText('2 of 3')).toBeVisible()

      // J goes to the next row, in place of the current history entry
      await page.keyboard.press('j')
      await expect(page).toHaveURL(/notification=transactional-3/)
      await expect(sheet.getByRole('heading', { name: 'Account Verification' })).toBeVisible()
    })
  })

  test.describe('Configuration', () => {
    test('shows template selection', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // The template field opens a picker listing the workspace templates
      await drawer.getByPlaceholder('Select email template').click()

      const picker = page.getByRole('dialog', { name: 'Select Template' })
      await expect(picker).toBeVisible()
      await expect(picker.getByRole('listitem').filter({ hasText: 'Welcome Email' })).toBeVisible()
    })

    test('shows tracking settings', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // Tracking defaults to following the workspace setting
      await expect(drawer.getByText('Follow workspace setting')).toBeVisible()

      // UTM defaults a new notification starts with
      await expect(drawer.getByLabel('utm_medium')).toHaveValue('email')
      await expect(drawer.getByLabel('utm_campaign')).toHaveValue('transactional')
    })
  })

  test.describe('API Integration Display', () => {
    test('shows the API command of a notification', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      await (await rowAction(page, 'Password Reset', 'API command')).click()

      const modal = page.getByRole('dialog', { name: 'API Command' })
      await expect(modal).toBeVisible()

      // The snippet targets the send endpoint with this notification's id
      await expect(modal).toContainText('/api/transactional.send')
      await expect(modal).toContainText('transactional-1')
      // A click on a row button does not open the sheet
      await expect(page).not.toHaveURL(/notification=/)
    })

    test('opens every language above the sheet, where J does nothing', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications?notification=transactional-1`)
      await waitForLoading(page)

      const sheet = page.locator('.app-full-drawer')
      await expect(sheet.getByRole('heading', { name: 'Password Reset' })).toBeVisible()
      await sheet.getByRole('button', { name: 'All languages' }).click()

      const modal = page.getByRole('dialog', { name: 'API Command' })
      await expect(modal).toBeVisible()
      await page.keyboard.press('j')
      await expect(page).toHaveURL(/notification=transactional-1/)
      await expect(modal).toBeVisible()
    })
  })

  test.describe('Edit Form Prefill', () => {
    test('edit notification drawer shows existing notification name', async ({
      authenticatedPageWithData
    }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page, 'Password Reset')

      await expect(drawer.getByLabel('Notification name')).toHaveValue('Password Reset')
    })

    test('edit notification preserves API identifier', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page, 'Password Reset')

      // The identifier is immutable once the notification exists
      const idInput = drawer.getByLabel('API Identifier')
      await expect(idInput).toHaveValue('transactional-1')
      await expect(idInput).toBeDisabled()
    })

    test('edit notification preserves description', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page, 'Password Reset')

      await expect(drawer.getByLabel('Description')).toHaveValue(
        'Sent when user requests password reset'
      )
    })

    test('edit notification preserves template selection', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page, 'Password Reset')

      // channels.email.template_id is tpl-1, whose name the picker input displays
      await expect(drawer.getByPlaceholder('Select email template')).toHaveValue('Welcome Email')
    })

    test('edit notification preserves tracking settings', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page, 'Password Reset')

      // Stored tracking_mode 'inherit' plus the stored UTM parameters
      await expect(drawer.getByText('Follow workspace setting')).toBeVisible()
      await expect(drawer.getByLabel('utm_source')).toHaveValue('notifuse')
      await expect(drawer.getByLabel('utm_content')).toHaveValue('password_reset')
    })
  })

  test.describe('Form Validation', () => {
    test('requires notification name', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByRole('button', { name: 'Save', exact: true }).click()

      await expect(drawer.getByText('Please enter a notification name')).toBeVisible()
    })

    test('requires email template selection', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // Name and identifier satisfied, template still missing
      await drawer.getByLabel('Notification name').fill('Test Notification')

      await drawer.getByRole('button', { name: 'Save', exact: true }).click()

      await expect(drawer.getByText('Please select an email template')).toBeVisible()
      await expect(drawer.getByText('Please enter a notification name')).toHaveCount(0)
    })
  })

  test.describe('Navigation', () => {
    test('navigates to transactional from sidebar', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      // Start at dashboard
      await page.goto(`/console/workspace/${WORKSPACE_ID}/`)
      await waitForLoading(page)

      // Click transactional link in sidebar
      const transactionalLink = page
        .locator('a[href*="transactional"], [data-menu-id*="transactional"]')
        .first()
      await transactionalLink.click()

      // Should be on transactional page
      await expect(page).toHaveURL(/transactional/)
    })

    test('can close create form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // An untouched form closes without the unsaved-changes confirmation
      await drawer.getByRole('button', { name: 'Close' }).click()

      await expect(drawer).toBeHidden()
    })
  })

  test.describe('Full Form Submission with Payload Verification', () => {
    test('creates transactional notification with all fields and verifies payload', async ({
      authenticatedPageWithData
    }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByLabel('Notification name').fill(testTransactionalData.name)

      // testTransactionalData.id contains dashes, which the API identifier rejects;
      // the value the form derives from the name is what actually gets submitted,
      // cut to the identifier's 32 characters.
      const expectedId = 'e2e_test_transactional_notificat'
      await expect(drawer.getByLabel('API Identifier')).toHaveValue(expectedId)

      await drawer.getByLabel('Description').fill(testTransactionalData.description!)

      // Pick the email template through the picker drawer
      await drawer.getByPlaceholder('Select email template').click()
      const picker = page.getByRole('dialog', { name: 'Select Template' })
      await picker
        .getByRole('listitem')
        .filter({ hasText: 'Welcome Email' })
        .getByRole('button', { name: 'Select', exact: true })
        .click()
      await expect(drawer.getByPlaceholder('Select email template')).toHaveValue('Welcome Email')

      await drawer.getByLabel('utm_source').fill('e2e-test')

      await drawer.getByRole('button', { name: 'Save', exact: true }).click()

      await expect
        .poll(() => requestCapture.getRequestCount(API_PATTERNS.TRANSACTIONAL_CREATE))
        .toBeGreaterThan(0)
      logCapturedRequests(requestCapture)

      const request = requestCapture.getLastRequest(API_PATTERNS.TRANSACTIONAL_CREATE)
      expect(request?.body, 'Transactional create body should not be empty').toBeTruthy()

      const body = request!.body as {
        workspace_id?: string
        notification?: {
          id?: string
          name?: string
          description?: string
          channels?: { email?: { template_id?: string } }
          tracking_settings?: Record<string, unknown>
        }
      }

      expect(body.workspace_id).toBe(WORKSPACE_ID)
      expect(body.notification?.id).toBe(expectedId)
      expect(body.notification?.name).toBe(testTransactionalData.name)
      expect(body.notification?.description).toBe(testTransactionalData.description)
      expect(body.notification?.channels?.email?.template_id).toBe('tpl-1')
      expect(body.notification?.tracking_settings?.utm_source).toBe('e2e-test')
      expect(body.notification?.tracking_settings?.utm_content).toBe(expectedId)
    })
  })
})
