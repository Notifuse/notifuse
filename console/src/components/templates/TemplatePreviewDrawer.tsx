import React, { useState, useEffect, useRef } from 'react'
import { Drawer, Typography, Spin, Alert, Tabs } from 'antd'
import type { Template, MjmlCompileError, Workspace } from '../../services/api/types'
import { templatesApi } from '../../services/api/template'
import { BlockInterface } from '../email_editor/Block' // Assuming BlockInterface is here
import { usePrismjs } from '../email_editor/UI/Widgets/PrismJS'

const { Text } = Typography

interface TemplatePreviewDrawerProps {
  record: Template
  workspace: Workspace
  templateData?: Record<string, any>
  children: React.ReactNode
}

const TemplatePreviewDrawer: React.FC<TemplatePreviewDrawerProps> = ({
  record,
  workspace,
  templateData,
  children
}) => {
  const [previewHtml, setPreviewHtml] = useState<string | null>(null)
  const [previewMjml, setPreviewMjml] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState<boolean>(false)
  const [error, setError] = useState<string | null>(null)
  const [mjmlError, setMjmlError] = useState<MjmlCompileError | null>(null)
  const [isOpen, setIsOpen] = useState<boolean>(false)
  const [activeTabKey, setActiveTabKey] = useState<string>('1') // State for active tab

  const preRef = useRef<HTMLPreElement>(null)
  usePrismjs(preRef, ['line-numbers'])

  // Removed usePrismjs hook call

  const fetchPreview = async () => {
    if (!workspace.id || !record.email?.visual_editor_tree) {
      setError('Missing workspace ID or template data.')
      setMjmlError(null)
      setPreviewMjml(null)
      setPreviewHtml(null)
      return
    }

    setIsLoading(true)
    setError(null)
    setMjmlError(null)
    setPreviewHtml(null)
    setPreviewMjml(null)
    setActiveTabKey('1') // Reset to HTML tab on new fetch

    try {
      let treeObject: BlockInterface | null = null
      if (record.email?.visual_editor_tree && typeof record.email.visual_editor_tree === 'string') {
        try {
          treeObject = JSON.parse(record.email.visual_editor_tree)
        } catch (parseError) {
          console.error('Failed to parse visual_editor_tree:', parseError)
          setError('Invalid template structure data.')
          setMjmlError(null)
          setPreviewMjml(null)
          setIsLoading(false)
          return
        }
      } else if (record.email?.visual_editor_tree) {
        treeObject = record.email.visual_editor_tree as unknown as BlockInterface
      }

      if (!treeObject) {
        setError('Template structure data is missing or invalid.')
        setMjmlError(null)
        setPreviewMjml(null)
        setIsLoading(false)
        return
      }

      const req = {
        workspace_id: workspace.id,
        message_id: 'preview',
        visual_editor_tree: treeObject as any,
        test_data: templateData || record.test_data || {}
      }

      // console.log('Compile Request:', req)
      const response = await templatesApi.compile(req)
      // console.log('Compile Response:', response)

      if (response.error) {
        setMjmlError(response.error)
        setPreviewMjml(response.mjml)
        setError(null)
        setPreviewHtml(null)
      } else {
        setPreviewHtml(response.html)
        setPreviewMjml(response.mjml)
        setError(null)
        setMjmlError(null)
      }
    } catch (err: any) {
      console.error('Compile Error:', err)
      const errorMsg =
        err.response?.data?.error || err.message || 'Failed to compile template preview.'
      setError(errorMsg)
      setMjmlError(null)
      setPreviewMjml(null)
    } finally {
      setIsLoading(false)
    }
  }

  useEffect(() => {
    if (isOpen && workspace.id) {
      fetchPreview()
    } else if (!isOpen) {
      // Reset state when drawer closes to avoid showing stale data briefly on reopen
      setPreviewHtml(null)
      setPreviewMjml(null)
      setError(null)
      setMjmlError(null)
      setIsLoading(false)
      setActiveTabKey('1')
    }
  }, [isOpen, record.id, record.version, workspace.id]) // Keep original dependencies

  const items = []

  if (previewHtml) {
    items.push({
      key: '1',
      label: 'HTML Preview',
      children: (
        <iframe
          srcDoc={previewHtml}
          className="w-full h-full border-0"
          style={{ height: '600px', width: '100%' }}
          title={`HTML Preview of ${record.name}`}
          sandbox="allow-same-origin"
        />
      )
    })
  }

  if (previewMjml) {
    items.push({
      key: '2',
      label: 'MJML Source',
      children: <MJMLPreview previewMjml={previewMjml} />
    })
  }

  // Add Template Data tab regardless of preview status
  const testData = templateData || record.test_data || {}
  items.push({
    key: '3',
    label: 'Template Data',
    children: <JsonDataViewer data={testData} />
  })

  const emailProvider = workspace.integrations?.find(
    (i) =>
      i.id ===
      (record.category === 'marketing'
        ? workspace.settings?.marketing_email_provider_id
        : workspace.settings?.transactional_email_provider_id)
  )?.email_provider

  const defaultSender = emailProvider?.senders.find((s) => s.is_default)
  const templateSender = emailProvider?.senders.find((s) => s.id === record.email?.sender_id)

  const drawerContent = (
    <div>
      {/* Header details */}
      <div className="mb-4 space-y-2">
        <div>
          <Text strong>From: </Text>
          {templateSender ? (
            <>
              <Text>
                {templateSender.name}
                <Text type="secondary"> &lt;{templateSender.email}&gt;</Text>
              </Text>
            </>
          ) : (
            <>
              {defaultSender ? (
                <Text>
                  {defaultSender.name}
                  <Text type="secondary"> &lt;{defaultSender.email}&gt;</Text>
                </Text>
              ) : (
                <Text>No default sender configured</Text>
              )}
            </>
          )}
        </div>
        {record.email?.reply_to && (
          <div>
            <Text strong>Reply to: </Text>
            <Text type="secondary">{record.email.reply_to}</Text>
          </div>
        )}
        <div>
          <Text strong>Subject: </Text>
          <Text>{record.email?.subject}</Text>
        </div>
        {record.email?.subject_preview && (
          <div>
            <Text strong>Subject preview: </Text>
            <Text type="secondary">{record.email.subject_preview}</Text>
          </div>
        )}
      </div>
      {/* Main content area */}
      <div className="flex flex-col">
        {isLoading && (
          <div className="flex items-center justify-center flex-grow">
            <Spin size="large" />
          </div>
        )}
        {!isLoading &&
          error &&
          !mjmlError && ( // General error (not MJML compilation error)
            <div className="p-4">
              <Alert message="Error loading preview" description={error} type="error" showIcon />
            </div>
          )}
        {!isLoading && mjmlError && (
          // MJML Compilation Error
          <div className="p-4 overflow-auto flex-grow flex flex-col">
            <Alert
              message={`MJML Compilation Error: ${mjmlError.message}`}
              type="error"
              showIcon
              description={
                mjmlError.details && mjmlError.details.length > 0 ? (
                  <ul className="list-disc list-inside mt-2 text-xs">
                    {mjmlError.details.map((detail, index) => (
                      <li key={index}>
                        Line {detail.line} ({detail.tagName}): {detail.message}
                      </li>
                    ))}
                  </ul>
                ) : (
                  'No specific details provided.'
                )
              }
              className="mb-4 flex-shrink-0" // Prevent alert from growing too large
            />
          </div>
        )}
        {!isLoading &&
          items.length > 0 && ( // Success case
            <Tabs
              activeKey={activeTabKey} // Control active tab
              onChange={setActiveTabKey} // Update state on tab change (onChange is preferred over onTabClick for controlled Tabs)
              className="flex flex-col flex-grow"
              items={items}
              destroyInactiveTabPane={false}
            />
          )}
        {!isLoading &&
          !error &&
          !mjmlError &&
          !previewHtml &&
          !previewMjml &&
          items.length === 0 && ( // Neither success nor error, initial or no data state
            <div className="flex items-center justify-center flex-grow text-gray-500">
              No preview available or template is empty.
            </div>
          )}
      </div>
    </div>
  )

  return (
    <>
      <div onClick={() => setIsOpen(true)}>{children}</div>
      <Drawer
        title={`${record.name}`}
        placement="right"
        width={650}
        open={isOpen}
        onClose={() => setIsOpen(false)}
        destroyOnClose={true}
        maskClosable={true}
        mask={true}
        keyboard={true}
        forceRender={false}
      >
        {drawerContent}
      </Drawer>
    </>
  )
}

