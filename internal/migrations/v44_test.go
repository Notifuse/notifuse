package migrations

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

func TestV44Migration_Metadata(t *testing.T) {
	m := &V44Migration{}
	assert.Equal(t, 44.0, m.GetMajorVersion())
	assert.True(t, m.HasSystemUpdate(), "it reads the finished send tasks")
	assert.True(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
}

func TestV44Migration_IsRegistered(t *testing.T) {
	migration, ok := GetRegisteredMigration(44.0)
	require.True(t, ok)
	assert.IsType(t, &V44Migration{}, migration)
}

func TestV44Migration_IsWhatLetsTheStampReachFortyFour(t *testing.T) {
	var selected []float64
	for _, m := range GetRegisteredMigrations() {
		if v := m.GetMajorVersion(); v > 43.0 && v <= 44.0 {
			selected = append(selected, v)
		}
	}
	assert.Equal(t, []float64{44.0}, selected)
}

// The exact statements of the workspace update, in order. The first one replaces the body of
// the timeline function (no trigger is dropped or created). The two SET LOCAL statements let
// the index builds outlive a configured statement_timeout, and bound the wait for the table
// lock behind a transaction already writing to it. Before the builds, outside their lock, the
// backfill dates the broadcasts that started sending without a start date (a scheduled send
// never got one) from their first message, in one grouped pass; it only fills a NULL, so a
// rerun writes nothing. After the builds, the memberships whose contact no longer exists are
// deleted with the two membership triggers disabled, so no segment.left entry or webhook
// delivery carries an erased address, and both triggers are enabled again. The exited recount
// runs after the builds and the delete, so it never holds automations rows while waiting
// for the message_history or contact_segments lock. The enrolment with a VARCHAR p_list_id,
// left by v20 and v21 on workspaces created before v25, is dropped before the JSONB overload
// is installed, so the two never coexist. No column is renamed or
// dropped, so no trigger function needs regenerating for a schema change. Without any own host, as
// here, the purge of the own links is not run (see the purge test below).
var v44ExpectedWorkspaceStatements = []string{
	schema.TrackMessageHistoryChangesFunction(),
	`SET LOCAL statement_timeout = 0`,
	`SET LOCAL lock_timeout = '5s'`,
	`UPDATE broadcasts b SET started_at = m.first_at
FROM (SELECT mh.broadcast_id, MIN(mh.created_at) AS first_at
      FROM message_history mh
      WHERE mh.broadcast_id IN (SELECT id FROM broadcasts
                                WHERE started_at IS NULL AND status NOT IN ('draft', 'scheduled'))
      GROUP BY mh.broadcast_id) m
WHERE b.id = m.broadcast_id AND b.started_at IS NULL AND b.status NOT IN ('draft', 'scheduled')`,
	`CREATE INDEX IF NOT EXISTS idx_message_history_failed_recent ON message_history(created_at DESC, id DESC) WHERE failed_at IS NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_message_history_bounced_recent ON message_history(created_at DESC, id DESC) WHERE bounced_at IS NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_message_history_complained_recent ON message_history(created_at DESC, id DESC) WHERE complained_at IS NOT NULL`,
	`ALTER TABLE contact_segments DISABLE TRIGGER contact_segment_changes_trigger`,
	`ALTER TABLE contact_segments DISABLE TRIGGER webhook_contact_segments`,
	`DELETE FROM contact_segments cs
WHERE NOT EXISTS (SELECT 1 FROM contacts c WHERE c.email = cs.email)`,
	`ALTER TABLE contact_segments ENABLE TRIGGER webhook_contact_segments`,
	`ALTER TABLE contact_segments ENABLE TRIGGER contact_segment_changes_trigger`,
	`UPDATE automations a
SET stats = jsonb_set(COALESCE(a.stats, '{}'::jsonb), '{exited}', to_jsonb(c.n)),
    updated_at = NOW()
FROM (SELECT automation_id, COUNT(*) AS n FROM contact_automations
      WHERE status = 'exited' GROUP BY automation_id) c
WHERE c.automation_id = a.id
  AND a.deleted_at IS NULL
  AND (a.stats IS NULL OR jsonb_typeof(a.stats) = 'object')
  AND c.n > CASE WHEN a.stats->>'exited' ~ '^[0-9]{1,18}$'
                 THEN (a.stats->>'exited')::bigint ELSE 0 END`,
	`DROP FUNCTION IF EXISTS automation_enroll_contact(VARCHAR, VARCHAR, VARCHAR, VARCHAR, VARCHAR)`,
	schema.AutomationEnrollContactWithTriggerFunction(),
	schema.AutomationEnrollContactRelayFunction(),
}

// expectV44TriggerBodies expects the step before the column is added: the live automations
// with an installed trigger, whose function bodies are replaced. None here.
func expectV44TriggerBodies(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(v38HealQuery).WillReturnRows(v38HealRows())
}

// expectV44SegmentQueries expects the step between the trigger bodies and the column: every
// segment's query recompiled. None here.
func expectV44SegmentQueries(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(segmentSelect).WillReturnRows(sqlmock.NewRows([]string{"id", "tree", "generated_sql", "generated_args"}))
}

const v44SegmentLastErrorColumnsSQL = `ALTER TABLE segments ADD COLUMN IF NOT EXISTS last_error JSONB, ADD COLUMN IF NOT EXISTS last_error_at TIMESTAMP WITH TIME ZONE`

// The segment's last-error columns come right after the segment queries are recompiled, and
// before the column on broadcasts.
func expectV44SegmentLastErrorColumns(mock sqlmock.Sqlmock) {
	mock.ExpectExec("^" + regexp.QuoteMeta(v44SegmentLastErrorColumnsSQL) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
}

const v44TestPauseColumnSQL = `ALTER TABLE broadcasts ADD COLUMN IF NOT EXISTS test_paused_seconds INTEGER NOT NULL DEFAULT 0`

// The column comes last, after the trigger bodies: its ACCESS EXCLUSIVE lock on broadcasts is
// held until the workspace commits.
func expectV44TestPauseColumn(mock sqlmock.Sqlmock) {
	mock.ExpectExec("^" + regexp.QuoteMeta(v44TestPauseColumnSQL) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
}

func TestV44Migration_UpdateWorkspaceRunsExactlyItsStatementsInOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	for _, stmt := range v44ExpectedWorkspaceStatements {
		mock.ExpectExec("^" + regexp.QuoteMeta(stmt) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	expectV44TriggerBodies(mock)
	expectV44SegmentQueries(mock)
	expectV44SegmentLastErrorColumns(mock)
	expectV44TestPauseColumn(mock)

	require.NoError(t, (&V44Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

// The timeline step keeps its own message: the run-window test of the manager matches it.
func TestV44Migration_UpdateWorkspaceReturnsTheTimelineError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(schema.TrackMessageHistoryChangesFunction())).
		WillReturnError(errors.New("boom"))

	err = (&V44Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "link click timeline step")
	assert.Contains(t, err.Error(), "ws1")
	assert.Contains(t, err.Error(), "boom")
	require.NoError(t, mock.ExpectationsWereMet())
}

// An error on any later statement is returned with the workspace ID, and the statements
// after it are not run (sqlmock fails on an unexpected Exec).
func TestV44Migration_UpdateWorkspaceStopsOnTheFirstError(t *testing.T) {
	for failing := 1; failing < len(v44ExpectedWorkspaceStatements); failing++ {
		stmt := v44ExpectedWorkspaceStatements[failing]
		t.Run(stmt, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			for _, before := range v44ExpectedWorkspaceStatements[:failing] {
				mock.ExpectExec("^" + regexp.QuoteMeta(before) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
			}
			mock.ExpectExec("^" + regexp.QuoteMeta(stmt) + "$").WillReturnError(errors.New("boom"))

			err = (&V44Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "ws1")
			assert.Contains(t, err.Error(), "boom")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// The purge of the instance's own links runs after the three index builds, under their
// SHARE lock, and before the exited recount, with the hosts of the API endpoint and of the
// workspace's current custom endpoint.
// v44PurgeAt is where the purge runs among the other statements: after the index builds,
// before contact_segments is locked.
func v44PurgeAt(t *testing.T) int {
	t.Helper()
	for i, stmt := range v44ExpectedWorkspaceStatements {
		if strings.HasPrefix(stmt, "ALTER TABLE contact_segments DISABLE TRIGGER") {
			require.True(t, strings.HasPrefix(v44ExpectedWorkspaceStatements[i-1], "CREATE INDEX"), "the builds come right before")
			return i
		}
	}
	t.Fatal("no DISABLE TRIGGER statement")
	return 0
}

func TestV44Migration_UpdateWorkspacePurgesTheOwnLinksBetweenTheBuildsAndTheMembershipLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	at := v44PurgeAt(t)
	for _, stmt := range v44ExpectedWorkspaceStatements[:at] {
		mock.ExpectExec("^" + regexp.QuoteMeta(stmt) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec("^" + regexp.QuoteMeta(v44OwnLinksPurgeSQL) + "$").
		WithArgs(`{"api.notifuse.test","mail.brand.example"}`).
		WillReturnResult(sqlmock.NewResult(0, 3))
	for _, stmt := range v44ExpectedWorkspaceStatements[at:] {
		mock.ExpectExec("^" + regexp.QuoteMeta(stmt) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	expectV44TriggerBodies(mock)
	expectV44SegmentQueries(mock)
	expectV44SegmentLastErrorColumns(mock)
	expectV44TestPauseColumn(mock)

	endpoint := "https://Mail.Brand.example./"
	workspace := &domain.Workspace{ID: "ws1", Settings: domain.WorkspaceSettings{CustomEndpointURL: &endpoint}}
	cfg := &config.Config{APIEndpoint: "https://api.notifuse.test:8443"}
	require.NoError(t, (&V44Migration{}).UpdateWorkspace(context.Background(), cfg, workspace, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV44Migration_ThePurgeErrorNamesItsStep(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	for _, stmt := range v44ExpectedWorkspaceStatements[:v44PurgeAt(t)] {
		mock.ExpectExec("^" + regexp.QuoteMeta(stmt) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec("^" + regexp.QuoteMeta(v44OwnLinksPurgeSQL) + "$").WillReturnError(errors.New("boom"))

	err = (&V44Migration{}).UpdateWorkspace(context.Background(), &config.Config{APIEndpoint: "https://api.notifuse.test"}, &domain.Workspace{ID: "ws1"}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remove the instance's own links")
	assert.Contains(t, err.Error(), "ws1")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV44OwnHosts(t *testing.T) {
	endpoint := func(s string) *domain.Workspace {
		return &domain.Workspace{Settings: domain.WorkspaceSettings{CustomEndpointURL: &s}}
	}
	cases := map[string]struct {
		api       string
		workspace *domain.Workspace
		want      []string
	}{
		"none": {workspace: &domain.Workspace{}},
		"the API endpoint, port and case dropped":   {api: "https://API.Notifuse.test:8443/", workspace: &domain.Workspace{}, want: []string{"api.notifuse.test"}},
		"a scheme-less API endpoint":                {api: "api.notifuse.test", workspace: &domain.Workspace{}, want: []string{"api.notifuse.test"}},
		"both, a trailing dot dropped":              {api: "https://api.notifuse.test", workspace: endpoint("https://mail.brand.example."), want: []string{"api.notifuse.test", "mail.brand.example"}},
		"the same host once":                        {api: "https://mail.brand.example", workspace: endpoint("http://MAIL.brand.example:8080"), want: []string{"mail.brand.example"}},
		"an IPv6 literal, canonical and as written": {api: "http://[2001:0DB8::1]:8080", workspace: &domain.Workspace{}, want: []string{"2001:db8::1", "2001:0db8::1"}},
		"an empty custom endpoint":                  {api: "https://api.notifuse.test", workspace: endpoint(""), want: []string{"api.notifuse.test"}},
		"an unreadable host is dropped":             {workspace: endpoint("https://%FF.example"), want: nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, v44OwnHosts(&config.Config{APIEndpoint: tc.api}, tc.workspace))
		})
	}
}

// A backslash in a plain SQL literal is read as an escape when standard_conforming_strings
// is off, which would change what the purge's patterns match.
func TestV44OwnLinksPurgeSQL_HoldsNoBackslash(t *testing.T) {
	assert.NotContains(t, v44OwnLinksPurgeSQL, `\`)
}

func v44CustomEventTriggerConfig(t *testing.T) []byte {
	t.Helper()
	name := "cart_abandoned"
	b, err := json.Marshal(domain.TimelineTriggerConfig{
		EventKind:       "custom_event",
		CustomEventName: &name,
		Frequency:       domain.TriggerFrequencyEveryTime,
	})
	require.NoError(t, err)
	return b
}

// The step before the column replaces each live automation's trigger function, and nothing else: no DROP, no
// CREATE TRIGGER, so contact_timeline is not locked while the update holds message_history.
func TestV44Migration_ReplacesTriggerBodiesToPassTheEvent(t *testing.T) {
	rec := &v38StatementRecorder{}
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(rec))
	require.NoError(t, err)
	defer db.Close()

	for _, stmt := range v44ExpectedWorkspaceStatements {
		mock.ExpectExec("^" + regexp.QuoteMeta(stmt) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	installed := "CREATE TRIGGER automation_trigger_cartauto AFTER INSERT ON contact_timeline " +
		"FOR EACH ROW WHEN ((new.kind = 'custom_event.cart_abandoned'::text)) EXECUTE FUNCTION automation_trigger_cartauto()"
	mock.ExpectQuery(v38HealQuery).WillReturnRows(v38HealRows().AddRow("cartauto", "node1", v44CustomEventTriggerConfig(t), installed))
	mock.ExpectExec("^SAVEPOINT v44_automation_trigger_bodies$").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("^CREATE OR REPLACE FUNCTION automation_trigger_cartauto").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("^RELEASE SAVEPOINT v44_automation_trigger_bodies$").WillReturnResult(sqlmock.NewResult(0, 0))
	expectV44SegmentQueries(mock)
	expectV44SegmentLastErrorColumns(mock)
	expectV44TestPauseColumn(mock)

	require.NoError(t, (&V44Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())

	body := rec.firstWithPrefix(t, "CREATE OR REPLACE FUNCTION automation_trigger_cartauto")
	assert.Contains(t, body, "ce.event_name = substring(NEW.kind from 14)")
	// An update, soft delete or restore of the event no longer starts the automation.
	assert.Contains(t, body, "IF NEW.operation IS DISTINCT FROM 'insert' THEN")
	assert.NotContains(t, rec.all(), "DROP TRIGGER")
	assert.NotContains(t, rec.all(), "CREATE TRIGGER automation_trigger_cartauto")
}

// The five-argument enrolment is installed before any body calls it, and the four-argument one
// becomes its relay.
func TestV44Migration_InstallsTheEnrolmentBeforeTheBodies(t *testing.T) {
	n := len(v44ExpectedWorkspaceStatements)
	assert.Equal(t, schema.AutomationEnrollContactWithTriggerFunction(), v44ExpectedWorkspaceStatements[n-2])
	assert.Equal(t, schema.AutomationEnrollContactRelayFunction(), v44ExpectedWorkspaceStatements[n-1])
}

const v44ExpectedFinishedSendsSQL = `SELECT workspace_id, broadcast_id,
       to_char(COALESCE(completed_at, updated_at), 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
FROM tasks
WHERE type = 'send_broadcast' AND status = 'completed' AND broadcast_id IS NOT NULL
  AND COALESCE(state->'send_broadcast'->>'phase', '') = ''`

const v44ExpectedRepairSQL = `UPDATE broadcasts b
SET status = 'processed', completed_at = COALESCE(b.completed_at, d.done::timestamptz, NOW()), updated_at = NOW()
FROM unnest($1::text[], $2::text[]) AS d(id, done)
WHERE b.id = d.id AND b.status = 'scheduled'`

// The system update reads the send tasks that finished with no phase, the mark of a
// scheduled A/B broadcast sent before 44, and keeps them by workspace for the workspace
// update. A second read replaces the first.
func TestV44Migration_UpdateSystemReadsTheFinishedSends(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := &V44Migration{finishedSends: map[string][]v44FinishedSend{"stale": {{broadcastID: "old"}}}}
	mock.ExpectQuery("^" + regexp.QuoteMeta(v44ExpectedFinishedSendsSQL) + "$").WillReturnRows(
		sqlmock.NewRows([]string{"workspace_id", "broadcast_id", "done"}).
			AddRow("ws1", "b1", "2026-11-01T10:00:00.000000Z").
			AddRow("ws1", "b2", "2026-11-02T10:00:00.000000Z").
			AddRow("ws2", "b3", "2026-11-03T10:00:00.000000Z"))

	require.NoError(t, m.UpdateSystem(context.Background(), &config.Config{}, db))
	require.NoError(t, mock.ExpectationsWereMet())
	assert.Equal(t, map[string][]v44FinishedSend{
		"ws1": {{"b1", "2026-11-01T10:00:00.000000Z"}, {"b2", "2026-11-02T10:00:00.000000Z"}},
		"ws2": {{"b3", "2026-11-03T10:00:00.000000Z"}},
	}, m.finishedSends)
}

func TestV44Migration_UpdateSystemReturnsItsErrors(t *testing.T) {
	t.Run("the read", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		mock.ExpectQuery("^" + regexp.QuoteMeta(v44ExpectedFinishedSendsSQL) + "$").WillReturnError(errors.New("boom"))
		err = (&V44Migration{}).UpdateSystem(context.Background(), &config.Config{}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})
	t.Run("a row", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		mock.ExpectQuery("^" + regexp.QuoteMeta(v44ExpectedFinishedSendsSQL) + "$").WillReturnRows(
			sqlmock.NewRows([]string{"workspace_id", "broadcast_id", "done"}).AddRow("ws1", "b1", "x").RowError(0, errors.New("row boom")))
		err = (&V44Migration{}).UpdateSystem(context.Background(), &config.Config{}, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "row boom")
	})
}

// A workspace with finished sends marks its broadcasts still scheduled as processed, before
// the start dates are backfilled so that they get one too; the other statements follow as
// without it.
func TestV44Migration_UpdateWorkspaceRepairsTheFinishedScheduledSends(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	backfill := 3
	require.True(t, strings.HasPrefix(v44ExpectedWorkspaceStatements[backfill], "UPDATE broadcasts b SET started_at"))
	for _, stmt := range v44ExpectedWorkspaceStatements[:backfill] {
		mock.ExpectExec("^" + regexp.QuoteMeta(stmt) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec("^"+regexp.QuoteMeta(v44ExpectedRepairSQL)+"$").
		WithArgs(`{"b1","b2"}`, `{"2026-11-01T10:00:00.000000Z","2026-11-02T10:00:00.000000Z"}`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	for _, stmt := range v44ExpectedWorkspaceStatements[backfill:] {
		mock.ExpectExec("^" + regexp.QuoteMeta(stmt) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	expectV44TriggerBodies(mock)
	expectV44SegmentQueries(mock)
	expectV44SegmentLastErrorColumns(mock)
	expectV44TestPauseColumn(mock)

	m := &V44Migration{finishedSends: map[string][]v44FinishedSend{
		"ws1":   {{"b1", "2026-11-01T10:00:00.000000Z"}, {"b2", "2026-11-02T10:00:00.000000Z"}},
		"other": {{"b3", "2026-11-03T10:00:00.000000Z"}},
	}}
	require.NoError(t, m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV44Migration_TheRepairErrorNamesItsStep(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	for _, stmt := range v44ExpectedWorkspaceStatements[:3] {
		mock.ExpectExec("^" + regexp.QuoteMeta(stmt) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec("^" + regexp.QuoteMeta(v44ExpectedRepairSQL) + "$").WillReturnError(errors.New("boom"))

	m := &V44Migration{finishedSends: map[string][]v44FinishedSend{"ws1": {{"b1", "2026-11-01T10:00:00.000000Z"}}}}
	err = m.UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scheduled broadcasts whose send finished")
	assert.Contains(t, err.Error(), "boom")
}

func TestV44Migration_TheTestPauseColumnErrorNamesItsStep(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	for _, stmt := range v44ExpectedWorkspaceStatements {
		mock.ExpectExec("^" + regexp.QuoteMeta(stmt) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	expectV44TriggerBodies(mock)
	expectV44SegmentQueries(mock)
	expectV44SegmentLastErrorColumns(mock)
	mock.ExpectExec("^" + regexp.QuoteMeta(v44TestPauseColumnSQL) + "$").WillReturnError(errors.New("boom"))

	err = (&V44Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "test_paused_seconds")
	assert.Contains(t, err.Error(), "ws1")
	assert.Contains(t, err.Error(), "boom")
	require.NoError(t, mock.ExpectationsWereMet())
}

// The segment columns come after the recompilation of the segment queries, which reads segments
// without them, and an error there stops the update before the column on broadcasts.
func TestV44Migration_TheSegmentLastErrorColumnsErrorNamesItsStep(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	for _, stmt := range v44ExpectedWorkspaceStatements {
		mock.ExpectExec("^" + regexp.QuoteMeta(stmt) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	expectV44TriggerBodies(mock)
	expectV44SegmentQueries(mock)
	mock.ExpectExec("^" + regexp.QuoteMeta(v44SegmentLastErrorColumnsSQL) + "$").WillReturnError(errors.New("boom"))

	err = (&V44Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "segments.last_error")
	assert.Contains(t, err.Error(), "ws1")
	assert.Contains(t, err.Error(), "boom")
	require.NoError(t, mock.ExpectationsWereMet())
}

// A rerun, on a workspace whose segments already have the columns, adds nothing: both are added
// only if they do not exist.
func TestV44Migration_TheSegmentLastErrorColumnsAreIdempotent(t *testing.T) {
	assert.Equal(t, 2, strings.Count(v44SegmentLastErrorSQL, "ADD COLUMN IF NOT EXISTS"))
	assert.Equal(t, v44SegmentLastErrorColumnsSQL, v44SegmentLastErrorSQL)
}
