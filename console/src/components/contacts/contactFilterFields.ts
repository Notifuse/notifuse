import { useMemo } from 'react'
import { useLingui } from '@lingui/react/macro'
import { CountriesFormOptions } from '../../lib/countries_timezones'
import { Languages } from '../../lib/languages'
import type { FilterField } from '../filters/types'
import { useTrackingConsentOptions } from './trackingConsentOptions'

export type ContactFilterGroup = 'identity' | 'location' | 'tracking'

export interface ContactFilterField extends FilterField {
  group: ContactFilterGroup
  /** Matched exactly by the server; the other fields are matched as a substring (ILIKE '%…%'). */
  exact?: boolean
}

/** The fields of the filter picker. The list, its status and the segments are the view, the tabs and a pill. */
export function useContactFilterFields(): ContactFilterField[] {
  const { t } = useLingui()
  const consent = useTrackingConsentOptions()
  return useMemo(
    () => [
      { key: 'email', group: 'identity' as const, label: t`Email`, type: 'string' as const },
      { key: 'external_id', group: 'identity' as const, label: t`External ID`, type: 'string' as const },
      { key: 'first_name', group: 'identity' as const, label: t`First Name`, type: 'string' as const },
      { key: 'last_name', group: 'identity' as const, label: t`Last Name`, type: 'string' as const },
      { key: 'full_name', group: 'identity' as const, label: t`Full Name`, type: 'string' as const },
      { key: 'phone', group: 'identity' as const, label: t`Phone`, type: 'string' as const },
      { key: 'language', group: 'location' as const, label: t`Language`, type: 'string' as const, options: Languages },
      {
        key: 'country',
        group: 'location' as const,
        label: t`Country`,
        type: 'string' as const,
        options: CountriesFormOptions
      },
      {
        key: 'tracking_opens_consent',
        group: 'tracking' as const,
        exact: true,
        label: t`Opens tracking consent`,
        type: 'string' as const,
        options: consent.filters
      },
      {
        key: 'tracking_clicks_consent',
        group: 'tracking' as const,
        exact: true,
        label: t`Clicks tracking consent`,
        type: 'string' as const,
        options: consent.filters
      }
    ],
    [t, consent.filters]
  )
}
