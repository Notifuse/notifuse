import type { BlockInterface } from '../../components/email_editor/Block'

// Authentication types
export interface SignInRequest {
  email: string
}

export interface SignInResponse {
  message: string
  code?: string
}

export interface VerifyCodeRequest {
  email: string
  code: string
}

export interface VerifyResponse {
  token: string
}

export interface GetCurrentUserResponse {
  user: {
    id: string
    email: string
    timezone: string
  }
  workspaces: Workspace[]
}

// Workspace types
export interface WorkspaceSettings {
  website_url?: string
  logo_url?: string | null
  cover_url?: string | null
  timezone: string
  file_manager?: FileManagerSettings
  transactional_email_provider_id?: string
  marketing_email_provider_id?: string
}

export interface FileManagerSettings {
  endpoint: string
  access_key: string
  bucket: string
  region?: string
  secret_key?: string
  encrypted_secret_key?: string
  cdn_endpoint?: string
}

export type EmailProviderKind = 'smtp' | 'ses' | 'sparkpost' | 'postmark' | 'mailgun' | 'mailjet'

export interface EmailProvider {
  kind: EmailProviderKind
  ses?: AmazonSES
  smtp?: SMTPSettings
  sparkpost?: SparkPostSettings
  postmark?: PostmarkSettings
  mailgun?: MailgunSettings
  mailjet?: MailjetSettings
  default_sender_email: string
  default_sender_name: string
}

export interface AmazonSES {
  region: string
  access_key: string
  secret_key?: string
  encrypted_secret_key?: string
  sandbox_mode: boolean
}

export interface SMTPSettings {
  host: string
  port: number
  username: string
  password?: string
  encrypted_password?: string
  use_tls: boolean
}

export interface SparkPostSettings {
  api_key?: string
  encrypted_api_key?: string
  sandbox_mode: boolean
  endpoint: string
}

export interface PostmarkSettings {
  server_token?: string
  encrypted_server_token?: string
}

export interface MailgunSettings {
  api_key?: string
  encrypted_api_key?: string
  domain: string
  region?: 'US' | 'EU'
}

export interface MailjetSettings {
  api_key?: string
  encrypted_api_key?: string
  secret_key?: string
  encrypted_secret_key?: string
  sandbox_mode: boolean
}

export type IntegrationType = 'email' | 'sms' | 'whatsapp'

export interface Integration {
  id: string
  name: string
  type: IntegrationType
  email_provider: EmailProvider
  created_at: string
  updated_at: string
}

export interface CreateWorkspaceRequest {
  id: string
  name: string
  settings: WorkspaceSettings
}

export interface Workspace {
  id: string
  name: string
  settings: WorkspaceSettings
  integrations?: Integration[]
  created_at: string
  updated_at: string
}

export interface CreateWorkspaceResponse {
  workspace: Workspace
}

export interface ListWorkspacesResponse {
  workspaces: Workspace[]
}

export interface GetWorkspaceResponse {
  workspace: Workspace
}

export interface UpdateWorkspaceRequest {
  id: string
  name?: string
  settings?: Partial<WorkspaceSettings>
}

export interface UpdateWorkspaceResponse {
  workspace: Workspace
}

export interface CreateAPIKeyRequest {
  workspace_id: string
  email_prefix: string
}

export interface CreateAPIKeyResponse {
  token: string
  email: string
}

export interface RemoveMemberRequest {
  workspace_id: string
  user_id: string
}

export interface RemoveMemberResponse {
  status: string
  message: string
}

export interface DeleteWorkspaceRequest {
  id: string
}

export interface DeleteWorkspaceResponse {
  status: string
}

// Integration related types
export interface CreateIntegrationRequest {
  workspace_id: string
  name: string
  type: IntegrationType
  provider: EmailProvider
}

export interface UpdateIntegrationRequest {
  workspace_id: string
  integration_id: string
  name: string
  provider: EmailProvider
}

export interface DeleteIntegrationRequest {
  workspace_id: string
  integration_id: string
}

