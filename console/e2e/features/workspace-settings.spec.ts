import { test, expect, requestCapture } from '../fixtures/auth'
import { waitForLoading } from '../fixtures/test-utils'
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

      // The fixture signs in as a workspace owner, so the button is never optional here.
      await expect(page.getByRole('button', { name: 'Invite Member', exact: true })).toBeVisible()
    })

    test('opens invite member drawer', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'Invite Member', exact: true }).click()

      // A drawer panel is a dialog too, labelled by its title, so this pins both the
      // host being open and the title it carries.
      const drawer = page.getByRole('dialog', { name: 'Invite Member', exact: true })
      await expect(drawer).toBeVisible()
      // The permissions matrix outgrew a centred dialog, so the host is a drawer.
      await expect(drawer).toHaveClass(/ant-drawer-section/)

      // Should have email input
      await expect(drawer.locator('input[placeholder*="email" i]')).toBeVisible()
    })

    test('opens create API key drawer', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'Create API Key', exact: true }).click()

      const drawer = page.getByRole('dialog', { name: 'Create API Key', exact: true })
      await expect(drawer).toBeVisible()
      await expect(drawer).toHaveClass(/ant-drawer-section/)

      // Should have the API key name input (the only textbox in the creation form)
      await expect(drawer.getByRole('textbox')).toBeVisible()
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

      // Genuine role fork: owners get the editable form, everyone else the
      // read-only descriptions. Both branches assert, so neither passes silently.
      const contentArea = page.locator('.ant-layout-content')
      const nameInput = contentArea.locator('input[placeholder*="workspace name" i]')
      if ((await nameInput.count()) > 0) {
        // Fill workspace name
        await nameInput.clear()
        await nameInput.fill('Updated Workspace Name')
        await expect(nameInput).toHaveValue('Updated Workspace Name')

        // Fill website URL - always part of the owner form
        const websiteInput = contentArea.getByRole('textbox', { name: 'Website URL' })
        await websiteInput.fill('https://example.com')
        await expect(websiteInput).toHaveValue('https://example.com')

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

      // The fixture signs in as a workspace owner, so the button is never optional here.
      await expect(page.getByRole('button', { name: 'Add Label', exact: true })).toBeVisible()
    })

    test('opens add custom field label modal', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/custom-fields`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'Add Label', exact: true }).click()

      const modal = page.getByRole('dialog', { name: 'Add Custom Field Label', exact: true })
      await expect(modal).toBeVisible()

      // Should have field selection radio group
      await expect(modal.locator('.ant-radio-group')).toBeVisible()

      // Should have label input
      await expect(modal.locator('input[placeholder*="Company Name" i]')).toBeVisible()
    })

    test('fills custom field label form', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/custom-fields`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'Add Label', exact: true }).click()

      const modal = page.getByRole('dialog', { name: 'Add Custom Field Label', exact: true })
      await expect(modal).toBeVisible()

      // Select a custom field (first available radio)
      const firstRadio = modal.locator('.ant-radio-input:not(:disabled)').first()
      await firstRadio.check()
      await expect(firstRadio).toBeChecked()

      // Fill label
      const labelInput = modal.locator('input[placeholder*="Company Name" i]')
      await labelInput.fill('Industry Type')
      await expect(labelInput).toHaveValue('Industry Type')

      // Verify Save button is visible
      await expect(modal.getByRole('button', { name: 'Save', exact: true })).toBeVisible()
    })
  })

  test.describe('SMTP Bridge Settings', () => {
    test('loads SMTP bridge settings page', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/smtp-bridge`)
      await waitForLoading(page)

      // Page should load
      await expect(page.locator('body')).toBeVisible()
      await expect(page).toHaveURL(/settings\/smtp-bridge/)
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

      // The fixture signs in as a workspace owner, so the danger zone is always rendered.
      await expect(page.locator('.ant-layout-content').getByText('Danger Zone').first()).toBeVisible()
      await expect(page.getByRole('button', { name: 'Delete Workspace', exact: true })).toBeVisible()
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
        settingsSidebar.locator('.ant-menu-item').filter({ hasText: 'SMTP Bridge' })
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

      // Fill workspace name - always editable in the owner form
      const nameInput = page.getByLabel('Workspace Name', { exact: false })
      await nameInput.fill(testWorkspaceSettingsData.name)

      // Select a timezone. The option list is built from window.TIMEZONES, which the
      // mocked /config.js does not serve, so the dropdown is legitimately empty here -
      // the fallback closes it again and leaves the field at its stored value.
      const timezoneSelect = page.locator('.ant-form-item').filter({ hasText: /timezone/i }).locator('.ant-select')
      await timezoneSelect.click()
      await page.locator('.ant-select-dropdown').waitFor({ state: 'visible' })
      const option = page.locator('.ant-select-item-option').filter({ hasText: /New_York|UTC/i }).first()
      if ((await option.count()) > 0) {
        await option.click()
      } else {
        await page.keyboard.press('Escape')
      }

      // Fill custom endpoint URL
      const endpointInput = page.getByLabel('Custom Endpoint', { exact: false })
      await endpointInput.fill(testWorkspaceSettingsData.custom_endpoint_url!)

      // Submit form - the save bar shows up as soon as the form is dirty
      await page.getByRole('button', { name: /save|update/i }).first().click()

      // Log captured requests
      await expect
        .poll(() => requestCapture.getRequestCount(API_PATTERNS.WORKSPACE_UPDATE))
        .toBeGreaterThan(0)
      logCapturedRequests(requestCapture)

      // Verify workspace update carried the values that were typed in
      const request = requestCapture.getLastRequest(API_PATTERNS.WORKSPACE_UPDATE)
      expect(request?.body, 'Workspace update body should not be empty').toBeTruthy()
      const body = request!.body as { name?: string; settings?: Record<string, unknown> }
      expect(body.name).toBe(testWorkspaceSettingsData.name)
      expect(body.settings?.custom_endpoint_url).toBe(testWorkspaceSettingsData.custom_endpoint_url)
    })
  })
})
