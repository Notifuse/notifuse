import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import App from './App'
import * as notificationCenterApi from './api/notification_center'
import { mockURLSearchParams, testData } from './test/utils'

// Mock the API module
vi.mock('./api/notification_center', () => ({
  getContactPreferences: vi.fn().mockResolvedValue({
    contact: {
      id: 'contact-123',
      email: 'test@example.com',
      first_name: 'John',
      last_name: 'Doe',
    },
    public_lists: [
      {
        id: 'newsletter',
        name: 'Newsletter',
        description: 'Weekly newsletter',
      },
    ],
    contact_lists: [
      {
        email: 'test@example.com',
        list_id: 'newsletter',
        list_name: 'Newsletter',
        status: 'active',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
    ],
    logo_url: 'https://example.com/logo.png',
    website_url: 'https://example.com',
  }),
  subscribeToLists: vi.fn().mockResolvedValue({ success: true }),
  unsubscribeOneClick: vi.fn().mockResolvedValue({ success: true }),
  updateContactPreferences: vi.fn().mockResolvedValue({ success: true }),
  parseNotificationCenterParams: vi.fn(),
}))

describe('Notification Center App', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('Auto-unsubscribe action', () => {
    it('should automatically unsubscribe when URL has action=unsubscribe and lid', async () => {
      const mockUnsubscribe = vi.mocked(notificationCenterApi.unsubscribeOneClick)
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue({
        ...testData.validParams,
        action: 'unsubscribe',
      })

      mockURLSearchParams({
        ...testData.validParams,
        action: 'unsubscribe',
      })

      render(<App />)

      await waitFor(() => {
        expect(mockUnsubscribe).toHaveBeenCalledWith({
          wid: testData.validParams.wid,
          email: testData.validParams.email,
          email_hmac: testData.validParams.email_hmac,
          lids: [testData.validParams.lid],
          mid: testData.validParams.mid,
        })
      })
    })

    it('should show success message after auto-unsubscribe', async () => {
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue({
        ...testData.validParams,
        action: 'unsubscribe',
      })

      mockURLSearchParams({
        ...testData.validParams,
        action: 'unsubscribe',
      })

      render(<App />)

      await waitFor(() => {
        expect(screen.getByText(/unsubscribed successfully/i)).toBeInTheDocument()
      })
    })

    it('should show error message if auto-unsubscribe fails', async () => {
      const mockUnsubscribe = vi.mocked(notificationCenterApi.unsubscribeOneClick)
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue({
        ...testData.validParams,
        action: 'unsubscribe',
      })

      mockUnsubscribe.mockRejectedValue(new Error('Unsubscribe failed'))

      mockURLSearchParams({
        ...testData.validParams,
        action: 'unsubscribe',
      })

      render(<App />)

      await waitFor(() => {
        expect(screen.getByText(/failed to unsubscribe/i)).toBeInTheDocument()
      })
    })

    it('should not unsubscribe if lid parameter is missing', async () => {
      const mockUnsubscribe = vi.mocked(notificationCenterApi.unsubscribeOneClick)
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      const paramsWithoutLid = {
        wid: testData.validParams.wid,
        email: testData.validParams.email,
        email_hmac: testData.validParams.email_hmac,
        lname: testData.validParams.lname,
        mid: testData.validParams.mid,
      }

      mockParseParams.mockReturnValue({
        ...paramsWithoutLid,
        action: 'unsubscribe',
      })

      mockURLSearchParams({
        ...paramsWithoutLid,
        action: 'unsubscribe',
      })

      render(<App />)

      await waitFor(() => {
        expect(mockUnsubscribe).not.toHaveBeenCalled()
      })
    })

    // The load performs the URL's action, so nothing the visitor does may re-run it
    it('runs the URL action only once, even after the visitor changes language', async () => {
      const user = userEvent.setup()
      const mockUnsubscribe = vi.mocked(notificationCenterApi.unsubscribeOneClick)
      mockUnsubscribe.mockResolvedValue({ success: true })
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue({
        ...testData.validParams,
        action: 'unsubscribe',
      })
      mockURLSearchParams({ ...testData.validParams, action: 'unsubscribe' })
      render(<App />)
      await screen.findByText(/unsubscribed successfully/i)

      await user.click(screen.getByRole('button', { name: /select language/i }))
      await user.click(screen.getByRole('menuitem', { name: 'Deutsch' }))
      await act(() => Promise.resolve())

      expect(mockUnsubscribe).toHaveBeenCalledTimes(1)
      expect(notificationCenterApi.getContactPreferences).toHaveBeenCalledTimes(1)
    })
  })

  // The server redirects a GET on the List-Unsubscribe header URL here. Anti-spam
  // scanners fetch that URL on their own, so loading the page must never unsubscribe.
  describe('Confirm-unsubscribe action (List-Unsubscribe header link)', () => {
    const confirmParams = { ...testData.validParams, action: 'confirm_unsubscribe' }

    beforeEach(() => {
      // An earlier test leaves unsubscribeOneClick rejecting; clearAllMocks keeps that
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValue({ success: true })
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(confirmParams)
      mockURLSearchParams(confirmParams)
    })

    it('asks to confirm for the linked list without unsubscribing on load', async () => {
      // Fake timers so the assertion also covers work deferred past the render that shows
      // the card, not only what ran before it appeared
      vi.useFakeTimers({ shouldAdvanceTime: true })
      try {
        render(<App />)

        const prompt = await screen.findByTestId('confirm-unsubscribe')
        expect(within(prompt).getByText('Newsletter')).toBeInTheDocument()
        expect(within(prompt).getByText(/stop receiving emails from this list/i)).toBeInTheDocument()
        // Named after the list: the list's row has an "Unsubscribe" button too
        expect(
          within(prompt).getByRole('button', { name: 'Unsubscribe: Newsletter' })
        ).toBeInTheDocument()

        await act(() => vi.runAllTimersAsync())
        expect(notificationCenterApi.unsubscribeOneClick).not.toHaveBeenCalled()
        expect(notificationCenterApi.updateContactPreferences).not.toHaveBeenCalled()
        expect(screen.getByTestId('confirm-unsubscribe')).toBeInTheDocument()
      } finally {
        vi.useRealTimers()
      }
    })

    it('unsubscribes when the visitor confirms', async () => {
      const user = userEvent.setup()
      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))

      expect(notificationCenterApi.unsubscribeOneClick).toHaveBeenCalledWith({
        wid: testData.validParams.wid,
        email: testData.validParams.email,
        email_hmac: testData.validParams.email_hmac,
        lids: [testData.validParams.lid],
        mid: testData.validParams.mid,
      })
      expect(await screen.findByText(/unsubscribed successfully/i)).toBeInTheDocument()
      expect(screen.queryByTestId('confirm-unsubscribe')).not.toBeInTheDocument()
      // The result is announced, and takes the focus the removed button had
      const banner = screen.getByText(/unsubscribed successfully/i).closest('[role="status"]')
      expect(banner).not.toBeNull()
      expect(banner).toHaveFocus()
      // The list's own row reflects the unsubscribe
      expect(screen.getByRole('button', { name: /^subscribe$/i })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /^unsubscribe$/i })).not.toBeInTheDocument()
    })

    it('asks about and unsubscribes only the linked list when the contact has several', async () => {
      const user = userEvent.setup()
      const activeOn = (listId: string, listName: string) => ({
        email: testData.contact.email,
        list_id: listId,
        list_name: listName,
        status: 'active',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      })
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [],
        // The linked list is not the first one
        contact_lists: [activeOn('newsletter', 'Newsletter'), activeOn('promotions', 'Promotions')],
      })
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue({
        ...confirmParams,
        lid: 'promotions',
      })
      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      expect(within(prompt).getByText('Promotions')).toBeInTheDocument()
      expect(within(prompt).queryByText('Newsletter')).not.toBeInTheDocument()

      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))

      expect(notificationCenterApi.unsubscribeOneClick).toHaveBeenCalledTimes(1)
      expect(notificationCenterApi.unsubscribeOneClick).toHaveBeenCalledWith(
        expect.objectContaining({ lids: ['promotions'] })
      )
      expect(await screen.findByText(/unsubscribed successfully/i)).toBeInTheDocument()
      // Newsletter keeps its Unsubscribe toggle; only Promotions flipped to Subscribe
      expect(screen.getAllByRole('button', { name: /^unsubscribe$/i })).toHaveLength(1)
      expect(screen.getAllByRole('button', { name: /^subscribe$/i })).toHaveLength(1)
    })

    it('keeps the prompt and shows an error when the unsubscribe fails', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockRejectedValueOnce(new Error('boom'))
      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))

      expect(await screen.findByText(/failed to unsubscribe/i)).toBeInTheDocument()
      expect(screen.getByText(/failed to unsubscribe/i).closest('[role="alert"]')).not.toBeNull()
      expect(screen.getByTestId('confirm-unsubscribe')).toBeInTheDocument()
    })

    it('tells a contact who already unsubscribed, instead of asking again', async () => {
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [{ id: 'newsletter', name: 'Newsletter' }],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'newsletter',
            list_name: 'Newsletter',
            status: 'unsubscribed',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      render(<App />)

      expect(await screen.findByRole('button', { name: /^subscribe$/i })).toBeInTheDocument()
      expect(screen.getByText(/already unsubscribed from this list/i)).toBeInTheDocument()
      expect(screen.queryByTestId('confirm-unsubscribe')).not.toBeInTheDocument()
      expect(notificationCenterApi.unsubscribeOneClick).not.toHaveBeenCalled()
    })

    it('says nothing about unsubscribing when the contact is on the list in another state', async () => {
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'newsletter',
            list_name: 'Newsletter',
            status: 'bounced',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      render(<App />)

      expect(await screen.findByText('Newsletter')).toBeInTheDocument()
      expect(screen.queryByText(/already unsubscribed/i)).not.toBeInTheDocument()
      expect(screen.queryByTestId('confirm-unsubscribe')).not.toBeInTheDocument()
    })

    it("dismisses the prompt when the visitor uses the list's own toggle instead", async () => {
      const user = userEvent.setup()
      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      const rowToggle = screen
        .getAllByRole('button', { name: /unsubscribe/i })
        .find((button) => !prompt.contains(button))
      expect(rowToggle).toBeDefined()
      await user.click(rowToggle!)

      await waitFor(() => {
        expect(notificationCenterApi.unsubscribeOneClick).toHaveBeenCalledTimes(1)
      })
      expect(screen.queryByTestId('confirm-unsubscribe')).not.toBeInTheDocument()

      // Re-subscribing from the row must not bring the confirmation back
      await user.click(await screen.findByRole('button', { name: /^subscribe$/i }))
      await waitFor(() => {
        expect(notificationCenterApi.subscribeToLists).toHaveBeenCalledTimes(1)
      })
      expect(screen.queryByTestId('confirm-unsubscribe')).not.toBeInTheDocument()
    })
  })

  describe('Auto-confirm action', () => {
    it('should automatically subscribe when URL has action=confirm and lid', async () => {
      const mockSubscribe = vi.mocked(notificationCenterApi.subscribeToLists)
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue({
        ...testData.validParams,
        action: 'confirm',
      })

      mockURLSearchParams({
        ...testData.validParams,
        action: 'confirm',
      })

      render(<App />)

      await waitFor(() => {
        expect(mockSubscribe).toHaveBeenCalledWith({
          workspace_id: testData.validParams.wid,
          contact: {
            id: '',
            email: testData.validParams.email,
            email_hmac: testData.validParams.email_hmac,
          },
          list_ids: [testData.validParams.lid],
        })
      })
    })

    it('should show success message after auto-confirm', async () => {
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue({
        ...testData.validParams,
        action: 'confirm',
      })

      mockURLSearchParams({
        ...testData.validParams,
        action: 'confirm',
      })

      render(<App />)

      await waitFor(() => {
        expect(screen.getByText(/subscription confirmed successfully/i)).toBeInTheDocument()
      })
    })

    it('should show error message if auto-confirm fails', async () => {
      const mockSubscribe = vi.mocked(notificationCenterApi.subscribeToLists)
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue({
        ...testData.validParams,
        action: 'confirm',
      })

      mockSubscribe.mockRejectedValue(new Error('Subscribe failed'))

      mockURLSearchParams({
        ...testData.validParams,
        action: 'confirm',
      })

      render(<App />)

      await waitFor(() => {
        expect(screen.getByText(/failed to confirm subscription/i)).toBeInTheDocument()
      })
    })
  })

  describe('Preview mode', () => {
    it('should detect preview mode with mid=preview', async () => {
      const mockGetPreferences = vi.mocked(notificationCenterApi.getContactPreferences)
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue({
        ...testData.previewParams,
      })

      mockURLSearchParams(testData.previewParams)

      render(<App />)

      await waitFor(() => {
        expect(mockGetPreferences).not.toHaveBeenCalled()
      })
    })

    it('should detect preview mode with test email john.doe@example.com', async () => {
      const mockGetPreferences = vi.mocked(notificationCenterApi.getContactPreferences)
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue(testData.previewParams)

      mockURLSearchParams(testData.previewParams)

      render(<App />)

      await waitFor(() => {
        expect(mockGetPreferences).not.toHaveBeenCalled()
      })
    })

    it('should detect preview mode with email_hmac=abc123', async () => {
      const mockGetPreferences = vi.mocked(notificationCenterApi.getContactPreferences)
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue(testData.previewParams)

      mockURLSearchParams(testData.previewParams)

      render(<App />)

      await waitFor(() => {
        expect(mockGetPreferences).not.toHaveBeenCalled()
      })
    })
  })

  describe('Missing parameters', () => {
    it('should show error when required parameters are missing', async () => {
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue(null)

      mockURLSearchParams({})

      render(<App />)

      await waitFor(() => {
        expect(screen.getByText(/missing/i)).toBeInTheDocument()
      })
    })
  })

  describe('Normal notification center loading', () => {
    it('should load contact preferences when valid params are provided', async () => {
      const mockGetPreferences = vi.mocked(notificationCenterApi.getContactPreferences)
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue(testData.validParams)

      mockURLSearchParams(testData.validParams)

      render(<App />)

      await waitFor(() => {
        expect(mockGetPreferences).toHaveBeenCalledWith({
          workspace_id: testData.validParams.wid,
          email: testData.validParams.email,
          email_hmac: testData.validParams.email_hmac,
        })
      })
    })

    it('should display contact name after loading', async () => {
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue(testData.validParams)

      mockURLSearchParams(testData.validParams)

      render(<App />)

      await waitFor(() => {
        expect(screen.getByText(/Welcome/i)).toBeInTheDocument()
        expect(screen.getByText(/John/i)).toBeInTheDocument()
      })
    })
  })

  describe('Manual unsubscribe via UI', () => {
    it('should call unsubscribeOneClick when clicking unsubscribe button', async () => {
      const user = userEvent.setup()
      const mockUnsubscribe = vi.mocked(notificationCenterApi.unsubscribeOneClick)
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue(testData.validParams)
      mockURLSearchParams(testData.validParams)

      render(<App />)

      // Wait for the component to load by finding unsubscribe button
      const unsubscribeButtons = await screen.findAllByRole('button', { name: /unsubscribe/i })
      expect(unsubscribeButtons.length).toBeGreaterThan(0)

      // Click the first unsubscribe button
      await user.click(unsubscribeButtons[0])

      await waitFor(() => {
        expect(mockUnsubscribe).toHaveBeenCalled()
      })
    })
  })

  describe('Manual subscribe via UI (Bug #181)', () => {
    it('should include email_hmac in subscribe payload for private list support', async () => {
      const user = userEvent.setup()
      const mockSubscribe = vi.mocked(notificationCenterApi.subscribeToLists)
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)
      const mockGetPreferences = vi.mocked(notificationCenterApi.getContactPreferences)

      // Mock preferences with an unsubscribed list to show Subscribe button
      mockGetPreferences.mockResolvedValue({
        contact: {
          id: 'contact-123',
          email: 'test@example.com',
          first_name: 'John',
          last_name: 'Doe',
        },
        public_lists: [
          {
            id: 'announcements',
            name: 'Announcements',
            description: 'Important updates',
          },
        ],
        contact_lists: [], // No subscriptions - will show Subscribe buttons
        logo_url: 'https://example.com/logo.png',
        website_url: 'https://example.com',
      })

      mockParseParams.mockReturnValue(testData.validParams)
      mockURLSearchParams(testData.validParams)

      render(<App />)

      // Wait for the component to load by finding subscribe button
      const subscribeButtons = await screen.findAllByRole('button', { name: /subscribe/i })
      expect(subscribeButtons.length).toBeGreaterThan(0)

      // Click the first subscribe button
      await user.click(subscribeButtons[0])

      await waitFor(() => {
        expect(mockSubscribe).toHaveBeenCalled()
      })

      // CRITICAL: Verify email_hmac is included in the contact object
      // This is the fix for Bug #181 - manual subscribe must include email_hmac
      const callArgs = mockSubscribe.mock.calls[0][0]
      expect(callArgs.contact.email_hmac).toBe(testData.validParams.email_hmac)
    })
  })

  describe('URL parameter parsing', () => {
    it('should handle URL with all required parameters', async () => {
      const mockGetPreferences = vi.mocked(notificationCenterApi.getContactPreferences)
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue(testData.validParams)

      mockURLSearchParams(testData.validParams)

      render(<App />)

      await waitFor(() => {
        expect(mockGetPreferences).toHaveBeenCalled()
      })
    })

    it('should handle URL with action parameter', async () => {
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue({
        ...testData.validParams,
        action: 'unsubscribe',
      })

      mockURLSearchParams({
        ...testData.validParams,
        action: 'unsubscribe',
      })

      render(<App />)

      await waitFor(() => {
        expect(mockParseParams).toHaveBeenCalled()
      })
    })
  })

  // The browser's language and timezone are saved on the visitor's first click, tap or key
  // press, never on load: email security scanners open these links in real browsers.
  describe('Preference sync on first interaction', () => {
    const browserTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone
    const credentials = {
      workspace_id: testData.validParams.wid,
      email: testData.validParams.email,
      email_hmac: testData.validParams.email_hmac,
    }
    const preferences = (contact: { language?: string; timezone?: string } = {}) => ({
      contact: { ...testData.contact, ...contact },
      public_lists: [],
      contact_lists: [],
    })

    beforeEach(() => {
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(testData.validParams)
      vi.mocked(notificationCenterApi.updateContactPreferences).mockResolvedValue({ success: true })
      mockURLSearchParams(testData.validParams)
    })

    it('saves nothing on load, even once every pending timer has run', async () => {
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences())
      vi.useFakeTimers({ shouldAdvanceTime: true })
      try {
        render(<App />)
        expect(await screen.findByText(/Welcome/i)).toBeInTheDocument()

        await act(() => vi.runAllTimersAsync())
        expect(notificationCenterApi.updateContactPreferences).not.toHaveBeenCalled()
      } finally {
        vi.useRealTimers()
      }
    })

    it("saves the browser's language and timezone once, on the first click", async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences())
      render(<App />)
      await screen.findByText(/Welcome/i)

      await user.click(document.body)

      await waitFor(() => {
        expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledTimes(1)
      })
      expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledWith({
        ...credentials,
        language: 'en',
        timezone: browserTimezone,
      })

      // Later interactions save nothing more
      await user.click(document.body)
      await user.keyboard('{Tab}')
      await act(() => Promise.resolve())
      expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledTimes(1)
    })

    it('counts a key press as the first interaction', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences())
      render(<App />)
      await screen.findByText(/Welcome/i)

      await user.keyboard('{Tab}')

      await waitFor(() => {
        expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledTimes(1)
      })
    })

    it('saves once loaded when the visitor interacted while the page was loading', async () => {
      const user = userEvent.setup()
      let finishLoading: (value: ReturnType<typeof preferences>) => void = () => {}
      vi.mocked(notificationCenterApi.getContactPreferences).mockReturnValueOnce(
        new Promise((resolve) => {
          finishLoading = resolve
        })
      )
      render(<App />)

      await user.click(document.body)
      expect(notificationCenterApi.updateContactPreferences).not.toHaveBeenCalled()

      finishLoading(preferences())
      await waitFor(() => {
        expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledWith({
          ...credentials,
          language: 'en',
          timezone: browserTimezone,
        })
      })
    })

    it('does not save a language the contact already has', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
        preferences({ language: 'fr' })
      )
      render(<App />)
      await screen.findByText(/Welcome/i)

      await user.click(document.body)

      await waitFor(() => {
        expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledTimes(1)
      })
      expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledWith({
        ...credentials,
        timezone: browserTimezone,
      })
    })

    it('saves nothing when the contact already has a language and the same timezone', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
        preferences({ language: 'fr', timezone: browserTimezone })
      )
      render(<App />)
      await screen.findByText(/Welcome/i)

      await user.click(document.body)
      await act(() => Promise.resolve())

      expect(notificationCenterApi.updateContactPreferences).not.toHaveBeenCalled()
    })

    it("saves the visitor's language choice after the detected one, so the choice wins", async () => {
      const user = userEvent.setup()
      let finishDetectedWrite: (value: { success: boolean }) => void = () => {}
      vi.mocked(notificationCenterApi.updateContactPreferences).mockReturnValueOnce(
        new Promise((resolve) => {
          finishDetectedWrite = resolve
        })
      )
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences())
      render(<App />)
      await screen.findByText(/Welcome/i)

      // Opening the language menu is the first interaction: the detected preferences go out
      await user.click(screen.getByRole('button', { name: /select language/i }))
      await waitFor(() => {
        expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledTimes(1)
      })
      expect(notificationCenterApi.updateContactPreferences).toHaveBeenLastCalledWith(
        expect.objectContaining({ language: 'en' })
      )

      // The choice waits for that write rather than racing it
      await user.click(screen.getByRole('menuitem', { name: 'Deutsch' }))
      await act(() => Promise.resolve())
      expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledTimes(1)

      finishDetectedWrite({ success: true })
      await waitFor(() => {
        expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledTimes(2)
      })
      expect(notificationCenterApi.updateContactPreferences).toHaveBeenLastCalledWith({
        ...credentials,
        language: 'de',
      })
    })

    it("still saves the visitor's language choice when the detected write failed", async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.updateContactPreferences).mockRejectedValueOnce(
        new Error('Network error')
      )
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(preferences())
      render(<App />)
      await screen.findByText(/Welcome/i)

      await user.click(screen.getByRole('button', { name: /select language/i }))
      await user.click(screen.getByRole('menuitem', { name: 'Deutsch' }))

      await waitFor(() => {
        expect(notificationCenterApi.updateContactPreferences).toHaveBeenCalledTimes(2)
      })
      expect(notificationCenterApi.updateContactPreferences).toHaveBeenLastCalledWith({
        ...credentials,
        language: 'de',
      })
    })
  })

  describe('Error handling', () => {
    it('should handle API errors gracefully', async () => {
      const mockGetPreferences = vi.mocked(notificationCenterApi.getContactPreferences)
      const mockParseParams = vi.mocked(notificationCenterApi.parseNotificationCenterParams)

      mockParseParams.mockReturnValue(testData.validParams)
      mockGetPreferences.mockRejectedValue(new Error('API Error'))

      mockURLSearchParams(testData.validParams)

      render(<App />)

      await waitFor(() => {
        // Both "Error" (heading) and "API Error" (message) contain the word "error"
        const errorTexts = screen.getAllByText(/error/i)
        expect(errorTexts.length).toBeGreaterThan(0)
      })
    })
  })
})

