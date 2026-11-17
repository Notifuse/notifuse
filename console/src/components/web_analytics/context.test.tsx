import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider
} from '@tanstack/react-router'
import { WebAnalyticsProvider } from './context'
import { useWebAnalytics } from './useWebAnalytics'
import { parseMinSessions } from './lib/minSessions'

// The global test setup stubs useNavigate; this test needs the real one, so the provider's writes reach
// the URL and come back through the route's validateSearch.
vi.mock('@tanstack/react-router', async () => await vi.importActual('@tanstack/react-router'))

// The router restores the scroll position after each navigation, which jsdom does not implement.
vi.stubGlobal('scrollTo', () => {})

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [] })
}))

function Probe() {
  const context = useWebAnalytics()
  return (
    <>
      <output aria-label="min sessions">{context.minSessions}</output>
      {[1, 10, 25].map((value) => (
        <button key={value} type="button" onClick={() => context.setMinSessions(value)}>
          {`Set ${value}`}
        </button>
      ))}
    </>
  )
}

/** A real router (memory history) whose page reads minSessions the way the web analytics route does. */
function renderAt(url: string) {
  const rootRoute = createRootRoute()
  const pageRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/',
    validateSearch: (search: Record<string, unknown>) => ({
      minSessions: parseMinSessions(search.minSessions)
    }),
    component: () => (
      <WebAnalyticsProvider workspaceId="ws1">
        <Probe />
      </WebAnalyticsProvider>
    )
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([pageRoute]),
    history: createMemoryHistory({ initialEntries: [url] })
  })
  render(<RouterProvider router={router} />)
  return router
}

const shown = () => screen.getByRole('status', { name: 'min sessions' }).textContent

describe('WebAnalyticsProvider minSessions', () => {
  it('reads an absent param as the default of 10', async () => {
    renderAt('/')
    expect(await screen.findByRole('status', { name: 'min sessions' })).toHaveTextContent('10')
  })

  it('keeps 1 in the URL and reads 1 back', async () => {
    const router = renderAt('/')
    await screen.findByRole('status', { name: 'min sessions' })

    fireEvent.click(screen.getByRole('button', { name: 'Set 1' }))

    await waitFor(() => expect(shown()).toBe('1'))
    expect(router.state.location.search).toMatchObject({ minSessions: 1 })
    expect(router.state.location.href).toContain('minSessions=1')
  })

  it('removes the param when the default is chosen again', async () => {
    const router = renderAt('/?minSessions=1')
    await waitFor(() => expect(shown()).toBe('1'))

    fireEvent.click(screen.getByRole('button', { name: 'Set 10' }))

    await waitFor(() => expect(shown()).toBe('10'))
    expect(router.state.location.search).not.toHaveProperty('minSessions')
    expect(router.state.location.href).not.toContain('minSessions')
  })

  it('keeps any other floor', async () => {
    const router = renderAt('/')
    await screen.findByRole('status', { name: 'min sessions' })

    fireEvent.click(screen.getByRole('button', { name: 'Set 25' }))

    await waitFor(() => expect(shown()).toBe('25'))
    expect(router.state.location.href).toContain('minSessions=25')
  })
})
