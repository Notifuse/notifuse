import { describe, it, expect, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import {
  assignColumn,
  autoMapColumns,
  mappingsForHeaders,
  rowToContact,
  useContactImportFields,
  type ContactImportField
} from './contactFields'

vi.mock('../../../contexts/AuthContext', () => ({
  useAuth: () => ({
    workspaces: [{ id: 'ws1', settings: { custom_field_labels: { custom_string_1: 'Company' } } }]
  })
}))

const fields: ContactImportField[] = [
  { key: 'email', label: 'Email', group: 'required' },
  { key: 'first_name', label: 'First Name', group: 'basic' },
  { key: 'custom_string_1', label: 'Company', group: 'custom' }
]

describe('useContactImportFields', () => {
  it('puts email first, then the standard fields, then the custom ones under their workspace label', () => {
    const { result } = renderHook(() => useContactImportFields('ws1'))
    const list = result.current
    expect(list[0]).toEqual({ key: 'email', label: 'Email', group: 'required' })
    expect(list.filter((f) => f.group === 'basic')).toHaveLength(14)
    expect(list.filter((f) => f.group === 'custom')).toHaveLength(20)
    expect(list.find((f) => f.key === 'custom_string_1')?.label).toBe('Company')
  })
})

describe('autoMapColumns', () => {
  it('maps a column named like a field key or label, whatever its case', () => {
    expect(autoMapColumns(['EMAIL', 'first name', 'company', 'Plan'], fields)).toEqual({
      email: 'EMAIL',
      first_name: 'first name',
      custom_string_1: 'company'
    })
  })
})

describe('mappingsForHeaders', () => {
  it('drops a saved mapping whose column this file does not have', () => {
    expect(mappingsForHeaders({ email: 'Email', first_name: 'Prénom' }, ['Email', 'Name'])).toEqual({ email: 'Email' })
  })
})

describe('assignColumn', () => {
  it('moves a column to a new field and frees its old one', () => {
    expect(assignColumn({ email: 'A', first_name: 'B' }, 'B', 'last_name')).toEqual({ email: 'A', last_name: 'B' })
  })

  it('stops importing a column when no field is given', () => {
    expect(assignColumn({ email: 'A', first_name: 'B' }, 'B', undefined)).toEqual({ email: 'A' })
  })
})

describe('rowToContact', () => {
  const headers = ['email', 'opens', 'json', 'n', 'blank']
  const mappings = {
    email: 'email',
    tracking_opens_consent: 'opens',
    custom_json_1: 'json',
    custom_number_1: 'n',
    first_name: 'blank'
  }

  it('converts consent, JSON and numbers, and sends an empty cell as null', () => {
    expect(rowToContact(['a@b.co', ' Granted ', '{"x":1}', '49.5', ''], headers, mappings)).toEqual({
      email: 'a@b.co',
      tracking_opens_consent: 'granted',
      custom_json_1: { x: 1 },
      custom_number_1: 49.5,
      first_name: null
    })
  })

  it('sends invalid JSON and a non-number as null', () => {
    const contact = rowToContact(['a@b.co', '', '{oops', 'abc', 'x'], headers, mappings)
    expect(contact.custom_json_1).toBeNull()
    expect(contact.custom_number_1).toBeNull()
    expect(contact.tracking_opens_consent).toBeNull()
  })

  it('leaves out a field whose cell the row does not have', () => {
    expect(rowToContact(['a@b.co'], headers, { email: 'email', first_name: 'blank' })).toEqual({ email: 'a@b.co' })
  })
})
