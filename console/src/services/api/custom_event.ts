import { api } from './client'

export interface CustomEvent {
  external_id: string
  email: string
  event_name: string
  properties: Record<string, any>
  occurred_at: string
  source: string
  integration_id?: string | null
  created_at: string
  updated_at: string
}

export interface CreateCustomEventRequest {
  workspace_id: string
  email: string
  event_name: string
  external_id: string
  properties?: Record<string, any>
  occurred_at?: string
  source?: string
  integration_id?: string
}

export interface CreateCustomEventResponse {
  event: CustomEvent
}

export interface BatchCreateCustomEventsRequest {
  workspace_id: string
  events: Array<{
    email: string
    event_name: string
    external_id: string
    properties?: Record<string, any>
    occurred_at?: string
    source?: string
    integration_id?: string
  }>
}

export interface BatchCreateCustomEventsResponse {
  event_ids: string[]
  count: number
}

export interface ListCustomEventsRequest {
  workspace_id: string
  email?: string
  event_name?: string
  limit?: number
  offset?: number
}

export interface ListCustomEventsResponse {
  events: CustomEvent[]
  count: number
}

export const customEventApi = {
  create: async (params: CreateCustomEventRequest): Promise<CreateCustomEventResponse> => {
    return api.post('/api/customEvent.create', params)
  },

  batchCreate: async (params: BatchCreateCustomEventsRequest): Promise<BatchCreateCustomEventsResponse> => {
    return api.post('/api/customEvent.batchCreate', params)
  },

  get: async (params: {
    workspace_id: string
    event_name: string
    external_id: string
  }): Promise<{ event: CustomEvent }> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('event_name', params.event_name)
    searchParams.append('external_id', params.external_id)
    return api.get<{ event: CustomEvent }>(`/api/customEvent.get?${searchParams.toString()}`)
  },

  list: async (params: ListCustomEventsRequest): Promise<ListCustomEventsResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)

    if (params.email) searchParams.append('email', params.email)
    if (params.event_name) searchParams.append('event_name', params.event_name)
    if (params.limit) searchParams.append('limit', params.limit.toString())
    if (params.offset) searchParams.append('offset', params.offset.toString())

    return api.get<ListCustomEventsResponse>(`/api/customEvent.list?${searchParams.toString()}`)
  }
}
