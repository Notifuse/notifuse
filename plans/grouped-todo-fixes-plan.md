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

Implementation notes (lot 2): a send-now task is due 5 s ahead, not 30 s: where tasks run through the cron
or a scheduler that dispatches over HTTP, `RunTaskNow` does nothing, and 30 s delayed every send now there
(`TestBroadcastSingleExecution_E2E_WinnerPickedThroughTheAPIDuringTheTestPhase` timed out on it). A run that
finds the broadcast still a draft hands the task back, so the shorter delay only costs a 30 s wait in the rare
case the broadcast has not committed. The three "winner already selected" guards are removed (2.14), and
`TestTask_StaleRunning_Reaped_Issue317` seeds its task for a scheduled broadcast, since a run no longer sends a
draft's.

Known, not handled (lot 2): the terminal writes of `ExecuteTask` have no lease fence; the pause handler parks the
task before the pause commits; `RunTaskNow` runs on `context.Background()` and is not released at shutdown;
the sending-started annotation of a resume during a paused run; the small gaps left by #566.

## Test commands for lot 2

```
go test ./internal/domain/ ./internal/service/... ./internal/repository/ ./internal/http/ ./internal/app/ ./cmd/api/
INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestBroadcast|TestAutomation|TestTask|TestScheduledBroadcast|TestDataFeed|TestRecipientFeed|TestSendTimeSender|TestCircuitBreaker|TestCron|TestRecurringTask|TestV44' -count=1
make test-integration  # the full suite once before the code review
```

## Lot 3: the automation executor and its step statistics

Reviewed before code (Opus xhigh, `/mnt/project-files/reviews/grouped-pr-lot3-plan-review.md`; corrections at the
end of the lot). No schema change: every point is Go code or a statement on existing columns.

### 3.1 A journey whose `context` is not a JSON object no longer holds its batch

`internal/repository/automation_postgres.go`, `ClaimScheduledContactAutomations`: a `context` that does not
decode into a map (an array, a scalar) is read as a nil context instead of failing the whole claim, so the
rest of the batch runs at once and that journey runs with an empty context (it was unreadable anyway; its
next persist writes `null`).

Test, `automation_postgres_test.go` (sqlmock): three claimed rows, the middle one with context `[1]`: three
journeys returned, the middle one with a nil `Context`, the others with theirs.

### 3.2 The writes after a step outlive a cancelled run

`internal/service/automation_executor.go`, `Execute`: once a node has run, the writes that record it
(`UpdateNodeExecution`, `UpdateContactAutomationIfActive`, the stat, the end event, and the writes of
`handleError`) run on `context.WithoutCancel(ctx)` bounded by `afterStepWriteTimeout = 10 * time.Second`. A
node whose own execution fails because the run's context is done (`ctx.Err() != nil`) is not a failed
attempt: its execution row is marked `failed` with a `completed_at` (so it counts no pass) and the journey is
left as it is, due again when its claim ends, with no retry spent. A shutdown in the middle of a step then
leaves no `processing` row behind; a process killed outright still does (known, not handled; the comment of
`nodeStatsVisitsSQL` is narrowed to that case).

Tests, `automation_executor_test.go`: a node that returns `context.Canceled` with a cancelled context: the
execution is updated to failed with a `CompletedAt`, no `UpdateContactAutomationIfActive`, no failed entry
created, `RetryCount` unchanged; a node that succeeds while the context is cancelled meanwhile (the mock
cancels it): the persist and the completed update still reach the repository with a context that is not
done.

### 3.3 A shutdown hands its unrun claims back

`ProcessBatch`: on `ctx.Err()`, the journeys claimed and not run yet get their claim released: a new
repository method `ReleaseContactAutomationClaim(ctx, workspaceID, id, token)` sets `scheduled_at = NOW()`
where `id`, `status = 'active'` and `scheduled_at = token` (a journey claimed again since, or ended, is left
alone). It runs on `WithoutCancel` with a 5 s bound; an error is logged at Warn and the claim ends on its own.
The journeys lose their order by due time (all were due), which does not matter to the scheduler.

Tests: sqlmock for the method (query, arguments, `RowsAffected` 0 and 1); `automation_executor_test.go`: a
context cancelled after the first journey releases the second and third with their tokens and does not run
them; `tests/integration/automation_claim_test.go`: a claim of three journeys, a cancelled `ProcessBatch`,
then the three are due now (`scheduled_at <= now()`), not in 5 minutes.

### 3.4 A failed persist after a step records the failure on that step

`Execute`, the path where `UpdateContactAutomationIfActive` fails after node X ran:
- X's execution row is marked `completed` with its output first (X did run; its output is needed by
  `buildContextFromNodeExecutions`), then `handleError` writes its failed row on X, not on the next step Y
  that `CurrentNodeID` already names. `handleError` gains the node the failure belongs to as a parameter;
  the other callers pass `ca.CurrentNodeID`.
- On a terminal step (`NextNodeID == nil`), the status is put back to `active` before `handleError`, so the
  retry does not persist `completed` with a retry time: the next run finds no current node and completes
  through `markAsCompleted`, which writes the stat and the end event.

Tests, `automation_executor_test.go`: persist fails on a step with a next one: the failed entry is on X, X's
execution is completed; persist fails on a terminal step: `handleError`'s persist writes status `active`,
then a run with `CurrentNodeID == nil` writes `completed`, the stat and the end event.

### 3.5 Completed, exited and failed are counted only when the journey's row changed

`markAsCompleted`, `markAsExited` and the failed branch of `handleError` increment the stat and write the end
event after `persistIfActive`, and only when it updated the row (`persistIfActive` returns `(bool, error)`).
The inline completion and exit of `Execute` already persist first and stop on `!updated`.

Tests: `UpdateContactAutomationIfActive` returning false: no `IncrementAutomationStat`, no timeline entry, for
each of the three; returning true: one each.

### 3.6 `nodeStats` compares a delay with the application's clock

`internal/repository/automation_stats_postgres.go`, `nodeStatsInPlaceSQL`: `now()` becomes `$2`, the time
passed by `GetNodeStats` (`time.Now().UTC()`), the clock that wrote `delay_until`. Test: the sqlmock
expectation takes the second argument.

### 3.7 A claim that keeps failing logs once every five minutes

`ProcessBatch` ("Failed to claim scheduled contacts in some workspaces") and `AutomationScheduler.processBatch`
("Failed to process automation batch") log an error whose text is the one they logged less than 5 minutes ago
at Debug instead of Error. One small type in the service package, `repeatedErrorLog` (a mutex, the last text,
the time), shared by both. Tests: same text twice within the window: Error then Debug; another text: Error;
same text after the window (injected clock): Error.

### 3.8 The automation handlers answer 404 for an unknown automation

`internal/http/automation_handler.go`: `writeUpdateFailure`, `handleDelete`, `handleActivate` and
`handlePause` answer 404 on `*domain.ErrNotFound`, as `handleGet` does. `DeleteTx` returns
`&domain.ErrNotFound{Entity: "automation", ID: id}` when no row was soft-deleted. Tests: one 404 case per
handler (`automation_handler_test.go`); sqlmock `DeleteTx` with 0 rows: `errors.As` the type; integration
(`tests/integration/automation_*`): delete, activate, pause and update of an unknown ID answer 404.

### 3.9 Deleting an automation counts the journeys it exits

`DeleteTx`: the exit and the `stats.exited` increment are one statement, as `ExitContactJourneysOnReply` does
(a CTE, with the same non-object guard), and it runs on `tx` when one is given. Tests: sqlmock of the
statement; integration: two active journeys, delete, `stats.exited` raised by two.

