import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ChangeEvent, DragEvent, ReactNode } from 'react'
import { Alert, App, Button, Dropdown, Form, Input, Modal, Result, Segmented, Tooltip } from 'antd'
import type { MenuProps } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import {
  CheckCircle2,
  ChevronRight,
  Clock,
  FolderPlus,
  HardDrive,
  LayoutGrid,
  List,
  Loader2,
  MoreHorizontal,
  RefreshCw,
  Search,
  Settings,
  Upload,
  XCircle
} from 'lucide-react'
import { filesize } from 'filesize'
import { copyToClipboard } from '../../lib/copyToClipboard'
import { useMediaQuery } from '../../lib/useMediaQuery'
import { PageHeader, Panel, RailLayout, RailSection, SidePanel } from '../layout'
import ButtonFilesSettings from './buttonSettings'
import { DeleteFolderModal } from './DeleteFolderModal'
import { FileGrid } from './FileGrid'
import { FilePreview } from './FilePreview'
import { FileTable } from './FileTable'
import { FolderTree } from './FolderTree'
import { downloadFile, downloadZip } from './fileDownloads'
import { useImageSize } from './imageDimensions'
import type { FileManagerProps, FileManagerSettings, StorageObject } from './interfaces'
import { MAX_OBJECTS, toStorageItems } from './storageObjects'
import {
  ancestorFolders,
  childrenOf,
  fileCounts,
  folderName,
  folderTree,
  parentFolder,
  searchFiles,
  totalSize
} from './storageTree'
import { useStorage } from './useStorage'

export type FileExplorerMode = 'page' | 'picker'
type View = 'list' | 'grid'

export interface FileExplorerProps extends FileManagerProps {
  mode: FileExplorerMode
}

const VIEW_KEYS: Record<FileExplorerMode, string> = {
  page: 'notifuse.files.view.page',
  picker: 'notifuse.files.view.picker'
}

const readView = (mode: FileExplorerMode): View => {
  try {
    const saved = window.localStorage.getItem(VIEW_KEYS[mode])
    if (saved === 'list' || saved === 'grid') return saved
  } catch {
    // Storage blocked (private window): the default view.
  }
  return mode === 'picker' ? 'grid' : 'list'
}

const saveView = (mode: FileExplorerMode, view: View) => {
  try {
    window.localStorage.setItem(VIEW_KEYS[mode], view)
  } catch {
    // Not kept: the view still changes for this visit.
  }
}

interface UploadStatusRow {
  file: File
  status: 'pending' | 'uploading' | 'done' | 'failed'
}

// Above the picker modal (1300), for the confirmations and dialogs it opens.
const PICKER_OVERLAY_Z = 1400

