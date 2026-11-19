import { msg, plural } from '@lingui/core/macro'
import type { I18n } from '@lingui/core'
import type { EmailBlock } from '../email_builder/types'
import { EmailBlockClass } from '../email_builder/EmailBlockClass'
import { conditionErrorsInTree } from '../email_builder/displayCondition'
import {
  importMjmlSource,
  MjmlImportError,
  type MjmlImportErrorReason
} from '../mjml-converter/mjml-to-json-browser'

/** Which picker the file came from: a Notifuse export, or MJML to convert to a visual tree. */
export type TemplateFileKind = 'json' | 'mjml'

export interface ImportWarning {
  title: string
  content: string
}

export type TemplateImport =
  | {
      ok: true
      tree: EmailBlock
      testData?: Record<string, unknown>
      /** Shown after the import, which still happens. */
      warnings: ImportWarning[]
    }
  | {
      ok: false
      /** With a title, the errors go to the validation modal; without, the only one to a toast. */
      title?: string
      errors: string[]
      /** For an MJML syntax error: the file's text, and the first line the parser names. */
      source?: string
      line?: number
    }

interface ExportedTemplate {
  emailTree: EmailBlock
  testData?: Record<string, unknown>
}

/** The checks of an imported tree, with the messages the editor has always shown. */
export function validateEmailTree(tree: unknown, i18n: I18n): string[] {
  if (!tree || typeof tree !== 'object') {
    return [i18n._(msg`Invalid tree structure: not an object`)]
  }
  if (!('id' in tree) || !('type' in tree)) {
    return [i18n._(msg`Invalid tree structure: missing id or type`)]
  }
  const node = tree as { id: unknown; type: unknown; children?: unknown }
  if (typeof node.id !== 'string' || typeof node.type !== 'string') {
    return [i18n._(msg`Invalid tree structure: missing id or type`)]
  }
  if (node.type !== 'mjml') {
    return [i18n._(msg`Invalid tree structure: root must be mjml type`)]
  }
  if (node.children && !Array.isArray(node.children)) {
    return [i18n._(msg`Invalid tree structure: children must be an array`)]
  }

  const structureErrors = EmailBlockClass.validateStructure(tree as EmailBlock)
  if (structureErrors.length > 0) {
    return structureErrors.map((error) => {
      if (error.includes('mj-raw cannot be placed inside mj-wrapper')) {
        return `${error}\n\n${i18n._(msg`Note: mj-raw components can only be placed directly in mj-body or mj-head, not inside mj-wrapper. Consider moving the mj-raw content outside the wrapper or converting it to appropriate MJML components.`)}`
      }
      if (error.includes('cannot be placed inside mj-raw')) {
        return `${error}\n\n${i18n._(msg`Note: This might be caused by an incorrectly structured MJML file. HTML content inside mj-raw should be stored as text content, not as child elements.`)}`
      }
      return error
    })
  }

  // Display conditions the server would refuse, and that would crash the editor if malformed.
  return conditionErrorsInTree(tree as EmailBlock)
}

function readText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error)
    reader.readAsText(file)
  })
}

function acceptsFile(file: File, kind: TemplateFileKind): boolean {
  if (kind === 'json') return file.type === 'application/json' || file.name.endsWith('.json')
  return (
    file.name.endsWith('.mjml') ||
    file.type === 'text/xml' ||
    file.type === 'application/xml' ||
    file.type === 'text/plain'
  )
}

function splitValidationMessage(error: unknown): string[] | null {
  if (error instanceof Error && error.message.includes('Validation failed:')) {
    return error.message.replace('Validation failed: ', '').split(', ')
  }
  return null
}

function importJson(content: string, i18n: I18n): TemplateImport {
  try {
    const parsed = JSON.parse(content)
    const exported = parsed && typeof parsed === 'object' && 'emailTree' in parsed
    const tree = (exported ? (parsed as ExportedTemplate).emailTree : parsed) as EmailBlock
    const testData = exported ? (parsed as ExportedTemplate).testData : undefined
    const errors = validateEmailTree(tree, i18n)
    if (errors.length > 0)
      return { ok: false, title: i18n._(msg`Import Validation Failed`), errors }
    return { ok: true, tree, ...(testData ? { testData } : {}), warnings: [] }
  } catch (error) {
    console.error('Import failed:', error)
    const errors = splitValidationMessage(error)
    if (errors) return { ok: false, title: i18n._(msg`Import Validation Failed`), errors }
    return {
      ok: false,
      errors: [i18n._(msg`Failed to import template. Please check the file format.`)]
    }
  }
}

