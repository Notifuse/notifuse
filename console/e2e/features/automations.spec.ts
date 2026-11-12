import { test, expect } from '../fixtures/auth'
import { waitForLoading } from '../fixtures/test-utils'
import type { Locator, Page } from '@playwright/test'

const WORKSPACE_ID = 'test-workspace'
const AUTOMATIONS = `/console/workspace/${WORKSPACE_ID}/automations`

const row = (page: Page, name: string): Locator => page.locator('.ant-table-tbody tr').filter({ hasText: name })
const editor = (page: Page): Locator => page.locator('[data-automation-editor]')

test.describe('Automations Feature', () => {
  test.describe('List', () => {
    test('shows the first-automation empty state', async ({ authenticatedPage }) => {
      const page = authenticatedPage
      await page.goto(AUTOMATIONS)
      await waitForLoading(page)

      await expect(page.getByText('No automations yet')).toBeVisible()
      await expect(page.getByRole('button', { name: 'Create your first automation' })).toBeVisible()
    })

    test('lists each automation with its status and journeys', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData
      await page.goto(AUTOMATIONS)
      await waitForLoading(page)

      await expect(row(page, 'Welcome series').getByText('Live', { exact: true })).toBeVisible()
      await expect(row(page, 'Welcome series').getByText('3,578')).toBeVisible()
      await expect(row(page, 'Tag trial users').getByText('Paused', { exact: true })).toBeVisible()
    })

    test('writes the status tab and the search to the URL', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData
      await page.goto(AUTOMATIONS)
      await waitForLoading(page)

      await page.locator('.ant-segmented-item').filter({ hasText: 'Paused' }).click()
      await expect(page).toHaveURL(/status=paused/)
      await expect(row(page, 'Welcome series')).toHaveCount(0)

      await page.getByRole('textbox', { name: 'Search by name or trigger' }).fill('trial_started')
      await expect(page).toHaveURL(/q=trial_started/)
      await expect(row(page, 'Tag trial users')).toBeVisible()
    })
  })

  test.describe('Editor', () => {
    test('opens a live automation read-only, with its stats', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData
      await page.goto(AUTOMATIONS)
      await waitForLoading(page)

      await row(page, 'Welcome series').click()
      await expect(page).toHaveURL(/automation=auto-welcome/)
      await expect(page).toHaveURL(/stats=1/)
      await expect(page.getByText('This automation is live. Pause it to edit.')).toBeVisible()
      await expect(page.getByRole('textbox', { name: 'Automation name' })).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Save', exact: true })).toHaveCount(0)
      await expect(editor(page).locator('[data-edge-label]').filter({ hasText: /^Yes · / })).toBeVisible()
    })

    test('saves a renamed paused automation under its id', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData
      await page.goto(`${AUTOMATIONS}?automation=auto-tag`)
      await expect(editor(page)).toBeVisible()

      await page.getByRole('textbox', { name: 'Automation name' }).fill('Tag trial users now')
      await expect(page.getByText('Unsaved changes')).toBeVisible()
      await expect(page.getByRole('button', { name: 'Activate', exact: true })).toBeDisabled()

      const request = page.waitForRequest((r) => r.url().includes('/api/automations.update'))
      await page.getByRole('button', { name: 'Save', exact: true }).click()
      const body = (await request).postDataJSON() as { automation: { id: string; name: string } }
      expect(body.automation).toMatchObject({ id: 'auto-tag', name: 'Tag trial users now' })
      await expect(page.getByText('Saved', { exact: true })).toBeVisible()
      await expect(page.getByText('Unsaved changes')).toHaveCount(0)
    })

    test('asks before closing with unsaved changes', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData
      await page.goto(`${AUTOMATIONS}?automation=auto-tag`)
      await expect(editor(page)).toBeVisible()

      await page.getByRole('textbox', { name: 'Automation name' }).fill('Renamed')
      await page.getByRole('button', { name: 'Close' }).first().click()
      await expect(page.locator('.ant-modal-confirm-title').filter({ hasText: 'Unsaved Changes' })).toBeVisible()
      await page.getByRole('button', { name: 'Close without saving' }).click()
      await expect(page).not.toHaveURL(/automation=/)
    })

    test('opens a new automation and refuses it without a name', async ({ authenticatedPage }) => {
      const page = authenticatedPage
      await page.goto(AUTOMATIONS)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'Create your first automation' }).click()
      await expect(page).toHaveURL(/automation=new/)
      await expect(editor(page)).toBeVisible()
      await page.getByRole('button', { name: 'Create', exact: true }).click()
      await expect(page.getByText('Please enter an automation name')).toBeVisible()
    })

    test('says an unknown automation was not found', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData
      await page.goto(`${AUTOMATIONS}?automation=does-not-exist`)
      await expect(page.getByText('Automation not found')).toBeVisible()
    })
  })
})
