import type { I18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import type {
  ContactListEntityData,
  ContactSegmentEntityData,
  ContactTimelineEntry,
  CustomEventEntityData,
  InboundWebhookEventEntityData,
  MessageHistoryEntityData
} from '../../../services/api/contact_timeline'
import { INBOUND_EVENT_LABELS } from '../../logs/labels'
import { changeNew } from '../../timeline/webPageUrl'

const LABELS = {
  click: msg`Click`,
  open: msg`Open`,
  delivered: msg`Delivered`,
  email: msg`Email`,
  list: msg`List`,
  joined: msg`Joined segment`,
  left: msg`Left segment`,
  created: msg`Contact created`,
  updated: msg`Profile updated`,
  event: msg`Event`,
  visit: msg`Site visit`,
  page: msg`Page view`,
  automation: msg`Automation`
}

/** A short label of an activity entry, for the Last activity KPI: « Click · Newsletter ». */
export function activityLabel(entry: ContactTimelineEntry, i18n: I18n): string {
  const join = (label: keyof typeof LABELS, detail?: string | null) =>
    detail ? `${i18n._(LABELS[label])} · ${detail}` : i18n._(LABELS[label])
  switch (entry.entity_type) {
    case 'message_history': {
      const data = entry.entity_data as MessageHistoryEntityData | undefined
      const name = data?.template_name
      if (entry.kind === 'email.link_clicked' || entry.changes?.clicked_at) return join('click', name)
      if (entry.changes?.opened_at) return join('open', name)
      if (entry.changes?.delivered_at) return join('delivered', name)
      return join('email', name)
    }
    case 'contact_list':
      return join('list', (entry.entity_data as ContactListEntityData | undefined)?.name)
    case 'contact_segment':
      return join(
        entry.kind === 'segment.left' ? 'left' : 'joined',
        (entry.entity_data as ContactSegmentEntityData | undefined)?.name
      )
    case 'contact':
      return join(entry.operation === 'insert' ? 'created' : 'updated')
    case 'custom_event':
      return join('event', (entry.entity_data as CustomEventEntityData | undefined)?.event_name ?? entry.kind)
    case 'inbound_webhook_event': {
      const data = entry.entity_data as InboundWebhookEventEntityData | undefined
      // The entry's own changes first, as the activity feed reads it.
      const type = changeNew<string>(entry, 'type') || data?.type || ''
      const name = data?.template_name
      // The provider's event type is an identifier: show its label, never the raw value.
      if (type === 'opened') return join('open', name)
      if (type === 'clicked') return join('click', name)
      const label = INBOUND_EVENT_LABELS[type as keyof typeof INBOUND_EVENT_LABELS]
      if (!label) return join('email', name)
      return name ? `${i18n._(label)} · ${name}` : i18n._(label)
    }
    case 'web_session':
      return join('visit')
    case 'web_page':
      return join('page')
    case 'automation':
      return join('automation')
    default:
      return entry.kind
  }
}
