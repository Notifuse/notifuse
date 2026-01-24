import { useEffect } from 'react'
import { useAuth } from '../contexts/AuthContext'
import { useNavigate } from '@tanstack/react-router'
import { Spin } from 'antd'
import { useLingui } from '@lingui/react/macro'

export function LogoutPage() {
  const { t } = useLingui()
  const { signout } = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    const performSignout = async () => {
      await signout()
      navigate({ to: '/console/signin' })
    }
    performSignout()
  }, [signout, navigate])

  return (
    <div
      style={{
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        minHeight: '100vh'
      }}
    >
      <Spin size="large" tip={t`Signing out...`} fullscreen />
    </div>
  )
}
