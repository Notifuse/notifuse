# Grouped fixes: the small points left open by earlier PRs

## Why

Earlier PRs each left a list of small points "known, not handled": edge cases, missing tests, comments that
drifted, small UX gaps. One PR collects them instead of one PR each. The list was reviewed point by point
against `dev` (48cfbc8a) before this plan; points that were wrong or already fixed were dropped.

The PR is built in lots, one area of the code each. Each lot has its section here, reviewed before its code
is written, then its own commits and its own code review. A point that turns out larger than its
description, or that needs a schema or environment variable change not already decided, is not done here:
it goes to the PR's "known, not handled" list.

## Lot 1: tracking links, custom domains, web version, public webhook and cron routes

Reviewed before code (spec review, Opus xhigh). What the review changed: 1.1 no longer reorders the checks (the
reorder fixes nothing), 1.4 runs after the index builds and keeps keys it cannot read, 1.6 is dropped
(unreachable), 1.14 waits for Pierre's decision, 1.15 clamps instead of refusing, the CHANGELOG folds into
the unreleased 44.0 lines.

### 1.1 Custom domain: tests of the uniqueness check, and a host that cannot be read

The point "the error tells whether a domain is used, before the DNS check" is not fixed here: running the DNS
check first changes nothing, since `VerifyDomainOwnership` (`internal/service/dns_verification_service.go:43`)
only checks that the domain points to the instance, which another workspace's domain already does. The real
fix is a per-workspace TXT token: known, not handled.

What is done:
- `dnsVerificationService` becomes a small interface declared in the service package
  (`customDomainVerifier { VerifyDomainOwnership(ctx, url string) error }`, `//go:generate mockgen`), satisfied by
  `*DNSVerificationService`. `covWorkspaceService` (workspace_service_integration_coverage_test.go:68) gets an
  accepting fake by default, so no test reaches a real resolver or the nil logger of `&DNSVerificationService{}`.
- On a change of custom endpoint only (a stored value never blocks an unrelated save), a custom endpoint whose
  `trackingtoken.HostOf` is empty is refused with a validation error: the uniqueness check skips such a host.

Tests, `internal/service/workspace_custom_domain_test.go`:
- the sub-test "lets a workspace edit the URL of a domain it shares" uses the mock verifier, expects
  `VerifyDomainOwnership` once and `Update` exactly once, and asserts no error (today `Update` is `AnyTimes` and
  the assertion conditional);
- a URL change that keeps the host calls the verifier (a mutant that skips the DNS check when the host is the
  same fails);
- `https://%FF.example` (empty `HostOf` after 1.7) is refused, `Update` not called.

### 1.2 Cap the forwarded hosts read per click

`internal/http/email_handler.go`, `clickOwnHosts`: every entry of every `X-Forwarded-Host` value is kept, and
each costs a hostname comparison in `sanitizeClickedURL`. With 1.9 the forwarded entries are those of
`X-Forwarded-Host` then the `host=` of `Forwarded`; keep only the **last** `maxForwardedHosts = 8` of them (the
proxies nearest the server append last, so a client cannot push them out). `r.Host`, the API host and the
endpoint host are always kept. The function comment ("every X-Forwarded-Host entry") is updated.

Tests, `internal/http/forwarded_hosts_test.go` (new) and the existing `clickOwnHosts` tests: 20 entries across
two header values give `r.Host`, the last 8 entries and the two endpoint hosts, in that order; empty entries do
not count.

### 1.3 (dropped) Fixed secret key in development

Pierre decided (14/11) to leave it as it is.

### 1.4 v44 removes the instance's own links recorded before 44.0

Decided by Pierre (14/11): v44, not released, removes from `clicked_links` the keys whose host is the
instance's, with the rule of `sanitizeClickedURL` (any port, case-insensitive, one trailing dot). Before 44.0,
behind a reverse proxy, unsubscribe and notification center links were recorded with the contact's address.

`internal/migrations/v44.go`, a new statement **after the three index builds and before the exited recount**:
once the SHARE lock is held no other transaction holds row locks on `message_history`, so the purge cannot
deadlock with a writer (the review reproduced the deadlock with the purge placed before the builds). The purge
writes no timeline entry (the link step only writes for keys that become verified), so it never touches
`automations`, and the order "message_history, then automations" of the type comment still holds.

```sql
UPDATE message_history mh
SET clicked_links = (SELECT COALESCE(jsonb_object_agg(e.key, e.value), '{}'::jsonb)
                     FROM jsonb_each(mh.clicked_links) e
                     WHERE NOT COALESCE(<host of e.key> = ANY($1), false))
WHERE jsonb_typeof(mh.clicked_links) = 'object'
  AND EXISTS (SELECT 1 FROM jsonb_object_keys(mh.clicked_links) k WHERE <host of k> = ANY($1))
```

- `<host of k>`: `regexp_replace(btrim(lower(substring(k from '(?i)^\s*https?://(?:[^/?#@]*@)?(\[[^]]*\]|[^/?#:]*)')), '[]'), '\.$', '')`.
  A key the expression cannot read gives NULL and is kept (`COALESCE(…, false)`). A key whose hostname is
  written in Unicode is not converted to Punycode (no IDNA in SQL); the instance's hosts are ASCII in practice.
