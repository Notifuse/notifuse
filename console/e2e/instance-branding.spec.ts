import { test, expect, type Page, type Route } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

// What index.html paints before React: the inline bootstrap reads window.INSTANCE_BRANDING from
// config.js and sets the title, the icons, the manifest and the splash. The React module is held
// back in these tests, so everything asserted here comes from the bootstrap alone.

const PNG = readFileSync(resolve(process.cwd(), 'public', 'neutral-icon.png'))
const LOGO = 'https://cdn.example.com/acme-logo.png'
const ICON = 'https://cdn.example.com/acme-icon.png'

function configJs(branding: Record<string, unknown> | null): string {
  const lines = [
    'window.API_ENDPOINT = "https://localapi.notifuse.com:4000";',
    'window.VERSION = "45.0";',
    'window.ROOT_EMAIL = "";',
    'window.IS_INSTALLED = true;',
    'window.TIMEZONES = [];',
    'window.OIDC_ENABLED = false;',
    'window.OIDC_BUTTON_LABEL = "";'
  ]
  if (branding) lines.push(`window.INSTANCE_BRANDING = ${JSON.stringify(branding)};`)
  return lines.join('\n')
}

const acme = {
  custom: true,
  name: 'Acme',
  logo_url: LOGO,
  icon_url: ICON,
  hide_help_menu: false,
  splash_logo_src: LOGO,
  favicon_src: '/console/neutral-icon.png',
  revision: 'r1'
}

async function holdReact(page: Page): Promise<() => void> {
  let release: () => void = () => {}
  const released = new Promise<void>((resolveRelease) => {
    release = resolveRelease
  })
  await page.route('**/src/main.tsx*', async (route: Route) => {
    await released
    await route.continue()
  })
  return release
}

async function serveConfig(page: Page, branding: Record<string, unknown> | null) {
  await page.route('**/config.js*', (route) =>
    route.fulfill({ status: 200, contentType: 'application/javascript', body: configJs(branding) })
  )
}

const faviconHref = (page: Page) =>
  page.evaluate(() => document.querySelector<HTMLLinkElement>('link[rel="icon"]')?.getAttribute('href'))

test.describe('index.html bootstrap', () => {
  test('keeps the Notifuse identity when config.js carries no brand', async ({ page }) => {
    await serveConfig(page, null)
    const release = await holdReact(page)
    await page.goto('/console/signin', { waitUntil: 'commit' })

    await expect(page).toHaveTitle('Console | Notifuse')
    await expect(page.locator('#preload-brand img')).toHaveAttribute('src', '/console/logo.png')
    expect(await faviconHref(page)).toBe('/console/favicon-32x32.png')
    await expect(page.locator('link[rel="manifest"]')).toHaveAttribute('href', '/console/instance.webmanifest')
    release()
  })

  test('paints the brand before React, then swaps in the icon once it loaded', async ({ page }) => {
    await serveConfig(page, acme)
    await page.route('https://cdn.example.com/**', (route) =>
      route.fulfill({ status: 200, contentType: 'image/png', body: PNG })
    )
    const release = await holdReact(page)
    await page.goto('/console/signin', { waitUntil: 'commit' })

    await expect(page).toHaveTitle('Console | Acme')
    const splash = page.locator('#preload-brand img')
    await expect(splash).toHaveAttribute('src', LOGO)
    await expect(splash).toHaveAttribute('alt', 'Acme')
    await expect(splash).toHaveAttribute('referrerpolicy', 'no-referrer')
    await expect.poll(() => faviconHref(page)).toBe(ICON)
    // No Notifuse asset is requested for a branded instance.
    await expect(page.locator('link[rel="mask-icon"]')).toHaveCount(0)
    await expect(page.locator('img[src="/console/logo.png"]')).toHaveCount(0)
    release()
  })

  test('keeps the neutral icon and shows the name when the images fail', async ({ page }) => {
    await serveConfig(page, acme)
    await page.route('https://cdn.example.com/**', (route) => route.fulfill({ status: 404, body: '' }))
    const release = await holdReact(page)
    await page.goto('/console/signin', { waitUntil: 'commit' })

    await expect(page.locator('#preload-brand .preload-splash-name')).toHaveText('Acme')
    expect(await faviconHref(page)).toBe('/console/neutral-icon.png')
    release()
  })

  test('shows the name in the splash when the brand has no logo', async ({ page }) => {
    await serveConfig(page, { ...acme, logo_url: '', icon_url: '', splash_logo_src: '' })
    const release = await holdReact(page)
    await page.goto('/console/signin', { waitUntil: 'commit' })

    await expect(page.locator('#preload-brand .preload-splash-name')).toHaveText('Acme')
    await expect(page.locator('link[rel="apple-touch-icon"]')).toHaveAttribute('href', '/console/neutral-icon.png')
    release()
  })

  test('starts on the neutral title before config.js answers', async ({ page }) => {
    let answer: () => void = () => {}
    const answered = new Promise<void>((resolveAnswer) => {
      answer = resolveAnswer
    })
    await page.route('**/config.js*', async (route) => {
      await answered
      await route.fulfill({ status: 200, contentType: 'application/javascript', body: configJs(acme) })
    })
    await page.goto('/console/signin', { waitUntil: 'commit' })
    await expect(page).toHaveTitle('Console')
    answer()
    await expect(page).toHaveTitle('Console | Acme')
  })
})

