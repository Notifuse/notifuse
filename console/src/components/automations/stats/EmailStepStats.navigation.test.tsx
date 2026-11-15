import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { Outlet, RouterProvider, createMemoryHistory, createRootRoute, createRoute, createRouter } from '@tanstack/react-router'
import { EmailStepStats } from './EmailStepStats'

vi.stubGlobal('scrollTo', () => {})

// View messages moves through the router, without loading the console again.
describe('EmailStepStats, View messages', () => {
  it('opens the step messages in Logs through the router', async () => {
    const rootRoute = createRootRoute({ component: () => <Outlet /> })
    const step = createRoute({
      getParentRoute: () => rootRoute,
      path: '/',
      component: () => (
        <EmailStepStats email={undefined} canReadHistory smtp={false} messages={{ workspaceId: 'ws1', automationId: 'auto-1', templateId: 'tpl-1' }} />
      )
    })
    const logs = createRoute({
      getParentRoute: () => rootRoute,
      path: '/console/workspace/$workspaceId/logs',
      component: () => <div>Logs page</div>
    })
    const router = createRouter({
      routeTree: rootRoute.addChildren([step, logs]),
      history: createMemoryHistory({ initialEntries: ['/'] })
    })
    render(
      <I18nProvider i18n={i18n}>
        <RouterProvider router={router} />
      </I18nProvider>
    )

    fireEvent.click(await screen.findByRole('link', { name: 'View messages' }))

    expect(await screen.findByText('Logs page')).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/console/workspace/ws1/logs')
    expect(router.state.location.search).toEqual({ automation_id: 'auto-1', template_id: 'tpl-1' })
  })
})
