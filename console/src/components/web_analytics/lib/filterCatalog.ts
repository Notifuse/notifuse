import { useLingui } from '@lingui/react/macro'
import {
  WEB_FILTER_SOURCE_FIELDS,
  WEB_FILTER_WRITABLE_DIMENSIONS,
  WebFilterAction,
  WebFilterOperator
} from '../../../services/api/web_analytics'
import { getDimensionLabel } from './dimensions'

/**
 * Vocabulary for the attribution rule builder. The field and dimension lists
 * are derived from the canonical arrays in services/api/web_analytics so a
 * rule can never reference something internal/domain/web_analytics_filters.go
 * rejects: adding an entry there fails the build here until it gets a label.
 */

export type FilterFieldCategory =
  | 'Channel'
  | 'UTM'
  | 'Traffic'
  | 'Pages'
  | 'Device'
  | 'Geo'
  | 'Custom'

/** Order the categories are rendered in, in every rule picker. */
const CATEGORY_ORDER: FilterFieldCategory[] = [
  'Channel',
  'UTM',
  'Traffic',
  'Pages',
  'Device',
  'Geo',
  'Custom'
]

export interface FilterFieldInfo {
  value: string
  label: string
  category: FilterFieldCategory
}

interface FieldMeta {
  label: string
  category: FilterFieldCategory
}

type SourceFieldName = (typeof WEB_FILTER_SOURCE_FIELDS)[number]
type WritableDimensionName = (typeof WEB_FILTER_WRITABLE_DIMENSIONS)[number]

const SOURCE_FIELD_META: Record<SourceFieldName, FieldMeta> = {
  utm_source: { label: 'UTM Source', category: 'UTM' },
  utm_medium: { label: 'UTM Medium', category: 'UTM' },
  utm_campaign: { label: 'UTM Campaign', category: 'UTM' },
  utm_term: { label: 'UTM Term', category: 'UTM' },
  utm_content: { label: 'UTM Content', category: 'UTM' },
  utm_id: { label: 'UTM ID', category: 'UTM' },
  utm_id_from: { label: 'UTM ID From', category: 'UTM' },

  referrer: { label: 'Referrer', category: 'Traffic' },
  referrer_domain: { label: 'Referrer Domain', category: 'Traffic' },
  referrer_path: { label: 'Referrer Path', category: 'Traffic' },
  is_direct: { label: 'Is Direct', category: 'Traffic' },

  landing_page: { label: 'Landing Page', category: 'Pages' },
  landing_domain: { label: 'Landing Domain', category: 'Pages' },
  landing_path: { label: 'Landing Path', category: 'Pages' },
  path: { label: 'Current Path', category: 'Pages' },

  device: { label: 'Device', category: 'Device' },
  browser: { label: 'Browser', category: 'Device' },
  browser_type: { label: 'Browser Type', category: 'Device' },
  os: { label: 'Operating System', category: 'Device' },
  user_agent: { label: 'User Agent', category: 'Device' },
  connection_type: { label: 'Connection Type', category: 'Device' },

  language: { label: 'Language', category: 'Geo' },
  timezone: { label: 'Timezone', category: 'Geo' }
}

const WRITABLE_DIMENSION_META: Record<WritableDimensionName, FieldMeta> = {
  channel: { label: 'Channel', category: 'Channel' },
  channel_group: { label: 'Channel Group', category: 'Channel' },

  custom_1: { label: 'Custom 1', category: 'Custom' },
  custom_2: { label: 'Custom 2', category: 'Custom' },
  custom_3: { label: 'Custom 3', category: 'Custom' },
  custom_4: { label: 'Custom 4', category: 'Custom' },
  custom_5: { label: 'Custom 5', category: 'Custom' },
  custom_6: { label: 'Custom 6', category: 'Custom' },
  custom_7: { label: 'Custom 7', category: 'Custom' },
  custom_8: { label: 'Custom 8', category: 'Custom' },
  custom_9: { label: 'Custom 9', category: 'Custom' },
  custom_10: { label: 'Custom 10', category: 'Custom' },

  utm_source: { label: 'UTM Source', category: 'UTM' },
  utm_medium: { label: 'UTM Medium', category: 'UTM' },
  utm_campaign: { label: 'UTM Campaign', category: 'UTM' },
  utm_term: { label: 'UTM Term', category: 'UTM' },
  utm_content: { label: 'UTM Content', category: 'UTM' },

  referrer_domain: { label: 'Referrer Domain', category: 'Traffic' },
  is_direct: { label: 'Is Direct', category: 'Traffic' }
}

