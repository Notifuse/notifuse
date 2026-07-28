import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { AddToListConfigForm } from './AddToListConfigForm'
import type { AddToListNodeConfig } from '../../../services/api/automation'

// antd's Select mounts a ResizeObserver; jsdom doesn't provide one.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

// The form only reads `lists` from the automation context.
vi.mock('../context', () => ({
  useAutomation: () => ({
    lists: [{ id: 'list123', name: 'My List' }]
  })
}))

const renderForm = (config: AddToListNodeConfig) =>
  render(
    <I18nProvider i18n={i18n}>
      <AddToListConfigForm config={config} onChange={vi.fn()} />
    </I18nProvider>
  )

describe('AddToListConfigForm', () => {
  it('defaults the subscription status to the canonical "active" value shown as "Subscribed"', () => {
    // With no status set the component falls back to 'active', which must map to
    // the "Subscribed" option. If the option value and the default disagree (the
    // original bug, where the option was 'subscribed'), antd renders no matching
    // label and this assertion fails.
    renderForm({ list_id: 'list123' } as AddToListNodeConfig)

    expect(screen.getByText('Subscribed')).toBeInTheDocument()
    // The invalid literal must never reach the rendered output.
    expect(screen.queryByText('subscribed')).not.toBeInTheDocument()
  })

  it('renders the "active" status with the "Subscribed" label', () => {
    renderForm({ list_id: 'list123', status: 'active' })
    expect(screen.getByText('Subscribed')).toBeInTheDocument()
  })

  it('renders the "pending" status with the "Pending" label', () => {
    renderForm({ list_id: 'list123', status: 'pending' })
    expect(screen.getByText('Pending')).toBeInTheDocument()
  })
})
