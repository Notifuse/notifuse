import type { Locator, Page } from '@playwright/test'
import { test, expect, requestCapture } from '../fixtures/auth'
import { waitForLoading, waitForSuccessMessage, clickButton } from '../fixtures/test-utils'
import { API_PATTERNS } from '../fixtures/request-capture'
import { fillContactForm } from '../fixtures/form-fillers'
import { testContactDataMinimal } from '../fixtures/form-data'
import { logCapturedRequests } from '../fixtures/payload-assertions'

const WORKSPACE_ID = 'test-workspace'

// antd 6 renamed `.ant-drawer-content` out of existence; the drawer's `role="dialog"`
// node now carries the title as its accessible name, so every drawer is addressed by
// role + name instead of by class.
const addContactDrawer = (page: Page) =>
  page.getByRole('dialog', { name: 'Add Contact', exact: true })

const updateContactDrawer = (page: Page) =>
  page.getByRole('dialog', { name: 'Update Contact', exact: true })

/** Open the row's "…" menu and pick Edit, which opens the prefilled upsert drawer. */
async function openEditDrawer(page: Page, rowIndex = 0): Promise<Locator> {
  const row = page.locator('.ant-table-row').nth(rowIndex)
  await row
    .locator('.contacts-actions-col button')
    .filter({ has: page.locator('svg[data-icon="ellipsis-vertical"]') })
    .click()
  await page.getByRole('menuitem', { name: 'Edit', exact: true }).click()

  const drawer = updateContactDrawer(page)
  await expect(drawer).toBeVisible()
  return drawer
}

/** Add one of the optional contact fields through the "Add an optional field" select. */
async function addOptionalField(drawer: Locator, page: Page, label: string): Promise<void> {
  const select = drawer.getByRole('combobox')
  await select.click()
  // The option list is virtualised, so search for the field instead of scrolling to it
  await select.fill(label)
  await expect(page.getByRole('option', { name: label, exact: true })).toHaveCount(1)
  await select.press('Enter')
  await expect(drawer.getByLabel(label, { exact: true })).toBeVisible()
}

