import { describe, expect, it } from 'vitest'
import type { TransactionalNotification } from '../../services/api/transactional_notifications'
import type { Workspace } from '../../services/api/types'
import {
  createRequest,
  dataToSend,
  draftOf,
  generateApiId,
  mergeDraft,
  sameDraft,
  trackingEffect,
  trackingToSend,
  updateRequest
} from './notificationForm'

const workspace = { id: 'ws1', settings: { website_url: 'https://www.acme.com' } } as unknown as Workspace

const notification = {
  id: 'password_reset',
  name: 'Password reset',
  description: 'Sent on request',
  channels: { email: { template_id: 'tpl', settings: { kept: true } } },
  tracking_settings: { enable_tracking: true, utm_source: 'acme.com', utm_term: 'spring', endpoint: 'https://t.acme.com' },
  metadata: { category: 'auth' },
  created_at: '',
  updated_at: ''
} as unknown as TransactionalNotification

describe('generateApiId', () => {
  it('lowercases, drops punctuation and joins words with one underscore', () => {
    expect(generateApiId('Order  Shipped!')).toBe('order_shipped')
    expect(generateApiId('')).toBe('')
  })

  it('fits 32 characters', () => {
    expect(generateApiId('Password reset email for a user list')).toBe('password_reset_email_for_a_user')
  })
})

describe('draftOf', () => {
  it('starts a new notification from the UTM defaults', () => {
    expect(draftOf(undefined, workspace).tracking_settings).toEqual({
      tracking_mode: 'inherit',
      utm_source: 'acme.com',
      utm_medium: 'email',
      utm_campaign: 'transactional',
      utm_content: '',
      utm_term: ''
    })
  })

  it('reads a stored absent mode as inherit, and fills every field', () => {
    const draft = draftOf(notification, workspace)
    expect(draft.tracking_settings.tracking_mode).toBe('inherit')
    expect(draft.tracking_settings.utm_term).toBe('spring')
    expect(draft.tracking_settings.utm_medium).toBe('')
    expect(draft.channels.email.template_id).toBe('tpl')
  })
})

describe('mergeDraft and sameDraft', () => {
  const base = draftOf(notification, workspace)

  it('keeps the base for fields the form has not reported', () => {
    const merged = mergeDraft(base, { tracking_settings: { utm_term: 'summer' } as never })
    expect(merged.channels.email.template_id).toBe('tpl')
    expect(merged.tracking_settings.utm_term).toBe('summer')
    expect(merged.tracking_settings.utm_source).toBe('acme.com')
  })

  it('reads a template the selector cleared (null) as no template, not the starting one', () => {
    const merged = mergeDraft(base, { channels: { email: { template_id: null as unknown as string } } })
    expect(merged.channels.email.template_id).toBe('')
    expect(sameDraft(base, merged)).toBe(false)
  })

  it('compares every field, the tracking ones included', () => {
    expect(sameDraft(base, mergeDraft(base, {}))).toBe(true)
    expect(sameDraft(base, mergeDraft(base, { description: '' }))).toBe(false)
    expect(sameDraft(base, mergeDraft(base, { tracking_settings: { utm_term: '' } as never }))).toBe(false)
  })
})

describe('requests', () => {
  it('keeps what the stored tracking block holds beyond the form', () => {
    expect(trackingToSend(draftOf(notification, workspace).tracking_settings, notification.tracking_settings)).toEqual({
      enable_tracking: true,
      endpoint: 'https://t.acme.com',
      tracking_mode: 'inherit',
      utm_source: 'acme.com',
      utm_medium: '',
      utm_campaign: '',
      utm_content: '',
      utm_term: 'spring'
    })
  })

  it('updates with an emptied description, the channel settings kept and no metadata', () => {
    const draft = { ...draftOf(notification, workspace), description: '' }
    const { updates } = updateRequest(draft, notification, 'ws1')
    expect(updates.description).toBe('')
    expect(updates.channels).toEqual({ email: { template_id: 'tpl', settings: { kept: true } } })
    expect('metadata' in updates).toBe(false)
  })

  it('leaves channels out for a notification managed by an integration', () => {
    const managed = { ...notification, integration_id: 'supabase' }
    expect(updateRequest(draftOf(managed, workspace), managed, 'ws1').updates.channels).toBeUndefined()
  })

  it('creates with the draft as typed', () => {
    const draft = { ...draftOf(undefined, workspace), id: 'x', name: 'X', channels: { email: { template_id: 't' } } }
    expect(createRequest(draft, 'ws1').notification).toEqual({
      id: 'x',
      name: 'X',
      description: '',
      channels: { email: { template_id: 't' } },
      tracking_settings: draft.tracking_settings
    })
  })
})

describe('dataToSend', () => {
  it('leaves out the variables every send fills in', () => {
    expect(dataToSend({ order: { id: 1 }, contact: { email: 'a' }, unsubscribe_url: 'u' })).toEqual({ order: { id: 1 } })
    expect(dataToSend(undefined)).toEqual({})
  })
})

describe('trackingEffect', () => {
  const on = { opens_enabled: true, clicks_enabled: true, unknown_consent: 'track' as const }
  it('follows the workspace, never turning on what it turned off', () => {
    expect(trackingEffect('inherit', on)).toEqual({ opens: 'on', clicks: 'on', utm: true })
    expect(trackingEffect('inherit', { ...on, clicks_enabled: false })).toEqual({
      opens: 'on',
      clicks: 'workspaceOff',
      utm: true
    })
  })

  it('keeps UTM parameters without tracking, and drops everything when disabled', () => {
    expect(trackingEffect('no_tracking', on)).toEqual({ opens: 'off', clicks: 'off', utm: true })
    expect(trackingEffect('disabled', on)).toEqual({ opens: 'off', clicks: 'off', utm: false })
  })
})
