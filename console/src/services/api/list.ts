import { api } from './client'
import type { Contact } from './contacts'

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
  slug?: string
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

export interface SubscribeToListsRequest {
  workspace_id: string
  contact: Contact
  list_ids: string[]
}

export const listsApi = {
  create: async (params: CreateListRequest): Promise<CreateListResponse> => {
    return api.post('/api/lists.create', params)
  },

  list: async (params: GetListsRequest): Promise<GetListsResponse> => {
    const searchParams = new URLSearchParams()

    // Add required param
    searchParams.append('workspace_id', params.workspace_id)

    return api.get<GetListsResponse>(`/api/lists.list?${searchParams.toString()}`)
  },

  get: async (params: GetListRequest): Promise<GetListResponse> => {
    const searchParams = new URLSearchParams()

    // Add required params
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('id', params.id)

    return api.get<GetListResponse>(`/api/lists.get?${searchParams.toString()}`)
  },

  update: async (params: UpdateListRequest): Promise<UpdateListResponse> => {
    return api.post('/api/lists.update', params)
  },

  delete: async (params: DeleteListRequest): Promise<DeleteListResponse> => {
    return api.post('/api/lists.delete', params)
  },

  stats: async (params: GetListStatsRequest): Promise<GetListStatsResponse> => {
    const searchParams = new URLSearchParams()

    // Add required params
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('list_id', params.list_id)

    return api.get<GetListStatsResponse>(`/api/lists.stats?${searchParams.toString()}`)
  },

  subscribe: async (params: SubscribeToListsRequest): Promise<{ success: boolean }> => {
    return api.post('/api/lists.subscribe', params)
  }
}
