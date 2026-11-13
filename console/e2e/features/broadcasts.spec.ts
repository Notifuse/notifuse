import { test, expect, requestCapture } from '../fixtures/auth'
import { waitForLoading } from '../fixtures/test-utils'
import { API_PATTERNS } from '../fixtures/request-capture'
import { logCapturedRequests } from '../fixtures/payload-assertions'
import type { Locator, Page } from '@playwright/test'

const WORKSPACE_ID = 'test-workspace'
const BROADCASTS = `/console/workspace/${WORKSPACE_ID}/broadcasts`

// The table row of a given broadcast.
const row = (page: Page, broadcastName: string): Locator =>
  page.locator('.ant-table-tbody tr').filter({ hasText: broadcastName })

// The composer and the detail are full drawers: antd puts role="dialog" on them.
const drawer = (page: Page): Locator => page.locator('.ant-drawer-open [role="dialog"]').last()

test.describe('Broadcasts Feature', () => {
  test.describe('List', () => {
    test('shows the first-broadcast empty state', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(BROADCASTS)
      await waitForLoading(page)

      await expect(page.getByText('No broadcasts yet')).toBeVisible()
      await expect(page.getByRole('button', { name: 'New broadcast' })).toBeVisible()
    })

    test('lists each broadcast with its status', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(BROADCASTS)
      await waitForLoading(page)

      await expect(row(page, 'January Newsletter').getByText('Draft', { exact: true })).toBeVisible()
      await expect(row(page, 'Product Launch').getByText('Sent', { exact: true })).toBeVisible()
      await expect(row(page, 'A/B Test Campaign').getByText('Scheduled', { exact: true })).toBeVisible()
      // The A/B broadcast says how many variants it tests.
      await expect(row(page, 'A/B Test Campaign').getByText('A/B · 2 variants')).toBeVisible()
    })

    test('writes the status tab and the search to the URL', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(BROADCASTS)
      await waitForLoading(page)

      await page.locator('.ant-segmented-item').filter({ hasText: 'Drafts' }).click()
      await expect(page).toHaveURL(/status=draft/)

      await page.getByRole('textbox', { name: 'Search by name' }).fill('launch')
      await expect(page).toHaveURL(/q=launch/)
    })
  })

  test.describe('Detail', () => {
    test('opens a sent broadcast in the detail drawer', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(BROADCASTS)
      await waitForLoading(page)

      await row(page, 'Product Launch').click()
      await expect(page).toHaveURL(/broadcast=bc-2/)
      const detail = drawer(page)
      await expect(detail.getByRole('heading', { name: 'Product Launch' })).toBeVisible()
      await expect(detail.getByText('Audience', { exact: true })).toBeVisible()
    })
  })

  test.describe('Composer', () => {
    test('opens a draft in the composer with its values', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(BROADCASTS)
      await waitForLoading(page)

      await row(page, 'January Newsletter').click()
      await expect(page).toHaveURL(/compose=bc-1/)
      const composer = drawer(page)
      await expect(composer.getByRole('textbox', { name: 'Broadcast name' })).toHaveValue('January Newsletter')
      await expect(composer.getByText('Newsletter', { exact: true }).first()).toBeVisible()
    })

    test('creates the broadcast once a list is chosen', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(BROADCASTS)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'New broadcast' }).click()
      await expect(page).toHaveURL(/compose=new/)
      const composer = drawer(page)

      await composer.getByRole('textbox', { name: 'Broadcast name' }).fill('Spring sale')
      await composer.getByRole('combobox', { name: 'List' }).click()
      await page.locator('.ant-select-item-option').filter({ hasText: 'Newsletter' }).first().click()

      // The autosave creates the broadcast, then the URL carries its id.
      await expect
        .poll(() => requestCapture.getRequestCount(API_PATTERNS.BROADCAST_CREATE))
        .toBeGreaterThan(0)
      logCapturedRequests(requestCapture)
      const body = requestCapture.getLastRequest(API_PATTERNS.BROADCAST_CREATE)?.body as {
        workspace_id?: string
        name?: string
        audience?: { list?: string; exclude_unsubscribed?: boolean }
        utm_parameters?: { campaign?: string }
        test_settings?: { enabled?: boolean }
      }
      expect(body.workspace_id).toBe(WORKSPACE_ID)
      expect(body.name).toBe('Spring sale')
      expect(body.audience?.list).toBe('list-1')
      expect(body.audience?.exclude_unsubscribed).toBe(true)
      expect(body.utm_parameters?.campaign).toBe('spring_sale')
      expect(body.test_settings?.enabled).toBe(false)
      await expect(page).toHaveURL(/compose=bc-1/)
    })

    test('closes an untouched new broadcast without a confirmation', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(BROADCASTS)
      await waitForLoading(page)

      await page.getByRole('button', { name: 'New broadcast' }).click()
      const composer = drawer(page)
      await expect(composer.getByRole('textbox', { name: 'Broadcast name' })).toBeVisible()
      await composer.getByRole('button', { name: 'Close' }).click()

      await expect(page).not.toHaveURL(/compose=/)
      expect(requestCapture.getRequestCount(API_PATTERNS.BROADCAST_CREATE)).toBe(0)
    })
  })

  test.describe('Navigation', () => {
    test('navigates to broadcasts from sidebar', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/`)
      await waitForLoading(page)

      await page.locator('a[href*="broadcasts"], [data-menu-id*="broadcasts"]').first().click()

      await expect(page).toHaveURL(/broadcasts/)
    })
  })
})
