import { api } from './client'
import type { EmailBlock } from '../../components/email_builder/types'
import type { TemplateBlock } from './workspace'

// Request types
export interface GetTemplateBlockRequest {
  workspace_id: string
  id: string
}

export interface ListTemplateBlocksRequest {
  workspace_id: string
}

export interface CreateTemplateBlockRequest {
  workspace_id: string
  name: string
  block: EmailBlock
}

export interface UpdateTemplateBlockRequest {
  workspace_id: string
  id: string
  name: string
  block: EmailBlock
}

export interface DeleteTemplateBlockRequest {
  workspace_id: string
  id: string
}

// Response types
export interface GetTemplateBlockResponse {
  block: TemplateBlock
}

export interface ListTemplateBlocksResponse {
  blocks: TemplateBlock[]
}

export interface CreateTemplateBlockResponse {
  block: TemplateBlock
}

export interface UpdateTemplateBlockResponse {
  block: TemplateBlock
}

export interface DeleteTemplateBlockResponse {
  success: boolean
}

// Define the API interface
export interface TemplateBlocksApi {
  list: (params: ListTemplateBlocksRequest) => Promise<ListTemplateBlocksResponse>
  get: (params: GetTemplateBlockRequest) => Promise<GetTemplateBlockResponse>
  create: (params: CreateTemplateBlockRequest) => Promise<CreateTemplateBlockResponse>
  update: (params: UpdateTemplateBlockRequest) => Promise<UpdateTemplateBlockResponse>
  delete: (params: DeleteTemplateBlockRequest) => Promise<DeleteTemplateBlockResponse>
}

export const templateBlocksApi: TemplateBlocksApi = {
  list: async (params: ListTemplateBlocksRequest): Promise<ListTemplateBlocksResponse> => {
    const url = `/api/templateBlocks.list?workspace_id=${params.workspace_id}`
    const response = await api.get<ListTemplateBlocksResponse>(url)
    return response
  },
  get: async (params: GetTemplateBlockRequest): Promise<GetTemplateBlockResponse> => {
    const url = `/api/templateBlocks.get?workspace_id=${params.workspace_id}&id=${params.id}`
    const response = await api.get<GetTemplateBlockResponse>(url)
    return response
  },
  create: async (params: CreateTemplateBlockRequest): Promise<CreateTemplateBlockResponse> => {
    const response = await api.post<CreateTemplateBlockResponse>(`/api/templateBlocks.create`, params)
    return response
  },
  update: async (params: UpdateTemplateBlockRequest): Promise<UpdateTemplateBlockResponse> => {
    const response = await api.post<UpdateTemplateBlockResponse>(`/api/templateBlocks.update`, params)
    return response
  },
  delete: async (params: DeleteTemplateBlockRequest): Promise<DeleteTemplateBlockResponse> => {
    const response = await api.post<DeleteTemplateBlockResponse>(`/api/templateBlocks.delete`, params)
    return response
  }
}

