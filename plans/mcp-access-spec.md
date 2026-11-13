# MCP access spec

**Status**: draft v4 for discussion.
**Date**: 2026-11-13
**Revision**: v4 folds the client and protocol sources of Appendix A (Q9) into the body. v3 had folded in decisions D17–D20 and two rounds of independent review, each point re-checked against the code.
**Base**: `chore/gomjml-notifuse-module` at `f4c3e472` (VERSION 43.2, a 44.0 WIP section in the changelog).
**Location**: committed to `dev` on 2026-11-13. **P**: when implementation starts, the outline of §12 moves to `plans/mcp-access-plan.md`, as `CLAUDE.md` names plans.
**Delivery**: two lots (D1).
- Lot 1 needs no migration.
- Lot 2 adds system tables and a `users` column, so a new major version numbered at implementation time.
- Two prerequisites come before any Lot 1 code (§10): an SDK spike (Q7) and the input/output schemas of every tool (Q8).

Labels: **D** = decision taken with Pierre; **F** = fact verified in the code or a cited source; **P** = proposal, open until approved.

## 1. Goal

Let an AI assistant (Claude, ChatGPT, Cursor, Claude Code, Mistral, n8n…) work in Notifuse through the Model Context Protocol. It can read contacts, lists, segments, templates, campaigns, message history and analytics. It can write drafts, send emails and delete objects, always within the permissions of the person or the key behind the connection.

## 2. Decisions

| # | Decision |
| - | - |
| D1 | Both credentials, delivered in two lots. Lot 1: `/mcp` with the existing API keys. Lot 2: OAuth authorization server (CIMD + DCR), consent screen, connected apps. |
| D2 | One endpoint `/mcp`, multi-workspace. Every tool takes a `workspace_id`, and a `list_workspaces` tool lists what the connection may reach. |
| D3 | The workspaces an OAuth connection reaches are chosen on the consent screen. A workspace joined later is not included until the user reconnects. |
| D4 | Permissions at consent: on a deployment licensed for RBAC, the full resource × read/write matrix; without the licence, a choice between "Read only" and "Read and write". |
| D5 | Sending is included without a separate opt-in. A tool that sends is available whenever the effective permissions allow it, and annotations ask clients for confirmation. |
| D6 | Destructive tools (deletes, campaign cancellation) are in v1, annotated `destructiveHint`. |
| D7 | Each workspace has an owner switch "Allow AI assistants (MCP)", on by default. |
| D8 | MCP access is free in every tier. The only licensed part is the custom matrix of D4, gated by the existing `rbac` feature. |
| D9 | `create_template` and `update_template` write MJML in code mode. Templates created through MCP open in the MJML editor, not the visual builder. |
| D10 | `activate_automation` is a `send` tool. |
| D11 | The spec stays in English, like the other documents in `plans/`. |
| D12 | No dedicated MCP key. Lot 1 accepts the existing API keys as they are. Presets without the licence exist only for OAuth grants, where they apply to MCP alone. |
| D13 | An OAuth grant stops working when its user has not signed in to the console for 90 days. |
| D14 | MCP sets list subscriptions exactly as the API does. `contactLists.updateStatus` accepts any status on an existing membership, including `active` for an unsubscribed, bounced or complained contact. `lists.subscribe` keeps its own rules (§3). |
| D15 | Audit-log coverage of the data plane for the whole product is revised later, as its own project. MCP follows its own rule meanwhile (D19). |
| D16 | No notification when a member connects an application. Owners see connections in the MCP tab and in the audit log. |
| D17 | A grant stopped by D13 is revoked for good at the first detection. The client gets `401` / `invalid_grant` and the user reconnects with a new consent. |
| D18 | Any valid console session may approve a connection; no recent sign-in is required. The resulting risk is accepted (§9 R9). |
| D19 | Every MCP tool that is not a read writes an audit row, even when its API route is not audited. The `audit_logs` licence gate still applies. |
| D20 | Lot 1 ships the whole catalogue. Without the `rbac` licence, every key is full access, so an assistant gets every right including sending and deleting; the docs and the MCP tab say so plainly. |

## 3. Current state

### Authentication

- **F** One HS256 secret signs every JWT.
  - A `user` token carries a session ID. The middleware only checks its presence; services verify the session through `VerifyUserSession`.
  - Console sessions last 30 days (`internal/app/app.go` ~728).
  - An `api_key` token is valid ten years and is revoked by deleting its `users` row; a revoked key surfaces as `ErrAPIKeyRevoked`.

  (`internal/http/middleware/auth.go`, `internal/service/auth_service.go`)
- **F** `RequireAuth` accepts any JWT whose `type` claim is non-empty. Invitation tokens are JWTs signed with the same secret and carry no `type`. A bearer that is not a JWT fails to parse and answers `401`.
- **F** `AuthenticateUserForWorkspace` caches the user under `WorkspaceUserKey(workspaceID)` but the membership under `UserWorkspaceKey`, which is not keyed by workspace (`auth_service.go` ~141-146, ~193-194). Authenticating A, then B, then A on one context returns B's membership for A. The services read for this spec authenticate one workspace per request; multi-workspace MCP would chain them.
- **F** Root emails are synthesised as owners of every workspace, with or without a membership row. Their permissions are set to `domain.FullPermissions` **by reference** (`auth_service.go` ~163-180), a package-level map that must never be mutated (`workspace.go` ~111-113).
- **F** The console keeps its JWT in `localStorage` (`console/src/contexts/AuthContext.tsx`).
- **F** Sign-in is by magic code or OIDC SSO, plus `user.rootSignin`, a programmatic root sign-in by email, timestamp and HMAC signature (`internal/domain/user.go` ~64-69), audited with the method `root_password`.
- **F** `pkg/crypto` has bcrypt helpers, but nothing in `internal/` calls them, so there are no user passwords. The `CLAUDE.md` mention of bcrypt for password hashing is outdated.
- **F** `User` has no last-sign-in field (`internal/domain/user.go` ~35-43).
- **F** `UserService.Logout` deletes every session of the user (`internal/service/user_service.go:540`).

### Permissions and licences

- **F** Services enforce permissions through `AuthenticateUserForWorkspace` then `UserWorkspace.HasPermission`, which returns true for any `owner` before reading the map (`internal/domain/workspace.go`).
- **F** An API key is a `member` row. `CreateAPIKey` turns a nil input into a full map, while a stored nil denies everything. An owner can edit a key's permissions afterwards through `workspaces.setUserPermissions`.
- **F** A customer-chosen restricted scope is gated by `rbac`. A product-fixed scope (`scopeFixedByProduct`) is documented as "not reachable from the API" (`workspace_service.go` ~1478-1495).
- **F** `template_i18n` is enforced in `TemplateService` (~230, ~404). `TemplateService.UpdateTemplate` refuses to switch a template between visual and code mode (~409).
- **F** The SSO licence has no write gate on the backend. Without it, `OIDCService.IsEnabled` is false (`oidc_service.go` ~175) and the console hides the SSO button (`SsoLicenceNotice.tsx`); `sso_gated` is only a field of the "Licence resolved" log line (`license_service.go` ~605). The `license.go` comment saying the feature "gates the console's write access" is stale.
- **F** `AnalyticsSchemaResource` maps each analytics schema to a resource's read permission, web analytics included (`internal/domain/analytics.go` ~800-830).

### Audit

- **F** The audit middleware classifies requests by their `/api/<action>` path, so `/mcp` is never classified. A route's action name is its path (`internal/http/middleware/audit.go`).
- **F** An `AuditRecord` is finalized once.
- **F** The data plane is excluded by design (`plans/audit-logs-v41-plan.md`, decision 1, "Control plane only"). `transactional.send`, `transactional.testTemplate`, `contacts.upsert`, `customEvents.upsert`, `lists.subscribe`, `contactLists.removeContact` and `contactLists.updateStatus` are in `AuditExcludedRoutes`. `broadcasts.create`, `broadcasts.schedule`, `templates.update` and `contacts.delete` are audited (`internal/domain/audit_actions.go`).
- **F** A test holds every `/api/` route to exactly one of `AuditedActions`, `AuditLoggedReads` and `AuditExcludedRoutes`.
- **F** Rows are written only under the `audit_logs` licence feature (Enterprise keys).

### Routes and services

- **F** The real routes come from the non-test files of `internal/http`. `contacts.get`, `contactLists.addContact`, `email.testTemplate`, `tasks.executePending`, `ses.nope` and `webhookSubscriptions.nope` appear only in tests.
- **F** `lists.subscribe` (authenticated) handles a contact by its status on the list:

  | Status on the list | Result |
  | - | - |
  | none (new) | `active` |
  | `active` | no-op |
  | `bounced` or `complained` | blocked |
  | `unsubscribed` | forced double opt-in: `pending`, with the confirmation email when the list has a template |

  (`internal/service/list_service.go` ~398-450, ~539)
