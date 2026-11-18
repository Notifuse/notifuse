import { useLingui } from '@lingui/react/macro'

export interface NotificationCenterListPreviewProps {
  name: string
  description?: string
}

/**
 * A public list as a contact sees it in the notification center before joining it: the row of
 * notification_center/src/App.tsx (grey row, name, description, a blue outlined « Subscribe »).
 * Nothing in it is interactive.
 */
export function NotificationCenterListPreview({ name, description }: NotificationCenterListPreviewProps) {
  const { t } = useLingui()
  const shownName = name.trim()
  const shownDescription = description?.trim()

  return (
    <section aria-label={t`In the notification center`} className="flex flex-col gap-2">
      <div className="text-overline font-semibold uppercase text-ink-3">{t`In the notification center`}</div>
      <div className="flex items-center justify-between gap-4 rounded-sm border border-line bg-subtle p-4">
        <div className="min-w-0 flex-1">
          <div className={shownName ? 'font-medium text-ink' : 'font-medium text-ink-4'}>
            {shownName || t`List name`}
          </div>
          {shownDescription ? <p className="mt-1 text-sm text-ink-2">{shownDescription}</p> : null}
        </div>
        {/* The widget's button, drawn: a span, so it takes no focus and no click. */}
        <span
          aria-hidden="true"
          data-preview-subscribe
          className="shrink-0 rounded-md border border-primary px-3 py-1 text-sm text-primary"
        >
          {t`Subscribe`}
        </span>
      </div>
      <p className="text-xs text-ink-3">
        {t`Preview: the name and description as the contact sees them, with the subscribe button.`}
      </p>
    </section>
  )
}
