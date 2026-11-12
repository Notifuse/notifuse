import { describe, it, expect } from 'vitest'
import { i18n } from '@lingui/core'
import type { ContactTimelineEntry } from '../../../services/api/contact_timeline'
import { activityLabel } from './activityLabel'

const entry = (patch: Partial<ContactTimelineEntry>): ContactTimelineEntry =>
  ({
    id: '1',
    email: 'a@x.com',
    operation: 'update',
    kind: '',
    changes: {},
    created_at: '',
    db_created_at: '',
    ...patch
  }) as ContactTimelineEntry

describe('activityLabel', () => {
  it('names the kind of activity and what it was about', () => {
    expect(
      activityLabel(
        entry({
          entity_type: 'message_history',
          kind: 'email.link_clicked',
          entity_data: { template_name: 'Newsletter' } as never
        }),
        i18n
      )
    ).toBe('Click · Newsletter')
    expect(activityLabel(entry({ entity_type: 'message_history', changes: { opened_at: {} } }), i18n)).toBe(
      'Open'
    )
    expect(
      activityLabel(entry({ entity_type: 'contact_list', entity_data: { name: 'Beta' } as never }), i18n)
    ).toBe('List · Beta')
    expect(
      activityLabel(
        entry({
          entity_type: 'contact_segment',
          kind: 'segment.left',
          entity_data: { name: 'Pro' } as never
        }),
        i18n
      )
    ).toBe('Left segment · Pro')
    expect(activityLabel(entry({ entity_type: 'contact', operation: 'insert' }), i18n)).toBe(
      'Contact created'
    )
    expect(activityLabel(entry({ entity_type: 'contact' }), i18n)).toBe('Profile updated')
    expect(
      activityLabel(
        entry({ entity_type: 'custom_event', entity_data: { event_name: 'order' } as never }),
        i18n
      )
    ).toBe('Event · order')
    expect(activityLabel(entry({ entity_type: 'web_session' }), i18n)).toBe('Site visit')
  })
})
