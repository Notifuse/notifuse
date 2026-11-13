# Test coverage of critical paths — plan

## Goal

Codecov reports 89.05 % on `dev` (unit tests only; the integration upload is
commented out in `.github/workflows/go.yml`). The uncovered lines in the critical
code are mostly refusal and recovery branches that no test exercises. This plan
adds tests for those branches in auth / security, sending and automations.

**No production code changes.** If a test exposes a defect, it is reported, not
fixed in this PR (unless the fix is trivial and approved).

Out of scope (not critical, or not reachable in production): `internal/app/app.go`,
`demo_*`, `pkg/tracing`, `llm_service*`, `config/config.go`, repository
"failed to get database connection" / scan-error branches.

### Not reachable in production (reported, not tested)

- `internal/service/broadcast/message_sender.go` (direct sender) is only built by
  `Factory.CreateMessageSender` when `useQueueSender` is false, and `app.go` always
  passes `true`. So `ErrCodeCircuitOpen` is never returned to
  `BroadcastOrchestrator.Process`, and the circuit-breaker pause block there
  (`orchestrator.go`, "Circuit breaker triggered - pausing broadcast") is dead code,
  as is the deferred circuit-open skip earlier in `Process`. `mintIdentifyToken` in
  `message_sender.go` stays live (called from the queue sender).
- `EmailQueueWorker.SetCallbacks` is never called outside tests, so the
  `onEmailSent` / `onEmailFailed` branches are dead too.
- `!token.Valid` after a nil error from `jwt.ParseWithClaims` cannot happen with
  golang-jwt v5 (`smtp_bridge_handler.go`, `auth_service.go`, `middleware/auth.go`);
  these defensive lines stay uncovered.
- The post-`Verify` `idToken.Issuer` check in OIDC *is* reachable: go-oidc accepts
  `iss = "accounts.google.com"` for the `https://accounts.google.com` provider, and
  `oidc_service.go` then rejects it. The fake IdP cannot serve that issuer, so the
  branch stays untested here.
- In `Process`, the three "test phase complete but winner already selected" branches
  cannot be reached: a winner known before the loop switches the phase up front, and
  the refresh at the loop top re-checks right after reassigning the broadcast. The
  "winner picked during the run" case is already covered by
  `TestOrchestrator_Process_WinnerPickedDuringTheRun…`.

### Possible defect found (reported, not fixed here)

- `EmailQueueWorker.processEntry`: when `MarkAsSent` fails after the provider accepted
  the email, the worker returns nil without writing message history; the row stays
  `processing` and `emailQueueStuckProcessing` hands it back after 2 minutes, so the
  email is likely sent a second time. A fix changes production behaviour and needs
  its own PR.

## P1 — Security and authentication

### 1. SMTP bridge — `internal/service/smtp_bridge_handler.go`

Test file: `internal/service/smtp_bridge_handler_refusals_test.go`.

`Authenticate`:
- **Rate limit**: policy of 1 attempt/min; a first bad attempt consumes it, the
  second attempt — even with a valid API key — is refused with
  "rate limit exceeded", and the account lookup is never made (auth service nil).
- **Success resets the limiter**: policy of 1 attempt/min; two successive valid
  authentications both succeed (the second would be refused without `Reset`).
- **Non-HMAC algorithm** (`alg: none` token, built with
  `jwt.UnsafeAllowNoneSignatureType`) is refused; no account lookup.
- **Expired token** is refused; no account lookup (extra assertion on the same
  branch as `Authenticate_InvalidToken`).
- The "success resets" test builds its own auth service expecting `GetUserByID` twice.
- **Email mismatch**: valid API-key token for `a@x`, username `b@x` → refused with
  "email does not match token"; no account lookup.

`HandleMessage`:
- **Missing `workspace_id`** → "workspace_id is required"; no workspace lookup, no send.
- **Workspace access denied** (`GetUserWorkspace` error) → refused, `SendNotification`
  never called.
