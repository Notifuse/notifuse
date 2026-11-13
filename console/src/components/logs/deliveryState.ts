import type { WebhookDelivery } from '../../services/api/webhook_subscription'
import type { DELIVERY_STATUS_LABELS } from './labels'

export type DeliveryState = keyof typeof DELIVERY_STATUS_LABELS

/** Retrying and Failed are both `failed`: the attempts left tell them apart. */
export function deliveryState(
  delivery: Pick<WebhookDelivery, 'status' | 'attempts' | 'max_attempts'>
): DeliveryState {
  if (delivery.status === 'failed') {
    return delivery.attempts < delivery.max_attempts ? 'retrying' : 'failed'
  }
  return delivery.status
}
