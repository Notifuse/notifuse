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
  resubscribeToList: vi.fn().mockResolvedValue({ success: true }),
  unsubscribeOneClick: vi.fn().mockResolvedValue({ success: true }),
  updateContactPreferences: vi.fn().mockResolvedValue({ success: true }),
  parseNotificationCenterParams: vi.fn(),
}))

describe('Notification Center App', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // The email's unsubscribe link. The cases it shares with the List-Unsubscribe header
  // link run for both actions in the describe that follows.
  describe('Unsubscribe link in the email (action=unsubscribe)', () => {
    const linkParams = { ...testData.validParams, action: 'unsubscribe' }

    beforeEach(() => {
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValue({ success: true })
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(linkParams)
      mockURLSearchParams(linkParams)
    })

    it('asks nothing and writes nothing without a list in the link', async () => {
      const withoutLid = {
        wid: testData.validParams.wid,
        email: testData.validParams.email,
        email_hmac: testData.validParams.email_hmac,
        mid: testData.validParams.mid,
        action: 'unsubscribe',
      }
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(withoutLid)
      mockURLSearchParams(withoutLid)
      vi.useFakeTimers({ shouldAdvanceTime: true })
      try {
        render(<App />)

        expect(await screen.findByText('Newsletter')).toBeInTheDocument()
        await act(() => vi.runAllTimersAsync())
        expect(screen.queryByTestId('confirm-unsubscribe')).not.toBeInTheDocument()
        expect(notificationCenterApi.unsubscribeOneClick).not.toHaveBeenCalled()
        expect(notificationCenterApi.updateContactPreferences).not.toHaveBeenCalled()
      } finally {
        vi.useRealTimers()
      }
    })

    it('keeps asking, and unsubscribes no one, when the visitor only changes the language', async () => {
      const user = userEvent.setup()
      render(<App />)
      await screen.findByTestId('confirm-unsubscribe')

      await user.click(screen.getByRole('button', { name: /select language/i }))
      await user.click(screen.getByRole('menuitem', { name: 'Deutsch' }))
      await act(() => Promise.resolve())

      expect(notificationCenterApi.unsubscribeOneClick).not.toHaveBeenCalled()
      expect(notificationCenterApi.getContactPreferences).toHaveBeenCalledTimes(1)
      expect(screen.getByTestId('confirm-unsubscribe')).toBeInTheDocument()
    })

    // The page reads the contact before anything else: an erased contact gets an error,
    // with nothing written in between
    it('writes nothing for a contact the workspace erased', async () => {
      vi.mocked(notificationCenterApi.getContactPreferences).mockRejectedValueOnce(
        new Error('Contact not found')
      )
      render(<App />)

      expect(await screen.findByText('Contact not found')).toBeInTheDocument()
      expect(notificationCenterApi.unsubscribeOneClick).not.toHaveBeenCalled()
      expect(screen.queryByText(/unsubscribed successfully/i)).not.toBeInTheDocument()
    })
  })

  // The email's unsubscribe link lands here, and so does the List-Unsubscribe header URL,
  // whose GET the server redirects. Email security scanners open every link of an email,
  // in a real browser too, so loading the page must never unsubscribe.
  describe.each([
    ['unsubscribe', "the email's unsubscribe link"],
    ['confirm_unsubscribe', 'the List-Unsubscribe header link'],
  ])('Unsubscribe confirmation, action=%s (%s)', (action) => {
    const confirmParams = { ...testData.validParams, action }

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
      expect(await screen.findByText(/unsubscribed from newsletter/i)).toBeInTheDocument()
      expect(screen.queryByTestId('confirm-unsubscribe')).not.toBeInTheDocument()
      // The result is announced, and takes the focus the removed button had
      const banner = screen.getByText(/unsubscribed from newsletter/i).closest('[role="status"]')
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
        // Public, so the unsubscribed row offers Subscribe again (private lists don't)
        public_lists: [
          { id: 'newsletter', name: 'Newsletter' },
          { id: 'promotions', name: 'Promotions' },
        ],
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
      expect(await screen.findByText(/unsubscribed from promotions/i)).toBeInTheDocument()
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

    // Automations also email contacts who aren't on their list, with that list's header
    // link. Telling them they already unsubscribed would leave them receiving its emails.
    it('asks a contact who is not on the linked public list, and unsubscribes on confirm', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [{ id: 'newsletter', name: 'Newsletter' }],
        contact_lists: [],
      })
      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      expect(within(prompt).getByText('Newsletter')).toBeInTheDocument()
      expect(screen.queryByText(/already unsubscribed/i)).not.toBeInTheDocument()

      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))

      expect(notificationCenterApi.unsubscribeOneClick).toHaveBeenCalledWith(
        expect.objectContaining({ lids: ['newsletter'] })
      )
      expect(await screen.findByText(/unsubscribed from newsletter/i)).toBeInTheDocument()
    })

    // Marketing emails reach a contact whose double opt-in is still pending, so the header
    // link must let them out too: the row's own button would subscribe them instead
    it('asks a contact whose subscription is still pending, and unsubscribes on confirm', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [{ id: 'newsletter', name: 'Newsletter' }],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'newsletter',
            list_name: 'Newsletter',
            status: 'pending',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      expect(within(prompt).getByText('Newsletter')).toBeInTheDocument()

      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))

      expect(notificationCenterApi.unsubscribeOneClick).toHaveBeenCalledWith(
        expect.objectContaining({ lids: ['newsletter'] })
      )
      expect(await screen.findByText(/unsubscribed from newsletter/i)).toBeInTheDocument()
    })

    it('asks a contact who is not on the linked private list, without showing its ID', async () => {
      const user = userEvent.setup()
      // The preferences carry neither the private list nor a row for it
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [],
        contact_lists: [],
      })
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue({
        ...confirmParams,
        lid: 'vipcustomers',
      })
      render(<App />)

      // The page doesn't know the list's name, and its ID means nothing to the contact
      const prompt = await screen.findByTestId('confirm-unsubscribe')
      expect(within(prompt).getByText(/stop receiving emails from this list/i)).toBeInTheDocument()
      expect(within(prompt).queryByText(/vipcustomers/)).not.toBeInTheDocument()

      await user.click(within(prompt).getByRole('button', { name: /^unsubscribe$/i }))

      expect(notificationCenterApi.unsubscribeOneClick).toHaveBeenCalledWith(
        expect.objectContaining({ lids: ['vipcustomers'] })
      )
      expect(await screen.findByText(/unsubscribed successfully/i)).toBeInTheDocument()
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

    it("keeps the prompt when the list's own Unsubscribe fails", async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockRejectedValueOnce(new Error('boom'))
      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      const rowToggle = screen
        .getAllByRole('button', { name: /unsubscribe/i })
        .find((button) => !prompt.contains(button))
      await user.click(rowToggle!)

      expect(await screen.findByText(/failed to unsubscribe/i)).toBeInTheDocument()
      expect(screen.getByTestId('confirm-unsubscribe')).toBeInTheDocument()
    })

    // A pending contact, or one not on the list, can choose the row's Subscribe instead:
    // the question about leaving the list is then answered
    it("dismisses the prompt when the visitor subscribes from the list's row instead", async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [{ id: 'newsletter', name: 'Newsletter' }],
        contact_lists: [],
      })
      render(<App />)

      await screen.findByTestId('confirm-unsubscribe')
      await user.click(screen.getByRole('button', { name: /^subscribe$/i }))

      await waitFor(() => {
        expect(screen.queryByTestId('confirm-unsubscribe')).not.toBeInTheDocument()
      })
      expect(notificationCenterApi.subscribeToLists).toHaveBeenCalledTimes(1)
      expect(notificationCenterApi.unsubscribeOneClick).not.toHaveBeenCalled()
    })
  })

  // The workspace decides who joins a private list: the server refuses a subscribe, so
  // the page must not offer one
  describe('Private lists', () => {
    it('lets the contact leave a private list and offers no way back in', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValue({ success: true })
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(testData.validParams)
      mockURLSearchParams(testData.validParams)
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'vip',
            list_name: 'VIP',
            status: 'active',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      render(<App />)

      await user.click(await screen.findByRole('button', { name: /^unsubscribe$/i }))

      await waitFor(() => {
        expect(notificationCenterApi.unsubscribeOneClick).toHaveBeenCalledWith(
          expect.objectContaining({ lids: ['vip'] })
        )
      })
      expect(screen.queryByRole('button', { name: /^subscribe$/i })).not.toBeInTheDocument()
      // Translated, not the raw "unsubscribed" status
      expect(screen.getByRole('button', { name: 'Unsubscribed' })).toBeDisabled()
      expect(notificationCenterApi.subscribeToLists).not.toHaveBeenCalled()
    })

    it('lets the contact undo a private list unsubscribe during the same session via the undo token', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValueOnce({
        success: true,
        unsubscribed_lists: [{ id: 'vip', name: 'VIP', resubscribe_token: 'tok-vip' }],
      })
      vi.mocked(notificationCenterApi.resubscribeToList).mockResolvedValueOnce({
        success: true,
        list: { id: 'vip', name: 'VIP' },
      })
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(testData.validParams)
      mockURLSearchParams(testData.validParams)
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'vip',
            list_name: 'VIP',
            status: 'active',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      render(<App />)

      await user.click(await screen.findByRole('button', { name: /^unsubscribe$/i }))
      const resubButton = await screen.findByRole('button', { name: /^resubscribe$/i })
      expect(resubButton).toBeEnabled()

      await user.click(resubButton)
      await waitFor(() => {
        expect(notificationCenterApi.resubscribeToList).toHaveBeenCalledWith({
          wid: testData.validParams.wid,
          email: testData.validParams.email,
          email_hmac: testData.validParams.email_hmac,
          lid: 'vip',
          token: 'tok-vip',
        })
      })
      expect(await screen.findByRole('button', { name: /^unsubscribe$/i })).toBeInTheDocument()
    })

    it('offers Resubscribe in the unsubscribe confirmation banner for a private list and restores it', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValueOnce({
        success: true,
        unsubscribed_lists: [{ id: 'vip', name: 'VIP Members', resubscribe_token: 'tok-vip-2' }],
      })
      vi.mocked(notificationCenterApi.resubscribeToList).mockResolvedValueOnce({
        success: true,
        list: { id: 'vip', name: 'VIP Members' },
      })
      // Private list absent from /preferences on load (e.g. only returned in unsubscribed_lists):
      // restoring adds it back to the subscriptions section.
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [],
        contact_lists: [],
      })
      const params = { ...testData.validParams, lid: 'vip', action: 'unsubscribe' }
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(params)
      mockURLSearchParams(params)

      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))

      expect(await screen.findByText(/you have been unsubscribed from vip members/i)).toBeInTheDocument()
      const undo = screen.getByTestId('undo-unsubscribe')
      expect(within(undo).getByText(/clicked by mistake/i)).toBeInTheDocument()

      await user.click(within(undo).getByRole('button', { name: /^resubscribe$/i }))

      await waitFor(() => {
        expect(notificationCenterApi.resubscribeToList).toHaveBeenCalledWith({
          wid: params.wid,
          email: params.email,
          email_hmac: params.email_hmac,
          lid: 'vip',
          token: 'tok-vip-2',
        })
      })
      expect(await screen.findByText(/you have been resubscribed to vip members/i)).toBeInTheDocument()
      expect(screen.queryByTestId('undo-unsubscribe')).not.toBeInTheDocument()
      // The restored private list is back in the subscriptions section
      expect(screen.getByText('VIP Members')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /^unsubscribe$/i })).toBeInTheDocument()
    })

    it('keeps the private list undo button usable after a transient network error so retrying succeeds', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValueOnce({
        success: true,
        unsubscribed_lists: [{ id: 'vip', name: 'VIP Members', resubscribe_token: 'tok-retry' }],
      })
      vi.mocked(notificationCenterApi.resubscribeToList)
        .mockRejectedValueOnce(new Error('Network error'))
        .mockResolvedValueOnce({ success: true, list: { id: 'vip', name: 'VIP Members' } })
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'vip',
            list_name: 'VIP Members',
            status: 'active',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      const params = { ...testData.validParams, lid: 'vip', action: 'unsubscribe' }
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(params)
      mockURLSearchParams(params)

      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))

      const undo = await screen.findByTestId('undo-unsubscribe')
      await user.click(within(undo).getByRole('button', { name: /^resubscribe$/i }))

      // Transient error: the token is NOT discarded so the contact can retry
      await waitFor(() => {
        expect(notificationCenterApi.resubscribeToList).toHaveBeenCalledTimes(1)
      })
      expect(screen.getByTestId('undo-unsubscribe')).toBeInTheDocument()

      await user.click(within(screen.getByTestId('undo-unsubscribe')).getByRole('button', { name: /^resubscribe$/i }))
      expect(await screen.findByText(/you have been resubscribed to vip members/i)).toBeInTheDocument()
      expect(notificationCenterApi.resubscribeToList).toHaveBeenCalledTimes(2)
    })

    it('drops the private list undo button when the server definitively rejects the token (400/409)', async () => {
      const user = userEvent.setup()
      const expiredErr = Object.assign(new Error('resubscribe token expired'), { status: 400 })
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValueOnce({
        success: true,
        unsubscribed_lists: [{ id: 'vip', name: 'VIP Members', resubscribe_token: 'tok-expired' }],
      })
      vi.mocked(notificationCenterApi.resubscribeToList).mockRejectedValueOnce(expiredErr)
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'vip',
            list_name: 'VIP Members',
            status: 'active',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      const params = { ...testData.validParams, lid: 'vip', action: 'unsubscribe' }
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(params)
      mockURLSearchParams(params)

      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))

      const undo = await screen.findByTestId('undo-unsubscribe')
      await user.click(within(undo).getByRole('button', { name: /^resubscribe$/i }))

      await waitFor(() => {
        expect(screen.queryByTestId('undo-unsubscribe')).not.toBeInTheDocument()
      })
    })

    it('offers no undo button and discloses no private list name when unsubscribe emitted no token', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValueOnce({
        success: true,
      })
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [],
        contact_lists: [],
      })
      const params = { ...testData.validParams, lid: 'secretvip', action: 'unsubscribe' }
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(params)
      mockURLSearchParams(params)

      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      await user.click(within(prompt).getByRole('button', { name: /^unsubscribe$/i }))

      expect(await screen.findByText(/you have been unsubscribed successfully/i)).toBeInTheDocument()
      expect(screen.queryByText(/secretvip/i)).not.toBeInTheDocument()
      expect(screen.queryByTestId('undo-unsubscribe')).not.toBeInTheDocument()
    })

    it('falls back to /subscribe on a public list when the undo token expired (400)', async () => {
      const user = userEvent.setup()
      const expiredErr = Object.assign(new Error('resubscribe token expired'), { status: 400 })
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValueOnce({
        success: true,
        unsubscribed_lists: [{ id: 'newsletter', name: 'Newsletter', resubscribe_token: 'tok-expired' }],
      })
      vi.mocked(notificationCenterApi.resubscribeToList).mockRejectedValueOnce(expiredErr)
      vi.mocked(notificationCenterApi.subscribeToLists).mockResolvedValueOnce({ success: true })
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [{ id: 'newsletter', name: 'Newsletter', description: 'Weekly newsletter' }],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'newsletter',
            list_name: 'Newsletter',
            status: 'active',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      const params = { ...testData.validParams, lid: 'newsletter', action: 'unsubscribe' }
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(params)
      mockURLSearchParams(params)

      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))

      const undo = await screen.findByTestId('undo-unsubscribe')
      await user.click(within(undo).getByRole('button', { name: /^resubscribe$/i }))

      await waitFor(() => {
        expect(notificationCenterApi.subscribeToLists).toHaveBeenCalledWith(
          expect.objectContaining({
            workspace_id: params.wid,
            list_ids: ['newsletter'],
          })
        )
      })
      expect(await screen.findByText(/you have been resubscribed to newsletter/i)).toBeInTheDocument()
      expect(screen.queryByTestId('undo-unsubscribe')).not.toBeInTheDocument()
    })

    it('supports confirm_unsubscribe followed by banner undo on a private list, and clears the banner on manual unsubscribe', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.unsubscribeOneClick)
        .mockResolvedValueOnce({
          success: true,
          unsubscribed_lists: [{ id: 'vip', name: 'VIP Members', resubscribe_token: 'tok-confirm' }],
        })
        .mockResolvedValueOnce({
          success: true,
          unsubscribed_lists: [{ id: 'vip', name: 'VIP Members', resubscribe_token: 'tok-manual' }],
        })
      vi.mocked(notificationCenterApi.resubscribeToList).mockResolvedValueOnce({
        success: true,
        list: { id: 'vip', name: 'VIP Members' },
      })
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'vip',
            list_name: 'VIP Members',
            status: 'active',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      const params = { ...testData.validParams, lid: 'vip', action: 'confirm_unsubscribe' }
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(params)
      mockURLSearchParams(params)

      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))

      const undo = await screen.findByTestId('undo-unsubscribe')
      await user.click(within(undo).getByRole('button', { name: /^resubscribe$/i }))

      const resubText = await screen.findByText(/you have been resubscribed to vip members/i)
      expect(resubText.closest('[role="status"]')).toHaveFocus()

      // Manual unsubscribe clears the stale resubscribed banner and offers a row Resubscribe button
      await user.click(screen.getByRole('button', { name: /^unsubscribe$/i }))
      await waitFor(() => {
        expect(screen.queryByText(/you have been resubscribed to vip members/i)).not.toBeInTheDocument()
      })
      expect(await screen.findByRole('button', { name: /^resubscribe$/i })).toBeEnabled()
    })

    it('never sends stale tracking consent fields when falling back to /subscribe after token expiration', async () => {
      const user = userEvent.setup()
      const expiredErr = Object.assign(new Error('resubscribe token expired'), { status: 400 })
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValueOnce({
        success: true,
        unsubscribed_lists: [{ id: 'newsletter', name: 'Newsletter', resubscribe_token: 'tok-expired' }],
      })
      vi.mocked(notificationCenterApi.resubscribeToList).mockRejectedValueOnce(expiredErr)
      vi.mocked(notificationCenterApi.subscribeToLists).mockResolvedValueOnce({ success: true })
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: {
          ...testData.contact,
          tracking_opens_consent: 'granted',
          tracking_clicks_consent: 'granted',
        },
        public_lists: [{ id: 'newsletter', name: 'Newsletter', description: 'Weekly newsletter' }],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'newsletter',
            list_name: 'Newsletter',
            status: 'active',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      const params = { ...testData.validParams, lid: 'newsletter', action: 'confirm_unsubscribe' }
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(params)
      mockURLSearchParams(params)

      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))
      const undo = await screen.findByTestId('undo-unsubscribe')
      await user.click(within(undo).getByRole('button', { name: /^resubscribe$/i }))

      await waitFor(() => {
        expect(notificationCenterApi.subscribeToLists).toHaveBeenCalledWith({
          workspace_id: params.wid,
          contact: {
            id: '',
            email: params.email,
            email_hmac: params.email_hmac,
          },
          list_ids: ['newsletter'],
        })
      })
    })

    it('does not fall back to /subscribe on a public list when /resubscribe returns 409 (e.g. bounced/complained/removed)', async () => {
      const user = userEvent.setup()
      const conflictErr = Object.assign(new Error('resubscribe not allowed'), { status: 409 })
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValueOnce({
        success: true,
        unsubscribed_lists: [{ id: 'newsletter', name: 'Newsletter', resubscribe_token: 'tok-conflict' }],
      })
      vi.mocked(notificationCenterApi.resubscribeToList).mockRejectedValueOnce(conflictErr)
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [{ id: 'newsletter', name: 'Newsletter' }],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'newsletter',
            list_name: 'Newsletter',
            status: 'active',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      const params = { ...testData.validParams, lid: 'newsletter', action: 'unsubscribe' }
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(params)
      mockURLSearchParams(params)

      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))

      const undo = await screen.findByTestId('undo-unsubscribe')
      await user.click(within(undo).getByRole('button', { name: /^resubscribe$/i }))

      await waitFor(() => {
        expect(notificationCenterApi.resubscribeToList).toHaveBeenCalledTimes(1)
      })
      expect(notificationCenterApi.subscribeToLists).not.toHaveBeenCalled()
      expect(screen.queryByText(/you have been resubscribed/i)).not.toBeInTheDocument()
      expect(screen.queryByTestId('undo-unsubscribe')).not.toBeInTheDocument()
    })

    it('preserves a private list undo banner when the visitor toggles another list', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.unsubscribeOneClick)
        .mockResolvedValueOnce({
          success: true,
          unsubscribed_lists: [{ id: 'vip', name: 'VIP Members', resubscribe_token: 'tok-vip-keep' }],
        })
        .mockResolvedValueOnce({
          success: true,
          unsubscribed_lists: [{ id: 'newsletter', name: 'Newsletter', resubscribe_token: 'tok-nl' }],
        })
      vi.mocked(notificationCenterApi.resubscribeToList).mockResolvedValueOnce({
        success: true,
        list: { id: 'vip', name: 'VIP Members' },
      })
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [{ id: 'newsletter', name: 'Newsletter' }],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'vip',
            list_name: 'VIP Members',
            status: 'active',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
          {
            email: testData.contact.email,
            list_id: 'newsletter',
            list_name: 'Newsletter',
            status: 'active',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      const params = { ...testData.validParams, lid: 'vip', action: 'unsubscribe' }
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(params)
      mockURLSearchParams(params)

      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))
      expect(await screen.findByTestId('undo-unsubscribe')).toBeInTheDocument()

      // Toggle a DIFFERENT list (Newsletter): VIP Members undo banner must remain usable
      await user.click(screen.getByRole('button', { name: /^unsubscribe$/i }))
      await waitFor(() => {
        expect(notificationCenterApi.unsubscribeOneClick).toHaveBeenCalledTimes(2)
      })
      const undo = screen.getByTestId('undo-unsubscribe')
      expect(undo).toBeInTheDocument()

      await user.click(within(undo).getByRole('button', { name: /^resubscribe$/i }))
      expect(await screen.findByText(/you have been resubscribed to vip members/i)).toBeInTheDocument()
    })

    it('clears the banner undo button too when a 409 rejection happens from the private list row', async () => {
      const user = userEvent.setup()
      const conflictErr = Object.assign(new Error('resubscribe not allowed'), { status: 409 })
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValueOnce({
        success: true,
        unsubscribed_lists: [{ id: 'vip', name: 'VIP Members', resubscribe_token: 'tok-vip-409' }],
      })
      vi.mocked(notificationCenterApi.resubscribeToList).mockRejectedValueOnce(conflictErr)
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'vip',
            list_name: 'VIP Members',
            status: 'active',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      const params = { ...testData.validParams, lid: 'vip', action: 'confirm_unsubscribe' }
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(params)
      mockURLSearchParams(params)

      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))
      expect(await screen.findByTestId('undo-unsubscribe')).toBeInTheDocument()

      // Click Resubscribe on the list ROW (not the banner)
      const rowResub = screen
        .getAllByRole('button', { name: /^resubscribe$/i })
        .find((btn) => !screen.getByTestId('undo-unsubscribe').contains(btn))
      expect(rowResub).toBeDefined()
      await user.click(rowResub!)

      await waitFor(() => {
        expect(screen.queryByTestId('undo-unsubscribe')).not.toBeInTheDocument()
      })
      expect(screen.getByRole('button', { name: 'Unsubscribed' })).toBeDisabled()
    })

    it('uses /resubscribe from the row on a public list when an undo token exists and keeps it on transient failure', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValueOnce({
        success: true,
        unsubscribed_lists: [{ id: 'newsletter', name: 'Newsletter', resubscribe_token: 'tok-pub-row' }],
      })
      vi.mocked(notificationCenterApi.resubscribeToList)
        .mockRejectedValueOnce(new Error('Network error'))
        .mockResolvedValueOnce({ success: true, list: { id: 'newsletter', name: 'Newsletter' } })
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [{ id: 'newsletter', name: 'Newsletter' }],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'newsletter',
            list_name: 'Newsletter',
            status: 'active',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(testData.validParams)
      mockURLSearchParams(testData.validParams)

      render(<App />)

      await user.click(await screen.findByRole('button', { name: /^unsubscribe$/i }))
      const subBtn = await screen.findByRole('button', { name: /^subscribe$/i })
      await user.click(subBtn)

      await waitFor(() => {
        expect(notificationCenterApi.resubscribeToList).toHaveBeenCalledTimes(1)
      })
      expect(notificationCenterApi.subscribeToLists).not.toHaveBeenCalled()

      // Retry succeeds using the preserved undo token
      await user.click(screen.getByRole('button', { name: /^subscribe$/i }))
      await waitFor(() => {
        expect(notificationCenterApi.resubscribeToList).toHaveBeenCalledTimes(2)
      })
      expect(await screen.findByRole('button', { name: /^unsubscribe$/i })).toBeInTheDocument()
    })

    it('clears the banner undo button on 400 invalid token on a public list so clicking again cannot bypass /resubscribe', async () => {
      const user = userEvent.setup()
      const invalidErr = Object.assign(new Error('invalid resubscribe token'), { status: 400 })
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValueOnce({
        success: true,
        unsubscribed_lists: [{ id: 'newsletter', name: 'Newsletter', resubscribe_token: 'tok-invalid' }],
      })
      vi.mocked(notificationCenterApi.resubscribeToList).mockRejectedValueOnce(invalidErr)
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce({
        contact: testData.contact,
        public_lists: [{ id: 'newsletter', name: 'Newsletter' }],
        contact_lists: [
          {
            email: testData.contact.email,
            list_id: 'newsletter',
            list_name: 'Newsletter',
            status: 'active',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
      })
      const params = { ...testData.validParams, lid: 'newsletter', action: 'confirm_unsubscribe' }
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(params)
      mockURLSearchParams(params)

      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))
      expect(await screen.findByTestId('undo-unsubscribe')).toBeInTheDocument()

      // Click the row button (which calls /resubscribe because an undo token exists) -> 400 invalid token
      const rowSub = screen.getByRole('button', { name: /^subscribe$/i })
      await user.click(rowSub)

      await waitFor(() => {
        expect(notificationCenterApi.resubscribeToList).toHaveBeenCalledTimes(1)
      })
      expect(notificationCenterApi.subscribeToLists).not.toHaveBeenCalled()
      expect(screen.queryByTestId('undo-unsubscribe')).not.toBeInTheDocument()
    })

    it('does not claim resubscribe success on expired-token fallback when /preferences reveals the contact is now bounced or complained', async () => {
      const user = userEvent.setup()
      const expiredErr = Object.assign(new Error('resubscribe token expired'), { status: 400 })
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValueOnce({
        success: true,
        unsubscribed_lists: [{ id: 'newsletter', name: 'Newsletter', resubscribe_token: 'tok-expired' }],
      })
      vi.mocked(notificationCenterApi.resubscribeToList).mockRejectedValueOnce(expiredErr)
      vi.mocked(notificationCenterApi.subscribeToLists).mockResolvedValueOnce({ success: true })
      vi.mocked(notificationCenterApi.getContactPreferences)
        .mockResolvedValueOnce({
          contact: testData.contact,
          public_lists: [{ id: 'newsletter', name: 'Newsletter' }],
          contact_lists: [
            {
              email: testData.contact.email,
              list_id: 'newsletter',
              list_name: 'Newsletter',
              status: 'active',
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
          ],
        })
        // After /subscribe returns {success:true}, /preferences shows the bounce kept by the server
        .mockResolvedValueOnce({
          contact: testData.contact,
          public_lists: [{ id: 'newsletter', name: 'Newsletter' }],
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

      const params = { ...testData.validParams, lid: 'newsletter', action: 'unsubscribe' }
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(params)
      mockURLSearchParams(params)

      render(<App />)

      const prompt = await screen.findByTestId('confirm-unsubscribe')
      await user.click(within(prompt).getByRole('button', { name: /unsubscribe/i }))

      const undo = await screen.findByTestId('undo-unsubscribe')
      await user.click(within(undo).getByRole('button', { name: /^resubscribe$/i }))

      await waitFor(() => {
        expect(notificationCenterApi.subscribeToLists).toHaveBeenCalledTimes(1)
      })
      expect(screen.queryByText(/you have been resubscribed/i)).not.toBeInTheDocument()
      expect(screen.queryByTestId('undo-unsubscribe')).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'bounced' })).toBeDisabled()
    })
  })

  // The double opt-in confirmation link. A security scanner that opens it must not
  // confirm the subscription in the contact's place: the visitor clicks.
  describe('Subscription confirmation link (action=confirm)', () => {
    const confirmParams = { ...testData.validParams, action: 'confirm' }
    const contactRow = (listId: string, listName: string, status: string) => ({
      email: testData.contact.email,
      list_id: listId,
      list_name: listName,
      status,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
    })
    const preferences = (
      publicLists: Array<{ id: string; name: string }>,
      contactLists: Array<ReturnType<typeof contactRow>>
    ) => ({ contact: testData.contact, public_lists: publicLists, contact_lists: contactLists })
    const newsletter = { id: 'newsletter', name: 'Newsletter' }

    beforeEach(() => {
      // An earlier test may leave subscribeToLists rejecting; clearAllMocks keeps that
      vi.mocked(notificationCenterApi.subscribeToLists).mockResolvedValue({ success: true })
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(confirmParams)
      mockURLSearchParams(confirmParams)
    })

    it('asks to confirm a pending subscription without subscribing on load', async () => {
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
        preferences([newsletter], [contactRow('newsletter', 'Newsletter', 'pending')])
      )
      // Fake timers so the assertion also covers work deferred past the render that shows
      // the card
      vi.useFakeTimers({ shouldAdvanceTime: true })
      try {
        render(<App />)

        const prompt = await screen.findByTestId('confirm-subscription')
        expect(within(prompt).getByText('Newsletter')).toBeInTheDocument()
        expect(within(prompt).getByText(/receive emails from this list/i)).toBeInTheDocument()
        expect(
          within(prompt).getByRole('button', { name: 'Confirm subscription: Newsletter' })
        ).toBeInTheDocument()

        await act(() => vi.runAllTimersAsync())
        expect(notificationCenterApi.subscribeToLists).not.toHaveBeenCalled()
        expect(notificationCenterApi.unsubscribeOneClick).not.toHaveBeenCalled()
        expect(notificationCenterApi.updateContactPreferences).not.toHaveBeenCalled()
        expect(screen.queryByText(/subscription confirmed/i)).not.toBeInTheDocument()
      } finally {
        vi.useRealTimers()
      }
    })

    it('confirms when the visitor clicks, then shows the list as subscribed', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences)
        .mockResolvedValueOnce(preferences([newsletter], [contactRow('newsletter', 'Newsletter', 'pending')]))
        .mockResolvedValueOnce(preferences([newsletter], [contactRow('newsletter', 'Newsletter', 'active')]))
      render(<App />)

      const prompt = await screen.findByTestId('confirm-subscription')
      await user.click(within(prompt).getByRole('button', { name: /confirm subscription/i }))

      expect(notificationCenterApi.subscribeToLists).toHaveBeenCalledTimes(1)
      expect(notificationCenterApi.subscribeToLists).toHaveBeenCalledWith({
        workspace_id: testData.validParams.wid,
        contact: {
          id: '',
          email: testData.validParams.email,
          email_hmac: testData.validParams.email_hmac,
        },
        list_ids: [testData.validParams.lid],
      })
      const banner = (await screen.findByText(/subscription confirmed successfully/i)).closest(
        '[role="status"]'
      )
      expect(banner).not.toBeNull()
      expect(banner).toHaveFocus()
      expect(screen.queryByTestId('confirm-subscription')).not.toBeInTheDocument()
      // The list's own row reflects the confirmation
      expect(screen.getByRole('button', { name: /^unsubscribe$/i })).toBeInTheDocument()
    })

    // The server answers success when it leaves the contact as they were, for example a
    // bounce kept on a list they were removed from, which the preferences don't show
    it('says the confirmation failed when the preferences show no subscription after it', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences)
        .mockResolvedValueOnce(preferences([newsletter], []))
        .mockResolvedValueOnce(preferences([newsletter], []))
      render(<App />)

      const prompt = await screen.findByTestId('confirm-subscription')
      await user.click(within(prompt).getByRole('button', { name: /confirm subscription/i }))

      const alert = (await screen.findByText(/failed to confirm subscription/i)).closest('[role="alert"]')
      expect(alert).not.toBeNull()
      expect(alert).toHaveFocus()
      expect(screen.queryByTestId('confirm-subscription')).not.toBeInTheDocument()
      expect(screen.queryByText(/subscription confirmed successfully/i)).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /^subscribe$/i })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /^unsubscribe$/i })).not.toBeInTheDocument()
    })

    it('says the confirmation failed when the server answers that it was not', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
        preferences([newsletter], [contactRow('newsletter', 'Newsletter', 'pending')])
      )
      vi.mocked(notificationCenterApi.subscribeToLists).mockResolvedValueOnce({ success: false })
      render(<App />)

      const prompt = await screen.findByTestId('confirm-subscription')
      await user.click(within(prompt).getByRole('button', { name: /confirm subscription/i }))

      expect(await screen.findByText(/failed to confirm subscription/i)).toBeInTheDocument()
      expect(screen.getByTestId('confirm-subscription')).toBeInTheDocument()
      expect(notificationCenterApi.getContactPreferences).toHaveBeenCalledTimes(1)
    })

    // The server accepted the confirmation; the preferences only could not be read back
    it("trusts the server's answer when the preferences cannot be read again", async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences)
        .mockResolvedValueOnce(preferences([newsletter], [contactRow('newsletter', 'Newsletter', 'pending')]))
        .mockRejectedValueOnce(new Error('network'))
      render(<App />)

      const prompt = await screen.findByTestId('confirm-subscription')
      await user.click(within(prompt).getByRole('button', { name: /confirm subscription/i }))

      expect(await screen.findByText(/subscription confirmed successfully/i)).toBeInTheDocument()
      expect(screen.queryByTestId('confirm-subscription')).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /^unsubscribe$/i })).toBeInTheDocument()
    })

    it('asks nothing and writes nothing without a list in the link', async () => {
      const withoutLid = {
        wid: testData.validParams.wid,
        email: testData.validParams.email,
        email_hmac: testData.validParams.email_hmac,
        action: 'confirm',
      }
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue(withoutLid)
      mockURLSearchParams(withoutLid)
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
        preferences([newsletter], [contactRow('newsletter', 'Newsletter', 'pending')])
      )
      render(<App />)

      expect(await screen.findByText('Newsletter')).toBeInTheDocument()
      expect(screen.queryByTestId('confirm-subscription')).not.toBeInTheDocument()
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
      expect(notificationCenterApi.subscribeToLists).not.toHaveBeenCalled()
    })

    it('keeps the prompt and shows an error when the confirmation fails', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
        preferences([newsletter], [contactRow('newsletter', 'Newsletter', 'pending')])
      )
      vi.mocked(notificationCenterApi.subscribeToLists).mockRejectedValueOnce(new Error('boom'))
      render(<App />)

      const prompt = await screen.findByTestId('confirm-subscription')
      await user.click(within(prompt).getByRole('button', { name: /confirm subscription/i }))

      expect(await screen.findByText(/failed to confirm subscription/i)).toBeInTheDocument()
      expect(screen.getByTestId('confirm-subscription')).toBeInTheDocument()
      expect(notificationCenterApi.getContactPreferences).toHaveBeenCalledTimes(1)
    })

    it.each([
      ['is not on the public list', [] as Array<ReturnType<typeof contactRow>>],
      ['left the public list', [contactRow('newsletter', 'Newsletter', 'unsubscribed')]],
    ])('asks a contact who %s', async (_case, rows) => {
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
        preferences([newsletter], rows)
      )
      render(<App />)

      const prompt = await screen.findByTestId('confirm-subscription')
      expect(within(prompt).getByText('Newsletter')).toBeInTheDocument()
      expect(notificationCenterApi.subscribeToLists).not.toHaveBeenCalled()
    })

    it('tells a subscribed contact, without asking or writing', async () => {
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
        preferences([newsletter], [contactRow('newsletter', 'Newsletter', 'active')])
      )
      render(<App />)

      expect(await screen.findByText(/already subscribed to this list/i)).toBeInTheDocument()
      expect(screen.queryByTestId('confirm-subscription')).not.toBeInTheDocument()
      expect(notificationCenterApi.subscribeToLists).not.toHaveBeenCalled()
    })

    it.each([
      ['bounced on the public list', [newsletter], [contactRow('newsletter', 'Newsletter', 'bounced')]],
      ['complained on the public list', [newsletter], [contactRow('newsletter', 'Newsletter', 'complained')]],
      ['not on a private list', [], []],
      ['unsubscribed from a private list', [], [contactRow('newsletter', 'Newsletter', 'unsubscribed')]],
    ])('says it cannot confirm, and writes nothing, for a contact %s', async (_case, publicLists, rows) => {
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
        preferences(publicLists, rows)
      )
      render(<App />)

      expect(await screen.findByText(/failed to confirm subscription/i)).toBeInTheDocument()
      expect(screen.queryByTestId('confirm-subscription')).not.toBeInTheDocument()
      expect(notificationCenterApi.subscribeToLists).not.toHaveBeenCalled()
    })

    // The workspace started this double opt-in, so its contact may confirm it, though the
    // private list's own row offers no Subscribe button
    it('confirms a pending private list, whose row then offers Unsubscribe', async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.parseNotificationCenterParams).mockReturnValue({
        ...confirmParams,
        lid: 'vip',
      })
      vi.mocked(notificationCenterApi.getContactPreferences)
        .mockResolvedValueOnce(preferences([], [contactRow('vip', 'VIP', 'pending')]))
        .mockResolvedValueOnce(preferences([], [contactRow('vip', 'VIP', 'active')]))
      render(<App />)

      const prompt = await screen.findByTestId('confirm-subscription')
      expect(within(prompt).getByText('VIP')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'pending' })).toBeDisabled()

      await user.click(within(prompt).getByRole('button', { name: /confirm subscription/i }))

      expect(await screen.findByText(/subscription confirmed successfully/i)).toBeInTheDocument()
      expect(notificationCenterApi.subscribeToLists).toHaveBeenCalledWith(
        expect.objectContaining({ list_ids: ['vip'] })
      )
      expect(screen.getByRole('button', { name: /^unsubscribe$/i })).toBeEnabled()
    })

    it("dismisses the prompt once the list's own Subscribe button went through", async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
        preferences([newsletter], [contactRow('newsletter', 'Newsletter', 'pending')])
      )
      render(<App />)

      const prompt = await screen.findByTestId('confirm-subscription')
      const rowButton = screen
        .getAllByRole('button', { name: /subscri/i })
        .find((button) => !prompt.contains(button))
      expect(rowButton).toBeDefined()
      await user.click(rowButton!)

      await waitFor(() => {
        expect(screen.queryByTestId('confirm-subscription')).not.toBeInTheDocument()
      })
      expect(notificationCenterApi.subscribeToLists).toHaveBeenCalledTimes(1)
    })

    // A contact removed from the list after a bounce: the server keeps the bounce and
    // still answers success, and the preferences show no subscription
    it("says so when the row's Subscribe went through but left the contact out", async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences)
        .mockResolvedValueOnce(preferences([newsletter], []))
        .mockResolvedValueOnce(preferences([newsletter], []))
      render(<App />)

      const prompt = await screen.findByTestId('confirm-subscription')
      const rowButton = screen
        .getAllByRole('button', { name: /^subscribe$/i })
        .find((button) => !prompt.contains(button))
      await user.click(rowButton!)

      expect(await screen.findByText(/failed to subscribe/i)).toBeInTheDocument()
      expect(screen.queryByText(/successfully subscribed/i)).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /^subscribe$/i })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /^unsubscribe$/i })).not.toBeInTheDocument()
    })

    it("keeps the prompt and the pending status when the row's Subscribe fails", async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
        preferences([newsletter], [contactRow('newsletter', 'Newsletter', 'pending')])
      )
      vi.mocked(notificationCenterApi.subscribeToLists).mockRejectedValueOnce(new Error('boom'))
      render(<App />)

      const prompt = await screen.findByTestId('confirm-subscription')
      const rowButton = screen
        .getAllByRole('button', { name: /subscri/i })
        .find((button) => !prompt.contains(button))
      await user.click(rowButton!)

      await waitFor(() => {
        expect(notificationCenterApi.subscribeToLists).toHaveBeenCalledTimes(1)
      })
      expect(await screen.findByText('pending')).toBeInTheDocument()
      expect(screen.getByTestId('confirm-subscription')).toBeInTheDocument()
    })

    it("clears the 'already subscribed' message when the visitor unsubscribes from the row", async () => {
      const user = userEvent.setup()
      vi.mocked(notificationCenterApi.unsubscribeOneClick).mockResolvedValue({ success: true })
      vi.mocked(notificationCenterApi.getContactPreferences).mockResolvedValueOnce(
        preferences([newsletter], [contactRow('newsletter', 'Newsletter', 'active')])
      )
      render(<App />)

      await screen.findByText(/already subscribed to this list/i)
      await user.click(screen.getByRole('button', { name: /^unsubscribe$/i }))

      await waitFor(() => {
        expect(screen.queryByText(/already subscribed to this list/i)).not.toBeInTheDocument()
      })
    })

    it('writes nothing for a contact the workspace erased', async () => {
      vi.mocked(notificationCenterApi.getContactPreferences).mockRejectedValueOnce(
        new Error('Contact not found')
      )
      render(<App />)

      expect(await screen.findByText('Contact not found')).toBeInTheDocument()
      expect(notificationCenterApi.subscribeToLists).not.toHaveBeenCalled()
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

