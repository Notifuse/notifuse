//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/pkg/totp"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// Two-factor authentication against a real PostgreSQL: the sign-in challenge, the counters
// stored on the account, the instance's requirement and the status codes a client reads.

func tfDecode(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body
}

// tfMagicCode asks for a sign-in code; the test server returns it.
func tfMagicCode(t *testing.T, client *testutil.APIClient, email string) string {
	t.Helper()
	resp, err := client.Post("/api/user.signin", map[string]string{"email": email})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	code, ok := tfDecode(t, resp)["code"].(string)
	require.True(t, ok, "the test server returns the code")
	return code
}

// tfVerify sends the magic code and returns the status and body.
func tfVerify(t *testing.T, client *testutil.APIClient, email, code string) (int, map[string]interface{}) {
	t.Helper()
	resp, err := client.Post("/api/user.verify", map[string]string{"email": email, "code": code})
	require.NoError(t, err)
	status := resp.StatusCode
	return status, tfDecode(t, resp)
}

// tfSignIn signs in with a magic code and returns the session token. The account must have no
// authenticator.
func tfSignIn(t *testing.T, client *testutil.APIClient, email string) string {
	t.Helper()
	status, body := tfVerify(t, client, email, tfMagicCode(t, client, email))
	require.Equal(t, http.StatusOK, status, body)
	token, ok := body["token"].(string)
	require.True(t, ok, body)
	return token
}

