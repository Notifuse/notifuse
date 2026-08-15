package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
)

func TestFederatedIdentityRepository_GetByIssuerSubject(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()
	repo := NewFederatedIdentityRepository(db)

	t.Run("found", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "user_id", "idp_issuer", "idp_sub", "created_at"}).
			AddRow("fi-1", "user-1", "https://idp.example.com", "sub-123", time.Now().UTC())
		mock.ExpectQuery(`SELECT id, user_id, idp_issuer, idp_sub, created_at FROM federated_identities WHERE idp_issuer=\$1 AND idp_sub=\$2`).
			WithArgs("https://idp.example.com", "sub-123").
			WillReturnRows(rows)

		fi, err := repo.GetByIssuerSubject(context.Background(), "https://idp.example.com", "sub-123")
		require.NoError(t, err)
		assert.Equal(t, "user-1", fi.UserID)
		assert.Equal(t, "sub-123", fi.IDPSub)
	})

	t.Run("not found maps to typed error", func(t *testing.T) {
		mock.ExpectQuery(`SELECT id, user_id, idp_issuer, idp_sub, created_at FROM federated_identities WHERE idp_issuer=\$1 AND idp_sub=\$2`).
			WithArgs("https://idp.example.com", "missing").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByIssuerSubject(context.Background(), "https://idp.example.com", "missing")
		var notFound *domain.ErrFederatedIdentityNotFound
		assert.True(t, errors.As(err, &notFound), "expected *ErrFederatedIdentityNotFound, got %v", err)
	})
}

func TestFederatedIdentityRepository_GetByUserAndIssuer(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()
	repo := NewFederatedIdentityRepository(db)

	rows := sqlmock.NewRows([]string{"id", "user_id", "idp_issuer", "idp_sub", "created_at"}).
		AddRow("fi-1", "user-1", "https://idp.example.com", "sub-123", time.Now().UTC())
	mock.ExpectQuery(`SELECT id, user_id, idp_issuer, idp_sub, created_at FROM federated_identities WHERE user_id=\$1 AND idp_issuer=\$2`).
		WithArgs("user-1", "https://idp.example.com").
		WillReturnRows(rows)

	fi, err := repo.GetByUserAndIssuer(context.Background(), "user-1", "https://idp.example.com")
	require.NoError(t, err)
	assert.Equal(t, "sub-123", fi.IDPSub)
}

