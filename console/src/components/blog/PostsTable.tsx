import { useMemo, useState } from 'react'
import { plural } from '@lingui/core/macro'
import { useLingui } from '@lingui/react/macro'
import {
  useInfiniteQuery,
  useMutation,
  useQueries,
  useQueryClient,
  type InfiniteData
} from '@tanstack/react-query'
import { App, Button, Dropdown, Empty, Result, Skeleton, Table, Tooltip } from 'antd'
import type { MenuProps, TableColumnType } from 'antd'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { ExternalLink, MoreHorizontal, Palette, Plus } from 'lucide-react'
import {
  blogPostsApi,
  type BlogCategory,
  type BlogPost,
  type BlogPostCounts,
  type BlogPostListResponse,
  type BlogPostStatus
} from '../../services/api/blog'
import { analyticsService } from '../../services/api/analytics'
import type { Workspace } from '../../services/api/types'
import type { BlogSearch } from '../../lib/searchParams'
import { useDebouncedUrlValue } from '../../lib/useDebouncedUrlValue'
import dayjs from '../../lib/dayjs'
import {
  CountTabs,
  Mono,
  PageHeader,
  Panel,
  Pill,
  TableToolbar,
  useNumberFormat,
  useRelativeDate,
  type CountTabOption
} from '../layout'
import { MissingMetaTagsWarning } from '../seo/MissingMetaTagsWarning'
import { PostDrawer } from './PostDrawer'
import { DeletePostModal } from './DeletePostModal'
import { PublishModal } from './PublishModal'
import { postPath, postState } from './postState'
import { postViewsQuery, viewsByPath, viewsWindow } from './postViews'

const ROUTE = '/console/workspace/$workspaceId/blog'
const PAGE_SIZE = 50

interface PostsTableProps {
  workspace: Workspace
  categories: BlogCategory[]
  /** Posts by status in the current category; absent while loading or after an error. */
  counts?: BlogPostCounts
  /** Workspace write, the right the post buttons have always followed. */
  canWritePosts: boolean
  /** Web analytics on in the workspace and readable by the member: the Views 30d column. */
  showViews: boolean
}

