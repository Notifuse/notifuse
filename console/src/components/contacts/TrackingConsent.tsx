import { useMemo, useState } from 'react'
import { App, Select, Space, Tag } from 'antd'
import dayjs from '../../lib/dayjs'
import { useLingui } from '@lingui/react/macro'
import type { TrackingConsent } from '../../services/api/contacts'

/** One consent as a tag: granted, denied, or unknown when none is recorded. */
export function TrackingConsentTag({ consent }: { consent?: TrackingConsent | null }) {
  const { t } = useLingui()
  if (consent === 'granted') return <Tag color="green">{t`Granted`}</Tag>
  if (consent === 'denied') return <Tag color="red">{t`Denied`}</Tag>
  return <Tag>{t`Unknown`}</Tag>
}

/** Both consents, as the contacts table shows them. */
export function TrackingConsentSummary({
  opens,
  clicks
}: {
  opens?: TrackingConsent | null
  clicks?: TrackingConsent | null
}) {
  const { t } = useLingui()
  return (
    <Space size={4} direction="vertical">
      <span>
        {t`Opens`} <TrackingConsentTag consent={opens} />
      </span>
      <span>
        {t`Clicks`} <TrackingConsentTag consent={clicks} />
      </span>
    </Space>
  )
}

/** The consent values a form or filter offers. */
export function useTrackingConsentOptions() {
  const { t } = useLingui()
  return useMemo(
    () => ({
      values: [
        { value: 'granted', label: t`Granted` },
        { value: 'denied', label: t`Denied` }
      ],
      filters: [
        { value: 'granted', label: t`Granted` },
        { value: 'denied', label: t`Denied` },
        { value: 'unknown', label: t`Unknown` }
      ]
    }),
    [t]
  )
}

/** A readable name for where a consent change came from. */
export function useTrackingConsentSourceLabel() {
  const { t } = useLingui()
  return (source?: string) => {
    switch (source) {
      case 'api':
        return t`API`
      case 'console':
        return t`Console`
      case 'import':
        return t`Import`
      case 'notification_center':
        return t`Notification center`
      case 'email_link':
        return t`Email link`
      case 'subscribe_form':
        return t`Subscription form`
      default:
        return source || '-'
    }
  }
}

interface TrackingConsentEditorProps {
  opens?: TrackingConsent | null
  clicks?: TrackingConsent | null
  updatedAt?: string
  source?: string
  disabled?: boolean
  /** Saves one consent; null records it as unknown again. */
  onSave: (field: 'tracking_opens_consent' | 'tracking_clicks_consent', value: TrackingConsent | null) => Promise<void>
}

/** The tracking consent block of the contact details. */
export function TrackingConsentEditor({
  opens,
  clicks,
  updatedAt,
  source,
  disabled,
  onSave
}: TrackingConsentEditorProps) {
  const { t } = useLingui()
  const { filters } = useTrackingConsentOptions()
  const sourceLabel = useTrackingConsentSourceLabel()
  const [saving, setSaving] = useState<string | null>(null)
  const { message } = App.useApp()
  const changedAt = updatedAt ? dayjs(updatedAt).format('LLL') : ''
  const changedFrom = sourceLabel(source)

  const save = async (field: 'tracking_opens_consent' | 'tracking_clicks_consent', value: string) => {
    setSaving(field)
    try {
      await onSave(field, value === 'unknown' ? null : (value as TrackingConsent))
    } catch (error) {
      message.error((error as Error)?.message || t`Failed to update field`)
    } finally {
      setSaving(null)
    }
  }

  return (
    <div className="mb-4">
      <div className="text-xs font-medium text-gray-500 mb-2">{t`Tracking consent`}</div>
      <Space direction="vertical" size={8} style={{ width: '100%' }}>
        <div className="flex items-center justify-between">
          <span>{t`Opens`}</span>
          <Select
            size="small"
            style={{ width: 140 }}
            value={opens || 'unknown'}
            options={filters}
            disabled={disabled}
            loading={saving === 'tracking_opens_consent'}
            onChange={(value) => save('tracking_opens_consent', value)}
          />
        </div>
        <div className="flex items-center justify-between">
          <span>{t`Clicks`}</span>
          <Select
            size="small"
            style={{ width: 140 }}
            value={clicks || 'unknown'}
            options={filters}
            disabled={disabled}
            loading={saving === 'tracking_clicks_consent'}
            onChange={(value) => save('tracking_clicks_consent', value)}
          />
        </div>
        {changedAt && (
          <div className="text-xs text-gray-500">{t`Last changed ${changedAt} from ${changedFrom}`}</div>
        )}
      </Space>
    </div>
  )
}
