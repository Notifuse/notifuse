import { Card, Row, Col, Typography, Button, Empty } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { useAuth } from '../contexts/AuthContext'
import { useNavigate } from '@tanstack/react-router'
import { MainLayout } from '../layouts/MainLayout'

const { Title, Text } = Typography

export function DashboardPage() {
  const { workspaces } = useAuth()
  const navigate = useNavigate()

  const handleWorkspaceClick = (workspaceId: string) => {
    navigate({
      to: '/workspace/$workspaceId/contacts',
      params: { workspaceId }
    })
  }

  const handleCreateWorkspace = () => {
    navigate({ to: '/workspace/create' })
  }

  return (
    <MainLayout>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '24px' }}>
        <Title level={2}>Workspaces</Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={handleCreateWorkspace}>
          Create Workspace
        </Button>
      </div>

      {workspaces.length === 0 ? (
        <Empty description="No workspaces found" style={{ margin: '48px 0' }} />
      ) : (
        <Row gutter={[24, 24]}>
          {workspaces.map((workspace) => (
            <Col xs={24} sm={12} md={8} lg={6} key={workspace.id}>
              <Card
                hoverable
                onClick={() => handleWorkspaceClick(workspace.id)}
                cover={
                  <div
                    style={{
                      height: '140px',
                      width: '100%',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      padding: '0',
                      background:
                        workspace.settings.cover_url || workspace.settings.logo_url
                          ? '#f5f5f5'
                          : '#e6f7ff',
                      overflow: 'hidden'
                    }}
                  >
                    {workspace.settings.cover_url ? (
                      <img
                        alt={workspace.name}
                        src={workspace.settings.cover_url}
                        style={{
                          width: '100%',
                          height: '100%',
                          objectFit: 'cover'
                        }}
                      />
                    ) : workspace.settings.logo_url ? (
                      <img
                        alt={workspace.name}
                        src={workspace.settings.logo_url}
                        style={{
                          maxWidth: '100%',
                          maxHeight: '100%',
                          padding: '16px',
                          objectFit: 'contain'
                        }}
                      />
                    ) : (
                      <Typography.Text strong style={{ color: '#1890ff' }}>
                        {workspace.name.substring(0, 2).toUpperCase()}
                      </Typography.Text>
                    )}
                  </div>
                }
              >
                <Card.Meta
                  title={workspace.name}
                  description={
                    <Text type="secondary" ellipsis>
                      ID: {workspace.id}
                    </Text>
                  }
                />
              </Card>
            </Col>
          ))}
        </Row>
      )}
    </MainLayout>
  )
}
