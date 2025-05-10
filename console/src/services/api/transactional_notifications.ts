import { api } from './client'

export type TransactionalChannel = 'email'
// Add other channels in the future (sms, push, etc.)

export interface ChannelTemplate {
  template_id: string
  version: number
  settings?: Record<string, any>
}

export interface ChannelTemplates {
  email?: ChannelTemplate
  // Other channel templates in the future
}

export interface TransactionalNotification {
  id: string
  name: string
  description: string
  channels: ChannelTemplates
  is_public: boolean
  metadata?: Record<string, any>
  created_at: string
  updated_at: string
  deleted_at?: string
}

export interface CreateTransactionalNotificationRequest {
  workspace_id: string
  notification: {
    id: string
    name: string
    description?: string
    channels: ChannelTemplates
    is_public?: boolean
    metadata?: Record<string, any>
  }
}

export interface UpdateTransactionalNotificationRequest {
  workspace_id: string
  id: string
  updates: {
    name?: string
    description?: string
    channels?: ChannelTemplates
    is_public?: boolean
    metadata?: Record<string, any>
  }
}

export interface ListTransactionalNotificationsRequest {
  workspace_id: string
  search?: string
  limit?: number
  offset?: number
}

export interface ListTransactionalNotificationsResponse {
  notifications: TransactionalNotification[]
  total: number
}

export interface GetTransactionalNotificationRequest {
  workspace_id: string
  id: string
}

export interface GetTransactionalNotificationResponse {
  notification: TransactionalNotification
}

export interface DeleteTransactionalNotificationRequest {
  workspace_id: string
  id: string
}

export interface Contact {
  email?: string
  phone?: string
  push_token?: string
  // Other contact methods in the future
}

export interface SendTransactionalNotificationRequest {
  workspace_id: string
  notification: {
    id: string
    contact: Contact
    channels?: TransactionalChannel[]
    data?: Record<string, any>
    metadata?: Record<string, any>
    cc?: string[]
    bcc?: string[]
  }
}

export interface SendTransactionalNotificationResponse {
  message_id: string
}

export const transactionalNotificationsApi = {
  list: async (
    params: ListTransactionalNotificationsRequest
  ): Promise<ListTransactionalNotificationsResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    if (params.search) searchParams.append('search', params.search)
    if (params.limit) searchParams.append('limit', params.limit.toString())
    if (params.offset) searchParams.append('offset', params.offset.toString())

    return api.get<ListTransactionalNotificationsResponse>(
      `/api/transactional.list?${searchParams.toString()}`
    )
  },

  get: async (
    params: GetTransactionalNotificationRequest
  ): Promise<GetTransactionalNotificationResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('id', params.id)

    return api.get<GetTransactionalNotificationResponse>(
      `/api/transactional.get?${searchParams.toString()}`
    )
  },

  create: async (
    params: CreateTransactionalNotificationRequest
  ): Promise<GetTransactionalNotificationResponse> => {
    return api.post<GetTransactionalNotificationResponse>('/api/transactional.create', params)
  },

  update: async (
    params: UpdateTransactionalNotificationRequest
  ): Promise<GetTransactionalNotificationResponse> => {
    return api.post<GetTransactionalNotificationResponse>('/api/transactional.update', params)
  },

  delete: async (params: DeleteTransactionalNotificationRequest): Promise<{ success: boolean }> => {
    return api.post<{ success: boolean }>('/api/transactional.delete', params)
  },

  send: async (
    params: SendTransactionalNotificationRequest
  ): Promise<SendTransactionalNotificationResponse> => {
    return api.post<SendTransactionalNotificationResponse>('/api/transactional.send', params)
  }
}
