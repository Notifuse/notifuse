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
  website_url: string
  logo_url: string | null
  cover_url: string | null
  timezone: string
  file_manager?: FileManagerSettings
}

export interface FileManagerSettings {
  endpoint: string
  access_key: string
  bucket: string
  region?: string
  secret_key?: string
  cdn_endpoint?: string
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
  settings?: {
    website_url?: string
    logo_url?: string | null
    cover_url?: string | null
    timezone?: string
  }
}

export interface UpdateWorkspaceResponse {
  workspace: Workspace
}

export interface DeleteWorkspaceRequest {
  id: string
}

export interface DeleteWorkspaceResponse {
  status: string
}

// Workspace Member types
export interface WorkspaceMember {
  user_id: string
  workspace_id: string
  role: string
  email: string
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
  variant_id: string
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
  description?: string
  content: string
  content_type: 'html' | 'plain'
  subject: string
  created_at: string
  updated_at: string
}

export interface GetTemplatesRequest {
  workspace_id: string
}

export interface GetTemplateRequest {
  workspace_id: string
  id: string
}

export interface CreateTemplateRequest {
  workspace_id: string
  name: string
  description?: string
  content: string
  content_type: 'html' | 'plain'
  subject: string
}

export interface UpdateTemplateRequest {
  workspace_id: string
  id: string
  name: string
  description?: string
  content: string
  content_type: 'html' | 'plain'
  subject: string
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
