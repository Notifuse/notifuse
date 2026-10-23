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
export const openTemplateEditor = async (page: Page, shown: object) => {
  await mockApi(page, 'templates.list', { templates: [shown], total: 1 })
  await mockApi(page, 'templates.get', { template: shown })

  await page.goto(`/console/workspace/${WORKSPACE_ID}/templates`)
  await waitForLoading(page)
  await page
    .locator('.ant-table-row')
    .first()
    .locator('button:has(svg[data-icon="pen-to-square"])')
    .click()
  const drawer = page.getByRole('dialog', { name: 'Edit email template', exact: true })
  await expect(drawer).toBeVisible()
  await drawer.getByRole('tab', { name: '2. Template' }).click()
  return drawer
}
