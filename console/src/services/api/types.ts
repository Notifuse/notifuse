import type { EmailBlock } from '../../components/email_builder/types'
import { Contact } from './contacts'
import { EmailOptions } from './transactional_notifications'

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

// Template Block type
export interface TemplateBlock {
  id: string
  name: string
  block: EmailBlock
  created: string
  updated: string
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
  email_tracking_enabled: boolean
  template_blocks?: TemplateBlock[]
  custom_endpoint_url?: string
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

export interface Sender {
  id: string
  email: string
  name: string
  is_default: boolean
}

export interface EmailProvider {
  kind: EmailProviderKind
  ses?: AmazonSES
  smtp?: SMTPSettings
  sparkpost?: SparkPostSettings
  postmark?: PostmarkSettings
  mailgun?: MailgunSettings
  mailjet?: MailjetSettings
  senders: Sender[]
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

export interface ListStats {
  total_active: number
  total_pending: number
  total_unsubscribed: number
  total_bounced: number
  total_complained: number
}

export interface GetListStatsRequest {
  workspace_id: string
  list_id: string
}

export interface GetListStatsResponse {
  list_id: string
  stats: ListStats
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
  sender_id?: string
  reply_to?: string
  subject: string
  subject_preview?: string
  compiled_preview: string // compiled html
  visual_editor_tree: EmailBlock
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
  test_data?: Record<string, any> | null
  tracking_settings?: TrackingSettings
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

// Notification Center types
export interface NotificationCenterRequest {
  workspace_id: string
  email: string
  email_hmac: string
}

export interface NotificationCenterResponse {
  contact: {
    email: string
    first_name?: string
    last_name?: string
  }
  lists: {
    id: string
    name: string
    description?: string
    status: string
  }[]
  workspace: {
    id: string
    name: string
    logo_url?: string
    website_url?: string
  }
}

export interface SubscribeToListsRequest {
  workspace_id: string
  contact: Contact
  list_ids: string[]
}

export interface UnsubscribeFromListsRequest {
  workspace_id: string
  email: string
  email_hmac: string
  list_ids: string[]
}

// Task related types
export type TaskStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'

export interface SendBroadcastState {
  broadcast_id: string
  total_recipients: number
  sent_count: number
  failed_count: number
  channel_type: string
  recipient_offset: number
}

export interface TaskState {
  progress?: number
  message?: string
  send_broadcast?: SendBroadcastState
}

export interface Task {
  id: string
  workspace_id: string
  type: string
  status: TaskStatus
  progress: number
  state?: TaskState
  error_message?: string
  created_at: string
  updated_at: string
  last_run_at?: string
  completed_at?: string
  next_run_after?: string
  timeout_after?: string
  max_runtime: number
  max_retries: number
  retry_count: number
  retry_interval: number
  broadcast_id?: string
}

export interface GetTaskResponse {
  task: Task
}

export interface ListTasksResponse {
  tasks: Task[]
  total_count: number
  limit: number
  offset: number
  has_more: boolean
}

// Invitation types
export interface WorkspaceInvitation {
  id: string
  workspace_id: string
  inviter_id: string
  email: string
  expires_at: string
  created_at: string
  updated_at: string
}

export interface Workspace {
  id: string
  name: string
  settings: WorkspaceSettings
}

export interface User {
  id: string
  email: string
  name: string
  type: string
  created_at: string
  updated_at: string
}

export interface VerifyInvitationTokenResponse {
  status: string
  invitation: WorkspaceInvitation
  workspace: Workspace
  valid: boolean
}

export interface AcceptInvitationResponse {
  status: string
  message: string
  workspace_id: string
  email: string
  token: string
  user: User
  expires_at: string
}
