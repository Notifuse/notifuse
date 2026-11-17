import { afterEach, describe, expect, it, vi } from 'vitest'
import { THEME_PRESETS } from './themePresets'
import { getMockDataForView } from '../../utils/mockBlogData'
import { renderBlogPage } from '../../utils/liquidRenderer'

const text = (html: string) =>
  new DOMParser().parseFromString(html, 'text/html').body.textContent?.replace(/\s+/g, ' ') ?? ''

describe('the default theme, rendered by the preview', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('writes the date as the server does whatever the browser language, a space before the reading time', async () => {
    // A French browser: liquidjs takes its month names from the browser unless told otherwise.
    const resolved = Intl.DateTimeFormat.prototype.resolvedOptions
    vi.spyOn(Intl.DateTimeFormat.prototype, 'resolvedOptions').mockImplementation(function (
      this: Intl.DateTimeFormat
    ) {
      return { ...resolved.call(this), locale: 'fr-FR' }
    })
    const preset = THEME_PRESETS[0]
    for (const view of ['home', 'post'] as const) {
      const result = await renderBlogPage(preset.files, view, getMockDataForView(view))
      expect(result.success).toBe(true)
      const page = text(result.html ?? '')
      expect(page).toMatch(/Mar \d{1,2}, 2024 · 8 min read/)
      expect(page).not.toMatch(/mars|2024·/)
    }
  })

  it('stretches the post text to the screen on a phone, rather than to its widest code line', () => {
    const css = THEME_PRESETS[0].files['styles.css']
    const mobile = css.slice(css.indexOf('/* Post page mobile styles */'))
    expect(mobile).toMatch(/\.post-content-wrapper \{[^}]*flex-direction: column;[^}]*align-items: stretch;/)
  })
})
