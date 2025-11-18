import { vi } from 'vitest'
import type {
  ContactPreferencesResponse,
  SubscribeResponse,
  UnsubscribeResponse,
} from '@/api/notification_center'

export const mockContactPreferencesResponse: ContactPreferencesResponse = {
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
    {
      id: 'announcements',
      name: 'Announcements',
      description: 'Important announcements',
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
}

export const mockSubscribeResponse: SubscribeResponse = {
  success: true,
}

export const mockUnsubscribeResponse: UnsubscribeResponse = {
  success: true,
}

export const createMockApiModule = () => ({
  getContactPreferences: vi.fn().mockResolvedValue(mockContactPreferencesResponse),
  subscribeToLists: vi.fn().mockResolvedValue(mockSubscribeResponse),
  unsubscribeOneClick: vi.fn().mockResolvedValue(mockUnsubscribeResponse),
  parseNotificationCenterParams: vi.fn(),
})

