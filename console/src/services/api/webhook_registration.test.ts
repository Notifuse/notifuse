import { describe, it, expectTypeOf } from 'vitest'
import type { RegisterWebhookRequest, WebhookEndpointStatus } from './webhook_registration'

// No provider registers a webhook for 'unsubscribe', so a registration that asked for one
// would be ignored without a word. Mailjet's unsub webhook is registered with 'complaint',
// as it always was, and its endpoint is reported back as recording unsubscribes, which is
// what the webhook now records it as.
//
// These are type assertions: `tsc -b` checks them, vitest strips the types and runs nothing.
describe('webhook registration event types', () => {
  it("reports an endpoint that records unsubscribes, and offers no 'unsubscribe' registration", () => {
    expectTypeOf<'unsubscribe'>().toExtend<WebhookEndpointStatus['event_type']>()
    expectTypeOf<'unsubscribe'>().not.toExtend<NonNullable<RegisterWebhookRequest['event_types']>[number]>()
    expectTypeOf<'complaint'>().toExtend<NonNullable<RegisterWebhookRequest['event_types']>[number]>()
  })
})
