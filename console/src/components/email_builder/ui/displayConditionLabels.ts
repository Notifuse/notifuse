import { useLingui } from '@lingui/react/macro'
import type { DisplayConditionOperator } from '../types'

export const CONDITION_MONO = 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace'

// useOperatorLabels names each operator as a rule reads: "order.total is greater than 100".
export function useOperatorLabels(): Record<DisplayConditionOperator, string> {
  const { t } = useLingui()
  return {
    is_true: t`is true`,
    is_blank: t`is false or empty`,
    is_present: t`is set`,
    is_absent: t`is not set`,
    eq: t`equals`,
    neq: t`does not equal`,
    contains: t`contains`,
    not_contains: t`does not contain`,
    gt: t`is greater than`,
    gte: t`is greater than or equal to`,
    lt: t`is less than`,
    lte: t`is less than or equal to`
  }
}
