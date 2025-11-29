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
  Tooltip
} from 'antd'
import type { FileManagerProps, StorageObject } from './interfaces'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ChangeEvent } from 'react'
import { Copy, Folder, Trash2, ExternalLink, Settings, RefreshCw, Plus } from 'lucide-react'
import { filesize } from 'filesize'
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

export default dayjs

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
  const { message } = App.useApp()
  const [currentPath, setCurrentPath] = useState(props.currentPath || '')
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([])
  const [items, setItems] = useState<StorageObject[] | undefined>(undefined)
  const [isLoading, setIsLoading] = useState(false)
  const [newFolderModalVisible, setNewFolderModalVisible] = useState(false)
  const [newFolderLoading, setNewFolderLoading] = useState(false)
  const s3ClientRef = useRef<S3Client | undefined>(undefined)
  const inputFileRef = useRef<HTMLInputElement>(null)
  const [isUploading, setIsUploading] = useState(false)
  const [form] = Form.useForm()

  // Check if file manager is in read-only mode
  const isReadOnly = props.readOnly || false

  const goToPath = (path: string) => {
    // reset selection on path change
    setSelectedRowKeys([])
    props.onSelect([])
    setCurrentPath(path)
  }

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
          item.file_info = {
            size: x.Size as number,
            size_human: filesize(x.Size || 0, { round: 0 }),
            content_type: GetContentType(key),
            url: baseUrl ? `${baseUrl}/${key}` : key
          }
        }

        return item
      })

      // console.log('new items', newItems)
      setItems(newItems)
      setIsLoading(false)
    }).catch((error: unknown) => {
      console.error('Failed to fetch objects:', error)
      message.error('Failed to fetch objects: ' + error)
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

    // eslint-disable-next-line react-hooks/set-state-in-effect
    fetchObjects()
  }, [props.settings, fetchObjects])

  const deleteObject = (key: string, isFolder: boolean) => {
    if (!s3ClientRef.current) {
      message.error('S3 client is not initialized.')
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
          message.success('Folder deleted successfully.')
          // go to previous path
          setCurrentPath(key.split('/').slice(0, -2).join('/') + '/')
        } else {
          message.success('File deleted successfully.')
        }
        // refresh
        fetchObjects()
      })
      .catch((error: unknown) => {
        message.error('Failed to delete file: ' + error)
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
      message.error('S3 client is not initialized.')
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
            message.error('Folder already exists.')
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
              message.success('Folder created successfully.')
              setNewFolderLoading(false)
              fetchObjects()
            })
            .catch((error: unknown) => {
              message.error('Failed to create folder: ' + error)
              setNewFolderLoading(false)
              props.onError(error instanceof Error ? error : new Error(String(error)))
            })
        })
        .catch((error: unknown) => {
          message.error('Failed to create folder: ' + error)
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

  const onFileChange = (e: ChangeEvent<HTMLInputElement>) => {
    if (!e.target.files) return
    if (isUploading) return
    if (!s3ClientRef.current) return

    // console.log(e.target.files)

    for (let i = 0; i < e.target.files.length; i++) {
      setIsUploading(true)
      const file = e.target.files.item(i) as File

      // Convert file to ArrayBuffer for browser compatibility with AWS SDK v3
      file
        .arrayBuffer()
        .then((arrayBuffer) => {
          const uint8Array = new Uint8Array(arrayBuffer)

          s3ClientRef
            .current!.send(
              new PutObjectCommand({
                Bucket: props.settings?.bucket || '',
                Key: currentPath + file.name,
                Body: uint8Array,
                ContentType: file.type
              })
            )
            .then(() => {
              message.success('File ' + file.name + ' uploaded successfully.')
              setIsUploading(false)
              fetchObjects()
            })
            .catch((error: unknown) => {
              message.error('Failed to upload file: ' + error)
              setIsUploading(false)
              props.onError(error instanceof Error ? error : new Error(String(error)))
            })
        })
        .catch((error: unknown) => {
          message.error('Failed to read file: ' + error)
          setIsUploading(false)
          props.onError(error instanceof Error ? error : new Error(String(error)))
        })
    }
  }

  const onBrowseFiles = () => {
    if (inputFileRef.current) {
      inputFileRef.current.click()
    }
  }

  if (!props.settings?.endpoint) {
    return (
      <Alert
        style={styles.marginBottomSmall}
        message={
          <>
            File storage is not configured.
            <ButtonFilesSettings
              settings={props.settings}
              onUpdateSettings={props.onUpdateSettings}
            >
              <Button type="link">Configure now</Button>
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
                  <Tooltip title="Delete folder" placement="bottom">
                    <Popconfirm
                      placement="topRight"
                      title={
                        <>
                          Do you want to delete the <b>{currentPath}</b> folder with all its
                          content?
                        </>
                      }
                      onConfirm={() => deleteObject(currentPath, true)}
                      okText="Delete folder"
                      cancelText="Cancel"
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
                  <Tooltip title="Delete folder (Read-only mode)" placement="bottom">
                    <Button size="small" type="text" disabled icon={<Trash2 size={16} />} />
                  </Tooltip>
                )}
                <Tooltip title="Refresh the list">
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
                    <Tooltip title="Storage settings">
                      <Button type="text" size="small">
                        <Settings size={16} />
                      </Button>
                    </Tooltip>
                  </ButtonFilesSettings>
                )}
                {isReadOnly && (
                  <Tooltip title="Storage settings (Read-only mode)">
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
                      multiple={false}
                    />
                    <Button
                      type="primary"
                      // size="small"
                      style={styles.pullRight}
                      loading={isUploading}
                    >
                      <Plus size={16} />
                      Upload
                    </Button>
                  </span>
                )}
                {isReadOnly && (
                  <Tooltip title="Upload file (Read-only mode)">
                    <Button
                      type="primary"
                      // size="small"
                      style={styles.pullRight}
                      disabled
                    >
                      <Plus size={16} />
                      Upload
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
              <Tooltip title={isReadOnly ? 'New folder (Read-only mode)' : 'Create new folder'}>
                <Button type="primary" ghost onClick={toggleNewFolderModal} disabled={isReadOnly}>
                  New folder
                </Button>
              </Tooltip>
            </Space>
          </div>
          <Table
            dataSource={itemsAtPath}
            loading={isLoading}
            pagination={false}
            size="middle"
            rowKey="key"
            locale={{ emptyText: 'Folder is empty' }}
            scroll={{ y: props.height ? props.height - 100 : undefined }}
            rowClassName={(record: StorageObject) => {
              return record.is_folder ? 'folder-row' : ''
            }}
            onRow={(record: StorageObject) => {
              return {
                onClick: () => {
                  if (record.is_folder) {
                    setCurrentPath(record.key)
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
                      disabled: !props.acceptItem(record as StorageObject)
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
                      <div onClick={toggleSelectionForItem.bind(null, item)}>
                        <Folder size={16} style={styles.primary} />
                      </div>
                    )
                  }
                  return (
                    <div onClick={toggleSelectionForItem.bind(null, item)}>
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
                title: 'Name',
                key: 'name',
                render: (item: StorageObject) => {
                  return <div onClick={toggleSelectionForItem.bind(null, item)}>{item.name}</div>
                }
              },
              {
                title: 'Size',
                key: 'size',
                render: (item: StorageObject) => {
                  return (
                    <div onClick={toggleSelectionForItem.bind(null, item)}>
                      {item.is_folder ? '-' : item.file_info.size_human}
                    </div>
                  )
                }
              },
              {
                title: 'Last modified',
                key: 'lastModified',
                render: (item: StorageObject) => {
                  return (
                    <Tooltip title={dayjs(item.last_modified).format('llll')}>
                      <div onClick={toggleSelectionForItem.bind(null, item)}>
                        {dayjs(item.last_modified).format('ll')}
                      </div>
                    </Tooltip>
                  )
                }
              },
              {
                title: '',
                key: 'actions',
                align: 'right',
                render: (item: StorageObject) => {
                  if (item.is_folder) return
                  return (
                    <Space>
                      <Tooltip title="Copy URL">
                        <Button
                          type="text"
                          size="small"
                          onClick={() => {
                            navigator.clipboard.writeText(item.file_info.url)
                            message.success('URL copied to clipboard.')
                          }}
                        >
                          <Copy size={16} />
                        </Button>
                      </Tooltip>
                      <Tooltip title="Open in a window">
                        <a href={item.file_info.url} target="_blank" rel="noreferrer">
                          <Button type="text" size="small">
                            <ExternalLink size={16} />
                          </Button>
                        </a>
                      </Tooltip>
                      {!isReadOnly && (
                        <Popconfirm
                          title="Do you want to permanently delete this file from your storage?"
                          onConfirm={() => deleteObject(item.key, false)}
                          placement="topRight"
                          okText="Delete"
                          cancelText="Cancel"
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
                        <Tooltip title="Delete file (Read-only mode)">
                          <Button type="text" size="small" disabled>
                            <Trash2 size={16} />
                          </Button>
                        </Tooltip>
                      )}
                    </Space>
                  )
                }
              }
            ]}
          />
        </>
      )}
      {newFolderModalVisible && (
        <Modal
          title="Create new folder"
          open={newFolderModalVisible}
          onCancel={toggleNewFolderModal}
          footer={[
            <Button key="cancel" onClick={toggleNewFolderModal}>
              Cancel
            </Button>,
            <Button
              key="create"
              type="primary"
              onClick={onSubmitNewFolder}
              loading={newFolderLoading}
              disabled={isReadOnly}
            >
              Create
            </Button>
          ]}
        >
          <Form form={form}>
            <Form.Item
              label="Folder name"
              name="name"
              rules={[
                {
                  required: true,
                  type: 'string',
                  validator(_rule, value, callback) {
                    // alphanumeric, lowercase, underscore, dash
                    if (!/^[a-z0-9_-]+$/.test(value)) {
                      callback(
                        'Only lowercase alphanumeric characters, underscore, and dash are allowed.'
                      )
                      return
                    }
                    callback()
                  }
                }
              ]}
            >
              <Space.Compact style={{ width: '100%' }}>
                <Input
                  style={{ width: 'auto', pointerEvents: 'none' }}
                  value={currentPath !== '/' ? currentPath : '/'}
                  readOnly
                />
                <Input
                  style={{ flex: 1 }}
                  onChange={(e) => {
                    // trim spaces
                    form.setFieldsValue({ folderName: e.target.value.trim() })
                  }}
                />
              </Space.Compact>
            </Form.Item>
          </Form>
        </Modal>
      )}
    </div>
  )
}
