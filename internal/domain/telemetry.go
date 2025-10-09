package domain

import (
	"context"
	"database/sql"
)

//go:generate mockgen -destination mocks/mock_telemetry_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain TelemetryRepository

// TelemetryMetrics represents aggregated metrics for a workspace
type TelemetryMetrics struct {
	ContactsCount      int    `json:"contacts_count"`
	BroadcastsCount    int    `json:"broadcasts_count"`
	TransactionalCount int    `json:"transactional_count"`
	MessagesCount      int    `json:"messages_count"`
	ListsCount         int    `json:"lists_count"`
	SegmentsCount      int    `json:"segments_count"`
	UsersCount         int    `json:"users_count"`
	LastMessageAt      string `json:"last_message_at"`
}

// TelemetryRepository defines the interface for telemetry data operations
type TelemetryRepository interface {
	// GetWorkspaceMetrics retrieves aggregated metrics for a specific workspace
	GetWorkspaceMetrics(ctx context.Context, workspaceID string) (*TelemetryMetrics, error)

	// CountContacts counts the total number of contacts in a workspace
	CountContacts(ctx context.Context, db *sql.DB) (int, error)

	// CountBroadcasts counts the total number of broadcasts in a workspace
	CountBroadcasts(ctx context.Context, db *sql.DB) (int, error)

	// CountTransactional counts the total number of transactional notifications in a workspace
	CountTransactional(ctx context.Context, db *sql.DB) (int, error)

	// CountMessages counts the total number of messages in a workspace
	CountMessages(ctx context.Context, db *sql.DB) (int, error)

	// CountLists counts the total number of lists in a workspace
	CountLists(ctx context.Context, db *sql.DB) (int, error)

	// CountSegments counts the total number of segments in a workspace
	CountSegments(ctx context.Context, db *sql.DB) (int, error)

	// CountUsers counts the total number of users in a workspace from the system database
	CountUsers(ctx context.Context, systemDB *sql.DB, workspaceID string) (int, error)

	// GetLastMessageAt gets the timestamp of the last message sent from the workspace
	GetLastMessageAt(ctx context.Context, db *sql.DB) (string, error)
}
