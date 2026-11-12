import type { Locator, Page } from '@playwright/test'
import { test, expect, requestCapture } from '../fixtures/auth'
import { waitForLoading, waitForSuccessMessage } from '../fixtures/test-utils'
import { API_PATTERNS } from '../fixtures/request-capture'
import { testListData } from '../fixtures/form-data'
import { logCapturedRequests } from '../fixtures/payload-assertions'

const WORKSPACE_ID = 'test-workspace'

// antd 6 renamed `.ant-drawer-content` out of existence; the drawer's `role="dialog"`
// node now carries the title as its accessible name, so every drawer is addressed by
// role + name instead of by class.
const createListDrawer = (page: Page) =>
  page.getByRole('dialog', { name: 'Create New List', exact: true })

const editListDrawer = (page: Page) => page.getByRole('dialog', { name: 'Edit List', exact: true })

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
      await expect(drawer.getByLabel('List ID', { exact: true })).toBeVisible()
    })

    test('fills and submits list form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByLabel('Name', { exact: true }).fill('Test Newsletter List')

      // The ID is derived from the name: lowercased, non-alphanumerics stripped
      await expect(drawer.getByLabel('List ID', { exact: true })).toHaveValue('testnewsletterlist')

      await drawer
        .getByLabel('Description', { exact: true })
        .fill('A test newsletter list with all fields')

      await drawer.getByRole('button', { name: 'Create', exact: true }).click()

      await waitForSuccessMessage(page)
      await expect(drawer).toBeHidden()
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

      const doubleOptIn = drawer.getByRole('switch', { name: 'Double Opt-in' })
      await expect(doubleOptIn).toBeVisible()
      await expect(doubleOptIn).not.toBeChecked()

      // Turning it on is what pulls in the confirmation template field
      await doubleOptIn.click()
      await expect(doubleOptIn).toBeChecked()
      await expect(drawer.getByPlaceholder('Select confirmation email template')).toBeVisible()
    })

    // The template picker only has something to show when the workspace has
    // templates, so this case runs against the populated fixture.
    test('shows template selection options', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)
      await drawer.getByRole('switch', { name: 'Double Opt-in' }).click()

      // The template field opens a picker listing the workspace templates
      await drawer.getByPlaceholder('Select confirmation email template').click()

      const picker = page.getByRole('dialog', { name: 'Select Template', exact: true })
      await expect(picker).toBeVisible()
      await expect(picker.getByText('Welcome Email')).toBeVisible()
      await expect(picker.getByRole('button', { name: 'Select', exact: true })).toHaveCount(3)
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

      const idInput = drawer.getByLabel('List ID', { exact: true })
      await expect(idInput).toHaveValue('list-1')
      // The ID is the list's key, so editing an existing list cannot change it
      await expect(idInput).toBeDisabled()
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
      await expect(drawer.getByRole('switch', { name: 'Double Opt-in' })).toBeChecked()
      await expect(drawer.getByRole('switch', { name: 'Public' })).toBeChecked()
    })
  })

  test.describe('Form Validation', () => {
    test('requires list name', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByRole('button', { name: 'Create', exact: true }).click()

      await expect(drawer.getByText('Please enter a list name')).toBeVisible()
      await expect(drawer.getByText('Please enter a list ID')).toBeVisible()
      expect(requestCapture.getRequestCount(API_PATTERNS.LIST_CREATE)).toBe(0)
    })

    test('validates list ID format', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByLabel('Name', { exact: true }).fill('Test List')

      const idInput = drawer.getByLabel('List ID', { exact: true })
      await idInput.fill('invalid@id!')

      await drawer.getByRole('button', { name: 'Create', exact: true }).click()

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
  })

  test.describe('Full Form Submission with Payload Verification', () => {
    // Uses the populated fixture: double opt-in requires picking a real template.
    test('creates list with all fields and verifies payload', async ({
      authenticatedPageWithData
    }) => {
      const page = authenticatedPageWithData

      await page.goto(CONTACTS_URL)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByLabel('Name', { exact: true }).fill(testListData.name)
      await drawer.getByLabel('Description', { exact: true }).fill(testListData.description || '')

      const publicSwitch = drawer.getByRole('switch', { name: 'Public' })
      if (testListData.is_public) {
        await publicSwitch.click()
      }
      await expect(publicSwitch).toBeChecked({ checked: !!testListData.is_public })

      const doubleOptInSwitch = drawer.getByRole('switch', { name: 'Double Opt-in' })
      if (testListData.is_double_optin) {
        await doubleOptInSwitch.click()
      }
      await expect(doubleOptInSwitch).toBeChecked({ checked: !!testListData.is_double_optin })

      // Double opt-in makes the confirmation template a required field
      await drawer.getByPlaceholder('Select confirmation email template').click()
      const picker = page.getByRole('dialog', { name: 'Select Template', exact: true })
      await picker
        .getByRole('listitem')
        .filter({ hasText: 'Welcome Email' })
        .getByRole('button', { name: 'Select', exact: true })
        .click()
      await expect(drawer.getByPlaceholder('Select confirmation email template')).toHaveValue(
        'Welcome Email'
      )

      await drawer.getByRole('button', { name: 'Create', exact: true }).click()

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
      await drawer.getByRole('button', { name: 'Create', exact: true }).click()

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
