import { describe, it, expect } from 'vitest'
import { i18n } from '@lingui/core'
import {
  ContactsTableSchema,
  ContactListsTableSchema,
  CustomEventsGoalsTableSchema,
  WebPageviewChangesSchema,
  WebSessionChangesSchema
} from './table_schemas'
import {
  AGGREGATE_LABELS,
  fieldDescription,
  fieldLabel,
  GOAL_COMPARISON_LABELS,
  GOAL_LABELS,
  labelledFields,
  LIST_STATUS_LABEL,
  OPERATOR_LABELS,
  SEGMENT_COLOR_LABELS,
  optionLabel,
  TRACKING_CONSENT_LABELS,
  TRACKING_CONSENT_SOURCE_LABELS
} from './segmentLabels'
import { SEGMENT_COLORS } from './segmentColor'
import { KNOWN_OPERATORS, OPERATORS_BY_FIELD_TYPE, OPERATORS_BY_JSON_VALUE_TYPE } from './filterOperators'
import {
  TRACKING_CONSENT_FIELD_OPTIONS,
  TRACKING_CONSENT_SOURCE_FIELD_OPTIONS
} from '../../lib/emailTracking'

const CUSTOM = /^custom_/

describe('segment labels', () => {
  it('names every standard contact field from the catalogue', () => {
    const labelled = labelledFields('contacts')
    for (const fieldName of Object.keys(ContactsTableSchema.fields).filter((name) => !CUSTOM.test(name))) {
      expect(labelled, fieldName).toContain(fieldName)
      expect(fieldDescription('contacts', fieldName, i18n), fieldName).toBeTruthy()
    }
  })

  it('names the custom fields by type and number until the workspace renames them', () => {
    expect(fieldLabel('contacts', 'custom_string_1', i18n)).toBe('Custom text 1')
    expect(fieldLabel('contacts', 'custom_number_2', i18n)).toBe('Custom number 2')
    expect(fieldLabel('contacts', 'custom_datetime_3', i18n)).toBe('Custom date 3')
    expect(fieldLabel('contacts', 'custom_json_4', i18n)).toBe('Custom JSON 4')
    expect(fieldLabel('contacts', 'custom_string_1', i18n, { custom_string_1: 'Plan' })).toBe('Plan')
  })

  it('puts the workspace label before the catalogue one', () => {
    expect(fieldLabel('contacts', 'country', i18n, { country: 'Land' })).toBe('Land')
    expect(fieldLabel('contacts', 'country', i18n)).toBe('Country')
  })

  it.each([
    ['web_pageview_changes', WebPageviewChangesSchema],
    ['web_session_changes', WebSessionChangesSchema]
  ] as const)('names and describes every key of %s', (schemaName, schema) => {
    for (const fieldName of Object.keys(schema.fields)) {
      expect(labelledFields(schemaName), fieldName).toContain(fieldName)
      expect(fieldDescription(schemaName, fieldName, i18n), fieldName).toBeTruthy()
    }
  })

  it('names every operator a filter row may show', () => {
    const offered = [
      ...Object.values(OPERATORS_BY_FIELD_TYPE).flat(),
      ...Object.values(OPERATORS_BY_JSON_VALUE_TYPE).flat(),
      ...KNOWN_OPERATORS
    ]
    for (const operator of offered) expect(OPERATOR_LABELS[operator], operator).toBeDefined()
  })

  it('names every closed option of the schemas', () => {
    for (const option of ContactListsTableSchema.fields.status.options ?? [])
      expect(LIST_STATUS_LABEL[String(option.value)], String(option.value)).toBeDefined()
    for (const option of CustomEventsGoalsTableSchema.fields.goal_type.options ?? [])
      expect(GOAL_LABELS[String(option.value)], String(option.value)).toBeDefined()
    for (const option of CustomEventsGoalsTableSchema.fields.aggregate_operator.options ?? [])
      expect(AGGREGATE_LABELS[String(option.value)], String(option.value)).toBeDefined()
    for (const option of CustomEventsGoalsTableSchema.fields.operator.options ?? [])
      expect(GOAL_COMPARISON_LABELS[String(option.value)], String(option.value)).toBeDefined()
    for (const option of TRACKING_CONSENT_FIELD_OPTIONS)
      expect(TRACKING_CONSENT_LABELS[option.value], option.value).toBeDefined()
    for (const option of TRACKING_CONSENT_SOURCE_FIELD_OPTIONS)
      expect(TRACKING_CONSENT_SOURCE_LABELS[option.value], option.value).toBeDefined()
  })

  it('translates the closed values of the consent fields only', () => {
    expect(optionLabel('tracking_opens_consent', 'granted', i18n)).toBe('Granted')
    expect(optionLabel('tracking_consent_source', 'email_link', i18n)).toBe('Email link')
    expect(optionLabel('country', 'FR', i18n)).toBeUndefined()
  })
})

describe('segment colour labels', () => {
  it('names every preset', () => {
    for (const color of SEGMENT_COLORS) expect(SEGMENT_COLOR_LABELS[color], color).toBeDefined()
  })
})
