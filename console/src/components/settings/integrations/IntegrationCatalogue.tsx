import type { ReactNode } from 'react'
import { Modal } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { EmailProviderKind } from '../../../services/api/types'
import { emailProviders } from '../../integrations/EmailProviders'
import { zapierProvider } from '../../integrations/ZapierProviders'
import { Pill } from '../../layout'
import { IntegrationIcon } from './IntegrationCard'
import { isDeprecatedEmailProvider, transactionalEmailOnly } from './providerDetails'
import { DeprecatedPill } from './DeprecatedPill'

/** What the owner picked in the catalogue: the drawer to open. */
export type CatalogueSelection =
  | { type: 'email'; kind: EmailProviderKind }
  | { type: 'supabase' }
  | { type: 'zapier' }

interface CatalogueEntry {
  key: string
  icon: ReactNode
  name: string
  description: string
  /** Under the description, wrapping: a restriction. */
  pill?: ReactNode
  selection: CatalogueSelection
}

export interface IntegrationCatalogueProps {
  open: boolean
  onClose: () => void
  onSelect: (selection: CatalogueSelection) => void
}

// The order of the mockup: the most used first.
const EMAIL_ORDER: EmailProviderKind[] = ['ses', 'smtp', 'sparkpost', 'postmark', 'mailgun', 'mailjet', 'sendgrid']

/**
 * "Add integration": the email providers and the connected apps. The AI models and Firecrawl are
 * added from Settings › AI › Assistant.
 */
export function IntegrationCatalogue({ open, onClose, onSelect }: IntegrationCatalogueProps) {
  const { t } = useLingui()

  const emailDescriptions: Record<EmailProviderKind, string> = {
    ses: t`AWS keys, optional tenant isolation`,
    smtp: t`Any SMTP server, basic or OAuth2 login`,
    sparkpost: t`API key, US or EU`,
    postmark: t`Server token, message stream`,
    mailgun: t`Domain and API key, US or EU`,
    mailjet: t`API and secret keys`,
    sendgrid: t`API key`
  }

  const email: CatalogueEntry[] = EMAIL_ORDER.flatMap((kind) => {
    const provider = emailProviders.find((p) => p.kind === kind)
    if (!provider) return []
    return [
      {
        key: `email-${kind}`,
        icon: provider.getIcon('', 14),
        name: provider.name,
        description: emailDescriptions[kind],
        pill:
          transactionalEmailOnly.includes(kind) || isDeprecatedEmailProvider(kind) ? (
            <>
              {transactionalEmailOnly.includes(kind) ? (
                <Pill tone="neutral" dot={false}>
                  {t`Transactional only`}
                </Pill>
              ) : null}
              <DeprecatedPill kind={kind} />
            </>
          ) : undefined,
        selection: { type: 'email', kind }
      }
    ]
  })

  const apps: CatalogueEntry[] = [
    {
      key: 'supabase',
      icon: <img src="/console/supabase.png" alt="Supabase" className="h-3.5" />,
      name: 'Supabase',
      description: t`Send auth emails, add new users as contacts`,
      selection: { type: 'supabase' }
    },
    {
      key: 'zapier',
      icon: zapierProvider.getIcon('', 14),
      name: zapierProvider.name,
      description: t`Triggers and actions in your Zaps`,
      selection: { type: 'zapier' }
    }
  ]

  const group = (title: string, entries: CatalogueEntry[]) => (
    <section className="flex flex-col gap-2">
      <h3 data-unreset className="m-0 text-xs font-semibold tracking-wide text-ink-3 uppercase">
        {title}
      </h3>
      {entries.map((entry) => (
        <button
          key={entry.key}
          type="button"
          data-catalogue-entry={entry.key}
          onClick={() => onSelect(entry.selection)}
          className="flex w-full items-center gap-3 rounded-lg border border-line bg-surface px-3 py-2.5 text-left cursor-pointer hover:border-primary-300 focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none"
        >
          {/* A column as wide as the widest wordmark, so the names line up. */}
          <span className="flex w-[88px] shrink-0">
            <IntegrationIcon>{entry.icon}</IntegrationIcon>
          </span>
          <span className="flex min-w-0 flex-1 flex-col">
            <span className="text-sm font-medium text-ink">{entry.name}</span>
            <span className="text-xs text-ink-3">{entry.description}</span>
            {/* Under the text: two pills beside the description squeeze it to one word per line. */}
            {entry.pill ? <span className="mt-1 flex flex-wrap items-center gap-1.5">{entry.pill}</span> : null}
          </span>
        </button>
      ))}
    </section>
  )

  return (
    <Modal open={open} onCancel={onClose} footer={null} title={t`Add integration`} width={820} destroyOnHidden>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {group(t`Email sending`, email)}
        {group(t`Connected apps`, apps)}
      </div>
    </Modal>
  )
}
