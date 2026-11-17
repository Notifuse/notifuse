import type { ReactNode } from 'react'
import { useLingui } from '@lingui/react/macro'
import { FullDrawer } from '../../layout'

export interface IntegrationDrawerProps {
  onClose: () => void
  /** "Add Mailgun integration": the bar's title and the dialog's name. */
  title: string
  /** Right side of the bar: Send a test email, Save. */
  topActions?: ReactNode
  children: ReactNode
}

/**
 * The full-screen drawer of an integration form. Mounted while open, so a form inside starts from
 * the integration every time. Its panels are centred at 820px, where the settings rows keep their
 * two columns.
 */
export function IntegrationDrawer({ onClose, title, topActions, children }: IntegrationDrawerProps) {
  const { t } = useLingui()
  return (
    <FullDrawer
      open
      onClose={onClose}
      backLabel={t`Integrations`}
      copyLink={false}
      ariaLabel={title}
      barTitle={
        <span data-drawer-title className="truncate text-sm font-semibold text-ink">
          {title}
        </span>
      }
      topActions={topActions}
    >
      <div className="mx-auto flex w-full max-w-[820px] flex-col gap-3">{children}</div>
    </FullDrawer>
  )
}
