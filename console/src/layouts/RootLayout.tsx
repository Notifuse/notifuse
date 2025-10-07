import { Outlet, useNavigate, useMatch } from '@tanstack/react-router'
import { Spin } from 'antd'
import { useAuth } from '../contexts/AuthContext'
import { useEffect } from 'react'

export function RootLayout() {
  const { isAuthenticated, loading, workspaces } = useAuth()
  const navigate = useNavigate()

  const isSigninRoute = useMatch({ from: '/signin', shouldThrow: false })
  const isAcceptInvitationRoute = useMatch({ from: '/accept-invitation', shouldThrow: false })
  const isLogoutRoute = useMatch({ from: '/logout', shouldThrow: false })
  const isWorkspaceCreateRoute = useMatch({ from: '/workspace/create', shouldThrow: false })
  const isSetupRoute = useMatch({ from: '/setup', shouldThrow: false })

  // Check if system is installed
  const isInstalled = window.IS_INSTALLED !== false

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
      navigate({ to: '/setup' })
      return
    }

    if (shouldRedirectToSignin) {
      navigate({ to: '/signin' })
      return
    }

    if (shouldRedirectToCreateWorkspace) {
      navigate({ to: '/workspace/create' })
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
