import React from 'react'
import {
  Alert,
  App,
  Button,
  Form,
  Input,
  Modal,
  Popconfirm,
  Popover,
  Space,
  Table,
  Tooltip,
  Typography
} from 'antd'
import {
  ClockCircleOutlined,
  LoadingOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined
} from '@ant-design/icons'
import type { FileManagerProps, StorageObject } from './interfaces'
import { useLingui } from '@lingui/react/macro'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ChangeEvent } from 'react'
import { Copy, Folder, Trash2, ExternalLink, Settings, RefreshCw, Plus, Download } from 'lucide-react'
import { filesize } from 'filesize'
import { zipSync } from 'fflate'
import ButtonFilesSettings from './buttonSettings'
import {
  S3Client,
  ListObjectsV2Command,
  type ListObjectsV2CommandInput,
  PutObjectCommand,
  type PutObjectCommandInput,
  DeleteObjectCommand,
  type DeleteObjectCommandInput
} from '@aws-sdk/client-s3'
import GetContentType from './fileExtensions'
import dayjs from 'dayjs'
import timezone from 'dayjs/plugin/timezone'
import utc from 'dayjs/plugin/utc'
import relativeTime from 'dayjs/plugin/relativeTime'
import localizedFormat from 'dayjs/plugin/localizedFormat'
import customParseFormat from 'dayjs/plugin/customParseFormat'
import isSameOrBefore from 'dayjs/plugin/isSameOrBefore'
import isSameOrAfter from 'dayjs/plugin/isSameOrAfter'
import isToday from 'dayjs/plugin/isToday'

// Extend dayjs with plugins
dayjs.extend(utc)
dayjs.extend(timezone)
dayjs.extend(relativeTime)
dayjs.extend(localizedFormat)
dayjs.extend(customParseFormat)
dayjs.extend(isSameOrBefore)
dayjs.extend(isSameOrAfter)
dayjs.extend(isToday)

// eslint-disable-next-line react-refresh/only-export-components -- Utility export co-located with component
export default dayjs

// Upload file status interface for tracking upload progress
interface UploadFileStatus {
  file: File
  status: 'pending' | 'uploading' | 'done' | 'failed'
  error?: string
}

// Common styles
const styles = {
  folderRow: {
    fontWeight: 'bold' as const,
    cursor: 'pointer'
  },
  filesContainer: {
    position: 'relative' as const,
    overflow: 'auto' as const
  },
  marginBottomSmall: { marginBottom: 16 },
  marginBottomLarge: { marginBottom: 24 },
  padding: { paddingBottom: 16 },
  pullRight: { float: 'right' as const },
  paddingRightSmall: { paddingRight: 8 },
  textRight: { textAlign: 'right' as const },
  primary: { color: '#1890ff' } // Default antd primary color - replace with actual color if different
}

