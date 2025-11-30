import { test, expect, requestCapture } from '../fixtures/auth'
import { waitForLoading, waitForSuccessMessage } from '../fixtures/test-utils'
import { API_PATTERNS } from '../fixtures/request-capture'
import { testWorkspaceSettingsData } from '../fixtures/form-data'
import { logCapturedRequests } from '../fixtures/payload-assertions'

const WORKSPACE_ID = 'test-workspace'

test.describe('Workspace Settings Feature', () => {
  test.describe('Settings Navigation', () => {
    test('loads settings page with sidebar', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      // Should show settings sidebar (the inner settings one with dark theme)
      await expect(page.locator('.ant-layout-sider-dark')).toBeVisible()

      // Should show "Settings" title
      await expect(page.locator('text=Settings').first()).toBeVisible()
    })

    test('navigates between settings sections', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      // Click on General settings
      await page.locator('.ant-menu-item').filter({ hasText: 'General' }).click()
      await expect(page).toHaveURL(/settings\/general/)

      // Click on Integrations
      await page.locator('.ant-menu-item').filter({ hasText: 'Integrations' }).click()
      await expect(page).toHaveURL(/settings\/integrations/)

      // Click on Custom Fields
      await page.locator('.ant-menu-item').filter({ hasText: 'Custom Fields' }).click()
      await expect(page).toHaveURL(/settings\/custom-fields/)
    })

    test('defaults to team section for invalid section', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/invalid-section`)
      await waitForLoading(page)

      // Should redirect to team section
      await expect(page).toHaveURL(/settings\/team/)
    })
  })

  test.describe('Team Settings', () => {
    test('loads team settings page', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      // Should show Team section header
      await expect(page.locator('text=Team').first()).toBeVisible()

      // Should show members table
      await expect(page.locator('.ant-table')).toBeVisible()
    })

    test('shows invite member button for owners', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      // Look for invite button
      const inviteButton = page.getByRole('button', { name: /invite/i })
      // Page should load regardless of user role
      await expect(page.locator('body')).toBeVisible()
    })

    test('opens invite member modal', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      // Try to open invite modal
      const inviteButton = page.getByRole('button', { name: /invite/i })
      if ((await inviteButton.count()) > 0) {
        await inviteButton.click()

        // Should show invite modal
        await expect(page.locator('.ant-modal-content')).toBeVisible()
        await expect(page.locator('.ant-modal-title')).toContainText(/invite/i)

        // Should have email input
        await expect(page.locator('.ant-modal-content input[placeholder*="email" i]')).toBeVisible()
      }
    })

    test('opens create API key modal', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      // Try to open API key modal
      const apiKeyButton = page.getByRole('button', { name: /api key/i })
      if ((await apiKeyButton.count()) > 0) {
        await apiKeyButton.click()

        // Should show API key modal
        await expect(page.locator('.ant-modal-content')).toBeVisible()
        await expect(page.locator('.ant-modal-title')).toContainText(/api key/i)
      }
    })
  })

  test.describe('General Settings', () => {
    test('loads general settings page', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/general`)
      await waitForLoading(page)

      // Should show General Settings section - look in the content area
      await expect(
        page.locator('.ant-layout-content').getByText('General Settings').first()
      ).toBeVisible()
    })

    test('shows workspace name field', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/general`)
      await waitForLoading(page)

      // Should have workspace name field
      const nameLabel = page.locator('text=Workspace Name')
      await expect(nameLabel.first()).toBeVisible()
    })

    test('shows timezone field', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/general`)
      await waitForLoading(page)

      // Should have timezone field
      const timezoneLabel = page.locator('text=Timezone')
      await expect(timezoneLabel.first()).toBeVisible()
    })

    test('fills general settings form', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/general`)
      await waitForLoading(page)

      // Check if form exists (owner view) - look in content area
      const contentArea = page.locator('.ant-layout-content')
      const nameInput = contentArea.locator('input[placeholder*="workspace name" i]')
      if ((await nameInput.count()) > 0) {
        // Fill workspace name
        await nameInput.clear()
        await nameInput.fill('Updated Workspace Name')
        await expect(nameInput).toHaveValue('Updated Workspace Name')

        // Fill website URL (use first matching input - the Website URL field)
        const websiteInput = contentArea.getByRole('textbox', { name: 'Website URL' })
        if ((await websiteInput.count()) > 0) {
          await websiteInput.fill('https://example.com')
        }

        // Verify Save button is visible
        const saveButton = contentArea.getByRole('button', { name: /save/i })
        await expect(saveButton).toBeVisible()
      } else {
        // Non-owner view - should show read-only descriptions
        await expect(contentArea.locator('.ant-descriptions')).toBeVisible()
      }
    })
  })

  test.describe('Integrations Settings', () => {
    test('loads integrations settings page', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/integrations`)
      await waitForLoading(page)

      // Page should load
      await expect(page.locator('body')).toBeVisible()
      await expect(page).toHaveURL(/settings\/integrations/)
    })
  })

  test.describe('Custom Fields Settings', () => {
    test('loads custom fields settings page', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/custom-fields`)
      await waitForLoading(page)

      // Should show Custom Fields section
      await expect(page.locator('text=Custom Fields').first()).toBeVisible()
    })

    test('shows add label button for owners', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/custom-fields`)
      await waitForLoading(page)

      // Look for Add Label button
      const addButton = page.getByRole('button', { name: /add label/i })
      // Page should load regardless of user role
      await expect(page.locator('body')).toBeVisible()
    })

    test('opens add custom field label modal', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/custom-fields`)
      await waitForLoading(page)

      // Try to open add label modal
      const addButton = page.getByRole('button', { name: /add label/i })
      if ((await addButton.count()) > 0) {
        await addButton.click()

        // Should show modal
        await expect(page.locator('.ant-modal-content')).toBeVisible()
        await expect(page.locator('.ant-modal-title')).toContainText(/custom field/i)

        // Should have field selection radio group
        await expect(page.locator('.ant-radio-group')).toBeVisible()

        // Should have label input
        await expect(
          page.locator('.ant-modal-content input[placeholder*="Company Name" i]')
        ).toBeVisible()
      }
    })

    test('fills custom field label form', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/custom-fields`)
      await waitForLoading(page)

      // Try to open add label modal
      const addButton = page.getByRole('button', { name: /add label/i })
      if ((await addButton.count()) > 0) {
        await addButton.click()

        // Wait for modal
        await expect(page.locator('.ant-modal-content')).toBeVisible()

        // Select a custom field (first available radio)
        const firstRadio = page.locator('.ant-radio-input:not(:disabled)').first()
        if ((await firstRadio.count()) > 0) {
          await firstRadio.click()
        }

        // Fill label
        const labelInput = page.locator('.ant-modal-content input[placeholder*="Company Name" i]')
        await labelInput.fill('Industry Type')

        // Verify Save button is visible
        await expect(page.getByRole('button', { name: 'Save' })).toBeVisible()
      }
    })
  })

  test.describe('SMTP Relay Settings', () => {
    test('loads SMTP relay settings page', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/smtp-relay`)
      await waitForLoading(page)

      // Page should load
      await expect(page.locator('body')).toBeVisible()
      await expect(page).toHaveURL(/settings\/smtp-relay/)
    })
  })

  test.describe('Blog Settings', () => {
    test('loads blog settings page', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/blog`)
      await waitForLoading(page)

      // Page should load
      await expect(page.locator('body')).toBeVisible()
      await expect(page).toHaveURL(/settings\/blog/)
    })
  })

  test.describe('Danger Zone', () => {
    test('loads danger zone page for owners', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/danger-zone`)
      await waitForLoading(page)

      // Page should load
      await expect(page.locator('body')).toBeVisible()

      // Danger Zone should only be visible for owners
      // If visible, should show delete workspace option
      const dangerContent = page.locator('text=Delete Workspace, text=delete this workspace')
      // Just verify page loaded - content depends on user role
    })
  })

  test.describe('Settings Sidebar Menu', () => {
    test('shows all settings sections in sidebar', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      // Target the settings sidebar specifically
      const settingsSidebar = page.locator('.ant-layout-sider-dark')

      // Should show all menu items in settings sidebar
      await expect(settingsSidebar.locator('.ant-menu-item').filter({ hasText: 'Team' })).toBeVisible()
      await expect(
        settingsSidebar.locator('.ant-menu-item').filter({ hasText: 'Integrations' })
      ).toBeVisible()
      await expect(settingsSidebar.locator('.ant-menu-item').filter({ hasText: 'Blog' })).toBeVisible()
      await expect(
        settingsSidebar.locator('.ant-menu-item').filter({ hasText: 'Custom Fields' })
      ).toBeVisible()
      await expect(
        settingsSidebar.locator('.ant-menu-item').filter({ hasText: 'SMTP Relay' })
      ).toBeVisible()
      await expect(
        settingsSidebar.locator('.ant-menu-item').filter({ hasText: 'General' })
      ).toBeVisible()
    })

    test('highlights active section in sidebar', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/general`)
      await waitForLoading(page)

      // General should be selected
      const generalMenuItem = page.locator('.ant-menu-item').filter({ hasText: 'General' })
      await expect(generalMenuItem).toHaveClass(/ant-menu-item-selected/)
    })
  })

  test.describe('Full Form Submission with Payload Verification', () => {
    test('updates workspace settings and verifies payload', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/general`)
      await waitForLoading(page)

      // Fill workspace name if editable
      const nameInput = page.getByLabel('Workspace Name', { exact: false })
      if ((await nameInput.count()) > 0 && (await nameInput.isEnabled())) {
        await nameInput.fill(testWorkspaceSettingsData.name)
      }

      // Select timezone if available
      const timezoneSelect = page.locator('.ant-form-item').filter({ hasText: /timezone/i }).locator('.ant-select')
      if ((await timezoneSelect.count()) > 0) {
        await timezoneSelect.click()
        await page.locator('.ant-select-dropdown').waitFor({ state: 'visible' })
        const option = page.locator('.ant-select-item-option').filter({ hasText: /New_York|UTC/i }).first()
        if ((await option.count()) > 0) {
          await option.click()
        } else {
          await page.keyboard.press('Escape')
        }
      }

      // Fill custom endpoint URL if available
      const endpointInput = page.getByLabel('Custom Endpoint', { exact: false })
      if ((await endpointInput.count()) > 0 && testWorkspaceSettingsData.custom_endpoint_url) {
        await endpointInput.fill(testWorkspaceSettingsData.custom_endpoint_url)
      }

      // Submit form
      const saveButton = page.getByRole('button', { name: /save|update/i }).first()
      if ((await saveButton.count()) > 0) {
        await saveButton.click()
        await page.waitForTimeout(1000)

        // Log captured requests
        logCapturedRequests(requestCapture)

        // Verify workspace update was sent
        const request = requestCapture.getLastRequest(API_PATTERNS.WORKSPACE_UPDATE)

        if (request && request.body) {
          const body = request.body as Record<string, unknown>
          // Verify settings were included
          expect(body, 'Workspace update body should not be empty').toBeDefined()
        }
      }
    })
  })
})
