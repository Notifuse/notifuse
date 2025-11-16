import { useState, useEffect, useRef } from 'react'
import { Drawer, Button, Input, App, Modal, Space, Tabs, Form } from 'antd'
import { ExclamationCircleOutlined } from '@ant-design/icons'
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
import { ThemePreset } from './themePresets'

const { TextArea } = Input

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
  workspace,
  presetData
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
  const [showSaveModal, setShowSaveModal] = useState(false)
  const [previewPage, setPreviewPage] = useState<'home' | 'category' | 'post'>('home')
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
      // New theme - use preset data if provided, otherwise use defaults
      const initialFiles = presetData?.files || DEFAULT_BLOG_TEMPLATES
      const initialStyling = presetData?.styling || DEFAULT_BLOG_STYLES
      const initialNotes = presetData ? `Created from ${presetData.name}` : ''

      setFiles(initialFiles)
      setPreviewFiles(initialFiles)
      setStyling(initialStyling)
      setPreviewStyling(initialStyling)
      form.setFieldsValue({ blog_settings: { styling: initialStyling } })
      setNotes(initialNotes)
      setSelectedFile('home')
      setActiveTab('templates')
      setHasUnsavedChanges(false)
    }
  }, [theme, open, localStorageKey, presetData])

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
    setShowSaveModal(true)
  }

  const handleConfirmSave = () => {
    setShowSaveModal(false)
    saveMutation.mutate()
  }

  const handleCancel = () => {
    if (hasUnsavedChanges) {
      modal.confirm({
        title: 'Unsaved Changes',
        icon: <ExclamationCircleOutlined />,
        content:
          'You have unsaved changes. Are you sure you want to close? Your changes will be lost.',
        okText: 'Close',
        cancelText: 'Cancel',
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

  const handleTabChange = (key: string) => {
    setActiveTab(key as 'templates' | 'styles')
  }

  const leftPanelSize = activeTab === 'templates' ? 50 : 40
  const rightPanelSize = activeTab === 'templates' ? 50 : 60

  return (
    <>
      <Drawer
        title={
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <span>
              {theme
                ? `Edit Theme v${theme.version}`
                : presetData
                  ? `Create New Theme - ${presetData.name}`
                  : 'Create New Theme'}
              {hasUnsavedChanges && <span style={{ color: '#faad14', marginLeft: 8 }}>●</span>}
            </span>
            <Space>
              <Button type="text" onClick={handleCancel}>
                Cancel
              </Button>
              <Button
                type="primary"
                onClick={handleSave}
                loading={saveMutation.isPending}
                disabled={!hasUnsavedChanges}
              >
                Save
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
        <PanelGroup key={`theme-editor-${activeTab}`} direction="horizontal">
          {/* Left: Tabs Content */}
          <Panel defaultSize={leftPanelSize} minSize={25} maxSize={80}>
            <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
              <Tabs
                activeKey={activeTab}
                onChange={handleTabChange}
                style={{ height: '100%', display: 'flex', flexDirection: 'column' }}
                tabBarStyle={{ margin: 0, paddingLeft: 16, paddingRight: 16 }}
                items={[
                  {
                    key: 'templates',
                    label: 'Templates',
                    children: (
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
                              {file.key === 'shared' && (
                                <div
                                  style={{
                                    padding: '12px 16px',
                                    background: '#f5f7fa',
                                    borderBottom: '1px solid #e0e0e0',
                                    fontSize: '13px',
                                    lineHeight: '1.6',
                                    maxHeight: '120px',
                                    overflowY: 'auto'
                                  }}
                                >
                                  <div
                                    style={{ fontWeight: 600, marginBottom: 6, color: '#1a1a1a' }}
                                  >
                                    📬 Newsletter Subscription Form
                                  </div>
                                  <div style={{ color: '#666', marginBottom: 8 }}>
                                    This template includes a ready-to-use newsletter subscription
                                    form. Copy the form HTML to your <strong>footer.liquid</strong>{' '}
                                    or <strong>home.liquid</strong> for site-wide newsletter
                                    signups.
                                  </div>
                                  <div style={{ color: '#666' }}>
                                    <strong>Available variables:</strong>{' '}
                                    <code
                                      style={{
                                        background: '#fff',
                                        padding: '2px 4px',
                                        borderRadius: 2
                                      }}
                                    >
                                      workspace
                                    </code>
                                    ,{' '}
                                    <code
                                      style={{
                                        background: '#fff',
                                        padding: '2px 4px',
                                        borderRadius: 2
                                      }}
                                    >
                                      public_lists
                                    </code>
                                    ,{' '}
                                    <code
                                      style={{
                                        background: '#fff',
                                        padding: '2px 4px',
                                        borderRadius: 2
                                      }}
                                    >
                                      post
                                    </code>
                                    ,{' '}
                                    <code
                                      style={{
                                        background: '#fff',
                                        padding: '2px 4px',
                                        borderRadius: 2
                                      }}
                                    >
                                      category
                                    </code>
                                    ,{' '}
                                    <code
                                      style={{
                                        background: '#fff',
                                        padding: '2px 4px',
                                        borderRadius: 2
                                      }}
                                    >
                                      posts
                                    </code>
                                    ,{' '}
                                    <code
                                      style={{
                                        background: '#fff',
                                        padding: '2px 4px',
                                        borderRadius: 2
                                      }}
                                    >
                                      categories
                                    </code>
                                  </div>
                                </div>
                              )}
                              <div style={{ flex: 1, minHeight: 0 }}>
                                <Editor
                                  height="100%"
                                  language="html"
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
                    )
                  },
                  {
                    key: 'styles',
                    label: 'Styles',
                    children: (
                      <div style={{ overflow: 'auto', height: '100%', padding: 16 }}>
                        <Form form={form} layout="vertical" onValuesChange={handleFormValuesChange}>
                          <BlogStyleSettings />
                        </Form>
                      </div>
                    )
                  }
                ]}
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

          {/* Right: Preview */}
          <Panel defaultSize={rightPanelSize} minSize={20} maxSize={75}>
            <Tabs
              activeKey={previewPage}
              onChange={(key) => setPreviewPage(key as 'home' | 'category' | 'post')}
              type="card"
              style={{ height: '100%', display: 'flex', flexDirection: 'column' }}
              tabBarStyle={{ margin: 0, paddingLeft: 16, paddingRight: 16 }}
              items={[
                {
                  key: 'home',
                  label: 'Home',
                  children: (
                    <div style={{ height: 'calc(100vh - 110px)', overflow: 'auto' }}>
                      <ThemePreview
                        files={previewFiles}
                        styling={previewStyling}
                        workspace={workspace}
                        view="home"
                      />
                    </div>
                  )
                },
                {
                  key: 'category',
                  label: 'Category',
                  children: (
                    <div style={{ height: 'calc(100vh - 110px)', overflow: 'auto' }}>
                      <ThemePreview
                        files={previewFiles}
                        styling={previewStyling}
                        workspace={workspace}
                        view="category"
                      />
                    </div>
                  )
                },
                {
                  key: 'post',
                  label: 'Post',
                  children: (
                    <div style={{ height: 'calc(100vh - 110px)', overflow: 'auto' }}>
                      <ThemePreview
                        files={previewFiles}
                        styling={previewStyling}
                        workspace={workspace}
                        view="post"
                      />
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
        title="Restore Draft?"
        open={showRestorePrompt}
        onOk={handleRestoreDraft}
        onCancel={handleDiscardDraft}
        okText="Restore Draft"
        cancelText="Discard Draft"
      >
        <p>A newer draft was found in local storage. Would you like to restore it or discard it?</p>
      </Modal>

      {/* Save Modal */}
      <Modal
        title="Save Theme"
        open={showSaveModal}
        onOk={handleConfirmSave}
        onCancel={() => setShowSaveModal(false)}
        okText="Save"
        cancelText="Cancel"
        confirmLoading={saveMutation.isPending}
      >
        <div style={{ marginBottom: 16 }}>
          <div style={{ fontSize: 12, color: '#8c8c8c', marginBottom: 8 }}>
            VERSION NOTES (OPTIONAL)
          </div>
          <TextArea
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder="Add notes about this version..."
            rows={4}
            style={{ resize: 'none' }}
          />
        </div>
      </Modal>
    </>
  )
}
