import { api } from './client'

// Types for contact list operations
export interface ContactList {
  email: string
  list_id: string
  status: string
  created_at: string
  updated_at: string
}

export interface GetContactListRequest {
  workspace_id: string
  email: string
  list_id: string
}

export interface GetContactsByListRequest {
  workspace_id: string
  list_id: string
  // Pagination could be added here
}

export interface GetListsByContactRequest {
  workspace_id: string
  email: string
  // Pagination could be added here
}

export interface UpdateContactListStatusRequest {
  workspace_id: string
  email: string
  list_id: string
  status: string
}

export interface RemoveContactFromListRequest {
  workspace_id: string
  email: string
  list_id: string
}

// Response types
export interface ContactListResponse {
  contact_list: ContactList
}

export interface ContactListsResponse {
  contact_lists: ContactList[]
}

export interface SuccessResponse {
  success: boolean
}

// updateStatus silently no-ops when the contact is not in the list; `found`
// distinguishes the no-op case so callers can surface it as "Skipped".
export interface UpdateContactListStatusResponse {
  success: boolean
  found?: boolean
  message?: string
}

// API client for contact list operations
export const contactListApi = {
  // Get a specific contact-list relationship by IDs
  getByIDs: async (params: GetContactListRequest): Promise<ContactListResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('email', params.email)
    searchParams.append('list_id', params.list_id)

    return api.get<ContactListResponse>(`/api/contactLists.getByIDs?${searchParams.toString()}`)
  },

  // Get all contacts for a specific list
  getContactsByList: async (params: GetContactsByListRequest): Promise<ContactListsResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('list_id', params.list_id)

    return api.get<ContactListsResponse>(
      `/api/contactLists.getContactsByList?${searchParams.toString()}`
    )
  },

  // Get all lists that a specific contact belongs to
  getListsByContact: async (params: GetListsByContactRequest): Promise<ContactListsResponse> => {
    const searchParams = new URLSearchParams()
    searchParams.append('workspace_id', params.workspace_id)
    searchParams.append('email', params.email)

    return api.get<ContactListsResponse>(
      `/api/contactLists.getListsByContact?${searchParams.toString()}`
    )
  },

  // Update the status of a contact in a list
  updateStatus: async (
    params: UpdateContactListStatusRequest
  ): Promise<UpdateContactListStatusResponse> => {
    return api.post<UpdateContactListStatusResponse>('/api/contactLists.updateStatus', params)
  },

  // Remove a contact from a list
  removeContact: async (params: RemoveContactFromListRequest): Promise<SuccessResponse> => {
    return api.post<SuccessResponse>('/api/contactLists.removeContact', params)
  }
}