// tfEnroll sets up an authenticator app on the signed-in session and returns its secret and
// the recovery codes.
func tfEnroll(t *testing.T, client *testutil.APIClient) ([]byte, []string) {
	t.Helper()
	resp, err := client.Post("/api/user.twoFactor.setup", map[string]string{})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	setup := tfDecode(t, resp)
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(setup["secret"].(string))
	require.NoError(t, err)

	resp, err = client.Post("/api/user.twoFactor.enable", map[string]interface{}{
		"code":                    totp.Code(secret, totp.Step(time.Now())),
		"sign_out_other_sessions": false,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var codes []string
	for _, c := range tfDecode(t, resp)["recovery_codes"].([]interface{}) {
		codes = append(codes, c.(string))
	}
	require.Len(t, codes, domain.RecoveryCodeCount)
	return secret, codes
}

// tfChallenge signs in by magic code an account that has an authenticator, and returns the
// challenge token.
func tfChallenge(t *testing.T, client *testutil.APIClient, email string) string {
	t.Helper()
	status, body := tfVerify(t, client, email, tfMagicCode(t, client, email))
	require.Equal(t, http.StatusOK, status, body)
	require.Equal(t, true, body["two_factor_required"], body)
	require.NotContains(t, body, "token")
	return body["challenge_token"].(string)
}

func tfSecondFactor(t *testing.T, client *testutil.APIClient, challenge string, proof map[string]string) (int, map[string]interface{}) {
	t.Helper()
	body := map[string]string{"challenge_token": challenge}
	for k, v := range proof {
		body[k] = v
	}
	resp, err := client.Post("/api/user.twoFactor.verify", body)
	require.NoError(t, err)
	status := resp.StatusCode
	return status, tfDecode(t, resp)
}

func TestTwoFactorAuthentication(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()
	client := suite.APIClient
	db := suite.DBManager.GetDB()

	t.Run("enrol, sign in with a challenge, then with a recovery code", func(t *testing.T) {
		const email = "twofactor-ann@example.com"
		client.SetToken(tfSignIn(t, client, email))
		secret, codes := tfEnroll(t, client)

		var stored, enabled int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM user_recovery_codes WHERE user_id = '550e8400-e29b-41d4-a716-446655440021'`).Scan(&stored))
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM user_totp WHERE user_id = '550e8400-e29b-41d4-a716-446655440021' AND enabled_at IS NOT NULL`).Scan(&enabled))
		assert.Equal(t, domain.RecoveryCodeCount, stored)
		assert.Equal(t, 1, enabled)

		client.SetToken("")
		challenge := tfChallenge(t, client, email)

		// A wrong code is a 422 and is counted on the account.
		status, body := tfSecondFactor(t, client, challenge, map[string]string{"code": "000000"})
		assert.Equal(t, http.StatusUnprocessableEntity, status, body)
		var failures int
		require.NoError(t, db.QueryRow(`SELECT failed_attempts FROM user_totp WHERE user_id = '550e8400-e29b-41d4-a716-446655440021'`).Scan(&failures))
		assert.Equal(t, 1, failures)

		// Enabling used this step's code, which is now refused as a replay. Forget that step,
		// as the next step would, so this step's code is accepted.
		_, err := db.Exec(`UPDATE user_totp SET last_used_step = 0 WHERE user_id = '550e8400-e29b-41d4-a716-446655440021'`)
		require.NoError(t, err)
		status, body = tfSecondFactor(t, client, challenge, map[string]string{"code": totp.Code(secret, totp.Step(time.Now()))})
		require.Equal(t, http.StatusOK, status, body)
		token := body["token"].(string)
		require.NotEmpty(t, token)

		// The session passed the second factor; the counter is back to zero.
		client.SetToken(token)
		me, err := client.Get("/api/user.me")
		require.NoError(t, err)
		meBody := tfDecode(t, me)
		assert.Equal(t, "totp", meBody["session_second_factor"])
		assert.Equal(t, true, meBody["two_factor"].(map[string]interface{})["enabled"])
		require.NoError(t, db.QueryRow(`SELECT failed_attempts FROM user_totp WHERE user_id = '550e8400-e29b-41d4-a716-446655440021'`).Scan(&failures))
		assert.Zero(t, failures)

		// A recovery code signs in once.
		client.SetToken("")
		challenge = tfChallenge(t, client, email)
		status, body = tfSecondFactor(t, client, challenge, map[string]string{"recovery_code": codes[0]})
		require.Equal(t, http.StatusOK, status, body)
		challenge = tfChallenge(t, client, email)
		status, _ = tfSecondFactor(t, client, challenge, map[string]string{"recovery_code": codes[0]})
		assert.Equal(t, http.StatusUnprocessableEntity, status, "a recovery code works once")
	})

	// The magic code is consumed in the transaction that issues the challenge: two requests
	// sending it at once get one challenge between them.
	t.Run("the same magic code sent twice at once opens one session", func(t *testing.T) {
		const email = "twofactor-bea@example.com"
		code := tfMagicCode(t, client, email)
		var wg sync.WaitGroup
		statuses := make([]int, 2)
		for i := range statuses {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				c := testutil.NewAPIClient(suite.ServerManager.GetURL())
				resp, err := c.Post("/api/user.verify", map[string]string{"email": email, "code": code})
				if err == nil {
					statuses[i] = resp.StatusCode
					_ = resp.Body.Close()
				}
			}(i)
		}
		wg.Wait()
		assert.ElementsMatch(t, []int{http.StatusOK, http.StatusUnauthorized}, statuses)
	})

	// Five codes against one challenge, sent in parallel: each is counted, under the user's
	// lock, and the challenge is then used up.
	t.Run("a challenge takes five attempts, however they arrive", func(t *testing.T) {
		const email = "twofactor-cy@example.com"
		client.SetToken(tfSignIn(t, client, email))
		tfEnroll(t, client)
		client.SetToken("")
		challenge := tfChallenge(t, client, email)

		var wg sync.WaitGroup
		statuses := make([]int, domain.MaxChallengeAttempts)
		for i := range statuses {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				c := testutil.NewAPIClient(suite.ServerManager.GetURL())
				resp, err := c.Post("/api/user.twoFactor.verify", map[string]string{"challenge_token": challenge, "code": fmt.Sprintf("00000%d", i)})
				if err == nil {
					statuses[i] = resp.StatusCode
					_ = resp.Body.Close()
				}
			}(i)
		}
		wg.Wait()
		for _, s := range statuses {
			assert.Equal(t, http.StatusUnprocessableEntity, s)
		}
		var attempts, failures int
		require.NoError(t, db.QueryRow(`SELECT challenge_attempts FROM user_sessions WHERE user_id = '550e8400-e29b-41d4-a716-446655440023' AND challenge_hash IS NOT NULL`).Scan(&attempts))
		require.NoError(t, db.QueryRow(`SELECT failed_attempts FROM user_totp WHERE user_id = '550e8400-e29b-41d4-a716-446655440023'`).Scan(&failures))
		assert.Equal(t, domain.MaxChallengeAttempts, attempts)
		assert.Equal(t, domain.MaxChallengeAttempts, failures)

		status, _ := tfSecondFactor(t, client, challenge, map[string]string{"code": "123456"})
		assert.Equal(t, http.StatusGone, status, "the challenge is used up")
	})

	t.Run("user.logout and user.updateLanguage", func(t *testing.T) {
		const email = "twofactor-dan@example.com"
		token := tfSignIn(t, client, email)

		// After signing out, the session is gone: 401 on the next call.
		client.SetToken(token)
		resp, err := client.Post("/api/user.logout", map[string]string{})
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		resp, err = client.Post("/api/user.updateLanguage", map[string]string{"language": "fr"})
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// The instance requires two-factor authentication: a session that has not passed it reaches
// its account and the setup routes, and nothing else, until it does.
func TestTwoFactorAuthentication_InstanceRequirement(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		cfg.RequireTwoFactor = true
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()
	client := suite.APIClient
	const email = "testuser@example.com" // owner of test-workspace-id

	// Two sessions of the same person: the second turns two-factor authentication on and
	// keeps the first one.
	older := testutil.NewAPIClient(suite.ServerManager.GetURL())
	older.SetToken(tfSignIn(t, older, email))
	client.SetToken(tfSignIn(t, client, email))

	// user.me: no workspace (a console session's workspace carries the S3 secret).
	resp, err := client.Get("/api/user.me")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	me := tfDecode(t, resp)
	assert.Equal(t, true, me["two_factor_required"])
	assert.Empty(t, me["workspaces"])

	// Everything else: 403 two_factor_required, never 401.
	for _, call := range []struct{ method, route string }{
		{http.MethodGet, "/api/workspaces.list"},
		{http.MethodGet, "/api/contacts.list?workspace_id=test-workspace-id"},
		{http.MethodGet, "/api/timeline.list?workspace_id=test-workspace-id&email=a@x.io"},
		{http.MethodGet, "/api/templates.list?workspace_id=test-workspace-id"},
	} {
		var resp *http.Response
		if call.method == http.MethodGet {
			resp, err = client.Get(call.route)
		}
		require.NoError(t, err)
		body := tfDecode(t, resp)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, call.route)
		assert.Equal(t, "two_factor_required", body["error"], call.route)
	}

	// Turning it on is open, and opens the rest.
	tfEnroll(t, client)
	resp, err = client.Get("/api/workspaces.list")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Turning it off is refused while the instance requires it.
	resp, err = client.Post("/api/user.twoFactor.disable", map[string]string{"code": "000000"})
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	// The older session never passed the second factor: still refused.
	resp, err = older.Get("/api/workspaces.list")
	require.NoError(t, err)
	body := tfDecode(t, resp)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, "two_factor_required", body["error"])

	// And a new sign-in by magic code alone opens nothing.
	tfChallenge(t, testutil.NewAPIClient(suite.ServerManager.GetURL()), email)
}

