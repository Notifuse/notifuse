import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { FieldTypeTime } from './type_time'
import { OperatorNotInTheLastDays, OperatorInTheLastDays } from './operator_time'
import type { DimensionFilter, FieldTypeRenderer } from '../../services/api/segment'

const renderOperator = (element: JSX.Element) =>
  render(<I18nProvider i18n={i18n}>{element}</I18nProvider>)

describe('not_in_the_last_days operator', () => {
  it('is offered for time fields', () => {
    const operators = new FieldTypeTime().operators.map((op) => op.type)
    expect(operators).toContain('not_in_the_last_days')
    // The positive form must stay available alongside it
    expect(operators).toContain('in_the_last_days')
  })

  it('renders the day count and signals that never-set dates match', () => {
    const filter: DimensionFilter = {
      field_name: 'custom_datetime_1',
      field_type: 'time',
      operator: 'not_in_the_last_days',
      string_values: ['30']
    }

    renderOperator(new OperatorNotInTheLastDays().render(filter))

    expect(screen.getByText('30')).toBeTruthy()
    expect(screen.getByText('not in the last')).toBeTruthy()
    expect(screen.getByText('days (or never)')).toBeTruthy()
  })

  it('does not claim never-set dates match for the positive operator', () => {
    const filter: DimensionFilter = {
      field_name: 'custom_datetime_1',
      field_type: 'time',
      operator: 'in_the_last_days',
      string_values: ['30']
    }

    renderOperator(new OperatorInTheLastDays().render(filter))

    expect(screen.queryByText('days (or never)')).toBeNull()
  })

  it('is resolved by the time field renderer without falling back to the error alert', () => {
    const filter: DimensionFilter = {
      field_name: 'custom_datetime_1',
      field_type: 'time',
      operator: 'not_in_the_last_days',
      string_values: ['30']
    }

    // Typed as the interface so the call matches how input.tsx and
    // input_dimension_filters.tsx invoke it: (filter, schema).
    const renderer: FieldTypeRenderer = new FieldTypeTime()

    const { container } = renderOperator(
      renderer.render(filter, {
        name: 'custom_datetime_1',
        title: 'Custom Date 1',
        type: 'time'
      })
    )

    expect(container.querySelector('.ant-alert-error')).toBeNull()
  })
})
