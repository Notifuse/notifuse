import { useState, useCallback } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Button, Card, Col, Drawer, Dropdown, Input, Popconfirm, Row, Switch } from 'antd'
import { DownOutlined } from '@ant-design/icons'
import { SUPPORTED_LANGUAGES } from '../../lib/languages'
import type { Workspace } from '../../services/api/types'
import type { EmailBlock } from '../email_builder/types'
import type { SavedBlock, SaveOperation } from '../email_builder/types'
import type { MjmlCompileError } from '../../services/api/template'
import { templatesApi } from '../../services/api/template'
import EmailBuilder from '../email_builder/EmailBuilder'
import MjmlCodeEditor from '../email_builder/MjmlCodeEditor'
import IphoneEmailPreview from './PhonePreview'

export interface TranslationEditorState {
  enabled: boolean
  subject: string
  subjectPreview: string
  visualEditorTree?: EmailBlock
  mjmlSource?: string
}

interface TemplateTranslationsTabProps {
  workspace: Workspace
  editorMode: 'visual' | 'code'
  translationsState: Record<string, TranslationEditorState>
  onTranslationsStateChange: (state: Record<string, TranslationEditorState>) => void
  defaultSubject: string
  defaultSubjectPreview: string
  defaultVisualEditorTree: EmailBlock
  defaultMjmlSource: string
  testData?: Record<string, unknown>
  onTestDataChange: (testData: Record<string, unknown>) => void
  savedBlocks: SavedBlock[]
  onSaveBlock: (block: EmailBlock, operation: SaveOperation, nameOrId: string) => void
}

