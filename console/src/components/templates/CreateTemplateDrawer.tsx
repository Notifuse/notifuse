import { useEffect, useState, useMemo, useRef } from 'react'
import { buildDefaultTestData } from './defaultTestData'
import { useLingui } from '@lingui/react/macro'
import { Button, Form, Input, App, Dropdown, MenuProps, Spin } from 'antd'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { templatesApi, type CreateTemplateRequest, type UpdateTemplateRequest } from '../../services/api/template'
import { ApiError } from '../../services/api/client'
import { useTemplateConflictModal } from './useTemplateConflictModal'
import { templateBlocksApi } from '../../services/api/template_blocks'
import type { Template, TemplateBlock, Workspace } from '../../services/api/types'
import EmailBuilder from '../email_builder/EmailBuilder'
import type {
  DeleteSavedBlockHandler,
  EmailBlock,
  RenameSavedBlockHandler,
  SaveBlockHandler
} from '../email_builder/types'
import type { PreviewRef } from '../email_builder/panels/Preview'
import { kebabCase } from 'lodash'
import { truncateIdentifier } from '../../utils/identifiers'
import { TEMPLATE_ID_MAX_LENGTH } from '../../constants/limits'
import { CircleHelp } from 'lucide-react'
import { Tour } from 'antd'
import { ImportExportButton } from './ImportExportButton'
import { useAuth } from '../../contexts/AuthContext'
import { EmailAIAssistant } from '../email_builder/EmailAIAssistant'
import { EmailBlockClass } from '../email_builder/EmailBlockClass'
import { parentIdOf, rebalanceAdded, rebalanceRemoved } from '../email_builder/utils/layoutRebalance'
import type { MJMLComponentType } from '../email_builder/types'
import MjmlCodeEditor, { STARTER_TEMPLATE } from '../email_builder/MjmlCodeEditor'
import type { MjmlCodeEditorRef } from '../email_builder/MjmlCodeEditor'
import type { MjmlCompileError } from '../../services/api/template'
import { SUPPORTED_LANGUAGES } from '../../lib/languages'
import TemplateTranslationsTab from './TemplateTranslationsTab'
import type { TranslationEditorState } from './TemplateTranslationsTab'
import type { EmailTemplate, TemplateTranslation } from '../../services/api/template'
import { customTextOrUndefined, missingRequiredLink, seedTextVersion } from './textVersionSeed'
import { TemplateCategoryTag } from './TemplateCategoryTag'
import { FullDrawer } from '../layout'
import {
  TemplateBarActions,
  TemplateBarTitle,
  TemplateStepSwitch,
  type TemplateStep
} from './TemplateDrawerBar'
import { TemplateSettingsStep } from './TemplateSettingsStep'
import { TemplateSettingsAside } from './TemplateSettingsAside'
import SendTemplateModal from './SendTemplateModal'
import { canSendTestEmail } from '../../services/api/permissions'
import { useWorkspacePermissions } from '../../contexts/AuthContext'
import { TemplateStartModal } from './TemplateStartModal'
import { MODEL_LABELS } from './gallery/labels'
import { useLanguageName } from './useLanguageName'
import { withBrand, type GalleryStart } from './gallery/buildStartingPoint'
import { useGalleryAssets, useGalleryModels } from './gallery/useGalleryModels'
import { useLogoWidth } from './gallery/logoWidth'
import { GALLERY_CONTENT } from './gallery/content'
import { galleryModel } from './gallery/models'
import { resolveGalleryLanguage } from './gallery/resolveGalleryLanguage'

const COPY_ID_SUFFIX = '-copy'

const templateIdFromName = (name: string) =>
  truncateIdentifier(kebabCase(name), TEMPLATE_ID_MAX_LENGTH)

interface CreateTemplateDrawerProps {
  workspace: Workspace
  template?: Template
  fromTemplate?: Template
  buttonProps?: Record<string, unknown>
  buttonContent?: React.ReactNode
  /**
   * Called when the drawer closes, with the template saved while it was open (the last revision),
   * or nothing when it closes without a save (cancelled in the gallery, or discarded).
   */
  onClose?: (saved?: Template) => void
  forceCategory?: string
  // The member may rename, replace, duplicate and delete saved blocks from the editor's library
  canManageSavedBlocks?: boolean
  /**
   * Controlled mode, for a page that opens the drawer itself: no button is rendered, the drawer
   * opens when this turns true and calls onClose for the page to set it back to false.
   */
  open?: boolean
  /** A new template only: the content of an imported file, put in the editor without saving. */
  initialTree?: EmailBlock
  initialTestData?: Record<string, unknown>
  /** A new template only: its name, from which its ID is derived. */
  initialName?: string
  /** A new template only: the gallery model it starts from, the gallery itself skipped. */
  startModelId?: string
  /** A new template only: the message of a save that closes the drawer, instead of the default. */
  successMessage?: string
}

// The fields of the Settings step: an error on one of them opens that step.
const SETTINGS_FIELDS = [
  'name',
  'id',
  'category',
  'email.sender_id',
  'email.subject',
  'email.subject_preview',
  'email.reply_to'
]

/**
 * What the drawer actually submits for a translation.
 *
 * `EmailTemplate` marks `compiled_preview` and `visual_editor_tree` as required because
 * that is what the API returns; a translation being SENT carries neither (the server
 * compiles the preview, and code mode has no visual tree), so the payload is modelled
 * as its own partial shape instead of being asserted into the response type.
 */
type TranslationEmailPayload = Partial<EmailTemplate> & Pick<EmailTemplate, 'subject'>

// The drawer's bar: 40px and its 1px rule (FullDrawer).
// The FullDrawer's top bar (h-12): the editor fills the rest of the screen, with no scroll
const HEADER_HEIGHT = 48

// Liquid variables named in the custom text warnings, outside the translated sentences.
const UNSUBSCRIBE_VARIABLE = '{{ unsubscribe_url }}'
const CONFIRM_VARIABLE = '{{ confirm_subscription_url }}'
// Help & Support dropdown component
const HelpSupportDropdown: React.FC<{ onStartTour: () => void }> = ({ onStartTour }) => {
  const { t } = useLingui()
  const menuItems: MenuProps['items'] = [
    {
      key: 'tour',
      label: t`Take a tour`,
      icon: <CircleHelp size={14} aria-hidden="true" />,
      onClick: onStartTour
    }
  ]

  return (
    <Dropdown menu={{ items: menuItems }} placement="bottomRight" trigger={['click']}>
      <Button title={t`Help & Support`} icon={<CircleHelp size={14} aria-hidden="true" />}>
        {t`Help`}
      </Button>
    </Dropdown>
  )
}
/**
 * Renders a Tag component with the appropriate color and translated label for an email template
 * category.
 */
// eslint-disable-next-line react-refresh/only-export-components -- Utility export co-located with component
export const renderCategoryTag = (category: string) => <TemplateCategoryTag category={category} />

