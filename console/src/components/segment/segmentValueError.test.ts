import { describe, it, expect } from 'vitest'
import { i18n } from '@lingui/core'
import { ApiError } from '../../services/api/errors'
import { readSegmentValueError, segmentLastErrorText, segmentValueErrorText } from './segmentValueError'

describe('readSegmentValueError', () => {
  it('reads the value error a 422 preview answer carries', () => {
    const details = {
      column: 'custom_json_1',
      path: ['renewed_at'],
      type: 'date' as const,
      email: 'a@b.c',
      value: '2026-02-30'
    }
    expect(readSegmentValueError(new ApiError('x', 422, { error: 'x', value_error: details }))).toEqual(details)
  })

  it('ignores any other failure', () => {
    expect(readSegmentValueError(new ApiError('x', 500, { error: 'x' }))).toBeUndefined()
    expect(readSegmentValueError(new ApiError('x', 422, { error: 'x' }))).toBeUndefined()
    expect(readSegmentValueError(new Error('x'))).toBeUndefined()
  })
})

describe('segmentValueErrorText', () => {
  it('names the contact, the value and the field, with the path inside it', () => {
    expect(
      segmentValueErrorText(
        {
          column: 'custom_json_1',
          path: ['billing', 'renewed_at'],
          type: 'date',
          email: 'jane@example.com',
          value: '2026-02-30'
        },
        i18n
      )
    ).toBe('jane@example.com has "2026-02-30" in Custom JSON 1 › billing › renewed_at, which is not a date.')
    expect(
      segmentValueErrorText(
        { column: 'custom_json_2', path: ['score'], type: 'number', email: 'a@b.c', value: 'abc' },
        i18n,
        {
          custom_json_2: 'Scores'
        }
      )
    ).toBe('a@b.c has "abc" in Scores › score, which is not a number.')
  })

  it('says less when the contact is not known or not shown', () => {
    expect(segmentValueErrorText({ column: 'custom_json_1', path: ['renewed_at'], type: 'date' }, i18n)).toBe(
      "A contact's value in Custom JSON 1 › renewed_at is not a date."
    )
    expect(segmentValueErrorText({ column: 'custom_json_1', path: [], type: 'number', email: '' }, i18n)).toBe(
      "A contact's value in Custom JSON 1 is not a number."
    )
    expect(segmentValueErrorText({ column: 'custom_json_1', path: ['n'], type: 'number', email: 'a@b.c' }, i18n)).toBe(
      'The value of a@b.c in Custom JSON 1 › n is not a number.'
    )
    expect(segmentValueErrorText({ email: 'a@b.c', value: 'x' }, i18n)).toBe(
      "A contact's value can't be read by this segment's conditions."
    )
  })
})

describe('segmentLastErrorText', () => {
  it('words a value error as the preview does, and any other failure plainly', () => {
    expect(
      segmentLastErrorText(
        { kind: 'value_conversion', source: 'build', column: 'custom_json_1', path: ['d'], type: 'date' },
        i18n
      )
    ).toBe("A contact's value in Custom JSON 1 › d is not a date.")
    expect(segmentLastErrorText({ kind: 'other', source: 'build', cause: 'boom' }, i18n)).toBe(
      'The last computation failed.'
    )
  })
})
