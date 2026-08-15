import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { transformSync } from '@babel/core'
import { describe, it, expect } from 'vitest'

// The Lingui macro rewrites t`…` only where `t` resolves to the binding useLingui()
// returned. Hand `t` to a helper as a parameter and the tagged templates inside it are
// left alone — and what that parameter holds at runtime is i18n._, which answers "" to a
// tagged-template call. The strings do not fall back to English; they vanish, in every
// language, and nothing in this suite can see it because the tests mock the macro away.
//
// So this runs the real macro over the console and asserts nothing survives it. The
// visitor is listed after the macro plugin, so anything the macro rewrote is already a
// call expression by the time the visitor sees the file.

const SRC = join(import.meta.dirname, '..')

interface TaggedTemplatePath {
  node: {
    tag: { type: string; name?: string }
    loc: { start: { line: number } } | null
  }
}

const TRANSLATION_TAGS = new Set(['t', 'translate', '_'])

const sourceFiles = (): string[] =>
  readdirSync(SRC, { recursive: true, encoding: 'utf8' })
    .filter((f) => /\.tsx?$/.test(f) && !/\.test\.tsx?$/.test(f) && !f.startsWith('__tests__'))
    .map((f) => join(SRC, f))

const untransformedIn = (file: string): string[] => {
  const source = readFileSync(file, 'utf8')
  if (!source.includes('`')) return []

  const found: string[] = []
  transformSync(source, {
    filename: file,
    // parserOpts rather than a syntax plugin: @babel/parser ships inside @babel/core,
    // which @lingui/babel-plugin-lingui-macro itself depends on.
    parserOpts: { plugins: ['typescript', 'jsx'] },
    plugins: [
      '@lingui/babel-plugin-lingui-macro',
      () => ({
        visitor: {
          TaggedTemplateExpression(path: TaggedTemplatePath) {
            const { tag, loc } = path.node
            if (tag.type === 'Identifier' && tag.name && TRANSLATION_TAGS.has(tag.name)) {
              const where = file.slice(file.indexOf('/src/') + 1)
              found.push(`${where}:${loc?.start.line ?? '?'} — ${tag.name}\`…\``)
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
  it('leaves no translation tagged template untransformed', () => {
    expect(sourceFiles().flatMap(untransformedIn)).toEqual([])
  })
})
