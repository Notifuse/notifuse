import { templatesApi } from '../../services/api/template'
import type {
  CompileTemplateRequest,
  CompileTemplateResponse,
  EmailTemplate
} from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import type { EmailBlock } from '../email_builder/types'

export interface TemplatePreviewInput {
  workspace: Workspace
  /** The email of a template, or of one of its translations. */
  email: EmailTemplate | undefined
  /** Liquid data to render with; `{}` when absent. */
  testData?: Record<string, unknown> | null
}

/** Why no compile request could be built: no content in the editor mode, or a tree that is not JSON. */
export type TemplatePreviewErrorCode = 'missing_content' | 'invalid_tree'

export type TemplatePreviewRequestResult =
  | { request: CompileTemplateRequest }
  | { error: TemplatePreviewErrorCode }

export class TemplatePreviewError extends Error {
  readonly code: TemplatePreviewErrorCode

  constructor(code: TemplatePreviewErrorCode) {
    super(code)
    this.name = 'TemplatePreviewError'
    this.code = code
  }
}

/**
 * The templates.compile request that previews an email as it is sent: the MJML source in code
 * mode, the visual tree otherwise. The subject and preview text travel with it so the server
 * renders them with the Liquid engine the send uses.
 */
export function buildTemplatePreviewRequest({
  workspace,
  email,
  testData
}: TemplatePreviewInput): TemplatePreviewRequestResult {
  const isCodeMode = email?.editor_mode === 'code'
  if (!email || (isCodeMode ? !email.mjml_source : !email.visual_editor_tree)) {
    return { error: 'missing_content' }
  }

  const request: CompileTemplateRequest = {
    workspace_id: workspace.id,
    message_id: 'preview',
    subject: email.subject,
    subject_preview: email.subject_preview,
    // The custom text of the language previewed; a translation without one receives the
    // automatic version, never the text of the default language.
    ...(email.text?.trim() ? { text: email.text } : {}),
    test_data: testData || {},
    tracking_settings: {
      enable_tracking: workspace.settings?.email_tracking_enabled || false,
      endpoint: workspace.settings?.custom_endpoint_url || undefined,
      workspace_id: workspace.id,
      message_id: 'preview'
    }
  }

  if (isCodeMode) {
    request.mjml_source = email.mjml_source
    return { request }
  }

  let tree: unknown = email.visual_editor_tree
  if (typeof tree === 'string') {
    try {
      tree = JSON.parse(tree)
    } catch {
      return { error: 'invalid_tree' }
    }
  }
  // A string such as "null" or "42" parses, but is no tree.
  if (!tree || typeof tree !== 'object') return { error: 'invalid_tree' }
  request.visual_editor_tree = tree as EmailBlock
  return { request }
}

/** Compiles the preview of an email; rejects with a TemplatePreviewError when no request can be built. */
export async function compileTemplatePreview(input: TemplatePreviewInput): Promise<CompileTemplateResponse> {
  const result = buildTemplatePreviewRequest(input)
  if ('error' in result) throw new TemplatePreviewError(result.error)
  return templatesApi.compile(result.request)
}
