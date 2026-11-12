import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App } from 'antd'
import type { Workspace } from '../../services/api/types'
import { InlineEditableField } from './InlineEditableField'

const workspace = { id: 'ws', settings: { timezone: 'UTC' } } as unknown as Workspace

function renderField(props: Partial<Parameters<typeof InlineEditableField>[0]> = {}) {
  const onSave = vi.fn(async () => {})
  render(
    <App>
      <InlineEditableField
        fieldKey="job_title"
        fieldType="string"
        label="Job Title"
        value="CTO"
        workspace={workspace}
        onSave={onSave}
        {...props}
      />
    </App>
  )
  return { onSave }
}

describe('InlineEditableField', () => {
  it('shows its label by default', () => {
    renderField()
    expect(screen.getByText('Job Title')).toBeInTheDocument()
    expect(screen.getByText('CTO')).toBeInTheDocument()
  })

  it('shows the value alone with hideLabel, in display and in edit mode', () => {
    renderField({ hideLabel: true })
    expect(screen.queryByText('Job Title')).toBeNull()
    fireEvent.click(screen.getByText('CTO'))
    expect(screen.getByRole('textbox')).toHaveValue('CTO')
    expect(screen.queryByText('Job Title')).toBeNull()
  })

  it('saves the edited value with hideLabel', async () => {
    const { onSave } = renderField({ hideLabel: true })
    fireEvent.click(screen.getByText('CTO'))
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'CEO' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(onSave).toHaveBeenCalledWith('job_title', 'CEO'))
  })

  it('does not enter edit mode when disabled', () => {
    renderField({ hideLabel: true, disabled: true })
    fireEvent.click(screen.getByText('CTO'))
    expect(screen.queryByRole('textbox')).toBeNull()
  })
})
