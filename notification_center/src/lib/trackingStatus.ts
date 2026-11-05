import type { ContactTrackingState } from '../api/notification_center'

/** The contact's email tracking state on one line: on, off, or on for some kinds only. */
export function trackingStatusKey(tracking: ContactTrackingState): string {
  const states = [
    ...(tracking.opens_enabled ? [tracking.opens_tracked] : []),
    ...(tracking.clicks_enabled ? [tracking.clicks_tracked] : [])
  ]
  if (states.every(Boolean)) return 'trackingStatusOn'
  if (!states.some(Boolean)) return 'trackingStatusOff'
  return 'trackingStatusPartial'
}
