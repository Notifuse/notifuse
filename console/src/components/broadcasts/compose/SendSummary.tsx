import type { ReactNode } from 'react'
import { useLingui } from '@lingui/react/macro'
import { CircleAlert, CircleCheck, CircleX, ListChecks } from 'lucide-react'
import { Panel } from '../../layout'
import {
  useCheckText,
  type CheckLevel,
  type CheckTextInput,
  type SummaryCheck
} from './sendSummaryChecks'

export interface SendSummaryProps extends CheckTextInput {
  checks: SummaryCheck[]
}

const ICON: Record<CheckLevel, ReactNode> = {
  ok: <CircleCheck size={14} className="text-success-fg" aria-hidden="true" />,
  warning: <CircleAlert size={14} className="text-warning-fg" aria-hidden="true" />,
  error: <CircleX size={14} className="text-danger-fg" aria-hidden="true" />
}

/** The checks before sending, each with its level. */
export function SendSummary(props: SendSummaryProps) {
  const { t } = useLingui()
  const text = useCheckText(props)
  return (
    <Panel title={t`Summary`} icon={<ListChecks size={14} />}>
      <ul data-unreset className="m-0 flex list-none flex-col gap-2 p-0">
        {props.checks.map((check) => (
          <li key={check.key} data-level={check.level} className="flex items-start gap-2 text-sm text-ink">
            <span className="mt-0.5 flex shrink-0">{ICON[check.level]}</span>
            <span className="min-w-0">{text(check)}</span>
          </li>
        ))}
      </ul>
    </Panel>
  )
}
