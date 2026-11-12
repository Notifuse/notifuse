import { plural } from '@lingui/core/macro'
import {
  TEMPLATE_USAGE_TYPES,
  type TemplateUsageCounts,
  type TemplateUsageReference,
  type TemplateUsageType
} from '../../services/api/template'

// Each label is a whole message per type, so every language counts with its own rules.
function countLabel(type: TemplateUsageType, count: number): string {
  switch (type) {
    case 'broadcasts':
      return plural(count, { one: '# newsletter', other: '# newsletters' })
    case 'transactional_notifications':
      return plural(count, { one: '# notification', other: '# notifications' })
    case 'automations':
      return plural(count, { one: '# automation', other: '# automations' })
    case 'lists':
      return plural(count, { one: '# list', other: '# lists' })
  }
}

/** The reference type of each usage type. */
const REFERENCE_TYPE: Record<TemplateUsageType, TemplateUsageReference['type']> = {
  broadcasts: 'broadcast',
  transactional_notifications: 'transactional_notification',
  automations: 'automation',
  lists: 'list'
}

export interface UsageSummary {
  total: number
  /** Null when nothing uses the template. */
  label: string | null
  /** One line per type used, in the order of the response. */
  details: string[]
}

/** "2 newsletters" for one type, "5 uses" with a line per type for several, null for none. */
export function summarizeUsage(counts: TemplateUsageCounts | undefined): UsageSummary {
  const used = TEMPLATE_USAGE_TYPES.filter((type) => (counts?.[type] ?? 0) > 0)
  if (used.length === 0) return { total: 0, label: null, details: [] }
  const details = used.map((type) => countLabel(type, counts?.[type] ?? 0))
  const total = used.reduce((sum, type) => sum + (counts?.[type] ?? 0), 0)
  return {
    total,
    label: used.length === 1 ? details[0] : plural(total, { one: '# use', other: '# uses' }),
    details
  }
}

export interface ReferenceGroup {
  type: TemplateUsageType
  items: TemplateUsageReference[]
  /** How many more use the template than the items shown, from the exact count. */
  more: number
}

/**
 * The references of one template grouped by type, in the order of the response, with at most
 * `limit` per type. The server returns at most 50 per type while its counts stay exact, so what
 * is left over is computed from the counts.
 */
export function groupReferences(
  references: TemplateUsageReference[],
  counts: TemplateUsageCounts | undefined,
  limit: number
): ReferenceGroup[] {
  return TEMPLATE_USAGE_TYPES.flatMap((type) => {
    const all = references.filter((ref) => ref.type === REFERENCE_TYPE[type])
    if (all.length === 0) return []
    const items = all.slice(0, limit)
    const total = Math.max(counts?.[type] ?? 0, all.length)
    return [{ type, items, more: total - items.length }]
  })
}
