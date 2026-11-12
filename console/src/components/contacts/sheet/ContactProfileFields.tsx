import { useState } from 'react'
import { Button } from 'antd'
import { Link } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import dayjs from '../../../lib/dayjs'
import type { Contact } from '../../../services/api/contacts'
import type { Workspace } from '../../../services/api/types'
import { computeCustomFieldLabel } from '../../../hooks/useCustomFieldLabel'
import { KeyValueList, Panel, type KeyValueItem } from '../../layout'
import { InlineEditableField } from '../InlineEditableField'
import { getFieldType } from '../fieldTypes'

type FieldValue = string | number | object | null | undefined
type SaveField = (fieldKey: string, value: string | number | object | null) => Promise<void>

const STANDARD_FIELDS = [
  'first_name',
  'last_name',
  'full_name',
  'phone',
  'address_line_1',
  'address_line_2',
  'country',
  'state',
  'postcode',
  'job_title',
  'timezone',
  'language',
  'external_id'
] as const

const CUSTOM_FIELDS = (['string', 'number', 'datetime', 'json'] as const).flatMap((kind) =>
  [1, 2, 3, 4, 5].map((n) => `custom_${kind}_${n}` as keyof Contact)
)

const isEmpty = (value: FieldValue) => value === null || value === undefined || value === ''

export interface ContactProfileFieldsProps {
  workspace: Workspace
  contact: Contact
  canWrite: boolean
  onSave: SaveField
}

interface FieldRow {
  key: string
  label: string
  value: FieldValue
  hint?: string
  technicalName?: string
  showTooltip?: boolean
}

/** One section of fields: the filled ones, then the empty ones behind « N empty fields ». */
function FieldSection({
  workspace,
  filled,
  empty,
  canWrite,
  onSave
}: {
  workspace: Workspace
  filled: FieldRow[]
  empty: FieldRow[]
  canWrite: boolean
  onSave: SaveField
}) {
  const { t } = useLingui()
  const [showEmpty, setShowEmpty] = useState(false)
  // Keyed by field: a row moves from the empty fields to the filled ones on save, and an editor
  // left open must keep its own field.
  const toItem = (field: FieldRow): KeyValueItem => ({
    key: field.key,
    label: field.label,
    hint: field.hint,
    value: (
      <InlineEditableField
        hideLabel
        fieldKey={field.key}
        fieldType={getFieldType(field.key)}
        label={field.label}
        technicalName={field.technicalName}
        showTooltip={field.showTooltip}
        value={field.value}
        workspace={workspace}
        onSave={onSave}
        disabled={!canWrite}
      />
    )
  })
  const count = empty.length
  return (
    <div className="flex flex-col gap-2">
      {filled.length > 0 ? <KeyValueList items={filled.map(toItem)} /> : null}
      {count > 0 ? (
        showEmpty ? (
          <KeyValueList items={empty.map(toItem)} />
        ) : (
          <Button type="link" size="small" className="self-start" onClick={() => setShowEmpty(true)}>
            {t`${plural(count, { one: '# empty field', other: '# empty fields' })}`}
          </Button>
        )
      ) : null}
    </div>
  )
}

/** The Profile and Custom fields cards of the contact sheet. */
export function ContactProfileFields({ workspace, contact, canWrite, onSave }: ContactProfileFieldsProps) {
  const { t } = useLingui()
  const labels: Record<(typeof STANDARD_FIELDS)[number], string> = {
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

  const standard: FieldRow[] = STANDARD_FIELDS.map((key) => {
    const value = contact[key] as FieldValue
    // The contact's local time next to its timezone: « Europe/Paris · 14:32 ».
    const hint =
      key === 'timezone' && typeof value === 'string' && value
        ? t`Local time ${dayjs().tz(value).format('HH:mm')}`
        : undefined
    return { key, label: labels[key], value, hint }
  })

  const labelled = (key: string) => !!workspace.settings?.custom_field_labels?.[key]
  const custom: FieldRow[] = CUSTOM_FIELDS.map((key) => {
    const label = computeCustomFieldLabel(key, workspace)
    return {
      key,
      label: label.displayLabel,
      technicalName: label.technicalName,
      showTooltip: label.showTooltip,
      value: contact[key] as FieldValue
    }
  })
  // Empty custom fields the workspace named come first in the folded group.
  const emptyCustom = custom
    .filter((field) => isEmpty(field.value))
    .sort((a, b) => Number(labelled(b.key)) - Number(labelled(a.key)))

  return (
    <>
      <Panel title={t`Profile`}>
        <FieldSection
          workspace={workspace}
          filled={standard.filter((field) => !isEmpty(field.value))}
          empty={standard.filter((field) => isEmpty(field.value))}
          canWrite={canWrite}
          onSave={onSave}
        />
      </Panel>
      <Panel
        title={t`Custom fields`}
        extra={
          <Link
            to="/console/workspace/$workspaceId/settings/$section"
            params={{ workspaceId: workspace.id, section: 'custom-fields' }}
            className="text-xs"
          >
            {t`Manage`}
          </Link>
        }
      >
        <FieldSection
          workspace={workspace}
          filled={custom.filter((field) => !isEmpty(field.value))}
          empty={emptyCustom}
          canWrite={canWrite}
          onSave={onSave}
        />
      </Panel>
    </>
  )
}
