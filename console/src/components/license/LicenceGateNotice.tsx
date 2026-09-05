import { useLingui } from '@lingui/react/macro'
import { useLicense } from '../../hooks/useLicense'
import { LicenceBlock } from './LicenceBlock'
import { LICENSE_REQUIRED_TIER, type LicenseFeature } from '../../types/license'

interface LicenceGateNoticeProps {
  feature: LicenseFeature
  /** The workspace the block is shown in, which is where its one button routes. */
  workspaceId?: string
  className?: string
}

/**
 * Shown above a control the deployment is not licensed for, BEFORE it is pressed.
 *
 * The backend still refuses with 402, and client.ts still turns that into a sentence — this is
 * not a gate, and it never was one: has() answers true for an unknown licence, so a deployment
 * that has paid is never told otherwise because the console was not told anything. What this
 * adds is the order of events. An operator evaluating Notifuse against the price list used to
 * meet a licensed capability as an error after pressing Save, which reads as a bug report; a
 * locked control that names the capability and the plan reads as an offer.
 *
 * It is a block above the control and not a blur over the form, for two reasons the call sites
 * depend on. A
 * deployment whose licence lapsed still owns the custom roles and translations it made, and the
 * backend lets it keep and remove them; a veil over the whole form would hide what is theirs
 * and contradict that promise. And the wording leads with what still works, the same way
 * SsoLicenceNotice does, because the state being described is a control that is locked, not a
 * product that is broken.
 *
 * One button, to Settings › Licence, for everyone. That page already offers root the key box
 * and the price list, and already tells a member who to ask — so the block repeats neither, and
 * never shows a "Buy" button to someone who cannot install what they would buy.
 *
 * It lives in this directory so that src/i18n/locales/catalogues.test.ts exempts its wording
 * from the eight-language gate: licence prose is expected to change once real operators have
 * read it. English and French, like the rest of the licence surface.
 */
export function LicenceGateNotice({
  feature,
  workspaceId,
  className = 'mb-4'
}: LicenceGateNoticeProps) {
  const { t } = useLingui()
  const { has } = useLicense()

  if (has(feature)) return null

  const tier = LICENSE_REQUIRED_TIER[feature]

  // One sentence per capability rather than a template, so a plural subject gets a plural verb.
  const titles: Record<LicenseFeature, string> = {
    rbac: t`Custom permissions require a Notifuse ${tier} licence.`,
    ses_tenant: t`SES tenant isolation requires a Notifuse ${tier} licence.`,
    sso: t`Single sign-on requires a Notifuse ${tier} licence.`,
    audit_logs: t`Audit logs require a Notifuse ${tier} licence.`,
    template_i18n: t`Template translations require a Notifuse ${tier} licence.`
  }

  // What the deployment keeps. Each mirrors the rule the backend actually enforces: a full
  // grant is never gated, a saved translation keeps being sent and can be switched off, and a
  // tenant that exists keeps resolving with no key at all.
  const stillWorks: Record<LicenseFeature, string> = {
    rbac: t`Every member keeps the access they have, and any member can still be given full access. Only a narrower grant needs a licence.`,
    ses_tenant: t`Sending through this integration is unaffected, and a tenant that already exists keeps being used. Switching isolation on needs a licence.`,
    sso: t`Everyone signs in with a login code in the meantime; nobody is locked out.`,
    audit_logs: t`This deployment keeps working without them.`,
    template_i18n: t`Translations already saved keep being sent, and switching one off is always allowed. Adding or editing one needs a licence.`
  }

  return (
    <LicenceBlock
      title={titles[feature]}
      description={stillWorks[feature]}
      workspaceId={workspaceId}
      className={className}
    />
  )
}
