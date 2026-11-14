# Segment and list membership reliability plan

Five places in the segment and list membership pipeline swallow a failed write or skip a
cleanup step, so membership silently drifts from what the segment definition or the automation
says. No schema change and no migration: every fix uses existing columns, the task's JSON state
and existing retry paths.

The plan was reviewed before implementation; the review's confirmed points are folded in below
(sections 4 to 7 and the "Known, not handled" list).

## 1. Segment build skips contacts it failed to add

**Today.** `SegmentBuildProcessor.processBatch` (`internal/service/segment_build_processor.go`)
logs a failed `AddContactToSegment` and moves on. The batch counts as processed, the cursor
advances past it, and the build finishes `active` without that contact. `RemoveOldMemberships`
then deletes the contact's previous-version row, so an existing member that still matches is
removed.

**Fix.** `processBatch` returns the error. `Process` turns it into `return false, err`; the task
service calls `MarkAsFailed`, which keeps the state last written by `saveProgress` and reschedules
the task while retries remain (60 s apart). The retry replays the failed batch; the upsert makes
already written rows harmless. `MatchedCount` increments of the failed batch only live in memory.
Matched emails are added in the order the query returned them (a slice, not a map), so the
behaviour and the tests are deterministic.

**Last attempt.** When the task has no retries left (`!task.HasRetriesLeft()`), the build must not
leave the segment `building` forever (the queue and the daily check skip `building` segments).
Before returning the error, it sets the segment back to `active` (status-only write, section 6,
conditional on the task's version) and, for a segment with `recompute_after`, moves it to the
next 5 AM, so the next daily rebuild tries again instead of a rebuild every scheduler tick. This
covers every error once the state is valid, including a last attempt that cannot read the segment
(the daily check marked it building before the task ran; nothing is rescheduled then), and runs on
a context detached from the task's, which is dead after a timeout or a shutdown.

**Unit tests** (`internal/service/segment_build_processor_reliability_test.go`, existing ones in
`segment_build_processor_test.go`):
- `TestSegmentBuildProcessor_Process_AddContactError`: two matching emails, the first add fails →
  `(false, err)`, the second email is not added, no `RemoveOldMemberships`, no status write to
  `active`, no `SaveState` after the failed batch.
- `TestSegmentBuildProcessor_Process_AddContactErrorLastAttempt`: same with
  `RetryCount == MaxRetries` → status written back to `active`, `UpdateRecomputeAfter` called with
  a future time, error still returned.

## 2. Segment queue drops contacts whose membership write failed

**Today.** `ContactSegmentQueueProcessor.processContact` logs a failed `AddContactToSegment`
(Warn) or `RemoveContactFromSegment` (Debug) and returns `nil`. The claim already deleted the
queue row, so the change is lost until the contact's next event. The comment "It's OK if the
contact wasn't in the segment" is wrong: deleting a non-member affects zero rows without error.

**Fix.** `processContact` still attempts every segment, collects the write errors and returns them
joined (`errors.Join`). `ProcessQueue` already re-enqueues a contact whose `processContact` fails.
Membership writes are idempotent: an add of an existing member is an upsert (no INSERT, no
`segment.joined`), a remove of a non-member deletes nothing (no `segment.left`).

**Unit tests** (`internal/service/contact_segment_queue_processor_test.go`):
- add fails on one of two segments → the other segment is still written, the contact is
  re-enqueued (sqlmock expects the requeue INSERT), processed count 0.
- remove fails → same.

## 3. Automation list steps succeed when the list write failed

**Today.** `AddToListNodeExecutor.Execute` and `RemoveFromListNodeExecutor.Execute` put the
repository error in the node output and return success.

**Fix.**
- Add to list: return the error. `AutomationExecutor.Execute` marks the node execution failed and
  calls `handleError`, which keeps `CurrentNodeID` on this step and retries with exponential
  backoff (2 min after the journey's first failure, then 4 min) until `RetryCount` reaches
  `MaxRetries`, then fails the journey. `AddContactToList` already leaves bounced and complained
  rows untouched without an error.
- Remove from list: `*domain.ErrContactListNotFound` (the contact has no row for the list) stays a
  success, with `"not_in_list": true` in the output. Any other error is returned.

**Unit tests** (`internal/service/automation_node_executor_test.go`):
- replace `TestAddToListNodeExecutor_Execute_AlreadyInList` (it pinned the swallowed error) with
  `TestAddToListNodeExecutor_Execute_RepositoryError`: error returned, nil result.
- `TestRemoveFromListNodeExecutor_Execute_NotInList`: repository returns
  `&domain.ErrContactListNotFound{}` → success, `NextNodeID` kept, `not_in_list` in the output.
- `TestRemoveFromListNodeExecutor_Execute_RepositoryError`: a generic error is returned.

## 4. Relative-date segments keep members that no longer match after the daily rebuild

**Today.** `SegmentRecomputeTaskProcessor.Process` creates a `build_segment` task at the
segment's current version. The build writes at that version and `RemoveOldMemberships` only
deletes `version < state.Version`, so a contact whose date left the window keeps its row until its
own next event reaches the queue. Manual rebuilds and tree edits already bump the version.

**Fix.** The daily rebuild gets its own generation, like a manual rebuild.

New repository method (compare-and-set, so a concurrent edit is never overwritten with the
snapshot returned by `GetSegmentsDueForRecompute`):

```go
// StartRecompute moves an active segment to the next version and marks it building, so the
// rebuild that follows writes its memberships at a generation of its own. It returns false
// when the segment is no longer active at fromVersion.
StartRecompute(ctx context.Context, workspaceID, segmentID string, fromVersion int64) (bool, error)
```
```sql
UPDATE segments SET version = version + 1, status = 'building', db_updated_at = $3
WHERE id = $1 AND version = $2 AND status = 'active'
```

Per due segment, the task is created first, then the bump. Bumping first would leave the segment
`building` with no task if the task creation then failed, and the check never selects a
`building` segment again.
1. Create the task with `Version: segment.Version + 1`. On error: log, skip the segment (nothing
   changed; the next check retries).
2. `StartRecompute(segment.ID, segment.Version)`. On error or `false`: log at Warn and keep going.

**Guard in the build (required by the ordering above).** At the start and in the per-batch check,
if `segment.Version < state.Version` the bump has not landed (another instance picked the task up
first, a transient error, or a concurrent rename wrote the old version back): the build writes
nothing and returns an error, so the task retries 60 s later. If the bump then lands, the retry
proceeds; if not, the segment is still `active` and due, and the next check creates a fresh task
and bumps to the same version. The earlier task's next retry then passes the guard too, so both
tasks build at that version: a duplicate scan of the same stored query, which converges. A tree
edit or a manual rebuild made just before the check bumps by one as well, so the check's task
runs beside the edit's own task in the same way.

Setting `building` in the compare-and-set keeps the next check from creating a second task while
this one is pending.

**Queue upsert never moves a row back.** `AddContactToSegment` sets
`version = GREATEST(contact_segments.version, EXCLUDED.version)`, so a queue pass that read the
segment before the bump cannot downgrade a row the build already rewrote at the new version (it
would then be deleted as stale).

**Repository tests** (`internal/repository/segment_postgres_test.go`, sqlmock):
- `StartRecompute`: true on 1 row, false on 0 rows, wrapped connection and exec errors.
- `AddContactToSegment`: the upsert uses `GREATEST`.

**Unit tests** (`internal/service/segment_recompute_task_processor_test.go`, update the "with
segments due for recompute" subtest):
- due segment at version 3 → task created with `Version: 4`, then `StartRecompute(id, 3)`.
- task creation fails → `StartRecompute` not called, the next segment still processed.
- `StartRecompute` returns false or an error → logged, processing continues.

**Build unit tests:** `TestSegmentBuildProcessor_Process_BumpNotLanded` (segment at 3, state at 4
→ error, no write at all; same when it happens at the per-batch check).

## 5. A build that matches nobody keeps stale members and is rebuilt every tick

**Today.** `Process` returns early when `MatchedCount == 0`, after setting the segment `active`,
without `RemoveOldMemberships` and without moving `recompute_after`. After an edit that matches
nobody, the old members stay (and still receive broadcasts, whose audience has no version filter)
while the console shows 0; a relative-date segment with no match stays due and is rebuilt every
scheduler tick.

**Fix.** Remove the early return; the end-of-build steps run whatever the number of matches.

**Unit tests:** update `TestSegmentBuildProcessor_Process_ZeroMatchedContacts`,
`TestSegmentBuildProcessor_Process_NoContacts` and
`TestSegmentBuildProcessor_Process_StateInitialization` (empty batch → now
`RemoveOldMemberships` is expected); add `TestSegmentBuildProcessor_Process_BatchesButNoMatch`
(a batch is evaluated, nobody matches, `RemoveOldMemberships` and `UpdateRecomputeAfter` called).

## 6. Build status writes no longer overwrite the segment

**Today.** The build writes the whole segment row three times (`building` at start, `active` at
the end, twice in the empty path) from the snapshot it read when the run started. A rename made
during a build is reverted, a version bump made during the build is written back, and a segment
deleted during a build is set `building` then `active` again, with its memberships re-added.

**Fix.** New repository method used for every status write of the build:

```go
// UpdateSegmentStatus sets the status of a segment still at version and not deleted.
// It returns false when the segment moved to another version or was deleted.
UpdateSegmentStatus(ctx context.Context, workspaceID, segmentID string, version int64, status string) (bool, error)
```
```sql
UPDATE segments SET status = $3, db_updated_at = $4
WHERE id = $1 AND version = $2 AND status <> 'deleted'
```

The build also stops at once (`true, nil`, nothing written) when the segment it reads is
`deleted`, at the start and at each per-batch check. When the final status write returns false,
the build completes without moving `recompute_after` (another build owns the segment now).

**Tests:** repository sqlmock tests for `UpdateSegmentStatus` (true, false, errors); build unit
tests `TestSegmentBuildProcessor_Process_DeletedSegment` (start and mid-build) and updated
expectations in the existing tests (`UpdateSegmentStatus` instead of `UpdateSegment`).

## 7. Build pagination by email instead of offset

**Today.** `GetBatchForSegment` pages with `ORDER BY email LIMIT/OFFSET`. A contact deleted
during a build, before the cursor, shifts the window and the build skips one later contact. With
section 4 every daily rebuild removes rows the build did not rewrite, so a skipped contact that
still matches would be removed and get a spurious `segment.left`.

**Fix.** New `ContactRepository.GetBatchForSegmentAfter(ctx, workspaceID, afterEmail, limit)`
(`WHERE email > $1 ORDER BY email LIMIT $2`) and a `LastEmail` field in `BuildSegmentState`
(`json:"last_email,omitempty"`, task JSON state, no schema change). The build pages by
`LastEmail`; `ContactOffset` keeps being updated for progress. A task already in flight with
`ContactOffset > 0` and no `LastEmail` fetches its next batch by offset once, then continues by
email.

**Tests:** repository sqlmock test for the keyset query; build unit tests use
`GetBatchForSegmentAfter` with the previous batch's last email, plus
`TestSegmentBuildProcessor_Process_ResumesLegacyOffset`.

## 8. Time budget before the end-of-build steps

The end steps (`RemoveOldMemberships` and the status write) run after the batch loop with no
budget check; the first daily rebuild after the upgrade can delete many accumulated rows, each
firing the membership triggers. If less than 60 s remain when the loop ends, the build saves its
progress and returns `false`, so the end steps run at the start of the next slice with a full
budget. Unit test: `TestSegmentBuildProcessor_Process_FinalStepsDeferredNearTimeout`.

## Integration tests (real Postgres, real repositories and processors)

New file `tests/integration/segment_membership_reliability_test.go`, one suite, one workspace per
subtest, reusing `createSegmentWithSQL`, `enqueueAged`, `memberCount`, `segQueueSize` and
`timelineKindCount`. Write failures are injected with a temporary BEFORE INSERT trigger raising for
one email, dropped afterwards (pattern of `broadcast_interruption_test.go`). Tasks run through the
real task service: `service.EnsureSegmentRecomputeTask` with the app's task repository, then
`client.ExecuteTask` (as `segment_e2e_test.go` does), and builds run through `tasks.execute`.

1. **Daily rebuild removes contacts that left the window.** Segment SQL on `created_at > NOW() -
   INTERVAL '7 days'`, `active`, `recompute_after` in the past, two members at the current
   version, one contact's `created_at` moved 30 days back. Run the check, assert one build task
   and the version + 1; run the check again before the build: no second task. Execute the build:
   the old contact has no row and one `segment.left`, the recent contact is a member at the new
   version with no new `segment.joined` or `segment.left`, the segment is `active` and
   `recompute_after` is in the future.
2. **Empty build cleans up and reschedules.** Both contacts out of the window: both rows removed,
   `active`, `recompute_after` in the future, the next check creates no task.
3. **Failed membership write fails the build, the retry completes it.** Three matching contacts,
   one with an existing row at the previous version, trigger raising for it. Execute the build:
   the task is pending with `retry_count = 1`, the segment is not `active`, and the existing row
   is still there. Drop the trigger, execute again: three members at the new version, `active`.
4. **Queue re-enqueues a contact whose membership write failed.** `ProcessQueue` returns 0 and the
   queue still holds the contact; drop the trigger, age the row, run again: member, queue empty.
5. **Deleted segment is not resurrected.** Delete the segment while its build task is pending,
   execute the task: status stays `deleted`, no membership rows.
6. **Automation list steps retry on a write failure.** New
   `tests/integration/automation_list_step_retry_test.go` with background workers: trigger →
   `add_to_list` → `remove_from_list` on a list the contact is not in. Trigger raising on
   `contact_lists` for the contact: the journey stays `active`, `retry_count = 1`, future
   `scheduled_at`, `current_node_id` on the add step, no list row. Drop the trigger, set
   `scheduled_at` to now: the journey completes, the contact is in the list.

## Known, not handled

- `SegmentService.UpdateSegment` writes the whole row from its own read. An edit whose read
  precedes the daily bump and whose write follows it puts the old version back (the build guard
  then makes the build retry) or, for a tree edit, writes the new tree at the same version as the
  running daily build; rows matching only the old tree are then kept until the next rebuild. The
  window is the few milliseconds between the edit's read and write.
- When such a whole-row write lands in the middle of a daily build, the build stops with an error
  at its next batch. The segment is back at its old version, active and due, so the next check
  bumps it to the same version again and a new build runs there; rows the stopped build wrote at
  that version and that no longer match are not removed by it, only by the next rebuild at a
  higher version.
- Duplicate builds at the same version (see the guard above) scan the contacts twice; one that
  starts after the other finished marks the segment building again until it ends.
- A membership insert that always fails (for example a broken trigger on `contact_timeline`)
  stops the build at that contact on every attempt; after the last one the segment is active
  with only the contacts before it rewritten. Before this change only that contact was skipped.
- Removing stale memberships fires the membership triggers for each row, including rows of
  contacts deleted before deletions cleaned memberships: those get a `segment.left` timeline row
  and webhook delivery again. Manual rebuilds and tree edits already did this; the daily rebuild
  now does it once at upgrade for every segment with relative dates. Not deleting them silently
  needs a trigger or data migration, out of scope here.
- A permanent membership write failure in the queue is retried on every pass (15 s debounce), as
  evaluation errors already are.
- Stale rows are deleted in one statement; section 8 gives it a full time slice but does not
  chunk it.

## CHANGELOG (44.0, unreleased)

- **Fix**: Segments with relative dates, such as "opened in the last 7 days", now drop the
  contacts that no longer match at their daily update, and a segment updated to match nobody is
  emptied. The first daily update after the upgrade removes the contacts that stopped matching
  earlier, so automations and webhooks on leaving a segment receive those exits at once.
- **Improvement**: A temporary database error while updating segment members, or in an
  automation's "Add to list" or "Remove from list" step, is now retried instead of leaving the
  contact out of the segment or the list.

## Run

```
make test-service
make test-repo
make test-domain
INTEGRATION_TESTS=true go test -race -tags integration,licdev -timeout 20m ./tests/integration/ \
  -run 'TestSegmentMembershipReliability|TestAutomationListStepRetry|TestSegmentQueueProcessorIntegration|TestSegmentE2E|TestAutomation$'
make test-unit
```