/** Fields a rule condition may read. */
export const SOURCE_FIELDS: FilterFieldInfo[] = WEB_FILTER_SOURCE_FIELDS.map((value) => ({
  value,
  ...SOURCE_FIELD_META[value]
}))

/** Dimensions a rule operation may write. */
export const WRITABLE_DIMENSIONS: FilterFieldInfo[] = WEB_FILTER_WRITABLE_DIMENSIONS.map(
  (value) => ({ value, ...WRITABLE_DIMENSION_META[value] })
)

export const OPERATORS: readonly WebFilterOperator[] = [
  'equals',
  'not_equals',
  'contains',
  'not_contains',
  'is_empty',
  'is_not_empty',
  'regex'
]

/** Operators that carry no value; their input is hidden and the value cleared. */
export const VALUELESS_OPERATORS: readonly WebFilterOperator[] = ['is_empty', 'is_not_empty']

export const FILTER_ACTIONS: readonly WebFilterAction[] = [
  'set_value',
  'unset_value',
  'set_default_value'
]

/** Tag suggestions offered by the rule form on top of the workspace's own tags. */
export const SUGGESTED_TAGS: readonly string[] = [
  'channel',
  'channel group',
  'marketing',
  'paid',
  'organic',
  'social',
  'direct',
  'referral',
  'email',
  'content',
  'page category',
  'funnel'
]

export function isValuelessOperator(operator: WebFilterOperator): boolean {
  return VALUELESS_OPERATORS.includes(operator)
}

export interface FilterFieldOption {
  value: string
  label: string
}

export interface FilterFieldOptionGroup {
  label: string
  options: FilterFieldOption[]
}

function groupFields(
  fields: FilterFieldInfo[],
  labelFor: (field: FilterFieldInfo) => string
): FilterFieldOptionGroup[] {
  const groups = new Map<FilterFieldCategory, FilterFieldOption[]>()
  for (const field of fields) {
    const option: FilterFieldOption = { value: field.value, label: labelFor(field) }
    const bucket = groups.get(field.category)
    if (bucket) bucket.push(option)
    else groups.set(field.category, [option])
  }
  return CATEGORY_ORDER.filter((category) => groups.has(category)).map((category) => ({
    label: category,
    options: groups.get(category) as FilterFieldOption[]
  }))
}

export const SOURCE_FIELD_OPTIONS: FilterFieldOptionGroup[] = groupFields(
  SOURCE_FIELDS,
  (field) => field.label
)

const SOURCE_FIELD_LABELS: Record<string, string> = Object.fromEntries(
  SOURCE_FIELDS.map((field) => [field.value, field.label])
)

export function getSourceFieldLabel(field: string): string {
  return SOURCE_FIELD_LABELS[field] ?? field
}

/** Custom slots are renamed per workspace, so the options depend on settings. */
export function buildDimensionOptions(
  customLabels?: Record<string, string>
): FilterFieldOptionGroup[] {
  return groupFields(WRITABLE_DIMENSIONS, (field) => getDimensionLabel(field.value, customLabels))
}

export interface FilterVocabulary {
  operators: Record<WebFilterOperator, string>
  actions: Record<WebFilterAction, string>
  actionDescriptions: Record<WebFilterAction, string>
  /** Compact wording used inside the tags of the rule table. */
  actionShorthands: Record<WebFilterAction, string>
}

/** Translated wording for the operators and actions stored on a rule. */
export function useFilterVocabulary(): FilterVocabulary {
  const { t } = useLingui()
  return {
    operators: {
      equals: t`equals`,
      not_equals: t`not equals`,
      contains: t`contains`,
      not_contains: t`not contains`,
      is_empty: t`is empty`,
      is_not_empty: t`is not empty`,
      regex: t`matches regex`
    },
    actions: {
      set_value: t`Set value`,
      unset_value: t`Clear value`,
      set_default_value: t`Set default`
    },
    actionDescriptions: {
      set_value: t`Always set to the specified value`,
      unset_value: t`Set to null`,
      set_default_value: t`Set only when currently empty`
    },
    actionShorthands: {
      set_value: t`set to`,
      unset_value: t`unset`,
      set_default_value: t`default to`
    }
  }
}
