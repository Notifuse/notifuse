import { test, expect } from '../fixtures/auth'
import type { Locator, Page } from '@playwright/test'
import { waitForLoading } from '../fixtures/test-utils'
import { testBlogPostData, testSEOData } from '../fixtures/form-data'

const WORKSPACE_ID = 'test-workspace'

// antd 6 renamed the inner wrapper of both overlays (.ant-drawer-content became
// .ant-drawer-section, .ant-modal-content became .ant-modal-container), so they are
// addressed here by role and accessible name - which is their title - instead.
function drawer(page: Page, title: string): Locator {
  return page.getByRole('dialog', { name: title, exact: true })
}

async function openCreatePostDrawer(page: Page): Promise<Locator> {
  // The button carries a plus icon whose aria-label joins the accessible name,
  // so the match is on the substring rather than the exact string.
  await page.getByRole('button', { name: 'New Post' }).click()
  const postDrawer = drawer(page, 'Create New Post')
  await expect(postDrawer).toBeVisible()
  return postDrawer
}

/**
 * Pick an option in an antd Select. The role="option" nodes live in a zero-sized
 * a11y-only listbox; the clickable ones are the .ant-select-item-option rows of the
 * visible dropdown, so those are what gets clicked.
 */
async function selectOption(
  page: Page,
  scope: Locator,
  fieldId: string,
  optionText: string
): Promise<void> {
  await scope
    .locator('.ant-form-item')
    .filter({ has: page.locator(`#${fieldId}`) })
    .locator('.ant-select')
    .click()
  // Each dropdown carries the a11y listbox of its own field, which tells them apart
  // when a previously opened one is still on screen.
  const dropdown = page.locator('.ant-select-dropdown').filter({
    has: page.locator(`#${fieldId}_list`)
  })
  await dropdown.waitFor({ state: 'visible' })
  await dropdown.locator('.ant-select-item-option').filter({ hasText: optionText }).click()
}

/** Fill every field the post form requires, so the form validates and submits. */
async function fillRequiredPostFields(
  page: Page,
  postDrawer: Locator,
  title: string,
  authorName: string
): Promise<void> {
  await postDrawer.locator('input[placeholder="Post title"]').fill(title)
  await selectOption(page, postDrawer, 'category_id', 'Engineering')
  await postDrawer.locator('#reading_time_minutes').fill('7')

  await postDrawer.getByRole('button', { name: 'Add Author' }).click()
  const authorModal = drawer(page, 'Add Author')
  await expect(authorModal).toBeVisible()
  await authorModal.locator('#name').fill(authorName)
  await authorModal.getByRole('button', { name: 'Add', exact: true }).click()
  await expect(authorModal).toBeHidden()
}

/** Fill the whole SEO block. Ids come from the form path, e.g. seo -> meta_title. */
async function fillSEOFields(
  page: Page,
  postDrawer: Locator,
  seo: {
    meta_title: string
    meta_description: string
    keywords: string[]
    canonical_url: string
    og_title: string
    og_description: string
    og_image: string
  }
): Promise<void> {
  await postDrawer.locator('#seo_meta_title').fill(seo.meta_title)
  await postDrawer.locator('#seo_meta_description').fill(seo.meta_description)
  for (const keyword of seo.keywords) {
    await postDrawer.locator('#seo_keywords').fill(keyword)
    await postDrawer.locator('#seo_keywords').press('Enter')
  }
  // Close the keyword dropdown so it stops covering the fields below it
  await postDrawer.locator('#seo_keywords').press('Escape')
  await selectOption(page, postDrawer, 'seo_meta_robots', 'Index and follow links')
  await postDrawer.locator('#seo_canonical_url').fill(seo.canonical_url)
  await postDrawer.locator('#seo_og_title').fill(seo.og_title)
  await postDrawer.locator('#seo_og_description').fill(seo.og_description)
  // ImageURLInput does not forward the Form.Item id to its Input, so this one is
  // reached through its form item instead.
  await postDrawer
    .locator('.ant-form-item')
    .filter({ hasText: 'Social Share Image URL' })
    .locator('input')
    .fill(seo.og_image)
}

