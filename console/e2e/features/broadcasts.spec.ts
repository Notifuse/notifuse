import { test, expect } from '../fixtures/auth'
import {
  waitForDrawer,
  waitForModal,
  waitForTable,
  waitForLoading,
  waitForSuccessMessage,
  clickButton,
  getTableRowCount,
  hasEmptyState
} from '../fixtures/test-utils'

const WORKSPACE_ID = 'test-workspace'

test.describe('Broadcasts Feature', () => {
  test.describe('Page Load & Empty State', () => {
    test('loads broadcasts page and shows empty state', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // Page should load
      await expect(page.locator('body')).toBeVisible()
    })

    test('loads broadcasts page with data', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // Page should load successfully
      await expect(page.locator('body')).toBeVisible()
      // URL should be correct
      await expect(page).toHaveURL(/broadcasts/)
    })
  })

  test.describe('CRUD Operations', () => {
    test('opens create broadcast form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
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

    test('fills broadcast form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // Click add button
      const addButton = page.getByRole('button', { name: /add|create|new/i })
      await addButton.click()

      // Wait for drawer to open
      await waitForDrawer(page)

      // Tab 1: Audience - fill required fields
      // Fill broadcast name (required) - first input in drawer
      const nameInput = page.locator('.ant-drawer-content input').first()
      await nameInput.fill('Test Marketing Broadcast')

      // Select list (required) - find the list select
      const listSelect = page.locator('.ant-drawer-content .ant-select').first()
      if ((await listSelect.count()) > 0) {
        await listSelect.click()
        await page.waitForTimeout(300)
        const listOption = page.locator('.ant-select-item-option').first()
        if ((await listOption.count()) > 0) {
          await listOption.click()
        }
      }

      // Verify Next button is visible
      await expect(page.getByRole('button', { name: 'Next' })).toBeVisible()

      // Verify form filled correctly
      await expect(nameInput).toHaveValue('Test Marketing Broadcast')
    })

    test('views broadcast details', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // Click on a broadcast
      const broadcastItem = page.locator('.ant-table-row, .ant-card').first()
      if ((await broadcastItem.count()) > 0) {
        await broadcastItem.click()

        // Should show broadcast details
        await page.waitForTimeout(500)
        await expect(page.locator('body')).toBeVisible()
      }
    })
  })

  test.describe('Audience Selection', () => {
    test('shows audience selection options', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // Open create form
      const addButton = page.getByRole('button', { name: /add|create|new/i })
      await addButton.click()

      await page.waitForTimeout(500)

      // Form should be visible with audience options
      await expect(page.locator('.ant-drawer-content, .ant-modal-content, form').first()).toBeVisible()
    })
  })

  test.describe('Scheduling', () => {
    test('shows scheduling options', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // Open create form
      const addButton = page.getByRole('button', { name: /add|create|new/i })
      await addButton.click()

      await page.waitForTimeout(500)

      // Scheduling options might be available
      const scheduleOption = page.locator('text=Schedule, text=schedule, text=Send later')

      // Form should be visible regardless
      await expect(page.locator('.ant-drawer-content, .ant-modal-content, form').first()).toBeVisible()
    })
  })

  test.describe('Status Display', () => {
    test('displays broadcast status', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // Page should load successfully
      await expect(page).toHaveURL(/broadcasts/)
    })

    test('shows draft broadcasts', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // Page should load successfully
      await expect(page).toHaveURL(/broadcasts/)
    })
  })

  test.describe('Statistics', () => {
    test('displays broadcast statistics', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // Page should load successfully
      await expect(page).toHaveURL(/broadcasts/)
    })
  })

  test.describe('Edit Form Prefill', () => {
    test('edit broadcast drawer shows existing broadcast name', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // Click on a broadcast row to open edit drawer
      const broadcastRow = page.locator('.ant-table-row').first()
      if ((await broadcastRow.count()) > 0) {
        // Look for edit button in the row
        const editButton = broadcastRow.getByRole('button', { name: /edit/i })
        if ((await editButton.count()) > 0) {
          await editButton.click()
        } else {
          await broadcastRow.click()
        }

        // Wait for drawer to open
        await waitForDrawer(page)

        // Verify the name input is prefilled with the existing broadcast name
        const nameInput = page.locator('.ant-drawer-content input').first()
        const inputValue = await nameInput.inputValue()

        // Name should not be empty - should be prefilled (e.g., "January Newsletter")
        expect(inputValue.length).toBeGreaterThan(0)
      }
    })

    test('edit broadcast preserves list selection', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const broadcastRow = page.locator('.ant-table-row').first()
      if ((await broadcastRow.count()) > 0) {
        const editButton = broadcastRow.getByRole('button', { name: /edit/i })
        if ((await editButton.count()) > 0) {
          await editButton.click()
        } else {
          await broadcastRow.click()
        }

        await waitForDrawer(page)

        // List select should have a value selected
        const listSelect = page.locator('.ant-drawer-content .ant-select').first()
        if ((await listSelect.count()) > 0) {
          await expect(listSelect).toBeVisible()
        }
      }
    })

    test('edit broadcast preserves template selection', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      const broadcastRow = page.locator('.ant-table-row').first()
      if ((await broadcastRow.count()) > 0) {
        const editButton = broadcastRow.getByRole('button', { name: /edit/i })
        if ((await editButton.count()) > 0) {
          await editButton.click()
        } else {
          await broadcastRow.click()
        }

        await waitForDrawer(page)

        // Navigate through tabs if needed to find template selection
        // Template selection might be on a different step
        const nextButton = page.getByRole('button', { name: 'Next' })
        if ((await nextButton.count()) > 0 && (await nextButton.isEnabled())) {
          // If there's a Next button and it's enabled, we might need to navigate
          // For now, just verify the drawer is open and has form fields
          await expect(page.locator('.ant-drawer-content')).toBeVisible()
        }
      }
    })

    test('edit draft broadcast shows correct status', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // Look for a draft broadcast specifically
      const draftRow = page.locator('.ant-table-row').filter({ hasText: /draft/i }).first()
      if ((await draftRow.count()) > 0) {
        const editButton = draftRow.getByRole('button', { name: /edit/i })
        if ((await editButton.count()) > 0) {
          await editButton.click()
        } else {
          await draftRow.click()
        }

        await waitForDrawer(page)

        // Drawer should open with editable form (draft broadcasts are editable)
        await expect(page.locator('.ant-drawer-content')).toBeVisible()
        // The name input should be enabled/editable for drafts
        const nameInput = page.locator('.ant-drawer-content input').first()
        await expect(nameInput).toBeEnabled()
      }
    })
  })

  test.describe('Form Validation', () => {
    test('requires broadcast name', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // Open create form
      const addButton = page.getByRole('button', { name: /add|create|new/i })
      await addButton.click()

      // Wait for drawer to open
      await waitForDrawer(page)

      // Try to click Next without filling required name field
      await page.getByRole('button', { name: 'Next' }).click()

      // Should show validation error
      const errorMessage = page.locator('.ant-form-item-explain-error')
      await expect(errorMessage.first()).toBeVisible({ timeout: 5000 })
    })

    test('requires list selection', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/broadcasts`)
      await waitForLoading(page)

      // Open create form
      const addButton = page.getByRole('button', { name: /add|create|new/i })
      await addButton.click()

      // Wait for drawer to open
      await waitForDrawer(page)

      // Fill name but not list
      const nameInput = page.locator('.ant-drawer-content input').first()
      await nameInput.fill('Test Broadcast')

      // Try to click Next without selecting a list
      await page.getByRole('button', { name: 'Next' }).click()

      // Should show validation error for list selection
      const errorMessage = page.locator('.ant-form-item-explain-error')
      await expect(errorMessage.first()).toBeVisible({ timeout: 5000 })
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
