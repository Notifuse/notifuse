import type { ThemeConfig } from 'antd'
import { colors as c } from '../../../theme/tokens'

/**
 * The antd theme of the settings panel, on top of the console's: antd's styles are unlayered, so
 * Tailwind classes cannot restyle its controls. Inherited values (placeholders) read as values, not
 * as disabled fields; a Segmented's track is the panel's grey and its chosen option stands out in
 * the primary colour.
 */
export const SETTINGS_PANEL_THEME: ThemeConfig = {
  token: {
    colorTextPlaceholder: c.ink4
  },
  components: {
    Segmented: {
      trackBg: c.fill,
      itemColor: c.ink2,
      itemSelectedColor: c.primary[700],
      boxShadowTertiary: `0 0 0 1px ${c.primary[300]}, 0 1px 2px rgba(23, 25, 35, 0.06)`
    }
  }
}
