import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Typography,
  Button,
  Table,
  Space,
  message,
  TableColumnType,
  Card,
  Empty,
  Segmented
} from 'antd'
import { useParams, useSearch, useNavigate } from '@tanstack/react-router'
import { blogPostsApi, blogCategoriesApi, BlogPost, BlogPostStatus } from '../../services/api/blog'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPenToSquare, faTrashCan } from '@fortawesome/free-regular-svg-icons'
import { PlusOutlined } from '@ant-design/icons'
import { useWorkspacePermissions, useAuth } from '../../contexts/AuthContext'
import dayjs from '../../lib/dayjs'
import { PostDrawer } from './PostDrawer'
import { DeletePostModal } from './DeletePostModal'
import { PostStatusTag } from './PostStatusTag'
import { CategoryDrawer } from './CategoryDrawer'
import { DeleteCategoryModal } from './DeleteCategoryModal'

const { Title, Paragraph } = Typography

interface PostsSearch {
  status?: BlogPostStatus
  category_id?: string
}

export function PostsTable() {
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/blog' })
  const navigate = useNavigate({ from: '/console/workspace/$workspaceId/blog' })
  const search = useSearch({ from: '/console/workspace/$workspaceId/blog' }) as PostsSearch
  const queryClient = useQueryClient()
  const { permissions } = useWorkspacePermissions(workspaceId)
  const { workspaces } = useAuth()
  
  // Get the current workspace
  const workspace = workspaces.find((w) => w.id === workspaceId)
  
  if (!workspace) {
    return null // Or handle the case where workspace is not found
  }

  const [drawerOpen, setDrawerOpen] = useState(false)
  const [editingPost, setEditingPost] = useState<BlogPost | null>(null)
  const [deleteModalOpen, setDeleteModalOpen] = useState(false)
  const [postToDelete, setPostToDelete] = useState<BlogPost | null>(null)
  const [categoryDrawerOpen, setCategoryDrawerOpen] = useState(false)
  const [deleteCategoryModalOpen, setDeleteCategoryModalOpen] = useState(false)

  const status = (search.status || 'all') as BlogPostStatus
  const categoryId = search.category_id

  // Fetch categories for filter
  const { data: categoriesData } = useQuery({
    queryKey: ['blog-categories', workspaceId],
    queryFn: () => blogCategoriesApi.list(workspaceId)
  })

  // Find the selected category
  const selectedCategory = categoryId
    ? (categoriesData?.categories ?? []).find((c) => c.id === categoryId)
    : null

  // Fetch posts
  const { data, isLoading } = useQuery({
    queryKey: ['blog-posts', workspaceId, status, categoryId],
    queryFn: () =>
      blogPostsApi.list(workspaceId, {
        status,
        category_id: categoryId,
        limit: 100
      })
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => blogPostsApi.delete(workspaceId, { id }),
    onSuccess: () => {
      message.success('Post deleted successfully')
      queryClient.invalidateQueries({ queryKey: ['blog-posts', workspaceId] })
      setDeleteModalOpen(false)
      setPostToDelete(null)
    },
    onError: (error: any) => {
      const errorMsg = error?.message || 'Failed to delete post'
      message.error(errorMsg)
    }
  })

  const publishMutation = useMutation({
    mutationFn: (id: string) => blogPostsApi.publish(workspaceId, { id }),
    onSuccess: () => {
      message.success('Post published successfully')
      queryClient.invalidateQueries({ queryKey: ['blog-posts', workspaceId] })
    },
    onError: (error: any) => {
      const errorMsg = error?.message || 'Failed to publish post'
      message.error(errorMsg)
    }
  })

  const unpublishMutation = useMutation({
    mutationFn: (id: string) => blogPostsApi.unpublish(workspaceId, { id }),
    onSuccess: () => {
      message.success('Post unpublished successfully')
      queryClient.invalidateQueries({ queryKey: ['blog-posts', workspaceId] })
    },
    onError: (error: any) => {
      const errorMsg = error?.message || 'Failed to unpublish post'
      message.error(errorMsg)
    }
  })

  const handleEdit = (post: BlogPost) => {
    setEditingPost(post)
    setDrawerOpen(true)
  }

  const handleDelete = (post: BlogPost) => {
    setPostToDelete(post)
    setDeleteModalOpen(true)
  }

  const handleCreateNew = () => {
    setEditingPost(null)
    setDrawerOpen(true)
  }

  const handleDrawerClose = () => {
    setDrawerOpen(false)
    setEditingPost(null)
  }

  const handleStatusChange = (value: string | number) => {
    navigate({
      search: (prev) => ({ ...prev, status: value as BlogPostStatus })
    })
  }

  const deleteCategoryMutation = useMutation({
    mutationFn: (id: string) => blogCategoriesApi.delete(workspaceId, { id }),
    onSuccess: () => {
      message.success('Category deleted successfully')
      queryClient.invalidateQueries({ queryKey: ['blog-categories', workspaceId] })
      setDeleteCategoryModalOpen(false)
      // Navigate to all posts after deletion
      navigate({
        search: (prev) => ({ ...prev, category_id: undefined })
      })
    },
    onError: (error: any) => {
      const errorMsg = error?.message || 'Failed to delete category'
      message.error(errorMsg)
    }
  })

  const getCategoryName = (categoryId?: string | null) => {
    if (!categoryId) return 'Uncategorized'
    const category = (categoriesData?.categories ?? []).find((c) => c.id === categoryId)
    return category?.settings.name || 'Unknown'
  }

  const columns: TableColumnType<BlogPost>[] = [
    {
      title: 'Title',
      dataIndex: ['settings', 'title'],
      key: 'title',
      render: (title: string, record: BlogPost) => (
        <div>
          <div className="font-medium">{title}</div>
          {record.settings.excerpt && (
            <div className="text-xs text-gray-500 mt-1 line-clamp-2">{record.settings.excerpt}</div>
          )}
        </div>
      )
    },
    {
      title: 'Slug',
      dataIndex: 'slug',
      key: 'slug',
      render: (slug: string) => (
        <code className="text-xs bg-gray-100 px-2 py-1 rounded">{slug}</code>
      )
    },
    {
      title: 'Category',
      dataIndex: 'category_id',
      key: 'category_id',
      render: (categoryId?: string | null) => (
        <span className="text-sm">{getCategoryName(categoryId)}</span>
      )
    },
    {
      title: 'Status',
      key: 'status',
      render: (_: any, record: BlogPost) => <PostStatusTag post={record} />
    },
    {
      title: 'Updated',
      dataIndex: 'updated_at',
      key: 'updated_at',
      render: (date: string) => dayjs(date).format('MMM D, YYYY')
    },
    {
      title: 'Actions',
      key: 'actions',
      width: 150,
      render: (_: any, record: BlogPost) => (
        <Space size="small">
          {permissions?.workspace?.write && (
            <>
              <Button
                type="text"
                size="small"
                icon={<FontAwesomeIcon icon={faPenToSquare} />}
                onClick={() => handleEdit(record)}
              />
              {record.published_at ? (
                <Button
                  type="text"
                  size="small"
                  onClick={() => unpublishMutation.mutate(record.id)}
                  loading={unpublishMutation.isPending}
                >
                  Unpublish
                </Button>
              ) : (
                <Button
                  type="text"
                  size="small"
                  onClick={() => publishMutation.mutate(record.id)}
                  loading={publishMutation.isPending}
                >
                  Publish
                </Button>
              )}
              <Button
                type="text"
                size="small"
                danger
                icon={<FontAwesomeIcon icon={faTrashCan} />}
                onClick={() => handleDelete(record)}
              />
            </>
          )}
        </Space>
      )
    }
  ]

  const hasPosts = !isLoading && data?.posts && data.posts.length > 0

  const getEmptyDescription = () => {
    if (status === 'draft') return 'No draft posts'
    if (status === 'published') return 'No published posts'
    if (categoryId) return 'No posts in this category'
    return 'No posts yet'
  }

  return (
    <div>
      <div className="flex justify-between items-start mb-6">
        <div>
          <Title level={4} className="!mb-2">
            {selectedCategory ? selectedCategory.settings.name : 'All Posts'}
          </Title>
          <Paragraph className="!mb-0 text-gray-600">
            {selectedCategory
              ? `Posts in ${selectedCategory.settings.name}`
              : 'Create and manage your blog content'}
          </Paragraph>
        </div>
        {permissions?.workspace?.write && (
          <Button type="primary" icon={<PlusOutlined />} onClick={handleCreateNew}>
            New Post
          </Button>
        )}
      </div>

      <div className="mb-4">
        <Segmented
          value={status}
          onChange={handleStatusChange}
          options={[
            { label: 'All Posts', value: 'all' },
            { label: 'Drafts', value: 'draft' },
            { label: 'Published', value: 'published' }
          ]}
        />
      </div>

      {hasPosts ? (
        <Card>
          <Table
            columns={columns}
            dataSource={data?.posts}
            loading={isLoading}
            rowKey="id"
            pagination={{
              pageSize: 50,
              showTotal: (total) => `Total ${total} posts`
            }}
          />
        </Card>
      ) : (
        <Card>
          <Empty description={getEmptyDescription()} />
        </Card>
      )}

      <PostDrawer
        open={drawerOpen}
        onClose={handleDrawerClose}
        post={editingPost}
        workspace={workspace}
        initialCategoryId={categoryId}
      />

      <DeletePostModal
        open={deleteModalOpen}
        post={postToDelete}
        onConfirm={() => postToDelete && deleteMutation.mutate(postToDelete.id)}
        onCancel={() => {
          setDeleteModalOpen(false)
          setPostToDelete(null)
        }}
        loading={deleteMutation.isPending}
      />

      <CategoryDrawer
        open={categoryDrawerOpen}
        onClose={() => setCategoryDrawerOpen(false)}
        category={selectedCategory || null}
        workspaceId={workspaceId}
      />

      <DeleteCategoryModal
        open={deleteCategoryModalOpen}
        category={selectedCategory || null}
        onConfirm={() => selectedCategory && deleteCategoryMutation.mutate(selectedCategory.id)}
        onCancel={() => setDeleteCategoryModalOpen(false)}
        loading={deleteCategoryMutation.isPending}
      />
    </div>
  )
}
