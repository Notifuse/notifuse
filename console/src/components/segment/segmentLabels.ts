import { msg } from '@lingui/core/macro'
import type { I18n, MessageDescriptor } from '@lingui/core'
import { LIST_STATUS_LABEL } from '../contacts/contactDisplay'

/**
 * Every word the segment editor and the segment summary show for a field, an operator or an
 * option, as catalogue messages. The schemas (table_schemas.ts) keep their English titles as the
 * technical description of each column; what a person reads comes from here, so that it follows
 * the console's language.
 */

export { LIST_STATUS_LABEL }

// Contact columns, keyed by column name. `full_name` is not a column but the Contacts filters
// offer it, and the summary describes those filters too.
export const CONTACT_FIELD_LABELS: Record<string, MessageDescriptor> = {
  email: msg`Email`,
  external_id: msg`External ID`,
  first_name: msg`First Name`,
  last_name: msg`Last Name`,
  full_name: msg`Full Name`,
  phone: msg`Phone`,
  country: msg`Country`,
  language: msg`Language`,
  timezone: msg`Timezone`,
  address_line_1: msg`Address Line 1`,
  address_line_2: msg`Address Line 2`,
  postcode: msg`Postcode`,
  state: msg`State`,
  job_title: msg`Job Title`,
  created_at: msg`Created At`,
  updated_at: msg`Updated At`,
  tracking_opens_consent: msg`Opens tracking consent`,
  tracking_clicks_consent: msg`Clicks tracking consent`,
  tracking_consent_source: msg`Tracking consent source`,
  tracking_consent_updated_at: msg`Tracking consent changed at`
}

// What each contact column holds, shown when the pointer rests on a field.
export const CONTACT_FIELD_DESCRIPTIONS: Record<string, MessageDescriptor> = {
  email: msg`The contact's email address`,
  external_id: msg`The contact's identifier in your own system`,
  first_name: msg`The contact's first name`,
  last_name: msg`The contact's last name`,
  phone: msg`The contact's phone number`,
  country: msg`The contact's country, as a two-letter code`,
  language: msg`The contact's language`,
  timezone: msg`The contact's time zone`,
  address_line_1: msg`First line of the contact's address`,
  address_line_2: msg`Second line of the contact's address`,
  postcode: msg`The contact's postal code`,
  state: msg`The contact's state or region`,
  job_title: msg`The contact's job title`,
  created_at: msg`When the contact was created`,
  updated_at: msg`When the contact was last changed`,
  tracking_opens_consent: msg`Whether the contact agreed to open tracking`,
  tracking_clicks_consent: msg`Whether the contact agreed to click tracking`,
  tracking_consent_source: msg`Where the last tracking consent change came from`,
  tracking_consent_updated_at: msg`When the tracking consent last changed`
}

const CUSTOM_FIELD = /^custom_(string|number|datetime|json)_(\d+)$/

/** « Custom text 1 », « Custom number 2 »…: a custom column's name before the workspace renames it. */
function customFieldDefaultLabel(fieldName: string, i18n: I18n): string | undefined {
  const match = CUSTOM_FIELD.exec(fieldName)
  if (!match) return undefined
  const number = match[2]
  switch (match[1]) {
    case 'string':
      return i18n._(msg`Custom text ${number}`)
    case 'number':
      return i18n._(msg`Custom number ${number}`)
    case 'datetime':
      return i18n._(msg`Custom date ${number}`)
    default:
      return i18n._(msg`Custom JSON ${number}`)
  }
}

/** The keys of a web page view's `changes`, by the condition's schema name. */
const WEB_PAGEVIEW_FIELD_LABELS: Record<string, MessageDescriptor> = {
  path: msg`Path`,
  duration_ms: msg`Time on page (ms)`,
  max_scroll: msg`Scroll depth (%)`,
  page_number: msg`Page number in visit`,
  entry_type: msg`Entry type`,
  session_id: msg`Visit ID`,
  landing_domain: msg`Domain`
}

const WEB_PAGEVIEW_FIELD_DESCRIPTIONS: Record<string, MessageDescriptor> = {
  path: msg`Path of the page viewed, without the domain`,
  duration_ms: msg`Engaged time on the page, in milliseconds`,
  max_scroll: msg`Furthest scroll position reached, as a percentage`,
  page_number: msg`Position of the page within the visit, starting at 1`,
  entry_type: msg`How the visitor arrived on the page`,
  session_id: msg`Identifier of the visit the page belongs to`,
  landing_domain: msg`Domain the visit started on, which the path belongs to`
}