/** Whether a modal or drawer other than the explorer's own covers it: keys then belong to it. */
function coveredByOverlay(root: HTMLElement | null): boolean {
  if (!root) return true
  const own = root.closest<HTMLElement>('.ant-modal-wrap, .ant-drawer')
  const overlays = document.querySelectorAll<HTMLElement>('.ant-drawer.ant-drawer-open, .ant-modal-wrap')
  const popupOpen =
    document.querySelector('.ant-popover:not(.ant-popover-hidden), .ant-dropdown:not(.ant-dropdown-hidden)') !==
    null
  if (popupOpen) return true
  return Array.from(overlays).some(
    (overlay) =>
      overlay !== own &&
      !overlay.contains(root) &&
      overlay.style.display !== 'none' &&
      (!own || (own.compareDocumentPosition(overlay) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0)
  )
}

const isEditable = (target: EventTarget | null) =>
  target instanceof HTMLElement &&
  (['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName) ||
    target.closest('[contenteditable]:not([contenteditable="false"]), [role="textbox"]') !== null)

/**
 * The file manager: folder tree, list or grid of a folder, and the chosen file's preview. The page
 * mode fills the Files page; the picker mode fills the image picker's modal and inserts a file.
 */
export function FileExplorer(props: FileExplorerProps) {
  const { mode } = props
  const isPicker = mode === 'picker'
  const readOnly = props.readOnly ?? false
  const { t, i18n } = useLingui()
  const { message, modal } = App.useApp()
  const overlayZ = isPicker ? PICKER_OVERLAY_Z : undefined

  const storage = useStorage({ workspaceId: props.workspaceId, settings: props.settings })
  const { items } = storage

  // Folder: the page keeps it in the URL, the picker in the provider.
  const [internalPath, setInternalPath] = useState(props.currentPath || '')
  const isControlledPath = props.controlledPath !== undefined && props.onPathChange !== undefined
  const currentPath = isControlledPath ? (props.controlledPath as string) : internalPath

  // Chosen file: the page keeps it in the URL, the picker in its own state.
  const [internalFile, setInternalFile] = useState<string | undefined>(undefined)
  const isControlledFile = props.onFileChange !== undefined
  const selectedKey = isControlledFile ? props.controlledFile : internalFile
  const setSelectedKey = useCallback(
    (key: string | undefined) => {
      if (props.onFileChange) props.onFileChange(key)
      else setInternalFile(key)
    },
    [props]
  )

  const [checkedKeys, setCheckedKeys] = useState<string[]>([])
  const [search, setSearch] = useState('')
  const [view, setView] = useState<View>(() => readView(mode))
  const [newFolderOpen, setNewFolderOpen] = useState(false)
  const [newFolderLoading, setNewFolderLoading] = useState(false)
  const [folderToDelete, setFolderToDelete] = useState<string | null>(null)
  const [isDeletingFolder, setIsDeletingFolder] = useState(false)
  const [isZipping, setIsZipping] = useState(false)
  const [isDeletingFiles, setIsDeletingFiles] = useState(false)
  const [uploadStatuses, setUploadStatuses] = useState<UploadStatusRow[]>([])
  const [isUploading, setIsUploading] = useState(false)
  const [isDragging, setIsDragging] = useState(false)
  const dragCounterRef = useRef(0)
  const abortRef = useRef<AbortController | null>(null)
  const inputFileRef = useRef<HTMLInputElement>(null)
  const rootRef = useRef<HTMLDivElement>(null)
  const [form] = Form.useForm()

  // A folder delete can take minutes: when it ends, the user may have moved on or left.
  const mountedRef = useRef(true)
  // Set synchronously, so a second confirmation before the next render is ignored too.
  const deletingFolderRef = useRef(false)
  const currentPathRef = useRef(currentPath)
  useEffect(() => {
    currentPathRef.current = currentPath
  }, [currentPath])
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  const wide = useMediaQuery('(min-width: 1200px)')
  // The picker's modal always has room for the preview column; the page needs 1200px.
  const previewAsColumn = isPicker || wide

  const counts = useMemo(() => fileCounts(items), [items])
  const tree = useMemo(() => folderTree(items), [items])
  const total = useMemo(() => totalSize(items), [items])
  const searching = search.trim() !== ''
  const visibleItems = useMemo(
    () => (searching ? searchFiles(items, search) : childrenOf(items, currentPath)),
    [items, search, searching, currentPath]
  )
  const visibleFiles = useMemo(() => visibleItems.filter((item) => !item.is_folder), [visibleItems])
  const selectedItem = useMemo(
    () => items.find((item) => item.key === selectedKey && !item.is_folder),
    [items, selectedKey]
  )
  const checkedFiles = useMemo(
    () => items.filter((item) => !item.is_folder && checkedKeys.includes(item.key)),
    [items, checkedKeys]
  )

  // A file of the URL that the bucket does not hold (deleted elsewhere) closes the preview,
  // unless the listing is incomplete and the file may lie beyond it.
  useEffect(() => {
    if (!selectedKey || !storage.isSuccess || storage.isFetching) return
    if (storage.isCapped || storage.cutShort) return
    if (!items.some((item) => item.key === selectedKey && !item.is_folder)) setSelectedKey(undefined)
  }, [selectedKey, storage.isSuccess, storage.isFetching, storage.isCapped, storage.cutShort, items, setSelectedKey])

  const isRefused = useCallback(
    (item: StorageObject) => isPicker && !item.is_folder && !props.acceptItem(item),
    [isPicker, props]
  )

  const goToPath = useCallback(
    (path: string) => {
      setCheckedKeys([])
      setSearch('')
      if (isControlledPath) {
        props.onPathChange?.(path)
      } else {
        setInternalPath(path)
      }
      if (!isControlledFile) setInternalFile(undefined)
    },
    [isControlledPath, isControlledFile, props]
  )

  const reportError = (error: unknown) =>
    props.onError(error instanceof Error ? error : new Error(String(error)))

  const insert = (item: StorageObject) => {
    if (item.is_folder || isRefused(item)) return
    props.onSelect([item])
  }

  const openItem = (item: StorageObject) => {
    if (item.is_folder) goToPath(item.key)
    else if (!isRefused(item)) setSelectedKey(item.key)
  }

  // J and K walk through the files of the list; Escape closes the page's preview column.
  const navIndex = selectedItem ? visibleFiles.findIndex((item) => item.key === selectedItem.key) : -1
  const onPrev = navIndex > 0 ? () => setSelectedKey(visibleFiles[navIndex - 1].key) : undefined
  const onNext =
    navIndex >= 0 && navIndex < visibleFiles.length - 1
      ? () => setSelectedKey(visibleFiles[navIndex + 1].key)
      : undefined
  const navRef = useRef({ onPrev, onNext, close: () => setSelectedKey(undefined) })
  useEffect(() => {
    navRef.current = { onPrev, onNext, close: () => setSelectedKey(undefined) }
  })
  const previewOpen = selectedItem !== undefined
  useEffect(() => {
    if (!previewOpen || !previewAsColumn) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.repeat) return
      if (event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) return
      if (isEditable(event.target) || coveredByOverlay(rootRef.current)) return
      const key = event.key.toLowerCase()
      // In the picker, Escape belongs to the modal, which closes.
      const action =
        key === 'j'
          ? navRef.current.onNext
          : key === 'k'
            ? navRef.current.onPrev
            : key === 'escape' && !isPicker
              ? navRef.current.close
              : undefined
      if (!action) return
      event.preventDefault()
      action()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [previewOpen, previewAsColumn, isPicker])

  // Actions

  const copyUrl = async (item: StorageObject) => {
    if (await copyToClipboard(item.file_info.url)) message.success(t`URL copied to clipboard.`)
    else message.error(t`Failed to copy to clipboard`)
  }

  const download = (item: StorageObject) => {
    downloadFile(item).catch((error: unknown) => {
      console.error('Download failed:', error)
      message.error(t`Failed to download file`)
    })
  }

  const confirmDeleteFile = (item: StorageObject) => {
    modal.confirm({
      title: t`Do you want to permanently delete this file from your storage?`,
      okText: t`Delete`,
      cancelText: t`Cancel`,
      okButtonProps: { danger: true },
      zIndex: overlayZ,
      onOk: () =>
        storage
          .deleteFile(item.key)
          .then(() => {
            message.success(t`File deleted successfully.`)
            setCheckedKeys((keys) => keys.filter((key) => key !== item.key))
            if (selectedKey === item.key) setSelectedKey(undefined)
          })
          .catch((error: unknown) => {
            message.error(t`Failed to delete file: ` + error)
            reportError(error)
          })
    })
  }

  const runFolderDelete = (folder: string) => {
    if (deletingFolderRef.current) return
    deletingFolderRef.current = true
    setIsDeletingFolder(true)
    storage
      .deleteFolder(folder)
      .then(({ failed, deletedFiles, failedFiles }) => {
        if (failed === 0) {
          message.success(t`Folder deleted successfully.`)
          // Go to the parent folder, unless the user has left the deleted folder or the file
          // manager in the meantime.
          if (mountedRef.current && currentPathRef.current.startsWith(folder)) {
            goToPath(parentFolder(folder))
          }
        } else {
          message.error(
            plural(deletedFiles, {
              one: `Deleted # file, ${failedFiles} failed`,
              other: `Deleted # files, ${failedFiles} failed`
            })
          )
        }
      })
      .catch((error: unknown) => {
        message.error(t`Failed to delete folder: ` + error)
        reportError(error)
      })
      .finally(() => {
        deletingFolderRef.current = false
        if (!mountedRef.current) return
        setIsDeletingFolder(false)
        setFolderToDelete(null)
      })
  }

  const downloadChecked = async () => {
    if (checkedFiles.length === 0) return
    setIsZipping(true)
    try {
      await downloadZip(checkedFiles)
      message.success(
        plural(checkedFiles.length, { one: 'Downloaded # file', other: 'Downloaded # files' })
      )
    } catch (error) {
      console.error('ZIP download failed:', error)
      message.error(t`Failed to download files`)
    } finally {
      if (mountedRef.current) setIsZipping(false)
    }
  }

  const confirmDeleteChecked = () => {
    const keys = checkedFiles.map((item) => item.key)
    if (keys.length === 0) return
    modal.confirm({
      title: plural(keys.length, {
        one: 'Delete # selected file?',
        other: 'Delete # selected files?'
      }),
      okText: t`Delete`,
      cancelText: t`Cancel`,
      okButtonProps: { danger: true },
      zIndex: overlayZ,
      onOk: async () => {
        setIsDeletingFiles(true)
        try {
          const { deleted, failed } = await storage.deleteFiles(keys)
          if (failed === 0) {
            message.success(plural(deleted, { one: 'Deleted # file', other: 'Deleted # files' }))
          } else {
            message.warning(
              plural(deleted, {
                one: `Deleted # file, ${failed} failed`,
                other: `Deleted # files, ${failed} failed`
              })
            )
          }
          setCheckedKeys([])
          if (selectedKey && keys.includes(selectedKey)) setSelectedKey(undefined)
        } finally {
          if (mountedRef.current) setIsDeletingFiles(false)
        }
      }
    })
  }

  const startUpload = async (files: File[]) => {
    if (files.length === 0 || isUploading) return
    setUploadStatuses(files.map((file) => ({ file, status: 'pending' })))
    setIsUploading(true)
    const controller = new AbortController()
    abortRef.current = controller
    const folder = currentPath
    try {
      const written = await storage.upload(files, folder, {
        signal: controller.signal,
        onStatus: (index, status) =>
          setUploadStatuses((rows) =>
            rows.map((row, rowIndex) => (rowIndex === index ? { ...row, status } : row))
          )
      })
      // The picker chooses the file just sent, when there is one and it can be used here.
      if (isPicker && files.length === 1 && written.length === 1 && mountedRef.current) {
        const [uploaded] = toStorageItems(
          [{ Key: written[0], Size: files[0].size, LastModified: new Date() }],
          props.settings as FileManagerSettings
        )
        if (props.acceptItem(uploaded)) setSelectedKey(uploaded.key)
      }
    } catch (error) {
      message.error(t`Upload failed: ` + error)
      reportError(error)
    } finally {
      abortRef.current = null
      if (mountedRef.current) {
        setIsUploading(false)
        setUploadStatuses([])
      }
    }
  }

  const onFileChange = (event: ChangeEvent<HTMLInputElement>) => {
    const files = event.target.files ? Array.from(event.target.files) : []
    // Reset, so the same file can be chosen again.
    event.target.value = ''
    void startUpload(files)
  }

  const cancelUpload = () => {
    abortRef.current?.abort()
    message.info(t`Import cancelled`)
  }

  const onDragEnter = (event: DragEvent) => {
    event.preventDefault()
    event.stopPropagation()
    if (readOnly) return
    dragCounterRef.current++
    if (dragCounterRef.current === 1) setIsDragging(true)
  }
  const onDragOver = (event: DragEvent) => {
    event.preventDefault()
    event.stopPropagation()
  }
  const onDragLeave = (event: DragEvent) => {
    event.preventDefault()
    event.stopPropagation()
    if (readOnly) return
    dragCounterRef.current = Math.max(0, dragCounterRef.current - 1)
    if (dragCounterRef.current === 0) setIsDragging(false)
  }
  const onDrop = (event: DragEvent) => {
    event.preventDefault()
    event.stopPropagation()
    dragCounterRef.current = 0
    setIsDragging(false)
    if (readOnly) {
      message.warning(t`Cannot upload in read-only mode`)
      return
    }
    void startUpload(Array.from(event.dataTransfer.files))
  }

  const submitNewFolder = () => {
    if (newFolderLoading) return
    form.validateFields().then(async (values: { name: string }) => {
      const key = currentPath + values.name + '/'
      form.resetFields()
      setNewFolderOpen(false)
      setNewFolderLoading(true)
      try {
        const outcome = await storage.createFolder(key)
        if (outcome === 'exists') message.error(t`Folder already exists.`)
        else message.success(t`Folder created successfully.`)
      } catch (error) {
        message.error(t`Failed to create folder: ` + error)
        reportError(error)
      } finally {
        if (mountedRef.current) setNewFolderLoading(false)
      }
    })
  }

  const updateSettings = async (settings: FileManagerSettings) => {
    await props.onUpdateSettings(settings)
    // Same bucket and endpoint, new keys: the cached listing may belong to the old ones.
    void storage.invalidate()
  }

  // Menus

  const writeEntry = (
    key: string,
    label: string,
    readOnlyTip: string,
    onClick: () => void,
    danger = false
  ): NonNullable<MenuProps['items']>[number] => ({
    key,
    danger,
    disabled: readOnly,
    label: readOnly ? <Tooltip title={readOnlyTip}>{label}</Tooltip> : label,
    onClick
  })

  const menuFor = (item: StorageObject): MenuProps['items'] => {
    if (item.is_folder) {
      return [
        { key: 'open', label: t`Open`, onClick: () => goToPath(item.key) },
        writeEntry('delete-folder', t`Delete folder`, t`Delete folder (Read-only mode)`, () =>
          setFolderToDelete(item.key), true)
      ]
    }
    return [
      { key: 'copy', label: t`Copy URL`, onClick: () => void copyUrl(item) },
      {
        key: 'open',
        label: t`Open in a new tab`,
        onClick: () => window.open(item.file_info.url, '_blank', 'noopener,noreferrer')
      },
      { key: 'download', label: t`Download`, onClick: () => download(item) },
      writeEntry('delete', t`Delete`, t`Delete file (Read-only mode)`, () => confirmDeleteFile(item), true)
    ]
  }

  // Pieces

  const uploadInput = (
    <input
      type="file"
      ref={inputFileRef}
      onChange={onFileChange}
      hidden
      accept={props.acceptFileType}
      multiple
      data-testid="file-manager-upload-input"
    />
  )

  const browseFiles = () => inputFileRef.current?.click()

  const settingsButton = readOnly ? (
    <Tooltip title={t`Storage settings (Read-only mode)`}>
      <Button
        type="text"
        size="small"
        disabled
        aria-label={t`Storage settings`}
        icon={<Settings size={15} aria-hidden="true" />}
      />
    </Tooltip>
  ) : (
    <ButtonFilesSettings
      settings={props.settings}
      onUpdateSettings={updateSettings}
      settingsInfo={props.settingsInfo}
    >
      <Tooltip title={t`Storage settings`}>
        <Button
          type="text"
          size="small"
          aria-label={t`Storage settings`}
          icon={<Settings size={15} aria-hidden="true" />}
        />
      </Tooltip>
    </ButtonFilesSettings>
  )

  const storageFooter = (
    <div className="flex items-center justify-between gap-2">
      <div className="flex min-w-0 flex-col">
        <span className="text-ink-3">{t`S3 storage`}</span>
        <span className="truncate text-ink-2">
          {props.settings?.bucket} · {filesize(total, { round: 0 })}
        </span>
      </div>
      {settingsButton}
    </div>
  )

  const folderTreeSection = (
    <RailSection title={t`Folders`}>
      <FolderTree tree={tree} counts={counts} currentPath={currentPath} onOpen={goToPath} />
    </RailSection>
  )

  const breadcrumb = (
    <nav aria-label={t`Breadcrumb`} className="flex min-w-0 items-center gap-1 text-sm">
      {[{ key: '', label: t`Root` }, ...ancestorFolders(currentPath).map((key) => ({ key, label: folderName(key) }))].map(
        (crumb, index, crumbs) => {
          const last = index === crumbs.length - 1 && !searching
          return (
            <span key={crumb.key || 'root'} className="flex min-w-0 items-center gap-1">
              {index > 0 ? <ChevronRight size={12} aria-hidden="true" className="shrink-0 text-ink-4" /> : null}
              {last ? (
                <span aria-current="page" className="truncate font-medium text-ink">
                  {crumb.label}
                </span>
              ) : (
                <button
                  type="button"
                  onClick={() => goToPath(crumb.key)}
                  className="truncate text-ink-3 hover:text-ink focus-visible:outline-none"
                >
                  {crumb.label}
                </button>
              )}
            </span>
          )
        }
      )}
    </nav>
  )

  const viewSwitch = (
    <Segmented<View>
      size="small"
      value={view}
      onChange={(value) => {
        setView(value)
        saveView(mode, value)
      }}
      options={[
        {
          value: 'grid',
          icon: <LayoutGrid size={14} aria-hidden="true" />,
          title: t`Grid`,
          label: <span className="sr-only">{t`Grid`}</span>
        },
        {
          value: 'list',
          icon: <List size={14} aria-hidden="true" />,
          title: t`List`,
          label: <span className="sr-only">{t`List`}</span>
        }
      ]}
    />
  )

  const toolbar =
    !isPicker && checkedFiles.length > 0 ? (
      <div
        data-selection-bar
        className="flex items-center gap-2 border-b border-line-soft bg-primary-50 px-3 py-2 text-sm"
      >
        <span className="font-medium text-ink">
          {plural(checkedFiles.length, { one: '# selected', other: '# selected' })}
        </span>
        <div className="flex flex-1 items-center justify-end gap-2">
          <Button size="small" loading={isZipping} onClick={() => void downloadChecked()}>
            {t`Download as zip`}
          </Button>
          {readOnly ? (
            <Tooltip title={t`Delete file (Read-only mode)`}>
              <Button size="small" danger disabled>
                {t`Delete`}
              </Button>
            </Tooltip>
          ) : (
            <Button size="small" danger loading={isDeletingFiles} onClick={confirmDeleteChecked}>
              {t`Delete`}
            </Button>
          )}
          <Button size="small" type="link" onClick={() => setCheckedKeys([])}>
            {t`Clear`}
          </Button>
        </div>
      </div>
    ) : (
      <div className="flex items-center gap-2 border-b border-line-soft px-3 py-2">
        <div className="min-w-0 flex-1">{breadcrumb}</div>
        <div className="flex shrink-0 items-center gap-2">
          <Input
            allowClear
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t`File name`}
            aria-label={t`File name`}
            prefix={<Search size={14} aria-hidden="true" />}
            style={{ width: 240 }}
          />
          <Tooltip title={t`Refresh the list`}>
            <Button
              type="text"
              size="small"
              aria-label={t`Refresh the list`}
              onClick={() => void storage.refetch()}
              icon={
                <RefreshCw
                  size={15}
                  aria-hidden="true"
                  className={storage.isFetching ? 'animate-spin' : undefined}
                />
              }
            />
          </Tooltip>
          {viewSwitch}
          {isPicker ? (
            <>
              <Tooltip title={readOnly ? t`Upload file (Read-only mode)` : undefined}>
                <Button
                  icon={<Upload size={14} aria-hidden="true" />}
                  disabled={readOnly}
                  loading={isUploading}
                  onClick={browseFiles}
                >
                  {t`Upload`}
                </Button>
              </Tooltip>
              <Tooltip title={readOnly ? t`New folder (Read-only mode)` : t`New folder`}>
                <Button
                  aria-label={t`New folder`}
                  icon={<FolderPlus size={14} aria-hidden="true" />}
                  disabled={readOnly}
                  onClick={() => setNewFolderOpen(true)}
                />
              </Tooltip>
            </>
          ) : null}
        </div>
      </div>
    )

  const emptyText = searching ? t`No files match your search` : t`Folder is empty`

  const listBody = storage.error ? (
    <Result
      status="error"
      title={t`Couldn't load the files`}
      subTitle={storage.error instanceof Error ? storage.error.message : String(storage.error)}
      extra={
        <Button type="primary" onClick={() => void storage.refetch()}>
          {t`Retry`}
        </Button>
      }
    />
  ) : view === 'list' ? (
    <FileTable
      items={visibleItems}
      loading={storage.isLoading}
      selectedKey={selectedItem?.key}
      checkedKeys={isPicker ? undefined : checkedKeys}
      onCheckedChange={isPicker ? undefined : setCheckedKeys}
      isRefused={isRefused}
      counts={counts}
      showFolder={searching}
      emptyText={emptyText}
      onOpen={openItem}
      onActivate={isPicker ? insert : undefined}
      menuFor={menuFor}
    />
  ) : storage.isLoading ? (
    <div className="flex justify-center py-12 text-ink-3">
      <Loader2 size={20} aria-label={t`Loading`} className="animate-spin" />
    </div>
  ) : visibleItems.length === 0 ? (
    <div className="py-12 text-center text-sm text-ink-3">{emptyText}</div>
  ) : (
    <FileGrid
      items={visibleItems}
      selectedKey={selectedItem?.key}
      isRefused={isRefused}
      onOpen={openItem}
      onActivate={isPicker ? insert : undefined}
      menuFor={menuFor}
    />
  )

  const listPanel = (
    <div
      className="relative"
      onDragEnter={onDragEnter}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
    >
      <Panel flush>
        {toolbar}
        {listBody}
      </Panel>
      {isDragging ? (
        <div className="pointer-events-none absolute inset-0 flex items-center justify-center rounded-lg border-2 border-dashed border-primary bg-primary-50/90">
          <span className="text-subtitle font-medium text-primary-700">{t`Drop files here to upload`}</span>
        </div>
      ) : null}
      {isUploading ? (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-4 rounded-lg bg-surface/95">
          <div className="text-subtitle font-semibold text-ink">{t`Importing files...`}</div>
          <ul data-unreset className="m-0 flex max-h-[300px] w-4/5 max-w-[400px] list-none flex-col overflow-y-auto p-0">
            {uploadStatuses.map((row, index) => (
              <li key={index} className="flex items-center gap-2 border-b border-line-soft py-2 text-sm">
                {row.status === 'pending' ? <Clock size={14} aria-hidden="true" className="text-ink-4" /> : null}
                {row.status === 'uploading' ? (
                  <Loader2 size={14} aria-hidden="true" className="animate-spin text-primary-500" />
                ) : null}
                {row.status === 'done' ? <CheckCircle2 size={14} aria-hidden="true" className="text-success-fg" /> : null}
                {row.status === 'failed' ? <XCircle size={14} aria-hidden="true" className="text-danger-fg" /> : null}
                <span className="min-w-0 flex-1 truncate text-ink">{row.file.name}</span>
                <span className="text-xs text-ink-3">{filesize(row.file.size, { round: 0 })}</span>
              </li>
            ))}
          </ul>
          <Button danger onClick={cancelUpload}>
            {t`Cancel`}
          </Button>
        </div>
      ) : null}
    </div>
  )

  const maxObjectsLabel = new Intl.NumberFormat(i18n.locale).format(MAX_OBJECTS)
  const alerts =
    storage.isCapped || storage.cutShort ? (
      <Alert
        type="warning"
        showIcon
        title={
          storage.isCapped
            ? t`Showing the first ${maxObjectsLabel} files of this bucket. Folder counts, total size and search only cover these files.`
            : t`The storage did not return the whole bucket. Folder counts, total size and search only cover the files shown.`
        }
      />
    ) : null

  const preview = selectedItem ? (
    <FilePreview
      item={selectedItem}
      showFolder={searching}
      onOpenFolder={goToPath}
      onCopyUrl={() => void copyUrl(selectedItem)}
      onDownload={() => download(selectedItem)}
      onDelete={isPicker ? undefined : () => confirmDeleteFile(selectedItem)}
      readOnly={readOnly}
    />
  ) : null

  const modals = (
    <>
      {uploadInput}
      <Modal
        title={t`Create new folder`}
        open={newFolderOpen}
        zIndex={overlayZ}
        onCancel={() => setNewFolderOpen(false)}
        footer={[
          <Button key="cancel" onClick={() => setNewFolderOpen(false)}>
            {t`Cancel`}
          </Button>,
          <Button key="create" type="primary" onClick={submitNewFolder} loading={newFolderLoading}>
            {t`Create`}
          </Button>
        ]}
        destroyOnHidden
      >
        <Form form={form} onFinish={submitNewFolder}>
          <Form.Item
            label={t`Folder name`}
            name="name"
            required
            rules={[
              {
                required: true,
                type: 'string',
                validator(_rule, value, callback) {
                  // Lowercase letters, digits, underscore and dash.
                  if (!/^[a-z0-9_-]+$/.test(value ?? '')) {
                    callback(t`Only lowercase alphanumeric characters, underscore, and dash are allowed.`)
                    return
                  }
                  callback()
                }
              }
            ]}
          >
            <Input
              addonBefore={currentPath !== '' ? currentPath : undefined}
              onChange={(event) => form.setFieldsValue({ name: event.target.value.trim() })}
            />
          </Form.Item>
        </Form>
      </Modal>
      <DeleteFolderModal
        folder={folderToDelete}
        workspaceId={props.workspaceId}
        bucket={props.settings?.bucket ?? ''}
        countFiles={storage.countFolderFiles}
        loading={isDeletingFolder}
        zIndex={overlayZ}
        onConfirm={runFolderDelete}
        onCancel={() => {
          if (!deletingFolderRef.current) setFolderToDelete(null)
        }}
      />
    </>
  )

  if (!props.settings?.endpoint) {
    const notConfigured = (
      <Panel>
        <div className="flex flex-col items-center gap-3 py-10 text-center">
          <HardDrive size={32} aria-hidden="true" className="text-ink-3" />
          <div className="text-base font-medium text-ink">{t`File storage is not configured.`}</div>
          <div className="text-sm text-ink-3">
            {t`Connect an S3-compatible bucket to upload images for your emails and blog.`}
          </div>
          {readOnly ? (
            <Tooltip title={t`Storage settings (Read-only mode)`}>
              <Button type="primary" disabled>
                {t`Configure now`}
              </Button>
            </Tooltip>
          ) : (
            <ButtonFilesSettings settings={props.settings} onUpdateSettings={updateSettings}>
              <Button type="primary">{t`Configure now`}</Button>
            </ButtonFilesSettings>
          )}
        </div>
      </Panel>
    )
    if (isPicker) return <div className="p-4">{notConfigured}</div>
    return (
      <div className="flex flex-col gap-3 px-5 pt-4 pb-6">
        <PageHeader title={t`File Manager`} />
        {notConfigured}
      </div>
    )
  }

  if (isPicker) {
    return (
      <div ref={rootRef} data-file-explorer="picker" className="flex h-[80vh] flex-col">
        <div className="flex min-h-0 flex-1">
          <div className="flex w-rail shrink-0 flex-col border-r border-line bg-surface">
            <div className="min-h-0 flex-1 overflow-y-auto px-2 py-3">{folderTreeSection}</div>
            <div className="border-t border-line-soft px-4 py-2.5 text-xs">{storageFooter}</div>
          </div>
          <div className="flex min-w-0 flex-1 flex-col gap-3 overflow-y-auto bg-canvas p-3">
            {alerts}
            {listPanel}
          </div>
          {preview ? (
            <div className="w-[340px] shrink-0 overflow-y-auto border-l border-line bg-canvas p-3">
              <Panel>{preview}</Panel>
            </div>
          ) : null}
        </div>
        <PickerFooter
          item={selectedItem}
          acceptFileType={props.acceptFileType}
          targetLabel={props.targetLabel}
          onCancel={props.onCancel}
          onInsert={() => selectedItem && insert(selectedItem)}
        />
        {modals}
      </div>
    )
  }

  const pageTitle = searching ? t`Search results` : currentPath ? folderName(currentPath) : t`Root`
  const pageCount = storage.isSuccess ? (searching ? visibleFiles.length : (counts.get(currentPath) ?? 0)) : undefined

  const headerActions = (
    <>
      <Tooltip title={readOnly ? t`New folder (Read-only mode)` : undefined}>
        <Button
          icon={<FolderPlus size={14} aria-hidden="true" />}
          disabled={readOnly}
          onClick={() => setNewFolderOpen(true)}
        >
          {t`New folder`}
        </Button>
      </Tooltip>
      <Tooltip title={readOnly ? t`Upload file (Read-only mode)` : undefined}>
        <Button
          type="primary"
          icon={<Upload size={14} aria-hidden="true" />}
          disabled={readOnly}
          loading={isUploading}
          onClick={browseFiles}
        >
          {t`Upload`}
        </Button>
      </Tooltip>
      {currentPath !== '' ? (
        <Dropdown
          trigger={['click']}
          placement="bottomRight"
          menu={{
            items: [
              writeEntry('delete-folder', t`Delete folder`, t`Delete folder (Read-only mode)`, () =>
                setFolderToDelete(currentPath), true)
            ]
          }}
        >
          <Button aria-label={t`More actions`} icon={<MoreHorizontal size={16} aria-hidden="true" />} />
        </Dropdown>
      ) : null}
    </>
  )

  return (
    <div ref={rootRef} data-file-explorer="page" className="contents">
      <RailLayout ariaLabel={t`Folders`} rail={folderTreeSection} footer={storageFooter}>
        <PageHeader title={pageTitle} count={pageCount} actions={headerActions} />
        {alerts}
        <div className="flex items-start gap-3">
          <div className="min-w-0 flex-1">{listPanel}</div>
          {preview && previewAsColumn ? (
            <aside aria-label={t`Preview`} className="w-[340px] shrink-0">
              <Panel>{preview}</Panel>
            </aside>
          ) : null}
        </div>
        {!previewAsColumn ? (
          <SidePanel
            open={selectedItem !== undefined}
            onClose={() => setSelectedKey(undefined)}
            title={selectedItem?.name ?? ''}
            copyLink
            nav={{
              position: navIndex >= 0 ? navIndex + 1 : undefined,
              total: navIndex >= 0 ? visibleFiles.length : undefined,
              onPrev,
              onNext
            }}
          >
            {preview}
          </SidePanel>
        ) : null}
        {modals}
      </RailLayout>
    </div>
  )
}

interface PickerFooterProps {
  item?: StorageObject
  acceptFileType: string
  targetLabel?: string
  onCancel?: () => void
  onInsert: () => void
}

/** The picker's footer: what the chosen file is and where it goes, Cancel and Insert. */
function PickerFooter({ item, acceptFileType, targetLabel, onCancel, onInsert }: PickerFooterProps) {
  const { t } = useLingui()
  const images = acceptFileType.startsWith('image/')
  const isImage = item?.file_info?.content_type?.startsWith('image/') ?? false
  const size = useImageSize(item && isImage ? item.file_info.url : undefined)

  let summary: ReactNode = t`Select a file`
  if (item) {
    const parts = [item.name]
    if (size) parts.push(`${size.width} × ${size.height}`)
    parts.push(item.file_info.size_human)
    if (targetLabel) parts.push(t`will be inserted into ${targetLabel}`)
    summary = parts.join(' · ')
  }

  return (
    <div
      data-picker-footer
      className="flex items-center justify-between gap-4 border-t border-line-soft px-4 py-2.5"
    >
      <span className="min-w-0 truncate text-sm text-ink-3">{summary}</span>
      <div className="flex shrink-0 items-center gap-2">
        <Button onClick={onCancel}>{t`Cancel`}</Button>
        <Button type="primary" disabled={!item} onClick={onInsert}>
          {images ? t`Insert image` : t`Select file`}
        </Button>
      </div>
    </div>
  )
}
