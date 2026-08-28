import { api } from './client'
import type { EmailBlock } from '../../components/email_builder/types'
import type { UserPermissions, StoredPermissions } from './permissions'

// Template Block type
export interface TemplateBlock {
  id: string
  name: string
  block: EmailBlock
  created: string
  updated: string
}

// SEO Settings type (matches blog.go's SEOSettings)
export interface SEOSettings {
  meta_title?: string
  meta_description?: string
  og_title?: string
  og_description?: string
  og_image?: string
  canonical_url?: string
  keywords?: string[]
  meta_robots?: string
}

// Blog Settings type (styling + SEO for blog)
export interface BlogSettings {
  title?: string
  logo_url?: string
  icon_url?: string
  styling?: Record<string, unknown> // EditorStyleConfig - stored as JSON
  seo?: SEOSettings
  home_page_size?: number
  category_page_size?: number
  feed_summary_only?: boolean
  feed_max_items?: number
}

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
  custom_field_labels?: Record<string, string>
  blog_enabled?: boolean
  blog_settings?: BlogSettings
  web_analytics?: import('./web_analytics').WebAnalyticsSettings
  default_language: string
  languages: string[]
}

export interface FileManagerSettings {
  provider?: string
  endpoint: string
  access_key: string
  bucket: string
  region?: string
  secret_key?: string
  encrypted_secret_key?: string
  cdn_endpoint?: string
  force_path_style?: boolean
}

export type EmailProviderKind = 'smtp' | 'ses' | 'sparkpost' | 'postmark' | 'mailgun' | 'mailjet' | 'sendgrid'

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
  sendgrid?: SendGridSettings
  senders: Sender[]
  rate_limit_per_minute: number
}

export interface AmazonSES {
  region: string
  access_key: string
  secret_key?: string
  encrypted_secret_key?: string

  /**
   * Managed tenant isolation: Notifuse provisions a tenant for this integration with its own
   * reputation profile and suppression list. Mutually exclusive with tenant_name.
   */
  tenant_isolation_enabled?: boolean

  /** Advanced: use a configuration set you manage instead of the one Notifuse creates. */
  configuration_set_name?: string
  /** Advanced: use a tenant you manage yourself. */
  tenant_name?: string

  /** Server-owned, read-only: written by webhook registration. */
  managed_configuration_set?: string
  /** Server-owned, read-only: written when isolation is provisioned. */
  managed_tenant_name?: string
}

export type SMTPAuthType = 'basic' | 'oauth2'
export type SMTPOAuth2Provider = 'microsoft' | 'google'

export interface SMTPSettings {
  host: string
  port: number
  username: string
  password?: string
  encrypted_password?: string
  encrypted_username?: string
  use_tls: boolean
  ehlo_hostname?: string

  // Authentication type: 'basic' (default) or 'oauth2'
  auth_type?: SMTPAuthType

