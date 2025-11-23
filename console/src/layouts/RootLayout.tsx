import { Outlet, useNavigate, useMatch } from '@tanstack/react-router'
import { Spin } from 'antd'
import { useAuth } from '../contexts/AuthContext'
import { useEffect } from 'react'

export function RootLayout() {
  const { isAuthenticated, loading, workspaces } = useAuth()
  const navigate = useNavigate()

  const isSigninRoute = useMatch({ from: '/console/signin', shouldThrow: false })
  const isAcceptInvitationRoute = useMatch({
    from: '/console/accept-invitation',
    shouldThrow: false
  })
  const isLogoutRoute = useMatch({ from: '/console/logout', shouldThrow: false })
  const isWorkspaceCreateRoute = useMatch({ from: '/console/workspace/create', shouldThrow: false })
  const isSetupRoute = useMatch({ from: '/console/setup', shouldThrow: false })

  // Check if system is installed (explicitly check for true to handle undefined case)
  const isInstalled = window.IS_INSTALLED === true

  const isPublicRoute = isSigninRoute || isAcceptInvitationRoute || isLogoutRoute || isSetupRoute

  // If system is not installed, redirect to setup wizard
  const shouldRedirectToSetup = !isInstalled && !isSetupRoute

  // If not authenticated and not on public routes, redirect to signin
  const shouldRedirectToSignin =
    !isLogoutRoute && !isSigninRoute && !isAuthenticated && !isPublicRoute && !shouldRedirectToSetup

  // If authenticated and has no workspaces, redirect to workspace creation
  const shouldRedirectToCreateWorkspace =
    isAuthenticated && workspaces.length === 0 && !isWorkspaceCreateRoute && !isLogoutRoute

  // console.log('isAuthenticated', isAuthenticated)
  // handle redirection...
  useEffect(() => {
    if (loading) return

    if (shouldRedirectToSetup) {
      navigate({ to: '/console/setup' })
      return
    }

    if (shouldRedirectToSignin) {
      navigate({ to: '/console/signin' })
      return
    }

    if (shouldRedirectToCreateWorkspace) {
      navigate({ to: '/console/workspace/create' })
      return
    }
  }, [loading, shouldRedirectToSetup, shouldRedirectToSignin, shouldRedirectToCreateWorkspace])

  if (
    loading ||
    shouldRedirectToSetup ||
    shouldRedirectToSignin ||
    shouldRedirectToCreateWorkspace
  ) {
    return (
      <div
        style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}
      >
        <Spin size="large" tip="Loading..." fullscreen />
      </div>
    )
  }

  return <Outlet />
}
