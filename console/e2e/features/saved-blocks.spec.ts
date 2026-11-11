import type { Page, Route } from '@playwright/test'
import { test, expect } from '../fixtures/auth'
import { mockApi, openTemplateEditor } from '../fixtures/template-editor'
import { mockTemplates, mockUserMeResponse, mockWorkspace } from '../fixtures/mock-data'

// A white section, and a saved "Header" whose attributes differ from it only in a long-hand padding: the shape
// the save dialog used to take for an already saved block, offering to overwrite it
const section = {
  id: 'section-products',
  type: 'mj-section',
  attributes: { backgroundColor: '#ffffff', paddingTop: '8px', paddingBottom: '8px' },
  children: [
    {
      id: 'column-products',
      type: 'mj-column',
      children: [{ id: 'text-products', type: 'mj-text', content: '<p>Our products</p>' }]
    }
  ]
}

const template = {
  ...mockTemplates[0],
  email: {
    ...mockTemplates[0].email,
    visual_editor_tree: {
      id: 'mjml-products',
      type: 'mjml',
      children: [{ id: 'body-products', type: 'mj-body', children: [section] }]
    }
  }
}

const savedBlocks = [
  {
    id: 'header-id',
    name: 'Header',
    block: {
      id: 'section-header',
      type: 'mj-section',
      attributes: { backgroundColor: '#ffffff', paddingTop: '24px', paddingBottom: '8px' },
      children: []
    },
    created: '2026-09-01T00:00:00Z',
    updated: '2026-09-01T00:00:00Z'
  },
  {
    id: 'cta-id',
    name: 'CTA',
    block: { id: 'button-cta', type: 'mj-button', attributes: {}, content: 'Shop' },
    created: '2026-10-01T00:00:00Z',
    updated: '2026-10-01T00:00:00Z'
  }
]

const workspace = {
  ...mockWorkspace,
  settings: { ...mockWorkspace.settings, template_blocks: savedBlocks }
}

// The shared mocks answer unknown endpoints with an empty success and capture no template block request, so the
// spec answers and records those itself
const recordBlockSaves = async (page: Page, createStatus = 200) => {
  const calls: { endpoint: string; body: Record<string, unknown> }[] = []
  const answer = (endpoint: string, status: number) => async (route: Route) => {
    calls.push({ endpoint, body: route.request().postDataJSON() })
    await route.fulfill({
      status,
      contentType: 'application/json',
      body: JSON.stringify(
        status === 200
          ? { block: { id: `${endpoint}d-id` } }
          : { error: 'Saving failed on the server' }
      )
    })
  }
  await page.route(
    'https://localapi.notifuse.com:4000/api/templateBlocks.create*',
    answer('create', createStatus)
  )
  await page.route(
    'https://localapi.notifuse.com:4000/api/templateBlocks.update*',
    answer('update', 200)
  )
  return calls
}

const openSaveDialog = async (page: Page) => {
  await mockApi(page, 'user.me', { ...mockUserMeResponse, workspaces: [workspace] })
  await mockApi(page, 'workspace.get', { workspace })
  const drawer = await openTemplateEditor(page, template)
  await drawer
    .locator('.ant-tree-title', { hasText: /^Section$/ })
    .first()
    .click()
  await drawer.locator('button:has(svg[data-icon="floppy-disk"])').click()
  const dialog = page.getByRole('dialog', { name: 'Save to library' })
  await expect(dialog).toBeVisible()
  return { dialog, drawer }
}

test.describe('Saving a block from the email editor', () => {
  test('saves a look-alike section as a new block', async ({ authenticatedPage: page }) => {
    const calls = await recordBlockSaves(page)
    const { dialog } = await openSaveDialog(page)

    await expect(dialog.getByRole('radio', { name: 'New block' })).toBeChecked()
    await expect(dialog.getByTestId('save-block-type')).toHaveText('Section')
    // A name is suggested from the block's text
    await expect(dialog.getByPlaceholder('Enter a name for this block')).toHaveValue(
      'Section · Our products'
    )
    await dialog.getByPlaceholder('Enter a name for this block').fill('Products')
    await dialog.getByRole('button', { name: 'Save' }).click()

    await expect(dialog).toBeHidden()
    expect(calls).toHaveLength(1)
    expect(calls[0].endpoint).toBe('create')
    expect(calls[0].body).toMatchObject({
      workspace_id: 'test-workspace',
      name: 'Products',
      block: { type: 'mj-section' }
    })
    await expect(page.locator('.ant-message-notice-success')).toHaveCount(1)
  })

  test('offers only saved sections when replacing a section', async ({
    authenticatedPage: page
  }) => {
    const calls = await recordBlockSaves(page)
    const { dialog } = await openSaveDialog(page)

    await dialog.getByRole('radio', { name: 'Replace an existing block (1)' }).click()
    await dialog.getByRole('combobox').click()

    const options = page.locator('.ant-select-item-option-content')
    await expect(options).toHaveText(['Header'])
    await options.first().click()
    await dialog.getByRole('button', { name: 'Replace' }).click()
    await expect(dialog).toBeHidden()
    expect(calls).toMatchObject([{ endpoint: 'update', body: { id: 'header-id', name: 'Header' } }])
  })

  test('shows a new block in the library, highlighted', async ({ authenticatedPage: page }) => {
    await recordBlockSaves(page)
    const { dialog, drawer } = await openSaveDialog(page)
    // After the save, the workspace is read again with the new block
    const refreshed = {
      ...workspace,
      settings: {
        ...workspace.settings,
        template_blocks: [
          ...savedBlocks,
          {
            id: 'created-id',
            name: 'Products',
            block: section,
            created: '2026-11-01T00:00:00Z',
            updated: '2026-11-01T00:00:00Z'
          }
        ]
      }
    }
    await mockApi(page, 'user.me', { ...mockUserMeResponse, workspaces: [refreshed] })
    await mockApi(page, 'workspace.get', { workspace: refreshed })

    await dialog.getByPlaceholder('Enter a name for this block').fill('Products')
    await dialog.getByRole('button', { name: 'Save' }).click()
    await expect(dialog).toBeHidden()

    await expect(drawer.getByTestId('library-panel')).toBeVisible()
    const card = drawer.locator('[data-library-card="created-id"]')
    await expect(card).toBeVisible()
    await expect(card.getByText('New', { exact: true })).toBeVisible()
    await expect(
      drawer.locator('[data-library-card="header-id"]').getByText('New', { exact: true })
    ).toHaveCount(0)
  })

  test('keeps the dialog open when saving fails', async ({ authenticatedPage: page }) => {
    const calls = await recordBlockSaves(page, 500)
    const { dialog } = await openSaveDialog(page)

    await dialog.getByPlaceholder('Enter a name for this block').fill('Products')
    await dialog.getByRole('button', { name: 'Save' }).click()

    await expect(page.locator('.ant-message-notice-error')).toHaveCount(1)
    // Read once, while the error shows: a success toast would still be there
    expect(await page.locator('.ant-message-notice-success').count()).toBe(0)
    expect(calls).toHaveLength(1)

    // The dialog is still open with the typed name, so saving again sends a second request. A dialog that had
    // closed would only be animating away by now, its Save button gone.
    await page.waitForTimeout(1000)
    await expect(dialog.getByPlaceholder('Enter a name for this block')).toHaveValue('Products')
    await dialog.getByRole('button', { name: 'Save' }).click()
    await expect.poll(() => calls.length).toBe(2)
  })
})
