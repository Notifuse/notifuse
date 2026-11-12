import type { ReactNode } from 'react'
import { App, Button, Tooltip } from 'antd'
import { ChevronDown, ChevronUp, Link2, X } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { copyToClipboard } from '../../lib/copyToClipboard'
import { useNumberFormat } from './format'

/** Previous and next of a drawer that walks through a list. */
export interface DrawerNav {
  /** 1-based position of the open item in the loaded list. */
  position?: number
  total?: number
  onPrev?: () => void
  onNext?: () => void
  /** After the position ("· Pro plan"). */
  context?: ReactNode
}

/** Previous and next buttons, then "4 of 12" when the position is known. */
export function DrawerNavButtons({ nav, showPosition }: { nav: DrawerNav; showPosition?: boolean }) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const hasPosition = nav.position !== undefined && nav.total !== undefined
  // Named after Lingui's placeholders: the catalogues read "{position} of {total}".
  const position = hasPosition ? int(nav.position!) : ''
  const total = hasPosition ? int(nav.total!) : ''
  return (
    <>
      <Tooltip title={t`Previous (K)`}>
        <Button
          type="text"
          size="small"
          aria-label={t`Previous`}
          icon={<ChevronUp size={15} aria-hidden="true" />}
          disabled={!nav.onPrev}
          onClick={nav.onPrev}
        />
      </Tooltip>
      <Tooltip title={t`Next (J)`}>
        <Button
          type="text"
          size="small"
          aria-label={t`Next`}
          icon={<ChevronDown size={15} aria-hidden="true" />}
          disabled={!nav.onNext}
          onClick={nav.onNext}
        />
      </Tooltip>
      {showPosition && (hasPosition || nav.context) ? (
        <span className="ml-1 flex min-w-0 items-center gap-1.5 text-xs text-ink-3 tabular-nums">
          {hasPosition ? <span data-drawer-position className="shrink-0">{t`${position} of ${total}`}</span> : null}
          {hasPosition && nav.context ? <span aria-hidden="true">·</span> : null}
          {nav.context ? <span className="truncate">{nav.context}</span> : null}
        </span>
      ) : null}
    </>
  )
}

/** Copies the page URL, which opens the same item again. */
export function CopyLinkButton() {
  const { t } = useLingui()
  const { message } = App.useApp()
  const handleCopy = async () => {
    if (await copyToClipboard(window.location.href)) message.success(t`Link copied`)
    else message.error(t`Failed to copy to clipboard`)
  }
  return (
    <Tooltip title={t`Copy link`}>
      <Button type="text" size="small" aria-label={t`Copy link`} icon={<Link2 size={15} aria-hidden="true" />} onClick={handleCopy} />
    </Tooltip>
  )
}

export function CloseButton({ onClose }: { onClose: () => void }) {
  const { t } = useLingui()
  return <Button type="text" size="small" aria-label={t`Close`} icon={<X size={16} aria-hidden="true" />} onClick={onClose} />
}
