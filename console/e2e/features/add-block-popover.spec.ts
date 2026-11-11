import type { Page } from '@playwright/test'
import { test, expect } from '../fixtures/auth'
import { baseBody, librarySavedBlocks, openLibraryEditor } from '../fixtures/library'

// The "+" menu of the Structure tab: its Saved blocks tab lists every saved block, those that can go at that place
// first, the others greyed with the reason. Inserting one is checked in the tree sent on save.

const openStructure = async (page: Page, savedBlocks: unknown[] = librarySavedBlocks) => {
  const editor = await openLibraryEditor(page, baseBody(), { savedBlocks })
  await editor.drawer.getByText('Structure', { exact: true }).click()
  // The "+" of a block of the tree
  const plus = (id: string) => editor.drawer.locator(`[data-tree-block-id="${id}"] .border-dashed`)
  const popover = page.locator('.add-block-popover:not(.ant-popover-hidden)')
  const open = async (id: string) => {
    await plus(id).scrollIntoViewIfNeeded()
    await plus(id).click()
    await expect(popover).toBeVisible()
    return popover
  }
  const row = (id: string) => popover.locator(`[data-saved-row="${id}"]`)
  return { ...editor, open, popover, row }
}

test.describe('The saved blocks of the "+" menu', () => {
  test('lists every saved block in a column, and says why some cannot go there', async ({
    authenticatedPage: page
  }) => {
    const editor = await openStructure(page)
    const popover = await editor.open('C1')
    const tab = popover.getByRole('tab', { name: /Saved blocks/ })
    await expect(tab).toHaveText(`Saved blocks (${librarySavedBlocks.length})`)
    await tab.click()

    // In order, by their ids: a row's text holds its thumbnail's
    const ids = (group: string) =>
      popover
        .getByTestId(group)
        .locator('[data-saved-row]')
        .evaluateAll((rows) => rows.map((r) => r.getAttribute('data-saved-row')))
    await expect
      .poll(() => ids('saved-available'))
      .toEqual(['lib-text', 'lib-button', 'lib-raw', 'lib-liquid'])
    expect(await ids('saved-unavailable')).toEqual([
      'lib-section',
      'lib-section-2col',
      'lib-column',
      'lib-wrapper'
    ])
    await expect(popover.getByText('Not available here')).toBeVisible()
    await expect(editor.row('lib-column')).toHaveAttribute('aria-disabled', 'true')
    await expect(editor.row('lib-column')).toContainText('Goes inside a section')
    await expect(editor.row('lib-section')).toContainText('Not allowed here')

    // A greyed block does nothing when clicked
    await editor.row('lib-section').click()
    await expect(popover).toBeVisible()

    await editor.row('lib-text').hover()
    const preview = popover.getByTestId('saved-preview')
    await expect(preview).toContainText('Signature')
    await expect(preview).toContainText('Text')
    await expect(preview).toContainText(
      'A copy is inserted. Later changes to the saved block do not change it.'
    )
    await expect(preview.getByTestId('block-thumb')).toBeVisible()
    await preview.getByRole('button', { name: 'Add Signature' }).click()
    await expect(popover).toBeHidden()

    const body = await editor.saved()
    const column = body.children![0].children![0]
    expect(column.id).toBe('C1')
    expect(column.children!.map((c) => c.type)).toEqual(['mj-text', 'mj-button', 'mj-text'])
    expect(JSON.stringify(column.children![2])).toContain('Lib text')
  })

  test('refuses a Liquid block in a column shown under a condition', async ({
    authenticatedPage: page
  }) => {
    const editor = await openStructure(page)
    const popover = await editor.open('CC')
    await popover.getByRole('tab', { name: /Saved blocks/ }).click()
    await expect(editor.row('lib-liquid')).toHaveAttribute('aria-disabled', 'true')
    await expect(editor.row('lib-liquid')).toContainText('Not allowed inside a conditional block')
    await expect(editor.row('lib-text')).not.toHaveAttribute('aria-disabled', 'true')
  })

  test('opens on the tab used last, in another "+"', async ({ authenticatedPage: page }) => {
    // Without the raw HTML block: its thumbnail is an iframe of the console's origin, where the test's start-up
    // script runs again and clears the storage
    const editor = await openStructure(
      page,
      librarySavedBlocks.filter((saved) => saved.block.type !== 'mj-raw')
    )
    let popover = await editor.open('C1')
    await popover.getByRole('tab', { name: /Saved blocks/ }).click()
    // A click outside closes the menu
    await editor.drawer.getByText('Structure', { exact: true }).click()
    await expect(editor.popover).toBeHidden()
    expect(
      await page.evaluate((key) => localStorage.getItem(key), 'notifuse.addBlockPopover.tab')
    ).toBe('saved')

    popover = await editor.open('C2')
    await expect(popover.getByRole('tab', { name: /Saved blocks/ })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  })
})
