import { readFileSync, readdirSync } from 'node:fs'
import { basename, join } from 'node:path'
import { transformSync } from '@babel/core'
import { getConfig } from '@lingui/conf'
import { mapMacroOptions, transform } from '@lingui/native-tools'
import { describe, it, expect } from 'vitest'

// The Lingui macro rewrites t`…` only where `t` resolves to the binding useLingui()
// returned. Hand `t` to a helper as a parameter and the tagged templates inside it are
// left alone — and what that parameter holds at runtime is i18n._, which answers "" to a
// tagged-template call. The strings do not fall back to English; they vanish, in every
// language, and nothing in this suite can see it because the tests mock the macro away.
//
// So this compiles the console the way the build does and asserts nothing survives it:
// vite.config.ts runs @lingui/vite-plugin's native macro transform, which only visits the
// files whose source matches the plugin's macro import filter, and leaves every other
// file as written. The output is then parsed, and any translation tag still applied to a
// template is a string that will vanish.

const SRC = join(import.meta.dirname, '..')

const config = getConfig({ configPath: join(SRC, '..', 'lingui.config.ts') })

// The same options @lingui/vite-plugin passes to the transform (descriptorFields aside,
// which only decides which fields the compiled descriptors keep).
const macroOptions = mapMacroOptions(config)

const macroPackages = [...new Set([...config.macro.corePackage, ...config.macro.jsxPackage])]
const escaped = macroPackages.map((id) => id.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')
// The filter @lingui/vite-plugin builds from the same config (buildMacroFilterRe).
const MACRO_IMPORT = new RegExp(`from ['"](?:${escaped})['"]`)
const MACRO_IMPORTS = new RegExp(MACRO_IMPORT.source, 'g')
const MACRO_PACKAGES = new RegExp(`['"](?:${escaped})['"]`, 'g')

interface TaggedTemplatePath {
  node: {
    tag: { type: string; name?: string }
    quasi: { quasis: { value: { raw: string } }[] }
  }
}

const TRANSLATION_TAGS = new Set(['t', 'translate', '_'])

// Every module the plugin's transform would visit, minus tests and the compiled catalogs.
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

const relative = (file: string): string => file.slice(file.indexOf('/src/') + 1)

// The output's line numbers are not the source's, so the string is located in the source.
const sourceLine = (source: string, tag: string, head: string): string => {
  const at = source.indexOf(`${tag}\`${head}`)
  return at < 0 ? '?' : String(source.slice(0, at).split('\n').length)
}

const untransformedIn = async (file: string, source: string): Promise<string[]> => {
  if (!source.includes('`')) return []

  const code = MACRO_IMPORT.test(source)
    ? (
        await transform(source, basename(file), {
          macro: { descriptorFields: 'all', ...macroOptions },
          sourceMaps: false
        })
      ).code
    : source

  const found: string[] = []
  transformSync(code, {
    filename: file,
    // TypeScript is stripped and JSX kept by the transform; the parser takes both.
    parserOpts: { plugins: ['typescript', 'jsx'] },
    plugins: [
      () => ({
        visitor: {
          TaggedTemplateExpression(path: TaggedTemplatePath) {
            const { tag, quasi } = path.node
            if (tag.type === 'Identifier' && tag.name && TRANSLATION_TAGS.has(tag.name)) {
              const head = quasi.quasis[0]?.value.raw ?? ''
              const text = quasi.quasis.map((q) => q.value.raw).join('${…}').slice(0, 60)
              found.push(`${relative(file)}:${sourceLine(source, tag.name, head)} — ${tag.name}\`${text}\``)
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
      .map(([file]) => relative(file))
    expect(missed).toEqual([])
  })
})
