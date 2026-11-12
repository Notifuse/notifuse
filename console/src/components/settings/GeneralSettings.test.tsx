import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
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

  // The API refuses a workspace name over 32 characters: the field says so as it is typed.
  it('limits the workspace name to 32 characters', () => {
    renderComponent(true)
    expect(screen.getByLabelText('Name')).toHaveAttribute('maxlength', '32')
  })

  it('does not save a workspace name over 32 characters', async () => {
    renderComponent(true)

    // A change event sets the value past the input's maxlength, as a paste can.
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'a'.repeat(33) }
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(
      await screen.findByText('Workspace name must be at most 32 characters long')
    ).toBeInTheDocument()
    expect(workspaceService.update).not.toHaveBeenCalled()
  })

  it('renders a read-only view (no editor) when the user is not an owner', () => {
    renderComponent(false)
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()
    expect(screen.getByText('My WS')).toBeInTheDocument()
  })

  it('holds the save bar back until the form is touched', () => {
    renderComponent(true)
    expect(screen.getByLabelText('Name')).toHaveValue('My WS')
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()
    expect(screen.queryByText('Unsaved changes')).toBeNull()
  })

  it('raises the save bar as soon as a field changes', () => {
    renderComponent(true)

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Renamed WS' } })

    expect(screen.getByText('Unsaved changes')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  })

  it('restores the stored values on Discard instead of emptying the form', () => {
    // resetFields() would blank the form: the values are pushed in through
    // setFieldsValue, so there are no initialValues to fall back on.
    renderComponent(true)

    const nameInput = screen.getByLabelText('Name')
    fireEvent.change(nameInput, { target: { value: 'Renamed WS' } })
    fireEvent.click(screen.getByRole('button', { name: /^Discard$/i }))

    expect(nameInput).toHaveValue('My WS')
    expect(screen.queryByText('Unsaved changes')).toBeNull()
  })

  it('saves the edited settings from the floating bar', async () => {
    renderComponent(true)

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Renamed WS' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

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

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Renamed WS' } })
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

    const endpoint = screen.getByLabelText('Domain')
    expect(endpoint).toHaveValue('https://old.example.com')
    fireEvent.change(endpoint, { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(workspaceService.update).toHaveBeenCalled())
    expect(sentSettings().custom_endpoint_url).toBe('')
  })

  it('keeps a custom endpoint URL the operator did not touch', async () => {
    renderComponent(true, makeWorkspace({ custom_endpoint_url: 'https://track.example.com' }))

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Renamed WS' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(workspaceService.update).toHaveBeenCalled())
    expect(sentSettings().custom_endpoint_url).toBe('https://track.example.com')
  })

  it('refuses to save an empty workspace name', async () => {
    renderComponent(true)

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(screen.getByText('Please enter workspace name')).toBeInTheDocument()
    )
    expect(workspaceService.update).not.toHaveBeenCalled()
  })
})

describe('GeneralSettings email tracking', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  const sentSettings = (): Record<string, unknown> =>
    JSON.parse(JSON.stringify(vi.mocked(workspaceService.update).mock.calls[0][0].settings))

  it('saves opens and clicks separately with the unknown-consent choice', async () => {
    renderComponent(
      true,
      makeWorkspace({
        email_tracking_enabled: true,
        email_tracking: { opens_enabled: true, clicks_enabled: true, unknown_consent: 'track' }
      })
    )

    fireEvent.click(screen.getByLabelText('Opens'))
    fireEvent.click(screen.getByLabelText("Don't track"))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(workspaceService.update).toHaveBeenCalled())
    expect(sentSettings().email_tracking).toEqual({
      opens_enabled: false,
      clicks_enabled: true,
      unknown_consent: 'no_track'
    })
    expect(sentSettings().email_tracking_enabled).toBe(true)
  })

  it('reads a workspace saved with the single switch as both on, unknown tracked', async () => {
    renderComponent(true, makeWorkspace({ email_tracking_enabled: true }))

    expect(screen.getByLabelText('Opens')).toBeChecked()
    expect(screen.getByLabelText('Clicks')).toBeChecked()
    expect(screen.getByLabelText(/^Track$/i)).toBeChecked()
  })

  it('turns the single switch off when neither is tracked', async () => {
    renderComponent(true, makeWorkspace({ email_tracking_enabled: true }))

    fireEvent.click(screen.getByLabelText('Opens'))
    fireEvent.click(screen.getByLabelText('Clicks'))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(workspaceService.update).toHaveBeenCalled())
    expect(sentSettings().email_tracking_enabled).toBe(false)
  })
})

