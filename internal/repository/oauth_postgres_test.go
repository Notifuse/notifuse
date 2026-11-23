package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
)

// Mock-backed tests: the statements' shape.

func TestOAuthRepository_ConsumptionIsOneConditionalUpdate(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()
	repo := NewOAuthRepository(db)
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`UPDATE oauth_authorization_requests SET consumed_at = $2
		WHERE id = $1 AND consumed_at IS NULL AND expires_at > $2
		RETURNING`)).WithArgs("req-1", now).WillReturnError(sql.ErrNoRows)
	_, err := repo.ConsumeRequest(context.Background(), "req-1", now)
	assert.ErrorIs(t, err, domain.ErrOAuthNotFound)

	mock.ExpectQuery(regexp.QuoteMeta(`UPDATE oauth_authorization_codes SET used_at = $2
		WHERE code_hash = $1 AND used_at IS NULL AND expires_at > $2
		RETURNING`)).WithArgs("hash", now).WillReturnError(sql.ErrNoRows)
	_, err = repo.UseCode(context.Background(), "hash", now)
	assert.ErrorIs(t, err, domain.ErrOAuthNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOAuthRepository_LockGrantReadsForUpdate(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()
	repo := NewOAuthRepository(db)

	mock.ExpectQuery(`FROM oauth_grants WHERE id = \$1 FOR UPDATE`).WithArgs("g1").WillReturnError(sql.ErrNoRows)
	_, err := repo.LockGrant(context.Background(), "g1")
	assert.ErrorIs(t, err, domain.ErrOAuthNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOAuthRepository_WithTx(t *testing.T) {
	t.Run("commits when fn succeeds", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		repo := NewOAuthRepository(db)
		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE oauth_tokens SET invalidated_at`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		require.NoError(t, repo.WithTx(context.Background(), func(tx domain.OAuthStore) error {
			return tx.InvalidateToken(context.Background(), "h", time.Now())
		}))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rolls back when fn fails", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		repo := NewOAuthRepository(db)
		mock.ExpectBegin()
		mock.ExpectRollback()
		sentinel := errors.New("boom")
		err := repo.WithTx(context.Background(), func(tx domain.OAuthStore) error { return sentinel })
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("nested runs in the same transaction", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()
		repo := NewOAuthRepository(db)
		mock.ExpectBegin()
		mock.ExpectCommit()
		require.NoError(t, repo.WithTx(context.Background(), func(tx domain.OAuthStore) error {
			return tx.(domain.OAuthRepository).WithTx(context.Background(), func(domain.OAuthStore) error { return nil })
		}))
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestOAuthRepository_CountGrantlessClientsExemptsHosts(t *testing.T) {
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()
	repo := NewOAuthRepository(db)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM oauth_clients c`).
		WithArgs(`{"https://claude.ai/%","https://my\\_app.example/%"}`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7))
	count, err := repo.CountGrantlessClients(context.Background(), []string{"claude.ai", "my_app.example"})
	require.NoError(t, err)
	assert.Equal(t, 7, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Real-database tests: the system schema installations run, the OAuth tables
// applied twice.

func setupOAuthSchemaDB(t *testing.T) *sql.DB {
	t.Helper()
	db := setupTelemetrySchemaDB(t)
	for round := 0; round < 2; round++ {
		for _, statement := range schema.OAuthTableDefinitions() {
			_, err := db.Exec(statement)
			require.NoErrorf(t, err, "round %d: %s", round, statement)
		}
	}
	return db
}

type oauthFixture struct {
	t    *testing.T
	db   *sql.DB
	repo domain.OAuthRepository
	ctx  context.Context
	now  time.Time
}

func newOAuthFixture(t *testing.T) *oauthFixture {
	db := setupOAuthSchemaDB(t)
	return &oauthFixture{t: t, db: db, repo: NewOAuthRepository(db), ctx: context.Background(), now: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)}
}

func (f *oauthFixture) user(kind domain.UserType) string {
	id := uuid.New().String()
	_, err := f.db.Exec(`INSERT INTO users (id, type, email, created_at, updated_at) VALUES ($1, $2, $3, NOW(), NOW())`, id, kind, id+"@example.com")
	require.NoError(f.t, err)
	return id
}

func (f *oauthFixture) workspace() string {
	id := "ws" + uuid.New().String()[:8]
	_, err := f.db.Exec(`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ($1, 'W', NOW(), NOW())`, id)
	require.NoError(f.t, err)
	return id
}

func (f *oauthFixture) grant(keyID, workspaceID, clientID string) *domain.OAuthGrant {
	grant := &domain.OAuthGrant{ID: uuid.New().String(), APIKeyUserID: keyID, WorkspaceID: workspaceID, ClientID: clientID, ClientName: "App", Resource: "https://n.example.com/mcp", CreatedVia: domain.OAuthGrantViaKey, CreatedAt: f.now}
	require.NoError(f.t, f.repo.CreateGrant(f.ctx, grant))
	return grant
}

func (f *oauthFixture) token(grantID, kind string, expiresAt time.Time) *domain.OAuthToken {
	token := &domain.OAuthToken{TokenHash: domain.HashOAuthSecret(uuid.New().String()), GrantID: grantID, PairID: uuid.New().String(), ClientID: "https://app.example.com/client", Kind: kind, ExpiresAt: expiresAt}
	require.NoError(f.t, f.repo.CreateTokens(f.ctx, token))
	return token
}

func (f *oauthFixture) code(grantID string, expiresAt time.Time) *domain.OAuthAuthorizationCode {
	code := &domain.OAuthAuthorizationCode{CodeHash: domain.HashOAuthSecret(uuid.New().String()), GrantID: grantID, ClientID: "https://app.example.com/client", RedirectURI: "https://app.example.com/cb", CodeChallenge: "c", Resource: "https://n.example.com/mcp", ExpiresAt: expiresAt}
	require.NoError(f.t, f.repo.CreateCode(f.ctx, code))
	return code
}

func (f *oauthFixture) request(expiresAt time.Time) *domain.OAuthAuthorizationRequest {
	request := &domain.OAuthAuthorizationRequest{
		ID: uuid.New().String(), ClientID: "https://app.example.com/client",
		ClientSnapshot: domain.OAuthClientSnapshot{ClientID: "https://app.example.com/client", ClientName: "App", RedirectURIs: []string{"https://app.example.com/cb"}},
		RedirectURI:    "https://app.example.com/cb", State: "s", CodeChallenge: "c", Resource: "https://n.example.com/mcp",
		BrowserBindingHash: domain.HashOAuthSecret("binding"), CreatedAt: f.now, ExpiresAt: expiresAt,
	}
	require.NoError(f.t, f.repo.CreateRequest(f.ctx, request))
	return request
}

func (f *oauthFixture) count(query string, args ...interface{}) int {
	var n int
	require.NoError(f.t, f.db.QueryRow(query, args...).Scan(&n))
	return n
}

func TestOAuthRepository_RealDB_CRUD(t *testing.T) {
	f := newOAuthFixture(t)
	keyID, workspaceID := f.user(domain.UserTypeAPIKey), f.workspace()

	t.Run("clients", func(t *testing.T) {
		expires := f.now.Add(time.Hour)
		client := &domain.OAuthClient{ClientID: "https://app.example.com/client", ClientName: "App", RedirectURIs: []string{"https://app.example.com/cb"}, Metadata: []byte(`{"client_name":"App"}`), CreatedAt: f.now, FetchedAt: &f.now, CacheExpiresAt: &expires}
		require.NoError(t, f.repo.SaveClient(f.ctx, client))
		got, err := f.repo.GetClient(f.ctx, client.ClientID)
		require.NoError(t, err)
		assert.Equal(t, "App", got.ClientName)
		assert.Equal(t, []string{"https://app.example.com/cb"}, got.RedirectURIs)
		assert.Nil(t, got.FetchFailedUntil)

		until := f.now.Add(5 * time.Minute)
		require.NoError(t, f.repo.MarkClientFetchFailed(f.ctx, client.ClientID, until, f.now))
		got, err = f.repo.GetClient(f.ctx, client.ClientID)
		require.NoError(t, err)
		assert.Equal(t, "App", got.ClientName, "a failure keeps the cached document")
		require.NotNil(t, got.FetchFailedUntil)
		assert.True(t, until.Equal(*got.FetchFailedUntil))

		require.NoError(t, f.repo.SaveClient(f.ctx, client))
		got, err = f.repo.GetClient(f.ctx, client.ClientID)
		require.NoError(t, err)
		assert.Nil(t, got.FetchFailedUntil, "a successful fetch clears the failure")

		require.NoError(t, f.repo.MarkClientFetchFailed(f.ctx, "https://new.example.com/client", until, f.now))
		got, err = f.repo.GetClient(f.ctx, "https://new.example.com/client")
		require.NoError(t, err)
		assert.Nil(t, got.FetchedAt)
		assert.Empty(t, got.RedirectURIs)

		_, err = f.repo.GetClient(f.ctx, "https://missing.example.com/client")
		assert.ErrorIs(t, err, domain.ErrOAuthNotFound)
	})

	t.Run("requests are consumed once and not after expiry", func(t *testing.T) {
		request := f.request(f.now.Add(10 * time.Minute))
		got, err := f.repo.GetRequest(f.ctx, request.ID)
		require.NoError(t, err)
		assert.Equal(t, request.ClientSnapshot, got.ClientSnapshot)
		assert.Equal(t, request.BrowserBindingHash, got.BrowserBindingHash)

		consumed, err := f.repo.ConsumeRequest(f.ctx, request.ID, f.now)
		require.NoError(t, err)
		require.NotNil(t, consumed.ConsumedAt)
		_, err = f.repo.ConsumeRequest(f.ctx, request.ID, f.now)
		assert.ErrorIs(t, err, domain.ErrOAuthNotFound)

		expiring := f.request(f.now.Add(10 * time.Minute))
		_, err = f.repo.ConsumeRequest(f.ctx, expiring.ID, f.now.Add(10*time.Minute))
		assert.ErrorIs(t, err, domain.ErrOAuthNotFound, "at its expiry a request is gone")
		_, err = f.repo.ConsumeRequest(f.ctx, expiring.ID, f.now.Add(10*time.Minute-time.Microsecond))
		assert.NoError(t, err)
	})

	t.Run("codes are used once and not after expiry", func(t *testing.T) {
		grant := f.grant(keyID, workspaceID, "https://app.example.com/client")
		code := f.code(grant.ID, f.now.Add(60*time.Second))
		_, err := f.repo.UseCode(f.ctx, code.CodeHash, f.now.Add(60*time.Second))
		assert.ErrorIs(t, err, domain.ErrOAuthNotFound)
		used, err := f.repo.UseCode(f.ctx, code.CodeHash, f.now)
		require.NoError(t, err)
		require.NotNil(t, used.UsedAt)
		_, err = f.repo.UseCode(f.ctx, code.CodeHash, f.now)
		assert.ErrorIs(t, err, domain.ErrOAuthNotFound)
		stored, err := f.repo.GetCode(f.ctx, code.CodeHash)
		require.NoError(t, err)
		assert.NotNil(t, stored.UsedAt)
	})

	t.Run("an access token resolves only while live", func(t *testing.T) {
		grant := f.grant(keyID, workspaceID, "https://app.example.com/client")
		access := f.token(grant.ID, domain.OAuthTokenKindAccess, f.now.Add(time.Hour))
		refresh := f.token(grant.ID, domain.OAuthTokenKindRefresh, f.now.Add(time.Hour))

		got, err := f.repo.GetAccessTokenGrant(f.ctx, access.TokenHash, f.now)
		require.NoError(t, err)
		assert.Equal(t, grant.ID, got.ID)
		_, err = f.repo.GetAccessTokenGrant(f.ctx, refresh.TokenHash, f.now)
		assert.ErrorIs(t, err, domain.ErrOAuthNotFound, "a refresh token is not an access token")
		_, err = f.repo.GetAccessTokenGrant(f.ctx, access.TokenHash, f.now.Add(time.Hour))
		assert.ErrorIs(t, err, domain.ErrOAuthNotFound, "expired at its expiry")

		require.NoError(t, f.repo.InvalidateToken(f.ctx, access.TokenHash, f.now))
		_, err = f.repo.GetAccessTokenGrant(f.ctx, access.TokenHash, f.now)
		assert.ErrorIs(t, err, domain.ErrOAuthNotFound, "invalidated")

		other := f.token(grant.ID, domain.OAuthTokenKindAccess, f.now.Add(time.Hour))
		revoked, err := f.repo.RevokeGrant(f.ctx, grant.ID, domain.OAuthRevokedByOwner, f.now)
		require.NoError(t, err)
		assert.True(t, revoked)
		revoked, err = f.repo.RevokeGrant(f.ctx, grant.ID, domain.OAuthRevokedByOwner, f.now)
		require.NoError(t, err)
		assert.False(t, revoked, "a revoked grant is not revoked again")
		_, err = f.repo.GetAccessTokenGrant(f.ctx, other.TokenHash, f.now)
		assert.ErrorIs(t, err, domain.ErrOAuthNotFound, "grant revoked")
	})

	t.Run("rotation, pairs and live access tokens", func(t *testing.T) {
		grant := f.grant(keyID, workspaceID, "https://app.example.com/client")
		old := f.token(grant.ID, domain.OAuthTokenKindRefresh, f.now.Add(time.Hour))
		require.NoError(t, f.repo.MarkTokenRotated(f.ctx, old.TokenHash, "next", f.now))
		require.NoError(t, f.repo.MarkTokenRotated(f.ctx, old.TokenHash, "later", f.now.Add(time.Minute)))
		got, err := f.repo.GetToken(f.ctx, old.TokenHash)
		require.NoError(t, err)
		assert.Equal(t, "next", got.ReplacedByHash, "rotated_at and its successor never move")
		assert.True(t, f.now.Equal(*got.RotatedAt))

		live := f.token(grant.ID, domain.OAuthTokenKindAccess, f.now.Add(time.Hour))
		expired := f.token(grant.ID, domain.OAuthTokenKindAccess, f.now)
		require.NoError(t, f.repo.InvalidateLiveAccessTokens(f.ctx, grant.ID, f.now))
		got, _ = f.repo.GetToken(f.ctx, live.TokenHash)
		assert.NotNil(t, got.InvalidatedAt)
		got, _ = f.repo.GetToken(f.ctx, expired.TokenHash)
		assert.Nil(t, got.InvalidatedAt, "an expired token is left alone")
		got, _ = f.repo.GetToken(f.ctx, old.TokenHash)
		assert.Nil(t, got.InvalidatedAt, "refresh tokens are not access tokens")

		pairA := &domain.OAuthToken{TokenHash: "a-access", GrantID: grant.ID, PairID: uuid.New().String(), ClientID: "c", Kind: domain.OAuthTokenKindAccess, ExpiresAt: f.now.Add(time.Hour)}
		pairR := &domain.OAuthToken{TokenHash: "a-refresh", GrantID: grant.ID, PairID: pairA.PairID, ClientID: "c", Kind: domain.OAuthTokenKindRefresh, ExpiresAt: f.now.Add(time.Hour)}
		require.NoError(t, f.repo.CreateTokens(f.ctx, pairA, pairR))
		require.NoError(t, f.repo.InvalidatePair(f.ctx, grant.ID, pairA.PairID, f.now))
		for _, hash := range []string{"a-access", "a-refresh"} {
			got, _ = f.repo.GetToken(f.ctx, hash)
			assert.NotNil(t, got.InvalidatedAt, hash)
		}
	})

	t.Run("list and last use", func(t *testing.T) {
		otherWorkspace := f.workspace()
		grant := f.grant(keyID, otherWorkspace, "https://app.example.com/client")
		items, err := f.repo.ListWorkspaceGrants(f.ctx, otherWorkspace)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, keyID+"@example.com", items[0].APIKeyEmail)
		assert.Equal(t, domain.OAuthGrantStatusActive, items[0].Status)

		require.NoError(t, f.repo.TouchGrant(f.ctx, grant.ID, f.now))
		require.NoError(t, f.repo.TouchGrant(f.ctx, grant.ID, f.now.Add(5*time.Minute-time.Second)))
		got, _ := f.repo.GetGrant(f.ctx, grant.ID)
		assert.True(t, f.now.Equal(*got.LastUsedAt), "written at most once per 5 minutes")
		require.NoError(t, f.repo.TouchGrant(f.ctx, grant.ID, f.now.Add(5*time.Minute)))
		got, _ = f.repo.GetGrant(f.ctx, grant.ID)
		assert.True(t, f.now.Add(5*time.Minute).Equal(*got.LastUsedAt))

		_, _ = f.repo.RevokeGrant(f.ctx, grant.ID, domain.OAuthRevokedByOwner, f.now)
		items, _ = f.repo.ListWorkspaceGrants(f.ctx, otherWorkspace)
		assert.Equal(t, domain.OAuthGrantStatusRevoked, items[0].Status)
		assert.Equal(t, domain.OAuthRevokedByOwner, items[0].RevokedReason)
	})
}

// A request and a code are consumed by exactly one of many concurrent calls.
func TestOAuthRepository_RealDB_AtomicConsumption(t *testing.T) {
	f := newOAuthFixture(t)
	request := f.request(f.now.Add(10 * time.Minute))
	grant := f.grant(f.user(domain.UserTypeAPIKey), f.workspace(), "c")
	code := f.code(grant.ID, f.now.Add(time.Minute))

	var wg sync.WaitGroup
	var mu sync.Mutex
	requests, codes := 0, 0
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := f.repo.ConsumeRequest(f.ctx, request.ID, f.now); err == nil {
				mu.Lock()
				requests++
				mu.Unlock()
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := f.repo.UseCode(f.ctx, code.CodeHash, f.now); err == nil {
				mu.Lock()
				codes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, 1, requests)
	assert.Equal(t, 1, codes)
}

// LockGrant serializes two transactions on one grant: the second waits for
// the first to commit.
func TestOAuthRepository_RealDB_GrantLock(t *testing.T) {
	f := newOAuthFixture(t)
	grant := f.grant(f.user(domain.UserTypeAPIKey), f.workspace(), "c")

	firstLocked := make(chan struct{})
	release := make(chan struct{})
	var order []string
	var mu sync.Mutex
	note := func(event string) {
		mu.Lock()
		order = append(order, event)
		mu.Unlock()
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		assert.NoError(t, f.repo.WithTx(f.ctx, func(tx domain.OAuthStore) error {
			if _, err := tx.LockGrant(f.ctx, grant.ID); err != nil {
				return err
			}
			close(firstLocked)
			<-release
			note("first commits")
			return nil
		}))
	}()
	go func() {
		defer wg.Done()
		<-firstLocked
		assert.NoError(t, f.repo.WithTx(f.ctx, func(tx domain.OAuthStore) error {
			if _, err := tx.LockGrant(f.ctx, grant.ID); err != nil {
				return err
			}
			note("second locks")
			return nil
		}))
	}()
	<-firstLocked
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	assert.Empty(t, order, "the second transaction must wait for the lock")
	mu.Unlock()
	close(release)
	wg.Wait()
	assert.Equal(t, []string{"first commits", "second locks"}, order)
}

// Deleting a key deletes its grants, codes and tokens; deleting a workspace
// deletes its grants; the owner who made a grant may leave.
func TestOAuthRepository_RealDB_Cascades(t *testing.T) {
	f := newOAuthFixture(t)

	keyID, workspaceID := f.user(domain.UserTypeAPIKey), f.workspace()
	grant := f.grant(keyID, workspaceID, "c")
	f.code(grant.ID, f.now.Add(time.Minute))
	f.token(grant.ID, domain.OAuthTokenKindAccess, f.now.Add(time.Hour))
	f.token(grant.ID, domain.OAuthTokenKindRefresh, f.now.Add(time.Hour))
	_, err := f.db.Exec(`DELETE FROM users WHERE id = $1`, keyID)
	require.NoError(t, err)
	assert.Zero(t, f.count(`SELECT COUNT(*) FROM oauth_grants WHERE id = $1`, grant.ID))
	assert.Zero(t, f.count(`SELECT COUNT(*) FROM oauth_authorization_codes WHERE grant_id = $1`, grant.ID))
	assert.Zero(t, f.count(`SELECT COUNT(*) FROM oauth_tokens WHERE grant_id = $1`, grant.ID))

	other := f.grant(f.user(domain.UserTypeAPIKey), workspaceID, "c")
	_, err = f.db.Exec(`DELETE FROM workspaces WHERE id = $1`, workspaceID)
	require.NoError(t, err)
	assert.Zero(t, f.count(`SELECT COUNT(*) FROM oauth_grants WHERE id = $1`, other.ID))

	owner := f.user(domain.UserTypeUser)
	shortcut := &domain.OAuthGrant{ID: uuid.New().String(), APIKeyUserID: f.user(domain.UserTypeAPIKey), WorkspaceID: f.workspace(), ClientID: "c", ClientName: "App", Resource: "r", CreatedVia: domain.OAuthGrantViaOwnerShortcut, CreatedByUserID: owner, CreatedAt: f.now}
	require.NoError(t, f.repo.CreateGrant(f.ctx, shortcut))
	_, err = f.db.Exec(`DELETE FROM users WHERE id = $1`, owner)
	require.NoError(t, err)
	got, err := f.repo.GetGrant(f.ctx, shortcut.ID)
	require.NoError(t, err)
	assert.Empty(t, got.CreatedByUserID)
}

// A key created with its grant is written in the grant's transaction: a later
// failure leaves no key.
func TestOAuthRepository_RealDB_KeyRollsBackWithItsGrant(t *testing.T) {
	f := newOAuthFixture(t)
	workspaceID := f.workspace()
	key := &domain.User{ID: uuid.New().String(), Email: "agent@n.example.com", Type: domain.UserTypeAPIKey}
	membership := &domain.UserWorkspace{UserID: key.ID, WorkspaceID: workspaceID, Role: "member", Permissions: domain.NewFullPermissions(), CreatedAt: f.now, UpdatedAt: f.now}

	err := f.repo.WithTx(f.ctx, func(tx domain.OAuthStore) error {
		if err := tx.CreateAPIKeyAccount(f.ctx, key, membership); err != nil {
			return err
		}
		_, err := tx.ConsumeRequest(f.ctx, "no-such-request", f.now)
		return err
	})
	require.ErrorIs(t, err, domain.ErrOAuthNotFound)
	assert.Zero(t, f.count(`SELECT COUNT(*) FROM users WHERE id = $1`, key.ID))
	assert.Zero(t, f.count(`SELECT COUNT(*) FROM user_workspaces WHERE user_id = $1`, key.ID))

	request := f.request(f.now.Add(time.Minute))
	require.NoError(t, f.repo.WithTx(f.ctx, func(tx domain.OAuthStore) error {
		if _, err := tx.ConsumeRequest(f.ctx, request.ID, f.now); err != nil {
			return err
		}
		return tx.CreateAPIKeyAccount(f.ctx, key, membership)
	}))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM users WHERE id = $1 AND type = 'api_key'`, key.ID))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM user_workspaces WHERE user_id = $1 AND role = 'member'`, key.ID))
}

func TestOAuthRepository_RealDB_RevokeGrantsWithoutCredential(t *testing.T) {
	f := newOAuthFixture(t)
	keyID, workspaceID := f.user(domain.UserTypeAPIKey), f.workspace()

	withRefresh := f.grant(keyID, workspaceID, "c")
	f.token(withRefresh.ID, domain.OAuthTokenKindRefresh, f.now.Add(time.Second))
	withCode := f.grant(keyID, workspaceID, "c")
	f.code(withCode.ID, f.now.Add(time.Second))

	expiredRefresh := f.grant(keyID, workspaceID, "c")
	f.token(expiredRefresh.ID, domain.OAuthTokenKindRefresh, f.now)
	rotatedOnly := f.grant(keyID, workspaceID, "c")
	rotated := f.token(rotatedOnly.ID, domain.OAuthTokenKindRefresh, f.now.Add(time.Hour))
	require.NoError(t, f.repo.MarkTokenRotated(f.ctx, rotated.TokenHash, "gone", f.now))
	invalidatedOnly := f.grant(keyID, workspaceID, "c")
	invalidated := f.token(invalidatedOnly.ID, domain.OAuthTokenKindRefresh, f.now.Add(time.Hour))
	require.NoError(t, f.repo.InvalidateToken(f.ctx, invalidated.TokenHash, f.now))
	accessOnly := f.grant(keyID, workspaceID, "c")
	f.token(accessOnly.ID, domain.OAuthTokenKindAccess, f.now.Add(time.Hour))
	usedCode := f.grant(keyID, workspaceID, "c")
	code := f.code(usedCode.ID, f.now.Add(time.Minute))
	_, err := f.repo.UseCode(f.ctx, code.CodeHash, f.now.Add(-time.Second))
	require.NoError(t, err)
	expiredCode := f.grant(keyID, workspaceID, "c")
	f.code(expiredCode.ID, f.now)

	revoked, err := f.repo.RevokeGrantsWithoutCredential(f.ctx, f.now)
	require.NoError(t, err)
	assert.Equal(t, int64(6), revoked)

	for grant, wantRevoked := range map[*domain.OAuthGrant]bool{
		withRefresh: false, withCode: false,
		expiredRefresh: true, rotatedOnly: true, invalidatedOnly: true, accessOnly: true, usedCode: true, expiredCode: true,
	} {
		got, err := f.repo.GetGrant(f.ctx, grant.ID)
		require.NoError(t, err)
		assert.Equal(t, wantRevoked, got.Revoked(), grant.ID)
		if wantRevoked {
			assert.Equal(t, domain.OAuthRevokedNoCredential, got.RevokedReason)
		}
	}
}

func TestOAuthRepository_RealDB_PurgeExpired(t *testing.T) {
	f := newOAuthFixture(t)
	grant := f.grant(f.user(domain.UserTypeAPIKey), f.workspace(), "c")
	expiredRequest, liveRequest := f.request(f.now), f.request(f.now.Add(time.Second))
	expiredCode, liveCode := f.code(grant.ID, f.now), f.code(grant.ID, f.now.Add(time.Second))
	expiredToken, liveToken := f.token(grant.ID, domain.OAuthTokenKindRefresh, f.now), f.token(grant.ID, domain.OAuthTokenKindAccess, f.now.Add(time.Second))

	requests, codes, tokens, err := f.repo.PurgeExpired(f.ctx, f.now)
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 1, 1}, []int64{requests, codes, tokens})

	_, err = f.repo.GetRequest(f.ctx, expiredRequest.ID)
	assert.ErrorIs(t, err, domain.ErrOAuthNotFound)
	_, err = f.repo.GetRequest(f.ctx, liveRequest.ID)
	assert.NoError(t, err)
	_, err = f.repo.GetCode(f.ctx, expiredCode.CodeHash)
	assert.ErrorIs(t, err, domain.ErrOAuthNotFound)
	_, err = f.repo.GetCode(f.ctx, liveCode.CodeHash)
	assert.NoError(t, err)
	_, err = f.repo.GetToken(f.ctx, expiredToken.TokenHash)
	assert.ErrorIs(t, err, domain.ErrOAuthNotFound)
	_, err = f.repo.GetToken(f.ctx, liveToken.TokenHash)
	assert.NoError(t, err)
}

