import type { MessageDescriptor } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import type { TimelineTriggerConfig } from '../../services/api/automation'

/** A trigger's label: one whole message with its values, and the value shown as code, if any. */
export interface TriggerLabel {
  message: MessageDescriptor
  /** The key of `message.values` rendered in Mono (a URL, an event name). */
  mono?: string
}

export interface TriggerNames {
  lists?: { id: string; name: string }[]
  segments?: { id: string; name: string }[]
  broadcasts?: { id: string; name: string }[]
  templates?: { id: string; name: string }[]
}

/** The event options of the trigger form, grouped as its cascader shows them. */
export const EVENT_KIND_OPTIONS: { value: string; label: MessageDescriptor; children?: { value: string; label: MessageDescriptor }[] }[] = [
  {
    value: 'contact',
    label: msg`Contact`,
    children: [
      { value: 'contact.created', label: msg`Created` },
      { value: 'contact.updated', label: msg`Updated` },
      { value: 'contact.deleted', label: msg`Deleted` }
    ]
  },
  {
    value: 'list',
    label: msg`List`,
    children: [
      { value: 'list.subscribed', label: msg`Subscribed` },
      { value: 'list.unsubscribed', label: msg`Unsubscribed` },
      { value: 'list.confirmed', label: msg`Confirmed` },
      { value: 'list.resubscribed', label: msg`Resubscribed` },
      { value: 'list.bounced', label: msg`Bounced` },
      { value: 'list.complained', label: msg`Complained` },
      { value: 'list.pending', label: msg`Pending` },
      { value: 'list.removed', label: msg`Removed` }
    ]
  },
  {
    value: 'segment',
    label: msg`Segment`,
    children: [
      { value: 'segment.joined', label: msg`Joined` },
      { value: 'segment.left', label: msg`Left` }
    ]
  },
  {
    value: 'email',
    label: msg`Email`,
    children: [
      { value: 'email.opened', label: msg`Opened` },
      { value: 'email.clicked', label: msg`Clicked` },
      { value: 'email.link_clicked', label: msg`Clicked a link` },
      { value: 'email.bounced', label: msg`Bounced` },
      { value: 'email.complained', label: msg`Complained` },
      { value: 'email.unsubscribed', label: msg`Unsubscribed` }
    ]
  },
  { value: 'custom_event', label: msg`Custom Event` }
]

const nameOf = (items: { id: string; name: string }[] | undefined, id: string) =>
  items?.find((item) => item.id === id)?.name || id

function listLabel(kind: string, list: string): MessageDescriptor | null {
  switch (kind) {
    case 'list.subscribed':
      return msg`Subscribed to ${list}`
    case 'list.unsubscribed':
      return msg`Unsubscribed from ${list}`
    case 'list.confirmed':
      return msg`Confirmed on ${list}`
    case 'list.resubscribed':
      return msg`Resubscribed to ${list}`
    case 'list.bounced':
      return msg`Bounced on ${list}`
    case 'list.complained':
      return msg`Complained on ${list}`
    case 'list.pending':
      return msg`Pending on ${list}`
    case 'list.removed':
      return msg`Removed from ${list}`
    default:
      return null
  }
}

function linkLabel(trigger: TimelineTriggerConfig, names: TriggerNames): TriggerLabel {
  const url = trigger.link_url?.trim() ?? ''
  const source = trigger.broadcast_id
    ? nameOf(names.broadcasts, trigger.broadcast_id)
    : trigger.template_id
      ? nameOf(names.templates, trigger.template_id)
      : ''
  if (url && source) return { message: msg`Clicked ${url} in ${source}`, mono: 'url' }
  if (url) return { message: msg`Clicked ${url}`, mono: 'url' }
  if (source) return { message: msg`Clicked a link in ${source}` }
  return { message: msg`Clicked a link` }
}

/**
 * What starts the automation, in words: a whole message per event with its names (the id when a
 * name is unknown or not readable). `null` without a trigger.
 */
export function describeTrigger(trigger: TimelineTriggerConfig | undefined, names: TriggerNames = {}): TriggerLabel | null {
  const kind = trigger?.event_kind
  if (!trigger || !kind) return null
  switch (kind) {
    case 'contact.created':
      return { message: msg`Contact created` }
    case 'contact.updated':
      return { message: msg`Contact updated` }
    case 'contact.deleted':
      return { message: msg`Contact deleted` }
    case 'segment.joined': {
      const segment = nameOf(names.segments, trigger.segment_id ?? '')
      return { message: msg`Joined segment ${segment}` }
    }
    case 'segment.left': {
      const segment = nameOf(names.segments, trigger.segment_id ?? '')
      return { message: msg`Left segment ${segment}` }
    }
    case 'email.opened':
      return { message: msg`Email opened` }
    case 'email.clicked':
      return { message: msg`Email clicked` }
    case 'email.bounced':
      return { message: msg`Email bounced` }
    case 'email.complained':
      return { message: msg`Email complained` }
    case 'email.unsubscribed':
      return { message: msg`Unsubscribed from an email` }
    case 'email.link_clicked':
      return linkLabel(trigger, names)
    case 'custom_event': {
      const name = trigger.custom_event_name ?? ''
      return { message: msg`Event ${name}`, mono: 'name' }
    }
    default: {
      const list = nameOf(names.lists, trigger.list_id ?? '')
      const label = listLabel(kind, list)
      if (label) return { message: label }
      return { message: msg`Event ${kind}`, mono: 'kind' }
    }
  }
}
