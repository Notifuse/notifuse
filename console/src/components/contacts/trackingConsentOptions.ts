import { useMemo } from 'react'
import { useLingui } from '@lingui/react/macro'

/** The consent values a form or filter offers. */
export function useTrackingConsentOptions() {
  const { t } = useLingui()
  return useMemo(
    () => ({
      values: [
        { value: 'granted', label: t`Granted` },
        { value: 'denied', label: t`Denied` }
      ],
      filters: [
        { value: 'granted', label: t`Granted` },
        { value: 'denied', label: t`Denied` },
        { value: 'unknown', label: t`Unknown` }
      ]
    }),
    [t]
  )
}
