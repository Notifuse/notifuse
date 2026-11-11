package migrations

import (
	"context"
	"errors"
	"regexp"
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

// The workspace update replaces the body of the timeline function and does nothing else: no
// trigger is dropped or created (that would lock message_history until commit), and no
// column is touched, so no automation trigger function needs regenerating.
func TestV44Migration_UpdateWorkspaceInstallsOnlyTheSharedTimelineFunction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("^" + regexp.QuoteMeta(schema.TrackMessageHistoryChangesFunction()) + "$").
		WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, (&V44Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestV44Migration_UpdateWorkspaceReturnsTheError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(schema.TrackMessageHistoryChangesFunction())).
		WillReturnError(errors.New("boom"))

	err = (&V44Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ws1")
	assert.Contains(t, err.Error(), "boom")
	require.NoError(t, mock.ExpectationsWereMet())
}
