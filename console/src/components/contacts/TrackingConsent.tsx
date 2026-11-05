import { useEffect, useMemo, useState } from 'react'
import { App, Button, Select, Space, Tag, Tooltip } from 'antd'
import { EditOutlined, InfoCircleOutlined } from '@ant-design/icons'
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

export type TrackingConsentField = 'tracking_opens_consent' | 'tracking_clicks_consent'

/** The consents to write; null records a consent as unknown again. */
export type TrackingConsentChanges = Partial<Record<TrackingConsentField, TrackingConsent | null>>

interface TrackingConsentEditorProps {
  opens?: TrackingConsent | null
  clicks?: TrackingConsent | null
  updatedAt?: string
  source?: string
  disabled?: boolean
  /** Saves the consents that changed, in one write. */
  onSave: (changes: TrackingConsentChanges) => Promise<void>
}

/**
 * The tracking consent row of the contact details: both consents as tags, the
 * date and source of the last change in a tooltip, and an edit button that
 * opens a form for both.
 */
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
  const { message } = App.useApp()
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [draftOpens, setDraftOpens] = useState<string>('unknown')
  const [draftClicks, setDraftClicks] = useState<string>('unknown')
  // The values the form opened with: only a consent the admin changed from these is
  // written, so one that changed elsewhere meanwhile is not overwritten.
  const [initial, setInitial] = useState({ opens: 'unknown', clicks: 'unknown' })
  const changedAt = updatedAt ? dayjs(updatedAt).format('LLL') : ''
  const changedFrom = sourceLabel(source)

  // A consent that changes elsewhere while the form is open shows its new value,
  // unless the admin already chose another one.
  useEffect(() => {
    if (!editing) return
    const nextOpens = opens || 'unknown'
    const nextClicks = clicks || 'unknown'
    if (nextOpens === initial.opens && nextClicks === initial.clicks) return
    if (draftOpens === initial.opens) setDraftOpens(nextOpens)
    if (draftClicks === initial.clicks) setDraftClicks(nextClicks)
    setInitial({ opens: nextOpens, clicks: nextClicks })
    // Only a change of the stored values matters here
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [opens, clicks])

  const startEdit = () => {
    setInitial({ opens: opens || 'unknown', clicks: clicks || 'unknown' })
    setDraftOpens(opens || 'unknown')
    setDraftClicks(clicks || 'unknown')
    setEditing(true)
  }

  const save = async () => {
    const changes: TrackingConsentChanges = {}
    if (draftOpens !== initial.opens) {
      changes.tracking_opens_consent = draftOpens === 'unknown' ? null : (draftOpens as TrackingConsent)
    }
    if (draftClicks !== initial.clicks) {
      changes.tracking_clicks_consent = draftClicks === 'unknown' ? null : (draftClicks as TrackingConsent)
    }
    if (Object.keys(changes).length === 0) {
      setEditing(false)
      return
    }
    setSaving(true)
    try {
      await onSave(changes)
      setEditing(false)
    } catch (error) {
      message.error((error as Error)?.message || t`Failed to update field`)
    } finally {
      setSaving(false)
    }
  }

  const label = (
    <span className="font-semibold text-slate-600">
      {t`Tracking consent`}
      {changedAt && (
        <Tooltip title={t`Last changed ${changedAt} from ${changedFrom}`} trigger={['hover', 'focus']}>
          <InfoCircleOutlined
            className="ml-1 text-gray-400"
            role="img"
            tabIndex={0}
            aria-label={t`Last change`}
            data-testid="tracking-consent-changed"
          />
        </Tooltip>
      )}
    </span>
  )

  if (editing) {
    return (
      <div className="py-2 px-4 text-xs bg-blue-50 border-b border-dashed border-gray-300">
        <div className="mb-2">{label}</div>
        <div className="grid grid-cols-2 gap-x-1 gap-y-2 items-center mb-2">
          <span className="text-slate-600">{t`Opens`}</span>
          <Select
            size="small"
            style={{ width: 140 }}
            aria-label={t`Opens`}
            value={draftOpens}
            options={filters}
            disabled={saving}
            onChange={setDraftOpens}
          />
          <span className="text-slate-600">{t`Clicks`}</span>
          <Select
            size="small"
            style={{ width: 140 }}
            aria-label={t`Clicks`}
            value={draftClicks}
            options={filters}
            disabled={saving}
            onChange={setDraftClicks}
          />
        </div>
        <div className="flex justify-end">
          <Space size="small">
            <Button size="small" type="link" onClick={() => setEditing(false)} disabled={saving}>
              {t`Cancel`}
            </Button>
            <Button size="small" type="primary" onClick={save} loading={saving}>
              {t`Save`}
            </Button>
          </Space>
        </div>
      </div>
    )
  }

  return (
    <div className="py-2 px-4 grid grid-cols-[1fr_auto] text-xs gap-1 border-b border-dashed border-gray-300">
      <div className="grid grid-cols-2 gap-1">
        {label}
        <TrackingConsentSummary opens={opens} clicks={clicks} />
      </div>
      <div className="flex items-start">
        {/* Laid out as narrow as the edit icon of the other fields, so the values line up;
            the negative margin keeps a larger click area */}
        <button
          type="button"
          className="-m-1.5 p-1.5 rounded border-0 bg-transparent leading-none text-gray-400 enabled:hover:text-blue-500 cursor-pointer disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-blue-500"
          aria-label={t`Edit tracking consent`}
          disabled={disabled}
          onClick={startEdit}
        >
          <EditOutlined />
        </button>
      </div>
    </div>
  )
}
