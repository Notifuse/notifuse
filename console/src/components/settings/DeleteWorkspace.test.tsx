import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { DeleteWorkspaceSection } from './DeleteWorkspace'
import type { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'

vi.mock('../../services/api/workspace', () => ({
  workspaceService: { delete: vi.fn() }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const onDeleteSuccess = vi.fn()
const workspace = { id: 'acme-prod', name: 'Acme', settings: {} } as unknown as Workspace

const renderSection = () =>
  render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <DeleteWorkspaceSection workspace={workspace} onDeleteSuccess={onDeleteSuccess} />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )

describe('DeleteWorkspaceSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('sits in a danger panel', () => {
    renderSection()

    const heading = screen.getByRole('heading', { name: 'Delete this workspace' })
    expect(heading.closest('section')).toHaveAttribute('data-tone', 'danger')
  })

  it('deletes only once the workspace id is typed', async () => {
    vi.mocked(workspaceService.delete).mockResolvedValue({ status: 'success' } as never)
    renderSection()

    const button = screen.getByRole('button', { name: 'Delete Workspace' })
    expect(button).toBeDisabled()
    fireEvent.change(screen.getByLabelText('Workspace ID'), { target: { value: 'acme' } })
    expect(button).toBeDisabled()
    fireEvent.change(screen.getByLabelText('Workspace ID'), { target: { value: 'acme-prod' } })
    fireEvent.click(button)

    await waitFor(() => expect(workspaceService.delete).toHaveBeenCalledWith({ id: 'acme-prod' }))
    await waitFor(() => expect(onDeleteSuccess).toHaveBeenCalled())
  })

  it('keeps the workspace and says so when the deletion fails', async () => {
    vi.mocked(workspaceService.delete).mockRejectedValue(new Error('nope'))
    renderSection()

    fireEvent.change(screen.getByLabelText('Workspace ID'), { target: { value: 'acme-prod' } })
    fireEvent.click(screen.getByRole('button', { name: 'Delete Workspace' }))

    expect(await screen.findByText('Failed to delete workspace')).toBeInTheDocument()
    expect(onDeleteSuccess).not.toHaveBeenCalled()
  })
})
