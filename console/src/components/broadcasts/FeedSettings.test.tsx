import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import type { ReactNode } from 'react'
import { GlobalFeedSettings } from './GlobalFeedSettings'
import { RecipientFeedSettings } from './RecipientFeedSettings'

const wrap = (node: ReactNode) =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <AntApp>{node}</AntApp>
    </QueryClientProvider>
  )

const feed = { enabled: true, url: 'https://api.example.com/feed', headers: [] }

describe.each([
  ['GlobalFeedSettings', (id?: string) => <GlobalFeedSettings workspaceId="ws1" broadcastId={id} value={feed} />],
  ['RecipientFeedSettings', (id?: string) => <RecipientFeedSettings workspaceId="ws1" broadcastId={id} value={feed} />]
])('%s test button', (_name, element) => {
  it('is disabled until the broadcast is saved, and says so', async () => {
    wrap(element())
    const button = screen.getByRole('button', { name: 'Test' })
    expect(button).toBeDisabled()
    fireEvent.mouseEnter(button.parentElement as HTMLElement)
    fireEvent.mouseOver(button)
    expect(await screen.findByText('Save the broadcast first')).toBeInTheDocument()
  })

  it('is enabled once the broadcast exists', () => {
    wrap(element('bc1'))
    expect(screen.getByRole('button', { name: 'Test' })).toBeEnabled()
  })
})

describe.each([
  ['GlobalFeedSettings', () => <GlobalFeedSettings workspaceId="ws1" broadcastId="bc1" value={{ enabled: true, url: '', headers: [] }} />],
  ['RecipientFeedSettings', () => <RecipientFeedSettings workspaceId="ws1" broadcastId="bc1" value={{ enabled: true, url: '', headers: [] }} />]
])('%s URL field', (_name, element) => {
  it('waits for the field to be left empty before saying the URL is required', () => {
    // The error showed as soon as the feed was switched on, before anything could be typed.
    wrap(element())
    expect(screen.queryByText('URL is required')).toBeNull()
    fireEvent.blur(screen.getByPlaceholderText(/^https:\/\/api\.example\.com/))
    expect(screen.getByText('URL is required')).toBeInTheDocument()
  })

  it('has no colon after its label', () => {
    wrap(element())
    // antd writes the colon from CSS unless the label carries this class.
    expect(screen.getByText('Feed URL').closest('label')).toHaveClass('ant-form-item-no-colon')
  })
})
