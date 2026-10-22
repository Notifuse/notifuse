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
// It runs in one transaction: a workspace no key opens, or settings none of which opens
// when there is no workspace, roll everything back, and the error names them.
func RotateSecretKey(ctx context.Context, db *sql.DB, current string, previous []string) (SecretKeyRotationResult, error) {
	var result SecretKeyRotationResult

	tx, err := db.BeginTx(ctx, nil)
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
	// With a workspace stored, one that opens shows the keys are right. Without one, a
	// setting must: otherwise the keys are not the ones the data is under, as when a key
	// from before 42.0 with a single comma in it is read as a list.
	if len(workspaces) == 0 && len(settings) > 0 && len(result.UnopenedSettings) == len(settings) {
		return result, fmt.Errorf("%w: settings %s, and no workspace is stored", crypto.ErrUndecryptable,
			strings.Join(result.UnopenedSettings, ", "))
	}

	if err := tx.Commit(); err != nil {
		return SecretKeyRotationResult{}, fmt.Errorf("failed to commit the rotation: %w", err)
	}
	return result, nil
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
