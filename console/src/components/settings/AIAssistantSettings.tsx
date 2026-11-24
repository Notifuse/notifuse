import { useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Button, Dropdown, Table } from 'antd'
import type { TableColumnsType } from 'antd'
import { Link } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { Plus } from 'lucide-react'

import type { Integration, LLMProviderKind, Workspace } from '../../services/api/types'
import { LLMIntegration } from '../integrations/LLMIntegration'
import { getLLMProviderIcon, getLLMProviderName, llmProviders } from '../integrations/LLMProviders'
import { FirecrawlIntegration } from '../integrations/FirecrawlIntegration'
import { firecrawlProvider } from '../integrations/FirecrawlProviders'
import { Mono, Panel, Pill } from '../layout'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { IntegrationIcon } from './integrations/IntegrationCard'
import { IntegrationDrawer } from './integrations/IntegrationDrawer'
import { credentialHint } from './integrations/secrets'
import { formatConnectedOn, useIntegrationActions } from './integrations/useIntegrationActions'

interface AIAssistantSettingsProps {
  workspace: Workspace | null
  onSave: (updatedWorkspace: Workspace) => Promise<void>
  isOwner: boolean
}

type FormRef = { submit: () => void } | null

/** A page where the assistant appears, linked from the page. */
interface AssistantUse {
  key: string
  to:
    | '/console/workspace/$workspaceId/blog'
    | '/console/workspace/$workspaceId/templates'
    | '/console/workspace/$workspaceId/web-analytics'
  label: string
  description: string
}

/** The drawer on screen, and the integration it edits (none to add one). */
type OpenDrawer =
  | { type: 'llm'; kind: LLMProviderKind; integration?: Integration }
  | { type: 'firecrawl'; integration?: Integration }

/** Whether an AI model or Firecrawl integration has a key stored: the API accepts one without. */
const hasStoredKey = (integration: Integration): boolean => {
  if (integration.type === 'firecrawl') {
    return Boolean(
      credentialHint(integration, 'firecrawl.api_key') || integration.firecrawl_settings?.encrypted_api_key
    )
  }
  const provider = integration.llm_provider
  if (!provider) return false
  return Boolean(credentialHint(integration, `${provider.kind}.api_key`) || provider[provider.kind]?.encrypted_api_key)
}

/**
 * Settings › AI › Assistant: the AI models the assistant runs on, Firecrawl for its web tools, and
 * the pages where it appears. Only owners add, change or remove an integration.
 */
