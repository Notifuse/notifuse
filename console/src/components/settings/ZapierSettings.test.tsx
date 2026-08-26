import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { ZapierSettings, ZAPIER_SETTINGS_ENABLED } from './ZapierSettings'
import { ZAPIER_KEY_GRANTS } from './zapierGrants'
import { workspaceService } from '../../services/api/workspace'
import {
  ALL_PERMISSION_RESOURCES,
  isPermissionEnforced,
  type PermissionResource,
  type UserPermissions
} from '../../services/api/permissions'

// services/api/client pulls in the router, which imports every page and cycles back into this
// module. Stubbing it keeps that graph out of the suite.
vi.mock('../../services/api/client', () => ({
  api: {
    post: vi.fn().mockResolvedValue({}),
    get: vi.fn().mockResolvedValue({})
  },
  ApiError: class ApiError extends Error {}
}))

vi.mock('../../services/api/workspace', () => ({
  workspaceService: {
    createAPIKey: vi.fn()
  }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const createAPIKey = vi.mocked(workspaceService.createAPIKey)

// Imported from the component rather than restated here. A test that repeats the grant list
// asserts the component agrees with the test, which is a tautology: it went on passing while
// `segments` was missing, and the two segment triggers 403'd for everyone who onboarded the way
// the screen tells them to.
const grantedResources = ZAPIER_KEY_GRANTS.map(([resource]) => resource)

const renderScreen = () =>
  render(
    <App>
      <ZapierSettings workspaceId="ws1" enabled />
    </App>
  )

const clipboardWriteText = vi.fn().mockResolvedValue(undefined)

beforeEach(() => {
  vi.clearAllMocks()
  window.API_ENDPOINT = 'https://api.notifuse.com'
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText: clipboardWriteText },
    configurable: true,
    writable: true
  })
  createAPIKey.mockResolvedValue({ token: 'tok_secret_value', email: 'zapier@api.notifuse.com' })
})

afterEach(() => {
  window.API_ENDPOINT = ''
})

describe('ZapierSettings ship flag', () => {
  it('stays closed until the Zapier app is published', () => {
    expect(ZAPIER_SETTINGS_ENABLED).toBe(false)
  })

  it('renders nothing when the flag is off', () => {
    const { container } = render(
      <App>
        <ZapierSettings workspaceId="ws1" />
      </App>
    )

    expect(container.querySelector('input')).toBeNull()
    expect(screen.queryByText('Create a Zapier API key')).not.toBeInTheDocument()
  })
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
})

