import { api } from './client'

export interface NotificationCenterParams {
  wid: string
  email: string
  email_hmac: string
  lid?: string
  mid?: string
  action?: string
}

export interface PreferencesRequest {
  workspace_id: string
  email: string
  email_hmac: string
}

export interface Contact {
  id: string
  email: string
  first_name?: string
  last_name?: string
  external_id?: string
  [key: string]: any
}

export interface List {
  id: string
  name: string
  description?: string
}

export interface ContactList {
  list_id: string
  subscribed: boolean
}

export interface TransactionalNotification {
  id: string
  name: string
  description?: string
}

export interface ContactPreferencesResponse {
  contact: Contact
  public_lists?: List[] | null
  contact_lists?: ContactList[] | null
  logo_url?: string
  website_url?: string
}

/**
 * Validates notification center parameters
 * Matches the Validate method in NotificationCenterRequest
 */
export function validateParams(params: Partial<NotificationCenterParams>): string | null {
  if (!params.email) {
    return 'email is required'
  }
  if (!params.email_hmac) {
    return 'email_hmac is required'
  }
  if (!params.wid) {
    return 'wid is required'
  }
  return null
}

export async function getContactPreferences(
  params: PreferencesRequest
): Promise<ContactPreferencesResponse> {
  const queryParams = new URLSearchParams({
    workspace_id: params.workspace_id,
    email: params.email,
    email_hmac: params.email_hmac
  }).toString()

  return api.get<ContactPreferencesResponse>(`/preferences?${queryParams}`)
}

export function parseNotificationCenterParams(): NotificationCenterParams | null {
  const searchParams = new URLSearchParams(window.location.search)

  const params: Partial<NotificationCenterParams> = {
    wid: searchParams.get('wid') || undefined,
    email: searchParams.get('email') || undefined,
    email_hmac: searchParams.get('email_hmac') || undefined,
    mid: searchParams.get('mid') || undefined,
    action: searchParams.get('action') || undefined
  }

  // Check if all required params are present
  if (!params.email) {
    return null
  }
  if (!params.email_hmac) {
    return null
  }
  if (!params.wid) {
    return null
  }

  return params as NotificationCenterParams
}

export interface SubscribeToListsRequest {
  workspace_id: string
  contact: Contact
  list_ids: string[]
}

export interface SubscribeResponse {
  success: boolean
}

export async function subscribeToLists(
  request: SubscribeToListsRequest
): Promise<SubscribeResponse> {
  return api.post<SubscribeResponse>('/subscribe', request)
}
