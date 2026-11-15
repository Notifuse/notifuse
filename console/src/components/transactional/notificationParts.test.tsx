import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { TransactionalNotification } from '../../services/api/transactional_notifications'
import type { Workspace } from '../../services/api/types'
import { DeletedTemplatePill, IntegrationIcon, MenuEntryLabel } from './notificationParts'

describe('MenuEntryLabel', () => {
  it('writes the reason under the label, without hover', () => {
    const { container } = render(<MenuEntryLabel label="Delete" reason="You need Transactional write permission" />)
    expect(screen.getByText('Delete')).toBeInTheDocument()
    expect(screen.getByText('You need Transactional write permission')).toBeInTheDocument()
    expect(container.textContent).toBe('DeleteYou need Transactional write permission')
  })

  it('shows the label alone without a reason, or with an empty one', () => {
    const { container, rerender } = render(<MenuEntryLabel label="Delete" />)
    expect(container.innerHTML).toBe('Delete')
    rerender(<MenuEntryLabel label="Delete" reason="" />)
    // No second, blank line.
    expect(container.innerHTML).toBe('Delete')
  })
})

describe('DeletedTemplatePill', () => {
  const renderPill = (short?: boolean) => render(<DeletedTemplatePill short={short} />)

  it('says « Deleted template » and, for screen readers, that the notification still sends with it', () => {
    renderPill()
    expect(screen.getByText('Deleted template')).not.toHaveAttribute('aria-hidden')
    expect(screen.getByText('This notification still sends with it')).toHaveClass('sr-only')
  })

  it('says « Deleted » alone when short, and the whole sentence for screen readers', () => {
    renderPill(true)
    expect(screen.getByText('Deleted')).toHaveAttribute('aria-hidden', 'true')
    expect(screen.queryByText('Deleted template')).not.toBeInTheDocument()
    expect(screen.getByText('Deleted template. This notification still sends with it')).toHaveClass('sr-only')
  })
})

describe('IntegrationIcon', () => {
  const workspace = {
    id: 'ws1',
    integrations: [
      { id: 'supabase', name: 'Supabase', type: 'supabase' },
      { id: 'zap', name: 'Zap', type: 'zapier' }
    ]
  } as unknown as Workspace
  const managedBy = (integration_id?: string) => ({ id: 'n1', integration_id }) as TransactionalNotification

  it('shows nothing for a notification no integration manages', () => {
    const { container } = render(<IntegrationIcon notification={managedBy()} workspace={workspace} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the Supabase logo, named as the integration that manages it', () => {
    render(<IntegrationIcon notification={managedBy('supabase')} workspace={workspace} />)
    const icon = screen.getByRole('img', { name: 'Managed by Supabase (supabase integration)' })
    expect(icon.querySelector('img')).toHaveAttribute('src', '/console/supabase.png')
  })

  it('shows a generic icon for another integration, or one no longer in the workspace', () => {
    const { rerender } = render(<IntegrationIcon notification={managedBy('zap')} workspace={workspace} />)
    expect(screen.getByRole('img', { name: 'Managed by Zap (zapier integration)' }).querySelector('svg')).not.toBeNull()
    rerender(<IntegrationIcon notification={managedBy('gone')} workspace={workspace} />)
    expect(screen.getByRole('img', { name: 'Managed by an integration' }).querySelector('svg')).not.toBeNull()
  })
})
