import type { Page, Route } from '@playwright/test'
import { test, expect } from '../fixtures/auth'
import {
  baseBody,
  librarySavedBlocks,
  openLibraryEditor,
  workspaceWithBlocks
} from '../fixtures/library'
import { mockUserMeResponse } from '../fixtures/mock-data'

// Managing saved blocks from the library's cards: rename, replace with the selected block, duplicate and delete,
// with real clicks. The API keeps the saved blocks it is sent, so the library shows them once refreshed, and
// each request's body is recorded.

type Saved = (typeof librarySavedBlocks)[number]

const json = (route: Route, data: unknown) =>
  route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(data) })

// The workspace's saved blocks as the API holds them; registered after the editor opened, these routes take over
const libraryStore = async (page: Page) => {
  let blocks: Saved[] = JSON.parse(JSON.stringify(librarySavedBlocks))
  const calls: { endpoint: string; body: Record<string, unknown> }[] = []
  const api = 'https://localapi.notifuse.com:4000/api'
  await page.route(`${api}/user.me*`, (route) =>
    json(route, { ...mockUserMeResponse, workspaces: [workspaceWithBlocks(blocks)] })
  )
  await page.route(`${api}/workspace.get*`, (route) =>
    json(route, { workspace: workspaceWithBlocks(blocks) })
  )
  const now = () => new Date().toISOString()
  await page.route(`${api}/templateBlocks.update*`, (route) => {
    const body = route.request().postDataJSON()
    calls.push({ endpoint: 'update', body })
    blocks = blocks.map((b) =>
      b.id === body.id ? { ...b, name: body.name, block: body.block, updated: now() } : b
    )
    return json(route, { block: blocks.find((b) => b.id === body.id) })
  })
  await page.route(`${api}/templateBlocks.create*`, (route) => {
    const body = route.request().postDataJSON()
    calls.push({ endpoint: 'create', body })
    const created = {
      id: 'copy-id',
      name: body.name,
      block: body.block,
      created: now(),
      updated: now()
    }
    blocks = [...blocks, created]
    return json(route, { block: created })
  })
  await page.route(`${api}/templateBlocks.delete*`, (route) => {
    const body = route.request().postDataJSON()
    calls.push({ endpoint: 'delete', body })
    blocks = blocks.filter((b) => b.id !== body.id)
    return json(route, { success: true })
  })
  return calls
}

const openManaged = async (page: Page, options: Parameters<typeof openLibraryEditor>[2] = {}) => {
  const editor = await openLibraryEditor(page, baseBody(), options)
  const calls = await libraryStore(page)
  const card = (key: string) => editor.drawer.locator(`[data-library-card="${key}"]`)
  // The menu of a card; Ant Design keeps closed menus in the page, hidden
  const menu = async (key: string, name: string) => {
    await card(key).hover()
    await card(key)
      .getByRole('button', { name: `More actions for ${name}` })
      .click()
    return page.locator('.ant-dropdown:not(.ant-dropdown-hidden)')
  }
  // Selects a block from the Structure tab, then comes back to the library
  const select = async (label: RegExp) => {
    await editor.drawer.getByText('Structure', { exact: true }).click()
    await editor.drawer.locator('.ant-tree-title', { hasText: label }).first().click()
    await editor.drawer.getByTestId('library-tab').click()
  }
  return { ...editor, calls, card, menu, select }
}