// The sign-in background, drawn by React (MainLayout) from config.js. React runs normally here.
test.describe('sign-in background', () => {
  const BACKGROUND = 'https://cdn.example.com/acme-bg.png?sig=a(b)c'

  test('shows the instance background under the form, without a referrer or the photo credit', async ({ page }) => {
    const photoRequests: string[] = []
    page.on('request', (request) => {
      // Images only: in dev, Vite also serves the photo's import as a module (?import).
      if (request.resourceType() === 'image' && request.url().includes('signin-background')) photoRequests.push(request.url())
    })
    let referer: string | undefined = 'not requested'
    await page.route('https://cdn.example.com/**', (route) => {
      referer = route.request().headers()['referer']
      return route.fulfill({ status: 200, contentType: 'image/png', body: PNG })
    })
    await serveConfig(page, { ...acme, background_url: BACKGROUND })
    await page.goto('/console/signin')

    const background = page.getByTestId('custom-background')
    await expect(background).toBeVisible()
    await expect(background).toHaveAttribute('src', BACKGROUND)
    await expect.poll(() => background.evaluate((img: HTMLImageElement) => img.complete && img.naturalWidth > 0)).toBe(true)
    expect(referer).toBeUndefined()
    expect(await background.evaluate((img) => getComputedStyle(img).objectFit)).toBe('cover')
    await expect(page.getByRole('link', { name: 'Photo by Sean Oulashin' })).toHaveCount(0)
    expect(photoRequests).toEqual([])

    // The form paints above the fixed image: the element under the middle of the card is the card's.
    const email = page.getByLabel('Email')
    await expect(email).toBeVisible()
    const box = await email.boundingBox()
    expect(box).not.toBeNull()
    const topmost = await page.evaluate(
      ([x, y]) => document.elementFromPoint(x, y)?.closest('.ant-card') !== null,
      [box!.x + box!.width / 2, box!.y + box!.height / 2]
    )
    expect(topmost).toBe(true)
    await email.fill('root@example.com')
    await expect(email).toHaveValue('root@example.com')
  })

  test('leaves a plain page when the background fails, never the Notifuse photo', async ({ page }) => {
    await page.route('https://cdn.example.com/**', (route) => route.fulfill({ status: 404, body: '' }))
    await serveConfig(page, { ...acme, background_url: BACKGROUND })
    await page.goto('/console/signin')

    await expect(page.getByLabel('Email')).toBeVisible()
    await expect(page.getByTestId('custom-background')).toHaveCount(0)
    await expect(page.getByRole('link', { name: 'Photo by Sean Oulashin' })).toHaveCount(0)
    expect(await page.locator('.ant-layout').first().evaluate((el) => getComputedStyle(el).backgroundImage)).toBe('none')
  })

  test('keeps the Notifuse photo and its credit without a background', async ({ page }) => {
    await serveConfig(page, acme)
    await page.goto('/console/signin')

    await expect(page.getByLabel('Email')).toBeVisible()
    await expect(page.getByRole('link', { name: 'Photo by Sean Oulashin' })).toBeVisible()
    expect(await page.locator('.ant-layout').first().evaluate((el) => getComputedStyle(el).backgroundImage)).toContain('signin-background')
  })

  test('fits a 375 px screen', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 })
    await page.route('https://cdn.example.com/**', (route) =>
      route.fulfill({ status: 200, contentType: 'image/png', body: PNG })
    )
    await serveConfig(page, { ...acme, background_url: BACKGROUND })
    await page.goto('/console/signin')

    await expect(page.getByLabel('Email')).toBeVisible()
    const imageBox = await page.getByTestId('custom-background').boundingBox()
    expect(imageBox).toEqual({ x: 0, y: 0, width: 375, height: 667 })
  })
})
