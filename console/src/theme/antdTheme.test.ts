import { describe, it, expect } from 'vitest'
import { readFileSync } from 'fs'
import { resolve } from 'path'
import { theme } from 'antd'
import { antdTheme } from './antdTheme'
import { colors } from './tokens'

const token = antdTheme.token!
const components = antdTheme.components!

/** An opaque or translucent colour as the RGB it shows over the white surface. */
function onSurface(color: string): [number, number, number] {
  const hex = color.match(/^#([0-9a-f]{6})$/i)
  if (hex) return [0, 2, 4].map((i) => parseInt(hex[1].slice(i, i + 2), 16)) as [number, number, number]
  const rgba = color.match(/^rgba\((\d+),\s*(\d+),\s*(\d+),\s*([\d.]+)\)$/)
  if (!rgba) throw new Error(`Unreadable colour ${color}`)
  const a = Number(rgba[4])
  return [1, 2, 3].map((i) => Math.round(255 - a * (255 - Number(rgba[i])))) as [number, number, number]
}

/** The WCAG contrast ratio between two colours, each shown over the surface. */
function contrast(a: string, b: string): number {
  const luminance = (rgb: [number, number, number]) => {
    const [r, g, bl] = rgb.map((v) => {
      const c = v / 255
      return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
    })
    return 0.2126 * r + 0.7152 * g + 0.0722 * bl
  }
  const [hi, lo] = [luminance(onSurface(a)), luminance(onSurface(b))].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

describe('antd theme', () => {
  it('derives 20px small lines, so a tag is as tall as a 22px small control', () => {
    const derived = theme.getDesignToken(antdTheme)
    expect(derived.fontSizeSM * derived.lineHeightSM).toBeCloseTo(20)
    expect(derived.controlHeightSM).toBe(22)
  })

  it('hovers table rows apart from the header, in a colour a neutral tag stays visible on', () => {
    const table = components.Table!
    const tag = components.Tag!.defaultBg as string
    const hover = table.rowHoverBg as string
    // Both present, so the comparisons below compare colours
    expect(hover).toBeTruthy()
    expect(tag).toBeTruthy()
    expect(onSurface(hover)).not.toEqual(onSurface(table.headerBg as string))
    // A tag stands out more on a hovered row than on the header
    expect(contrast(tag, hover)).toBeGreaterThan(contrast(tag, table.headerBg as string))
  })

  it('hovers table rows in an opaque colour, so a fixed column hides what scrolls under it', () => {
    expect(components.Table!.rowHoverBg).toMatch(/^#[0-9a-f]{6}$/i)
  })

  it('makes the hover of a primary button stand further from the button than the 600 step it had', () => {
    const button = token.colorPrimary as string
    const hover = token.colorPrimaryHover as string
    const pressed = token.colorPrimaryActive as string
    expect(contrast(button, hover)).toBeGreaterThan(contrast(button, colors.primary[600]))
    // and a pressed button darker again
    expect(contrast(button, pressed)).toBeGreaterThan(contrast(button, hover))
  })

  it('leaves the right border of the menus to antd: the sidebar takes its own off in the overrides', () => {
    expect(components.Menu).not.toHaveProperty('activeBarBorderWidth')
  })

  it('takes its colours from the Nuit claire tokens', () => {
    expect(token).toMatchObject({
      colorPrimary: '#6a5af9',
      colorPrimaryHover: '#4b3fd6',
      colorPrimaryActive: '#3b33c1',
      colorPrimaryBorder: '#c9c4ff',
      colorLink: '#5b4beb',
      colorLinkHover: '#4b3fd6',
      colorInfo: '#2563eb',
      colorSuccess: '#16a34a',
      colorWarning: '#d97706',
      colorError: '#dc2626',
      colorText: '#171923',
      colorTextSecondary: '#545a6e',
      colorTextTertiary: '#666c81',
      colorTextQuaternary: '#9095a7',
      colorBorder: '#e1e3ea',
      colorBorderSecondary: '#eceef3',
      colorBgLayout: '#f3f4f7',
      colorBgContainer: '#ffffff',
      colorBgElevated: '#ffffff',
      colorFillTertiary: 'rgba(23, 25, 35, 0.05)',
      controlItemBgActive: '#efedff',
      controlOutline: 'rgba(106, 90, 249, 0.18)'
    })
  })

  it('sets the density of the spec', () => {
    expect(token).toMatchObject({
      fontSize: 13,
      fontSizeSM: 12,
      fontSizeLG: 14,
      fontSizeHeading4: 18,
      fontSizeHeading5: 15,
      lineHeight: 1.45,
      lineHeightHeading4: 1.3,
      lineHeightHeading5: 1.35,
      controlHeight: 28,
      controlHeightSM: 22,
      controlHeightLG: 34,
      borderRadius: 6,
      borderRadiusSM: 4,
      borderRadiusXS: 3,
      borderRadiusLG: 8,
      controlOutlineWidth: 2,
      motionDurationMid: '0.12s',
      motionDurationSlow: '0.18s'
    })
    expect(token.fontFamily).toMatch(/^'Inter Variable'/)
    expect(token.fontFamilyCode).toMatch(/^'JetBrains Mono Variable'/)
  })

  it('sizes tables, menus and cards from the tokens', () => {
    expect(components.Table).toMatchObject({
      headerBg: '#f8f9fb',
      headerColor: '#666c81',
      borderColor: '#eceef3',
      rowHoverBg: '#fbfaff',
      rowSelectedBg: '#f5f4ff',
      rowSelectedHoverBg: '#efedff',
      cellFontSize: 12.5,
      cellFontSizeSM: 12,
      cellPaddingBlock: 12,
      cellPaddingBlockMD: 7,
      cellPaddingBlockSM: 6
    })
    expect(components.Menu).toMatchObject({
      itemHeight: 30,
      itemSelectedBg: '#efedff',
      itemSelectedColor: '#4b3fd6',
      itemColor: '#545a6e',
      groupTitleFontSize: 10.5
    })
    expect(components.Card).toMatchObject({ colorBorderSecondary: '#e1e3ea', headerHeight: 40, bodyPadding: 14 })
    expect(components.Layout).toMatchObject({ bodyBg: '#f3f4f7', lightSiderBg: '#ffffff' })
    // The chrome no longer uses Layout.Header nor Layout.Sider.
    expect(components.Layout).not.toHaveProperty('headerHeight')
    expect(components.Layout).not.toHaveProperty('headerPadding')
    expect(components.Layout).not.toHaveProperty('siderBg')
    // The collapsed sidebar is 56px wide, and its menu as wide as it.
    expect(components.Menu).toMatchObject({ collapsedWidth: 56 })
    expect(components.Modal).toMatchObject({ contentBg: '#ffffff', headerBg: '#ffffff', footerBg: '#ffffff' })
    expect(components.Timeline).toMatchObject({ dotBg: '#ffffff' })
  })

  it('writes no colour literal: every value comes from tokens.ts', () => {
    const source = readFileSync(resolve(__dirname, 'antdTheme.ts'), 'utf8')
    expect(source).not.toMatch(/#[0-9a-f]{3,8}\b/i)
    expect(source).not.toMatch(/rgba?\(|hsla?\(/i)
  })
})
