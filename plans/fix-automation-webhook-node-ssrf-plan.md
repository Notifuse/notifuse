# Fix: SSRF through the automation webhook node (GHSA-88mf-6797-r264)

Created 2026-10-15.

- **Advisory:** GHSA-88mf-6797-r264, reported 2026-10-05.
- **Sibling:** GHSA-mvfg-323w-qgv9, already fixed for webhook subscriptions in `e9483de7` on `dev` (unreleased).
- **Target:** 42.0. Pierre decided on 2026-10-16 that the unreleased 41.1 work ships as v42, together with B24's migration (`plans/fix-removed-contacts-list-membership-plan.md`, P2). This fix bumps nothing itself: `## [41.1] - WIP` is renamed to 42.0, and `VERSION` moves in the commit that adds `v42.go`.
- **Status:** **implemented 2026-10-15, uncommitted.** 17 files changed plus 2 new tests on `dev`, and 3 files in `../docs`. Every cycle was seen red, then green. The final step passed: the touched unit packages and the 7 named integration suites. Commit only on an explicit go.
- **Decided on 2026-10-15:**
  - D1: one opt-out.
  - D2: a refused step fails after its retries.
  - `SECURITY.md` names the owner-endpoint exception.
  - The changelog carries a bold "read this before upgrading" line.
- **Still recommendations:** D3–D7.

**Review log, 2026-10-15.** An independent pass re-read every claim against `8ce64aee` and ran each red/green scenario with `go test -overlay`, so no repository file was written. It also checked the `jsonb_set` path on the test Postgres. Its corrections are folded in:
- the `last_error` and `handleError` cites;
- `Output` is an empty map, not nil;
- the refusal message misreported redirects;
- D1 against the written principle;
- the `node-root` fixture;
- trailing-dot spellings;
- wrong backoff text in a comment and the docs;
- `TestWebhookPayloadSamples`;
- ending the plan with the test step.

**Second check, 2026-10-15.**
- Ran Step 1's acceptance test, as written here, on `c974080a` against the real stack (`go test -overlay`, no repository file written). It failed exactly as predicted: subtests 1, 2 and 4 failed and 3 passed.
- Subtest 4's failure is the reproduction of the report: `receiver hits=1`, and the webhook node completed with `output: {response: {leak: "SHOULD NEVER BE READ"}, status_code: 200, url: http://127.0.0.1:…}`. That's readable by anyone with automations read.
- The test's source is `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/ea1dddbd-bfc1-4a51-8459-9ec0f4025169/scratchpad/acceptance/automation_webhook_ssrf_test.go`.

## What a member can do today

1. A member or API key with `automations:write` saves an automation whose webhook node points at the deployment's own network: `http://127.0.0.1:…`, `http://10.0.0.5/…`, a sibling container, or a public URL that 302-redirects to the metadata endpoint.
2. A contact enters the automation.
3. The member reads up to 10 KiB of the internal response back through `automations.nodeExecutions`.

The node can also send a bearer token of the member's choosing. `e9483de7` closed the same hole for subscriptions but never touched this executor, which is how the reporter found it.

## Verified facts

