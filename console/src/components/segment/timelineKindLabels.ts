import { msg } from '@lingui/core/macro'
import type { MessageDescriptor } from '@lingui/core'

/**
 * The activities an Activity condition counts, in the order the editor offers them, worded as the
 * automations' triggers word the same events (« Email opened »). The editor's picker and the saved
 * condition read this one list.
 */
export const TIMELINE_KIND_LABELS: { value: string; label: MessageDescriptor }[] = [
  { value: 'email.sent', label: msg`Email sent` },
  { value: 'email.opened', label: msg`Email opened` },
  { value: 'email.clicked', label: msg`Email clicked` },
  { value: 'email.bounced', label: msg`Email bounced` },
  { value: 'email.complained', label: msg`Email complained` },
  { value: 'email.unsubscribed', label: msg`Unsubscribed from an email` },
  { value: 'web.pageview', label: msg`View web page` },
  { value: 'web.session', label: msg`Visit website` }
]

export const timelineKindLabel = (kind: string | undefined): MessageDescriptor | undefined =>
  TIMELINE_KIND_LABELS.find((item) => item.value === kind)?.label
