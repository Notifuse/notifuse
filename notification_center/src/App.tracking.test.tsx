import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import App from './App'
import * as notificationCenterApi from './api/notification_center'
import type { ContactPreferencesResponse } from './api/notification_center'
import { mockURLSearchParams, testData } from './test/utils'

vi.mock('./api/notification_center', () => ({
  getContactPreferences: vi.fn(),
  subscribeToLists: vi.fn().mockResolvedValue({ success: true }),
  unsubscribeOneClick: vi.fn().mockResolvedValue({ success: true }),
  updateContactPreferences: vi.fn().mockResolvedValue({ success: true }),
  parseNotificationCenterParams: vi.fn()
}))

const preferences = (
  tracking?: ContactPreferencesResponse['tracking']
): ContactPreferencesResponse => ({
  contact: { ...testData.contact, language: 'en', timezone: Intl.DateTimeFormat().resolvedOptions().timeZone },
  public_lists: [],
  contact_lists: [],
  ...(tracking ? { tracking } : {})
})

const allTracked = { opens_enabled: true, clicks_enabled: true, opens_tracked: true, clicks_tracked: true }

const credentials = {
  workspace_id: testData.validParams.wid,
  email: testData.validParams.email,
  email_hmac: testData.validParams.email_hmac
}

const useParams = (action?: string) => {
  const params = { ...testData.validParams, ...(action ? { action } : {}) }
  vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(params)
  mockURLSearchParams(params)
}

// Opens the tracking dialog from the status line.
const openDialog = async (user: ReturnType<typeof userEvent.setup>) => {
  const line = await screen.findByTestId('email-tracking')
  await user.click(within(line).getByRole('button', { name: 'Change' }))
  return screen.getByRole('dialog', { name: 'Email performance tracking' })
}

describe('Email tracking status line', () => {
  beforeEach(() => {
    vi.mocked(notificationCenterApi.updateContactPreferences).mockResolvedValue({ success: true })
    useParams()
  })

  it.each([
    ['on', allTracked],
    ['off', { ...allTracked, opens_tracked: false, clicks_tracked: false }],
    ['partly on', { ...allTracked, clicks_tracked: false }],
    // A kind the workspace does not track says nothing about the contact
    ['on', { ...allTracked, clicks_enabled: false, clicks_tracked: false }]
  ])('says tracking is %s, on one line, with no dialog open', async (status, tracking) => {
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(tracking))
    render(<App />)

    const line = await screen.findByTestId('email-tracking')
    expect(line).toHaveTextContent(`Email performance tracking: ${status}`)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('has no line when the workspace tracks nothing', async () => {
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
      preferences({ opens_enabled: false, clicks_enabled: false, opens_tracked: false, clicks_tracked: false })
    )
    render(<App />)

    await screen.findByText(/Welcome/i)
    expect(screen.queryByTestId('email-tracking')).not.toBeInTheDocument()
  })
})

