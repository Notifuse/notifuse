import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
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
import type { Workspace } from '../../services/api/types'
import { ThemeEditorDrawer } from './ThemeEditorDrawer'
import { ThemePreset, THEME_PRESETS } from './themePresets'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import timezone from 'dayjs/plugin/timezone'
import utc from 'dayjs/plugin/utc'

dayjs.extend(relativeTime)
dayjs.extend(utc)
dayjs.extend(timezone)

interface RecentThemesTableProps {
  workspaceId: string
  workspace: Workspace
}

export function RecentThemesTable({ workspaceId, workspace }: RecentThemesTableProps) {
  const { t } = useLingui()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()
  const [limit, setLimit] = useState(3)
  const [editorOpen, setEditorOpen] = useState(false)
  const [selectedTheme, setSelectedTheme] = useState<BlogTheme | null>(null)
  const [selectedPreset, setSelectedPreset] = useState<ThemePreset | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['blog-themes', workspaceId, limit],
    queryFn: () => blogThemesApi.list(workspaceId, { limit, offset: 0 })
  })

  const themes = data?.themes || []
  const totalCount = data?.total_count || 0
  const hasMore = totalCount > limit
  const hasPublishedTheme = themes.some(
    (theme) => theme.published_at !== null && theme.published_at !== undefined
  )

  const publishMutation = useMutation({
    mutationFn: (version: number) => blogThemesApi.publish(workspaceId, { version }),
    onSuccess: () => {
      message.success(t`Theme published successfully`)
      queryClient.invalidateQueries({ queryKey: ['blog-themes', workspaceId] })
    },
    onError: (error: Error) => {
      message.error(error?.message || t`Failed to publish theme`)
    }
  })

  const handleEdit = (theme: BlogTheme) => {
    // Always open editor directly, regardless of published state
    setSelectedTheme(theme)
    setEditorOpen(true)
  }

  const handleCreate = () => {
    // Automatically use the default theme preset
    setSelectedTheme(null)
    setSelectedPreset(THEME_PRESETS[0])
    setEditorOpen(true)
  }

  const handlePreview = (theme: BlogTheme) => {
    if (!workspace?.settings?.custom_endpoint_url) {
      message.warning(t`Blog custom endpoint URL is not configured`)
      return
    }

    const baseUrl = workspace.settings.custom_endpoint_url
    const previewUrl = `${baseUrl}/?preview_theme_version=${theme.version}`
    window.open(previewUrl, '_blank')
  }

  const handlePublish = (theme: BlogTheme) => {
    modal.confirm({
      title: t`Publish Theme`,
      icon: <ExclamationCircleOutlined />,
      content: (
        <div>
          <p>
            {t`Are you sure you want to publish theme v${theme.version}? This will make it live on your blog.`}
          </p>
          <p style={{ marginTop: 8, color: '#8c8c8c' }}>
            {t`The currently published theme will be unpublished automatically.`}
          </p>
        </div>
      ),
      okText: t`Publish`,
      okType: 'primary',
      cancelText: t`Cancel`,
      onOk: () => publishMutation.mutate(theme.version)
    })
  }

  const handleLoadMore = () => {
    setLimit((prev) => prev + 5)
  }

  const columns = [
    {
      title: t`Version`,
      dataIndex: 'version',
      key: 'version',
      width: 60,
      render: (version: number) => {
        return <span>{version}</span>
      }
    },
    {
      title: t`Status`,
      key: 'status',
      width: 140,
      render: (record: BlogTheme) => {
        const isPublished = record.published_at !== null && record.published_at !== undefined
        if (isPublished) {
          return <Badge status="success" text={t`Live`} />
        }
        // Find the most recent version (highest version number)
        const mostRecentVersion =
          themes.length > 0 ? Math.max(...themes.map((t) => t.version)) : record.version
        const isMostRecent = record.version === mostRecentVersion
        return (
          <Tooltip title={t`Publish this theme`}>
            <Button
              type="primary"
              size="small"
              ghost={!isMostRecent}
              icon={<CloudUploadOutlined />}
              onClick={() => handlePublish(record)}
              loading={publishMutation.isPending}
            >
              {t`Publish`}
            </Button>
          </Tooltip>
        )
      }
    },
    {
      title: t`Notes`,
      dataIndex: 'notes',
      key: 'notes',
      ellipsis: true,
      render: (notes: string) => {
        if (!notes) return <span style={{ color: '#8c8c8c' }}>{t`No notes`}</span>
        return (
          <Tooltip title={notes}>
            <span>{notes}</span>
          </Tooltip>
        )
      }
    },
    {
      title: t`Last Edited`,
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
      title: t`Actions`,
      key: 'actions',
      width: 120,
      align: 'right' as const,
      render: (record: BlogTheme) => {
        return (
          <Space size="small">
            <Tooltip title={t`Edit theme`}>
              <Button
                type="text"
                size="small"
                icon={<EditOutlined />}
                onClick={() => handleEdit(record)}
              />
            </Tooltip>
            <Tooltip title={t`Preview this theme in a new tab`}>
              <Button
                type="text"
                size="small"
                icon={<EyeOutlined />}
                onClick={() => handlePreview(record)}
              />
            </Tooltip>
          </Space>
        )
      }
    }
  ]

  if (themes.length === 0 && !isLoading) {
    return (
      <div style={{ marginTop: 24 }}>
        <Empty description={t`No themes yet`} image={Empty.PRESENTED_IMAGE_SIMPLE}>
          <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
            {t`Create First Theme`}
          </Button>
        </Empty>

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
        <h3 style={{ margin: 0 }}>{t`Theme Versions`}</h3>
        <Button
          type="primary"
          ghost={hasPublishedTheme}
          icon={<PlusOutlined />}
          onClick={handleCreate}
        >
          {t`New Theme`}
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
          <Button onClick={handleLoadMore}>{t`Show More (${totalCount - limit} remaining)`}</Button>
        </div>
      )}

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
