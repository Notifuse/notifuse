package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// UpdateDKIMKeys locks the workspace row, lets mutate decide the new keys from the
// stored ones, and writes settings.dkim_keys alone: the rest of the settings is never
// rewritten from a copy. READ COMMITTED, so a writer that waited on the lock reads the
// row as the one before it left it.
func (r *workspaceRepository) UpdateDKIMKeys(ctx context.Context, workspaceID string, mutate domain.DKIMKeysMutator) ([]domain.DKIMKey, error) {
	tx, err := r.systemDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var keysJSON, integrationsJSON []byte
	err = tx.QueryRowContext(ctx,
		`SELECT settings->'dkim_keys', integrations FROM workspaces WHERE id = $1 FOR UPDATE`,
		workspaceID,
	).Scan(&keysJSON, &integrationsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &domain.ErrWorkspaceNotFound{WorkspaceID: workspaceID}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the DKIM keys: %w", err)
	}

	keys, err := decodeDKIMKeys(keysJSON)
	if err != nil {
		return nil, err
	}
	var integrations domain.Integrations
	if len(integrationsJSON) > 0 {
		if err := json.Unmarshal(integrationsJSON, &integrations); err != nil {
			return nil, fmt.Errorf("failed to read the integrations: %w", err)
		}
	}

	// The ciphertexts already stored, read before mutate can touch the slice.
	stored := make(map[string]bool, len(keys))
	for _, k := range keys {
		stored[k.EncryptedPrivateKey] = true
	}

	mutation, err := mutate(keys, integrations)
	if err != nil {
		return nil, err
	}
	if !mutation.Changed {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit: %w", err)
		}
		return keys, nil
	}

	newKeys := mutation.Keys
	if newKeys == nil {
		newKeys = []domain.DKIMKey{}
	}
	encoded, err := json.Marshal(newKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the DKIM keys: %w", err)
	}
	// Only the private keys this write brings are opened: a stored key that no longer
	// opens fails its own sends, never the writes about other domains.
	var added []domain.DKIMKey
	for _, k := range newKeys {
		if k.EncryptedPrivateKey != "" && !stored[k.EncryptedPrivateKey] {
			added = append(added, k)
		}
	}
	if len(added) > 0 {
		check, err := json.Marshal(map[string][]domain.DKIMKey{"dkim_keys": added})
		if err != nil {
			return nil, fmt.Errorf("failed to encode the DKIM keys: %w", err)
		}
		if err := domain.CheckDKIMKeys(check, r.secretKey); err != nil {
			return nil, err
		}
	}

	query := `UPDATE workspaces SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{dkim_keys}', $1::jsonb, true) WHERE id = $2`
	args := []interface{}{encoded, workspaceID}
	if mutation.TouchUpdatedAt {
		query = `UPDATE workspaces SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{dkim_keys}', $1::jsonb, true), updated_at = $3 WHERE id = $2`
		args = append(args, time.Now().UTC())
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("failed to write the DKIM keys: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}
	return newKeys, nil
}

// GetWorkspaceDKIMKeys reads settings.dkim_keys alone: no AfterLoad, nothing decrypted.
func (r *workspaceRepository) GetWorkspaceDKIMKeys(ctx context.Context, workspaceID string) ([]domain.DKIMKey, error) {
	var keysJSON []byte
	err := r.systemDB.QueryRowContext(ctx,
		`SELECT settings->'dkim_keys' FROM workspaces WHERE id = $1`, workspaceID,
	).Scan(&keysJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &domain.ErrWorkspaceNotFound{WorkspaceID: workspaceID}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the DKIM keys: %w", err)
	}
	return decodeDKIMKeys(keysJSON)
}

// ListWorkspacesDKIMMetadata strips the private keys in the query, so they never leave
// the database for the periodic check.
func (r *workspaceRepository) ListWorkspacesDKIMMetadata(ctx context.Context) ([]domain.WorkspaceDKIMMetadata, error) {
	rows, err := r.systemDB.QueryContext(ctx, `
		SELECT id, (
			SELECT jsonb_agg(k - 'encrypted_private_key')
			FROM jsonb_array_elements(settings->'dkim_keys') AS k
		)
		FROM workspaces
		WHERE jsonb_typeof(settings->'dkim_keys') = 'array'
			AND jsonb_array_length(settings->'dkim_keys') > 0
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("failed to list the DKIM keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// A workspace whose keys cannot be decoded is left out and named in the error, which
	// comes with the others: one bad row does not stop the checks of every workspace.
	var result []domain.WorkspaceDKIMMetadata
	var skipped []error
	for rows.Next() {
		var id string
		var keysJSON []byte
		if err := rows.Scan(&id, &keysJSON); err != nil {
			return nil, fmt.Errorf("failed to scan the DKIM keys: %w", err)
		}
		keys, err := decodeDKIMKeys(keysJSON)
		if err != nil {
			skipped = append(skipped, fmt.Errorf("workspace %s: %w", id, err))
			continue
		}
		result = append(result, domain.WorkspaceDKIMMetadata{WorkspaceID: id, Keys: keys})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list the DKIM keys: %w", err)
	}
	return result, errors.Join(skipped...)
}

func decodeDKIMKeys(raw []byte) ([]domain.DKIMKey, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var keys []domain.DKIMKey
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, fmt.Errorf("failed to decode the DKIM keys: %w", err)
	}
	return keys, nil
}
