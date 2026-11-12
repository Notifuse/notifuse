import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'fs'
import { join, relative, resolve } from 'path'
import { PRIMARY_COLOR, COMPARISON_COLOR } from '../components/web_analytics/lib/types'

const SRC = resolve(__dirname, '..')
const read = (path: string) => readFileSync(join(SRC, path), 'utf8')

// The console's interface colours come from tokens.ts. These files used to write the old violet
// or the old #F9F9F9 background by hand.
const MIGRATED = [
  'App.tsx',
  'index.css',
  'layouts/WorkspaceLayout.tsx',
  'components/settings/SettingsSidebar.tsx',
  'components/blog/BlogSidebar.tsx',
  'components/automations/nodes/BaseNode.tsx',
  'components/broadcasts/BroadcastLinkStats.tsx',
  'components/web_analytics/CountryMapView.tsx',
  'components/web_analytics/TrafficHeatmapWidget.tsx',
  'components/web_analytics/lib/types.ts'
]

const OLD_COLOURS = /#7763f1|rgba\(\s*119\s*,\s*99\s*,\s*241|#f9f9f9/i

function sourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry)
    if (statSync(path).isDirectory()) return entry === 'identity' ? [] : sourceFiles(path)
    return /\.(tsx?|css)$/.test(entry) ? [relative(SRC, path)] : []
  })
}

describe('hard-coded colours', () => {
  it.each(MIGRATED)('%s writes neither the old violet nor #F9F9F9', (file) => {
    expect(read(file)).not.toMatch(OLD_COLOURS)
  })

  it('charts take the primary and comparison colours from the tokens', () => {
    expect(PRIMARY_COLOR).toBe('#6a5af9')
    expect(COMPARISON_COLOR).toBe('#b4b8c7')
    const types = read('components/web_analytics/lib/types.ts')
    expect(types).toMatch(/from '\.\.\/\.\.\/\.\.\/theme\/tokens'/)
    const lines = types.split('\n').filter((line) => /PRIMARY_COLOR|COMPARISON_COLOR/.test(line))
    for (const line of lines) expect(line).not.toMatch(/#[0-9a-f]{3,8}\b/i)
  })

  it('nothing reads the :root variables removed from index.css', () => {
    const offenders = sourceFiles(SRC).filter((file) =>
      /var\(--(primary|background|foreground)\)/.test(read(file))
    )
    expect(offenders).toEqual([])
  })
})
