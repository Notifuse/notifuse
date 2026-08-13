package repository

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// The projection's statement shape and batching, which the integration tests
// cover end to end but cannot pin cheaply: how many transactions a run opens,
// what each carries, and the order they go in.
//
// The properties tested here are the ones that were bugs at some point in this
// file's short life — an unbounded transaction, a non-deterministic date order,
// a duplicate email aborting the re-queue — so they are worth holding still
// even though a live database would catch a broken statement first.

func projectionSession(id string, date time.Time, email string) *domain.WebSession {
	session := &domain.WebSession{SessionDate: date, ID: id}
	if email != "" {
		session.ContactEmail = &email
	}
	return session
}

// expectChunk queues one chunk's transaction: pages, session, re-queue — and
// binds the date and the exact session ids that chunk must carry.
//
// The arguments are the whole point. Matching only on the statement text made
// every expectation interchangeable, so the assertions read as if they pinned
// which sessions went in which transaction and in which date order while
// pinning neither: inverting the date sort, deleting it, and replacing
// ids[start:end] with ids all left the file green.
func expectChunk(mock sqlmock.Sqlmock, date time.Time, ids ...string) {
	expectChunkOwnedBy(mock, date, []string{"her@example.com"}, ids...)
}

// expectChunkOwnedBy is expectChunk with an explicit set of contacts to re-queue,
// for the identity-switch case where the previous owner joins them.
func expectChunkOwnedBy(mock sqlmock.Sqlmock, date time.Time, queued []string, ids ...string) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO contact_timeline")).
		WithArgs(date, pq.Array(ids), webNavigationMaxPagesPerSession).
		WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("her@example.com"))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO contact_timeline")).
		WithArgs(date, pq.Array(ids)).
		WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("her@example.com"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT DISTINCT email FROM contact_timeline")).
		WithArgs(pq.Array(ids)).
		WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("her@example.com"))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO contact_segment_queue")).
		WithArgs(pq.Array(queued)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func TestProjectContactNavigation(t *testing.T) {
	date := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()

	t.Run("one chunk per webNavigationSessionChunkSize sessions", func(t *testing.T) {
		// The chunk exists to bound how long one transaction holds locks: every
		// projected row is locked by ON CONFLICT DO UPDATE before its WHERE can
		// decline it, and each session carries up to the page cap. A run that
		// opened a single transaction for everything is what this pins against.
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		ids := make([]string, 0, webNavigationSessionChunkSize+1)
		sessions := make([]*domain.WebSession, 0, webNavigationSessionChunkSize+1)
		for i := 0; i <= webNavigationSessionChunkSize; i++ {
			// Distinct and valid: a collision would silently shrink the batch and
			// make the chunk count come out right for the wrong reason.
			id := fmt.Sprintf("11111111-1111-1111-1111-%012d", i)
			ids = append(ids, id)
			sessions = append(sessions, projectionSession(id, date, "her@example.com"))
		}
		sort.Strings(ids)
		require.Len(t, ids, webNavigationSessionChunkSize+1)

		// Membership, not just count: the chunk exists to bound how many rows one
		// transaction locks, so a chunk carrying every id would defeat it while
		// still opening the expected number of transactions.
		expectChunk(mock, date, ids[:webNavigationSessionChunkSize]...)
		expectChunk(mock, date, ids[webNavigationSessionChunkSize:]...)

		require.NoError(t, repo.ProjectContactNavigation(ctx, waTestWorkspace, sessions))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("one transaction per partition date, oldest first", func(t *testing.T) {
		// Dates come out of a map, and map order is randomised. Two replicas
		// walking a run that straddles UTC midnight in opposite date order take
		// contact_segment_queue's locks in opposite order and deadlock, so the
		// sort is load-bearing rather than cosmetic. sqlmock is ordered by
		// default, so queueing the older date's transaction first is the
		// assertion.
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		older := date.AddDate(0, 0, -1)
		// Each expectation is bound to its own date, so sqlmock's ordered matcher
		// fails on an argument mismatch if the dates come out newest-first.
		expectChunk(mock, older, "11111111-1111-1111-1111-111111111111")
		expectChunk(mock, date, "22222222-2222-2222-2222-222222222222")

		require.NoError(t, repo.ProjectContactNavigation(ctx, waTestWorkspace, []*domain.WebSession{
			projectionSession("22222222-2222-2222-2222-222222222222", date, "her@example.com"),
			projectionSession("11111111-1111-1111-1111-111111111111", older, "her@example.com"),
		}))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a session repeated across tabs is passed down once", func(t *testing.T) {
		// The buffer keys per (session, tab), so a multi-tab visit hands the same
		// session id over once per tab.
		//
		// This is an array-size and chunk-count optimisation, NOT a correctness
		// invariant — an earlier comment here claimed a duplicate would abort the
		// re-queue with "ON CONFLICT DO UPDATE command cannot affect row a second
		// time", which is wrong: session ids are only ever bound to `= ANY($2)`,
		// where duplicates are harmless. It is the EMAILS that reach unnest, and
		// they are deduped separately from the RETURNING results.
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		// One id in the bound array, not two.
		expectChunk(mock, date, "33333333-3333-3333-3333-333333333333")

		require.NoError(t, repo.ProjectContactNavigation(ctx, waTestWorkspace, []*domain.WebSession{
			projectionSession("33333333-3333-3333-3333-333333333333", date, "her@example.com"),
			projectionSession("33333333-3333-3333-3333-333333333333", date, "her@example.com"),
		}))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an identity switch re-queues the contact the rows moved away from", func(t *testing.T) {
		// A shared browser or a second login re-points a session at somebody else.
		// The upsert moves the rows and RETURNING names only the new owner, so
		// without this the previous contact keeps segment memberships derived
		// from rows they no longer have.
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		const id = "88888888-8888-8888-8888-888888888888"
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO contact_timeline")).
			WithArgs(date, pq.Array([]string{id}), webNavigationMaxPagesPerSession).
			WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("bob@example.com"))
		mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO contact_timeline")).
			WithArgs(date, pq.Array([]string{id})).
			WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("bob@example.com"))
		// The session row still names Alice at the moment it is read back.
		mock.ExpectQuery(regexp.QuoteMeta("SELECT DISTINCT email FROM contact_timeline")).
			WithArgs(pq.Array([]string{id})).
			WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("alice@example.com"))
		// Both, in collation order.
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO contact_segment_queue")).
			WithArgs(pq.Array([]string{"alice@example.com", "bob@example.com"})).
			WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectCommit()

		require.NoError(t, repo.ProjectContactNavigation(ctx, waTestWorkspace, []*domain.WebSession{
			projectionSession(id, date, "bob@example.com"),
		}))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no sessions means no connection is even taken", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		require.NoError(t, repo.ProjectContactNavigation(ctx, waTestWorkspace, nil))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an anonymous beat is still projected, and the statement decides", func(t *testing.T) {
		// Identity is NOT tested in Go: contact_email is sticky in the database,
		// so a visitor who logs out mid-visit keeps beating anonymously against
		// rows that are still identified. Filtering here would freeze that visit's
		// timeline at the last identified beat.
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		expectChunk(mock, date, "44444444-4444-4444-4444-444444444444")

		require.NoError(t, repo.ProjectContactNavigation(ctx, waTestWorkspace, []*domain.WebSession{
			projectionSession("44444444-4444-4444-4444-444444444444", date, ""),
		}))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("nothing changed means no re-queue", func(t *testing.T) {
		// A projection that rewrites nothing must cost no segment-queue traffic:
		// an unchanged row declines its DO UPDATE and returns no email, and every
		// actively-browsing visitor flushes about once a minute.
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO contact_timeline")).
			WithArgs(date, pq.Array([]string{"55555555-5555-5555-5555-555555555555"}), webNavigationMaxPagesPerSession).
			WillReturnRows(sqlmock.NewRows([]string{"email"}))
		mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO contact_timeline")).
			WithArgs(date, pq.Array([]string{"55555555-5555-5555-5555-555555555555"})).
			WillReturnRows(sqlmock.NewRows([]string{"email"}))
		// No ExpectExec for contact_segment_queue: sqlmock fails the test if one
		// is issued anyway.
		mock.ExpectCommit()

		require.NoError(t, repo.ProjectContactNavigation(ctx, waTestWorkspace, []*domain.WebSession{
			projectionSession("55555555-5555-5555-5555-555555555555", date, "her@example.com"),
		}))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a failed chunk does not abandon the rest of the run", func(t *testing.T) {
		// Each chunk commits on its own and the projection is idempotent, so a
		// deadlock or lock timeout on one says nothing about the next — and there
		// is no second chance for the ones skipped, because the buffer marks an
		// entry clean before the flush and only a new beat re-dirties it.
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		older := date.AddDate(0, 0, -1)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO contact_timeline")).
			WithArgs(older, pq.Array([]string{"66666666-6666-6666-6666-666666666666"}), webNavigationMaxPagesPerSession).
			WillReturnError(assertDeadlock{})
		mock.ExpectRollback()
		// The LATER date still runs, bound to its own id — so this pins which
		// chunk failed and which one carried on, not merely that two ran.
		expectChunk(mock, date, "77777777-7777-7777-7777-777777777777")

		err := repo.ProjectContactNavigation(ctx, waTestWorkspace, []*domain.WebSession{
			projectionSession("66666666-6666-6666-6666-666666666666", older, "her@example.com"),
			projectionSession("77777777-7777-7777-7777-777777777777", date, "her@example.com"),
		})
		require.Error(t, err, "the failure is still reported")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// assertDeadlock stands in for a transient database error.
type assertDeadlock struct{}

func (assertDeadlock) Error() string { return "deadlock detected" }
