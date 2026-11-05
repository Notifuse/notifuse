# Provider throttling slows sending down instead of pausing broadcasts — plan

Branch: `fix/provider-throttling` (from `dev`, PR to `dev`). Docs: `Notifuse/docs`, branch `docs/provider-throttling`, PR to `dev`.
No schema change, no migration, no new environment variable.

Reviews of this plan: Gemini (`/mnt/project-files/reviews/fix-provider-throttling-gemini-brut-plan.md`) and an independent xhigh review; outcomes in "Plan review outcomes" at the end, verdicts in `/mnt/project-files/reviews/fix-provider-throttling-gemini-verif-plan.md`.

## The problem

Since 42.0 the email queue really sends at the integration's **Rate Limit for Marketing**. When that limit is above what the provider accepts (net of transactional traffic and of other integrations on the same account), the provider answers "too fast" (throttling), and today:

1. **Each throttled email spends one of the recipient's attempts** (`handleError`, `internal/service/queue/worker.go:853`): the row goes `failed` with a retry 1, then 2 minutes later, and is given up at its third refusal (the row's `max_attempts`, 3). Throttling is not the recipient's fault and says nothing about the email.
2. **Each throttled email counts as a provider failure** for the circuit breaker (`RecordFailure`, `circuit_breaker.go:145`; the classifiers mark 429 / `Throttling` as `ErrorTypeProvider`, and SES v2's `TooManyRequestsException` as `ErrorTypeUnknown`, which counts too). Five in a row open the circuit.
3. **The open circuit pauses every broadcast on the integration** (`PauseForCircuitBreaker`, `broadcast_service.go:935`, wired in `app.go:1362`) and emails the workspace owners. A paused broadcast must be resumed by hand. Automation emails wait for the 1-minute cooldown.
4. **After the cooldown, one probe goes out, and if it passes the integration sends at the full configured rate again** (`GetOrCreateLimiter` resets the limiter to the configured rate every round, `rate_limiter.go:33-38`), so it gets throttled again.

## Goal

When the provider says "too fast", Notifuse sends slower on that integration, climbs back towards the configured limit once the provider accepts again, sends the throttled email again without spending its attempt, and does not count the throttle towards the circuit breaker, so broadcasts keep sending.

The circuit breaker keeps handling real outages, and throttling that slowing down cannot fix: a quota the error names, and refusals that continue at a tenth of the limit.

## Design

### Part 1 — recognise a throttle (`pkg/emailerror`)

- **`ClassifiedError.RateLimited bool`** (`error.go`): set only with `Type = ErrorTypeProvider` and `Retryable = true`. The provider refused because of the sending pace, and the same email is expected to go through later at a lower pace. `IsProviderError` and `ShouldTriggerCircuitBreaker` are unchanged; the worker decides (Part 3).
- **Quota words**, helper `isQuotaSignal(text)`: `daily`, `hourly`, `monthly`, `quota`, `credits`. Text containing any of them is never `RateLimited`: a quota does not come back by slowing down. AWS reports "Daily message quota exceeded" and "Maximum sending rate exceeded" under the same throttling code (https://docs.aws.amazon.com/ses/latest/dg/manage-sending-quotas-errors.html).
- **Per provider**, each check returns at once, before the recipient/provider phrase lists, so a list can neither steal it nor change its retryability:
  - **SES** (`ses.go`; the string is `SES error: <Code>: <Message>` or `SES error: api error <Code>: <Message>`, from `wrapSESError`, `ses_service.go:1292`): the string contains `TooManyRequestsException`, `ThrottlingException`, `Throttling` or `maximum sending rate` → `Type = Provider`, `Retryable = true`, `RateLimited = !isQuotaSignal(errStr)`, return. This also keeps a v2 quota refusal retryable whatever its wording (today "Daily message quota exceeded" is provider and retryable through `ses.go:82`; a wording without "quota" would otherwise have fallen to `Retryable = false`). No change to `sesProviderPatterns`.
  - **SendGrid, Mailgun, Mailjet, SparkPost** (`API returned non-OK status code 429`) and **Postmark** (`postmark API error (429): <body>`): `httpStatus == 429` → `Type = Provider`, `Retryable = true`, `RateLimited = !isQuotaSignal(errStr)`, return. The four first carry no response body, so a quota they report as 429 is seen as throttling: it slows the integration down to its floor before counting (Part 3), then pauses as any provider failure. The body is not added to their errors: their recipient patterns are bare substrings (`"550"`…`"554"` in `sendgrid.go:25-29`, `"5002"`/`"2008"` in `sparkpost.go:18-19`) that a JSON body would match by accident.
  - **SMTP** (`smtp.go`), inside the existing `extractSMTPCode` branch: a 4xx code, and the **reply text after `code NNN:`** contains a throttle phrase — `rate limit`, `ratelimit`, `throttl`, `too many messages`, `too many emails`, `too much mail`, `sending rate`, `submission rate`, `rate exceeded` — and no quota word. At `RCPT TO` (`RCPT TO rejected for %s with code`, `smtp_service.go:455`) a refusal that blames the mailbox is not a throttle: an enhanced code `4.2.x`, or the words `mailbox` or `receiving` in the reply text (not `user`, which relays also use for the authenticated sender: `450 4.7.1 User rate limit exceeded`) (Gmail's `4.2.1 … receiving mail at a rate …` is about that recipient, and slowing the whole integration would not help). A sender rate limit refused at RCPT, as Postfix policy services do (`450 4.7.1 … rate limit exceeded`), is a throttle. Matching the reply text only, not the whole string, keeps a recipient address out of the match; no current string outside RCPT carries an address, so this is defensive. Real strings (`smtpReplyDetail` drops the code from the text, `SendEmail` wraps): `failed to send email: message rejected with code 454: Throttling failure: Maximum sending rate exceeded.` (SES's SMTP endpoint), `failed to send email: MAIL FROM rejected with code 451: 4.7.1 Error: too much mail from 10.0.0.1`. The greeting carries no text (`unexpected greeting code: 421`) and stays an ordinary provider error.
  - **Unknown provider kind**: `httpStatus == 429` → provider, retryable, `RateLimited = !isQuotaSignal(errStr)`, as for the HTTP providers.
- Everything else is classified as today, including SES `LimitExceededException` without a throttle code (provider, not retryable, `ses.go:78-88`).

### Part 2 — the integration's pace adapts (`internal/service/queue/rate_limiter.go`)

`IntegrationRateLimiter` keeps, per integration, a state under its own mutex instead of a bare `*rate.Limiter`. Rates are stored **per minute** as `float64` and converted with `/ 60` only when handed to `x/time/rate`; anything reported as an `int` uses `math.Round`.

```go
type adaptiveLimiter struct {
    mu         sync.Mutex
    limiter    *rate.Limiter
    ceiling    float64   // configured rate, per minute
    effective  float64   // current pace, per minute; floor <= effective <= ceiling
    generation uint64    // decreases and resets so far; never goes back
    recovering bool      // an accepted send since the last throttle
    stepFrom   time.Time // when the next recovery step is counted from
    floorSince time.Time // when the pace reached the floor; zero above it
}
```

- **Constants** (code, not configuration): `throttleDecrease = 0.75` (keep 75 % of the current pace); floor `max(0.10 * ceiling, 1)` per minute; `floorDwell = 30 * time.Second` (how long the pace must have sat at the floor before a throttle counts); recovery `+5 %` of the ceiling every `10 s`.
- **Clock**: `IntegrationRateLimiter.now func() time.Time` (default `time.Now`, set by a test constructor). It is read inside the state's mutex, and every `x/time/rate` call takes it explicitly (`ReserveN`, `DelayFrom`, `CancelAt`, `TokensAt`, `SetLimitAt`); `GetStats` uses `TokensAt(now)` instead of `Tokens()`.
- **Configured rate changes** (a ceiling different from the stored one): the state resets: `effective = ceiling`, `recovering = false`, a fresh limiter, `generation++`, and `floorSince` set to `now` when the ceiling is its own floor (1/min) and to zero otherwise; a new state is initialised the same way, so at 1/min the 30 s dwell counts from creation. Editing the limit is a fresh start in both directions.
- **Decrease, `RecordThrottle(integrationID string, ratePerMinute int, generation uint64) (atFloor bool)`**, under the mutex (creating the state from `ratePerMinute` if it is missing):
  - A throttle with an **older generation** returns `false` and changes nothing: it comes from a send reserved before the latest decrease (K sends in flight are throttled together; one event must cut the pace once, as the breaker's epochs do, and a send dispatched above the floor must not count as a refusal at the floor).
  - A throttle of the **current generation** always sets `recovering = false` (so an accepted send just before cannot lift the pace off the floor 10 s later). Then:
    - at the floor: returns `atFloor = now - floorSince >= floorDwell`, changes nothing else. Before the dwell, throttles at the floor give the email back like any other: a burst of refusals of a few seconds does not pause broadcasts (at 6,000/min the pace reaches the floor in under 2 s);
    - above it: `effective = max(floor, effective * 0.75)` (setting `floorSince = now` when it lands on the floor), `generation++`, and the limiter is **replaced** by a new one at the new pace whose first token is taken at `now` (so the next send waits one interval). Replacing it, rather than `SetLimitAt`, drops the debt of the tokens reserved ahead by the sends waiting in this round: with `x/time/rate` a lowered limit keeps that debt and stretches it (`SetLimitAt` keeps `tokens`, `rate.go:306-315`), which at the floor could hold the lane for K / floor minutes. One Warn log per decrease names the integration, the old and new pace per minute.
- **Waiting sends**: `ReserveBefore` returns the generation with the reservation. After `waitForToken`, `processLane`'s goroutine checks `CurrentGeneration(integrationID)`; if it changed, the row is not claimed (it stays as it is for the next round, like a send stopped by the round's open circuit), and a probe is given back with `CancelProbe`, as on the `!waitForToken` path (`worker.go:538-541`): `processLane`'s own deferred `CancelProbe` does not fire once `probeSent` is set, and a leaked probe would keep `Allow` refusing until a restart. The dispatch loop also stops dispatching once the generation differs from the one of the round's first reservation, as it does once the circuit opens (`worker.go:516`), so the rows left go out in order next round. So no send reserved at the old pace goes out after a decrease.
- **Recovery** starts only after an answer that shows the provider took the request at this pace: **`RecordAccepted(integrationID string, generation uint64)`**, called on a successful send and on a refusal that blames the recipient (`IsRecipientError`), sets `recovering = true` and `stepFrom = now` when `effective < ceiling`, `!recovering` and `generation == current`. While `recovering`, every access (`TokenDueBy`, `ReserveBefore`, `EffectivePerMinute`, `RecordThrottle`, `RecordAccepted`) applies `steps = floor((now - stepFrom) / 10s)`, `effective = min(ceiling, effective + steps * 0.05 * ceiling)`, `stepFrom += steps * 10s`, then `SetLimitAt(now, effective/60)` if it changed (raising the limit has no debt to stretch), clearing `floorSince` once above the floor. With no stored state, `RecordAccepted` does nothing. Idle time after an accepted send counts; time spent waiting after a throttle does not. Without this, the waits themselves recovered the pace: at a ceiling up to 60/min a token at the floor comes more than 10 s after the last, so a step always lifted the pace off the floor before the next throttle and `atFloor` never came true (simulated by the review: zero at-floor throttles in 2 h for 5–55/min).
- **`ReserveBefore`** holds the state's mutex across recovery, `ReserveN`, `CancelAt` and the generation read; it returns `(slot Slot, ok bool)` with `Slot{Reservation *rate.Reservation; Delay time.Duration; Generation uint64}`.
- **`EffectivePerMinute(integrationID string, ratePerMinute int) int`**: the current pace, rounded; `processLane` uses it for `laneBatchSize`.
- **`GetOrCreateLimiter`** becomes unexported (`state(integrationID, ratePerMinute) *adaptiveLimiter`): it returned the inner `*rate.Limiter`, which a decrease now replaces. Its test uses (`rate_limiter_test.go:22-90, 290-335`, `worker_test.go:645-646`) move to `EffectivePerMinute`, `TokenDueBy` and the stats. **`GetCurrentRate`** loads the new state and returns the effective pace per second.
- **Stats** (`RateLimiterStats`): `RatePerMinute` is the effective pace; add `ConfiguredPerMinute`. Not exposed by any endpoint today.
- An Info log when `effective` gets back to the ceiling.
- **Lock order**: the state's mutex, then the limiter's. Nothing takes them the other way.

Worked example (configured 5,000/min, provider accepts 3,000/min): 5,000 → 3,750 → 2,812, then +250 every 10 s once emails go through: 3,062 is throttled and drops to 2,297, which climbs back past 3,000 in 30 s, and so on. The review's event-ordered simulation (token-bucket provider, 10 sends at once) gives 70–84 % of what the provider accepts depending on the rate (5,000 vs 3,000: 80 %; 600 vs 100: 84 %; 25 vs 10: 70 %), no collapse, no opening, and about 150 throttled emails per hour at 5,000 vs 3,000, each sent again without spending an attempt.

Termination for a provider refusing everything as throttling: each throttle of the current generation lowers the pace, no recovery happens without an accepted answer, so the pace reaches the floor after at most 9 decreases (0.75^8 = 0.1001 > 0.10); 30 s later, each throttle at the floor counts as a provider failure, and 5 open the circuit (on an open circuit, the first counted probe re-opens it). Simulated by the review: the circuit opens after about 31 s at 6,000/min, 44 s at 600, 107 s at 60, 231 s at 25 (the console's default, `Integrations.tsx:505`) and 300 s at 1/min. Bursts of refusals of 2 to 30 s at 3,000–6,000/min do not open it.

### Part 3 — the worker handles a throttle (`internal/service/queue/worker.go`)

- `processLane` passes the slot's generation to its goroutine, which skips the row when the generation changed during `waitForToken` (Part 2), and on to `processEntry` (new parameter). It uses `EffectivePerMinute` for `laneBatchSize`.
- In `processEntry`, right after classification (`worker.go:786`), when `classifiedErr.RateLimited`:
  - `atFloor := w.rateLimiter.RecordThrottle(integration.ID, integrationRate(integration), generation)`.
  - **Not at floor — the email is given back:**
    - `settled = true`, no `RecordFailure`. On a probe, `CancelProbe(integration.ID, ticket)`: a throttle shows neither that the provider is back nor that it is down. The next probe can go as soon as the lowered pace gives a token (`TokenDueBy` is checked before `Allow`, `worker.go:448`), and no recovery happens meanwhile, so throttled probes walk the pace down to the floor, where a throttled probe counts and re-opens the circuit. `CancelProbe`'s doc becomes "gives back a probe that settled nothing: never sent, or throttled".
    - Message history: only when the pre-send upsert ran (automation on a provider that sets its own Message-ID, Mailgun, `worker.go:765`), the failure is upserted with `writeCtx`, so that row does not read as sent; the next successful send clears it. Otherwise nothing is written: a first upsert is an INSERT, which fires the `email.sent` webhook (`webhook_triggers.go:345`) and timeline rows for an email not sent yet.
    - `queueRepo.ReleaseThrottled(writeCtx, workspace.ID, entry.ID, err.Error())` (Part 4). On error, log at Error; the row stays `processing` and is reclaimed after 2 minutes, attempt spent, as a crashed send is.
    - No `onEmailFailed` callback (nothing failed; nothing registers one today). Return nil.
  - **At floor for 30 s — the throttle is a provider failure**, through the existing path unchanged (`RecordFailure`, held-retry branch, `handleError`).
- On success, and on a refusal that blames the recipient, `w.rateLimiter.RecordAccepted(integration.ID, generation)`.
- A throttled send cut by the shutdown grace takes the existing `sendCtx.Err()` branch first (`worker.go:772`), unchanged.

### Part 4 — give the row back (`internal/repository/email_queue_postgres.go`, `internal/domain/email_queue.go`)

New `EmailQueueRepository` method, mock regenerated (`go generate ./internal/domain/...`):

```go
// ReleaseThrottled gives back a row whose send the provider refused for its pace: the
// row is due again at once and the attempt its claim counted is returned. Only a row
// still processing is changed.
ReleaseThrottled(ctx context.Context, workspaceID string, id string, errorMsg string) error
```

```sql
UPDATE email_queue
SET status = 'pending',
    attempts = LEAST(GREATEST(attempts - 1, 0), GREATEST(max_attempts - 1, 0)),
    next_retry_at = NULL, last_error = $2, updated_at = NOW()
WHERE id = $1 AND status = 'processing'
```

- `pending` with no retry date is due at once (`emailQueueDuePending`, `email_queue_postgres.go:152`), keeps its priority and creation time, so it goes out first next round, at the lower pace.
- `LEAST(…, max_attempts - 1)`: a row reclaimed from stuck processing (`emailQueueStuckProcessing` has no attempts guard, `:154`) can be at `max_attempts + 1` after its claim; minus one would leave it `pending` at `max_attempts`, which no fetch takes and `GetSourceCounts` counts as pending forever (the rule at `:145-148`). The clamp keeps it due.
- `AND status = 'processing'`: a row no longer processing (deleted after another instance sent it, given up) is left alone. Zero rows affected is not an error.
- Like `MarkAsFailed`, the write does not know which claim it belongs to (candidate 1, separate PR); on one instance the round barrier keeps a row from being claimed twice.

### Part 5 — docs (`Notifuse/docs`, `features/broadcast-campaigns.mdx`)

Describe the behaviour as it is, with no comparison to earlier versions:

- **Sending Speed**, paragraph at line 351: when the provider answers that emails arrive too fast, the integration slows down by a quarter each time, down to a tenth of its limit, and once emails go through again it climbs back by 5 % of its limit every 10 seconds. The refused email is sent again at the slower pace and keeps its attempts. An SMTP refusal counts as "too fast" when it has a 4xx code and says so (rate limit, too many messages…), unless it blames the recipient's mailbox. Refusals that name a daily, hourly or monthly quota, and refusals that continue for 30 seconds at a tenth of the limit, are provider failures: several in a row pause the integration's broadcasts. SendGrid, Mailgun, Mailjet and SparkPost refusals carry only their status code, so a quota they answer with 429 looks like "too fast": the integration first slows down to a tenth of its limit. Keep the advice to set the limit at or below what the provider accepts (each climb past it costs a few refused emails). After an opening, the integration resumes at the pace it had, not necessarily at full speed. The existing text on the A/B tests and the SES client's retries stays.
- **Circuit Breaker Protection**, line 359: "After 5 consecutive transmission failures" gains "(a provider asking to slow down is not a failure, see Sending Speed)".
- **Common Failure Scenarios**: "Temporary outages or rate limit exceeded" becomes "Temporary outages, a sending quota reached, or a provider still refusing at a tenth of the limit".
- Per-instance bullet: each instance slows down on its own.
- Message History: an email given back for its pace shows nothing until it is sent (Mailgun automation emails show the refusal until then).

### Part 6 — CHANGELOG and version

- Bump `VERSION` in `config/config.go` to 42.3, the version Pierre opened for this change, then `make sdk-build` on Node 24 and commit `web_analytics_sdk/dist/` and `web_analytics_sdk/package.json` (CI diffs them byte for byte, `Makefile:64-73`).
- Entry:
  > **Improvement**: When an email provider answers that emails arrive too fast, the email integration slows down, then climbs back to its rate limit once emails go through. The refused email is sent again at the slower pace and keeps its attempts, and its broadcast keeps sending. A quota named in the refusal, or refusals that continue for 30 seconds at a tenth of the rate limit, pause the integration's broadcasts.
- The 42.0 entry is left as it is unless Pierre says the "no comparison with earlier versions" rule covers the CHANGELOG.

## Not in scope

- **The SES client's own retries** (`RetryMaxAttempts: 4`, `ses_service.go:209`): a throttled SES send is retried inside the call for several seconds before Notifuse sees it.
- **`Retry-After`** headers and **response bodies** of the four HTTP providers whose errors carry only the status (see Part 1).
- **Fixed-window providers** (per minute or per hour): slowing down helps only once the window turns; a refusal naming an hourly quota counts as a provider failure.
- **Sharing the pace across instances**: each instance slows on the throttles it sees.
- **Transactional emails** are sent from the API, not the queue, and are not slowed down.
- **A manual pause or a cancel while a send is throttled**: the row given back goes `pending` and can be sent while the broadcast is paused, or after it is cancelled (`DeleteBySourceTx` leaves processing rows), as a failed one does today through `MarkAsFailed` (candidate 3).

## Tests

| Implementation file | Test file | Cases |
|---|---|---|
| `pkg/emailerror/error.go`, `classifier.go`, `ses.go`, `smtp.go`, `sendgrid.go`, `mailgun.go`, `mailjet.go`, `sparkpost.go`, `postmark.go` | `pkg/emailerror/classifier_test.go` | Each case pins Type, Retryable and RateLimited. RateLimited: SES `SES error: TooManyRequestsException: Maximum sending rate exceeded.`, `SES error: api error Throttling: Maximum sending rate exceeded.`, `SES error: ThrottlingException: Rate exceeded`; SendGrid/Mailgun/Mailjet/SparkPost `API returned non-OK status code 429`; Postmark `postmark API error (429): {"ErrorCode":0,"Message":"Rate limit exceeded"}`; SMTP `failed to send email: message rejected with code 454: Throttling failure: Maximum sending rate exceeded.`, `failed to send email: MAIL FROM rejected with code 451: 4.7.1 Error: too much mail from 10.0.0.1`, `failed to send email: DATA rejected with code 450: 4.7.0 Message submission rate for this client has exceeded the configured limit`; unknown kind `status code: 429`. Provider, retryable, not RateLimited: SES `SES error: TooManyRequestsException: Daily message quota exceeded.`, SES `SES error: TooManyRequestsException: Daily sending limit exceeded` (retryable although no "quota"), Postmark 429 with `monthly` in the body, SMTP `failed to send email: message rejected with code 454: Throttling failure: Daily message quota exceeded.`, SMTP `failed to send email: message rejected with code 452: 4.3.1 Daily sending quota exceeded` (what the worker classifies from the quota server), SMTP `unexpected greeting code: 421`, SMTP RCPT `failed to send email: RCPT TO rejected for a@b.c with code 450: 4.2.1 The user you are trying to contact is receiving mail at a rate that prevents additional messages from being delivered`. RateLimited at RCPT: `failed to send email: RCPT TO rejected for a@b.c with code 450: 4.7.1 <a@b.c>: Recipient address rejected: rate limit exceeded`. The reply-text helper on its own: an address containing `ratelimit` before `code NNN:` does not match. Unchanged from today: SparkPost `API returned non-OK status code 420` (unknown, not retryable), SendGrid 500, SES `AccessDeniedException`, SMTP `code 550: user unknown, rate limit` (recipient: 5xx wins). |
| `internal/service/queue/rate_limiter.go` | `internal/service/queue/rate_limiter_test.go` | Injected clock throughout. A current-generation throttle keeps 75 % and bumps the generation; an older generation changes nothing and returns `false`, including right after the pace reached the floor; at the floor, throttles return `false` for 30 s and `true` after; `RecordAccepted` then a throttle at the floor stops recovery (10 s later the pace is still the floor and the throttle still reports `true`); the limiter replaced on a decrease has no debt: after K tokens reserved ahead, the next token after a decrease is one new interval away; repeated throttles stop at the floor (10 % of the ceiling, at least 1/min); at 1/min the floor is the ceiling and a throttle reports `atFloor` once 30 s have passed since the state was created. `RecordAccepted` with no state does nothing. No recovery after a throttle however long the clock runs; after `RecordAccepted` +5 % of the ceiling per full 10 s, partial intervals nothing, never above the ceiling, idle time counts; `RecordAccepted` with an older generation starts nothing. A provider refusing everything, simulated at 1, 25, 60, 600 and 6,000/min with sends spaced by the limiter's own tokens: at most 9 decreases, then `atFloor` once 30 s have passed at the floor, at each rate. Changed configured rate resets the pace, in both directions, and bumps the generation. `ReserveBefore` returns the current generation and a delay at the effective pace; `TokenDueBy` and `EffectivePerMinute` follow it; `EffectivePerMinute` for 29/min is 29 (rounding). 20 goroutines throttling with one generation cut the pace once (race detector). Existing `ReserveBefore` tests (`rate_limiter_test.go:98-158`) move to the new signature. |
| `internal/service/queue/worker.go` | `internal/service/queue/worker_test.go` (gomock) | A rate-limited refusal not at floor: `ReleaseThrottled` with the entry and the error, no `MarkAsFailed`/`MarkAsPermanentlyFailed`, no history upsert for a broadcast row, a failure upsert for a Mailgun automation row, `GetFailures` 0, pace lowered. Throttles down to the floor and for 30 s there (count derived from the constants) leave the circuit closed; then five at the floor open it and call the callback once. A throttled probe: probe given back, circuit still open, next `Allow` grants a probe once a token is due. A row whose token wait spans a decrease is not claimed, and the dispatch loop stops there; a probe whose wait spans a generation change is given back (next `Allow` grants a probe). A recipient refusal calls `RecordAccepted`. K concurrent throttled sends of one round lower the pace once. Success calls `RecordAccepted`. `ReleaseThrottled` failing is logged. Lane batch size follows the effective pace. The existing `LaneCircuit` subtests that open the circuit with SES's "ThrottlingException: Maximum sending rate exceeded" (`worker_test.go:2463, 2514, 2545`) switch to a provider error that is retryable and not a throttle (`status code: 503`, through the HTTP fallback; `ServiceUnavailable` is not retryable under SES, `ses.go:78-88`); the `throttled` variable (`:1746-1747`) is renamed and re-commented; `fakeQueue.wire` gains `ReleaseThrottled`. |
| `internal/repository/email_queue_postgres.go` | `internal/repository/email_queue_postgres_test.go` (sqlmock) | `ReleaseThrottled`: exact SQL and argument order; zero rows affected is not an error; exec error wrapped; `getDB` error. |
| same | `tests/integration/email_queue_e2e_test.go` (real PostgreSQL) | `ReleaseThrottled` on a processing row: pending, attempts minus one, no retry date, last error set, `FetchPendingForIntegration` returns it at once; a row reclaimed from stuck processing at `max_attempts + 1`: pending at `max_attempts - 1` and fetched; on a pending, failed or paused row: unchanged. |
| whole flow | `tests/integration/email_queue_throttle_test.go` (new, `//go:build integration`, `TestEmailQueueThrottle…`, real worker) | A fake SMTP server in the test, modelled on `quotaSMTPServer` (`broadcast_partial_failure_test.go:25`), answers `451 4.7.1 Rate limit exceeded` to DATA when more than N **accepted** messages arrived in the last second. Configured limit between 2× and 5× N×60 (below 10×, so the floor stays under the server's rate). A few hundred queued broadcast emails: each recipient delivered once, no row ends failed, queue empty, broadcast never paused, circuit never opened, measured pace below the configured limit. Second case: the server answers `451 4.7.1 Rate limit exceeded` to everything (limit ≥ 600/min): the circuit opens within a minute and the broadcast is paused. Third case: `452 4.3.1 Daily sending quota exceeded` to everything: the circuit opens as today. |
| `console/src/components/settings/Integrations.test.tsx` | itself | Comment only (`:152`): "a limit above the provider's leads to throttling, which slows sending down". |

## Final step — run the tests

```
go test -race ./pkg/emailerror/...
go test -race ./internal/service/queue/...
make test-repo
make test-unit
INTEGRATION_TESTS=true go test -race -tags integration,licdev -timeout 20m ./tests/integration/ -run 'TestEmailQueue|TestCircuitBreaker|TestBroadcastESPFailure|TestBroadcastPartialFailure' -v
make test-integration
cd console && npm test -- Integrations
```

## Plan review outcomes

| Source | Finding | Verdict | Change |
|---|---|---|---|
| Gemini 1 | A lowered limit keeps the debt of reservations taken ahead and stretches it; sends already waiting go out at the old pace | Confirmed (`rate.go:306-315`, `worker.go:519-543`) | Limiter replaced on a decrease; a send whose wait spans a decrease is not claimed |
| Gemini 2, 6; xhigh 1 | Time-based recovery during waits keeps the pace off the floor: below 60/min a forever-throttling provider never counts | Confirmed (arithmetic, simulation) | Recovery only after an accepted send; termination test at 1–6,000/min |
| Gemini 3 | Four HTTP providers carry no body, so a quota 429 is seen as throttling | Confirmed | Kept: the floor bounds it; documented. Adding bodies rejected (bare-substring recipient patterns would match them) |
| Gemini 4; xhigh 6 | `toomanyrequests` in SES provider patterns can make a quota refusal non-retryable | Confirmed (`ses.go:78-88`) | SES throttle codes return early, retryable; no pattern added |
| Gemini 5; xhigh 7 | SMTP check must sit in the code branch; phrases matched on the whole string hit addresses; per-recipient throttles at RCPT | Confirmed | Reply text only, RCPT excluded, phrases added, `hourly` added to quota words |
| Gemini 7 | Float truncation `29/60*60` → 28; clock injection gaps | Confirmed | Per-minute storage, `math.Round`, explicit times everywhere |
| Gemini 8; xhigh 8 | History writes on a given-back email | Confirmed (`webhook_triggers.go:345`) | Failure upserted only after a pre-send upsert (Mailgun automation), with `writeCtx` |
| Gemini 9; xhigh 2 | Release can leave a pending row at `max_attempts` | Confirmed (`email_queue_postgres.go:152-154`) | `LEAST(…, max_attempts - 1)` and e2e case |
| xhigh 3, 4, 5 | Existing worker tests break; "10 refusals" contradicts the floor; wrong expectations and SMTP strings | Confirmed | Tests table corrected |
| xhigh 9 | Lock/clock/generation details; "back to its limit in a minute" wrong; cancel case | Confirmed | Stated in Part 2; Not in scope extended |
| xhigh 10, 11, 12 | Docs, CHANGELOG wording, SDK build, test tags | Confirmed | Parts 5 and 6, test commands |
| Gemini 10 | Callers of the changed signature | Confirmed | Tests table |
| Gemini v2 NF1 | A stale-generation throttle right after reaching the floor counts as a floor refusal; an accepted send at the floor keeps recovery on across floor throttles | Confirmed | Stale generation returns false; current generation always stops recovery |
| Gemini v2 NF3; xhigh v2 N4 | A probe skipped after a generation change leaks `probeInFlight` | Confirmed (`worker.go:524-541`) | `CancelProbe` on that path; dispatch loop stops; test |
| Gemini v2 NF5 | SES `LimitExceededException` without "quota" is not retryable | Confirmed, existing behaviour | Unchanged, stated in Part 1 |
| Gemini v2 NF6, NF7; xhigh v2 N9 | `GetCurrentRate` type assertion; unknown kind quota check; exported `GetOrCreateLimiter` returns a replaced limiter | Confirmed | Part 2, Part 1 |
| xhigh v2 N1 | A 2 s burst of refusals at 6,000/min still pauses broadcasts | Plausible (simulation) | 30 s dwell at the floor before a throttle counts |
| xhigh v2 N2 | RCPT exclusion misses Postfix policy rate limits | Plausible | Only mailbox-blaming RCPT refusals excluded |
| Gemini v3 1, 2 | `floorSince` at creation/reset; `user` in the RCPT exclusion hits sender limits | Confirmed | Part 2 reset rule; `user` dropped |
| xhigh v2 N3, N5–N8 | Test error not retryable; recovery on recipient refusals; throughput figures; SparkPost docs sentence; test strings | Confirmed | Tests table, Part 2, docs |
