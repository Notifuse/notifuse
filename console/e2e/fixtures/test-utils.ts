import { Page, Locator } from '@playwright/test'

/**
 * Wait for Ant Design drawer to open
 */
export async function waitForDrawer(page: Page, timeout = 5000): Promise<Locator> {
  const drawer = page.locator('.ant-drawer-content')
  await drawer.waitFor({ state: 'visible', timeout })
  return drawer
}

/**
 * Wait for Ant Design drawer to close
 */
export async function waitForDrawerClose(page: Page, timeout = 5000): Promise<void> {
  await page.locator('.ant-drawer-content').waitFor({ state: 'hidden', timeout })
}

/**
 * Wait for Ant Design modal to open
 */
export async function waitForModal(page: Page, timeout = 5000): Promise<Locator> {
  const modal = page.locator('.ant-modal-content')
  await modal.waitFor({ state: 'visible', timeout })
  return modal
}

/**
 * Wait for Ant Design modal to close
 */
export async function waitForModalClose(page: Page, timeout = 5000): Promise<void> {
  await page.locator('.ant-modal-content').waitFor({ state: 'hidden', timeout })
}

/**
 * Wait for Ant Design table to load data
 */
export async function waitForTable(page: Page, timeout = 10000): Promise<Locator> {
  const table = page.locator('.ant-table-tbody')
  await table.waitFor({ state: 'visible', timeout })
  // Wait for spinner to disappear
  await page.locator('.ant-spin-spinning').waitFor({ state: 'hidden', timeout }).catch(() => {})
  return table
}

/**
 * Wait for loading spinner to disappear
 */
export async function waitForLoading(page: Page, timeout = 15000): Promise<void> {
  await page.waitForLoadState('networkidle')
  await page.locator('.ant-spin-spinning').waitFor({ state: 'hidden', timeout }).catch(() => {})
}

/**
 * Wait for success message
 */
export async function waitForSuccessMessage(page: Page, timeout = 5000): Promise<void> {
  await page.locator('.ant-message-success').waitFor({ state: 'visible', timeout })
}

/**
 * Wait for error message
 */
export async function waitForErrorMessage(page: Page, timeout = 5000): Promise<void> {
  await page.locator('.ant-message-error').waitFor({ state: 'visible', timeout })
}

/**
 * Fill an Ant Design input by name attribute
 */
export async function fillInput(page: Page, name: string, value: string): Promise<void> {
  const input = page.locator(`input[name="${name}"], textarea[name="${name}"]`)
  await input.fill(value)
}

/**
 * Fill an Ant Design input by placeholder text
 */
export async function fillInputByPlaceholder(
  page: Page,
  placeholder: string,
  value: string
): Promise<void> {
  const input = page.locator(`input[placeholder="${placeholder}"]`)
  await input.fill(value)
}

/**
 * Select an option from Ant Design Select dropdown
 */
export async function selectOption(page: Page, selector: string, optionText: string): Promise<void> {
  // Click the select to open dropdown
  await page.locator(selector).click()
  // Wait for dropdown options to appear
  await page.locator('.ant-select-dropdown').waitFor({ state: 'visible' })
  // Click the option
  await page.locator(`.ant-select-item-option`).filter({ hasText: optionText }).click()
}

/**
 * Select an option from Ant Design Select by clicking the select first
 */
export async function selectFromDropdown(
  page: Page,
  selectLocator: Locator,
  optionText: string
): Promise<void> {
  await selectLocator.click()
  await page.locator('.ant-select-dropdown').waitFor({ state: 'visible' })
  await page.locator(`.ant-select-item-option`).filter({ hasText: optionText }).click()
}

/**
 * Click a button by its text content
 */
export async function clickButton(page: Page, buttonText: string): Promise<void> {
  await page.getByRole('button', { name: buttonText }).click()
}

/**
 * Click the submit/save button in a form
 */
export async function submitForm(page: Page): Promise<void> {
  // Try common submit button texts
  const submitButton = page
    .getByRole('button', { name: /save|submit|create|update|confirm/i })
    .first()
  await submitButton.click()
}

/**
 * Click a table row action button (edit, delete, view, etc.)
 */
export async function clickTableRowAction(
  page: Page,
  rowIndex: number,
  actionName: string
): Promise<void> {
  const row = page.locator('.ant-table-row').nth(rowIndex)
  const actionButton = row.getByRole('button', { name: actionName })
  if ((await actionButton.count()) > 0) {
    await actionButton.click()
  } else {
    // Try clicking a dropdown menu first
    const menuButton = row.locator('.ant-dropdown-trigger, [data-icon="more"]')
    if ((await menuButton.count()) > 0) {
      await menuButton.click()
      await page.locator('.ant-dropdown-menu-item').filter({ hasText: actionName }).click()
    }
  }
}

/**
 * Get the number of rows in an Ant Design table
 */
export async function getTableRowCount(page: Page): Promise<number> {
  return await page.locator('.ant-table-row').count()
}

/**
 * Check if an Ant Design checkbox/switch is checked
 */
export async function isChecked(locator: Locator): Promise<boolean> {
  const isSwitch = (await locator.locator('.ant-switch').count()) > 0
  if (isSwitch) {
    return (await locator.locator('.ant-switch-checked').count()) > 0
  }
  return await locator.locator('input[type="checkbox"]').isChecked()
}

/**
 * Toggle an Ant Design switch
 */
export async function toggleSwitch(page: Page, selector: string): Promise<void> {
  await page.locator(selector).locator('.ant-switch').click()
}

/**
 * Upload a file using Ant Design Upload component
 */
export async function uploadFile(page: Page, selector: string, filePath: string): Promise<void> {
  const fileInput = page.locator(`${selector} input[type="file"]`)
  await fileInput.setInputFiles(filePath)
}

/**
 * Confirm a delete modal by typing the confirmation text
 */
export async function confirmDelete(page: Page, confirmationText: string): Promise<void> {
  const modal = await waitForModal(page)
  await modal.locator('input').fill(confirmationText)
  await modal.getByRole('button', { name: /delete|confirm|yes/i }).click()
}

/**
 * Navigate to a workspace page
 */
export async function navigateToWorkspacePage(
  page: Page,
  workspaceId: string,
  pagePath: string
): Promise<void> {
  await page.goto(`/console/workspace/${workspaceId}/${pagePath}`)
  await waitForLoading(page)
}

/**
 * Get text content from a specific table cell
 */
export async function getTableCellText(
  page: Page,
  rowIndex: number,
  columnIndex: number
): Promise<string> {
  const cell = page.locator('.ant-table-row').nth(rowIndex).locator('td').nth(columnIndex)
  return (await cell.textContent()) || ''
}

/**
 * Search in an Ant Design table by filling the search input
 */
export async function searchInTable(page: Page, searchText: string): Promise<void> {
  const searchInput = page.locator('input[placeholder*="Search"], input[placeholder*="search"]')
  await searchInput.fill(searchText)
  await searchInput.press('Enter')
  await waitForLoading(page)
}

/**
 * Clear all filters in a form
 */
export async function clearFilters(page: Page): Promise<void> {
  const clearButton = page.getByRole('button', { name: /clear|reset/i })
  if ((await clearButton.count()) > 0) {
    await clearButton.click()
    await waitForLoading(page)
  }
}

/**
 * Check if page has no data (empty state)
 */
export async function hasEmptyState(page: Page): Promise<boolean> {
  const emptyState = page.locator('.ant-empty, .ant-table-empty')
  return (await emptyState.count()) > 0
}
