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

  it('open and close the full-width drawers without their slide, ending on a transition', () => {
    const css = stripComments(read('theme/antd-overrides.css'))
    expect(css).toMatch(
      /\.app-drawer-instant \.ant-drawer-mask,\s*\.app-drawer-instant \.ant-drawer-content-wrapper\s*\{\s*transition-duration: 1ms;/
    )
    // Every full-width drawer opts in.
    const fullWidth = sourceFiles(SRC).filter((file) => /size=(\{'100%'\}|"100%")/.test(read(file)))
    expect(fullWidth.filter((file) => !read(file).includes('app-drawer-instant'))).toEqual([])
    expect(sourceFiles(SRC).filter((file) => read(file).includes('drawer-no-transition'))).toEqual([])
  })

  it('fold the sidebar without the active bar, and colour its icons from the theme', () => {
    const css = stripComments(read('theme/antd-overrides.css'))
    expect(css).toMatch(/\.ant-menu-inline-collapsed \.ant-menu-item-selected\s*\{\s*box-shadow: none;/)
    expect(css).toMatch(/\.app-sidebar-nav \.ant-menu \.ant-menu-item-icon\s*\{\s*color: var\(--ant-color-text-tertiary\);/)
    expect(css).toMatch(/\.app-sidebar-nav\.is-collapsed \.ant-menu-inline-collapsed\s*\{\s*width: 100%;/)
  })

  it('fold and open the sidebar menu without the width, padding and label transitions of antd', () => {
    const css = stripComments(read('theme/antd-overrides.css'))
    const rule = /(\.app-sidebar-nav \.ant-menu\.ant-menu-root[^{}]*)\{\s*transition: none;\s*\}/.exec(css)
    expect(rule).not.toBeNull()
    const scoped = rule![1].split(',').map((selector) => selector.trim())
    // Each outranks antd's own rule for the same element, ".ant-menu-inline.ant-menu-root .ant-menu-item"
    // being the most specific of them.
    expect(scoped).toEqual([
      '.app-sidebar-nav .ant-menu.ant-menu-root',
      '.app-sidebar-nav .ant-menu.ant-menu-root .ant-menu-item',
      '.app-sidebar-nav .ant-menu.ant-menu-root .ant-menu-submenu-title',
      '.app-sidebar-nav .ant-menu.ant-menu-root .ant-menu-item-icon',
      '.app-sidebar-nav .ant-menu.ant-menu-root .ant-menu-item-icon + span',
      '.app-sidebar-nav .ant-menu.ant-menu-root .ant-menu-title-content'
    ])
  })

  it('give a sorted fixed column an opaque fill, on both sides', () => {
    const css = stripComments(read('theme/antd-overrides.css'))
    expect(css).toMatch(
      /\.ant-table-wrapper td\.ant-table-cell-fix\.ant-table-column-sort\s*\{\s*background: var\(--ant-color-bg-container\);/
    )
  })

  it('keep the hover of an open row in the Table\'s own hover colour', () => {
    const css = stripComments(read('theme/antd-overrides.css'))
    expect(css).toMatch(/\.ant-table-tbody > tr\.is-hover > td\s*\{\s*background: var\(--ant-table-row-hover-bg\);/)
  })

  it('take the right border off the sidebar menu only', () => {
    const css = stripComments(read('theme/antd-overrides.css'))
    expect(css).toMatch(/\.app-sidebar-nav \.ant-menu-root\.ant-menu-inline\s*\{\s*border-inline-end: 0;/)
  })

  it('hand a plain link\'s colour back to its classes, and mark the live page\'s back link plain', () => {
    const css = stripComments(read('theme/antd-overrides.css'))
    expect(css).toMatch(
      /a\[data-plain-link\],\s*a\[data-plain-link\]:hover,\s*a\[data-plain-link\]:active\s*\{\s*color: revert-layer;/
    )
    expect(read('pages/WebAnalyticsLivePage.tsx')).toMatch(/data-plain-link\s+className="[^"]*text-ink-3/)
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
      /:where\(h1, h2, h3, h4, h5, h6, p, ol, ul, dl, dt, dd\)\[data-unreset\] \{\s*margin: revert-layer;\s*font-weight: revert-layer;\s*\}/
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
