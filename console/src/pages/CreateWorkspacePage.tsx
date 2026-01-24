import { useState } from 'react'
import { Form, Input, Button, Tooltip, App } from 'antd'
import { useNavigate } from '@tanstack/react-router'
import { InfoCircleOutlined, ArrowLeftOutlined } from '@ant-design/icons'
import { workspaceService } from '../services/api/workspace'
import { useAuth } from '../contexts/AuthContext'
import { MainLayout, MainLayoutSidebar } from '../layouts/MainLayout'
import { getBrowserTimezone } from '../lib/timezoneNormalizer'
import { useLingui } from '@lingui/react/macro'

export function CreateWorkspacePage() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [form] = Form.useForm()
  const { refreshWorkspaces } = useAuth()
  const { message } = App.useApp()

  // Generate workspace ID from name (alphanumeric only, max 20 chars)
  const generateWorkspaceId = (name: string) => {
    if (!name) return ''
    // remove spaces and remove non-alphanumeric characters
    return name
      .toLowerCase()
      .replace(/[^a-z0-9]/g, '')
      .substring(0, 20)
  }

  // Update generated ID when name changes
  const handleNameChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const name = e.target.value
    const id = generateWorkspaceId(name)
    form.setFieldsValue({ id })
  }

  const onFinish = async (values: { name: string; id: string; website_url?: string }) => {
    try {
      setLoading(true)
      let logoUrl = null
      let coverUrl = null

      // If website URL is provided, detect favicon and cover image
      if (values.website_url) {
        try {
          const faviconResponse = await workspaceService.detectFavicon(values.website_url)
          logoUrl = faviconResponse.iconUrl
          coverUrl = faviconResponse.coverUrl || null
        } catch (error) {
          console.error('Error detecting website assets:', error)
          // Don't fail the whole process if detection fails
        }
      }

      // Get user's timezone (normalized to canonical IANA name)
      const timezone = getBrowserTimezone()

      // Create workspace with API
      await workspaceService.create({
        id: generateWorkspaceId(values.id),
        name: values.name,
        settings: {
          website_url: values.website_url || '',
          logo_url: logoUrl,
          cover_url: coverUrl,
          timezone: timezone,
          email_tracking_enabled: true
        }
      })

      await refreshWorkspaces()

      // Navigate to the new workspace
      message.success(t`Workspace "${values.name}" created successfully!`)
      // wait for the refreshWorkspaces to propagate the new workspaces list to the root layout
      window.setTimeout(() => {
        navigate({
          to: '/console/workspace/$workspaceId',
          params: { workspaceId: values.id }
        })
      }, 100)
    } catch (error) {
      console.error('Error creating workspace:', error)
      message.error(error instanceof Error ? error.message : t`Failed to create workspace`)
      setLoading(false)
    }
  }

  const handleBackToDashboard = () => {
    navigate({ to: '/console' })
  }

  return (
    <MainLayout>
      <MainLayoutSidebar
        title={t`New workspace`}
        extra={
          <Button
            type="primary"
            ghost
            icon={<ArrowLeftOutlined />}
            onClick={handleBackToDashboard}
            style={{ padding: '4px', lineHeight: 1 }}
          />
        }
      >
        <Form
          name="create-workspace"
          layout="vertical"
          onFinish={onFinish}
          autoComplete="off"
          form={form}
          initialValues={{ id: '' }}
        >
          <Form.Item
            label={t`Workspace Name`}
            name="name"
            rules={[
              { required: true, message: t`Please enter a workspace name` },
              { min: 3, message: t`Workspace name must be at least 3 characters long` }
            ]}
          >
            <Input placeholder={t`Enter a name for your workspace`} onChange={handleNameChange} />
          </Form.Item>

          <Form.Item
            label={
              <span>
                {t`Workspace ID`} &nbsp;
                <Tooltip title={t`This ID will be used in URLs and API requests. It can only contain lowercase letters and numbers.`}>
                  <InfoCircleOutlined />
                </Tooltip>
              </span>
            }
            name="id"
            rules={[
              { required: true, message: t`Workspace ID is required` },
              {
                pattern: /^[a-z0-9]+$/,
                message: t`ID can only contain lowercase letters and numbers`
              }
            ]}
          >
            <Input
              placeholder="workspaceid"
              suffix={
                <Tooltip title={t`ID is automatically generated but can be modified if needed`}>
                  <InfoCircleOutlined style={{ color: 'rgba(0,0,0,.45)' }} />
                </Tooltip>
              }
            />
          </Form.Item>

          <Form.Item
            label={t`Website URL`}
            name="website_url"
            rules={[
              {
                pattern: /^(https?:\/\/)?([\da-z.-]+)\.([a-z.]{2,6})([/\w .-]*)*\/?$/,
                message: t`Please enter a valid URL`,
                validateTrigger: 'onBlur'
              }
            ]}
            extra={t`We'll automatically detect and use your website's favicon`}
          >
            <Input placeholder="https://example.com" />
          </Form.Item>

          <Form.Item>
            <Button
              type="primary"
              htmlType="submit"
              loading={loading}
              style={{ width: '100%', marginTop: 20 }}
            >
              {t`Create Workspace`}
            </Button>
          </Form.Item>
        </Form>
      </MainLayoutSidebar>
    </MainLayout>
  )
}