// Against PostgreSQL with a session time zone far from UTC: the deadlines stored as
// TIMESTAMP without zone are compared at UTC, so a magic code and a challenge behave the
// same whatever the connection's zone.
func TestTwoFactorAuthentication_SessionTimeZone(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()
	db := suite.DBManager.GetDB()

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = conn.ExecContext(ctx, `SET TIME ZONE 'Pacific/Kiritimati'`)
	require.NoError(t, err)
	var zone string
	require.NoError(t, conn.QueryRowContext(ctx, `SHOW TimeZone`).Scan(&zone))
	require.Equal(t, "Pacific/Kiritimati", zone)

	// The repository's own queries, on that connection: a code that expired a minute ago is
	// refused and reported as expired, one that expires in a minute is consumed.
	const userID = "550e8400-e29b-41d4-a716-446655440024"
	for _, row := range []struct{ id, hash, offset string }{
		{"7a1e0c4e-0000-4000-8000-000000000001", "expired-code-hash", "- interval '1 minute'"},
		{"7a1e0c4e-0000-4000-8000-000000000002", "live-code-hash", "+ interval '1 minute'"},
	} {
		_, err = conn.ExecContext(ctx, `
			INSERT INTO user_sessions (id, user_id, expires_at, created_at, magic_code, magic_code_expires_at)
			VALUES ($1, $2, (now() AT TIME ZONE 'UTC') + interval '1 day', now() AT TIME ZONE 'UTC', $3,
				(now() AT TIME ZONE 'UTC') `+row.offset+`)`, row.id, userID, row.hash)
		require.NoError(t, err)
	}
	authTx := repository.NewAuthTxRepository()

	_, err = authTx.ConsumeMagicCode(ctx, conn, userID, "expired-code-hash")
	var notFound *domain.ErrSessionNotFound
	assert.ErrorAs(t, err, &notFound, "expired at UTC, whatever the connection's zone")
	expired, err := authTx.HasExpiredMagicCode(ctx, conn, userID, "expired-code-hash")
	require.NoError(t, err)
	assert.True(t, expired)

	session, err := authTx.ConsumeMagicCode(ctx, conn, userID, "live-code-hash")
	require.NoError(t, err)
	assert.Equal(t, "7a1e0c4e-0000-4000-8000-000000000002", session.ID)
}

// v45 creates the two-factor tables on a database upgraded from 44, and runs again without
// error, as every part of v45 must until 45.0 ships.
func TestTwoFactorAuthentication_MigrationRunsTwice(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()
	db := suite.DBManager.GetDB()

	for _, table := range []string{"user_totp", "user_recovery_codes"} {
		var exists bool
		require.NoError(t, db.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists))
		assert.True(t, exists, table)
	}
	runV45System(t, db)
	runV45System(t, db)
}

func runV45System(t *testing.T, db *sql.DB) {
	t.Helper()
	m, ok := migrations.GetRegisteredMigration(45.0)
	require.True(t, ok)
	tx, err := db.Begin()
	require.NoError(t, err)
	require.NoError(t, m.UpdateSystem(context.Background(), &config.Config{}, tx))
	require.NoError(t, tx.Commit())
}
