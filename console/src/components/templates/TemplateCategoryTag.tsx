import { useLingui } from '@lingui/react/macro'
import { Pill } from '../layout'
import { TEMPLATE_CATEGORY_LABELS, isTemplateCategory } from './templateCategories'

/**
 * A template category as a pill, with its translated label: the same colours on every screen, the
 * Templates list's (marketing stands out, the others are neutral). It used to be an antd tag in
 * other colours outside the list.
 */
export function TemplateCategoryTag({ category }: { category: string }) {
  const { i18n } = useLingui()
  // A category the console does not know yet keeps its raw value rather than vanishing.
  const label = isTemplateCategory(category) ? i18n._(TEMPLATE_CATEGORY_LABELS[category]) : category
  return (
    <Pill tone={category === 'marketing' ? 'primary' : 'neutral'} dot={false}>
      {label}
    </Pill>
  )
}
