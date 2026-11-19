import { msg } from '@lingui/core/macro'
import type { MessageDescriptor } from '@lingui/core'
import type { GalleryModel, GalleryModelId, GalleryUseCase } from './types'

// The console's own words for the models: the gallery shows them in the member's language,
// while the emails are written in the workspace's.
export const MODEL_LABELS: Record<GalleryModelId, { name: MessageDescriptor; description: MessageDescriptor }> = {
  blank: { name: msg`Simple`, description: msg`A logo, a paragraph and the footer, to write your own email.` },
  verify: { name: msg`Verification code`, description: msg`A one-time code to sign in or confirm an action.` },
  reset: { name: msg`Password reset`, description: msg`A reset link, with the link in plain text under the button.` },
  confirm: { name: msg`Subscription confirmation`, description: msg`The double opt-in email: one confirmation button.` },
  invite: { name: msg`Team invitation`, description: msg`An invitation to join a team or a space.` },
  receipt: { name: msg`Payment receipt`, description: msg`The receipt of a payment or a subscription.` },
  order: { name: msg`Order confirmation`, description: msg`The summary sent right after a purchase: items, totals, address.` },
  shipping: { name: msg`Order shipped`, description: msg`A shipping update with the carrier and the tracking link.` },
  cart: { name: msg`Abandoned cart`, description: msg`A reminder listing the items of the cart, from the event that started the automation.` },
  promo: { name: msg`Sale`, description: msg`A limited offer: a visual, a promo code and featured products.` },
  newsletter: { name: msg`Newsletter`, description: msg`A featured article, two articles in a list and a quote.` },
  announce: { name: msg`Product announcement`, description: msg`The launch of a product or a feature, in three points.` },
  event: { name: msg`Event invitation`, description: msg`A webinar or an event: date, agenda, registration.` },
  welcome: { name: msg`Welcome`, description: msg`Welcomes a new subscriber with three first steps.` },
  winback: { name: msg`We miss you`, description: msg`Brings inactive contacts back with an offer.` },
  unsub: { name: msg`Unsubscribe confirmation`, description: msg`Confirms an unsubscription and links to the preferences.` }
}

export const USE_CASE_LABELS: Record<GalleryUseCase, MessageDescriptor> = {
  account: msg`Account and security`,
  billing: msg`Billing`,
  shop: msg`E-commerce`,
  news: msg`Newsletters and announcements`,
  lifecycle: msg`Lifecycle`
}

export const SENT_BY_LABELS: Record<GalleryModel['sentBy'], MessageDescriptor> = {
  any: msg`Any send`,
  api: msg`The transactional API`,
  broadcast: msg`A newsletter`,
  automation: msg`An automation`,
  list: msg`A list's emails`
}