- `$1`: for the API endpoint and the workspace's current custom endpoint, `trackingtoken.HostOf` and the
  lowercase literal hostname (they differ for a non-canonical IPv6 literal, 1.8), empty ones dropped,
  duplicates removed. With none, the statement is skipped.
- `clicked_at` and the other keys stay, `updated_at` is not touched. Every key on those hosts goes, blog posts
  served on the custom domain included (`sanitizeClickedURL` leaves them out too since 44.0); keys on a former
  custom domain stay. Idempotent: a second run matches no row.
- Cost: the purge reads every row with clicked links while the SHARE lock is held, so sending and tracking wait
  for it too. The type comment and the CHANGELOG line of the index builds say so.
- `v44WorkspaceStatement` gains an `args` field.

Tests:
- `internal/migrations/v44_test.go`: the expected statements include the purge, in its place, with its
  argument (sqlmock `WithArgs`); without any endpoint host it is not run; its error names the step.
- `tests/integration/v44_own_links_purge_test.go` (new), on a real workspace database: keys on the API host
  (port, upper case, trailing dot), on the custom domain, on another host, an unreadable key (`https:/x`) beside
  an own key, a row with only own keys. After the migration: own keys gone, the others and `clicked_at` kept,
  the only-own row is `{}`, no `email.link_clicked` entry written, a second run changes nothing.

### 1.5 The web version renders the language that was sent

`internal/service/web_version_service.go:294` resolves with the workspace's current default language.

- `internal/domain/template.go`: `(*Template).ResolvedLanguage(contactLanguage, defaultLanguage) string` (the
  translation key `ResolveEmailContent` picks, `""` for the base content, also for a translation whose `Email` is
  nil) and `(*Template).EmailContentForLanguage(lang string) *EmailTemplate`. `ResolveEmailContent` becomes
  `EmailContentForLanguage(ResolvedLanguage(...))`, so the two cannot drift.
- `EmailQueuePayload.RenderedLanguage *string` (`json:"rendered_language,omitempty"`), set by
  `internal/service/broadcast/queue_message_sender.go` and `internal/service/automation_node_executor.go`.
  (`message_sender.go`, the direct sender, is unused in production; lot 2 removes it from the orchestrator path.)
- `internal/service/queue/worker.go`, `upsertMessageHistory`: a set value goes to
  `MessageData.Metadata["rendered_language"]`, stored unencrypted. The upsert's `ON CONFLICT` keeps the first
  insert's `message_data`, which already holds it.
- `Render`: a string `rendered_language` in the metadata picks the content with `EmailContentForLanguage`;
  without it (messages sent before), today's resolution.
- `SendToIndividual` loses its `broadcast_id` in lot 2 (decided by Pierre), so its web version is not rendered
  and needs nothing here.

Tests: `internal/domain/template_test.go` (`ResolvedLanguage`: no translations, default language, known key,
unknown key, key with nil `Email`; `EmailContentForLanguage`: `""`, known key with sender fallback, unknown key);
`queue_message_sender_test.go` and `automation_node_executor_test.go` (payload carries `"fr"` / `""`);
`queue/worker_test.go` (metadata key set, absent without the field); `web_version_service_test.go` (sent in `fr`,
default now `fr`: French content; `""` with a French contact: base content; no metadata: old resolution);
`internal/repository/message_history_postgre_test.go` or an integration test: the metadata survives the
encrypted round trip.

### 1.6 (dropped) Click route and endpoints written in NFD or full width

Unreachable in production: Go's resolver refuses a non-ASCII name before any query, so the DNS check refuses
every non-ASCII custom endpoint. Known, not handled.

### 1.7 `HostOf` and invalid UTF-8

