import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'fs'
import { join, relative, resolve } from 'path'

const SRC = resolve(__dirname, '..')
const read = (path: string) => readFileSync(join(SRC, path), 'utf8')
const stripComments = (css: string) => css.replace(/\/\*[\s\S]*?\*\//g, '')

function sourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry)
    if (statSync(path).isDirectory()) return sourceFiles(path)
    return /\.(ts|tsx)$/.test(path) && !path.endsWith('antdOverrides.test.ts') ? [relative(SRC, path)] : []
  })
}

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
  // Checked below, rule by rule.
  /^index\.css$/
]

// Console classes that scope the antd rules allowed to stay in index.css.
const INDEX_SCOPES = [
  '.drawer-body-no-padding',
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

  it('leave no rule or class of the former sider', () => {
    const offenders = [...cssFiles(SRC), ...sourceFiles(SRC)].filter((file) =>
      read(file).includes('workspace-sider')
    )
    expect(offenders).toEqual([])
  })

  it('fold the sidebar without the active bar, and colour its icons from the theme', () => {
    const css = stripComments(read('theme/antd-overrides.css'))
    expect(css).toMatch(/\.ant-menu-inline-collapsed \.ant-menu-item-selected\s*\{\s*box-shadow: none;/)
    expect(css).toMatch(/\.app-sidebar-nav \.ant-menu \.ant-menu-item-icon\s*\{\s*color: var\(--ant-color-text-tertiary\);/)
    expect(css).toMatch(/\.app-sidebar-nav\.is-collapsed \.ant-menu-inline-collapsed\s*\{\s*width: 100%;/)
  })

  it('put the full-width drawers on the canvas', () => {
    const css = stripComments(read('theme/antd-overrides.css'))
    expect(css).toMatch(
      /\.app-full-drawer \.ant-drawer-section,\s*\.app-full-drawer \.ant-drawer-body\s*\{\s*background: var\(--ant-color-bg-layout\);/
    )
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

  it('load the fonts from main.tsx, and the overrides after index.css', () => {
    const main = read('main.tsx')
    expect(main).toMatch(/import '\.\/theme\/fonts\.css'/)
    expect(main).toMatch(/import '\.\/theme\/antd-overrides\.css'/)
    expect(main.indexOf("'./index.css'")).toBeLessThan(main.indexOf("'./theme/antd-overrides.css'"))
    // index.css gives pre/code/kbd/samp the code face over antd's reset, at equal specificity.
    expect(main.indexOf("'antd/dist/reset.css'")).toBeGreaterThan(-1)
    expect(main.indexOf("'antd/dist/reset.css'")).toBeLessThan(main.indexOf("'./index.css'"))
  })

  it('hand the margin and weight of data-unreset elements back from the unlayered reset to Tailwind', () => {
    const css = stripComments(read('index.css'))
    expect(css).toMatch(
      /:where\(h1, h2, h3, h4, h5, h6, p, dl, dt, dd\)\[data-unreset\] \{\s*margin: revert-layer;\s*font-weight: revert-layer;\s*\}/
    )
  })

  it('declare each latin-ext face before the latin face of the same family and style', () => {
    // The face declared last wins a code point both unicode-ranges list: it must be the latin one.
    const fonts = stripComments(read('theme/fonts.css'))
    const faces = [
      ...fonts.matchAll(
        /url\(@fontsource-variable\/[a-z-]+\/files\/([a-z-]+?)-(latin(?:-ext)?)-wght-(normal|italic)/g
      )
    ]
    const order = faces.map((m) => `${m[1]} ${m[3]} ${m[2]}`)
    const pairs = new Set(faces.map((m) => `${m[1]} ${m[3]}`))
    expect(pairs.size).toBeGreaterThan(0)
    for (const pair of pairs) {
      expect(order.indexOf(`${pair} latin-ext`)).toBeGreaterThanOrEqual(0)
      expect(order.indexOf(`${pair} latin-ext`)).toBeLessThan(order.indexOf(`${pair} latin`))
    }
  })

  it('embed the latin and latin-ext subsets of the fonts only', () => {
    const fonts = stripComments(read('theme/fonts.css'))
    const urls = [...fonts.matchAll(/url\(([^)]+)\)/g)].map((m) => m[1])
    expect(urls.length).toBeGreaterThan(0)
    for (const url of urls) expect(url).toMatch(/@fontsource-variable\/[a-z-]+\/files\/[a-z-]+-latin(-ext)?-wght-(normal|italic)\.woff2/)
  })

  it('keep the email canvas rules at the specificity of the global rules they replace', () => {
    const editor = stripComments(read('components/email_builder/editor.css'))
    expect(editor).toMatch(/:where\(\.email-canvas\) h1\s*\{/)
    expect(editor).toMatch(/:where\(\.email-canvas\) a\s*\{/)
    expect(editor).toMatch(/:where\(\.email-canvas\) a:hover\s*\{/)
  })
})
