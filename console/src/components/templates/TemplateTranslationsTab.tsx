import { useState, useCallback, useMemo, useRef } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Button, Drawer, Input, Popconfirm, Switch, Tag } from 'antd'
import { Languages, RotateCcw } from 'lucide-react'
import { Panel } from '../layout'
import { SUPPORTED_LANGUAGES } from '../../lib/languages'
import type { Workspace } from '../../services/api/types'
import type { EmailBlock } from '../email_builder/types'
import type {
  DeleteSavedBlockHandler,
  RenameSavedBlockHandler,
  SavedBlock,
  SaveBlockHandler
} from '../email_builder/types'
import type { MjmlCompileError } from '../../services/api/template'
import { templatesApi } from '../../services/api/template'
import EmailBuilder from '../email_builder/EmailBuilder'
import MjmlCodeEditor from '../email_builder/MjmlCodeEditor'
import IphoneEmailPreview from './PhonePreview'
import { useLicense } from '../../hooks/useLicense'
import { LicenceGateNotice } from '../license/LicenceGateNotice'
import type { CustomTextVersion } from '../email_builder/panels/TextVersionPanel'
import { customTextOrUndefined, seedTextVersion } from './textVersionSeed'
import { ImportExportButton } from './ImportExportButton'

export interface TranslationEditorState {
  enabled: boolean
  subject: string
  subjectPreview: string
  visualEditorTree?: EmailBlock
  mjmlSource?: string
  // The custom plain-text version of this language; undefined for the automatic one.
  // Never copied from the default language: each language writes its own.
  text?: string
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
  onSaveBlock: SaveBlockHandler
  canManageSavedBlocks?: boolean
  onRenameSavedBlock?: RenameSavedBlockHandler
  onDeleteSavedBlock?: DeleteSavedBlockHandler
  // Template category: decides which links a custom text is reminded to keep.
  category?: string
  // The template's name, which an exported translation's file is named after, with its language
  templateName?: string
  // Set by a save refused for a translation missing its subject or preview text: the detail
  // shows that language and marks its empty fields. A new attempt shows it again.
  missing?: { lang: string; attempt: number } | null
  // The sender the inbox mockup names, as on the Settings step: the template's sender or the workspace.
  senderName?: string
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
  onSaveBlock,
  canManageSavedBlocks,
  onRenameSavedBlock,
  onDeleteSavedBlock,
  category,
  templateName,
  missing,
  senderName
}) => {
  const { t, i18n } = useLingui()
  const [editorDrawerLang, setEditorDrawerLang] = useState<string | null>(null)
  // The language the detail shows: the first switched on when the tab opens, else the first, then the member's
  // pick. Kept, so switching another language on or off does not move the detail.
  const [chosenLang, setChosenLang] = useState<string | null>(() => {
    const languages = (workspace.settings.languages || []).filter(
      (l) => l !== (workspace.settings.default_language || 'en')
    )
    return languages.find((lang) => translationsState[lang]?.enabled) ?? languages[0] ?? null
  })
  // A refused save brings the language it is about into the detail, once per attempt.
  const [shownAttempt, setShownAttempt] = useState(0)
  if (missing && missing.attempt !== shownAttempt) {
    setShownAttempt(missing.attempt)
    setChosenLang(missing.lang)
  }
  // Advisory only; the server's TranslationsWiden check is the gate. What is locked here
  // mirrors what that check refuses: switching a language ON and editing what a translation
  // says. Switching one OFF stays open, because removing a translation never needs a licence
  // and a lapsed deployment must be able to take back what it made.
  const { has } = useLicense()
  const locked = !has('template_i18n')

  const defaultLanguage = workspace.settings.default_language || 'en'
  // Filtered on the same default the list shows, so a workspace without one does not list it twice
  const translationLanguages = (workspace.settings.languages || []).filter((l) => l !== defaultLanguage)

  /** A language's name in the console's language (« français » → « Français »), else its English name. */
  const displayNames = useMemo(() => {
    try {
      return new Intl.DisplayNames([i18n.locale], { type: 'language' })
    } catch {
      return undefined
    }
  }, [i18n.locale])
  const languageName = (code: string): string => {
    try {
      const name = displayNames?.of(code)
      if (name && name !== code) return name.charAt(0).toLocaleUpperCase(i18n.locale) + name.slice(1)
    } catch {
      // An unknown locale or code: the English name below
    }
    return SUPPORTED_LANGUAGES[code] || code
  }

  // The latest state, so a change sent by a handler of an earlier render (a seed answering
  // late) merges into the edits made since instead of reverting them.
  const translationsStateRef = useRef(translationsState)
  translationsStateRef.current = translationsState

  const updateLangState = useCallback(
    (lang: string, updates: Partial<TranslationEditorState>) => {
      const current = translationsStateRef.current
      const next = {
        ...current,
        [lang]: {
          ...(current[lang] || {
            enabled: false,
            subject: '',
            subjectPreview: ''
          }),
          ...updates
        }
      }
      translationsStateRef.current = next
      onTranslationsStateChange(next)
    },
    [onTranslationsStateChange]
  )

  const handleToggle = useCallback(
    (lang: string, checked: boolean) => {
      if (checked) {
        const existing = translationsState[lang]
        if (existing && (existing.subject || existing.subjectPreview || existing.visualEditorTree || existing.mjmlSource || existing.text)) {
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
        return {
          html: response.html || '',
          mjml: response.mjml || '',
          errors: [],
          text: response.text,
          test_data: response.test_data
        }
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
          error: response.error,
          text: response.text
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

  const resetLanguage = (lang: string) =>
    updateLangState(lang, {
      subject: defaultSubject || '',
      subjectPreview: defaultSubjectPreview || '',
      visualEditorTree:
        editorMode === 'visual' ? (JSON.parse(JSON.stringify(defaultVisualEditorTree)) as EmailBlock) : undefined,
      mjmlSource: editorMode === 'code' ? defaultMjmlSource : undefined,
      text: undefined
    })

  // The language shown: the one picked while it is still a translation language, else the first switched on,
  // else the first
  const selectedLang =
    chosenLang && translationLanguages.includes(chosenLang)
      ? chosenLang
      : (translationLanguages.find((lang) => translationsState[lang]?.enabled) ?? translationLanguages[0])

  const activeLangState = editorDrawerLang ? translationsState[editorDrawerLang] : null

  const activeCustomText: CustomTextVersion | undefined =
    editorDrawerLang && activeLangState
      ? {
          value: activeLangState.text,
          onChange: (value) => updateLangState(editorDrawerLang, { text: value }),
          onSeed: () =>
            seedTextVersion(
              workspace,
              editorMode === 'code'
                ? { mjmlSource: activeLangState.mjmlSource || '' }
                : { visualEditorTree: activeLangState.visualEditorTree || defaultVisualEditorTree }
            ),
          category,
          isTranslation: true,
          disabled: locked
        }
      : undefined

  const languageCode = (code: string) => (
    <span className="w-8 shrink-0 rounded-sm bg-fill py-0.5 text-center font-mono text-xs text-ink-3 uppercase">
      {code}
    </span>
  )

  /** The default language's text beside the field that translates it. */
  const sourcePair = (sourceLabel: string, sourceText: string, field: React.ReactNode, error?: string) => (
    <div className="grid grid-cols-2 gap-4">
      <div className="flex flex-col gap-1 rounded-md bg-subtle px-3 py-2">
        <span className="text-xs text-ink-3">{sourceLabel}</span>
        <span className="text-sm break-words text-ink-2">{sourceText || '—'}</span>
      </div>
      <div className="flex flex-col gap-1">
        {field}
        {error ? <span className="text-sm text-danger-fg">{error}</span> : null}
      </div>
    </div>
  )

  const detail = (lang: string) => {
    const langState = translationsState[lang]
    const enabled = langState?.enabled || false
    const name = languageName(lang)
    const defaultName = languageName(defaultLanguage)
    const title = t`${name} translation`
    // After a refused save, the empty required fields are marked.
    const subjectMissing = Boolean(missing) && !langState?.subject
    const previewMissing = Boolean(missing) && !langState?.subjectPreview

    if (!enabled || !langState) {
      return (
        <Panel icon={<Languages size={14} aria-hidden="true" />} title={title}>
          <div className="flex flex-col items-start gap-3">
            <p className="m-0 text-sm text-ink-3">
              {locked
                ? t`Your licence does not cover translations: see the notice above.`
                : t`Enable this translation to edit it.`}
            </p>
            <Button type="primary" disabled={locked} onClick={() => handleToggle(lang, true)}>
              {t`Enable`}
            </Button>
          </div>
        </Panel>
      )
    }

    return (
      <Panel
        icon={<Languages size={14} aria-hidden="true" />}
        title={title}
        extra={
          <Popconfirm
            title={t`Reset translation`}
            description={t`This will replace all content with the default language template. Continue?`}
            onConfirm={() => resetLanguage(lang)}
            okText={t`Reset`}
            cancelText={t`Cancel`}
            disabled={locked}
          >
            <Button size="small" icon={<RotateCcw size={13} aria-hidden="true" />} disabled={locked}>
              {t`Reset`}
            </Button>
          </Popconfirm>
        }
      >
        <div className="flex flex-col gap-4">
          {sourcePair(
            t`Subject · ${defaultName}`,
            defaultSubject,
            <>
              <label htmlFor={`translation-subject-${lang}`} className="text-sm font-medium">
                {t`Subject`}
              </label>
              <Input
                id={`translation-subject-${lang}`}
                value={langState.subject}
                disabled={locked}
                status={subjectMissing ? 'error' : undefined}
                onChange={(e) => updateLangState(lang, { subject: e.target.value })}
                placeholder={t`Email subject`}
              />
            </>,
            subjectMissing ? t`Enter the ${name} subject` : undefined
          )}
          {sourcePair(
            t`Subject preview · ${defaultName}`,
            defaultSubjectPreview,
            <>
              <label htmlFor={`translation-preview-${lang}`} className="text-sm font-medium">
                {t`Subject preview`}
              </label>
              <Input
                id={`translation-preview-${lang}`}
                value={langState.subjectPreview}
                disabled={locked}
                status={previewMissing ? 'error' : undefined}
                onChange={(e) => updateLangState(lang, { subjectPreview: e.target.value })}
                placeholder={t`Preview text`}
              />
            </>,
            previewMissing ? t`Enter the ${name} preview text` : undefined
          )}
          <div className="flex gap-4 border-t border-line-soft pt-4">
            {/* The inbox line of this language, where the mockup has a thumbnail of the content */}
            <div className="w-80 shrink-0 overflow-hidden">
              <div style={{ transform: 'scale(0.8)', transformOrigin: 'top left', height: 208 }}>
                <IphoneEmailPreview
                  sender={senderName || workspace.name}
                  subject={langState.subject || t`Email subject`}
                  previewText={langState.subjectPreview || t`Preview text will appear here...`}
                  timestamp={t`Now`}
                  // The Settings step's mockup: same clock, same sender.
                  currentTime="9:41"
                />
              </div>
            </div>
            <div className="flex flex-col items-start gap-2">
              <span className="text-sm font-medium text-ink">{t`Content`}</span>
              {customTextOrUndefined(langState.text) !== undefined ? (
                <Tag color="purple">{t`Text version: Custom`}</Tag>
              ) : (
                <Tag>{t`Text version: Automatic`}</Tag>
              )}
              <span className="text-xs text-ink-3">{t`Reset replaces all the content with the default language's.`}</span>
              <Button type="primary" disabled={locked} onClick={() => setEditorDrawerLang(lang)}>
                {t`Open email editor`}
              </Button>
            </div>
          </div>
        </div>
      </Panel>
    )
  }

  return (
    <div className="p-6">
      <LicenceGateNotice feature="template_i18n" workspaceId={workspace.id} className="mb-4" />
      <div className="flex items-start gap-4">
        {/* The workspace's languages, each switched on or off; a click shows one in the detail */}
        <aside className="flex w-72 shrink-0 flex-col gap-0.5 rounded-lg border border-line bg-surface p-3">
          <div className="px-2 pb-2 text-xs font-medium tracking-wide text-ink-3 uppercase">
            {t`Workspace languages`}
          </div>
          <div className="flex items-center gap-2 px-2 py-1.5">
            {languageCode(defaultLanguage)}
            <span className="min-w-0 flex-1 truncate text-sm text-ink">{languageName(defaultLanguage)}</span>
            <span className="text-xs text-ink-3">{t`Default`}</span>
          </div>
          {translationLanguages.map((lang) => {
            const enabled = translationsState[lang]?.enabled || false
            const name = languageName(lang)
            const isSelected = lang === selectedLang
            return (
              <div
                key={lang}
                data-language={lang}
                className={`flex items-center gap-2 rounded-md pr-2 ${isSelected ? 'bg-primary-50' : 'hover:bg-fill'}`}
              >
                <button
                  type="button"
                  aria-pressed={isSelected}
                  onClick={() => setChosenLang(lang)}
                  className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 px-2 py-1.5 text-left"
                >
                  {languageCode(lang)}
                  <span className="truncate text-sm text-ink">{name}</span>
                </button>
                <Switch
                  size="small"
                  aria-label={t`Translate into ${name}`}
                  checked={enabled}
                  disabled={locked && !enabled}
                  onChange={(checked) => handleToggle(lang, checked)}
                />
              </div>
            )
          })}
          <p className="m-0 mt-2 px-2 text-xs text-ink-3">{t`Languages come from the workspace settings.`}</p>
        </aside>
        <div className="min-w-0 flex-1">{selectedLang ? detail(selectedLang) : null}</div>
      </div>

      {editorDrawerLang && activeLangState && (
        <Drawer
          title={t`Edit ${languageName(editorDrawerLang)} translation`}
          size="100%"
          rootClassName="app-drawer-instant"
          open={!!editorDrawerLang}
          onClose={() => setEditorDrawerLang(null)}
          className="drawer-body-no-padding"
          keyboard={false}
          mask={{ closable: false }}
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
              height="calc(100vh - 61px)"
              templateId={`translation-${editorDrawerLang}`}
              textVersion={{ custom: activeCustomText }}
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
              canManageSavedBlocks={canManageSavedBlocks}
              onRenameSavedBlock={onRenameSavedBlock}
              onDeleteSavedBlock={onDeleteSavedBlock}
              hiddenBlocks={['mj-title', 'mj-preview']}
              height="calc(100vh - 61px)"
              textVersion={{ enabled: true, custom: activeCustomText }}
              toolbarActions={
                // Imports into, and exports, the open language only: the default language is left as it is
                <ImportExportButton
                  onImport={(imported) => updateLangState(editorDrawerLang, { visualEditorTree: imported })}
                  tree={activeLangState.visualEditorTree || defaultVisualEditorTree}
                  testData={testData}
                  workspaceId={workspace.id}
                  templateName={templateName ? `${templateName} (${editorDrawerLang})` : undefined}
                />
              }
            />
          )}
        </Drawer>
      )}
    </div>
  )
}

export default TemplateTranslationsTab
