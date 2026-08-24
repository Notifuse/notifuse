import type { Locator, Page } from '@playwright/test'
import { test, expect, requestCapture } from '../fixtures/auth'
import { waitForLoading } from '../fixtures/test-utils'
import { API_PATTERNS } from '../fixtures/request-capture'
import { testTemplateData } from '../fixtures/form-data'
import { logCapturedRequests } from '../fixtures/payload-assertions'

const WORKSPACE_ID = 'test-workspace'

// antd 6 dropped .ant-drawer-content: the drawer's role="dialog" now sits on div.ant-drawer-section
// and takes its accessible name from the drawer title, so role+name is the stable handle and it
// pins which of the template drawers (create / edit / clone / preview) is on screen.
const templateDrawer = (page: Page, title: string) =>
  page.getByRole('dialog', { name: title, exact: true })

const openCreateDrawer = async (page: Page) => {
  await page.getByRole('button', { name: 'Create Template', exact: true }).click()
  const drawer = templateDrawer(page, 'Create an email template')
  await expect(drawer).toBeVisible()
  return drawer
}

// The row actions are icon-only buttons, so they are addressed by the icon they carry.
const rowAction = (page: Page, icon: string): Locator =>
  page.locator('.ant-table-row').first().locator(`button:has(svg[data-icon="${icon}"])`)

const openEditDrawer = async (page: Page) => {
  await rowAction(page, 'pen-to-square').click()
  const drawer = templateDrawer(page, 'Edit email template')
  await expect(drawer).toBeVisible()
  return drawer
}

