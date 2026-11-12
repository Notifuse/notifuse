import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { defaultStringifySearch } from '@tanstack/react-router'
import { FailedMessagesTable } from './FailedMessagesTable'
import { listMessages } from '../../services/api/messages_history'
import type { Workspace } from '../../services/api/types'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useNavigate: () => navigate
}))

vi.mock('../../services/api/messages_history', () => ({
  listMessages: vi.fn()
}))

// The table pulls in the message drawers and their api clients; the card only needs a slot.
vi.mock('../messages/MessageHistoryTable', () => ({
  MessageHistoryTable: () => null
}))

i18n.loadAndActivate({ locale: 'en', messages: {} })

const workspace = { id: 'ws1', name: 'Test', settings: { timezone: 'UTC' } } as unknown as Workspace

describe('FailedMessagesTable', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(listMessages).mockResolvedValue({ messages: [], next_cursor: undefined, has_more: false })
  })

  // "View more" passed the string 'true', which the router writes JSON-quoted
  // (is_failed=%22true%22): the message history read it as not "true" and listed the
  // messages that had not failed.
  it('opens the message history filtered on failed messages', async () => {
    render(
      <I18nProvider i18n={i18n}>
        <FailedMessagesTable workspace={workspace} />
      </I18nProvider>
    )
    await waitFor(() => expect(listMessages).toHaveBeenCalled())

    fireEvent.click(screen.getByText('View more'))

    expect(navigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/logs',
      params: { workspaceId: 'ws1' },
      search: { tab: 'messages', is_failed: true }
    })
    const { search } = navigate.mock.calls[0][0] as { search: Record<string, unknown> }
    // What the message history reads back from the address with URLSearchParams.
    const written = new URLSearchParams(defaultStringifySearch(search))
    expect(written.get('is_failed')).toBe('true')
    expect(written.get('tab')).toBe('messages')
  })

  it('a string value would be written quoted, which the message history does not read as true', () => {
    expect(new URLSearchParams(defaultStringifySearch({ is_failed: 'true' })).get('is_failed')).toBe(
      '"true"'
    )
  })
})
