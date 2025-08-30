import { api } from './client'
import {
  CreateWorkspaceRequest,
  CreateWorkspaceResponse,
  ListWorkspacesResponse,
  GetWorkspaceResponse,
  UpdateWorkspaceRequest,
  UpdateWorkspaceResponse,
  DeleteWorkspaceRequest,
  DeleteWorkspaceResponse,
  GetWorkspaceMembersResponse,
  InviteMemberRequest,
  InviteMemberResponse,
  CreateAPIKeyResponse,
  CreateAPIKeyRequest,
  RemoveMemberRequest,
  RemoveMemberResponse,
  CreateIntegrationRequest,
  CreateIntegrationResponse,
  UpdateIntegrationRequest,
  UpdateIntegrationResponse,
  DeleteIntegrationRequest,
  DeleteIntegrationResponse,
  VerifyInvitationTokenResponse,
  AcceptInvitationResponse
} from './types'

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
    api.post<AcceptInvitationResponse>('/api/workspaces.acceptInvitation', { token })
}
