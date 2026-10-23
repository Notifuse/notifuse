import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import type { Locator } from '@playwright/test'
import { test, expect } from '../fixtures/auth'
import { mockTemplates } from '../fixtures/mock-data'
import { openTemplateEditor } from '../fixtures/template-editor'

// Scrollbars that take room, as on Windows and Linux, rather than the hidden ones
// headless Chromium has by default
test.use({
  launchOptions: async ({ launchOptions }, provide) => {
    await provide({ ...launchOptions, ignoreDefaultArgs: ['--hide-scrollbars'] })
  }
})

// A real font with Latin glyphs, from the package that installs KaTeX
const fontFile = () =>
  readFileSync(
    createRequire(
      createRequire(import.meta.url).resolve('@ant-design/x-markdown/package.json')
    ).resolve('katex/dist/fonts/KaTeX_Main-Regular.woff2')
  )

const paragraphs = Array.from(
  { length: 6 },
  (_, index) => `<p>Paragraph ${index + 1} of the raw block, long enough to wrap in a narrow column.</p>`
).join('')

// Raw HTML in a font the template imports, under CSS that makes the body follow the
// frame's own height and drops the body's padding; then raw HTML sized on the frame's
// viewport, with a stray end tag, and with margins that collapse through the body
const rawFontTemplate = {
  ...mockTemplates[0],
  email: {
    ...mockTemplates[0].email,
    visual_editor_tree: {
      id: 'mjml-x',
      type: 'mjml',
      children: [
        {
          id: 'head-x',
          type: 'mj-head',
          children: [
            {
              id: 'font-x',
              type: 'mj-font',
              attributes: { name: 'Probe Sans', href: 'https://fonts.example.com/probe.css' }
            },
            {
              id: 'style-x',
              type: 'mj-style',
              // The scrollbar a template asks for still takes nothing from the measurement
              content: 'html, body { height: 100%; } body { padding: 0; } html { scrollbar-width: thin; }'
            }
          ]
        },
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
                    {
                      id: 'raw-font',
                      type: 'mj-raw',
                      content:
                        `<div style="font-family: 'Probe Sans', serif">${paragraphs}` +
                        '<p id="end">Last paragraph</p></div>'
                    },
                    {
                      id: 'raw-vh',
                      type: 'mj-raw',
                      content: `<div style="min-height: 50vh">${paragraphs}</div><p id="end">Last vh</p>`
                    },
                    {
                      id: 'raw-escaped',
                      type: 'mj-raw',
                      content:
                        `<p>Inside</p></div>${paragraphs}` +
                        // As tall as a quarter of its width: any width taken from it shows
                        '<div style="aspect-ratio: 4 / 1"></div><p id="end">Last escaped</p>'
                    },
                    {
                      id: 'raw-margins',
                      type: 'mj-raw',
                      content: `<div style="margin: 80px 0">${paragraphs}<p id="end">Last margined</p></div>`
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

// The frame's height, whether it shows its content down to `#end`, and how much
// room it leaves below it
const frameFit = (frameElement: Locator) =>
  frameElement.evaluate((element: HTMLIFrameElement) => {
    const end = element.contentDocument?.getElementById('end')
    const bottom = end?.getBoundingClientRect().bottom ?? Infinity
    return {
      height: element.clientHeight,
      covers: bottom <= element.clientHeight,
      room: element.clientHeight - bottom
    }
  })

// Whether the frame is now taller than `height` and shows its content down to `#end`
const grownAndCovering = async (frameElement: Locator, height: number) => {
  const fit = await frameFit(frameElement)
  return fit.covers && fit.height > height
}

test.describe('Raw HTML block on the canvas', () => {
  test("loads the template's fonts and shows all its content", async ({
    authenticatedPageWithData: page
  }) => {
    const requested: string[] = []
    const referrers: (string | undefined)[] = []
    let releaseFont = () => {}
    const fontReleased = new Promise<void>((resolve) => {
      releaseFont = resolve
    })
    await page.context().route('https://fonts.example.com/**', async (route) => {
      const url = route.request().url()
      requested.push(url)
      referrers.push(route.request().headers()['referer'])
      if (url.endsWith('/probe.css')) {
        // Three times the size, so the font's arrival makes the text taller
        return route.fulfill({
          contentType: 'text/css',
          body:
            "@font-face { font-family: 'Probe Sans'; size-adjust: 300%;" +
            " src: url('https://fonts.example.com/probe.woff2') format('woff2'); }"
        })
      }
      // The font arrives when the test lets it, after the frame has loaded
      await fontReleased
      return route.fulfill({ contentType: 'font/woff2', body: fontFile() })
    })
    const drawer = await openTemplateEditor(page, rawFontTemplate)

    const frameElement = drawer.locator('[data-block-id="raw-font"] iframe')
    const frame = frameElement.contentFrame()
    await expect(frame.getByText('Last paragraph')).toBeVisible()
    // The font is linked and used: its file is asked for (once per frame showing it)
    await expect
      .poll(() => [...new Set(requested)])
      .toEqual(['https://fonts.example.com/probe.css', 'https://fonts.example.com/probe.woff2'])
    // The console's address goes to neither (the font file names its stylesheet)
    expect(referrers.filter((referrer) => referrer?.includes(new URL(page.url()).host))).toEqual([])
    await expect.poll(() => frameFit(frameElement).then((fit) => fit.covers)).toBe(true)
    const beforeFont = (await frameFit(frameElement)).height

    // Its arrival makes the text taller: the frame grows with it
    releaseFont()
    await expect
      .poll(() =>
        frameElement.evaluate(
          (element: HTMLIFrameElement) =>
            element.contentDocument?.fonts.check("16px 'Probe Sans'") ?? false
        )
      )
      .toBe(true)
    await expect.poll(() => grownAndCovering(frameElement, beforeFont)).toBe(true)
    const wide = (await frameFit(frameElement)).height
    const others = ['raw-vh', 'raw-escaped', 'raw-margins'].map((id) =>
      drawer.locator(`[data-block-id="${id}"] iframe`)
    )
    const [vh, escaped, margined] = others
    // Content let out of the canvas's markup, or whose margins collapse through the
    // body, still shows down to its last line
    await expect(escaped.contentFrame().getByText('Last escaped')).toBeAttached()
    await expect.poll(() => frameFit(escaped).then((fit) => fit.covers)).toBe(true)
    // Measured at the width it is shown at, the scrollbar a collapsed frame would
    // have taking none of it: the content and the 10px below it, no more, no less
    await expect.poll(() => frameFit(escaped).then((fit) => fit.room)).toBeGreaterThanOrEqual(9)
    expect((await frameFit(escaped)).room).toBeLessThanOrEqual(11)
    await expect(margined.contentFrame().getByText('Last margined')).toBeAttached()
    await expect.poll(() => frameFit(margined).then((fit) => fit.covers)).toBe(true)
    await expect(vh.contentFrame().getByText('Last vh')).toBeAttached()
    await expect.poll(() => frameFit(vh).then((fit) => fit.height > 100)).toBe(true)
    const othersWide = await Promise.all(others.map((frame) => frameFit(frame)))

    // A narrower column rewraps the text: the frame grows to keep showing all of it
    const column = drawer.locator('[data-block-id="column-x"]')
    const columnWidth = await column.evaluate((element: HTMLElement) => {
      const width = element.style.width
      element.style.width = '200px'
      return width
    })
    await expect.poll(() => grownAndCovering(frameElement, wide)).toBe(true)

    // Back to its width, each frame is back to its height, although the body follows
    // the frame's height, and content sized on the frame's viewport did not grow
    await column.evaluate((element: HTMLElement, width) => {
      element.style.width = width
    }, columnWidth)
    await expect.poll(() => frameFit(frameElement).then((fit) => fit.height)).toBe(wide)
    await expect
      .poll(() => Promise.all(others.map((frame) => frameFit(frame))))
      .toEqual(othersWide)
  })
})
