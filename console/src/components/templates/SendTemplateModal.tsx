import { useState, useEffect } from 'react'
import { Modal, Button, Input, Select, Typography, Form, App, Upload, Space, Tag } from 'antd'
import { UploadOutlined, DeleteOutlined } from '@ant-design/icons'
import { Workspace, Template, Integration } from '../../services/api/types'
import {
  transactionalNotificationsApi,
  Attachment
} from '../../services/api/transactional_notifications'
import { emailProviders } from '../integrations/EmailProviders'

const { Text } = Typography
const { Option } = Select

interface SendTemplateModalProps {
  isOpen: boolean
  onClose: () => void
  template: Template | null
  workspace: Workspace | null
  loading?: boolean
  withCCAndBCC?: boolean
}

export default function SendTemplateModal({
  isOpen,
  onClose,
  template,
  workspace,
  loading = false,
  withCCAndBCC = false
}: SendTemplateModalProps) {
  const [email, setEmail] = useState('')
  const [selectedIntegrationId, setSelectedIntegrationId] = useState<string>('')
  const [selectedSenderId, setSelectedSenderId] = useState<string>('')
  const [sendLoading, setSendLoading] = useState(false)
  const [fromName, setFromName] = useState<string>('')
  const [ccEmails, setCcEmails] = useState<string[]>([])
  const [bccEmails, setBccEmails] = useState<string[]>([])
  const [replyTo, setReplyTo] = useState<string>('')
  const [attachments, setAttachments] = useState<Attachment[]>([])
  const [showAdvancedOptions, setShowAdvancedOptions] = useState(false)
  const [form] = Form.useForm()
  const { message } = App.useApp()

  // Filter to only email integrations
  const emailIntegrations =
    workspace?.integrations?.filter(
      (integration) => integration.type === 'email' && integration.email_provider?.kind
    ) || []

  // Set default integration when modal opens or template changes
  useEffect(() => {
    if (isOpen && workspace && emailIntegrations.length > 0 && !selectedIntegrationId) {
      const defaultId =
        template?.category === 'marketing'
          ? workspace.settings?.marketing_email_provider_id
          : workspace.settings?.transactional_email_provider_id

      // Use the appropriate default or the first available integration
      setSelectedIntegrationId(
        defaultId && emailIntegrations.some((i) => i.id === defaultId)
          ? defaultId
          : emailIntegrations[0]?.id || ''
      )

      // set first sender from email integration
      setSelectedSenderId(emailIntegrations[0]?.email_provider?.senders[0]?.id || '')
    }
  }, [isOpen, template, workspace, emailIntegrations, selectedIntegrationId])

  // Reset form when modal opens
  useEffect(() => {
    if (isOpen) {
      setEmail('')
      setFromName('')
      setCcEmails([])
      setBccEmails([])
      setReplyTo('')
      setAttachments([])
      setShowAdvancedOptions(false)
      form.resetFields()
    }
  }, [isOpen, form, withCCAndBCC])

  const handleSend = async () => {
    if (!template || !workspace || !selectedIntegrationId) return

    setSendLoading(true)
    try {
      const response = await transactionalNotificationsApi.testTemplate(
        workspace.id,
        template.id,
        selectedIntegrationId,
        selectedSenderId,
        email,
        {
          from_name: fromName || undefined,
          cc: ccEmails,
          bcc: bccEmails,
          reply_to: replyTo,
          attachments: attachments.length > 0 ? attachments : undefined
        }
      )

      if (response.success) {
        message.success('Test email sent successfully')
        onClose()
      } else {
        message.error(`Failed to send test email: ${response.error || 'Unknown error'}`)
      }
    } catch (error: any) {
      const errorMessage =
        error?.response?.status === 400 && error?.response?.data?.message
          ? error.response.data.message
          : error?.message || 'Something went wrong'
      message.error(`Error: ${errorMessage}`)
    } finally {
      setSendLoading(false)
    }
  }

  // Convert file to base64
  const fileToBase64 = (file: File): Promise<string> => {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.readAsDataURL(file)
      reader.onload = () => {
        const base64 = reader.result as string
        // Remove the data URL prefix (e.g., "data:image/png;base64,")
        const base64Content = base64.split(',')[1]
        resolve(base64Content)
      }
      reader.onerror = (error) => reject(error)
    })
  }

  // Handle file upload
  const handleFileUpload = async (file: File) => {
    try {
      // Check file size (3MB limit per file)
      const maxSize = 3 * 1024 * 1024
      if (file.size > maxSize) {
        message.error(`File ${file.name} exceeds 3MB limit`)
        return false
      }

      // Use functional form to get current state for validation
      let shouldAbort = false
      setAttachments((currentAttachments) => {
        // Check total attachments size (10MB total)
        const totalSize = currentAttachments.reduce((sum, att) => {
          // Approximate size from base64 (base64 is ~4/3 of original size)
          return sum + (att.content.length * 3) / 4
        }, 0)

        if (totalSize + file.size > 10 * 1024 * 1024) {
          message.error('Total attachments size exceeds 10MB limit')
          shouldAbort = true
          return currentAttachments
        }

        // Check maximum number of attachments
        if (currentAttachments.length >= 20) {
          message.error('Maximum 20 attachments allowed')
          shouldAbort = true
          return currentAttachments
        }

        return currentAttachments
      })

      if (shouldAbort) {
        return false
      }

      const base64Content = await fileToBase64(file)

      const newAttachment: Attachment = {
        filename: file.name,
        content: base64Content,
        content_type: file.type || 'application/octet-stream',
        disposition: 'attachment'
      }

      // Use functional form to ensure we're working with the latest state
      setAttachments((prev) => [...prev, newAttachment])
      message.success(`File ${file.name} added`)
      return false // Prevent default upload behavior
    } catch (error) {
      message.error(`Failed to process file ${file.name}`)
      return false
    }
  }

  // Remove attachment
  const removeAttachment = (index: number) => {
    setAttachments((prev) => prev.filter((_, i) => i !== index))
  }

  // Format file size for display
  const formatFileSize = (bytes: number): string => {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  const renderIntegrationOption = (integration: Integration) => {
    const providerKind = integration.email_provider?.kind
    // Find the provider info to get the icon
    const providerInfo = emailProviders.find((p) => p.kind === providerKind)

    return (
      <Option key={integration.id} value={integration.id}>
        <span className="mr-1">
          {providerInfo ? providerInfo.getIcon('mr-1') : <span className="h-5 w-5 inline-block" />}
        </span>
        <span>{integration.name}</span>
      </Option>
    )
  }

  const selectedIntegration = emailIntegrations.find(
    (integration) => integration.id === selectedIntegrationId
  )

  return (
    <Modal
      title="Send Test Email"
      open={isOpen}
      onCancel={onClose}
      footer={[
        <Button key="cancel" onClick={onClose}>
          Cancel
        </Button>,
        <Button
          key="send"
          type="primary"
          onClick={handleSend}
          disabled={!email || !selectedIntegrationId || loading || sendLoading}
          loading={loading || sendLoading}
        >
          Send Test Email
        </Button>
      ]}
      width={showAdvancedOptions ? 600 : 520}
    >
      <Form form={form} layout="vertical">
        <div className="py-2 space-y-4">
          <p>Send a test email using this template to verify how it will look.</p>

          <Form.Item label="Email Integration">
            <Select
              className="w-full"
              placeholder="Select an email integration"
              value={selectedIntegrationId}
              onChange={setSelectedIntegrationId}
              disabled={emailIntegrations.length === 0}
            >
              {emailIntegrations.map(renderIntegrationOption)}
            </Select>
            {emailIntegrations.length === 0 && (
              <Text type="warning" className="mt-1 block">
                No email integrations available. Please configure one in Settings.
              </Text>
            )}
          </Form.Item>

          <Form.Item label="Sender">
            <Select
              className="w-full"
              placeholder="Select a sender"
              value={selectedSenderId}
              onChange={setSelectedSenderId}
              options={selectedIntegration?.email_provider?.senders.map((sender) => ({
                label: `${sender.name} <${sender.email}>`,
                value: sender.id
              }))}
            />
          </Form.Item>

          <Form.Item label="Recipient Email" required>
            <Input
              placeholder="recipient@example.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              type="email"
            />
          </Form.Item>

          {!showAdvancedOptions && (
            <Button type="link" onClick={() => setShowAdvancedOptions(true)} className="!p-0">
              + add from name, CC, BCC, reply-to, attachments
            </Button>
          )}

          {showAdvancedOptions && (
            <>
              <Form.Item label="From Name (override)">
                <Input
                  placeholder="Custom sender name (optional)"
                  value={fromName}
                  onChange={(e) => setFromName(e.target.value)}
                  allowClear
                />
                <Text type="secondary" className="text-xs mt-1 block">
                  Override the default sender name for this test email
                </Text>
              </Form.Item>

              <Form.Item label="CC Recipients">
                <Select
                  mode="tags"
                  placeholder="Enter CC email addresses"
                  value={ccEmails}
                  onChange={setCcEmails}
                  tokenSeparators={[',', ' ']}
                  allowClear
                />
              </Form.Item>

              <Form.Item label="BCC Recipients">
                <Select
                  mode="tags"
                  placeholder="Enter BCC email addresses"
                  value={bccEmails}
                  onChange={setBccEmails}
                  tokenSeparators={[',', ' ']}
                  allowClear
                />
              </Form.Item>

              <Form.Item label="Reply-To">
                <Input
                  placeholder="Enter Reply-To email address"
                  value={replyTo}
                  onChange={(e) => setReplyTo(e.target.value)}
                  allowClear
                />
              </Form.Item>

              <Form.Item label="Attachments">
                <Upload beforeUpload={handleFileUpload} showUploadList={false} multiple>
                  <Button icon={<UploadOutlined />} disabled={attachments.length >= 20}>
                    Upload Files
                  </Button>
                </Upload>
                <Text type="secondary" className="text-xs mt-1 block">
                  Max 3MB per file, 10MB total, 20 files maximum
                </Text>
                {attachments.length > 0 && (
                  <Space direction="vertical" className="mt-2 w-full">
                    {attachments.map((att, index) => {
                      // Calculate approximate file size from base64
                      const sizeBytes = (att.content.length * 3) / 4
                      return (
                        <div
                          key={index}
                          className="flex items-center justify-between p-2 border border-gray-200 rounded"
                        >
                          <Space>
                            <Text>{att.filename}</Text>
                            <Tag>{formatFileSize(sizeBytes)}</Tag>
                          </Space>
                          <Button
                            type="text"
                            danger
                            size="small"
                            icon={<DeleteOutlined />}
                            onClick={() => removeAttachment(index)}
                          />
                        </div>
                      )
                    })}
                  </Space>
                )}
              </Form.Item>
            </>
          )}
        </div>
      </Form>
    </Modal>
  )
}
