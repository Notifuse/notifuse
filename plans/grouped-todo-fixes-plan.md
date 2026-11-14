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
ceiling of 250, 250; `max_tasks=0` runs and reports the default 100; `tests/integration/task_handler_test.go:403-420`
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

## Lot 2: broadcasts, tasks, sending queue

The points that depend on #568, #569 and #570 (still open on 14/11: plain-text "task not found" errors, the
404 read from the error text, the non-UUID id of `tasks.execute`, the cancelled request on reset and trigger,
the log noise of `ExecuteTask`, the duplicated reset/trigger tests, the unverified error logs, the
status-only sub-test, `repo.Get` through `WithTransaction`, the doubled "failed to get task" prefix) wait for
those PRs and go to lot 7. The automation executor points (claims kept 5 min at shutdown, log noise of a
failing claim, the deadlock retry loop) go to lot 3.

### 2.1 One way for a run to hand its task back for later

`internal/domain/task.go`: a new error type `TaskRequeueError{After time.Duration; Reason string}`
(`Error()` gives the reason). A processor returns it when its task must run again later without counting a
failed attempt. `internal/service/task_service.go`, `ExecuteTask`: in the processor goroutine an error that
`errors.As` a `*TaskRequeueError` is not logged as "Task processing failed"; in the `processErr` branch, before
the cancellation check, it is handled with `MarkAsPending(bgCtx, workspace, taskID, now+After, task.Progress,
task.State)` (the same write as an unfinished run, so the saved state and the retry budget are kept), logged
at Info, and `ExecuteTask` returns nil. A failed `MarkAsPending` is returned as today's "failed to mark task
as pending" error.

Tests, `internal/service/task_service_test.go`: a processor returning `&TaskRequeueError{After: time.Minute}`
gives one `MarkAsPending` whose next run is about a minute away (`gomock` matcher on the time, ±5 s), no
`MarkAsFailed`, nil from `ExecuteTask`; wrapped in `fmt.Errorf("%w")` too; a failing `MarkAsPending` returns
the error.

### 2.2 F-26: a run that finds its broadcast paused parks the task

`internal/service/broadcast/orchestrator.go`, the loop's "paused during processing" branch returns
`false, &domain.TaskRequeueError{After: pausedRecheckInterval, ...}` with `pausedRecheckInterval = time.Minute`,
instead of `allDone = false`, which `MarkAsPending` made due at once, so each scheduler tick ran the task again
for nothing. Not 24 hours like the pause handler: a resume committed while this run still held the task finds
it held (`WakeTask` refuses), and only this delay brings it back. A resume after the park wakes it at once
(`WakeTask` takes a pending task). The deferred last-attempt finalizer leaves a `TaskRequeueError` alone.

Tests, `orchestrator_test.go`: a broadcast refreshed as paused returns the requeue error with a one-minute
delay and pauses the queue entries; on a last attempt the finalizer writes no status (no
`UpdateBroadcastStatusUnless`).

### 2.3 F-04: the final `processed` write respects a pause or a cancel committed meanwhile

`orchestrator.go`, the end of `Process` (today a full-row `UpdateBroadcast` on `context.Background()`): in
`WithTransaction`, `GetBroadcastForUpdateTx`, then
- `paused`: nothing written, the run returns the requeue error of 2.2 (the queue entries are paused already;
  after the resume the next run finds every recipient queued and writes `processed`);
- `cancelled` or `processed`: nothing written, `true, nil`;
- otherwise the final fields (status `processed`, `completed_at`, `winner_sent_at` for the winner phase,
  `enqueued_count`, the phase counts of `setPhaseRecipientCounts`, `updated_at`) are set **on the locked row**
  and written with `UpdateBroadcastTx`, so a column another writer changed since the run's last refresh is not
  written back.
The no-recipient `processed` write (`orchestrator.go:906`) goes through the same helper.

Tests, `orchestrator_test.go`: locked row paused (requeue, no write), cancelled (true, no write), processing
(written: the written broadcast holds the locked row's other columns, e.g. a `name` changed meanwhile, and
the final fields); the no-recipient path; a failing transaction returns the error.

### 2.4 F-08: no progress write past the run's lease

