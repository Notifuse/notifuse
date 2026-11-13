import { useEffect, useState } from 'react'
import { App, Button, Select, Space, Tag, Tooltip } from 'antd'
import { Info, Pencil } from 'lucide-react'
import dayjs from '../../lib/dayjs'
import { useLingui } from '@lingui/react/macro'
import type { TrackingConsent } from '../../services/api/contacts'
import { useTrackingConsentOptions } from './trackingConsentOptions'

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

/** A readable name for where a consent change came from. */
function useTrackingConsentSourceLabel() {
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
 * The tracking consent panel of the contact sheet: both consents as tags, the date and
 * source of the last change in a tooltip, and an edit button that opens a form for both.
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

  // When and where the consents last changed, beside the values.
  const changeInfo = changedAt ? (
    <Tooltip title={t`Last changed ${changedAt} from ${changedFrom}`} trigger={['hover', 'focus']}>
      <span
        role="img"
        tabIndex={0}
        aria-label={t`Last change`}
        data-testid="tracking-consent-changed"
        className="flex text-ink-4"
      >
        <Info size={14} aria-hidden="true" />
      </span>
    </Tooltip>
  ) : null

  if (editing) {
    return (
      <div className="flex flex-col gap-3 text-sm">
        <div className="grid grid-cols-[auto_1fr] items-center gap-x-3 gap-y-2">
          <span className="text-ink-3">{t`Opens`}</span>
          <Select
            size="small"
            aria-label={t`Opens`}
            value={draftOpens}
            options={filters}
            disabled={saving}
            onChange={setDraftOpens}
          />
          <span className="text-ink-3">{t`Clicks`}</span>
          <Select
            size="small"
            aria-label={t`Clicks`}
            value={draftClicks}
            options={filters}
            disabled={saving}
            onChange={setDraftClicks}
          />
        </div>
        <div className="flex justify-end gap-2">
          <Button size="small" onClick={() => setEditing(false)} disabled={saving}>
            {t`Cancel`}
          </Button>
          <Button size="small" type="primary" onClick={save} loading={saving}>
            {t`Save`}
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="flex items-start justify-between gap-3 text-sm">
      <div className="grid grid-cols-[auto_1fr] items-center gap-x-3 gap-y-1.5">
        <span className="text-ink-3">{t`Opens`}</span>
        <span>
          <TrackingConsentTag consent={opens} />
        </span>
        <span className="text-ink-3">{t`Clicks`}</span>
        <span>
          <TrackingConsentTag consent={clicks} />
        </span>
      </div>
      <div className="flex items-center gap-1">
        {changeInfo}
        <Button
          type="text"
          size="small"
          aria-label={t`Edit tracking consent`}
          icon={<Pencil size={14} aria-hidden="true" />}
          disabled={disabled}
          onClick={startEdit}
        />
      </div>
    </div>
  )
}
