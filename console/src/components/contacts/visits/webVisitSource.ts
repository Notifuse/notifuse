import type { I18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import type { ContactVisitSession } from '../../../services/api/web_analytics'

export type VisitSourceIcon = 'mail' | 'search' | 'social' | 'display' | 'video' | 'direct' | 'other'
export type DeviceIcon = 'desktop' | 'mobile' | 'tablet' | 'other'

export interface VisitSource {
  icon: VisitSourceIcon
  label: string
  /** The channel, then the UTM source when there is one. */
  detail: string
}

type SessionSource = Pick<
  ContactVisitSession,
  'channel_group' | 'channel' | 'referrer_domain' | 'utm_campaign' | 'utm_source'
>

/** Icon and label of a session's source, from its channel group (the attribution rules' output). */
export function visitSource(session: SessionSource, i18n: I18n): VisitSource {
  const referrer = session.referrer_domain || undefined
  const detail = [session.channel, session.utm_source].filter(Boolean).join(' · ')
  const source = (icon: VisitSourceIcon, label: string): VisitSource => ({ icon, label, detail })
  switch (session.channel_group) {
    case 'email':
      return source('mail', session.utm_campaign || i18n._(msg`Email`))
    case 'search-organic':
      return source('search', referrer ?? i18n._(msg`Organic search`))
    case 'search-paid':
      return source('search', referrer ?? i18n._(msg`Paid search`))
    case 'social-organic':
      return source('social', referrer ?? i18n._(msg`Organic social`))
    case 'social-paid':
      return source('social', referrer ?? i18n._(msg`Paid social`))
    case 'display-banner':
      return source('display', i18n._(msg`Display`))
    case 'video-paid':
      return source('video', i18n._(msg`Paid video`))
    case 'direct':
      return source('direct', i18n._(msg`Direct`))
    case 'not-mapped':
      return source('other', referrer ?? i18n._(msg`Other`))
    default:
      // A workspace's own attribution rule: its group name as written.
      return source('other', session.channel_group || i18n._(msg`Other`))
  }
}

export function deviceIcon(device: string): DeviceIcon {
  if (device === 'desktop' || device === 'mobile' || device === 'tablet') return device
  return 'other'
}
