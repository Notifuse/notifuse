import { readFileSync, readdirSync } from 'node:fs'
import { basename, join } from 'node:path'
import { transformSync } from '@babel/core'
import { transform } from '@lingui/native-tools'
import { describe, it, expect } from 'vitest'

// The Lingui macro rewrites t`…` only where `t` resolves to the binding useLingui()
// returned. Hand `t` to a helper as a parameter and the tagged templates inside it are
// left alone — and what that parameter holds at runtime is i18n._, which answers "" to a
// tagged-template call. The strings do not fall back to English; they vanish, in every
// language, and nothing in this suite can see it because the tests mock the macro away.
//
// So this compiles the console the way the build does and asserts nothing survives it:
// vite.config.ts runs @lingui/vite-plugin's native macro transform, which only visits
// files whose source matches MACRO_IMPORT below, and leaves every other file as written.
// The output is then parsed, and any translation tag still applied to a template is a
// string that will vanish.

const SRC = join(import.meta.dirname, '..')

// The filter @lingui/vite-plugin builds from Lingui's default macro packages
// (buildMacroFilterRe, not exported). lingui.config.ts sets no `macro` option, so these
// are the packages it uses.
const MACRO_IMPORT = /from ['"](?:@lingui\/macro|@lingui\/core\/macro|@lingui\/react\/macro)['"]/
const MACRO_PACKAGE = /['"]@lingui\/(?:core\/|react\/)?macro['"]/

interface TaggedTemplatePath {
  node: {
    tag: { type: string; name?: string }
    quasi: { quasis: { value: { raw: string } }[] }
  }
}

const TRANSLATION_TAGS = new Set(['t', 'translate', '_'])

const sourceFiles = (): string[] =>
  readdirSync(SRC, { recursive: true, encoding: 'utf8' })
    .filter((f) => /\.tsx?$/.test(f) && !/\.test\.tsx?$/.test(f) && !f.startsWith('__tests__'))
    .map((f) => join(SRC, f))

const relative = (file: string): string => file.slice(file.indexOf('/src/') + 1)

const untransformedIn = async (file: string): Promise<string[]> => {
  const source = readFileSync(file, 'utf8')
  if (!source.includes('`')) return []

  // TypeScript is stripped and JSX kept, so the output still needs the jsx parser plugin.
  const code = MACRO_IMPORT.test(source)
    ? (await transform(source, basename(file), { sourceMaps: false })).code
    : source

  const found: string[] = []
  transformSync(code, {
    filename: file,
    // parserOpts rather than a syntax plugin: @babel/parser ships inside @babel/core.
    parserOpts: { plugins: ['typescript', 'jsx'] },
    plugins: [
      () => ({
        visitor: {
          TaggedTemplateExpression(path: TaggedTemplatePath) {
            const { tag, quasi } = path.node
            if (tag.type === 'Identifier' && tag.name && TRANSLATION_TAGS.has(tag.name)) {
              // The output's line numbers are not the source's, so name the string instead.
              const text = quasi.quasis.map((q) => q.value.raw).join('${…}').slice(0, 60)
              found.push(`${relative(file)} — ${tag.name}\`${text}\``)
            }
          }
        }
      })
    ],
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
    const found = await Promise.all(sourceFiles().map(untransformedIn))
    expect(found.flat()).toEqual([])
  }, 60_000)

  // The plugin skips a file whose macro import its filter does not recognise, and the macro
  // then reaches the bundle uncompiled, where it throws on first use.
  it('writes every macro import in a form the build compiles', () => {
    const missed = sourceFiles().filter((file) => {
      const source = readFileSync(file, 'utf8')
      return MACRO_PACKAGE.test(source) && !MACRO_IMPORT.test(source)
    })
    expect(missed.map(relative)).toEqual([])
  })
})