`orchestrator.go`: a helper `leaseContext(ctx)` returns `context.WithoutCancel(ctx)` bounded by `ctx`'s
deadline when it has one. `ExecuteTask` gives the processor a context whose deadline is the lease
(`timeoutAt + taskExecutionGrace`, the `timeout_after` it stored), so a write on it survives a shutdown's
cancellation (the cursor of the last batch is kept, as `context.Background()` kept it) but never runs past the
lease, after which another execution may own the task. Used by `SaveProgressState` (instead of
`context.Background()`), by the final write of 2.3 and by `handleTestPhaseCompletion`.

Tests: `SaveProgressState` with a context whose deadline has passed returns an error and the repository sees
a context already done (mock `SaveState` asserting `ctx.Err() != nil` or `ctx.Deadline()` equal to the
parent's); with a cancelled but not expired context the write goes through (context not done).

### 2.5 F-33: a last attempt whose finalization fails is retried, not failed

`orchestrator.go`, deferred finalizer: when `GetBroadcast` or the status write fails, the named `err` becomes
`&domain.TaskRequeueError{After: time.Duration(task.RetryInterval) * time.Second, Reason: ...}` (wrapping the
cause in the reason), so the task service puts the task back without marking it failed, and the next run
(still its last attempt) finalizes again. `stillLastAttempt` is unchanged (a failed read already keeps the
decision).

Tests: `GetBroadcast` fails in the finalizer → `Process` returns a `*TaskRequeueError` with the task's retry
interval; the status write fails → same; the write succeeds → the original error.

### 2.6 F-15 and F-27: the task of a scheduled broadcast is created in the request, and run by its ID

- `domain.TaskService` gains `ScheduleBroadcastTask(ctx, workspaceID, broadcastID string, sendNow bool,
  scheduledAt *time.Time) (taskID string, err error)` and `RunTaskNow(workspaceID, taskID string)`; the
  domain mock is regenerated.
- `TaskService.ScheduleBroadcastTask` holds today's `handleBroadcastScheduled` transaction and returns its
  error. An existing task of the broadcast (only an orphan of a schedule whose broadcast write rolled back
  can exist: only a draft can be scheduled) is reset: status pending, fresh `SendBroadcast` state, retry count
  0, next run now or at the scheduled time; a running one is refused with an error (the request fails, the
  user retries).
- `RunTaskNow` runs `ExecuteTask(context.Background(), workspaceID, taskID, now+MaxRuntime)` in a goroutine
  when `autoExecuteImmediate` is on (it is on exactly when this instance runs its own scheduler, i.e. direct
  execution), after reading the task's `MaxRuntime`; otherwise nothing (the scheduler or cron dispatches it).
- `BroadcastService.ScheduleBroadcast` calls `ScheduleBroadcastTask` inside its transaction, after
  `UpdateBroadcastTx`, instead of `PublishWithAck`; an error rolls the broadcast back and fails the request.
  After the commit, for a send-now, `RunTaskNow`. The `EventBroadcastScheduled` publish and
  `handleBroadcastScheduled` subscription go: the task service is its only subscriber.
- The resume's "start now" (`handleBroadcastResumed`, `ExecutePendingTasks(…, 1)` after a 100 ms sleep) uses
  `RunTaskNow(task.ID)` too.
- `orchestrator.Process` refuses a `draft` broadcast: the task is created before the broadcast commits, so the
  scheduler may pick it up in that window, and an orphan task must never send a draft. A draft is requeued
  after 30 s (2.1) while the task is younger than 10 minutes, and its task completed without sending after
  that (logged at Warn); a later schedule resets it (above).

Tests: `task_service_test.go` (`ScheduleBroadcastTask`: new task for now and for later with `NextRunAfter`;
existing pending or completed task reset; running task refused; repository error returned; `RunTaskNow` runs
the given task only, not another due one, and nothing when auto-execute is off); `broadcast_service_test.go`
(`ScheduleBroadcast`: the task scheduler's error fails the request and `UpdateBroadcastTx`'s transaction
returns it; send-now calls `RunTaskNow` with the returned ID after the commit, a later schedule does not);
`orchestrator_test.go` (draft: young task requeued, old task completed without loading templates);
integration, `tests/integration/broadcast_schedule_task_test.go` (new): send now on a workspace with a due task
of another broadcast runs this broadcast's task (its emails reach Mailpit) and leaves the other task pending
with its `next_run_after` unchanged.

### 2.7 F-29: the recipient feed's retry wait ends with the run

`internal/service/broadcast/data_feed.go:278`: `time.Sleep(retryDelay)` becomes a `select` on `ctx.Done()`
(returns `ctx.Err()` wrapped, so the run is seen as interrupted) and a `time.NewTimer(retryDelay)` (stopped
on the way out).

Test, `data_feed_test.go`: a server failing with 503, a retry delay of 10 s, a context cancelled after 50 ms:
`FetchRecipient` returns within 1 s with `errors.Is(err, context.Canceled)`.

### 2.8 F-31: shutdown keeps to its 70 seconds

- `cmd/api/main.go`, `runServer`: the wait after the shutdown signal also selects `ctx.Done()` and returns
  `ctx.Err()` (logged) when the 70 s run out.
- `internal/app/app.go`, `cleanupResources`: the connection manager's and the database's `Close` run through
  `closeWithin(ctx, name, fn)`, which returns when `fn` ends or `ctx` is done (logged at Error, the close goes
  on in its goroutine).

Tests: `cmd/api/main_test.go` with a fake app whose `Shutdown` blocks: `runServer` returns once the context
expires (the timeout injected through a package variable `shutdownTimeout`, set to 50 ms in the test);
`internal/app/app_test.go`: `closeWithin` with a blocking close returns at the context's end with an error,
and passes a close's error through.

### 2.9 F-03 and Exit on reply: the journey check before an automation email

Integration test first, `tests/integration/automation_exit_on_reply_last_email_test.go` (new): an Exit on reply
automation whose last step is an email; the contact enters; the email must reach Mailpit and the journey be
`completed`. If it fails as expected, `internal/service/queue/worker.go:753-775` changes:
- the email is dropped only when the journey is `exited` (a reply or a removal), not when it is `completed`
  or `failed`: the executor marks the journey completed in the same pass as its last email;
- a missing journey (`domain.ErrContactAutomationNotFound`, a new sentinel that
  `automation_postgres.go:962` wraps, text unchanged) is a finished journey: the email is dropped;
- any other lookup error fails closed (Pierre, 14/11): the entry goes back to the queue with
  `MarkAsFailed(…, &nextRetry)` like a failed send (`CalculateNextRetryTime(entry.Attempts)`), without a
  message history failure (nothing was sent) and logged at Warn. After its attempts the entry fails for good.
If the integration test passes as is, only the fail-closed and missing-journey changes are made, and the
exited-only rule is left out.

Tests, `worker_test.go`: completed journey sends; exited journey drops the entry (`MarkAsSent`, no
`SendEmail`); not found drops it; another error calls `MarkAsFailed` with a next retry and no `SendEmail`, no
`Upsert`. `automation_postgres_test.go`: the not-found error `errors.Is` the sentinel and keeps its text.

### 2.10 `exclude_unsubscribed` is fixed once the broadcast has sent

`internal/domain/broadcast.go:735`: an `audience.exclude_unsubscribed` that changes the stored value on a
broadcast that is neither draft nor scheduled is refused ("exclude_unsubscribed can only be changed on a draft
or scheduled broadcast"), as `tracking_mode` is. Tests, `broadcast_test.go`: paused + change refused; paused +
same value accepted; draft and scheduled accept the change.

### 2.11 `broadcasts.sendToIndividual`

`internal/service/broadcast_service.go`, `SendToIndividual`:
- the subject goes through `notifuse_mjml.ProcessLiquidTemplate(emailContent.Subject, templateData,
  "email_subject")`, as the queue sender does; an error fails the send;
- the message history row has no `broadcast_id` (Pierre, 14/11): the test send no longer counts in the
  broadcast's statistics;
- `WebVersion: false` in its template data: without `broadcast_id` the web version page refuses the message,
  so `view_in_browser_url` is left empty rather than a link to a 404.
`internal/http/broadcast_handler.go:60`: the route gets `restrictedInDemo`, as `broadcasts.schedule`.

Tests: `broadcast_service_test.go` (subject `Hello {{ contact.first_name }}` arrives rendered; an invalid
Liquid subject fails; the recorded message has `BroadcastID == nil`; template data without
`view_in_browser_url` value); `broadcast_handler_test.go` (demo mode answers as the other restricted routes).

### 2.12 The sender of each batch, and a resume with nothing left to queue

- `orchestrator.go`: before each `SendBatch`, the workspace is read again and the run's `emailProvider` and
  `integrationID` refreshed; `checkVariationSenders` runs on the batch's templates, and a missing sender pauses
  the broadcast as at the start of the run (`pauseForSendError`). A failed read keeps the run's copy (logged
  at Warn). Test: a provider that loses the sender between two batches pauses before the second `SendBatch`.
- `broadcast_service.go:707`, `ResumeBroadcast`: the sender check is also skipped when the broadcast's task
  state shows every recipient queued (phase single or winner, `RecipientOffset >= TotalRecipients > 0`), read
  with `taskRepo.GetTaskByBroadcastID`; a failed read keeps the check. The console's modal (lot 6) still decides
  on `completed_at`: known, not handled there. Tests: all queued → no sender check; test phase → check; read
  error → check.

### 2.13 Dead code and small fixes

- The circuit-breaker branches of `Process` (`orchestrator.go:634-643` in the finalizer, `:1430-1520` in the
  loop) go: the queue sender never returns `ErrCodeCircuitOpen` (`app.go` always passes `useQueueSender =
  true`, and the queue side pauses on an open circuit, `PauseForCircuitBreaker`). Their tests go too. The
  direct sender itself stays (known, not handled).
- `EmailQueueWorker.SetCallbacks`, `onEmailSent`, `onEmailFailed` and their two call sites go, with their tests.
- `PauseReason` writes the variation's name as `“%s”` instead of `%q`. Test: a name with `"` and `\` is shown
  as written.
- `ErrBroadcastShouldPause`'s text becomes "broadcast should pause until the user acts".
- The two unexported `checkVariationSenders` are renamed `requireKnownVariationSenders` (service) and
  `findUnavailableVariationSender` (orchestrator).

### 2.14 A/B tests

- `tests/integration/broadcast_ab_testing_e2e_test.go:365`, `testManualWinnerSelectionDuringTestPhase`: waits
  for `processed` and checks `test_phase_recipient_count + winner_phase_recipient_count` equals the audience
  and the winner phase count equals what was sent after the pick.
- `orchestrator_test.go`: one test per uncovered switch of the A/B fix (`orchestrator.go:1021` resume in
  `winner_selected`, and the three "winner already selected" guards at the end of the test phase, `:1260`,
  `:1304`, `:1381`), each asserting the winner phase is entered with the offset the test phase reached.

### 2.15 CHANGELOG (44.0)

Released behaviour, so Fix lines, short and client-facing:
- Fix: a broadcast paused while its last emails are queued stays paused, and a broadcast paused during a run
  no longer keeps its task busy.
- Fix: scheduling a broadcast fails when its sending task cannot be created, instead of leaving a broadcast
  that never sends; "send now" starts that broadcast, not another one that is due.
- Fix: a broadcast whose final status cannot be written is retried instead of failing; a run cut short no
  longer moves the progress of the next one back.
- Fix: with Exit on reply, the last email of a journey is sent; an email whose journey cannot be checked is
  retried later instead of being sent.
- Fix: the test send of a broadcast renders Liquid in the subject and no longer counts in the broadcast's
  statistics; it is unavailable in the demo.
- Improvement: `audience.exclude_unsubscribed` can only be changed on a draft or scheduled broadcast; shutdown
  keeps to its 70 seconds; a recipient feed retry stops with the run.
(The integration test of 2.9 decides the first half of the Exit on reply line.)

### Corrections after the plan review

The plan review (`/mnt/project-files/reviews/grouped-pr-lot2-plan-review.md`, Opus, xhigh) confirmed problems in
ten points. They replace what the points above say where the two differ.

- **2.1** The requeue does not go through `MarkAsPending`, which resets `retry_count`. A new repository method
  `RequeueTask(ctx, workspace, id, status, nextRunAfter, progress, state)` writes the status (pending, or
  paused for 2.2), progress, state and next run, clears the lease, keeps `retry_count`, and matches only
  `status = running` (else `ErrTaskNotRunning`, logged at Info: a cancel marked the task failed meanwhile, and
  that decision stands). `TaskRequeueError` gains `Park bool` (requeue as paused) and `AfterRequeue
  func(context.Context)`, run once the requeue is written. Tests: sqlmock cases for the method,
  `task_lease_test.go` case (a running task requeued keeps its retry count; a failed one is left failed).
- **2.2** The run that finds its broadcast paused pauses the queue entries, then breaks out of the loop (so the
  progress block after the loop runs) and returns a requeue with `Park: true`, 24 hours, and an `AfterRequeue`
  that reads the broadcast again and wakes the task (`WakeTask`, now) when it is no longer paused. The resume's
  wake moves from `handleBroadcastResumed` to `BroadcastService.ResumeBroadcast`, after its commit: either the
  resume's wake finds the parked task, or the run's second read sees the resume. Existing tests that change:
  `orchestrator_pause_test.go:170-172`, `:372-374`, the paused row of `:542-550`.
- **2.3** In the final write, the paused case also calls `pauseQueueEntries` and the cancelled case
  `deleteQueueEntries`, so the last batch's rows committed after the pause's or cancel's own statement are not
  sent. The draft check of 2.6 runs before the no-recipient path. Existing tests move from `UpdateBroadcast`
  to `WithTransaction` + `GetBroadcastForUpdateTx` + `UpdateBroadcastTx`.
- **2.4** `ExecuteTask` already bounds the processor's context by the lease. Added: the lease fence. `ExecuteTask`
  truncates the lease to microseconds, stores it, and puts it in the processor's context
  (`domain.WithTaskLease`); `SaveStateTx` adds `timeout_after = lease` when the context carries one and returns
  `ErrTaskNotRunning` when no row matches. No signature change, no schema change. `leaseContext` keeps the
  context values, so the fence holds on it too.
- **2.5** The finalizer requeues (keeping the retry count, so the next run is still the last attempt) only when
  no status write committed and the broadcast still exists: `ErrBroadcastNotFound` keeps the original error, and
  the path where `failed` was written and the pause write failed keeps it too. A write that keeps failing is
  retried once per retry interval.
- **2.6** `ScheduleBroadcastTask` runs in one system-DB transaction: `GetTaskByBroadcastIDTx` (`FOR UPDATE`),
  then `CreateTx`, or a conditional reset (`ResetBroadcastTask`, one `UPDATE` guarded by `notHeld`, which sets
  `created_at = now`, state, `retry_count = 0`, pending, next run). A task held by a live lease is refused.
  `GetTaskByBroadcastIDTx` wraps `domain.ErrTaskNotFound` (same text) so "no task" is told from a failed read.
  The draft refusal is the first thing `Process` does on every run. A send-now task is created with
  `next_run_after = now + 30 s` so the scheduler and cron cannot pick it before the broadcast commits;
  `RunTaskNow` ignores `next_run_after`. The resume and `SelectWinner` call `RunTaskNow(task.ID)` after their
  commit instead of `ExecutePendingTasks(…, 1)`. `RunTaskNow` logs `ErrTaskAlreadyRunning` at Debug. The
  integration test goes into `broadcast_scheduled_task_test.go`, with `TaskScheduler.Enabled = true` in the
  suite's config, no ticker, the queue worker started, and the other task with a NULL `next_run_after` and an
  older `created_at`. Existing tests to rewrite: `task_service_test.go:869`, `:882-930`, `:2084-2410`,
  `qb_task_gap_test.go:1079`, and the `ScheduleBroadcast` cases that mock `PublishWithAck`.
- **2.10** (commit `10b7fea`) Both guards (`exclude_unsubscribed` and `tracking_mode`) also refuse the change
  when `StartedAt` is set: a broadcast scheduled for later keeps `scheduled` while it sends.
- **2.12** No extra sender check: the batch's `SendBatch` already checks the senders of the provider it is
  given. The fix is the refreshed provider; a refresh that finds no marketing provider keeps the run's copy
  (logged at Warn), so the batch fails as it would have. The 44.0 sender bullet (CHANGELOG.md:83) is amended.
- **2.13** The feed pause reason no longer carries the sentinel's text (done in `af05284`).
- **2.14** The three guards at `:1260`, `:1304`, `:1381` cannot be reached and are removed; only the `:1021`
  test is written.
- **2.15** As the review's table says: F-33 and F-08 as one Improvement, the Exit on reply line split, the
  Improvement line split per change, the 44.0 web version bullet names the test send.

Known, not handled (lot 2): the terminal writes of `ExecuteTask` have no lease fence; the pause handler parks the
task before the pause commits; `RunTaskNow` runs on `context.Background()` and is not released at shutdown;
the sending-started annotation of a resume during a paused run; the small gaps left by #566.

## Test commands for lot 2

```
go test ./internal/domain/ ./internal/service/... ./internal/repository/ ./internal/http/ ./internal/app/ ./cmd/api/
INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestBroadcast|TestAutomation|TestTask|TestScheduledBroadcast|TestDataFeed|TestRecipientFeed|TestSendTimeSender|TestCircuitBreaker|TestCron|TestRecurringTask|TestV44' -count=1
make test-integration  # the full suite once before the code review
```
