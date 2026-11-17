import type { Locator, Page } from '@playwright/test'
import { test, expect, requestCapture } from '../fixtures/auth'
import { waitForLoading } from '../fixtures/test-utils'
import { API_PATTERNS } from '../fixtures/request-capture'
import { testTemplateData } from '../fixtures/form-data'
import { logCapturedRequests } from '../fixtures/payload-assertions'

const WORKSPACE_ID = 'test-workspace'

// The template drawer is a full-screen drawer: its dialog holds the 40px bar, whose title says
// which template is open ("New template", or the name and its version).
const templateDrawer = (page: Page, title: string | RegExp) =>
  page
    .getByRole('dialog')
    .filter({ has: page.locator('[data-drawer-bar]') })
    .filter({ has: page.getByTestId('template-bar-name').filter({ hasText: title }) })

// A step of the bar's switch; the selected one carries antd's selected class.
const step = (drawer: Locator, name: string) =>
  drawer.locator('.ant-segmented-item').filter({ hasText: new RegExp(`^${name}`) })

// The header's button; the empty state repeats it below the table.
const newTemplateButton = (page: Page): Locator =>
  page.getByRole('button', { name: 'New template', exact: true }).first()

// The open drawer, whatever its bar's title says once a name is typed.
const openDrawer = (page: Page) => page.getByRole('dialog').filter({ has: page.locator('[data-drawer-bar]') })

// New template asks where to start first: Blank, here.
const openCreateDrawer = async (page: Page) => {
  await newTemplateButton(page).click()
  await page.locator('[data-model="blank"]').click()
  await expect(templateDrawer(page, 'New template')).toBeVisible()
  return openDrawer(page)
}

// The row actions sit in the row's "More actions" menu.
const rowAction = async (page: Page, action: string) => {
  await page
    .locator('.ant-table-row')
    .first()
    .getByRole('button', { name: 'More actions', exact: true })
    .click()
  await page.getByRole('menuitem', { name: action, exact: true }).click()
}

const openEditDrawer = async (page: Page) => {
  await rowAction(page, 'Edit')
  await expect(templateDrawer(page, /v\d+$/)).toBeVisible()
  return openDrawer(page)
}

test.describe('Templates Feature', () => {
  test.describe('Page Load & Empty State', () => {
    test('loads templates page and shows empty state', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const content = page.locator('main')
      await expect(content.getByText('No templates found')).toBeVisible()
      await expect(content.getByText('Create your first template to get started')).toBeVisible()
      await expect(newTemplateButton(page)).toBeVisible()
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

      // Under the name: template.email.subject, then the subject preview
      const welcomeRow = rows.filter({ hasText: 'Welcome Email' })
      await expect(
        welcomeRow.getByText('Welcome to {{workspace.name}}! · Your account is ready', {
          exact: true
        })
      ).toBeVisible()

      // Used in: what templates.usage counts for each template
      const newsletterRow = rows.filter({ hasText: 'Monthly Newsletter' })
      await expect(newsletterRow.getByText('1 newsletter', { exact: true })).toBeVisible()
      await expect(welcomeRow.getByText('Not used', { exact: true })).toBeVisible()
    })
  })

  test.describe('CRUD Operations', () => {
    test('opens create template form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // The drawer opens on its Settings step; every step can be opened from the bar
      await expect(step(drawer, 'Settings')).toHaveClass(/ant-segmented-item-selected/)
      await expect(step(drawer, 'Content')).toBeVisible()
      await expect(drawer.getByRole('button', { name: 'Create template' })).toBeVisible()
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

      await expect(drawer.getByRole('button', { name: 'Create template' })).toBeVisible()
    })

    test('views template details', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      // The row's preview action opens the template's own drawer, titled after it
      await rowAction(page, 'Preview')

      // Its title also names the version and category after the name: « Welcome Email v1 · … »
      const preview = page.getByRole('dialog', { name: /^Welcome Email/ })
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

      // Both lines the inbox shows are in the Envelope card of the Settings step
      await expect(drawer.getByLabel('Subject', { exact: true })).toBeVisible()
      await expect(drawer.getByLabel('Preview text', { exact: true })).toBeVisible()
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
      await expect(drawer.getByLabel('Subject', { exact: true })).toHaveValue(
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
      await expect(drawer.getByText('Envelope', { exact: true })).toBeVisible()
      await expect(drawer.getByLabel('Reply to', { exact: true })).toBeVisible()
      await expect(drawer.getByLabel('Sender', { exact: true })).toBeVisible()
      await expect(drawer.getByText('Custom sender, from the transactional email provider')).toBeVisible()
    })
  })

  test.describe('Form Validation', () => {
    test('shows form validation on submit', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // Saving an untouched template from its Content step reports every required field of the
      // Settings step, opens it and marks it in the bar, and nothing is sent.
      await step(drawer, 'Content').click()
      await drawer.getByRole('button', { name: 'Create template' }).click()

      await expect(step(drawer, 'Settings')).toHaveClass(/ant-segmented-item-selected/)
      await expect(step(drawer, 'Settings').locator('[data-step-error]')).toBeVisible()
      await expect(drawer.locator('.ant-form-item-explain-error')).toHaveText([
        'Please enter Template name',
        'ID must contain only lowercase letters, numbers, underscores, and hyphens',
        'Please enter Category',
        'Please enter Subject',
        'Please enter Preview text'
      ])
      expect(requestCapture.getRequestCount(API_PATTERNS.TEMPLATE_CREATE)).toBe(0)
    })

    test('shows form with required subject field', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // The subject is marked required and starts empty
      await expect(drawer.locator('label.ant-form-item-required[for="email_subject"]')).toBeVisible()
      await expect(drawer.getByLabel('Subject', { exact: true })).toHaveValue('')
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
      await expect(newTemplateButton(page)).toBeVisible()
    })

    test('can close create form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
      await waitForLoading(page)

      const drawer = await openCreateDrawer(page)

      // The drawer is deliberately not dismissible by mask or keyboard, only by its own controls;
      // untouched, it closes without asking
      await drawer.locator('[data-drawer-bar]').getByRole('button', { name: 'Close', exact: true }).click()
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
      await drawer.getByLabel('Subject', { exact: true }).fill(testTemplateData.subject)
      await drawer
        .getByLabel('Preview text', { exact: true })
        .fill(testTemplateData.subject_preview!)

      // Step 2: the email itself, which starts from the default blocks
      await step(drawer, 'Content').click()
      await expect(step(drawer, 'Content')).toHaveClass(/ant-segmented-item-selected/)
      await drawer.getByRole('button', { name: 'Create template' }).click()

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
