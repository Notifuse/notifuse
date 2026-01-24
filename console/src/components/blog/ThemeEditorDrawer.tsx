import { useState, useEffect, useRef } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Drawer, Button, Input, App, Modal, Space, Tabs, Segmented } from 'antd'
import { ExclamationCircleOutlined } from '@ant-design/icons'
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'
import Editor from '@monaco-editor/react'
import type { editor } from 'monaco-editor'
import { BlogTheme, BlogThemeFiles, blogThemesApi } from '../../services/api/blog'
import { useQueryClient, useMutation } from '@tanstack/react-query'
import { ThemePreview } from './ThemePreview'
import { useDebouncedCallback } from 'use-debounce'
import { Workspace } from '../../services/api/types'
import { ThemePreset, THEME_PRESETS } from './themePresets'

const { TextArea } = Input

interface DraftWindow extends Window {
  __themeDraft?: DraftState
}

interface ThemeEditorDrawerProps {
  open: boolean
  onClose: () => void
  theme: BlogTheme | null
  workspaceId: string
  workspace?: Workspace | null
  presetData?: ThemePreset | null
}

interface ThemeFileType {
  key: keyof BlogThemeFiles
  label: string
}

const THEME_FILES: ThemeFileType[] = [
  { key: 'home.liquid', label: 'home.liquid' },
  { key: 'category.liquid', label: 'category.liquid' },
  { key: 'post.liquid', label: 'post.liquid' },
  { key: 'header.liquid', label: 'header.liquid' },
  { key: 'footer.liquid', label: 'footer.liquid' },
  { key: 'shared.liquid', label: 'shared.liquid' },
  { key: 'styles.css', label: 'styles.css' },
  { key: 'scripts.js', label: 'scripts.js' }
]

interface DraftState {
  files: BlogThemeFiles
  notes: string
  selectedFile: keyof BlogThemeFiles
  timestamp: number
}

const getLocalStorageKey = (workspaceId: string, version: number | null) =>
  `notifuse-theme-draft-${workspaceId}-${version || 'new'}`

