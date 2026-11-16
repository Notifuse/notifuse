import type { Page, Route } from '@playwright/test'
import { expect } from './auth'
import { waitForLoading } from './test-utils'

export const WORKSPACE_ID = 'test-workspace'

const json = (route: Route, data: unknown) =>
  route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(data) })

// The fixtures route every API call through one handler; a route added here is
// registered later, so Playwright tries it first.
export const mockApi = async (page: Page, path: string, data: unknown) => {
  await page.route(`https://localapi.notifuse.com:4000/api/${path}*`, (route) => json(route, data))
}

// Opens the visual editor of a template, the API answering with that template
// The label of the drawer's Content step, in the console's language
export interface EditorLabels {
  contentStep: string
}
const ENGLISH: EditorLabels = { contentStep: 'Content' }

export const openTemplateEditor = async (
  page: Page,
  shown: object,
  labels: EditorLabels = ENGLISH
) => {
  await mockApi(page, 'templates.list', { templates: [shown], total: 1 })
  await mockApi(page, 'templates.get', { template: shown })
  await mockApi(page, 'templates.usage', { usage: {}, references: [], hidden_types: [] })

  await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
  await waitForLoading(page)
  // A click on the row opens the editor of a member who can write templates
  await page.locator('.ant-table-row').first().click()
  // The template drawer is full-screen: its dialog holds the 40px bar with the steps.
  const drawer = page.getByRole('dialog').filter({ has: page.locator('[data-drawer-bar]') })
  await expect(drawer).toBeVisible()
  await drawer
    .locator('[data-drawer-bar-center] .ant-segmented-item')
    .filter({ hasText: new RegExp(`^${labels.contentStep}`) })
    .click()
  return drawer
}
