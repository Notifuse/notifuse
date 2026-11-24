import { useState } from 'react'
import type { ReactNode } from 'react'
import { Alert, App, Button, Switch, Table, Tabs } from 'antd'
import type { TableColumnsType } from 'antd'
import { Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Trans, useLingui } from '@lingui/react/macro'
import { Copy } from 'lucide-react'

import type { Workspace, WorkspaceMember } from '../../services/api/types'
import { ApiError } from '../../services/api/errors'
import {
  isKnownOAuthClient,
  mcpApi,
  OAUTH_GRANT_NOT_FOUND,
  oauthClientHost,
  workspaceGrantsQuery,
  type OAuthGrant
} from '../../services/api/mcp'
import { copyToClipboard } from '../../lib/copyToClipboard'
import { useLicense } from '../../hooks/useLicense'
import { Mono, Panel, Pill, useRelativeDate } from '../layout'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { SettingsRow, SettingsRows } from './SettingsRow'
import { AccessCell } from './WorkspaceMembers'

interface MCPSettingsProps {
  workspace: Workspace | null
  /** The workspace's members and API keys, for the access of each connection's key. */
  members: WorkspaceMember[]
  isOwner: boolean
  onWorkspaceUpdate: (workspace: Workspace) => Promise<void>
}

type MCPClient = 'claude-ai' | 'chatgpt' | 'claude-code' | 'cursor'

/** The name of an API key: createAPIKey builds its address from the name typed. */
const keyName = (email: string) => email.split('@')[0]

const claudeCodeCommand = (url: string) =>
  `claude mcp add --transport http notifuse ${url} \\\n  --header "Authorization: Bearer YOUR_API_KEY"`

const cursorConfig = (url: string) =>
  JSON.stringify({ mcpServers: { notifuse: { url, headers: { Authorization: 'Bearer YOUR_API_KEY' } } } }, null, 2)

/** A link to Settings › Team & API keys, where the keys are created. */
function TeamLink({ workspaceId, children }: { workspaceId: string; children: ReactNode }) {
  return (
    <Link
      to="/console/workspace/$workspaceId/settings/$section"
      params={{ workspaceId, section: 'team' }}
      className="text-primary-600 hover:text-primary-700"
    >
      {children}
    </Link>
  )
}

/** A code block with its copy button in the corner. */
function CodeBlock({ code, copyLabel, onCopy }: { code: string; copyLabel: string; onCopy: (text: string) => void }) {
  const { t } = useLingui()
  return (
    <div className="relative">
      <pre className="m-0 rounded-md border border-line-soft bg-subtle px-3.5 py-3 pr-20 font-mono text-xs break-all whitespace-pre-wrap text-ink-2">
        {code}
      </pre>
      <div className="absolute top-2 right-2">
        <Button size="small" aria-label={copyLabel} onClick={() => onCopy(code)}>
          {t`Copy`}
        </Button>
      </div>
    </div>
  )
}

/**
 * Settings › AI › MCP: the address assistants connect to, how each client connects, the owner
 * switch, and for owners the connections made through the OAuth connection page.
 */
