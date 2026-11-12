import React, { useState, useEffect, useMemo } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Drawer, Typography, Spin, Alert, Tabs, Tag, Space, Descriptions, Segmented } from 'antd'
import type { Template, MjmlCompileError, Workspace } from '../../services/api/types'
import { templatesApi } from '../../services/api/template'
import { buildTemplatePreviewRequest } from './compileTemplatePreview'
import { Highlight, themes } from 'prism-react-renderer'
import type { MessageHistory } from '../../services/api/messages_history'
import { SUPPORTED_LANGUAGES } from '../../lib/languages'

const { Text } = Typography

interface TemplatePreviewDrawerProps {
  record: Template
  workspace: Workspace
  templateData?: Record<string, unknown>
  messageHistory?: MessageHistory
  /** Opens the drawer on click; not needed in controlled mode. */
  children?: React.ReactNode
  /** Controlled mode, for a page that opens the drawer itself; onClose asks it to close. */
  open?: boolean
  onClose?: () => void
}

const TemplatePreviewDrawer: React.FC<TemplatePreviewDrawerProps> = ({
  record,
  workspace,
  templateData,
  messageHistory,
  children,
  open,
  onClose
}) => {
  const { t } = useLingui()
  const [previewHtml, setPreviewHtml] = useState<string | null>(null)
  const [previewMjml, setPreviewMjml] = useState<string | null>(null)
  // Plain-text version sent with the email; null when the compile returned none.
  const [previewText, setPreviewText] = useState<string | null>(null)
  // Where the previewed text comes from, and why a custom text was not used.
  const [previewTextSource, setPreviewTextSource] = useState<'auto' | 'custom' | null>(null)
  const [previewTextError, setPreviewTextError] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState<boolean>(false)
  const [error, setError] = useState<string | null>(null)
  const [mjmlError, setMjmlError] = useState<MjmlCompileError | null>(null)
  const [ownOpen, setOwnOpen] = useState<boolean>(false)
  const controlled = open !== undefined
  const isOpen = controlled ? open : ownOpen
  const close = () => (controlled ? onClose?.() : setOwnOpen(false))
  const [activeTabKey, setActiveTabKey] = useState<string>('1') // State for active tab
  const [renderedSubject, setRenderedSubject] = useState<string | null>(null)
  const [renderedSubjectPreview, setRenderedSubjectPreview] = useState<string | null>(null)
  const [selectedLanguage, setSelectedLanguage] = useState<string | null>(null)
  // Effective template data returned by the compile endpoint (includes the workspace
  // object the server injects). Displayed in the Template Data tab so it matches the render.
  const [effectiveTestData, setEffectiveTestData] = useState<Record<string, unknown> | null>(null)

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
    const built = workspace.id
      ? buildTemplatePreviewRequest({
          workspace,
          email: effectiveEmail,
          testData: templateData || record.test_data
        })
      : ({ error: 'missing_content' } as const)

    if ('error' in built && built.error === 'missing_content') {
      setError(t`Missing workspace ID or template data.`)
      setMjmlError(null)
      setPreviewMjml(null)
      setPreviewHtml(null)
      setPreviewText(null)
      return
    }

    setIsLoading(true)
    setError(null)
    setMjmlError(null)
    setPreviewHtml(null)
    setPreviewMjml(null)
    setPreviewText(null)
    setRenderedSubject(null)
    setRenderedSubjectPreview(null)
    setEffectiveTestData(null)
    setActiveTabKey('1') // Reset to HTML tab on new fetch
    setPreviewTextSource(null)
    setPreviewTextError(null)

    try {
      if ('error' in built) {
        setError(t`Invalid template structure data.`)
        setMjmlError(null)
        setPreviewMjml(null)
        setIsLoading(false)
        return
      }
      const req = built.request

      // console.log('Compile Request:', req)
      const response = await templatesApi.compile(req)
      // console.log('Compile Response:', response)

      // Server returns rendered subject/subject_preview on both success and
      // MJML-error paths, so update them either way before branching.
      setRenderedSubject(response.subject ?? null)
      setRenderedSubjectPreview(response.subject_preview ?? null)
      // The server echoes back the effective template data (with the injected
      // workspace object) so the Template Data tab matches what was rendered.
      setEffectiveTestData(response.test_data ?? null)

      if (response.error) {
        setMjmlError(response.error)
        setPreviewMjml(response.mjml)
        setError(null)
        setPreviewHtml(null)
        setPreviewText(null)
      } else {
        setPreviewHtml(response.html)
        setPreviewMjml(response.mjml)
        setPreviewText(response.text ?? null)
        setPreviewTextSource(response.text_source ?? null)
        setPreviewTextError(response.text_error ?? null)
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
      setPreviewText(null)
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
      setPreviewText(null)
      setPreviewTextSource(null)
      setPreviewTextError(null)
      setError(null)
      setMjmlError(null)
      setIsLoading(false)
      setActiveTabKey('1')
      setRenderedSubject(null)
      setRenderedSubjectPreview(null)
      setEffectiveTestData(null)
      setSelectedLanguage(null)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- fetchPreview is stable
  }, [isOpen, record.id, record.version, workspace.id, effectiveLanguage])

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

  if (previewText !== null) {
    items.push({
      key: 'text',
      label: t`Text version`,
      children: (
        <div>
          <div className="flex items-center gap-2 mb-2">
            {previewTextSource === 'custom' ? (
              <Tag color="purple">{t`Custom`}</Tag>
            ) : previewTextSource === 'auto' ? (
              <Tag>{t`Automatic`}</Tag>
            ) : null}
            <span className="text-xs text-gray-500">
              {previewTextSource === 'custom'
                ? t`Written by hand for this template and sent with the HTML version. Its links are not tracked.`
                : t`Sent with the HTML version and generated from it. Its links are tracked like the links of the HTML.`}
            </span>
          </div>
          {previewTextError && (
            <Alert
              type="warning"
              showIcon
              className="mb-2"
              message={t`The custom text could not be used, the automatic version is sent instead: ${previewTextError}`}
            />
          )}
          {previewText.trim() ? (
            <pre
              data-testid="preview-text-version"
              className="text-sm text-gray-800 bg-white border border-gray-200 rounded"
              style={{
                margin: 0,
                padding: '16px 20px',
                lineHeight: 1.6,
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
                fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace'
              }}
            >
              {previewText}
            </pre>
          ) : (
            <div className="text-gray-500">{t`No text version: this email will be sent as HTML only.`}</div>
          )}
        </div>
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

  // Add Template Data tab regardless of preview status. Prefer the effective data the
  // server rendered with (includes the injected workspace object); fall back to the
  // local data before the first compile resolves.
  const testData = effectiveTestData || templateData || record.test_data || {}
  items.push({
    key: '3',
    label: t`Template Data`,
    children: <JsonDataViewer data={testData} />
  })

  // Only transactional sends carry metadata, so this tab is pushed only when there is
  // something in it — an always-present tab reading {} on every broadcast preview is
  // worse than no tab. The table column, not this, is what guarantees metadata is
  // reachable: this drawer opens only once a template id resolves to a fetched template.
  const messageMetadata = messageHistory?.message_data?.metadata
  if (messageMetadata && Object.keys(messageMetadata).length > 0) {
    items.push({
      key: '4',
      label: t`Metadata`,
      children: <JsonDataViewer data={messageMetadata} />
    })
  }

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
          <Text>{renderedSubject ?? effectiveEmail?.subject}</Text>
        </Descriptions.Item>

        {(renderedSubjectPreview || effectiveEmail?.subject_preview) && (
          <Descriptions.Item label={t`Subject preview`}>
            <Text>{renderedSubjectPreview ?? effectiveEmail?.subject_preview}</Text>
          </Descriptions.Item>
        )}

        {/* Channel Options Display - CC and BCC */}
        {messageHistory?.channel_options?.cc && messageHistory.channel_options.cc.length > 0 && (
          <Descriptions.Item label={t`CC`}>
            <Space size={[0, 4]} wrap>
              {messageHistory.channel_options.cc.map((email, idx) => (
                <Tag variant="filled" key={idx} color="blue" className="text-xs">
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
                <Tag variant="filled" key={idx} color="purple" className="text-xs">
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
              <Alert title={t`Error loading preview`} description={error} type="error" showIcon />
            </div>
          )}
        {!isLoading && mjmlError && (
          // MJML Compilation Error
          <div className="p-4 overflow-auto flex-grow flex flex-col">
            <Alert
              title={t`MJML Compilation Error: ${mjmlError.message}`}
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
      {controlled ? null : <div onClick={() => setOwnOpen(true)}>{children}</div>}
      <Drawer
        title={`${record.name}`}
        placement="right"
        size={650}
        open={isOpen}
        onClose={close}
        destroyOnHidden={true}
        mask={{ enabled: true, closable: true }}
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