const WEB_SESSION_FIELD_LABELS: Record<string, MessageDescriptor> = {
  landing_path: msg`Entry page`,
  exit_path: msg`Exit page`,
  pageview_count: msg`Pages viewed`,
  duration_ms: msg`Visit duration (ms)`,
  max_scroll: msg`Scroll depth (%)`,
  goal_count: msg`Goals reached`,
  goal_value: msg`Goal value`,
  referrer_domain: msg`Referrer domain`,
  utm_source: msg`UTM source`,
  utm_medium: msg`UTM medium`,
  utm_campaign: msg`UTM campaign`,
  utm_content: msg`UTM content`,
  channel: msg`Channel`,
  channel_group: msg`Channel group`,
  device: msg`Device`,
  browser: msg`Browser`,
  os: msg`Operating system`,
  country: msg`Country`
}

const WEB_SESSION_FIELD_DESCRIPTIONS: Record<string, MessageDescriptor> = {
  landing_path: msg`Path of the first page of the visit`,
  exit_path: msg`Path of the last page of the visit`,
  pageview_count: msg`Number of pages viewed during the visit`,
  duration_ms: msg`Engaged time across the whole visit, in milliseconds`,
  max_scroll: msg`Deepest scroll reached on any page of the visit`,
  goal_count: msg`Number of goals fired during the visit`,
  goal_value: msg`Total value of the goals fired during the visit`,
  referrer_domain: msg`Domain the visitor arrived from`,
  utm_source: msg`utm_source of the visit`,
  utm_medium: msg`utm_medium of the visit`,
  utm_campaign: msg`utm_campaign of the visit`,
  utm_content: msg`utm_content of the visit, which names the creative`,
  channel: msg`Acquisition channel resolved from the attribution rules`,
  channel_group: msg`Group the acquisition channel belongs to`,
  device: msg`Device category of the visit`,
  browser: msg`Browser used for the visit`,
  os: msg`Operating system used for the visit`,
  country: msg`Country the visit came from`
}

const FIELD_LABELS_BY_SCHEMA: Record<string, Record<string, MessageDescriptor>> = {
  contacts: CONTACT_FIELD_LABELS,
  web_pageview_changes: WEB_PAGEVIEW_FIELD_LABELS,
  web_session_changes: WEB_SESSION_FIELD_LABELS
}

const FIELD_DESCRIPTIONS_BY_SCHEMA: Record<string, Record<string, MessageDescriptor>> = {
  contacts: CONTACT_FIELD_DESCRIPTIONS,
  web_pageview_changes: WEB_PAGEVIEW_FIELD_DESCRIPTIONS,
  web_session_changes: WEB_SESSION_FIELD_DESCRIPTIONS
}

/** The labelled fields of a schema, for the tests that keep this file in step with the schemas. */
export const labelledFields = (schemaName: string): string[] =>
  Object.keys(FIELD_LABELS_BY_SCHEMA[schemaName] ?? {})

/**
 * The name of a field: the workspace's own label first (custom fields can be renamed), then the
 * catalogue label, then a custom column's default name, then the raw key.
 */
export function fieldLabel(
  schemaName: string,
  fieldName: string,
  i18n: I18n,
  customFieldLabels?: Record<string, string>
): string {
  const custom = customFieldLabels?.[fieldName]
  if (custom) return custom
  const descriptor = FIELD_LABELS_BY_SCHEMA[schemaName]?.[fieldName]
  if (descriptor) return i18n._(descriptor)
  if (schemaName === 'contacts') return customFieldDefaultLabel(fieldName, i18n) ?? fieldName
  return fieldName
}

/** What a field holds, or undefined for a custom field (its meaning is the workspace's). */
export function fieldDescription(schemaName: string, fieldName: string, i18n: I18n): string | undefined {
  const descriptor = FIELD_DESCRIPTIONS_BY_SCHEMA[schemaName]?.[fieldName]
  return descriptor ? i18n._(descriptor) : undefined
}

/** The groups of the contact field picker, in order. A field in none of them goes last. */
export const CONTACT_FIELD_GROUPS: { label: MessageDescriptor; fields: string[] }[] = [
  {
    label: msg`Identity`,
    fields: ['email', 'external_id', 'first_name', 'last_name', 'phone', 'job_title']
  },
  {
    label: msg`Location`,
    fields: ['country', 'language', 'timezone', 'address_line_1', 'address_line_2', 'postcode', 'state']
  },
  {
    label: msg`Email tracking`,
    fields: [
      'tracking_opens_consent',
      'tracking_clicks_consent',
      'tracking_consent_source',
      'tracking_consent_updated_at'
    ]
  },
  {
    label: msg`Custom fields`,
    fields: [
      ...[1, 2, 3, 4, 5].map((n) => `custom_string_${n}`),
      ...[1, 2, 3, 4, 5].map((n) => `custom_number_${n}`),
      ...[1, 2, 3, 4, 5].map((n) => `custom_datetime_${n}`),
      ...[1, 2, 3, 4, 5].map((n) => `custom_json_${n}`)
    ]
  },
  { label: msg`Contact dates`, fields: ['created_at', 'updated_at'] }
]

