import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { IntegrationCard, IntegrationIcon } from './IntegrationCard'

describe('IntegrationCard', () => {
  it('lays out its header, alert, body and footer', () => {
    render(
      <IntegrationCard
        icon={<span>ICON</span>}
        title="Office relay"
        subtitle="SMTP"
        status={<span>Connected</span>}
        actions={<button type="button">Edit</button>}
        alert={<span>Alert</span>}
        footer={<span>Senders</span>}
      >
        <span>Body</span>
      </IntegrationCard>
    )
    for (const text of ['ICON', 'Office relay', 'SMTP', 'Connected', 'Alert', 'Body', 'Senders']) {
      expect(screen.getByText(text)).toBeInTheDocument()
    }
    expect(screen.getByRole('button', { name: 'Edit' })).toBeInTheDocument()
  })

  it('shows the logo of a card without a frame around it', () => {
    render(<IntegrationCard icon={<img alt="Mailgun" src="/console/mailgun.svg" />} title="Mailgun" />)
    const tile = screen.getByAltText('Mailgun').parentElement
    expect(tile?.className).toContain('h-7')
    expect(tile?.className).not.toMatch(/\bborder\b/)
  })

  it('draws no body nor footer when there is none', () => {
    const { container } = render(<IntegrationCard icon={null} title="Bare" />)
    expect(container.querySelectorAll('[data-integration-card] > div')).toHaveLength(1)
  })
})

describe('IntegrationIcon', () => {
  it('puts a logo in the tile shared by the cards and the catalogue, without a frame', () => {
    render(
      <IntegrationIcon>
        <img alt="Mailgun" src="/console/mailgun.svg" />
      </IntegrationIcon>
    )
    expect(screen.getByAltText('Mailgun').parentElement?.className).toContain('h-7')
    expect(screen.getByAltText('Mailgun').parentElement?.className).not.toMatch(/\bborder\b/)
  })
})
