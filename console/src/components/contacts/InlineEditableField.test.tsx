import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App, Drawer } from 'antd'
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

  it('shows its hint under the value, and not under the editor', () => {
    renderField({ hideLabel: true, hint: 'Local time 14:32' })
    expect(screen.getByText('Local time 14:32')).toBeInTheDocument()
    fireEvent.click(screen.getByText('CTO'))
    expect(screen.getByRole('textbox')).toBeInTheDocument()
    expect(screen.queryByText('Local time 14:32')).toBeNull()
  })

  it('does not enter edit mode when disabled', () => {
    renderField({ hideLabel: true, disabled: true })
    fireEvent.click(screen.getByText('CTO'))
    expect(screen.queryByRole('textbox')).toBeNull()
  })

  it('cancels the edit on Escape without closing the drawer around it', () => {
    const onClose = vi.fn()
    const onSave = vi.fn(async () => {})
    render(
      <App>
        <Drawer open onClose={onClose}>
          <InlineEditableField
            fieldKey="job_title"
            fieldType="string"
            label="Job Title"
            value="CTO"
            workspace={workspace}
            onSave={onSave}
            hideLabel
          />
        </Drawer>
      </App>
    )
    fireEvent.click(screen.getByText('CTO'))
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: 'CEO' } })
    fireEvent.keyDown(input, { key: 'Escape', code: 'Escape', keyCode: 27 })
    expect(onClose).not.toHaveBeenCalled()
    expect(screen.queryByRole('textbox')).toBeNull()
    expect(screen.getByText('CTO')).toBeInTheDocument()
    expect(onSave).not.toHaveBeenCalled()
    // Out of edit mode, Escape closes the drawer as before.
    fireEvent.keyDown(screen.getByText('CTO'), { key: 'Escape', code: 'Escape', keyCode: 27 })
    expect(onClose).toHaveBeenCalled()
  })
})
