import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'fs'
import { join, relative, resolve } from 'path'
import { PRIMARY_COLOR, COMPARISON_COLOR } from '../components/web_analytics/lib/types'

const SRC = resolve(__dirname, '..')
const read = (path: string) => readFileSync(join(SRC, path), 'utf8')

// The console's interface colours come from tokens.ts: no source file writes the old violet or the old #F9F9F9
// background by hand, except these, whose colours are not the console's
const EXCEPTIONS = [
  // The themes of published blogs, which keep their own palette
  'components/blog/themePresets.ts',
  // The colour broadcast annotations are written with, kept in step with domain.AnnotationBroadcastColor
  'components/web_analytics/annotations/AnnotationFormModal.tsx'
]

const OLD_COLOURS = /#7763f1|rgba\(\s*119\s*,\s*99\s*,\s*241|#f9f9f9/i

function sourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry)
    if (statSync(path).isDirectory()) return sourceFiles(path)
    return /\.(tsx?|css)$/.test(entry) ? [relative(SRC, path)] : []
  })
}

describe('hard-coded colours', () => {
  it('no source file writes the old violet or #F9F9F9, tests and the listed exceptions aside', () => {
    const offenders = sourceFiles(SRC).filter(
      (file) => !/\.test\.tsx?$/.test(file) && !EXCEPTIONS.includes(file) && OLD_COLOURS.test(read(file))
    )
    expect(offenders).toEqual([])
  })

  it.each(EXCEPTIONS)('%s still writes an old colour, so its exception is still needed', (file) => {
    expect(read(file)).toMatch(OLD_COLOURS)
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
