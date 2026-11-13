import { useMemo } from 'react'
import { useLingui } from '@lingui/react/macro'
import { CountriesFormOptions } from '../../lib/countries_timezones'
import { Languages } from '../../lib/languages'
import type { FilterField } from '../filters/types'
import { useTrackingConsentOptions } from './trackingConsentOptions'

/** The fields of « Filter by… ». The list, its status and the segments are the view, the tabs and a chip. */
export function useContactFilterFields(): FilterField[] {
  const { t } = useLingui()
  const consent = useTrackingConsentOptions()
  return useMemo(
    () => [
      { key: 'email', label: t`Email`, type: 'string' as const },
      { key: 'external_id', label: t`External ID`, type: 'string' as const },
      { key: 'first_name', label: t`First Name`, type: 'string' as const },
      { key: 'last_name', label: t`Last Name`, type: 'string' as const },
      { key: 'full_name', label: t`Full Name`, type: 'string' as const },
      { key: 'phone', label: t`Phone`, type: 'string' as const },
      { key: 'language', label: t`Language`, type: 'string' as const, options: Languages },
      { key: 'country', label: t`Country`, type: 'string' as const, options: CountriesFormOptions },
      {
        key: 'tracking_opens_consent',
        label: t`Opens tracking consent`,
        type: 'string' as const,
        options: consent.filters
      },
      {
        key: 'tracking_clicks_consent',
        label: t`Clicks tracking consent`,
        type: 'string' as const,
        options: consent.filters
      }
    ],
    [t, consent.filters]
  )
}
