import type { ThemeConfig } from 'antd'
import { alpha, colors as c, typography as t, radii, shadows as s, motion } from './tokens'

const seconds = (ms: number) => `${ms / 1000}s`

/**
 * The console's antd theme, built only from tokens.ts: no colour is written here.
 *
 * Kept as one object literal so TypeScript rejects a misspelled token name. Components do not
 * pass a local `theme` to a nested ConfigProvider to change a colour or a size; what the theme
 * cannot express goes to theme/antd-overrides.css, with its reason.
 */
export const antdTheme: ThemeConfig = {
  token: {
    colorPrimary: c.primary[500],
    colorPrimaryHover: c.primary[600],
    colorPrimaryActive: c.primary[700],
    colorPrimaryBorder: c.primary[300],
    colorLink: c.primary[600],
    colorLinkHover: c.primary[700],
    colorInfo: c.status.info,
    colorSuccess: c.status.success,
    colorWarning: c.status.warning,
    colorError: c.status.danger,
    colorText: c.ink,
    colorTextSecondary: c.ink2,
    colorTextTertiary: c.ink3,
    colorTextQuaternary: c.ink4,
    colorBorder: c.line,
    colorBorderSecondary: c.lineSoft,
    colorBgLayout: c.canvas,
    colorBgContainer: c.surface,
    colorBgElevated: c.surface,
    // Translucent, as antd's own: a filled control or a text-button hover must still show on the
    // canvas, which an opaque fill two levels away from it would not.
    colorFillTertiary: alpha(c.ink, 0.05),
    controlItemBgActive: c.primary[100],
    fontFamily: t.sans,
    fontFamilyCode: t.mono,
    fontSize: t.size.base,
    fontSizeSM: t.size.xs,
    // Written out so the drawer title size is a choice rather than a derivation (antd also gives 14).
    fontSizeLG: 14,
    fontSizeHeading4: t.size.title,
    fontSizeHeading5: t.size.subtitle,
    lineHeight: t.lineHeight.base,
    // antd derives lineHeightSM from its own fontSizeSM (10 for a 13px base), which gives 12px text
    // a 21.6px line and 23.6px tags; 20px keeps a tag as tall as a small control.
    lineHeightSM: 20 / t.size.xs,
    lineHeightHeading4: t.lineHeight.title,
    lineHeightHeading5: t.lineHeight.subtitle,
    controlHeight: 28,
    controlHeightSM: 22,
    controlHeightLG: 34,
    borderRadius: radii.md,
    borderRadiusSM: radii.sm,
    borderRadiusXS: radii.xs,
    borderRadiusLG: radii.lg,
    boxShadow: s.popup,
    boxShadowSecondary: s.popup,
    controlOutline: s.focus,
    controlOutlineWidth: 2,
    motionDurationMid: seconds(motion.fast),
    motionDurationSlow: seconds(motion.slow)
  },
  components: {
    Layout: {
      bodyBg: c.canvas,
      headerBg: c.canvas,
      siderBg: c.surface,
      lightSiderBg: c.surface,
      headerHeight: 48,
      headerPadding: '0 16px 0 20px'
    },
    Menu: {
      itemHeight: 30,
      itemMarginBlock: 1,
      itemMarginInline: 8,
      itemPaddingInline: 10,
      iconSize: 14,
      collapsedIconSize: 15,
      itemBg: 'transparent',
      subMenuItemBg: 'transparent',
      itemColor: c.ink2,
      itemHoverBg: c.fill,
      itemHoverColor: c.ink,
      itemSelectedBg: c.primary[100],
      itemSelectedColor: c.primary[700],
      groupTitleFontSize: t.size.overline,
      groupTitleColor: c.ink3
    },
    Card: {
      // antd draws the card border (and its header divider) with colorBorderSecondary.
      colorBorderSecondary: c.line,
      headerHeight: 40,
      headerFontSize: t.size.base,
      bodyPadding: 14,
      headerPadding: 14
    },
    Table: {
      headerBg: c.subtle,
      headerColor: c.ink3,
      headerSplitColor: 'transparent',
      borderColor: c.lineSoft,
      // Subtle rather than fill: a neutral tag (fill) must stay visible on a hovered row.
      rowHoverBg: c.subtle,
      rowSelectedBg: c.primary[50],
      rowSelectedHoverBg: c.primary[100],
      cellFontSize: t.size.sm,
      cellFontSizeMD: t.size.sm,
      cellFontSizeSM: t.size.xs,
      cellPaddingInline: 8,
      cellPaddingBlock: 12,
      cellPaddingBlockMD: 7,
      cellPaddingBlockSM: 6,
      headerSortActiveBg: c.subtle,
      headerSortHoverBg: c.subtle,
      bodySortBg: 'transparent'
    },
    Tabs: {
      horizontalMargin: '0 0 12px 0',
      horizontalItemPadding: '8px 0',
      horizontalItemGutter: 18,
      titleFontSize: t.size.base,
      itemColor: c.ink3,
      itemSelectedColor: c.ink,
      inkBarColor: c.primary[500]
    },
    Modal: {
      contentBg: c.surface,
      headerBg: c.surface,
      footerBg: c.surface
    },
    Button: {
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      paddingInline: 12,
      paddingInlineSM: 8,
      fontWeight: 500
    },
    Segmented: {
      itemSelectedBg: c.surface,
      trackBg: c.line,
      trackPadding: 2
    },
    Tag: {
      defaultBg: c.fill,
      defaultColor: c.ink2
    },
    Timeline: {
      dotBg: c.surface
    },
    Statistic: {
      contentFontSize: t.size.kpi,
      titleFontSize: t.size.xs
    }
  }
}