// Filter operators, as the editor offers them and the condition sentence reads them.
export const OPERATOR_LABELS: Record<string, MessageDescriptor> = {
  is_set: msg`is set`,
  is_not_set: msg`is not set`,
  equals: msg`equals`,
  not_equals: msg`doesn't equal`,
  contains: msg`contains`,
  not_contains: msg`doesn't contain`,
  starts_with: msg`starts with`,
  ends_with: msg`ends with`,
  gt: msg`greater than`,
  lt: msg`less than`,
  gte: msg`greater than or equal to`,
  lte: msg`less than or equal to`,
  before_date: msg`before`,
  after_date: msg`after`,
  in_date_range: msg`between`,
  not_in_date_range: msg`not between`,
  in_the_last_days: msg`in the last`,
  not_in_the_last_days: msg`not in the last`,
  in_array: msg`is in the array`
}

export const operatorLabel = (operator: string, i18n: I18n): string => {
  const descriptor = OPERATOR_LABELS[operator]
  return descriptor ? i18n._(descriptor) : operator
}

// Closed values of the tracking consent columns, keyed by stored value.
export const TRACKING_CONSENT_LABELS: Record<string, MessageDescriptor> = {
  granted: msg`Granted`,
  denied: msg`Denied`
}

export const TRACKING_CONSENT_SOURCE_LABELS: Record<string, MessageDescriptor> = {
  api: msg`API`,
  console: msg`Console`,
  import: msg`Import`,
  notification_center: msg`Notification center`,
  email_link: msg`Email link`,
  subscribe_form: msg`Subscription form`
}

/** The translated label of a closed option of a field, or undefined for an open value. */
export function optionLabel(fieldName: string, value: string, i18n: I18n): string | undefined {
  const table =
    fieldName === 'tracking_opens_consent' || fieldName === 'tracking_clicks_consent'
      ? TRACKING_CONSENT_LABELS
      : fieldName === 'tracking_consent_source'
        ? TRACKING_CONSENT_SOURCE_LABELS
        : undefined
  const descriptor = table?.[value]
  return descriptor ? i18n._(descriptor) : undefined
}

// Goal types, worded as the summary and the editor both read them.
export const GOAL_LABELS: Record<string, MessageDescriptor> = {
  '*': msg`Any goal`,
  purchase: msg`Purchase`,
  subscription: msg`Subscription`,
  lead: msg`Lead`,
  signup: msg`Sign-up`,
  booking: msg`Booking`,
  trial: msg`Trial`,
  other: msg`Other goal`
}

export const AGGREGATE_LABELS: Record<string, MessageDescriptor> = {
  count: msg`count`,
  sum: msg`total`,
  avg: msg`average`,
  min: msg`minimum`,
  max: msg`maximum`
}

export const GOAL_COMPARISON_LABELS: Record<string, MessageDescriptor> = {
  gte: msg`greater than or equal to`,
  lte: msg`less than or equal to`,
  eq: msg`equal to`,
  between: msg`between`
}

// The short sign a comparison reads as inside a sentence (« total ≥ 500 »).
export const GOAL_COMPARISON_SIGNS: Record<string, string> = {
  gte: '≥',
  lte: '≤',
  eq: '=',
  between: ''
}

export const COUNT_OPERATOR_LABELS: Record<string, MessageDescriptor> = {
  at_least: msg`at least`,
  at_most: msg`at most`,
  exactly: msg`exactly`
}

export const TIMEFRAME_LABELS: Record<string, MessageDescriptor> = {
  anytime: msg`anytime`,
  in_date_range: msg`in date range`,
  before_date: msg`before date`,
  after_date: msg`after date`,
  in_the_last_days: msg`in the last`
}

// The colour presets of a segment, keyed as segmentColor.ts names them.
export const SEGMENT_COLOR_LABELS: Record<string, MessageDescriptor> = {
  magenta: msg`Magenta`,
  red: msg`Red`,
  volcano: msg`Volcano`,
  orange: msg`Orange`,
  gold: msg`Gold`,
  lime: msg`Lime`,
  green: msg`Green`,
  cyan: msg`Cyan`,
  blue: msg`Blue`,
  geekblue: msg`Indigo`,
  purple: msg`Purple`,
  grey: msg`Grey`
}

/** A message from one of the tables above, or the raw value when the table does not know it. */
export const labelOf = (
  table: Record<string, MessageDescriptor>,
  value: string | undefined,
  i18n: I18n
): string => {
  if (value === undefined) return ''
  const descriptor = table[value]
  return descriptor ? i18n._(descriptor) : value
}

/** An instant in the console's language; anything else as it is. */
export function formatInstant(value: string | undefined, i18n: I18n): string {
  if (!value) return ''
  const date = new Date(value)
  if (!/^\d{4}-\d{2}-\d{2}/.test(value) || Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}
