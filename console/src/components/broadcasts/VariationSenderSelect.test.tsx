import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { VariationSenderSelect } from './VariationSenderSelect'
import { templatesApi } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'

vi.mock('../../services/api/template', () => ({
  templatesApi: { list: vi.fn() }
}))

const workspace = {
  id: 'ws1',
  settings: { marketing_email_provider_id: 'mkt' },
  integrations: [
    {
      id: 'mkt',
      email_provider: {
        senders: [
          { id: 's1', name: 'Default', email: 'hello@example.com', is_default: true },
          { id: 's2', name: 'Newsletter', email: 'news@example.com', is_default: false },
          { id: 's3', name: 'Promos', email: 'promos@example.com', is_default: false }
        ]
      }
    }
  ]
} as unknown as Workspace

const renderSelect = (props: {
  value?: string
  onChange?: (v?: string) => void
  templateId?: string
  ws?: Workspace
}) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <VariationSenderSelect
        workspace={props.ws ?? workspace}
        value={props.value}
        onChange={props.onChange}
        templateId={props.templateId}
      />
    </QueryClientProvider>
  )
}

describe('VariationSenderSelect', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(templatesApi.list as Mock).mockResolvedValue({
      templates: [{ id: 'tmpl1', email: { sender_id: 's2' } }]
    })
  })

  it("names the template's sender while none is chosen", async () => {
    renderSelect({ templateId: 'tmpl1' })
    expect(await screen.findByText("Template's sender (Newsletter)")).toBeInTheDocument()
  })

  it("offers the marketing integration's senders, tagged template's and default", async () => {
    renderSelect({ templateId: 'tmpl1' })
    await screen.findByText("Template's sender (Newsletter)")
    await userEvent.click(screen.getByRole('combobox'))

    expect(await screen.findByText('Promos <promos@example.com>')).toBeInTheDocument()
    expect(screen.getByText('Newsletter <news@example.com>').parentElement).toHaveTextContent(
      "template's"
    )
    expect(screen.getByText('Default <hello@example.com>').parentElement).toHaveTextContent(
      'default'
    )
  })

  it('reports the chosen sender', async () => {
    const onChange = vi.fn()
    renderSelect({ templateId: 'tmpl1', onChange })
    await userEvent.click(screen.getByRole('combobox'))
    await userEvent.click(await screen.findByText('Promos <promos@example.com>'))
    expect(onChange).toHaveBeenCalledWith('s3')
  })

  it('shows a chosen sender that has left the integration as deleted', () => {
    renderSelect({ value: 'gone' })
    expect(screen.getByText('gone (deleted)')).toBeInTheDocument()
  })

  it('is disabled without a marketing integration', async () => {
    renderSelect({ ws: { id: 'ws1', settings: {} } as unknown as Workspace })
    await waitFor(() => expect(screen.getByRole('combobox')).toBeDisabled())
  })
})
