import { useState } from 'react'
import { App, Button, Empty } from 'antd'
import { useParams, useNavigate, useSearch } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { PostsTable } from '../components/blog/PostsTable'
import { BlogSidebar } from '../components/blog/BlogSidebar'
import { CategoryDrawer } from '../components/blog/CategoryDrawer'
import { DeleteCategoryModal } from '../components/blog/DeleteCategoryModal'
import { PostDrawer } from '../components/blog/PostDrawer'
import { blogCategoriesApi, blogPostsApi, type BlogCategory } from '../services/api/blog'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import type { BlogSearch } from '../lib/searchParams'
import { RailLayout } from '../components/layout'

const ROUTE = '/console/workspace/$workspaceId/blog'

export function BlogPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: ROUTE })
  const navigate = useNavigate({ from: ROUTE })
  const search = useSearch({ from: ROUTE }) as BlogSearch
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const { workspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId)
  // Posts follow workspace write, as they always have; categories follow blog write.
  const canWritePosts = Boolean(permissions?.workspace?.write)
  const canManageCategories = Boolean(permissions?.blog?.write)

  const [categoryDrawer, setCategoryDrawer] = useState<{ category: BlogCategory | null } | null>(
    null
  )
  const [categoryToDelete, setCategoryToDelete] = useState<BlogCategory | null>(null)
  const [postDrawerOpen, setPostDrawerOpen] = useState(false)

  const categoriesQuery = useQuery({
    queryKey: ['blog-categories', workspaceId],
    queryFn: () => blogCategoriesApi.list(workspaceId)
  })
  const categories = categoriesQuery.data?.categories ?? []
  const activeCategoryId = search.category_id ?? null

  // B1, under the posts' key so every post change refreshes it.
  const countsQuery = useQuery({
    queryKey: ['blog-posts', workspaceId, 'counts', activeCategoryId],
    queryFn: () => blogPostsApi.counts(workspaceId, activeCategoryId ?? undefined),
    // The rail's counts do not change with the category: keep them while another category's
    // load. The status tabs do, so the table gets none until they arrive.
    placeholderData: keepPreviousData
  })
  const counts = countsQuery.data

  const deleteCategoryMutation = useMutation({
    mutationFn: (id: string) => blogCategoriesApi.delete(workspaceId, { id }),
    onSuccess: (_, id) => {
      message.success(t`Category deleted successfully`)
      void queryClient.invalidateQueries({ queryKey: ['blog-categories', workspaceId] })
      void queryClient.invalidateQueries({ queryKey: ['blog-posts', workspaceId] })
      setCategoryToDelete(null)
      if (activeCategoryId === id) {
        navigate({ search: (prev: BlogSearch) => ({ ...prev, category_id: undefined }) })
      }
    },
    onError: (error: unknown) => {
      message.error(error instanceof Error ? error.message : t`Failed to delete category`)
    }
  })

  const workspace = workspaces.find((w) => w.id === workspaceId)
  if (!workspace) {
    return null
  }

  const showViews =
    Boolean(workspace.settings?.web_analytics?.enabled) && Boolean(permissions?.web_analytics?.read)

  const renderMain = () => {
    if (categoriesQuery.isSuccess && categories.length === 0) {
      return (
        <div className="mt-24">
          <Empty description={t`No categories yet`}>
            {canManageCategories ? (
              <Button
                type="primary"
                icon={<Plus size={14} aria-hidden="true" />}
                onClick={() => setCategoryDrawer({ category: null })}
              >
                {t`Create Your First Category`}
              </Button>
            ) : null}
          </Empty>
        </div>
      )
    }
    if (counts && counts.all_total === 0 && !activeCategoryId) {
      return (
        <div className="mt-24">
          <Empty description={t`Create your first blog post to get started`}>
            <Button
              type="primary"
              icon={<Plus size={14} aria-hidden="true" />}
              disabled={!canWritePosts}
              onClick={() => setPostDrawerOpen(true)}
            >
              {t`Create Your First Post`}
            </Button>
          </Empty>
        </div>
      )
    }
    return (
      <PostsTable
        workspace={workspace}
        categories={categories}
        counts={countsQuery.isPlaceholderData ? undefined : counts}
        canWritePosts={canWritePosts}
        showViews={showViews}
      />
    )
  }

  return (
    <RailLayout
      ariaLabel={t`Blog`}
      rail={
        <BlogSidebar
          workspaceId={workspaceId}
          activeCategoryId={activeCategoryId}
          counts={counts}
          canManageCategories={canManageCategories}
          onCategoryChange={(categoryId) =>
            navigate({
              search: (prev: BlogSearch) => ({ ...prev, category_id: categoryId ?? undefined })
            })
          }
          onNewCategory={() => setCategoryDrawer({ category: null })}
          onEditCategory={(category) => setCategoryDrawer({ category })}
          onDeleteCategory={setCategoryToDelete}
        />
      }
    >
      {renderMain()}

      <CategoryDrawer
        open={categoryDrawer !== null}
        onClose={() => setCategoryDrawer(null)}
        category={categoryDrawer?.category ?? null}
        workspaceId={workspaceId}
      />

      <DeleteCategoryModal
        open={categoryToDelete !== null}
        category={categoryToDelete}
        onConfirm={() => categoryToDelete && deleteCategoryMutation.mutate(categoryToDelete.id)}
        onCancel={() => setCategoryToDelete(null)}
        loading={deleteCategoryMutation.isPending}
      />

      <PostDrawer
        open={postDrawerOpen}
        onClose={() => setPostDrawerOpen(false)}
        post={null}
        workspace={workspace}
        initialCategoryId={activeCategoryId}
      />
    </RailLayout>
  )
}
