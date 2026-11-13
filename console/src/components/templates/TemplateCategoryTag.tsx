import { Tag } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { TEMPLATE_CATEGORY_LABELS, isTemplateCategory } from './templateCategories'

/** A template category as a coloured tag, with its translated label. */
export function TemplateCategoryTag({ category }: { category: string }) {
  const { i18n } = useLingui()
  let color = 'default'
  if (['marketing', 'transactional'].includes(category)) {
    color = 'green'
  } else if (category === 'welcome') {
    color = 'blue'
  } else if (['opt_in', 'unsubscribe', 'bounce', 'blocklist'].includes(category)) {
    color = 'purple'
  }
  // A category the console does not know yet keeps its raw value rather than vanishing.
  const label = isTemplateCategory(category) ? i18n._(TEMPLATE_CATEGORY_LABELS[category]) : category
  return (
    <Tag variant="filled" color={color}>
      {label}
    </Tag>
  )
}
