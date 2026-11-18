import { useLingui } from '@lingui/react/macro'
import type { Contact } from '../../../services/api/contacts'
import { useAuth } from '../../../contexts/AuthContext'
import { getCustomFieldLabel } from '../../../hooks/useCustomFieldLabel'

export type ContactFieldGroup = 'required' | 'basic' | 'custom'

export interface ContactImportField {
  key: string
  label: string
  group: ContactFieldGroup
}

/** A contact field key → the CSV column imported into it. */
export type ColumnMappings = Record<string, string>

const CUSTOM_KINDS = ['string', 'number', 'datetime', 'json'] as const

/** The contact fields a CSV column can be imported into, custom ones under the workspace's labels. */
export function useContactImportFields(workspaceId: string): ContactImportField[] {
  const { t } = useLingui()
  const { workspaces } = useAuth()
  const workspace = workspaces.find((w) => w.id === workspaceId)

  const basic: Array<[string, string]> = [
    ['external_id', t`External ID`],
    ['first_name', t`First Name`],
    ['last_name', t`Last Name`],
    ['phone', t`Phone`],
    ['country', t`Country`],
    ['timezone', t`Timezone`],
    ['language', t`Language`],
    ['address_line_1', t`Address Line 1`],
    ['address_line_2', t`Address Line 2`],
    ['postcode', t`Postcode`],
    ['state', t`State`],
    ['job_title', t`Job Title`],
    ['tracking_opens_consent', t`Opens tracking consent (granted or denied)`],
    ['tracking_clicks_consent', t`Clicks tracking consent (granted or denied)`]
  ]
  const custom = CUSTOM_KINDS.flatMap((kind) =>
    [1, 2, 3, 4, 5].map((n) => {
      const key = `custom_${kind}_${n}`
      return { key, label: getCustomFieldLabel(key, workspace), group: 'custom' as const }
    })
  )
  return [
    { key: 'email', label: t`Email`, group: 'required' },
    ...basic.map(([key, label]) => ({ key, label, group: 'basic' as const })),
    ...custom
  ]
}

/** Columns named like a field (its key or its label, any case) are mapped to it; a later column wins. */
export function autoMapColumns(headers: string[], fields: ContactImportField[]): ColumnMappings {
  const mappings: ColumnMappings = {}
  for (const header of headers) {
    const name = header.trim().toLowerCase()
    const field = fields.find((f) => f.key.toLowerCase() === name || f.label.toLowerCase() === name)
    if (field) mappings[field.key] = header
  }
  return mappings
}

/** The mappings whose column is in this file: a saved mapping may name a column that is gone. */
export function mappingsForHeaders(mappings: ColumnMappings, headers: string[]): ColumnMappings {
  const kept: ColumnMappings = {}
  for (const [field, column] of Object.entries(mappings)) {
    if (typeof column === 'string' && headers.includes(column)) kept[field] = column
  }
  return kept
}

/** Maps `column` to `field`, or to nothing when `field` is undefined; a column feeds one field. */
export function assignColumn(mappings: ColumnMappings, column: string, field: string | undefined): ColumnMappings {
  const next: ColumnMappings = {}
  for (const [key, value] of Object.entries(mappings)) {
    if (value !== column) next[key] = value
  }
  if (field) next[field] = column
  return next
}

/** One CSV row as a contact: values converted for typed fields, empty cells sent as null. */
export function rowToContact(row: unknown[], headers: string[], mappings: ColumnMappings): Partial<Contact> {
  const contact: Record<string, unknown> = {}
  for (const [field, column] of Object.entries(mappings)) {
    const index = headers.indexOf(column)
    if (index === -1 || row[index] === undefined) continue
    let value: unknown = row[index]
    if (field.startsWith('custom_json_') && value) {
      try {
        value = JSON.parse(String(value))
      } catch {
        value = null
      }
    } else if ((field === 'tracking_opens_consent' || field === 'tracking_clicks_consent') && typeof value === 'string') {
      // "Granted" and " denied " are what spreadsheets hold; an empty cell stays empty and keeps
      // the stored consent.
      value = value.trim().toLowerCase()
    } else if (field.startsWith('custom_number_')) {
      if (typeof value === 'string' && value.trim() !== '') {
        const number = Number(value)
        value = Number.isNaN(number) ? null : number
      } else {
        value = null
      }
    }
    contact[field] = value !== '' ? value : null
  }
  return contact as Partial<Contact>
}
