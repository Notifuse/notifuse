import { api } from './client'
import type { EmailBlock } from '../../components/email_builder/types'

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
  rate_limit_per_minute: number
}

export interface AmazonSES {
  region: string
  access_key: string
  secret_key?: string
  encrypted_secret_key?: string
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
  invitation_expires_at?: string
  invitation_id?: string
  permissions: UserPermissions
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

// Permission types
export interface ResourcePermissions {
  read: boolean
  write: boolean
}

export interface UserPermissions {
  contacts: ResourcePermissions
  lists: ResourcePermissions
  templates: ResourcePermissions
  broadcasts: ResourcePermissions
  transactional: ResourcePermissions
  workspace: ResourcePermissions
  message_history: ResourcePermissions
}

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

  // Invitation endpoints
  verifyInvitationToken: (token: string) =>
    api.post<VerifyInvitationTokenResponse>('/api/workspaces.verifyInvitationToken', { token }),

  acceptInvitation: (token: string) =>
    api.post<AcceptInvitationResponse>('/api/workspaces.acceptInvitation', { token }),

  deleteInvitation: (data: DeleteInvitationRequest) =>
    api.post<DeleteInvitationResponse>('/api/workspaces.deleteInvitation', data),

  setUserPermissions: (data: SetUserPermissionsRequest) =>
    api.post<SetUserPermissionsResponse>('/api/workspaces.setUserPermissions', data)
}
