import type { ReactNode } from 'react'
import { Tooltip, type TooltipProps } from 'antd'
import { CircleHelp, Info } from 'lucide-react'

export interface HelpTipProps {
  /** The name of the button ("About these numbers"). */
  label: string
  /** The explanation: in the tooltip, and in a hidden copy a control can point to. */
  children: ReactNode
  /** The id of the hidden copy, for the aria-describedby of the control it explains. */
  id?: string
  /** The text the "?" follows (a panel or field title), set beside it. */
  title?: ReactNode
  /** The glyph: "help" (a "?", the default) or "info" (an "i", for the help of a settings field). */
  icon?: 'help' | 'info'
  /** Where the tooltip opens (default: antd's, above). */
  placement?: TooltipProps['placement']
}

/**
 * A "?" that holds the long explanation of a field. A button, so the keyboard reaches it and the
 * tooltip opens on focus as on hover; the hidden copy lets a screen reader hear it with the
 * control it explains, once.
 */
export function HelpTip({ label, children, id, title, icon = 'help', placement }: HelpTipProps) {
  const Glyph = icon === 'info' ? Info : CircleHelp
  const tip = (
    <>
      <Tooltip title={children} placement={placement} trigger={['hover', 'focus']} styles={{ root: { maxWidth: 360 } }}>
        <button
          type="button"
          aria-label={label}
          className="inline-flex cursor-help items-center rounded-full border-0 bg-transparent p-0 text-ink-4 hover:text-ink-2 focus-visible:text-ink-2"
        >
          <Glyph size={13} aria-hidden="true" data-icon={icon} />
        </button>
      </Tooltip>
      {/* Hidden, not visually hidden: a description reads it, browsing and a label's text do not. */}
      {id ? (
        <span id={id} hidden>
          {children}
        </span>
      ) : null}
    </>
  )
  return title === undefined ? (
    tip
  ) : (
    <span className="inline-flex items-center gap-1.5">
      {title}
      {tip}
    </span>
  )
}