const TemplateTranslationsTab: React.FC<TemplateTranslationsTabProps> = ({
  workspace,
  editorMode,
  translationsState,
  onTranslationsStateChange,
  defaultSubject,
  defaultSubjectPreview,
  defaultVisualEditorTree,
  defaultMjmlSource,
  testData,
  onTestDataChange,
  savedBlocks,
  onSaveBlock
}) => {
  const { t } = useLingui()
  const [editorDrawerLang, setEditorDrawerLang] = useState<string | null>(null)

  const translationLanguages = (workspace.settings.languages || []).filter(
    (l) => l !== workspace.settings.default_language
  )

  const updateLangState = useCallback(
    (lang: string, updates: Partial<TranslationEditorState>) => {
      onTranslationsStateChange({
        ...translationsState,
        [lang]: {
          ...(translationsState[lang] || {
            enabled: false,
            subject: '',
            subjectPreview: ''
          }),
          ...updates
        }
      })
    },
    [translationsState, onTranslationsStateChange]
  )

  const handleToggle = useCallback(
    (lang: string, checked: boolean) => {
      if (checked) {
        const existing = translationsState[lang]
        if (existing && (existing.subject || existing.subjectPreview || existing.visualEditorTree || existing.mjmlSource)) {
          // Re-enable with existing data
          updateLangState(lang, { enabled: true })
        } else {
          // Clone defaults
          updateLangState(lang, {
            enabled: true,
            subject: defaultSubject || '',
            subjectPreview: defaultSubjectPreview || '',
            visualEditorTree:
              editorMode === 'visual'
                ? (JSON.parse(JSON.stringify(defaultVisualEditorTree)) as EmailBlock)
                : undefined,
            mjmlSource: editorMode === 'code' ? defaultMjmlSource : undefined
          })
        }
      } else {
        updateLangState(lang, { enabled: false })
      }
    },
    [
      translationsState,
      updateLangState,
      defaultSubject,
      defaultSubjectPreview,
      defaultVisualEditorTree,
      defaultMjmlSource,
      editorMode
    ]
  )

  const handleCompileVisual = useCallback(
    async (tree: EmailBlock, builderTestData?: Record<string, unknown>) => {
      try {
        const response = await templatesApi.compile({
          workspace_id: workspace.id,
          message_id: 'preview',
          visual_editor_tree: tree,
          test_data: builderTestData || {},
          channel: 'email',
          tracking_settings: {
            enable_tracking: workspace.settings?.email_tracking_enabled || false,
            endpoint: workspace.settings?.custom_endpoint_url || undefined,
            workspace_id: workspace.id,
            message_id: 'preview'
          }
        })
        if (response.error) {
          return {
            html: '',
            mjml: response.mjml || '',
            errors: [response.error as unknown as Record<string, unknown>]
          }
        }
        return { html: response.html || '', mjml: response.mjml || '', errors: [] }
      } catch (error) {
        const err = error as Error
        return {
          html: '',
          mjml: '',
          errors: [{ message: err.message || 'Compilation failed' }]
        }
      }
    },
    [workspace.id, workspace.settings?.email_tracking_enabled, workspace.settings?.custom_endpoint_url]
  )

  const handleCompileCode = useCallback(
    async (mjml: string, codeTestData?: Record<string, unknown>) => {
      try {
        const response = await templatesApi.compile({
          workspace_id: workspace.id,
          message_id: 'preview',
          mjml_source: mjml,
          test_data: codeTestData || {},
          channel: 'email',
          tracking_settings: {
            enable_tracking: workspace.settings?.email_tracking_enabled || false,
            endpoint: workspace.settings?.custom_endpoint_url || undefined,
            workspace_id: workspace.id,
            message_id: 'preview'
          }
        })
        return {
          html: response.html || '',
          mjml: response.mjml || '',
          error: response.error
        }
      } catch (error) {
        const err = error as Error
        return {
          html: '',
          mjml: '',
          error: {
            message: err.message || 'Compilation failed',
            details: []
          } as MjmlCompileError
        }
      }
    },
    [workspace.id, workspace.settings?.email_tracking_enabled, workspace.settings?.custom_endpoint_url]
  )

  const activeLangState = editorDrawerLang ? translationsState[editorDrawerLang] : null

  return (
    <div className="p-8">
      <div className="flex flex-col gap-6 items-center">
        {translationLanguages.map((lang) => {
          const langState = translationsState[lang]
          const enabled = langState?.enabled || false
          const langName = SUPPORTED_LANGUAGES[lang] || lang

          return (
            <Card
              key={lang}
              title={`${langName} (${lang})`}
              style={{ backgroundColor: '#fff', width: 900 }}
              bodyStyle={enabled && langState ? undefined : { display: 'none' }}
              extra={
                <Switch
                  checked={enabled}
                  onChange={(checked) => handleToggle(lang, checked)}
                  checkedChildren={t`Enabled`}
                  unCheckedChildren={t`Disabled`}
                />
              }
            >
              {enabled && langState ? (
                <Row gutter={24}>
                  <Col span={12}>
                    <div className="mb-4">
                      <label className="block text-sm font-medium mb-1">{t`Subject`}</label>
                      <Input
                        value={langState.subject}
                        onChange={(e) => updateLangState(lang, { subject: e.target.value })}
                        placeholder={t`Email subject`}
                      />
                    </div>
                    <div className="mb-4">
                      <label className="block text-sm font-medium mb-1">{t`Subject preview`}</label>
                      <Input
                        value={langState.subjectPreview}
                        onChange={(e) =>
                          updateLangState(lang, { subjectPreview: e.target.value })
                        }
                        placeholder={t`Preview text`}
                      />
                    </div>
                    <Button.Group style={{ display: 'flex' }}>
                      <Button type="primary" block onClick={() => setEditorDrawerLang(lang)}>
                        {t`Open email editor`}
                      </Button>
                      <Dropdown
                        menu={{
                          items: [
                            {
                              key: 'reset',
                              label: (
                                <Popconfirm
                                  title={t`Reset translation`}
                                  description={t`This will replace all content with the default language template. Continue?`}
                                  onConfirm={() => {
                                    updateLangState(lang, {
                                      subject: defaultSubject || '',
                                      subjectPreview: defaultSubjectPreview || '',
                                      visualEditorTree:
                                        editorMode === 'visual'
                                          ? (JSON.parse(JSON.stringify(defaultVisualEditorTree)) as EmailBlock)
                                          : undefined,
                                      mjmlSource: editorMode === 'code' ? defaultMjmlSource : undefined
                                    })
                                  }}
                                  okText={t`Reset`}
                                  cancelText={t`Cancel`}
                                >
                                  <span style={{ display: 'block' }}>{t`Reset`}</span>
                                </Popconfirm>
                              )
                            }
                          ]
                        }}
                        trigger={['click']}
                      >
                        <Button type="primary" icon={<DownOutlined />} />
                      </Dropdown>
                    </Button.Group>
                  </Col>
                  <Col span={12}>
                    <div className="flex justify-center" style={{ transform: 'scale(0.8)', transformOrigin: 'top center', height: 208 }}>
                      <IphoneEmailPreview
                        sender={t`Sender Name`}
                        subject={langState.subject || t`Email Subject`}
                        previewText={langState.subjectPreview || t`Preview text will appear here...`}
                        timestamp={t`Now`}
                        currentTime="12:12"
                      />
                    </div>
                  </Col>
                </Row>
              ) : null}
            </Card>
          )
        })}
      </div>

      {editorDrawerLang && activeLangState && (
        <Drawer
          title={t`Edit ${SUPPORTED_LANGUAGES[editorDrawerLang] || editorDrawerLang} translation`}
          width="100%"
          open={!!editorDrawerLang}
          onClose={() => setEditorDrawerLang(null)}
          className="drawer-no-transition drawer-body-no-padding"
          keyboard={false}
          maskClosable={false}
          extra={
            <Button type="primary" onClick={() => setEditorDrawerLang(null)}>
              {t`Done`}
            </Button>
          }
        >
          {editorMode === 'code' ? (
            <MjmlCodeEditor
              mjmlSource={activeLangState.mjmlSource || ''}
              onMjmlSourceChange={(source) =>
                updateLangState(editorDrawerLang, { mjmlSource: source })
              }
              onCompile={handleCompileCode}
              testData={testData}
              onTestDataChange={onTestDataChange}
              height="calc(100vh - 66px)"
              templateId={`translation-${editorDrawerLang}`}
            />
          ) : (
            <EmailBuilder
              tree={activeLangState.visualEditorTree || defaultVisualEditorTree}
              onTreeChange={(tree) =>
                updateLangState(editorDrawerLang, { visualEditorTree: tree })
              }
              onCompile={handleCompileVisual}
              testData={testData}
              onTestDataChange={onTestDataChange}
              savedBlocks={savedBlocks}
              onSaveBlock={onSaveBlock}
              hiddenBlocks={['mj-title', 'mj-preview']}
              height="calc(100vh - 66px)"
            />
          )}
        </Drawer>
      )}
    </div>
  )
}

export default TemplateTranslationsTab
