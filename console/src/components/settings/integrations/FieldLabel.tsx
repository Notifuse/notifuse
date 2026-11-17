import type { ReactNode } from 'react'
import { useLingui } from '@lingui/react/macro'

// The marks are drawn by CSS, as antd draws its own required mark: the label's text stays the
// field's name, which is what a label query and the `htmlFor` association read.

/** The label of a field the form refuses to save empty: a red star after it. */
export function RequiredLabel({ children }: { children: ReactNode }) {
  return <span className="after:ml-0.5 after:text-danger-fg after:content-['*']">{children}</span>
}

/** The label of a field that can stay empty: "optional" after it, in a lighter ink. */
export function OptionalLabel({ children }: { children: ReactNode }) {
  const { t } = useLingui()
  return (
    <span
      data-optional={t`optional`}
      className="after:ml-1 after:font-normal after:text-ink-4 after:content-[attr(data-optional)]"
    >
      {children}
    </span>
  )
}
