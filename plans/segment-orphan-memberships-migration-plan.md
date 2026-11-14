# Segment memberships of deleted contacts: v44 cleanup plan

## Problem

Until 38.0, deleting a contact left its `contact_segments` rows in place (CHANGELOG 38.0: "segment
memberships ... were not cleaned at all"). Since 38.0 `ContactService.DeleteContact` removes the
contact row first and its memberships afterwards (`SegmentRepository.DeleteForEmail`), so a
deletion that fails between the two steps leaves rows behind too. `contact_segments` has no
foreign key to `contacts`, so nothing else removes them.

These rows inflate segment member counts and show erased addresses in the segment member list.
They stay until their segment is rebuilt at a higher version, or the address is queued for
segment recomputation again (`ContactSegmentQueueProcessor`). Once 44.0 rebuilds
relative-date segments every day at a new version, the first daily rebuild after the upgrade
deletes them through `RemoveOldMemberships` (the contact no longer exists, so it is never re-added).
Each delete fires the two `AFTER INSERT OR DELETE` triggers of `contact_segments`:

- `contact_segment_changes_trigger` (`track_contact_segment_changes()`) writes a `segment.left`
  row to `contact_timeline` with the deleted address, which also feeds the automation trigger
  functions attached to `contact_timeline`;
- `webhook_contact_segments` (`webhook_contact_segments_trigger()`) queues a `segment.left`
  delivery, with the deleted address in its payload, for every matching subscription.

So the upgrade would put erased addresses back on the timeline, in webhook payloads and possibly
in automations. Manual rebuilds and segment edits already do this today for the segments they
touch; the daily rebuild makes it happen on every relative-date segment at once.

## Fix

The v44 workspace update (44.0 is unreleased, so `v44.go` is amended rather than adding v45)
deletes every membership whose address has no contact, with the two triggers disabled for that
statement only:

```sql
ALTER TABLE contact_segments DISABLE TRIGGER contact_segment_changes_trigger
ALTER TABLE contact_segments DISABLE TRIGGER webhook_contact_segments
DELETE FROM contact_segments cs WHERE NOT EXISTS (SELECT 1 FROM contacts c WHERE c.email = cs.email)
ALTER TABLE contact_segments ENABLE TRIGGER webhook_contact_segments
ALTER TABLE contact_segments ENABLE TRIGGER contact_segment_changes_trigger
```

- **Why disable the triggers.** The rows belong to contacts that no longer exist; nobody should
  be told they left. `session_replication_role = replica` would avoid the table lock but needs a
  superuser (or `GRANT SET ON PARAMETER` from PostgreSQL 15). Triggers are named explicitly (as in
  v30) rather than `DISABLE TRIGGER USER`, so the re-enable cannot switch on any *other* trigger of
  the table. Like `CREATE INDEX` in v44 already, `ALTER TABLE` needs table ownership, which the
  migration user has. Both triggers exist on every
  workspace at 43 (`internal/database/init.go`, v8 and v19).
- **Transactional.** The migration manager runs the workspace update in one transaction. The
  disabled state is a catalog change, invisible to other sessions until commit, and the
  `SHARE ROW EXCLUSIVE` lock that `ALTER TABLE ... DISABLE TRIGGER` takes keeps every other write
  to `contact_segments` waiting until then (reads go through), so no concurrent write slips
  through without its triggers. On failure everything rolls back, triggers included.
- **Idempotent.** A rerun takes the lock again and deletes only the memberships orphaned since the
  previous run (for example when another workspace failed the first run), without events either.
- **Matching.** `contacts.email` is the primary key and a membership row only ever gets exactly the
  `contacts.email` it was computed from (segment SQL and queue processor), so `=` is the right
  test; `LOWER()` would be wrong for rows written before normalisation and would skip the index. The anti-join scans `contact_segments`
  once; `statement_timeout` is already lifted at that point.

### Position in the transaction

The five statements go **after the three index builds and before the exited recount**.

- Not at the end: the recount row-locks `automations`, and a runtime membership write takes
  `contact_segments` first and then, through the timeline trigger, updates `automations`. Taking
  the table lock after the recount could deadlock with such a write.
- Not before the builds: the lock is held until commit, so placing it early would make every
  membership write (segment builds, the segment queue, contact deletions) wait for the whole
  duration of the index builds.
- After the builds, the migration already holds the `SHARE` lock on `message_history` and the
  backfill's locks on `broadcasts`. Every membership write is a single autocommit statement, and
  its triggers write `contact_timeline`, `webhook_deliveries` and the automation tables, never
  `message_history` or `broadcasts`, so waiting for `contact_segments` cannot form a cycle. The wait is bounded by the
  `lock_timeout = '5s'` already set: past it the migration fails and is retried at the next start.
  While it waits, writes to `message_history` wait too, as they already do behind the recount.
- The recount then runs with `contact_segments` locked as well; it does not touch it, and the
  window adds only the delete and the recount.

The v44 type comment and the comment above `v44ExpectedWorkspaceStatements` are updated to say
this.

### Not done

- Timeline entries already written for deleted contacts, orphan rows of `contact_segment_queue` and
  of `contact_lists`: outside this change.
- A deletion that fails between the contact row and its memberships can still leave rows after
  44.0; the next rebuild then emits their exits as it does today.
- A trigger whose state was changed by hand before the upgrade (disabled, replica or always) is
  set back to firing normally. v37's `restoreTriggerEnabledState` keeps the original state; not
  worth it for two triggers Notifuse alone manages.

## Changelog

One sentence folded into the 44.0 entry on relative-date segments (that entry describes the first
daily rebuild's exits): the upgrade removes, without any exit, the segment memberships of contacts
deleted before 38.0, so segment counts can drop. If that entry is not on dev yet when the PR
opens, a separate **Fix** bullet in 44.0 instead (the rows were left by released versions).

## Files and tests

| File | Change | Tests |
|------|--------|-------|
| `internal/migrations/v44.go` | five statements above, in `v44WorkspaceStatements()` after the index builds; comments | `internal/migrations/v44_test.go` |
| `internal/migrations/v44_test.go` | `v44ExpectedWorkspaceStatements` gains the five statements at the same place; comment | `TestV44Migration_UpdateWorkspaceRunsExactlyItsStatementsInOrder` (exact order), `TestV44Migration_UpdateWorkspaceStopsOnTheFirstError` (each new statement failing stops the rest) |
| `tests/integration/v44_orphan_segment_memberships_migration_test.go` (new) | real Postgres | see below |
| `CHANGELOG.md` | see above | — |

`TestV44OrphanSegmentMembershipsMigration`, on a workspace created by the suite (already at 44):

1. Setup: a contact `kept@`, a segment, and a webhook subscription to `segment.left` and
   `segment.joined`. Insert memberships for `kept@` and for `gone@` (no contact row). Record the
   counts of `segment.left` timeline rows and of webhook deliveries.
2. Run `V44Migration.UpdateWorkspace` in a transaction and commit, twice.
3. Assert: the `gone@` membership is gone, the `kept@` one stays; no new `segment.left` timeline
   row and no new webhook delivery for `gone@`; both triggers are enabled (`pg_trigger.tgenabled =
   'O'`).
4. Assert the triggers still work after the run: deleting the `kept@` membership writes a
   `segment.left` timeline row and queues a delivery.
5. Failure between disable and enable: with `contacts` locked `ACCESS EXCLUSIVE` from another
   connection, the delete (the first statement reading `contacts`) fails on the lock timeout; the
   error names the step, and after rollback the orphan is still there and both triggers are 'O'.
   This runs first.
6. While a successful run is still uncommitted, a membership insert from another connection with
   `lock_timeout = '200ms'` fails with 55P03 and a read goes through.

The plan was reviewed before implementation; the review's points are folded in above (lock level,
lock order on `broadcasts`, matching, rerun, test steps 5 and 6, changelog, test command).

## Test commands

```
make test-migrations
INTEGRATION_TESTS=true TEST_DB_HOST=localhost TEST_DB_PORT=5433 \
  go test -race -tags integration,licdev -timeout 20m ./tests/integration/ \
  -run 'TestV44|TestMigration|TestTimelineLinkClicked'
```
