import { test, expect } from '../fixtures/auth'
import type { Page } from '@playwright/test'
import { mockApi, WORKSPACE_ID } from '../fixtures/template-editor'
import { mockUser, mockWorkspace } from '../fixtures/mock-data'

// A workspace whose files live in an S3-compatible bucket, answered by the routes below.
const workspace = {
  ...mockWorkspace,
  id: WORKSPACE_ID,
  settings: {
    ...(mockWorkspace as { settings: object }).settings,
    file_manager: {
      endpoint: 'https://s3.example.com',
      bucket: 'media',
      access_key: 'key',
      secret_key: 'secret',
      region: 'us-east-1',
      cdn_endpoint: 'https://cdn.example.com',
      force_path_style: true
    }
  }
}

const OBJECTS = [
  { key: 'newsletters/', size: 0 },
  { key: 'newsletters/hero.png', size: 2048 },
  { key: 'newsletters/pricing.pdf', size: 1024 },
  { key: 'newsletters/archive/old.png', size: 512 },
  { key: 'logo.png', size: 300 }
]

const cors = {
  'access-control-allow-origin': '*',
  'access-control-allow-headers': '*',
  'access-control-allow-methods': 'GET,PUT,POST,DELETE,HEAD',
  'access-control-expose-headers': '*'
}

// Answers ListObjectsV2 from OBJECTS and records the deletes; serves every file as a small image.
async function mockStorage(page: Page) {
  const deleted: string[] = []
  await page.route('https://s3.example.com/**', async (route) => {
    const request = route.request()
    if (request.method() === 'OPTIONS') return route.fulfill({ status: 200, headers: cors })
    const url = new URL(request.url())
    if (request.method() === 'DELETE') {
      deleted.push(decodeURIComponent(url.pathname.replace('/media/', '')))
      return route.fulfill({ status: 204, headers: cors })
    }
    const prefix = url.searchParams.get('prefix') ?? ''
    const contents = OBJECTS.filter(
      (object) => object.key.startsWith(prefix) && !deleted.includes(object.key)
    )
      .map(
        (object) =>
          `<Contents><Key>${object.key}</Key><LastModified>2026-11-01T10:00:00.000Z</LastModified><Size>${object.size}</Size></Contents>`
      )
      .join('')
    return route.fulfill({
      status: 200,
      headers: { ...cors, 'content-type': 'application/xml' },
      body: `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>media</Name><Prefix>${prefix}</Prefix><IsTruncated>false</IsTruncated>${contents}</ListBucketResult>`
    })
  })
  await page.route('https://cdn.example.com/**', (route) =>
    route.fulfill({
      status: 200,
      headers: { 'content-type': 'image/svg+xml' },
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="600"><rect width="100%" height="100%" fill="#6a5af9"/></svg>'
    })
  )
  return { deleted }
}

test.describe('File manager', () => {
  test.beforeEach(async ({ authenticatedPageWithData: page }) => {
    await mockApi(page, 'user.me', { user: mockUser, workspaces: [workspace] })
  })

  test('opens a folder from the tree and previews a file from the URL', async ({
    authenticatedPageWithData: page
  }) => {
    await mockStorage(page)
    await page.goto(`/console/workspace/${WORKSPACE_ID}/file-manager`)

    const folders = page.getByRole('navigation', { name: 'Folders' })
    await expect(folders.getByRole('button', { name: 'Root 4' })).toBeVisible()
    await folders.getByRole('button', { name: /^newsletters/ }).click()

    await expect(page).toHaveURL(/path=newsletters%2F/)
    await expect(page.getByRole('heading', { level: 1, name: 'newsletters' })).toBeVisible()
    await expect(page.getByRole('row', { name: /archive/ })).toContainText('1 file')

    await page.getByText('hero.png').click()
    await expect(page).toHaveURL(/file=newsletters%2Fhero\.png/)
    // The preview reads the image's size once it has loaded.
    await expect(page.locator('[data-file-preview]')).toContainText('1200 × 600')

    await page.reload()
    await expect(page.locator('[data-file-preview]')).toContainText('hero.png')
  })

  test('searches the whole bucket by file name', async ({ authenticatedPageWithData: page }) => {
    await mockStorage(page)
    await page.goto(`/console/workspace/${WORKSPACE_ID}/file-manager`)
    await expect(page.getByText('logo.png')).toBeVisible()

    await page.getByRole('textbox', { name: 'File name' }).fill('png')
    await expect(page.getByRole('heading', { level: 1, name: 'Search results' })).toBeVisible()
    await expect(page.getByRole('row', { name: /old\.png/ })).toContainText('newsletters/archive/')
  })

  test('deletes a folder with its files and goes back up', async ({
    authenticatedPageWithData: page
  }) => {
    const storage = await mockStorage(page)
    await page.goto(`/console/workspace/${WORKSPACE_ID}/file-manager?path=newsletters%2F`)
    await expect(page.getByText('hero.png')).toBeVisible()

    await page.getByRole('button', { name: 'More actions' }).click()
    await page.getByRole('menuitem', { name: 'Delete folder' }).click()
    const dialog = page.getByRole('dialog', { name: 'Delete folder' })
    await expect(dialog).toContainText('Delete the folder newsletters and its 3 files?')
    await dialog.getByRole('button', { name: 'Delete folder' }).click()

    await expect(page.getByText('Folder deleted successfully.')).toBeVisible()
    await expect(page).not.toHaveURL(/path=/)
    expect(storage.deleted.sort()).toEqual([
      'newsletters/',
      'newsletters/archive/old.png',
      'newsletters/hero.png',
      'newsletters/pricing.pdf'
    ])
  })
})
