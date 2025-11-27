import { api } from './client'

export interface ListContactsRequest {
  workspace_id: string
  // Optional filters
  email?: string
  external_id?: string
  first_name?: string
  last_name?: string
  phone?: string
  country?: string
  language?: string
  with_contact_lists?: boolean
  list_id?: string
  contact_list_status?: string
  segments?: string[]
  // Pagination
  limit?: number
  cursor?: string
}

export interface Contact {
  email: string
  external_id?: string
  timezone?: string
  language?: string
  first_name?: string
  last_name?: string
  phone?: string
  address_line_1?: string
  address_line_2?: string
  country?: string
  postcode?: string
  state?: string
  job_title?: string

  custom_string_1?: string
  custom_string_2?: string
  custom_string_3?: string
  custom_string_4?: string
  custom_string_5?: string

  custom_number_1?: number
  custom_number_2?: number
  custom_number_3?: number
  custom_number_4?: number
  custom_number_5?: number

  custom_datetime_1?: string
  custom_datetime_2?: string
  custom_datetime_3?: string
  custom_datetime_4?: string
  custom_datetime_5?: string

  custom_json_1?: any
  custom_json_2?: any
  custom_json_3?: any
  custom_json_4?: any
  custom_json_5?: any

  created_at: string
  updated_at: string

  contact_lists: {
    email: string
    list_id: string
    status: string
    created_at: string
    updated_at: string
  }[]

  contact_segments?: {
    segment_id: string
    version?: number
    matched_at?: string
    computed_at?: string
  }[]
}

export interface ListContactsResponse {
  contacts: Contact[]
  next_cursor?: string
}

export enum UpsertContactOperationAction {
  Create = 'create',
  Update = 'update',
  Error = 'error'
}

export interface UpsertContactOperation {
  action: UpsertContactOperationAction
  email?: string
  error?: string
}

export interface BatchImportContactsResponse {
  operations: UpsertContactOperation[]
  error?: string
}

export interface DeleteContactResponse {
  success: boolean
}

export interface GetTotalContactsResponse {
  total_contacts: number
}

export const contactsApi = {
  list: async (params: ListContactsRequest): Promise<ListContactsResponse> => {
    const searchParams = new URLSearchParams()

    // Add required param
    searchParams.append('workspace_id', params.workspace_id)

    // Add optional params if they exist
    if (params.email) searchParams.append('email', params.email)
    if (params.external_id) searchParams.append('external_id', params.external_id)
    if (params.first_name) searchParams.append('first_name', params.first_name)
    if (params.last_name) searchParams.append('last_name', params.last_name)
    if (params.phone) searchParams.append('phone', params.phone)
    if (params.country) searchParams.append('country', params.country)
    if (params.language) searchParams.append('language', params.language)
    if (params.list_id) searchParams.append('list_id', params.list_id)
    if (params.contact_list_status)
      searchParams.append('contact_list_status', params.contact_list_status)
    if (params.segments && params.segments.length > 0) {
      params.segments.forEach((segment) => searchParams.append('segments[]', segment))
    }
    if (params.limit) searchParams.append('limit', params.limit.toString())
    if (params.cursor) searchParams.append('cursor', params.cursor)
    if (params.with_contact_lists)
      searchParams.append('with_contact_lists', params.with_contact_lists.toString())
    return api.get<ListContactsResponse>(`/api/contacts.list?${searchParams.toString()}`)
  },

  upsert: async (params: {
    workspace_id: string
    contact: Partial<Contact>
  }): Promise<UpsertContactOperation> => {
    return api.post('/api/contacts.upsert', {
      workspace_id: params.workspace_id,
      contact: params.contact
    })
  },

  batchImport: async (params: {
    workspace_id: string
    contacts: Partial<Contact>[]
    subscribe_to_lists?: string[]
  }): Promise<BatchImportContactsResponse> => {
    return api.post('/api/contacts.import', {
      workspace_id: params.workspace_id,
      contacts: params.contacts,
      subscribe_to_lists: params.subscribe_to_lists
    })
  },

  delete: async (params: {
    workspace_id: string
    email: string
  }): Promise<DeleteContactResponse> => {
    return api.post('/api/contacts.delete', {
      workspace_id: params.workspace_id,
      email: params.email
    })
  },

  getTotalContacts: async (params: { workspace_id: string }): Promise<GetTotalContactsResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    return api.get<GetTotalContactsResponse>(`/api/contacts.count?${searchParams.toString()}`)
  }
}
