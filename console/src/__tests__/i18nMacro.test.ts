import { readFileSync, readdirSync } from 'node:fs'
import { join, relative } from 'node:path'
import { transformSync } from '@babel/core'
import { lingui } from '@lingui/vite-plugin'
import { describe, it, expect } from 'vitest'

// The Lingui macro rewrites t`…` only where `t` resolves to the binding useLingui()
// returned. Hand `t` to a helper as a parameter and the tagged templates inside it are
// left alone — and what that parameter holds at runtime is i18n._, which answers "" to a
// tagged-template call. The strings do not fall back to English; they vanish, in every
// language, and nothing in this suite can see it because the tests mock the macro away.
//
// So this compiles the console with the build's own macro step and asserts nothing
// survives it: the transform hook of lingui({ macroTransform: true }), as vite.config.ts
// configures it, called with its own filter on every file the build would hand it. The
// output is then parsed, and any translation tag still applied to a template is a string
// that will vanish.

const SRC = join(import.meta.dirname, '..')
const configPath = join(SRC, '..', 'lingui.config.ts')

interface MacroTransformHook {
  filter: { id: RegExp; code: RegExp }
  handler: (this: { environment: { config: { isProduction: boolean } } }, code: string, id: string) => Promise<{ code: string }>
}

const macroPlugin = lingui({ macroTransform: true, configPath }).find(
  (plugin) => plugin.name === 'vite-plugin-lingui-macro-transform'
)
if (!macroPlugin) throw new Error('lingui({ macroTransform: true }) registered no macro transform')
const macroTransform = macroPlugin.transform as unknown as MacroTransformHook

// The production build, whose descriptors keep only the message id.
const productionBuild = { environment: { config: { isProduction: true } } }

const compile = async (file: string, source: string): Promise<string> => {
  if (!macroTransform.filter.id.test(file) || !macroTransform.filter.code.test(source)) return source
  const result: unknown = await macroTransform.handler.call(productionBuild, source, file)
  if (typeof result !== 'object' || result === null || typeof (result as { code?: unknown }).code !== 'string') {
    throw new Error(`@lingui/vite-plugin's macro transform no longer returns { code } (${where(file)}): update this test`)
  }
  return (result as { code: string }).code
}

// The plugin's filter is `from '<macro package>'`; without the `from ` it matches the
// package named anywhere, so the two counts below compare like with like.
const MACRO_IMPORTS = new RegExp(macroTransform.filter.code.source, 'g')
const MACRO_PACKAGES = new RegExp(macroTransform.filter.code.source.replace(/^from /, ''), 'g')

interface TaggedTemplatePath {
  node: {
    tag: { type: string; name?: string }
    quasi: { quasis: { value: { raw: string } }[] }
    loc: { start: { line: number } } | null
  }
}

const TRANSLATION_TAGS = new Set(['t', 'translate', '_'])

// Every module under src/, minus tests and the compiled catalogs; compile() applies the
// plugin's own id filter on top.
const sources: Map<string, string> = new Map(
  readdirSync(SRC, { recursive: true, encoding: 'utf8' })
    .filter(
      (f) =>
        /\.(?:[jt]sx?|[cm][jt]s)$/.test(f) &&
        !/\.test\.[jt]sx?$/.test(f) &&
        !f.startsWith('__tests__') &&
        !f.startsWith(join('i18n', 'locales'))
    )
    .map((f) => [join(SRC, f), readFileSync(join(SRC, f), 'utf8')])
)

const where = (file: string): string => join('src', relative(SRC, file))

// Offsets where `needle` occurs in `source` not preceded by an identifier character.
const sourceOffsets = (source: string, needle: string): number[] => {
  const found: number[] = []
  for (let at = source.indexOf(needle); at >= 0; at = source.indexOf(needle, at + 1)) {
    if (!/[\w$]/.test(source[at - 1] ?? '')) found.push(at)
  }
  return found
}

const untransformedIn = async (file: string, source: string): Promise<string[]> => {
  if (!source.includes('`')) return []

  const code = await compile(file, source)
  const compiled = code !== source

  const found: string[] = []
  transformSync(code, {
    filename: file,
    // TypeScript is stripped and JSX kept by the transform; the parser takes both.
    parserOpts: { plugins: ['typescript', 'jsx'] },
    plugins: [
      () => ({
        visitor: {
          TaggedTemplateExpression(path: TaggedTemplatePath) {
            const { tag, quasi, loc } = path.node
            if (tag.type === 'Identifier' && tag.name && TRANSLATION_TAGS.has(tag.name)) {
              const text = quasi.quasis.map((q) => q.value.raw).join('${…}').slice(0, 60)
              // A compiled file's line numbers are not the source's: there the line is
              // only known when the template's first chunk is non-empty and found once in
              // the source, as the start of an identifier.
              const chunk = quasi.quasis[0]?.value.raw ?? ''
              const at = chunk ? sourceOffsets(source, `${tag.name}\`${chunk}`) : []
              const line = !compiled
                ? String(loc?.start.line ?? '?')
                : at.length === 1
                  ? String(source.slice(0, at[0]).split('\n').length)
                  : '?'
              found.push(`${where(file)}:${line} — ${tag.name}\`${text}\``)
            }
          }
        }
      })
    ],
    // Parsing and visiting only: nothing reads the generated code.
    code: false,
    configFile: false,
    babelrc: false
  })
  return found
}

describe('Lingui macro coverage', () => {
  // Explicit timeout: this compiles every console source file, so it is CPU-bound and
  // scales with the machine. A GitHub runner (two shared vCPUs, with the rest of the suite
  // competing for them) once took several times longer than a dev laptop and blew vitest's
  // 5s default. Raised here rather than globally so the rest of the suite keeps a tight
  // bound and a genuine hang still surfaces as one.
  it('leaves no translation tagged template untransformed', async () => {
    const found = await Promise.all([...sources].map(([file, source]) => untransformedIn(file, source)))
    expect(found.flat()).toEqual([])
  }, 60_000)

  // The plugin only compiles a file whose source matches its import filter, and only static
  // imports at that. A macro package named any other way (an import the filter misses, a
  // dynamic import, a require) reaches the bundle uncompiled and throws on first use.
  it('names macro packages only in imports the build compiles', () => {
    const missed = [...sources]
      .filter(([, source]) => (source.match(MACRO_PACKAGES) ?? []).length > (source.match(MACRO_IMPORTS) ?? []).length)
      .map(([file]) => where(file))
    expect(missed).toEqual([])
  })
})