export function AIAssistantSettings({ workspace, onSave, isOwner }: AIAssistantSettingsProps) {
  const { t } = useLingui()
  const [drawer, setDrawer] = useState<OpenDrawer | null>(null)
  const { saving, saveIntegration, confirmDelete } = useIntegrationActions(workspace, onSave)
  const formRef = useRef<FormRef>(null)

  if (!workspace) {
    return null
  }

  const integrations = workspace.integrations ?? []
  const models = integrations.filter((integration) => integration.type === 'llm')
  // The server only uses the first Firecrawl integration (llm_service.go), so the page offers to
  // connect one only while there is none.
  const firecrawls = integrations.filter((integration) => integration.type === 'firecrawl')

  const closeDrawer = () => setDrawer(null)

  const providerDescriptions: Record<LLMProviderKind, string> = {
    anthropic: t`Claude models`,
    openai: t`Or any OpenAI-compatible endpoint`,
    gemini: t`Gemini models`
  }

  const noApiKey = <Pill tone="warning">{t`No API key`}</Pill>

  const rowActions = (integration: Integration, onEdit: (() => void) | null) =>
    isOwner ? (
      <span className="inline-flex gap-2">
        <Button size="small" onClick={onEdit ?? undefined} disabled={!onEdit}>
          {t`Edit`}
        </Button>
        <Button size="small" danger onClick={() => confirmDelete(integration.id)}>
          {t`Remove`}
        </Button>
      </span>
    ) : null

  const columns: TableColumnsType<Integration> = [
    {
      key: 'name',
      title: t`Name`,
      render: (_, integration) => (
        <span className="inline-flex flex-wrap items-center gap-2">
          <span className="text-ink">{integration.name}</span>
          {hasStoredKey(integration) ? null : noApiKey}
        </span>
      )
    },
    {
      key: 'provider',
      title: t`Provider`,
      render: (_, integration) => {
        const kind = integration.llm_provider?.kind
        if (!kind) return '—'
        return (
          <span className="inline-flex items-center gap-2">
            <span aria-hidden="true">
              <IntegrationIcon>{getLLMProviderIcon(kind, 14)}</IntegrationIcon>
            </span>
            {getLLMProviderName(kind)}
          </span>
        )
      }
    },
    {
      key: 'model',
      title: t`Model`,
      render: (_, integration) => {
        const provider = integration.llm_provider
        const model = provider ? provider[provider.kind]?.model : undefined
        return model ? <Mono>{model}</Mono> : t`Not configured`
      }
    },
    {
      key: 'connected',
      title: t`Connected`,
      render: (_, integration) => <span className="text-ink-3">{formatConnectedOn(integration.created_at)}</span>
    },
    ...(isOwner
      ? [
          {
            key: 'actions',
            align: 'right' as const,
            render: (_: unknown, integration: Integration) => {
              const kind = integration.llm_provider?.kind
              return rowActions(
                integration,
                kind ? () => setDrawer({ type: 'llm', kind, integration }) : null
              )
            }
          }
        ]
      : [])
  ]

  const addModel = isOwner ? (
    <Dropdown
      trigger={['click']}
      placement="bottomRight"
      menu={{
        items: llmProviders.map((provider) => ({
          key: provider.kind,
          label: (
            <span className="flex flex-col">
              <span className="text-sm text-ink">{provider.name}</span>
              <span className="text-xs text-ink-3">{providerDescriptions[provider.kind]}</span>
            </span>
          ),
          onClick: () => setDrawer({ type: 'llm', kind: provider.kind })
        }))
      }}
    >
      <Button icon={<Plus size={14} aria-hidden="true" />}>{t`Add a model`}</Button>
    </Dropdown>
  ) : null

  const firecrawlRow = (title: string, subtitle: ReactNode, actions: ReactNode, key?: string) => (
    <div
      key={key}
      data-firecrawl-row
      className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-line-soft px-3 py-2.5"
    >
      <span className="flex min-w-0 items-center gap-3">
        <span aria-hidden="true">
          <IntegrationIcon>{firecrawlProvider.getIcon('', 14)}</IntegrationIcon>
        </span>
        <span className="flex min-w-0 flex-col">
          <span className="text-sm font-medium text-ink">{title}</span>
          <span className="text-xs text-ink-3">{subtitle}</span>
        </span>
      </span>
      {actions}
    </div>
  )

  const firecrawlStatus =
    firecrawls.length === 0 ? (
      <Pill tone="neutral" dot={false}>
        {t`Not connected`}
      </Pill>
    ) : hasStoredKey(firecrawls[0]) ? (
      <Pill tone="success" dot={false}>
        {/* A status: the bare "Connected" is the label of a date in several languages. */}
        {t({ message: 'Connected', context: 'status' })}
      </Pill>
    ) : (
      noApiKey
    )

  const uses: AssistantUse[] = [
    {
      key: 'blog',
      to: '/console/workspace/$workspaceId/blog',
      label: t`Blog`,
      description: t`Drafts and edits posts`
    },
    {
      key: 'email-editor',
      to: '/console/workspace/$workspaceId/templates',
      label: t`Email editor`,
      description: t`Writes and edits templates`
    },
    {
      key: 'web-analytics',
      to: '/console/workspace/$workspaceId/web-analytics',
      label: t`Web Analytics`,
      description: t`Answers questions about your traffic`
    }
  ]

  const drawerTitle = (editing: boolean, name: string) =>
    editing ? t`Edit ${name} integration` : t`Add ${name} integration`

  const saveButton = (
    <Button type="primary" onClick={() => formRef.current?.submit()} loading={saving} disabled={!isOwner}>
      {t`Save`}
    </Button>
  )

  const renderDrawer = () => {
    if (!drawer) return null
    if (drawer.type === 'llm') {
      return (
        <IntegrationDrawer
          onClose={closeDrawer}
          title={drawerTitle(Boolean(drawer.integration), getLLMProviderName(drawer.kind))}
          topActions={saveButton}
          backLabel={t`AI assistant`}
        >
          <LLMIntegration
            integration={drawer.integration}
            workspace={workspace}
            providerKind={drawer.kind}
            onSave={(integration) =>
              saveIntegration('llm', integration, {
                editing: Boolean(drawer.integration),
                success: t`LLM integration saved successfully`,
                failure: t`Failed to save LLM integration`,
                onSaved: closeDrawer
              })
            }
            isOwner={isOwner}
            formRef={formRef}
          />
        </IntegrationDrawer>
      )
    }
    return (
      <IntegrationDrawer
        onClose={closeDrawer}
        title={drawerTitle(Boolean(drawer.integration), 'Firecrawl')}
        topActions={saveButton}
        backLabel={t`AI assistant`}
      >
        <FirecrawlIntegration
          integration={drawer.integration}
          workspace={workspace}
          onSave={(integration) =>
            saveIntegration('firecrawl', integration, {
              editing: Boolean(drawer.integration),
              success: t`Firecrawl integration saved successfully`,
              failure: t`Failed to save Firecrawl integration`,
              onSaved: closeDrawer
            })
          }
          isOwner={isOwner}
          formRef={formRef}
        />
      </IntegrationDrawer>
    )
  }

  return (
    <>
      <SettingsSectionHeader
        title={t`AI assistant`}
        description={t`The assistant in the blog, the email editor and Web Analytics.`}
      />

      <section data-ai-block="models">
        <Panel
          flush
          title={t`AI models`}
          extra={addModel}
          footer={t`Works with Anthropic, OpenAI or an OpenAI-compatible endpoint, and Gemini. Only owners can add, change or remove a model.`}
        >
          <p className="m-0 px-3.5 pt-3 pb-2 text-xs text-ink-3">
            {t`The assistant runs on a model you connect with your own provider key. If you connect more than one, people pick the model in the assistant.`}
          </p>
          <Table<Integration>
            rowKey="id"
            size="middle"
            columns={columns}
            dataSource={models}
            pagination={false}
            locale={{ emptyText: t`No model yet` }}
          />
        </Panel>
      </section>

      <section data-ai-block="web-access">
        <Panel title={t`Web access`} extra={firecrawlStatus}>
          <div className="flex flex-col gap-2.5">
            <p className="m-0 text-xs text-ink-3">
              {t`Optional: lets the assistant read a web page you give it and search the web.`}
            </p>
            {firecrawls.length === 0
              ? firecrawlRow(
                  firecrawlProvider.name,
                  t`Your Firecrawl key, or your own Firecrawl server`,
                  isOwner ? (
                    <Button onClick={() => setDrawer({ type: 'firecrawl' })}>{t`Connect`}</Button>
                  ) : null
                )
              : firecrawls.map((integration) =>
                  firecrawlRow(
                    integration.name,
                    <Mono>{integration.firecrawl_settings?.base_url || 'api.firecrawl.dev'}</Mono>,
                    rowActions(integration, () => setDrawer({ type: 'firecrawl', integration })),
                    integration.id
                  )
                )}
          </div>
        </Panel>
      </section>

      <section data-ai-block="uses">
        <Panel title={t`Where the assistant appears`}>
          <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-3">
            {uses.map((use) => (
              <Link
                key={use.key}
                to={use.to}
                params={{ workspaceId: workspace.id }}
                className="flex flex-col gap-0.5 rounded-md border border-line-soft px-3 py-2.5 hover:border-primary-300"
              >
                <span className="text-sm font-medium text-ink">{use.label}</span>
                <span className="text-xs text-ink-3">{use.description}</span>
              </Link>
            ))}
          </div>
        </Panel>
      </section>

      {renderDrawer()}
    </>
  )
}
