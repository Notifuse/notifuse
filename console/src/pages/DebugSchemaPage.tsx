import { useState } from 'react'
import {
  Card,
  Typography,
  Space,
  Button,
  Alert,
  Collapse,
  Table,
  Tag,
  Statistic,
  Row,
  Col,
  Checkbox,
  Spin,
  Result,
  Layout,
  message
} from 'antd'
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
  ReloadOutlined,
  ToolOutlined,
  ArrowLeftOutlined
} from '@ant-design/icons'
import { useAuth } from '../contexts/AuthContext'
import { useNavigate } from '@tanstack/react-router'
import { isRootUser } from '../services/api/auth'
import {
  debugApi,
  SchemaVerificationResult,
  SchemaRepairResult,
  WorkspaceVerification,
  FunctionVerification,
  TriggerVerification
} from '../services/api/debug'

const { Title, Paragraph, Text } = Typography
const { Panel } = Collapse

const { Content } = Layout

export function DebugSchemaPage() {
  const { user } = useAuth()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [repairing, setRepairing] = useState(false)
  const [verificationResult, setVerificationResult] = useState<SchemaVerificationResult | null>(
    null
  )
  const [repairResult, setRepairResult] = useState<SchemaRepairResult | null>(null)
  const [selectedWorkspaces, setSelectedWorkspaces] = useState<string[]>([])
  const [repairFunctions, setRepairFunctions] = useState(true)
  const [repairTriggers, setRepairTriggers] = useState(true)

  // Check if user is root
  if (!isRootUser(user?.email)) {
    return (
      <Layout style={{ minHeight: '100vh', background: '#f5f5f5' }}>
        <Content style={{ padding: '24px' }}>
          <Result
            status="403"
            title="Access Denied"
            subTitle="This page is only accessible to the root user."
            extra={
              <Button type="primary" onClick={() => navigate({ to: '/console/' })}>
                Go to Dashboard
              </Button>
            }
          />
        </Content>
      </Layout>
    )
  }

  const handleVerify = async () => {
    setLoading(true)
    setVerificationResult(null)
    setRepairResult(null)
    try {
      const result = await debugApi.verifySchema()
      setVerificationResult(result)
      // Pre-select workspaces that have issues
      const failedWorkspaces = result.workspace_dbs
        .filter((ws) => ws.status !== 'passed')
        .map((ws) => ws.workspace_id)
      setSelectedWorkspaces(failedWorkspaces)
    } catch (error) {
      message.error('Failed to verify schemas')
      console.error('Verification error:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleRepair = async () => {
    if (!repairFunctions && !repairTriggers) {
      message.warning('Please select at least one repair option')
      return
    }

    setRepairing(true)
    setRepairResult(null)
    try {
      const result = await debugApi.repairSchema({
        workspace_ids: selectedWorkspaces.length > 0 ? selectedWorkspaces : undefined,
        repair_functions: repairFunctions,
        repair_triggers: repairTriggers
      })
      setRepairResult(result)
      message.success(
        `Repair complete: ${result.summary.successful_repairs}/${result.summary.total_workspaces} workspaces repaired`
      )
      // Re-verify after repair
      await handleVerify()
    } catch (error) {
      message.error('Failed to repair schemas')
      console.error('Repair error:', error)
    } finally {
      setRepairing(false)
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'passed':
        return <CheckCircleOutlined style={{ color: '#52c41a' }} />
      case 'failed':
        return <CloseCircleOutlined style={{ color: '#f5222d' }} />
      case 'error':
        return <ExclamationCircleOutlined style={{ color: '#faad14' }} />
      default:
        return null
    }
  }

  const getStatusTag = (status: string) => {
    switch (status) {
      case 'passed':
        return <Tag color="success">Passed</Tag>
      case 'failed':
        return <Tag color="error">Failed</Tag>
      case 'error':
        return <Tag color="warning">Error</Tag>
      case 'success':
        return <Tag color="success">Success</Tag>
      case 'partial':
        return <Tag color="warning">Partial</Tag>
      default:
        return <Tag>{status}</Tag>
    }
  }

  const functionColumns = [
    {
      title: 'Function Name',
      dataIndex: 'name',
      key: 'name'
    },
    {
      title: 'Status',
      dataIndex: 'exists',
      key: 'exists',
      render: (exists: boolean) =>
        exists ? (
          <Tag color="success">
            <CheckCircleOutlined /> Exists
          </Tag>
        ) : (
          <Tag color="error">
            <CloseCircleOutlined /> Missing
          </Tag>
        )
    }
  ]

  const triggerColumns = [
    {
      title: 'Trigger Name',
      dataIndex: 'name',
      key: 'name'
    },
    {
      title: 'Table',
      dataIndex: 'table_name',
      key: 'table_name'
    },
    {
      title: 'Status',
      dataIndex: 'exists',
      key: 'exists',
      render: (exists: boolean) =>
        exists ? (
          <Tag color="success">
            <CheckCircleOutlined /> Exists
          </Tag>
        ) : (
          <Tag color="error">
            <CloseCircleOutlined /> Missing
          </Tag>
        )
    }
  ]

  const renderWorkspaceVerification = (workspace: WorkspaceVerification) => {
    const missingFunctions = workspace.trigger_functions.filter((f) => !f.exists)
    const missingTriggers = workspace.triggers.filter((t) => !t.exists)

    return (
      <Panel
        header={
          <Space>
            {getStatusIcon(workspace.status)}
            <Text strong>{workspace.workspace_name}</Text>
            <Text type="secondary">({workspace.workspace_id})</Text>
            {getStatusTag(workspace.status)}
            {missingFunctions.length > 0 && (
              <Tag color="orange">{missingFunctions.length} missing functions</Tag>
            )}
            {missingTriggers.length > 0 && (
              <Tag color="orange">{missingTriggers.length} missing triggers</Tag>
            )}
          </Space>
        }
        key={workspace.workspace_id}
        extra={
          <Checkbox
            checked={selectedWorkspaces.includes(workspace.workspace_id)}
            onChange={(e) => {
              e.stopPropagation()
              if (e.target.checked) {
                setSelectedWorkspaces([...selectedWorkspaces, workspace.workspace_id])
              } else {
                setSelectedWorkspaces(
                  selectedWorkspaces.filter((id) => id !== workspace.workspace_id)
                )
              }
            }}
            onClick={(e) => e.stopPropagation()}
          >
            Select for repair
          </Checkbox>
        }
      >
        {workspace.error && (
          <Alert
            message="Error"
            description={workspace.error}
            type="error"
            showIcon
            style={{ marginBottom: 16 }}
          />
        )}

        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <Card title="Trigger Functions" size="small">
            <Table
              dataSource={workspace.trigger_functions}
              columns={functionColumns}
              rowKey="name"
              pagination={false}
              size="small"
              rowClassName={(record: FunctionVerification) =>
                !record.exists ? 'ant-table-row-error' : ''
              }
            />
          </Card>

          <Card title="Triggers" size="small">
            <Table
              dataSource={workspace.triggers}
              columns={triggerColumns}
              rowKey="name"
              pagination={false}
              size="small"
              rowClassName={(record: TriggerVerification) =>
                !record.exists ? 'ant-table-row-error' : ''
              }
            />
          </Card>
        </Space>
      </Panel>
    )
  }

  return (
    <Layout style={{ minHeight: '100vh', background: '#f5f5f5' }}>
      <Content style={{ padding: '24px' }}>
        <div style={{ maxWidth: '1400px', margin: '0 auto' }}>
          <Space direction="vertical" size="large" style={{ width: '100%' }}>
            <div>
              <Space>
                <Button
                  icon={<ArrowLeftOutlined />}
                  onClick={() => navigate({ to: '/console/' })}
                >
                  Back to Dashboard
                </Button>
              </Space>
              <Title level={2} style={{ marginTop: 16 }}>
                Schema Verification & Repair
              </Title>
              <Paragraph type="secondary">
                Verify database schemas, triggers, and functions across all workspace databases.
                Repair any missing or misconfigured components.
              </Paragraph>
            </div>

            <Card>
              <Space>
                <Button
                  type="primary"
                  icon={<ReloadOutlined />}
                  onClick={handleVerify}
                  loading={loading}
                >
                  Verify Schemas
                </Button>
                {verificationResult && (
                  <>
                    <Checkbox
                      checked={repairFunctions}
                      onChange={(e) => setRepairFunctions(e.target.checked)}
                    >
                      Repair Functions
                    </Checkbox>
                    <Checkbox
                      checked={repairTriggers}
                      onChange={(e) => setRepairTriggers(e.target.checked)}
                    >
                      Repair Triggers
                    </Checkbox>
                    <Button
                      type="primary"
                      danger
                      icon={<ToolOutlined />}
                      onClick={handleRepair}
                      loading={repairing}
                      disabled={selectedWorkspaces.length === 0 && verificationResult !== null}
                    >
                      Repair Selected ({selectedWorkspaces.length || 'All'})
                    </Button>
                  </>
                )}
              </Space>
            </Card>

            {loading && (
              <Card>
                <div style={{ textAlign: 'center', padding: '40px' }}>
                  <Spin size="large" />
                  <Paragraph style={{ marginTop: 16 }}>Verifying schemas...</Paragraph>
                </div>
              </Card>
            )}

            {repairResult && (
              <Alert
                message="Repair Complete"
                description={
                  <Space direction="vertical">
                    <Text>
                      Total workspaces: {repairResult.summary.total_workspaces} | Successful:{' '}
                      {repairResult.summary.successful_repairs} | Failed:{' '}
                      {repairResult.summary.failed_repairs}
                    </Text>
                    <Text>
                      Functions recreated: {repairResult.summary.functions_recreated} | Triggers
                      recreated: {repairResult.summary.triggers_recreated}
                    </Text>
                  </Space>
                }
                type={repairResult.summary.failed_repairs === 0 ? 'success' : 'warning'}
                showIcon
                closable
              />
            )}

            {verificationResult && !loading && (
              <>
                <Row gutter={16}>
                  <Col span={6}>
                    <Card>
                      <Statistic
                        title="Total Databases"
                        value={verificationResult.summary.total_databases}
                      />
                    </Card>
                  </Col>
                  <Col span={6}>
                    <Card>
                      <Statistic
                        title="Passed"
                        value={verificationResult.summary.passed_databases}
                        valueStyle={{ color: '#3f8600' }}
                        prefix={<CheckCircleOutlined />}
                      />
                    </Card>
                  </Col>
                  <Col span={6}>
                    <Card>
                      <Statistic
                        title="Failed"
                        value={verificationResult.summary.failed_databases}
                        valueStyle={{
                          color: verificationResult.summary.failed_databases > 0 ? '#cf1322' : '#000'
                        }}
                        prefix={<CloseCircleOutlined />}
                      />
                    </Card>
                  </Col>
                  <Col span={6}>
                    <Card>
                      <Statistic
                        title="Total Issues"
                        value={verificationResult.summary.total_issues}
                        valueStyle={{
                          color: verificationResult.summary.total_issues > 0 ? '#faad14' : '#000'
                        }}
                        prefix={<ExclamationCircleOutlined />}
                      />
                    </Card>
                  </Col>
                </Row>

                <Card title="Workspace Databases">
                  {verificationResult.workspace_dbs.length === 0 ? (
                    <Alert message="No workspace databases found" type="info" showIcon />
                  ) : (
                    <Collapse accordion>
                      {verificationResult.workspace_dbs.map((workspace) =>
                        renderWorkspaceVerification(workspace)
                      )}
                    </Collapse>
                  )}
                </Card>

                <Card title="Verification Details" size="small">
                  <Paragraph type="secondary">
                    Verified at: {new Date(verificationResult.verified_at).toLocaleString()}
                  </Paragraph>
                  <Collapse ghost>
                    <Panel header="Raw JSON" key="json">
                      <pre
                        style={{
                          background: '#f5f5f5',
                          padding: '16px',
                          borderRadius: '4px',
                          overflow: 'auto',
                          maxHeight: '400px',
                          fontSize: '12px'
                        }}
                      >
                        {JSON.stringify(verificationResult, null, 2)}
                      </pre>
                    </Panel>
                  </Collapse>
                </Card>
              </>
            )}
          </Space>
        </div>
      </Content>
    </Layout>
  )
}
