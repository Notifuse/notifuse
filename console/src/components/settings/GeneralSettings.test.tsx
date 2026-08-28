import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { GeneralSettings } from './GeneralSettings'
import type { Workspace } from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'

// The save bar guards navigation away from unsaved edits; the suite renders the
// section outside a router, so the blocker is stubbed idle.
vi.mock('@tanstack/react-router', () => ({
  useBlocker: () => ({ status: 'idle', proceed: undefined, reset: undefined })
}))

vi.mock('../../services/api/workspace', () => ({
  workspaceService: {
    update: vi.fn().mockResolvedValue({ status: 'success' }),
    get: vi.fn().mockResolvedValue({ workspace: { id: 'ws1', settings: {} } }),
    detectFavicon: vi.fn()
  }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const makeWorkspace = (settingsOverrides: Record<string, unknown> = {}): Workspace =>
  ({
    id: 'ws1',
    name: 'My WS',
    settings: {
      website_url: 'https://example.com',
      timezone: 'UTC',
      email_tracking_enabled: false,
      languages: ['en'],
      default_language: 'en',
      ...settingsOverrides
    }
  }) as unknown as Workspace

const renderComponent = (isOwner: boolean, workspace: Workspace | null = makeWorkspace()) =>
  render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <GeneralSettings workspace={workspace} onWorkspaceUpdate={vi.fn()} isOwner={isOwner} />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )

describe('GeneralSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders a read-only view (no editor) when the user is not an owner', () => {
    renderComponent(false)
    expect(screen.queryByRole('button', { name: /Save Changes/i })).toBeNull()
    expect(screen.getByText('My WS')).toBeInTheDocument()
  })

  it('holds the save bar back until the form is touched', () => {
    renderComponent(true)
    expect(screen.getByLabelText(/Workspace Name/i)).toHaveValue('My WS')
    expect(screen.queryByRole('button', { name: /Save Changes/i })).toBeNull()
    expect(screen.queryByText('You have unsaved changes')).toBeNull()
  })

  it('raises the save bar as soon as a field changes', () => {
    renderComponent(true)

    fireEvent.change(screen.getByLabelText(/Workspace Name/i), { target: { value: 'Renamed WS' } })

    expect(screen.getByText('You have unsaved changes')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Save Changes/i })).toBeEnabled()
  })

  it('restores the stored values on Discard instead of emptying the form', () => {
    // resetFields() would blank the form: the values are pushed in through
    // setFieldsValue, so there are no initialValues to fall back on.
    renderComponent(true)

    const nameInput = screen.getByLabelText(/Workspace Name/i)
    fireEvent.change(nameInput, { target: { value: 'Renamed WS' } })
    fireEvent.click(screen.getByRole('button', { name: /^Discard$/i }))

    expect(nameInput).toHaveValue('My WS')
    expect(screen.queryByText('You have unsaved changes')).toBeNull()
  })

  it('saves the edited settings from the floating bar', async () => {
    renderComponent(true)

    fireEvent.change(screen.getByLabelText(/Workspace Name/i), { target: { value: 'Renamed WS' } })
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() =>
      expect(workspaceService.update).toHaveBeenCalledWith(
        expect.objectContaining({
          id: 'ws1',
          name: 'Renamed WS',
          // The panel rebuilds the whole settings object, so an untouched
          // field must survive the save.
          settings: expect.objectContaining({ website_url: 'https://example.com' })
        })
      )
    )
  })

  it('saves on Cmd/Ctrl+S once there are changes to save', async () => {
    renderComponent(true)

    fireEvent.keyDown(window, { key: 's', metaKey: true })
    expect(workspaceService.update).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText(/Workspace Name/i), { target: { value: 'Renamed WS' } })
    fireEvent.keyDown(window, { key: 's', metaKey: true })

    await waitFor(() =>
      expect(workspaceService.update).toHaveBeenCalledWith(
        expect.objectContaining({ name: 'Renamed WS' })
      )
    )
  })

  // The settings the update actually puts on the wire. A value of undefined survives in the
  // mock argument but not in the request body, and workspaces.update restores every setting
  // its body leaves out - which is how an emptied field came back filled.
  const sentSettings = (): Record<string, unknown> =>
    JSON.parse(JSON.stringify(vi.mocked(workspaceService.update).mock.calls[0][0].settings))

  it('clears the custom endpoint URL when the field is emptied', async () => {
    renderComponent(true, makeWorkspace({ custom_endpoint_url: 'https://old.example.com' }))

    const endpoint = screen.getByLabelText(/Custom Endpoint URL/i)
    expect(endpoint).toHaveValue('https://old.example.com')
    fireEvent.change(endpoint, { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() => expect(workspaceService.update).toHaveBeenCalled())
    expect(sentSettings().custom_endpoint_url).toBe('')
  })

  it('keeps a custom endpoint URL the operator did not touch', async () => {
    renderComponent(true, makeWorkspace({ custom_endpoint_url: 'https://track.example.com' }))

    fireEvent.change(screen.getByLabelText(/Workspace Name/i), { target: { value: 'Renamed WS' } })
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() => expect(workspaceService.update).toHaveBeenCalled())
    expect(sentSettings().custom_endpoint_url).toBe('https://track.example.com')
  })

  it('refuses to save an empty workspace name', async () => {
    renderComponent(true)

    fireEvent.change(screen.getByLabelText(/Workspace Name/i), { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() =>
      expect(screen.getByText('Please enter workspace name')).toBeInTheDocument()
    )
    expect(workspaceService.update).not.toHaveBeenCalled()
  })
})
