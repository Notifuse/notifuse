import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import type { RefObject } from 'react'
import { act, render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { ZapierSettings, type ZapierSettingsProps } from './ZapierSettings'
import {
  workspaceService,
  type ConnectZapierResponse,
  type Integration
} from '../../services/api/workspace'

// Stubbing the whole service keeps services/api/client — and the router it imports, which cycles
// back into this module — out of the suite.
vi.mock('../../services/api/workspace', () => ({
  workspaceService: {
    connectZapier: vi.fn()
  }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const connectZapier = vi.mocked(workspaceService.connectZapier)

const CONNECT_RESPONSE: ConnectZapierResponse = {
  status: 'success',
  token: 'tok_secret_value',
  email: 'zapier-marketing-3f9a1c02@api.notifuse.com',
  integration_id: 'int_zapier_1'
}

// A card that already exists, which is what puts the body in edit mode.
const EXISTING: Integration = {
  id: 'int_zapier_1',
  name: 'Marketing',
  type: 'zapier',
  zapier_settings: { api_key_email: 'zapier-marketing-3f9a1c02@api.notifuse.com' },
  created_at: '2026-08-01T10:00:00Z',
  updated_at: '2026-08-01T10:00:00Z'
}

const renderScreen = (props: Partial<ZapierSettingsProps> = {}) =>
  render(
    <App>
      <ZapierSettings workspaceId="ws1" isOwner {...props} />
    </App>
  )

const labelField = () => screen.getByLabelText('Label') as HTMLInputElement
// A regex, not the exact name: antd's spinner is an aria-labelled icon, so a button in flight
// answers to "loading Connect Zapier".
const connectButton = () => screen.getByRole('button', { name: /Connect Zapier/ })

const clipboardWriteText = vi.fn().mockResolvedValue(undefined)

beforeEach(() => {
  vi.clearAllMocks()
  window.API_ENDPOINT = 'https://api.notifuse.com'
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText: clipboardWriteText },
    configurable: true,
    writable: true
  })
  connectZapier.mockResolvedValue(CONNECT_RESPONSE)
})

afterEach(() => {
  window.API_ENDPOINT = ''
})

describe('the printed API URL', () => {
  it('strips trailing slashes, the top cause of broken self-hosted connections', () => {
    window.API_ENDPOINT = 'https://notifuse.example.com/'
    renderScreen()

    expect((screen.getByLabelText('API URL') as HTMLInputElement).value).toBe(
      'https://notifuse.example.com'
    )
  })

  it('falls back to the current origin when no API endpoint is configured', () => {
    window.API_ENDPOINT = ''
    renderScreen()

    expect((screen.getByLabelText('API URL') as HTMLInputElement).value).toBe(
      window.location.origin
    )
  })

  it('prints no workspace prefix, and copies', async () => {
    renderScreen()

    const urlInput = screen.getByLabelText('API URL') as HTMLInputElement
    expect(urlInput.value).toBe('https://api.notifuse.com')
    expect(urlInput.value).not.toContain('ws1.')

    fireEvent.click(screen.getAllByRole('button', { name: /Copy/ })[0])

    await waitFor(() => {
      expect(clipboardWriteText).toHaveBeenCalledWith('https://api.notifuse.com')
    })
  })
})

describe('connect mode', () => {
  it('links to the Zapier documentation', () => {
    renderScreen()

    expect(screen.getByText('Read the Zapier setup guide')).toHaveAttribute(
      'href',
      'https://docs.notifuse.com/integrations/zapier'
    )
  })

  it('seeds the label and mints a key under the trimmed value', async () => {
    renderScreen()

    expect(labelField().value).toBe('Zapier')

    // The server takes the label verbatim, both as the card name and as the seed of the key
    // address, and antd's whitespace rule lets a padded one through.
    fireEvent.change(labelField(), { target: { value: '  Marketing  ' } })
    fireEvent.click(connectButton())

    await waitFor(() => expect(connectZapier).toHaveBeenCalledTimes(1))
    expect(connectZapier).toHaveBeenCalledWith({ workspace_id: 'ws1', label: 'Marketing' })
  })

  it('refuses an all-blank label without asking the server', async () => {
    renderScreen()

    fireEvent.change(labelField(), { target: { value: '   ' } })
    fireEvent.click(connectButton())

    // Server-side a blank label is accepted and mints a real key on a card named "   ".
    expect(
      await screen.findByText('Please enter a label for this connection')
    ).toBeInTheDocument()
    expect(connectZapier).not.toHaveBeenCalled()
  })

  it('shows the token once, with the warning that it cannot be retrieved again', async () => {
    const onDone = vi.fn()
    renderScreen({ onDone })

    expect(screen.queryByLabelText('API key token')).not.toBeInTheDocument()

    fireEvent.click(connectButton())

    const tokenField = (await screen.findByLabelText('API key token')) as HTMLTextAreaElement
    expect(tokenField.value).toBe('tok_secret_value')
    expect(
      screen.getByText(
        'This token is displayed once and cannot be retrieved again. Copy it now and paste it into Zapier.'
      )
    ).toBeInTheDocument()
    // The panel replaces the form, so a second key cannot be minted by a stray Enter.
    expect(screen.queryByLabelText('Label')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Done' }))

    await waitFor(() => {
      expect(screen.queryByLabelText('API key token')).not.toBeInTheDocument()
    })
    expect(onDone).toHaveBeenCalledTimes(1)
    expect(screen.queryByText('tok_secret_value')).not.toBeInTheDocument()
  })

  it('copies the token', async () => {
    renderScreen()

    fireEvent.click(connectButton())
    await screen.findByLabelText('API key token')

    const copyButtons = screen.getAllByRole('button', { name: /Copy/ })
    fireEvent.click(copyButtons[copyButtons.length - 1])

    await waitFor(() => {
      expect(clipboardWriteText).toHaveBeenCalledWith('tok_secret_value')
    })
  })

  it('leaves the token panel standing while the caller refreshes the workspace', async () => {
    // The token exists in exactly one response and nothing can reissue it, so the refresh the
    // caller runs must not take the panel down with it.
    const onConnected = vi.fn()
    renderScreen({ onConnected })

    fireEvent.click(connectButton())

    await waitFor(() => expect(onConnected).toHaveBeenCalledTimes(1))
    expect(screen.getByLabelText('API key token')).toBeInTheDocument()
  })

  it('keeps the form open and surfaces the server message on failure', async () => {
    connectZapier.mockRejectedValue(new Error('api key email already in use'))

    renderScreen()
    fireEvent.click(connectButton())

    expect(await screen.findByText('api key email already in use')).toBeInTheDocument()
    expect(screen.queryByLabelText('API key token')).not.toBeInTheDocument()
    expect(connectButton()).toBeEnabled()
  })

  it('falls back to its own wording when the failure carries no message', async () => {
    connectZapier.mockRejectedValue(new Error(''))

    renderScreen()
    fireEvent.click(connectButton())

    expect(await screen.findByText('Failed to connect Zapier')).toBeInTheDocument()
  })

  it('disables the action for a non-owner rather than answering with a 403', () => {
    renderScreen({ isOwner: false })

    expect(connectButton()).toBeDisabled()
  })

  it('disables the action while a connection is in flight', async () => {
    let resolveConnect!: (value: ConnectZapierResponse) => void
    connectZapier.mockReturnValue(
      new Promise<ConnectZapierResponse>((resolve) => {
        resolveConnect = resolve
      })
    )
    const formRef: RefObject<{ submit: () => void } | null> = { current: null }
    renderScreen({ formRef })

    fireEvent.click(connectButton())

    // Nothing server-side blocks a second connect: it would mint a second key and a second card.
    await waitFor(() => expect(connectButton()).toBeDisabled())
    fireEvent.click(connectButton())
    // And the form itself refuses too, since the drawer footer can submit it past the button.
    await act(async () => {
      formRef.current?.submit()
    })
    expect(connectZapier).toHaveBeenCalledTimes(1)

    resolveConnect(CONNECT_RESPONSE)
    expect(await screen.findByLabelText('API key token')).toBeInTheDocument()
  })
})

describe('edit mode', () => {
  it('offers no connect action, which would mint a second key on a rename', () => {
    renderScreen({ integration: EXISTING })

    expect(labelField().value).toBe('Marketing')
    expect(screen.queryByRole('button', { name: /Connect/i })).not.toBeInTheDocument()
    expect(connectZapier).not.toHaveBeenCalled()
  })

  it('saves the trimmed label and carries the minted address forward', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    const formRef: RefObject<{ submit: () => void } | null> = { current: null }
    renderScreen({ integration: EXISTING, onSave, formRef })

    fireEvent.change(labelField(), { target: { value: '  Sales  ' } })
    // Submitted through the ref, which is how the drawer footer's Save reaches this form.
    await act(async () => {
      formRef.current?.submit()
    })

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
    const saved = onSave.mock.calls[0][0] as Integration
    expect(saved.name).toBe('Sales')
    expect(saved.id).toBe('int_zapier_1')
    // The address is minted once and never reissued; a rename must not drop it on the floor.
    expect(saved.zapier_settings).toEqual({
      api_key_email: 'zapier-marketing-3f9a1c02@api.notifuse.com'
    })
  })
})
