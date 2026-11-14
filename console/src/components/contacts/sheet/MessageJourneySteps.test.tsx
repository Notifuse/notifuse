import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { MessageHistory } from '../../../services/api/messages_history'
import { MessageJourneySteps } from './MessageJourneySteps'

const base = {
  id: 'm1',
  sent_at: '2026-11-10T09:00:00Z',
  created_at: '2026-11-10T09:00:00Z'
}

function renderSteps(extra: Partial<MessageHistory>, smtp = false) {
  const { container } = render(
    <MessageJourneySteps message={{ ...base, ...extra } as MessageHistory} smtp={smtp} timezone="UTC" />
  )
  const lit = (key: string) => container.querySelector(`[data-step="${key}"]`)?.getAttribute('data-on')
  return { container, lit }
}

describe('MessageJourneySteps', () => {
  it('lights delivered and opened, leaves clicked off', () => {
    const { lit } = renderSteps({
      delivered_at: '2026-11-10T09:01:00Z',
      opened_at: '2026-11-10T10:00:00Z'
    })
    expect(lit('delivered')).toBe('true')
    expect(lit('opened')).toBe('true')
    expect(lit('clicked')).toBe('false')
  })

  it('reads a click as an open and a delivery, whatever was reported', () => {
    const { lit } = renderSteps({ clicked_at: '2026-11-10T10:00:00Z' })
    expect(lit('delivered')).toBe('true')
    expect(lit('opened')).toBe('true')
    expect(lit('clicked')).toBe('true')
  })

  it('shows Sent instead of Delivered for an SMTP provider that reported nothing', () => {
    const { lit } = renderSteps({}, true)
    expect(screen.getByText('Sent')).toBeInTheDocument()
    expect(screen.queryByText('Delivered')).toBeNull()
    expect(lit('sent')).toBe('true')
  })

  it('shows only the failure or the bounce', () => {
    const failed = renderSteps({
      failed_at: '2026-11-10T09:01:00Z',
      delivered_at: '2026-11-10T09:01:00Z'
    })
    expect(screen.getByText('Failed')).toBeInTheDocument()
    expect(failed.container.querySelector('[data-step]')).toBeNull()
    failed.container.remove()
    renderSteps({ bounced_at: '2026-11-10T09:01:00Z' })
    expect(screen.getByText('Bounced')).toBeInTheDocument()
  })

  it('follows the steps with a complaint and an unsubscribe', () => {
    renderSteps({
      delivered_at: '2026-11-10T09:01:00Z',
      complained_at: '2026-11-10T11:00:00Z',
      unsubscribed_at: '2026-11-10T11:00:00Z'
    })
    expect(screen.getByText('Marked as spam')).toBeInTheDocument()
    expect(screen.getByText('Unsubscribed')).toBeInTheDocument()
  })
})
