import { describe, it, expect } from 'vitest'
import { colors, shadows, alpha, typography, radii, motion, layout } from './tokens'

// WCAG 2.x relative luminance and contrast ratio, kept here so tokens.ts holds values only.
function luminance(hex: string): number {
  const channels = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
  const [r, g, b] = channels.map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function contrast(fg: string, bg: string): number {
  const [a, b] = [luminance(fg), luminance(bg)].sort((x, y) => y - x)
  return (a + 0.05) / (b + 0.05)
}

const AA = 4.5

describe('colour tokens', () => {
  it.each([
    ['ink', colors.ink],
    ['ink2', colors.ink2],
    ['ink3', colors.ink3]
  ])('%s reads at AA on surface, subtle and canvas', (_name, fg) => {
    for (const bg of [colors.surface, colors.subtle, colors.canvas]) {
      expect(contrast(fg, bg)).toBeGreaterThanOrEqual(AA)
    }
  })

  it.each(Object.entries(colors.tones))('the %s tone reads at AA', (_name, tone) => {
    expect(contrast(tone.fg, tone.bg)).toBeGreaterThanOrEqual(AA)
  })

  it('white reads at AA on the primary button', () => {
    expect(contrast('#ffffff', colors.primary[500])).toBeGreaterThanOrEqual(AA)
  })

  it('links read at AA on white', () => {
    expect(contrast(colors.primary[600], '#ffffff')).toBeGreaterThanOrEqual(AA)
  })

  it('active items read at AA on their violet background', () => {
    expect(contrast(colors.primary[700], colors.primary[100])).toBeGreaterThanOrEqual(AA)
  })

  it('writes every base colour as a 6-digit lowercase hex, so an opacity suffix can be appended', () => {
    const walk = (value: unknown, path: string): Array<[string, unknown]> => {
      if (value && typeof value === 'object') {
        return Object.entries(value).flatMap(([key, child]) => walk(child, `${path}.${key}`))
      }
      return [[path, value]]
    }
    const leaves = walk(colors, 'colors')
    expect(leaves.length).toBeGreaterThan(0)
    for (const [path, value] of leaves) {
      expect({ path, value }).toEqual({ path, value: expect.stringMatching(/^#[0-9a-f]{6}$/) })
    }
  })

  it('keeps the brand violet and the chart colours the spec chose', () => {
    expect(colors.primary[500]).toBe('#6a5af9')
    expect(colors.chart.primary).toBe('#6a5af9')
    expect(colors.chart.comparison).toBe('#b4b8c7')
  })
})

describe('alpha', () => {
  it('turns a hex colour and an opacity into rgba', () => {
    expect(alpha('#6a5af9', 0.5)).toBe('rgba(106, 90, 249, 0.5)')
  })

  it('refuses anything but a 6-digit hex', () => {
    expect(() => alpha('#fff', 1)).toThrow()
    expect(() => alpha('rgb(0, 0, 0)', 1)).toThrow()
  })
})

describe('derived tokens', () => {
  it('draws the focus ring in the primary violet at 18 %', () => {
    expect(shadows.focus).toBe('rgba(106, 90, 249, 0.18)')
  })

  it('exposes the type scale, radii, durations and structural widths of the spec', () => {
    expect(typography.size.base).toBe(13)
    expect(typography.size.sm).toBe(12.5)
    expect(typography.size.title).toBe(18)
    expect(radii).toEqual({ xs: 3, sm: 4, md: 6, lg: 8 })
    expect(motion).toMatchObject({ fast: 120, slow: 180, pillPulse: 1600 })
    expect(layout).toMatchObject({ sidebar: 228, sidebarCollapsed: 56, rail: 220, sidepanel: 520 })
  })
})