  // OAuth2 fields
  oauth2_provider?: SMTPOAuth2Provider // 'microsoft' or 'google'
  oauth2_tenant_id?: string // Microsoft only
  oauth2_client_id?: string
  oauth2_client_secret?: string
  encrypted_oauth2_client_secret?: string
  oauth2_refresh_token?: string // Google only
  encrypted_oauth2_refresh_token?: string // Google only
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
  message_stream?: string
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

export interface SendGridSettings {
  api_key?: string
  encrypted_api_key?: string
}

export type IntegrationType =
  | 'email'
  | 'sms'
  | 'whatsapp'
  | 'supabase'
  | 'llm'
  | 'firecrawl'
  | 'zapier'

// LLM Provider types
export type LLMProviderKind = 'anthropic' | 'openai' | 'gemini'

export interface AnthropicSettings {
  api_key?: string
  encrypted_api_key?: string
  model: string
}

export interface OpenAISettings {
  api_key?: string
  encrypted_api_key?: string
  model: string
  base_url?: string
  reasoning_effort?: string // '', none, minimal, low, medium, high, xhigh
}

export interface GeminiSettings {
  api_key?: string
  encrypted_api_key?: string
  model: string
}

export interface LLMProvider {
  kind: LLMProviderKind
  anthropic?: AnthropicSettings
  openai?: OpenAISettings
  gemini?: GeminiSettings
}

// Firecrawl settings for web scraping and search
export interface FirecrawlSettings {
  api_key?: string
  encrypted_api_key?: string
  base_url?: string
}

/**
 * Everything a Zapier connection records: the address of the API key minted for it when the
 * card was added.
 *
 * Display only — nothing on either side reads it to make a decision — and immutable, so a card
 * renamed later keeps the address its key was minted under, and the two can read differently.
 * Not a credential: the token exists once, in the response to workspaces.connectZapier.
 */
export interface ZapierSettings {
  api_key_email: string
}

export interface SupabaseAuthEmailHookSettings {
  signature_key?: string
  encrypted_signature_key?: string
}

export interface SupabaseUserCreatedHookSettings {
  signature_key?: string
  encrypted_signature_key?: string
  add_user_to_lists?: string[] // Array of list IDs
  custom_json_field?: string
  reject_disposable_email?: boolean // Reject user creation if email is disposable
}

export interface SupabaseIntegrationSettings {
  auth_email_hook: SupabaseAuthEmailHookSettings
  before_user_created_hook: SupabaseUserCreatedHookSettings
}

export interface Integration {
  id: string
  name: string
  type: IntegrationType
  email_provider?: EmailProvider
  supabase_settings?: SupabaseIntegrationSettings
  llm_provider?: LLMProvider
  firecrawl_settings?: FirecrawlSettings
  zapier_settings?: ZapierSettings
  /**
   * Last few characters of each configured credential, keyed like
   * "smtp.password" or "mailjet.secret_key". Read-only: the server computes it
   * on every read and never stores it. The credentials themselves are not returned.
   */
  credential_hints?: Record<string, string>
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
  // Omitted means full access, mirroring the server default.
  permissions?: UserPermissions
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
  /**
   * Zapier is excluded because the server rejects it here at two layers: a Zapier record only
   * exists alongside the API key that workspaces.connectZapier mints for it, and this request
   * cannot mint one. Widening IntegrationType without this would make the call compile and fail
   * at runtime with a 400.
   */
  type: Exclude<IntegrationType, 'zapier'>
  provider?: EmailProvider
  supabase_settings?: SupabaseIntegrationSettings
  llm_provider?: LLMProvider
  firecrawl_settings?: FirecrawlSettings
}

export interface UpdateIntegrationRequest {
  workspace_id: string
  integration_id: string
  name: string
  provider?: EmailProvider
  supabase_settings?: SupabaseIntegrationSettings
  llm_provider?: LLMProvider
  firecrawl_settings?: FirecrawlSettings
  // No zapier_settings, deliberately. The server rebuilds the integration from scratch on every
  // update and refills the settings from the stored record, so a field here could only express a
  // wipe of the minted address. Renaming a card is the whole of what an update does to one.
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

/**
 * Connecting Zapier mints an API key and records it as an integration, in one call.
 *
 * The label is all the caller chooses. It names the card and seeds the address of the key, which
 * the server derives itself so no client can claim an address belonging to a key it did not
 * create — and it carries no permission scope, because the grant is domain.ZapierKeyPermissions'
 * to choose.
 */
export interface ConnectZapierRequest {
  workspace_id: string
  label: string
}

export interface ConnectZapierResponse {
  status: string
  /** Shown once. Nothing stores it, so a caller that drops it cannot ask for it again. */
  token: string
  /** Address of the key just minted, also recorded on the integration as api_key_email. */
  email: string
  integration_id: string
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
  invitation_expires_at?: string
  invitation_id?: string
  // Received, not constructed: the stored map may be partial, and synthesised invitation rows
  // carry null. Read it through `?? createEmptyPermissions()`.
  permissions: StoredPermissions | undefined
}

export interface GetWorkspaceMembersResponse {
  members: WorkspaceMember[]
}

// Workspace Member Invitation types
export interface InviteMemberRequest {
  workspace_id: string
  email: string
  permissions: UserPermissions
}

export interface InviteMemberResponse {
  status: string
  message: string
}

// Permission types live in ./permissions, which imports nothing — the constructors are called at
// module scope by consumers that sit in an import cycle with ./client. Only the types are
// re-exported here, because most call sites already import them from this module.
export type {
  ResourcePermissions,
  PermissionResource,
  UserPermissions,
  StoredPermissions
} from './permissions'

// Set User Permissions types
export interface SetUserPermissionsRequest {
  workspace_id: string
  user_id: string
  permissions: UserPermissions
}

export interface SetUserPermissionsResponse {
  status: string
  message: string
}

export interface SetCustomFieldLabelsRequest {
  workspace_id: string
  custom_field_labels: Record<string, string>
}

export interface SetCustomFieldLabelsResponse {
  status: string
  message: string
}

export interface SetBlogSettingsRequest {
  workspace_id: string
  // Absent leaves the stored flag as it is; only the controls that exist to turn
  // the blog on or off send one.
  blog_enabled?: boolean
  blog_settings?: BlogSettings
}

export interface SetBlogSettingsResponse {
  status: string
  message: string
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

export interface DeleteInvitationRequest {
  invitation_id: string
}

export interface DeleteInvitationResponse {
  status: string
  message: string
}

interface DetectFaviconResponse {
  iconUrl: string
  coverUrl?: string
}

export const workspaceService = {
  list: () => api.get<ListWorkspacesResponse>('/api/workspaces.list'),

  get: (id: string) => api.get<GetWorkspaceResponse>(`/api/workspaces.get?id=${id}`),

  create: (data: CreateWorkspaceRequest) =>
    api.post<CreateWorkspaceResponse>('/api/workspaces.create', data),

  update: (data: UpdateWorkspaceRequest) =>
    api.post<UpdateWorkspaceResponse>('/api/workspaces.update', data),

  delete: (data: DeleteWorkspaceRequest) =>
    api.post<DeleteWorkspaceResponse>('/api/workspaces.delete', data),

  detectFavicon: (url: string) => api.post<DetectFaviconResponse>('/api/detect-favicon', { url }),

  getMembers: (id: string) =>
    api.get<GetWorkspaceMembersResponse>(`/api/workspaces.members?id=${id}`),

  inviteMember: (data: InviteMemberRequest) =>
    api.post<InviteMemberResponse>('/api/workspaces.inviteMember', data),

  createAPIKey: (data: CreateAPIKeyRequest) =>
    api.post<CreateAPIKeyResponse>('/api/workspaces.createAPIKey', data),

  removeMember: (data: RemoveMemberRequest) =>
    api.post<RemoveMemberResponse>('/api/workspaces.removeMember', data),

  // Integration endpoints
  createIntegration: (data: CreateIntegrationRequest) =>
    api.post<CreateIntegrationResponse>('/api/workspaces.createIntegration', data),

  updateIntegration: (data: UpdateIntegrationRequest) =>
    api.post<UpdateIntegrationResponse>('/api/workspaces.updateIntegration', data),

  deleteIntegration: (data: DeleteIntegrationRequest) =>
    api.post<DeleteIntegrationResponse>('/api/workspaces.deleteIntegration', data),

  // The label is trimmed here rather than at the one screen that calls this, because the server
  // takes it verbatim as the card's name and accepts a blank-looking one: "   " would mint a real
  // key and leave a card with nothing to read on it.
  connectZapier: (data: ConnectZapierRequest) =>
    api.post<ConnectZapierResponse>('/api/workspaces.connectZapier', {
      ...data,
      label: data.label.trim()
    }),

  // Invitation endpoints
  verifyInvitationToken: (token: string) =>
    api.post<VerifyInvitationTokenResponse>('/api/workspaces.verifyInvitationToken', { token }),

  acceptInvitation: (token: string) =>
    api.post<AcceptInvitationResponse>('/api/workspaces.acceptInvitation', { token }),

  deleteInvitation: (data: DeleteInvitationRequest) =>
    api.post<DeleteInvitationResponse>('/api/workspaces.deleteInvitation', data),

  setUserPermissions: (data: SetUserPermissionsRequest) =>
    api.post<SetUserPermissionsResponse>('/api/workspaces.setUserPermissions', data),

  setCustomFieldLabels: (data: SetCustomFieldLabelsRequest) =>
    api.post<SetCustomFieldLabelsResponse>('/api/workspaces.setCustomFieldLabels', data),

  setBlogSettings: (data: SetBlogSettingsRequest) =>
    api.post<SetBlogSettingsResponse>('/api/workspaces.setBlogSettings', data)
}
