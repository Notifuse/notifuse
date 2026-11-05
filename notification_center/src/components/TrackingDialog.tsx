import { useEffect, useId, useRef } from 'react'
import { Button } from '@/components/ui/button'
import type { ContactTrackingState } from '../api/notification_center'

export type TrackingKind = 'opens' | 'clicks'

/** What the dialog shows: the choice of each kind, or the email link's opt-out confirmation. */
export type TrackingDialogMode = 'manage' | 'optout'

// The look of an outlined button in the primary color, for every tracking action.
const primaryGhost = 'cursor-pointer border-blue-500 text-blue-500 hover:bg-blue-50 hover:text-blue-600'

interface TrackingDialogProps {
  mode: TrackingDialogMode
  tracking: ContactTrackingState
  saving: boolean
  /** Set when the opt-out could not be saved. */
  optoutError?: string
  t: (key: string) => string
  onToggle: (kind: TrackingKind, allowed: boolean) => void
  onConfirmOptout: () => void
  onClose: () => void
}

/** A modal over the notification center where the contact changes their tracking choice. */
export function TrackingDialog({
  mode,
  tracking,
  saving,
  optoutError,
  t,
  onToggle,
  onConfirmOptout,
  onClose
}: TrackingDialogProps) {
  const titleId = useId()
  const dialogRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    dialogRef.current?.focus()
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [onClose])

  const rows = (
    [
      {
        kind: 'opens',
        enabled: tracking.opens_enabled,
        tracked: tracking.opens_tracked,
        label: t('openTracking'),
        description: t('openTrackingDescription')
      },
      {
        kind: 'clicks',
        enabled: tracking.clicks_enabled,
        tracked: tracking.clicks_tracked,
        label: t('clickTracking'),
        description: t('clickTrackingDescription')
      }
    ] as const
  ).filter((row) => row.enabled)

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onClick={onClose}>
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        data-testid="tracking-dialog"
        className="w-full max-w-md rounded-md bg-white p-5 shadow-lg focus:outline-none"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id={titleId} className="text-md font-medium">
          {t('emailTracking')}
        </h2>

        {mode === 'optout' ? (
          <>
            <p className="mt-2 text-sm text-gray-600">{t('trackingOptoutPrompt')}</p>
            {optoutError && (
              <p role="alert" className="mt-3 text-sm text-red-600">
                {optoutError}
              </p>
            )}
            <div className="mt-5 flex justify-end gap-2">
              <Button variant="ghost" size="sm" className="cursor-pointer" onClick={onClose} disabled={saving}>
                {t('cancel')}
              </Button>
              <Button
                variant="outline"
                size="sm"
                className={primaryGhost}
                onClick={onConfirmOptout}
                disabled={saving}
              >
                {saving ? t('processing') : t('trackingOptoutConfirm')}
              </Button>
            </div>
          </>
        ) : (
          <>
            <p className="mt-2 mb-4 text-sm text-gray-600">{t('emailTrackingIntro')}</p>
            <div className="space-y-3">
              {rows.map((row) => (
                <div key={row.kind} className="flex items-center justify-between gap-4">
                  <div className="flex-1">
                    <div className="text-sm font-medium">
                      {row.label}
                      <span className="ml-2 rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-600">
                        {row.tracked ? t('trackingOn') : t('trackingOff')}
                      </span>
                    </div>
                    <p className="mt-1 text-xs text-gray-500">{row.description}</p>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => onToggle(row.kind, !row.tracked)}
                    disabled={saving}
                    aria-label={`${row.tracked ? t('trackingStop') : t('trackingAllow')}: ${row.label}`}
                    className={primaryGhost}
                  >
                    {row.tracked ? t('trackingStop') : t('trackingAllow')}
                  </Button>
                </div>
              ))}
            </div>
            <div className="mt-5 flex justify-end">
              <Button variant="ghost" size="sm" className="cursor-pointer" onClick={onClose}>
                {t('close')}
              </Button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
