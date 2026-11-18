import type { Locator, Page } from '@playwright/test'
import { test, expect, requestCapture } from '../fixtures/auth'
import { waitForLoading, waitForSuccessMessage } from '../fixtures/test-utils'
import { API_PATTERNS } from '../fixtures/request-capture'
import { testListData } from '../fixtures/form-data'
import { logCapturedRequests } from '../fixtures/payload-assertions'
import { mockTemplates } from '../fixtures/mock-data'

const WORKSPACE_ID = 'test-workspace'

// antd 6 renamed `.ant-drawer-content` out of existence; the drawer's `role="dialog"`
// node now carries the title as its accessible name, so every drawer is addressed by
// role + name instead of by class.
const createListDrawer = (page: Page) => page.getByRole('dialog', { name: 'New list', exact: true })

const editListDrawer = (page: Page) => page.getByRole('dialog', { name: 'Edit list', exact: true })

/**
 * The window lists the Opt-in templates alone, and the mock templates have none: this one answers
 * templates.list with Welcome Email as an Opt-in template.
 */
async function withOptinTemplate(page: Page) {
  await page.route('**/api/templates.list*', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ templates: mockTemplates.map((tpl) => ({ ...tpl, category: 'opt_in' })) })
    })
  )
}

/** The confirmation template window, opened from the card's « Choose », and a template picked in it. */
async function pickTemplate(page: Page, drawer: Locator, name: string) {
  await drawer.getByRole('button', { name: 'Choose', exact: true }).click()
  const picker = page.getByRole('dialog', { name: /Choose a template/ })
  await expect(picker).toBeVisible()
  await picker.getByRole('button', { name: new RegExp(`^${name}`) }).dblclick()
  await expect(picker).toBeHidden()
}

const CONTACTS_URL = `/console/workspace/${WORKSPACE_ID}/contacts`

/** The rail row of a list, found by its name (the row also carries its active count). */
const railList = (page: Page, listName: string) =>
  page.locator('[data-rail-item]').filter({ hasText: listName })

/**
 * Open the create drawer from the Contacts rail: « New list » beside the Lists title, or
 * « Create your first list » in an empty rail.
 */
async function openCreateDrawer(page: Page): Promise<Locator> {
  const newList = page.getByRole('button', { name: 'New list', exact: true })
  const firstList = page.getByRole('button', { name: 'Create your first list', exact: true })
  await newList.or(firstList).first().click()
  const drawer = createListDrawer(page)
  await expect(drawer).toBeVisible()
  return drawer
}

/** Open a list's view from the rail, then its edit drawer from the settings bar. */
async function openEditDrawer(page: Page, listName = 'Newsletter'): Promise<Locator> {
  await railList(page, listName).click()
  await page.getByRole('button', { name: 'Edit list', exact: true }).click()
  const drawer = editListDrawer(page)
  await expect(drawer).toBeVisible()
  return drawer
}

