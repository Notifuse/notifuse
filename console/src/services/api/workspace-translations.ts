import { api } from './client'

export interface WorkspaceTranslation {
  locale: string
  content: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface UpsertWorkspaceTranslationRequest {
  workspace_id: string
  locale: string
  content: Record<string, unknown>
}

export interface ListWorkspaceTranslationsResponse {
  translations: WorkspaceTranslation[]
}

export interface DeleteWorkspaceTranslationRequest {
  workspace_id: string
  locale: string
}

export interface WorkspaceTranslationsApi {
  list: (workspaceId: string) => Promise<ListWorkspaceTranslationsResponse>
  upsert: (params: UpsertWorkspaceTranslationRequest) => Promise<void>
  delete: (params: DeleteWorkspaceTranslationRequest) => Promise<void>
}

export const workspaceTranslationsApi: WorkspaceTranslationsApi = {
  list: async (workspaceId: string) => {
    return api.get<ListWorkspaceTranslationsResponse>(
      `/api/workspace_translations.list?workspace_id=${workspaceId}`
    )
  },
  upsert: async (params: UpsertWorkspaceTranslationRequest) => {
    return api.post<void>('/api/workspace_translations.upsert', params)
  },
  delete: async (params: DeleteWorkspaceTranslationRequest) => {
    return api.post<void>('/api/workspace_translations.delete', params)
  }
}