- **F** `contactLists.updateStatus` requires `lists:write` and `contacts:write`. It accepts `active`, `pending`, `unsubscribed`, `bounced` and `complained` on an existing membership, and answers success with `found: false` when the contact is not on the list (`contact_list_service.go` ~160-205, `contact_list.go` ~276-290).
- **F** `transactional.send` deduplicates on `external_id`: a second call with the same value returns the first message's ID without sending (`transactional_service.go` ~672-688).
- **F** A broadcast can only be scheduled from `draft` (`broadcast_service.go` ~329). `broadcasts.sendToIndividual` requires `broadcasts:write` (~1119).
- **F** `RestrictedInDemo` wraps routes at registration, including every `workspaces.set*` route (`workspace_handler.go` ~86-90).
- **F** Unknown non-API paths answer a `307` to `/console` (`root_handler.go` ~141-142). The console is served under `/console/*` on any host.
- **F** `CORSMiddleware` allows `Accept, Content-Type, Content-Length, Accept-Encoding, Authorization`, always sends `Access-Control-Allow-Credentials: true`, exposes no header, and supports an origin list (`cors.go`).

### Shared packages and data

- **F** `pkg/ratelimiter` is in-memory, per process, and fails closed on a namespace without a policy.
- **F** `ClientIP` takes the first `X-Forwarded-For` entry, which is client-supplied unless a trusted proxy overwrites it.
- **F** `pkg/safehttpclient` refuses private addresses in its `DialContext`, so every hop is checked. Its `CheckRedirect` only limits schemes and the redirect count, and `New()` returns a 10 s client that follows redirects (`safehttpclient.go` ~68-118).
- **F** Workspace settings are one JSON blob; owner-only parts have dedicated endpoints.
- Not verified in code: Notifuse Cloud runs one instance per tenant at `<slug>.notifuse.cloud`. No MCP code exists in the notifuse, cloud or docs repositories (searched).

## 4. External constraints