func TestOAuthRepository_RealDB_PurgeClients(t *testing.T) {
	f := newOAuthFixture(t)
	keyID, workspaceID := f.user(domain.UserTypeAPIKey), f.workspace()
	save := func(clientID string, createdAt time.Time, lastUsed *time.Time) {
		expires := createdAt.Add(time.Hour)
		require.NoError(t, f.repo.SaveClient(f.ctx, &domain.OAuthClient{ClientID: clientID, ClientName: "App", RedirectURIs: []string{"https://x/cb"}, CreatedAt: createdAt, FetchedAt: &createdAt, CacheExpiresAt: &expires}))
		if lastUsed != nil {
			_, err := f.db.Exec(`UPDATE oauth_clients SET last_used_at = $2 WHERE client_id = $1`, clientID, *lastUsed)
			require.NoError(t, err)
		}
	}
	exists := func(clientID string) bool {
		_, err := f.repo.GetClient(f.ctx, clientID)
		return err == nil
	}

	// Failed fetches: purged once fetch_failed_until passes.
	require.NoError(t, f.repo.MarkClientFetchFailed(f.ctx, "https://failed-past.example/c", f.now, f.now.Add(-5*time.Minute)))
	require.NoError(t, f.repo.MarkClientFetchFailed(f.ctx, "https://failed-future.example/c", f.now.Add(time.Second), f.now.Add(-5*time.Minute)))
	// Fetched, never led to a grant: purged at 24 h.
	save("https://grantless-24h.example/c", f.now.Add(-24*time.Hour), nil)
	save("https://grantless-recent.example/c", f.now.Add(-24*time.Hour+time.Second), nil)
	// A live grant keeps its client, however old.
	save("https://live-grant.example/c", f.now.Add(-800*24*time.Hour), nil)
	f.grant(keyID, workspaceID, "https://live-grant.example/c")
	// A failed refetch of a client with a grant keeps it.
	save("https://failed-with-grant.example/c", f.now.Add(-48*time.Hour), nil)
	f.grant(keyID, workspaceID, "https://failed-with-grant.example/c")
	require.NoError(t, f.repo.MarkClientFetchFailed(f.ctx, "https://failed-with-grant.example/c", f.now.Add(-time.Second), f.now.Add(-time.Minute)))
	// Every grant revoked: purged 365 days after the last use.
	old := f.now.Add(-365 * 24 * time.Hour)
	save("https://revoked-365.example/c", f.now.Add(-800*24*time.Hour), &old)
	revokedGrant := f.grant(keyID, workspaceID, "https://revoked-365.example/c")
	_, _ = f.repo.RevokeGrant(f.ctx, revokedGrant.ID, domain.OAuthRevokedByOwner, old)
	recent := f.now.Add(-365*24*time.Hour + time.Second)
	save("https://revoked-recent.example/c", f.now.Add(-800*24*time.Hour), &recent)
	recentGrant := f.grant(keyID, workspaceID, "https://revoked-recent.example/c")
	_, _ = f.repo.RevokeGrant(f.ctx, recentGrant.ID, domain.OAuthRevokedByOwner, recent)

	count, err := f.repo.CountGrantlessClients(f.ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, 4, count, "the failed and the grantless records")

	purged, err := f.repo.PurgeClients(f.ctx, f.now)
	require.NoError(t, err)
	assert.Equal(t, int64(3), purged)
	for clientID, kept := range map[string]bool{
		"https://failed-past.example/c":       false,
		"https://failed-future.example/c":     true,
		"https://grantless-24h.example/c":     false,
		"https://grantless-recent.example/c":  true,
		"https://live-grant.example/c":        true,
		"https://failed-with-grant.example/c": true,
		"https://revoked-365.example/c":       false,
		"https://revoked-recent.example/c":    true,
	} {
		assert.Equal(t, kept, exists(clientID), clientID)
	}
}

