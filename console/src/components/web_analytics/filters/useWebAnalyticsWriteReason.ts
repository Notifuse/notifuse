import { useLingui } from '@lingui/react/macro'

/** Why a write control is disabled for a member without the web analytics write permission. */
export function useWebAnalyticsWriteReason(): string {
  const { t } = useLingui()
  return t`You don't have write permission for web analytics`
}
