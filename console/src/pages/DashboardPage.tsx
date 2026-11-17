import { Card, Typography, Button, Empty, Tooltip } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { useAuth } from '../contexts/AuthContext'
import { useNavigate } from '@tanstack/react-router'
import { MainLayout, MainLayoutSidebar } from '../layouts/MainLayout'
import { isRootUser } from '../services/api/auth'
import { useLingui } from '@lingui/react/macro'
import { SystemSettingsDrawer } from '../components/settings/SystemSettingsDrawer'
import { PendingInvitations } from '../components/workspace/PendingInvitations'
import { useState } from 'react'
import { useUpdateCheck } from '../hooks/useUpdateCheck'
import { UpdateCard } from '../components/updates/UpdateCard'
import { UpdateDrawer } from '../components/updates/UpdateDrawer'
import { UpdateFooter } from '../components/updates/UpdateFooter'

const { Text } = Typography

export function DashboardPage() {
  const { t } = useLingui()
  const { workspaces, user, serverVersion } = useAuth()
  const navigate = useNavigate()
  const updates = useUpdateCheck()
  const [updatesOpen, setUpdatesOpen] = useState(false)

  const handleWorkspaceClick = (workspaceId: string) => {
    navigate({
      to: '/console/workspace/$workspaceId',
      params: { workspaceId }
    })
  }

  const handleCreateWorkspace = () => {
    navigate({ to: '/console/workspace/create' })
  }

  return (
    <MainLayout>
      <MainLayoutSidebar
        title={t`Select workspace`}
        footer={
          <div className="flex flex-col gap-3">
            {updates.data && updates.available && !updates.dismissed && (
              <UpdateCard data={updates.data} onOpen={() => setUpdatesOpen(true)} onIgnore={updates.ignore} />
            )}
            <UpdateFooter
              // The version the update check speaks of, so the page never shows two.
              version={serverVersion || window.VERSION || '1.0'}
              dismissedLatest={updates.data && updates.dismissed ? updates.data.latest : null}
              onOpen={() => setUpdatesOpen(true)}
            />
          </div>
        }
        extra={
          isRootUser(user?.email) ? (
            <div style={{ display: 'flex', gap: '8px' }}>
              {/* Root only, and no licence gate stands in front of it: system settings is
                  where SSO is switched on and off, and it is where a deployment running SSO
                  without a licence for it is told so. */}
              <SystemSettingsDrawer workspaceId={workspaces[0]?.id} />
              <Tooltip title={t`New workspace`}>
                <Button
                  type="primary"
                  ghost
                  aria-label={t`New workspace`}
                  icon={<PlusOutlined />}
                  onClick={handleCreateWorkspace}
                  style={{ padding: '4px', lineHeight: 1 }}
                />
              </Tooltip>
            </div>
          ) : undefined
        }
      >
        <PendingInvitations />
        {workspaces.length === 0 ? (
          <Empty description={t`No workspaces`} style={{ margin: '24px 0' }} />
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            {workspaces.map((workspace) => (
              <Card
                key={workspace.id}
                hoverable
                size="small"
                onClick={() => handleWorkspaceClick(workspace.id)}
                // Reached and opened from the keyboard, as a link would be.
                role="link"
                tabIndex={0}
                aria-label={workspace.name}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault()
                    handleWorkspaceClick(workspace.id)
                  }
                }}
                style={{ marginBottom: '8px' }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <div
                    style={{
                      width: '32px',
                      height: '32px',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      background: workspace.settings.logo_url ? '#f5f5f5' : '#e6f7ff',
                      borderRadius: '4px',
                      overflow: 'hidden'
                    }}
                  >
                    {workspace.settings.logo_url ? (
                      <img
                        alt={workspace.name}
                        src={workspace.settings.logo_url}
                        style={{
                          maxWidth: '100%',
                          maxHeight: '100%',
                          objectFit: 'contain'
                        }}
                      />
                    ) : (
                      <Typography.Text strong style={{ color: '#1890ff' }}>
                        {workspace.name.substring(0, 2).toUpperCase()}
                      </Typography.Text>
                    )}
                  </div>
                  <div>
                    <div style={{ fontWeight: 500 }}>{workspace.name}</div>
                    <Text type="secondary" style={{ fontSize: '11px' }} ellipsis>
                      {t`ID:`} {workspace.id}
                    </Text>
                  </div>
                </div>
              </Card>
            ))}
          </div>
        )}
      </MainLayoutSidebar>
      {updates.data && updates.available && (
        <UpdateDrawer open={updatesOpen} onClose={() => setUpdatesOpen(false)} data={updates.data} />
      )}
    </MainLayout>
  )
}
