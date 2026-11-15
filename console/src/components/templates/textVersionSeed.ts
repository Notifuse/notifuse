import { templatesApi } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import type { EmailBlock } from '../email_builder/types'
import type { TextVersionSeed } from '../email_builder/panels/TextVersionPanel'

const countLiquidTags = (s: string) => (s.match(/\{%/g) || []).length

// The Liquid tags of the MJML that can reach its text: none in comments, in the head (the preview text, the
// title, the styles) or in the attributes of a tag. An attribute value may hold a ">" inside quotes.
const liquidTagsOfContent = (mjml: string) =>
  countLiquidTags(
    mjml
      .replace(/<!--[\s\S]*?-->/g, '')
      .replace(/<mj-head[\s>][\s\S]*?<\/mj-head>/g, '')
      .replace(/<[a-zA-Z/][^"'>]*(?:(?:"[^"]*"|'[^']*')[^"'>]*)*>/g, '')
  )

// The start of the notice plainTextVersion (pkg/notifuse_mjml/template_compilation.go) gives as text_error for
// Liquid written between the blocks of a section or a column: accepted markup that may land elsewhere, not an error
const PLACEMENT_NOTICE = 'Liquid placed between the blocks'

// A text that is empty or only whitespace is no custom text: the automatic version.
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
    const placement = !!response.text_error?.startsWith(PLACEMENT_NOTICE)
    return {
      text,
      textError: placement ? undefined : response.text_error,
      liquidPlacement: placement,
      // Tags the content of the MJML holds and the text does not were written between blocks, or are the
      // display conditions of blocks, which the compile drops.
      lostLiquid: liquidTagsOfContent(response.mjml || '') > countLiquidTags(text)
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
