import { useLingui } from '@lingui/react/macro'
import { useQuery } from '@tanstack/react-query'
import { Dropdown } from 'antd'
import { Folder, MoreHorizontal, Newspaper, Plus } from 'lucide-react'
import { blogCategoriesApi, type BlogCategory, type BlogPostCounts } from '../../services/api/blog'
import { MissingMetaTagsWarning } from '../seo/MissingMetaTagsWarning'
import { RailItem, RailSection } from '../layout'

interface BlogSidebarProps {
  workspaceId: string
  activeCategoryId: string | null
  /** Posts by category; absent while loading or after an error, so no number shows. */
  counts?: BlogPostCounts
  /** Blog write: create, edit and delete categories. */
  canManageCategories: boolean
  onCategoryChange: (categoryId: string | null) => void
  onNewCategory: () => void
  onEditCategory: (category: BlogCategory) => void
  onDeleteCategory: (category: BlogCategory) => void
}

const MENU_BUTTON =
  'grid size-5 cursor-pointer place-items-center rounded-sm border-0 bg-transparent text-ink-3 hover:bg-line-soft hover:text-ink focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none'

/** The Blog's left rail: all posts, then one entry per category with its count and menu. */
export function BlogSidebar({
  workspaceId,
  activeCategoryId,
  counts,
  canManageCategories,
  onCategoryChange,
  onNewCategory,
  onEditCategory,
  onDeleteCategory
}: BlogSidebarProps) {
  const { t } = useLingui()
  const { data: categoriesData, isSuccess } = useQuery({
    queryKey: ['blog-categories', workspaceId],
    queryFn: () => blogCategoriesApi.list(workspaceId)
  })

  const categories = categoriesData?.categories ?? []

  return (
    <div className="flex flex-col gap-px">
      <RailItem
        icon={<Newspaper size={14} />}
        label={t`All posts`}
        count={counts?.all_total}
        active={!activeCategoryId}
        onClick={() => onCategoryChange(null)}
      />
      <RailSection
        title={t`Categories`}
        action={
          canManageCategories
            ? { label: t`New category`, icon: <Plus size={14} />, onClick: onNewCategory }
            : undefined
        }
      >
        {isSuccess && categories.length === 0 ? (
          <span className="px-2 py-1 text-sm text-ink-3">{t`No categories yet`}</span>
        ) : null}
        {categories.map((category) => {
          const name = category.settings.name
          return (
            <RailItem
              key={category.id}
              icon={<Folder size={14} />}
              label={
                <span className="inline-flex max-w-full items-center gap-1.5">
                  <span className="truncate">{name}</span>
                  <MissingMetaTagsWarning seo={category.settings.seo} />
                </span>
              }
              count={counts ? (counts.by_category?.[category.id] ?? 0) : undefined}
              active={activeCategoryId === category.id}
              onClick={() => onCategoryChange(category.id)}
              trailingMode="actions"
              trailing={
                canManageCategories ? (
                  <Dropdown
                    trigger={['click']}
                    menu={{
                      items: [
                        { key: 'edit', label: t`Edit`, onClick: () => onEditCategory(category) },
                        {
                          key: 'delete',
                          label: t`Delete`,
                          danger: true,
                          onClick: () => onDeleteCategory(category)
                        }
                      ]
                    }}
                  >
                    <button
                      type="button"
                      aria-label={t`Actions for ${name}`}
                      className={MENU_BUTTON}
                    >
                      <MoreHorizontal size={14} aria-hidden="true" />
                    </button>
                  </Dropdown>
                ) : undefined
              }
            />
          )
        })}
      </RailSection>
    </div>
  )
}