Read in the code at `8ce64aee`; all of it still holds at `c974080a`. That commit (the favicon work, #432) changed only `CHANGELOG.md`, `public_handler.go`, `public_handler_favicon_test.go` and `SegmentsFilter.tsx`. Its changelog bullet sits at line 39; this plan changes only line 7.

**F1 — The node builds its own plain client.**
- `NewWebhookNodeExecutor` returns `&http.Client{Timeout: 30 * time.Second}` (`internal/service/automation_node_executor.go:1109-1114`).
- Its transport is nil, so Go's default transport is used. That means no address check, the default redirect policy (POST becomes GET on 301/302/303), and `HTTP(S)_PROXY` is honoured.
- The executor is registered at `internal/service/automation_executor.go:51`. `NewAutomationExecutor`'s only caller is `internal/app/app.go:1254`; tests build `AutomationExecutor{…}` literals instead.

**F2 — What comes back to the member.**
- The node reads up to 10 KiB of the body (`automation_node_executor.go:1157`). On success it's stored as `output.response` (`:1193-1197`).
- On a 4xx or 5xx the body goes into the error string (`:1163-1170`), which lands in three places:
  - the node's own `automation_node_executions` row (`error`, set at `automation_executor.go:136-142`);
  - `contact_automations.last_error`, set by `handleError` (`:249`) and saved by `persistIfActive` (`:297`);
  - a second row `handleError` writes (from `:282`), under the webhook's `node_id` but with the placeholder `node_type` `trigger` (`:289`).
- The node's own row starts with `Output: make(map[string]interface{})` (`:366`), and the failure path never sets it, so a failed row has an empty output, not a nil one.
- `automations.nodeExecutions` returns the contact automation and its rows (`internal/http/automation_handler.go:325-327`).
- `Authorization: Bearer <secret>` comes from the node config (`automation_node_executor.go:1145-1147`).

**F3 — Nothing checks the URL when it's saved.**
- `WebhookNodeConfig.Validate` only checks the `http(s)://` prefix (`internal/domain/automation.go:741-750`), and it only runs at execution, through `parseWebhookNodeConfig` (`automation_node_executor.go:1222-1238`).
- `AutomationNode.Validate` never reads webhook config (`domain/automation.go:391-426`). It leaves incomplete drafts savable on purpose.
- The console treats an empty URL as valid (`console/src/components/automations/config/WebhookConfigForm.tsx`, `isValidUrl`).

**F4 — Who can save one.** `automations:write` is required by `Create` (`internal/service/automation_service.go:41-47`) and `Update` (`:120-126`).

**F5 — Every node error is retried.**
- `handleError` (`automation_executor.go:246-298`) increments `RetryCount`. At `max_retries` (default 3, `internal/database/init.go:477`) it marks the journey `failed` (`:253-265`).
- Otherwise it waits `1<<RetryCount` minutes (`:266-270`): 2 min after the first failure, then 4 min.
- A contact automation's `RetryCount` is never reset. The only reset in non-test code is for tasks (`task_service.go:1344`), so "3 attempts" is the budget of a journey with no earlier failure.
- There is no non-retryable class. Three texts are wrong about this, and `TestAutomationExecutor_Execute_WebhookNode_ClientError_TriggersRetry` pins the retry:
  - the "4xx … fail immediately" comment at `automation_node_executor.go:1164`;
  - the "1min, 2min, 4min" comment at `automation_executor.go:267`;
  - "1min, 2min, 4min, 8min…" in `../docs/features/automations.mdx:312`.

**F6 — Pieces from `e9483de7` that can be reused.**
- `safehttpclient.New()` (`pkg/safehttpclient/safehttpclient.go:72-120`):
  - Each connection dials through a `DialContext` that refuses private, loopback, link-local and reserved addresses. That covers each redirect hop, because a new host means a new dial.
  - It has a 10 s timeout and sets no proxy.
  - A refusal returns `safehttpclient.ErrPrivateIP`.
- `rejectPrivateWebhookHost(host)` is the save-time spelling check, in the same package (`internal/service/webhook_subscription_service.go:220`). `TestValidateURL` (`:1058`) covers it. It accepts a trailing dot (`localhost.`, `svc.default.svc.cluster.local.`, `metadata.google.internal.`) and short IPv4 forms (`127.1`, `0x7f000001`). That's verified by running it.
- `config.Webhook.AllowPrivateDeliveryHosts` is read from `WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS` (`config/config.go:214-224`, `:996`).
- `app.go:1177-1206` wires the guard, then the opt-out, then tracing.

**F7 — Tracing keeps the guard.** `tracing.WrapHTTPClient` keeps the base transport as its `Base`, and copies `Timeout`, `Jar` and `CheckRedirect` (`pkg/tracing/helpers.go:106-122`). Run in the audit: `ErrPrivateIP` survives `*url.Error`, a `%w` wrap and the tracing wrapper.

**F8 — A service error the handler doesn't recognise becomes a 500.**
- `handleCreate` only maps permission errors (`automation_handler.go:67-74`).
- `handleUpdate` only maps permission, `TriggerConditionError` and conflict errors (`:154-175`).
- `domain.ValidationError` exists, with a value receiver (`internal/domain/errors.go:54-68`). Other handlers match it with `errors.As` on a value target (`workspace_handler.go:260-264`, `annotation_handler.go:196-200`). Neither automation handler nor `AutomationService` uses it today.
- The console shows the response's `error` field (`console/src/services/api/client.ts:49-58`) as `Failed to create/update automation: …` (`AutomationContext.tsx:261-262`, `:276-277`).

**F9 — Call sites the signature changes touch.**
- `NewWebhookNodeExecutor(mockLogger)` appears in 8 tests in `automation_node_executor_test.go` (`:3243`–`:3629`) and 6 in `automation_executor_test.go` (`:1149`–`:1540`).
- All but `NodeType` (`:3243`) and `InvalidConfig` (`:3587`) post to an `httptest` server on 127.0.0.1.
- `NewAutomationService` appears 12 times in `automation_service_test.go`, including the helper at `:1224`, plus once in `app.go:1051`.
- No other constructor call or struct literal exists outside `.claude/worktrees/`.

**F10 — Who saves webhook nodes.**
- Through the API, only `testWebhookNode` (`tests/integration/automation_e2e_test.go:3238`). Its URL is an `httptest` server (`:3298`), and it runs under `TestAutomation` (`:153`), whose suite builder (`:158-160`) does not opt out.
- The harness builds its config as a literal with no global opt-out (`tests/testutil/server.go:92`).
- The demo seeder goes through `AutomationService.Create` (`internal/service/demo_automations.go:394`) with `https://example.com/hooks/crm/vip` (`:315-317`).
- Only `Create` and `Update` write nodes; nothing caches automations.

**F11 — Nodes are stored as an ordered JSON array.**
- `automations.nodes` is JSONB (`init.go:456`), holding the JSON array of `AutomationNode` in request order (`internal/repository/automation_postgres.go:72`).
- Checked on the test Postgres: `jsonb_set(nodes, '{1,config,url}', to_jsonb($1::text))` replaces only the second node's URL and keeps its `secret`.

**F12 — Other outbound URLs, set by owners only.**
- SparkPost `Endpoint`, OpenAI-compatible `BaseURL` and Firecrawl `BaseURL` are integration settings. Creating or updating one requires the owner role (`workspace_service.go:2006-2017`, `:2407-2418`).
- S3 `Endpoint` (workspace settings) and SMTP `Host` are also set by admins.
- All of these are out of scope.

## Decisions (recommendations, not verified facts)

**D1 — One opt-out for both paths. Decided 2026-10-15.** `WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS` covers webhook nodes too, and Step 3 rewrites the written principle to match.
- *The principle.* `config/config.go:219-222` says "an operator who needs one should not be granted the other without naming it", and `SECURITY.md:77-79` calls the opt-outs "each scoped to one feature". The separation those texts draw is between features: fetching feed content versus posting webhooks.
  - Subscriptions and webhook nodes are one feature: posting an event about a contact to a URL a member chose.
  - They serve the same internal receivers: n8n, an internal API.
- *The cost.* Different permissions gate the two (`webhook_subscriptions:write` and `automations:write`), so switching it on trusts holders of either with the internal network. The setting is already deployment-wide, so it already ignores which workspace or member holds the permission. Step 3 rewrites both passages to say this.
- *Rejected alternative:* a separate `AUTOMATION_WEBHOOK_ALLOW_PRIVATE_HOSTS`. It would have kept the principle as written, at the cost of one more variable for operators to find.

**D2 — A refused request takes the ordinary retry path. Decided 2026-10-15.** That's up to 3 attempts, then the journey is `failed` (F5).
- *Why:* the failure is per contact and bounded, and nothing is auto-disabled, unlike the subscription case `e9483de7` had to protect.
- *Alternative, "don't count the refusal":* retries forever, adding 2 `automation_node_executions` rows per attempt.
- *Alternative, "fail fast":* needs a new non-retryable error class in the executor, to save at most two dials.

**D3 — The save-time check covers only the nodes in the request.**
- A full save (the console always sends every node) checks every webhook node, unchanged ones included. Subscriptions behave the same way: their updates always carry the URL.
- A partial update that omits `nodes` keeps the stored ones (`automation_service.go:149-159`) and doesn't re-check them. `Activate` doesn't check either. Nodes saved before the upgrade are the dial-time guard's job, so a rename or an activation can't fail because of them.

**D4 — Incomplete drafts stay savable; the spelling check is about clear errors, not protection.**
- An empty, non-string or unparseable URL is skipped at save time and left to run time (F3).
- This plan makes `rejectPrivateWebhookHost` ignore one trailing dot, which closes `localhost.`-style spellings on both paths (F6).
- Short IPv4 forms (`127.1`, `0x7f000001`) still pass at save time. They never connect: either the lookup fails, or the dial-time guard refuses the address.

**D5 — The error type.** The service returns the existing `domain.ValidationError`, and both automation handlers map it to 400 with its `Message` (F8). A dedicated type would add nothing.

**D6 — One function builds every outgoing-webhook client.** Extract `newOutgoingWebhookClient(allowPrivateHosts, tracingEnabled bool, timeout time.Duration) *http.Client` from `app.go:1189-1198`.
- Use it for the delivery worker (10 s, unchanged) and the webhook node (30 s, unchanged).
- The two paths then can't drift, and "tracing wraps after the guard" becomes unit-testable.
- `e9483de7` is unreleased, so refactoring it costs nothing.

**D7 — A nil client fails closed.** `NewWebhookNodeExecutor(log, nil)` uses `safehttpclient.New()` with a 30 s timeout. `NewWebhookDeliveryWorker` does the opposite and falls back to a plain client (out of scope).

## Design

**Dial time:**
- The node executor sends with an injected client.
- App wiring passes `newOutgoingWebhookClient(cfg.Webhook.AllowPrivateDeliveryHosts, cfg.Tracing.Enabled, 30*time.Second)` through a new trailing `webhookHTTPClient *http.Client` parameter on `NewAutomationExecutor`.
- On `errors.Is(err, safehttpclient.ErrPrivateIP)`, the node returns `webhook request refused: <url>, or a redirect from it, leads to a private or reserved address, which webhook nodes do not connect to; set WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS=true to deliver to your own network: %w`.
  - The wrap keeps `errors.Is` true.
  - The wording covers redirects: the audit ran a public first hop that answered 302 to loopback, and the old wording blamed the public URL.

**Save time:**
- `AutomationService` takes `allowPrivateWebhookHosts bool` as a new last constructor parameter, as `NewWebhookSubscriptionService` did in `e9483de7`.
- New `validateWebhookNodeTargets(nodes []*domain.AutomationNode, allowPrivateHosts bool) error`:
  - skips non-webhook nodes and empty, non-string or unparseable URLs (D4);
  - otherwise calls `rejectPrivateWebhookHost(parsed.Hostname())`;
  - returns `domain.NewValidationError("webhook node <id>: <reason>")`.
- It's called in `Create` after `automation.Validate()` (`:55`), and in `Update` after `automation.Validate()` (`:128`) and before `GetByID` (`:132`), so omitted nodes are never re-checked (D3).
- `rejectPrivateWebhookHost` gains `host = strings.TrimSuffix(host, ".")` at the top.
- There's no DNS resolution at save time; the reason is in the `validateURL` comment (`webhook_subscription_service.go:170-182`).

**HTTP:** `handleCreate` and `handleUpdate` map `domain.ValidationError` to 400, right after `writePermissionError`.

## TDD sequence

Write the test, run it, and see it fail for the stated reason before writing the code. A test marked "guard" passes before and after the change: it pins behaviour that must not move.

### Step 0 — Baseline (ran 2026-10-15 at `8ce64aee`: all pass)

```
go test -race ./internal/service/ -run 'TestWebhookNodeExecutor|TestParseWebhookNodeConfig|TestAutomationExecutor_Execute_WebhookNode|TestAutomationService_|TestDemoAutomations|TestValidateURL|TestWebhookDeliveryWorker'
go test -race ./internal/http/ -run TestAutomationHandler
go test -race ./internal/app/ ./pkg/safehttpclient/
```

### Step 1 — Acceptance test first; red on HEAD

New file `tests/integration/automation_webhook_ssrf_test.go`, tagged `//go:build integration`, with `TestAutomationWebhookNodeSSRFProtection`.

**Setup:**
- Same as `TestAutomation` (`automation_e2e_test.go:153-187`): owner user, workspace, SMTP provider, login, `StartBackgroundWorkers`.
- **Without** `cfg.Webhook.AllowPrivateDeliveryHosts`: this suite is the stock deployment, like `webhook_delivery_ssrf_test.go`.
- Bodies are raw maps shaped like `testWebhookNode`'s (`:3272-3312`): a `custom_event` trigger, then the webhook node.

**Subtests:**
1. **"create refuses a webhook node aimed at an internal target".**
   - URLs: `http://127.0.0.1:8080/hook`, `http://[::1]:8080/hook`, `http://169.254.169.254/latest/meta-data/`, `http://10.0.0.5/hook`, `http://localhost:3000/hook`, `http://svc.default.svc.cluster.local/hook`.
   - Each `POST /api/automations.create` returns 400, and the body contains `WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS`.
   - *Red on HEAD:* 201.
2. **"update refuses switching a node to an internal target".**
   - Create with `https://hooks.example.com/notifuse?token=abc123` (201).
   - `automations.update` the same automation with the URL `http://10.0.0.5/hook`. Expect 400.
   - *Red on HEAD:* 200.
3. **"a public receiver and an unconfigured draft are still saved"** (guard). The public URL with a token query gets 201, and so does `"url": ""`.
4. **"the executor refuses a stored private URL".**
   - Start a receiver `httptest` server with an atomic hit counter, answering `SHOULD NEVER BE READ`.
   - In order:
     1. Create the automation with `https://hooks.example.com/legacy`.
     2. Through `suite.DBManager.GetWorkspaceDB`, run `UPDATE automations SET nodes = jsonb_set(nodes, '{1,config,url}', to_jsonb($1::text)) WHERE id = $2` with the receiver URL (F11).
     3. `automations.get` it, to prove the flip landed.
     4. Activate, create the contact, and fire the custom event.
   - Poll `client.GetContactNodeExecutions` by hand for 15 s, every 200 ms, until `node_executions` has an entry with `node_type == "webhook"`, `node_id == <webhook node>` and `action == "failed"`.
     - Filter on `node_type`: `handleError` adds a second failed row under the same `node_id` with the placeholder type `trigger` (F2).
     - Don't use `testutil.WaitForCondition`: it fails with a bare timeout. Keep the last rows seen, and fail with `require.NotNil(refused, "…receiver hits=%d; node executions seen: %v")`, so the red run shows the leak itself.
   - Assert:
     - the entry's `error` contains `private or reserved` and `WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS`;
     - its `output` has no `response` key (it's absent, or `{}`);
     - `contact_automation.last_error` contains the same text;
     - after a 200 ms grace period, the receiver has 0 hits.
   - *Red on HEAD (observed):* `receiver hits=1`, and the webhook row is `completed` with `output.response = {"leak":"SHOULD NEVER BE READ"}`. The red output itself is the reproduction of the report.

Run with `docker compose -f tests/compose.test.yaml up -d` started:

```
INTEGRATION_TESTS=true go test -race -tags integration,licdev -timeout 20m ./tests/integration/ -run '^TestAutomationWebhookNodeSSRFProtection$' -v
```

Expected on HEAD: 1, 2 and 4 fail; 3 passes. Keep the output.

### Cycle A — The node sends through an injected client (service)

**A1, structural, no behaviour change:**
- `NewWebhookNodeExecutor(log logger.Logger, httpClient *http.Client)`, still building the plain 30 s client and ignoring the parameter.
- `automation_executor.go:51` passes `nil` for now.
- Add a test helper `permissiveWebhookClient()`, returning `&http.Client{Timeout: 5 * time.Second}` (what the opt-out builds), and pass it at the 14 call sites (F9).
- Step 0 stays green.

**A2, red.** In `internal/service/automation_node_executor_test.go`:
- **`TestWebhookNodeExecutor_Execute_SendsThroughTheInjectedClient`.**
  - A recording `RoundTripper` answers 200 `{"ok":true}`. The URL is `https://receiver.invalid/hook`; `.invalid` never resolves (RFC 6761), so bypassing the injected client fails fast on DNS. The node has secret `s3cret`.
  - Assert one recorded request: `POST`, that URL, `Authorization: Bearer s3cret`, `Content-Type: application/json`. Also `output.status_code == 200` and `output.response.ok == true`.
  - *Red:* "no such host" (0.66 s in the audit).
- **`TestWebhookNodeExecutor_Execute_RefusesAPrivateTarget`.**
  - A hit-counting `httptest` server and `NewWebhookNodeExecutor(log, safehttpclient.New())`.
  - Assert: nil result, `errors.Is(err, safehttpclient.ErrPrivateIP)`, the message contains the URL and `WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS`, and the server has 0 hits.
  - *Red:* the server is hit and no error is returned.
- **`TestWebhookNodeExecutor_Execute_RefusesARedirectIntoThePrivateNetwork`.**
  - A test transport stands in for a public host: it answers requests for `public.example` itself with a 302 to `<loopback server>/latest/meta-data/`, and hands every other hop to `safehttpclient.New().Transport`. The client uses the safe `CheckRedirect`.
  - Node URL: `http://public.example/hook` (`.example` is reserved, RFC 2606).
  - Assert: `ErrPrivateIP`, 0 hits, and the message says `or a redirect from it`.
  - *Red:* DNS failure for `public.example`, which isn't `ErrPrivateIP`.
  - This pins the reporter's headline attack: POST, then 302, then the metadata endpoint.
- **`TestNewWebhookNodeExecutor_NilClientFailsClosed`.** A nil client against the loopback server gives `ErrPrivateIP` and 0 hits, and `executor.httpClient.Timeout == 30*time.Second`. *Red:* the server is hit.

In `internal/service/automation_executor_test.go`:
- **`TestAutomationExecutor_Execute_WebhookNode_PrivateTargetRefused_IsRetried`.**
  - Mirror `…_ServerError_TriggersRetry` (`:1292-1365`), with `NewWebhookNodeExecutor(mockLogger, safehttpclient.New())` and a hit-counting loopback receiver that **answers 500**.
    - With a 500, a run without the guard takes the same mock path and fails on the assertions below.
    - With a 200 it would stop instead on gomock's "Unexpected call to IncrementAutomationStat(… "completed")".
  - Capture the single `UpdateNodeExecution` argument with `DoAndReturn`. Expect `Action == failed`, `Error` containing `WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS`, and `Output` without `response` or `status_code`. It is the empty map from `createNodeExecution`, not nil (F2).
  - Then `RetryCount == 1`, `ScheduledAt != nil`, `LastError` containing the same text, and 0 hits. This pins D2.
  - *Red:* 1 hit, and `LastError` reads `webhook returned server error: 500`.

**A3, green:**
- Store the client. A nil client becomes `safehttpclient.New()` with `Timeout = 30 * time.Second` (D7).
- At `:1150-1153`, return the refusal error from Design when `errors.Is(err, safehttpclient.ErrPrivateIP)`. Otherwise keep `webhook request failed: %w`.
- Update the struct's doc comment.
- Correct the "4xx … fail immediately" comment at `:1164`: every error is retried (F5). This is comment-only, in the function being edited.

From here on, production webhook nodes are guarded (nil means the safe client), but the opt-out can't reach them until B3. `TestAutomation/WebhookNode` is therefore red until B3: "timeout waiting for webhook to be called", at `automation_e2e_test.go:3376`.

Run: `go test -race ./internal/service/ -run 'TestWebhookNodeExecutor|TestNewWebhookNodeExecutor|TestAutomationExecutor_Execute_WebhookNode' -v`.

### Cycle B — One builder for outgoing-webhook clients (app)

**B1, red.** New `internal/app/outgoing_webhook_client_test.go`, `TestNewOutgoingWebhookClient`. A hit-counting loopback receiver; each subtest POSTs through the returned client:
- "refuses a private target by default" `(false, false, 10s)`: `ErrPrivateIP`, 0 hits.
- "the guard survives tracing" `(false, true, 10s)`: `ErrPrivateIP`, 0 hits. This pins F7.
- "the opt-out reaches the private network" `(true, false, 10s)`: 200 and 1 hit.
- "…with tracing" `(true, true, 10s)`: 200 and 1 hit.
- "keeps the timeout it is given": `Timeout` is 10 s and 30 s, for both the guarded and the opted-out client.

Stub the function first (`return &http.Client{Timeout: timeout}`) so it compiles. *Red:* the two refusal subtests.

**B2, green:**

```go
client := safehttpclient.New()
if allowPrivateHosts {
    client = &http.Client{}
}
client.Timeout = timeout
if tracingEnabled {
    client = tracing.WrapHTTPClient(client)
}
return client
```

Move the reasoning from the comment at `app.go:1177-1188` onto the function.

**B3, refactor and wire:**
- `app.go:1189-1198` becomes `newOutgoingWebhookClient(…, 10*time.Second)`. Keep a single `Warn` when the opt-out is on, reworded to name subscriptions and webhook nodes.
- `NewAutomationExecutor` gains a trailing `webhookHTTPClient *http.Client` and passes it to `NewWebhookNodeExecutor(log, webhookHTTPClient)`.
- `app.go:1254` passes `newOutgoingWebhookClient(…, 30*time.Second)`, commented as the node's previous timeout.
- Fix the backoff comment at `automation_executor.go:267`: "2 min, then 4 min; the journey fails at max_retries".
- In `TestAutomation`'s suite builder (`automation_e2e_test.go:158-160`), set `cfg.Webhook.AllowPrivateDeliveryHosts = true`. Use the comment from `webhook_worker_e2e_test.go:73-77`, pointing to `TestAutomationWebhookNodeSSRFProtection` for the secure default.
- `go test -race ./internal/app/` stays green, including `TestAppInitServices`, `TestAppInitHandlers` and `TestAppInitTracingEnabled`.

### Cycle C — Save-time refusal (service)

**C1, structural:**
- `NewAutomationService(repo, authService, logger, allowPrivateWebhookHosts bool)` stores the flag.
- The 12 test call sites (F9) pass `false`, the secure default. None of those tests saves a webhook node with a `url`.
- `app.go:1051` passes `a.config.Webhook.AllowPrivateDeliveryHosts`.
- Everything stays green.

**C2, red.** In `internal/service/automation_service_test.go`, `TestAutomationService_WebhookNodeTargets`. Each subtest gets its own controller, like `newAutomationTransitionMocks`.

*Fixture:* `createTestAutomationService` sets `RootNodeID: "node-root"` with no nodes (`:31`). The trigger node you add must be `node-root`, or `Validate()` fails first and the refusal cases go red for the wrong reason and never turn green.

- **Create refuses an internal target.**
  - Table: `http://127.0.0.1:8080/hook`, `http://[::1]/hook`, `http://169.254.169.254/latest/meta-data/`, `http://10.0.0.5/hook`, `http://localhost:3000/hook`, `http://svc.default.svc.cluster.local/hook`, `http://localhost./hook`.
  - Assert `errors.As(err, &domain.ValidationError{})`, and that the message contains the node ID and `WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS`.
  - Set no `repo.Create` expectation. gomock v1.6.0 fails the test on any unexpected call.
  - *Red:* "Unexpected call to Create".
- **Create accepts** (guard): `https://hooks.example.com/notifuse?token=abc123`, `""` (a draft, D4), and `http://n8n:5678/webhook` (a single label is decided at dial time). `repo.Create` is called once each.
- **Create with the opt-out on** (guard): `allowPrivateWebhookHosts = true` accepts `http://127.0.0.1:8080/hook`.
- **Create ignores a `url` key on a non-webhook node** (guard): `add_to_list` with `"url": "http://127.0.0.1/"`.
- **Update refuses an internal target in the request's nodes.**
  - Use a raw JSON body via `decodeUpdateAutomationRequest`, like `TestAutomationService_Update_OmittedNodesKeepTheStoredWorkflow` (`:612`).
  - Set no `GetByID` or `UpdateIfStatus` expectation.
  - *Red:* "Unexpected call to GetByID" (`automation_service.go:132`). gomock stops at the first unexpected call.
- **Update that omits `nodes` doesn't re-check the stored workflow** (guard, pins D3).
  - Model it on `:612`: a rename body with no `nodes` key.
  - `GetByID` returns a stored workflow whose root node exists and whose webhook node is on `http://10.0.0.5/hook`, with status draft, so no trigger regeneration.
  - `UpdateIfStatus` receives the stored nodes, and the error is nil.

In `internal/service/webhook_subscription_service_test.go`, add three rows to `TestValidateURL`: `http://localhost./hook`, `http://svc.default.svc.cluster.local./hook` and `http://metadata.google.internal./computeMetadata/v1/`, each refused. *Red:* accepted today (F6).

In `internal/service/demo_automations_test.go`, extend `TestDemoAutomations_AreUsable` (`:24`) with `require.NoError(t, validateWebhookNodeTargets(automation.Nodes, false))` (guard). The seeder goes through `Create` (F10), so a demo node aimed at an internal host would break the demo reset. It compiles from C3.

**C3, green:** implement `validateWebhookNodeTargets` and its two calls, and add the trailing-dot trim to `rejectPrivateWebhookHost` (Design).

Run: `go test -race ./internal/service/ -run 'TestAutomationService_|TestDemoAutomations|TestValidateURL' -v`.

### Cycle D — The refusal is a 400, not a 500 (http)

**D1, red.** In `internal/http/automation_handler_test.go`:
- **New `TestAutomationHandler_Create_ErrorMapping`**, table-driven like `TestAutomationHandler_Update_ErrorMapping` (`:429-497`):
  - bare `domain.NewValidationError("webhook node n1: …")` → 400 with exactly that `Message`;
  - wrapped in `fmt.Errorf("invalid automation: %w", …)` → 400;
  - a generic error → 500 `Failed to create automation`;
  - a permission error → 403.
- **Two new rows in `TestAutomationHandler_Update_ErrorMapping`:** bare and wrapped validation errors, each a 400 with the `Message`.
- *Red:* 500 today.

**D2, green.** In `handleCreate` (after `:69-71`) and `handleUpdate` (after `:156-158`): `var validationErr domain.ValidationError; if errors.As(err, &validationErr) { WriteJSONError(w, validationErr.Message, http.StatusBadRequest); return }`.

Run: `go test -race ./internal/http/ -run TestAutomationHandler -v`.

### Step 2 — Acceptance green

- Re-run Step 1: all four subtests pass.
- Run `TestAutomation`: green with B3's opt-out. Without it, `WebhookNode` fails with a 400 on create (since Cycle C/D) or a webhook timeout (since A3).

### Step 3 — Comments and prose (no tests)

Under D1 (one opt-out):

- **`config/config.go`:**
  - The `Webhook` field comment (`:30-32`) and `AllowPrivateDeliveryHosts` (`:215-218`) cover subscriptions **and automation webhook nodes**.
  - Reword the principle (`:219-222`): the separation is between features (fetching feeds versus posting webhooks); subscriptions and webhook nodes are one feature; enabling the setting trusts holders of `webhook_subscriptions:write` and `automations:write`.
- **`env.example` (`:183-192`):**
  - :184 "post events to a URL supplied by a workspace member" names both paths.
  - :188 becomes "refuses to save a subscription or an automation webhook node that names such a target".
  - :191 becomes "disables SSRF protection for outgoing webhooks".
- **`SECURITY.md` (`:72-80`):** replace the first hardening bullet with the text below. Keep the file's existing 80-column wrapping. "Each scoped to one feature" stays true. The last sentence was decided on 2026-10-15: the current text claims protection that owner-configured endpoints don't have.

  > - Run Notifuse behind a network egress policy where possible. Several features make outbound HTTP requests to addresses a workspace member supplies — broadcast data feeds, outgoing webhooks (subscriptions and automation webhook nodes), and link/favicon previews. All of them are SSRF-protected by default: private, loopback, link-local and reserved targets are refused at dial time, on the first hop and on every redirect. Two settings relax this, each scoped to one feature and both off by default — `BROADCAST_DATA_FEED_ALLOW_PRIVATE_HOSTS` for data feeds and `WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS` for outgoing webhooks. Leave them disabled unless you deliberately target your own internal network. Integration endpoints that only workspace owners configure — an OpenAI-compatible or Firecrawl base URL, a SparkPost endpoint, S3, an SMTP host — are not restricted, because they often point at internal services such as a local LLM; grant the owner role accordingly.
- **`CHANGELOG.md`:** rewrite **only** the `[41.1]` (to be `[42.0]`) Security bullet at line 7, so it covers both paths. Leave the #432 favicon bullet (committed in `c974080a`, line 39) as it is. Four sentences:

  > - **Security**: Outgoing webhooks are no longer a server-side request forgery (SSRF) vector. Webhook subscriptions and automation webhook nodes posted to a member-supplied URL with a plain HTTP client, so anyone holding `webhook_subscriptions:write` or `automations:write` could aim them at the deployment's own network — the cloud metadata endpoint, an internal service — and read the response back through `webhookSubscriptions.test`, the delivery log or an automation's node executions. Both now refuse private, loopback, link-local and reserved addresses at dial time and on every redirect, and refuse to save a URL that names one. Self-hosted deployments that deliberately deliver to their own network can opt out with `WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS=true`; until they do, a refused subscription delivery is retried without counting toward the automatic-disable threshold, and a refused automation step fails like any other webhook error.
  >
  >   **If your webhooks reach a service on your own network — n8n or another container on the same Docker network, an internal API — read this before upgrading.** Subscriptions and automation webhook nodes aimed at a private address stop delivering until `WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS=true` is set, so set it before you upgrade. A contact that reaches such a webhook node in the meantime fails that journey after its retries.

  The bold paragraph was decided on 2026-10-15. It follows the SSO precedent at `CHANGELOG.md:81`, which is an indented paragraph under its bullet.

- **`../docs`** (a separate repository; ships as its own PR, and is easy to forget):
  - **`features/automations.mdx`, "Webhook Node" (`:203-210`):** add "Where it can deliver":
    - the receiver must be reachable from the public internet;
    - private, loopback, link-local and reserved addresses are refused, after a redirect too;
    - saving such a URL is refused;
    - a refused step is retried, then fails the contact's journey;
    - copy the self-hosted `<Note>` from `features/webhooks.mdx:150-152`.
  - **`features/automations.mdx:312`:** fix "1min, 2min, 4min, 8min…" to "2 min, then 4 min; after `max_retries` (3) the journey is `failed`" (F5).
  - **`self-hosting/installation.mdx`:** the table (`:173-174`) has no `WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS` row; mvfg missed it. Add a "Webhook Configuration" row covering both paths, and say that outgoing webhooks connect directly and ignore `HTTP_PROXY`/`HTTPS_PROXY` unless it's on.
  - **`features/webhooks.mdx:151`:** "…can create subscriptions" becomes "…can create subscriptions or edit automations". Add "The same setting covers automation webhook nodes."

## Implementation file → test file

| Implementation | Test file | Cases |
|---|---|---|
| `internal/service/automation_node_executor.go` | `internal/service/automation_node_executor_test.go` | `…_SendsThroughTheInjectedClient`, `…_RefusesAPrivateTarget`, `…_RefusesARedirectIntoThePrivateNetwork`, `TestNewWebhookNodeExecutor_NilClientFailsClosed`; 8 existing tests on `permissiveWebhookClient()` |
| `internal/service/automation_executor.go` | `internal/service/automation_executor_test.go` | `…_WebhookNode_PrivateTargetRefused_IsRetried`; 6 existing tests switched |
| `internal/service/automation_service.go` | `internal/service/automation_service_test.go`, `demo_automations_test.go` | `TestAutomationService_WebhookNodeTargets` (6 subtests), `TestDemoAutomations_AreUsable` assertion, 12 call sites pass `false` |
| `internal/service/webhook_subscription_service.go` (trailing dot) | `internal/service/webhook_subscription_service_test.go` | 3 new rows in `TestValidateURL` |
| `internal/http/automation_handler.go` | `internal/http/automation_handler_test.go` | `TestAutomationHandler_Create_ErrorMapping` (new), 2 rows in `…_Update_ErrorMapping` |
| `internal/app/app.go` | `internal/app/outgoing_webhook_client_test.go` (new) | `TestNewOutgoingWebhookClient` (5 subtests); existing `TestAppInit*` |
| full stack | `tests/integration/automation_webhook_ssrf_test.go` (new) | `TestAutomationWebhookNodeSSRFProtection` (4 subtests) |
| `tests/integration/automation_e2e_test.go` | itself | opt-out in `TestAutomation`'s builder |
| `config/config.go`, `env.example`, `SECURITY.md`, `CHANGELOG.md`, `../docs` | none (comments and prose) | — |

## Side effects and upgrade notes

- **A webhook node aimed at a private host fails after the upgrade,** until the URL changes or the operator opts out. A contact reaching it is retried up to twice, about 6 minutes in all, then marked `failed`. Contacts failed in that window aren't re-run once the opt-out is set. The changelog says how to opt out.
- **Saving such an automation with its nodes fails with that message** until it's fixed. Renaming through an API call that omits `nodes`, or activating, still works (D3).
- **A sibling container addressed by a single-label name** (`http://n8n:5678`) saves, then is refused at dial time with the message naming the variable. That's deliberate, as for subscriptions.
- **Proxies:** outgoing webhooks stop honouring `HTTP(S)_PROXY` under the secure default (F6). Subscriptions already do since `e9483de7`, which is in the same release. The installation docs row says so; with the opt-out, the behaviour is unchanged.
- **Tracing:** with tracing on, webhook node calls now produce spans. The node's old client was never wrapped.

## Out of scope (observed, not fixed here)

- `NewWebhookDeliveryWorker` falls back to a plain client when given nil (`webhook_delivery_worker.go:248-252`), so it fails open. Production always passes a client.
- **Owner-only outbound URLs (F12):** SparkPost `Endpoint`, OpenAI-compatible `BaseURL`, Firecrawl `BaseURL`, S3 `Endpoint`, SMTP `Host`. They're often legitimately private (Ollama or Firecrawl on localhost), so they need their own decision. They're the likely next report.
- **Console:** no change is needed, since the 400's message is displayed. An optional hint in `WebhookConfigForm` ("must be reachable from the public internet") would need a Lingui extract.

## Addendum — code review follow-ups (2026-10-15)

`/code-review high` reported 8 findings. Four concern this change:

- **#8, fixed.** The new test helpers had separated `TestEmailNodeExecutor_Execute_WebIdentity` from its doc comment.
- **#3, fixed.** The `Update` comment claimed a rename is never refused. Console saves resend every node, so a console rename is refused while a pre-upgrade private URL remains (D3); only the comment was wrong.
- **#7, declined.** Reusing `validateURL` for webhook nodes would refuse half-typed drafts (D4). Both paths already share `rejectPrivateWebhookHost`, which is what decides what counts as private.
- **#6, decided 2026-10-15.** Activation is not refused (D3 stands). Instead, the console shows the error where the URL is:
  - `/config.js` exposes `window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS` (`internal/http/root_handler.go`; tested by `TestServeConfigJS_WebhookPrivateHosts`).
  - `console/src/components/automations/utils/webhookTarget.ts` is a TypeScript mirror of `rejectPrivateWebhookHost` and the `safehttpclient` ranges. Its tests (`webhookTarget.test.ts`) mirror `TestValidateURL`.
  - `WebhookConfigForm` shows an inline error (`WebhookConfigForm.test.tsx`).
  - `WebhookNode` shows a canvas marker (`WebhookNode.test.tsx`).
  - `validateFlow` blocks Save with the same translated message (`flowConverter.test.ts`).
  - There are two new catalogue strings, translated in all 7 locales; `catalogues.test.ts` requires that.

The other four findings (#1, #2, #4, #5) belong to the unsubscribe and broadcast-audience work uncommitted in the same tree. They were left untouched.

**Second review: the console change, by an independent pass.** Nothing blocking. The TypeScript ranges matched `IsPrivateIP` on 260,216 addresses with 0 mismatches.

Fixed:
- inaccurate comments, in the helper and in `root_handler.go` and its test;
- `window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS` is now optional in the type;
- an IPv4 host is flagged only when typed as a dotted quad. This matches the server's save check (`net.ParseIP`) and stops a half-typed public address from flashing "0.0.0.52".

Left as follow-ups:
- a shared URL fixture read by both `TestValidateURL` and `webhookTarget.test.ts`, so the two rule sets cannot drift;
- the server's save check misses Unicode spellings and IPv6 addresses with a zone. The dial-time guard refuses both;
- `config.js` exposes the boolean publicly. That's accepted: the OIDC and SMTP flags use the same channel.

## Release and disclosure (manual, outward-facing; after the final step passes)

1. Commit on your go. The release is v42, not 41.1 (Pierre, 2026-10-16): the unreleased 41.1 work ships as v42 together with B24's `v42` migration (`plans/fix-removed-contacts-list-membership-plan.md`, P2). It carries both fixes.
2. Accept GHSA-mvfg-323w-qgv9 and GHSA-88mf-6797-r264. Set affected to `<= 41.0`, patched to `42.0`, and the severity; the reporter proposed 7.7 for 88mf.
3. Request CVEs, and credit kta1kri (mvfg) and morgan-coded (88mf).
4. Publish both advisories together once 42.0 is tagged. Publishing mvfg first would point at 88mf.
5. Reply to both reporters now with the timeline. `SECURITY.md` promises an acknowledgement within 3 business days, and both are past it.

## Final step — Run the tests for the touched layers

```
go build ./... && go vet ./internal/... ./pkg/... ./config/...
make test-service
make test-http
go test -race ./internal/app/ ./pkg/safehttpclient/ ./config/
docker compose -f tests/compose.test.yaml up -d
INTEGRATION_TESTS=true go test -race -tags integration,licdev -timeout 20m ./tests/integration/ -run '^(TestAutomationWebhookNodeSSRFProtection|TestAutomation|TestWebhookDeliverySSRFProtection|TestWebhookWorkerEndToEnd|TestWebhookPayloadAndSignatureVerification|TestWebhookPayloadSamples|TestDemoAutomationsSeedRealHistory|TestLicenseGates|TestOIDCIntegration)$' -v
cd console && npx vitest run && npx tsc -b && npx eslint --max-warnings 0 src/components/automations src/vite-env.d.ts
```

- The `^…$` anchors matter: a bare `-run TestAutomation` matches every `TestAutomation*` suite.
- `TestWebhookPayloadSamples`, `TestWebhookWorkerEndToEnd` and `TestWebhookPayloadAndSignatureVerification` are the opt-out suites that deliver to loopback, so B3's refactor of the delivery client runs under them.
- Check the `--- PASS` lines, not the `ok`: without `INTEGRATION_TESTS=true` every test is skipped and still prints `ok`.
- If time allows, run `make test-unit` as the final gate.
