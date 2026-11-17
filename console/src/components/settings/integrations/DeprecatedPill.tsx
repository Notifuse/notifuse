import { Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { EmailProviderKind } from '../../../services/api/types'
import { Pill } from '../../layout'
import { isDeprecatedEmailProvider, type DeprecatedEmailProviderKind } from './providerDetails'

/** "Deprecated" on a provider that will be removed, with the reason in a tooltip; nothing for the others. */
export function DeprecatedPill({ kind }: { kind: EmailProviderKind }) {
  const { t } = useLingui()
  if (!isDeprecatedEmailProvider(kind)) return null
  // One reason per deprecated provider: adding one to the list without its reason does not compile
  const reasons: Record<DeprecatedEmailProviderKind, string> = {
    sparkpost: t`This integration will be removed soon: SparkPost no longer accepts new customers.`,
    mailjet: t`This integration will be removed soon: Mailjet does not allow sending marketing emails through its API.`
  }
  const reason = reasons[kind]
  return (
    <Tooltip title={reason}>
      <span className="inline-flex">
        <Pill tone="warning" dot={false}>
          {t`Deprecated`}
        </Pill>
      </span>
    </Tooltip>
  )
}