export function ThemeEditorDrawer({
  open,
  onClose,
  theme,
  workspaceId,
  workspace,
  presetData
}: ThemeEditorDrawerProps) {
  const { t } = useLingui()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()
  const [selectedFile, setSelectedFile] = useState<keyof BlogThemeFiles>('home.liquid')
  const [files, setFiles] = useState<BlogThemeFiles>(THEME_PRESETS[0].files)
  const [notes, setNotes] = useState('')
  const [hasUnsavedChanges, setHasUnsavedChanges] = useState(false)
  const [showRestorePrompt, setShowRestorePrompt] = useState(false)
  const [showSaveModal, setShowSaveModal] = useState(false)
  const [previewPage, setPreviewPage] = useState<'home' | 'category' | 'post'>('home')
  const [isFullscreen, setIsFullscreen] = useState(false)
  const saveTimeoutRef = useRef<NodeJS.Timeout>()
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null)

  // Debounced preview files for performance
  const [previewFiles, setPreviewFiles] = useState<BlogThemeFiles>(files)
  const debouncedSetPreviewFiles = useDebouncedCallback((newFiles: BlogThemeFiles) => {
    setPreviewFiles(newFiles)
  }, 300)

  const isPublished = theme?.published_at !== null && theme?.published_at !== undefined
  const localStorageKey = getLocalStorageKey(workspaceId, theme?.version || null)

  // Helper function to load theme data - defined before useEffect that uses it
  const loadThemeData = () => {
    if (theme) {
      setFiles(theme.files)
      setPreviewFiles(theme.files)
      setNotes(theme.notes || '')
      setSelectedFile('home.liquid')
      setHasUnsavedChanges(false)
    }
  }

  // Load theme data or draft from localStorage
  useEffect(() => {
    if (!open) return

    if (theme) {
      // Check for localStorage draft
      const draftStr = localStorage.getItem(localStorageKey)
      if (draftStr) {
        try {
          const draft: DraftState = JSON.parse(draftStr)
          // Offer to restore draft if it's newer than the theme
          if (draft.timestamp > new Date(theme.updated_at).getTime()) {
            setShowRestorePrompt(true)
            // Temporarily store draft for potential restoration
            ;(window as DraftWindow).__themeDraft = draft
          } else {
            // Draft is older, discard it
            localStorage.removeItem(localStorageKey)
            loadThemeData()
          }
        } catch (e) {
          console.error('Failed to parse draft from localStorage', e)
          loadThemeData()
        }
      } else {
        loadThemeData()
      }
    } else {
      // New theme - use preset data if provided, otherwise use defaults
      const initialFiles = presetData?.files || THEME_PRESETS[0].files
      const initialNotes = presetData ? `Created from ${presetData.name}` : ''

      setFiles(initialFiles)
      setPreviewFiles(initialFiles)
      setNotes(initialNotes)
      setSelectedFile('home.liquid')
      setHasUnsavedChanges(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [theme, open, localStorageKey, presetData])

  const handleRestoreDraft = () => {
    const draft = (window as DraftWindow).__themeDraft
    if (draft) {
      setFiles(draft.files)
      setPreviewFiles(draft.files)
      setNotes(draft.notes)
      setSelectedFile(draft.selectedFile)
      setHasUnsavedChanges(true)
      message.info(t`Draft restored from local storage`)
    }
    setShowRestorePrompt(false)
    delete (window as DraftWindow).__themeDraft
  }

  const handleDiscardDraft = () => {
    localStorage.removeItem(localStorageKey)
    setShowRestorePrompt(false)
    delete (window as DraftWindow).__themeDraft
    loadThemeData()
  }

  // Auto-save to localStorage (debounced)
  useEffect(() => {
    if (!open) return

    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current)
    }

    saveTimeoutRef.current = setTimeout(() => {
      const draft: DraftState = {
        files,
        notes,
        selectedFile,
        timestamp: Date.now()
      }
      localStorage.setItem(localStorageKey, JSON.stringify(draft))
    }, 500)

    return () => {
      if (saveTimeoutRef.current) {
        clearTimeout(saveTimeoutRef.current)
      }
    }
  }, [files, notes, selectedFile, open, isPublished, localStorageKey])

  // Track unsaved changes
  useEffect(() => {
    if (!theme) {
      // New theme
      const hasContent =
        JSON.stringify(files) !== JSON.stringify(THEME_PRESETS[0].files) || notes.trim() !== ''
      setHasUnsavedChanges(hasContent)
    } else {
      // Existing theme
      const filesChanged = JSON.stringify(files) !== JSON.stringify(theme.files)
      const notesChanged = notes !== (theme.notes || '')
      setHasUnsavedChanges(filesChanged || notesChanged)
    }
  }, [files, notes, theme])

  // Update preview files with debounce
  useEffect(() => {
    debouncedSetPreviewFiles(files)
  }, [files, debouncedSetPreviewFiles])

  const handleEditorDidMount = (editor: editor.IStandaloneCodeEditor) => {
    editorRef.current = editor
    editor.updateOptions({
      automaticLayout: true
    })
  }

  const handleEditorChange = (value: string | undefined) => {
    if (value === undefined) return
    setFiles((prev) => ({
      ...prev,
      [selectedFile]: value
    }))
  }

  const saveMutation = useMutation({
    mutationFn: async () => {
      const isPublished = theme?.published_at !== null && theme?.published_at !== undefined

      if (!theme) {
        // Create new theme
        return await blogThemesApi.create(workspaceId, { files, notes })
      } else if (isPublished) {
        // Published theme with changes: create new version
        return await blogThemesApi.create(workspaceId, {
          files,
          notes: notes ? `Edited from v${theme.version}: ${notes}` : `Edited from v${theme.version}`
        })
      } else {
        // Unpublished draft: update in place
        return await blogThemesApi.update(workspaceId, {
          version: theme.version,
          files,
          notes
        })
      }
    },
    onSuccess: (data) => {
      const isPublished = theme?.published_at !== null && theme?.published_at !== undefined

      if (isPublished) {
        message.success(t`Created new version v${data.theme.version}`)
      } else if (theme) {
        message.success(t`Theme updated successfully`)
      } else {
        message.success(t`Theme created successfully`)
      }

      setHasUnsavedChanges(false)
      localStorage.removeItem(localStorageKey)
      queryClient.invalidateQueries({ queryKey: ['blog-themes', workspaceId] })
      onClose()
    },
    onError: (error: Error) => {
      message.error(error?.message || t`Failed to save theme`)
    }
  })

  const handleSave = () => {
    setShowSaveModal(true)
  }

  const handleConfirmSave = () => {
    setShowSaveModal(false)
    saveMutation.mutate()
  }

  const handleCancel = () => {
    if (hasUnsavedChanges) {
      modal.confirm({
        title: t`Unsaved Changes`,
        icon: <ExclamationCircleOutlined />,
        content:
          t`You have unsaved changes. Are you sure you want to close? Your changes will be lost.`,
        okText: t`Close`,
        cancelText: t`Cancel`,
        onOk: () => {
          localStorage.removeItem(localStorageKey)
          onClose()
        }
      })
    } else {
      localStorage.removeItem(localStorageKey)
      onClose()
    }
  }

  const leftPanelSize = 50
  const rightPanelSize = 50

  return (
    <>
      <Drawer
        title={
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <span>
              {theme
                ? t`Edit Theme v${theme.version}`
                : presetData
                  ? t`Create New Theme - ${presetData.name}`
                  : t`Create New Theme`}
              {hasUnsavedChanges && <span style={{ color: '#faad14', marginLeft: 8 }}>●</span>}
            </span>
            <Space>
              <Button type="text" onClick={handleCancel}>
                {t`Cancel`}
              </Button>
              <Button
                type="primary"
                onClick={handleSave}
                loading={saveMutation.isPending}
                disabled={!hasUnsavedChanges}
              >
                {t`Save`}
              </Button>
            </Space>
          </div>
        }
        open={open}
        onClose={handleCancel}
        width="100%"
        closable={false}
        styles={{ body: { padding: 0, height: 'calc(100vh - 55px)' } }}
      >
        <PanelGroup direction="horizontal" key={isFullscreen ? 'fullscreen' : 'split'}>
          {/* Left: File Editor */}
          {!isFullscreen && (
            <>
              <Panel defaultSize={leftPanelSize} minSize={25} maxSize={80}>
                <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
                  {/* File Tabs */}
                  <Tabs
                    activeKey={selectedFile}
                    onChange={(key) => setSelectedFile(key as keyof BlogThemeFiles)}
                    type="card"
                    size="small"
                    style={{
                      height: 'calc(100vh - 110px)',
                      display: 'flex',
                      flexDirection: 'column'
                    }}
                    tabBarStyle={{ margin: 0 }}
                    items={THEME_FILES.map((file) => ({
                      key: file.key,
                      label: file.label,
                      children: (
                        <div
                          style={{
                            height: 'calc(100vh - 155px)',
                            display: 'flex',
                            flexDirection: 'column'
                          }}
                        >
                          <div style={{ flex: 1, minHeight: 0 }}>
                            <Editor
                              height="100%"
                              language={file.key === 'styles.css' ? 'css' : 'html'}
                              value={files[file.key]}
                              onChange={handleEditorChange}
                              onMount={handleEditorDidMount}
                              theme="vs-light"
                              options={{
                                minimap: { enabled: false },
                                fontSize: 14,
                                lineNumbers: 'on',
                                scrollBeyondLastLine: false,
                                wordWrap: 'on',
                                automaticLayout: true,
                                tabSize: 2
                              }}
                            />
                          </div>
                        </div>
                      )
                    }))}
                  />
                </div>
              </Panel>

              <PanelResizeHandle
                style={{
                  width: 1,
                  background: '#e0e0e0',
                  cursor: 'col-resize',
                  position: 'relative'
                }}
              />
            </>
          )}

          {/* Right: Preview */}
          <Panel defaultSize={isFullscreen ? 100 : rightPanelSize} minSize={20} maxSize={100}>
            <Tabs
              activeKey={previewPage}
              onChange={(key) => setPreviewPage(key as 'home' | 'category' | 'post')}
              type="card"
              style={{ height: '100%', display: 'flex', flexDirection: 'column' }}
              tabBarStyle={{ margin: 0, paddingLeft: 16, paddingRight: 16 }}
              tabBarExtraContent={{
                right: (
                  <Segmented
                    size="small"
                    value={isFullscreen ? 'fullscreen' : 'split'}
                    onChange={(value) => setIsFullscreen(value === 'fullscreen')}
                    options={[
                      { label: t`Split`, value: 'split' },
                      { label: t`Fullscreen`, value: 'fullscreen' }
                    ]}
                  />
                )
              }}
              items={[
                {
                  key: 'home',
                  label: t`Home`,
                  children: (
                    <div style={{ height: 'calc(100vh - 110px)', overflow: 'auto' }}>
                      <ThemePreview files={previewFiles} workspace={workspace} view="home" />
                    </div>
                  )
                },
                {
                  key: 'category',
                  label: t`Category`,
                  children: (
                    <div style={{ height: 'calc(100vh - 110px)', overflow: 'auto' }}>
                      <ThemePreview files={previewFiles} workspace={workspace} view="category" />
                    </div>
                  )
                },
                {
                  key: 'post',
                  label: t`Post`,
                  children: (
                    <div style={{ height: 'calc(100vh - 110px)', overflow: 'auto' }}>
                      <ThemePreview files={previewFiles} workspace={workspace} view="post" />
                    </div>
                  )
                }
              ]}
            />
          </Panel>
        </PanelGroup>
      </Drawer>

      {/* Restore Draft Modal */}
      <Modal
        title={t`Restore Draft?`}
        open={showRestorePrompt}
        onOk={handleRestoreDraft}
        onCancel={handleDiscardDraft}
        okText={t`Restore Draft`}
        cancelText={t`Discard Draft`}
      >
        <p>{t`A newer draft was found in local storage. Would you like to restore it or discard it?`}</p>
      </Modal>

      {/* Save Modal */}
      <Modal
        title={t`Save Theme`}
        open={showSaveModal}
        onOk={handleConfirmSave}
        onCancel={() => setShowSaveModal(false)}
        okText={t`Save`}
        cancelText={t`Cancel`}
        confirmLoading={saveMutation.isPending}
      >
        <div style={{ marginBottom: 16 }}>
          <div style={{ fontSize: 12, color: '#8c8c8c', marginBottom: 8 }}>
            {t`VERSION NOTES (OPTIONAL)`}
          </div>
          <TextArea
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder={t`Add notes about this version...`}
            rows={4}
            style={{ resize: 'none' }}
          />
        </div>
      </Modal>
    </>
  )
}