- Defensive nil check (`GetUserWorkspace` mock returns nil, nil; the real repository
  returns `ErrUserNotInWorkspace` instead) → refused, no send.
- **Message-ID as idempotency key**: header `Message-ID: <abc@host>` and no
  `external_id` in JSON → `params.ExternalID == "abc@host"`; with `external_id` in
  JSON, the JSON value wins.

### 2. OIDC — `internal/service/oidc_service.go`

Test file: `internal/service/oidc_fail_closed_test.go` (with the fake IdP of
`oidc_idp_test.go`, which gains a counter of `/token` hits).

`HandleCallback`:
- **`?iss=` mismatch (RFC 9207)** → "oidc issuer mismatch", and the token endpoint
  is never called.
- **Matching `?iss=`** → proceeds (happy path still passes with `Iss` set).
- **Token response without `id_token`** → "missing id_token", no user lookup.

`resolveOrProvisionUser` (fail-closed branches):
- Linked identity found but user load fails → error, no email lookup.
- Federated lookup fails with a non-not-found error → error, no email lookup, no create.
- Bridge: user found by email and already linked with the **same** sub → returns the
  user, no `Create` on federated identities.
- Bridge: `GetByUserAndIssuer` fails with a non-not-found error → error, no link.
- `GetUserByEmailInsensitive` fails with a non-not-found error → error, no create.
- JIT enabled with an empty allowlist → `ErrOIDCAccountNotProvisioned`, no create.
- JIT create race: `CreateUser` returns `ErrUserExists` → re-fetch, link the
  **winner's** user ID, return the winner.
- JIT create race, re-fetch fails → error, no link.
- JIT create fails with another error → error, no link.
- `linkIdentity`: federated `Create` fails with a non-duplicate error → error.
- `mintSession`: session creation fails → error, no token.
- `OpenFlowState`: empty input, and authentic ciphertext whose plaintext is not JSON
  → error (garbled ciphertext is already covered).

### 3. Auth tokens — `internal/service/auth_service.go`

Test file: `internal/service/auth_service_secret_test.go`.

- `ValidateInvitationToken` refuses an `alg: none` token (wrong secret is already
  covered).
- **Empty JWT secret**: `GetSecret` returns an empty slice → `ValidateInvitationToken`
  errors, and `GenerateUserAuthToken`, `GenerateAPIAuthToken`,
  `GenerateInvitationToken` return `""` (never a token signed with an empty key).
  Use a real logger so the log branches run.
- `ValidateInvitationToken` with a `GetSecret` error → "secret not available"
  (the Generate* cases already exist).

### 3b. Sign-in brute-force limits — `internal/service/user_service.go`

Test file: `internal/service/user_service_rate_limit_test.go`.

- Magic-code `SignIn` and root sign-in refuse once the rate limiter denies the
  email, without touching the repository or sending a code.

### 3c. Transactional send failures — `internal/service/transactional_service.go`

Test file: `internal/service/transactional_service_send_failures_test.go`.

- No email provider configured → error, no message history.
- `SendEmailForTemplate` fails → "failed to send notification through any channel".
- Channel filter matching no enabled channel → the documented error.

### 4. Automation permissions — `internal/service/automation_service.go`

Test file: `internal/service/automation_service_permissions_test.go`.

- Member with **read-only** automations permission: `Create`, `Update`, `Delete`,
  `Activate`, `Pause` return a `*domain.PermissionError`; the repository is never
  called (strict gomock, no expectation).
- Member with **no** automations permission: `Get`, `List`,
  `GetContactNodeExecutions` return a `*domain.PermissionError`; no repository call.
- `Activate` / `Pause`: authentication failure → "failed to authenticate".

## P2 — Sending

### 5. Recipient data feed — `internal/service/broadcast/data_feed.go`

Test file: `internal/service/broadcast/data_feed_failures_test.go` (uses
`NewUnsafeDataFeedFetcher` against `httptest`).