### 3.10 The reply exit's deadlock loop

`ExitContactJourneysOnReply`: `updated_at` is taken again on each attempt; when the last attempt is still a
deadlock, the error says so ("failed to exit contact journeys on reply after %d deadlocks: %w"), so the caller's
log names it. Test "another error is not retried" asserts the error contains "fk violation" (a retried
error would fail on the unexpected second query, which `ExpectationsWereMet` does not see), and a deadlock
test asserts the attempt count in the text.

### 3.11 Small validation fixes

- `internal/domain/automation.go`, `validateLinkClickFilters`: `broadcast_id` and `template_id` longer than
  `MaxTriggerIDFilterLength = 64` are refused. Tests: 64 accepted, 65 refused, for each.
- `with_counts` (`list.go:173`) and `with_journey_counts` (`automation.go:1093`): "must be a boolean". Tests
  updated.

### 3.12 Tests

- `automation_claim_test.go:129`: `activeJourneys` takes `runCtx`.
- `automation_node_stats_test.go`: a delay followed by an email step: the email step's visits are 0 while the
  delay runs; automation C's sub-tests each set up their own automation and journey.
- `automation_stats_postgres_test.go`: `notPassedQuery` checks `e.contact_automation_id = ca.id`, `visitsQuery`
  checks the `completed_at IS NULL` of the failed attempts.
- `nodeStats` SQL details (integration, `automation_node_stats_test.go`): an email step with one completed
  execution whose output has an error and one without: `errors` 1 and `completed` 1; an email execution with
  no message and no queue entry is not queued; an A/B split counted by `variant_id`, with a `variant_name`
  that differs; the transaction is read-only (a sqlmock test asserting `BeginTx` with `ReadOnly: true` and
  `sql.LevelRepeatableRead`).
- `TestAutomation/LongChainWithoutDelay`: run with `-count=10`; if it fails, the wait it shows is fixed, else
  it is listed as not reproduced.

### 3.13 CHANGELOG (44.0)

Each point extends a 44.0 line, which is unreleased, rather than adding a Fix:
- the `automations.get` 404 line names update, delete, activate and pause too;
- the reply exits line ("count in its exited total") adds the journeys an automation's deletion exits, and that
  an exit landing during a step is no longer counted twice;
- the claim line (several instances) adds that a shutdown hands back the journeys it had not run, that a journey
  whose saved context cannot be read no longer delays the others, and that a failing claim logs once every
  5 minutes;
- the `automations.nodeStats` line: a shutdown no longer adds a pass, a failed save is counted on its step, and
  waiting journeys follow the application's clock;
- the "Clicked a link" trigger line: broadcast and template filters are limited to 64 characters.

Known, not handled (lot 3): the cost of `nodeStatsNotPassedSQL` on many long waits is not measured again; a
process killed in the middle of a step still leaves a `processing` row.

### Corrections after the plan review (lot 3)

The plan review confirmed seven blocking and six minor problems. These replace what the points above say where
the two differ.

- **3.6 is dropped.** A delay ends when the claim finds `ca.scheduled_at <= NOW()`, the database clock
  (`automation_postgres.go:1205`), which is the clock `nodeStatsInPlaceSQL` already uses. The TODO point is closed
  with that evidence, and its CHANGELOG half-sentence goes.
- **3.4, exits.** A step that ends the journey with `Exited` and no next step is not put back to `active`. The
  retry keeps `CurrentNodeID == nil` and `ExitReason`, and a run that finds no current node with an `ExitReason`
  calls `markAsExited(*ExitReason)`; without one, `markAsCompleted`. Test: an email step on an unsubscribed
  contact whose save fails, then the next run writes `exited` with reason `unsubscribed`, the exited stat and the
  end event.
- **3.4, the failure is not a failed attempt on any step.** X's execution is marked completed (the email counters
  and the delay's waiting state need that row); the save failure writes no failed execution row, `retry_count`
  and `last_error` record it, and the journey retries from Y, which is what runs next. `handleError` keeps the
  later of its backoff and the `ScheduledAt` the step set, so a delay keeps its resume time.
- **3.2, the loop stops on a cancelled run.** Each iteration starts with `if ctx.Err() != nil { return }`: the
  previous persist left the journey due when its lease ends. Test: trigger then delay, the context cancelled
  during the trigger's persist: the delay is neither run nor persisted.
- **3.2, every `handleError` caller.** When the run's context is done, `handleError` writes nothing (no retry
  spent, no failed row, no stat), whichever caller it has, including the reads of `GetByID` and
  `GetContactByEmail`. Test: `GetByID` returns `context.Canceled` on a cancelled context: no
  `UpdateContactAutomationIfActive`, no failed entry, `RetryCount` unchanged.
- **3.2, claims narrowed.** A shutdown can still leave a `processing` row: write errors stay ignored, and the
  write bounds can outlast `AutomationScheduler.Stop`'s 5 s wait, after which the pools close. The comment of
  `nodeStatsVisitsSQL` keeps "an attempt whose failure was not written". The concurrent-exit path (`!updated`)
  marks the step completed, since the step ran. Bounds: 5 s per after-step write, 3 s for all the releases of
  3.3. Known, not handled: the scheduler goroutine is not awaited by `cleanupResources`.
- **3.3, tests.** The integration test cancels `runCtx` from the first journey's webhook receiver; journeys 2 and
  3 are due now, journey 1 is not due (held) or has ended. The unit test "on shutdown the journeys left keep
  their claim and are not renewed" (`automation_executor_test.go:2348`) and the comment at
  `automation_executor.go:258` are rewritten for the releases.
- **3.12, read-only transaction.** go-sqlmock ignores `BeginTx` options, so the test uses a small fake
  `driver.Connector` whose connection implements `driver.ConnBeginTx` and records the `driver.TxOptions`.
- **3.9 is reduced to a comment.** A deleted automation's stats are never read (`GetByIDTx` filters
  `deleted_at`, no caller lists deleted ones, the v44 recount skips them), so `DeleteTx` gets a comment saying
  its exits are not counted and why, and it still runs on `tx` when one is given. No CHANGELOG change.
- **3.11.** The `with_journey_counts` message also changes in `openapi/paths/automations.yaml` and the bundled
  `openapi.json`.
- **3.7.** `repeatedErrorLog` works as a zero value (a value field, the clock defaulting to `time.Now`), since
  tests build the executor by struct literal.
- **3.10.** A test pins that `updated_at` is taken again on each attempt (two attempts, two distinct times).
- **Reached going down.** The notifuse half (the nodeStats description in `openapi/paths/automations.yaml` and
  `openapi.json`) is done with the docs half.
- **CHANGELOG.** Four points fix behaviour that shipped in 43.0 and get their own line:
  - "Fix: A journey whose saved context is not a JSON object no longer stops the automations of its
    workspace." (3.1)
  - "Fix: An automation step that finishes while Notifuse shuts down is saved, instead of running again after
    the restart (an email step could send its email twice)." (3.2)
  - "Fix: When saving a journey's last step fails, the journey is now counted as completed or exited and its end
    shows in the contact's activity." (3.4)
  - "Fix: A journey ended by a reply or by its automation's deletion while one of its steps runs is no longer
    counted as completed or failed, nor shown as such in the contact's activity." (3.5)

  The 404 Fix line names update, delete, activate and pause. 3.3, 3.7 and 3.11 fold into their 44.0 lines,
  worded as behaviour ("a shutdown hands back the journeys it has not run"), never "no longer".

