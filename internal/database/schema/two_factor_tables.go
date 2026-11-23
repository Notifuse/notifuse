package schema

// Two-factor authentication, in the SYSTEM database only.
//
// user_totp holds one authenticator app per user. A row whose enabled_at is NULL is a
// setup that was started and not confirmed: it does not count as two-factor
// authentication. The active secret and the secret being set up are separate, so
// replacing the app does not break the one in use until the new code is confirmed.
// Secrets are sealed with SECRET_KEY and bound to the user (crypto.EncryptStringAAD);
// RotateSecretKey re-encrypts them.
//
// user_recovery_codes holds the one-time codes, as SHA-256 hashes bound to the user.
// They carry 120 bits each, so a plain hash is enough (NIST SP 800-63B-4 §3.1.2), and
// they do not depend on SECRET_KEY, so a key rotation leaves them working.
//
// user_sessions gains how the session was opened (auth_method), the second factor it
// passed, and the pending sign-in challenge: a session with challenge_hash set is
// waiting for its second factor and is not usable.
//
// Every time is TIMESTAMPTZ and compared to now() in SQL. The older user_sessions
// columns are TIMESTAMP written in UTC from Go; the new ones do not repeat that.
//
// Shared verbatim by internal/database/init.go (fresh installs) and the v45 migration
// (upgrades), so the two cannot drift. Constraints never live in the CREATE TABLE
// (house rule of system_tables.go): the foreign keys are added by DO blocks that resolve
// ::regclass, so they come after the tables.

// TwoFactorTableDefinitions returns the DDL of two-factor authentication, in an order that
// is safe to run more than once.
func TwoFactorTableDefinitions() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS user_totp (
	user_id UUID PRIMARY KEY,
	secret_encrypted TEXT,
	enabled_at TIMESTAMPTZ,
	last_used_step BIGINT NOT NULL DEFAULT 0,
	pending_secret_encrypted TEXT,
	pending_expires_at TIMESTAMPTZ,
	pending_attempts INTEGER NOT NULL DEFAULT 0,
	failed_attempts INTEGER NOT NULL DEFAULT 0,
	locked_until TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS user_recovery_codes (
	id UUID PRIMARY KEY,
	user_id UUID NOT NULL,
	code_hash CHAR(64) NOT NULL,
	used_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL,
	UNIQUE (user_id, code_hash)
)`,
		`ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS auth_method VARCHAR(20)`,
		`ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS second_factor VARCHAR(20)`,
		`ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS second_factor_at TIMESTAMPTZ`,
		`ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS challenge_hash CHAR(64)`,
		`ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS challenge_expires_at TIMESTAMPTZ`,
		`ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS challenge_attempts INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS pending_invitation_id UUID`,
		`ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS authenticated_at TIMESTAMPTZ`,
		// user.twoFactor.verify finds the session by this hash alone, on a public route.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_user_sessions_challenge_hash ON user_sessions (challenge_hash) WHERE challenge_hash IS NOT NULL`,
		`DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint
		WHERE conname = 'user_totp_user_id_fkey'
		AND conrelid = 'user_totp'::regclass
	) THEN
		ALTER TABLE user_totp
			ADD CONSTRAINT user_totp_user_id_fkey
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
	END IF;
END $$`,
		`DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint
		WHERE conname = 'user_recovery_codes_user_id_fkey'
		AND conrelid = 'user_recovery_codes'::regclass
	) THEN
		ALTER TABLE user_recovery_codes
			ADD CONSTRAINT user_recovery_codes_user_id_fkey
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
	END IF;
END $$`,
	}
}
