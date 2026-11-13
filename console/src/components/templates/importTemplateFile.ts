import { msg } from '@lingui/core/macro'
import type { I18n } from '@lingui/core'
import type { EmailBlock } from '../email_builder/types'
import { EmailBlockClass } from '../email_builder/EmailBlockClass'
import { conditionErrorsInTree } from '../email_builder/displayCondition'
import {
  convertMjmlToJsonBrowser,
  hasDisplayConditionLiquid
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

function importMjml(content: string, i18n: I18n): TemplateImport {
  try {
    const tree = convertMjmlToJsonBrowser(content)
    const errors = validateEmailTree(tree, i18n)
    if (errors.length > 0)
      return { ok: false, title: i18n._(msg`MJML Import Validation Failed`), errors }
    const warnings = hasDisplayConditionLiquid(content)
      ? [
          {
            title: i18n._(msg`Display conditions not imported`),
            content: i18n._(
              msg`This file holds the display conditions of an export. They are not imported: the blocks they wrapped now show to every recipient. Add their conditions again in the block settings.`
            )
          }
        ]
      : []
    return { ok: true, tree, warnings }
  } catch (error) {
    console.error('MJML import failed:', error)
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
      return { ok: false, title: i18n._(msg`MJML Syntax Error`), errors: [error.message] }
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
