package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
)

// TrackingWorkspaceRow is what the email tracking routes read about a workspace: its
// secret key, still encrypted with SECRET_KEY, and its custom endpoint URL as stored.
type TrackingWorkspaceRow struct {
	EncryptedSecretKey string
	CustomEndpointURL  string
}

// GetTrackingWorkspace reads only the two settings the tracking routes need, by primary
// key. It deliberately does not load the workspace: loading decrypts every integration,
// and one that no key opens would then fail every click for an otherwise healthy
// workspace. It returns *domain.ErrWorkspaceNotFound when there is no row.
func GetTrackingWorkspace(ctx context.Context, db *sql.DB, id string) (TrackingWorkspaceRow, error) {
	var row TrackingWorkspaceRow
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(settings->>'encrypted_secret_key', ''), COALESCE(settings->>'custom_endpoint_url', '') FROM workspaces WHERE id = $1`,
		id,
	).Scan(&row.EncryptedSecretKey, &row.CustomEndpointURL)
	if errors.Is(err, sql.ErrNoRows) {
		return TrackingWorkspaceRow{}, &domain.ErrWorkspaceNotFound{WorkspaceID: id}
	}
	if err != nil {
		return TrackingWorkspaceRow{}, fmt.Errorf("failed to read tracking workspace: %w", err)
	}
	return row, nil
}