test.describe('Contacts Feature', () => {
  test.describe('Page Load & Empty State', () => {
    test('loads contacts page and shows empty state', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Should show Contacts heading
      await expect(page.getByText('Contacts', { exact: true }).first()).toBeVisible()

      // The table renders its own empty text when the list endpoint returns no contacts
      await expect(
        page.getByText('No contacts found. Add some contacts to get started.')
      ).toBeVisible()
    })

    test('loads contacts page with data', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await expect(page).toHaveURL(/contacts/)

      // The three mocked contacts each get a row
      await expect(page.locator('.ant-table-row')).toHaveCount(3)
    })
  })

  test.describe('CRUD Operations', () => {
    test('opens add contact drawer', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await clickButton(page, 'Add')

      const drawer = addContactDrawer(page)
      await expect(drawer).toBeVisible()

      // Email is the only field the form shows up front
      await expect(drawer.getByLabel('Email', { exact: true })).toBeVisible()
    })

    test('creates a new contact with required fields', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await clickButton(page, 'Add')
      const drawer = addContactDrawer(page)
      await expect(drawer).toBeVisible()

      await drawer.getByLabel('Email', { exact: true }).fill('newcontact@example.com')
      await drawer.getByRole('button', { name: 'Save', exact: true }).click()

      await waitForSuccessMessage(page)
      await expect(drawer).toBeHidden()

      const request = requestCapture.getLastRequest(API_PATTERNS.CONTACT_UPSERT)
      expect(request?.body, 'Contact upsert body should not be empty').toBeTruthy()
      const contact = (request!.body as { contact: Record<string, unknown> }).contact
      expect(contact.email).toBe('newcontact@example.com')
    })

    test('creates a new contact with all fields', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await clickButton(page, 'Add')
      const drawer = addContactDrawer(page)
      await expect(drawer).toBeVisible()

      await drawer.getByLabel('Email', { exact: true }).fill('complete@example.com')

      // Optional fields only exist once they are picked from the select
      await addOptionalField(drawer, page, 'First Name')
      await drawer.getByLabel('First Name', { exact: true }).fill('Test')
      await addOptionalField(drawer, page, 'Last Name')
      await drawer.getByLabel('Last Name', { exact: true }).fill('User')

      await drawer.getByRole('button', { name: 'Save', exact: true }).click()

      await waitForSuccessMessage(page)

      const request = requestCapture.getLastRequest(API_PATTERNS.CONTACT_UPSERT)
      expect(request?.body, 'Contact upsert body should not be empty').toBeTruthy()
      const contact = (request!.body as { contact: Record<string, unknown> }).contact
      expect(contact.email).toBe('complete@example.com')
      expect(contact.first_name).toBe('Test')
      expect(contact.last_name).toBe('User')
    })

    test('views contact details in drawer', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // The eye button in the row's action column opens the read-only details drawer
      await page
        .locator('.ant-table-row')
        .first()
        .locator('.contacts-actions-col button')
        .filter({ has: page.locator('svg[data-icon="eye"]') })
        .click()

      const drawer = page.getByRole('dialog', { name: 'Contact Details', exact: true })
      await expect(drawer).toBeVisible()
      await expect(drawer.getByText('john@example.com').first()).toBeVisible()
    })

    test('closes contact drawer', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await page
        .locator('.ant-table-row')
        .first()
        .locator('.contacts-actions-col button')
        .filter({ has: page.locator('svg[data-icon="eye"]') })
        .click()

      const drawer = page.getByRole('dialog', { name: 'Contact Details', exact: true })
      await expect(drawer).toBeVisible()

      await drawer.getByRole('button', { name: 'Close', exact: true }).click()

      await expect(drawer).toBeHidden()
    })
  })

  test.describe('Filtering & Search', () => {
    test('filters contacts by email search', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      // Each filter field is a button that opens a popover with its input
      await page.getByRole('button', { name: 'Email', exact: true }).click()
      await page.getByPlaceholder('Filter by Email').fill('john@example.com')
      await page.getByRole('button', { name: 'Confirm', exact: true }).click()

      // The filter is applied by writing it into the URL query string
      await expect(page).toHaveURL(/[?&]email=john%40example\.com/)
      await expect(page.getByRole('button', { name: 'Email: john@example.com' })).toBeVisible()
    })

    test('shows a filter button for every contact field', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      for (const field of ['Email', 'External ID', 'First Name', 'Last Name', 'Country', 'List']) {
        await expect(page.getByRole('button', { name: field, exact: true })).toBeVisible()
      }
    })
  })

  test.describe('Table Display', () => {
    test('displays contact email column', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await expect(page.getByRole('columnheader', { name: 'Email', exact: true })).toBeVisible()
      await expect(page.locator('.ant-table-row').first()).toContainText('john@example.com')
    })

    test('displays multiple contacts', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      const rows = page.locator('.ant-table-row')
      await expect(rows).toHaveCount(3)
      await expect(rows.nth(0)).toContainText('john@example.com')
      await expect(rows.nth(1)).toContainText('jane@example.com')
      await expect(rows.nth(2)).toContainText('bob@example.com')
    })
  })

  test.describe('Edit Form Prefill', () => {
    test('edit contact drawer shows existing contact email', async ({
      authenticatedPageWithData
    }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      const emailInput = drawer.getByLabel('Email', { exact: true })
      await expect(emailInput).toHaveValue('john@example.com')
      // The email identifies the contact, so it cannot be edited
      await expect(emailInput).toBeDisabled()
    })

    test('edit contact shows existing first name', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      await expect(drawer.getByLabel('First Name', { exact: true })).toHaveValue('John')
    })

    test('edit contact shows existing last name', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      await expect(drawer.getByLabel('Last Name', { exact: true })).toHaveValue('Doe')
    })

    test('edit contact preserves custom fields', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      await expect(drawer.getByLabel('Custom String 1', { exact: true })).toHaveValue('Acme Corp')
      await expect(drawer.getByLabel('Custom String 2', { exact: true })).toHaveValue('Pro')
    })

    test('edit contact preserves location fields', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      await expect(drawer.getByLabel('Address Line 1', { exact: true })).toHaveValue('123 Main St')
      await expect(drawer.getByLabel('Address Line 2', { exact: true })).toHaveValue('Apt 4')
      await expect(drawer.getByLabel('State', { exact: true })).toHaveValue('NY')
      await expect(drawer.getByLabel('Postcode', { exact: true })).toHaveValue('10001')
    })
  })

  test.describe('Validation', () => {
    test('shows error for invalid email format', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await clickButton(page, 'Add')
      const drawer = addContactDrawer(page)
      await expect(drawer).toBeVisible()

      await drawer.getByLabel('Email', { exact: true }).fill('invalid-email')
      await drawer.getByRole('button', { name: 'Save', exact: true }).click()

      await expect(drawer.getByText('Please enter a valid email')).toBeVisible()
      expect(requestCapture.getRequestCount(API_PATTERNS.CONTACT_UPSERT)).toBe(0)
    })

    test('requires email field', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await clickButton(page, 'Add')
      const drawer = addContactDrawer(page)
      await expect(drawer).toBeVisible()

      await drawer.getByRole('button', { name: 'Save', exact: true }).click()

      await expect(drawer.getByText('Email is required')).toBeVisible()
      expect(requestCapture.getRequestCount(API_PATTERNS.CONTACT_UPSERT)).toBe(0)
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
      await expect(page.getByText('Contacts', { exact: true }).first()).toBeVisible()
    })
  })

  test.describe('Full Form Submission with Payload Verification', () => {
    test('creates contact with email and verifies payload', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await clickButton(page, 'Add')
      const drawer = addContactDrawer(page)
      await expect(drawer).toBeVisible()

      // Fill only the required email field (email is always visible)
      await fillContactForm(page, testContactDataMinimal)

      await drawer.getByRole('button', { name: 'Save', exact: true }).click()

      await expect
        .poll(() => requestCapture.getRequestCount(API_PATTERNS.CONTACT_UPSERT))
        .toBeGreaterThan(0)
      logCapturedRequests(requestCapture)

      const request = requestCapture.getLastRequest(API_PATTERNS.CONTACT_UPSERT)
      expect(request?.body, 'Contact upsert body should not be empty').toBeTruthy()
      const body = request!.body as Record<string, unknown>
      expect(body.contact, 'Contact object should exist in request').toBeDefined()

      const contact = body.contact as Record<string, unknown>
      expect(contact.email).toBe(testContactDataMinimal.email)
      expect(contact.workspace_id).toBe(WORKSPACE_ID)
    })

    test('verifies custom fields are sent in payload', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
      await waitForLoading(page)

      await clickButton(page, 'Add')
      const drawer = addContactDrawer(page)
      await expect(drawer).toBeVisible()

      await drawer.getByLabel('Email', { exact: true }).fill('custom-fields-test@example.com')

      // Custom fields have to be added to the form before they can be filled
      await addOptionalField(drawer, page, 'Custom String 1')
      await drawer.getByLabel('Custom String 1', { exact: true }).fill('Custom Value 1')
      await addOptionalField(drawer, page, 'Custom Number 1')
      await drawer.getByLabel('Custom Number 1', { exact: true }).fill('123')

      await drawer.getByRole('button', { name: 'Save', exact: true }).click()

      await expect
        .poll(() => requestCapture.getRequestCount(API_PATTERNS.CONTACT_UPSERT))
        .toBeGreaterThan(0)

      const request = requestCapture.getLastRequest(API_PATTERNS.CONTACT_UPSERT)
      expect(request?.body, 'Contact upsert body should not be empty').toBeTruthy()
      const contact = (request!.body as { contact: Record<string, unknown> }).contact

      expect(contact.email).toBe('custom-fields-test@example.com')
      expect(contact.custom_string_1).toBe('Custom Value 1')
      expect(contact.custom_number_1).toBe(123)
    })
  })
})
