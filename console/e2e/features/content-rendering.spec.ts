import type { Locator, Page, Route } from '@playwright/test'
import { test, expect } from '../fixtures/auth'
import { waitForLoading } from '../fixtures/test-utils'
import {
  mockTemplates,
  mockWorkspace,
  mockUserMeResponse,
  mockContacts
} from '../fixtures/mock-data'

const WORKSPACE_ID = 'test-workspace'

// Content saved by one member, or reported by a visitor, and opened by another
// member. Every snippet that could run records itself in the console's
// window.__probes (through `parent`, which is the console itself at the top level
// and from any frame it holds); the console must never run any of them, so the
// list stays undefined. Each probe is named after where it sits, so a failure
// says which surface ran it.
const probe = (name: string) => `(parent.__probes = parent.__probes || []).push('${name}')`

const firedProbes = (page: Page) =>
  page.evaluate(() => (window as unknown as { __probes?: string[] }).__probes)

const json = (route: Route, data: unknown) =>
  route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(data) })

// The fixtures route every API call through one handler; a route added here is
// registered later, so Playwright tries it first.
const mockApi = async (page: Page, path: string, data: unknown) => {
  await page.route(`https://localapi.notifuse.com:4000/api/${path}*`, (route) => json(route, data))
}

const textContent =
  '<p>Hello {{ contact.first_name }}' +
  `<img src="x" onerror="${probe('canvas-text')}">` +
  `<a href="javascript:${probe('text-link')}">link</a>` +
  '<span style="position: fixed; inset: 0; z-index: 2147483647">cover</span></p>'

// Whether everything a block draws at a fixed position lies within the block
const fixedContentInside = async (block: Locator) => {
  const area = await block.boundingBox()
  const boxes = await block.locator('[style*="position: fixed"]').evaluateAll((elements) =>
    elements.map((element) => element.getBoundingClientRect().toJSON() as DOMRect)
  )
  expect(area).not.toBeNull()
  expect(boxes).not.toEqual([])
  return boxes.every(
    (box) =>
      area !== null &&
      box.left >= area.x - 1 &&
      box.top >= area.y - 1 &&
      box.right <= area.x + area.width + 1 &&
      box.bottom <= area.y + area.height + 1
  )
}

// Text whose shadow falls outside the text's own box, to the left and below. The
// email shows it in the block's padding.
const shadowedContent =
  '<p style="margin: 0"><span style="text-shadow: -12px 20px 0 red">Buy now</span></p>'

// A band drawn over the text and as far past its box as the containment lets it,
// on top of anything else the content could be stacked with
const bandedContent =
  '<p style="margin: 0">Banded <span style="position: fixed; top: -25px; left: -25px; ' +
  'right: -25px; height: 50px; z-index: 2147483647; background: rgba(255, 0, 0, 0.5)">' +
  'band</span></p>'

// Whether a block draws anything in the strip just past its content's box on one
// side: the strip as drawn, compared with the strip once the content is hidden.
const drawsPast = async (block: Locator, side: 'left' | 'bottom') => {
  const content = block.locator('[style*="contain"]')
  await expect(content).toHaveCount(1)
  await content.scrollIntoViewIfNeeded()
  const box = await content.boundingBox()
  expect(box).not.toBeNull()
  if (box === null) return false
  const clip =
    side === 'left'
      ? { x: box.x - 16, y: box.y, width: 16, height: box.height + 16 }
      : { x: box.x, y: box.y + box.height, width: box.width, height: 16 }
  const strip = () => block.page().screenshot({ clip, animations: 'disabled' })
  const drawn = await strip()
  await content.evaluate((element: HTMLElement) => {
    element.style.visibility = 'hidden'
  })
  const blank = await strip()
  await content.evaluate((element: HTMLElement) => {
    element.style.removeProperty('visibility')
  })
  return !drawn.equals(blank)
}

const buttonContent =
  `<p onmouseover="${probe('button-block')}">Buy now` +
  `<span><img src="x" onerror="${probe('button-span')}"></span></p>`

const template = {
  ...mockTemplates[0],
  email: {
    ...mockTemplates[0].email,
    visual_editor_tree: {
      id: 'mjml-x',
      type: 'mjml',
      children: [
        {
          id: 'body-x',
          type: 'mj-body',
          children: [
            {
              id: 'section-x',
              type: 'mj-section',
              children: [
                {
                  id: 'column-x',
                  type: 'mj-column',
                  children: [
                    { id: 'text-x', type: 'mj-text', content: textContent },
                    { id: 'text-shadow', type: 'mj-text', content: shadowedContent },
                    { id: 'text-band', type: 'mj-text', content: bandedContent },
                    {
                      id: 'button-x',
                      type: 'mj-button',
                      attributes: { href: `javascript:${probe('button-href')}` },
                      content: buttonContent
                    },
                    {
                      id: 'image-x',
                      type: 'mj-image',
                      attributes: {
                        src: 'https://localapi.notifuse.com:4000/i.png',
                        href: `javascript:${probe('image-href')}`
                      }
                    }
                  ]
                }
              ]
            }
          ]
        }
      ]
    }
  }
}

