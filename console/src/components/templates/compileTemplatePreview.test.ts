import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  buildTemplatePreviewRequest,
  compileTemplatePreview,
  TemplatePreviewError
} from './compileTemplatePreview'
import { templatesApi } from '../../services/api/template'
import type { EmailTemplate } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'

vi.mock('../../services/api/template', () => ({
  templatesApi: { compile: vi.fn() }
}))

const workspace = {
  id: 'ws-1',
  name: 'Acme',
  settings: {
    timezone: 'UTC',
    email_tracking_enabled: true,
    custom_endpoint_url: 'https://links.acme.io'
  },
  integrations: []
} as unknown as Workspace

const tree = { id: 'root', type: 'mjml', children: [] }

const visualEmail = {
  subject: 'Hello {{ contact.first_name }}',
  subject_preview: 'Inside: news',
  visual_editor_tree: tree
} as unknown as EmailTemplate

describe('buildTemplatePreviewRequest', () => {
  it('sends the MJML source in code mode, and no tree', () => {
    const email = {
      editor_mode: 'code',
      mjml_source: '<mjml></mjml>',
      visual_editor_tree: tree,
      subject: 'S'
    } as unknown as EmailTemplate
    const result = buildTemplatePreviewRequest({ workspace, email })
    expect('request' in result && result.request.mjml_source).toBe('<mjml></mjml>')
    expect('request' in result && result.request.visual_editor_tree).toBeUndefined()
  })

  it('sends the tree in visual mode, given as an object', () => {
    const result = buildTemplatePreviewRequest({ workspace, email: visualEmail })
    expect(result).toEqual({
      request: {
        workspace_id: 'ws-1',
        message_id: 'preview',
        subject: 'Hello {{ contact.first_name }}',
        subject_preview: 'Inside: news',
        test_data: {},
        tracking_settings: {
          enable_tracking: true,
          endpoint: 'https://links.acme.io',
          workspace_id: 'ws-1',
          message_id: 'preview'
        },
        visual_editor_tree: tree
      }
    })
  })

  it('parses a tree stored as a JSON string', () => {
    const email = { ...visualEmail, visual_editor_tree: JSON.stringify(tree) } as unknown as EmailTemplate
    const result = buildTemplatePreviewRequest({ workspace, email })
    expect('request' in result && result.request.visual_editor_tree).toEqual(tree)
  })

  it('reports an unparsable tree', () => {
    const email = { ...visualEmail, visual_editor_tree: '{not json' } as unknown as EmailTemplate
    expect(buildTemplatePreviewRequest({ workspace, email })).toEqual({ error: 'invalid_tree' })
  })

  it.each(['null', '0', 'false', '"text"'])('reports a tree string that parses to %s as invalid', (value) => {
    const email = { ...visualEmail, visual_editor_tree: value } as unknown as EmailTemplate
    expect(buildTemplatePreviewRequest({ workspace, email })).toEqual({ error: 'invalid_tree' })
  })

  it('reads the content of the editor mode only', () => {
    // Code mode with a tree but no source, visual mode with a source but no tree.
    expect(
      buildTemplatePreviewRequest({
        workspace,
        email: { editor_mode: 'code', visual_editor_tree: tree, subject: 'S' } as unknown as EmailTemplate
      })
    ).toEqual({ error: 'missing_content' })
    expect(
      buildTemplatePreviewRequest({
        workspace,
        email: { mjml_source: '<mjml></mjml>', subject: 'S' } as unknown as EmailTemplate
      })
    ).toEqual({ error: 'missing_content' })
  })

  it('reports missing content in either mode', () => {
    expect(
      buildTemplatePreviewRequest({ workspace, email: { subject: 'S' } as unknown as EmailTemplate })
    ).toEqual({ error: 'missing_content' })
    expect(
      buildTemplatePreviewRequest({
        workspace,
        email: { editor_mode: 'code', subject: 'S' } as unknown as EmailTemplate
      })
    ).toEqual({ error: 'missing_content' })
    expect(buildTemplatePreviewRequest({ workspace, email: undefined })).toEqual({
      error: 'missing_content'
    })
  })

  it('sends the custom text only when it is not blank', () => {
    const blank = buildTemplatePreviewRequest({
      workspace,
      email: { ...visualEmail, text: '   ' } as unknown as EmailTemplate
    })
    expect('request' in blank).toBe(true)
    expect('request' in blank && 'text' in blank.request).toBe(false)
    const custom = buildTemplatePreviewRequest({
      workspace,
      email: { ...visualEmail, text: 'Plain {{ x }}' } as unknown as EmailTemplate
    })
    expect('request' in custom && custom.request.text).toBe('Plain {{ x }}')
  })

  it('passes the test data given', () => {
    const result = buildTemplatePreviewRequest({
      workspace,
      email: visualEmail,
      testData: { contact: { first_name: 'Ada' } }
    })
    expect('request' in result && result.request.test_data).toEqual({ contact: { first_name: 'Ada' } })
  })

  it('leaves tracking off and the endpoint out when the workspace sets neither', () => {
    const bare = { ...workspace, settings: { timezone: 'UTC' } } as unknown as Workspace
    const result = buildTemplatePreviewRequest({ workspace: bare, email: visualEmail })
    expect('request' in result && result.request.tracking_settings).toEqual({
      enable_tracking: false,
      endpoint: undefined,
      workspace_id: 'ws-1',
      message_id: 'preview'
    })
  })
})

describe('compileTemplatePreview', () => {
  beforeEach(() => {
    vi.mocked(templatesApi.compile).mockReset()
  })

  it('compiles the request it builds', async () => {
    vi.mocked(templatesApi.compile).mockResolvedValue({ html: '<p>hi</p>', mjml: '<mjml/>' })
    const response = await compileTemplatePreview({ workspace, email: visualEmail })
    expect(response.html).toBe('<p>hi</p>')
    expect(templatesApi.compile).toHaveBeenCalledTimes(1)
    expect(templatesApi.compile).toHaveBeenCalledWith(
      expect.objectContaining({ workspace_id: 'ws-1', subject: 'Hello {{ contact.first_name }}', visual_editor_tree: tree })
    )
  })

  it('rejects with the code when no request can be built, without calling the server', async () => {
    const email = { ...visualEmail, visual_editor_tree: '{not json' } as unknown as EmailTemplate
    await expect(compileTemplatePreview({ workspace, email })).rejects.toMatchObject({
      code: 'invalid_tree'
    })
    await expect(compileTemplatePreview({ workspace, email })).rejects.toBeInstanceOf(
      TemplatePreviewError
    )
    expect(templatesApi.compile).not.toHaveBeenCalled()
  })
})
