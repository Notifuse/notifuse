import { useState, useEffect, useRef } from 'react'
import { Drawer, Button, Menu, Input, App, Modal, Space, Tabs, Form } from 'antd'
import { SaveOutlined, CloseOutlined, ExclamationCircleOutlined } from '@ant-design/icons'
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'
import Editor from '@monaco-editor/react'
import type { editor } from 'monaco-editor'
import { BlogTheme, BlogThemeFiles, blogThemesApi } from '../../services/api/blog'
import { useQueryClient, useMutation } from '@tanstack/react-query'
import { ThemePreview } from './ThemePreview'
import { DEFAULT_BLOG_TEMPLATES } from '../../utils/defaultBlogTemplates'
import { DEFAULT_BLOG_STYLES } from '../../utils/defaultBlogStyles'
import { BlogStyleSettings } from '../settings/BlogStyleSettings'
import { useDebouncedCallback } from 'use-debounce'
import { Workspace } from '../../services/api/types'

const { TextArea } = Input

interface ThemeEditorDrawerProps {
  open: boolean
  onClose: () => void
  theme: BlogTheme | null
  workspaceId: string
  workspace?: Workspace | null
}

interface ThemeFileType {
  key: keyof BlogThemeFiles
  label: string
}

const THEME_FILES: ThemeFileType[] = [
  { key: 'home', label: 'home.liquid' },
  { key: 'category', label: 'category.liquid' },
  { key: 'post', label: 'post.liquid' },
  { key: 'header', label: 'header.liquid' },
  { key: 'footer', label: 'footer.liquid' },
  { key: 'shared', label: 'shared.liquid' }
]

interface DraftState {
  files: BlogThemeFiles
  styling: any
  notes: string
  selectedFile: keyof BlogThemeFiles
  activeTab: 'templates' | 'styles'
  timestamp: number
}

const getLocalStorageKey = (workspaceId: string, version: number | null) =>
  `notifuse-theme-draft-${workspaceId}-${version || 'new'}`

