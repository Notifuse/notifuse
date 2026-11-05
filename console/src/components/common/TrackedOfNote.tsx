import { useLingui } from '@lingui/react/macro'

/** Under an open or click rate: how many emails the rate is computed over. */
export function TrackedOfNote({ tracked }: { tracked: number }) {
  const { t } = useLingui()
  return <div className="text-xs text-gray-500">{t`of ${tracked} tracked`}</div>
}
