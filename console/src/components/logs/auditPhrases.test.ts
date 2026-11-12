import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { lingui } from '@lingui/vite-plugin'
import { generateMessageId } from '@lingui/message-utils/generateMessageId'
import { describe, expect, it } from 'vitest'

// The phrases are chosen at run time, so the build's macro step must keep `<0>{target}</0>` in
// each message: that is the id the catalogs hold and the placeholder Trans fills.
interface MacroTransformHook {
  handler: (
    this: { environment: { config: { isProduction: boolean } } },
    code: string,
    id: string
  ) => Promise<{ code: string }>
}

const configPath = join(import.meta.dirname, '..', '..', '..', 'lingui.config.ts')
const macroPlugin = lingui({ macroTransform: true, configPath }).find(
  (plugin) => plugin.name === 'vite-plugin-lingui-macro-transform'
)
const macroTransform = macroPlugin?.transform as unknown as MacroTransformHook
const file = join(import.meta.dirname, 'auditPhrases.ts')

const compile = async (isProduction: boolean) =>
  (
    await macroTransform.handler.call(
      { environment: { config: { isProduction } } },
      readFileSync(file, 'utf8'),
      file
    )
  ).code

const MESSAGE = 'Created the template <0>{target}</0>'

describe('AUDIT_PHRASES through the macro step', () => {
  it('keeps the target placeholder in the message', async () => {
    expect(await compile(false)).toContain(JSON.stringify(MESSAGE).slice(1, -1))
  })

  it('names the message in production by the id of that same text', async () => {
    expect(await compile(true)).toContain(generateMessageId(MESSAGE))
  })
})
