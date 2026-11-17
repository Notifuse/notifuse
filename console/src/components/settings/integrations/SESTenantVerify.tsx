import { Alert } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Mono } from '../../layout'
import type { SESTenantVerifyOutcome } from './useSESTenantVerify'

/** What the check found, under the tenant field. */
export function SESTenantVerifyResult({ outcome }: { outcome: SESTenantVerifyOutcome | null }) {
  const { t } = useLingui()
  if (!outcome) return null

  if ('denied' in outcome) {
    return (
      <Alert
        type="warning"
        showIcon
        title={t`These AWS credentials cannot read SES tenants (needs ses:GetTenant and ses:ListTenantResources).`}
      />
    )
  }
  if ('error' in outcome) return <Alert type="error" showIcon title={outcome.error} />

  const { verification } = outcome
  const name = verification.tenant_name
  const set = verification.configuration_set_name
  if (!verification.exists) {
    return <Alert type="error" showIcon title={t`Tenant ${name} does not exist in this AWS account and region.`} />
  }

  const accountWide =
    verification.suppression_scope === 'ACCOUNT' ? t`Its bounces still go to the account-wide suppression list.` : null
  const paused = verification.sending_status === 'DISABLED' ? t`SES paused sending for this tenant.` : null
  const notes = [paused, accountWide].filter(Boolean).join(' ')

  if (set && !verification.configuration_set_associated) {
    return (
      <Alert
        type="error"
        showIcon
        title={t`Configuration set ${set} is not associated with tenant ${name}: every send would fail.`}
        description={
          verification.fix_command || notes ? (
            <div className="flex flex-col gap-1">
              {notes ? <span>{notes}</span> : null}
              {verification.fix_command ? <Mono copy>{verification.fix_command}</Mono> : null}
            </div>
          ) : undefined
        }
      />
    )
  }

  return (
    <Alert
      type={paused ? 'warning' : set ? 'success' : 'info'}
      showIcon
      title={
        set
          ? t`Tenant ${name} exists and configuration set ${set} is associated with it.`
          : t`Tenant ${name} exists. No configuration set to check it against yet: register webhooks or name one.`
      }
      description={notes || undefined}
    />
  )
}
