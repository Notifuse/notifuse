import { useState } from 'react'
import { Layout, Button, Empty, App } from 'antd'
import { useParams, useNavigate, useSearch } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { PlusOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { PostsTable } from '../components/blog/PostsTable'
import { BlogSidebar } from '../components/blog/BlogSidebar'
import { CategoryDrawer } from '../components/blog/CategoryDrawer'
import { DeleteCategoryModal } from '../components/blog/DeleteCategoryModal'
import { PostDrawer } from '../components/blog/PostDrawer'
import { blogCategoriesApi, blogPostsApi, BlogCategory } from '../services/api/blog'
import { useAuth } from '../contexts/AuthContext'

const { Sider, Content } = Layout

interface BlogSearch {
  category_id?: string
}

export function BlogPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/blog' })
  const navigate = useNavigate({ from: '/console/workspace/$workspaceId/blog' })
  const search = useSearch({ from: '/console/workspace/$workspaceId/blog' }) as BlogSearch
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const { workspaces } = useAuth()

  // All hooks must be called before any conditional returns
  const [categoryDrawerOpen, setCategoryDrawerOpen] = useState(false)
  const [editingCategory, setEditingCategory] = useState<BlogCategory | null>(null)
  const [deleteCategoryModalOpen, setDeleteCategoryModalOpen] = useState(false)
  const [categoryToDelete, setCategoryToDelete] = useState<BlogCategory | null>(null)
  const [postDrawerOpen, setPostDrawerOpen] = useState(false)

  const { data: categoriesData } = useQuery({
    queryKey: ['blog-categories', workspaceId],
    queryFn: () => blogCategoriesApi.list(workspaceId)
  })

  const categories = categoriesData?.categories ?? []
  const hasCategories = categories.length > 0
  const activeCategoryId = search.category_id || null

  // Check if there are any posts (without filters) for empty state
  const { data: allPostsData } = useQuery({
    queryKey: ['blog-posts', workspaceId, 'all', undefined],
    queryFn: () => blogPostsApi.list(workspaceId, { status: 'all', limit: 1 }),
    enabled: hasCategories
  })

  const hasPosts = allPostsData && allPostsData.total_count > 0

  const deleteCategoryMutation = useMutation({
    mutationFn: (id: string) => blogCategoriesApi.delete(workspaceId, { id }),
    onSuccess: () => {
      message.success(t`Category deleted successfully`)
      queryClient.invalidateQueries({ queryKey: ['blog-categories', workspaceId] })
      setDeleteCategoryModalOpen(false)
      setCategoryToDelete(null)
      // Navigate to all posts if the deleted category was active
      if (activeCategoryId === categoryToDelete?.id) {
        navigate({
          search: (prev) => ({ ...prev, category_id: undefined })
        })
      }
    },
    onError: (error: unknown) => {
      const errorMsg = error instanceof Error ? error.message : t`Failed to delete category`
      message.error(errorMsg)
    }
  })

  // Get the current workspace - conditional check AFTER all hooks
  const workspace = workspaces.find((w) => w.id === workspaceId)

  if (!workspace) {
    return null // Or handle the case where workspace is not found
  }

  const handleCategoryChange = (categoryId: string | null) => {
    navigate({
      search: (prev) => ({ ...prev, category_id: categoryId || undefined })
    })
  }

  const handleNewCategory = () => {
    setEditingCategory(null)
    setCategoryDrawerOpen(true)
  }

  const handleEditCategory = (category: BlogCategory) => {
    setEditingCategory(category)
    setCategoryDrawerOpen(true)
  }

  const handleDeleteCategory = (category: BlogCategory) => {
    setCategoryToDelete(category)
    setDeleteCategoryModalOpen(true)
  }

  const handleCategoryDrawerClose = () => {
    setCategoryDrawerOpen(false)
    setEditingCategory(null)
  }

  const handleCreatePost = () => {
    setPostDrawerOpen(true)
  }

  const handlePostDrawerClose = () => {
    setPostDrawerOpen(false)
  }

  return (
    <Layout style={{ minHeight: 'calc(100vh - 48px)' }}>
      <Sider
        width={250}
        style={{
          borderRight: '1px solid #f0f0f0',
          overflow: 'auto'
        }}
      >
        <BlogSidebar
          workspaceId={workspaceId}
          activeCategoryId={activeCategoryId}
          onCategoryChange={handleCategoryChange}
          onNewCategory={handleNewCategory}
          onEditCategory={handleEditCategory}
          onDeleteCategory={handleDeleteCategory}
        />
      </Sider>
      <Layout>
        <Content>
          <div style={{ padding: '24px' }}>
            {!hasCategories ? (
              <Empty description={t`No categories yet`} style={{ marginTop: '100px' }}>
                <Button type="primary" icon={<PlusOutlined />} onClick={handleNewCategory}>
                  {t`Create Your First Category`}
                </Button>
              </Empty>
            ) : !hasPosts && !activeCategoryId ? (
              <Empty
                description={t`Create your first blog post to get started`}
                style={{ marginTop: '100px' }}
              >
                <Button type="primary" icon={<PlusOutlined />} onClick={handleCreatePost}>
                  {t`Create Your First Post`}
                </Button>
              </Empty>
            ) : (
              <PostsTable />
            )}
          </div>
        </Content>
      </Layout>

      <CategoryDrawer
        open={categoryDrawerOpen}
        onClose={handleCategoryDrawerClose}
        category={editingCategory}
        workspaceId={workspaceId}
      />

      <DeleteCategoryModal
        open={deleteCategoryModalOpen}
        category={categoryToDelete}
        onConfirm={() => categoryToDelete && deleteCategoryMutation.mutate(categoryToDelete.id)}
        onCancel={() => {
          setDeleteCategoryModalOpen(false)
          setCategoryToDelete(null)
        }}
        loading={deleteCategoryMutation.isPending}
      />

      <PostDrawer
        open={postDrawerOpen}
        onClose={handlePostDrawerClose}
        post={null}
        workspace={workspace}
        initialCategoryId={activeCategoryId}
      />
    </Layout>
  )
}
