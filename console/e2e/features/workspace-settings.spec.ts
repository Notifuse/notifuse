import { test, expect, requestCapture } from '../fixtures/auth'
import { waitForLoading } from '../fixtures/test-utils'
import { API_PATTERNS } from '../fixtures/request-capture'
import { testWorkspaceSettingsData } from '../fixtures/form-data'
import { logCapturedRequests } from '../fixtures/payload-assertions'

const WORKSPACE_ID = 'test-workspace'
// "Name a slot" without a named slot, "Name a free slot" under the table once one is named.
const NAME_A_SLOT = /^Name a (free )?slot$/

test.describe('Workspace Settings Feature', () => {
  test.describe('Settings Navigation', () => {
    test('loads settings page with sidebar', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      // Should show the settings rail, a navigation named after its title
      await expect(page.getByRole('navigation', { name: 'Settings' })).toBeVisible()

      // Should show "Settings" title
      await expect(page.locator('text=Settings').first()).toBeVisible()
    })

    test('navigates between settings sections', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      // Click on General settings
      await page
        .getByRole('navigation', { name: 'Settings' })
        .getByRole('button', { name: 'General', exact: true })
        .click()
      await expect(page).toHaveURL(/settings\/general/)

      // Click on Integrations
      await page
        .getByRole('navigation', { name: 'Settings' })
        .getByRole('button', { name: 'Integrations', exact: true })
        .click()
      await expect(page).toHaveURL(/settings\/integrations/)

      // Click on Custom Fields
      await page
        .getByRole('navigation', { name: 'Settings' })
        .getByRole('button', { name: 'Custom fields', exact: true })
        .click()
      await expect(page).toHaveURL(/settings\/custom-fields/)
    })

    test('defaults to general section for invalid section', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/invalid-section`)
      await waitForLoading(page)

      // Should redirect to the general section
      await expect(page).toHaveURL(/settings\/general/)
    })
  })

  test.describe('Team Settings', () => {
    test('loads team settings page', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      // Should show Team & API keys section header
      await expect(page.locator('[data-rail-content]').getByText('Team & API keys').first()).toBeVisible()

      // Should show members table
      await expect(page.locator('.ant-table')).toBeVisible()
    })

    test('shows invite member button for owners', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      // The fixture signs in as a workspace owner, so the button is never optional here.
      await expect(page.getByRole('button', { name: 'Invite', exact: true })).toBeVisible()
    })

    test('opens invite member drawer', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'Invite', exact: true }).click()

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

      await page.getByRole('button', { name: 'New key', exact: true }).click()

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

      // Should show the General header and its first panel - look in the content area
      const content = page.locator('[data-rail-content]')
      await expect(content.getByRole('heading', { name: 'General', exact: true })).toBeVisible()
      await expect(content.getByRole('heading', { name: 'Workspace', exact: true })).toBeVisible()
    })

    test('shows workspace name field', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/general`)
      await waitForLoading(page)

      // Should have workspace name field
      await expect(page.locator('[data-rail-content]').getByLabel('Name', { exact: true })).toBeVisible()
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
      const contentArea = page.locator('[data-rail-content]')
      const nameInput = contentArea.locator('input[placeholder*="workspace name" i]')
      if ((await nameInput.count()) > 0) {
        // Fill workspace name
        await nameInput.clear()
        await nameInput.fill('Updated Workspace Name')
        await expect(nameInput).toHaveValue('Updated Workspace Name')

        // Fill website URL - always part of the owner form
        const websiteInput = contentArea.getByRole('textbox', { name: 'Website' })
        await websiteInput.fill('https://example.com')
        await expect(websiteInput).toHaveValue('https://example.com')

        // Verify Save button is visible
        const saveButton = contentArea.getByRole('button', { name: /save/i })
        await expect(saveButton).toBeVisible()
      } else {
        // Non-owner view - the same rows, with the values as text
        await expect(contentArea.locator('[data-settings-row]').first()).toBeVisible()
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

      // Should show Custom fields section
      await expect(page.locator('[data-rail-content]').getByText('Custom fields').first()).toBeVisible()
    })

    test('shows the button naming a slot for owners', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/custom-fields`)
      await waitForLoading(page)

      // The fixture signs in as a workspace owner, so the button is never optional here.
      await expect(page.getByRole('button', { name: NAME_A_SLOT })).toBeEnabled()
    })

    test('opens add custom field label modal', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/custom-fields`)
      await waitForLoading(page)

      await page.getByRole('button', { name: NAME_A_SLOT }).click()

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

      await page.getByRole('button', { name: NAME_A_SLOT }).click()

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
      await expect(page.locator('[data-rail-content]').getByText('Danger zone').first()).toBeVisible()
      await expect(page.getByRole('button', { name: 'Delete Workspace', exact: true })).toBeVisible()
    })
  })

  test.describe('Settings Sidebar Menu', () => {
    test('shows all settings sections in sidebar', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/team`)
      await waitForLoading(page)

      // Target the settings sidebar specifically
      const settingsSidebar = page.getByRole('navigation', { name: 'Settings' })

      // Should show all menu items in settings sidebar
      await expect(
        settingsSidebar.getByRole('button', { name: 'Team & API keys', exact: true })
      ).toBeVisible()
      await expect(
        settingsSidebar.getByRole('button', { name: 'Integrations', exact: true })
      ).toBeVisible()
      await expect(settingsSidebar.getByRole('button', { name: 'Blog', exact: true })).toBeVisible()
      await expect(
        settingsSidebar.getByRole('button', { name: 'Custom fields', exact: true })
      ).toBeVisible()
      await expect(
        settingsSidebar.getByRole('button', { name: 'SMTP bridge', exact: true })
      ).toBeVisible()
      await expect(
        settingsSidebar.getByRole('button', { name: 'General', exact: true })
      ).toBeVisible()
    })

    test('highlights active section in sidebar', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/general`)
      await waitForLoading(page)

      // General should be the current page
      const generalMenuItem = page
        .getByRole('navigation', { name: 'Settings' })
        .getByRole('button', { name: 'General', exact: true })
      await expect(generalMenuItem).toHaveAttribute('aria-current', 'page')
    })
  })

  test.describe('Full Form Submission with Payload Verification', () => {
    test('updates workspace settings and verifies payload', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/general`)
      await waitForLoading(page)

      // Fill workspace name - always editable in the owner form
      const nameInput = page.locator('[data-rail-content]').getByLabel('Name', { exact: true })
      await nameInput.fill(testWorkspaceSettingsData.name)

      // Select a timezone. The option list is built from window.TIMEZONES, which the
      // mocked /config.js does not serve, so the dropdown is legitimately empty here -
      // the fallback closes it again and leaves the field at its stored value.
      const timezoneSelect = page.locator('[data-settings-row]').filter({ hasText: /timezone/i }).locator('.ant-select')
      await timezoneSelect.click()
      await page.locator('.ant-select-dropdown').waitFor({ state: 'visible' })
      const option = page.locator('.ant-select-item-option').filter({ hasText: /New_York|UTC/i }).first()
      if ((await option.count()) > 0) {
        await option.click()
      } else {
        await page.keyboard.press('Escape')
      }

      // Fill custom endpoint URL
      const endpointInput = page.locator('[data-rail-content]').getByLabel('Domain', { exact: true })
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
