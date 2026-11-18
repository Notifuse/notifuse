// The colours a segment can be given are antd's preset names (button_upsert.tsx). Most are also CSS
// colour names, but `volcano` and `geekblue` are not, so a dot painted with the bare name had no
// colour at all. These are the presets' primary shades (antd's colour-6).
const PRESETS: Record<string, string> = {
  magenta: '#eb2f96',
  red: '#f5222d',
  volcano: '#fa541c',
  orange: '#fa8c16',
  gold: '#faad14',
  lime: '#a0d911',
  green: '#52c41a',
  cyan: '#13c2c2',
  blue: '#1677ff',
  geekblue: '#2f54eb',
  purple: '#722ed1'
}

/** The colour of a segment's dot: its preset's shade, a CSS colour as it is, else the form's default preset. */
export function segmentDotColor(color?: string | null): string {
  const value = color?.trim().toLowerCase()
  if (!value) return PRESETS.blue
  return PRESETS[value] ?? color!.trim()
}

/** The colours a segment can be given, in the order the picker offers them; blue is the default. */
export const SEGMENT_COLORS = [
  'magenta',
  'red',
  'volcano',
  'orange',
  'gold',
  'lime',
  'green',
  'cyan',
  'blue',
  'geekblue',
  'purple',
  'grey'
] as const

export const DEFAULT_SEGMENT_COLOR = 'blue'
