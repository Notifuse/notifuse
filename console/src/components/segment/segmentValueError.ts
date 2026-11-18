import type { I18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import { ApiError } from '../../services/api/errors'
import type { SegmentLastError, SegmentValueErrorDetails } from '../../services/api/segment'
import { fieldLabel } from './segmentLabels'

/** The value error a segments.preview answer carries (422), or undefined for any other failure. */
export function readSegmentValueError(error: unknown): SegmentValueErrorDetails | undefined {
  if (!(error instanceof ApiError) || error.status !== 422) return undefined
  const details = (error.data as { value_error?: SegmentValueErrorDetails } | null | undefined)?.value_error
  return details && typeof details === 'object' ? details : undefined
}

/**
 * What a value error says, translated: which contact holds which value in which field, as much of
 * it as is known. The field is the custom field's label followed by the path inside it.
 */
export function segmentValueErrorText(
  details: SegmentValueErrorDetails,
  i18n: I18n,
  customFieldLabels?: Record<string, string>
): string {
  const field = details.column
    ? [fieldLabel('contacts', details.column, i18n, customFieldLabels), ...(details.path ?? [])].join(' › ')
    : ''
  const value = details.value ?? ''
  const email = details.email ?? ''

  // The queue names the contact that raised even when it could not read the value back
  if (field && details.type === 'date') {
    if (email && value) return i18n._(msg`${email} has "${value}" in ${field}, which is not a date.`)
    if (email) return i18n._(msg`The value of ${email} in ${field} is not a date.`)
    return i18n._(msg`A contact's value in ${field} is not a date.`)
  }
  if (field && details.type === 'number') {
    if (email && value) return i18n._(msg`${email} has "${value}" in ${field}, which is not a number.`)
    if (email) return i18n._(msg`The value of ${email} in ${field} is not a number.`)
    return i18n._(msg`A contact's value in ${field} is not a number.`)
  }
  return i18n._(msg`A contact's value can't be read by this segment's conditions.`)
}

/** Why a segment's last computation failed, as the contacts view shows it. */
export function segmentLastErrorText(
  lastError: SegmentLastError,
  i18n: I18n,
  customFieldLabels?: Record<string, string>
): string {
  if (lastError.kind === 'value_conversion') return segmentValueErrorText(lastError, i18n, customFieldLabels)
  return i18n._(msg`The last computation failed.`)
}