Timeout (5 s), retries (2) and retry delay (5 s, plain `time.Sleep`) are hard-coded,
so the timeout and connection-refused retry paths would cost 10–25 s each: not tested.

`FetchRecipient`:
- **Cancelled context** (`context.WithCancel`, cancelled while the handler blocks; the
  handler returns on `r.Context().Done()`) → "request cancelled", a single hit.
- **Invalid JSON** 200 response → "invalid JSON response", a single hit (no retry).
- **Empty error body**: 400 with an empty body → error "HTTP error 400: Bad Request",
  and the logged `body` field is "(empty)" (mock logger matcher).
- **Nil payload** → server receives `{}`.
- `null` JSON body → returns a map with `_success`, not nil.

`FetchGlobal`: cancelled context (`WithCancel`, the existing test uses a timeout and
lands in another branch), empty error body logged as "(empty)", `null` body.

### 6. Orchestrator A/B and cancellation — `internal/service/broadcast/orchestrator.go`

Test file: `internal/service/broadcast/orchestrator_stop_test.go` (reusing
`sendingStartedMocks`, `abBroadcast`, `recordBatches`).

- **Test phase ends with no winner** (status transition already covered in
  `orchestrator_test.go`): add the assertion that only the 50 % sample is sent.
- **Cancelled at fetch time**: `GetBroadcast` answers `processing` for the reads
  before the loop and the loop-top refresh, and `cancelled` for the `FetchBatch`
  read → `allDone` true, nil error, `SendBatch` never called, broadcast not marked
  `processed`.
- **Time limit reached between batches** (real `time.Now()`): the first `SendBatch`
  sleeps past `timeoutAt` → `allDone` false, the first batch's progress saved, no
  second batch.

### 7. Queue sender — `internal/service/broadcast/queue_message_sender.go`

Test file: `internal/service/broadcast/queue_message_sender_batch_failures_test.go`.

- `GetBroadcast` fails → returns `(0, len(recipients), err)`, nothing enqueued.
- Every recipient fails to build (empty templates map, or broken Liquid subject) →
  `(0, n, nil)` and `Enqueue` is never called.
- Contact language set → the contact's language is passed to entry building
  (translation itself is already covered).
- `timeoutAt` already past → nothing built, nothing enqueued.

## P3 — Automations HTTP

### 8. `internal/http/automation_handler.go`

Test file: `internal/http/automation_handler_requests_test.go`.

Only the cases not already covered:
- wrong HTTP method → 405, service not called: get, list, update, delete, activate,
  pause, nodeExecutions;
- invalid JSON body → 400, service not called: create, update, delete, activate, pause;
- request failing `Validate()` → 400: delete, activate, pause;
- service returns `*domain.PermissionError` → 403: get, list, delete, pause,
  nodeExecutions;
- service returns `domain.ErrAPIKeyRevoked` → 401: each endpoint above (checks the
  wiring; the helper itself is tested in `utils_test.go`). `nodeStats` is not in the
  table.

## Spec review

Reviewed by an independent sub-agent on 2026-11-13; confirmed remarks are folded in
above (unreachable branches, duplicates removed, data-feed timing, possible
double send).

## Rules for the tests

- Each test asserts the behaviour (refusal, no side effect via strict gomock),
  not only that a line ran.
- No count in a test name or comment unless an assertion pins it.
- No production code change; any defect found is reported in the PR.

## Verification

```bash
go test -race ./internal/service/ -run 'SMTPBridge|OIDC|HandleCallback|Resolve|LinkIdentity|OpenFlowState|MintSession|AuthService|AutomationService|UserService|Transactional'
go test -race ./internal/service/broadcast/
go test -race ./internal/http/ -run Automation
make test-unit
go test -coverprofile=coverage.out ./internal/service/... ./internal/http/... && go tool cover -func=coverage.out
```

Then compare the per-file coverage of the files above with the Codecov baseline
(Codecov on dev @ 02af998; the branch is based on dev @ bcffc25).