test.describe('Managing saved blocks from the library', () => {
  test('renames a block with Enter, sending its stored block', async ({
    authenticatedPage: page
  }) => {
    const editor = await openManaged(page)
    await (
      await editor.menu('lib-text', 'Signature')
    )
      .getByRole('menuitem', { name: 'Rename' })
      .click()
    const input = editor.card('lib-text').getByRole('textbox', { name: 'Block name' })
    await expect(input).toBeFocused()
    await input.fill('Footer signature')
    await input.press('Enter')

    await expect.poll(() => editor.calls.length).toBe(1)
    expect(editor.calls[0]).toMatchObject({
      endpoint: 'update',
      body: {
        workspace_id: 'test-workspace',
        id: 'lib-text',
        name: 'Footer signature',
        block: { id: 'lt', type: 'mj-text' }
      }
    })
    await expect(editor.card('lib-text')).toContainText('Footer signature')
    await expect(page.getByText('Block renamed to "Footer signature"')).toBeVisible()
  })

  test('Escape cancels a rename', async ({ authenticatedPage: page }) => {
    const editor = await openManaged(page)
    await (
      await editor.menu('lib-text', 'Signature')
    )
      .getByRole('menuitem', { name: 'Rename' })
      .click()
    const input = editor.card('lib-text').getByRole('textbox', { name: 'Block name' })
    await input.fill('Something else')
    await input.press('Escape')
    await expect(input).toBeHidden()
    await expect(editor.card('lib-text')).toContainText('Signature')
    // The editor drawer stays open: Escape only left the name
    await expect(editor.drawer).toBeVisible()
    expect(editor.calls).toHaveLength(0)
  })

  test('duplicates a block, tagging the copy "New"', async ({ authenticatedPage: page }) => {
    const editor = await openManaged(page)
    await (
      await editor.menu('lib-button', 'Shop button')
    )
      .getByRole('menuitem', { name: 'Duplicate' })
      .click()

    await expect.poll(() => editor.calls.length).toBe(1)
    expect(editor.calls[0]).toMatchObject({
      endpoint: 'create',
      body: { name: 'Shop button (copy)', block: { type: 'mj-button', content: 'Lib button' } }
    })
    const copy = editor.card('copy-id')
    await expect(copy).toContainText('Shop button (copy)')
    await expect(copy.getByText('New', { exact: true })).toBeVisible()
  })

  test('replaces a saved section with the selected section, after confirming', async ({
    authenticatedPage: page
  }) => {
    const editor = await openManaged(page)
    await editor.select(/^Section$/)

    // A text cannot take the place of the selected section
    const textMenu = await editor.menu('lib-text', 'Signature')
    const refused = textMenu.getByRole('menuitem', { name: /Replace with the selected block/ })
    await expect(refused).toHaveAttribute('aria-disabled', 'true')
    await expect(refused).toContainText('Select a block of the same type')
    await page.keyboard.press('Escape')
    await expect(page.locator('.ant-dropdown:not(.ant-dropdown-hidden)')).toHaveCount(0)

    await (
      await editor.menu('lib-section', 'Promo section')
    )
      .getByRole('menuitem', { name: /Replace with the selected block/ })
      .click()
    const confirm = page.locator('.ant-modal-confirm')
    await expect(confirm).toContainText('Replace "Promo section" with the selected block?')
    await expect(confirm).toContainText('Templates that already contain this block do not change.')
    await confirm.getByRole('button', { name: 'Replace' }).click()

    await expect.poll(() => editor.calls.length).toBe(1)
    const sent = editor.calls[0]
    expect(sent).toMatchObject({
      endpoint: 'update',
      body: { id: 'lib-section', name: 'Promo section' }
    })
    expect(JSON.stringify(sent.body.block)).toContain('Alpha')
    await expect(editor.card('lib-section').getByText('New', { exact: true })).toHaveCount(0)
  })

  test('deletes a block after confirming, and Cancel sends nothing', async ({
    authenticatedPage: page
  }) => {
    const editor = await openManaged(page)
    await (
      await editor.menu('lib-text', 'Signature')
    )
      .getByRole('menuitem', { name: 'Delete' })
      .click()
    const confirm = page.locator('.ant-modal-confirm')
    await expect(confirm).toContainText('Delete "Signature" from the library?')
    await confirm.getByRole('button', { name: 'Cancel' }).click()
    await expect(confirm).toBeHidden()
    expect(editor.calls).toHaveLength(0)

    await (
      await editor.menu('lib-text', 'Signature')
    )
      .getByRole('menuitem', { name: 'Delete' })
      .click()
    await page.locator('.ant-modal-confirm').getByRole('button', { name: 'Delete' }).click()
    await expect.poll(() => editor.calls.length).toBe(1)
    expect(editor.calls[0]).toMatchObject({
      endpoint: 'delete',
      body: { workspace_id: 'test-workspace', id: 'lib-text' }
    })
    await expect(editor.card('lib-text')).toHaveCount(0)
    await expect(editor.drawer.getByTestId('library-count')).toHaveText(
      String(librarySavedBlocks.length - 1)
    )
  })

  test("confirms in the console's language", async ({ authenticatedPage: page }) => {
    await page.addInitScript(() => localStorage.setItem('locale', 'fr'))
    const editor = await openManaged(page, {
      labels: { drawer: 'Modifier le modèle d email', templateTab: '2. Modèle' }
    })
    const card = editor.card('lib-text')
    await card.hover()
    await card.getByRole('button', { name: 'Plus d’actions pour Signature' }).click()
    await page
      .locator('.ant-dropdown:not(.ant-dropdown-hidden)')
      .getByRole('menuitem', { name: 'Supprimer' })
      .click()
    const confirm = page.locator('.ant-modal-confirm')
    await expect(confirm).toContainText('Supprimer « Signature » de la bibliothèque ?')
    await expect(confirm.getByRole('button', { name: 'Annuler' })).toBeVisible()
    await confirm.getByRole('button', { name: 'Supprimer' }).click()
    await expect.poll(() => editor.calls.length).toBe(1)
    expect(editor.calls[0]).toMatchObject({ endpoint: 'delete', body: { id: 'lib-text' } })
  })
})
