import type { ReactNode } from 'react'
import { ConfigProvider } from 'antd'

export interface SettingsRowProps {
  label: ReactNode
  help?: ReactNode
  /** Id of the field, so the label is a <label> that focuses it. */
  htmlFor?: string
  /** Beside the label, on its right (rare). */
  action?: ReactNode
  children: ReactNode
}

/**
 * One row of a settings form: label and help on the left, the field on the right. Put rows in a
 * `SettingsRows`, inside a Panel. Under 720px of container the row stacks: a container query, as
 * the room left depends on the sidebar and the rail, not on the window.
 */
export function SettingsRow({ label, help, htmlFor, action, children }: SettingsRowProps) {
  const labelClass = 'text-sm font-medium text-ink'
  return (
    <div
      data-settings-row
      className="grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)] gap-x-6 gap-y-2 border-t border-line-soft py-3 first:border-t-0 @max-[720px]:grid-cols-1"
    >
      <div className="flex min-w-0 flex-col gap-0.5">
        <div className="flex items-center justify-between gap-2">
          {htmlFor ? (
            <label htmlFor={htmlFor} className={labelClass}>
              {label}
            </label>
          ) : (
            <span className={labelClass}>{label}</span>
          )}
          {action}
        </div>
        {help ? <div className="line-clamp-2 text-xs text-ink-3">{help}</div> : null}
      </div>
      <div data-settings-field className="min-w-0 max-w-[420px]">
        {children}
      </div>
    </div>
  )
}

// The row spaces its rows itself: a form item inside drops the bottom margin antd gives stacked
// labelled fields, and keeps its validation message under the field.
const ROW_FORM_THEME = { components: { Form: { itemMarginBottom: 0 } } }

/** The rows of a Panel: the container the rows' query reads, and the form theme of their fields. */
export function SettingsRows({ children }: { children: ReactNode }) {
  return (
    <ConfigProvider theme={ROW_FORM_THEME}>
      <div data-settings-rows className="@container -my-3">
        {children}
      </div>
    </ConfigProvider>
  )
}
