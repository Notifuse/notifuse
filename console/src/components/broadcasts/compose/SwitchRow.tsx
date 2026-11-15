import { useId, type ReactNode } from 'react'
import { ConfigProvider, Switch } from 'antd'
import { HelpTip } from '../../layout'

// The fields above a switch row keep half of antd's margin: the row's rule and padding space
// them from it, as in the mockup.
const FIELDS_THEME = { components: { Form: { itemMarginBottom: 12 } } }

/** The fields of a card that ends with a SwitchRow. */
export function FieldsAboveSwitchRow({ children }: { children: ReactNode }) {
  return <ConfigProvider theme={FIELDS_THEME}>{children}</ConfigProvider>
}

export interface SwitchRowProps {
  /** The title beside the switch, and the switch's name. */
  title: string
  /** One line under the title. */
  hint: ReactNode
  /** The long explanation behind a "?", which becomes the switch's description. */
  help?: { label: string; content: ReactNode }
  checked: boolean
  onChange: (checked: boolean) => void
}

/**
 * A switch on the right of its title and one-line hint, under a rule: one line of the composer for
 * an on/off setting. The switch is described by the long explanation when there is one (the hint
 * says the same in short), else by the hint.
 */
export function SwitchRow({ title, hint, help, checked, onChange }: SwitchRowProps) {
  const id = useId()
  const hintId = `${id}-hint`
  const helpId = `${id}-help`
  return (
    <div className="flex items-center justify-between gap-4 border-t border-line-soft pt-3">
      <div className="flex min-w-0 flex-col">
        <span className="text-sm text-ink">
          {help ? (
            <HelpTip title={title} label={help.label} id={helpId}>
              {help.content}
            </HelpTip>
          ) : (
            title
          )}
        </span>
        <span id={hintId} className="text-xs text-ink-3">
          {hint}
        </span>
      </div>
      <Switch
        aria-label={title}
        aria-describedby={help ? helpId : hintId}
        checked={checked}
        onChange={(value) => onChange(value)}
      />
    </div>
  )
}