test.describe('Lists Feature', () => {
  test.describe('Page Load & Empty State', () => {
    test('the former Lists page opens Contacts, whose rail offers a first list', async ({
      authenticatedPage
    }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/lists`)
      await waitForLoading(page)

      await expect(page).toHaveURL(/\/contacts/)
      await expect(page.getByRole('button', { name: 'Create your first list', exact: true })).toBeVisible()
    })

    test('lists the lists in the rail, by name', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      for (const name of ['Beta Testers', 'Marketing Updates', 'Newsletter']) {
        await expect(railList(page, name)).toBeVisible()
      }
      await expect(page.getByRole('button', { name: 'New list', exact: true })).toBeVisible()
    })
  })

  test.describe('CRUD Operations', () => {
    test('opens create list form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await expect(drawer.getByLabel('Name', { exact: true })).toBeVisible()
      // The ID is derived from the name and shown under it until « Modify » opens it
      await expect(drawer.getByText('ID derived from the name')).toBeVisible()
      await expect(drawer.getByRole('button', { name: 'Create list', exact: true })).toBeVisible()
    })

    test('fills and submits list form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByLabel('Name', { exact: true }).fill('Test Newsletter List')

      // The ID is derived from the name: lowercased, non-alphanumerics stripped, then checked
      await expect(drawer.getByText('testnewsletterlist', { exact: true })).toBeVisible()
      await expect(drawer.getByText('available', { exact: true })).toBeVisible()

      await drawer
        .getByLabel('Description', { exact: true })
        .fill('A test newsletter list with all fields')

      await drawer.getByRole('button', { name: 'Create list', exact: true }).click()

      await waitForSuccessMessage(page)
      await expect(drawer).toBeHidden()
      // The new list opens, without the close guard stopping the navigation
      await expect(page).toHaveURL(/list_id=/)
      await expect(page.getByText('Discard your changes?')).toHaveCount(0)
    })

    test('views list details', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)
      await railList(page, 'Newsletter').click()

      await expect(page).toHaveURL(/list_id=list-1/)
      await expect(page.getByRole('heading', { name: 'Newsletter' })).toBeVisible()
      await expect(page.getByText('Monthly newsletter subscribers')).toBeVisible()
      // The settings bar: visibility, opt-in and the list's ID
      await expect(page.getByText('Public', { exact: true })).toBeVisible()
      await expect(page.getByText('Double opt-in', { exact: true })).toBeVisible()
      await expect(page.getByText('list-1', { exact: true })).toBeVisible()
    })
  })

  test.describe('List Configuration', () => {
    test('shows double opt-in setting', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      const doubleOptIn = drawer.getByRole('switch', { name: 'Double opt-in' })
      await expect(doubleOptIn).toBeVisible()
      await expect(doubleOptIn).not.toBeChecked()

      // Turning it on is what pulls in the confirmation template card
      await doubleOptIn.click()
      await expect(doubleOptIn).toBeChecked()
      await expect(drawer.getByText('Choose a confirmation template', { exact: true }).first()).toBeVisible()
    })

    // The template picker only has something to show when the workspace has
    // templates, so this case runs against the populated fixture.
    test('shows template selection options', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData
      await withOptinTemplate(page)

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)
      await drawer.getByRole('switch', { name: 'Double opt-in' }).click()

      // The card's « Choose » opens the window listing the workspace templates
      await drawer.getByRole('button', { name: 'Choose', exact: true }).click()

      const picker = page.getByRole('dialog', { name: /Choose a template/ })
      await expect(picker).toBeVisible()
      await expect(picker.getByRole('button', { name: /^Welcome Email/ })).toBeVisible()
    })
  })

  test.describe('List Statistics', () => {
    test('displays subscriber counts', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)
      await railList(page, 'Newsletter').click()

      // The list's KPI strip reads /api/lists.stats
      const cells = page.locator('[data-kpi-strip] [data-kpi-cell]')
      await expect(cells.filter({ hasText: 'Active' })).toContainText('150')
      await expect(cells.filter({ hasText: 'Pending' })).toContainText('25')
      await expect(cells.filter({ hasText: 'Unsubscribed' })).toContainText('10')
    })
  })

  test.describe('Edit Form Prefill', () => {
    test('edit list drawer shows existing list name', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      await expect(drawer.getByLabel('Name', { exact: true })).toHaveValue('Newsletter')
    })

    test('edit list preserves list ID', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      // The ID is the list's key, so editing an existing list shows it read-only, with a copy button
      await expect(drawer.getByText('list-1', { exact: true })).toBeVisible()
      await expect(drawer.getByLabel('ID', { exact: true })).toHaveCount(0)
      await expect(drawer.getByRole('button', { name: 'Copy', exact: true })).toBeVisible()
    })

    test('edit list preserves description', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      await expect(drawer.getByLabel('Description', { exact: true })).toHaveValue(
        'Monthly newsletter subscribers'
      )
    })

    test('edit list preserves double opt-in setting', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      // The first mocked list is both public and double opt-in
      await expect(drawer.getByRole('switch', { name: 'Double opt-in' })).toBeChecked()
      await expect(drawer.getByRole('switch', { name: 'Public list' })).toBeChecked()
      // Public: the list's row as the notification center shows it
      await expect(drawer.getByRole('region', { name: 'In the notification center' })).toContainText('Newsletter')
    })
  })

  test.describe('Form Validation', () => {
    test('requires list name', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByRole('button', { name: 'Create list', exact: true }).click()

      await expect(drawer.getByText('Please enter a list name')).toBeVisible()
      expect(requestCapture.getRequestCount(API_PATTERNS.LIST_CREATE)).toBe(0)
    })

    test('validates list ID format', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByLabel('Name', { exact: true }).fill('Test List')

      await drawer.getByRole('button', { name: 'Modify', exact: true }).click()
      const idInput = drawer.getByLabel('ID', { exact: true })
      await idInput.fill('invalid@id!')

      await drawer.getByRole('button', { name: 'Create list', exact: true }).click()

      await expect(drawer.getByText('ID must be alphanumeric')).toBeVisible()
      expect(requestCapture.getRequestCount(API_PATTERNS.LIST_CREATE)).toBe(0)
    })
  })

  test.describe('Navigation', () => {
    test('reaches the lists from the Contacts entry of the sidebar', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/`)
      await waitForLoading(page)

      await page.locator('a[href*="/contacts"]').first().click()

      await expect(page).toHaveURL(/contacts/)
      await expect(page.getByRole('button', { name: 'Create your first list', exact: true })).toBeVisible()
    })

    test('can close create form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByRole('button', { name: 'Close', exact: true }).click()

      await expect(drawer).toBeHidden()
    })

    test('asks before closing with unsaved changes', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)
      await drawer.getByLabel('Name', { exact: true }).fill('Half done')
      await drawer.getByRole('button', { name: 'Close', exact: true }).click()

      const confirm = page.getByRole('dialog', { name: 'Discard your changes?' })
      await confirm.getByRole('button', { name: 'Keep editing', exact: true }).click()
      await expect(drawer.getByLabel('Name', { exact: true })).toHaveValue('Half done')

      await drawer.getByRole('button', { name: 'Cancel', exact: true }).click()
      await confirm.getByRole('button', { name: 'Discard', exact: true }).click()
      await expect(drawer).toBeHidden()
    })

    // Escape in the template window closes that window only: antd closes its top layer.
    test('closes only the template window on Escape', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData
      await withOptinTemplate(page)

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)
      await drawer.getByRole('switch', { name: 'Double opt-in' }).click()
      await drawer.getByRole('button', { name: 'Choose', exact: true }).click()
      const picker = page.getByRole('dialog', { name: /Choose a template/ })
      await expect(picker).toBeVisible()

      await page.keyboard.press('Escape')
      await expect(picker).toBeHidden()
      await expect(drawer).toBeVisible()
      await expect(page.getByText('Discard your changes?')).toHaveCount(0)
    })

    test('refuses an ID another list has while typing', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      // The mock lists' ids (list-1) are not valid list IDs, so the check never asks for them:
      // here lists.get finds a list under any ID, as the server would for a taken one.
      const TAKEN_ID = 'newsletter'
      await page.route('**/api/lists.get*', (route) =>
        route.fulfill({
          contentType: 'application/json',
          body: JSON.stringify({ list: { id: TAKEN_ID, name: 'Newsletter' } })
        })
      )

      const drawer = await openCreateDrawer(page)
      await drawer.getByLabel('Name', { exact: true }).fill('Another')
      await drawer.getByRole('button', { name: 'Modify', exact: true }).click()
      await drawer.getByLabel('ID', { exact: true }).fill(TAKEN_ID)

      await expect(drawer.getByText(`A list already has the ID “${TAKEN_ID}”. Change it.`)).toBeVisible()
      await expect(drawer.getByRole('button', { name: 'Create list', exact: true })).toBeDisabled()
    })
  })

  test.describe('Full Form Submission with Payload Verification', () => {
    // Uses the populated fixture: double opt-in requires picking a real template.
    test('creates list with all fields and verifies payload', async ({
      authenticatedPageWithData
    }) => {
      const page = authenticatedPageWithData
      await withOptinTemplate(page)

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByLabel('Name', { exact: true }).fill(testListData.name)
      await drawer.getByLabel('Description', { exact: true }).fill(testListData.description || '')

      const publicSwitch = drawer.getByRole('switch', { name: 'Public list' })
      if (testListData.is_public) {
        await publicSwitch.click()
      }
      await expect(publicSwitch).toBeChecked({ checked: !!testListData.is_public })

      const doubleOptInSwitch = drawer.getByRole('switch', { name: 'Double opt-in' })
      if (testListData.is_double_optin) {
        await doubleOptInSwitch.click()
      }
      await expect(doubleOptInSwitch).toBeChecked({ checked: !!testListData.is_double_optin })

      // Double opt-in makes the confirmation template a required field
      await pickTemplate(page, drawer, 'Welcome Email')
      await expect(drawer.locator('[data-template-card]')).toContainText('Welcome Email')

      await drawer.getByRole('button', { name: 'Create list', exact: true }).click()

      await expect
        .poll(() => requestCapture.getRequestCount(API_PATTERNS.LIST_CREATE))
        .toBeGreaterThan(0)
      logCapturedRequests(requestCapture)

      const request = requestCapture.getLastRequest(API_PATTERNS.LIST_CREATE)
      expect(request?.body, 'List create body should not be empty').toBeTruthy()
      const body = request!.body as Record<string, unknown>

      expect(body.workspace_id).toBe(WORKSPACE_ID)
      expect(body.name).toBe(testListData.name)
      expect(body.id).toBe('e2etestlist')
      expect(body.description).toBe(testListData.description)
      expect(body.is_double_optin).toBe(testListData.is_double_optin)
      expect(body.is_public).toBe(testListData.is_public)
      expect(body.double_optin_template).toEqual({ id: 'tpl-1', version: 1 })
    })

    test('verifies list configuration settings in payload', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByLabel('Name', { exact: true }).fill('Configuration Test List')

      // Both toggles left at their defaults: the payload must spell them out as false
      await drawer.getByRole('button', { name: 'Create list', exact: true }).click()

      await expect
        .poll(() => requestCapture.getRequestCount(API_PATTERNS.LIST_CREATE))
        .toBeGreaterThan(0)

      const request = requestCapture.getLastRequest(API_PATTERNS.LIST_CREATE)
      expect(request?.body, 'List create body should not be empty').toBeTruthy()
      const body = request!.body as Record<string, unknown>

      expect(body.name).toBe('Configuration Test List')
      expect(body.id).toBe('configurationtestlist')
      expect(body.is_double_optin).toBe(false)
      expect(body.is_public).toBe(false)
    })
  })
})