## Test commands for lot 3

```
go test ./internal/domain/ ./internal/service/ ./internal/repository/ ./internal/http/
INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestAutomation' -count=1
```

## Lot 4: the tasks API, the email webhook's statuses, the compile request

Reviewed before code (to be filled in after the review). #568, #569 and #570 are merged in dev, so the points
that waited for them are done here.

### 4.1 A missing task is `domain.ErrTaskNotFound` everywhere, and recognised only as such

- `internal/repository/task_postgres.go`: `UpdateTx`, `Delete`, `MarkAsCompletedTx` and `MarkAsFailedTx`
  return `domain.ErrTaskNotFound` (same text) instead of `fmt.Errorf("task not found")`.
- `internal/http/task_handler.go`: `GetTask`, `DeleteTask` and `authorizeExistingTask` answer 404 on
  `errors.Is(err, domain.ErrTaskNotFound)` alone; the `strings.Contains(err.Error(), "not found")` checks go.
- `internal/service/task_service.go`, `ResetTask` and `TriggerTask`: a failed read is wrapped as
  "failed to load task: %w" (the repository already says "failed to get task").

Tests: sqlmock, each of the four methods with 0 rows: `errors.Is(err, domain.ErrTaskNotFound)`;
`TestTaskRepository_GetWithTransaction` gains the no-row case through `Get`; handler tests: an error whose
text contains "not found" but is not the sentinel answers 500 on get, delete and the authorization read; the
service tests of reset and trigger assert the error text has the prefix once.

### 4.2 `tasks.execute` refuses an ID that is not a UUID

`internal/domain/task.go`, `ExecuteTaskRequest.Validate`: `uuid.Parse(r.ID)` fails → "task id must be a
UUID" (400 through the handler's existing branch). Tests: domain (valid UUID, `not-a-uuid`), handler (400,
no service call).

### 4.3 Reset and trigger cancelled by the client

`ResetTask` and `TriggerTask` handlers: an error that `errors.Is(err, context.Canceled)` is logged at Debug
(no Error), and answered 500 as now (the client is gone). Tests: the service returns a wrapped
`context.Canceled`: the strict logger mock expects `Debug` and no `Error`.

### 4.4 One log per failed task read on execution

`TaskService.ExecuteTask`: a missing task is logged at Warn, and a failed read is not logged in the service
any more (it is returned wrapped, and each caller logs it: the handler, `ExecutePendingTasks`'s loop,
`runTaskNow`, the segment service). Tests: the strict logger mock of the service sees `Warn` for a missing
task and nothing for a read error; the handler's 500 case expects `Error` once.

### 4.5 Task handler tests

- `task_handler_refusals_test.go`, `TestTaskHandler_ResetAndTrigger_ServiceFailures`: the cases that
  `task_handler_reset_trigger_errors_test.go` covers go; what only it covers stays.
- `task_handler_execute_lookup_errors_test.go` and `task_handler_reset_trigger_errors_test.go`: each 500 case
  expects `Error` `.Times(1)` on a logger mock that refuses other calls to `Error`; "the task is gone when the
  execution reads it" asserts the body.
- `internal/domain/nullable_time_text_test.go`: `"10000-01-01"` and `"1900-01-01 00:00:00+00:09:21"` refused; a
  zoneless value has `Location() == time.UTC`.

### 4.6 F-14: the email webhook answers by the kind of failure

