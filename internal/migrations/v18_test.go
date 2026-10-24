package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV18Migration_Metadata(t *testing.T) {
	migration := &V18Migration{}
	assert.Equal(t, 18.0, migration.GetMajorVersion())
	assert.False(t, migration.HasSystemUpdate(), "V18 only changes workspace databases")
	assert.True(t, migration.HasWorkspaceUpdate())
	assert.False(t, migration.ShouldRestartServer())
}

func TestV18Migration_UpdateSystem_Noop(t *testing.T) {
	migration := &V18Migration{}
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// No expectations: any SQL at all would fail, proving the system side is
	// a genuine no-op rather than a forgotten implementation.
	require.NoError(t, migration.UpdateSystem(context.Background(), &config.Config{}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

// v18Steps lists, in execution order, a distinctive substring of each
// statement UpdateWorkspace issues and the wrapper message its failure must
// surface with. The order matters: the migration is a sequence of dependent
// DDL steps, and running a later step after an early failure (e.g. dropping
// columns before the timeline functions are replaced) corrupts the schema.
var v18Steps = []struct {
	match string
	wrap  string
}{
	{"CREATE TABLE IF NOT EXISTS custom_events", "failed to create custom_events table"},
	{"ADD COLUMN IF NOT EXISTS goal_name", "failed to add missing columns to custom_events"},
	{"idx_custom_events_email", "failed to create custom_events indexes"},
	{"track_custom_event_timeline", "failed to create track_custom_event_timeline function"},
	{"custom_event_timeline_trigger", "failed to create custom_event_timeline_trigger"},
	{"track_contact_list_changes", "failed to update track_contact_list_changes function"},
	{"insert_contact_list", "failed to migrate contact_list timeline entries"},
	{"track_contact_segment_changes", "failed to update track_contact_segment_changes function"},
	{"join_segment", "failed to migrate segment timeline entries"},
	{"track_contact_changes", "failed to update track_contact_changes function"},
	{"insert_contact", "failed to migrate contact timeline entries"},
	{"DELETE FROM contact_segments", "failed to delete contact_segments for deprecated segments"},
	{"DELETE FROM segments", "failed to delete segments using deprecated fields"},
	{"DROP COLUMN IF EXISTS lifetime_value", "failed to drop deprecated contact columns"},
	{"REPLACE", "failed to rename table to source in segment trees"},
}

func TestV18Migration_UpdateWorkspace_FullSequence(t *testing.T) {
	migration := &V18Migration{}
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	for _, step := range v18Steps {
		mock.ExpectExec(step.match).WillReturnResult(sqlmock.NewResult(0, 0))
	}

	workspace := &domain.Workspace{ID: "ws-1", Name: "Test"}
	require.NoError(t, migration.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db))
	require.NoError(t, mock.ExpectationsWereMet(),
		"every DDL step must run exactly once, in order")
}

func TestV18Migration_UpdateWorkspace_StopsAtFirstFailure(t *testing.T) {
	// Failing each step in turn must abort the migration with that step's
	// wrapper. If execution continued, sqlmock would report an unexpected
	// query and the error would not match.
	for i, step := range v18Steps {
		t.Run(step.wrap, func(t *testing.T) {
			migration := &V18Migration{}
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			for _, prev := range v18Steps[:i] {
				mock.ExpectExec(prev.match).WillReturnResult(sqlmock.NewResult(0, 0))
			}
			sentinel := errors.New("ddl exploded")
			mock.ExpectExec(step.match).WillReturnError(sentinel)

			workspace := &domain.Workspace{ID: "ws-1", Name: "Test"}
			err = migration.UpdateWorkspace(context.Background(), &config.Config{}, workspace, db)
			require.ErrorContains(t, err, step.wrap+":")
			assert.ErrorIs(t, err, sentinel)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
