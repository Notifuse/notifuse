import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App, message } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { ApiError } from '../services/api/errors'

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'ws1' })
}))

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', name: 'Workspace' }] }),
  useWorkspacePermissions: () => ({ permissions: { lists: { read: true, write: true } } })
}))

// The icon is the only content of the delete button; render something a test can find.
vi.mock('@fortawesome/react-fontawesome', () => ({
  FontAwesomeIcon: ({ icon }: { icon: { iconName: string } }) => (
    <span data-icon={icon.iconName} />
  )
}))

// Keep the page render shallow: these children play no part in deleting a list.
vi.mock('../components/lists/ListDrawer', () => ({ CreateListDrawer: () => null }))
vi.mock('../components/lists/ImportContactsToListButton', () => ({
  ImportContactsToListButton: () => null
}))
vi.mock('../components/lists/ListStats', () => ({ ListStats: () => null }))
vi.mock('../components/templates/TemplatePreviewDrawer', () => ({ default: () => null }))
vi.mock('../components/templates/CreateTemplateDrawer', () => ({
  CreateTemplateDrawer: () => null
}))
vi.mock('../services/api/template', () => ({ templatesApi: { get: vi.fn() } }))

const deleteList = vi.fn()
vi.mock('../services/api/list', () => ({
  listsApi: {
    list: vi.fn().mockResolvedValue({
      lists: [
        {
          id: 'newsletter',
          name: 'Newsletter',
          is_double_optin: false,
          is_public: false,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z'
        }
      ]
    }),
    delete: (...args: unknown[]) => deleteList(...args)
  }
}))

import { ListsPage } from './ListsPage'

const renderPage = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <App>
          <ListsPage />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
}

// Opens the delete dialog of the one list, confirms with its id and submits.
const deleteTheList = async (container: HTMLElement) => {
  await screen.findByText('Newsletter')
  const trash = container.querySelector('[data-icon="trash-can"]')?.closest('button')
  expect(trash).toBeTruthy()
  fireEvent.click(trash as HTMLButtonElement)

  fireEvent.change(await screen.findByPlaceholderText('Enter list ID to confirm'), {
    target: { value: 'newsletter' }
  })
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
}

describe('ListsPage list deletion', () => {
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

    const { container } = renderPage()
    await deleteTheList(container)

    await waitFor(() => expect(message.error).toHaveBeenCalledWith({ content: shown, duration: 10 }))
    expect(deleteList).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'newsletter' })
  })

  it("shows the server's sentence for a refusal that lists no automations", async () => {
    // A body without the automations: an older server, or another conflict.
    const refusal =
      'cannot delete list "Newsletter" while the live automation "Welcome series" uses it: pause the automation first, or change it to stop using the list'
    deleteList.mockRejectedValue(new ApiError(refusal, 409, { error: refusal }))

    const { container } = renderPage()
    await deleteTheList(container)

    await waitFor(() =>
      expect(message.error).toHaveBeenCalledWith({ content: refusal, duration: 10 })
    )
  })

  it('keeps the generic message for any other failure', async () => {
    deleteList.mockRejectedValue(new ApiError('pq: connection reset', 500))

    const { container } = renderPage()
    await deleteTheList(container)

    await waitFor(() => expect(message.error).toHaveBeenCalledWith('Failed to delete list'))
    expect(message.error).not.toHaveBeenCalledWith('pq: connection reset')
  })
})
