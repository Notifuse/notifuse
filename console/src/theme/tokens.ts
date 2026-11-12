/**
 * The Nuit claire identity, defined once.
 *
 * The antd theme (antdTheme.ts) is built from these values, and the Tailwind `@theme` block of
 * index.css repeats them by hand because Tailwind reads CSS, not TypeScript;
 * tailwindTheme.contract.test.ts keeps the two in step.
 *
 * Tokens are named by role (surface, ink-2…) rather than by hue, so a dark theme can later swap
 * the values without renaming anything. Every colour is a 6-digit hex: existing code appends an
 * opacity suffix to them (`${PRIMARY_COLOR}40`).
 */

const gray = {
  50: '#f8f9fb',
  100: '#f1f2f6',
  200: '#e1e3ea',
  300: '#cdd0da',
  400: '#9095a7',
  500: '#666c81',
  600: '#545a6e',
  700: '#3d4255',
  800: '#262a38',
  900: '#171923'
} as const

const primary = {
  50: '#f5f4ff',
  100: '#efedff',
  300: '#c9c4ff',
  400: '#8c82ff',
  500: '#6a5af9',
  600: '#5b4beb',
  700: '#4b3fd6'
} as const

export const colors = {
  /** Page background and full-width drawers. */
  canvas: '#f3f4f7',
  /** Cards, tables, sidebar, left rail, side drawers, popups. */
  surface: '#ffffff',
  /** Table headers, secondary areas inside a card. */
  subtle: gray[50],
  /** Row hover, neutral pill, icon background. */
  fill: gray[100],
  /** Card and control borders. */
  line: gray[200],
  /** Separators inside a card, table rows. */
  lineSoft: '#eceef3',
  /** Body text and titles. */
  ink: gray[900],
  /** Labels, secondary text. */
  ink2: gray[600],
  /** Metadata, table headers, help. */
  ink3: gray[500],
  /** Placeholders, disabled text and icons; never for text meant to be read. */
  ink4: gray[400],
  gray,
  primary,
  /** Logo dot and the workspace badge gradient only, never in the interface. */
  brandPink: '#e91e63',
  /** antd's status colours: icons, badges and messages. Too light for white text. */
  status: {
    info: '#2563eb',
    success: '#16a34a',
    warning: '#d97706',
    danger: '#dc2626'
  },
  /** Background and text of the status pills. */
  tones: {
    neutral: { bg: gray[100], fg: gray[600] },
    primary: { bg: primary[100], fg: primary[700] },
    info: { bg: '#eaf2fe', fg: '#1d4ed8' },
    success: { bg: '#e9f7ef', fg: '#15803d' },
    warning: { bg: '#fff4e5', fg: '#b45309' },
    danger: { bg: '#fdecec', fg: '#b91c1c' }
  },
  /** ECharts series, read by the web analytics and dashboard charts. */
  chart: {
    primary: primary[500],
    comparison: '#b4b8c7',
    series2: '#0ea5e9',
    series3: '#14b8a6'
  }
} as const

/** `alpha('#6a5af9', 0.5)` → `rgba(106, 90, 249, 0.5)`. */
export function alpha(hex: string, opacity: number): string {
  const match = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex)
  if (!match) throw new Error(`alpha() expects a 6-digit hex colour, got ${hex}`)
  const [r, g, b] = match.slice(1).map((channel) => parseInt(channel, 16))
  return `rgba(${r}, ${g}, ${b}, ${opacity})`
}

export const typography = {
  sans: "'Inter Variable', Inter, system-ui, -apple-system, 'Segoe UI', sans-serif",
  mono: "'JetBrains Mono Variable', ui-monospace, 'SF Mono', Menlo, monospace",
  /** Sizes in px, by role. */
  size: {
    overline: 10.5,
    '2xs': 11.5,
    xs: 12,
    sm: 12.5,
    base: 13,
    subtitle: 15,
    kpi: 17,
    title: 18
  },
  lineHeight: {
    overline: 1.3,
    '2xs': 1.3,
    xs: 1.4,
    sm: 1.45,
    base: 1.45,
    subtitle: 1.35,
    kpi: 1.35,
    title: 1.3
  }
} as const

/** Radii in px; named like Tailwind's so a token and its class read the same. */
export const radii = { xs: 3, sm: 4, md: 6, lg: 8 } as const

export const shadows = {
  card: '0 1px 2px rgb(17 19 30 / .04)',
  popup: '0 8px 24px -6px rgb(17 19 30 / .18), 0 0 0 1px rgb(17 19 30 / .06)',
  /** 2px focus ring of the controls. */
  focus: alpha(primary[500], 0.18)
} as const

/** Durations in ms. */
export const motion = { fast: 120, slow: 180 } as const

/** Structural sizes in px. */
export const layout = {
  sidebarHead: 48,
  sidebar: 228,
  sidebarCollapsed: 56,
  rail: 220,
  aside: 320,
  sidepanel: 520
} as const
