import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
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

      const paramsWithoutLid = { ...testData.validParams }
      delete (paramsWithoutLid as any).lid

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