const openTemplateEditor = async (page: Page) => {
  await mockApi(page, 'templates.list', { templates: [template], total: 1 })
  await mockApi(page, 'templates.get', { template })
  await mockApi(page, 'templates.compile', {
    html:
      '<html><body><p>Compiled</p>' +
      `<script>${probe('preview-script')}</script>` +
      `<img src="x" onerror="${probe('preview-image')}">` +
      '</body></html>'
  })

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

test.describe('Template content in the email editor', () => {
  test('canvas and block editors show it without running it', async ({
    authenticatedPageWithData: page
  }) => {
    const drawer = await openTemplateEditor(page)

    const textBlock = drawer.locator('[data-block-id="text-x"]')
    const buttonBlock = drawer.locator('[data-block-id="button-x"]')
    await expect(textBlock).toContainText('Hello {{ contact.first_name }}')
    await expect(buttonBlock).toContainText('Buy now')
    // Content positioned over the page stays inside its block, on the canvas...
    expect(await fixedContentInside(textBlock)).toBe(true)

    // Selected, each block opens in its editor
    await textBlock.click()
    await expect(textBlock.locator('.ProseMirror')).toContainText('Hello')
    // ...and in the editor
    expect(await fixedContentInside(textBlock)).toBe(true)
    await buttonBlock.click()
    await expect(buttonBlock.locator('.ProseMirror')).toContainText('Buy now')
    await buttonBlock.locator('.ProseMirror').hover()

    // Side panel of the selected button shows its link target as text
    await expect.soft(drawer.getByText(`javascript:${probe('button-href')}`)).toBeVisible()
    await expect.soft(drawer.locator('a[href^="javascript:"]')).toHaveCount(0)

    // Give image loads time to fail and fire whatever they would fire
    await page.waitForTimeout(1500)
    expect(await firedProbes(page)).toBeUndefined()
  })

  test('canvas and block editors draw what spills past the text, as the email does', async ({
    authenticatedPageWithData: page
  }) => {
    const drawer = await openTemplateEditor(page)
    const block = drawer.locator('[data-block-id="text-shadow"]')
    await expect(block).toContainText('Buy now')
    expect(await drawsPast(block, 'left')).toBe(true)
    expect(await drawsPast(block, 'bottom')).toBe(true)

    // Selected, the block shows its action buttons over its left padding
    await block.click()
    await expect(block.locator('.ProseMirror')).toContainText('Buy now')
    await expect(block.locator('.ProseMirror span[style*="text-shadow"]')).toHaveCount(1)
    expect(await drawsPast(block, 'bottom')).toBe(true)
  })

  test('block toolbars stay above what a block draws past its text', async ({
    authenticatedPageWithData: page
  }) => {
    const drawer = await openTemplateEditor(page)
    const block = drawer.locator('[data-block-id="text-band"]')
    await block.click()
    await expect(block.locator('.ProseMirror')).toContainText('Banded')

    const content = block.locator('[style*="contain"]')
    const formatting = block.locator('.tiptap-toolbar')
    const clone = drawer.locator('button:has(svg[data-icon="clone"])')
    const [area, formattingBox, cloneBox] = await Promise.all(
      [content, formatting, clone].map((locator) => locator.boundingBox())
    )
    if (area === null || formattingBox === null || cloneBox === null) {
      throw new Error('the block, its formatting toolbar or its clone button is not laid out')
    }
    const hit = (x: number, y: number) =>
      page.evaluate(([px, py]) => {
        const element = document.elementFromPoint(px, py)
        if (element?.closest('.tiptap-toolbar')) return 'formatting toolbar'
        if (element?.closest('button')?.querySelector('svg[data-icon="clone"]')) return 'clone'
        if (element?.closest('.ProseMirror')) return 'content'
        return element?.tagName ?? null
      }, [x, y] as const)

    // The band reaches past the text, right beside the formatting toolbar...
    const formattingBottom = formattingBox.y + formattingBox.height - 2
    expect(formattingBottom).toBeLessThan(area.y)
    expect(await hit(formattingBox.x + formattingBox.width + 8, formattingBottom)).toBe('content')
    // ...and not over either toolbar
    expect(await hit(formattingBox.x + formattingBox.width / 2, formattingBottom)).toBe(
      'formatting toolbar'
    )
    const cloneCenter = [cloneBox.x + cloneBox.width / 2, cloneBox.y + cloneBox.height / 2] as const
    expect(await hit(...cloneCenter)).toBe('clone')
  })

  test('preview shows the compiled email without running it', async ({
    authenticatedPageWithData: page
  }) => {
    const drawer = await openTemplateEditor(page)

    await drawer.getByText('Preview', { exact: true }).click()
    const frame = drawer.locator('iframe[title="Email Preview"]')
    await expect
      .soft(frame)
      .toHaveAttribute('sandbox', 'allow-popups allow-popups-to-escape-sandbox')
    await expect(frame.contentFrame().getByText('Compiled')).toBeVisible()

    await page.waitForTimeout(1500)
    expect(await firedProbes(page)).toBeUndefined()
  })
})

test.describe('Blog theme preview', () => {
  test('runs theme scripts away from the console', async ({ authenticatedPageWithData: page }) => {
    const blogWorkspace = {
      ...mockWorkspace,
      settings: {
        ...mockWorkspace.settings,
        blog_enabled: true,
        custom_endpoint_url: 'https://blog.example.com'
      }
    }
    const files = {
      'home.liquid':
        '<html><body><p id="theme-home">Theme home</p>' +
        '<script>{% include "scripts" %}</script></body></html>',
      'category.liquid': '<html><body>Category</body></html>',
      'post.liquid': '<html><body>Post</body></html>',
      'header.liquid': '',
      'footer.liquid': '',
      'shared.liquid': '',
      'styles.css': '',
      // Runs inside the preview, using storage and cookies as a theme may; marks
      // the page it can reach, then tries the console
      'scripts.js':
        "localStorage.setItem('mode', 'dark'); sessionStorage.setItem('seen', '1');" +
        "document.cookie = 'consent=yes; path=/'; localStorage.theme = 'light';" +
        "document.body.setAttribute('data-theme-script', [localStorage.getItem('mode')," +
        " sessionStorage.getItem('seen'), document.cookie, localStorage.getItem('theme')].join(' '));" +
        `try { ${probe('theme-script')} } catch (e) {}`
    }
    const theme = {
      version: 1,
      published_at: null,
      files,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-10T00:00:00Z'
    }
    await mockApi(page, 'user.me', { ...mockUserMeResponse, workspaces: [blogWorkspace] })
    await mockApi(page, 'blogThemes.list', { themes: [theme], total: 1 })
    await mockApi(page, 'blogThemes.get', { theme })

    await page.goto(`/console/workspace/${WORKSPACE_ID}/settings/blog`)
    await waitForLoading(page)
    // The edit action is an icon button, named only by its tooltip
    await page.getByRole('button', { name: 'edit' }).first().click()

    const frame = page.locator('iframe[title="Blog Preview"]').first()
    await expect.soft(frame).toHaveAttribute('sandbox', 'allow-scripts')
    const preview = frame.contentFrame()
    await expect(preview.locator('#theme-home')).toHaveText('Theme home')
    await expect(preview.locator('body')).toHaveAttribute(
      'data-theme-script',
      'dark 1 consent=yes light'
    )

    expect(await firedProbes(page)).toBeUndefined()
  })
})

test.describe('Contact timeline page views', () => {
  test('opens only pages of the visited site', async ({ authenticatedPageWithData: page }) => {
    const pageView = (id: string, path: string) => ({
      id,
      email: mockContacts[0].email,
      operation: 'insert',
      entity_type: 'web_page',
      kind: 'web.pageview',
      entity_id: `sess-1:${id}`,
      changes: {
        path: { new: path },
        page_number: { new: 1 },
        landing_domain: { new: 'www.example.com' }
      },
      created_at: '2026-08-12T10:01:00Z',
      db_created_at: '2026-08-12T10:01:00Z'
    })
    // The workspace website vouches for the visit domain
    const siteWorkspace = {
      ...mockWorkspace,
      settings: { ...mockWorkspace.settings, website_url: 'https://www.example.com' }
    }
    await mockApi(page, 'user.me', { ...mockUserMeResponse, workspaces: [siteWorkspace] })
    await page.route('https://www.example.com/**', (route) =>
      route.fulfill({ status: 200, contentType: 'text/html', body: '<p>Visited site</p>' })
    )
    await mockApi(page, 'timeline.list', {
      timeline: [
        pageView('1', `javascript:${probe('timeline-path')}`),
        pageView('2', '//other.example/landing'),
        pageView('3', '/pricing')
      ]
    })

    await page.goto(`/console/workspace/${WORKSPACE_ID}/contacts`)
    await waitForLoading(page)
    await page
      .locator('.ant-table-row')
      .first()
      .locator('.contacts-actions-col button')
      .filter({ has: page.locator('svg[data-icon="eye"]') })
      .click()
    const contact = page.getByRole('dialog', { name: 'Contact Details', exact: true })
    await expect(contact).toBeVisible()
    await expect(contact.getByText('/pricing', { exact: true })).toBeVisible()

    // The script path is listed as text, with nothing to open
    await expect.soft(contact.getByText(`javascript:${probe('timeline-path')}`)).toBeVisible()
    await expect.soft(contact.getByRole('button', { name: 'View website' })).toHaveCount(2)

    // Two slashes stay on the visited site
    await contact
      .getByText('//other.example/landing', { exact: true })
      .locator('..')
      .getByRole('button', { name: 'View website' })
      .click()
    const viewer = page.getByRole('dialog', { name: 'View website', exact: true })
    await expect(viewer.getByText('https://www.example.com//other.example/landing')).toBeVisible()
    await expect(viewer.locator('iframe')).toHaveAttribute(
      'src',
      'https://www.example.com//other.example/landing'
    )

    expect(await firedProbes(page)).toBeUndefined()
  })
})
