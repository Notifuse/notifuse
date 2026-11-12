import { describe, it, expect } from 'vitest'
import { readFileSync } from 'fs'
import { resolve } from 'path'
import { theme } from 'antd'
import { antdTheme } from './antdTheme'

const token = antdTheme.token!
const components = antdTheme.components!

describe('antd theme', () => {
  it('derives 20px small lines, so a tag is as tall as a 22px small control', () => {
    const derived = theme.getDesignToken(antdTheme)
    expect(derived.fontSizeSM * derived.lineHeightSM).toBeCloseTo(20)
    expect(derived.controlHeightSM).toBe(22)
  })

  it('hovers table rows in a colour a neutral tag stays visible on', () => {
    expect(components.Table?.rowHoverBg).not.toBe(components.Tag?.defaultBg)
  })

  it('takes its colours from the Nuit claire tokens', () => {
    expect(token).toMatchObject({
      colorPrimary: '#6a5af9',
      colorPrimaryHover: '#5b4beb',
      colorPrimaryActive: '#4b3fd6',
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
      rowHoverBg: '#f8f9fb',
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
    expect(components.Menu).toMatchObject({ collapsedWidth: 56, activeBarBorderWidth: 0 })
    expect(components.Modal).toMatchObject({ contentBg: '#ffffff', headerBg: '#ffffff', footerBg: '#ffffff' })
    expect(components.Timeline).toMatchObject({ dotBg: '#ffffff' })
  })

  it('writes no colour literal: every value comes from tokens.ts', () => {
    const source = readFileSync(resolve(__dirname, 'antdTheme.ts'), 'utf8')
    expect(source).not.toMatch(/#[0-9a-f]{3,8}\b/i)
    expect(source).not.toMatch(/rgba?\(|hsla?\(/i)
  })
})
