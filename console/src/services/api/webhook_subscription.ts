import { api } from './client'

export interface CustomEventFilters {
  goal_type?: string
  event_names?: string[]
}

export interface WebhookSubscription {
  id: string
  name: string
  url: string
  secret: string
  event_types: string[]
  custom_event_filters?: CustomEventFilters
  enabled: boolean
  description: string
  success_count: number
  failure_count: number
  last_delivery_at?: string
  created_at: string
  updated_at: string
}

export interface WebhookDelivery {
  id: string
  subscription_id: string
  event_type: string
  payload: Record<string, unknown>
  status: 'pending' | 'delivered' | 'failed'
  attempts: number
  max_attempts: number
  next_attempt_at: string
  last_attempt_at?: string
  delivered_at?: string
  last_response_status?: number
  last_response_body?: string
  last_error?: string
  created_at: string
}

export interface CreateWebhookSubscriptionRequest {
  workspace_id: string
  name: string
  url: string
  description?: string
  event_types: string[]
  custom_event_filters?: CustomEventFilters
}

export interface UpdateWebhookSubscriptionRequest {
  workspace_id: string
  id: string
  name: string
  url: string
  description?: string
  event_types: string[]
  custom_event_filters?: CustomEventFilters
  enabled: boolean
}

export interface ToggleWebhookSubscriptionRequest {
  workspace_id: string
  id: string
  enabled: boolean
}

export interface TestWebhookResponse {
  success: boolean
  status_code: number
  response_body: string
  error?: string
}

export interface GetDeliveriesResponse {
  deliveries: WebhookDelivery[]
  total: number
  limit: number
  offset: number
}

export const webhookSubscriptionApi = {
  create: async (
    params: CreateWebhookSubscriptionRequest
  ): Promise<{ subscription: WebhookSubscription }> => {
    return api.post('/api/webhook_subscription.create', params)
  },

  list: async (workspaceId: string): Promise<{ subscriptions: WebhookSubscription[] }> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', workspaceId)
    return api.get<{ subscriptions: WebhookSubscription[] }>(
      `/api/webhook_subscription.list?${searchParams.toString()}`
    )
  },

  get: async (
    workspaceId: string,
    id: string
  ): Promise<{ subscription: WebhookSubscription }> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', workspaceId)
    searchParams.append('id', id)
    return api.get<{ subscription: WebhookSubscription }>(
      `/api/webhook_subscription.get?${searchParams.toString()}`
    )
  },

  update: async (
    params: UpdateWebhookSubscriptionRequest
  ): Promise<{ subscription: WebhookSubscription }> => {
    return api.post('/api/webhook_subscription.update', params)
  },

  delete: async (workspaceId: string, id: string): Promise<{ success: boolean }> => {
    return api.post('/api/webhook_subscription.delete', {
      workspace_id: workspaceId,
      id
    })
  },

  toggle: async (
    params: ToggleWebhookSubscriptionRequest
  ): Promise<{ subscription: WebhookSubscription }> => {
    return api.post('/api/webhook_subscription.toggle', params)
  },

  regenerateSecret: async (
    workspaceId: string,
    id: string
  ): Promise<{ subscription: WebhookSubscription }> => {
    return api.post('/api/webhook_subscription.regenerate_secret', {
      workspace_id: workspaceId,
      id
    })
  },

  getDeliveries: async (
    workspaceId: string,
    subscriptionId: string,
    limit?: number,
    offset?: number
  ): Promise<GetDeliveriesResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', workspaceId)
    searchParams.append('subscription_id', subscriptionId)
    if (limit !== undefined) searchParams.append('limit', limit.toString())
    if (offset !== undefined) searchParams.append('offset', offset.toString())
    return api.get<GetDeliveriesResponse>(
      `/api/webhook_subscription.deliveries?${searchParams.toString()}`
    )
  },

  test: async (workspaceId: string, id: string): Promise<TestWebhookResponse> => {
    return api.post('/api/webhook_subscription.test', {
      workspace_id: workspaceId,
      id
    })
  },

  getEventTypes: async (): Promise<{ event_types: string[] }> => {
    return api.get<{ event_types: string[] }>('/api/webhook_subscription.event_types')
  }
}