export const FileManager = (props: FileManagerProps) => {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [internalPath, setInternalPath] = useState(props.currentPath || '')
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([])
  const [items, setItems] = useState<StorageObject[] | undefined>(undefined)
  const [isLoading, setIsLoading] = useState(false)
  const [newFolderModalVisible, setNewFolderModalVisible] = useState(false)
  const [newFolderLoading, setNewFolderLoading] = useState(false)
  const s3ClientRef = useRef<S3Client | undefined>(undefined)
  const inputFileRef = useRef<HTMLInputElement>(null)
  const [isUploading, setIsUploading] = useState(false)
  const [isZipping, setIsZipping] = useState(false)
  const [isDeleting, setIsDeleting] = useState(false)
  const [uploadFileStatuses, setUploadFileStatuses] = useState<UploadFileStatus[]>([])
  const [isDragging, setIsDragging] = useState(false)
  const dragCounterRef = useRef(0)
  const abortControllerRef = useRef<AbortController | null>(null)
  const [form] = Form.useForm()

  // Check if file manager is in read-only mode
  const isReadOnly = props.readOnly || false

  // Check if file manager is operating in controlled mode
  const isControlledMode = props.controlledPath !== undefined && props.onPathChange !== undefined

  // Effective current path: use controlled prop if in controlled mode, else internal state
  const currentPath = isControlledMode ? props.controlledPath! : internalPath

  const selectedFileCount = useMemo(() => {
    if (!items) return 0
    return items.filter(
      (item) => selectedRowKeys.includes(item.key) && !item.is_folder
    ).length
  }, [items, selectedRowKeys])

  const goToPath = useCallback((path: string) => {
    // reset selection on path change
    setSelectedRowKeys([])
    props.onSelect([])

    if (isControlledMode) {
      props.onPathChange!(path)
    } else {
      setInternalPath(path)
    }
  }, [isControlledMode, props])

  const fetchObjects = useCallback(() => {
    if (!s3ClientRef.current || !props.settings?.bucket) return

    setIsLoading(true)
    const input: ListObjectsV2CommandInput = {
      Bucket: props.settings.bucket
    }

    const command = new ListObjectsV2Command(input)
    s3ClientRef.current.send(command).then((response) => {
      // console.log('response', response)
      if (!response.Contents) {
        setItems([])
        setIsLoading(false)
        return
      }

      const newItems = response.Contents.map((x) => {
        const key = x.Key as string

        // Construct the base URL for accessing files
        let baseUrl = ''

        if (props.settings?.cdn_endpoint && props.settings.cdn_endpoint.trim() !== '') {
          // Use CDN endpoint if provided
          baseUrl = props.settings.cdn_endpoint.replace(/\/$/, '') // Remove trailing slash
        } else if (props.settings?.endpoint && props.settings?.bucket) {
          // Construct URL from S3 endpoint and bucket
          const cleanEndpoint = props.settings.endpoint.replace(/\/$/, '') // Remove trailing slash
          baseUrl = `${cleanEndpoint}/${props.settings.bucket}`
        }

        const isFolder = key.endsWith('/')
        let name =
          key
            .split('/')
            .filter((x) => x !== '')
            .pop() || ''

        if (!isFolder) {
          name = key.split('/').pop() || ''
        }

        // console.log('item', x)

        let itemPath = ''
        const pathParts = key.split('/')

        if (isFolder) {
          itemPath = pathParts.slice(0, pathParts.length - 2).join('/') + '/'
          // console.log('folder path', itemCurrentPath)
        } else {
          itemPath = pathParts.slice(0, pathParts.length - 1).join('/') + '/'
          // console.log('file path', itemCurrentPath)
        }

        if (itemPath === '/') itemPath = ''

        const item = {
          key: key,
          name: name,
          path: itemPath,
          is_folder: isFolder,
          last_modified: x.LastModified
        } as StorageObject

        if (!isFolder) {
          // Encode each path segment separately to preserve folder structure
          // This ensures spaces and special chars are encoded but slashes are kept
          const encodedKey = key
            .split('/')
            .map((segment) => encodeURIComponent(segment))
            .join('/')
          item.file_info = {
            size: x.Size as number,
            size_human: filesize(x.Size || 0, { round: 0 }),
            content_type: GetContentType(key),
            url: baseUrl ? `${baseUrl}/${encodedKey}` : encodedKey
          }
        }

        return item
      })

      // console.log('new items', newItems)
      setItems(newItems)
      setIsLoading(false)
    }).catch((error: unknown) => {
      console.error('Failed to fetch objects:', error)
      message.error(t`Failed to fetch objects: ` + error)
      setIsLoading(false)
    })
  }, [props.settings, message])

  // Initialize or reinitialize S3 client when settings change
  useEffect(() => {
    // Don't initialize if settings are not provided or endpoint is empty/undefined
    if (!props.settings || !props.settings.endpoint || props.settings.endpoint === '') {
      s3ClientRef.current = undefined
      return
    }

    // Always recreate the S3 client when settings change
    s3ClientRef.current = new S3Client({
      endpoint: props.settings.endpoint,
      credentials: {
        accessKeyId: props.settings.access_key || '',
        secretAccessKey: props.settings.secret_key || ''
      },
      region: props.settings.region || 'us-east-1',
      forcePathStyle: props.settings.force_path_style ?? false
    })

    fetchObjects()
  }, [props.settings, fetchObjects])

  const deleteObject = (key: string, isFolder: boolean) => {
    if (!s3ClientRef.current) {
      message.error(t`S3 client is not initialized.`)
      return
    }

    const s3Client = s3ClientRef.current

    const input: DeleteObjectCommandInput = {
      Bucket: props.settings?.bucket || '',
      Key: key
    }

    s3Client
      .send(new DeleteObjectCommand(input))
      .then(() => {
        if (isFolder) {
          fetchObjects()
          message.success(t`Folder deleted successfully.`)
          // go to previous path
          const parentPath = key.split('/').slice(0, -2).join('/')
          goToPath(parentPath ? parentPath + '/' : '')
        } else {
          message.success(t`File deleted successfully.`)
        }
        // refresh
        fetchObjects()
      })
      .catch((error: unknown) => {
        message.error(t`Failed to delete file: ` + error)
        props.onError(error instanceof Error ? error : new Error(String(error)))
      })
  }

  const selectItem = (items: StorageObject[]) => {
    console.log('selected items', items)
  }

  const toggleSelectionForItem = (item: StorageObject) => {
    // ignore items not accepted
    if (!props.acceptItem(item)) return

    if (props.multiple) {
      let newKeys = [...selectedRowKeys]
      // remove if exists
      if (newKeys.includes(item.key)) {
        newKeys = selectedRowKeys.filter((k) => k !== item.key)
      } else {
        newKeys.push(item.key)
      }
      setSelectedRowKeys(newKeys)
      props.onSelect(items ? items.filter((x) => newKeys.includes(x.key)) : [])
    } else {
      setSelectedRowKeys([item.key])
      props.onSelect([item])
    }
  }

  const toggleNewFolderModal = () => {
    setNewFolderModalVisible(!newFolderModalVisible)
  }

  const onSubmitNewFolder = () => {
    if (!s3ClientRef.current) {
      message.error(t`S3 client is not initialized.`)
      return
    }

    if (newFolderLoading) return

    const s3Client = s3ClientRef.current

    form.validateFields().then((values) => {
      setNewFolderLoading(true)

      // create folder in S3
      const folderName = values.name
      const key = currentPath === '' ? folderName + '/' : currentPath + folderName + '/'

      const input: ListObjectsV2CommandInput = {
        Bucket: props.settings?.bucket || '',
        Prefix: key
      }

      s3Client
        .send(new ListObjectsV2Command(input))
        .then((response) => {
          // console.log('response', response)
          if (response.Contents && response.Contents.length > 0) {
            message.error(t`Folder already exists.`)
            return
          }

          const input: PutObjectCommandInput = {
            Bucket: props.settings?.bucket || '',
            Key: key,
            Body: ''
          }

          s3Client
            .send(new PutObjectCommand(input))
            .then(() => {
              message.success(t`Folder created successfully.`)
              setNewFolderLoading(false)
              fetchObjects()
            })
            .catch((error: unknown) => {
              message.error(t`Failed to create folder: ` + error)
              setNewFolderLoading(false)
              props.onError(error instanceof Error ? error : new Error(String(error)))
            })
        })
        .catch((error: unknown) => {
          message.error(t`Failed to create folder: ` + error)
          setNewFolderLoading(false)
          props.onError(error instanceof Error ? error : new Error(String(error)))
        })

      form.resetFields()
      toggleNewFolderModal()
    })
  }

  const itemsAtPath = useMemo(() => {
    if (!items) return []
    return items
      .filter((x) => x.path === currentPath)
      .sort((a, b) => {
        // by folders first, then by last_modified
        if (a.is_folder && !b.is_folder) return -1
        if (!a.is_folder && b.is_folder) return 1
        if (a.last_modified > b.last_modified) return -1
        if (a.last_modified < b.last_modified) return 1
        return 0
      })
  }, [items, currentPath])

  const startUpload = async (files: File[]) => {
    if (files.length === 0 || !s3ClientRef.current) return

    // Initialize status list with all files as pending
    const initialStatuses: UploadFileStatus[] = files.map((file) => ({
      file,
      status: 'pending'
    }))
    setUploadFileStatuses(initialStatuses)
    setIsUploading(true)
    abortControllerRef.current = new AbortController()

    for (let i = 0; i < files.length; i++) {
      if (abortControllerRef.current?.signal.aborted) {
        break
      }

      const file = files[i]

      // Update current file to 'uploading'
      setUploadFileStatuses((prev) =>
        prev.map((item, idx) => (idx === i ? { ...item, status: 'uploading' } : item))
      )

      try {
        const arrayBuffer = await file.arrayBuffer()
        const uint8Array = new Uint8Array(arrayBuffer)

        await s3ClientRef.current.send(
          new PutObjectCommand({
            Bucket: props.settings?.bucket || '',
            Key: currentPath + file.name,
            Body: uint8Array,
            ContentType: file.type
          }),
          { abortSignal: abortControllerRef.current?.signal }
        )

        // Mark as done
        setUploadFileStatuses((prev) =>
          prev.map((item, idx) => (idx === i ? { ...item, status: 'done' } : item))
        )
      } catch (error) {
        if ((error as Error).name === 'AbortError') {
          break
        }
        // Mark as failed
        setUploadFileStatuses((prev) =>
          prev.map((item, idx) => (idx === i ? { ...item, status: 'failed', error: String(error) } : item))
        )
      }
    }

    setIsUploading(false)
    setUploadFileStatuses([])
    abortControllerRef.current = null
    fetchObjects()
  }

  const onFileChange = (e: ChangeEvent<HTMLInputElement>) => {
    if (!e.target.files || e.target.files.length === 0) return
    if (isUploading || !s3ClientRef.current) return

    const files = Array.from(e.target.files)
    startUpload(files)

    // Reset input so same file can be selected again
    e.target.value = ''
  }

  const handleCancelUpload = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
    }
    message.info(t`Import cancelled`)
  }

  const handleDragEnter = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (!isReadOnly) {
      dragCounterRef.current++
      if (dragCounterRef.current === 1) {
        setIsDragging(true)
      }
    }
  }

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
  }

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    dragCounterRef.current--
    if (dragCounterRef.current === 0) {
      setIsDragging(false)
    }
  }

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    dragCounterRef.current = 0
    setIsDragging(false)

    if (isReadOnly) {
      message.warning(t`Cannot upload in read-only mode`)
      return
    }

    const files = Array.from(e.dataTransfer.files)
    if (files.length > 0) {
      startUpload(files)
    }
  }

  const onBrowseFiles = () => {
    if (inputFileRef.current) {
      inputFileRef.current.click()
    }
  }

  const downloadSelectedAsZip = async () => {
    const selectedFiles = items?.filter(
      (item) => selectedRowKeys.includes(item.key) && !item.is_folder
    ) || []

    if (selectedFiles.length === 0) {
      message.warning(t`No files selected`)
      return
    }

    setIsZipping(true)
    try {
      const zipData: Record<string, Uint8Array> = {}

      for (const file of selectedFiles) {
        const response = await fetch(file.file_info.url)
        if (!response.ok) throw new Error(`Failed to fetch ${file.name}`)
        const arrayBuffer = await response.arrayBuffer()
        zipData[file.name] = new Uint8Array(arrayBuffer)
      }

      const zipped = zipSync(zipData)
      const blob = new Blob([zipped as BlobPart], { type: 'application/zip' })
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `files-${Date.now()}.zip`
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      URL.revokeObjectURL(url)

      message.success(t`Downloaded ${selectedFiles.length} file(s)`)
    } catch (error) {
      console.error('ZIP download failed:', error)
      message.error(t`Failed to download files`)
    } finally {
      setIsZipping(false)
    }
  }

  const deleteSelectedItems = async () => {
    if (!s3ClientRef.current || !props.settings?.bucket) {
      message.error(t`S3 client is not initialized.`)
      return
    }

    const selectedItems = items?.filter(
      (item) => selectedRowKeys.includes(item.key)
    ) || []

    if (selectedItems.length === 0) {
      message.warning(t`No items selected`)
      return
    }

    setIsDeleting(true)
    let deletedCount = 0
    let failedCount = 0

    for (const item of selectedItems) {
      try {
        await s3ClientRef.current.send(
          new DeleteObjectCommand({
            Bucket: props.settings.bucket,
            Key: item.key
          })
        )
        deletedCount++
      } catch (error) {
        console.error(`Failed to delete ${item.name}:`, error)
        failedCount++
      }
    }

    setIsDeleting(false)
    setSelectedRowKeys([])
    props.onSelect([])
    fetchObjects()

    if (failedCount === 0) {
      message.success(t`Deleted ${deletedCount} item(s)`)
    } else {
      message.warning(t`Deleted ${deletedCount}, failed ${failedCount}`)
    }
  }

  if (!props.settings?.endpoint) {
    return (
      <Alert
        style={styles.marginBottomSmall}
        message={
          <>
            {t`File storage is not configured.`}
            <ButtonFilesSettings
              settings={props.settings}
              onUpdateSettings={props.onUpdateSettings}
            >
              <Button type="link">{t`Configure now`}</Button>
            </ButtonFilesSettings>
          </>
        }
        type="warning"
        showIcon
      />
    )
  }

  return (
    <div style={{ ...styles.filesContainer, height: props.height }}>
      {props.settings?.endpoint !== '' && (
        <>
          <div style={{ ...styles.padding, borderBottom: '1px solid rgba(0,0,0,0.1)' }}>
            <div style={styles.pullRight}>
              <Space>
                {currentPath !== '' && !isReadOnly && (
                  <Tooltip title={t`Delete folder`} placement="bottom">
                    <Popconfirm
                      placement="topRight"
                      title={
                        <>
                          {t`Do you want to delete the`} <b>{currentPath}</b> {t`folder with all its content?`}
                        </>
                      }
                      onConfirm={() => deleteObject(currentPath, true)}
                      okText={t`Delete folder`}
                      cancelText={t`Cancel`}
                      okButtonProps={{
                        danger: true
                      }}
                    >
                      <Button
                        size="small"
                        type="text"
                        onClick={() => fetchObjects()}
                        icon={<Trash2 size={16} />}
                      />
                    </Popconfirm>
                  </Tooltip>
                )}
                {currentPath !== '' && isReadOnly && (
                  <Tooltip title={t`Delete folder (Read-only mode)`} placement="bottom">
                    <Button size="small" type="text" disabled icon={<Trash2 size={16} />} />
                  </Tooltip>
                )}
                <Tooltip title={t`Refresh the list`}>
                  <Button
                    size="small"
                    type="text"
                    onClick={() => fetchObjects()}
                    icon={<RefreshCw size={16} />}
                  />
                </Tooltip>

                {!isReadOnly && (
                  <ButtonFilesSettings
                    settings={props.settings}
                    onUpdateSettings={props.onUpdateSettings}
                    settingsInfo={props.settingsInfo}
                  >
                    <Tooltip title={t`Storage settings`}>
                      <Button type="text" size="small">
                        <Settings size={16} />
                      </Button>
                    </Tooltip>
                  </ButtonFilesSettings>
                )}
                {isReadOnly && (
                  <Tooltip title={t`Storage settings (Read-only mode)`}>
                    <Button type="text" size="small" disabled>
                      <Settings size={16} />
                    </Button>
                  </Tooltip>
                )}
                {!isReadOnly && (
                  <span role="button" onClick={onBrowseFiles}>
                    <input
                      type="file"
                      ref={inputFileRef}
                      onChange={onFileChange}
                      hidden
                      accept={props.acceptFileType}
                      multiple
                    />
                    <Button
                      type="primary"
                      // size="small"
                      style={styles.pullRight}
                      loading={isUploading}
                    >
                      <Plus size={16} />
                      {t`Upload`}
                    </Button>
                  </span>
                )}
                {isReadOnly && (
                  <Tooltip title={t`Upload file (Read-only mode)`}>
                    <Button
                      type="primary"
                      // size="small"
                      style={styles.pullRight}
                      disabled
                    >
                      <Plus size={16} />
                      {t`Upload`}
                    </Button>
                  </Tooltip>
                )}
                </Space>
            </div>

            <Space>
              <div>
                <Button type="text" onClick={() => goToPath('')}>
                  {props.settings?.bucket || ''}
                </Button>
                {currentPath
                  .split('/')
                  .filter((x) => x !== '')
                  .map((part, index, array) => {
                    const isLast = index === array.length - 1
                    const fullPath = array.slice(0, index + 1).join('/') + '/'
                    return (
                      <React.Fragment key={fullPath}>
                        /
                        <Button
                          disabled={isLast}
                          type="text"
                          // size="small"
                          onClick={() => goToPath(fullPath)}
                        >
                          {part}
                        </Button>
                      </React.Fragment>
                    )
                  })}
              </div>
              {selectedRowKeys.length === 0 && (
                <Button type="primary" ghost onClick={toggleNewFolderModal} disabled={isReadOnly}>
                  {t`New folder`}
                </Button>
              )}
              {selectedRowKeys.length > 0 && !isReadOnly && (
                <Popconfirm
                  title={t`Delete ${selectedRowKeys.length} selected item(s)?`}
                  onConfirm={deleteSelectedItems}
                  okText={t`Delete`}
                  cancelText={t`Cancel`}
                  okButtonProps={{ danger: true }}
                >
                  <Button type="primary" ghost loading={isDeleting}>
                    <Trash2 size={16} /> {t`Delete`} ({selectedRowKeys.length})
                  </Button>
                </Popconfirm>
              )}
              {selectedFileCount > 1 && (
                <Button type="primary" ghost loading={isZipping} onClick={downloadSelectedAsZip}>
                  <Download size={16} /> {t`Zip`} ({selectedFileCount})
                </Button>
              )}
            </Space>
          </div>

          {/* Table container with drag and drop */}
          <div
            style={{ position: 'relative', flex: 1 }}
            onDragEnter={handleDragEnter}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
          >
            <Table
              dataSource={itemsAtPath}
            loading={isLoading}
            pagination={false}
            size="middle"
            rowKey="key"
            locale={{ emptyText: t`Folder is empty` }}
            scroll={{ y: props.height ? props.height - 100 : undefined }}
            rowClassName={(record: StorageObject) => {
              return record.is_folder ? 'folder-row' : ''
            }}
            onRow={(record: StorageObject) => {
              return {
                onClick: () => {
                  if (record.is_folder) {
                    goToPath(record.key)
                  }
                },
                style: record.is_folder ? styles.folderRow : undefined
              }
            }}
            rowSelection={
              props.withSelection
                ? {
                    type: props.multiple ? 'checkbox' : 'radio',
                    selectedRowKeys: selectedRowKeys,
                    onChange: (selectedRowKeys: React.Key[], selectedRows: StorageObject[]) => {
                      setSelectedRowKeys(selectedRowKeys)
                      selectItem(selectedRows)
                    },
                    getCheckboxProps: (record: StorageObject) => ({
                      disabled: record.is_folder || !props.acceptItem(record as StorageObject)
                    })
                  }
                : undefined
            }
            columns={[
              {
                title: '',
                key: 'preview',
                render: (item: StorageObject) => {
                  if (item.is_folder) {
                    return (
                      <div>
                        <Folder size={16} style={styles.primary} />
                      </div>
                    )
                  }
                  return (
                    <div>
                      {item.file_info.content_type.includes('image') && (
                        <Popover
                          placement="right"
                          content={
                            <img src={item.file_info.url} alt="" style={{ maxHeight: '400px' }} />
                          }
                        >
                          <img
                            src={item.file_info.url}
                            alt=""
                            height="30"
                            style={{ maxWidth: '100px', maxHeight: '100px' }}
                          />
                        </Popover>
                      )}
                    </div>
                  )
                }
              },
              {
                title: t`Name`,
                key: 'name',
                render: (item: StorageObject) => {
                  return <div>{item.name}</div>
                }
              },
              {
                title: t`Size`,
                key: 'size',
                render: (item: StorageObject) => {
                  return <div>{item.is_folder ? '-' : item.file_info.size_human}</div>
                }
              },
              {
                title: t`Last modified`,
                key: 'lastModified',
                render: (item: StorageObject) => {
                  return (
                    <Tooltip title={dayjs(item.last_modified).format('llll')}>
                      <div>{dayjs(item.last_modified).format('ll')}</div>
                    </Tooltip>
                  )
                }
              },
              {
                title: '',
                key: 'actions',
                align: 'right',
                width: 300,
                render: (item: StorageObject) => {
                  if (item.is_folder) return
                  return (
                    <Space>
                      <Tooltip title={t`Copy URL`}>
                        <Button
                          type="text"
                          size="small"
                          onClick={() => {
                            navigator.clipboard.writeText(item.file_info.url)
                            message.success(t`URL copied to clipboard.`)
                          }}
                        >
                          <Copy size={16} />
                        </Button>
                      </Tooltip>
                      <Tooltip title={t`Open in a window`}>
                        <a href={item.file_info.url} target="_blank" rel="noreferrer">
                          <Button type="text" size="small">
                            <ExternalLink size={16} />
                          </Button>
                        </a>
                      </Tooltip>
                      <Tooltip title={t`Download file`}>
                        <Button
                          type="text"
                          size="small"
                          onClick={async (e) => {
                            e.stopPropagation()
                            try {
                              const response = await fetch(item.file_info.url)
                              const blob = await response.blob()
                              const url = URL.createObjectURL(blob)
                              const link = document.createElement('a')
                              link.href = url
                              link.download = item.name
                              document.body.appendChild(link)
                              link.click()
                              document.body.removeChild(link)
                              URL.revokeObjectURL(url)
                            } catch (error) {
                              console.error('Download failed:', error)
                              message.error(t`Failed to download file`)
                            }
                          }}
                        >
                          <Download size={16} />
                        </Button>
                      </Tooltip>
                      {!isReadOnly && (
                        <Popconfirm
                          title={t`Do you want to permanently delete this file from your storage?`}
                          onConfirm={() => deleteObject(item.key, false)}
                          placement="topRight"
                          okText={t`Delete`}
                          cancelText={t`Cancel`}
                          okButtonProps={{
                            danger: true
                          }}
                        >
                          <Button type="text" size="small">
                            <Trash2 size={16} />
                          </Button>
                        </Popconfirm>
                      )}
                      {isReadOnly && (
                        <Tooltip title={t`Delete file (Read-only mode)`}>
                          <Button type="text" size="small" disabled>
                            <Trash2 size={16} />
                          </Button>
                        </Tooltip>
                      )}
                      {props.withSelection && props.acceptItem(item) && (
                        <Button
                          type="primary"
                          size="small"
                          style={{ marginRight: 16 }}
                          onClick={() => toggleSelectionForItem(item)}
                        >
                          {t`Select`}
                        </Button>
                      )}
                    </Space>
                  )
                }
              }
            ]}
            />

            {/* Drag visual feedback overlay */}
            {isDragging && (
              <div
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  right: 0,
                  bottom: 0,
                  backgroundColor: 'rgba(24, 144, 255, 0.1)',
                  border: '2px dashed #1890ff',
                  borderRadius: 8,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  zIndex: 999,
                  pointerEvents: 'none'
                }}
              >
                <Typography.Text style={{ fontSize: 18, color: '#1890ff' }}>
                  {t`Drop files here to upload`}
                </Typography.Text>
              </div>
            )}
          </div>
        </>
      )}
      {newFolderModalVisible && (
        <Modal
          title={t`Create new folder`}
          open={newFolderModalVisible}
          onCancel={toggleNewFolderModal}
          footer={[
            <Button key="cancel" onClick={toggleNewFolderModal}>
              {t`Cancel`}
            </Button>,
            <Button
              key="create"
              type="primary"
              onClick={onSubmitNewFolder}
              loading={newFolderLoading}
              disabled={isReadOnly}
            >
              {t`Create`}
            </Button>
          ]}
        >
          <Form form={form}>
            <Form.Item
              label={t`Folder name`}
              name="name"
              required
              rules={[
                {
                  required: true,
                  type: 'string',
                  validator(_rule, value, callback) {
                    // alphanumeric, lowercase, underscore, dash
                    if (!/^[a-z0-9_-]+$/.test(value)) {
                      callback(
                        t`Only lowercase alphanumeric characters, underscore, and dash are allowed.`
                      )
                      return
                    }
                    callback()
                  }
                }
              ]}
            >
              <Input
                addonBefore={currentPath !== '' ? currentPath : undefined}
                style={{ width: '100%' }}
                onChange={(e) => {
                  // trim spaces
                  form.setFieldsValue({ name: e.target.value.trim() })
                }}
              />
            </Form.Item>
          </Form>
        </Modal>
      )}

      {/* Import overlay with status list */}
      {isUploading && (
        <div
          style={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            backgroundColor: 'rgba(255, 255, 255, 0.95)',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 1000
          }}
        >
          <Typography.Title level={4} style={{ marginBottom: 16 }}>
            {t`Importing files...`}
          </Typography.Title>

          <div
            style={{
              maxHeight: 300,
              overflowY: 'auto',
              width: '80%',
              maxWidth: 400
            }}
          >
            {uploadFileStatuses.map((item, index) => (
              <div
                key={index}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  padding: '8px 0',
                  borderBottom: '1px solid #f0f0f0'
                }}
              >
                {item.status === 'pending' && <ClockCircleOutlined style={{ color: '#999' }} />}
                {item.status === 'uploading' && <LoadingOutlined style={{ color: '#1890ff' }} />}
                {item.status === 'done' && <CheckCircleOutlined style={{ color: '#52c41a' }} />}
                {item.status === 'failed' && <CloseCircleOutlined style={{ color: '#ff4d4f' }} />}
                <span
                  style={{
                    marginLeft: 8,
                    flex: 1,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap'
                  }}
                >
                  {item.file.name}
                </span>
                <span style={{ marginLeft: 8, color: '#999', fontSize: 12 }}>
                  {filesize(item.file.size, { round: 0 })}
                </span>
              </div>
            ))}
          </div>

          <Button onClick={handleCancelUpload} style={{ marginTop: 24 }} danger>
            {t`Cancel`}
          </Button>
        </div>
      )}
    </div>
  )
}
