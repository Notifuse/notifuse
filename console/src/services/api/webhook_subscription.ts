import { api } from './client'

export interface CustomEventFilters {
  goal_types?: string[]
  event_names?: string[]
}

export interface WebhookSubscriptionSettings {
  event_types: string[]
  custom_event_filters?: CustomEventFilters
  // Narrow the fan-out of the list.* and segment.* event types to specific ids. Absent or empty
  // means no filter — every list, every segment. Zapier sets these when it registers a Zap; the
  // console offers no UI for them, so it never has a value of its own to send for either.
  list_ids?: string[]
  segment_ids?: string[]
}

export interface WebhookSubscription {
  id: string
  name: string
  url: string
  secret: string
  settings: WebhookSubscriptionSettings
  // Flattened from settings by backend MarshalJSON
  event_types?: string[]
  custom_event_filters?: CustomEventFilters
  list_ids?: string[]
  segment_ids?: string[]
  enabled: boolean
  // Attribution for whatever created the subscription: absent or empty means a user made it by
  // hand, 'zapier' means a Zap registered it. Typed as a plain string rather than a union so a
  // value from a newer server is a value the console can still render, not a type error.
  source?: string
  // Delivery attempts that have failed back to back, reset to zero by the first success.
  consecutive_failures?: number
  // When the current run of failures started, cleared by the first success. The count alone
  // does not retire an endpoint: a burst can fail twenty deliveries in one poll, so the
  // threshold only acts once this shows the failures have persisted.
  failing_since?: string
  // Present only on a subscription Notifuse switched off itself after sustained delivery
  // failure, which is what tells it apart from one the user switched off.
  disabled_reason?: string
  last_delivery_at?: string
  created_at: string
  updated_at: string
}

export interface WebhookDelivery {
  id: string
  subscription_id: string
  event_type: string
  payload: Record<string, unknown>
  // 'delivering' is a durable status, not a transient in-memory one: claiming a delivery is a
  // status change written to the row, so a batch in flight is visible to anyone reading the
  // table. A union that omits it makes those rows render as an untranslated raw literal and
  // silently excludes them from the Pending count.
  status: 'pending' | 'delivering' | 'delivered' | 'failed'
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
  event_types: string[]
  custom_event_filters?: CustomEventFilters
}

// webhookSubscriptions.update replaces name, url and event_types, and patches everything else:
// a key this request does not carry leaves the stored value alone. That is why the optional
// fields below are optional in the type as well as on the wire — omitting one is a statement
// the endpoint understands ("say nothing about it"), not a hole a caller has to plug.
//
// To clear a filter, name it as empty: [] for the id filters, {} for the custom event ones.
// Omitting it would keep whatever is stored, which for a Zap registered against one list is
// the difference between the events it asked for and every list event in the workspace.
export interface UpdateWebhookSubscriptionRequest {
  workspace_id: string
  id: string
  name: string
  url: string
  event_types: string[]
  custom_event_filters?: CustomEventFilters
  list_ids?: string[]
  segment_ids?: string[]
  // Optional because the console deliberately never sends it: the drawer has no switch, and a
  // flag echoed back from the value read when the drawer opened would re-enable a subscription
  // somebody disabled meanwhile. Typing it as required only forced the one caller to cast the
  // key away, which is how a required field ends up being the one nobody sends.
  enabled?: boolean
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
    return api.post('/api/webhookSubscriptions.create', params)
  },

  list: async (workspaceId: string): Promise<{ subscriptions: WebhookSubscription[] }> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', workspaceId)
    return api.get<{ subscriptions: WebhookSubscription[] }>(
      `/api/webhookSubscriptions.list?${searchParams.toString()}`
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
      `/api/webhookSubscriptions.get?${searchParams.toString()}`
    )
  },

  update: async (
    params: UpdateWebhookSubscriptionRequest
  ): Promise<{ subscription: WebhookSubscription }> => {
    // Deliberately a pass-through. Which filters a save means to remove is a statement about
    // what the user just did to a form, and only the caller holding that form knows it — a
    // default applied here would be inherited invisibly by every future caller, including
    // ones with no UI behind them.
    return api.post('/api/webhookSubscriptions.update', params)
  },

  delete: async (workspaceId: string, id: string): Promise<{ success: boolean }> => {
    return api.post('/api/webhookSubscriptions.delete', {
      workspace_id: workspaceId,
      id
    })
  },

  toggle: async (
    params: ToggleWebhookSubscriptionRequest
  ): Promise<{ subscription: WebhookSubscription }> => {
    return api.post('/api/webhookSubscriptions.toggle', params)
  },

  regenerateSecret: async (
    workspaceId: string,
    id: string
  ): Promise<{ subscription: WebhookSubscription }> => {
    return api.post('/api/webhookSubscriptions.regenerateSecret', {
      workspace_id: workspaceId,
      id
    })
  },

  getDeliveries: async (
    workspaceId: string,
    subscriptionId?: string,
    limit?: number,
    offset?: number
  ): Promise<GetDeliveriesResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', workspaceId)
    if (subscriptionId) searchParams.append('subscription_id', subscriptionId)
    if (limit !== undefined) searchParams.append('limit', limit.toString())
    if (offset !== undefined) searchParams.append('offset', offset.toString())
    return api.get<GetDeliveriesResponse>(
      `/api/webhookSubscriptions.deliveries?${searchParams.toString()}`
    )
  },

  test: async (
    workspaceId: string,
    id: string,
    eventType: string
  ): Promise<TestWebhookResponse> => {
    return api.post('/api/webhookSubscriptions.test', {
      workspace_id: workspaceId,
      id,
      event_type: eventType
    })
  },

  getEventTypes: async (): Promise<{ event_types: string[] }> => {
    return api.get<{ event_types: string[] }>('/api/webhookSubscriptions.eventTypes')
  }
}