func TestFederatedIdentityRepository_Create(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()
	repo := NewFederatedIdentityRepository(db)

	t.Run("success auto-fills id", func(t *testing.T) {
		mock.ExpectExec(`INSERT INTO federated_identities \(id, user_id, idp_issuer, idp_sub, created_at\) VALUES \(\$1, \$2, \$3, \$4, \$5\)`).
			WithArgs(sqlmock.AnyArg(), "user-1", "https://idp.example.com", "sub-123", sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		fi := &domain.FederatedIdentity{UserID: "user-1", IDPIssuer: "https://idp.example.com", IDPSub: "sub-123"}
		err := repo.Create(context.Background(), fi)
		require.NoError(t, err)
		assert.NotEmpty(t, fi.ID, "Create should auto-fill the id")
	})

	t.Run("unique violation maps to ErrFederatedIdentityExists", func(t *testing.T) {
		mock.ExpectExec(`INSERT INTO federated_identities`).
			WithArgs(sqlmock.AnyArg(), "user-1", "https://idp.example.com", "sub-999", sqlmock.AnyArg()).
			WillReturnError(&pq.Error{
				Code:       "23505",
				Constraint: "federated_identities_user_id_idp_issuer_key",
				Message:    `duplicate key value violates unique constraint "federated_identities_user_id_idp_issuer_key"`,
			})

		fi := &domain.FederatedIdentity{UserID: "user-1", IDPIssuer: "https://idp.example.com", IDPSub: "sub-999"}
		err := repo.Create(context.Background(), fi)
		var exists *domain.ErrFederatedIdentityExists
		assert.True(t, errors.As(err, &exists), "expected *ErrFederatedIdentityExists, got %v", err)
	})

	// A server with lc_messages set to a non-English locale returns the same 23505 with
	// translated text (this is verbatim what PostgreSQL 17 emits under fr_FR). Detection
	// has to key off the SQLSTATE or OIDC reports auth_failed instead of link_conflict.
	t.Run("localized unique violation still maps to ErrFederatedIdentityExists", func(t *testing.T) {
		mock.ExpectExec(`INSERT INTO federated_identities`).
			WithArgs(sqlmock.AnyArg(), "user-1", "https://idp.example.com", "sub-fr", sqlmock.AnyArg()).
			WillReturnError(&pq.Error{
				Code:       "23505",
				Constraint: "federated_identities_idp_issuer_idp_sub_key",
				Message:    "la valeur d'une cl\u00e9 dupliqu\u00e9e rompt la contrainte unique \u00ab federated_identities_idp_issuer_idp_sub_key \u00bb",
			})

		fi := &domain.FederatedIdentity{UserID: "user-1", IDPIssuer: "https://idp.example.com", IDPSub: "sub-fr"}
		err := repo.Create(context.Background(), fi)
		var exists *domain.ErrFederatedIdentityExists
		assert.True(t, errors.As(err, &exists), "expected *ErrFederatedIdentityExists, got %v", err)
	})

	// The fall-through branch: an outage must stay an outage. Reporting it as a conflict
	// would tell the caller the link is taken when the database was merely unreachable.
	t.Run("non-unique-violation error is not reported as a conflict", func(t *testing.T) {
		mock.ExpectExec(`INSERT INTO federated_identities`).
			WithArgs(sqlmock.AnyArg(), "user-1", "https://idp.example.com", "sub-down", sqlmock.AnyArg()).
			WillReturnError(&pq.Error{Code: "53300", Message: "sorry, too many clients already"})

		fi := &domain.FederatedIdentity{UserID: "user-1", IDPIssuer: "https://idp.example.com", IDPSub: "sub-down"}
		err := repo.Create(context.Background(), fi)
		require.Error(t, err)
		var exists *domain.ErrFederatedIdentityExists
		assert.False(t, errors.As(err, &exists), "an outage must not be reported as an identity conflict")
		assert.Contains(t, err.Error(), "failed to create federated identity")
	})
}

func TestFederatedIdentityRepository_ListByUserID(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()
	repo := NewFederatedIdentityRepository(db)

	rows := sqlmock.NewRows([]string{"id", "user_id", "idp_issuer", "idp_sub", "created_at"}).
		AddRow("fi-1", "user-1", "https://idp-a.example.com", "sub-a", time.Now().UTC()).
		AddRow("fi-2", "user-1", "https://idp-b.example.com", "sub-b", time.Now().UTC())
	mock.ExpectQuery(`SELECT id, user_id, idp_issuer, idp_sub, created_at FROM federated_identities WHERE user_id=\$1 ORDER BY created_at`).
		WithArgs("user-1").
		WillReturnRows(rows)

	list, err := repo.ListByUserID(context.Background(), "user-1")
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestUserRepository_GetUserByEmailInsensitive(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()
	repo := NewUserRepository(db)

	t.Run("matches regardless of stored casing", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "email", "name", "type", "language", "created_at", "updated_at"}).
			AddRow("u-1", "Jane@Corp.com", "Jane", domain.UserTypeUser, "en", time.Now().UTC(), time.Now().UTC())
		mock.ExpectQuery(`SELECT id, email, name, type, language, created_at, updated_at FROM users WHERE lower\(email\)=lower\(\$1\)`).
			WithArgs("jane@corp.com").
			WillReturnRows(rows)

		u, err := repo.GetUserByEmailInsensitive(context.Background(), "jane@corp.com")
		require.NoError(t, err)
		assert.Equal(t, "Jane@Corp.com", u.Email)
	})

	t.Run("no row maps to ErrUserNotFound", func(t *testing.T) {
		mock.ExpectQuery(`SELECT id, email, name, type, language, created_at, updated_at FROM users WHERE lower\(email\)=lower\(\$1\)`).
			WithArgs("nobody@corp.com").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetUserByEmailInsensitive(context.Background(), "nobody@corp.com")
		var notFound *domain.ErrUserNotFound
		assert.True(t, errors.As(err, &notFound), "expected *ErrUserNotFound, got %v", err)
	})
}