// The count agrees with domain.IsKnownOAuthClient: a fetched record on a
// known host, any path, its host in any ASCII case, is outside the cap; a
// failed fetch, a look-alike host and an unknown host are inside.
func TestOAuthRepository_RealDB_CountGrantlessClients(t *testing.T) {
	f := newOAuthFixture(t)
	fetched := map[string]bool{
		"https://claude.ai/oauth/x":        true,
		"https://Claude.AI/arbitrary/path": true,
		"HTTPS://CHATGPT.COM/x":            true,
		"https://claude.a\u0130/x":         true,
		"https://claude.ai.evil.example/x": true,
		"https://other.example/x":          true,
		"https://claude.ai/failed":         false,
		"https://ChatGPT.com/failed":       false,
	}
	inside := 0
	for clientID, ok := range fetched {
		if ok {
			expires := f.now.Add(time.Hour)
			require.NoError(t, f.repo.SaveClient(f.ctx, &domain.OAuthClient{ClientID: clientID, ClientName: "App", RedirectURIs: []string{"https://app.example/cb"}, CreatedAt: f.now, FetchedAt: &f.now, CacheExpiresAt: &expires}))
		} else {
			require.NoError(t, f.repo.MarkClientFetchFailed(f.ctx, clientID, f.now, f.now))
		}
		if !ok || !domain.IsKnownOAuthClient(clientID) {
			inside++
		}
	}
	withGrant := "https://granted.example/x"
	require.NoError(t, f.repo.MarkClientFetchFailed(f.ctx, withGrant, f.now, f.now))
	f.grant(f.user(domain.UserTypeAPIKey), f.workspace(), withGrant)

	count, err := f.repo.CountGrantlessClients(f.ctx, domain.OAuthKnownClientHosts)
	require.NoError(t, err)
	assert.Equal(t, 5, count, "the look-alike, the unknown host and the three failed fetches")
	assert.Equal(t, inside, count, "the service's rule and the count agree")
}