// Integration responses
export interface CreateIntegrationResponse {
  integration_id: string
}

export interface UpdateIntegrationResponse {
  status: string
}

export interface DeleteIntegrationResponse {
  status: string
}

// Workspace Member types
export interface WorkspaceMember {
  user_id: string
  workspace_id: string
  role: string
  email: string
  type: 'user' | 'api_key'
  created_at: string
  updated_at: string
}

export interface GetWorkspaceMembersResponse {
  members: WorkspaceMember[]
}

// Workspace Member Invitation types
export interface InviteMemberRequest {
  workspace_id: string
  email: string
  role: string
}

export interface InviteMemberResponse {
  status: string
  message: string
}

// List types
export interface TemplateReference {
  id: string
  version: number
}

export interface List {
  id: string
  name: string
  is_double_optin: boolean
  is_public: boolean
  description?: string
  total_active: number
  total_pending: number
  total_unsubscribed: number
  total_bounced: number
  total_complained: number
  double_optin_template?: TemplateReference
  welcome_template?: TemplateReference
  unsubscribe_template?: TemplateReference
  created_at: string
  updated_at: string
}

export interface CreateListRequest {
  workspace_id: string
  id: string
  name: string
  is_double_optin: boolean
  is_public: boolean
  description?: string
  double_optin_template?: TemplateReference
  welcome_template?: TemplateReference
  unsubscribe_template?: TemplateReference
}

export interface GetListsRequest {
  workspace_id: string
  with_templates?: boolean
}

export interface GetListRequest {
  workspace_id: string
  id: string
}

export interface UpdateListRequest {
  workspace_id: string
  id: string
  name: string
  is_double_optin: boolean
  is_public: boolean
  description?: string
  double_optin_template?: TemplateReference
  welcome_template?: TemplateReference
  unsubscribe_template?: TemplateReference
}

export interface DeleteListRequest {
  workspace_id: string
  id: string
}

export interface GetListsResponse {
  lists: List[]
}

export interface GetListResponse {
  list: List
}

export interface CreateListResponse {
  list: List
}

export interface UpdateListResponse {
  list: List
}

export interface DeleteListResponse {
  status: string
}

export type ContactListTotalType = 'pending' | 'unsubscribed' | 'bounced' | 'complained' | 'active'

// Template types
export interface Template {
  id: string
  name: string
  version: number
  channel: 'email'
  email?: EmailTemplate
  category: string
  template_macro_id?: string
  utm_source?: string
  utm_medium?: string
  utm_campaign?: string
  test_data?: Record<string, any>
  settings?: Record<string, any>
  created_at: string
  updated_at: string
}

export interface EmailTemplate {
  from_address: string
  from_name: string
  reply_to?: string
  subject: string
  subject_preview?: string
  mjml: string // html
  visual_editor_tree: BlockInterface
  text?: string
}

export interface GetTemplatesRequest {
  workspace_id: string
  category?: string
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
  email: EmailTemplate
  category: string
  template_macro_id?: string
  utm_source?: string
  utm_medium?: string
  utm_campaign?: string
  test_data?: Record<string, any>
  settings?: Record<string, any>
}

export interface UpdateTemplateRequest {
  workspace_id: string
  id: string
  name: string
  channel: string
  email: EmailTemplate
  category: string
  template_macro_id?: string
  utm_source?: string
  utm_medium?: string
  utm_campaign?: string
  test_data?: Record<string, any>
  settings?: Record<string, any>
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

export interface CompileTemplateRequest {
  workspace_id: string
  message_id: string
  visual_editor_tree: BlockInterface
  test_data?: Record<string, any> | null
  enable_tracking?: boolean
  utm_source?: string
  utm_medium?: string
  utm_campaign?: string
  utm_content?: string
  utm_term?: string
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
  recipient_email: string
  cc?: string[]
  bcc?: string[]
  reply_to?: string
}

export interface TestTemplateResponse {
  success: boolean
  error?: string
}
