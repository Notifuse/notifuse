package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// setupAuditLogTest records every statement so assertions can be made about
// the SQL text — which WHERE clauses a filter produced — rather than only
// about the regexp an expectation matched.
func setupAuditLogTest(t *testing.T, seen *[]string) (*auditLogRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(
		func(expectedSQL, actualSQL string) error {
			*seen = append(*seen, actualSQL)
			return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
		})))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewAuditLogRepository(db).(*auditLogRepository)
	return repo, mock, db
}

func auditRows(n int, from time.Time) *sqlmock.Rows {
	rows := sqlmock.NewRows(auditLogColumns)
	for i := 0; i < n; i++ {
		rows.AddRow(
			"id"+string(rune('a'+i)), from.Add(-time.Duration(i)*time.Minute), "ws1", "templates.update", "templates", "success", 200,
			"user", "u1", "ann@example.com", "Ann", "member", "session",
			"template", "t1", "Welcome", "203.0.113.9", "curl", "req",
			[]byte(`{"name":{"old":"a","new":"b"}}`), []byte(`{"k":"v"}`),
		)
	}
	return rows
}

func TestAuditLogRepository_Insert(t *testing.T) {
	t.Run("writes every column, NULL for what is empty", func(t *testing.T) {
		var seen []string
		repo, mock, _ := setupAuditLogTest(t, &seen)

		at := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
		mock.ExpectExec("INSERT INTO audit_logs").WithArgs(
			"id1", at, "ws1", "templates.update", "templates", "success", 200,
			"user", "u1", "ann@example.com", nil, "member", "session",
			"template", "t1", nil, "203.0.113.9", nil, "req-1",
			[]byte(`{"name":{"old":"a","new":"b"}}`), nil,
		).WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Insert(context.Background(), &domain.AuditLog{
			ID: "id1", OccurredAt: at, WorkspaceID: "ws1", Action: "templates.update", Category: "templates",
			Outcome: "success", StatusCode: 200, ActorType: "user", ActorID: "u1", ActorEmail: "ann@example.com",
			ActorRole: "member", AuthMethod: "session", TargetType: "template", TargetID: "t1",
			IPAddress: "203.0.113.9", RequestID: "req-1",
			Changes: map[string]domain.AuditChange{"name": {Old: "a", New: "b"}},
		})
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
		assert.Contains(t, seen[0], strings.Join(auditLogColumns, ","))
	})

	t.Run("deployment-level rows have a NULL workspace and status", func(t *testing.T) {
		var seen []string
		repo, mock, _ := setupAuditLogTest(t, &seen)
		mock.ExpectExec("INSERT INTO audit_logs").WithArgs(
			"id2", sqlmock.AnyArg(), nil, "audit.purged", "audit", "success", nil,
			"system", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, []byte(`{"deleted":3}`),
		).WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.Insert(context.Background(), &domain.AuditLog{
			ID: "id2", OccurredAt: time.Now(), Action: "audit.purged", Category: "audit", Outcome: "success",
			ActorType: "system", Metadata: map[string]any{"deleted": 3},
		})
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("wraps the driver error", func(t *testing.T) {
		var seen []string
		repo, mock, _ := setupAuditLogTest(t, &seen)
		mock.ExpectExec("INSERT INTO audit_logs").WillReturnError(errors.New("boom"))
		err := repo.Insert(context.Background(), &domain.AuditLog{ID: "x", Action: "a", Category: "c", Outcome: "success", ActorType: "system"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to insert audit log")
	})
}

func TestAuditLogRepository_List(t *testing.T) {
	t.Run("every filter becomes a predicate", func(t *testing.T) {
		var seen []string
		repo, mock, _ := setupAuditLogTest(t, &seen)
		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := from.Add(24 * time.Hour)
		mock.ExpectQuery("SELECT .+ FROM audit_logs").WillReturnRows(auditRows(0, to))

		_, next, err := repo.List(context.Background(), domain.AuditLogFilter{
			Scope: domain.AuditScopeWorkspace, WorkspaceID: "ws1",
			Actions: []string{"a", "b"}, Categories: []string{"templates"},
			ActorID: "u1", ActorEmail: "ann", ActorType: "user", Outcome: "denied",
			TargetType: "template", TargetID: "t1", IPAddress: "203.0.113.9",
			From: &from, To: &to, Limit: 10,
		})
		require.NoError(t, err)
		assert.Empty(t, next)

		sqlText := seen[0]
		for _, fragment := range []string{
			"workspace_id = $", "action IN ($", "category IN ($", "actor_id = $", "actor_email ILIKE $",
			"actor_type = $", "outcome = $", "target_type = $", "target_id = $", "ip_address = $",
			"occurred_at >= $", "occurred_at <= $", "ORDER BY occurred_at DESC, id DESC", "LIMIT 11",
		} {
			assert.Contains(t, sqlText, fragment)
		}
	})

	t.Run("deployment scope has no workspace predicate unless asked", func(t *testing.T) {
		var seen []string
		repo, mock, _ := setupAuditLogTest(t, &seen)
		mock.ExpectQuery("SELECT .+ FROM audit_logs").WillReturnRows(auditRows(0, time.Now()))
		_, _, err := repo.List(context.Background(), domain.AuditLogFilter{Scope: domain.AuditScopeDeployment, Limit: 5})
		require.NoError(t, err)
		assert.NotContains(t, seen[0], "workspace_id = $")

		mock.ExpectQuery("SELECT .+ FROM audit_logs").WillReturnRows(auditRows(0, time.Now()))
		_, _, err = repo.List(context.Background(), domain.AuditLogFilter{Scope: domain.AuditScopeDeployment, WorkspaceID: "ws9", Limit: 5})
		require.NoError(t, err)
		assert.Contains(t, seen[1], "workspace_id = $")
	})

	t.Run("fetches limit+1 and hands back a cursor when there is more", func(t *testing.T) {
		var seen []string
		repo, mock, _ := setupAuditLogTest(t, &seen)
		newest := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
		mock.ExpectQuery("SELECT .+ FROM audit_logs").WillReturnRows(auditRows(3, newest))

		logs, next, err := repo.List(context.Background(), domain.AuditLogFilter{WorkspaceID: "ws1", Limit: 2})
		require.NoError(t, err)
		require.Len(t, logs, 2)
		require.NotEmpty(t, next)
		cursorAt, cursorID, err := domain.DecodeAuditCursor(next)
		require.NoError(t, err)
		assert.Equal(t, logs[1].ID, cursorID)
		assert.True(t, logs[1].OccurredAt.Equal(cursorAt))
		assert.Equal(t, "b", logs[0].Changes["name"].New)
		assert.Equal(t, "v", logs[0].Metadata["k"])
		assert.Equal(t, 200, logs[0].StatusCode)

		// The next page uses the keyset predicate.
		mock.ExpectQuery("SELECT .+ FROM audit_logs").WillReturnRows(auditRows(0, newest))
		_, _, err = repo.List(context.Background(), domain.AuditLogFilter{WorkspaceID: "ws1", Limit: 2, Cursor: next})
		require.NoError(t, err)
		assert.Regexp(t, regexp.MustCompile(`occurred_at < \$\d+ OR \(occurred_at = \$\d+ AND id < \$\d+\)`), seen[1])
	})

	t.Run("an undecodable cursor is refused before the query", func(t *testing.T) {
		var seen []string
		repo, _, _ := setupAuditLogTest(t, &seen)
		_, _, err := repo.List(context.Background(), domain.AuditLogFilter{WorkspaceID: "ws1", Cursor: "!!"})
		require.Error(t, err)
		assert.Empty(t, seen)
	})

	t.Run("an empty page is a non-nil slice", func(t *testing.T) {
		var seen []string
		repo, mock, _ := setupAuditLogTest(t, &seen)
		mock.ExpectQuery("SELECT .+ FROM audit_logs").WillReturnRows(auditRows(0, time.Now()))
		logs, _, err := repo.List(context.Background(), domain.AuditLogFilter{WorkspaceID: "ws1"})
		require.NoError(t, err)
		assert.NotNil(t, logs)
		assert.Empty(t, logs)
	})
}

func TestAuditLogRepository_GetByID(t *testing.T) {
	var seen []string
	repo, mock, _ := setupAuditLogTest(t, &seen)

	mock.ExpectQuery("SELECT .+ FROM audit_logs WHERE id = \\$1").WithArgs("ida").WillReturnRows(auditRows(1, time.Now()))
	log, err := repo.GetByID(context.Background(), "ida")
	require.NoError(t, err)
	assert.Equal(t, "ida", log.ID)

	mock.ExpectQuery("SELECT .+ FROM audit_logs WHERE id = \\$1").WithArgs("missing").WillReturnError(sql.ErrNoRows)
	_, err = repo.GetByID(context.Background(), "missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrAuditLogNotFound))
}

func TestAuditLogRepository_Stream(t *testing.T) {
	t.Run("walks every page until the last", func(t *testing.T) {
		var seen []string
		repo, mock, _ := setupAuditLogTest(t, &seen)
		newest := time.Now().UTC()
		// First page carries page+1 rows, so there is a second one.
		mock.ExpectQuery("SELECT .+ FROM audit_logs").WillReturnRows(auditRows(auditStreamPageSize+1, newest))
		mock.ExpectQuery("SELECT .+ FROM audit_logs").WillReturnRows(auditRows(1, newest))

		var count int
		err := repo.Stream(context.Background(), domain.AuditLogFilter{WorkspaceID: "ws1", Cursor: "ignored"}, func(*domain.AuditLog) error {
			count++
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, auditStreamPageSize+1, count)
		assert.Len(t, seen, 2)
		assert.Contains(t, seen[0], "LIMIT 1001")
		assert.NotContains(t, seen[0], "occurred_at <", "an export ignores the caller's cursor and starts at the top")
		assert.Contains(t, seen[1], "occurred_at <")
	})

	t.Run("the callback's error stops the walk and comes back unwrapped", func(t *testing.T) {
		var seen []string
		repo, mock, _ := setupAuditLogTest(t, &seen)
		mock.ExpectQuery("SELECT .+ FROM audit_logs").WillReturnRows(auditRows(3, time.Now()))
		stop := errors.New("stop")
		var count int
		err := repo.Stream(context.Background(), domain.AuditLogFilter{WorkspaceID: "ws1"}, func(*domain.AuditLog) error {
			count++
			return stop
		})
		assert.Same(t, stop, err)
		assert.Equal(t, 1, count)
	})
}

func TestAuditLogRepository_Purge(t *testing.T) {
	var seen []string
	repo, mock, _ := setupAuditLogTest(t, &seen)
	before := time.Date(2025, 9, 5, 0, 0, 0, 0, time.UTC)

	// The purge goes through the SQL function, never a bare DELETE: the guard
	// trigger would refuse one.
	mock.ExpectQuery(`SELECT audit_logs_purge\(\$1, \$2\)`).
		WithArgs(sql.NullString{String: "ws1", Valid: true}, before).
		WillReturnRows(sqlmock.NewRows([]string{"audit_logs_purge"}).AddRow(int64(7)))
	deleted, err := repo.Purge(context.Background(), ptr("ws1"), before)
	require.NoError(t, err)
	assert.Equal(t, int64(7), deleted)

	mock.ExpectQuery(`SELECT audit_logs_purge\(\$1, \$2\)`).
		WithArgs(sql.NullString{}, before).
		WillReturnRows(sqlmock.NewRows([]string{"audit_logs_purge"}).AddRow(int64(2)))
	deleted, err = repo.Purge(context.Background(), nil, before)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	mock.ExpectQuery(`SELECT audit_logs_purge_orphans\(\$1, \$2\)`).
		WithArgs(sqlmock.AnyArg(), before).
		WillReturnRows(sqlmock.NewRows([]string{"audit_logs_purge_orphans"}).AddRow(int64(1)))
	deleted, err = repo.PurgeOrphans(context.Background(), []string{"ws1", "ws2"}, before)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	for _, stmt := range seen {
		assert.NotContains(t, stmt, "DELETE")
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func ptr(s string) *string { return &s }

// Values that would overflow a column are cut to fit: a row with a truncated
// field beats a row that never made it, and an oversized value must not be a
// way to keep an action out of the log.
func TestAuditLogRepository_Insert_ClampsOversizedValues(t *testing.T) {
	var seen []string
	repo, mock, _ := setupAuditLogTest(t, &seen)
	longIP := strings.Repeat("9", 60)
	longTarget := strings.Repeat("t", 300)
	longWorkspace := strings.Repeat("w", 40)

	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(
		"id1", sqlmock.AnyArg(), strings.Repeat("w", auditIDWidth), "templates.update", "templates", "success", nil,
		"user", nil, nil, nil, nil, nil,
		"template", strings.Repeat("t", auditNameWidth), nil, strings.Repeat("9", auditIPWidth), nil, nil,
		nil, nil,
	).WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Insert(context.Background(), &domain.AuditLog{
		ID: "id1", OccurredAt: time.Now(), WorkspaceID: longWorkspace, Action: "templates.update", Category: "templates",
		Outcome: "success", ActorType: "user", TargetType: "template", TargetID: longTarget, IPAddress: longIP,
	})
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestClampRunes(t *testing.T) {
	assert.Equal(t, "abc", clampRunes("abc", 5))
	assert.Equal(t, "ab", clampRunes("abc", 2))
	assert.Equal(t, "éé", clampRunes("ééé", 2), "characters, not bytes")
	assert.Equal(t, "", clampRunes("", 2))
}
