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

  it('shows why the server refused to delete a list that live automations use', async () => {
    // The 409 names the automations to pause; the generic "Failed to delete list" left the
    // user with no way to find out what was holding the list.
    const refusal =
      'cannot delete list "Newsletter" while the live automation "Welcome series" uses it: pause the automation first, or change it to stop using the list'
    deleteList.mockRejectedValue(new ApiError(refusal, 409, { error: refusal }))

    const { container } = renderPage()
    await deleteTheList(container)

    await waitFor(() => expect(message.error).toHaveBeenCalledWith(refusal))
    expect(deleteList).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'newsletter' })
  })

  it('keeps the generic message for any other failure', async () => {
    deleteList.mockRejectedValue(new ApiError('pq: connection reset', 500))

    const { container } = renderPage()
    await deleteTheList(container)

    await waitFor(() => expect(message.error).toHaveBeenCalledWith('Failed to delete list'))
    expect(message.error).not.toHaveBeenCalledWith('pq: connection reset')
  })
})
