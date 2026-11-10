import { templatesApi } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import type { EmailBlock } from '../email_builder/types'
import type { TextVersionSeed } from '../email_builder/panels/TextVersionPanel'

const countLiquidTags = (s: string) => (s.match(/\{%/g) || []).length

// A custom text is stored without surrounding blank-only values: "" or whitespace is the
// automatic version.
export const customTextOrUndefined = (text?: string | null): string | undefined =>
  text && text.trim() !== '' ? text : undefined

// seedTextVersion returns the automatic text of the email with its Liquid kept, as the
// starting point of a custom text. No test data and no tracking, so the text holds the
// template's variables and direct links rather than one contact's values and preview
// tracking tokens.
export async function seedTextVersion(
  workspace: Workspace,
  content: { visualEditorTree?: EmailBlock; mjmlSource?: string }
): Promise<TextVersionSeed> {
  try {
    const response = await templatesApi.compile({
      workspace_id: workspace.id,
      message_id: 'preview',
      ...(content.mjmlSource !== undefined
        ? { mjml_source: content.mjmlSource }
        : { visual_editor_tree: content.visualEditorTree }),
      channel: 'email',
      preserve_liquid: true,
      tracking_settings: {
        enable_tracking: false,
        workspace_id: workspace.id,
        message_id: 'preview'
      }
    })
    if (response.error) {
      return { error: response.error.message }
    }
    const text = response.text ?? ''
    return {
      text,
      textError: response.text_error,
      // Tags the MJML holds and the text does not were written between blocks, where
      // the compile drops them.
      lostLiquid: countLiquidTags(response.mjml || '') > countLiquidTags(text)
    }
  } catch (error) {
    return { error: (error as Error).message }
  }
}

// Variables a custom text should keep, by template category.
const REQUIRED_VARIABLE: Record<string, string> = {
  marketing: 'unsubscribe_url',
  opt_in: 'confirm_subscription_url'
}

// missingRequiredLink returns the variable a custom text of this category should hold
// and does not, or undefined.
export function missingRequiredLink(category: string | undefined, text?: string): string | undefined {
  const variable = category ? REQUIRED_VARIABLE[category] : undefined
  if (!variable || !customTextOrUndefined(text)) return undefined
  return text!.includes(variable) ? undefined : variable
}