describe('GeneralSettings rows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  const sentSettings = (): Record<string, unknown> =>
    JSON.parse(JSON.stringify(vi.mocked(workspaceService.update).mock.calls[0][0].settings))

  const openReplace = async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Replace' }))
    return screen.findByLabelText('Logo URL')
  }

  it('puts the settings in four panels', () => {
    renderComponent(true)

    const titles = Array.from(document.querySelectorAll('[data-panel-head] h3')).map((h) => h.textContent)
    expect(titles).toEqual(['Workspace', 'Language & timezone', 'Email tracking', 'Custom domain'])
  })

  it('keeps the logo when saving without opening Replace', async () => {
    renderComponent(true, makeWorkspace({ logo_url: 'https://example.com/logo.png' }))

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Renamed WS' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(workspaceService.update).toHaveBeenCalled())
    expect(sentSettings().logo_url).toBe('https://example.com/logo.png')
  })

  it('empties the logo with Remove', async () => {
    renderComponent(true, makeWorkspace({ logo_url: 'https://example.com/logo.png' }))

    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    expect(screen.queryByRole('button', { name: 'Remove' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(workspaceService.update).toHaveBeenCalled())
    expect(sentSettings().logo_url).toBeNull()
  })

  it('shows a logo URL typed in Replace and says when it is not one', async () => {
    renderComponent(true)

    const input = await openReplace()
    fireEvent.change(input, { target: { value: 'not a url' } })

    expect(await screen.findByText('Please enter a valid URL')).toBeInTheDocument()
    expect(screen.getByText('Unsaved changes')).toBeInTheDocument()
  })

  it('raises the save bar when the icon of the website is detected', async () => {
    vi.mocked(workspaceService.detectFavicon).mockResolvedValue({ iconUrl: 'https://example.com/favicon.png' })
    renderComponent(true)

    await openReplace()
    fireEvent.click(screen.getByRole('button', { name: /Detect from website/ }))

    await waitFor(() =>
      expect(document.querySelector('[data-logo-preview]')).toHaveAttribute('src', 'https://example.com/favicon.png')
    )
    expect(workspaceService.detectFavicon).toHaveBeenCalledWith('https://example.com')
    expect(screen.getByText('Unsaved changes')).toBeInTheDocument()
  })

  it('falls back to the initial when the logo cannot be loaded', () => {
    renderComponent(true, makeWorkspace({ logo_url: 'https://example.com/broken.png' }))

    fireEvent.error(document.querySelector('[data-logo-preview]') as HTMLElement)

    expect(screen.getByText("This image couldn't be loaded")).toBeInTheDocument()
    expect(document.querySelector('[data-workspace-badge]')).toHaveTextContent('M')
  })

  it('replaces a removed default language with the first one left', async () => {
    renderComponent(true, makeWorkspace({ languages: ['en', 'fr'], default_language: 'fr' }))

    const row = screen.getByText('Supported languages').closest('[data-settings-row]') as HTMLElement
    const removes = row.querySelectorAll('.ant-select-selection-item-remove')
    fireEvent.click(removes[removes.length - 1])
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(workspaceService.update).toHaveBeenCalled())
    expect(sentSettings().languages).toEqual(['en'])
    expect(sentSettings().default_language).toBe('en')
  })

  it('keeps the long consent text in the tooltip of its label', async () => {
    renderComponent(true)

    expect(screen.queryByText(/Italian Garante/)).toBeNull()
    fireEvent.mouseEnter(screen.getByLabelText('About consent'))

    expect(await screen.findByText(/Italian Garante/)).toBeInTheDocument()
  })

  it('writes the DNS record for the domain typed', async () => {
    renderComponent(true)

    const record = document.querySelector('[data-dns-record]') as HTMLElement
    expect(within(record).getByText('blog.yourdomain.com')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Domain'), { target: { value: 'https://links.acme.io' } })
    expect(await within(record).findByText('links.acme.io')).toBeInTheDocument()
    expect(within(record).getByText('CNAME')).toBeInTheDocument()
  })

  it('marks a saved domain as in use', () => {
    renderComponent(true, makeWorkspace({ custom_endpoint_url: 'https://links.acme.io' }))
    expect(screen.getByText('In use')).toBeInTheDocument()
  })

  it('lists the values without a field nor a save bar for a member who is not the owner', () => {
    renderComponent(
      false,
      makeWorkspace({ languages: ['en', 'fr'], default_language: 'fr', email_tracking_enabled: true })
    )

    expect(screen.queryByRole('textbox')).toBeNull()
    expect(screen.queryByRole('switch')).toBeNull()
    expect(screen.queryByText('Unsaved changes')).toBeNull()
    const languages = screen.getByText('Supported languages').closest('[data-settings-row]') as HTMLElement
    expect(languages.textContent).toContain('English')
    expect(languages.textContent).toContain('French')
    expect(screen.getAllByText('On')).toHaveLength(2)
    expect(screen.getByText('Default (API endpoint)')).toBeInTheDocument()
  })
})