/** The blog's main column: header, status tabs and search, and the posts, 50 at a time. */
export function PostsTable({
  workspace,
  categories,
  counts,
  canWritePosts,
  showViews
}: PostsTableProps) {
  const { t } = useLingui()
  const { message, modal } = App.useApp()
  const relative = useRelativeDate()
  const { int } = useNumberFormat()
  const navigate = useNavigate({ from: ROUTE })
  const search = useSearch({ from: ROUTE }) as BlogSearch
  const queryClient = useQueryClient()
  const workspaceId = workspace.id
  const timezone = workspace.settings?.timezone || 'UTC'

  const status: BlogPostStatus = search.status ?? 'all'
  const categoryId = search.category_id
  const q = search.q ?? ''
  const category = categoryId ? categories.find((c) => c.id === categoryId) : undefined

  const [searchInput, setSearchInput] = useDebouncedUrlValue(q, (value) =>
    navigate({
      replace: true,
      search: (prev: BlogSearch) => ({ ...prev, q: value || undefined })
    })
  )

  const [drawer, setDrawer] = useState<{ post: BlogPost | null } | null>(null)
  const [postToDelete, setPostToDelete] = useState<BlogPost | null>(null)
  const [postToPublish, setPostToPublish] = useState<BlogPost | null>(null)

  // B2: the posts of the view, a page of 50 at a time, appended by "Load more".
  const postsQuery = useInfiniteQuery<
    BlogPostListResponse,
    Error,
    InfiniteData<BlogPostListResponse, number>,
    readonly unknown[],
    number
  >({
    queryKey: ['blog-posts', workspaceId, 'list', status, categoryId ?? null, q],
    queryFn: ({ pageParam }) =>
      blogPostsApi.list(workspaceId, {
        status,
        category_id: categoryId,
        search: q || undefined,
        limit: PAGE_SIZE,
        page: pageParam
      }),
    initialPageParam: 1,
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((sum, p) => sum + (p.posts?.length ?? 0), 0)
      // The server sends no next-page flag: more remain while fewer than its total are loaded.
      return loaded < last.total_count && (last.posts?.length ?? 0) > 0
        ? pages.length + 1
        : undefined
    }
  })

  const pages = postsQuery.data?.pages ?? []
  // The API answers an empty page with "posts": null.
  const posts = pages.flatMap((p) => p.posts ?? [])
  const total = pages.length > 0 ? pages[pages.length - 1].total_count : undefined

  const categoryOf = (post: BlogPost) =>
    post.category_id ? categories.find((c) => c.id === post.category_id) : undefined

  // B3: page views of each live post over 30 days, one query per page of posts.
  const viewsRange = useMemo(() => viewsWindow(), [])
  const viewQueries = useQueries({
    queries: pages.map((p) => {
      const paths = (p.posts ?? []).flatMap((post) => {
        const slug = categoryOf(post)?.slug
        return slug && postState(post) === 'published' ? [postPath(slug, post.slug)] : []
      })
      return {
        queryKey: ['blog-views-30d', workspaceId, viewsRange[0], paths],
        queryFn: async () =>
          viewsByPath(
            (await analyticsService.query(postViewsQuery(paths, viewsRange), workspaceId)).data
          ),
        enabled: showViews && paths.length > 0,
        staleTime: 5 * 60 * 1000
      }
    })
  })
  const views = new Map<string, number>()
  for (const result of viewQueries) result.data?.forEach((count, path) => views.set(path, count))
  const viewsFailed = viewQueries.some((result) => result.isError)

  const invalidatePosts = () =>
    queryClient.invalidateQueries({ queryKey: ['blog-posts', workspaceId] })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => blogPostsApi.delete(workspaceId, { id }),
    onSuccess: () => {
      message.success(t`Post deleted successfully`)
      void invalidatePosts()
      setPostToDelete(null)
    },
    onError: (error: Error) => message.error(error?.message || t`Failed to delete post`)
  })

  const unpublishMutation = useMutation({
    mutationFn: (id: string) => blogPostsApi.unpublish(workspaceId, { id }),
    onSuccess: () => {
      message.success(t`Post unpublished successfully`)
      void invalidatePosts()
    },
    onError: (error: Error) => message.error(error?.message || t`Failed to unpublish post`)
  })

  const loadMore = () =>
    postsQuery.fetchNextPage().then((result) => {
      if (result.isFetchNextPageError) {
        message.error(result.error?.message || t`Couldn't load more posts`)
      }
    })

  const setStatus = (value: BlogPostStatus) =>
    navigate({
      search: (prev: BlogSearch) => ({ ...prev, status: value === 'all' ? undefined : value })
    })

  const baseUrl = (
    workspace.settings?.custom_endpoint_url ||
    window.API_ENDPOINT ||
    'https://example.com'
  ).replace(/\/+$/, '')
  const blogUrl = category ? `${baseUrl}/${category.slug}` : baseUrl
  const blogDomain = baseUrl.replace(/^https?:\/\//, '')

  const postUrl = (post: BlogPost) =>
    `${baseUrl}/${categoryOf(post)?.slug || 'uncategorized'}/${post.slug}`

  const fullDate = (date: string) => dayjs(date).tz(timezone).format('lll')

  const confirmUnpublish = (post: BlogPost) =>
    modal.confirm({
      title: t`Unpublish post`,
      content:
        postState(post) === 'scheduled'
          ? t`The post goes back to drafts and will not be published on its date.`
          : t`Are you sure you want to unpublish this post?`,
      okText: t`Unpublish`,
      cancelText: t`Cancel`,
      onOk: () => unpublishMutation.mutateAsync(post.id)
    })

  const rowMenu = (post: BlogPost): MenuProps['items'] => {
    const state = postState(post)
    return [
      { key: 'edit', label: t`Edit`, onClick: () => setDrawer({ post }) },
      ...(state === 'published'
        ? [
            {
              key: 'open',
              label: t`Open on web`,
              onClick: () => window.open(postUrl(post), '_blank', 'noopener')
            }
          ]
        : []),
      state === 'draft'
        ? { key: 'publish', label: t`Publish`, onClick: () => setPostToPublish(post) }
        : { key: 'unpublish', label: t`Unpublish`, onClick: () => confirmUnpublish(post) },
      { type: 'divider' as const },
      { key: 'delete', label: t`Delete`, danger: true, onClick: () => setPostToDelete(post) }
    ]
  }

  const renderPublished = (post: BlogPost) => {
    const state = postState(post)
    const updatedAt = fullDate(post.updated_at)
    const title =
      state === 'draft'
        ? t`Updated ${updatedAt} in ${timezone}`
        : t`Published ${fullDate(post.published_at as string)} · Updated ${updatedAt} in ${timezone}`
    let content
    if (state === 'draft') {
      content = <span className="text-ink-3">{t`Edited ${relative(post.updated_at)}`}</span>
    } else if (state === 'scheduled') {
      const days = dayjs(post.published_at).diff(dayjs(), 'day')
      content = (
        <span className="text-info-fg">
          {days === 0
            ? t`Scheduled today`
            : plural(days, { one: 'Scheduled in # day', other: 'Scheduled in # days' })}
        </span>
      )
    } else {
      content = <span className="text-ink-2">{relative(post.published_at as string)}</span>
    }
    return <Tooltip title={title}>{content}</Tooltip>
  }

  const columns: TableColumnType<BlogPost>[] = [
    {
      title: t`Post`,
      key: 'post',
      render: (_: unknown, post: BlogPost) => {
        const minutes = post.settings.reading_time_minutes
        return (
          <div className="flex min-w-0 flex-col">
            <Tooltip
              title={
                <div className="flex flex-col gap-1.5">
                  <Mono>{post.slug}</Mono>
                  {post.settings.featured_image_url ? (
                    <img
                      src={post.settings.featured_image_url}
                      alt=""
                      className="max-w-[240px] rounded-sm"
                    />
                  ) : null}
                </div>
              }
            >
              <span className="truncate font-medium text-ink">{post.settings.title}</span>
            </Tooltip>
            {minutes > 0 ? (
              <span className="text-xs text-ink-3">
                {plural(minutes, { one: '# min read', other: '# min read' })}
              </span>
            ) : null}
          </div>
        )
      }
    },
    ...(categoryId
      ? []
      : [
          {
            title: t`Category`,
            key: 'category',
            render: (_: unknown, post: BlogPost) => (
              <Pill tone="neutral" dot={false}>
                {!post.category_id
                  ? t`Uncategorized`
                  : (categoryOf(post)?.settings.name ?? t`Unknown`)}
              </Pill>
            )
          }
        ]),
    {
      title: t`Status`,
      key: 'status',
      render: (_: unknown, post: BlogPost) => {
        const state = postState(post)
        return (
          <span className="inline-flex items-center gap-1.5">
            {state === 'published' ? (
              <Pill tone="success">{t`Published`}</Pill>
            ) : state === 'scheduled' ? (
              <Pill tone="info">{t`Scheduled`}</Pill>
            ) : (
              <Pill tone="neutral">{t`Draft`}</Pill>
            )}
            <MissingMetaTagsWarning seo={post.settings.seo} />
          </span>
        )
      }
    },
    {
      title: t`Author`,
      key: 'author',
      render: (_: unknown, post: BlogPost) => {
        const authors = post.settings.authors ?? []
        if (authors.length === 0) return <span className="text-ink-4">—</span>
        const others = authors.length - 1
        const names = (
          <span className="text-ink-2">
            {authors[0].name}
            {others > 0 ? <span className="text-ink-3"> +{int(others)}</span> : null}
          </span>
        )
        return others > 0 ? (
          <Tooltip title={authors.map((author) => author.name).join(', ')}>{names}</Tooltip>
        ) : (
          names
        )
      }
    },
    {
      title: t`Published`,
      key: 'published',
      render: (_: unknown, post: BlogPost) => renderPublished(post)
    },
    ...(showViews
      ? [
          {
            title: (
              <Tooltip
                title={t`Page views of this post's path over the last 30 days, counted by web analytics`}
              >
                <span className="cursor-help">{t`Views 30d`}</span>
              </Tooltip>
            ),
            key: 'views',
            align: 'right' as const,
            render: (_: unknown, post: BlogPost) => {
              const slug = categoryOf(post)?.slug
              const count = slug ? views.get(postPath(slug, post.slug)) : undefined
              if (count === undefined) {
                return viewsFailed ? (
                  <Tooltip title={t`Couldn't load`}>
                    <span className="text-ink-4">—</span>
                  </Tooltip>
                ) : (
                  <span className="text-ink-4">—</span>
                )
              }
              return <span className="text-ink-2 tabular-nums">{int(count)}</span>
            }
          }
        ]
      : []),
    ...(canWritePosts
      ? [
          {
            key: 'actions',
            width: 48,
            render: (_: unknown, post: BlogPost) => (
              // The menu is not a click on the row.
              <span onClick={(event) => event.stopPropagation()}>
                <Dropdown trigger={['click']} menu={{ items: rowMenu(post) }}>
                  <Button
                    type="text"
                    size="small"
                    aria-label={t`More actions`}
                    icon={<MoreHorizontal size={16} aria-hidden="true" />}
                  />
                </Dropdown>
              </span>
            )
          }
        ]
      : [])
  ]

  const tabs: CountTabOption<BlogPostStatus>[] = [
    { value: 'all', label: t`All`, count: counts?.total },
    { value: 'published', label: t`Published`, count: counts?.published },
    { value: 'scheduled', label: t`Scheduled`, count: counts?.scheduled },
    { value: 'draft', label: t`Drafts`, count: counts?.draft }
  ]

  const emptyDescription = () => {
    if (q) return t`No posts match your search`
    if (status === 'draft') return t`No draft posts`
    if (status === 'published') return t`No published posts`
    if (status === 'scheduled') return t`No scheduled posts`
    if (categoryId) return t`No posts in this category`
    return t`No posts yet`
  }

  const renderBody = () => {
    if (postsQuery.isPending) {
      return (
        <div data-testid="posts-skeleton" className="px-3.5 py-3">
          <Skeleton active title={false} paragraph={{ rows: 6 }} />
        </div>
      )
    }
    if (postsQuery.isError && pages.length === 0) {
      return (
        <Result
          status="error"
          title={t`Couldn't load the posts`}
          subTitle={postsQuery.error.message}
          extra={<Button onClick={() => void postsQuery.refetch()}>{t`Retry`}</Button>}
        />
      )
    }
    if (posts.length === 0) {
      return (
        <div className="py-10">
          <Empty description={emptyDescription()} />
        </div>
      )
    }
    return (
      <Table<BlogPost>
        columns={columns}
        dataSource={posts}
        rowKey="id"
        pagination={false}
        rowClassName={() => (canWritePosts ? 'cursor-pointer' : '')}
        onRow={(post) => ({
          onClick: canWritePosts ? () => setDrawer({ post }) : undefined
        })}
      />
    )
  }

  const shown = int(posts.length)
  const footer =
    total !== undefined && posts.length > 0 ? (
      <div className="flex items-center justify-between">
        <span className="text-sm text-ink-3">
          {plural(total, { one: `${shown} of # post`, other: `${shown} of # posts` })}
        </span>
        {postsQuery.hasNextPage ? (
          <Button
            size="small"
            loading={postsQuery.isFetchingNextPage}
            onClick={() => void loadMore()}
          >
            {t`Load more`}
          </Button>
        ) : null}
      </div>
    ) : undefined

  return (
    <div className="flex flex-col gap-3">
      <PageHeader
        title={category ? category.settings.name : t`All posts`}
        count={total}
        description={
          <span className="inline-flex flex-wrap items-center gap-1">
            {t`Blog published on`}
            <a
              href={blogUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-primary-600 hover:text-primary-700"
            >
              {category ? `${blogDomain}/${category.slug}` : blogDomain}
              <ExternalLink size={12} aria-hidden="true" />
            </a>
          </span>
        }
        actions={
          <>
            <Button
              icon={<Palette size={14} aria-hidden="true" />}
              disabled={!canWritePosts}
              onClick={() =>
                navigate({
                  to: '/console/workspace/$workspaceId/settings/$section',
                  params: { workspaceId, section: 'blog' }
                })
              }
            >
              {t`Theme`}
            </Button>
            <Button
              type="primary"
              icon={<Plus size={14} aria-hidden="true" />}
              disabled={!canWritePosts}
              onClick={() => setDrawer({ post: null })}
            >
              {t`New post`}
            </Button>
          </>
        }
      />

      <Panel flush footer={footer}>
        <TableToolbar
          start={<CountTabs<BlogPostStatus> options={tabs} value={status} onChange={setStatus} />}
          end={
            <TableToolbar.Search
              value={searchInput}
              onChange={setSearchInput}
              placeholder={t`Title`}
            />
          }
        />
        {renderBody()}
      </Panel>

      <PostDrawer
        open={drawer !== null}
        onClose={() => setDrawer(null)}
        post={drawer?.post ?? null}
        workspace={workspace}
        initialCategoryId={categoryId}
      />

      <DeletePostModal
        open={postToDelete !== null}
        post={postToDelete}
        onConfirm={() => postToDelete && deleteMutation.mutate(postToDelete.id)}
        onCancel={() => setPostToDelete(null)}
        loading={deleteMutation.isPending}
      />

      <PublishModal
        post={postToPublish}
        visible={postToPublish !== null}
        onClose={() => setPostToPublish(null)}
        workspaceId={workspaceId}
        workspace={workspace}
      />
    </div>
  )
}
