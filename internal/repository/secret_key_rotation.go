package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
)

// secretKeyRotationLockID is the transaction-level advisory lock RotateSecretKey
// holds, so instances starting together rotate once: the next one finds nothing left
// to do. An arbitrary id that nothing else locks.
const secretKeyRotationLockID int64 = 4_242_000_001

// SecretKeyRotationResult counts the rows RotateSecretKey re-encrypted, and names the
// settings it left because no key opens them.
type SecretKeyRotationResult struct {
	Workspaces       int
	Settings         int
	UnopenedSettings []string
	// UserTOTPs counts the authenticator secrets re-encrypted.
	UserTOTPs int
	// DroppedTOTPSetups counts the setups dropped because no key opens their secret.
	DroppedTOTPSetups int
	// UnopenedTOTPUsers names the people whose active authenticator secret no key opens.
	// They cannot pass their second factor until the root user resets it.
	UnopenedTOTPUsers []string
}

type rotationWorkspaceRow struct {
	id           string
	settings     []byte
	integrations []byte
}

type rotationSettingRow struct {
	key   string
	value string
}

// RotateSecretKey makes every value Notifuse encrypted with SECRET_KEY open with
// current: the encrypted fields of each workspace's settings and integrations
// (domain.EncryptedFieldPaths), and the encrypted_ rows of the settings table. A value a
// previous key opens is re-encrypted with current; with no previous key, the values are
// only checked. Each workspace is then opened as loading it does
// (domain.CheckStoredWorkspace), so a value nothing reads, such as one under a provider
// an integration does not use, is left as it is even when no key opens it.
//
// Settings rows a previous key opens are re-encrypted too. One no key opens is left as
// it is and named in the result: loading the settings at startup skips it, as before
// 42.0, and rows nothing reads any more stay behind (v29 left encrypted_smtp_relay_*).
// With no workspace stored, at least one of the rows must open.
//
// The authenticator secrets of two-factor authentication (user_totp) are re-encrypted too,
// each bound to its user; see rotateUserTOTPs for what happens to one no key opens, which
// may drop a setup in progress even with no previous key.
//
// It runs in one transaction: a workspace no key opens, or settings none of which opens
// when there is no workspace, roll everything back, and the error names them.
func RotateSecretKey(ctx context.Context, db *sql.DB, current string, previous []string) (SecretKeyRotationResult, error) {
	var result SecretKeyRotationResult

	// READ COMMITTED whatever the server's default: an instance that waited on the lock
	// reads the rows as the one before it left them, where a stricter level fails on them.
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return result, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, secretKeyRotationLockID); err != nil {
		return result, fmt.Errorf("failed to lock the rotation: %w", err)
	}

	workspaces, err := lockWorkspaceDocuments(ctx, tx)
	if err != nil {
		return result, err
	}
	settingsPaths, integrationsPaths := domain.EncryptedFieldPaths()
	for _, w := range workspaces {
		settings, settingsChanged, err := crypto.ReencryptFields(w.settings, settingsPaths, current, previous)
		if err != nil {
			return result, fmt.Errorf("workspace %s, settings: %w", w.id, err)
		}
		integrations, integrationsChanged, err := crypto.ReencryptFields(w.integrations, integrationsPaths, current, previous)
		if err != nil {
			return result, fmt.Errorf("workspace %s, integrations: %w", w.id, err)
		}
		if err := domain.CheckStoredWorkspace(settings, integrations, current); err != nil {
			return result, fmt.Errorf("workspace %s: %w", w.id, err)
		}
		// Loading a workspace never opens its DKIM keys, so CheckStoredWorkspace does
		// not reach them: a key left unopenable would only fail at its next send.
		if err := domain.CheckDKIMKeys(settings, current); err != nil {
			return result, fmt.Errorf("workspace %s: %w", w.id, err)
		}
		if !settingsChanged && !integrationsChanged {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE workspaces SET settings = $1, integrations = $2 WHERE id = $3`,
			nullableDocument(settings), nullableDocument(integrations), w.id,
		); err != nil {
			return result, fmt.Errorf("failed to update workspace %s: %w", w.id, err)
		}
		result.Workspaces++
	}

	settings, err := lockEncryptedSettings(ctx, tx)
	if err != nil {
		return result, err
	}
	for _, setting := range settings {
		value, changed, err := crypto.ReencryptValue(setting.value, current, previous)
		if errors.Is(err, crypto.ErrUndecryptable) {
			result.UnopenedSettings = append(result.UnopenedSettings, setting.key)
			continue
		}
		if err != nil {
			return result, fmt.Errorf("setting %s: %w", setting.key, err)
		}
		if !changed {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE settings SET value = $1 WHERE key = $2`, value, setting.key); err != nil {
			return result, fmt.Errorf("failed to update setting %s: %w", setting.key, err)
		}
		result.Settings++
	}
	// A workspace stored (each one opened, or the rotation stopped above) or a setting that
	// opened shows the keys are right; so does an active authenticator secret that opens.
	keysProven := len(workspaces) > 0 || len(settings) > len(result.UnopenedSettings)
	openedTOTPs, err := rotateUserTOTPs(ctx, tx, current, previous, keysProven, &result)
	if err != nil {
		return result, err
	}

	// Without a workspace, a setting or an authenticator secret must open: otherwise the
	// keys are not the ones the data is under, as when a key from before 42.0 with a
	// single comma in it is read as a list.
	if len(workspaces) == 0 && openedTOTPs == 0 && len(settings) > 0 && len(result.UnopenedSettings) == len(settings) {
		return result, fmt.Errorf("%w: settings %s, and no workspace is stored", crypto.ErrUndecryptable,
			strings.Join(result.UnopenedSettings, ", "))
	}

	if err := tx.Commit(); err != nil {
		return SecretKeyRotationResult{}, fmt.Errorf("failed to commit the rotation: %w", err)
	}
	return result, nil
}

