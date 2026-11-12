package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutomationRepository_ExitContactJourneysOnReply(t *testing.T) {
	// The regexes assert the load-bearing predicates (not just the table name): a mutation
	// dropping status='active', exit_on_reply=TRUE, deleted_at IS NULL, or the entered_at
	// bound must fail this test rather than passing on a substring match.
	before := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)

	// The journeys are exited and their automations' stats.exited raised in one statement:
	// the exits counted per automation, the automation rows locked in id order (no deadlock
	// with enrollment, which locks them too), stats guarded against a JSON null or scalar.
	const exitPredicates = `(?s)WITH exited AS \(\s*UPDATE contact_automations\s+SET status = 'exited', scheduled_at = NULL, exit_reason = \$1.*WHERE contact_email = \$2.*status = 'active'.*entered_at < \$3.*exit_on_reply = TRUE AND deleted_at IS NULL`
	const statsUpdate = `.*RETURNING automation_id.*COUNT\(\*\) AS n FROM exited GROUP BY automation_id.*ORDER BY a\.id FOR UPDATE OF a.*UPDATE automations a\s+SET stats = \(CASE WHEN jsonb_typeof\(a\.stats\) = 'object' THEN a\.stats ELSE '\{\}'::jsonb END\) \|\|\s*jsonb_build_object\('exited', COALESCE\(\(CASE WHEN jsonb_typeof\(a\.stats\) = 'object' THEN a\.stats->>'exited' END\)::int, 0\) \+ c\.n\),\s*updated_at = \$4.*RETURNING c\.n`

	t.Run("with automation id scopes to that automation and raises its exited count", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		autoID := "auto-1"
		mock.ExpectQuery(exitPredicates+`\s*\)\s*AND automation_id = \$5`+statsUpdate).
			WithArgs("replied", "jane@x.com", before, sqlmock.AnyArg(), autoID).
			WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))

		n, err := repo.ExitContactJourneysOnReply(context.Background(), "ws", "jane@x.com", &autoID, "replied", before)
		require.NoError(t, err)
		assert.Equal(t, 1, n)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("without automation id sums the exits of every automation", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(exitPredicates+statsUpdate).
			WithArgs("replied", "jane@x.com", before, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(2).AddRow(1))

		n, err := repo.ExitContactJourneysOnReply(context.Background(), "ws", "jane@x.com", nil, "replied", before)
		require.NoError(t, err)
		assert.Equal(t, 3, n)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an empty automation id is no filter", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		empty := ""
		mock.ExpectQuery(exitPredicates+statsUpdate).
			WithArgs("replied", "jane@x.com", before, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"n"}))

		n, err := repo.ExitContactJourneysOnReply(context.Background(), "ws", "jane@x.com", &empty, "replied", before)
		require.NoError(t, err)
		assert.Equal(t, 0, n)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no journey to exit returns zero", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(exitPredicates + statsUpdate).
			WillReturnRows(sqlmock.NewRows([]string{"n"}))

		n, err := repo.ExitContactJourneysOnReply(context.Background(), "ws", "jane@x.com", nil, "replied", before)
		require.NoError(t, err)
		assert.Equal(t, 0, n)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error is wrapped", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(exitPredicates + statsUpdate).WillReturnError(errors.New("boom"))

		_, err := repo.ExitContactJourneysOnReply(context.Background(), "ws", "jane@x.com", nil, "replied", before)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to exit contact journeys on reply")
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("row iteration error is returned", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(exitPredicates + statsUpdate).
			WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1).RowError(0, errors.New("conn reset")))

		_, err := repo.ExitContactJourneysOnReply(context.Background(), "ws", "jane@x.com", nil, "replied", before)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conn reset")
	})
}
