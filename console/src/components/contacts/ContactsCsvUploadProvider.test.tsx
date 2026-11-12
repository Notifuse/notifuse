import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const captured = vi.hoisted(() => ({ onSuccess: undefined as undefined | (() => void) }))

vi.mock('./ContactsCsvUploadDrawer', () => ({
  ContactsCsvUploadDrawer: ({ onSuccess }: { onSuccess?: () => void }) => {
    captured.onSuccess = onSuccess
    return <div>upload drawer</div>
  }
}))

import { ContactsCsvUploadProvider, useContactsCsvUpload } from './ContactsCsvUploadProvider'

function Opener() {
  const { openDrawerWithSelectedList } = useContactsCsvUpload()
  return (
    <button type="button" onClick={() => openDrawerWithSelectedList('ws', [], 'news')}>
      open
    </button>
  )
}

describe('ContactsCsvUploadProvider', () => {
  it('refreshes the contacts, the lists, their counts and the total after an import', async () => {
    const queryClient = new QueryClient()
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    render(
      <QueryClientProvider client={queryClient}>
        <ContactsCsvUploadProvider>
          <Opener />
        </ContactsCsvUploadProvider>
      </QueryClientProvider>
    )
    fireEvent.click(screen.getByRole('button', { name: 'open' }))
    expect(await screen.findByText('upload drawer')).toBeInTheDocument()
    act(() => captured.onSuccess?.())
    const keys = invalidate.mock.calls.map(([filters]) => filters?.queryKey)
    expect(keys).toEqual(
      expect.arrayContaining([
        ['contacts', 'ws'],
        ['lists', 'ws'],
        ['list-stats', 'ws'],
        ['lists-with-counts', 'ws'],
        ['total-contacts', 'ws']
      ])
    )
  })
})
