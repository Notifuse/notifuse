import { useLingui } from '@lingui/react/macro'
import { useQuery } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { blogCategoriesApi, BlogCategory } from '../../services/api/blog'
import { MissingMetaTagsWarning } from '../seo/MissingMetaTagsWarning'
import { RailItem, RailSection } from '../layout'

interface BlogSidebarProps {
  workspaceId: string
  activeCategoryId: string | null
  onCategoryChange: (categoryId: string | null) => void
  onNewCategory: () => void
  onEditCategory?: (category: BlogCategory) => void
  onDeleteCategory?: (category: BlogCategory) => void
}

const ACTION =
  'grid size-5 cursor-pointer place-items-center rounded-sm border-0 bg-transparent text-ink-3 hover:bg-line-soft hover:text-ink focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none'

/** The Blog's left rail: all posts, then one entry per category with its edit and delete actions. */
export function BlogSidebar({
  workspaceId,
  activeCategoryId,
  onCategoryChange,
  onNewCategory,
  onEditCategory,
  onDeleteCategory
}: BlogSidebarProps) {
  const { t } = useLingui()
  const { data: categoriesData } = useQuery({
    queryKey: ['blog-categories', workspaceId],
    queryFn: () => blogCategoriesApi.list(workspaceId)
  })

  const categories = categoriesData?.categories ?? []

  return (
    <div className="flex flex-col gap-px">
      <RailItem label={t`All Posts`} active={!activeCategoryId} onClick={() => onCategoryChange(null)} />
      <RailSection title={t`Categories`} action={{ label: t`New Category`, icon: <Plus size={14} />, onClick: onNewCategory }}>
        {categories.map((category) => {
          const name = category.settings.name
          return (
            <RailItem
              key={category.id}
              label={
                <span className="inline-flex max-w-full items-center gap-1.5">
                  <span className="truncate">{name}</span>
                  <MissingMetaTagsWarning seo={category.settings.seo} />
                </span>
              }
              active={activeCategoryId === category.id}
              onClick={() => onCategoryChange(category.id)}
              trailingMode="actions"
              trailing={
                onEditCategory || onDeleteCategory ? (
                  <>
                    {onEditCategory ? (
                      <button
                        type="button"
                        aria-label={t`Edit ${name}`}
                        title={t`Edit`}
                        className={ACTION}
                        onClick={() => onEditCategory(category)}
                      >
                        <Pencil size={12} aria-hidden="true" />
                      </button>
                    ) : null}
                    {onDeleteCategory ? (
                      <button
                        type="button"
                        aria-label={t`Delete ${name}`}
                        title={t`Delete`}
                        className={ACTION}
                        onClick={() => onDeleteCategory(category)}
                      >
                        <Trash2 size={12} aria-hidden="true" />
                      </button>
                    ) : null}
                  </>
                ) : undefined
              }
            />
          )
        })}
      </RailSection>
    </div>
  )
}
