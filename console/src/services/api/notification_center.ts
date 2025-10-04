import { api } from './client'
import type { Contact } from './contacts'

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

export const notificationCenterApi = {
  // Get notification center data for a contact
  getNotificationCenter: async (
    params: NotificationCenterRequest
  ): Promise<NotificationCenterResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('email', params.email)
    searchParams.append('email_hmac', params.email_hmac)

    return api.get<NotificationCenterResponse>(`/notification-center?${searchParams.toString()}`)
  },

  // Subscribe to lists - public route
  subscribe: async (params: SubscribeToListsRequest): Promise<{ success: boolean }> => {
    return api.post('/subscribe', params)
  },

  // One-click unsubscribe for Gmail header link
  unsubscribeOneClick: async (
    params: UnsubscribeFromListsRequest
  ): Promise<{ success: boolean }> => {
    return api.post('/unsubscribe-oneclick', params)
  }
}