export function ThemeEditorDrawer({
  open,
  onClose,
  theme,
  workspaceId,
  workspace
}: ThemeEditorDrawerProps) {
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()
  const [form] = Form.useForm()
  const [activeTab, setActiveTab] = useState<'templates' | 'styles'>('templates')
  const [selectedFile, setSelectedFile] = useState<keyof BlogThemeFiles>('home')
  const [files, setFiles] = useState<BlogThemeFiles>(DEFAULT_BLOG_TEMPLATES)
  const [styling, setStyling] = useState<any>(DEFAULT_BLOG_STYLES)
  const [notes, setNotes] = useState('')
  const [hasUnsavedChanges, setHasUnsavedChanges] = useState(false)
  const [showRestorePrompt, setShowRestorePrompt] = useState(false)
  const saveTimeoutRef = useRef<NodeJS.Timeout>()
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null)

  // Debounced preview files and styling for performance
  const [previewFiles, setPreviewFiles] = useState<BlogThemeFiles>(files)
  const [previewStyling, setPreviewStyling] = useState<any>(styling)
  const debouncedSetPreviewFiles = useDebouncedCallback((newFiles: BlogThemeFiles) => {
    setPreviewFiles(newFiles)
  }, 300)
  const debouncedSetPreviewStyling = useDebouncedCallback((newStyling: any) => {
    setPreviewStyling(newStyling)
  }, 300)

  const isPublished = theme?.published_at !== null && theme?.published_at !== undefined
  const localStorageKey = getLocalStorageKey(workspaceId, theme?.version || null)

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
            ;(window as any).__themeDraft = draft
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
      // New theme - use defaults
      setFiles(DEFAULT_BLOG_TEMPLATES)
      setPreviewFiles(DEFAULT_BLOG_TEMPLATES)
      setStyling(DEFAULT_BLOG_STYLES)
      setPreviewStyling(DEFAULT_BLOG_STYLES)
      form.setFieldsValue({ blog_settings: { styling: DEFAULT_BLOG_STYLES } })
      setNotes('')
      setSelectedFile('home')
      setActiveTab('templates')
      setHasUnsavedChanges(false)
    }
  }, [theme, open, localStorageKey])

  const loadThemeData = () => {
    if (theme) {
      setFiles(theme.files)
      setPreviewFiles(theme.files)
      const themeStyling = theme.styling || DEFAULT_BLOG_STYLES
      setStyling(themeStyling)
      setPreviewStyling(themeStyling)
      form.setFieldsValue({ blog_settings: { styling: themeStyling } })
      setNotes(theme.notes || '')
      setSelectedFile('home')
      setActiveTab('templates')
      setHasUnsavedChanges(false)
    }
  }

  const handleRestoreDraft = () => {
    const draft = (window as any).__themeDraft as DraftState
    if (draft) {
      setFiles(draft.files)
      setPreviewFiles(draft.files)
      setStyling(draft.styling)
      setPreviewStyling(draft.styling)
      form.setFieldsValue({ blog_settings: { styling: draft.styling } })
      setNotes(draft.notes)
      setSelectedFile(draft.selectedFile)
      setActiveTab(draft.activeTab)
      setHasUnsavedChanges(true)
      message.info('Draft restored from local storage')
    }
    setShowRestorePrompt(false)
    delete (window as any).__themeDraft
  }

  const handleDiscardDraft = () => {
    localStorage.removeItem(localStorageKey)
    setShowRestorePrompt(false)
    delete (window as any).__themeDraft
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
        styling,
        notes,
        selectedFile,
        activeTab,
        timestamp: Date.now()
      }
      localStorage.setItem(localStorageKey, JSON.stringify(draft))
    }, 500)

    return () => {
      if (saveTimeoutRef.current) {
        clearTimeout(saveTimeoutRef.current)
      }
    }
  }, [files, styling, notes, selectedFile, activeTab, open, isPublished, localStorageKey])

  // Track unsaved changes
  useEffect(() => {
    if (!theme) {
      // New theme
      const hasContent =
        JSON.stringify(files) !== JSON.stringify(DEFAULT_BLOG_TEMPLATES) ||
        JSON.stringify(styling) !== JSON.stringify(DEFAULT_BLOG_STYLES) ||
        notes.trim() !== ''
      setHasUnsavedChanges(hasContent)
    } else {
      // Existing theme
      const filesChanged = JSON.stringify(files) !== JSON.stringify(theme.files)
      const stylingChanged =
        JSON.stringify(styling) !== JSON.stringify(theme.styling || DEFAULT_BLOG_STYLES)
      const notesChanged = notes !== (theme.notes || '')
      setHasUnsavedChanges(filesChanged || stylingChanged || notesChanged)
    }
  }, [files, styling, notes, theme])

  // Update preview files and styling with debounce
  useEffect(() => {
    debouncedSetPreviewFiles(files)
  }, [files, debouncedSetPreviewFiles])

  useEffect(() => {
    debouncedSetPreviewStyling(styling)
  }, [styling, debouncedSetPreviewStyling])

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

  const handleFormValuesChange = (changedValues: any) => {
    if (changedValues.blog_settings?.styling) {
      setStyling(changedValues.blog_settings.styling)
    }
  }

  const saveMutation = useMutation({
    mutationFn: async () => {
      const isPublished = theme?.published_at !== null && theme?.published_at !== undefined

      if (!theme) {
        // Create new theme
        return await blogThemesApi.create(workspaceId, { files, styling, notes })
      } else if (isPublished) {
        // Published theme with changes: create new version
        return await blogThemesApi.create(workspaceId, {
          files,
          styling,
          notes: notes ? `Edited from v${theme.version}: ${notes}` : `Edited from v${theme.version}`
        })
      } else {
        // Unpublished draft: update in place
        return await blogThemesApi.update(workspaceId, {
          version: theme.version,
          files,
          styling,
          notes
        })
      }
    },
    onSuccess: (data) => {
      const isPublished = theme?.published_at !== null && theme?.published_at !== undefined

      if (isPublished) {
        message.success(`Created new version v${data.theme.version}`)
      } else if (theme) {
        message.success('Theme updated successfully')
      } else {
        message.success('Theme created successfully')
      }

      setHasUnsavedChanges(false)
      localStorage.removeItem(localStorageKey)
      queryClient.invalidateQueries({ queryKey: ['blog-themes', workspaceId] })
      onClose()
    },
    onError: (error: any) => {
      message.error(error?.message || 'Failed to save theme')
    }
  })

  const handleSave = () => {
    saveMutation.mutate()
  }

  const handleClose = () => {
    if (hasUnsavedChanges) {
      modal.confirm({
        title: 'Unsaved Changes',
        icon: <ExclamationCircleOutlined />,
        content:
          'You have unsaved changes. Your changes are saved in local storage. Are you sure you want to close?',
        okText: 'Close',
        cancelText: 'Cancel',
        onOk: () => {
          onClose()
        }
      })
    } else {
      localStorage.removeItem(localStorageKey)
      onClose()
    }
  }

  const handleDiscard = () => {
    modal.confirm({
      title: 'Discard Changes',
      icon: <ExclamationCircleOutlined />,
      content:
        'Are you sure you want to discard all changes? This will remove the local storage draft.',
      okText: 'Discard',
      okType: 'danger',
      cancelText: 'Cancel',
      onOk: () => {
        localStorage.removeItem(localStorageKey)
        setHasUnsavedChanges(false)
        onClose()
      }
    })
  }

  return (
    <>
      <Drawer
        title={
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <span>
              {theme ? `Edit Theme v${theme.version}` : 'Create New Theme'}
              {hasUnsavedChanges && <span style={{ color: '#faad14', marginLeft: 8 }}>●</span>}
            </span>
            <Space>
              {hasUnsavedChanges && (
                <Button onClick={handleDiscard} danger>
                  Discard
                </Button>
              )}
              <Button
                type="primary"
                icon={<SaveOutlined />}
                onClick={handleSave}
                loading={saveMutation.isPending}
                disabled={!hasUnsavedChanges}
              >
                Save
              </Button>
              <Button icon={<CloseOutlined />} onClick={handleClose}>
                Close
              </Button>
            </Space>
          </div>
        }
        open={open}
        onClose={handleClose}
        width="100%"
        closable={false}
        styles={{ body: { padding: 0, height: 'calc(100vh - 55px)' } }}
      >
        <PanelGroup direction="horizontal" autoSaveId={`theme-editor-${workspaceId}`}>
          {/* Left: Tabs Sidebar - ~250px on typical screens */}
          <Panel defaultSize={15} minSize={12} maxSize={25}>
            <div
              style={{
                display: 'flex',
                flexDirection: 'column',
                height: '100%',
                borderRight: '1px solid #f0f0f0'
              }}
            >
              <Tabs
                activeKey={activeTab}
                onChange={(key) => setActiveTab(key as 'templates' | 'styles')}
                style={{ flex: 1, display: 'flex', flexDirection: 'column' }}
                tabBarStyle={{ margin: 0, paddingLeft: 16, paddingRight: 16 }}
                items={[
                  {
                    key: 'templates',
                    label: 'Templates',
                    children: (
                      <div style={{ overflow: 'auto', height: '100%' }}>
                        <Menu
                          mode="inline"
                          selectedKeys={[selectedFile]}
                          items={THEME_FILES.map((file) => ({
                            key: file.key,
                            label: file.label,
                            onClick: () => setSelectedFile(file.key)
                          }))}
                          style={{ border: 'none' }}
                        />
                      </div>
                    )
                  },
                  {
                    key: 'styles',
                    label: 'Styles',
                    children: (
                      <div style={{ overflow: 'auto', height: '100%', padding: 16 }}>
                        <Form
                          form={form}
                          layout="vertical"
                          onValuesChange={handleFormValuesChange}
                        >
                          <BlogStyleSettings />
                        </Form>
                      </div>
                    )
                  }
                ]}
              />

              <div
                style={{
                  borderTop: '1px solid #f0f0f0',
                  padding: 16,
                  display: 'flex',
                  flexDirection: 'column'
                }}
              >
                <div style={{ fontSize: 12, color: '#8c8c8c', marginBottom: 8 }}>VERSION NOTES</div>
                <TextArea
                  value={notes}
                  onChange={(e) => setNotes(e.target.value)}
                  placeholder="Add notes about this version..."
                  rows={4}
                  style={{ resize: 'none' }}
                />
              </div>
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

          {/* Middle: Monaco Editor (only shown for Templates tab) */}
          {activeTab === 'templates' && (
            <Panel defaultSize={40} minSize={25} maxSize={60}>
              <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
                <div
                  style={{
                    padding: '8px 16px',
                    borderBottom: '1px solid #f0f0f0',
                    background: '#fafafa',
                    fontSize: 13
                  }}
                >
                  Editing: <strong>{THEME_FILES.find((f) => f.key === selectedFile)?.label}</strong>
                </div>
                <div style={{ flex: 1 }}>
                  <Editor
                    height="100%"
                    language="html"
                    value={files[selectedFile]}
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
            </Panel>
          )}

          {activeTab === 'templates' && (
            <PanelResizeHandle
              style={{
                width: 1,
                background: '#e0e0e0',
                cursor: 'col-resize',
                position: 'relative'
              }}
            />
          )}

          {/* Right: Preview */}
          <Panel defaultSize={activeTab === 'templates' ? 40 : 80} minSize={25} maxSize={80}>
            <ThemePreview files={previewFiles} styling={previewStyling} workspace={workspace} />
          </Panel>
        </PanelGroup>
      </Drawer>

      {/* Restore Draft Modal */}
      <Modal
        title="Restore Draft?"
        open={showRestorePrompt}
        onOk={handleRestoreDraft}
        onCancel={handleDiscardDraft}
        okText="Restore Draft"
        cancelText="Discard Draft"
      >
        <p>A newer draft was found in local storage. Would you like to restore it or discard it?</p>
      </Modal>
    </>
  )
}
