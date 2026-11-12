import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'fs'
import { join, relative, resolve } from 'path'

const SRC = resolve(__dirname, '..')
const read = (path: string) => readFileSync(join(SRC, path), 'utf8')
const stripComments = (css: string) => css.replace(/\/\*[\s\S]*?\*\//g, '')

function cssFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry)
    if (statSync(path).isDirectory()) return cssFiles(path)
    return path.endsWith('.css') ? [relative(SRC, path)] : []
  })
}

/** Selectors of every rule, comments removed (at-rule preludes are skipped). */
function selectors(css: string): string[] {
  const out: string[] = []
  const re = /([^{};]+)\{/g
  let match: RegExpExecArray | null
  while ((match = re.exec(stripComments(css)))) {
    const prelude = match[1].trim()
    if (prelude.startsWith('@')) continue
    out.push(...prelude.split(',').map((s) => s.trim()))
  }
  return out
}

// Files that style antd inside their own component, or that are the home of global antd rules.
const ALLOWED = [
  /^theme\/antd-overrides\.css$/,
  /^components\/email_builder\/editor\.css$/,
  /^components\/blog_editor\//,
  /^components\/segment\/input\.module\.css$/,
  /^identity\//,
  // Checked below, rule by rule.
  /^index\.css$/
]

// Console classes that scope the antd rules allowed to stay in index.css.
const INDEX_SCOPES = [
  '.drawer-body-no-padding',
  '.workspace-sider',
  '.tabs-in-header',
  '.vertical-tabs'
]

describe('global antd rules', () => {
  it('live in antd-overrides.css: no other stylesheet reaches into .ant-*', () => {
    const offenders = cssFiles(SRC)
      .filter((file) => !ALLOWED.some((re) => re.test(file)))
      .filter((file) => stripComments(read(file)).includes('.ant-'))
    expect(offenders).toEqual([])
  })

  it('in index.css, scope every .ant-* selector with a console class', () => {
    const unscoped = selectors(read('index.css'))
      .filter((selector) => selector.includes('.ant-'))
      .filter((selector) => !INDEX_SCOPES.some((scope) => selector.includes(scope)))
    expect(unscoped).toEqual([])
  })

  it('no longer round every popover from the blog toolbar stylesheet', () => {
    expect(stripComments(read('components/blog_editor/toolbars/floating-toolbar.css'))).not.toContain(
      '.ant-popover-inner'
    )
  })

  it('limit the block actions menu rules to that menu', () => {
    const css = read('components/blog_editor/menus/block-actions/block-actions.css')
    const dropdownSelectors = selectors(css).filter((selector) => selector.includes('.ant-dropdown'))
    expect(dropdownSelectors.length).toBeGreaterThan(0)
    for (const selector of dropdownSelectors) {
      // antd puts rootClassName on the .ant-dropdown element itself, hence the compound form.
      expect(selector).toMatch(/^\.ant-dropdown\.block-actions-dropdown\b/)
    }
    expect(read('components/blog_editor/menus/block-actions/BlockActionsMenu.tsx')).toMatch(
      /rootClassName="block-actions-dropdown"/
    )
  })

  it('load the fonts and the overrides from main.tsx, after index.css', () => {
    const main = read('main.tsx')
    expect(main).toMatch(/import '\.\/theme\/fonts\.css'/)
    expect(main).toMatch(/import '\.\/theme\/antd-overrides\.css'/)
    expect(main.indexOf("'./index.css'")).toBeLessThan(main.indexOf("'./theme/antd-overrides.css'"))
  })

  it('embed the latin subset of the fonts only', () => {
    const fonts = stripComments(read('theme/fonts.css'))
    const urls = [...fonts.matchAll(/url\(([^)]+)\)/g)].map((m) => m[1])
    expect(urls.length).toBeGreaterThan(0)
    for (const url of urls) expect(url).toMatch(/@fontsource-variable\/[a-z-]+\/files\/[a-z-]+-latin-wght-(normal|italic)\.woff2/)
  })

  it('keep the email canvas rules at the specificity of the global rules they replace', () => {
    const editor = stripComments(read('components/email_builder/editor.css'))
    expect(editor).toMatch(/:where\(\.email-canvas\) h1\s*\{/)
    expect(editor).toMatch(/:where\(\.email-canvas\) a\s*\{/)
    expect(editor).toMatch(/:where\(\.email-canvas\) a:hover\s*\{/)
  })
})
