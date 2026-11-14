import { useLingui } from '@lingui/react/macro'
import type { Workspace } from '../../../services/api/types'
import { computeCustomFieldLabel } from '../../../hooks/useCustomFieldLabel'

export type StandardField =
  | 'first_name'
  | 'last_name'
  | 'full_name'
  | 'phone'
  | 'address_line_1'
  | 'address_line_2'
  | 'country'
  | 'state'
  | 'postcode'
  | 'job_title'
  | 'timezone'
  | 'language'
  | 'external_id'

/** The names of the contact's standard fields, in the console's language. */
export function useStandardFieldLabels(): Record<StandardField, string> {
  const { t } = useLingui()
  return {
    first_name: t`First Name`,
    last_name: t`Last Name`,
    full_name: t`Full Name`,
    phone: t`Phone`,
    address_line_1: t`Address Line 1`,
    address_line_2: t`Address Line 2`,
    country: t`Country`,
    state: t`State`,
    postcode: t`Postcode`,
    job_title: t`Job Title`,
    timezone: t`Timezone`,
    language: t`Language`,
    external_id: t`External ID`
  }
}

/**
 * The name of any contact field: a standard one, a tracking consent field the contact trigger also
 * records, or a custom one with the workspace's label.
 */
export function useContactFieldLabel(workspace: Workspace): (key: string) => string {
  const { t } = useLingui()
  const labels: Record<string, string> = {
    ...useStandardFieldLabels(),
    tracking_opens_consent: t`Opens tracking consent`,
    tracking_clicks_consent: t`Clicks tracking consent`,
    tracking_consent_source: t`Tracking consent source`
  }
  return (key) =>
    key in labels
      ? labels[key]
      : key.startsWith('custom_')
        ? computeCustomFieldLabel(key, workspace).displayLabel
        : key
}
