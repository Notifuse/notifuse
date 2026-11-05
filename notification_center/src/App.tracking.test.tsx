import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen, waitFor, within } from '@testing-library/react'
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

describe('Email tracking section', () => {
  beforeEach(() => {
    vi.mocked(notificationCenterApi.updateContactPreferences).mockResolvedValue({ success: true })
    useParams()
  })

  it('shows what the workspace tracks and whether it is tracked for the contact', async () => {
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
      preferences({ opens_enabled: true, clicks_enabled: false, opens_tracked: false, clicks_tracked: false })
    )
    render(<App />)

    const section = await screen.findByTestId('email-tracking')
    expect(within(section).getByText('Email tracking')).toBeInTheDocument()
    expect(within(section).getByText('Open tracking')).toBeInTheDocument()
    // The workspace does not track clicks, so there is nothing to choose about them
    expect(within(section).queryByText('Click tracking')).not.toBeInTheDocument()
    expect(within(section).getByRole('button', { name: 'Allow: Open tracking' })).toBeInTheDocument()
  })

  it('has no section when the workspace tracks nothing', async () => {
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
      preferences({ opens_enabled: false, clicks_enabled: false, opens_tracked: false, clicks_tracked: false })
    )
    render(<App />)

    await screen.findByText(/Welcome/i)
    expect(screen.queryByTestId('email-tracking')).not.toBeInTheDocument()
  })

  it('records a refusal from the notification center', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    render(<App />)

    const section = await screen.findByTestId('email-tracking')
    await user.click(within(section).getByRole('button', { name: 'Stop: Click tracking' }))

    expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledWith({
      ...credentials,
      tracking_clicks_consent: 'denied'
    })
    expect(
      await within(section).findByRole('button', { name: 'Allow: Click tracking' })
    ).toBeInTheDocument()
    // Opens are left as they were
    expect(within(section).getByRole('button', { name: 'Stop: Open tracking' })).toBeInTheDocument()
  })

  it('records an agreement from the notification center', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
      preferences({ ...allTracked, opens_tracked: false })
    )
    render(<App />)

    const section = await screen.findByTestId('email-tracking')
    await user.click(within(section).getByRole('button', { name: 'Allow: Open tracking' }))

    expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledWith({
      ...credentials,
      tracking_opens_consent: 'granted'
    })
    expect(await within(section).findByRole('button', { name: 'Stop: Open tracking' })).toBeInTheDocument()
  })

  it('puts the choice back and says so when the save fails', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    vi.mocked(notificationCenterApi.updateContactPreferences).mockRejectedValueOnce(new Error('boom'))
    render(<App />)

    const section = await screen.findByTestId('email-tracking')
    await user.click(within(section).getByRole('button', { name: 'Stop: Open tracking' }))

    expect(await screen.findByText(/could not save your tracking choice/i)).toBeInTheDocument()
    expect(within(section).getByRole('button', { name: 'Stop: Open tracking' })).toBeInTheDocument()
  })

  it('moves to the section when the email links to it, writing nothing', async () => {
    useParams('tracking')
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      render(<App />)

      const heading = await screen.findByRole('heading', { name: 'Email tracking' })
      // Focus moves in an effect, after the section has rendered
      await waitFor(() => expect(heading).toHaveFocus())

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

  it('asks for a confirmation and writes nothing on load', async () => {
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    // Fake timers so the assertion covers work deferred past the render, too: email
    // security scanners open these links in real browsers
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      render(<App />)

      const prompt = await screen.findByTestId('confirm-tracking-optout')
      expect(within(prompt).getByRole('button', { name: 'Stop tracking' })).toBeInTheDocument()

      await act(() => vi.runAllTimersAsync())
      expect(notificationCenterApi.updateContactPreferences).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('refuses both opens and clicks from the email link when confirmed', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    render(<App />)

    const prompt = await screen.findByTestId('confirm-tracking-optout')
    await user.click(within(prompt).getByRole('button', { name: 'Stop tracking' }))

    expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledWith({
      ...credentials,
      tracking_opens_consent: 'denied',
      tracking_clicks_consent: 'denied',
      consent_source: 'email_link'
    })
    const banner = await screen.findByRole('status')
    expect(banner).toHaveTextContent(/no longer tracked/i)
    expect(banner).toHaveFocus()
    expect(screen.queryByTestId('confirm-tracking-optout')).not.toBeInTheDocument()

    const section = screen.getByTestId('email-tracking')
    expect(within(section).getByRole('button', { name: 'Allow: Open tracking' })).toBeInTheDocument()
    expect(within(section).getByRole('button', { name: 'Allow: Click tracking' })).toBeInTheDocument()
  })

  it('keeps the prompt and shows an error when the save fails', async () => {
    const user = userEvent.setup()
    vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences(allTracked))
    vi.mocked(notificationCenterApi.updateContactPreferences).mockRejectedValueOnce(new Error('boom'))
    render(<App />)

    const prompt = await screen.findByTestId('confirm-tracking-optout')
    await user.click(within(prompt).getByRole('button', { name: 'Stop tracking' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/could not save your tracking choice/i)
    expect(screen.getByTestId('confirm-tracking-optout')).toBeInTheDocument()
  })
})
