import { Alert } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useLicense } from '../../hooks/useLicense'
import { LICENSE_PRICING_URL } from '../../types/license'

/**
 * Told to an operator who has switched single sign-on on without a licence that covers it.
 *
 * SSO is the one licensed capability with no control of its own to refuse. The other three
 * answer a 402 the moment somebody presses the button, and the console explains the refusal on
 * the spot; the SSO gate simply stops offering the button on the sign-in page. That is
 * deliberate — the sign-in page is unauthenticated, and an error there naming a licence would
 * publish this deployment's commercial status to anyone who loaded it — but it leaves an
 * operator who just switched SSO on watching nothing happen at all.
 *
 * So it is said here, in the drawer where they switched it on, to the only audience that can
 * act on it. It leads with what still works, because the failure being described is a button
 * that is missing, not a login that is broken: magic-code sign-in is unconditional, every SSO
 * account carries a verified email address, and a session already minted is unaffected.
 *
 * It lives in this directory rather than inline in the drawer because
 * src/i18n/locales/catalogues.test.ts exempts src/components/license/ from the eight-language
 * gate by path: the licence wording is expected to change once real operators have read it, and
 * translating it eight times before it settles pays for it twice. English and French, like the
 * rest of the licence surface.
 */
export function SsoLicenceNotice({ oidcEnabled }: { oidcEnabled: boolean }) {
  const { t } = useLingui()
  const { has } = useLicense()

  // has() answers true for an unknown licence, the way every advisory read in this console
  // does: a deployment that has paid must never be told it has not because the console was
  // not told anything.
  if (!oidcEnabled || has('sso')) return null

  return (
    <Alert
      className="!mb-4"
      type="warning"
      showIcon
      title={t`Single sign-on is switched on, and this deployment's licence does not include it.`}
      description={
        <>
          {t`The SSO button stays hidden on the sign-in page until a licence covering it is installed. Everyone signs in with a login code in the meantime — nobody is locked out, and no session was ended.`}{' '}
          <a href={LICENSE_PRICING_URL} target="_blank" rel="noopener noreferrer">
            {t`Buy a licence`}
          </a>
        </>
      }
    />
  )
}
