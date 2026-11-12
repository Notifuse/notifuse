import { describe, it, expect } from 'vitest'
import { readFileSync } from 'fs'
import { resolve } from 'path'
import { colors, typography, radii, shadows, layout } from './tokens'

// Tailwind reads index.css, not tokens.ts, so the @theme block repeats the values by hand.
// This contract is what keeps the two from drifting.
const css = readFileSync(resolve(__dirname, '../index.css'), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '')

function themeBlock(source: string): Map<string, string> {
  const start = source.indexOf('@theme {')
  expect(start).toBeGreaterThanOrEqual(0)
  // The block may hold nested @keyframes, so match braces rather than stop at the first `}`.
  let depth = 0
  let end = start
  for (let i = source.indexOf('{', start); i < source.length; i++) {
    if (source[i] === '{') depth++
    if (source[i] === '}') depth--
    if (depth === 0) {
      end = i
      break
    }
  }
  const body = source.slice(source.indexOf('{', start) + 1, end).replace(/@keyframes[^{]*\{(?:[^{}]*\{[^{}]*\})*[^{}]*\}/g, '')
  const declarations = new Map<string, string>()
  for (const declaration of body.split(';')) {
    const colon = declaration.indexOf(':')
    if (colon < 0) continue
    const name = declaration.slice(0, colon).trim()
    if (!name.startsWith('--')) continue
    declarations.set(name, declaration.slice(colon + 1).trim())
  }
  return declarations
}

const px = (n: number) => `${n}px`

function expectedTheme(): Map<string, string> {
  const expected = new Map<string, string>([
    ['--font-sans', typography.sans],
    ['--font-mono', typography.mono],
    ['--color-canvas', colors.canvas],
    ['--color-surface', colors.surface],
    ['--color-subtle', colors.subtle],
    ['--color-fill', colors.fill],
    ['--color-line', colors.line],
    ['--color-line-soft', colors.lineSoft],
    ['--color-ink', colors.ink],
    ['--color-ink-2', colors.ink2],
    ['--color-ink-3', colors.ink3],
    ['--color-ink-4', colors.ink4],
    ['--color-primary', colors.primary[500]],
    ['--color-brand-pink', colors.brandPink],
    ['--shadow-card', shadows.card],
    ['--shadow-popup', shadows.popup],
    ['--spacing-sidebar-head', px(layout.sidebarHead)],
    ['--spacing-sidebar', px(layout.sidebar)],
    ['--spacing-sidebar-collapsed', px(layout.sidebarCollapsed)],
    ['--spacing-rail', px(layout.rail)],
    ['--spacing-aside', px(layout.aside)],
    ['--spacing-sidepanel', px(layout.sidepanel)]
  ])
  for (const [step, value] of Object.entries(colors.gray)) expected.set(`--color-gray-${step}`, value)
  for (const [step, value] of Object.entries(colors.primary)) expected.set(`--color-primary-${step}`, value)
  for (const [tone, { bg, fg }] of Object.entries(colors.tones)) {
    // The primary tone is drawn with primary-100 and primary-700, which the scale already declares.
    if (tone === 'primary') continue
    expected.set(`--color-${tone}-bg`, bg)
    expected.set(`--color-${tone}-fg`, fg)
  }
  for (const [role, size] of Object.entries(typography.size)) {
    expected.set(`--text-${role}`, px(size))
    expected.set(`--text-${role}--line-height`, String(typography.lineHeight[role as keyof typeof typography.lineHeight]))
  }
  for (const [name, value] of Object.entries(radii)) expected.set(`--radius-${name}`, px(value))
  return expected
}

// Hex colours compare case-insensitively; numbers compare as numbers (1.3 vs 1.30).
function normalize(value: string): string {
  const v = value.trim().toLowerCase().replace(/\s+/g, ' ')
  return /^-?\d*\.?\d+$/.test(v) ? String(Number(v)) : v
}

describe('Tailwind @theme contract', () => {
  const block = themeBlock(css)
  const expected = expectedTheme()

  it.each([...expected.entries()])('%s matches tokens.ts', (name, value) => {
    expect(block.has(name)).toBe(true)
    expect(normalize(block.get(name)!)).toBe(normalize(value))
  })

  it('declares nothing outside tokens.ts', () => {
    const extra = [...block.keys()].filter((name) => !expected.has(name) && !name.startsWith('--animate-'))
    expect(extra).toEqual([])
  })

  it('drops the duplicate :root variables and the dark colour scheme', () => {
    const root = css.slice(css.indexOf(':root {'), css.indexOf('}', css.indexOf(':root {')))
    expect(root).not.toMatch(/--background|--foreground|--primary\s*:/)
    expect(root).toMatch(/background-color:\s*var\(--color-canvas\)/)
    expect(root).toMatch(/color:\s*var\(--color-ink\)/)
    expect(root).toMatch(/color-scheme:\s*light;/)
    expect(root).toMatch(/font-family:\s*var\(--font-sans\)/)
  })

  it('colours the links outside antd\'s App in the base layer, so utilities still win over them', () => {
    expect(css).toMatch(/@layer base\s*\{[^@]*\ba\s*\{[^}]*var\(--color-primary-600\)/)
    const outsideBaseLayer = css.replace(/@layer base\s*\{[\s\S]*?\n\}/, '')
    expect(outsideBaseLayer).not.toMatch(/(^|\n)\s*a(:hover)?\s*\{/)
    expect(css).not.toMatch(/(^|\n)\s*h1\s*\{/)
  })
})
