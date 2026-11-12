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
