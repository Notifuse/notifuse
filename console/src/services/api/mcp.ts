import { queryOptions } from '@tanstack/react-query'
import { api } from './client'
import type { UserPermissions } from './types'

// MCP access to a workspace: the owner switch, and the connections applications made through the
// OAuth connection page with one of the workspace's API keys.

/** workspace.settings.mcp. Absent settings mean enabled. */
export interface MCPSettings {
  disabled: boolean
}

export interface SetMCPSettingsRequest {
  workspace_id: string
  disabled: boolean
}

export interface SetMCPSettingsResponse {
  mcp: MCPSettings
}

export type OAuthGrantStatus = 'active' | 'revoked'

/** How a connection was made: an API key pasted on the connection page, or an owner's shortcut. */
export type OAuthGrantCreatedVia = 'key' | 'owner_shortcut'

/** A connection, as GET /api/oauth.listWorkspaceGrants lists it (domain.OAuthGrantListItem). */
export interface OAuthGrant {
  id: string
  /** The user ID of the API key the connection was made with. */
  api_key_user_id: string
  /** The API key's address; its name is the part before "@". */
  api_key_email: string
  workspace_id: string
  /** The client's metadata document URL. */
  client_id: string
  client_name: string
  resource: string
  created_via: OAuthGrantCreatedVia
  created_by_user_id?: string
  created_at: string
  last_used_at?: string
  revoked_at?: string
  revoked_reason?: string
  status: OAuthGrantStatus
}

export interface ListWorkspaceGrantsResponse {
  grants: OAuthGrant[]
}

export interface RevokeGrantRequest {
  grant_id: string
}

export interface RevokeGrantResponse {
  success: boolean
}

/** Answered by revokeGrant for a connection that no longer exists. */
export const OAUTH_GRANT_NOT_FOUND = 'oauth_grant_not_found'

/**
 * Client hosts the connection page does not warn about (domain.OAuthKnownClientHosts). Any other
 * client is shown as unknown.
 */
export const KNOWN_OAUTH_CLIENT_HOSTS: readonly string[] = ['claude.ai', 'chatgpt.com']

/** The host of a client_id, or the client_id itself when it does not parse. */
export function oauthClientHost(clientId: string): string {
  try {
    return new URL(clientId).host.toLowerCase()
  } catch {
    return clientId
  }
}

/**
 * Whether a client_id is on a known host, as domain.IsKnownOAuthClient decides it: only ASCII
 * letters are folded, so no other character passes for a letter of a known host.
 */
export function isKnownOAuthClient(clientId: string): boolean {
  return KNOWN_OAUTH_CLIENT_HOSTS.some((host) => {
    const prefix = `https://${host}/`
    return clientId.slice(0, prefix.length).replace(/[A-Z]/g, (c) => c.toLowerCase()) === prefix
  })
}

/** A workspace the signed-in caller owns, for the owner shortcut of the connection page. */
export interface OAuthOwnedWorkspace {
  id: string
  name: string
  /** The workspace's MCP switch is off: listed, never selectable. */
  mcp_disabled: boolean
}

/** What the connection page shows (domain.OAuthAuthorizationRequestView). */
export interface OAuthAuthorizationRequest {
  request_id: string
  /** The client's metadata document URL. */
  client_id: string
  /** The name the client gives itself in its metadata document. */
  client_name: string
  /** The host of client_id. */
  client_host: string
  /** client_host is one of KNOWN_OAUTH_CLIENT_HOSTS. */
  known_client: boolean
  /** The host the browser returns to. */
  redirect_host: string
  /** The browser returns to a program on this computer (127.0.0.1, [::1], localhost). */
  loopback_redirect: boolean
  expires_at: string
  /** Only for a console session that owns workspaces. */
  owned_workspaces?: OAuthOwnedWorkspace[]
}

/** Where the connection page sends the browser once the request is approved or denied. */
export interface OAuthRedirect {
  redirect_url: string
}

export interface ApproveWithKeyRequest {
  request_id: string
  api_key: string
}

/** The owner shortcut: the key is created with these, as workspaces.createAPIKey creates one. */
export interface CreateKeyAndApproveRequest {
  request_id: string
  workspace_id: string
  /** The part of the key's address before "@". */
  name: string
  permissions: UserPermissions
}

export interface DenyAuthorizationRequest {
  request_id: string
}

/** The codes the connection page switches on, in the "error" key of a refused call. */
export const OAUTH_ERROR = {
  requestUnavailable: 'oauth_request_unavailable',
  browserMismatch: 'oauth_browser_mismatch',
  notAnAPIKey: 'oauth_not_an_api_key',
  workspaceMCPDisabled: 'oauth_workspace_mcp_disabled',
  keyNameTaken: 'oauth_key_name_taken',
  rateLimited: 'rate_limited',
  twoFactorRequired: 'two_factor_required'
} as const

/** The connection page's calls. The binding cookie rides along: the page and the API share an origin. */
export const oauthApi = {
  getAuthorizationRequest: (requestId: string): Promise<OAuthAuthorizationRequest> => {
    const params = new URLSearchParams({ request_id: requestId })
    return api.get<OAuthAuthorizationRequest>(`/api/oauth.getAuthorizationRequest?${params.toString()}`)
  },

  approveWithKey: (data: ApproveWithKeyRequest): Promise<OAuthRedirect> =>
    api.post<OAuthRedirect>('/api/oauth.approveWithKey', data),

  createKeyAndApprove: (data: CreateKeyAndApproveRequest): Promise<OAuthRedirect> =>
    api.post<OAuthRedirect>('/api/oauth.createKeyAndApprove', data),

  deny: (data: DenyAuthorizationRequest): Promise<OAuthRedirect> =>
    api.post<OAuthRedirect>('/api/oauth.deny', data)
}

/**
 * Whether the connection page may send the browser to a redirect URL: http and https only. The
 * server accepts no other scheme; the page checks again rather than open whatever it is given.
 */
export function isWebRedirectURL(url: string): boolean {
  try {
    const { protocol } = new URL(url)
    return protocol === 'https:' || protocol === 'http:'
  } catch {
    return false
  }
}

export const mcpApi = {
  setMCPSettings: (data: SetMCPSettingsRequest): Promise<SetMCPSettingsResponse> =>
    api.post<SetMCPSettingsResponse>('/api/workspaces.setMCPSettings', data),

  listWorkspaceGrants: (workspaceId: string): Promise<ListWorkspaceGrantsResponse> => {
    const params = new URLSearchParams({ workspace_id: workspaceId })
    return api.get<ListWorkspaceGrantsResponse>(`/api/oauth.listWorkspaceGrants?${params.toString()}`)
  },

  revokeGrant: (data: RevokeGrantRequest): Promise<RevokeGrantResponse> =>
    api.post<RevokeGrantResponse>('/api/oauth.revokeGrant', data)
}

/**
 * A workspace's connections, read under one key by the MCP page and Team & API keys: revoking one
 * refreshes both. Owners only; the server refuses anyone else.
 */
export function workspaceGrantsQuery(workspaceId: string) {
  return queryOptions({
    queryKey: ['mcp-grants', workspaceId],
    queryFn: () => mcpApi.listWorkspaceGrants(workspaceId)
  })
}