test.describe('Blog Feature', () => {
  test.describe('Page Load', () => {
    test('loads blog page', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      await expect(page).toHaveURL(/blog/)
      // Category rail on the left, posts on the right.
      await expect(page.getByText('Categories', { exact: true })).toBeVisible()
      await expect(page.getByRole('heading', { name: 'All Posts' })).toBeVisible()
      await expect(page.getByRole('button', { name: 'New Post' })).toBeVisible()
    })

    test('loads blog page with posts', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      await expect(page).toHaveURL(/blog/)
      // The four mocked posts, with their slugs and categories.
      await expect(page.locator('.ant-table-row')).toHaveCount(4)
      await expect(
        page.locator('.ant-table-row').filter({ hasText: 'Getting Started with Email Marketing' })
      ).toContainText('getting-started-email-marketing')
      await expect(
        page.locator('.ant-table-row').filter({ hasText: 'New Feature: A/B Testing' })
      ).toContainText('Product Updates')
    })
  })

  test.describe('Blog Posts CRUD', () => {
    test('opens create post form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      const postDrawer = await openCreatePostDrawer(page)

      await expect(postDrawer.locator('input[placeholder="Post title"]')).toBeVisible()
      await expect(postDrawer.getByRole('button', { name: 'Create', exact: true })).toBeVisible()
    })

    test('fills blog post form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      const postDrawer = await openCreatePostDrawer(page)

      // Fill post title (required)
      const titleInput = postDrawer.locator('input[placeholder="Post title"]')
      await titleInput.fill('Test Blog Post Title')
      await expect(titleInput).toHaveValue('Test Blog Post Title')

      // Slug is derived from the title
      await expect(postDrawer.locator('input[placeholder="post-slug"]')).toHaveValue(
        'test-blog-post-title'
      )

      // Fill excerpt (optional)
      const excerptInput = postDrawer.locator('#excerpt')
      await excerptInput.fill('This is a test blog post excerpt')
      await expect(excerptInput).toHaveValue('This is a test blog post excerpt')

      await expect(postDrawer.getByRole('button', { name: 'Create', exact: true })).toBeVisible()
    })

    test('views post details', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      // The row actions are icon-only buttons; the pencil is the edit one.
      const row = page.locator('.ant-table-row').filter({ hasText: 'Draft Post' })
      await row
        .getByRole('button')
        .filter({ has: page.locator('[data-icon="pen-to-square"]') })
        .click()

      const editDrawer = drawer(page, 'Edit Post')
      await expect(editDrawer).toBeVisible()
      await expect(editDrawer.locator('input[placeholder="Post title"]')).toHaveValue('Draft Post')
      await expect(editDrawer.locator('input[placeholder="post-slug"]')).toHaveValue('draft-post')
    })
  })

  test.describe('Blog Categories', () => {
    test('shows category management', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      // Every mocked category is listed in the rail, next to the create entry point.
      const rail = page.locator('.ant-menu').filter({ hasText: 'All Posts' })
      await expect(rail.locator('.ant-menu-item').filter({ hasText: 'Engineering' })).toBeVisible()
      await expect(
        rail.locator('.ant-menu-item').filter({ hasText: 'Product Updates' })
      ).toBeVisible()
      await expect(rail.locator('.ant-menu-item').filter({ hasText: 'Company News' })).toBeVisible()
      await expect(page.getByRole('button', { name: 'New Category' })).toBeVisible()

      // Picking a category scopes the list to it
      await rail.locator('.ant-menu-item').filter({ hasText: 'Engineering' }).click()
      await expect(page).toHaveURL(/category_id=cat-1/)
      await expect(page.getByRole('heading', { name: 'Engineering' })).toBeVisible()
    })
  })

  test.describe('Post Status', () => {
    test('displays post status', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      // A post with a past published_at reads as published, one without as a draft.
      await expect(
        page
          .locator('.ant-table-row')
          .filter({ hasText: 'Getting Started with Email Marketing' })
          .locator('.ant-tag')
      ).toHaveText('Published')
      await expect(
        page.locator('.ant-table-row').filter({ hasText: 'Draft Post' }).locator('.ant-tag')
      ).toHaveText('Draft')
    })

    test('shows draft posts', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      // The filter is what the page owns: it re-queries with the status and records it
      // in the URL. The mocked backend answers every status with the same four posts,
      // so the rows themselves prove nothing here.
      const listRequest = page.waitForRequest(
        (request) =>
          request.url().includes('/api/blogPosts.list') && request.url().includes('status=draft')
      )
      await page.locator('.ant-segmented-item').filter({ hasText: 'Drafts' }).click()
      await listRequest

      await expect(page).toHaveURL(/status=draft/)
      await expect(page.locator('.ant-segmented-item-selected')).toHaveText('Drafts')
    })

    test('shows published posts', async ({ authenticatedPageWithData }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      const listRequest = page.waitForRequest(
        (request) =>
          request.url().includes('/api/blogPosts.list') && request.url().includes('status=published')
      )
      await page.locator('.ant-segmented-item').filter({ hasText: 'Published' }).click()
      await listRequest

      await expect(page).toHaveURL(/status=published/)
      await expect(page.locator('.ant-segmented-item-selected')).toHaveText('Published')
    })
  })

  test.describe('Rich Editor', () => {
    test('shows post editor', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      const postDrawer = await openCreatePostDrawer(page)

      const editor = postDrawer.locator('.ProseMirror')
      await expect(editor).toBeVisible()
      await expect(editor).toHaveAttribute('contenteditable', 'true')
    })
  })

  test.describe('Form Validation', () => {
    test('requires post title', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      const postDrawer = await openCreatePostDrawer(page)

      // Submit an untouched form
      await postDrawer.getByRole('button', { name: 'Create', exact: true }).click()

      await expect(postDrawer.getByText('Please enter a post title')).toBeVisible()
    })

    test('shows form with required fields', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      const postDrawer = await openCreatePostDrawer(page)

      // The fields a post cannot be created without are marked as required
      for (const label of ['Title', 'Slug', 'Category', 'Reading Time', 'Authors']) {
        await expect(
          postDrawer.locator('.ant-form-item-label label.ant-form-item-required', {
            hasText: label
          })
        ).toBeVisible()
      }

      await expect(postDrawer.getByRole('button', { name: 'Create', exact: true })).toBeVisible()
    })
  })

  test.describe('Navigation', () => {
    test('navigates to blog from sidebar', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      // Start at dashboard
      await page.goto(`/console/workspace/${WORKSPACE_ID}/`)
      await waitForLoading(page)

      // Blog sits inside the collapsed "Content" group, which has to be opened first
      await page.locator('.ant-menu-submenu-title').filter({ hasText: 'Content' }).click()
      await page.locator('.ant-menu-item').filter({ hasText: 'Blog' }).click()

      await expect(page).toHaveURL(/\/blog/)
      await expect(page.getByRole('heading', { name: 'All Posts' })).toBeVisible()
    })

    test('can close create form', async ({ authenticatedPage }) => {
      const page = authenticatedPage

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      const postDrawer = await openCreatePostDrawer(page)

      await postDrawer.getByRole('button', { name: 'Close' }).click()

      // A new post starts with editor content, so the drawer asks before dropping it
      const confirm = page.getByRole('dialog').filter({ hasText: 'Unsaved changes' })
      await expect(confirm).toBeVisible()
      await confirm.getByRole('button', { name: 'Yes', exact: true }).click()

      await expect(postDrawer).toBeHidden()
    })
  })

  test.describe('Full Form Submission with Payload Verification', () => {
    test('creates blog post with all fields and verifies SEO in payload', async ({
      authenticatedPageWithData
    }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      const postDrawer = await openCreatePostDrawer(page)

      await fillRequiredPostFields(
        page,
        postDrawer,
        testBlogPostData.title,
        testBlogPostData.authors[0].name
      )
      await postDrawer.locator('#excerpt').fill(testBlogPostData.excerpt!)
      await postDrawer
        .locator('.ant-form-item')
        .filter({ hasText: 'Featured Image URL' })
        .locator('input')
        .fill(testBlogPostData.featured_image_url!)
      await fillSEOFields(page, postDrawer, {
        meta_title: testSEOData.meta_title!,
        meta_description: testSEOData.meta_description!,
        keywords: testSEOData.keywords!,
        canonical_url: testSEOData.canonical_url!,
        og_title: testSEOData.og_title!,
        og_description: testSEOData.og_description!,
        og_image: testSEOData.og_image!
      })

      // The shared request-capture fixture only knows the blog.post.* URL spelling, so the
      // real one the client calls is awaited directly.
      const createRequest = page.waitForRequest((request) =>
        request.url().includes('/api/blogPosts.create')
      )
      await postDrawer.getByRole('button', { name: 'Create', exact: true }).click()
      const body = (await createRequest).postDataJSON()

      expect(body.title).toBe(testBlogPostData.title)
      expect(body.slug).toBe('e2e-test-blog-post-with-full-seo-settings')
      expect(body.category_id).toBe('cat-1')
      expect(body.reading_time_minutes).toBe(7)
      expect(body.excerpt).toBe(testBlogPostData.excerpt)
      expect(body.featured_image_url).toBe(testBlogPostData.featured_image_url)
      expect(body.authors).toEqual([
        { name: testBlogPostData.authors[0].name, avatar_url: '' }
      ])

      // SEO has to travel under `seo`, not at the top level - that was the bug this
      // covers, and namePrefix={['seo']} on SEOSettingsForm is what keeps it there.
      expect(body.seo.meta_title).toBe(testSEOData.meta_title)
      expect(body.seo.meta_description).toBe(testSEOData.meta_description)

      await expect(postDrawer).toBeHidden()
    })

    test('fills all SEO fields and verifies they appear in request', async ({
      authenticatedPageWithData
    }) => {
      const page = authenticatedPageWithData

      await page.goto(`/console/workspace/${WORKSPACE_ID}/blog`)
      await waitForLoading(page)

      const postDrawer = await openCreatePostDrawer(page)

      await fillRequiredPostFields(page, postDrawer, 'SEO Test Post', 'Test Author')

      const seoTestData = {
        meta_title: 'SEO Meta Title Test',
        meta_description: 'This is a test meta description for SEO verification',
        keywords: ['test', 'seo', 'e2e'],
        meta_robots: 'index,follow',
        canonical_url: 'https://example.com/test-canonical',
        og_title: 'Open Graph Title',
        og_description: 'Open Graph Description for social sharing',
        og_image: 'https://example.com/og-image.png'
      }
      await fillSEOFields(page, postDrawer, seoTestData)

      const createRequest = page.waitForRequest((request) =>
        request.url().includes('/api/blogPosts.create')
      )
      await postDrawer.getByRole('button', { name: 'Create', exact: true }).click()
      const body = (await createRequest).postDataJSON()

      // Every SEO field that was typed has to come back out of the form, under `seo`
      expect(body.seo).toEqual(seoTestData)
    })
  })
})
