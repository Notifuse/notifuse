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
	assert.False(t, m.HasSystemUpdate())
	assert.True(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
	assert.NoError(t, m.UpdateSystem(context.Background(), &config.Config{}, nil))
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
// runs last, after the builds and the delete, so it never holds automations rows while waiting
// for the message_history or contact_segments lock. No column is renamed or
// dropped, so no automation trigger function needs regenerating. Without any own host, as
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
	schema.AutomationEnrollContactWithTriggerFunction(),
	schema.AutomationEnrollContactRelayFunction(),
}

// expectV44TriggerBodies expects the last step of the workspace update: the live automations
// with an installed trigger, whose function bodies are replaced. None here.
func expectV44TriggerBodies(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(v38HealQuery).WillReturnRows(v38HealRows())
}

func TestV44Migration_UpdateWorkspaceRunsExactlyItsStatementsInOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	for _, stmt := range v44ExpectedWorkspaceStatements {
		mock.ExpectExec("^" + regexp.QuoteMeta(stmt) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	expectV44TriggerBodies(mock)

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

// The last step replaces each live automation's trigger function, and nothing else: no DROP, no
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
