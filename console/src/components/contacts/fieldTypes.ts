export type FieldType = 'string' | 'number' | 'datetime' | 'json' | 'timezone' | 'language' | 'country'

// Determine field type from field key
export const getFieldType = (fieldKey: string): FieldType => {
  if (fieldKey.startsWith('custom_number_')) return 'number'
  if (fieldKey.startsWith('custom_datetime_')) return 'datetime'
  if (fieldKey.startsWith('custom_json_')) return 'json'
  if (fieldKey === 'timezone') return 'timezone'
  if (fieldKey === 'language') return 'language'
  if (fieldKey === 'country') return 'country'
  return 'string'
}
