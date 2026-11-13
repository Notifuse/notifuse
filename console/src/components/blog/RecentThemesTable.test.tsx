import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import dayjs from '../../lib/dayjs'
import { activateDayjsLocale } from '../../i18n/dayjsLocales'
import type { Workspace } from '../../services/api/types'
import { RecentThemesTable } from './RecentThemesTable'

vi.mock('../../services/api/blog', () => ({
  blogThemesApi: {
    list: vi.fn().mockResolvedValue({
      themes: [
        {
          version: 3,
          published_at: null,
          files: {},
          created_at: '2026-08-09T09:00:00Z',
          updated_at: '2026-08-09T09:00:00Z'
        }
      ],
      total_count: 1
    }),
    publish: vi.fn()
  }
}))
vi.mock('./ThemeEditorDrawer', () => ({ ThemeEditorDrawer: () => null }))

afterEach(() => {
  activateDayjsLocale('en')
})

function renderTable() {
  const workspace = { id: 'ws1', settings: { timezone: 'Asia/Tokyo' } } as unknown as Workspace
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <App>
        <RecentThemesTable workspaceId="ws1" workspace={workspace} />
      </App>
    </QueryClientProvider>
  )
}

describe('RecentThemesTable last edited tooltip', () => {
  it('gives the full date in the console language, on the workspace clock', async () => {
    activateDayjsLocale('fr')
    renderTable()

    fireEvent.mouseEnter(await screen.findByText(dayjs('2026-08-09T09:00:00Z').fromNow()))

    // 09:00 UTC is 18:00 in Tokyo.
    expect(await screen.findByText('9 août 2026 18:00 Asia/Tokyo')).toBeInTheDocument()
  })
})
