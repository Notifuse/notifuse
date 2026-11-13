import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { ALL_CUSTOM_FIELDS, CustomFieldsConfiguration } from './CustomFieldsConfiguration'
import type { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'

// Mock the workspace API service. The component must call setCustomFieldLabels
// (the dedicated, permission-checked endpoint) and NOT update (owner-only).
vi.mock('../../services/api/workspace', () => ({
  workspaceService: {
    setCustomFieldLabels: vi.fn().mockResolvedValue({ status: 'success' }),
    update: vi.fn(),
    get: vi.fn().mockResolvedValue({ workspace: { id: 'ws1', settings: { custom_field_labels: {} } } })
  }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const makeWorkspace = (labels: Record<string, string> = {}): Workspace =>
  ({
    id: 'ws1',
    name: 'WS',
    settings: { custom_field_labels: labels }
  }) as unknown as Workspace

const renderComponent = (canManage: boolean, labels: Record<string, string> = {}) =>
  render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <CustomFieldsConfiguration
            workspace={makeWorkspace(labels)}
            onWorkspaceUpdate={vi.fn()}
            canManage={canManage}
          />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )

const rows = () =>
  Array.from(document.querySelectorAll('tbody tr.ant-table-row')).map((row) => row.textContent ?? '')

const footer = () => document.querySelector('[data-custom-fields-footer]')?.textContent ?? ''

const openMenuOf = async (label: string) => {
  const row = screen.getByText(label).closest('tr') as HTMLElement
  fireEvent.click(within(row).getByRole('button', { name: 'Actions' }))
}

const total = ALL_CUSTOM_FIELDS.length

describe('CustomFieldsConfiguration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('lists the named slots in the order of the slots, not of the stored labels', () => {
    renderComponent(true, { custom_number_1: 'Score', custom_json_2: 'Preferences', custom_string_2: 'Company' })

    expect(rows()).toHaveLength(3)
    expect(rows()[0]).toContain('Company')
    expect(rows()[0]).toContain('custom_string_2')
    expect(rows()[0]).toContain('Text')
    expect(rows()[1]).toContain('Score')
    expect(rows()[1]).toContain('Number')
    expect(rows()[2]).toContain('Preferences')
    expect(rows()[2]).toContain('JSON')
  })

  it('counts the named slots against every slot there is', () => {
    renderComponent(true, { custom_string_1: 'Company', custom_datetime_1: 'Birthday' })

    expect(screen.getByText(`2 of ${total} slots named`)).toBeInTheDocument()
    expect(rows()[1]).toContain('Date')
  })

  it('counts the free slots of each type', () => {
    const labels = { custom_string_1: 'Company', custom_string_2: 'Industry', custom_json_1: 'Preferences' }
    renderComponent(true, labels)

    const freeOf = (prefix: string) =>
      ALL_CUSTOM_FIELDS.filter((field) => field.key.startsWith(prefix) && !(field.key in labels)).length
    expect(footer()).toContain(
      `${freeOf('custom_string')} text, ${freeOf('custom_number')} number, ${freeOf('custom_datetime')} date, ${freeOf('custom_json')} JSON free`
    )
    expect(screen.getByRole('button', { name: /Name a free slot/ })).toBeEnabled()
  })

  it('says every slot is named when none is left', () => {
    const labels = Object.fromEntries(ALL_CUSTOM_FIELDS.map((field) => [field.key, `Label ${field.key}`]))
    renderComponent(true, labels)

    expect(footer()).toBe(`All ${total} slots are named`)
    expect(screen.queryByRole('button', { name: /Name a free slot/ })).toBeNull()
  })

  it('invites to name a first slot when none is named', () => {
    renderComponent(true)

    expect(screen.getByText('No custom field is named yet')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Name a slot' })).toBeEnabled()
  })

  it('disables the menu and the link without the right to manage custom fields', () => {
    renderComponent(false, { custom_string_1: 'Company Name' })

    expect(screen.getByText('Company Name')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Actions' })).toBeDisabled()
    expect(screen.getByRole('button', { name: /Name a free slot/ })).toBeDisabled()
  })

  it('disables naming a first slot without the right', () => {
    renderComponent(false)
    expect(screen.getByRole('button', { name: 'Name a slot' })).toBeDisabled()
  })

  it('saves a new label via setCustomFieldLabels (not workspace.update)', async () => {
    renderComponent(true)

    fireEvent.click(screen.getByRole('button', { name: 'Name a slot' }))

    // Select a field and enter a label in the modal.
    fireEvent.click(screen.getByText('custom_string_1'))
    fireEvent.change(screen.getByPlaceholderText(/e\.g\., Company Name/i), {
      target: { value: 'My Company' }
    })
    fireEvent.click(screen.getByRole('button', { name: /^Save$/i }))

    await waitFor(() => {
      expect(workspaceService.setCustomFieldLabels).toHaveBeenCalledWith({
        workspace_id: 'ws1',
        custom_field_labels: { custom_string_1: 'My Company' }
      })
    })
    expect(workspaceService.update).not.toHaveBeenCalled()
  })

  it('renames a label from its menu', async () => {
    renderComponent(true, { custom_string_1: 'Company', custom_number_1: 'Score' })

    await openMenuOf('Company')
    fireEvent.click(await screen.findByText('Rename'))
    const input = await screen.findByDisplayValue('Company')
    fireEvent.change(input, { target: { value: 'Employer' } })
    fireEvent.click(screen.getByRole('button', { name: /^Save$/i }))

    await waitFor(() =>
      expect(workspaceService.setCustomFieldLabels).toHaveBeenCalledWith({
        workspace_id: 'ws1',
        custom_field_labels: { custom_string_1: 'Employer', custom_number_1: 'Score' }
      })
    )
  })

  it('removes a label after saying the contact values stay', async () => {
    renderComponent(true, { custom_string_1: 'Company', custom_number_1: 'Score' })

    await openMenuOf('Score')
    fireEvent.click(await screen.findByText('Remove label'))
    expect(await screen.findByText('The values stay on your contacts; only the label is removed.')).toBeInTheDocument()
    expect(workspaceService.setCustomFieldLabels).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Yes' }))

    await waitFor(() =>
      expect(workspaceService.setCustomFieldLabels).toHaveBeenCalledWith({
        workspace_id: 'ws1',
        custom_field_labels: { custom_string_1: 'Company' }
      })
    )
  })
})
