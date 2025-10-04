import { api } from './client'
import type { EmailProviderKind } from './workspace'

export type EmailEventType = 'delivered' | 'bounce' | 'complaint' | 'click' | 'open'

export interface WebhookEndpointStatus {
  webhook_id: string
  url: string
  event_type: EmailEventType
  active: boolean
}

export interface WebhookRegistrationStatus {
  email_provider_kind: EmailProviderKind
  is_registered: boolean
  endpoints?: WebhookEndpointStatus[]
  error?: string
  provider_details?: Record<string, any>
}

export interface RegisterWebhookRequest {
  workspace_id: string
  integration_id: string
  base_url: string
  event_types?: EmailEventType[]
}

export interface RegisterWebhookResponse {
  status: WebhookRegistrationStatus
}

export interface GetWebhookStatusRequest {
  workspace_id: string
  integration_id: string
}

export interface GetWebhookStatusResponse {
  status: WebhookRegistrationStatus
}

/**
 * Register webhooks for an email provider integration
 */
export async function registerWebhook(
  request: RegisterWebhookRequest
): Promise<RegisterWebhookResponse> {
  return api.post<RegisterWebhookResponse>('/api/webhooks.register', request)
}

/**
 * Get the current status of webhooks for an email provider integration
 */
export async function getWebhookStatus(
  request: GetWebhookStatusRequest
): Promise<GetWebhookStatusResponse> {
  const { workspace_id, integration_id } = request
  return api.get<GetWebhookStatusResponse>(
    `/api/webhooks.status?workspace_id=${workspace_id}&integration_id=${integration_id}`
  )
}