export function MCPSettings({ workspace, members, isOwner, onWorkspaceUpdate }: MCPSettingsProps) {
  const { t, i18n } = useLingui()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()
  const relativeDate = useRelativeDate()
  const [client, setClient] = useState<MCPClient>('claude-ai')
  // Without custom permissions every key has full access; with them, only some keys do.
  const customPermissions = useLicense().has('rbac')

  const instanceEnabled = window.MCP_ENABLED === true
  const mcpUrl = window.MCP_URL ?? ''
  const oauthUnavailable = Boolean(window.MCP_OAUTH_UNAVAILABLE_REASON)
  const workspaceId = workspace?.id ?? ''

  const grants = useQuery({
    ...workspaceGrantsQuery(workspaceId),
    enabled: Boolean(workspace) && isOwner && instanceEnabled,
    retry: false
  })

  const toggle = useMutation({
    mutationFn: (enabled: boolean) => mcpApi.setMCPSettings({ workspace_id: workspaceId, disabled: !enabled }),
    onSuccess: async (response) => {
      if (!workspace) return
      await onWorkspaceUpdate({ ...workspace, settings: { ...workspace.settings, mcp: response.mcp } })
      message.success(
        response.mcp.disabled ? t`MCP turned off for this workspace` : t`MCP turned on for this workspace`
      )
    },
    onError: (error) => {
      message.error(error instanceof Error && error.message ? error.message : t`Failed to change MCP access`)
    }
  })

  const revoke = useMutation({
    mutationFn: (grant: OAuthGrant) => mcpApi.revokeGrant({ grant_id: grant.id }),
    onSuccess: () => {
      message.success(t`Connection revoked`)
    },
    onError: (error) => {
      if (error instanceof ApiError && error.status === 404 && error.message === OAUTH_GRANT_NOT_FOUND) {
        message.info(t`This connection no longer exists`)
        return
      }
      message.error(error instanceof Error && error.message ? error.message : t`Failed to revoke the connection`)
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: workspaceGrantsQuery(workspaceId).queryKey })
  })

  if (!workspace) return null

  if (!instanceEnabled) {
    return (
      <>
        <SettingsSectionHeader
          title={t`MCP`}
          description={t`Let an AI assistant such as Claude, ChatGPT or Cursor read and work in this workspace through the Model Context Protocol.`}
        />
        <Panel>
          <Alert
            type="info"
            showIcon
            title={
              window.MCP_DISABLED_REASON === 'demo' ? (
                t`MCP is not available on the demo.`
              ) : (
                <Trans>
                  MCP is turned off on this instance. The administrator enables it with <Mono>MCP_ENABLED=true</Mono>.
                </Trans>
              )
            }
          />
        </Panel>
      </>
    )
  }

  const enabled = !workspace.settings.mcp?.disabled

  const copy = async (text: string, success: string) => {
    if (await copyToClipboard(text)) message.success(success)
    else message.error(t`Failed to copy to clipboard`)
  }

  const switchHelp = !isOwner
    ? t`Only an owner can change this.`
    : enabled
      ? t`When off, no assistant reaches this workspace. Nothing is revoked.`
      : t`When off, no assistant reaches this workspace, whatever key it uses.`

  const url = mcpUrl || '-'

  const steps: Record<MCPClient, ReactNode[]> = {
    'claude-ai': [
      <Trans key="1">In claude.ai, open the connector settings and add a custom connector.</Trans>,
      <Trans key="2">
        Paste <Mono>{url}</Mono> as its URL and select <strong>Connect</strong>.
      </Trans>,
      <Trans key="3">Notifuse opens and asks for an API key. Paste the key you created for Claude.</Trans>
    ],
    chatgpt: [
      <Trans key="1">In ChatGPT, open the connector settings and create a connector.</Trans>,
      <Trans key="2">
        Paste <Mono>{url}</Mono> as its URL and choose OAuth as its authentication.
      </Trans>,
      <Trans key="3">Notifuse opens and asks for an API key. Paste the key you created for ChatGPT.</Trans>
    ],
    'claude-code': [
      <Trans key="1">
        Create an API key for Claude Code in <TeamLink workspaceId={workspace.id}>Team & API keys</TeamLink>.
      </Trans>,
      <Trans key="2">
        Run the command below in a terminal, with the key in place of <Mono>YOUR_API_KEY</Mono>.
      </Trans>,
      <Trans key="3">
        In Claude Code, run <Mono>/mcp</Mono> to check that notifuse is connected.
      </Trans>
    ],
    cursor: [
      <Trans key="1">
        Create an API key for Cursor in <TeamLink workspaceId={workspace.id}>Team & API keys</TeamLink>.
      </Trans>,
      <Trans key="2">
        Add the server below to <Mono>.cursor/mcp.json</Mono> in your project, or to <Mono>~/.cursor/mcp.json</Mono> for
        every project, with the key in place of <Mono>YOUR_API_KEY</Mono>.
      </Trans>,
      <Trans key="3">In the Cursor settings, check that notifuse is listed under MCP.</Trans>
    ]
  }

  const confirmRevoke = (grant: OAuthGrant) => {
    const application = grant.client_name
    const key = keyName(grant.api_key_email)
    modal.confirm({
      title: t`Revoke this connection?`,
      content: t`${application} loses the access it has through the key ${key}. The key keeps working.`,
      okText: t`Revoke`,
      okButtonProps: { danger: true },
      cancelText: t`Cancel`,
      onOk: () => revoke.mutateAsync(grant).catch(() => undefined)
    })
  }

  const shortDate = (value: string) =>
    new Intl.DateTimeFormat(i18n.locale, { dateStyle: 'medium' }).format(new Date(value))

  const membersById = new Map(members.map((member) => [member.user_id, member]))

  const columns: TableColumnsType<OAuthGrant> = [
    {
      key: 'key',
      title: t`API key`,
      render: (_, grant) => <Mono>{keyName(grant.api_key_email)}</Mono>
    },
    {
      key: 'application',
      title: t`Application`,
      render: (_, grant) => (
        <span className="flex flex-col">
          <span className="inline-flex items-center gap-1.5">
            <span className="font-medium text-ink">{grant.client_name}</span>
            {isKnownOAuthClient(grant.client_id) ? null : (
              <Pill tone="warning" dot={false}>
                {t`Unknown`}
              </Pill>
            )}
          </span>
          <span className="text-xs text-ink-3">{oauthClientHost(grant.client_id)}</span>
        </span>
      )
    },
    {
      key: 'access',
      title: t`Access`,
      render: (_, grant) => {
        const member = membersById.get(grant.api_key_user_id)
        return member ? <AccessCell member={member} /> : null
      }
    },
    {
      key: 'connected',
      title: t`Connected`,
      render: (_, grant) => <span className="text-ink-3">{shortDate(grant.created_at)}</span>
    },
    {
      key: 'last-used',
      title: t`Last used`,
      render: (_, grant) => (
        <span className="text-ink-3">{grant.last_used_at ? relativeDate(grant.last_used_at) : t`Never`}</span>
      )
    },
    {
      key: 'actions',
      align: 'right',
      render: (_, grant) => (
        <Button size="small" danger onClick={() => confirmRevoke(grant)}>
          {t`Revoke`}
        </Button>
      )
    }
  ]

  const activeGrants = (grants.data?.grants ?? []).filter((grant) => grant.status === 'active')

  const grantsError = (() => {
    if (!grants.error) return null
    if (grants.error instanceof ApiError && grants.error.status === 403) return t`Only owners can see the connections.`
    return t`Couldn't load the connections.`
  })()

  return (
    <>
      <SettingsSectionHeader
        title={t`MCP`}
        description={t`Let an AI assistant such as Claude, ChatGPT or Cursor read and work in this workspace through the Model Context Protocol. Each assistant connects with an API key and gets exactly that key's access.`}
      />

      <section data-mcp-block="access">
        <Panel>
          <SettingsRows>
            <SettingsRow label={t`Allow MCP connections`} help={switchHelp}>
              <span className="inline-flex items-center gap-2.5">
                <Switch
                  checked={enabled}
                  disabled={!isOwner}
                  loading={toggle.isPending}
                  aria-label={t`Allow MCP connections`}
                  onChange={(checked) => toggle.mutate(checked)}
                />
                <span className="text-sm text-ink-2">{enabled ? t`On` : t`Off`}</span>
              </span>
            </SettingsRow>
            {!enabled && !isOwner ? (
              <div className="py-3">
                <Alert
                  type="info"
                  showIcon
                  title={t`An owner turned off MCP for this workspace. Connected assistants get no answer until it is turned back on.`}
                />
              </div>
            ) : null}
            <SettingsRow
              label={t`MCP server URL`}
              help={
                enabled
                  ? t`The address you give your assistant.`
                  : t`Calls to this workspace answer "not found" while the switch is off.`
              }
            >
              <div className="flex gap-2">
                <div
                  data-mcp-url
                  className={`flex min-w-0 flex-1 items-center rounded-md border border-line bg-subtle px-2.5 py-1 ${enabled ? '' : 'opacity-60'}`}
                >
                  <span className="truncate font-mono text-xs text-ink-2">{url}</span>
                </div>
                {mcpUrl ? (
                  <Button
                    icon={<Copy size={14} aria-hidden="true" />}
                    aria-label={t`Copy the MCP server URL`}
                    onClick={() => copy(mcpUrl, t`Copied`)}
                  >
                    {t`Copy`}
                  </Button>
                ) : null}
              </div>
            </SettingsRow>
            {!enabled && isOwner ? (
              <div className="border-t border-line-soft py-3 text-xs text-ink-3">
                {t`Turning it back on restores existing connections: nothing is revoked.`}
              </div>
            ) : null}
            <SettingsRow
              label={t`Connection page`}
              help={
                oauthUnavailable
                  ? t`claude.ai and ChatGPT need it.`
                  : t`Where claude.ai and ChatGPT ask for the API key.`
              }
            >
              {oauthUnavailable ? (
                <Pill tone="warning" dot={false}>
                  {t`Unavailable`}
                </Pill>
              ) : (
                <Pill tone="success" dot={false}>
                  {t`Available`}
                </Pill>
              )}
            </SettingsRow>
            {oauthUnavailable ? (
              <div className="py-3">
                <Alert
                  type="warning"
                  showIcon
                  title={
                    <Trans>
                      <Mono>API_ENDPOINT</Mono> must use https and have no path. It is{' '}
                      <Mono>{window.API_ENDPOINT}</Mono>.
                    </Trans>
                  }
                  description={t`Claude Code and Cursor can still connect with the key in a header.`}
                />
              </div>
            ) : null}
          </SettingsRows>
        </Panel>
      </section>

      <section data-mcp-block="connect">
        <Panel title={t`Connect an assistant`}>
          <div className="flex flex-col gap-3.5">
            <p className="m-0 text-xs text-ink-3">
              <Trans>
                Every assistant needs an API key. Create one per assistant in{' '}
                <TeamLink workspaceId={workspace.id}>Team & API keys</TeamLink>, with only the permissions it needs.
              </Trans>
            </p>
            <Alert
              type="warning"
              showIcon
              title={t`The assistant gets every right its key has.`}
              description={
                customPermissions
                  ? t`A key with full access lets the assistant send emails, change templates and delete contacts.`
                  : t`Without a licence that includes custom permissions, every key has full access: the assistant can send emails, change templates and delete contacts.`
              }
            />
            <div>
              <Tabs
                size="small"
                activeKey={client}
                onChange={(key) => setClient(key as MCPClient)}
                items={[
                  { key: 'claude-ai', label: 'claude.ai' },
                  { key: 'chatgpt', label: 'ChatGPT' },
                  { key: 'claude-code', label: 'Claude Code' },
                  { key: 'cursor', label: 'Cursor' }
                ]}
              />
              <ol data-mcp-steps className="m-0 flex list-decimal flex-col gap-1.5 pl-5 text-sm text-ink-2">
                {steps[client].map((step, index) => (
                  <li key={index}>{step}</li>
                ))}
              </ol>
            </div>
            {client === 'claude-code' || client === 'cursor' ? (
              <div className="flex flex-col gap-2 border-t border-line-soft pt-3">
                {client === 'cursor' ? (
                  <CodeBlock
                    code={cursorConfig(url)}
                    copyLabel={t`Copy the configuration`}
                    onCopy={(text) => copy(text, t`Copied`)}
                  />
                ) : (
                  <CodeBlock
                    code={claudeCodeCommand(url)}
                    copyLabel={t`Copy the command`}
                    onCopy={(text) => copy(text, t`Copied`)}
                  />
                )}
              </div>
            ) : null}
          </div>
        </Panel>
      </section>

      {isOwner ? (
        <section data-mcp-block="connections">
          <Panel
            flush
            title={
              <span className="inline-flex items-center gap-2">
                {t`Connections`}
                <Pill tone="neutral" dot={false}>
                  {t`Owners`}
                </Pill>
              </span>
            }
            footer={t`Assistants that send the key in a header (Claude Code, Cursor) are not listed here: they use the key directly.`}
          >
            <p className="m-0 px-3.5 pt-3 pb-2 text-xs text-ink-3">
              {t`Assistants connected through the connection page. Revoking one keeps its key; deleting the key ends all its connections.`}
            </p>
            {grantsError ? (
              <div className="px-3.5 pb-3">
                <Alert type="error" showIcon title={grantsError} />
              </div>
            ) : (
              <Table<OAuthGrant>
                rowKey="id"
                size="middle"
                columns={columns}
                dataSource={activeGrants}
                loading={grants.isLoading}
                pagination={false}
                locale={{ emptyText: t`No connections yet` }}
              />
            )}
          </Panel>
        </section>
      ) : null}

      <section data-mcp-block="tools">
        <Panel
          title={t`What an assistant can do`}
          footer={t`With the audit log licence, every action that is not a read is logged under the key that made it.`}
        >
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div className="rounded-md border border-line-soft px-3 py-2.5">
              <Pill tone="neutral" dot={false}>
                {t`Read`}
              </Pill>
              <p className="m-0 mt-1.5 text-sm text-ink-2">
                {t`Contacts, lists, segments, templates, broadcasts, message history and analytics.`}
              </p>
            </div>
            <div className="rounded-md border border-line-soft px-3 py-2.5">
              <Pill tone="primary" dot={false}>
                {t`Write`}
              </Pill>
              <p className="m-0 mt-1.5 text-sm text-ink-2">
                {t`Draft templates and broadcasts, update contacts and list subscriptions.`}
              </p>
            </div>
            <div className="rounded-md border border-line-soft px-3 py-2.5">
              <Pill tone="warning" dot={false}>
                {t`Send and delete`}
              </Pill>
              <p className="m-0 mt-1.5 text-sm text-ink-2">
                {t`Send emails, start automations, cancel broadcasts, delete objects. These tools are flagged so clients can ask you to confirm.`}
              </p>
            </div>
          </div>
        </Panel>
      </section>
    </>
  )
}
