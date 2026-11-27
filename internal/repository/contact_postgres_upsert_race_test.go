package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A contact created by another call between UpsertContact's lookup and its insert makes the
// insert fail on contacts_pkey; UpsertContact then runs again instead of failing. Any other
// unique violation is returned as it is.
func TestUpsertContactRunsAgainWhenTheContactWasCreatedMeanwhile(t *testing.T) {
	setup := func(t *testing.T) (*contactRepository, sqlmock.Sqlmock) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		t.Cleanup(cleanup)
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil).AnyTimes()
		return NewContactRepository(workspaceRepo).(*contactRepository), mock
	}
	lookup := `SELECT ` + contactColumnsPattern + ` FROM contacts c WHERE c\.email = \$1 FOR UPDATE`

	t.Run("a contacts_pkey violation runs the upsert again", func(t *testing.T) {
		repo, mock := setup(t)
		mock.ExpectBegin()
		mock.ExpectQuery(lookup).WithArgs("new@example.com").WillReturnError(sql.ErrNoRows)
		mock.ExpectExec(`INSERT INTO contacts`).WillReturnError(&pq.Error{Code: "23505", Constraint: "contacts_pkey"})
		mock.ExpectRollback()
		mock.ExpectBegin()
		mock.ExpectQuery(lookup).WithArgs("new@example.com").WillReturnError(sql.ErrNoRows)
		mock.ExpectExec(`INSERT INTO contacts`).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		_, err := repo.UpsertContact(context.Background(), "ws", &domain.Contact{Email: "new@example.com"})
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("another unique violation is returned", func(t *testing.T) {
		repo, mock := setup(t)
		mock.ExpectBegin()
		mock.ExpectQuery(lookup).WithArgs("new@example.com").WillReturnError(sql.ErrNoRows)
		mock.ExpectExec(`INSERT INTO contacts`).WillReturnError(&pq.Error{Code: "23505", Constraint: "some_other_key"})
		mock.ExpectRollback()

		_, err := repo.UpsertContact(context.Background(), "ws", &domain.Contact{Email: "new@example.com"})
		require.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
