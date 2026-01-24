import { Alert, Button, Col, Form, Input, Modal, Row, Select, Switch, Typography, message } from 'antd'
import { useState, useEffect } from 'react'
import { useForm } from 'antd/lib/form/Form'
import type { FileManagerSettings } from './interfaces'
import { ListObjectsV2Command, type ListObjectsV2CommandInput, S3Client } from '@aws-sdk/client-s3'
import { S3_PROVIDERS, getProviderById, generateEndpoint, type S3Provider } from './s3Providers'
import { useLingui } from '@lingui/react/macro'

const { Text } = Typography

interface ButtonFilesSettingsProps {
  children: React.ReactNode
  settings?: FileManagerSettings
  onUpdateSettings: (settings: FileManagerSettings) => Promise<void>
  settingsInfo?: React.ReactNode
}

type ScreenType = 'provider' | 'settings'

const ButtonFilesSettings = (props: ButtonFilesSettingsProps) => {
  const { t } = useLingui()
  const [loading, setLoading] = useState(false)
  const [form] = useForm()
  const [settingsVisible, setSettingsVisible] = useState(false)
  const [currentScreen, setCurrentScreen] = useState<ScreenType>('provider')
  const [selectedProvider, setSelectedProvider] = useState<S3Provider | null>(null)

  // Determine initial screen based on whether settings already exist
  const hasExistingSettings = props.settings?.endpoint && props.settings?.access_key

  useEffect(() => {
    if (!settingsVisible) return

    // Guard: Don't populate form if settings aren't loaded yet
    const hasSettings = props.settings?.endpoint || props.settings?.access_key

    if (hasSettings) {
      // Existing settings: go to settings screen
      const existingProvider = props.settings?.provider
        ? getProviderById(props.settings.provider)
        : null
      const resolvedProvider = existingProvider || getProviderById('other') || null
      setSelectedProvider(resolvedProvider)
      setCurrentScreen('settings')

      // Populate form with all settings
      form.setFieldsValue({
        provider: props.settings?.provider,
        endpoint: props.settings?.endpoint,
        region: props.settings?.region,
        access_key: props.settings?.access_key,
        secret_key: props.settings?.secret_key,
        bucket: props.settings?.bucket,
        cdn_endpoint: props.settings?.cdn_endpoint,
        force_path_style: props.settings?.force_path_style ?? resolvedProvider?.forcePathStyle ?? false
      })
    } else {
      // New config: start with provider selection
      setCurrentScreen('provider')
      setSelectedProvider(null)
      form.resetFields()
    }
  }, [settingsVisible, props.settings, form])

  const toggleSettings = () => {
    setSettingsVisible(!settingsVisible)
  }

  const handleProviderSelect = (provider: S3Provider) => {
    setSelectedProvider(provider)

    // Pre-fill form with provider defaults
    const defaultRegion = provider.defaultRegion || provider.regionPlaceholder || ''
    const generatedEndpoint = generateEndpoint(provider, defaultRegion)

    form.setFieldsValue({
      provider: provider.id,
      endpoint: generatedEndpoint || props.settings?.endpoint || '',
      region: defaultRegion || props.settings?.region || '',
      access_key: props.settings?.access_key || '',
      secret_key: props.settings?.secret_key || '',
      bucket: props.settings?.bucket || '',
      cdn_endpoint: props.settings?.cdn_endpoint || '',
      force_path_style: provider.forcePathStyle
    })

    setCurrentScreen('settings')
  }

  const handleBackToProviders = () => {
    setCurrentScreen('provider')
  }

  const handleRegionChange = (region: string) => {
    if (selectedProvider && selectedProvider.endpointTemplate.includes('{region}')) {
      const newEndpoint = generateEndpoint(selectedProvider, region)
      form.setFieldsValue({ endpoint: newEndpoint })
    }
  }

  const onFinish = () => {
    form
      .validateFields()
      .then((values: FileManagerSettings) => {
        if (loading) return

        setLoading(true)

        // check if the bucket can be reached
        const input: ListObjectsV2CommandInput = {
          Bucket: values.bucket || ''
        }
        const command = new ListObjectsV2Command(input)

        const s3Client = new S3Client({
          endpoint: values.endpoint || '',
          credentials: {
            accessKeyId: values.access_key || '',
            secretAccessKey: values.secret_key || ''
          },
          region: values.region || 'us-east-1',
          forcePathStyle: values.force_path_style ?? false
        })

        if (values.region === '') {
          delete values.region
        }
        if (values.cdn_endpoint === '') {
          delete values.cdn_endpoint
        }

        s3Client
          .send(command)
          .then(() => {
            props
              .onUpdateSettings(values)
              .then(() => {
                message.success(t`The workspace settings have been updated!`)
                setLoading(false)
                toggleSettings()
              })
              .catch((error) => {
                console.error(error)
                message.error(error.toString())
                setLoading(false)
              })
          })
          .catch((e: Error) => {
            console.error(e)
            message.error(e.toString())
            setLoading(false)
          })
      })
      .catch((e: Error) => {
        console.error(e)
        message.error(e.toString())
        setLoading(false)
      })
  }

  const renderProviderSelection = () => (
    <div>
      <Text style={{ display: 'block', marginBottom: 16 }}>
        {t`Select your storage provider to pre-fill the configuration:`}
      </Text>
      <Row gutter={[12, 12]}>
        {S3_PROVIDERS.map((provider) => (
          <Col span={12} key={provider.id}>
            <div
              onClick={() => handleProviderSelect(provider)}
              className="flex items-center gap-3 p-3 bg-white border border-gray-200 rounded-lg cursor-pointer transition-all hover:border-primary"
            >
              <img
                src={provider.logo}
                alt={provider.name}
                className="w-6 h-6 flex-shrink-0"
              />
              <Text strong style={{ fontSize: 13 }}>
                {provider.name}
              </Text>
            </div>
          </Col>
        ))}
      </Row>
    </div>
  )

  const renderSettingsForm = () => (
    <div>
      {hasExistingSettings && (
        <Button type="link" onClick={handleBackToProviders} style={{ padding: 0, marginBottom: 16 }}>
          &larr; {t`Change provider`}
        </Button>
      )}

      {props.settingsInfo}

      <Form
        form={form}
        layout="horizontal"
        labelCol={{ span: 6 }}
        wrapperCol={{ span: 18 }}
        style={{ marginTop: hasExistingSettings ? 0 : 24, marginBottom: 40 }}
        onFinish={onFinish}
      >
        <Form.Item name="provider" hidden>
          <Input />
        </Form.Item>


        <Alert
          message={
            selectedProvider
              ? t`Configuring ${selectedProvider.name}`
              : t`Your files can be uploaded to any S3 compatible storage.`
          }
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          banner
        />

        <Form.Item
          label={t`S3 Endpoint`}
          name="endpoint"
          rules={[{ type: 'url', required: true }]}
          help={selectedProvider?.endpointHelp}
        >
          <Input placeholder={selectedProvider?.endpointPlaceholder || 'https://storage.googleapis.com'} />
        </Form.Item>

        {selectedProvider?.regionOptions ? (
          <Form.Item
            label={t`S3 region`}
            name="region"
            rules={[{ type: 'string', required: selectedProvider?.regionRequired }]}
          >
            <Select
              placeholder={selectedProvider?.regionPlaceholder || 'us-east-1'}
              onChange={handleRegionChange}
              allowClear={!selectedProvider?.regionRequired}
            >
              {selectedProvider.regionOptions.map((region) => (
                <Select.Option key={region} value={region}>
                  {region}
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
        ) : (
          <Form.Item
            label={t`S3 region`}
            name="region"
            rules={[{ type: 'string', required: selectedProvider?.regionRequired || false }]}
          >
            <Input
              placeholder={selectedProvider?.regionPlaceholder || 'us-east-1'}
              onChange={(e) => handleRegionChange(e.target.value)}
            />
          </Form.Item>
        )}

        <Form.Item label={t`S3 access key`} name="access_key" rules={[{ type: 'string', required: true }]}>
          <Input />
        </Form.Item>

        <Form.Item label={t`S3 secret key`} name="secret_key" rules={[{ type: 'string', required: true }]}>
          <Input type="password" />
        </Form.Item>

        <Form.Item label={t`S3 bucket`} name="bucket" rules={[{ type: 'string', required: true }]}>
          <Input />
        </Form.Item>

        <Form.Item
          label={t`Path-style access`}
          name="force_path_style"
          valuePropName="checked"
          help={t`Use path-style URLs (bucket in path instead of subdomain)`}
        >
          <Switch disabled={selectedProvider ? !selectedProvider.showForcePathStyle : false} />
        </Form.Item>

        <Form.Item
          label={t`CDN endpoint`}
          name="cdn_endpoint"
          help={t`URL of the CDN that caches your files`}
          rules={[{ type: 'url', required: false }]}
        >
          <Input placeholder="https://cdn.yourbusiness.com" />
        </Form.Item>
      </Form>
    </div>
  )

  const getModalTitle = () => {
    if (currentScreen === 'provider') {
      return t`Select storage provider`
    }
    return t`File storage settings`
  }

  const getModalFooter = () => {
    if (currentScreen === 'provider') {
      return null
    }
    return [
      <Button key="cancel" loading={loading} onClick={toggleSettings}>
        {t`Cancel`}
      </Button>,
      <Button key="submit" loading={loading} type="primary" onClick={onFinish}>
        {t`Save`}
      </Button>
    ]
  }

  return (
    <span>
      <span onClick={toggleSettings}>{props.children}</span>
      <Modal
        title={getModalTitle()}
        open={settingsVisible}
        onCancel={toggleSettings}
        footer={getModalFooter()}
        width={currentScreen === 'provider' ? 520 : 600}
      >
        {currentScreen === 'provider' ? renderProviderSelection() : renderSettingsForm()}
      </Modal>
    </span>
  )
}

export default ButtonFilesSettings
