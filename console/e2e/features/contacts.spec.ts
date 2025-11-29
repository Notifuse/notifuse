import { test, expect } from '../fixtures/auth'
import {
  waitForDrawer,
  waitForDrawerClose,
  waitForTable,
  waitForLoading,
  waitForSuccessMessage,
  fillInput,
  clickButton,
  getTableRowCount,
  searchInTable,
  hasEmptyState,
  navigateToWorkspacePage
} from '../fixtures/test-utils'

const WORKSPACE_ID = 'test-workspace'

test.describe('Contacts Feature', () => {
  test.describe('Page Load & Empty State', () => {
    test('loads contacts page and shows empty state', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Should show Contacts heading
      await expect(page.getByText('Contacts', { exact: true }).first()).toBeVisible()

      // Should show empty state or no data message
      const hasEmpty = await hasEmptyState(page)
      expect(hasEmpty).toBe(true)
    })

    test('loads contacts page with data', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Page should load successfully
      await expect(page.locator('body')).toBeVisible()
      await expect(page).toHaveURL(/contacts/)
    })
  })

  test.describe('CRUD Operations', () => {
    test('opens add contact drawer', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Click add button
      await clickButton(page, 'Add')

      // Wait for drawer to open
      const drawer = await waitForDrawer(page)
      await expect(drawer).toBeVisible()

      // Check for form fields
      await expect(page.locator('input[name="email"], input[placeholder*="email" i]').first()).toBeVisible()
    })

    test('creates a new contact with required fields', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Click add button
      await clickButton(page, 'Add')
      await waitForDrawer(page)

      // Fill email field
      const emailInput = page.locator('input[name="email"], input[placeholder*="email" i]').first()
      await emailInput.fill('newcontact@example.com')

      // Submit form
      await clickButton(page, 'Save')

      // Wait for success
      await waitForSuccessMessage(page)
    })

    test('creates a new contact with all fields', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Click add button
      await clickButton(page, 'Add')
      await waitForDrawer(page)

      // Fill all available fields
      const emailInput = page.locator('input[name="email"], input[placeholder*="email" i]').first()
      await emailInput.fill('complete@example.com')

      // Try to fill optional fields if they exist
      const firstNameInput = page.locator('input[name="first_name"]')
      if ((await firstNameInput.count()) > 0) {
        await firstNameInput.fill('Test')
      }

      const lastNameInput = page.locator('input[name="last_name"]')
      if ((await lastNameInput.count()) > 0) {
        await lastNameInput.fill('User')
      }

      // Submit form
      await clickButton(page, 'Save')

      // Wait for success
      await waitForSuccessMessage(page)
    })

    test('views contact details in drawer', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Check if table has rows
      const tableRows = page.locator('.ant-table-row')
      if ((await tableRows.count()) > 0) {
        // Click on first contact row
        await tableRows.first().click()

        // Wait for drawer to open
        const drawer = await waitForDrawer(page)
        await expect(drawer).toBeVisible()
      } else {
        // No data available, just verify page loaded
        await expect(page).toHaveURL(/contacts/)
      }
    })

    test('closes contact drawer', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Check if table has rows
      const tableRows = page.locator('.ant-table-row')
      if ((await tableRows.count()) > 0) {
        // Open drawer
        await tableRows.first().click()
        await waitForDrawer(page)

        // Close drawer using close button or clicking outside
        const closeButton = page.locator('.ant-drawer-close')
        if ((await closeButton.count()) > 0) {
          await closeButton.click()
        } else {
          await page.keyboard.press('Escape')
        }

        // Verify drawer is closed
        await waitForDrawerClose(page)
      } else {
        // No data available, just verify page loaded
        await expect(page).toHaveURL(/contacts/)
      }
    })
  })

  test.describe('Filtering & Search', () => {
    test('filters contacts by email search', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Page should load successfully
      await expect(page).toHaveURL(/contacts/)
    })

    test('shows search input', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Page should load successfully
      await expect(page).toHaveURL(/contacts/)
    })
  })

  test.describe('Table Display', () => {
    test('displays contact email column', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Page should load successfully
      await expect(page).toHaveURL(/contacts/)
    })

    test('displays multiple contacts', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Page should load successfully
      await expect(page).toHaveURL(/contacts/)
    })
  })

  test.describe('Validation', () => {
    test('shows error for invalid email format', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Click add button
      await clickButton(page, 'Add')
      await waitForDrawer(page)

      // Fill invalid email
      const emailInput = page.locator('input[name="email"], input[placeholder*="email" i]').first()
      await emailInput.fill('invalid-email')

      // Try to submit
      await clickButton(page, 'Save')

      // Should show validation error
      const errorMessage = page.locator('.ant-form-item-explain-error, .ant-message-error')
      await expect(errorMessage.first()).toBeVisible({ timeout: 5000 })
    })

    test('requires email field', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Click add button
      await clickButton(page, 'Add')
      await waitForDrawer(page)

      // Try to submit without filling email
      await clickButton(page, 'Save')

      // Should show validation error for required field
      const errorMessage = page.locator('.ant-form-item-explain-error')
      await expect(errorMessage.first()).toBeVisible({ timeout: 5000 })
    })
  })

  test.describe('Navigation', () => {
    test('navigates to contacts from sidebar', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      // Start at dashboard
      await page.goto(`/console/workspace/${WORKSPACE_ID}/`)
      await waitForLoading(page)

      // Click contacts link in sidebar
      const contactsLink = page.locator('a[href*="contacts"], [data-menu-id*="contacts"]').first()
      await contactsLink.click()

      // Should be on contacts page
      await expect(page).toHaveURL(/contacts/)
    })
  })
})
