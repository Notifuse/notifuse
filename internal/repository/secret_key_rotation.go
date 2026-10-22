package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
)

// secretKeyRotationLockID is the transaction-level advisory lock RotateSecretKey
// holds, so instances starting together rotate once: the next one finds nothing left
// to do. An arbitrary id that nothing else locks.
const secretKeyRotationLockID int64 = 4_242_000_001

// SecretKeyRotationResult counts the rows RotateSecretKey re-encrypted.
type SecretKeyRotationResult struct {
	Workspaces int
	Settings   int
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
// (domain.EncryptedFieldPaths), and the encrypted_ rows of the settings table. A value a previous key opens is
// re-encrypted with current; with no previous key, the values are only checked.
//
// It runs in one transaction: a value no key opens rolls everything back, and the
// error names where it is.
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
			return result, fmt.Errorf("workspace %s, settings %w", w.id, err)
		}
		integrations, integrationsChanged, err := crypto.ReencryptFields(w.integrations, integrationsPaths, current, previous)
		if err != nil {
			return result, fmt.Errorf("workspace %s, integrations %w", w.id, err)
		}
		if !settingsChanged && !integrationsChanged {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE workspaces SET settings = $1, integrations = $2 WHERE id = $3`,
			settings, integrations, w.id,
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

	if err := tx.Commit(); err != nil {
		return SecretKeyRotationResult{}, fmt.Errorf("failed to commit the rotation: %w", err)
	}
	return result, nil
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

// lockEncryptedSettings reads the non-empty encrypted_ settings, locking the rows in
// key order.
func lockEncryptedSettings(ctx context.Context, tx *sql.Tx) ([]rotationSettingRow, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT key, value FROM settings WHERE key LIKE 'encrypted\_%' AND value <> '' ORDER BY key FOR UPDATE`)
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
