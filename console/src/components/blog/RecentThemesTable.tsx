import { useState } from 'react'
import { Table, Button, Space, Tooltip, App, Empty, Badge } from 'antd'
import {
  EditOutlined,
  EyeOutlined,
  CloudUploadOutlined,
  ExclamationCircleOutlined,
  PlusOutlined
} from '@ant-design/icons'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { blogThemesApi, BlogTheme } from '../../services/api/blog'
import { ThemeEditorDrawer } from './ThemeEditorDrawer'
import { ThemeSelectionModal } from './ThemeSelectionModal'
import { ThemePreset } from './themePresets'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import timezone from 'dayjs/plugin/timezone'
import utc from 'dayjs/plugin/utc'

dayjs.extend(relativeTime)
dayjs.extend(utc)
dayjs.extend(timezone)

interface RecentThemesTableProps {
  workspaceId: string
  workspace: any
}

export function RecentThemesTable({ workspaceId, workspace }: RecentThemesTableProps) {
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()
  const [limit, setLimit] = useState(3)
  const [editorOpen, setEditorOpen] = useState(false)
  const [selectionModalOpen, setSelectionModalOpen] = useState(false)
  const [selectedTheme, setSelectedTheme] = useState<BlogTheme | null>(null)
  const [selectedPreset, setSelectedPreset] = useState<ThemePreset | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['blog-themes', workspaceId, limit],
    queryFn: () => blogThemesApi.list(workspaceId, { limit, offset: 0 })
  })

  const themes = data?.themes || []
  const totalCount = data?.total_count || 0
  const hasMore = totalCount > limit

  const publishMutation = useMutation({
    mutationFn: (version: number) => blogThemesApi.publish(workspaceId, { version }),
    onSuccess: () => {
      message.success('Theme published successfully')
      queryClient.invalidateQueries({ queryKey: ['blog-themes', workspaceId] })
    },
    onError: (error: any) => {
      message.error(error?.message || 'Failed to publish theme')
    }
  })

  const handleEdit = (theme: BlogTheme) => {
    // Always open editor directly, regardless of published state
    setSelectedTheme(theme)
    setEditorOpen(true)
  }

  const handleCreate = () => {
    setSelectedTheme(null)
    setSelectedPreset(null)
    setSelectionModalOpen(true)
  }

  const handleSelectTheme = (preset: ThemePreset) => {
    setSelectedPreset(preset)
    setSelectedTheme(null)
    setEditorOpen(true)
  }

  const handlePreview = (theme: BlogTheme) => {
    if (!workspace?.settings?.custom_endpoint_url) {
      message.warning('Blog custom endpoint URL is not configured')
      return
    }

    const baseUrl = workspace.settings.custom_endpoint_url
    const previewUrl = `${baseUrl}/?preview_theme_version=${theme.version}`
    window.open(previewUrl, '_blank')
  }

  const handlePublish = (theme: BlogTheme) => {
    modal.confirm({
      title: 'Publish Theme',
      icon: <ExclamationCircleOutlined />,
      content: (
        <div>
          <p>
            Are you sure you want to publish theme v{theme.version}? This will make it live on your
            blog.
          </p>
          <p style={{ marginTop: 8, color: '#8c8c8c' }}>
            The currently published theme will be unpublished automatically.
          </p>
        </div>
      ),
      okText: 'Publish',
      okType: 'primary',
      cancelText: 'Cancel',
      onOk: () => publishMutation.mutate(theme.version)
    })
  }

  const handleLoadMore = () => {
    setLimit((prev) => prev + 5)
  }

  const columns = [
    {
      title: 'Version',
      dataIndex: 'version',
      key: 'version',
      width: 23,
      render: (version: number) => {
        return <span>{version}</span>
      }
    },
    {
      title: 'Status',
      key: 'status',
      width: 100,
      render: (record: BlogTheme) => {
        const isLive = record.published_at !== null && record.published_at !== undefined
        return isLive ? <Badge status="success" text="Live" /> : null
      }
    },
    {
      title: 'Notes',
      dataIndex: 'notes',
      key: 'notes',
      ellipsis: true,
      render: (notes: string) => {
        if (!notes) return <span style={{ color: '#8c8c8c' }}>No notes</span>
        return (
          <Tooltip title={notes}>
            <span>{notes}</span>
          </Tooltip>
        )
      }
    },
    {
      title: 'Last Edited',
      dataIndex: 'updated_at',
      key: 'updated_at',
      width: 160,
      render: (date: string) => {
        const tz = workspace?.settings?.timezone || 'UTC'
        return (
          <Tooltip title={`${dayjs(date).tz(tz).format('MMM DD, YYYY HH:mm')} ${tz}`}>
            {dayjs(date).fromNow()}
          </Tooltip>
        )
      }
    },
    {
      title: 'Actions',
      key: 'actions',
      width: 200,
      align: 'right' as const,
      render: (record: BlogTheme) => {
        const isPublished = record.published_at !== null && record.published_at !== undefined
        return (
          <Space size="small">
            <Tooltip title="Edit theme">
              <Button
                type="text"
                size="small"
                icon={<EditOutlined />}
                onClick={() => handleEdit(record)}
              />
            </Tooltip>
            <Tooltip title="Preview this theme in a new tab">
              <Button
                type="text"
                size="small"
                icon={<EyeOutlined />}
                onClick={() => handlePreview(record)}
              />
            </Tooltip>
            {!isPublished && (
              <Tooltip title="Publish this theme">
                <Button
                  type="text"
                  size="small"
                  icon={<CloudUploadOutlined />}
                  onClick={() => handlePublish(record)}
                  loading={publishMutation.isPending}
                />
              </Tooltip>
            )}
          </Space>
        )
      }
    }
  ]

  if (themes.length === 0 && !isLoading) {
    return (
      <div style={{ marginTop: 24 }}>
        <Empty description="No themes yet" image={Empty.PRESENTED_IMAGE_SIMPLE}>
          <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
            Create First Theme
          </Button>
        </Empty>

        <ThemeSelectionModal
          open={selectionModalOpen}
          onClose={() => setSelectionModalOpen(false)}
          onSelectTheme={handleSelectTheme}
          workspace={workspace}
        />

        <ThemeEditorDrawer
          open={editorOpen}
          onClose={() => {
            setEditorOpen(false)
            setSelectedPreset(null)
          }}
          theme={selectedTheme}
          presetData={selectedPreset}
          workspaceId={workspaceId}
          workspace={workspace}
        />
      </div>
    )
  }

  return (
    <div>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: 16
        }}
      >
        <h3 style={{ margin: 0 }}>Theme Versions</h3>
        <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
          New Theme
        </Button>
      </div>

      <Table
        columns={columns}
        showHeader={false}
        dataSource={themes}
        rowKey="version"
        loading={isLoading}
        pagination={false}
      />

      {hasMore && (
        <div style={{ textAlign: 'center', marginTop: 16 }}>
          <Button onClick={handleLoadMore}>Show More ({totalCount - limit} remaining)</Button>
        </div>
      )}

      <ThemeSelectionModal
        open={selectionModalOpen}
        onClose={() => setSelectionModalOpen(false)}
        onSelectTheme={handleSelectTheme}
      />

      <ThemeEditorDrawer
        open={editorOpen}
        onClose={() => {
          setEditorOpen(false)
          setSelectedPreset(null)
        }}
        theme={selectedTheme}
        presetData={selectedPreset}
        workspaceId={workspaceId}
        workspace={workspace}
      />
    </div>
  )
}