`pkg/trackingtoken/token.go:89`: `HostOf("https://%FF.example")` gives `xn--zn7c.example`. Return `""` when the
hostname is not valid UTF-8; with 1.1, such an endpoint is refused at save. The uppercase sharp s and the Bidi
rule stay (Go's IDNA tables; known, not handled).

Tests, `TestHostOf`: `%FF` → `""`; `https://straße.example` → `xn--strae-oqa.example` (pins `Transitional(false)`,
transitional mapping gives `strasse.example`); `https://-ü.example` → `xn----eha.example` (pins
`CheckHyphens(false)`). `Transitional(false)` stays: it has an effect.

### 1.8 IPv6 literals in their canonical form

`HostOf`: a hostname that `netip.ParseAddr` reads is returned as `addr.String()` (zone kept, IPv4 unchanged).
`internal/http/tracking_redirect.go`, `classifyTrackedDestination`: an IP-literal destination host is
canonicalized the same way before the comparison (an IP literal cannot be a look-alike), so
`API_ENDPOINT=https://[2001:0DB8::1]` keeps redirecting its own unsigned links.

Tests: `TestHostOf` (both IPv6 spellings, an IPv4 literal, a zone); `tracking_redirect_test.go` (non-canonical
API endpoint and destination redirect).

### 1.9 Read `Forwarded: host=` (RFC 7239)

Helper `forwardedHosts(h http.Header) []string`: the `X-Forwarded-Host` entries, then the `host=` parameters of
`Forwarded` (case-insensitive name, quoted value unquoted), empty values skipped. `clickOwnHosts` keeps the last 8
(1.2). `webVersionRequestHost` keeps today's rule for `X-Forwarded-Host` (its first entry, if not empty) and
otherwise takes the first `Forwarded` host, else `r.Host`; `X-Forwarded-Host: , b` still falls back as today.

Tests, `internal/http/forwarded_hosts_test.go`: `Forwarded: for=1.2.3.4;host=a.example, host="b.example:8443"`,
`Host=` in mixed case, both headers (order), empty entries; `webVersionRequestHost` with only `Forwarded`, and with
an empty first `X-Forwarded-Host` entry.

### 1.10 The `status == WorkspaceKnown` guards of the click routes

`internal/http/email_handler.go:195` (`/visit`) and `:376` (`/r/`): the guards that only read `ws.EndpointHost`
for a known workspace are not covered. Keep them and test both in `internal/http/email_handler_test.go`: a
lookup answering `WorkspaceUnavailable` with a non-empty `EndpointHost`, a destination on that host gets the
confirmation page; on `/r/`, `VisitLink` receives own hosts without it.

### 1.11 `/visit` passes no own hosts

`handleClickRedirection` records with an empty URL, which `sanitizeClickedURL` never reads hosts for. Pass `nil`,
drop the `endpointHost` lines, remove the `/visit` sub-test that asserted the list.

### 1.12 One host comparison

`tracking_redirect.go`: `hostnameOf` and `asciiHostEqual` duplicate `trackingtoken.SameHost` / `SameHostname`
(without the trailing dot). Use the trackingtoken functions; `lowerASCII` goes. The case
`"https://track.brand.com./u"` of `tracking_redirect_test.go:42` flips to a redirect (the recorder already trims
the dot), `TestAsciiHostEqual` is deleted; the Kelvin-sign case stays a confirmation page (also pinned by
`TestSameHost`).

### 1.13 Demo: the clicked-link error branch

`internal/service/demo_service.go:2042`: test in `demo_service_test.go` where `SetClicked` fails for the first
clicked message: the second is still updated and the function returns nil.

### 1.14 A rate limit on `/webhooks/email` (F-38): waiting for Pierre

The review showed that a 429 loses events (SNS retries for about 60 s, Mailgun never retries delivery
notifications), that a per-IP limit caps the whole instance and is bypassed through `X-Forwarded-For`, and that
`pkg/ratelimiter` costs about 150 KB and 60 µs per call at such sizes, under a lock shared with sign-in. Pierre
chooses between leaving it out (recommended), a high per-workspace limit, or the original plan. Not coded until
then. The rate limiter's cleanup, which resets every namespace holding a colon at each tick
(`pkg/ratelimiter/ratelimiter.go:196-213`), is a separate defect: known, not handled.

### 1.15 `/api/cron`: a ceiling on `max_tasks` (F-24)

Decided by Pierre (14/11): a ceiling only. A refusal would silently stop an external crontab that sends more,
so the value is clamped: `TaskHandler.SetCronTaskCeiling(n)` (called in `internal/app/app.go` with
`max(100, TASK_SCHEDULER_MAX_TASKS)`, 100 when unset) and `ExecutePendingTasks` receives
`min(max_tasks, ceiling)`; a value of 0 or less keeps meaning the default (100), as today
(`task_service.go:386`). The response's `max_tasks` is the value used. A setter rather than a constructor
argument keeps the open PRs that construct the handler unchanged.

Tests: `internal/http/task_handler_test.go`: `?max_tasks=1000` runs with 100 and answers `max_tasks: 100`; with a
ceiling of 250, 250; `max_tasks=0` keeps 0 (the service default); `tests/integration/task_handler_test.go:403-420`
(`max_tasks=0`, 202) stays green.

### 1.16 CHANGELOG (44.0)

The web version, the click route's host rule and the uniqueness check are unreleased (#502, #507): no new lines
for them, their 44.0 lines are amended.
- Line 57 (clicks behind a reverse proxy): `Forwarded` is read like `X-Forwarded-Host`; links on the
  instance's hostname on another port, and blog links on the custom domain, are not recorded per link either
  (the transverse point on what the rule leaves out); "Links recorded before the upgrade stay" becomes:
  upgrading to 44 removes the ones recorded on the API endpoint or the current custom domain.
- Line 47 (index builds): the wait also covers the removal of those links.
- Line 73 (web version): it shows the language the email was sent in; `Forwarded` is accepted beside
  `X-Forwarded-Host`.
- New line, Improvement: `/api/cron` runs at most 100 tasks per call, or `TASK_SCHEDULER_MAX_TASKS` when higher.

## Test commands for lot 1

```
go test ./pkg/trackingtoken/ ./internal/domain/ ./internal/service/... ./internal/http/... ./internal/migrations/ ./internal/app/
INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestV44|TestTaskHandler|TestWebVersion|TestTracking' -count=1
make test-integration  # the full suite once before the code review
```
