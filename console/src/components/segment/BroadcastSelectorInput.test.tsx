import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const broadcasts = [
  { id: 'b1', name: 'Newsletter #1', test_settings: { variations: [{ template_id: 'weekly' }] } },
  { id: 'b2', name: 'A/B test', test_settings: { variations: [{ template_id: 'weekly' }, { template_id: 'promo' }] } },
  { id: 'b3', name: 'Win-back', test_settings: { variations: [{ template_id: 'winback' }] } }
]

vi.mock('../../services/api/broadcast', () => ({
  broadcastApi: {
    list: vi.fn(async () => ({ broadcasts, total_count: broadcasts.length })),
    get: vi.fn(async ({ id }: { id: string }) => ({ broadcast: broadcasts.find((b) => b.id === id) }))
  }
}))

import BroadcastSelectorInput from './BroadcastSelectorInput'

function renderSelector(props: { templateId?: string; value?: string; onChange?: (value: string | null) => void }) {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <BroadcastSelectorInput workspaceId="ws" {...props} />
    </QueryClientProvider>
  )
}

const shownOptions = () => [...document.querySelectorAll('.ant-select-item-option')].map((el) => el.textContent)

describe('BroadcastSelectorInput', () => {
  it('offers every broadcast without a template', async () => {
    renderSelector({})
    fireEvent.mouseDown(screen.getByRole('combobox'))
    await waitFor(() => expect(shownOptions()).toEqual(['Newsletter #1', 'A/B test', 'Win-back']))
  })

  it('offers only the broadcasts that sent the chosen template', async () => {
    renderSelector({ templateId: 'weekly' })
    fireEvent.mouseDown(screen.getByRole('combobox'))
    await waitFor(() => expect(shownOptions()).toEqual(['Newsletter #1', 'A/B test']))
  })

  it('drops a chosen broadcast that did not send the template chosen next', async () => {
    const onChange = vi.fn()
    renderSelector({ templateId: 'weekly', value: 'b3', onChange })
    await waitFor(() => expect(onChange).toHaveBeenCalledWith(null))
  })

  it('keeps a chosen broadcast that sent the template', async () => {
    const onChange = vi.fn()
    renderSelector({ templateId: 'promo', value: 'b2', onChange })
    expect(await screen.findByText('A/B test')).toBeInTheDocument()
    expect(onChange).not.toHaveBeenCalled()
  })
})
