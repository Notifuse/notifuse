import { PageHeader } from '../layout'

interface SettingsSectionHeaderProps {
  title: string
  description: string
  /** `page` (default): the title of a settings page, an h1. `section`: a part of one, an h2 under a rule. */
  level?: 'page' | 'section'
  className?: string
}

/** The title and description of a settings page, or of a section inside one. */
export function SettingsSectionHeader({ title, description, level = 'page', className }: SettingsSectionHeaderProps) {
  const classes = [level === 'section' ? 'border-t border-line-soft pt-4' : '', className ?? '']
    .filter(Boolean)
    .join(' ')
  return <PageHeader title={title} description={description} level={level} className={classes || undefined} />
}
