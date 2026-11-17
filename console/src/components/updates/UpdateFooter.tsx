import { useLingui } from '@lingui/react/macro'

interface UpdateFooterProps {
  /** The version shown, as the account menu shows it. */
  version: string
  /** The release whose card was dismissed, offered as a discreet link; null for none. */
  dismissedLatest: string | null
  onOpen: () => void
}

/** The foot of the workspace list: this server's version and, once the card was dismissed, a way back. */
export function UpdateFooter({ version, dismissedLatest, onOpen }: UpdateFooterProps) {
  const { t } = useLingui()
  return (
    <div className="flex items-center gap-2.5 border-t border-line-soft pt-2.5 text-2xs text-ink-3">
      <span className="font-mono">Notifuse v{version}</span>
      {dismissedLatest && (
        <button
          type="button"
          onClick={onOpen}
          className="group inline-flex cursor-pointer items-center gap-1.5 border-0 bg-transparent p-0 text-xs"
        >
          <span aria-hidden className="inline-block size-1.5 rounded-full bg-primary-500" />
          {/* The colour sits on the span: the unlayered button reset outranks a utility on the button. */}
          <span className="text-primary-600 group-hover:underline">{t`${dismissedLatest} available`}</span>
        </button>
      )}
    </div>
  )
}