describe('ZapierSettings', () => {
  it('prints the API URL without a workspace prefix and copies it', async () => {
    renderScreen()

    const urlInput = screen.getByLabelText('API URL') as HTMLInputElement
    expect(urlInput.value).toBe('https://api.notifuse.com')
    expect(urlInput.value).not.toContain('ws1.')

    fireEvent.click(screen.getAllByRole('button', { name: /Copy/ })[0])

    await waitFor(() => {
      expect(clipboardWriteText).toHaveBeenCalledWith('https://api.notifuse.com')
    })
  })

  it('links to the Zapier documentation', () => {
    renderScreen()

    expect(screen.getByText('Read the Zapier setup guide')).toHaveAttribute(
      'href',
      'https://docs.notifuse.com/integrations/zapier'
    )
  })

  it('grants read and write on exactly the resources Zapier needs', async () => {
    renderScreen()

    fireEvent.click(screen.getByRole('button', { name: 'Create a Zapier API key' }))

    await waitFor(() => expect(createAPIKey).toHaveBeenCalledTimes(1))

    const request = createAPIKey.mock.calls[0][0]
    expect(request.workspace_id).toBe('ws1')
    expect(request.email_prefix).toBe('zapier')

    const permissions = request.permissions as UserPermissions
    for (const [resource, verbs] of ZAPIER_KEY_GRANTS) {
      expect(permissions[resource]).toEqual(verbs)
    }

    // Every other resource is denied. The verbs the API cannot gate stay granted on purpose —
    // a stored `false` there is permanent, since backfills only ever add missing keys.
    const others = ALL_PERMISSION_RESOURCES.filter(
      (resource) => !grantedResources.includes(resource)
    )
    const granted: PermissionResource[] = []
    for (const resource of others) {
      if (permissions[resource].read && isPermissionEnforced(resource, 'read')) {
        granted.push(resource)
      }
      if (permissions[resource].write && isPermissionEnforced(resource, 'write')) {
        granted.push(resource)
      }
    }
    expect(granted).toEqual([])
  })

  // Every trigger the screen advertises has to work with the key the screen creates. These are
  // named one by one rather than looped, because the failure mode is a permission nobody
  // remembered the trigger needed: `segments` was missing, so the segment picker rendered a
  // permission error and "Test trigger" 403'd on segments.contacts — two of the six visible
  // triggers unusable, which is also the obligation that gates Zapier's public review.
  it('grants every permission the advertised triggers and actions require', async () => {
    renderScreen()

    fireEvent.click(screen.getByRole('button', { name: 'Create a Zapier API key' }))
    await waitFor(() => expect(createAPIKey).toHaveBeenCalledTimes(1))

    const permissions = createAPIKey.mock.calls[0][0].permissions as UserPermissions

    // Turning any Zap on and off.
    expect(permissions.webhook_subscriptions).toEqual({ read: true, write: true })

    // Contact triggers, and the sample data behind every other trigger: both list triggers read
    // contacts.list and both segment triggers read segments.contacts?expand=contact, which is
    // gated on contacts:read as well as segments:read.
    expect(permissions.contacts.read).toBe(true)

    // The list picker, plus the member lookup behind New List Subscriber.
    expect(permissions.lists.read).toBe(true)

    // The segment picker, plus the member lookup behind Contact Joined/Left Segment.
    expect(permissions.segments.read).toBe(true)

    // Create or Update Contact, and Subscribe Contact to List.
    expect(permissions.contacts.write).toBe(true)
    expect(permissions.lists.write).toBe(true)

    // Nothing in the app writes a segment, so the key does not get to.
    expect(permissions.segments.write).toBe(false)
  })

  it('sends the edited key name', async () => {
    renderScreen()

    fireEvent.change(screen.getByLabelText('API key name'), { target: { value: 'Zapier Prod' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create a Zapier API key' }))

    await waitFor(() => expect(createAPIKey).toHaveBeenCalledTimes(1))
    expect(createAPIKey.mock.calls[0][0].email_prefix).toBe('zapier_prod')
  })

  it('shows the token once, with the warning that it cannot be retrieved again', async () => {
    renderScreen()

    expect(screen.queryByLabelText('API key token')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Create a Zapier API key' }))

    const tokenField = (await screen.findByLabelText('API key token')) as HTMLTextAreaElement
    expect(tokenField.value).toBe('tok_secret_value')
    expect(
      screen.getByText(
        'This token is displayed once and cannot be retrieved again. Copy it now and paste it into Zapier.'
      )
    ).toBeInTheDocument()

    // Dismissing the panel discards the token: the create form comes back and the token is gone.
    fireEvent.click(screen.getByRole('button', { name: 'Done' }))

    await waitFor(() => {
      expect(screen.queryByLabelText('API key token')).not.toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: 'Create a Zapier API key' })).toBeInTheDocument()
    expect(screen.queryByText('tok_secret_value')).not.toBeInTheDocument()
  })

  it('copies the token', async () => {
    renderScreen()

    fireEvent.click(screen.getByRole('button', { name: 'Create a Zapier API key' }))
    await screen.findByLabelText('API key token')

    const copyButtons = screen.getAllByRole('button', { name: /Copy/ })
    fireEvent.click(copyButtons[copyButtons.length - 1])

    await waitFor(() => {
      expect(clipboardWriteText).toHaveBeenCalledWith('tok_secret_value')
    })
  })

  it('keeps the form open and surfaces the error when creation fails', async () => {
    createAPIKey.mockRejectedValue(new Error('api key email already in use'))

    renderScreen()
    fireEvent.click(screen.getByRole('button', { name: 'Create a Zapier API key' }))

    expect(await screen.findByText('api key email already in use')).toBeInTheDocument()
    expect(screen.queryByLabelText('API key token')).not.toBeInTheDocument()
  })
})