`internal/service/inbound_webhook_event_service.go`, `ProcessWebhook`:
- a workspace that does not exist: the repository's `*domain.ErrWorkspaceNotFound` passes through wrapped;
- no integration with that ID: `domain.ErrInboundIntegrationNotFound` (the reply route's sentinel);
- an integration of a kind without webhook parsing: `domain.ErrInboundProviderUnsupported`;
- a payload the provider's parser refuses: a new sentinel `domain.ErrWebhookPayloadInvalid`, wrapped with the
  parser's error.
`internal/http/inbound_webhook_event_handler.go`, `handleIncomingWebhook`: 404 for the first two, 400 for the
next two, 500 otherwise (database, storage, infrastructure), logged at Error only for the 500s and Warn for
the others. A parser step that is not about the payload (an HTTP call, as the SES subscription confirmation
may make) keeps its error unwrapped, so it answers 500.

Tests: service, one case per sentinel and a `StoreEvents` failure that is none of them; handler, one row per
status; integration (`tests/integration/webhook_*`): an unknown integration answers 404, a garbled body 400.

### 4.7 The compile request's text and the API reference

- `CompileTemplateRequest.Validate`: a `text` over `MaxCustomTextSize` bytes is refused, as a saved template's
  is. Test: 64 KiB accepted, one byte more refused.
- `openapi/components/schemas/template.yaml`: the `text` description says the 64 KiB limit counts bytes
  (`maxLength` counts characters); the compile request documents `preserve_liquid`. The generated
  `openapi.json` of this repository and of the docs repository are regenerated.

### 4.8 The small gaps left by #566

- `assertAuditReason` finalizes with the status the refusal actually answers (passed by each caller).
- `automations.nodeStats` handler table: a revoked API key (401) and a permission refusal wrapped by the
  service (403).
- The recipient feed's retries: with the retry delay from the feed settings set to a few milliseconds, a
  timeout and a refused connection are each retried, then fail after the configured attempts.

### 4.9 CHANGELOG (44.0)

The task API lines of 44.0 (unreleased, #569/#570) are extended rather than given Fix lines: `tasks.execute`
answers 400 for an ID that is not a UUID; 404 only for a task that does not exist (now strictly). F-14 is a
fix to released behaviour: "Fix: the email provider webhook answers 400 only for a payload it cannot read,
404 for an unknown workspace or integration, and 500 for a failure on our side, so providers retry it." The
compile request limit: "Improvement: `templates.compile` refuses a text version over 64 KiB, as saving does."

### Corrections after the plan review (lot 4)

The plan review (`/mnt/project-files/reviews/grouped-pr-lot4-plan-review.md`, Opus, xhigh) confirmed four
blocking and ten minor problems. These replace what the points above say where the two differ.

- **4.1** The handler tests that return a plain `errors.New("task not found")` and expect 404
  (`task_handler_test.go:578`, `:803`, `task_handler_refusals_test.go:71`) return the sentinel. The repository's
  existing zero-row tests gain `ErrorIs` rather than new cases.
- **4.2** The tests whose task IDs are not UUIDs get UUIDs (`task_handler_test.go:122, 196, 223, 255, 287, 1412,
  1716`, `task_handler_refusals_test.go:83`, `task_handler_execute_lookup_errors_test.go:24`, domain
  `task_test.go:725`). `tasks.get`, `tasks.delete`, `tasks.reset` and `tasks.trigger` refuse a non-UUID ID with a
  400 too, since they answer 500 with an Error log today and are reachable with an API key.
- **4.3** A cancellation is recognised from the request's context (`r.Context().Err() != nil`) as well as from
  the error, so a statement cancelled by lib/pq or a `sql.ErrTxDone` counts; the authorization reads take the
  same rule.
- **4.4** Warn for a missing task applies to every caller: the scheduler loop, `runTaskNow` and the segment
  service log `*domain.ErrNotFound` at Warn.
- **4.5** The 500 cases assert the logged error field holds the internal error's text; the Equal-only UTC check
  of `feed_nullable_gap_test.go:85-89` asserts `Location() == time.UTC` too.
- **4.6** `validateSNSURL` refusing a SubscribeURL is a payload error (400); a failed `client.Do`, and a non-2xx
  answer from AWS to the confirmation, are ours (500), so SNS sends the confirmation again. The existing tests
  whose outcome changes are updated (the handler's ProcessError case, the shared logger mock gains Warn, the
  "unsupported email provider kind" assertion). The run pattern covers every `/webhooks/email` end-to-end test.
  The Fix line says 400 for a payload it cannot read or an integration that is not an email provider.
- **4.7** `MaxCustomTextSize` moves to `pkg/notifuse_mjml` (internal/domain keeps an alias). The check runs on
  the normalised text, before `Validate`'s early return for `mjml_source`, with a test in each mode. It answers
  400 as saving does (the source is the caller's input, unlike a rendering problem, which stays a `text_error`);
  the compile request's `text` description names the limit; `./pkg/notifuse_mjml/` joins the test command.
- **4.8** The feed's retry delay and per-attempt timeout are fixed in `internal/domain/data_feed.go:175-188`:
  the fetcher gets an injection point of its own (fields set by its constructor, overridden in tests), not a
  setting of `RecipientFeedSettings`, which is the broadcast's stored JSON. The cancelled-feed bound of
  `data_feed_failures_test.go:126` uses it as well.
- **4.9** The compile limit is folded into the 44.0 plain-text Feature line (`CHANGELOG.md:77`): the request's
  `text` field is 44.0 work.

## Test commands for lot 4

```
go test ./internal/domain/ ./internal/service/... ./internal/repository/ ./internal/http/
INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestTask|TestWebhook|TestInbound|TestTemplate' -count=1
```

## Lot 5: the public blog (robots, sitemap, feeds, cache) and the blog API

Reviewed before code (to be filled in after the review). No schema change.

### 5.1 robots.txt and the sitemap answer as the feeds do

`internal/http/root_handler.go`:
- `serveBlogRobots` and `serveBlogSitemap` answer 405 outside GET and HEAD, as `serveBlogFeed` does.
- robots.txt names the sitemap by its absolute URL, `Sitemap: https://<host>/sitemap.xml`, the host being
  `hostWithoutPort(r.Host)` in lower case (the custom domain the workspace was found by).
- The sitemap's URLs take that lower-case host too.

Tests, `root_handler_test.go`: POST on both answers 405; robots.txt on `Blog.Example.com:8080` names
`https://blog.example.com/sitemap.xml`; the sitemap's `loc` values are lower case.

### 5.2 Sitemap content

- `lastmod` of a post is its last change, `GREATEST(p.published_at, p.updated_at)` read by
  `ListSitemapPosts` (`domain.BlogSitemapPost` gains `LastModified`).
- Each category with a listed post has its page `/<category>` listed next to its feed; the URL budget counts
  both (`cost` 3 for a post opening a new category).
- The sitemap is written straight to the response, and kept in the handler's cache for `BlogCacheTTL` under a
  key per workspace and host, as the pages are.
- More than 50 000 URLs still drops the oldest posts: an index of sitemaps is left out (known, not handled).

Tests: `writeBlogSitemap` table (a category page listed once, the budget with category pages, the
`lastmod` of an edited post); repository sqlmock of the query; handler: a second request within the TTL does
not call the service (`X-Cache: HIT`); `tests/integration/blog_sitemap_test.go`: every listed URL answers 200
(GET in a loop), and an edited post's `lastmod` is its edit date.

### 5.3 Category slugs taken by another route are refused

`internal/domain/blog.go`: `ReservedBlogCategorySlugs` lists the first path segments the server routes before
the blog: `api`, `console`, `notification-center`, `v`, `t`, `r`, `visit`, `opens`, `preferences`,
`subscribe`, `unsubscribe`, `unsubscribe-oneclick`, `health`, `healthz`, `track`, `webhooks`; and a slug
starting with `api`, `console` or `notification-center` (the root handler matches those by prefix,
`root_handler.go:105-117`). Category create and update refuse them ("slug is reserved by another route").
Existing categories are not touched. Tests: one refused slug per kind (exact, prefix), a neighbour accepted
(`apis` refused, `vip` accepted, `tracking` accepted).

### 5.4 Feeds: If-None-Match decides alone when present

`serveBlogFeed`: If-Modified-Since is read only when the request has no If-None-Match (RFC 9110 §13.1.3).
Test: a stale ETag with a recent If-Modified-Since answers 200.

### 5.5 A scheduled post shows on time on the cached pages

The home and category page cache entries live until the next scheduled publication when it comes before
`BlogCacheTTL`: a repository method `NextScheduledPublishAt(ctx, now)` (the smallest `published_at > now`
of a live post, or none), read on a cache miss; the TTL is `min(BlogCacheTTL, next - now)` and at least one
second. Tests: sqlmock of the query; handler: a post due in 30 s gives the entry a TTL of 30 s (the cache
fake records it); integration: a post scheduled 2 s ahead shows on the home page after it is due.

### 5.6 Blog and template API input

- `ListBlogPostsRequest.Validate`: a `search` with a NUL byte or invalid UTF-8 answers 400; a `category_id`
  that is not a UUID answers 400 (the column is a UUID, `init.go:398`).
- `GetTemplateUsageRequest.FromURLParams`: a `template_id` with a NUL byte or invalid UTF-8 answers 400.
Tests: domain table per case; handler 400 for `blogPosts.list?category_id=news`.

### 5.7 Dead code and comments

`BlogService.ListPublicPosts` (no caller) goes, with its interface method, mock and tests; the comments of
`ListPublicSitemapPosts` (service and interface) say "the live posts the sitemap can list"; the 150-character
comment of `writeBlogSitemap` is wrapped. `internal/repository/blog_postgres_test.go`: a sub-test
`PublishedBefore` + `Search` that pins the SQL placeholders' numbering.

### 5.8 CHANGELOG (44.0)

The sitemap, robots.txt and feeds are 44.0 work (unreleased): their lines take the changes (absolute sitemap
URL, category pages, last change date, 405, the conditional GET rule). The reserved slugs, the scheduled post
on time, and the 400s are released behaviour: "Fix: a scheduled blog post appears on the home and category
pages when it is due, not up to 5 minutes later.", "Improvement: a blog category cannot take a name another
route already answers (`api`, `track`, `unsubscribe`, …).", "Fix: `blogPosts.list` answers 400 for an
unreadable search or category ID, and `templates.usage` for an unreadable template ID, instead of 500."

## Test commands for lot 5

```
go test ./internal/domain/ ./internal/service/ ./internal/repository/ ./internal/http/
INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestBlog' -count=1
```

## Lot 6: templates, the plain-text version, the visual editor's MJML, system emails

Reviewed before code (to be filled in after the review). No schema change.

### 6.1 The automatic plain-text version (`pkg/plaintext`)

One table row in `plaintext_test.go` per case, written first, then the local fix:
- `]` leaves `trailingPunctuation`: Markdown-style autolinkers take it into the link, so `[Unsubscribe
  (https://e.com/u)]` must read `… (https://e.com/u) ]`; the comment of `trailingPunctuation` and
  `plans/feat-plain-text-alternative-plan.md:419` say which autolinkers leave what out.
- In `<pre>`, a link's URL is kept apart from the text around it by a space (`«https://…»` becomes
  `« https://… »`).
- A Unicode space other than NBSP at the edge of a text node (`&#x202F;`, `&#x2003;`) separates the words as an
  ASCII space does: the edge test reads the first and last rune with `unicode.IsSpace`.
- An `<li>` whose first child is a `<pre>` or a nested list writes its marker before that content.
- `start="3px"` numbers from 3, as browsers read the leading integer.
- `mailto:?subject=…` (no address) writes its label alone.
- Nested anchors whose outer label starts with a URL: the row is written; if the parser already splits the
  anchors and both destinations are kept, the row stays as a pin and nothing else changes.
- A `mj-social-element` with click tracking: its icon link and its label link name one destination; the pair is
  written once as `Label (url)`. Detected on the HTML that gomjml produces for a social element (an icon cell
  and a label cell that share an href inside one element's table), so other layouts keep every destination.

### 6.2 Custom text and the compile preview

- `plainTextVersion`: a custom text that renders blank sets `text_error` ("the custom text renders blank for
  this data") only when the request asks for the automatic text (`IncludeText`, the `templates.compile`
  preview); send paths are unchanged.
- The starting point of a custom text (`PreserveLiquid`): a loop around table rows, or an `mj-raw` inside a
  column, sets the same notice the split tag pair sets, so the preview warns that the Liquid may be misplaced.
  Rows added to `TestCompileTemplate_CustomTextSeed`.
- `CompileTemplateRequest.Validate` calls `ValidateDisplayConditions(req.VisualEditorTree)` for a visual tree,
  so an invalid condition answers its own error rather than "'if' tag was never closed".
- The code-mode path (`MjmlSource`) removes the top-level `__dc*` keys from the data before
  `ProcessLiquidTemplate`, with `withoutConditionNames`, as the visual path does.
- Exported conditions and the 100 KB limit: a test with 50 conditional sections of 20 rules measures the
  exported size; the generated Liquid is shortened where the test shows most of it is (variable names, the
  numeric guard), and the "Conditional blocks" doc page names the limit (docs PR).

### 6.3 Templates API

- `TemplateService.UpdateTemplate`: a stored translation carried over (request without `translations`) is not
  validated again; the request's own translations still are. Test: a stored translation that no longer
  validates, an update without `translations` succeeds; one with that translation answers 400 naming it.
- `templates.usage` counts what a broadcast will send: `winning_template` once a winner is set, the first
  variation when `test_settings.enabled` is false, every variation otherwise. Tests: sqlmock of the SQL and an
  integration case per kind.
- `broadcasts.get` with `with_templates`: the template loop moves into `BroadcastService.GetBroadcast`,
  guarded by `HasPermission(templates, read)` like `ListBroadcasts`; a caller without it gets the variations
  without templates and no Warn per variation. Tests: service with and without the permission.

### 6.4 System emails send the text part first

`pkg/mailer/mailer.go` (the three senders): the text is the main body and the HTML the alternative, so
`multipart/alternative` goes from the plainest to the richest (RFC 2046 §5.1.4). Tests: the mailer tests read
the parts' order.

### 6.5 MJML converter

- `pkg/notifuse_mjml/model.go` formatted with gofmt, in its own commit.
- The comments of `escapeAttributeValue` and of Step 3 (`template_compilation.go:103-113`) describe what the
  gomjml version in `go.mod` writes by hand (only the section's `bgcolor`), name `mj-font`'s `href` among the
  exceptions, and are wrapped. The doubling itself and the `background-url` quote wait for the next gomjml
  version (known, not handled).
- Coverage of the skip paths (`converter_display_condition_test.go`): an evaluation error from a failing
  engine, and a typed-nil child next to a conditional one.
- `liquid_validate_test.go`: an `include` on a later line of a multi-line `{% liquid %}` is refused; the error
  branches of the custom text and of the validation environment get a case each.

### 6.6 Sender and headers tests

- `email_service_test.go:2831`: the translation's sender is not the integration's default, so a fall back to
  the default fails the From assertion.
- The translation's own sender wins on the broadcast queue sender, the automation email node and the template
  test send: one sub-test each.
- An integration test sends a message through the SMTP bridge with a `Reply-To` to a template that has its own
  reply-to, and reads which one Mailpit received.
- The List-Unsubscribe header tests get a row with tracking off (`tracking_mode = no_tracking`).
- The block that reads `oneclick_unsubscribe_url` when `SendsListUnsubscribeHeader()` is true becomes
  `(*Broadcast).ListUnsubscribeURL(data)`, tested once, called from the three places.
- `orchestrator_sender_test.go`: the pause reason's `VariationNumber` and `VariationCount` are asserted; the
  sending-started annotation is asserted for `processing` and not for `testing`.

### 6.7 The list test email

`plans/feat-plain-text-alternative-plan.md` says 44.0 (lines 23, 242, 244, 359); an integration sub-test in
`plain_text_alternative_e2e_test.go` checks that the text part of a list's test email holds the list's links.

### 6.8 CHANGELOG (44.0)

The plain-text version, the custom text, display conditions and the code export are 44.0 work: their lines take
the changes. Released behaviour: "Fix: system emails (invitations, sign-in codes) put their text part first, as
mail clients expect.", "Fix: updating a template without its translations no longer fails on a stored
translation.", "Fix: `templates.usage` counts only the templates a broadcast will send.", and
"Improvement: `broadcasts.get` with `with_templates` leaves the templates out for a caller without Templates
read, without a lookup per variation."

## Test commands for lot 6

```
go test ./pkg/... ./internal/domain/ ./internal/service/... ./internal/repository/ ./internal/http/
INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestTemplate|TestPlainText|TestSMTPBridge|TestBroadcast' -count=1
```

## Lot 7: analytics, contacts and visits, the timeline, the demo, the audit log, the test helpers

Reviewed before code (to be filled in after the review). No schema change.

### 7.1 Analytics

- `count_transactional_messages` (`internal/domain/analytics.go:160`) counts what the `origin` dimension calls
  transactional: `transactional_notification_id IS NOT NULL AND automation_id IS NULL AND broadcast_id IS NULL`.
  Released measure, so a Fix line. Tests: the schema's filter; integration: an automation email and a template
  test are not counted.
- `pkg/analytics/sql_builder.go`, ORDER BY: a measure is ordered by its output alias, not by its raw SQL
  (`*` for a count, `sent_at` for a max, neither valid outside the aggregate). Tests: order by `count_sent`
  and by `last_sent_at` build valid SQL (sqlmock, and an integration query).
- `fillTimeSeriesGaps`: rows are keyed by time and dimension values, so a query with a dimension (such as
  `origin`) keeps every row of a period; each period gets a row per dimension value seen in the data. A filled
  row has 0 for count and sum measures and null for the others (`max`, `min`, `avg`), read from the measure
  types `ProcessRows` now receives. Tests: two origins on one day both kept; a missing day gets a row per
  origin; `last_sent_at` of a filled row is nil.
- `last_sent_at` of a broadcast or an automation is the time it was queued: the measure's description says so.

### 7.2 Contacts and web visits

- `contacts.list` search also matches `first_name || ' ' || last_name`, so "Jane Doe" finds a contact with
  only those two fields. Test: sqlmock of the condition; integration: the contact is found.
- `webAnalytics.contactSessionPages`: the response gains `pages_truncated` and `goals_truncated`, true when the
  repository returned `ContactSessionPagesMax` rows of that kind (it reads one more row to tell). Tests:
  repository with 201 rows; handler; openapi.
- `ContactVisitsRequest` and the session pages request: `Validate` refuses an email over 254 bytes, a NUL
  byte or invalid UTF-8 (moved from `FromQuery`, which keeps calling it). Tests: domain table.

### 7.3 Timeline

- `timeline.list` answers `[]` when the contact has no entry: the repository starts from an empty slice.
  Test: repository with no row; handler body `"entries":[]`.
- A test walks `domain.TimelineEntityTypes` and requires each type to be in `timelineEntityPermissions` or in
  a new explicit `timelinePublicEntityTypes` list, so a type added later fails until it is classified.

### 7.4 Demo

- The A/B broadcast of the demo records `TestPhaseRecipientCount` (its `SamplePercentage` of the recipients
  generated) and `WinnerPhaseRecipientCount` (the rest). Test: demo service, the two counts add up to the
  recipients and the test count is the sample.
- A test generates every demo transactional message type and requires a notification ID for each
  (`demoNotificationIDs`).

### 7.5 Audit log of blog themes

`blogThemes.create` names its target in the handler (`record.SetTarget` with the new version) instead of
reading it from a response the middleware cut at 64 KiB. Tests: handler sets the target; middleware with a
response over 64 KiB keeps a target set by the handler.

### 7.6 Coverage gaps

- `internal/app`: a test fails when `WithContactVisits` is not wired (the app built for tests answers the
  route).
- Integration: the web analytics half of the timeline permissions fix, and API key access to `timeline.list`.
- `templates.usage`: the guards (empty IDs filtered, the variations' CASE, `LIMIT 50` with `COUNT(*) OVER ()`)
  get integration cases with data that each would get wrong.
- A test compares instants across time zones for the timeline's `created_at` (a value written in UTC read under
  another session time zone).

### 7.7 Test helpers

- The three copies of the per-recipient Mailpit wait (`list_unsubscribe_headers_test.go:466`,
  `broadcast_liquid_template_test.go:661`, `broadcast_data_feed_email_test.go:965`) use
  `testutil.WaitForMailpitMessageByRecipient`.
- `transactional_handler_test.go:1294, 1411, 1890, 2012`: a failed `GetMailpitMessage` stops the scan and polls
  again, as the helper does, instead of skipping the message.
- `GetMailpitMessagesByRecipient` used by `sendToList`: waits for the count from the list alone, then reads
  the headers once.
- `TestWebhookWorkerEndToEnd/a_slow_batch_renews…`: the lease and the slow batch are set short in the test (the
  worker's lease is a constructor option already, or becomes one), so the case waits seconds, not 30 s.
- `TestWorkspaceLock_GrantedAsTheContextEnds`: run alone and under load with `-count=20`; the race it shows is
  fixed, else it is listed as not reproduced.

### 7.8 Small fixes

- `internal/service/list_service.go:493`: the comment names the double opt-in email.
- `openapi/paths/webhook-subscriptions.yaml:381`: "An empty value lists every status; any other value answers
  400", `failed` also covers deliveries waiting for a retry; the 400 says "missing parameters or unknown
  status"; `openapi.json` regenerated here and in the docs repository.

### 7.9 CHANGELOG (44.0)

Released behaviour: "Fix: the `count_transactional_messages` measure no longer counts automation emails and
other messages.", "Fix: analytics queries can be ordered by a measure.", "Fix: analytics time series with a
dimension keep every value of each period.", "Fix: the contact search finds a contact by first and last name
together.", "Fix: `timeline.list` answers an empty list instead of null for a contact without entries.",
"Fix: the audit log names the theme a `blogThemes.create` saved." The session pages flags and the visits email
limit extend the 44.0 lines of those endpoints, which are unreleased.

## Test commands for lot 7

```
go test ./pkg/... ./internal/...
INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestAnalytics|TestContact|TestTimeline|TestDemo|TestWebAnalytics|TestTemplateUsage|TestWebhookWorker|TestWorkspaceLock|TestTransactional|TestListUnsubscribe|TestBroadcast' -count=1
```

## Lot 8: console

Reviewed before code (to be filled in after the review). Every user-facing string goes through Lingui and is
translated in the eight catalogues (`console/CLAUDE.md`). Each point gets a vitest test next to its component
(created when the component has none), written first. Paths are under `console/src/`.

### 8.1 Automations

- `automations/hooks/useAutomationCanvas.ts:584`, `deleteEdge`: the `branch` case clears the path's
  `next_node_id` (paths and default path). Test: delete each kind of branch edge.
- `AutomationEditor.tsx`: Activate and Pause stay disabled until the automation's refetch after the action has
  settled. Test: the button is disabled while the refetch is pending.
- When `readOnly` turns on while changes are unsaved, the live banner says the unsaved changes cannot be saved
  until the automation is paused. Test.
- One `FullDrawer` whose content goes from the skeleton to the editor (`AutomationEditor.tsx:70`, `:542`), so
  the first load slides once; a reopen with cached data shows the editor without the skeleton. Test: one
  drawer element through the load.
- `stats/NodeStatsFooter.tsx:50`: the root's label reads "passes" ("# passes") rather than "entered", since a
  loop back to the trigger counts each time. Test.
- `nodes/TriggerNode.tsx`: the broadcast and template filters are shown, by name when the lists are loaded,
  else by ID. Test.
- `NodeConfigPanel.tsx:40`, `TriggerFormConfig` gains `link_url`, `broadcast_id`, `template_id`.
- `context/AutomationContext.tsx`: the callbacks of the context value are `useCallback`s, so the memo holds;
  a test asserts the value keeps its identity across a re-render and changes when `canReadTemplates` loads.
  `markAsChanged` (empty) and its five calls in `useAutomationCanvas.ts`, and the unreachable live-save
  warning, go.
- Small editor details: ⌘ on Mac in the shortcut tooltips; `EmailStepPreview` tells a missing template (404)
  from another read error; "View messages" is a router `Link`; the open rate shows a skeleton while loading;
  the stats context value and payload key are memoised; the Tailwind margins on antd `Input`/`Form.Item` in
  `EditorSettings.tsx` move to the components' own props or a wrapper; under 1200 px the "+" buttons stay
  beside the step panel (the panel reserves its width).

### 8.2 Email builder and saved blocks

- `ui/SavedBlockCard.tsx:174`: Escape with `e.keyCode === 229` (Safari's end of composition) does not cancel.
- `EmailBuilder.tsx:870-893`, `changeLibrary` returns false when another change is running, and
  `ui/SaveBlockModal.tsx` disables Save (`loading`) while the library is busy.
- `ui/SaveBlockModal.tsx`: `headKeyOf(emailTree)` is computed only while open.
- `utils/suggestBlockName.ts`: entities are decoded with `DOMParser`; the space is removed only before `,` and
  `.`.
- `ui/SavedBlockCard.tsx`, `panels/LibraryPanel.tsx`: dnd-kit's `attributes` are spread on the handle; the
  announcements are made from `handleEnd` with the real outcome (inserted, refused with its reason, nothing).
- A renamed card that no longer matches the search stays visible, highlighted, until the filter changes.
- Focus returns to the card's "More actions" button after rename or duplicate, and to the next card (or the
  list) after delete.
- `ui/AddBlockPopover.tsx`: saved block rows are `button`s (disabled with their reason as text, no redundant
  `title`); the last rows are kept while the popover closes.
- `TemplateSelectorInput.tsx:208, 242, 281`: the three drawers get `canManageSavedBlocks` from
  `permissions?.templates?.write`.
- `CreateTemplateDrawer.tsx:637`: Replace has its own refresh failure message.
- `CreateTemplateDrawer.tsx:659-680`, rename: the workspace is read again before the update, so a block
  replaced elsewhere keeps its new content.
- Column widths: `utils/columnWidths.ts`, `rebalanceAfterRemove` leaves the widths alone when removing a group
  leaves no room (as on add); a group in px counts by its px over the body width minus the section padding;
  changing a group's width warns when the section goes over 100 %; the AI assistant's add, remove and move go
  through the same rebalance helper, extracted from `EmailBuilder.tsx`; on the canvas, the columns of a group
  take their widths (`MjColumnBlock.tsx:315-322`). The canvas limits of `sectionChildWidth.ts` (a row over 100 %
  squeezed, Liquid-made columns, `mj-class` widths) stay (known, not handled).
- `MjmlCodeEditor.tsx`: an empty source sets the text output to '' without compiling.
- `textVersionSeed.ts`: Liquid counted outside attributes, `mj-preview` and comments; the "Liquid lost" notice
  names display conditions. `TextVersionPanel.tsx`: an email with no text shows "The email holds no text yet".
- `EmailAIAssistant.tsx`: its messages, title and placeholder go through `t`; `AIAssistantChat.tsx`: the
  markdown headings take the theme's sizes.
- The old violet `#7763F1` becomes the `primary-500` tokens in the files the TODO lists, with their tests; and
  `theme/noHardcodedColors.test.ts` walks every `.ts`, `.tsx` and `.css` of `src/` with the listed exceptions,
  so it would have caught it.
- Import and export of MJML/JSON in a translation's visual editor (#448, approved 14/11):
  `TemplateTranslationsTab.tsx:397` passes `toolbarActions` with an `ImportExportButton` bound to the open
  language. Tests: import replaces only that language's tree, the default language is untouched, export gives
  the translation's tree.
- `ui/SavedBlockCard.test.tsx:223-225`: the drag test asserts no drag starts; `LibraryPanel.test.tsx`: the
  scroll to the highlighted card with a filter on. `columnWidths.test.ts:78`: the comment is corrected.

### 8.3 Templates, transactional, broadcasts

- `templates/TemplatePreviewDrawer.tsx`: opened from the message history for a broadcast email, it shows the
  variation's `sender_id` sender first.
- `messages/MessageHistoryTable.tsx:53-81`: the template preview query waits for `canReadTemplates`; without it
  the template's ID is shown.
- `blog/PostDrawer.tsx`: editing is disabled with its reason when Templates read or write is missing.
- `pages/BlogPage.tsx:29`: `canWritePosts` reads `permissions?.blog?.write`.
- `services/api/permissions.ts`: `broadcasts.sendToIndividual` is listed under Templates read too.
- `lib/transactionalApiSnippets.ts`: `email_options.sender_id` as a commented option, fixture updated.
- `broadcasts/useBroadcastActions.ts`: a resume refused for a missing sender shows a translated sentence; other
  errors the translated generic text.
- `transactional/TransactionalDetail.tsx`, `TransactionalTable.tsx`, `notificationParts.tsx`: the reasons of the
  disabled Send test and Edit, of "(default)" and of "Deleted template" reach the keyboard (`aria-disabled`
  buttons with `aria-describedby`, `sr-only` text); a rate link with no value is named "…: no data, view in the
  message log"; disabled menu entries are `aria-disabled` and stay reachable.
- `components/templates/TemplateSelector.tsx` and its export go (no import).

### 8.4 Logs, messages, contacts

- `logs/LogsSearchBox.tsx`: `picked` resets when the URL clears the search; the hint sits in an `aria-live`
  container that is always there.
- `logs/MessageDetail.tsx:496-531`: a double opt-in message without a marketing provider shows no From; `SourceLink`
  builds its `Link` itself.
- `logs/AuditLogTab.tsx:142`: Refresh resets `['audit-logs', scope, workspaceId]` only. Tests: the key with the
  user ID (a second `getMembers` when the user changes); Refresh moves `from` later.
- `logs/period.ts`: `CountedPeriod` becomes `RollingPeriod`.
- `messages/MessageColumnsSelector.tsx:92`: the tooltip no longer describes the button.
- `pages/ContactsPage.tsx`: the search write settles before the push to the contact (await the first
  `navigate`), tested on a memory history; the loaded page count survives closing the sheet (kept outside the
  history entry); the optimistic delete applies to every `['contacts', workspaceId]` key
  (`setQueriesData`); the "In any of" chip is hidden when `segment_id` is set.
- `contacts/sheet/ContactDetailsBody.tsx:126`: `cancelQueries` before `setQueryData`.
- `settings/WorkspaceMembers.tsx:345`: leading and trailing `_` and `-` are removed from the API key name when
  sent.
- `file_manager/storageObjects.ts`: its two errors are translated; empty path segments are ignored; deleting a
  prefix with no object says there is nothing to delete. `FileExplorer.test.tsx`: a second folder delete
  after the first ends, and cancel refused while deleting.
- `settings/Integrations.tsx:1202`: "Type: …" is translated, with a readable type name.

### 8.5 Dashboard, analytics and web analytics

- `analytics/DashboardKpis.tsx:240`: the contacts tone is neutral while the new count loads; one test per
  placeholder guard (`:162`, `:169`); a test pins each block's query key and `refetchInterval`.
- `web_analytics/lib/dates.ts:92, 111`: the start of a day is `dayjs.tz(day, zone)` without `startOf('day')`;
  the test runs with `process.env.TZ` set to another zone. `MessageHistoryTable.test.tsx` and
  `formatters.test.tsx` set `TZ` away from Tokyo for their Tokyo cases.
- `web_analytics/lib/format.ts`: `-0` is not shown (`rounded || 0`); the compact currency without a compact part
  drops the cents.
- `web_analytics/filters/BackfillStatus.tsx:94`: the progress percent is formatted by `formatPercent`;
  `LiveButton.tsx:47`, `LiveSessionMap.tsx:77`, `explore/MinSessionsInput.tsx:57` format their counts;
  `FilterBuilder.tsx:316` gets `max={100}` for rates and a `precision`.
- `MessageHistoryTable.tsx` and `utils/formatters.tsx`: an unknown time zone falls back to UTC.
- One render test, in French, for each web analytics component the TODO lists (`Delta.tsx`,
  `explore/ExploreTable.tsx`, `explore/BreakdownDrawer.tsx`, `lib/csv.ts`, `MetricSummary.tsx`,
  `goals/GoalCard.tsx`, `goals/GoalDashboardDrawer.tsx`, `DimensionTableWidget.tsx`, `tabs/DashboardTab.tsx`,
  `TrafficHeatmapWidget.tsx`).
- `analytics/useOriginSeries.ts`: once lot 7's gap filling keeps every origin, one query with the `origin`
  dimension replaces the four; the log axis threshold reads the series without "other"'s small values.
- The chart font sizes of `OriginVolumeChart.tsx` and `SendsPerDayChart.tsx` come from `theme/tokens.ts`.

### 8.6 Layout, theme, blog

- `layouts/app-shell/gravatar.ts:7`: `d=404`, and the avatar shows the initials on error (Pierre, 14/11). Test:
  the URL, and the initials after an image error.
- `WorkspaceSwitcher.tsx`: `aria-haspopup`, `aria-expanded`, a fallback name, a max width with truncation; the
  three sidebar menus give the focus back to their button on Escape.
- `AppSidebar.tsx:137`: the logo goes to `firstAccessiblePage(...)` with a matching label.
- `components/layout/SidePanel.tsx`: the tab content is rendered in antd's `items[].children`; the own tab
  resets on close. `useDrawerNavKeys.ts:36`: only click-opened popovers, or one holding the focus, block J/K.
  `KpiStrip.tsx`: the trend's room is kept while loading. `Mono.tsx`: copies the rendered `textContent`.
  `RailLayout.tsx:140, 163`: `has-[:focus-visible]` instead of `focus-within`. `MetaLine.test.tsx` added.
- `theme/antdTheme.ts`: `Table.rowHoverBg` takes a tint distinct from `Table.headerBg` (Pierre, 14/11), and
  `antdTheme.test.ts` computes the contrast ratio between `Tag.defaultBg` and the hover tint; `colorPrimaryHover`
  one step darker; `Menu.activeBarBorderWidth: 0` moves to the sidebar's own `ConfigProvider`; a sorted fixed
  column has an opaque background (`td.ant-table-cell-fix.ant-table-column-sort`); links marked
  `data-plain-link` inherit their color (`antd-overrides.css`), used by `WebAnalyticsLivePage.tsx:125-131`.
- `blog/ThemeEditorDrawer.tsx`: the editor fills the drawer with a flex column (`min-h-0 flex-1`).
- `utils/mockBlogData.ts`: the Unsplash photos of people become neutral images shipped with the console (an
  SVG landscape, a generic avatar).
- `ThemePresetPreviewDrawer.tsx` and `ThemeSelectionModal` go (unreachable).
- `__tests__/WorkspaceLayout.test.tsx:251`: `useSidebarCollapsed` is tested with `renderHook` and a throwing
  `setItem`. `KeyValueList.test.tsx:22` and `WorkspaceLayout.tsx:161` wording.
- `contexts/AuthContext.tsx`: a test that `useWorkspacePermissions` leaves `loading` after a reset of
  `['workspace-members']`.

### 8.7 Translations and test setup

- `i18n/locales/fr.po`: the apostrophes lost in about 280 translations are restored (a script inserts `'` where
  a French elision lost it: `d `, `l `, `qu `, `n `, `s `, `j `, `c ` before a vowel or `h`, then each change is
  read in the diff); `e2e/features/library-management.spec.ts:216` follows.
- "Root" translated in the seven languages; German "Pp." becomes "PP"; Catalan sidebar verbs harmonised.
- `__tests__/setup.tsx`: `t` is created once at module level, and a descriptor is resolved with its `id`,
  `context` and `values`.
- `e2e/fixtures/auth.ts:560-566, 581-587`: the storage reset runs in the top frame only, and the `mj-raw`
  exclusion of `add-block-popover.spec.ts:90-93` goes; `e2e/fixtures/mock-data.ts:818` answers
  `analytics.query` by its measures and dimensions.
- `PostDrawer` and `ContactsCsvUploadDrawer` get test files with their resume prompt dates in French;
  `ExportContactsModal.test.tsx` and the import drawer check the French thousands separator.

### 8.8 CHANGELOG (44.0)

Most points touch 44.0 console work (the redesign is unreleased): they extend its lines or need none. Lines
added: "Improvement: a translation's visual editor can import and export MJML and JSON." Released behaviour
fixed: "Fix: members with Blog write but not Workspace write can write blog posts in the console, and the
others no longer see buttons that fail." "Fix: the automation editor clears a branch path when its link is
deleted."

## Test commands for lot 8

```
cd console && npm test && npx tsc --noEmit && npm run lint
cd console && npm run lingui:extract   # then the eight catalogues are complete
```

## Lot 9: brand, CHANGELOG and plans; the other repositories

Reviewed before code (to be filled in after the review).

### 9.1 notifuse: brand files

- The six `brand/*.svg` get `width`/`height` equal to their viewBox, keep `<title>` with `role="img"` and lose
  the duplicate `aria-label`.
- A `brand/README.md` of a few lines: "square" is the N icon, not the `logo-square.png` of docs and website;
  which file to use where.
- A variant of the icon with a margin for round crops, `brand/notifuse-square-color-padded.svg`
  (viewBox `-40 -40 560 560`).
- A console test compares the path `d` values of `components/brand/NotifuseLogo.tsx` with those of `brand/`, and
  a test in the same file checks that the black and white variants have the colour variant's geometry.
- The N and the word are centred in their frames (Pierre, 14/11), in the six files and in the console's
  `NotifuseLogo` and `NotifuseMark`: the SVGs become the reference, the PNGs keep the old offset.

### 9.2 notifuse: CHANGELOG and plans

- The "Behind a reverse proxy…" line of 44.0 adds that links to the instance's host name on another port, and
  behind a proxy that forwards nothing the blog's links, are no longer recorded one by one.
- "keep the sender and reply-to they were queued with" (`CHANGELOG.md:127`).
- The two task Improvement lines of 44.0 become one.
- The Columns button: its accessible name gets a Fix line (it existed in 43.0).
- The 44.0 section is sorted by type (Feature, Improvement, Fix, Security), as the last step of the PR, after
  every lot's lines are in.
- Plans: `test-coverage-critical-paths-plan.md` (the OIDC issuer check is reachable),
  `fix-task-reset-trigger-errors-plan.md` (line width, the repeated sentence), `feat-send-time-sender-plan.md:135,
  138` (file names), `fix-translation-sender-reply-to-plan.md` (wrap at 100), and the eight plans that cite
  `/mnt/project-files/…`: the path goes, a one-line summary of the review stays.
- `CLAUDE.md` gains the architecture instruction, copied from `cadrer-le-document-d-architecture-de-reference.md`
  as written there.

### 9.3 docs (one PR to dev)

The twelve points of the TODO's docs section, as each describes them, plus the doc halves of lots 3 to 7
(`nodeStats` "Reached" of a past period; the links that trigger nothing; the conditional block's text in a
custom text; `{% render %}` for blog themes; sender wording; the sitemap's content; the file manager's 10 000
objects; "Use for marketing"; the image order on provider pages; the unused `smtp_form.png` and `favicon.svg`;
the regenerated `openapi.json`; the Conditional blocks size limit; the webhook delivery `status` filter).

### 9.4 cloud (one PR to its default branch)

The twenty points of the TODO's cloud section, in `billing-api`, `billing-app` and `billing-admin`, each with
the test its entry names; `make test` and `make lint` in each component. The "Owner" line: the `getTenant`
query takes `listTenants`'s LATERAL join and the e2e spec pins it (the request shape and the answer, both sides).

### 9.5 architecture (one PR to main)

The eight points: `IsAncestor` tells exit code 1 from other git errors; files read with `git show sha:path`
(export-ignore and symlinks); `flows/broadcast.md` and `findings/pilot.md` brought to the current dev, which
this PR changes again (F-04, F-08, F-15, F-26, F-27, F-29, F-31, F-33, F-03, F-14 set `fixed` once the notifuse
PR is merged, with its merge commit); the unverified facts of F-07, F-11, F-14, F-19, F-25, F-28 checked where
the code or provider documentation shows them; package-level field resolution; the nightly report cut at a
line with a count; malformed IDs and `(Retired)` reported; the refcheck rule for D-01's scope. Commits as
Pierre, checks of `CLAUDE.md` step 3 before pushing.

### 9.6 website and liquidgo

- website: `public/favicon.svg` becomes `brand/notifuse-square-color.svg`.
- liquidgo: `StrainerTemplate.Invoke` checks each argument's type against the parameter before
  `reflect.Call` and returns an `ArgumentError`; keyword arguments are evaluated in sorted key order; the
  skipped keyword argument test runs and asserts the HTML; the stale TODOs and commented cases go.
