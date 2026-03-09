import React, { useState, useEffect, useMemo } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Drawer, Typography, Spin, Alert, Tabs, Tag, Space, Descriptions, Segmented } from 'antd'
import type { Template, MjmlCompileError, Workspace } from '../../services/api/types'
import { templatesApi } from '../../services/api/template'
import type { CompileTemplateRequest } from '../../services/api/template'
import type { EmailBlock } from '../email_builder/types'
import { Highlight, themes } from 'prism-react-renderer'
import { Liquid } from 'liquidjs'
import type { MessageHistory } from '../../services/api/messages_history'
import { SUPPORTED_LANGUAGES } from '../../lib/languages'

const { Text } = Typography

interface TemplatePreviewDrawerProps {
  record: Template
  workspace: Workspace
  templateData?: Record<string, unknown>
  messageHistory?: MessageHistory
  children: React.ReactNode
}

const TemplatePreviewDrawer: React.FC<TemplatePreviewDrawerProps> = ({
  record,
  workspace,
  templateData,
  messageHistory,
  children
}) => {
  const { t } = useLingui()
  const [previewHtml, setPreviewHtml] = useState<string | null>(null)
  const [previewMjml, setPreviewMjml] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState<boolean>(false)
  const [error, setError] = useState<string | null>(null)
  const [mjmlError, setMjmlError] = useState<MjmlCompileError | null>(null)
  const [isOpen, setIsOpen] = useState<boolean>(false)
  const [activeTabKey, setActiveTabKey] = useState<string>('1') // State for active tab
  const [processedSubject, setProcessedSubject] = useState<string | null>(null)
  const [selectedLanguage, setSelectedLanguage] = useState<string | null>(null)

  const availableLanguages = useMemo(() => {
    if (messageHistory) return []
    const defaultLang = workspace.settings?.default_language || 'en'
    const langs: { label: string; value: string }[] = [
      { label: SUPPORTED_LANGUAGES[defaultLang] || defaultLang, value: defaultLang }
    ]
    if (record.translations) {
      for (const [code, translation] of Object.entries(record.translations)) {
        if (
          code !== defaultLang &&
          translation.email &&
          (translation.email.visual_editor_tree || translation.email.mjml_source)
        ) {
          langs.push({ label: SUPPORTED_LANGUAGES[code] || code, value: code })
        }
      }
    }
    return langs
  }, [record.translations, workspace.settings?.default_language, messageHistory])

  const showLanguageSelector = availableLanguages.length > 1
  const effectiveLanguage = selectedLanguage || workspace.settings?.default_language || 'en'

  const effectiveEmail = useMemo(() => {
    const defaultLang = workspace.settings?.default_language || 'en'
    if (effectiveLanguage === defaultLang) return record.email
    return record.translations?.[effectiveLanguage]?.email || record.email
  }, [effectiveLanguage, record.email, record.translations, workspace.settings?.default_language])

  const fetchPreview = async () => {
    const isCodeMode = effectiveEmail?.editor_mode === 'code'

    if (!workspace.id || (!isCodeMode && !effectiveEmail?.visual_editor_tree) || (isCodeMode && !effectiveEmail?.mjml_source)) {
      setError(t`Missing workspace ID or template data.`)
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
      // Build compile request based on editor mode
      const req: Partial<CompileTemplateRequest> = {
        workspace_id: workspace.id,
        message_id: 'preview',
        test_data: templateData || record.test_data || {},
        tracking_settings: {
          enable_tracking: workspace.settings?.email_tracking_enabled || false,
          endpoint: workspace.settings?.custom_endpoint_url || undefined,
          workspace_id: workspace.id,
          message_id: 'preview'
        }
      }

      if (isCodeMode) {
        // Code mode: use mjml_source directly
        req.mjml_source = effectiveEmail!.mjml_source
      } else {
        // Visual mode: parse visual_editor_tree
        let treeObject: EmailBlock | null = null
        if (effectiveEmail?.visual_editor_tree && typeof effectiveEmail.visual_editor_tree === 'string') {
          try {
            treeObject = JSON.parse(effectiveEmail.visual_editor_tree)
          } catch (parseError) {
            console.error('Failed to parse visual_editor_tree:', parseError)
            setError(t`Invalid template structure data.`)
            setMjmlError(null)
            setPreviewMjml(null)
            setIsLoading(false)
            return
          }
        } else if (effectiveEmail?.visual_editor_tree) {
          treeObject = effectiveEmail.visual_editor_tree as unknown as EmailBlock
        }

        if (!treeObject) {
          setError(t`Template structure data is missing or invalid.`)
          setMjmlError(null)
          setPreviewMjml(null)
          setIsLoading(false)
          return
        }

        req.visual_editor_tree = treeObject
      }

      // console.log('Compile Request:', req)
      const response = await templatesApi.compile(req as CompileTemplateRequest)
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
    } catch (err) {
      console.error('Compile Error:', err)
      const error = err as { response?: { data?: { error?: string } }; message?: string }
      const errorMsg =
        error.response?.data?.error || error.message || t`Failed to compile template preview.`
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
      setProcessedSubject(null)
      setSelectedLanguage(null)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- fetchPreview is stable
  }, [isOpen, record.id, record.version, workspace.id, effectiveLanguage])

  // Process subject with Liquid using provided template data
  useEffect(() => {
    if (!isOpen) return

    const subject = effectiveEmail?.subject || ''
    const data = templateData || record.test_data || {}

    try {
      if (subject && (subject.includes('{{') || subject.includes('{%'))) {
        const engine = new Liquid()
        const rendered = engine.parseAndRenderSync(subject, data)
        setProcessedSubject(rendered)
      } else {
        setProcessedSubject(subject)
      }
    } catch {
      // Fallback to raw subject on any rendering error
      setProcessedSubject(subject)
    }
  }, [isOpen, effectiveEmail?.subject, templateData, record.test_data])

  const items = []

  if (previewHtml) {
    items.push({
      key: '1',
      label: t`HTML Preview`,
      children: (
        <iframe
          srcDoc={previewHtml}
          className="w-full h-full border-0"
          style={{ height: '600px', width: '100%' }}
          title={t`HTML Preview of ${record.name}`}
          sandbox=""
        />
      )
    })
  }

  if (previewMjml) {
    items.push({
      key: '2',
      label: t`MJML Source`,
      children: <MJMLPreview previewMjml={previewMjml} />
    })
  }

  // Add Template Data tab regardless of preview status
  const testData = templateData || record.test_data || {}
  items.push({
    key: '3',
    label: t`Template Data`,
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
      <Descriptions bordered={false} size="small" column={1} className="mb-4">
        <Descriptions.Item label={t`From`}>
          {messageHistory?.channel_options?.from_name ? (
            <>
              <Text>
                {messageHistory.channel_options.from_name} &lt;
                {templateSender?.email || defaultSender?.email || t`no email`}&gt;
              </Text>
              {(templateSender || defaultSender) && (
                <Text type="secondary" className="text-xs pl-2">
                  {t`(original: ${templateSender?.name || defaultSender?.name})`}
                </Text>
              )}
            </>
          ) : (
            <>
              {templateSender ? (
                <>
                  <Text>
                    {templateSender.name}
                    <Text> &lt;{templateSender.email}&gt;</Text>
                  </Text>
                </>
              ) : (
                <>
                  {defaultSender ? (
                    <Text>
                      {defaultSender.name}
                      <Text> &lt;{defaultSender.email}&gt;</Text>
                    </Text>
                  ) : (
                    <Text>{t`No default sender configured`}</Text>
                  )}
                </>
              )}
            </>
          )}
        </Descriptions.Item>

        {(record.email?.reply_to || messageHistory?.channel_options?.reply_to) && (
          <Descriptions.Item label={t`Reply to`}>
            {messageHistory?.channel_options?.reply_to ? (
              <>
                <Text>{messageHistory.channel_options.reply_to}</Text>
                {record.email?.reply_to && (
                  <Text type="secondary" className="text-xs pl-2">
                    {t`(original: ${record.email.reply_to})`}
                  </Text>
                )}
              </>
            ) : (
              <Text>{record.email?.reply_to || t`Not set`}</Text>
            )}
          </Descriptions.Item>
        )}

        <Descriptions.Item label={t`Subject`}>
          <Text>{processedSubject ?? effectiveEmail?.subject}</Text>
        </Descriptions.Item>

        {effectiveEmail?.subject_preview && (
          <Descriptions.Item label={t`Subject preview`}>
            <Text>{effectiveEmail.subject_preview}</Text>
          </Descriptions.Item>
        )}

        {/* Channel Options Display - CC and BCC */}
        {messageHistory?.channel_options?.cc && messageHistory.channel_options.cc.length > 0 && (
          <Descriptions.Item label={t`CC`}>
            <Space size={[0, 4]} wrap>
              {messageHistory.channel_options.cc.map((email, idx) => (
                <Tag bordered={false} key={idx} color="blue" className="text-xs">
                  {email}
                </Tag>
              ))}
            </Space>
          </Descriptions.Item>
        )}

        {messageHistory?.channel_options?.bcc && messageHistory.channel_options.bcc.length > 0 && (
          <Descriptions.Item label={t`BCC`}>
            <Space size={[0, 4]} wrap>
              {messageHistory.channel_options.bcc.map((email, idx) => (
                <Tag bordered={false} key={idx} color="purple" className="text-xs">
                  {email}
                </Tag>
              ))}
            </Space>
          </Descriptions.Item>
        )}
        {showLanguageSelector && (
          <Descriptions.Item label={t`Language`}>
            <Segmented
              size="small"
              value={effectiveLanguage}
              onChange={(value) => setSelectedLanguage(value as string)}
              options={availableLanguages}
            />
          </Descriptions.Item>
        )}
      </Descriptions>
      {/* Main content area */}
      <div className="flex flex-col mt-4">
        {isLoading && (
          <div className="flex items-center justify-center flex-grow">
            <Spin size="large" />
          </div>
        )}
        {!isLoading &&
          error &&
          !mjmlError && ( // General error (not MJML compilation error)
            <div className="p-4">
              <Alert message={t`Error loading preview`} description={error} type="error" showIcon />
            </div>
          )}
        {!isLoading && mjmlError && (
          // MJML Compilation Error
          <div className="p-4 overflow-auto flex-grow flex flex-col">
            <Alert
              message={t`MJML Compilation Error: ${mjmlError.message}`}
              type="error"
              showIcon
              description={
                mjmlError.details && mjmlError.details.length > 0 ? (
                  <ul className="list-disc list-inside mt-2 text-xs">
                    {mjmlError.details.map((detail, index) => (
                      <li key={index}>
                        {t`Line ${detail.line} (${detail.tagName}): ${detail.message}`}
                      </li>
                    ))}
                  </ul>
                ) : (
                  t`No specific details provided.`
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
              destroyOnHidden={false}
            />
          )}
        {!isLoading &&
          !error &&
          !mjmlError &&
          !previewHtml &&
          !previewMjml &&
          items.length === 0 && ( // Neither success nor error, initial or no data state
            <div className="flex items-center justify-center flex-grow text-gray-500">
              {t`No preview available or template is empty.`}
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

const JsonDataViewer = ({ data }: { data: Record<string, unknown> }) => {
  const prettyJson = JSON.stringify(data, null, 2)

  return (
    <div className="rounded" style={{ maxWidth: '100%' }}>
      <Highlight theme={themes.github} code={prettyJson} language="json">
        {({ className, style, tokens, getLineProps, getTokenProps }) => (
          <pre
            className={className}
            style={{
              ...style,
              margin: '0',
              borderRadius: '4px',
              padding: '10px',
              fontSize: '12px',
              wordWrap: 'break-word',
              whiteSpace: 'pre-wrap',
              wordBreak: 'normal'
            }}
          >
            {tokens.map((line, i) => (
              <div key={i} {...getLineProps({ line })}>
                <span
                  style={{
                    display: 'inline-block',
                    width: '2em',
                    userSelect: 'none',
                    opacity: 0.3
                  }}
                >
                  {i + 1}
                </span>
                {line.map((token, key) => (
                  <span key={key} {...getTokenProps({ token })} />
                ))}
              </div>
            ))}
          </pre>
        )}
      </Highlight>
    </div>
  )
}

const MJMLPreview = ({ previewMjml }: { previewMjml: string }) => {
  return (
    <div className="overflow-auto">
      <Highlight theme={themes.github} code={previewMjml} language="xml">
        {({ className, style, tokens, getLineProps, getTokenProps }) => (
          <pre
            className={className}
            style={{
              ...style,
              fontSize: '12px',
              margin: 0,
              padding: '10px',
              wordWrap: 'break-word',
              whiteSpace: 'pre-wrap',
              wordBreak: 'normal'
            }}
          >
            {tokens.map((line, i) => (
              <div key={i} {...getLineProps({ line })}>
                <span
                  style={{
                    display: 'inline-block',
                    width: '2em',
                    userSelect: 'none',
                    opacity: 0.3
                  }}
                >
                  {i + 1}
                </span>
                {line.map((token, key) => (
                  <span key={key} {...getTokenProps({ token })} />
                ))}
              </div>
            ))}
          </pre>
        )}
      </Highlight>
    </div>
  )
}

export default TemplatePreviewDrawer
