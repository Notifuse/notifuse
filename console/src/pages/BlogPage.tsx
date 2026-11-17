import { useState, type ReactNode } from 'react'
import { App, Button, Empty, Tooltip } from 'antd'
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
import { RailCard } from '../components/layout'

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
  // Posts and categories follow blog write, as the server does
  const canWritePosts = Boolean(permissions?.blog?.write)
  const canManageCategories = Boolean(permissions?.blog?.write)
  // A post's content is a template: the drawer reads it with Templates read and saves it with Templates write
  const editRefusal =
    permissions && !(permissions.templates?.read && permissions.templates?.write)
      ? t`Writing or editing a post also needs Templates read and write: its content is a template.`
      : undefined

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
  const titleSearch = search.q?.trim() || undefined

  // B1, under the posts' key so every post change refreshes it. The tabs count what they list, so
  // the counts follow the search too; the rail's all_total and by_category never do.
  const countsQuery = useQuery({
    queryKey: ['blog-posts', workspaceId, 'counts', activeCategoryId, titleSearch ?? null],
    queryFn: () => blogPostsApi.counts(workspaceId, activeCategoryId ?? undefined, titleSearch),
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
      // Deleted from its own drawer: nothing is left to edit.
      setCategoryDrawer((open) => (open?.category?.id === id ? null : open))
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

  const rail = (
    <RailCard ariaLabel={t`Blog`}>
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
    </RailCard>
  )

  // An empty state sits beside the card, where the table would be.
  const besideRail = (content: ReactNode) => (
    <div className="flex items-start gap-3">
      {rail}
      <div className="min-w-0 flex-1">{content}</div>
    </div>
  )

  const renderMain = () => {
    if (categoriesQuery.isSuccess && categories.length === 0) {
      return besideRail(
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
      return besideRail(
        <div className="mt-24">
          <Empty description={t`Create your first blog post to get started`}>
            <Tooltip title={canWritePosts ? editRefusal : undefined}>
              <span className="inline-block">
                <Button
                  type="primary"
                  icon={<Plus size={14} aria-hidden="true" />}
                  disabled={!canWritePosts || !!editRefusal}
                  onClick={() => setPostDrawerOpen(true)}
                >
                  {t`Create Your First Post`}
                </Button>
              </span>
            </Tooltip>
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
        editRefusal={editRefusal}
        showViews={showViews}
        rail={rail}
      />
    )
  }

  // The page route declares the rail layout, so the chrome adds no margin: this column carries the
  // page margins, as on the Contacts page.
  return (
    <div data-rail-content className="flex min-w-0 flex-col gap-3 px-5 pt-4 pb-6">
      {renderMain()}

      <CategoryDrawer
        open={categoryDrawer !== null}
        onClose={() => setCategoryDrawer(null)}
        category={categoryDrawer?.category ?? null}
        workspace={workspace}
        postCount={
          categoryDrawer?.category && counts ? (counts.by_category?.[categoryDrawer.category.id] ?? 0) : undefined
        }
        onDelete={setCategoryToDelete}
        // The new category opens, so its first post is one click away.
        onCreated={(category) =>
          navigate({ search: (prev: BlogSearch) => ({ ...prev, category_id: category.id }) })
        }
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
    </div>
  )
}