test.describe('Templates Feature', () => {
  test.describe('Page Load & Empty State', () => {
    test('loads templates page and shows empty state', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const content = page.locator('.ant-layout-content')
      await expect(content.getByText('No templates found')).toBeVisible()
      await expect(content.getByText('Create your first template to get started')).toBeVisible()
      await expect(page.getByRole('button', { name: 'Create Template', exact: true })).toBeVisible()
    })

    test('loads templates page with data', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      // The mocked workspace owns three templates, all listed in the table
      const rows = page.locator('.ant-table-row')
      await expect(rows).toHaveCount(3)
      for (const name of ['Welcome Email', 'Monthly Newsletter', 'Unsubscribe Confirmation']) {
        await expect(page.locator('.ant-table').getByText(name, { exact: true })).toBeVisible()
      }

      // The Subject column reads template.email.subject, with the subject preview under it
      const welcomeRow = rows.filter({ hasText: 'Welcome Email' })
      await expect(
        welcomeRow.getByText('Welcome to {{workspace.name}}!', { exact: true })
      ).toBeVisible()
      await expect(welcomeRow.getByText('Your account is ready', { exact: true })).toBeVisible()
    })
  })

  test.describe('CRUD Operations', () => {
    test('opens create template form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // The creation wizard opens on its first step
      await expect(drawer.getByRole('tab', { name: '1. Settings' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      await expect(drawer.getByRole('tab', { name: '2. Template' })).toBeVisible()
    })

    test('fills template form fields', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // Naming a new template derives its API id
      const nameInput = drawer.getByLabel('Template name')
      await nameInput.fill('Test Email Template')
      await expect(nameInput).toHaveValue('Test Email Template')
      await expect(drawer.getByLabel('Template ID (utm_content)')).toHaveValue(
        'test-email-template'
      )

      // Category is a required choice, and picking one shows it back
      await drawer.getByLabel('Category').click()
      await page.locator('.ant-select-item-option').filter({ hasText: 'Marketing' }).click()
      await expect(
        drawer.locator('.ant-form-item').filter({ hasText: 'Category' }).locator('.ant-select-content')
      ).toHaveText('Marketing')

      await expect(drawer.getByRole('button', { name: 'Next', exact: true })).toBeVisible()
    })

    test('views template details', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      // The row's preview action opens the template's own drawer, titled after it
      await rowAction(page, 'eye').click()

      const preview = templateDrawer(page, 'Welcome Email')
      await expect(preview).toBeVisible()
      await expect(preview.getByText('From', { exact: true })).toBeVisible()
      // The compile mock returns no rendered subject, so the drawer falls back to the
      // template's own email payload for both the subject and its preview line
      await expect(preview.getByText('Subject', { exact: true })).toBeVisible()
      await expect(
        preview.getByText('Welcome to {{workspace.name}}!', { exact: true })
      ).toBeVisible()
      await expect(preview.getByText('Subject preview', { exact: true })).toBeVisible()
      await expect(preview.getByText('Your account is ready', { exact: true })).toBeVisible()
    })
  })

  test.describe('Template Editor', () => {
    test('shows template name field', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      const nameInput = drawer.getByLabel('Template name')
      await expect(nameInput).toBeVisible()
      await expect(nameInput).toHaveValue('')
      await expect(nameInput).toHaveAttribute('placeholder', 'i.e: Welcome Email')
    })

    test('shows subject field', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // Both subject lines of the sender section are part of the settings step
      await expect(drawer.getByLabel('Email subject', { exact: true })).toBeVisible()
      await expect(drawer.getByLabel('Subject preview', { exact: true })).toBeVisible()
    })
  })

  test.describe('Categories', () => {
    test('shows category selection', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      await drawer.getByLabel('Category').click()

      // The full list of template categories is offered
      const options = page.locator('.ant-select-item-option')
      await expect(options).toHaveCount(7)
      for (const category of [
        'Marketing',
        'Transactional',
        'Welcome',
        'Opt-in',
        'Unsubscribe',
        'Bounce',
        'Blocklist'
      ]) {
        await expect(options.filter({ hasText: category })).toBeVisible()
      }
    })
  })

  test.describe('Edit Form Prefill', () => {
    test('edit template drawer shows existing template name', async ({
      authenticatedPageWithData
    }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      await expect(drawer.getByLabel('Template name')).toHaveValue('Welcome Email')
      // An existing template keeps its id, which is why the field is locked
      await expect(drawer.getByLabel('Template ID (utm_content)')).toHaveValue('tpl-1')
      await expect(drawer.getByLabel('Template ID (utm_content)')).toBeDisabled()
    })

    test('edit template preserves category selection', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      await expect(
        drawer.locator('.ant-form-item').filter({ hasText: 'Category' }).locator('.ant-select-content')
      ).toHaveText('Welcome')
    })

    test('edit template preserves subject line', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      // The drawer seeds the subject from template.email.subject
      await expect(drawer.getByLabel('Email subject', { exact: true })).toHaveValue(
        'Welcome to {{workspace.name}}!'
      )
    })

    test('edit template preserves from email', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openEditDrawer(page)

      // There is no from-address field: the sender comes from the workspace email provider, so
      // the editable half of the sender is the reply-to and the provider's custom sender.
      await expect(drawer.getByText('Sender', { exact: true })).toBeVisible()
      await expect(drawer.getByLabel('Reply to', { exact: true })).toBeVisible()
      await expect(
        drawer.getByLabel('Custom sender (transactional email provider)', { exact: true })
      ).toBeVisible()
    })
  })

  test.describe('Form Validation', () => {
    test('shows form validation on submit', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // Saving an untouched wizard reports every required field of the settings step and
      // switches back to it, and nothing is sent.
      await drawer.getByRole('button', { name: 'Next', exact: true }).click()
      await drawer.getByRole('button', { name: 'Save', exact: true }).click()

      await expect(drawer.getByRole('tab', { name: '1. Settings' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      await expect(drawer.locator('.ant-form-item-explain-error')).toHaveText([
        'Please enter Template name',
        'ID must contain only lowercase letters, numbers, underscores, and hyphens',
        'Please enter Category',
        'Please enter Email subject',
        'Please enter Subject preview'
      ])
      expect(requestCapture.getRequestCount(API_PATTERNS.TEMPLATE_CREATE)).toBe(0)
    })

    test('shows form with required subject field', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // The subject is marked required and starts empty
      const subjectItem = drawer.locator('.ant-form-item').filter({ hasText: 'Email subject' })
      await expect(subjectItem.locator('.ant-form-item-required')).toBeVisible()
      await expect(drawer.getByLabel('Email subject', { exact: true })).toHaveValue('')
    })
  })

  test.describe('Navigation', () => {
    test('navigates to templates from sidebar', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      // Start at dashboard
      await page.goto(`/console/workspace/${WORKSPACE_ID}/`)
      await waitForLoading(page)

      // Templates lives under the collapsed "Content" group of the sidebar
      await page.getByRole('menuitem', { name: 'Content' }).click()
      await page.getByRole('link', { name: 'Templates' }).click()

      // Should be on templates page
      await expect(page).toHaveURL(/templates/)
      await expect(page.getByRole('button', { name: 'Create Template', exact: true })).toBeVisible()
    })

    test('can close create form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // The drawer is deliberately not dismissible by mask or keyboard, only by its own controls
      await drawer.getByRole('button', { name: 'Close' }).click()
      await expect(drawer).toBeHidden()
    })
  })

  test.describe('Full Form Submission with Payload Verification', () => {
    test('creates template with all fields and verifies payload', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // Step 1: settings
      await drawer.getByLabel('Template name').fill(testTemplateData.name)
      await drawer.getByLabel('Template ID (utm_content)').fill(testTemplateData.id)
      await drawer.getByLabel('Category').click()
      await page.locator('.ant-select-item-option').filter({ hasText: 'Marketing' }).click()
      await drawer.getByLabel('Email subject', { exact: true }).fill(testTemplateData.subject)
      await drawer
        .getByLabel('Subject preview', { exact: true })
        .fill(testTemplateData.subject_preview!)

      // Step 2: the email itself, which starts from the default blocks
      await drawer.getByRole('button', { name: 'Next', exact: true }).click()
      await expect(drawer.getByRole('tab', { name: '2. Template' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      await drawer.getByRole('button', { name: 'Save', exact: true }).click()

      await expect
        .poll(() => requestCapture.getRequestCount(API_PATTERNS.TEMPLATE_CREATE))
        .toBeGreaterThan(0)
      logCapturedRequests(requestCapture)

      // Verify the create request carried what was typed in
      const request = requestCapture.getLastRequest(API_PATTERNS.TEMPLATE_CREATE)
      expect(request?.body, 'Template create body should not be empty').toBeTruthy()
      const body = request!.body as {
        name?: string
        id?: string
        category?: string
        channel?: string
        email?: { subject?: string; subject_preview?: string; editor_mode?: string }
      }
      expect(body.name).toBe(testTemplateData.name)
      expect(body.id).toBe(testTemplateData.id)
      expect(body.category).toBe(testTemplateData.category)
      expect(body.channel).toBe('email')
      expect(body.email?.subject).toBe(testTemplateData.subject)
      expect(body.email?.subject_preview).toBe(testTemplateData.subject_preview)
      expect(body.email?.editor_mode).toBe('visual')

      // And the drawer closes on success
      await expect(drawer).toBeHidden()
    })
  })
})
