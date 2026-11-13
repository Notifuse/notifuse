import { describe, it, expect } from 'vitest'
import { i18n } from '@lingui/core'
import { describeTrigger, EVENT_KIND_OPTIONS } from './describeTrigger'
import { VALID_EVENT_KINDS, type TimelineTriggerConfig } from '../../services/api/automation'

const trigger = (over: Partial<TimelineTriggerConfig>): TimelineTriggerConfig => ({
  event_kind: 'contact.created',
  frequency: 'once',
  ...over
})
const text = (t: TimelineTriggerConfig | undefined, names = {}) => {
  const label = describeTrigger(t, names)
  return label ? i18n._(label.message) : null
}
const names = {
  lists: [{ id: 'l1', name: 'Newsletter' }],
  segments: [{ id: 's1', name: 'Pro plan' }],
  broadcasts: [{ id: 'b1', name: 'November news' }],
  templates: [{ id: 'tpl1', name: 'Welcome' }]
}

describe('describeTrigger', () => {
  it('names the contact events', () => {
    expect(text(trigger({ event_kind: 'contact.created' }))).toBe('Contact created')
    expect(text(trigger({ event_kind: 'contact.updated' }))).toBe('Contact updated')
    expect(text(trigger({ event_kind: 'contact.deleted' }))).toBe('Contact deleted')
  })

  it('names each list action with the list, or its id when unknown', () => {
    expect(text(trigger({ event_kind: 'list.subscribed', list_id: 'l1' }), names)).toBe('Subscribed to Newsletter')
    expect(text(trigger({ event_kind: 'list.confirmed', list_id: 'l1' }), names)).toBe('Confirmed on Newsletter')
    expect(text(trigger({ event_kind: 'list.removed', list_id: 'lx' }), names)).toBe('Removed from lx')
    for (const kind of VALID_EVENT_KINDS.filter((k) => k.startsWith('list.'))) {
      expect(text(trigger({ event_kind: kind, list_id: 'l1' }), names)).toContain('Newsletter')
    }
  })

  it('names the segment events', () => {
    expect(text(trigger({ event_kind: 'segment.joined', segment_id: 's1' }), names)).toBe('Joined segment Pro plan')
    expect(text(trigger({ event_kind: 'segment.left', segment_id: 's2' }), names)).toBe('Left segment s2')
  })

  it('names the email events', () => {
    expect(text(trigger({ event_kind: 'email.opened' }))).toBe('Email opened')
    expect(text(trigger({ event_kind: 'email.bounced' }))).toBe('Email bounced')
  })

  it('names a clicked link, alone, in a broadcast or in a template', () => {
    const plain = describeTrigger(trigger({ event_kind: 'email.link_clicked', link_url: 'acme.io/pricing' }))
    expect(plain?.mono).toBe('url')
    expect(i18n._(plain!.message)).toBe('Clicked acme.io/pricing')
    expect(text(trigger({ event_kind: 'email.link_clicked', link_url: 'acme.io', broadcast_id: 'b1' }), names)).toBe(
      'Clicked acme.io in November news'
    )
    expect(text(trigger({ event_kind: 'email.link_clicked', template_id: 'tpl1' }), names)).toBe('Clicked a link in Welcome')
    expect(text(trigger({ event_kind: 'email.link_clicked' }))).toBe('Clicked a link')
  })

  it('shows a custom event name as code', () => {
    const label = describeTrigger(trigger({ event_kind: 'custom_event', custom_event_name: 'trial_ending' }))
    expect(label?.mono).toBe('name')
    expect(i18n._(label!.message)).toBe('Event trial_ending')
  })

  it('has a label for every event kind', () => {
    for (const kind of VALID_EVENT_KINDS) expect(text(trigger({ event_kind: kind }))).toBeTruthy()
  })

  it('is null without a trigger', () => {
    expect(describeTrigger(undefined)).toBeNull()
    expect(describeTrigger(trigger({ event_kind: '' }))).toBeNull()
  })

  it('offers every event kind in the form options', () => {
    const offered = EVENT_KIND_OPTIONS.flatMap((group) => (group.children ? group.children.map((c) => c.value) : [group.value]))
    expect([...offered].sort()).toEqual([...VALID_EVENT_KINDS].sort())
  })
})
