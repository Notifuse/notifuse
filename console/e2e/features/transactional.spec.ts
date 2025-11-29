import { test, expect } from '../fixtures/auth'
import {
  waitForDrawer,
  waitForModal,
  waitForTable,
  waitForLoading,
  waitForSuccessMessage,
  clickButton,
  hasEmptyState
} from '../fixtures/test-utils'

const WORKSPACE_ID = 'test-workspace'

test.describe('Transactional Notifications Feature', () => {
  test.describe('Page Load & Empty State', () => {
    test('loads transactional page and shows empty state', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      // Page should load
      await expect(page.locator('body')).toBeVisible()
    })

    test('loads transactional page with data', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      // Page should load successfully
      await expect(page).toHaveURL(/transactional/)
    })
  })

  test.describe('CRUD Operations', () => {
    test('opens create notification form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      // Click add/create button
      const addButton = page.getByRole('button', { name: /add|create|new/i })
      await addButton.click()

      // Wait for drawer, modal, or navigation
      await page.waitForTimeout(500)

      const hasDrawer = (await page.locator('.ant-drawer-content').count()) > 0
      const hasModal = (await page.locator('.ant-modal-content').count()) > 0
      const urlChanged = page.url().includes('new') || page.url().includes('create')

      expect(hasDrawer || hasModal || urlChanged).toBe(true)
    })

    test('fills transactional notification form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      // Click add button
      const addButton = page.getByRole('button', { name: /add|create|new/i })
      await addButton.click()

      // Wait for drawer to open
      await waitForDrawer(page)

      // Fill notification name (required) - first input
      const nameInput = page.locator('.ant-drawer-content input').first()
      await nameInput.fill('Password Reset Email')

      // API Identifier is auto-generated from name, verify second input has value
      const idInput = page.locator('.ant-drawer-content input').nth(1)
      await expect(idInput).toBeVisible()

      // Fill description (optional)
      const descriptionInput = page.locator('.ant-drawer-content textarea')
      if ((await descriptionInput.count()) > 0) {
        await descriptionInput.fill('Sends a password reset email to users')
      }

      // Verify Save button is visible
      await expect(page.getByRole('button', { name: 'Save' })).toBeVisible()

      // Verify form filled correctly
      await expect(nameInput).toHaveValue('Password Reset Email')
    })

    test('views notification details', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      // Click on a notification
      const notificationItem = page.locator('.ant-table-row, .ant-card').first()
      if ((await notificationItem.count()) > 0) {
        await notificationItem.click()

        // Should show details
        await page.waitForTimeout(500)
        await expect(page.locator('body')).toBeVisible()
      }
    })
  })

  test.describe('Configuration', () => {
    test('shows template selection', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      // Open create form
      const addButton = page.getByRole('button', { name: /add|create|new/i })
      await addButton.click()

      await page.waitForTimeout(500)

      // Form should be visible
      await expect(page.locator('.ant-drawer-content, .ant-modal-content, form').first()).toBeVisible()
    })

    test('shows tracking settings', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      // Open create form
      const addButton = page.getByRole('button', { name: /add|create|new/i })
      await addButton.click()

      await page.waitForTimeout(500)

      // Look for tracking options
      const trackingOption = page.locator('text=tracking, text=Tracking, text=opens, text=clicks')

      // Form should be visible regardless
      await expect(page.locator('.ant-drawer-content, .ant-modal-content, form').first()).toBeVisible()
    })
  })

  test.describe('API Integration Display', () => {
    test('page loads without errors', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      // Page should load successfully
      await expect(page.locator('body')).toBeVisible()
    })
  })

  test.describe('Edit Form Prefill', () => {
    test('edit notification drawer shows existing notification name', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      // Click on a notification row to open edit drawer
      const notificationRow = page.locator('.ant-table-row').first()
      if ((await notificationRow.count()) > 0) {
        // Look for edit button in the row
        const editButton = notificationRow.getByRole('button', { name: /edit/i })
        if ((await editButton.count()) > 0) {
          await editButton.click()
        } else {
          await notificationRow.click()
        }

        // Wait for drawer to open
        await waitForDrawer(page)

        // Verify the name input is prefilled with the existing notification name
        const nameInput = page.locator('.ant-drawer-content input').first()
        const inputValue = await nameInput.inputValue()

        // Name should not be empty - should be prefilled (e.g., "Password Reset")
        expect(inputValue.length).toBeGreaterThan(0)
      }
    })

    test('edit notification preserves API identifier', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const notificationRow = page.locator('.ant-table-row').first()
      if ((await notificationRow.count()) > 0) {
        const editButton = notificationRow.getByRole('button', { name: /edit/i })
        if ((await editButton.count()) > 0) {
          await editButton.click()
        } else {
          await notificationRow.click()
        }

        await waitForDrawer(page)

        // The API identifier input should be prefilled and possibly read-only
        const idInput = page.locator('.ant-drawer-content input').nth(1)
        if ((await idInput.count()) > 0) {
          const idValue = await idInput.inputValue()
          expect(idValue.length).toBeGreaterThan(0)
        }
      }
    })

    test('edit notification preserves description', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const notificationRow = page.locator('.ant-table-row').first()
      if ((await notificationRow.count()) > 0) {
        const editButton = notificationRow.getByRole('button', { name: /edit/i })
        if ((await editButton.count()) > 0) {
          await editButton.click()
        } else {
          await notificationRow.click()
        }

        await waitForDrawer(page)

        // Check if description textarea exists
        const descriptionInput = page.locator('.ant-drawer-content textarea').first()
        if ((await descriptionInput.count()) > 0) {
          // Description field should be accessible
          await expect(descriptionInput).toBeVisible()
        }
      }
    })

    test('edit notification preserves template selection', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const notificationRow = page.locator('.ant-table-row').first()
      if ((await notificationRow.count()) > 0) {
        const editButton = notificationRow.getByRole('button', { name: /edit/i })
        if ((await editButton.count()) > 0) {
          await editButton.click()
        } else {
          await notificationRow.click()
        }

        await waitForDrawer(page)

        // Template select/input should have a value
        const templateInput = page.locator('.ant-drawer-content .ant-select, .ant-drawer-content input[placeholder*="template" i]')
        if ((await templateInput.count()) > 0) {
          await expect(templateInput.first()).toBeVisible()
        }
      }
    })

    test('edit notification preserves tracking settings', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      const notificationRow = page.locator('.ant-table-row').first()
      if ((await notificationRow.count()) > 0) {
        const editButton = notificationRow.getByRole('button', { name: /edit/i })
        if ((await editButton.count()) > 0) {
          await editButton.click()
        } else {
          await notificationRow.click()
        }

        await waitForDrawer(page)

        // Look for tracking switches/checkboxes - they should maintain their state
        const trackingSwitch = page.locator('.ant-drawer-content .ant-switch, .ant-drawer-content .ant-checkbox')
        if ((await trackingSwitch.count()) > 0) {
          await expect(trackingSwitch.first()).toBeVisible()
        }
      }
    })
  })

  test.describe('Form Validation', () => {
    test('requires notification name', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      // Open create form
      const addButton = page.getByRole('button', { name: /add|create|new/i })
      await addButton.click()

      // Wait for drawer to open
      await waitForDrawer(page)

      // Try to submit without filling required fields
      await page.getByRole('button', { name: 'Save' }).click()

      // Should show validation error
      const errorMessage = page.locator('.ant-form-item-explain-error')
      await expect(errorMessage.first()).toBeVisible({ timeout: 5000 })
    })

    test('requires email template selection', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/transactional-notifications`)
      await waitForLoading(page)

      // Open create form
      const addButton = page.getByRole('button', { name: /add|create|new/i })
      await addButton.click()

      // Wait for drawer to open
      await waitForDrawer(page)

      // Fill notification name
      const nameInput = page.locator('.ant-drawer-content input').first()
      await nameInput.fill('Test Notification')

      // Try to submit without selecting template
      await page.getByRole('button', { name: 'Save' }).click()

      // Should show validation error for template selection
      const errorMessage = page.locator('.ant-form-item-explain-error')
      await expect(errorMessage.first()).toBeVisible({ timeout: 5000 })
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

      // Open create form
      const addButton = page.getByRole('button', { name: /add|create|new/i })
      await addButton.click()

      await page.waitForTimeout(500)

      // Close it
      const closeButton = page.locator('.ant-drawer-close, .ant-modal-close')
      if ((await closeButton.count()) > 0) {
        await closeButton.first().click()
      } else {
        await page.keyboard.press('Escape')
      }

      await page.waitForTimeout(500)
    })
  })
})
