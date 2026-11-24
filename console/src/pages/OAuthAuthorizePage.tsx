import { useRef, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { Alert, App, Button, Form, Input, Select, Spin } from 'antd'
import { CircleCheck, CircleX, TriangleAlert } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { useBrandName } from '../hooks/useBrandName'
import { useAuth } from '../contexts/AuthContext'
import { useLicense } from '../hooks/useLicense'
import { ApiError } from '../services/api/errors'
import {
  OAUTH_ERROR,
  isWebRedirectURL,
  oauthApi,
  type OAuthAuthorizationRequest,
  type OAuthRedirect
} from '../services/api/mcp'
import { createFullPermissions, grantsFullPermissions } from '../services/api/permissions'
import type { UserPermissions } from '../services/api/types'
import { InstanceLogo } from '../components/brand/InstanceLogo'
import { KeyValueList, Mono, Pill, type KeyValueItem } from '../components/layout'
import { LicenceGateNotice } from '../components/license/LicenceGateNotice'
import { PermissionScopePresets } from '../components/settings/PermissionScopePresets'
import { PermissionsMatrix } from '../components/settings/PermissionsMatrix'
import {
  API_KEY_PREFIX_MAX_LENGTH,
  apiKeyDomain,
  finalApiKeyPrefix,
  toApiKeyPrefix
} from '../components/settings/apiKeyPrefix'
import { oauthAuthorizeReturnPath, type OAuthAuthorizeReturnPath } from '../components/two_factor/signInAgain'

/**
 * The MCP connection page. An application (claude.ai, ChatGPT, Claude Code) sent the browser here
 * through GET /oauth/authorize; the person approves the request with an API key, pasted or, for an
 * owner signed in to the console, created on the page. The key itself never leaves this page but
 * in the approve call: it is not logged, not put in a URL, and cleared once the request is
 * approved.
 */

/** How the page ended: a card in place of the form. */
type Ending =
  | { kind: 'unavailable' }
  | { kind: 'other-browser' }
  | { kind: 'not-web' }
  | { kind: 'returning'; approved: boolean }

function errorCode(error: unknown): string | undefined {
  if (!(error instanceof ApiError)) return undefined
  const data = error.data as { error?: unknown } | null | undefined
  return typeof data?.error === 'string' ? data.error : undefined
}

/** The page's frame: the logo above one card, a line under it. */
function Frame({ children, footer, wide = false }: { children: ReactNode; footer?: ReactNode; wide?: boolean }) {
  return (
    <div className="flex min-h-screen flex-col items-center gap-5 bg-canvas px-5 py-10">
      <InstanceLogo variant="full" maxWidth={150} maxHeight={32} notifuseHeight={24} />
      <div className={`flex w-full flex-col gap-5 ${wide ? 'max-w-[640px]' : 'max-w-[520px]'}`}>{children}</div>
      {footer ? <p className="m-0 max-w-[520px] text-center text-xs text-ink-3">{footer}</p> : null}
    </div>
  )
}

const ICON_TONES = {
  danger: 'bg-danger-bg text-danger-fg',
  warning: 'bg-warning-bg text-warning-fg',
  success: 'bg-success-bg text-success-fg'
} as const

function IconTile({ tone, children }: { tone: keyof typeof ICON_TONES; children: ReactNode }) {
  return (
    <span aria-hidden="true" className={`grid size-10 shrink-0 place-items-center rounded-lg ${ICON_TONES[tone]}`}>
      {children}
    </span>
  )
}

/** A card that ends the page: why it cannot go on, and what to do. */
function EndCard({
  tone,
  title,
  children,
  action
}: {
  tone: keyof typeof ICON_TONES
  title: string
  children: ReactNode
  action?: ReactNode
}) {
  const icon =
    tone === 'success' ? <CircleCheck size={20} /> : tone === 'warning' ? <TriangleAlert size={20} /> : <CircleX size={20} />
  return (
    <section
      data-end-card={tone}
      className="flex flex-col gap-3.5 rounded-lg border border-line bg-surface px-7 pb-6 pt-7 shadow-card"
    >
      <IconTile tone={tone}>{icon}</IconTile>
      <h1 className="m-0 text-title font-semibold text-ink">{title}</h1>
      <div className="flex flex-col gap-2 text-ink-2">{children}</div>
      {action ? <div>{action}</div> : null}
    </section>
  )
}

export function OAuthAuthorizePage() {
  const { t } = useLingui()
  const brand = useBrandName()
  const { request: requestId } = useSearch({ from: '/console/oauth/authorize' })
  const { user } = useAuth()
  const [ending, setEnding] = useState<Ending | null>(null)
  const [endingName, setEndingName] = useState('')

  // Read once: a request is single use, and reading it again after an approval would only say
  // it is gone. The signed-in user is part of the key, so a sign-in reads the owned workspaces.
  const request = useQuery({
    queryKey: ['oauth-authorization-request', requestId ?? '', user?.id ?? ''],
    queryFn: () => oauthApi.getAuthorizationRequest(requestId ?? ''),
    enabled: Boolean(requestId),
    retry: false,
    gcTime: 0,
    staleTime: Infinity,
    refetchOnWindowFocus: false
  })

  const name = request.data ? request.data.client_name || request.data.client_host : endingName

  const end = (next: Ending, clientName: string) => {
    setEndingName(clientName)
    setEnding(next)
  }

  if (ending?.kind === 'returning') {
    return (
      <Frame>
        <EndCard
          tone="success"
          title={ending.approved ? t`Connected. Returning you to ${name}…` : t`Cancelled. Returning you to ${name}…`}
        >
          <p className="m-0">{t`You can close this page if nothing happens.`}</p>
        </EndCard>
      </Frame>
    )
  }

  if (ending?.kind === 'other-browser') {
    return (
      <Frame>
        <EndCard tone="danger" title={t`Finish connecting in the browser you started in`}>
          <p className="m-0">
            {t`This request was started in another browser. A connection can only be finished where it began, so that nobody can send you a link to approve their request.`}
          </p>
          <p className="m-0">{t`Go back to ${name} and connect again from this browser.`}</p>
        </EndCard>
      </Frame>
    )
  }

  if (ending?.kind === 'not-web') {
    return (
      <Frame>
        <EndCard tone="danger" title={t`${brand} did not open the application's address`}>
          <p className="m-0">{t`${name} asked to return to an address that is not a web page.`}</p>
          <p className="m-0">{t`Go back to ${name} and connect again.`}</p>
        </EndCard>
      </Frame>
    )
  }

  const unavailable = ending?.kind === 'unavailable' || !requestId || (request.error instanceof ApiError && request.error.status === 410)
  if (unavailable) {
    return (
      <Frame>
        <EndCard tone="danger" title={t`This connection request can't continue`}>
          {name ? (
            <>
              <p className="m-0">
                {t`It is more than 10 minutes old, or it was already used or cancelled. Nothing was shared with ${name}.`}
              </p>
              <p className="m-0">{t`Go back to ${name} and connect again.`}</p>
            </>
          ) : (
            <>
              <p className="m-0">
                {t`It is more than 10 minutes old, or it was already used or cancelled. Nothing was shared with the application.`}
              </p>
              <p className="m-0">{t`Go back to your application and connect again.`}</p>
            </>
          )}
        </EndCard>
      </Frame>
    )
  }

  if (request.error) {
    if (request.error instanceof ApiError && request.error.status === 404) {
      return (
        <Frame>
          <EndCard tone="danger" title={t`MCP is not available on this server`}>
            <p className="m-0">{t`Nothing was shared with the application. The server's administrator can turn it on.`}</p>
          </EndCard>
        </Frame>
      )
    }
    return (
      <Frame>
        <EndCard
          tone="danger"
          title={t`Couldn't load this connection request`}
          action={<Button onClick={() => void request.refetch()}>{t`Try again`}</Button>}
        >
          <p className="m-0">{t`Check your connection, then try again.`}</p>
        </EndCard>
      </Frame>
    )
  }

  if (!request.data) {
    return (
      <Frame>
        <div className="flex justify-center py-10">
          <Spin size="large" />
        </div>
      </Frame>
    )
  }

  return <AuthorizeForm request={request.data} onEnd={(next) => end(next, name)} />
}

interface AuthorizeFormProps {
  request: OAuthAuthorizationRequest
  onEnd: (ending: Ending) => void
}

function AuthorizeForm({ request, onEnd }: AuthorizeFormProps) {
  const { t } = useLingui()
  const brand = useBrandName()
  const { message } = App.useApp()
  const { user } = useAuth()
  const ownedWorkspaces = request.owned_workspaces ?? []
  const isOwner = ownedWorkspaces.length > 0
  const [mode, setMode] = useState<'paste' | 'create'>(isOwner ? 'create' : 'paste')
  const [denying, setDenying] = useState(false)
  // One call to the authorization server at a time, whatever starts it: a second Enter in the key
  // field, Enter while Cancel is pending, or a confirmation that lands during another call sends
  // nothing. A ref, so a call started in the same tick as the first still sees it.
  const inFlight = useRef(false)
  const exclusive = async (call: () => Promise<void>) => {
    if (inFlight.current) return
    inFlight.current = true
    try {
      await call()
    } finally {
      inFlight.current = false
    }
  }

  const name = request.client_name || request.client_host
  const returnPath = oauthAuthorizeReturnPath(request.request_id)

  /** Sends the browser on, to an http(s) address only. */
  const follow = (redirect: OAuthRedirect, approved: boolean) => {
    if (!isWebRedirectURL(redirect.redirect_url)) {
      onEnd({ kind: 'not-web' })
      return
    }
    onEnd({ kind: 'returning', approved })
    window.location.assign(redirect.redirect_url)
  }

  /** The refusals that end the page whatever was asked. */
  const endOnRefusal = (error: unknown): boolean => {
    const code = errorCode(error)
    if (code === OAUTH_ERROR.requestUnavailable) {
      onEnd({ kind: 'unavailable' })
      return true
    }
    if (code === OAUTH_ERROR.browserMismatch) {
      onEnd({ kind: 'other-browser' })
      return true
    }
    return false
  }

  const deny = () =>
    exclusive(async () => {
      setDenying(true)
      try {
        follow(await oauthApi.deny({ request_id: request.request_id }), false)
      } catch (error) {
        if (!endOnRefusal(error)) message.error(t`Couldn't cancel the request. Try again.`)
      } finally {
        setDenying(false)
      }
    })

  const details: KeyValueItem[] = [
    {
      key: 'application',
      label: t`Application`,
      value: (
        <span className="flex flex-wrap items-center gap-1.5">
          {name}
          {request.known_client ? (
            <Pill tone="success" dot={false}>
              {request.client_host}
            </Pill>
          ) : (
            <Pill tone="warning" dot={false}>
              {t`Unknown`}
            </Pill>
          )}
        </span>
      )
    },
    ...(request.known_client
      ? []
      : [{ key: 'published', label: t`Published at`, value: <Mono>{request.client_host}</Mono> }]),
    { key: 'returns', label: t`Returns to`, value: <Mono>{request.redirect_host}</Mono> },
    ...(mode === 'create' && user?.email
      ? [
          {
            key: 'signed-in',
            label: t`Signed in as`,
            value: (
              <span className="flex flex-wrap items-center gap-1.5">
                {user.email}
                <span className="text-xs text-ink-3">{t`Owner`}</span>
              </span>
            )
          }
        ]
      : [])
  ]

  const intro = !request.known_client
    ? t`${brand} does not know this application. Its name was chosen by whoever published it, at the address below.`
    : mode === 'create'
      ? t`${name} will work with an API key of its own. Create it here, or paste one you already have.`
      : t`${name} will work with the API key you give here, in that key's workspace and with that key's access.`

  return (
    <Frame
      wide={mode === 'create'}
      footer={t`Deleting the key in ${brand} ends this connection. An owner can also revoke it in Settings › AI › MCP.`}
    >
      <section className="rounded-lg border border-line bg-surface shadow-card">
        <div className="flex items-start gap-3.5 px-6 pb-4.5 pt-5.5">
          {request.known_client ? (
            <span
              aria-hidden="true"
              className="grid size-11 shrink-0 place-items-center rounded-lg bg-fill font-semibold text-ink-2"
            >
              {Array.from(name)[0]?.toUpperCase()}
            </span>
          ) : (
            <IconTile tone="warning">
              <TriangleAlert size={20} />
            </IconTile>
          )}
          <div className="min-w-0">
            <h1 className="m-0 text-title font-semibold text-ink">
              {request.known_client ? t`Connect ${name} to ${brand}` : t`Connect "${name}" to ${brand}`}
            </h1>
            <p className="m-0 mt-1 text-ink-2">{intro}</p>
          </div>
        </div>

        <div className="flex flex-col gap-3 border-t border-line-soft px-6 py-4">
          <KeyValueList labelWidth="auto" items={details} />
          {request.loopback_redirect ? (
            <Alert
              type="warning"
              showIcon
              title={t`This application returns to a program on your own computer. Connect only if you started this connection yourself, just now.`}
            />
          ) : null}
        </div>

        {mode === 'paste' ? (
          <PasteKeySection
            request={request}
            name={name}
            denying={denying}
            onDeny={() => void deny()}
            exclusive={exclusive}
            onApproved={(redirect) => follow(redirect, true)}
            endOnRefusal={endOnRefusal}
            switchLink={
              isOwner ? (
                <Button type="link" onClick={() => setMode('create')}>
                  {t`Create a key for this agent`}
                </Button>
              ) : returnPath ? (
                <Link to="/console/signin" search={{ return_to: returnPath }} className="text-sm">
                  {t`Owner? Sign in to create a key here`}
                </Link>
              ) : null
            }
          />
        ) : (
          <CreateKeySection
            request={request}
            name={name}
            denying={denying}
            onDeny={() => void deny()}
            exclusive={exclusive}
            onApproved={(redirect) => follow(redirect, true)}
            endOnRefusal={endOnRefusal}
            onPaste={() => setMode('paste')}
            returnPath={returnPath}
          />
        )}
      </section>
    </Frame>
  )
}

interface SectionProps {
  request: OAuthAuthorizationRequest
  name: string
  denying: boolean
  onDeny: () => void
  onApproved: (redirect: OAuthRedirect) => void
  endOnRefusal: (error: unknown) => boolean
  /** Runs the call unless another one to the authorization server is in flight. */
  exclusive: (call: () => Promise<void>) => Promise<void>
}

function PasteKeySection({
  request,
  name,
  denying,
  onDeny,
  onApproved,
  endOnRefusal,
  exclusive,
  switchLink
}: SectionProps & { switchLink: ReactNode }) {
  const { t } = useLingui()
  const brand = useBrandName()
  const [apiKey, setApiKey] = useState('')
  const [keyError, setKeyError] = useState<string | null>(null)
  const [connecting, setConnecting] = useState(false)

  const connect = async () => {
    const key = apiKey.trim()
    if (!key) return
    await exclusive(async () => {
      setConnecting(true)
      setKeyError(null)
      try {
        const redirect = await oauthApi.approveWithKey({ request_id: request.request_id, api_key: key })
        // The key served its one purpose: it does not stay in memory while the browser leaves.
        setApiKey('')
        onApproved(redirect)
      } catch (error) {
        if (endOnRefusal(error)) return
        const code = errorCode(error)
        if (code === OAUTH_ERROR.notAnAPIKey) {
          setKeyError(
            t`This is not a ${brand} API key, or the key was deleted. Sign-in links and invitations don't work here. Create a key in Settings › Team & API keys.`
          )
        } else if (code === OAUTH_ERROR.workspaceMCPDisabled) {
          setKeyError(t`MCP is switched off in this key's workspace. An owner can switch it on in Settings › AI › MCP.`)
        } else if (code === OAUTH_ERROR.rateLimited) {
          setKeyError(t`Too many attempts from this address. Wait a minute, then try again.`)
        } else {
          setKeyError(t`Couldn't connect. Try again in a moment.`)
        }
      } finally {
        setConnecting(false)
      }
    })
  }

  return (
    <>
      <div className="flex flex-col gap-2 border-t border-line-soft px-6 py-4">
        <label htmlFor="oauth-api-key" className="text-sm font-semibold text-ink">
          {t`API key`}
        </label>
        <Input.Password
          id="oauth-api-key"
          value={apiKey}
          onChange={(event) => {
            setApiKey(event.target.value)
            setKeyError(null)
          }}
          onPressEnter={() => void connect()}
          autoComplete="off"
          spellCheck={false}
          visibilityToggle={false}
          status={keyError ? 'error' : undefined}
          aria-invalid={keyError ? true : undefined}
          aria-describedby={keyError ? 'oauth-api-key-error oauth-api-key-help' : 'oauth-api-key-help'}
          placeholder="eyJhbGciOiJIUzI1NiIs…"
        />
        {keyError ? (
          <div id="oauth-api-key-error" role="alert" className="text-xs text-danger-fg">
            {keyError}
          </div>
        ) : null}
        <div id="oauth-api-key-help" className="text-xs text-ink-3">
          {request.known_client
            ? t`Create one in Settings › Team & API keys. Give ${name} a key of its own, with only the permissions it needs. ${brand} never shows this key to ${name}.`
            : t`Create one in Settings › Team & API keys. The application gets this key's workspace and access. Give it a key of its own.`}
        </div>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 border-t border-line-soft px-6 py-4">
        <div data-oauth-footer-link>{switchLink}</div>
        {/* ml-auto keeps the buttons on the right when they wrap below a long link. */}
        <div data-oauth-footer-actions className="ml-auto flex shrink-0 gap-2">
          <Button onClick={onDeny} loading={denying} disabled={connecting}>
            {t`Cancel`}
          </Button>
          <Button type="primary" onClick={() => void connect()} loading={connecting} disabled={!apiKey.trim() || denying}>
            {t`Connect`}
          </Button>
        </div>
      </div>
    </>
  )
}

/** Why the shortcut cannot create a key for now; pasting one still works. */
type ShortcutBlock = 'two-factor' | 'session'

function CreateKeySection({
  request,
  name,
  denying,
  onDeny,
  onApproved,
  endOnRefusal,
  exclusive,
  onPaste,
  returnPath
}: SectionProps & { onPaste: () => void; returnPath: OAuthAuthorizeReturnPath | undefined }) {
  const { t } = useLingui()
  const { modal } = App.useApp()
  const navigate = useNavigate()
  const { has } = useLicense()
  // As in the Create API Key drawer: without the rbac licence only full access is accepted.
  const rbacLocked = !has('rbac')
  const workspaces = request.owned_workspaces ?? []
  const [workspaceId, setWorkspaceId] = useState<string | undefined>(
    () => workspaces.find((workspace) => !workspace.mcp_disabled)?.id
  )
  const [keyName, setKeyName] = useState(() => toApiKeyPrefix(name))
  const [permissions, setPermissions] = useState<UserPermissions>(createFullPermissions)
  const [creating, setCreating] = useState(false)
  const [nameError, setNameError] = useState<string | null>(null)
  const [workspaceError, setWorkspaceError] = useState<string | null>(null)
  const [formError, setFormError] = useState<string | null>(null)
  const [block, setBlock] = useState<ShortcutBlock | null>(null)

  const domainName = apiKeyDomain(window.API_ENDPOINT)
  const workspace = workspaces.find((candidate) => candidate.id === workspaceId)
  const anyDisabled = workspaces.some((candidate) => candidate.mcp_disabled)

  const create = async () => {
    if (!workspace) return
    const prefix = finalApiKeyPrefix(keyName)
    if (!prefix) {
      setNameError(t`Please enter an API key name`)
      return
    }
    await exclusive(async () => {
      setCreating(true)
      setNameError(null)
      setWorkspaceError(null)
      setFormError(null)
      try {
        onApproved(
          await oauthApi.createKeyAndApprove({
            request_id: request.request_id,
            workspace_id: workspace.id,
            name: prefix,
            permissions
          })
        )
      } catch (error) {
        if (endOnRefusal(error)) return
        const code = errorCode(error)
        if (code === OAUTH_ERROR.keyNameTaken) {
          setNameError(t`An API key already uses this name, here or in another workspace. Pick another name.`)
        } else if (code === OAUTH_ERROR.workspaceMCPDisabled) {
          setWorkspaceError(
            t`MCP is switched off in this workspace. Switch it on in Settings › AI › MCP, or pick another workspace.`
          )
        } else if (code === OAUTH_ERROR.twoFactorRequired) {
          setBlock('two-factor')
        } else if (error instanceof ApiError && error.status === 401) {
          setBlock('session')
        } else {
          // A licence or permission refusal arrives already worded by the API client.
          setFormError(error instanceof Error && error.message ? error.message : t`Couldn't create the key. Try again.`)
        }
      } finally {
        setCreating(false)
      }
    })
  }

  // A full-access key reaches everything: the person confirms, with the application's address in
  // view, before the shortcut makes one.
  const submit = () => {
    if (!workspace) return
    if (!grantsFullPermissions(permissions)) {
      void create()
      return
    }
    const workspaceName = workspace.name
    const clientHost = request.client_host
    modal.confirm({
      title: t`Give ${name} full access?`,
      content: t`The key will have every right in ${workspaceName}, sending emails and deleting contacts included. It goes to the application published at ${clientHost}.`,
      okText: t`Create key and connect`,
      cancelText: t`Cancel`,
      onOk: () => create()
    })
  }

  const pasteButton = (
    <Button type="link" onClick={onPaste}>
      {t`Paste an existing key`}
    </Button>
  )

  return (
    <>
      <div className="flex flex-col gap-3.5 border-t border-line-soft px-6 py-4">
        <div>
          <div className="text-sm font-semibold text-ink">{t`Create a key for this agent`}</div>
          <p className="m-0 mt-0.5 text-xs text-ink-3">
            {t`Nobody sees this key. It serves only this connection, and you can delete it later in Team & API keys.`}
          </p>
        </div>

        {block === 'two-factor' ? (
          <Alert
            type="warning"
            showIcon
            title={t`This server requires two-factor authentication before you can create a key. You can still paste an existing key.`}
            description={
              <div className="mt-2 flex flex-wrap gap-2">
                <Button type="primary" href="/console/account/security" target="_blank" rel="noopener noreferrer">
                  {t`Set up two-factor authentication`}
                </Button>
                <Button onClick={onPaste}>{t`Paste an existing key`}</Button>
              </div>
            }
          />
        ) : block === 'session' ? (
          <Alert
            type="warning"
            showIcon
            title={t`Your session has expired. Sign in again to create a key here. The request stays open until it is 10 minutes old.`}
            description={
              <div className="mt-2 flex flex-wrap gap-2">
                {returnPath ? (
                  <Button
                    type="primary"
                    onClick={() => navigate({ to: '/console/signin', search: { return_to: returnPath } })}
                  >
                    {t`Sign in again`}
                  </Button>
                ) : null}
                <Button onClick={onPaste}>{t`Paste an existing key`}</Button>
              </div>
            }
          />
        ) : (
          <Form layout="vertical" component="div">
            <Form.Item
              label={t`Workspace`}
              htmlFor="oauth-workspace"
              validateStatus={workspaceError ? 'error' : undefined}
              help={
                workspaceError ??
                (anyDisabled ? t`A workspace with MCP off can't be picked. Switch it on in Settings › AI › MCP first.` : undefined)
              }
            >
              <Select
                id="oauth-workspace"
                value={workspaceId}
                onChange={(value: string) => {
                  setWorkspaceId(value)
                  setWorkspaceError(null)
                }}
                virtual={false}
                placeholder={t`Pick a workspace`}
                options={workspaces.map((candidate) => ({
                  value: candidate.id,
                  disabled: candidate.mcp_disabled,
                  label: candidate.mcp_disabled ? (
                    <span className="flex items-center justify-between gap-2">
                      <span>{candidate.name}</span>
                      <span className="text-xs text-ink-3">{t`MCP off`}</span>
                    </span>
                  ) : (
                    candidate.name
                  )
                }))}
              />
            </Form.Item>

            <Form.Item
              label={t`Key name`}
              htmlFor="oauth-key-name"
              required
              validateStatus={nameError ? 'error' : undefined}
              help={nameError ?? undefined}
            >
              <Input
                id="oauth-key-name"
                value={keyName}
                onChange={(event) => {
                  setKeyName(toApiKeyPrefix(event.target.value))
                  setNameError(null)
                }}
                maxLength={API_KEY_PREFIX_MAX_LENGTH}
                suffix={<span className="text-ink-3">{'@' + domainName}</span>}
              />
            </Form.Item>

            <Form.Item label={t`Permissions`}>
              <LicenceGateNotice feature="rbac" workspaceId={workspace?.id} />
              {rbacLocked ? (
                <div className="mb-4">
                  <Alert
                    type="warning"
                    showIcon
                    title={t`The assistant gets every right its key has.`}
                    description={t`Without a licence that includes custom permissions, every key has full access: the assistant can send emails, change templates and delete contacts.`}
                  />
                </div>
              ) : null}
              <PermissionScopePresets value={permissions} onChange={setPermissions} fullAccessOnly={rbacLocked} />
              <PermissionsMatrix value={permissions} onChange={setPermissions} fullAccessOnly={rbacLocked} />
            </Form.Item>

            {formError ? <Alert type="error" showIcon title={formError} /> : null}
          </Form>
        )}
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 border-t border-line-soft px-6 py-4">
        <div data-oauth-footer-link>{pasteButton}</div>
        <div data-oauth-footer-actions className="ml-auto flex shrink-0 gap-2">
          <Button onClick={onDeny} loading={denying} disabled={creating}>
            {t`Cancel`}
          </Button>
          <Button
            type="primary"
            onClick={submit}
            loading={creating}
            disabled={block !== null || !workspace || denying || (rbacLocked && !grantsFullPermissions(permissions))}
          >
            {t`Create key and connect`}
          </Button>
        </div>
      </div>
    </>
  )
}