describe('Email tracking dialog', () => {
  beforeEach(() => {
    vi.mocked(notificationCenterApi.updateContactPreferences).mockResolvedValue({ success: true })
    useParams()
  })

  it('shows only the kinds the workspace tracks', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
      preferences({ opens_enabled: true, clicks_enabled: false, opens_tracked: false, clicks_tracked: false })
    )
    render(<App />)

    const dialog = await openDialog(user)
    expect(within(dialog).getByText('Open tracking')).toBeInTheDocument()
    expect(within(dialog).queryByText('Click tracking')).not.toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: 'Allow: Open tracking' })).toBeInTheDocument()
  })

  it('records a refusal, and the status line follows', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    render(<App />)

    const dialog = await openDialog(user)
    await user.click(within(dialog).getByRole('button', { name: 'Stop: Click tracking' }))

    expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledWith({
      ...credentials,
      tracking_clicks_consent: 'denied'
    })
    expect(await within(dialog).findByRole('button', { name: 'Allow: Click tracking' })).toBeInTheDocument()
    // Opens are left as they were
    expect(within(dialog).getByRole('button', { name: 'Stop: Open tracking' })).toBeInTheDocument()
    expect(screen.getByTestId('email-tracking')).toHaveTextContent('Email performance tracking: partly on')
  })

  it('records an agreement', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
      preferences({ ...allTracked, opens_tracked: false })
    )
    render(<App />)

    const dialog = await openDialog(user)
    await user.click(within(dialog).getByRole('button', { name: 'Allow: Open tracking' }))

    expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledWith({
      ...credentials,
      tracking_opens_consent: 'granted'
    })
    expect(await within(dialog).findByRole('button', { name: 'Stop: Open tracking' })).toBeInTheDocument()
  })

  it('puts the choice back and says so when the save fails', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    vi.mocked(notificationCenterApi.updateContactPreferences).mockRejectedValueOnce(new Error('boom'))
    render(<App />)

    const dialog = await openDialog(user)
    await user.click(within(dialog).getByRole('button', { name: 'Stop: Open tracking' }))

    expect(await screen.findByText(/could not save your tracking choice/i)).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: 'Stop: Open tracking' })).toBeInTheDocument()
  })

  it('closes with its button and with Escape', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    render(<App />)

    let dialog = await openDialog(user)
    await user.click(within(dialog).getByRole('button', { name: 'Close' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    // Focus is back on the link that opened it
    expect(screen.getByRole('button', { name: 'Change' })).toHaveFocus()

    dialog = await openDialog(user)
    expect(dialog).toHaveFocus()
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('keeps Tab within the dialog', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    render(<App />)

    const dialog = await openDialog(user)
    const buttons = within(dialog).getAllByRole('button')
    buttons[buttons.length - 1].focus()
    await user.tab()
    expect(buttons[0]).toHaveFocus()
    await user.tab({ shift: true })
    expect(buttons[buttons.length - 1]).toHaveFocus()
  })

  it('opens on load when the email links to it, writing nothing', async () => {
    useParams('tracking')
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      render(<App />)

      const dialog = await screen.findByRole('dialog', { name: 'Email performance tracking' })
      expect(within(dialog).getByRole('button', { name: 'Stop: Open tracking' })).toBeInTheDocument()

      await act(() => vi.runAllTimersAsync())
      expect(notificationCenterApi.updateContactPreferences).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('Tracking opt-out link', () => {
  beforeEach(() => {
    vi.mocked(notificationCenterApi.updateContactPreferences).mockResolvedValue({ success: true })
    useParams('tracking_optout')
  })

  it('asks for a confirmation in the dialog and writes nothing on load', async () => {
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    // Fake timers so the assertion covers work deferred past the render, too: email
    // security scanners open these links in real browsers
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      render(<App />)

      const dialog = await screen.findByRole('dialog', { name: 'Email performance tracking' })
      expect(within(dialog).getByRole('button', { name: 'Stop tracking' })).toBeInTheDocument()

      await act(() => vi.runAllTimersAsync())
      expect(notificationCenterApi.updateContactPreferences).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('refuses both opens and clicks from the email link when confirmed, then shows the new choice', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    render(<App />)

    const dialog = await screen.findByRole('dialog', { name: 'Email performance tracking' })
    await user.click(within(dialog).getByRole('button', { name: 'Stop tracking' }))

    expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledWith({
      ...credentials,
      tracking_opens_consent: 'denied',
      tracking_clicks_consent: 'denied',
      consent_source: 'email_link'
    })
    expect(await screen.findByText(/no longer tracked/i)).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: 'Allow: Open tracking' })).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: 'Allow: Click tracking' })).toBeInTheDocument()
    expect(screen.getByTestId('email-tracking')).toHaveTextContent('Email performance tracking: off')
  })

  it('cancels without writing anything', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    render(<App />)

    const dialog = await screen.findByRole('dialog', { name: 'Email performance tracking' })
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(notificationCenterApi.updateContactPreferences).not.toHaveBeenCalled()
    expect(screen.getByTestId('email-tracking')).toHaveTextContent('Email performance tracking: on')
  })

  it('cannot be closed while the refusal is being saved', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    let finish: (value: { success: boolean }) => void = () => {}
    vi.mocked(notificationCenterApi.updateContactPreferences).mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve
      })
    )
    render(<App />)

    const dialog = await screen.findByRole('dialog', { name: 'Email performance tracking' })
    await user.click(within(dialog).getByRole('button', { name: 'Stop tracking' }))
    await user.keyboard('{Escape}')
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await act(async () => finish({ success: true }))
    expect(await within(dialog).findByRole('button', { name: 'Allow: Open tracking' })).toBeInTheDocument()
  })

  it('records the refusal even while the workspace tracks nothing', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
      preferences({ opens_enabled: false, clicks_enabled: false, opens_tracked: false, clicks_tracked: false })
    )
    render(<App />)

    const dialog = await screen.findByRole('dialog', { name: 'Email performance tracking' })
    await user.click(within(dialog).getByRole('button', { name: 'Stop tracking' }))

    expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledWith({
      ...credentials,
      tracking_opens_consent: 'denied',
      tracking_clicks_consent: 'denied',
      consent_source: 'email_link'
    })
    expect(await screen.findByText(/no longer tracked/i)).toBeInTheDocument()
    // Nothing is tracked to choose about, so there is no dialog or line left
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.queryByTestId('email-tracking')).not.toBeInTheDocument()
  })

  it('keeps the confirmation and shows an error when the save fails', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    vi.mocked(notificationCenterApi.updateContactPreferences).mockRejectedValueOnce(new Error('boom'))
    render(<App />)

    const dialog = await screen.findByRole('dialog', { name: 'Email performance tracking' })
    await user.click(within(dialog).getByRole('button', { name: 'Stop tracking' }))

    expect(await within(dialog).findByRole('alert')).toHaveTextContent(/could not save your tracking choice/i)
    expect(within(dialog).getByRole('button', { name: 'Stop tracking' })).toBeInTheDocument()
  })
})
