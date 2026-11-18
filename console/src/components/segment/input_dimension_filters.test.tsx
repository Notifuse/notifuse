import { useState } from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { InputDimensionFilters } from './input_dimension_filters'
import { ContactsTableSchema } from './table_schemas'
import type { DimensionFilter } from '../../services/api/segment'

vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })

describe('InputDimensionFilters', () => {
  it('hands the keyboard to the field of a row it adds', async () => {
    const Wrapper = () => {
      const [value, setValue] = useState<DimensionFilter[]>([
        { field_name: 'email', field_type: 'string', operator: 'is_set' }
      ])
      return <InputDimensionFilters schema={ContactsTableSchema} value={value} onChange={setValue} />
    }
    render(
      <I18nProvider i18n={i18n}>
        <Wrapper />
      </I18nProvider>
    )
    fireEvent.click(screen.getByText('Add a filter'))
    await waitFor(() => {
      const rows = document.querySelectorAll('[data-filter-row]')
      expect(rows).toHaveLength(2)
      expect(rows[1].contains(document.activeElement)).toBe(true)
    })
  })
})