const JsonDataViewer = ({ data }: { data: any }) => {
  const codeRef = useRef<HTMLDivElement>(null)
  // Apply syntax highlighting
  usePrismjs(codeRef, ['line-numbers'])

  const prettyJson = JSON.stringify(data, null, 2)

  return (
    <div ref={codeRef} className="rounded" style={{ maxWidth: '100%' }}>
      <pre
        className="line-numbers"
        style={{
          margin: '0',
          borderRadius: '4px',
          padding: '10px',
          fontSize: '12px',
          wordWrap: 'break-word',
          whiteSpace: 'pre-wrap',
          wordBreak: 'normal'
        }}
      >
        <code className="language-json">{prettyJson}</code>
      </pre>
    </div>
  )
}

const MJMLPreview = ({ previewMjml }: { previewMjml: string }) => {
  const preRef = useRef<HTMLPreElement>(null)
  usePrismjs(preRef, ['line-numbers'])

  return (
    <div className="overflow-auto">
      <pre
        ref={preRef}
        className="language-xml"
        style={{
          fontSize: '12px',
          margin: 0,
          padding: '10px',
          wordWrap: 'break-word',
          whiteSpace: 'pre-wrap',
          wordBreak: 'normal'
        }}
      >
        <code className="language-xml">{previewMjml}</code>
      </pre>
    </div>
  )
}

export default TemplatePreviewDrawer
