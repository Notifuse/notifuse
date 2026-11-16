import { useLingui } from '@lingui/react/macro'
import { SeoChecks, type SeoCheck } from '../../seo/SeoChecks'
import { useSeoGapChecks } from '../../seo/useSeoGapChecks'
import { seoGaps } from '../../seo/seoGaps'
import type { CategoryDraft, EffectiveCategorySeo } from './categorySeo'

export interface CategoryChecksProps {
  draft: CategoryDraft
  effective: EffectiveCategorySeo
  /** The category exists: the checks read as its state rather than as a pre-flight. */
  editing: boolean
  /** Brings the field of a check into view and focuses it. */
  onShowField: (path: string[]) => void
}

/**
 * The category's checks: its name and slug, then each SEO tag it leaves empty (the rule of the
 * sidebar's warning), with what the page shows instead.
 */
export function CategoryChecks({ draft, effective, editing, onShowField }: CategoryChecksProps) {
  const { t } = useLingui()
  const gapCheck = useSeoGapChecks(effective, t`Uses the blog's share image`)

  const named = !!draft.name?.trim() && !!draft.slug?.trim()
  const checks: SeoCheck[] = [
    named
      ? { key: 'name', ok: true, label: t`Name and slug` }
      : { key: 'name', ok: false, label: t`Name and slug required`, field: ['name'] },
    ...seoGaps(draft.seo).map((gap) => ({ key: gap, ok: false, ...gapCheck[gap] }))
  ]

  return (
    <SeoChecks title={editing ? t`Search and share` : t`Before you create`} checks={checks} onShowField={onShowField} />
  )
}
