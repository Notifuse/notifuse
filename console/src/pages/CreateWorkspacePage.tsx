import { useState } from 'react'
import { Form, Input, Button, Typography, Card, Tooltip, App } from 'antd'
import { useNavigate } from '@tanstack/react-router'
import { InfoCircleOutlined, ArrowLeftOutlined } from '@ant-design/icons'
import { workspaceService } from '../services/api/workspace'
import { useAuth } from '../contexts/AuthContext'
import { MainLayout, MainLayoutSidebar } from '../layouts/MainLayout'

const { Title } = Typography

export function CreateWorkspacePage() {
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

      // Get user's timezone
      const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone

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
      message.success(`Workspace "${values.name}" created successfully!`)
      // wait for the refreshWorkspaces to propagate the new workspaces list to the root layout
      window.setTimeout(() => {
        navigate({
          to: '/workspace/$workspaceId',
          params: { workspaceId: values.id }
        })
      }, 100)
    } catch (error: any) {
      console.error('Error creating workspace:', error)
      message.error(error.message)
      setLoading(false)
    }
  }

  const handleBackToDashboard = () => {
    navigate({ to: '/' })
  }

  return (
    <MainLayout>
      <MainLayoutSidebar
        title="New workspace"
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
            label="Workspace Name"
            name="name"
            rules={[
              { required: true, message: 'Please enter a workspace name' },
              { min: 3, message: 'Workspace name must be at least 3 characters long' }
            ]}
          >
            <Input placeholder="Enter a name for your workspace" onChange={handleNameChange} />
          </Form.Item>

          <Form.Item
            label={
              <span>
                Workspace ID &nbsp;
                <Tooltip title="This ID will be used in URLs and API requests. It can only contain lowercase letters, numbers, and hyphens.">
                  <InfoCircleOutlined />
                </Tooltip>
              </span>
            }
            name="id"
            rules={[
              { required: true, message: 'Workspace ID is required' },
              {
                pattern: /^[a-z0-9-]+$/,
                message: 'ID can only contain lowercase letters, numbers, and hyphens'
              }
            ]}
          >
            <Input
              placeholder="workspace-id"
              suffix={
                <Tooltip title="ID is automatically generated but can be modified if needed">
                  <InfoCircleOutlined style={{ color: 'rgba(0,0,0,.45)' }} />
                </Tooltip>
              }
            />
          </Form.Item>

          <Form.Item
            label="Website URL"
            name="website_url"
            rules={[
              {
                pattern: /^(https?:\/\/)?([\da-z.-]+)\.([a-z.]{2,6})([/\w .-]*)*\/?$/,
                message: 'Please enter a valid URL',
                validateTrigger: 'onBlur'
              }
            ]}
            extra="We'll automatically detect and use your website's favicon"
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
              Create Workspace
            </Button>
          </Form.Item>
        </Form>
      </MainLayoutSidebar>
    </MainLayout>
  )
}
