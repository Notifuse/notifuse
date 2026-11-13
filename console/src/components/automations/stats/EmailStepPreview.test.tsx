import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Workspace } from '../../../services/api/types'
import { templatesApi } from '../../../services/api/template'
import { EmailStepPreview } from './EmailStepPreview'

vi.mock('../../../services/api/template', () => ({ templatesApi: { get: vi.fn() } }))
vi.mock('../../broadcasts/detail/TemplatePreviewFrame', () => ({ TemplatePreviewFrame: () => <div data-testid="frame" /> }))
vi.mock('../../templates', () => ({ TemplatePreviewPopover: ({ children }: { children: ReactNode }) => <>{children}</> }))

const workspace = {
  id: 'ws1',
  settings: { marketing_email_provider_id: 'mkt' },
  integrations: [
    {
      id: 'mkt',
      email_provider: {
        kind: 'ses',
        senders: [
          { id: 's-default', name: 'Acme', email: 'hello@acme.io', is_default: true },
          { id: 's-news', name: 'Acme News', email: 'news@acme.io' }
        ]
      }
    },
    { id: 'other', email_provider: { kind: 'smtp', senders: [{ id: 'o1', name: 'Other', email: 'o@acme.io', is_default: true }] } }
  ]
} as unknown as Workspace

const template = (over: Record<string, unknown> = {}) => ({
  template: { id: 'tpl-1', name: 'Welcome & co', email: { subject: 'Hello there', sender_id: 's-news' }, ...over }
})

const renderPreview = (config: Record<string, unknown>, canReadTemplates = true) =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>
        <EmailStepPreview workspace={workspace} config={config} canReadTemplates={canReadTemplates} />
      </I18nProvider>
    </QueryClientProvider>
  )

beforeEach(() => vi.mocked(templatesApi.get).mockReset())

describe('EmailStepPreview', () => {
  it("shows the template's sender and subject, the render and the links", async () => {
    vi.mocked(templatesApi.get).mockResolvedValue(template() as never)
    renderPreview({ template_id: 'tpl-1' })
    expect(await screen.findByText('Acme News <news@acme.io>')).toBeInTheDocument()
    expect(screen.getByText('Hello there')).toBeInTheDocument()
    expect(screen.getByTestId('frame')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Preview/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Open template/ })).toHaveAttribute('href', '/console/workspace/ws1/templates?preview=tpl-1')
    expect(templatesApi.get).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'tpl-1' })
  })

  it("prefers the step's overrides and integration", async () => {
    vi.mocked(templatesApi.get).mockResolvedValue(template({ email: { subject: 'Hello there' } }) as never)
    const { unmount } = renderPreview({ template_id: 'tpl-1', integration_id: 'other' })
    expect(await screen.findByText('Other <o@acme.io>')).toBeInTheDocument()
    unmount()
    renderPreview({ template_id: 'tpl-1', from_override: 'Team <team@acme.io>', subject_override: 'Psst' })
    expect(await screen.findByText('Team <team@acme.io>')).toBeInTheDocument()
    expect(screen.getByText('Psst')).toBeInTheDocument()
  })

  it('says why there is no preview without Templates read, and reads nothing', () => {
    renderPreview({ template_id: 'tpl-1' }, false)
    expect(screen.getByText('You need access to templates to preview this email.')).toBeInTheDocument()
    expect(templatesApi.get).not.toHaveBeenCalled()
  })

  it('shows nothing without a template', () => {
    const { container } = renderPreview({})
    expect(container).toBeEmptyDOMElement()
  })
})