- **F** The MCP revision dated 2026-07-28 ([changelog](https://modelcontextprotocol.io/specification/2026-07-28/changelog)):
  - is stateless: no `initialize`, no `Mcp-Session-Id`, and `server/discover` is mandatory;
  - requires the `Mcp-Method` and `Mcp-Name` headers;
  - adds `resultType` on every result, and mandatory `ttlMs` and `cacheScope` on list results;
  - makes a client re-issue a request whose stream broke, under a new ID;
  - deprecates DCR in favour of CIMD.

  Its tools page allows `tools/list` to "vary by the authorization presented on the request". It says a tool returning structured content "SHOULD also return the serialized JSON in a TextContent block".
- **F** The authorization spec ties `403` to `insufficient_scope` and step-up re-authorization, and lets a client add `offline_access` when the authorization server lists it. Protected Resource Metadata "SHOULD NOT" list `offline_access`.
- **F** Go SDK v1.7.0 ([release notes](https://github.com/modelcontextprotocol/go-sdk/releases)):
  - supports 2026-07-28 on Streamable HTTP only with `StreamableHTTPOptions.Stateless = true`;
  - keeps compatibility with 2025-11-25 and earlier;
  - offers resource-server helpers (`RequireBearerTokenOptions`, `oauthex.MatchesResource`);
  - offers an opt-in `PropagateRequestCancellation`.

  The release notes describe no authorization server.
- **F** Claude's connector requirements ([authentication](https://claude.com/docs/connectors/building/authentication.md)):
  - OAuth with DCR or CIMD. CIMD is normally used when the authorization server advertises `client_id_metadata_document_supported: true` and `none` in `token_endpoint_auth_methods_supported`.
  - Static headers are in beta.
  - PKCE S256.
  - Redirect URIs: `https://claude.ai/api/mcp/auth_callback`, plus loopback on any port for Claude Code.
  - Refresh tokens rotated; an invalid one answers `invalid_grant`.
  - The token endpoint accepts form-urlencoded bodies.
  - Refresh happens on a `401` and up to five minutes before expiry.
  - A `401` carries `resource_metadata`; the PRM `resource` equals the URL as entered; only the first `authorization_servers` entry is used.
  - Limits: 10 s for OAuth endpoints and 30 s for refresh.
  - The instance must be reachable from `160.79.104.0/21`.
- **F** Streamable HTTP ([transport](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http)):
  - servers "MUST validate the `Origin` header on all incoming connections", answering `403` when it is present and invalid;
  - the body of a POST is a single JSON-RPC request, so one tool call per HTTP request;
  - an `Mcp-Method`/`Mcp-Name` mismatch answers `400` with `HeaderMismatch`.
- **F** Authorization ([security considerations](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/security-considerations), [discovery](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/authorization-server-discovery)):
  - every authorization server endpoint over HTTPS;
  - redirect URIs `localhost` or HTTPS, matched exactly;
  - no automatic redirect to an untrusted URI;
  - CIMD fetches guarded against SSRF;
  - an extra warning for localhost-only redirects, and the redirect hostname shown at consent.

  For an issuer with a path, clients look for metadata by path insertion at the host root (`https://host/.well-known/oauth-authorization-server/<path>`).
- **F** ChatGPT ([Apps SDK authentication](https://developers.openai.com/apps-sdk/build/auth)):
  - OAuth only: it "cannot present custom API keys".
  - It prioritizes CIMD. Its CIMD document publishes the plural `token_endpoint_auth_methods_supported: ["none", "private_key_jwt"]` and the legacy singular `token_endpoint_auth_method: "private_key_jwt"`, and it picks a method from the intersection with the authorization server's list.
  - With DCR it registers once per connection and reuses the client, which must stay valid while the connection is in use (`invalid_client` otherwise).
  - It sends `resource` on authorization and token requests.
  - It expects `iss` "in every successful and error authorization response", compared as an exact string. When `iss` is supported it uses the stable redirect URI `https://chatgpt.com/connector_platform_oauth_redirect`.
- **F** Cursor ([MCP docs](https://cursor.com/docs/context/mcp)):
  - remote servers by `url` with `headers`, supporting `${env:NAME}` interpolation;
  - OAuth through DCR as a public client; CIMD is not mentioned;
  - static OAuth callbacks `https://www.cursor.com/agents/mcp/oauth/callback` and `http://localhost:8787/callback`.
- **F** Claude Code's CIMD document (`https://claude.ai/oauth/claude-code-client-metadata`) declares `redirect_uris` `http://localhost/callback` and `http://127.0.0.1/callback`, and `token_endpoint_auth_method: "none"`.
- Not verified:
  - whether locally running clients reach an instance that is not public when they authenticate with a header;
  - the Mistral and n8n requirements.

  With CIMD, the Notifuse server must reach the client's metadata host (`claude.ai`, `chatgpt.com`…).

## 5. Architecture

### 5.1 Endpoint, transport, URLs, switches

- **P** The MCP server lives in a delivery-layer package `internal/mcp`.
  - It is mounted on the stdlib mux at `/mcp` with `github.com/modelcontextprotocol/go-sdk` v1.7.x.
  - Streamable HTTP only, `Stateless: true`, and `PropagateRequestCancellation` off: an interrupted POST never cancels a send in flight.
  - Tools call services, never HTTP handlers.
- **P** `MCP_ENABLED` (environment variable, default `true`) is the instance-wide switch. When false, `/mcp`, `/oauth/*` and the MCP `.well-known` documents answer `404`.
- **P** MCP and OAuth are off on demo deployments (`IsDemo`), whatever `MCP_ENABLED` says.
- **P** One canonical-URL function derives `issuer`, `iss`, `resource` (issuer + `/mcp`), the PRM URLs and the console's URL from `API_ENDPOINT`: scheme, lowercase host, port only if non-default, no trailing slash. A received `resource` passes through the same function before comparison.
- **P** Lot 2's authorization server is unavailable when `API_ENDPOINT` is not `https` (loopback hosts excepted) or carries a path.
  - Clients find a path issuer's metadata at the host root (§4), which a Notifuse served under a path prefix does not own.
  - The binary still starts: `/oauth/*` answers `404` and the MCP tab shows "OAuth unavailable" with the reason.
  - Lot 1 works in both cases.
- **P** When `API_ENDPOINT` changes, grants bound to the old resource are refused (§7.3).
- **P** `/mcp`, `/oauth/*` and the MCP `.well-known` paths answer only when `r.Host` (port stripped, lowercase, as `root_handler.go` does) equals the canonical host. Any other host gets a JSON `404` naming the expected host. A reverse proxy must forward the original `Host`, which the docs say. `/mcp/` answers a JSON `404` naming the canonical URL.
- **P** The host restriction, credential resolution and CORS for these paths are mounted inside the mux, like the audit middleware, so the integration harness sees them.
- **P** `Origin` on `/mcp`, as the transport requires (§4):
  - A request carrying an `Origin` header outside the allowlist gets `403` before any credential is read.
  - The allowlist is the canonical origin plus `MCP_ALLOWED_ORIGINS` (new environment variable, comma-separated, empty by default).
  - Requests without `Origin`, as hosted clients send them from their servers, are not affected.
  - The docs tell MCP Inspector users to add its origin or use its proxy mode.
  - The SDK spike checks whether v1.7 already validates `Origin` (Q7).
- **P** CORS on these paths, whatever `CORS_ALLOW_ORIGIN` says:
  - On `/mcp`, `Access-Control-Allow-Origin` reflects an allowed origin only.
  - On `/oauth/*` and the MCP `.well-known` documents, it is `*`.
  - Never `Access-Control-Allow-Credentials` on these paths.
  - Allowed request headers: the five current ones plus `Mcp-Protocol-Version`, `Mcp-Method`, `Mcp-Name` and `Mcp-Session-Id`.
  - Exposed header: `WWW-Authenticate`.

  No `Mcp-Param-*` header, since no tool declares `x-mcp-header`.

### 5.2 Credentials on `/mcp`

The handler resolves the credential before any JSON-RPC processing. Every refusal is an HTTP `401` with `WWW-Authenticate: Bearer` (Lot 2 adds `resource_metadata` and `scope="mcp"`), never a tool error.

- **Lot 1**: API key JWTs (`type = api_key`). A revoked key (`ErrAPIKeyRevoked`) is a `401`.
- **Refused**: console session tokens (`type = user`) and tokens without `type` (invitations).
- **Lot 2**: opaque OAuth access tokens (§7.3). An invalid, expired or revoked token, or a revoked grant (D13/D17 included), is a `401`.
- **P** OAuth access tokens do not parse as JWTs, so `RequireAuth` refuses them on `/api/*`. A test pins it.
- **P** In Lot 1 the MCP `.well-known` paths answer a JSON `404` instead of the console redirect:
  - `/.well-known/oauth-protected-resource*`
  - `/.well-known/oauth-authorization-server*`
  - `/.well-known/openid-configuration`

### 5.3 Identity, effective permissions, and the tool middleware

- **P** The handler places an `MCPCaller` in the context: the credential kind, the user ID, and for OAuth the grant (workspaces, preset or matrix, resource, client name).
  - For an API key it also sets `UserIDKey` and `UserTypeKey = api_key`, as `RequireAuth` does.
  - It never sets `SessionIDKey` or `SystemCallKey`. A test asserts the second.
- **P** **Lot 2 only**: `AuthenticateUserFromContext` gains a case evaluated first. When the context carries an `MCPCaller` of kind `oauth`:
  - it loads the grant's user and requires `Type == user`, with no session;
  - a context carrying both an OAuth `MCPCaller` and an `api_key` type is refused;
  - nothing in Lot 1 builds an OAuth `MCPCaller`, and a Lot 1 test asserts it.
- **P** `AuthenticateUserForWorkspace` runs in this order:
  1. load the membership and apply root elevation;
  2. when an `MCPCaller` is present, check reachability (§5.4) and answer not found when it fails, with the same answer as `ErrUserNotInWorkspace`;
  3. call `SetActor` with the original membership, so an owner is audited as an owner;
  4. demote a copy to `member` carrying `effective`, always a freshly allocated map;
  5. cache, keyed and checked by workspace (`userWorkspace.WorkspaceID == workspaceID`). This fixes §3's latent bug for every caller.
- **P** `effective` for a call in workspace W:
  - `base` = full permissions if the user owns W or is root, otherwise the stored membership permissions. For an API key, its stored permissions.
  - Preset "Read and write": a copy of `base`.
  - Preset "Read only": a copy of `base` with every write false.
  - Licensed custom matrix: `base ∩ matrix`.
  - Presets are resolved at call time, never stored as maps, and `GrantUnenforcedPermissions` is not applied to grants.
  - A test asserts that `domain.FullPermissions` is unchanged after a read-only call by root.
- **P** Every MCP caller is handled as a `member` carrying `effective`, owners included. Owner-only operations are therefore never reachable through MCP.
- **P** The tool middleware, in this order, for every `tools/call`:
  1. tool lookup (unknown or not listed for this credential → `isError`);
  2. argument validation;
  3. for every tool except `list_workspaces`, `AuthenticateUserForWorkspace` on the `workspace_id`;
  4. permission pre-check of the tool's pairs against `effective`, including pairs that depend on arguments;
  5. rate limits (§5.7);
  6. a new `AuditRecord`;
  7. the service call with the enriched context;
  8. outcome mapping and audit `Finalize`.
- **P** Each tool declares its `(resource, verb)` pairs, and a `send` or `destructive` tool must declare a write.
  - The pre-check is defence in depth where the service checks too. It is the only barrier where it does not, and those cases are listed in the implementation inventory (§12).
  - An integration test proves every pair against the real services (§12).

### 5.4 Workspace scoping (D2, D3)

- Every tool except `list_workspaces` requires `workspace_id`, with no default.
- **P** A workspace is reachable when all of these hold:
  - it is in the grant's selected workspaces (OAuth), or the key has a membership row in it;
  - the user is a member or root;
  - its MCP switch is on.

  Any failure, including a nonexistent workspace, gives one not-found tool error. Grant validity is checked at token resolution, as a `401` (§5.2).
- **P** Consent pre-checks every workspace the user is a member of (D3). For root it lists every workspace but pre-checks only those with a membership row.
- **P** Every tool result names the workspace (`workspace: {id, name}`).
- **F/P** `tools/list` varies by authorization. It lists a tool when at least one reachable workspace grants the tool's static pairs (argument-dependent pairs are checked at call time).
  - `cacheScope: "private"`, `ttlMs: 60000`, deterministic order, and `listChanged: false` while `subscriptions/listen` is out of v1.
  - Its cost is bounded by the grant's selected workspaces.
  - The SDK spike confirms that the SDK can do this (Q7).

### 5.5 Workspace switch (D7)

- **P** `WorkspaceSettings.MCP *MCPSettings{Disabled bool}`. nil means enabled, so no backfill is needed.
- **P** A new owner-only route `workspaces.setMCPSettings`, `RestrictedInDemo` like its `workspaces.set*` neighbours, and audited. `UpdateWorkspace` preserves the field, and a `workspaces.update` carrying `mcp` changes nothing.
- **P** Disabling hides the workspace from every MCP credential without revoking anything. API keys keep working on `/api/*`.

### 5.6 Audit (D19)

- **P** Every non-read MCP tool writes a row, refusals included, under its API route's action name.
  - A route in `AuditedActions` uses that spec.
  - A route in `AuditExcludedRoutes` uses a separate `MCPAuditedActions` table (category, target type) that the middleware never reads. A test asserts that the table is a subset of `AuditExcludedRoutes`, disjoint from `AuditedActions`, and that every non-read tool maps to exactly one of the two.
  - `auditLogs.actions` lists the MCP-only actions.
- **P** Each row carries:
  - the IP, user agent and request ID of the `/mcp` request;
  - metadata `channel: "mcp"`, `mcp_tool`, and `oauth_client` (Lot 2);
  - `AuditAuthAPIKey`, or a new `AuditAuthOAuth` in Lot 2.

  Metadata by tool:

  | Tool kind | Metadata |
  | - | - |
  | `send` | recipient email(s) or audience (list, segment), template or notification ID |
  | Subscription change | email, list, previous and new status |
  | Contact upsert or custom event | email |
  | Delete | target |

  Logs and traces never carry tool argument values (§5.10); the audit row is the one place a recipient is recorded.
- **P** Outcomes map to `Finalize` statuses:

  | Outcome | Status |
  | - | - |
  | success | 200 |
  | validation | 400 |
  | licence refusal | 402, naming the feature |
  | permission refusal | 403, naming resource and permission |
  | not found | 404 |
  | rate limit | 429 |
  | internal | 500 |
- **P** Lot 2 OAuth events are written directly by `OAuthService`, one row per workspace the grant covers. Their routes go in a new fourth list, `AuditSelfRecordedRoutes`, which the classification test accepts.

  | Event | Recorded |
  | - | - |
  | `oauth.approve` | yes |
  | `oauth.revokeGrant` | yes |
  | `oauth.removeGrantWorkspace` | yes |
  | revocation by D17 | yes, action `oauth.revokeGrant`, reason `console_inactivity` |
  | revocation on token reuse | yes, action `oauth.revokeGrant`, reason `token_reuse` |
  | `/oauth/revoke` | yes, action `oauth.revoke` |
  | `oauth.getAuthorizationRequest`, `oauth.listGrants`, `oauth.listWorkspaceGrants` | no (reads) |
  | `oauth.deny` | no (creates nothing) |
- **F** The `audit_logs` licence gate applies: a Community instance records no row (§9).

### 5.7 Rate limiting

- **P** Limits are applied in the tool middleware, and a refusal is a tool error naming the limit.

  | Namespace | Key | Limit |
  | - | - | - |
  | `mcp` | user or key | 120 calls per minute, `tools/list` included |
  | `mcp_send` | user or key | 30 per minute, for `send` tools and `write, open world` tools |
  | `mcp_auth_fail` | IP | 60 per minute, for requests to `/mcp` with an invalid credential, bounding database lookups |
  | `oauth_authorize` | IP | 30 per minute |
  | `oauth_register` | IP | 10 per hour, plus a global client cap (§7.2) |
  | `oauth_token_ip` | IP | 600 per minute |
  | `oauth_token_credential` | SHA-256 of the presented code or refresh token | 10 per minute |

  The two `oauth_token` limits are independent. The per-IP one is the bound that matters.
- **P** Hosted clients share egress addresses (Anthropic: `160.79.104.0/21`), so per-IP limits apply to every user of that client together and are set generously. Limits are per process: N API containers multiply them by N. Both facts are in the docs.
- **P** A test pins every namespace and its values against literals.

### 5.8 Replay and idempotency

- **F** Under 2026-07-28 a client re-issues a request whose stream broke.
- **P** `send_transactional` makes `external_id` **required**. The service already deduplicates on it.
- **F/P** Other `send` tools:
  - `schedule_broadcast`, `resume_broadcast`, `activate_automation` and `select_broadcast_winner` are state transitions; a replay meets the wrong state and fails. Verified for schedule, to verify for the other three.
  - `send_template_test` and `send_broadcast_to_contact` can send twice on replay. This is accepted and documented.
- **P** Deletes replayed answer not found. A create may duplicate on replay; the docs say so.
- **P** §6.2 gives `idempotentHint` per tool.

### 5.9 Tool outputs

- **P** Every result carries `structuredContent` and, as the MCP SHOULD requires, a `TextContent` holding the serialized JSON of the same object.
- **P** Editable content is returned whole and unaltered: MJML, template subject and preheader, blog post body, segment tree.
- **P** Other free-text fields (contact fields, message excerpts, timeline entries) are capped at 2 000 characters with control characters other than `\n` and `\t` removed, and a truncated field is flagged (`"truncated": true`).
- **P** Lists default to 20 items and 50 at most, with an opaque cursor. `query_analytics` returns at most 1 000 rows within a 10 s timeout, and `preview_segment` also has a 10 s timeout.
- **P** None of this stops prompt injection (§9 R1).

### 5.10 Logging hygiene

- **P** Logs and traces never contain tokens, codes, refresh tokens, authorization request IDs or tool argument values. They record the tool name, workspace, outcome and duration.

## 6. Tool catalogue v1

### 6.1 Classes and annotations

| Class | `readOnlyHint` | `destructiveHint` | `openWorldHint` | `idempotentHint` default |
| - | - | - | - | - |
| `read` | true | false | false | true |
| `write` | false | false | false (true when marked "open world") | per tool, see §6.2 |
| `send` | false | true | true | false unless §6.2 says otherwise |
| `destructive` | false | true | false | true |

- A `write` tool is marked "open world" when it can trigger an email or change what live sends emit. It then counts in `mcp_send`.
- **F** Clients may treat annotations as untrusted hints, so a confirmation is never a server-side guarantee.
- **P** Errors:
  - Credential problems are HTTP `401` (§5.2).
  - Everything else is `isError` with a stable code (`not_found`, `permission_denied`, `validation`, `licence_required`, `rate_limited`, `internal`), plus the resource and verb when relevant.
  - Permission refusals deliberately deviate from the authorization spec's `403`. Notifuse has a single scope (`mcp`), so a `403` would trigger a step-up re-authorization that can change nothing, and the refusal depends on the workspace named in the call.
- **P** Schemas use `type`, `properties`, `required`, `items` and `enum`. Whether the recursive segment tree can be expressed without `$ref` is Q8.

### 6.2 Tools

The tool names are pinned by a test against a literal list. Every row is a target; a mismatch found during implementation is corrected here first. "Idem." is the `idempotentHint` value when it differs from the class default.

| Tool | Wraps | Permissions | Class | Idem. |
| - | - | - | - | - |
| `list_workspaces` | reachable workspaces (§5.4) | — | read | |
| `get_workspace_overview` | `workspaces.get` + `lists.list`, whitelisted fields: id, name, timezone, default language, languages, website URL, custom field labels, blog and web analytics enabled, provider kinds (no IDs, no secrets), list summaries | workspace:read, lists:read | read | |
| `search_contacts` | `contacts.list` | contacts:read | read | |
| `get_contact` | `contacts.getByEmail` or `contacts.getByExternalID`, + `contactLists.getListsByContact` | contacts:read | read | |
| `count_contacts` | `contacts.count` | contacts:read | read | |
| `get_contact_timeline` | `timeline.list` | contacts:read | read | |
| `upsert_contact` | `contacts.upsert` | contacts:write | write, open world | true |
| `delete_contact` | `contacts.delete` | contacts:write | destructive | |
| `list_lists` | `lists.list`, `lists.stats` | lists:read | read | |
| `create_list` | `lists.create` | lists:write | write | false |
| `update_list` | `lists.update` | lists:write | write | true |
| `delete_list` | `lists.delete` | lists:write | destructive | |
| `subscribe_contact_to_list` | `lists.subscribe`, authenticated, with its rules (§3): a re-subscription sends a confirmation email | lists:write (+ pair pinned in implementation) | write, open world | true |
| `set_contact_list_status` | `contactLists.updateStatus`, any status on an existing membership (D14) | lists:write, contacts:write | write, open world | true |
| `remove_contact_from_list` | `contactLists.removeContact` | lists:write | destructive | |
| `list_segments` / `get_segment` | `segments.list` / `segments.get` | segments:read | read | |
| `preview_segment` | `segments.preview` | segments:read | read | |
| `create_segment` | `segments.create` | segments:write | write | false |
| `update_segment` | `segments.update` | segments:write | write | true |
| `delete_segment` | `segments.delete` | segments:write | destructive | |
| `list_templates` / `get_template` | `templates.list` / `templates.get`, which return `editor_mode` | templates:read | read | |
| `render_template` | `templates.compile` | templates:read | read | |
| `create_template` | `templates.create`, code mode (D9) | templates:write | write | false |
| `update_template` | `templates.update`, code mode. A visual template is refused with an explicit `validation` error naming the editor mode | templates:write | write, open world | true |
| `delete_template` | `templates.delete` | templates:write | destructive | |
| `list_broadcasts` / `get_broadcast` | `broadcasts.list` / `broadcasts.get` | broadcasts:read | read | |
| `get_broadcast_report` | `messages.broadcastStats`, `broadcastLinkStats`, `broadcastVariationStats` | broadcasts:read and message_history:read | read | |
| `create_broadcast` | `broadcasts.create` | broadcasts:write | write | false |
| `update_broadcast` | `broadcasts.update` | broadcasts:write | write, open world | true |
| `send_template_test` | `transactional.testTemplate` | transactional:write, templates:read; + lists:read with a list; + contacts:read with cc/bcc | send | |
| `send_broadcast_to_contact` | `broadcasts.sendToIndividual` | broadcasts:write | send | |
| `schedule_broadcast` | `broadcasts.schedule`; `when` is required, `"now"` or an RFC 3339 time | broadcasts:write | send | true |
| `select_broadcast_winner` | `broadcasts.selectWinner` | broadcasts:write | send | true |
| `pause_broadcast` | `broadcasts.pause` | broadcasts:write | write | true |
| `resume_broadcast` | `broadcasts.resume` | broadcasts:write | send | true |
| `cancel_broadcast` | `broadcasts.cancel` | broadcasts:write | destructive | |
| `delete_broadcast` | `broadcasts.delete` | broadcasts:write | destructive | |
| `list_transactional_notifications` / `get_transactional_notification` | `transactional.list` / `transactional.get` | transactional:read | read | |
| `create_transactional_notification` | `transactional.create` | transactional:write | write | false |
| `update_transactional_notification` | `transactional.update` | transactional:write | write, open world | true |
| `delete_transactional_notification` | `transactional.delete` | transactional:write | destructive | |
| `send_transactional` | `transactional.send`, `external_id` required | transactional:write | send | true |
| `search_messages` | `messages.list` | message_history:read | read | |
| `list_analytics_schemas` / `query_analytics` | `analytics.schemas` / `analytics.query`, without `webhook_deliveries` | per schema, checked against the `schema` argument | read | |
| `list_automations` / `get_automation` | `automations.list` / `automations.get` | automations:read | read | |
| `get_automation_runs` | `automations.nodeExecutions` | automations:read | read | |
| `activate_automation` | `automations.activate` (D10) | automations:write | send | true |
| `pause_automation` | `automations.pause` | automations:write | write | true |
| `delete_automation` | `automations.delete` | automations:write | destructive | |
| `list_blog_posts` / `get_blog_post` / `list_blog_categories` | `blogPosts.list` / `blogPosts.get` / `blogCategories.list` | blog:read | read | |
| `create_blog_post` | `blogPosts.create` | blog:write | write | false |
| `update_blog_post` | `blogPosts.update` | blog:write | write | true |
| `publish_blog_post` | `blogPosts.publish` | blog:write | write, open world | true |
| `unpublish_blog_post` | `blogPosts.unpublish` | blog:write | write | true |
| `delete_blog_post` | `blogPosts.delete` | blog:write | destructive | |
| `track_custom_event` | `customEvents.upsert` | contacts:write | write, open world | true |

### 6.3 Out of v1

- **Settings and access**: workspace settings, integrations and provider credentials, members, invitations, API key management, licence, setup, tasks.
- **Data plumbing**: webhook subscriptions and the `webhook_deliveries` schema; imports; file manager.
- **Content and configuration**: template blocks, blog themes, blog category management, automation creation and editing, annotations, audit logs, usage.
- **Other routes**: `segments.contacts`, `segments.rebuild`, `customEvents.list`/`get`, `inboundWebhookEvents.list`, `broadcasts.retryFailed`, `getTestResults`, `refreshGlobalFeed`, `testRecipientFeed`.
- **Protocol features**: MCP resources and prompts, `subscriptions/listen`, the tasks extension, elicitation (MRTR).
- Directory submissions.

## 7. OAuth authorization server (Lot 2)

### 7.1 Endpoints

All endpoints live on the canonical host. They are unavailable in the cases of §5.1.

**Discovery**

- `GET /.well-known/oauth-protected-resource/mcp` and `GET /.well-known/oauth-protected-resource` (RFC 9728):
  - `resource` = canonical `/mcp`;
  - `authorization_servers` = [issuer];
  - `scopes_supported` = [`mcp`];
  - `bearer_methods_supported` = [`header`].
- `GET /.well-known/oauth-authorization-server` (RFC 8414):
  - `issuer`;
  - `authorization_endpoint`, `token_endpoint`, `registration_endpoint` and `revocation_endpoint`;
  - `response_types_supported`: [`code`];
  - `grant_types_supported`: [`authorization_code`, `refresh_token`];
  - `code_challenge_methods_supported`: [`S256`];
  - `token_endpoint_auth_methods_supported`: [`none`];
  - `client_id_metadata_document_supported`: true;
  - `authorization_response_iss_parameter_supported`: true;
  - `scopes_supported`: [`mcp`, `offline_access`].

**`GET /oauth/authorize`**

The endpoint is rate-limited. It validates:

- `response_type=code`;
- `client_id`, through a CIMD fetch or a DCR lookup;
- `redirect_uri`, matched exactly except for the loopback rule, with the scheme whitelist: `https`, or `http` on a loopback host;
- `code_challenge` with `code_challenge_method=S256`, required;
- `resource`, compared canonically when present; an older client that omits it is bound to the canonical one.

Unknown scopes are ignored and `mcp` is granted. Any failure renders an error page and never redirects.

On success it stores a request with:

- an ID of 128 random bits;
- a 10-minute expiry;
- a snapshot of the client;
- `user_id` empty;
- `consumed_at` empty.

It then redirects to `/console/oauth/authorize?request=<id>`. That console route sends `Referrer-Policy: no-referrer`.

**Console calls (`/api/`)**

These calls require `type = user` with a verified session (D18), and refuse API keys.

- `oauth.getAuthorizationRequest {request_id}` binds the request to its first signed-in reader. Anyone else gets an error telling them to restart from their client.
- `oauth.approve {request_id, workspace_ids, preset | permissions}`:
  - refuses a workspace the caller is neither a member of nor root for, and a switched-off workspace;
  - validates the matrix with `UserPermissions.Validate`, limited to `AllPermissionResources` (opt-in resources such as `audit_logs` excluded);
  - atomically consumes the request;
  - creates the grant (with `resource`) and a single-use code valid 60 s;
  - returns `{redirect_url}` with `code`, `state` when the client sent one, and `iss`.
- `oauth.deny {request_id}` consumes the request and returns `{redirect_url}` with `error=access_denied`, `state` and `iss`.
- `iss` is byte-for-byte the metadata `issuer` on every authorization response, success or error. Clients compare it as an exact string, and ChatGPT then uses its stable redirect URI (§4).
- `oauth.listGrants`: the caller's grants, each with client, workspaces, preset or matrix, created at, last used at and status.
- `oauth.listWorkspaceGrants {workspace_id}`: owner only.
- `oauth.revokeGrant {grant_id}`: own grants.
- `oauth.removeGrantWorkspace {grant_id, workspace_id}`: owner only.

**`POST /oauth/token`**

The body is form-urlencoded. The endpoint never fetches CIMD; it uses the snapshot taken at authorization.

- `authorization_code` checks:
  - the `client_id` equals the code's;
  - `redirect_uri` and PKCE S256;
  - `resource` canonically;
  - single use: reusing a code revokes the grant.
- `refresh_token` checks the `client_id`, then rotates under a row lock. Rotation revokes the previous pair's access token.

  The grace rule keeps a single surviving chain:
  - a replaced refresh token presented again within 60 s of its rotation invalidates the pair issued at that rotation and issues a new pair;
  - a replaced or invalidated token presented outside that window revokes the grant.
- A revoked grant, including by D17, answers `invalid_grant`.
- **P** A refresh token is issued whether or not `offline_access` was requested.

**`POST /oauth/register`** (DCR, RFC 7591, JSON)

- Public clients only: `token_endpoint_auth_method` is `none` or absent.
- Redirect URIs follow the scheme whitelist.
- `client_name` is at most 100 characters.
- At the global cap (§7.2) it answers `503` with an RFC 7591 error, and the MCP tab shows it.

**`POST /oauth/revoke`** (RFC 7009)

- `client_id` is required and must match the token's client.
- Revoking an access token revokes that token; revoking a refresh token revokes the grant.

### 7.2 Clients

- **P** CIMD:
  - `client_id` is an `https` URL with a path.
  - It is fetched with a dedicated client built on `safehttpclient`'s `DialContext`, with a 5 s timeout and `CheckRedirect` returning `http.ErrUseLastResponse`, so any `3xx` is a failure.
  - The body is capped at 64 KB and must be JSON containing `client_id` (equal to the URL), `client_name` and `redirect_uris`.
  - Authentication method:
    - When the plural `token_endpoint_auth_methods_supported` is present, the document is accepted if it contains `none`, and the singular field is ignored. ChatGPT's document lists `["none", "private_key_jwt"]` with a singular `private_key_jwt`; since this server advertises only `none`, the intersection is `{none}`.
    - When only the singular `token_endpoint_auth_method` is present, it must be `none` (Claude Code's document).
    - When neither is present, `none` is assumed.
    - Any other combination is refused.
  - Cache: positive entries follow HTTP cache headers within 5 min–24 h; a failure is cached for 5 minutes.
  - The fetcher is injectable for tests.
- **P** Client records (DCR and CIMD) share one global cap of 10 000 rows.
  - A client that never led to a grant is purged after 24 h.
  - A client that has had a grant is purged only after 365 days without use and with no non-revoked grant. ChatGPT reuses one DCR client for the life of a connection and fails with `invalid_client` if it disappears (§4).
- **P** The consent page shows:
  - the signed-in account;
  - the client name;
  - the redirect host;
  - for CIMD, the `client_id` host;
  - for DCR, "unverified application";
  - a warning when every redirect URI is loopback.

  Consent is always shown.
- **P** Every `/console` response carries `Content-Security-Policy: frame-ancestors 'none'` and `X-Frame-Options: DENY`.
- **P** The console navigates only to `http(s)` redirect URLs. The return path after sign-in is restricted to `/console/oauth/authorize?request=…`.

### 7.3 Tokens

- **P** Access tokens are opaque (`nfmcp_at_` + 32 random bytes), hashed with SHA-256, and valid 1 h.
- **P** Refresh tokens (`nfmcp_rt_…`) are hashed and rotated, and expire after 90 days without use.
- **P** Token resolution on `/mcp` checks, in turn:
  - the hash and the expiry;
  - the grant: not revoked; user exists with `Type == user`; `resource` equal to the current canonical resource; D13.

  Any failure is a `401`. A D13 failure first revokes the grant (D17).

### 7.4 Grants

- **P** A grant holds:
  - user and client, with a snapshot of the client name;
  - `resource`;
  - the selected workspaces;
  - the preset, or a custom matrix when licensed;
  - created at, last used at (written at most once per 5 minutes), revoked at, revocation reason.
- **P** D13 reference date = the later of `users.last_console_signin_at` and the grant's creation. The grant is revoked when that date is more than 90 days old.
  - The column is new and nullable.
  - It is updated at every successful magic-code or OIDC sign-in, and never by `oauth.approve` or `user.rootSignin`, which is programmatic.
  - With 30-day sessions, a user active in the console signs in at least every 30 days.
- **P** Several grants may coexist for one user and client. Reconnecting never revokes anything automatically. To change workspaces or permissions, connect again and revoke the old grant from the list.
- **P** Lifecycle:
  - A console logout does not revoke grants.
  - Removing a member deletes that workspace from the member's grants; re-adding them does not restore it.
  - The periodic job (§7.5) revokes a grant with no workspace left, or with neither a valid refresh token nor a live code.
  - Deleting a user or a workspace cascades.
- **P** Licence: without `rbac`, `oauth.approve` accepts only the two presets; with it, any valid matrix. A stored matrix stays enforced after the licence lapses.

### 7.5 Data model and periodic job (system database, new major)

| Table | Columns |
| - | - |
| `oauth_clients` | `client_id` (pk), `kind` (`dcr`/`cimd`), `client_name`, `redirect_uris` jsonb, `metadata` jsonb, `created_at`, `last_used_at`, `fetched_at`, `cache_expires_at`, `fetch_failed_until` |
| `oauth_authorization_requests` | `id` (pk), `client_id`, `client_snapshot` jsonb, `redirect_uri`, `state`, `code_challenge`, `resource`, `user_id` (nullable), `consumed_at`, `expires_at` |
| `oauth_authorization_codes` | `code_hash` (pk), `grant_id`, `client_id`, `redirect_uri`, `code_challenge`, `resource`, `expires_at`, `used_at` |
| `oauth_grants` | `id`, `user_id` (fk users, cascade), `client_id`, `client_name`, `resource`, `preset`, `permissions` jsonb (custom matrix only), `created_at`, `last_used_at`, `revoked_at`, `revoked_reason` |
| `oauth_grant_workspaces` | `grant_id` (fk, cascade), `workspace_id` (fk workspaces, cascade) |
| `oauth_tokens` | `token_hash` (pk), `grant_id` (fk, cascade), `client_id`, `kind` (`access`/`refresh`), `expires_at`, `rotated_at`, `replaced_by_hash`, `invalidated_at` |
| `users` | new column `last_console_signin_at timestamptz NULL` |

**P** A background worker modelled on `AuditLogPurgeWorker` runs every hour. It:

- revokes grants per §7.4 and per D13;
- purges expired requests, codes and tokens;
- purges clients per §7.2.

## 8. Console

- **P** The consent page lives at `/console/oauth/authorize`, outside the workspace layout, and requires sign-in. Its `request` param goes through `validateSearch`. The page shows:
  - the client identity;
  - workspace checkboxes, pre-checked per §5.4, with switched-off workspaces greyed out and the reason shown;
  - permissions: the radio "Read only / Read and write" without the licence, or the matrix with preset shortcuts with it;
  - Approve and Deny buttons.
- **P** A Workspace Settings tab, "AI assistants (MCP)":
  - The canonical MCP URL with a copy button.
  - Lot 1 shows setup snippets only for clients verified by hand: Claude Code with a header, and Cursor. It also shows:
    - a link to API key management;
    - the warning of D20 (a full-access key gives the assistant every right, sending and deleting included);
    - the note that claude.ai and ChatGPT need Lot 2.
  - Lot 2 adds the claude.ai and ChatGPT snippets, the OAuth status ("available", or "unavailable" with the reason, §5.1), "Your connections" and, for owners, "All connections to this workspace".
  - The owner switch (D7).
- **P** Lot 2 also adds an account-level page, "My AI connections", in the user menu, so a user sees every grant they hold without opening each workspace (§9 R9).
- **P** The docs say consent runs on the `API_ENDPOINT` host; a user signed in on a custom domain signs in again there.
- Every string is wrapped with Lingui.

## 9. Accepted risks

- **R1 (D5)**: A prompt injection in content the model reads (contact fields, message content, blog content) can push workspace data to an attacker-chosen address. It can do so through `send` tools, or through `write, open world` tools: `update_template` on a template a live notification uses changes every later send.
  - No preset or matrix separates drafting from sending (`transactional:write`, `broadcasts:write`). The "Read only" preset (Lot 2) is the only hard barrier.
  - Annotations are hints.
  - `mcp_send` bounds the call rate but not the reach of one `update_template`.
  - Audit rows exist only under `audit_logs`.
- **R2 (D6)**: An irreversible action on a wrong ID or workspace. Mitigations: `destructiveHint`, the mandatory `workspace_id` echoed in results, audit (licensed).
- **R3 (D2, D3)**: Acting in the wrong workspace, or an injection read in workspace A driving an action in workspace B of the same grant. Mitigations: no default workspace, the workspace named in results, the root pre-check rule. The cross-workspace injection remains.
- **R4**: Contact data reaches the LLM provider of the client. Mitigations: the workspace and instance switches, a note in the docs.
- **R5 (Lot 2)**: Mass client registration. Mitigations: the per-IP limit (needs a trusted proxy; CORS `*` lets a web page spread registrations across its visitors' addresses), the global cap, the purges. At the cap, new connections fail until the purge frees rows.
- **R6 (Lot 2)**: Consent phishing with a registered client. Mitigations: "unverified application", the redirect host, the loopback warning. Binding `state` is the client's job.
- **R7 (D14)**: An assistant can reactivate a contact who withdrew consent, as an API caller can. Audited under D19 when licensed.
- **R8 (D20)**: In Lot 1 on an instance without `rbac`, every key is full access. A connected assistant can send and delete, and without `audit_logs` leaves no audit trace. API keys are also unaffected by D13.
- **R9 (D18)**: A stolen console session can approve a durable grant. That grant survives the victim's logout, and D13 never stops it while the victim keeps signing in. The trace is "My AI connections", the MCP tab, and the audit log when licensed.
- **R10**: After a departure handled at the identity provider, an OAuth assistant keeps working up to 90 days after the person's last console sign-in (D13).
- **R11**: Per-IP OAuth limits are shared by every user of a hosted client, and every limit is multiplied by the number of API containers.

## 10. Delivery

**Prerequisites before Lot 1 code:**

- **Q7, SDK spike**, a throwaway server outside the repository:
  - per-request `tools/list` filtering and `cacheScope`;
  - `isError` results and the mapping to audit statuses;
  - stateless mode serving a 2025-11-25 client (Claude Code) and the MCP Inspector;
  - `PropagateRequestCancellation` off by default.
- **Q8, tool schemas**: the input and output schema of every tool, the error codes, the cursor format, and what `list_workspaces` returns.

**Lot 1:**

- the SDK dependency and `/mcp` with API keys;
- the auth-layer changes of §5.3 (OAuth case excluded) with the cache fix;
- the whole catalogue (D20), with the tool middleware, audit (D19), rate limits, replay rules and output rules;
- the workspace and instance switches, MCP off in demo, the `.well-known` 404s, CORS;
- the MCP tab;
- a docs page in `../docs`;
- a changelog Feature entry. The cache fix is folded into it with no `Fix` line, since no shipped caller reaches the bug (`CLAUDE.md`).

No migration; the WIP version it joins is chosen at implementation time.

**Lot 2:**

- the OAuth server, the consent page, connected apps and "My AI connections";
- the periodic worker;
- the tables and the `users` column in a new `vN.go`;
- a docs update;
- a changelog Feature entry announcing that upgrading opens client registration and outbound CIMD fetches, and how `MCP_ENABLED` turns them off.

VERSION becomes N.0 in the commit that adds `vN.go`.

**Not in `openapi/`**: MCP is documented on the docs page.

**Manual checks**:

- Lot 1: MCP Inspector; Claude Code and Cursor with a header; what claude.ai shows when given the URL without a header, written into the docs.
- Lot 2: Claude Code with OAuth (CIMD); claude.ai on a public instance; ChatGPT (CIMD, then DCR); Cursor (DCR).

## 11. Open questions

- Q1: Tool footprint. Measure `tools/list`; above roughly 12k tokens, merge read tools rather than add `action` parameters.
- Q3: Token lifetimes (1 h access, 90 days idle refresh, 60 s rotation grace).
- Q5: Later, a Claude directory listing for Notifuse Cloud with the URL pattern `^https://[a-z0-9-]+\.notifuse\.cloud/mcp$`, CIMD first.
- Q7: The SDK spike (§10).
- Q8: Tool schemas (§10), including the recursive segment tree. Without `$ref` it may need a looser rule for that one argument.
- Q9: The Mistral and n8n requirements. Everything else listed in v3 was collected (Appendix A) and folded into v4.

Settled: Q2 → D9, Q4 → D10, Q6 → D16.

## 12. Implementation outline

Rules for every test below: lists (tool names, namespaces and limits, MCP-only audit actions, annotation values) are compared against literals written in the test, never against the registry or constant they protect. Every number this spec states (120, 30, 60 s, 90 days, 1 h, 2 000, 1 000) is pinned by a literal assertion.

### Lot 1

- `go.mod`: add `github.com/modelcontextprotocol/go-sdk` v1.7.x.
- `config/config.go`: `MCP_ENABLED`, `MCP_ALLOWED_ORIGINS`.
  - Tests in `config/config_test.go`: defaults, override, comma-separated parsing.
- `internal/domain/mcp.go`: `MCPSettings`, classes and annotation table, `MCPCaller`, preset resolution.
  - Tests in `internal/domain/mcp_test.go`:
    - annotation values per class, as literals;
    - each preset leaves `domain.FullPermissions` unchanged;
    - `read_only` removes every write;
    - a matrix intersects.
- `internal/domain/workspace.go`: `WorkspaceSettings.MCP`.
  - Tests in `workspace_test.go`: JSON round trip with and without the field.
- `internal/domain/audit_actions.go`: `workspaces.setMCPSettings` and `MCPAuditedActions`.
  - Tests in `audit_actions_test.go`:
    - classification still holds;
    - `MCPAuditedActions` is a subset of `AuditExcludedRoutes` and disjoint from `AuditedActions`, with literal contents.
- `internal/service/auth_service.go`: `MCPCaller` for API keys, reachability, `SetActor` order, demotion with a fresh map, cache fix.
  - Tests in `auth_service_test.go`:
    - an owner and a root with read-only effective permissions cannot write;
    - `FullPermissions` is unchanged after root goes through MCP;
    - an unreachable workspace (not selected, switched off, member removed, nonexistent) gives one answer;
    - A, B, A on one context returns A's membership;
    - the audit actor of a demoted owner is `owner`;
    - the plain path is unchanged;
    - no OAuth `MCPCaller` is built anywhere in Lot 1.
- `internal/service/workspace_service.go`: `SetMCPSettings` (owner only) and preservation in `UpdateWorkspace`.
  - Tests in `workspace_service_test.go`: a member is refused; the setting is stored; a `workspaces.update` carrying `mcp` changes nothing.
- `internal/http/workspace_handler.go`: the route, `RestrictedInDemo`.
  - Tests in `workspace_handler_test.go`: demo refusal, owner-only.
- `internal/mcp/server.go`, `caller.go`, `middleware.go`, `errors.go`, `output.go`, and `tools_*.go` per area.
  - Tests in `internal/mcp/*_test.go` with mocks:
    - the literal tool list;
    - per tool: pairs, class, `idempotentHint`, required `workspace_id`;
    - middleware order: authentication before the pre-check, the pre-check before the service;
    - argument-dependent pairs;
    - `isError` codes;
    - pagination bounds;
    - output caps and the truncation flag, with editable fields untouched;
    - the serialized JSON in `TextContent`;
    - an audit row for every non-read tool, refusals included, with metadata per class;
    - `external_id` required;
    - `when` required;
    - a visual template refused by `update_template`.
- `internal/http/mcp_handler.go`: credential resolution and `401`s (revoked key, session token, invitation token, malformed), host check, `/mcp/`, `.well-known` 404s, `MCP_ENABLED`, demo, the canonical URL function.
  - Tests in `mcp_handler_test.go`: one per rule, plus an opaque token refused on `/api/*`, and `SystemCallKey` never set.
- `internal/http/middleware/cors.go` and the `/mcp` handler: the MCP path policy and `Origin` validation.
  - Tests in `cors_test.go` and `mcp_handler_test.go`:
    - on `/mcp`, a foreign `Origin` gets `403` before credentials are read;
    - an allowed `Origin` is reflected;
    - no `Origin` passes;
    - `*` on `/oauth/*` and `.well-known`, without credentials, even with `CORS_ALLOW_ORIGIN` set;
    - the nine allowed headers;
    - the exposed header;
    - other paths unchanged.
- `internal/app/app.go`: mount `/mcp` inside the mux; rate-limit namespaces.
  - Tests in `app_test.go`: the literal namespace and limit list.
- Console:
  - `console/src/components/settings/MCPSettings.tsx` (new), the tab entry in `console/src/pages/WorkspaceSettingsPage.tsx`, and `console/src/services/api/mcp.ts` (new).
  - Vitest tests: owner and member views, the switch, the canonical URL in snippets, the D20 warning, the Lot 1 client list.
- `tests/integration/mcp_api_key_test.go`:
  - API key → `tools/list` → `upsert_contact` → `get_contact`;
  - **table-driven over the literal tool list**: for each tool and each declared pair, a key whose permissions lack that pair is refused by the real services;
  - switched-off workspace;
  - `send_transactional` replayed with the same `external_id` sends once;
  - an audit row with `channel: mcp` for a write under the licdev licence;
  - MCP off in demo.
- Implementation inventory, written into this spec before Lot 1 is merged: every service method a tool calls, with how it authenticates. Any method that does not use `AuthenticateUserForWorkspace` is named, and its tool relies on the pre-check alone.

### Lot 2

- `internal/migrations/vN.go`: §7.5.
  - Tests in `vN_test.go`: idempotent DDL, system database only, cascades.
- `internal/domain/oauth.go`: client metadata validation, redirect matching and scheme whitelist, PKCE S256, D13 reference date, preset and licence rule.
  - Tests in `oauth_test.go`:
    - loopback positive and negative (other host, other path, `https` loopback, IPv6);
    - `javascript:` and `data:` refused;
    - RFC 7636 vectors;
    - a missing method refused;
    - D13 at 89 and 91 days, with NULL, and with a recent grant.
- `internal/domain/user.go`: `LastConsoleSigninAt`. `internal/repository/user_postgres.go`: read and write.
  - Tests in `user_postgres_test.go`.
- `internal/repository/oauth_postgres.go`.
  - Tests in `oauth_postgres_test.go`: CRUD, atomic consumption, row lock on rotation, cascades, purge queries.
- `internal/service/oauth_service.go`.
  - Tests in `oauth_service_test.go`:
    - approve: a non-member workspace, a switched-off workspace and an invalid matrix are refused; approve replay and deny replay are refused; approve does not write `last_console_signin_at`;
    - the request bound to its first reader;
    - code single use, with reuse revoking the grant;
    - PKCE and `resource` (canonical comparison, missing);
    - `client_id` mismatch at token, refresh and revoke;
    - rotation: the single-chain grace rule, two concurrent refreshes, reuse after grace revoking the grant, the previous access token revoked;
    - D17 revocation and its audit reason;
    - CIMD: no redirect followed, `3xx` and non-JSON are failures, private IP, size cap, `client_id` mismatch, cache bounds, negative cache, no fetch at the token endpoint;
    - CIMD authentication methods with literal fixtures: ChatGPT's (plural with `none`, singular `private_key_jwt`) accepted; Claude Code's (singular `none`) accepted; plural without `none` refused; singular `private_key_jwt` alone refused;
    - client purge at 24 h for a client without a grant, and at 365 days for one that had a grant;
    - DCR validation, the global cap answer;
    - licence: matrix refused without `rbac`, presets accepted, matrix kept after lapse;
    - member removal and re-add;
    - a grant left with no workspace;
    - audit rows: one per workspace for approve and revoke, `AuditSelfRecordedRoutes`.
- `internal/service/oauth_worker.go`, modelled on `AuditLogPurgeWorker`.
  - Tests in `oauth_worker_test.go`: each revocation and purge rule.
- `internal/service/auth_service.go`: the OAuth case.
  - Tests: a user loaded from the grant; an `api_key` account refused; mixed context refused.
- `internal/service/user_service.go`, `oidc_service.go`: update `last_console_signin_at`; `RootSignin` does not.
  - Tests: each path.
- `internal/service/workspace_service.go`: the `RemoveMember` hook.
  - Tests: the workspace leaves the member's grants.
- `internal/domain/audit_actions.go`, `audit_log.go`: `AuditSelfRecordedRoutes`, `AuditAuthOAuth`.
  - Tests: classification with four lists, literal contents.
- `internal/http/oauth_handler.go`.
  - Tests in `oauth_handler_test.go`:
    - metadata documents (PRM without `offline_access`);
    - form-urlencoded;
    - `invalid_grant`;
    - `iss` and `state`, with `iss` equal byte for byte to the metadata `issuer` on success and on error;
    - the error page with no redirect;
    - `WWW-Authenticate` with `resource_metadata` and `scope`;
    - console endpoints refuse API keys;
    - OAuth unavailable on `http` or a path in `API_ENDPOINT`.
- `internal/http/root_handler.go`: `frame-ancestors`, `X-Frame-Options`, and `Referrer-Policy` on the consent route.
  - Tests in `root_handler_test.go`.
- `internal/app/app.go`: OAuth routes and the worker.
  - Tests in `app_test.go`.
- Console:
  - `console/src/pages/OAuthAuthorizePage.tsx` and its route in `console/src/router.tsx`;
  - connected apps in `MCPSettings.tsx`;
  - `console/src/pages/AIConnectionsPage.tsx` and its user-menu entry.
  - Vitest tests:
    - `validateSearch`;
    - radio without the licence, matrix with it;
    - root pre-check and greyed-out workspaces;
    - a non-`http(s)` redirect not followed;
    - the deny path;
    - owner removal;
    - "My AI connections" lists grants across workspaces.
- `tests/integration/mcp_oauth_test.go`:
  - the full DCR flow → authorize with a console session → code → `tools/call` → refresh rotation → revoke → `401`;
  - an owner with "Read only" refused on every `write`, `send` and `destructive` tool (table-driven);
  - a refresh token presented as a bearer on `/mcp`, and an access token presented to `/oauth/token`, both refused;
  - D13 at 91 days end to end: `401`, then `invalid_grant`;
  - CIMD through the injectable fetcher.

### Final step: run the tests

- `make test-domain`, `make test-service`, `make test-repo`, `make test-http`, `make test-migrations` (Lot 2).
- `go test -race ./internal/mcp ./config ./internal/app ./pkg/...`
- Integration:
  - `docker compose -f tests/compose.test.yaml up -d`;
  - then only the new tests: `INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestMCP'`.
- `cd console && npm test`.

## Appendix A — Sources collected for Q9 (2026-11-13)

Collected during the review of v3. Every change below is folded into the v4 body (§4, §5.1, §7.1, §7.2, §10, §11, §12); this appendix keeps the sources and the reasoning.

### A.1 Sources

- MCP 2026-07-28: [Security Considerations](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/security-considerations), [Authorization Server Discovery](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/authorization-server-discovery), [Streamable HTTP](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http), [Caching](https://modelcontextprotocol.io/specification/2026-07-28/server/utilities/caching), [Pagination](https://modelcontextprotocol.io/specification/2026-07-28/server/utilities/pagination).
- OpenAI: [Apps SDK — Authentication](https://developers.openai.com/apps-sdk/build/auth).
- Cursor: [MCP docs](https://cursor.com/docs/context/mcp).
- Claude Code's CIMD document: `https://claude.ai/oauth/claude-code-client-metadata`.

### A.2 Findings that change the v3 body

- **A1. `/mcp` must validate `Origin`.**
  - Source (Streamable HTTP, Security): "Servers MUST validate the `Origin` header on all incoming connections to prevent DNS rebinding attacks. If the `Origin` header is present and invalid, servers MUST respond with HTTP 403 Forbidden."
  - v3 §5.1 gives `/mcp` `Access-Control-Allow-Origin: *`, which contradicts this.
  - Hosted clients (claude.ai, ChatGPT) call from their servers and send no `Origin`.
  - **v4 proposal**:
    - On `/mcp`, a request carrying an `Origin` header outside an allowlist gets `403`. The allowlist is the canonical origin plus `MCP_ALLOWED_ORIGINS` (new, empty by default).
    - CORS on `/mcp` reflects allowed origins only.
    - `/oauth/*` and the `.well-known` documents keep `*`.
    - The docs tell MCP Inspector users to add its origin, or to use its proxy mode.
    - The SDK spike checks whether v1.7 already validates `Origin`.
- **A2. ChatGPT's CIMD document would be refused by v3 §7.2.**
  - Source (OpenAI): ChatGPT's production CIMD document publishes `"token_endpoint_auth_methods_supported": ["none", "private_key_jwt"]` and the legacy singular `"token_endpoint_auth_method": "private_key_jwt"`. ChatGPT selects a method from the intersection with the authorization server's list.
  - Authorization servers that read the plural field "should accept any method in the intersection … They must reject methods outside the intersection".
  - v3 requires the singular field to be `none` or absent, so it would reject ChatGPT.
  - **v4 proposal**: when the plural field is present, accept the document if `none` is in it and ignore the singular field. When only the singular is present, require `none` (Claude Code's document: `"token_endpoint_auth_method": "none"`). Since the authorization server advertises only `none`, the intersection is `{none}` and ChatGPT uses it.
- **A3. ChatGPT reuses a DCR client for the life of a connection.**
  - Source (OpenAI): "ChatGPT runs DCR once per MCP server connection, then keeps and reuses the registered OAuth client … If your authorization server expires, deletes, or replaces either credential, users and reviewers may receive an `invalid_client` error."
  - v3 §7.2 purges a client unused for 30 days with no non-revoked grant, which can break an idle ChatGPT connection.
  - **v4 proposal**: purge after 24 h a client that never led to a grant. Purge a client that has ever had a grant only after 365 days without use. The global cap stays the protection.
- **A4. ChatGPT and Cursor need Lot 2 or DCR.**
  - Source (OpenAI): ChatGPT "does not support machine-to-machine OAuth grants … nor can it present custom API keys". ChatGPT therefore works only with Lot 2's OAuth, which v3 §8 already says.
  - Source (Cursor): headers with `${env:NAME}` interpolation for a key (Lot 1). DCR as a public client. CIMD is not mentioned, so Cursor relies on DCR in Lot 2. Its static-OAuth callbacks are `https://www.cursor.com/agents/mcp/oauth/callback` and `http://localhost:8787/callback`; the second is covered by the loopback rule.
  - **v4 proposal**: §4 gains these facts. DCR stays required (it is not optional while Cursor lacks CIMD).
- **A5. A path in `API_ENDPOINT`.**
  - Source (Authorization Server Discovery): for an issuer with a path, clients look for metadata by path insertion at the host root (`https://host/.well-known/oauth-authorization-server/<path>`).
  - A Notifuse served under a path prefix behind a proxy does not own the host root.
  - v3 §5.1's refusal of OAuth when `API_ENDPOINT` has a path is therefore justified. **v4**: cite this source and close that part of Q9.
- **A6. `iss` on every authorization response, compared exactly.**
  - Source (OpenAI): "Return `iss` in every successful and error authorization response. Its value must exactly match the metadata `issuer`; clients use exact string comparison and do not normalize trailing slashes, paths, ports, or casing."
  - With `iss` supported, ChatGPT uses the stable redirect URI `https://chatgpt.com/connector_platform_oauth_redirect`. v3 already returns `iss` on approve and deny and uses one canonical issuer.
  - **v4**: add the citation, and a test that the `iss` string equals the metadata `issuer` byte for byte.

### A.3 Findings that confirm v3

- One JSON-RPC request per POST ("The body of the HTTP POST MUST be a single JSON-RPC request or …"). One tool call per HTTP request, hence one audit record per request (§5.6).
- `Mcp-Method` and `Mcp-Name` are required, and a mismatch answers `400` with `HeaderMismatch`; the SDK is expected to handle it, which the spike verifies.
- Security Considerations:
  - every authorization server endpoint over HTTPS;
  - redirect URIs `localhost` or HTTPS;
  - exact redirect matching;
  - no automatic redirect to an untrusted URI;
  - CIMD SSRF care, an extra warning for localhost-only redirects, and the redirect hostname shown at consent;
  - refresh rotation for public clients;
  - tokens validated as issued for this server.

  All are in v3 §5.1, §7.1, §7.2 and §7.3.
- Caching: `ttlMs >= 0` is required on complete results, and `cacheScope: "private"` means user-specific data that shared caches must not serve to other users, as v3 §5.4 assumes.
- Claude Code's CIMD document: `client_name` "Claude Code", `redirect_uris` `http://localhost/callback` and `http://127.0.0.1/callback`, `token_endpoint_auth_method` `none`. This matches v3's loopback rule.

### A.4 Still open

- Mistral and n8n requirements were not collected.
- The OpenAI plans that include developer mode differ between third-party sources; the docs page should link to OpenAI rather than list plans.
