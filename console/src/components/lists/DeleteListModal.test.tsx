import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App, message } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { ApiError } from '../../services/api/errors'
import type { List } from '../../services/api/types'

const deleteList = vi.fn()
vi.mock('../../services/api/list', () => ({
  listsApi: {
    delete: (...args: unknown[]) => deleteList(...args)
  }
}))

import { DeleteListModal } from './DeleteListModal'

const list: List = {
  id: 'newsletter',
  name: 'Newsletter',
  is_double_optin: false,
  is_public: false,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
}

let onDeleted = vi.fn()
let onClose = vi.fn()
let queryClient: QueryClient

const renderModal = () => {
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <App>
          <DeleteListModal workspaceId="ws1" list={list} open onClose={onClose} onDeleted={onDeleted} />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
}

// Confirms with the list's id and submits.
const deleteTheList = async () => {
  fireEvent.change(await screen.findByPlaceholderText('Enter list ID to confirm'), {
    target: { value: 'newsletter' }
  })
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
}

describe('DeleteListModal confirmation', () => {
  beforeEach(() => {
    deleteList.mockReset()
    onDeleted = vi.fn()
    onClose = vi.fn()
  })

  it('waits for the list ID before deleting', async () => {
    renderModal()
    expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled()
    fireEvent.change(screen.getByPlaceholderText('Enter list ID to confirm'), { target: { value: 'news' } })
    expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled()
    expect(screen.getByText("ID doesn't match")).toBeInTheDocument()
  })

  it('deletes the list, refreshes the lists and their counts, then hands over', async () => {
    deleteList.mockResolvedValue({})
    renderModal()
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    await deleteTheList()
    await waitFor(() => expect(onDeleted).toHaveBeenCalled())
    expect(deleteList).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'newsletter' })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['lists', 'ws1'] })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['lists-with-counts', 'ws1'] })
  })

  it('closes on cancel', () => {
    renderModal()
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onClose).toHaveBeenCalled()
  })
})

describe('DeleteListModal', () => {
  beforeEach(() => {
    deleteList.mockReset()
    vi.mocked(message.error).mockClear()
  })

  // The 409 lists the automations holding the list, which the generic "Failed to delete list"
  // left the user no way to find out. The console says it in its own, translated words, and
  // leaves it on screen long enough to read the names.
  const refusedWith = (automations: string[], count = automations.length) =>
    new ApiError('the server sentence', 409, {
      error: 'the server sentence',
      automations: automations.map((name, i) => ({ id: `auto-${i}`, name })),
      automations_count: count
    })

  it.each([
    {
      case: 'one automation, named',
      refusal: refusedWith(['Welcome series']),
      shown:
        'The list "Newsletter" can\'t be deleted while the live automation "Welcome series" uses it. Pause it first, or change it to stop using the list.'
    },
    {
      case: 'two automations, both named',
      refusal: refusedWith(['Welcome series', 'Win-back']),
      shown:
        'The list "Newsletter" can\'t be deleted while the live automations "Welcome series" and "Win-back" use it. Pause them first, or change them to stop using the list.'
    },
    {
      case: 'three automations, all named',
      refusal: refusedWith(['A', 'B', 'C']),
      shown:
        'The list "Newsletter" can\'t be deleted while the live automations "A", "B" and "C" use it. Pause them first, or change them to stop using the list.'
    },
    // Past three, the others are counted rather than named.
    {
      case: 'four automations, three named and one counted',
      refusal: refusedWith(['A', 'B', 'C', 'D']),
      shown:
        'The list "Newsletter" can\'t be deleted while the live automations "A", "B", "C" and 1 more use it. Pause them first, or change them to stop using the list.'
    },
    {
      case: 'five automations, three named and two counted',
      refusal: refusedWith(['A', 'B', 'C', 'D', 'E']),
      shown:
        'The list "Newsletter" can\'t be deleted while the live automations "A", "B", "C" and 2 more use it. Pause them first, or change them to stop using the list.'
    },
    // A member who may not read automations is told how many, and not which.
    {
      case: 'one automation, counted',
      refusal: refusedWith([], 1),
      shown:
        'The list "Newsletter" can\'t be deleted while a live automation uses it. Ask someone with access to automations to pause it.'
    },
    {
      case: 'several automations, counted',
      refusal: refusedWith([], 3),
      shown:
        'The list "Newsletter" can\'t be deleted while 3 live automations use it. Ask someone with access to automations to pause them.'
    }
  ])('explains a refusal while live automations use the list: $case', async ({ refusal, shown }) => {
    deleteList.mockRejectedValue(refusal)

    renderModal()
    await deleteTheList()

    await waitFor(() => expect(message.error).toHaveBeenCalledWith({ content: shown, duration: 10 }))
    expect(deleteList).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'newsletter' })
  })

  it("shows the server's sentence for a refusal that lists no automations", async () => {
    // A body without the automations: an older server, or another conflict.
    const refusal =
      'cannot delete list "Newsletter" while the live automation "Welcome series" uses it: pause the automation first, or change it to stop using the list'
    deleteList.mockRejectedValue(new ApiError(refusal, 409, { error: refusal }))

    renderModal()
    await deleteTheList()

    await waitFor(() => expect(message.error).toHaveBeenCalledWith({ content: refusal, duration: 10 }))
  })

  it('keeps the generic message for any other failure', async () => {
    deleteList.mockRejectedValue(new ApiError('pq: connection reset', 500))

    renderModal()
    await deleteTheList()

    await waitFor(() => expect(message.error).toHaveBeenCalledWith('Failed to delete list'))
    expect(message.error).not.toHaveBeenCalledWith('pq: connection reset')
  })
})
