import { useLingui } from '@lingui/react/macro'
import { Menu, Divider, Button, Space } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { blogCategoriesApi, BlogCategory } from '../../services/api/blog'
import { faPenToSquare, faTrashCan } from '@fortawesome/free-regular-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { MissingMetaTagsWarning } from '../seo/MissingMetaTagsWarning'

interface BlogSidebarProps {
  workspaceId: string
  activeCategoryId: string | null
  onCategoryChange: (categoryId: string | null) => void
  onNewCategory: () => void
  onEditCategory?: (category: BlogCategory) => void
  onDeleteCategory?: (category: BlogCategory) => void
}

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

  const handleEdit = (e: React.MouseEvent, category: BlogCategory) => {
    e.stopPropagation()
    onEditCategory?.(category)
  }

  const handleDelete = (e: React.MouseEvent, category: BlogCategory) => {
    e.stopPropagation()
    onDeleteCategory?.(category)
  }

  const menuItems = [
    {
      key: 'all',
      label: t`All Posts`
    },
    ...categories.map((category) => ({
      key: category.id,
      label: (
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            width: '100%'
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <span>{category.settings.name}</span>
            <MissingMetaTagsWarning seo={category.settings.seo} />
          </div>
          {(onEditCategory || onDeleteCategory) && (
            <Space size={4} onClick={(e) => e.stopPropagation()}>
              {onEditCategory && (
                <Button
                  type="text"
                  size="small"
                  icon={<FontAwesomeIcon icon={faPenToSquare} />}
                  onClick={(e) => handleEdit(e, category)}
                  style={{ padding: '0 4px', height: '20px', fontSize: '12px' }}
                />
              )}
              {onDeleteCategory && (
                <Button
                  type="text"
                  size="small"
                  icon={<FontAwesomeIcon icon={faTrashCan} />}
                  onClick={(e) => handleDelete(e, category)}
                  style={{ padding: '0 4px', height: '20px', fontSize: '12px' }}
                />
              )}
            </Space>
          )}
        </div>
      )
    }))
  ]

  const selectedKey = activeCategoryId || 'all'

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div className="text-xl font-medium pt-6 pl-6">{t`Categories`}</div>
      <Divider className="!my-4" />
      <Menu
        mode="inline"
        selectedKeys={[selectedKey]}
        items={menuItems}
        onClick={({ key }) => onCategoryChange(key === 'all' ? null : key)}
        style={{ borderRight: 0, backgroundColor: '#F9F9F9' }}
      />
      <Divider className="!my-4" />
      <div className="px-6 pb-6">
        <Button type="primary" ghost icon={<PlusOutlined />} onClick={onNewCategory} block>
          {t`New Category`}
        </Button>
      </div>
    </div>
  )
}
