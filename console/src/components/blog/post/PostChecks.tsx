import { useLingui } from '@lingui/react/macro'
import { SeoChecks, type SeoCheck } from '../../seo/SeoChecks'

export interface PostChecksProps {
  /** From usePostChecks, which the drawer also reads for its step mark and summary. */
  checks: SeoCheck[]
  onShowField: (path: string[]) => void
}

/** The post's checks, in the Details step. */
export function PostChecks({ checks, onShowField }: PostChecksProps) {
  const { t } = useLingui()
  return <SeoChecks title={t`Search and share`} checks={checks} onShowField={onShowField} />
}