// rotateUserTOTPs re-encrypts with current the authenticator secrets a previous key opens,
// the active one and the one being set up, each bound to its user, and returns how many
// active secrets opened.
//
// It reads and opens every row first, then decides, because what it may do with a secret no
// key opens depends on whether the keys are known to be right: keysProven (a workspace or a
// setting opened), or an active secret that opened.
//   - Keys proven: an active secret no key opens is left as it is and its user named in
//     UnopenedTOTPUsers (that person needs a reset by the root user, and the rest of the
//     instance starts); a setup whose secret no key opens is dropped, as it would fail anyway.
//   - Keys not proven: an active secret no key opens rolls the rotation back, naming the
//     user, and setups are left as they are: the keys are more likely wrong than the data.
func rotateUserTOTPs(ctx context.Context, tx *sql.Tx, current string, previous []string, keysProven bool, result *SecretKeyRotationResult) (int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT user_id, secret_encrypted, pending_secret_encrypted FROM user_totp
		WHERE secret_encrypted IS NOT NULL OR pending_secret_encrypted IS NOT NULL
		ORDER BY user_id FOR UPDATE`)
	if err != nil {
		return 0, fmt.Errorf("failed to read authenticator secrets: %w", err)
	}
	type totpRow struct {
		userID          string
		secret, pending sql.NullString
		// filled by the first pass
		secretOpened, secretChanged   bool
		pendingOpened, pendingChanged bool
	}
	var totps []*totpRow
	for rows.Next() {
		t := &totpRow{}
		if err := rows.Scan(&t.userID, &t.secret, &t.pending); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("failed to scan an authenticator secret: %w", err)
		}
		totps = append(totps, t)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("failed to read authenticator secrets: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("failed to read authenticator secrets: %w", err)
	}

	// First pass: open everything, write nothing.
	opened := 0
	var unopened []string
	for _, t := range totps {
		aad, err := domain.TOTPAAD(t.userID)
		if err != nil {
			return 0, err
		}
		if t.secret.Valid {
			value, changed, err := crypto.ReencryptValueAAD(t.secret.String, current, previous, aad)
			switch {
			case errors.Is(err, crypto.ErrUndecryptable):
				unopened = append(unopened, t.userID)
			case err != nil:
				return 0, fmt.Errorf("authenticator secret of user %s: %w", t.userID, err)
			default:
				opened++
				t.secretOpened, t.secretChanged, t.secret.String = true, changed, value
			}
		}
		if t.pending.Valid {
			value, changed, err := crypto.ReencryptValueAAD(t.pending.String, current, previous, aad)
			switch {
			case errors.Is(err, crypto.ErrUndecryptable):
			case err != nil:
				return 0, fmt.Errorf("authenticator setup of user %s: %w", t.userID, err)
			default:
				t.pendingOpened, t.pendingChanged, t.pending.String = true, changed, value
			}
		}
	}
	proven := keysProven || opened > 0
	if !proven && len(unopened) > 0 {
		return opened, fmt.Errorf("%w: authenticator secrets of users %s", crypto.ErrUndecryptable, strings.Join(unopened, ", "))
	}

	// Second pass: write.
	for _, t := range totps {
		dropPending := t.pending.Valid && !t.pendingOpened && proven
		if !t.secretChanged && !t.pendingChanged && !dropPending {
			continue
		}
		if dropPending {
			_, err = tx.ExecContext(ctx, `
				UPDATE user_totp SET secret_encrypted = $2, pending_secret_encrypted = NULL, pending_expires_at = NULL,
					pending_attempts = 0, updated_at = now()
				WHERE user_id = $1`, t.userID, t.secret)
		} else {
			_, err = tx.ExecContext(ctx, `
				UPDATE user_totp SET secret_encrypted = $2, pending_secret_encrypted = $3, updated_at = now()
				WHERE user_id = $1`, t.userID, t.secret, t.pending)
		}
		if err != nil {
			return opened, fmt.Errorf("failed to update the authenticator secret of user %s: %w", t.userID, err)
		}
		if t.secretChanged || t.pendingChanged {
			result.UserTOTPs++
		}
		if dropPending {
			result.DroppedTOTPSetups++
		}
	}
	result.UnopenedTOTPUsers = unopened
	return opened, nil
}

// SecretKeyOpensAUserTOTP reports whether key opens at least one active authenticator
// secret, or there is none, and how many it tried. It reads without locks and stops at the
// first that opens. Secrets being set up are left out: the rotation drops those no key
// opens, so they must not stop a start.
func SecretKeyOpensAUserTOTP(ctx context.Context, db *sql.DB, key string) (bool, int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT user_id, secret_encrypted FROM user_totp
		WHERE enabled_at IS NOT NULL AND secret_encrypted IS NOT NULL
		ORDER BY user_id`)
	if err != nil {
		return false, 0, fmt.Errorf("failed to read authenticator secrets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tried := 0
	for rows.Next() {
		var userID, value string
		if err := rows.Scan(&userID, &value); err != nil {
			return false, tried, fmt.Errorf("failed to read authenticator secrets: %w", err)
		}
		tried++
		aad, err := domain.TOTPAAD(userID)
		if err != nil {
			continue
		}
		if _, err := crypto.DecryptFromHexStringAAD(value, key, aad); err == nil {
			return true, tried, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, tried, fmt.Errorf("failed to read authenticator secrets: %w", err)
	}
	return tried == 0, tried, nil
}

// nullableDocument writes back a column read as NULL as NULL: the driver would send an
// empty slice as an empty string, which is not JSON.
func nullableDocument(doc []byte) any {
	if doc == nil {
		return nil
	}
	return doc
}

// lockWorkspaceDocuments reads every workspace's settings and integrations, locking
// the rows in id order. They are read in full before any update, as a connection runs
// one statement at a time.
func lockWorkspaceDocuments(ctx context.Context, tx *sql.Tx) ([]rotationWorkspaceRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, settings, integrations FROM workspaces ORDER BY id FOR UPDATE`)
	if err != nil {
		return nil, fmt.Errorf("failed to read workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var workspaces []rotationWorkspaceRow
	for rows.Next() {
		var w rotationWorkspaceRow
		if err := rows.Scan(&w.id, &w.settings, &w.integrations); err != nil {
			return nil, fmt.Errorf("failed to scan workspace: %w", err)
		}
		workspaces = append(workspaces, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read workspaces: %w", err)
	}
	return workspaces, nil
}

// encryptedSettingsQuery reads the non-empty encrypted_ settings in key order: the
// settings both the startup pass and the single-key check take as the data under the key.
const encryptedSettingsQuery = `SELECT key, value FROM settings WHERE key LIKE 'encrypted\_%' AND value <> '' ORDER BY key`

// lockEncryptedSettings reads the non-empty encrypted_ settings, locking the rows in
// key order.
func lockEncryptedSettings(ctx context.Context, tx *sql.Tx) ([]rotationSettingRow, error) {
	rows, err := tx.QueryContext(ctx, encryptedSettingsQuery+` FOR UPDATE`)
	if err != nil {
		return nil, fmt.Errorf("failed to read settings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var settings []rotationSettingRow
	for rows.Next() {
		var setting rotationSettingRow
		if err := rows.Scan(&setting.key, &setting.value); err != nil {
			return nil, fmt.Errorf("failed to scan setting: %w", err)
		}
		settings = append(settings, setting)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read settings: %w", err)
	}
	return settings, nil
}

// SecretKeyOpensAWorkspace reports whether key opens the key of at least one stored
// workspace, or there is none, and how many it tried. It reads one small field per
// workspace, without locks, and stops at the first that opens: with a single key, a
// key that opens none means every workspace load would fail.
func SecretKeyOpensAWorkspace(ctx context.Context, db *sql.DB, key string) (bool, int, error) {
	opens, tried, err := keyOpensAStoredValue(ctx, db,
		`SELECT id, settings->>'encrypted_secret_key' FROM workspaces WHERE COALESCE(settings->>'encrypted_secret_key', '') <> ''`,
		key)
	if err != nil {
		return false, len(tried), fmt.Errorf("failed to read workspace keys: %w", err)
	}
	return opens, len(tried), nil
}

// SecretKeyOpensASetting reports whether key opens at least one stored encrypted_
// setting, or there is none, and which it tried. It reads without locks and stops at the
// first that opens: with no workspace stored, these settings are the data under the key.
func SecretKeyOpensASetting(ctx context.Context, db *sql.DB, key string) (bool, []string, error) {
	opens, tried, err := keyOpensAStoredValue(ctx, db, encryptedSettingsQuery, key)
	if err != nil {
		return false, tried, fmt.Errorf("failed to read settings: %w", err)
	}
	return opens, tried, nil
}

// keyOpensAStoredValue runs query, which selects a name and an encrypted value, and
// reports whether key opens one of the values, stopping at the first that opens, or
// there is none; and the names it tried.
func keyOpensAStoredValue(ctx context.Context, db *sql.DB, query, key string) (bool, []string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return false, nil, err
	}
	defer func() { _ = rows.Close() }()

	var tried []string
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return false, tried, err
		}
		tried = append(tried, name)
		if _, err := crypto.DecryptFromHexString(value, key); err == nil {
			return true, tried, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, tried, err
	}
	return len(tried) == 0, tried, nil
}
