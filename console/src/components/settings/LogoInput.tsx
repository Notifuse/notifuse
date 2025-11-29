import { useState } from 'react'
import { Button, Form, Input, Image, App, Space } from 'antd'
import { SearchOutlined } from '@ant-design/icons'
import { workspaceService } from '../../services/api/workspace'

interface LogoInputProps {
  name?: string
  label?: string
  placeholder?: string
  rules?: Array<{ type?: 'url'; message?: string }>
}

export function LogoInput({
  name = 'logo_url',
  label = 'Logo URL',
  placeholder = 'https://example.com/logo.png',
  rules = [{ type: 'url' as const, message: 'Please enter a valid URL' }]
}: LogoInputProps) {
  const [isDetectingIcon, setIsDetectingIcon] = useState(false)
  const { message } = App.useApp()
  const [form] = Form.useForm()

  // Get the form from context
  const formInstance = Form.useFormInstance() || form

  const handleDetectIcon = async () => {
    const website = formInstance.getFieldValue('website_url')
    if (!website) {
      message.error('Please enter a website URL first')
      return
    }

    setIsDetectingIcon(true)
    try {
      const { iconUrl } = await workspaceService.detectFavicon(website)

      if (iconUrl) {
        formInstance.setFieldsValue({ [name]: iconUrl })
        message.success('Icon detected successfully')
      } else {
        message.warning('No icon found')
      }
    } catch (error: unknown) {
      console.error('Error detecting icon:', error)
      message.error('Failed to detect icon: ' + ((error as Error).message || error))
    } finally {
      setIsDetectingIcon(false)
    }
  }

  return (
    <Form.Item label={label}>
      <Space.Compact style={{ width: '100%' }}>
        <Form.Item noStyle shouldUpdate={(prev, current) => prev[name] !== current[name]}>
          {() => {
            const logoUrl = formInstance.getFieldValue(name)
            return logoUrl ? (
              <div
                style={{
                  width: 40,
                  height: 32,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  border: '1px solid #d9d9d9',
                  borderRight: 0,
                  borderRadius: '6px 0 0 6px',
                  background: '#fafafa'
                }}
              >
                <Image src={logoUrl} alt="Logo Preview" height={24} preview={false} />
              </div>
            ) : null
          }}
        </Form.Item>
        <Form.Item name={name} noStyle rules={rules}>
          <Input placeholder={placeholder} style={{ flex: 1 }} />
        </Form.Item>
        <Button
          icon={<SearchOutlined />}
          onClick={handleDetectIcon}
          loading={isDetectingIcon}
        >
          Detect from website URL
        </Button>
      </Space.Compact>
    </Form.Item>
  )
}
