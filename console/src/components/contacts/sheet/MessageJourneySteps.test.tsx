import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
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

  it('says whether each step happened in words, not only in colour', () => {
    const { container } = renderSteps({ delivered_at: '2026-11-10T09:01:00Z', opens_tracked: false })
    const spoken = [...container.querySelectorAll('[data-step] .sr-only')].map((node) => node.textContent)
    expect(spoken).toEqual(['Delivered: Nov 10, 2026 9:01 AM', 'Not opened: Opens were not tracked', 'Not clicked'])
    container.querySelectorAll('[data-step]').forEach((step) => expect(step.getAttribute('tabindex')).toBe('0'))
  })

  it('opens a step tooltip from the keyboard', async () => {
    const { container } = renderSteps({ delivered_at: '2026-11-10T09:01:00Z', opens_tracked: false })
    fireEvent.focus(container.querySelector('[data-step="opened"]') as HTMLElement)
    expect(await screen.findByRole('tooltip')).toHaveTextContent('Opens were not tracked')
  })

  it('opens the failure reason from the keyboard', async () => {
    const { container } = renderSteps({ bounced_at: '2026-11-10T09:01:00Z', status_info: 'Mailbox full' })
    fireEvent.focus(container.querySelector('[tabindex="0"]') as HTMLElement)
    expect(await screen.findByRole('tooltip')).toHaveTextContent('Mailbox full')
  })

  it('keeps the failure reason readable and shows a complaint after a bounce', () => {
    const { container } = renderSteps({
      bounced_at: '2026-11-10T09:01:00Z',
      status_info: 'Mailbox full',
      complained_at: '2026-11-10T09:05:00Z'
    })
    expect(container.querySelector('.sr-only')?.textContent).toBe('Mailbox full')
    expect(screen.getByText('Bounced')).toBeInTheDocument()
    expect(screen.getByText('Marked as spam')).toBeInTheDocument()
  })
})
