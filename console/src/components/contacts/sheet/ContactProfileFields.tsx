import { useState } from 'react'
import { Button } from 'antd'
import { Link } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import dayjs from '../../../lib/dayjs'
import type { Contact } from '../../../services/api/contacts'
import type { Workspace } from '../../../services/api/types'
import { computeCustomFieldLabel } from '../../../hooks/useCustomFieldLabel'
import { KeyValueList, type KeyValueItem } from '../../layout'
import { SHEET_LABEL_WIDTH, SheetSection } from './SheetSection'
import { useStandardFieldLabels } from './contactFieldLabels'
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
  // Keyed by field: when a field moves from the empty group to the filled one, the rows after it
  // shift, and an editor left open on one of them must keep its own field.
  const toItem = (field: FieldRow): KeyValueItem => ({
    key: field.key,
    label: field.label,
    value: (
      <InlineEditableField
        hideLabel
        // Under the value it describes, and gone while the field is edited.
        hint={field.hint}
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
      {filled.length > 0 ? <KeyValueList labelWidth={SHEET_LABEL_WIDTH} items={filled.map(toItem)} /> : null}
      {count > 0 ? (
        showEmpty ? (
          <KeyValueList labelWidth={SHEET_LABEL_WIDTH} items={empty.map(toItem)} />
        ) : (
          <Button type="link" size="small" className="self-start" onClick={() => setShowEmpty(true)}>
            {t`${plural(count, { one: '# empty field', other: '# empty fields' })}`}
          </Button>
        )
      ) : null}
    </div>
  )
}

/** The Profile and Custom fields sections of the contact sheet. */
export function ContactProfileFields({ workspace, contact, canWrite, onSave }: ContactProfileFieldsProps) {
  const { t } = useLingui()
  const labels = useStandardFieldLabels()

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
      <SheetSection title={t`Profile`}>
        <FieldSection
          workspace={workspace}
          filled={standard.filter((field) => !isEmpty(field.value))}
          empty={standard.filter((field) => isEmpty(field.value))}
          canWrite={canWrite}
          onSave={onSave}
        />
      </SheetSection>
      <SheetSection
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
      </SheetSection>
    </>
  )
}
