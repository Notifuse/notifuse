import { useLingui } from '@lingui/react/macro'
import type { Workspace } from '../../services/api/types'
import { getCustomFieldLabel } from '../../hooks/useCustomFieldLabel'

export const CUSTOM_STRING = [1, 2, 3, 4, 5].map(
  (n) => `custom_string_${n}` as `custom_string_${1 | 2 | 3 | 4 | 5}`
)
export const CUSTOM_NUMBER = [1, 2, 3, 4, 5].map(
  (n) => `custom_number_${n}` as `custom_number_${1 | 2 | 3 | 4 | 5}`
)
export const CUSTOM_DATETIME = [1, 2, 3, 4, 5].map(
  (n) => `custom_datetime_${n}` as `custom_datetime_${1 | 2 | 3 | 4 | 5}`
)
export const CUSTOM_JSON = [1, 2, 3, 4, 5].map(
  (n) => `custom_json_${n}` as `custom_json_${1 | 2 | 3 | 4 | 5}`
)

export const DEFAULT_VISIBLE_COLUMNS: Record<string, boolean> = {
  first_name: false,
  last_name: false,
  full_name: false,
  language: true,
  timezone: false,
  country: true,
  lists: true,
  segments: true,
  phone: false,
  address: false,
  job_title: false,
  created_at: false,
  custom_string_1: false,
  custom_string_2: false,
  custom_string_3: false,
  custom_string_4: false,
  custom_string_5: false,
  custom_number_1: false,
  custom_number_2: false,
  custom_number_3: false,
  custom_number_4: false,
  custom_number_5: false,
  custom_datetime_1: false,
  custom_datetime_2: false,
  custom_datetime_3: false,
  custom_datetime_4: false,
  custom_datetime_5: false,
  custom_json_1: false,
  custom_json_2: false,
  custom_json_3: false,
  custom_json_4: false,
  custom_json_5: false,
  tracking_consent: false
}

/** The columns a member can show or hide, in the selector's order. */
export function useSelectableColumns(
  workspace: Workspace | undefined
): { key: string; title: string }[] {
  const { t } = useLingui()
  return [
    { key: 'lists', title: t`Lists` },
    { key: 'segments', title: t`Segments` },
    { key: 'first_name', title: t`First Name` },
    { key: 'last_name', title: t`Last Name` },
    { key: 'full_name', title: t`Full Name` },
    { key: 'phone', title: t`Phone` },
    { key: 'country', title: t`Country` },
    { key: 'language', title: t`Language` },
    { key: 'timezone', title: t`Timezone` },
    { key: 'address', title: t`Address` },
    { key: 'job_title', title: t`Job Title` },
    { key: 'created_at', title: t`Created At` },
    { key: 'tracking_consent', title: t`Tracking consent` },
    ...[...CUSTOM_STRING, ...CUSTOM_NUMBER, ...CUSTOM_DATETIME, ...CUSTOM_JSON].map((key) => ({
      key,
      title: getCustomFieldLabel(key, workspace)
    }))
  ]
}
