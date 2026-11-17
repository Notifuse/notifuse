import type { ReactNode } from 'react'
import { Modal } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { EmailProviderKind, LLMProviderKind } from '../../../services/api/types'
import { emailProviders } from '../../integrations/EmailProviders'
import { llmProviders } from '../../integrations/LLMProviders'
import { firecrawlProvider } from '../../integrations/FirecrawlProviders'
import { zapierProvider } from '../../integrations/ZapierProviders'
import { Pill } from '../../layout'
import { IntegrationIcon } from './IntegrationCard'
import { transactionalEmailOnly } from './providerDetails'

/** What the owner picked in the catalogue: the drawer to open. */
export type CatalogueSelection =
  | { type: 'email'; kind: EmailProviderKind }
  | { type: 'llm'; kind: LLMProviderKind }
  | { type: 'firecrawl' }
  | { type: 'supabase' }
  | { type: 'zapier' }

interface CatalogueEntry {
  key: string
  icon: ReactNode
  name: string
  description: string
  /** On the right of the entry: a restriction, or why it cannot be picked. */
  pill?: ReactNode
  /** Shown but not clickable. */
  disabled?: boolean
  selection: CatalogueSelection
}

export interface IntegrationCatalogueProps {
  open: boolean
  onClose: () => void
  onSelect: (selection: CatalogueSelection) => void
  /**
   * The server only ever uses the first Firecrawl integration (llm_service.go), so a second one
   * would never be used: once one exists the entry says so instead of opening a form.
   */
  firecrawlConfigured: boolean
}

// The order of the mockup: the most used first.
const EMAIL_ORDER: EmailProviderKind[] = ['ses', 'smtp', 'sparkpost', 'postmark', 'mailgun', 'mailjet', 'sendgrid']

/** "Add integration": every integration the workspace can add, grouped by what it is used for. */
export function IntegrationCatalogue({ open, onClose, onSelect, firecrawlConfigured }: IntegrationCatalogueProps) {
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
  const llmDescriptions: Record<LLMProviderKind, string> = {
    anthropic: t`Claude models`,
    openai: t`Or any OpenAI-compatible endpoint`,
    gemini: t`Gemini models`
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
        pill: transactionalEmailOnly.includes(kind) ? (
          <Pill tone="neutral" dot={false}>
            {t`Transactional only`}
          </Pill>
        ) : undefined,
        selection: { type: 'email', kind }
      }
    ]
  })

  const ai: CatalogueEntry[] = [
    ...llmProviders.map((provider) => ({
      key: `llm-${provider.kind}`,
      icon: provider.getIcon('', 14),
      name: provider.name,
      description: llmDescriptions[provider.kind],
      selection: { type: 'llm', kind: provider.kind } as CatalogueSelection
    })),
    {
      key: 'firecrawl',
      icon: firecrawlProvider.getIcon('', 14),
      name: firecrawlProvider.name,
      description: t`Lets the assistant read web pages and search`,
      pill: firecrawlConfigured ? (
        <Pill tone="success" dot={false}>
          {t`Configured`}
        </Pill>
      ) : undefined,
      disabled: firecrawlConfigured,
      selection: { type: 'firecrawl' }
    }
  ]

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
          disabled={entry.disabled}
          onClick={() => onSelect(entry.selection)}
          className="flex w-full items-center gap-3 rounded-lg border border-line bg-surface px-3 py-2.5 text-left enabled:cursor-pointer enabled:hover:border-primary-300 focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none disabled:cursor-default"
        >
          {/* A column as wide as the widest wordmark, so the names line up. */}
          <span className="flex w-[88px] shrink-0">
            <IntegrationIcon>{entry.icon}</IntegrationIcon>
          </span>
          <span className="flex min-w-0 flex-1 flex-col">
            <span className="text-sm font-medium text-ink">{entry.name}</span>
            <span className="text-xs text-ink-3">{entry.description}</span>
          </span>
          {entry.pill}
        </button>
      ))}
    </section>
  )

  return (
    <Modal open={open} onCancel={onClose} footer={null} title={t`Add integration`} width={820} destroyOnHidden>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {group(t`Email sending`, email)}
        <div className="flex flex-col gap-4">
          {group(t`AI assistant`, ai)}
          {group(t`Connected apps`, apps)}
        </div>
      </div>
    </Modal>
  )
}