/**
 * The first line an XML parser names in its error, in the three forms it comes in: libxml2 (Chrome,
 * Safari) « error on line 6 at column 12: … mj-text line 5 … », Firefox « Line Number 6, Column 3 »,
 * and jsdom « 6:12: unexpected close tag. » right after « Invalid MJML syntax: ». Only the first
 * number counts: libxml2 names the opening tag's line after it.
 */
export function syntaxErrorLine(message: string): number | undefined {
  const match =
    /error on line (\d+)/i.exec(message) ??
    /Line Number (\d+)/i.exec(message) ??
    /Invalid MJML syntax:\s*(\d+):\d+:/.exec(message)
  const line = match ? Number(match[1]) : NaN
  return Number.isInteger(line) && line > 0 ? line : undefined
}

/** What an MJML import error says, in the console's language. */
function importErrorMessage(error: MjmlImportError, i18n: I18n): string {
  const tag = error.subject
  const messages: Record<MjmlImportErrorReason, () => string> = {
    not_closed: () => i18n._(msg`<${tag}> is not closed.`),
    closes_nothing: () => i18n._(msg`</${tag}> closes no open tag.`),
    liquid_in_tag: () =>
      i18n._(msg`Liquid inside a tag cannot be imported into the visual editor.`),
    liquid_between_blocks: () =>
      i18n._(msg`Liquid between the blocks of <${tag}> cannot be imported into the visual editor.`),
    outside_root: () => i18n._(msg`Liquid or markup outside <mjml> cannot be imported.`),
    mark_not_closed: () =>
      i18n._(msg`This notifuse:${tag} mark of a Notifuse export is not closed.`),
    mark_not_opened: () =>
      i18n._(msg`This notifuse:${tag} mark of a Notifuse export closes no mark.`),
    condition_mark_invalid: () =>
      i18n._(
        msg`This notifuse:condition mark of a Notifuse export holds no block or no valid condition.`
      )
  }
  return messages[error.reason]()
}

function importMjml(content: string, i18n: I18n): TemplateImport {
  try {
    const { tree, liquidBlocksKept: count } = importMjmlSource(content)
    const errors = validateEmailTree(tree, i18n)
    if (errors.length > 0)
      return { ok: false, title: i18n._(msg`MJML Import Validation Failed`), errors }
    // Liquid written between blocks, without the marks of a Notifuse export, is kept in Liquid
    // blocks: nothing is lost, but those passages are no longer edited visually.
    const warnings =
      count > 0
        ? [
            {
              title: i18n._(msg`Liquid kept in Liquid blocks`),
              content: i18n._(
                msg`${plural(count, {
                  one: '# passage written between blocks was kept as a Liquid block: its Liquid is unchanged and it is edited as code.',
                  other:
                    '# passages written between blocks were kept as Liquid blocks: their Liquid is unchanged and they are edited as code.'
                })}`
              )
            }
          ]
        : []
    return { ok: true, tree, warnings }
  } catch (error) {
    console.error('MJML import failed:', error)
    if (error instanceof MjmlImportError) {
      return {
        ok: false,
        title: i18n._(msg`MJML Syntax Error`),
        errors: [importErrorMessage(error, i18n)],
        source: content,
        line: error.line
      }
    }
    if (!(error instanceof Error)) {
      return {
        ok: false,
        errors: [i18n._(msg`Failed to import MJML template. Please check the MJML syntax.`)]
      }
    }
    if (
      error.message.includes('Invalid MJML syntax') ||
      error.message.includes('Root element must be')
    ) {
      return {
        ok: false,
        title: i18n._(msg`MJML Syntax Error`),
        errors: [error.message],
        source: content,
        line: syntaxErrorLine(error.message)
      }
    }
    const errors = splitValidationMessage(error)
    if (errors) return { ok: false, title: i18n._(msg`MJML Validation Failed`), errors }
    return { ok: false, title: i18n._(msg`MJML Import Error`), errors: [error.message] }
  }
}

/**
 * Reads and checks a template file: a Notifuse export (a JSON tree, with or without its test
 * data) or an MJML file, converted to a visual tree. Nothing is saved: the caller puts the tree in
 * an editor.
 */
export async function importTemplateFile(
  file: File,
  kind: TemplateFileKind,
  i18n: I18n
): Promise<TemplateImport> {
  if (!acceptsFile(file, kind)) {
    return {
      ok: false,
      errors: [
        kind === 'json'
          ? i18n._(msg`Please select a JSON file`)
          : i18n._(msg`Please select an MJML file`)
      ]
    }
  }
  let content: string
  try {
    content = await readText(file)
  } catch {
    return { ok: false, errors: [i18n._(msg`Failed to read the file`)] }
  }
  return kind === 'json' ? importJson(content, i18n) : importMjml(content, i18n)
}

/** The name of a template imported from a file: the file name without its extension. */
export function templateNameFromFile(fileName: string): string {
  return fileName.replace(/\.[^.]+$/, '')
}
