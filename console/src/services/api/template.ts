import { api } from './client'
import type { EmailBlock } from '../../components/email_builder/types'
import type { EmailOptions } from './transactional_notifications'
import type { EmailProvider } from './workspace'

// Template types
export interface Template {
  id: string
  name: string
  version: number
  channel: 'email' | 'web'
  email?: EmailTemplate
  web?: WebTemplate
  category: string
  template_macro_id?: string
  integration_id?: string
  utm_source?: string
  utm_medium?: string
  utm_campaign?: string
  test_data?: Record<string, unknown>
  settings?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface EmailTemplate {
  sender_id?: string
  reply_to?: string
  subject: string
  subject_preview?: string
  compiled_preview: string // compiled html
  visual_editor_tree: EmailBlock
  text?: string
}

export interface WebTemplate {
  content?: unknown // Tiptap JSON (source of truth)
  html?: string // Pre-rendered HTML for display
  plain_text?: string // Extracted text for search indexing
}

export interface GetTemplatesRequest {
  workspace_id: string
  category?: string
  channel?: string
}

export interface GetTemplateRequest {
  workspace_id: string
  id: string
  version?: number
}

export interface CreateTemplateRequest {
  workspace_id: string
  id: string
  name: string
  channel: string
  email?: EmailTemplate
  web?: WebTemplate
  category: string
  template_macro_id?: string
  utm_source?: string
  utm_medium?: string
  utm_campaign?: string
  test_data?: Record<string, unknown>
  settings?: Record<string, unknown>
}

export interface UpdateTemplateRequest {
  workspace_id: string
  id: string
  name: string
  channel: string
  email?: EmailTemplate
  web?: WebTemplate
  category: string
  template_macro_id?: string
  utm_source?: string
  utm_medium?: string
  utm_campaign?: string
  test_data?: Record<string, unknown>
  settings?: Record<string, unknown>
}

export interface DeleteTemplateRequest {
  workspace_id: string
  id: string
}

export interface GetTemplatesResponse {
  templates: Template[]
}

export interface GetTemplateResponse {
  template: Template
}

export interface CreateTemplateResponse {
  template: Template
}

export interface UpdateTemplateResponse {
  template: Template
}

export interface DeleteTemplateResponse {
  status: string
}

// Represents a detail within an MJML compilation error
export interface MjmlErrorDetail {
  line: number
  message: string
  tagName: string
}

// Represents the structured error returned by the MJML compiler
export interface MjmlCompileError {
  message: string
  details: MjmlErrorDetail[]
}

export interface TrackingSettings {
  enable_tracking: boolean
  endpoint?: string
  utm_source?: string
  utm_medium?: string
  utm_campaign?: string
  utm_content?: string
  utm_term?: string
  workspace_id?: string
  message_id?: string
}

export interface CompileTemplateRequest {
  workspace_id: string
  message_id: string
  visual_editor_tree: EmailBlock
  test_data?: Record<string, unknown> | null
  tracking_settings?: TrackingSettings
  channel?: string // "email" or "web" - filters blocks by visibility
  preserve_liquid?: boolean // When true, skip Liquid template processing and preserve raw syntax
}

export interface CompileTemplateResponse {
  mjml: string
  html: string
  error?: MjmlCompileError // Use the structured error type, optional
}

export interface TestEmailProviderRequest {
  provider: EmailProvider
  to: string
  workspace_id: string
}

export interface TestEmailProviderResponse {
  success: boolean
  error?: string
}

// Test template types
export interface TestTemplateRequest {
  workspace_id: string
  template_id: string
  integration_id: string
  sender_id: string
  recipient_email: string
  email_options?: EmailOptions
}

export interface TestTemplateResponse {
  success: boolean
  error?: string
}

// Define the API interfaces
export interface TemplatesApi {
  list: (params: GetTemplatesRequest) => Promise<GetTemplatesResponse>
  get: (params: GetTemplateRequest) => Promise<GetTemplateResponse>
  create: (params: CreateTemplateRequest) => Promise<CreateTemplateResponse>
  update: (params: UpdateTemplateRequest) => Promise<UpdateTemplateResponse>
  delete: (params: DeleteTemplateRequest) => Promise<DeleteTemplateResponse>
  compile: (params: CompileTemplateRequest) => Promise<CompileTemplateResponse>
}

export const templatesApi: TemplatesApi = {
  list: async (params: GetTemplatesRequest): Promise<GetTemplatesResponse> => {
    let url = `/api/templates.list?workspace_id=${params.workspace_id}`
    if (params.category) {
      url += `&category=${params.category}`
    }
    if (params.channel) {
      url += `&channel=${params.channel}`
    }
    const response = await api.get<GetTemplatesResponse>(url)
    return response
  },
  get: async (params: GetTemplateRequest): Promise<GetTemplateResponse> => {
    const url = `/api/templates.get?workspace_id=${params.workspace_id}&id=${params.id}&version=${params.version || 0}`
    const response = await api.get<GetTemplateResponse>(url)
    return response
  },
  create: async (params: CreateTemplateRequest): Promise<CreateTemplateResponse> => {
    const response = await api.post<CreateTemplateResponse>(`/api/templates.create`, params)
    return response
  },
  update: async (params: UpdateTemplateRequest): Promise<UpdateTemplateResponse> => {
    const response = await api.post<UpdateTemplateResponse>(`/api/templates.update`, params)
    return response
  },
  delete: async (params: DeleteTemplateRequest): Promise<DeleteTemplateResponse> => {
    const response = await api.post<DeleteTemplateResponse>(`/api/templates.delete`, params)
    return response
  },
  compile: async (params: CompileTemplateRequest): Promise<CompileTemplateResponse> => {
    const response = await api.post<CompileTemplateResponse>(`/api/templates.compile`, params)
    return response
  }
}
