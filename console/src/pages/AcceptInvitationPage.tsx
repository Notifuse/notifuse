import { Form, Button, Card, App, Spin, Typography } from 'antd'
import { useAuth } from '../contexts/AuthContext'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { useState, useEffect } from 'react'
import { workspaceService } from '../services/api/workspace'
import { MainLayout } from '../layouts/MainLayout'
import type { VerifyInvitationTokenResponse } from '../services/api/types'

const { Title, Text } = Typography

export function AcceptInvitationPage() {
  const { signin, signout, user } = useAuth()
  const navigate = useNavigate()
  const search = useSearch({ from: '/accept-invitation' })
  const [loading, setLoading] = useState(true)
  const [accepting, setAccepting] = useState(false)
  const [accepted, setAccepted] = useState(false)
  const [invitationData, setInvitationData] = useState<VerifyInvitationTokenResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const { message } = App.useApp()

  // Get token from URL parameter
  const token = search.token as string

  useEffect(() => {
    // Don't verify token again if invitation has already been accepted
    if (accepted) {
      return
    }

    if (!token) {
      setError('Invalid invitation link: missing token')
      setLoading(false)
      return
    }

    // If user is already logged in, sign them out first
    if (user) {
      signout()
    }

    // Verify the invitation token
    const verifyToken = async () => {
      try {
        setLoading(true)
        const response = await workspaceService.verifyInvitationToken(token)
        setInvitationData(response)
        setError(null)
      } catch (error: any) {
        const errorMessage = error?.message || 'Invalid or expired invitation token'
        setError(errorMessage)
        message.error(errorMessage)
      } finally {
        setLoading(false)
      }
    }

    verifyToken()
  }, [token, user, signout, message, accepted])

  const handleAcceptInvitation = async () => {
    if (!token || !invitationData) return

    try {
      setAccepting(true)
      const response = await workspaceService.acceptInvitation(token)

      // Mark invitation as accepted immediately to prevent re-verification
      setAccepted(true)

      // Sign in the user with the returned token
      await signin(response.token)

      message.success('Invitation accepted successfully! Welcome to the workspace.')

      // Navigate to the dashboard
      setTimeout(() => {
        navigate({ to: '/' })
      }, 100)
    } catch (error: any) {
      const errorMessage = error?.message || 'Failed to accept invitation'
      message.error(errorMessage)
      setError(errorMessage)
    } finally {
      setAccepting(false)
    }
  }

  const handleDecline = () => {
    navigate({ to: '/signin' })
  }

  if (loading) {
    return (
      <MainLayout>
        <div className="flex items-center justify-center h-[calc(100vh-48px)]">
          <Card style={{ width: 500, textAlign: 'center' }}>
            <Spin size="large" />
            <div style={{ marginTop: 16 }}>
              <Text>Verifying invitation...</Text>
            </div>
          </Card>
        </div>
      </MainLayout>
    )
  }

  if (error || !invitationData) {
    return (
      <MainLayout>
        <div className="flex items-center justify-center h-[calc(100vh-48px)]">
          <Card style={{ width: 500 }}>
            <div style={{ textAlign: 'center', marginBottom: 24 }}>
              <Title level={3} type="danger">
                Invalid Invitation
              </Title>
              <Text type="secondary">
                {error || 'This invitation link is invalid or has expired.'}
              </Text>
            </div>
            <Button type="primary" block onClick={() => navigate({ to: '/signin' })}>
              Go to Sign In
            </Button>
          </Card>
        </div>
      </MainLayout>
    )
  }

  return (
    <MainLayout>
      <div className="flex items-center justify-center h-[calc(100vh-48px)]">
        <Card style={{ width: 500 }}>
          <div style={{ textAlign: 'center', marginBottom: 24 }}>
            <Title level={3}>Workspace Invitation</Title>
            <Text type="secondary">
              You've been invited to join <strong>{invitationData.workspace.name}</strong>
            </Text>
          </div>

          <div
            style={{ marginBottom: 24, padding: 16, backgroundColor: '#f5f5f5', borderRadius: 8 }}
          >
            <div style={{ marginBottom: 8 }}>
              <Text strong>Workspace:</Text> {invitationData.workspace.name}
            </div>
            <div style={{ marginBottom: 8 }}>
              <Text strong>Email:</Text> {invitationData.invitation.email}
            </div>
            <div>
              <Text strong>Expires:</Text>{' '}
              {new Date(invitationData.invitation.expires_at).toLocaleDateString()}
            </div>
          </div>

          <Form layout="vertical">
            <Form.Item style={{ marginBottom: 12 }}>
              <Button
                type="primary"
                block
                size="large"
                loading={accepting}
                onClick={handleAcceptInvitation}
              >
                Accept Invitation
              </Button>
            </Form.Item>
            <Form.Item style={{ marginBottom: 0 }}>
              <Button block onClick={handleDecline} disabled={accepting}>
                Decline
              </Button>
            </Form.Item>
          </Form>
        </Card>
      </div>
    </MainLayout>
  )
}
