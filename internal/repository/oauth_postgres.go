package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/Notifuse/notifuse/internal/domain"
)

// sqlQuerier is a *sql.DB or a *sql.Tx.
type sqlQuerier interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// oauthRepository stores the OAuth authorization server in the system
// database. Bound to the database, or, inside WithTx, to one transaction.
type oauthRepository struct {
	db *sql.DB
	q  sqlQuerier
	tx bool
}

// NewOAuthRepository creates the OAuth repository on the system database.
func NewOAuthRepository(db *sql.DB) domain.OAuthRepository {
	return &oauthRepository{db: db, q: db}
}

// WithTx runs fn in one transaction. Called on a repository already inside a
// transaction, it runs fn in that one.
func (r *oauthRepository) WithTx(ctx context.Context, fn func(tx domain.OAuthStore) error) error {
	if r.tx {
		return fn(r)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	if err := fn(&oauthRepository{db: r.db, q: tx, tx: true}); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func nullTimeArg(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}

func timePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	value := t.Time
	return &value
}

func nullStringArg(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// lastUsedThreshold is the last_used_at a row must be older than for a new
// write: last_used_at is written at most once per OAuthLastUsedInterval.
func lastUsedThreshold(now time.Time) time.Time {
	return now.Add(-domain.OAuthLastUsedInterval)
}

// Clients

const oauthClientColumns = `client_id, client_name, redirect_uris, metadata, created_at, last_used_at, fetched_at, cache_expires_at, fetch_failed_until`

func scanOAuthClient(row interface{ Scan(...interface{}) error }) (*domain.OAuthClient, error) {
	var client domain.OAuthClient
	var redirectURIs, metadata []byte
	var lastUsed, fetched, cacheExpires, failedUntil sql.NullTime
	if err := row.Scan(&client.ClientID, &client.ClientName, &redirectURIs, &metadata, &client.CreatedAt, &lastUsed, &fetched, &cacheExpires, &failedUntil); err != nil {
		return nil, err
	}
	if len(redirectURIs) > 0 {
		if err := json.Unmarshal(redirectURIs, &client.RedirectURIs); err != nil {
			return nil, fmt.Errorf("failed to decode redirect_uris: %w", err)
		}
	}
	client.Metadata = metadata
	client.LastUsedAt = timePtr(lastUsed)
	client.FetchedAt = timePtr(fetched)
	client.CacheExpiresAt = timePtr(cacheExpires)
	client.FetchFailedUntil = timePtr(failedUntil)
	return &client, nil
}

func (r *oauthRepository) GetClient(ctx context.Context, clientID string) (*domain.OAuthClient, error) {
	client, err := scanOAuthClient(r.q.QueryRowContext(ctx, `SELECT `+oauthClientColumns+` FROM oauth_clients WHERE client_id = $1`, clientID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrOAuthNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth client: %w", err)
	}
	return client, nil
}

func (r *oauthRepository) SaveClient(ctx context.Context, client *domain.OAuthClient) error {
	redirectURIs, err := json.Marshal(client.RedirectURIs)
	if err != nil {
		return fmt.Errorf("failed to encode redirect_uris: %w", err)
	}
	metadata := []byte(client.Metadata)
	if len(metadata) == 0 {
		metadata = []byte("{}")
	}
	_, err = r.q.ExecContext(ctx, `
		INSERT INTO oauth_clients (client_id, client_name, redirect_uris, metadata, created_at, fetched_at, cache_expires_at, fetch_failed_until)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULL)
		ON CONFLICT (client_id) DO UPDATE SET
			client_name = EXCLUDED.client_name,
			redirect_uris = EXCLUDED.redirect_uris,
			metadata = EXCLUDED.metadata,
			fetched_at = EXCLUDED.fetched_at,
			cache_expires_at = EXCLUDED.cache_expires_at,
			fetch_failed_until = NULL`,
		client.ClientID, client.ClientName, redirectURIs, metadata, client.CreatedAt, nullTimeArg(client.FetchedAt), nullTimeArg(client.CacheExpiresAt))
	if err != nil {
		return fmt.Errorf("failed to save oauth client: %w", err)
	}
	return nil
}

func (r *oauthRepository) MarkClientFetchFailed(ctx context.Context, clientID string, until, now time.Time) error {
	_, err := r.q.ExecContext(ctx, `
		INSERT INTO oauth_clients (client_id, created_at, fetch_failed_until)
		VALUES ($1, $2, $3)
		ON CONFLICT (client_id) DO UPDATE SET fetch_failed_until = EXCLUDED.fetch_failed_until`,
		clientID, now, until)
	if err != nil {
		return fmt.Errorf("failed to record the oauth client fetch failure: %w", err)
	}
	return nil
}

// likeEscape escapes the LIKE wildcards of a literal.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (r *oauthRepository) CountGrantlessClients(ctx context.Context, exemptHosts []string) (int, error) {
	patterns := make([]string, 0, len(exemptHosts))
	for _, host := range exemptHosts {
		patterns = append(patterns, "https://"+likeEscape(strings.ToLower(host))+"/%")
	}
	// translate folds the ASCII letters only, as domain.IsKnownOAuthClient
	// does: lower() would follow the collation and could fold other
	// characters into a known host's letters.
	var count int
	err := r.q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM oauth_clients c
		WHERE NOT EXISTS (SELECT 1 FROM oauth_grants g WHERE g.client_id = c.client_id)
		  AND NOT (c.fetch_failed_until IS NULL
		           AND translate(c.client_id, 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz') LIKE ANY ($1::text[]))`,
		pq.Array(patterns)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count oauth clients: %w", err)
	}
	return count, nil
}

func (r *oauthRepository) TouchClient(ctx context.Context, clientID string, now time.Time) error {
	_, err := r.q.ExecContext(ctx, `
		UPDATE oauth_clients SET last_used_at = $2
		WHERE client_id = $1 AND (last_used_at IS NULL OR last_used_at <= $3)`,
		clientID, now, lastUsedThreshold(now))
	if err != nil {
		return fmt.Errorf("failed to touch oauth client: %w", err)
	}
	return nil
}

// Requests

const oauthRequestColumns = `id, client_id, client_snapshot, redirect_uri, state, code_challenge, resource, browser_binding_hash, created_at, consumed_at, expires_at`

func scanOAuthRequest(row interface{ Scan(...interface{}) error }) (*domain.OAuthAuthorizationRequest, error) {
	var request domain.OAuthAuthorizationRequest
	var snapshot []byte
	var consumed sql.NullTime
	if err := row.Scan(&request.ID, &request.ClientID, &snapshot, &request.RedirectURI, &request.State, &request.CodeChallenge, &request.Resource, &request.BrowserBindingHash, &request.CreatedAt, &consumed, &request.ExpiresAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(snapshot, &request.ClientSnapshot); err != nil {
		return nil, fmt.Errorf("failed to decode client_snapshot: %w", err)
	}
	request.ConsumedAt = timePtr(consumed)
	return &request, nil
}

func (r *oauthRepository) CreateRequest(ctx context.Context, request *domain.OAuthAuthorizationRequest) error {
	snapshot, err := json.Marshal(request.ClientSnapshot)
	if err != nil {
		return fmt.Errorf("failed to encode client_snapshot: %w", err)
	}
	_, err = r.q.ExecContext(ctx, `
		INSERT INTO oauth_authorization_requests (`+oauthRequestColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULL, $10)`,
		request.ID, request.ClientID, snapshot, request.RedirectURI, request.State, request.CodeChallenge, request.Resource, request.BrowserBindingHash, request.CreatedAt, request.ExpiresAt)
	if err != nil {
		return fmt.Errorf("failed to create oauth authorization request: %w", err)
	}
	return nil
}

func (r *oauthRepository) GetRequest(ctx context.Context, id string) (*domain.OAuthAuthorizationRequest, error) {
	request, err := scanOAuthRequest(r.q.QueryRowContext(ctx, `SELECT `+oauthRequestColumns+` FROM oauth_authorization_requests WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrOAuthNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth authorization request: %w", err)
	}
	return request, nil
}

func (r *oauthRepository) ConsumeRequest(ctx context.Context, id string, now time.Time) (*domain.OAuthAuthorizationRequest, error) {
	request, err := scanOAuthRequest(r.q.QueryRowContext(ctx, `
		UPDATE oauth_authorization_requests SET consumed_at = $2
		WHERE id = $1 AND consumed_at IS NULL AND expires_at > $2
		RETURNING `+oauthRequestColumns, id, now))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrOAuthNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to consume oauth authorization request: %w", err)
	}
	return request, nil
}

// Grants

const oauthGrantColumns = `id, api_key_user_id, workspace_id, client_id, client_name, resource, created_via, created_by_user_id, created_at, last_used_at, revoked_at, revoked_reason`

func scanOAuthGrant(row interface{ Scan(...interface{}) error }, extra ...interface{}) (*domain.OAuthGrant, error) {
	var grant domain.OAuthGrant
	var createdBy, reason sql.NullString
	var lastUsed, revoked sql.NullTime
	dest := []interface{}{&grant.ID, &grant.APIKeyUserID, &grant.WorkspaceID, &grant.ClientID, &grant.ClientName, &grant.Resource, &grant.CreatedVia, &createdBy, &grant.CreatedAt, &lastUsed, &revoked, &reason}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return nil, err
	}
	grant.CreatedByUserID = createdBy.String
	grant.RevokedReason = reason.String
	grant.LastUsedAt = timePtr(lastUsed)
	grant.RevokedAt = timePtr(revoked)
	return &grant, nil
}

func (r *oauthRepository) CreateGrant(ctx context.Context, grant *domain.OAuthGrant) error {
	_, err := r.q.ExecContext(ctx, `
		INSERT INTO oauth_grants (`+oauthGrantColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULL, NULL, NULL)`,
		grant.ID, grant.APIKeyUserID, grant.WorkspaceID, grant.ClientID, grant.ClientName, grant.Resource, grant.CreatedVia, nullStringArg(grant.CreatedByUserID), grant.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create oauth grant: %w", err)
	}
	return nil
}

func (r *oauthRepository) getGrant(ctx context.Context, id, suffix string) (*domain.OAuthGrant, error) {
	grant, err := scanOAuthGrant(r.q.QueryRowContext(ctx, `SELECT `+oauthGrantColumns+` FROM oauth_grants WHERE id = $1`+suffix, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrOAuthNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth grant: %w", err)
	}
	return grant, nil
}

func (r *oauthRepository) GetGrant(ctx context.Context, id string) (*domain.OAuthGrant, error) {
	return r.getGrant(ctx, id, "")
}

func (r *oauthRepository) LockGrant(ctx context.Context, id string) (*domain.OAuthGrant, error) {
	return r.getGrant(ctx, id, " FOR UPDATE")
}

func (r *oauthRepository) RevokeGrant(ctx context.Context, id, reason string, now time.Time) (bool, error) {
	result, err := r.q.ExecContext(ctx, `
		UPDATE oauth_grants SET revoked_at = $3, revoked_reason = $2
		WHERE id = $1 AND revoked_at IS NULL`, id, reason, now)
	if err != nil {
		return false, fmt.Errorf("failed to revoke oauth grant: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read affected rows: %w", err)
	}
	return affected > 0, nil
}

func (r *oauthRepository) TouchGrant(ctx context.Context, id string, now time.Time) error {
	_, err := r.q.ExecContext(ctx, `
		UPDATE oauth_grants SET last_used_at = $2
		WHERE id = $1 AND (last_used_at IS NULL OR last_used_at <= $3)`,
		id, now, lastUsedThreshold(now))
	if err != nil {
		return fmt.Errorf("failed to touch oauth grant: %w", err)
	}
	return nil
}

func (r *oauthRepository) ListWorkspaceGrants(ctx context.Context, workspaceID string) ([]*domain.OAuthGrantListItem, error) {
	rows, err := r.q.QueryContext(ctx, `
		SELECT g.id, g.api_key_user_id, g.workspace_id, g.client_id, g.client_name, g.resource, g.created_via, g.created_by_user_id,
			g.created_at, g.last_used_at, g.revoked_at, g.revoked_reason, u.email
		FROM oauth_grants g
		JOIN users u ON u.id = g.api_key_user_id
		WHERE g.workspace_id = $1
		ORDER BY g.created_at DESC, g.id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list oauth grants: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := []*domain.OAuthGrantListItem{}
	for rows.Next() {
		var email string
		grant, err := scanOAuthGrant(rows, &email)
		if err != nil {
			return nil, fmt.Errorf("failed to scan oauth grant: %w", err)
		}
		status := domain.OAuthGrantStatusActive
		if grant.Revoked() {
			status = domain.OAuthGrantStatusRevoked
		}
		items = append(items, &domain.OAuthGrantListItem{OAuthGrant: *grant, APIKeyEmail: email, Status: status})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list oauth grants: %w", err)
	}
	return items, nil
}

// Codes

const oauthCodeColumns = `code_hash, grant_id, client_id, redirect_uri, code_challenge, resource, expires_at, used_at`

func scanOAuthCode(row interface{ Scan(...interface{}) error }) (*domain.OAuthAuthorizationCode, error) {
	var code domain.OAuthAuthorizationCode
	var used sql.NullTime
	if err := row.Scan(&code.CodeHash, &code.GrantID, &code.ClientID, &code.RedirectURI, &code.CodeChallenge, &code.Resource, &code.ExpiresAt, &used); err != nil {
		return nil, err
	}
	code.UsedAt = timePtr(used)
	return &code, nil
}

func (r *oauthRepository) CreateCode(ctx context.Context, code *domain.OAuthAuthorizationCode) error {
	_, err := r.q.ExecContext(ctx, `
		INSERT INTO oauth_authorization_codes (`+oauthCodeColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULL)`,
		code.CodeHash, code.GrantID, code.ClientID, code.RedirectURI, code.CodeChallenge, code.Resource, code.ExpiresAt)
	if err != nil {
		return fmt.Errorf("failed to create oauth code: %w", err)
	}
	return nil
}

func (r *oauthRepository) GetCode(ctx context.Context, codeHash string) (*domain.OAuthAuthorizationCode, error) {
	code, err := scanOAuthCode(r.q.QueryRowContext(ctx, `SELECT `+oauthCodeColumns+` FROM oauth_authorization_codes WHERE code_hash = $1`, codeHash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrOAuthNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth code: %w", err)
	}
	return code, nil
}

func (r *oauthRepository) UseCode(ctx context.Context, codeHash string, now time.Time) (*domain.OAuthAuthorizationCode, error) {
	code, err := scanOAuthCode(r.q.QueryRowContext(ctx, `
		UPDATE oauth_authorization_codes SET used_at = $2
		WHERE code_hash = $1 AND used_at IS NULL AND expires_at > $2
		RETURNING `+oauthCodeColumns, codeHash, now))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrOAuthNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to use oauth code: %w", err)
	}
	return code, nil
}

// Tokens

const oauthTokenColumns = `token_hash, grant_id, pair_id, client_id, kind, expires_at, rotated_at, replaced_by_hash, invalidated_at`

func (r *oauthRepository) CreateTokens(ctx context.Context, tokens ...*domain.OAuthToken) error {
	for _, token := range tokens {
		_, err := r.q.ExecContext(ctx, `
			INSERT INTO oauth_tokens (`+oauthTokenColumns+`)
			VALUES ($1, $2, $3, $4, $5, $6, NULL, NULL, NULL)`,
			token.TokenHash, token.GrantID, token.PairID, token.ClientID, token.Kind, token.ExpiresAt)
		if err != nil {
			return fmt.Errorf("failed to create oauth token: %w", err)
		}
	}
	return nil
}

func (r *oauthRepository) GetToken(ctx context.Context, tokenHash string) (*domain.OAuthToken, error) {
	var token domain.OAuthToken
	var rotated, invalidated sql.NullTime
	var replacedBy sql.NullString
	err := r.q.QueryRowContext(ctx, `SELECT `+oauthTokenColumns+` FROM oauth_tokens WHERE token_hash = $1`, tokenHash).
		Scan(&token.TokenHash, &token.GrantID, &token.PairID, &token.ClientID, &token.Kind, &token.ExpiresAt, &rotated, &replacedBy, &invalidated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrOAuthNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}
	token.RotatedAt = timePtr(rotated)
	token.InvalidatedAt = timePtr(invalidated)
	token.ReplacedByHash = replacedBy.String
	return &token, nil
}

func (r *oauthRepository) GetAccessTokenGrant(ctx context.Context, tokenHash string, now time.Time) (*domain.OAuthGrant, error) {
	grant, err := scanOAuthGrant(r.q.QueryRowContext(ctx, `
		SELECT g.id, g.api_key_user_id, g.workspace_id, g.client_id, g.client_name, g.resource, g.created_via, g.created_by_user_id,
			g.created_at, g.last_used_at, g.revoked_at, g.revoked_reason
		FROM oauth_tokens t
		JOIN oauth_grants g ON g.id = t.grant_id
		WHERE t.token_hash = $1 AND t.kind = '`+domain.OAuthTokenKindAccess+`'
		  AND t.invalidated_at IS NULL AND t.expires_at > $2
		  AND g.revoked_at IS NULL`, tokenHash, now))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrOAuthNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve oauth access token: %w", err)
	}
	return grant, nil
}

func (r *oauthRepository) MarkTokenRotated(ctx context.Context, tokenHash, replacedByHash string, now time.Time) error {
	_, err := r.q.ExecContext(ctx, `
		UPDATE oauth_tokens SET rotated_at = $3, replaced_by_hash = $2
		WHERE token_hash = $1 AND rotated_at IS NULL`, tokenHash, replacedByHash, now)
	if err != nil {
		return fmt.Errorf("failed to rotate oauth token: %w", err)
	}
	return nil
}

func (r *oauthRepository) InvalidateToken(ctx context.Context, tokenHash string, now time.Time) error {
	_, err := r.q.ExecContext(ctx, `
		UPDATE oauth_tokens SET invalidated_at = $2
		WHERE token_hash = $1 AND invalidated_at IS NULL`, tokenHash, now)
	if err != nil {
		return fmt.Errorf("failed to invalidate oauth token: %w", err)
	}
	return nil
}

func (r *oauthRepository) InvalidatePair(ctx context.Context, grantID, pairID string, now time.Time) error {
	_, err := r.q.ExecContext(ctx, `
		UPDATE oauth_tokens SET invalidated_at = $3
		WHERE grant_id = $1 AND pair_id = $2 AND invalidated_at IS NULL`, grantID, pairID, now)
	if err != nil {
		return fmt.Errorf("failed to invalidate oauth token pair: %w", err)
	}
	return nil
}

func (r *oauthRepository) InvalidateLiveAccessTokens(ctx context.Context, grantID string, now time.Time) error {
	_, err := r.q.ExecContext(ctx, `
		UPDATE oauth_tokens SET invalidated_at = $2
		WHERE grant_id = $1 AND kind = '`+domain.OAuthTokenKindAccess+`' AND invalidated_at IS NULL AND expires_at > $2`, grantID, now)
	if err != nil {
		return fmt.Errorf("failed to invalidate oauth access tokens: %w", err)
	}
	return nil
}

// API key created with its grant

func (r *oauthRepository) CreateAPIKeyAccount(ctx context.Context, key *domain.User, membership *domain.UserWorkspace) error {
	if err := insertUserRow(ctx, r.q, key); err != nil {
		return err
	}
	return upsertUserWorkspaceRow(ctx, r.q, membership)
}

// Periodic job

func (r *oauthRepository) RevokeGrantsWithoutCredential(ctx context.Context, now time.Time) (int64, error) {
	result, err := r.q.ExecContext(ctx, `
		UPDATE oauth_grants g SET revoked_at = $1, revoked_reason = '`+domain.OAuthRevokedNoCredential+`'
		WHERE g.revoked_at IS NULL
		  AND NOT EXISTS (
			SELECT 1 FROM oauth_tokens t
			WHERE t.grant_id = g.id AND t.kind = '`+domain.OAuthTokenKindRefresh+`'
			  AND t.rotated_at IS NULL AND t.invalidated_at IS NULL AND t.expires_at > $1
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM oauth_authorization_codes c
			WHERE c.grant_id = g.id AND c.used_at IS NULL AND c.expires_at > $1
		  )`, now)
	if err != nil {
		return 0, fmt.Errorf("failed to revoke oauth grants without credential: %w", err)
	}
	return result.RowsAffected()
}

func (r *oauthRepository) PurgeExpired(ctx context.Context, now time.Time) (requests, codes, tokens int64, err error) {
	purge := func(table string) (int64, error) {
		result, err := r.q.ExecContext(ctx, `DELETE FROM `+table+` WHERE expires_at <= $1`, now)
		if err != nil {
			return 0, fmt.Errorf("failed to purge %s: %w", table, err)
		}
		return result.RowsAffected()
	}
	if requests, err = purge("oauth_authorization_requests"); err != nil {
		return 0, 0, 0, err
	}
	if codes, err = purge("oauth_authorization_codes"); err != nil {
		return requests, 0, 0, err
	}
	if tokens, err = purge("oauth_tokens"); err != nil {
		return requests, codes, 0, err
	}
	return requests, codes, tokens, nil
}

func (r *oauthRepository) PurgeClients(ctx context.Context, now time.Time) (int64, error) {
	result, err := r.q.ExecContext(ctx, `
		DELETE FROM oauth_clients c
		WHERE (
			NOT EXISTS (SELECT 1 FROM oauth_grants g WHERE g.client_id = c.client_id)
			AND ((c.fetch_failed_until IS NOT NULL AND c.fetch_failed_until <= $1) OR c.created_at <= $2)
		) OR (
			EXISTS (SELECT 1 FROM oauth_grants g WHERE g.client_id = c.client_id)
			AND NOT EXISTS (SELECT 1 FROM oauth_grants g WHERE g.client_id = c.client_id AND g.revoked_at IS NULL)
			AND COALESCE(c.last_used_at, c.created_at) <= $3
		)`, now, now.Add(-domain.OAuthGrantlessClientRetention), now.Add(-domain.OAuthRevokedClientRetention))
	if err != nil {
		return 0, fmt.Errorf("failed to purge oauth clients: %w", err)
	}
	return result.RowsAffected()
}