export function CreateTemplateDrawer({
  workspace,
  template,
  fromTemplate,
  buttonProps = {},
  buttonContent,
  onClose,
  forceCategory,
  canManageSavedBlocks = false,
  open,
  initialTree,
  initialTestData,
  initialName,
  startModelId: initialModelId,
  successMessage
}: CreateTemplateDrawerProps) {
  const { t, i18n } = useLingui()
  const [isOpen, setIsOpen] = useState(false)
  const [form] = Form.useForm()
  const queryClient = useQueryClient()
  const { conflictModal, showConflict } = useTemplateConflictModal()
  // The template revision the current editor content is based on. Sent as
  // base_version on save so the server can reject a stale-base overwrite (409).
  const baseVersionRef = useRef<number | undefined>(template?.version)
  // Whether the user has edited since the drawer opened. Guards the freshen-on-open
  // reseed so a background refetch never silently discards in-progress edits.
  const dirtyRef = useRef(false)
  // The same, for what the bar shows ("Unsaved changes").
  const [dirty, setDirty] = useState(false)
  const markDirty = () => {
    dirtyRef.current = true
    setDirty(true)
  }
  const markClean = () => {
    dirtyRef.current = false
    setDirty(false)
  }
  // The last saved revision when the drawer stays open after a save (Save and send a test).
  const [savedTemplate, setSavedTemplate] = useState<Template | undefined>(template)
  // The last revision saved since the drawer opened, handed to onClose.
  const savedSinceOpenRef = useRef<Template | undefined>(undefined)
  const [testTemplate, setTestTemplate] = useState<Template | null>(null)
  // Set by "Save and send": the save opens the test send instead of closing the drawer.
  const sendAfterSaveRef = useRef(false)
  const [stepsWithErrors, setStepsWithErrors] = useState<TemplateStep[]>([])
  // The translation a save was refused for: the Translations step shows it with its empty fields marked.
  const [translationMissing, setTranslationMissing] = useState<{ lang: string; attempt: number } | null>(null)
  const missingAttemptRef = useRef(0)
  const { permissions } = useWorkspacePermissions(workspace.id)
  const languageName = useLanguageName()
  const canTest = canSendTestEmail(permissions)
  // The gallery: a new template starts from a model or a template chosen in the start modal.
  const [startModalOpen, setStartModalOpen] = useState(false)
  // What a new template started from, shown in the Content card with a Change button.
  const [startInfo, setStartInfo] = useState<{ name: string; detail: string } | undefined>()
  // The template a new one copies (Duplicate, or Your templates): its editor mode is kept.
  const [cloneSource, setCloneSource] = useState<Template | undefined>(fromTemplate)
  // The gallery model a new template started from: its visual content locks the editor mode.
  const [startModelId, setStartModelId] = useState<string | undefined>()
  // A pick is applied once: a second click while the first looks up a free ID is ignored.
  const pickingRef = useRef(false)
  // Each close starts a new round: a pick still being prepared from an earlier one applies nothing.
  const startRoundRef = useRef(0)
  // A model given before opening is being built: the drawer is open on a loading state.
  const [preparing, setPreparing] = useState(false)
  // The gallery's images, served by this instance's console.
  const assets = useGalleryAssets()
  // Blank, in the workspace's language: the content a template without one starts from.
  const logoWidth = useLogoWidth(workspace.settings.logo_url || undefined)
  const initialModel = initialModelId ? galleryModel(initialModelId) : undefined
  const createDefaultBlocks = (): EmailBlock =>
    galleryModel('blank')!.build(
      withBrand(GALLERY_CONTENT[resolveGalleryLanguage(workspace.settings.default_language)], workspace.name),
      { ...assets, logoUrl: workspace.settings.logo_url || undefined, logoWidth }
    ).tree
  // The last submitted form values, so the conflict dialog can re-submit ("overwrite").
  const lastSubmittedValuesRef = useRef<Record<string, unknown> | null>(null)
  const [tab, setTab] = useState<TemplateStep>('settings')
  const [loading, setLoading] = useState(false)
  const { message, modal } = App.useApp()
  const { refreshWorkspaces } = useAuth()
  const [tourOpen, setTourOpen] = useState(false)
  const [forcedViewMode, setForcedViewMode] = useState<'edit' | 'preview' | null>(null)
  const [selectedBlockId, setSelectedBlockId] = useState<string | null>(null)
  const [emailBuilderHeight, setEmailBuilderHeight] = useState<string>(
    `calc(100vh - ${HEADER_HEIGHT}px)`
  )
  const [editorMode, setEditorMode] = useState<'visual' | 'code'>(() => {
    if (template?.email?.editor_mode === 'code') return 'code'
    if (fromTemplate?.email?.editor_mode === 'code') return 'code'
    return 'visual'
  })
  const [mjmlSource, setMjmlSource] = useState<string>(() => {
    if (template?.email?.mjml_source) return template.email.mjml_source
    if (fromTemplate?.email?.mjml_source) return fromTemplate.email.mjml_source
    return STARTER_TEMPLATE
  })
  const [translationsState, setTranslationsState] = useState<Record<string, TranslationEditorState>>({})
  // The custom plain-text version of the default language; undefined for the automatic one.
  const [emailText, setEmailText] = useState<string | undefined>(() =>
    customTextOrUndefined(template?.email?.text ?? fromTemplate?.email?.text)
  )
  // Each opening of the drawer is a session: a text change sent by a handler of an
  // earlier session (a seed answering after Cancel or Save) is dropped.
  const [textSession, setTextSession] = useState(0)
  const textSessionRef = useRef(0)
  const changeEmailText = (value?: string) => {
    if (textSession !== textSessionRef.current) return
    markDirty()
    setEmailText(value)
  }

  const translationLanguages = (workspace.settings.languages || []).filter(
    (l) => l !== workspace.settings.default_language
  )
  const showTranslationsTab = translationLanguages.length > 0

  // Refs for tour targets
  const treePanelRef = useRef<HTMLDivElement>(null)
  const editPanelRef = useRef<HTMLDivElement>(null)
  const settingsPanelRef = useRef<HTMLDivElement>(null)
  const previewSwitcherRef = useRef<HTMLDivElement>(null)
  const mobileDesktopSwitcherRef = useRef<HTMLDivElement>(null)
  const templateDataRef = useRef<PreviewRef>(null)
  const importExportButtonRef = useRef<HTMLDivElement>(null)
  const codeEditorRef = useRef<MjmlCodeEditorRef>(null)

  // set the tree apart to avoid rerendering the Email Editor when the tree changes
  const [visualEditorTree, setVisualEditorTree] = useState<EmailBlock>(() => {
    if (template && template.email?.visual_editor_tree) {
      // Check if visual_editor_tree is already an object
      if (typeof template.email.visual_editor_tree === 'object') {
        return template.email.visual_editor_tree as unknown as EmailBlock
      }

      // Otherwise parse it from string
      try {
        return JSON.parse(template.email.visual_editor_tree) as EmailBlock
      } catch (error) {
        console.error('Error parsing visual editor tree:', error)
        message.error(t`Error loading template: Invalid template data`)
        return createDefaultBlocks()
      }
    }
    return createDefaultBlocks()
  })

  // Always-current snapshot of the tree, so async callers (the AI assistant reading
  // the tree after its edits) never see a stale value captured in an old closure.
  const visualEditorTreeRef = useRef(visualEditorTree)
  visualEditorTreeRef.current = visualEditorTree

  // Add Form.useWatch for the email fields - must be called before conditional returns
  const senderID = Form.useWatch(['email', 'sender_id'], form)
  const emailSubject = Form.useWatch(['email', 'subject'], form)
  const emailPreview = Form.useWatch(['email', 'subject_preview'], form)
  const watchedName = Form.useWatch(['name'], form)
  const watchedTestData = Form.useWatch(['test_data'], form) as Record<string, unknown> | undefined
  const watchedCategory = Form.useWatch(['category'], form)
  const categoryValue = forceCategory || watchedCategory

  const emailProvider = useMemo(() => {
    const providerId =
      categoryValue === 'marketing'
        ? workspace.settings.marketing_email_provider_id
        : workspace.settings.transactional_email_provider_id
    return workspace.integrations?.find((integration) => integration.id === providerId)
  // eslint-disable-next-line react-hooks/exhaustive-deps -- Provider IDs are part of workspace settings
  }, [workspace.integrations, categoryValue])

  const emailSender = useMemo(() => {
    if (emailProvider) {
      return emailProvider.email_provider?.senders.find((sender) => sender.id === senderID)
    }
    return null
  }, [emailProvider, senderID])

  const createTemplateMutation = useMutation({
    mutationFn: (values: Record<string, unknown>) => {
      if (template) {
        return templatesApi.update({
          ...(values as Omit<UpdateTemplateRequest, 'channel' | 'workspace_id' | 'id'>),
          channel: 'email',
          workspace_id: workspace.id,
          id: template.id,
          base_version: baseVersionRef.current
        })
      } else {
        return templatesApi.create({
          ...(values as Omit<CreateTemplateRequest, 'channel' | 'workspace_id'>),
          channel: 'email',
          workspace_id: workspace.id
        })
      }
    },
    onSuccess: (response) => {
      codeEditorRef.current?.clearDraft()
      // Advance the base to the just-saved revision so a subsequent save isn't a false conflict.
      baseVersionRef.current = response.template.version
      markClean()
      savedSinceOpenRef.current = response.template
      const version = response.template.version
      message.success(
        template
          ? t`Saved as v${version}`
          : (!sendAfterSaveRef.current && successMessage) || t`Template created successfully`
      )
      // Saving an existing template keeps the editor open on the new revision: the author goes
      // on editing, and closes it when done. A new template still closes, as the field or page
      // that opened it picks it up.
      if (sendAfterSaveRef.current || template) {
        // The drawer stays open on the new revision; after "Save and send", the test send,
        // which sends what is saved, opens on it.
        const sendTest = sendAfterSaveRef.current
        sendAfterSaveRef.current = false
        setSavedTemplate(response.template)
        if (sendTest) setTestTemplate(response.template)
        queryClient.invalidateQueries({
          queryKey: ['templates', workspace.id]
        })
        if (template)
          queryClient.invalidateQueries({
            queryKey: ['template', workspace.id, template.id]
          })
        setLoading(false)
        return
      }
      handleClose()
      queryClient.invalidateQueries({ queryKey: ['templates', workspace.id] })
      setLoading(false)
    },
    onError: (error) => {
      sendAfterSaveRef.current = false
      // 409: the template advanced past our base while editing. Let the user decide
      // between keeping their work (overwrite) or loading the latest (discard theirs).
      if (error instanceof ApiError && error.status === 409) {
        const latest = (error.data as { latest_version?: number } | undefined)?.latest_version
        showConflict({
          latestVersion: latest,
          baseVersion: baseVersionRef.current,
          onOverwrite: () => {
            // Adopt the server's latest as the new base and re-submit. Fall back to 0
            // (skip the check, force the write) when the server didn't report a version,
            // so "overwrite" can never loop on a conflict it cannot resolve.
            baseVersionRef.current = latest ?? 0
            if (lastSubmittedValuesRef.current) {
              createTemplateMutation.mutate(lastSubmittedValuesRef.current)
            }
          },
          onReload: () => {
            void freshenFromServer(true)
          }
        })
        setLoading(false)
        return
      }
      message.error(template ? t`Failed to update template: ${error.message}` : t`Failed to create template: ${error.message}`)
      // A refused text version of the default language is written in the email editor;
      // one of a translation, in the translations.
      if (error.message.includes('text version') && !error.message.includes("translation '")) {
        setTab('template')
      }
      setLoading(false)
    }
  })

  const defaultTestData = useMemo(
    () =>
      buildDefaultTestData(
        workspace.settings.custom_endpoint_url || window.API_ENDPOINT,
        workspace.id
      ),
    [workspace.settings.custom_endpoint_url, workspace.id]
  )
  // A model chosen before the drawer opened (startModelId), built as the gallery builds it.
  const gallery = useGalleryModels(workspace, assets, defaultTestData)

  const loadTranslations = (translations?: Record<string, TemplateTranslation>) => {
    if (!translations) return
    const loaded: Record<string, TranslationEditorState> = {}
    for (const [lang, trans] of Object.entries(translations)) {
      loaded[lang] = {
        enabled: true,
        subject: trans.email?.subject || '',
        subjectPreview: trans.email?.subject_preview || '',
        visualEditorTree: trans.email?.visual_editor_tree
          ? (typeof trans.email.visual_editor_tree === 'object'
              ? (JSON.parse(JSON.stringify(trans.email.visual_editor_tree)) as EmailBlock)
              : (JSON.parse(trans.email.visual_editor_tree as unknown as string) as EmailBlock))
          : undefined,
        mjmlSource: trans.email?.mjml_source || undefined,
        text: customTextOrUndefined(trans.email?.text)
      }
    }
    setTranslationsState(loaded)
  }

  // Reseed every editor surface from a freshly-fetched template revision.
  const seedEditorFromLatest = (latest: Template) => {
    setEditorMode(latest.email?.editor_mode === 'code' ? 'code' : 'visual')
    if (latest.email?.editor_mode === 'code' && latest.email?.mjml_source) {
      setMjmlSource(latest.email.mjml_source)
    }
    form.setFieldsValue({
      name: latest.name,
      id: latest.id || templateIdFromName(latest.name),
      category: latest.category || undefined,
      email: {
        sender_id: latest.email?.sender_id || undefined,
        reply_to: latest.email?.reply_to || undefined,
        subject: latest.email?.subject || '',
        subject_preview: latest.email?.subject_preview || '',
        content: latest.email?.visual_editor_tree || '',
        visual_editor_tree: latest.email?.visual_editor_tree || createDefaultBlocks()
      },
      test_data: latest.test_data || defaultTestData
    })
    setEmailText(customTextOrUndefined(latest.email?.text))
    loadTranslations(latest.translations)
    if (latest.email?.visual_editor_tree) {
      if (typeof latest.email.visual_editor_tree === 'object') {
        setVisualEditorTree(latest.email.visual_editor_tree as unknown as EmailBlock)
      } else {
        try {
          setVisualEditorTree(JSON.parse(latest.email.visual_editor_tree) as EmailBlock)
        } catch (error) {
          console.error('Error parsing visual editor tree:', error)
        }
      }
    }
  }

  // Freshen the editor against the latest server revision. The list snapshot the drawer
  // opens from can be stale (refetchOnWindowFocus is off), so we re-read on open to
  // minimize conflicts. Reseeds only when safe (not dirty) or when forced by the user.
  const freshenFromServer = async (force = false) => {
    if (!template) return
    try {
      const { template: latest } = await templatesApi.get({
        workspace_id: workspace.id,
        id: template.id,
        version: 0
      })
      if (latest.version === baseVersionRef.current) return
      if (force || !dirtyRef.current) {
        seedEditorFromLatest(latest)
        baseVersionRef.current = latest.version
        setSavedTemplate(latest)
        markClean()
        if (!force) {
          message.info(t`Loaded the latest version of this template (v${latest.version}).`)
        }
      }
    } catch {
      // A forced reload (from the conflict dialog) is user-initiated, so surface the
      // failure. A background freshen-on-open stays silent — the current content is kept
      // and the server-side 409 check still guards the save.
      if (force) {
        message.error(t`Could not load the latest version. Please try again.`)
      }
    }
  }

  // A new template copying another: Duplicate, or a template picked in Your templates.
  const seedFromTemplate = (source: Template) => {
    // Clone template functionality - lock to source template's editor mode
    setEditorMode(source.email?.editor_mode === 'code' ? 'code' : 'visual')
    if (source.email?.editor_mode === 'code' && source.email?.mjml_source) {
      setMjmlSource(source.email.mjml_source)
    }
    // Append "copy" as suffix instead of "Copy of" prefix. The ID's base is cut so the
    // suffix fits the limit, and a copy never takes the original's (cut) ID.
    const copyIdBase = truncateIdentifier(
      kebabCase(source.name),
      TEMPLATE_ID_MAX_LENGTH - COPY_ID_SUFFIX.length
    )
    form.setFieldsValue({
      name: `${source.name} copy`,
      id: copyIdBase ? `${copyIdBase}${COPY_ID_SUFFIX}` : 'copy',
      category: source.category || forceCategory || undefined,
      email: {
        sender_id: source.email?.sender_id || undefined,
        reply_to: source.email?.reply_to || undefined,
        subject: source.email?.subject || '',
        subject_preview: source.email?.subject_preview || '',
        content: source.email?.visual_editor_tree || '',
        visual_editor_tree: source.email?.visual_editor_tree || createDefaultBlocks()
      },
      test_data: source.test_data || defaultTestData
    })

    setEmailText(customTextOrUndefined(source.email?.text))
    loadTranslations(source.translations)

    // Update the visual editor tree
    if (source.email?.visual_editor_tree) {
      if (typeof source.email.visual_editor_tree === 'object') {
        setVisualEditorTree(source.email.visual_editor_tree as unknown as EmailBlock)
      } else {
        try {
          setVisualEditorTree(JSON.parse(source.email.visual_editor_tree) as EmailBlock)
        } catch (error) {
          console.error('Error parsing visual editor tree:', error)
          message.error(t`Error loading template: Invalid template data`)
        }
      }
    }
  }

  const showDrawer = () => {
    if (template) {
      // Set editor mode from existing template
      setEditorMode(template.email?.editor_mode === 'code' ? 'code' : 'visual')
      if (template.email?.editor_mode === 'code' && template.email?.mjml_source) {
        setMjmlSource(template.email.mjml_source)
      }
      form.setFieldsValue({
        name: template.name,
        id: template.id || templateIdFromName(template.name),
        category: template.category || undefined,
        email: {
          sender_id: template.email?.sender_id || undefined,
          reply_to: template.email?.reply_to || undefined,
          subject: template.email?.subject || '',
          subject_preview: template.email?.subject_preview || '',
          content: template.email?.visual_editor_tree || '',
          visual_editor_tree: template.email?.visual_editor_tree || createDefaultBlocks()
        },
        test_data: template.test_data || defaultTestData
      })
      setEmailText(customTextOrUndefined(template.email?.text))
      loadTranslations(template.translations)
      // Track the revision this content is based on, then freshen against the server
      // so a stale list snapshot doesn't lead to a silent overwrite of newer edits.
      baseVersionRef.current = template.version
      setSavedTemplate(template)
      markClean()
      void freshenFromServer()
    } else if (fromTemplate) {
      seedFromTemplate(fromTemplate)
    } else if (initialModel) {
      // A model chosen before the drawer: it opens at once on a loading state, then on that model
      // as a pick in the gallery would, once the logo's size and a free ID are known. Closed
      // before then, nothing opens later.
      const round = startRoundRef.current
      setPreparing(true)
      setIsOpen(true)
      void gallery.start(initialModel).then((start) => {
        if (round === startRoundRef.current) void pickModel(start)
      })
      return
    } else if (!initialTree) {
      // A new template starts from a choice: the drawer opens once it is made.
      setStartModalOpen(true)
      return
    } else {
      // A new template from an imported file: nothing is saved before Save.
      if (initialName) {
        form.setFieldsValue({
          name: initialName,
          id: templateIdFromName(initialName)
        })
      }
      if (initialTestData) form.setFieldsValue({ test_data: initialTestData })
      if (initialTree) {
        setEditorMode('visual')
        setVisualEditorTree(initialTree)
      }
    }
    setIsOpen(true)

    // Calculate EmailBuilder height after drawer opens
    setTimeout(() => {
      // Height calculation: 100vh - drawer header - tabs - some padding
      setEmailBuilderHeight(`calc(100vh - ${HEADER_HEIGHT}px)`)
    }, 100)
  }

  // The first free ID from a name: the name's, then -2, -3… A template made twice from the same
  // model would otherwise be refused on Save for an ID it did not choose.
  const freeTemplateId = async (base: string): Promise<string> => {
    for (let n = 1; n < 10; n++) {
      const candidate = n === 1 ? base : truncateIdentifier(`${base}-${n}`, TEMPLATE_ID_MAX_LENGTH)
      try {
        await templatesApi.get({ workspace_id: workspace.id, id: candidate })
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) return candidate
        return base
      }
    }
    return base
  }

  const openAfterStart = () => {
    setPreparing(false)
    setStartModalOpen(false)
    markClean()
    setTab('settings')
    setIsOpen(true)
    setTimeout(() => setEmailBuilderHeight(`calc(100vh - ${HEADER_HEIGHT}px)`), 100)
  }

  const pickModel = async (start: GalleryStart) => {
    if (pickingRef.current) return
    pickingRef.current = true
    try {
      await applyModel(start)
    } finally {
      pickingRef.current = false
    }
  }

  const applyModel = async (start: GalleryStart) => {
    const round = startRoundRef.current
    // Blank leaves the name, subject and preview text to write, as a new template always did.
    const blank = start.modelId === 'blank'
    const id = blank ? undefined : await freeTemplateId(templateIdFromName(start.name))
    // Closed while the ID was looked up: the pick is dropped.
    if (round !== startRoundRef.current) return
    setCloneSource(undefined)
    setEditorMode('visual')
    setMjmlSource(STARTER_TEMPLATE)
    setEmailText(undefined)
    form.setFieldsValue({
      name: blank ? undefined : start.name,
      id,
      category: forceCategory || start.category,
      // A model sets no sender: one left by an earlier choice is not carried over.
      email: {
        sender_id: undefined,
        reply_to: undefined,
        subject: blank ? '' : start.subject,
        subject_preview: blank ? '' : start.subjectPreview
      },
      test_data: start.testData
    })
    setVisualEditorTree(start.tree)
    setTranslationsState({})
    // Blank's translations would only repeat its placeholder; a model's are its content.
    if (!blank) loadTranslations(start.translations)
    setStartModelId(blank ? undefined : start.modelId)
    const label = MODEL_LABELS[start.modelId as keyof typeof MODEL_LABELS]
    const languages = [start.language, ...Object.keys(start.translations)]
      .map(languageName)
      .join(', ')
    setStartInfo({
      name: label ? i18n._(label.name) : start.name,
      detail: t`Gallery · ${languages}`
    })
    openAfterStart()
  }

  const pickTemplate = (source: Template) => {
    setStartModelId(undefined)
    setCloneSource(source)
    seedFromTemplate(source)
    setStartInfo({ name: source.name, detail: t`Your templates` })
    openAfterStart()
  }

  // Change, from the Content card: the content will be replaced, so ask over changes.
  const changeStart = () => {
    if (!dirtyRef.current) {
      setStartModalOpen(true)
      return
    }
    modal.confirm({
      title: t`Replace the content?`,
      content: t`The changes you made will be replaced by the model you choose.`,
      okText: t`Choose another`,
      cancelText: t`Keep editing`,
      onOk: () => setStartModalOpen(true)
    })
  }

  const cancelStart = () => {
    setStartModalOpen(false)
    // Cancelled before the drawer opened: nothing was created.
    if (!isOpen) handleClose()
  }

  const handleClose = (notify = true) => {
    startRoundRef.current++
    setPreparing(false)
    setIsOpen(false)
    markClean()
    setStepsWithErrors([])
    setTranslationMissing(null)
    form.resetFields()
    setTab('settings')
    setEditorMode('visual')
    setMjmlSource(STARTER_TEMPLATE)
    setTranslationsState({})
    setEmailText(undefined)
    setStartInfo(undefined)
    setStartModelId(undefined)
    setCloneSource(fromTemplate)
    setSavedTemplate(template)
    textSessionRef.current++
    setTextSession(textSessionRef.current)
    const saved = savedSinceOpenRef.current
    savedSinceOpenRef.current = undefined
    if (notify && onClose) {
      onClose(saved)
    }
  }

  // Controlled mode: follow the page's open flag. Opening runs what the button runs; a close
  // the page decides on its own resets the drawer without calling onClose back.
  const controlled = open !== undefined
  const showDrawerRef = useRef(showDrawer)
  showDrawerRef.current = showDrawer
  const handleCloseRef = useRef(handleClose)
  handleCloseRef.current = handleClose
  const isOpenRef = useRef(isOpen)
  isOpenRef.current = isOpen
  useEffect(() => {
    if (!controlled) return
    if (open && !isOpenRef.current) showDrawerRef.current()
    if (!open && isOpenRef.current) handleCloseRef.current(false)
  }, [controlled, open])

  // Test data is saved with the template: a change through the builders' Template data tab, which
  // antd does not report as a form change, counts as one.
  const changeTestData = (testData: Record<string, unknown>) => {
    markDirty()
    form.setFieldsValue({ test_data: testData })
  }

  const handleImport = (tree: EmailBlock) => {
    markDirty()
    setVisualEditorTree(tree)
  }

  const handleSaveBlock: SaveBlockHandler = async (block, operation, nameOrId) => {
    let existingBlock: TemplateBlock | undefined
    let savedId = nameOrId
    try {
      if (operation === 'create') {
        const response = await templateBlocksApi.create({
          workspace_id: workspace.id,
          name: nameOrId,
          block: block
        })
        savedId = response.block.id
      } else {
        existingBlock = workspace.settings.template_blocks?.find((b) => b.id === nameOrId)
        if (!existingBlock) {
          throw new Error(t`Template block not found`)
        }
        await templateBlocksApi.update({
          workspace_id: workspace.id,
          id: nameOrId,
          name: existingBlock.name, // Preserve the existing name
          block: block
        })
      }
    } catch (error) {
      console.error('Failed to save template block:', error)
      const err = error as Error
      message.error(err?.message || t`Failed to save template block`)
      throw error
    }

    message.success(
      operation === 'create'
        ? t`Template block "${nameOrId}" saved successfully`
        : t`Template block "${existingBlock?.name}" updated successfully`
    )

    await refreshSavedBlocks(
      operation === 'create'
        ? t`Block saved. Reload the page to see it in your saved blocks.`
        : t`Block replaced. Reload the page to see its new content in your saved blocks.`
    )
    return savedId
  }

  // The change is made: a failed refresh only leaves the editor's list of saved blocks behind, so it is reported
  // with what was done, without failing the change, which would invite making it a second time
  const refreshSavedBlocks = async (warning: string) => {
    try {
      queryClient.invalidateQueries({ queryKey: ['workspace', workspace.id] })
      // Refresh workspaces in AuthContext so the EmailBuilder shows the saved blocks without a page refresh
      await refreshWorkspaces()
    } catch (error) {
      console.error('Failed to refresh the workspace after changing a template block:', error)
      message.warning(warning)
    }
  }

  const failedBlockChange = (error: unknown, fallback: string) => {
    console.error('Failed to change template block:', error)
    message.error((error as Error)?.message || fallback)
  }

  const handleRenameSavedBlock: RenameSavedBlockHandler = async (id, name) => {
    try {
      // The API replaces the whole entry, so the block goes with the new name. It is read from the server
      // rather than from the loaded workspace: a block replaced elsewhere since keeps its new content.
      const { block: current } = await templateBlocksApi.get({
        workspace_id: workspace.id,
        id
      })
      await templateBlocksApi.update({
        workspace_id: workspace.id,
        id,
        name,
        block: current.block
      })
    } catch (error) {
      failedBlockChange(error, t`Failed to rename the block`)
      // Deleted elsewhere: the library stops showing it
      if (error instanceof ApiError && error.status === 404) {
        await refreshSavedBlocks(t`Reload the page to update your saved blocks.`)
      }
      throw error
    }
    message.success(t`Block renamed to "${name}"`)
    await refreshSavedBlocks(t`Block renamed. Reload the page to update your saved blocks.`)
  }

  const handleDeleteSavedBlock: DeleteSavedBlockHandler = async (id) => {
    const name = workspace.settings.template_blocks?.find((b) => b.id === id)?.name ?? id
    try {
      await templateBlocksApi.delete({ workspace_id: workspace.id, id })
    } catch (error) {
      // Already gone, for instance deleted from another tab: what was asked is done
      if (!(error instanceof ApiError && error.status === 404)) {
        failedBlockChange(error, t`Failed to delete the block`)
        throw error
      }
    }
    message.success(t`Block "${name}" deleted`)
    await refreshSavedBlocks(t`Block deleted. Reload the page to update your saved blocks.`)
  }

  // Closing loses what changed since the drawer opened: ask first.
  const requestClose = () => {
    if (!dirtyRef.current) {
      handleClose()
      return
    }
    modal.confirm({
      title: t`Discard your changes?`,
      content: t`The changes you made since opening this template will be lost.`,
      okText: t`Discard`,
      okButtonProps: { danger: true },
      cancelText: t`Keep editing`,
      onOk: () => handleClose()
    })
  }

  const savedVersion = template ? (baseVersionRef.current ?? template.version) : undefined
  const testDisabledReason = !template
    ? t`Create the template to send it as a test`
    : !canTest
      ? t`You need Templates read and Transactional write permissions to send test emails`
      : undefined

  // A test sends the saved version. With changes waiting, say so and offer to save them first.
  const sendTest = () => {
    const saved = savedTemplate ?? template
    if (!saved) return
    if (!dirtyRef.current) {
      setTestTemplate(saved)
      return
    }
    const next = (savedVersion ?? saved.version) + 1
    const instance = modal.confirm({
      title: t`Send the saved version?`,
      // The test sends the template's latest saved revision, which may be newer than the one
      // this drawer opened if someone saved since: say "the saved version", not a number.
      content: t`Your changes are not saved yet. A test sends the saved version.`,
      closable: true,
      footer: (
        <div className="mt-4 flex justify-end gap-2">
          <Button onClick={() => instance.destroy()}>{t`Cancel`}</Button>
          <Button
            onClick={() => {
              instance.destroy()
              setTestTemplate(saved)
            }}
          >
            {t`Send the saved version`}
          </Button>
          <Button
            type="primary"
            onClick={() => {
              instance.destroy()
              sendAfterSaveRef.current = true
              form.submit()
            }}
          >
            {t`Save v${next} and send`}
          </Button>
        </div>
      )
    })
  }

  const hasSettingsErrors = () =>
    form
      .getFieldsError()
      .some((field) => field.errors.length > 0 && SETTINGS_FIELDS.includes(field.name.join('.')))

  return (
    <>
      {controlled ? null : (
        <Button type="primary" onClick={showDrawer} {...buttonProps}>
          {buttonContent ||
            (template ? t`Edit template` : fromTemplate ? t`Clone template` : t`Create template`)}
        </Button>
      )}
      {isOpen && (
        <FullDrawer
          open={isOpen}
          onClose={requestClose}
          backLabel={t`Templates`}
          ariaLabel={template ? t`Edit email template` : t`Create an email template`}
          copyLink={false}
          push={false}
          keyboard={false}
          fill
          barTitle={
            <TemplateBarTitle
              name={watchedName ?? ''}
              version={savedVersion}
              updatedAt={(savedTemplate ?? template)?.updated_at}
              dirty={dirty}
            />
          }
          barCenter={
            preparing ? undefined : (
              <TemplateStepSwitch
                step={tab}
                onChange={setTab}
                showTranslations={showTranslationsTab}
                translationCount={
                  Object.values(translationsState).filter((state) => state.enabled).length
                }
                stepsWithErrors={stepsWithErrors}
              />
            )
          }
          topActions={
            preparing ? undefined : (
              <TemplateBarActions
                version={savedVersion}
                dirty={dirty}
                saving={loading || createTemplateMutation.isPending}
                onSave={() => form.submit()}
                testDisabledReason={testDisabledReason}
                onSendTest={sendTest}
              />
            )
          }
        >
          {/* A model given before opening is still being built: nothing to edit or save yet. */}
          {preparing ? (
            <div data-testid="template-preparing" className="flex flex-1 items-center justify-center gap-2 text-sm text-ink-3">
              <Spin size="small" />
              <span>{t`Loading...`}</span>
            </div>
          ) : null}
          <Form
            form={form}
            layout="vertical"
            className={preparing ? 'hidden' : 'flex min-h-0 min-w-0 flex-1 flex-col'}
            onValuesChange={() => {
              markDirty()
            }}
            onFieldsChange={() => {
              if (stepsWithErrors.includes('settings') && !hasSettingsErrors()) {
                setStepsWithErrors((steps) => steps.filter((step) => step !== 'settings'))
              }
            }}
            onFinish={(values) => {
              setLoading(true)
              setStepsWithErrors([])
              if (editorMode === 'code') {
                values.email.editor_mode = 'code'
                values.email.mjml_source = mjmlSource
                // Code mode doesn't use visual_editor_tree
                delete values.email.visual_editor_tree
              } else {
                values.email.editor_mode = 'visual'
                values.email.visual_editor_tree = visualEditorTree
              }
              const defaultText = customTextOrUndefined(emailText)
              if (defaultText !== undefined) {
                values.email.text = defaultText
              } else {
                delete values.email.text
              }
              // Texts that miss the link their category needs: a warning, not a refusal.
              const missingLinkLanguages: string[] = []
              const missingVariable = missingRequiredLink(categoryValue, defaultText)
              if (missingVariable) {
                const lang = workspace.settings.default_language
                missingLinkLanguages.push((lang && SUPPORTED_LANGUAGES[lang]) || lang || '')
              }

              // Validate and build translations from state
              if (showTranslationsTab) {
                // Check enabled translations have required fields
                for (const [lang, state] of Object.entries(translationsState)) {
                  if (!state.enabled) continue
                  if (!state.subject || !state.subjectPreview) {
                    const langName = SUPPORTED_LANGUAGES[lang] || lang
                    message.error(t`${langName} translation is missing required fields (subject and preview)`)
                    setTab('translations')
                    setStepsWithErrors(['translations'])
                    missingAttemptRef.current += 1
                    setTranslationMissing({ lang, attempt: missingAttemptRef.current })
                    sendAfterSaveRef.current = false
                    setLoading(false)
                    return
                  }
                }

                const translations: Record<string, { email: TranslationEmailPayload }> = {}
                for (const [lang, state] of Object.entries(translationsState)) {
                  if (!state.enabled) continue
                  const emailTranslation: TranslationEmailPayload = {
                    subject: state.subject,
                    subject_preview: state.subjectPreview || ''
                  }
                  if (editorMode === 'code') {
                    emailTranslation.editor_mode = 'code'
                    emailTranslation.mjml_source = state.mjmlSource || ''
                  } else {
                    emailTranslation.editor_mode = 'visual'
                    emailTranslation.visual_editor_tree = state.visualEditorTree || visualEditorTree
                  }
                  const translationText = customTextOrUndefined(state.text)
                  if (translationText !== undefined) {
                    emailTranslation.text = translationText
                    if (missingRequiredLink(categoryValue, translationText)) {
                      missingLinkLanguages.push(SUPPORTED_LANGUAGES[lang] || lang)
                    }
                  }
                  translations[lang] = { email: emailTranslation }
                }
                // Always send translations (even empty) so disabling all clears them on the server
                values.translations = translations
              }

              if (missingLinkLanguages.length > 0) {
                const languages = missingLinkLanguages.filter(Boolean).join(', ')
                message.warning(
                  categoryValue === 'opt_in'
                    ? t`The custom text version (${languages}) has no confirmation link. Add ${CONFIRM_VARIABLE} so recipients can confirm their subscription.`
                    : t`The custom text version (${languages}) has no unsubscribe link. Add ${UNSUBSCRIBE_VARIABLE} so recipients can unsubscribe.`
                )
              }

              // Remember the payload so the conflict dialog can re-submit it verbatim.
              lastSubmittedValuesRef.current = values
              createTemplateMutation.mutate(values)
            }}
            onFinishFailed={(info) => {
              // Every step opens at any time, so a field in error may sit on another step:
              // open the step that holds it and mark it.
              const inSettings = (info.errorFields || []).some((field) =>
                SETTINGS_FIELDS.includes(field.name.join('.'))
              )
              if (inSettings) {
                setTab('settings')
                setStepsWithErrors(['settings'])
              }
              sendAfterSaveRef.current = false
              setLoading(false)
            }}
            initialValues={{
              'email.visual_editor_tree': visualEditorTree,
              category: forceCategory || undefined,
              test_data: (!template && !fromTemplate && initialTestData) || defaultTestData
            }}
          >
            <Form.Item name="test_data" hidden>
              <Input type="hidden" />
            </Form.Item>

            <div className="relative flex min-h-0 flex-1 flex-col">
              <div
                className="min-h-0 flex-1 overflow-y-auto px-5 pt-4 pb-6"
                style={{ display: tab === 'settings' ? 'block' : 'none' }}
              >
                <div className="flex flex-col gap-3 min-[1200px]:flex-row min-[1200px]:items-start">
                  <div className="flex min-w-0 flex-1 flex-col gap-3">
                    <TemplateSettingsStep
                      isSaved={!!template}
                      forceCategory={forceCategory}
                      editorMode={editorMode}
                      onEditorModeChange={setEditorMode}
                      editorModeLocked={!!(template || cloneSource || startModelId)}
                      showLanguage={showTranslationsTab}
                      defaultLanguage={workspace.settings.default_language}
                      senders={emailProvider?.email_provider?.senders ?? []}
                      categoryValue={categoryValue}
                      testData={watchedTestData}
                      onDirty={markDirty}
                      onNameChange={(name) => {
                        if (!template) {
                          form.setFieldsValue({ id: templateIdFromName(name) })
                          void form.validateFields(['id']).catch(() => undefined)
                        }
                      }}
                      validateId={async (value) => {
                        if (!value || template) return
                        let existing: { template?: Template } | undefined
                        try {
                          existing = await templatesApi.get({
                            workspace_id: workspace.id,
                            id: value
                          })
                        } catch {
                          return
                        }
                        // A deleted template keeps its id and no list shows it, so
                        // "already exists" alone sends the author looking for it.
                        throw new Error(
                          existing?.template?.deleted_at
                            ? t`This ID belongs to a deleted template and can't be reused`
                            : t`Template ID already exists`
                        )
                      }}
                    />
                  </div>
                  <aside className="flex shrink-0 flex-col gap-3 min-[1200px]:sticky min-[1200px]:top-0 min-[1200px]:w-aside">
                    {tab === 'settings' ? (
                      <TemplateSettingsAside
                        workspace={workspace}
                        email={{
                          subject: emailSubject,
                          subject_preview: emailPreview,
                          editor_mode: editorMode,
                          ...(editorMode === 'code'
                            ? { mjml_source: mjmlSource }
                            : {
                                visual_editor_tree:
                                  visualEditorTree as unknown as EmailTemplate['visual_editor_tree']
                              })
                        }}
                        testData={watchedTestData}
                        senderName={emailSender?.name || workspace.name}
                        onOpenContent={() => setTab('template')}
                        startingPoint={
                          startInfo ? { ...startInfo, onChange: changeStart } : undefined
                        }
                      />
                    ) : null}
                  </aside>
                </div>
              </div>

              <div style={{ display: tab === 'template' ? 'block' : 'none' }}>
                <Form.Item dependencies={['id']} style={{ margin: 0 }}>
                  {(form) => {
                    const testData = form.getFieldValue('test_data')
                    const templateId = form.getFieldValue('id')

                    if (editorMode === 'code') {
                      return (
                        <MjmlCodeEditor
                          ref={codeEditorRef}
                          mjmlSource={mjmlSource}
                          onMjmlSourceChange={(source) => {
                            markDirty()
                            setMjmlSource(source)
                          }}
                          onCompile={async (
                            mjml: string,
                            codeTestData?: Record<string, unknown>
                          ) => {
                            try {
                              const response = await templatesApi.compile({
                                workspace_id: workspace.id,
                                message_id: 'preview',
                                mjml_source: mjml,
                                test_data: codeTestData || {},
                                channel: 'email',
                                tracking_settings: {
                                  enable_tracking:
                                    workspace.settings?.email_tracking_enabled || false,
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
                          }}
                          testData={testData}
                          onTestDataChange={changeTestData}
                          height={emailBuilderHeight}
                          templateId={templateId || 'new'}
                          textVersion={{
                            custom: {
                              value: emailText,
                              onChange: changeEmailText,
                              onSeed: () => seedTextVersion(workspace, { mjmlSource }),
                              category: categoryValue
                            }
                          }}
                        />
                      )
                    }

                    return (
                      <EmailBuilder
                        tree={visualEditorTree}
                        onTreeChange={(tree) => {
                          // Visual edits go through here; mark dirty so a freshen-on-open
                          // refetch never silently reseeds over in-progress work.
                          markDirty()
                          setVisualEditorTree(tree)
                        }}
                        onCompile={async (
                          tree: EmailBlock,
                          builderTestData?: Record<string, unknown>
                        ) => {
                          try {
                            const response = await templatesApi.compile({
                              workspace_id: workspace.id,
                              message_id: 'preview',
                              visual_editor_tree: tree,
                              test_data: builderTestData || {},
                              channel: 'email',
                              tracking_settings: {
                                enable_tracking:
                                  workspace.settings?.email_tracking_enabled || false,
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
                              errors: [
                                {
                                  message: err.message || 'Compilation failed'
                                }
                              ]
                            }
                          }
                        }}
                        testData={testData}
                        onTestDataChange={changeTestData}
                        treePanelRef={treePanelRef as React.RefObject<HTMLDivElement>}
                        editPanelRef={editPanelRef as React.RefObject<HTMLDivElement>}
                        settingsPanelRef={settingsPanelRef as React.RefObject<HTMLDivElement>}
                        previewSwitcherRef={previewSwitcherRef as React.RefObject<HTMLDivElement>}
                        mobileDesktopSwitcherRef={
                          mobileDesktopSwitcherRef as React.RefObject<HTMLDivElement>
                        }
                        templateDataRef={templateDataRef as React.RefObject<PreviewRef>}
                        forcedViewMode={forcedViewMode}
                        savedBlocks={workspace.settings.template_blocks || []}
                        onSaveBlock={handleSaveBlock}
                        canManageSavedBlocks={canManageSavedBlocks}
                        onRenameSavedBlock={handleRenameSavedBlock}
                        onDeleteSavedBlock={handleDeleteSavedBlock}
                        onSelectBlock={setSelectedBlockId}
                        selectedBlockId={selectedBlockId}
                        hiddenBlocks={['mj-title', 'mj-preview']}
                        height={emailBuilderHeight}
                        textVersion={{
                          enabled: true,
                          custom: {
                            value: emailText,
                            onChange: changeEmailText,
                            onSeed: () =>
                              seedTextVersion(workspace, {
                                visualEditorTree: visualEditorTreeRef.current
                              }),
                            category: categoryValue
                          }
                        }}
                        toolbarActions={
                          <div className="flex items-center gap-2">
                            <HelpSupportDropdown
                              onStartTour={() => {
                                setTourOpen(true)
                              }}
                            />
                            <div ref={importExportButtonRef}>
                              <ImportExportButton
                                onImport={handleImport}
                                // onTestDataImport={handleTestDataImport}
                                tree={visualEditorTree}
                                testData={testData}
                                workspaceId={workspace.id}
                                templateName={template?.name}
                              />
                            </div>
                          </div>
                        }
                      />
                    )
                  }}
                </Form.Item>
              </div>

              {showTranslationsTab && (
                <div style={{ display: tab === 'translations' ? 'block' : 'none' }}>
                  <TemplateTranslationsTab
                    workspace={workspace}
                    editorMode={editorMode}
                    translationsState={translationsState}
                    onTranslationsStateChange={(s) => {
                      markDirty()
                      setTranslationsState(s)
                      // Every switched-on translation complete again: the step loses its error mark.
                      if (
                        translationMissing &&
                        Object.values(s).every((state) => !state.enabled || (state.subject && state.subjectPreview))
                      ) {
                        setTranslationMissing(null)
                        setStepsWithErrors((steps) => steps.filter((step) => step !== 'translations'))
                      }
                    }}
                    defaultSubject={emailSubject}
                    defaultSubjectPreview={emailPreview}
                    defaultVisualEditorTree={visualEditorTree}
                    defaultMjmlSource={mjmlSource}
                    testData={form.getFieldValue('test_data')}
                    onTestDataChange={changeTestData}
                    savedBlocks={workspace.settings.template_blocks || []}
                    onSaveBlock={handleSaveBlock}
                    canManageSavedBlocks={canManageSavedBlocks}
                    onRenameSavedBlock={handleRenameSavedBlock}
                    onDeleteSavedBlock={handleDeleteSavedBlock}
                    category={categoryValue}
                    templateName={template?.name}
                    missing={translationMissing}
                    senderName={emailSender?.name || workspace.name}
                  />
                </div>
              )}
            </div>
          </Form>
          <Tour
            open={tourOpen}
            onClose={() => {
              setTourOpen(false)
              // Reset forced view mode when tour closes
              setForcedViewMode(null)
              // Mark tour as seen
              localStorage.setItem('email-builder-tour-seen', 'true')
            }}
            onChange={(current) => {
              // Change email builder state based on tour step
              switch (current) {
                case 2: {
                  // Edit panel step (0-indexed)
                  // Select the body block to demonstrate block selection
                  const bodyBlock = visualEditorTree.children?.find(
                    (child) => child.type === 'mj-body'
                  )
                  if (bodyBlock) {
                    setSelectedBlockId(bodyBlock.id)
                  }
                  setForcedViewMode('edit')
                  break
                }
                case 4: // Preview step (0-indexed)
                case 5: // Mobile/Desktop preview step
                  // Automatically switch to preview mode when reaching the preview steps
                  setForcedViewMode('preview')
                  break
                case 6: // Template Data step
                  // Switch to preview mode and open template data editor
                  setForcedViewMode('preview')
                  // Open the template data editor using the ref
                  setTimeout(() => {
                    templateDataRef.current?.openTemplateDataEditor()
                  }, 500) // Small delay to ensure preview mode is active
                  break
                case 7: // Import/Export step
                  // Switch back to edit mode for import/export step
                  setForcedViewMode('edit')
                  // Close template data editor if open
                  templateDataRef.current?.closeTemplateDataEditor()
                  break
                default:
                  // For other steps, ensure we're in edit mode
                  setForcedViewMode('edit')
                  break
              }
            }}
            steps={[
              {
                title: t`Welcome to Email Builder!`,
                description:
                  t`Let's take a quick tour to help you get started with building beautiful emails using MJML.`,
                target: null // Center of screen
              },
              {
                title: t`Content Structure Tree`,
                description:
                  t`This is your content structure tree. You can drag and drop blocks to reorganize your email layout. Click the + buttons to add new blocks, or drag blocks from one section to another.`,
                target: () => treePanelRef.current!,
                placement: 'right' as const
              },
              {
                title: t`Visual Email Editor`,
                description:
                  t`This is your visual email editor. Click on any element in your email to select it. Selected elements will be highlighted with a blue border and show editing options.`,
                target: () => editPanelRef.current!,
                placement: 'top' as const
              },
              {
                title: t`Block Settings Panel`,
                description:
                  t`When you select a block, its settings appear here. Modify colors, text, spacing, alignment, and other properties to customize your email design.`,
                target: () => settingsPanelRef.current!,
                placement: 'left' as const
              },
              {
                title: t`Preview Your Email`,
                description:
                  t`Switch to Preview mode to see how your email will look to recipients. This shows the final rendered version with all styling applied.`,
                target: () => previewSwitcherRef.current!,
                placement: 'bottom' as const
              },
              {
                title: t`Mobile & Desktop Preview`,
                description:
                  t`Toggle between mobile and desktop views to see how your email appears on different devices. Mobile view shows a 400px width while desktop shows the full width.`,
                target: () => mobileDesktopSwitcherRef.current!,
                placement: 'left' as const
              },
              {
                title: t`Template Data & Liquid Templating`,
                description:
                  t`Use the Template Data tab to define dynamic content for your emails. Add variables like {{ name }} or {{ company }} in your email content, then define their values here. The Liquid templating engine supports conditionals, loops, and filters for powerful personalization.`,
                target: () =>
                  (templateDataRef.current?.getTemplateDataTabRef() as HTMLElement) || null,
                placement: 'top' as const
              },
              {
                title: t`Import & Export Templates`,
                description:
                  t`Use this button to import saved email templates or export your finished emails. You can import JSON/MJML templates or export as HTML, MJML, or JSON for future use.`,
                target: () => importExportButtonRef.current!,
                placement: 'bottom' as const
              }
            ]}
            indicatorsRender={(current, total) => (
              <span
                style={{
                  color: '#1890ff',
                  fontSize: '12px',
                  fontWeight: 'bold'
                }}
              >
                {current + 1} / {total}
              </span>
            )}
          />

          {/* AI Email Assistant - persists across tab switches, hidden in code mode */}
          <EmailAIAssistant
            hidden={tab !== 'template' || editorMode === 'code'}
            workspace={workspace}
            currentSubject={emailSubject}
            currentPreviewText={emailPreview}
            category={categoryValue}
            onUpdateSubject={(subject) => {
              markDirty()
              form.setFieldValue(['email', 'subject'], subject)
            }}
            onUpdatePreviewText={(preview) => {
              markDirty()
              form.setFieldValue(['email', 'subject_preview'], preview)
            }}
            validateOnComplete={async () => {
              // Compile the freshly-edited tree; surface MJML errors so a broken
              // generation is never presented as success.
              try {
                const response = await templatesApi.compile({
                  workspace_id: workspace.id,
                  message_id: 'preview',
                  visual_editor_tree: visualEditorTreeRef.current,
                  test_data: {},
                  channel: 'email',
                  tracking_settings: {
                    enable_tracking: workspace.settings?.email_tracking_enabled || false,
                    endpoint: workspace.settings?.custom_endpoint_url || undefined,
                    workspace_id: workspace.id,
                    message_id: 'preview'
                  }
                })
                if (response.error) {
                  const details = (response.error.details || [])
                    .map((d) => (d.line ? `• line ${d.line}: ${d.message}` : `• ${d.message}`))
                    .join('\n')
                  const errorText = [response.error.message, details].filter(Boolean).join('\n')
                  return {
                    ok: false,
                    errorText: errorText || 'MJML compilation failed'
                  }
                }
                return { ok: true }
              } catch (error) {
                return {
                  ok: false,
                  errorText: (error as Error).message || 'Compilation failed'
                }
              }
            }}
            callbacks={{
              getEmailTree: () => visualEditorTreeRef.current,
              setEmailTree: (tree) => {
                markDirty()
                setVisualEditorTree(tree)
              },
              onAddBlock: (parentId, blockType, position, content, attributes) => {
                markDirty()
                // Use functional updater to ensure we have the latest state
                setVisualEditorTree((prevTree) => {
                  // Create a new block with defaults
                  const newBlock = EmailBlockClass.createBlock(
                    blockType as MJMLComponentType,
                    undefined,
                    content,
                    prevTree
                  )
                  // Apply custom attributes if provided
                  if (attributes) {
                    newBlock.attributes = {
                      ...newBlock.attributes,
                      ...attributes
                    }
                  }
                  // Insert into tree
                  const updatedTree = EmailBlockClass.insertBlockIntoTree(
                    prevTree,
                    parentId,
                    newBlock,
                    position ?? (prevTree.children?.length || 0)
                  )
                  if (updatedTree) {
                    // A column or a group takes its part as in the editor, the other columns keeping their ratio
                    rebalanceAdded(updatedTree, parentId, newBlock.id)
                    // Schedule selection update after state is applied
                    setTimeout(() => setSelectedBlockId(newBlock.id), 0)
                    return updatedTree
                  }
                  return prevTree
                })
              },
              onUpdateBlock: (blockId, updates) => {
                markDirty()
                // Use functional updater to ensure atomic updates
                setVisualEditorTree((prevTree) =>
                  EmailBlockClass.mergeBlockUpdate(prevTree, blockId, updates)
                )
              },
              onDeleteBlock: (blockId) => {
                markDirty()
                setVisualEditorTree((prevTree) => {
                  const removedType = EmailBlockClass.findBlockById(prevTree, blockId)?.type
                  const parentId = parentIdOf(prevTree, blockId)
                  const updatedTree = EmailBlockClass.removeBlockFromTree(prevTree, blockId)
                  if (updatedTree) {
                    if (parentId) rebalanceRemoved(updatedTree, parentId, removedType)
                    // Clear selection if deleted block was selected
                    if (selectedBlockId === blockId) {
                      setTimeout(() => setSelectedBlockId(null), 0)
                    }
                    return updatedTree
                  }
                  return prevTree
                })
              },
              onMoveBlock: (blockId, newParentId, position) => {
                markDirty()
                setVisualEditorTree((prevTree) => {
                  const movedType = EmailBlockClass.findBlockById(prevTree, blockId)?.type
                  const sourceId = parentIdOf(prevTree, blockId)
                  const updatedTree = EmailBlockClass.moveBlockInTree(
                    prevTree,
                    blockId,
                    newParentId,
                    position
                  )
                  if (!updatedTree) return prevTree
                  // Both containers keep their proportions; a reorder within one leaves the widths alone
                  if (sourceId && sourceId !== newParentId) {
                    rebalanceRemoved(updatedTree, sourceId, movedType)
                    rebalanceAdded(updatedTree, newParentId, blockId)
                  }
                  return updatedTree
                })
              },
              onSelectBlock: (blockId) => {
                setSelectedBlockId(blockId)
              }
            }}
          />
        </FullDrawer>
      )}
      {conflictModal}
      {startModalOpen ? (
        <TemplateStartModal
          open
          workspace={workspace}
          assets={assets}
          defaultTestData={defaultTestData}
          forceCategory={forceCategory}
          canReadTemplates={!!permissions?.templates?.read}
          onCancel={cancelStart}
          onPickModel={(start) => void pickModel(start)}
          onPickTemplate={pickTemplate}
        />
      ) : null}
      {/* Outside the drawer: Save and send a test keeps it open, a later close must not cut the send. */}
      <SendTemplateModal
        isOpen={Boolean(testTemplate)}
        onClose={() => setTestTemplate(null)}
        template={testTemplate}
        workspace={workspace}
        canReadLists={!!permissions?.lists?.read}
        canReadContacts={!!permissions?.contacts?.read}
      />
    </>
  )
}
