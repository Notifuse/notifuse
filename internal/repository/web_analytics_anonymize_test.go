package repository

import (
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
)

// AnonymizeContact erases a deleted contact's email from every web-analytics
// table. Skipping a table (or stamping instead of NULLing) leaves the address
// behind permanently, because the ingest upsert's COALESCE can never clear it.

func TestWebAnalyticsRepository_AnonymizeContact(t *testing.T) {
	t.Run("blank email is a no-op (a blank match would anonymize the wrong rows)", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()
		for _, email := range []string{"", "   ", "\t\n"} {
			require.NoError(t, repo.AnonymizeContact(t.Context(), waTestWorkspace, email), "email %q", email)
		}
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("every web table is nulled for exactly that email", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()
		for _, table := range schema.WebAnalyticsTableNames {
			mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf("UPDATE %s SET contact_email = NULL WHERE contact_email = $1", table))).
				WithArgs("gone@example.com").
				WillReturnResult(sqlmock.NewResult(0, 2))
		}
		require.NoError(t, repo.AnonymizeContact(t.Context(), waTestWorkspace, "gone@example.com"))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a failure names its table and stops the sweep", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()
		require.GreaterOrEqual(t, len(schema.WebAnalyticsTableNames), 2,
			"this test needs a second table to prove the sweep stops")
		sentinel := errors.New("partition dropped")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE " + schema.WebAnalyticsTableNames[0])).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(regexp.QuoteMeta("UPDATE " + schema.WebAnalyticsTableNames[1])).
			WillReturnError(sentinel)

		err := repo.AnonymizeContact(t.Context(), waTestWorkspace, "gone@example.com")
		require.ErrorContains(t, err, "failed to anonymize "+schema.WebAnalyticsTableNames[1]+":")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet(),
			"tables after the failure must not be touched half-anonymized")
	})
}

func TestClampSmallintAndInt32(t *testing.T) {
	// Boundary-exact: an off-by-one either truncates legitimate values or
	// still overflows the column (32767 / 2147483647 are the column maxima).
	assert.Equal(t, 0, clampSmallint(-1))
	assert.Equal(t, 0, clampSmallint(0))
	assert.Equal(t, 123, clampSmallint(123))
	assert.Equal(t, 32767, clampSmallint(32767))
	assert.Equal(t, 32767, clampSmallint(32768))

	assert.Equal(t, int64(0), clampInt32(-5))
	assert.Equal(t, int64(0), clampInt32(0))
	assert.Equal(t, int64(999), clampInt32(999))
	assert.Equal(t, int64(2147483647), clampInt32(2147483647))
	assert.Equal(t, int64(2147483647), clampInt32(2147483648))
}
