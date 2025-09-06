package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// InitializeDatabase creates all necessary database tables if they don't exist
func InitializeDatabase(db *sql.DB, rootEmail string, cfg *config.Config, migrationLogger logger.Logger) error {
	// Run all table creation queries
	for _, query := range schema.TableDefinitions {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	// Run migration statements for schema changes
	for _, query := range schema.GetMigrationStatements() {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to run migration: %w", err)
		}
	}

	// Run version migrations
	migrationManager := migrations.NewManager(migrationLogger)

	ctx := context.Background()
	if err := migrationManager.RunMigrations(ctx, cfg, db); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Create root user if it doesn't exist
	if rootEmail != "" {
		// Check if root user exists
		var exists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", rootEmail).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check root user existence: %w", err)
		}

		if !exists {
			// Create root user
			rootUser := &domain.User{
				ID:        uuid.New().String(),
				Email:     rootEmail,
				Name:      "Root User",
				Type:      domain.UserTypeUser,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}

			query := `
				INSERT INTO users (id, email, name, type, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6)
			`
			_, err = db.Exec(query,
				rootUser.ID,
				rootUser.Email,
				rootUser.Name,
				rootUser.Type,
				rootUser.CreatedAt,
				rootUser.UpdatedAt,
			)
			if err != nil {
				return fmt.Errorf("failed to create root user: %w", err)
			}
		}
	}

	return nil
}

// InitializeWorkspaceDatabase creates the necessary tables for a workspace database
func InitializeWorkspaceDatabase(db *sql.DB) error {
	// Create workspace tables
	queries := []string{
		`CREATE TABLE IF NOT EXISTS contacts (
			email VARCHAR(255) PRIMARY KEY,
			external_id VARCHAR(255),
			timezone VARCHAR(50),
			language VARCHAR(50),
			first_name VARCHAR(255),
			last_name VARCHAR(255),
			phone VARCHAR(50),
			address_line_1 VARCHAR(255),
			address_line_2 VARCHAR(255),
			country VARCHAR(100),
			postcode VARCHAR(20),
			state VARCHAR(100),
			job_title VARCHAR(255),
			lifetime_value DECIMAL,
			orders_count INTEGER,
			last_order_at TIMESTAMP WITH TIME ZONE,
			custom_string_1 VARCHAR(255),
			custom_string_2 VARCHAR(255),
			custom_string_3 VARCHAR(255),
			custom_string_4 VARCHAR(255),
			custom_string_5 VARCHAR(255),
			custom_number_1 DECIMAL,
			custom_number_2 DECIMAL,
			custom_number_3 DECIMAL,
			custom_number_4 DECIMAL,
			custom_number_5 DECIMAL,
			custom_datetime_1 TIMESTAMP WITH TIME ZONE,
			custom_datetime_2 TIMESTAMP WITH TIME ZONE,
			custom_datetime_3 TIMESTAMP WITH TIME ZONE,
			custom_datetime_4 TIMESTAMP WITH TIME ZONE,
			custom_datetime_5 TIMESTAMP WITH TIME ZONE,
			custom_json_1 JSONB,
			custom_json_2 JSONB,
			custom_json_3 JSONB,
			custom_json_4 JSONB,
			custom_json_5 JSONB,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_email ON contacts(email)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_external_id ON contacts(external_id)`,
		`CREATE TABLE IF NOT EXISTS lists (
			id VARCHAR(32) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			is_double_optin BOOLEAN NOT NULL DEFAULT FALSE,
			is_public BOOLEAN NOT NULL DEFAULT FALSE,
			description TEXT,
			double_optin_template JSONB,
			welcome_template JSONB,
			unsubscribe_template JSONB,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP WITH TIME ZONE
		)`,
		`CREATE TABLE IF NOT EXISTS contact_lists (
			email VARCHAR(255) NOT NULL,
			list_id VARCHAR(32) NOT NULL,
			status VARCHAR(20) NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP WITH TIME ZONE,
			PRIMARY KEY (email, list_id)
		)`,
		`CREATE TABLE IF NOT EXISTS templates (
			id VARCHAR(32) NOT NULL,
			name VARCHAR(255) NOT NULL,
			version INTEGER NOT NULL,
			channel VARCHAR(20) NOT NULL,
			email JSONB NOT NULL,
			category VARCHAR(20) NOT NULL,
			template_macro_id VARCHAR(32),
			test_data JSONB,
			settings JSONB,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP WITH TIME ZONE,
			PRIMARY KEY (id, version)
		)`,
		`CREATE TABLE IF NOT EXISTS broadcasts (
			id VARCHAR(255) NOT NULL,
			workspace_id VARCHAR(32) NOT NULL,
			name VARCHAR(255) NOT NULL,
			status VARCHAR(20) NOT NULL,
			audience JSONB NOT NULL,
			schedule JSONB NOT NULL,
			test_settings JSONB NOT NULL,
			utm_parameters JSONB,
			metadata JSONB,
			winning_template VARCHAR(32),
			test_sent_at TIMESTAMP WITH TIME ZONE,
			winner_sent_at TIMESTAMP WITH TIME ZONE,
			test_phase_recipient_count INTEGER DEFAULT 0,
			winner_phase_recipient_count INTEGER DEFAULT 0,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
			started_at TIMESTAMP WITH TIME ZONE,
			completed_at TIMESTAMP WITH TIME ZONE,
			cancelled_at TIMESTAMP WITH TIME ZONE,
			paused_at TIMESTAMP WITH TIME ZONE,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE IF NOT EXISTS message_history (
			id VARCHAR(255) NOT NULL PRIMARY KEY,
			contact_email VARCHAR(255) NOT NULL,
			external_id VARCHAR(255),
			broadcast_id VARCHAR(255),
			template_id VARCHAR(32) NOT NULL,
			template_version INTEGER NOT NULL,
			channel VARCHAR(20) NOT NULL,
			status_info VARCHAR(255),
			message_data JSONB NOT NULL,
			sent_at TIMESTAMP WITH TIME ZONE NOT NULL,
			delivered_at TIMESTAMP WITH TIME ZONE,
			failed_at TIMESTAMP WITH TIME ZONE,
			opened_at TIMESTAMP WITH TIME ZONE,
			clicked_at TIMESTAMP WITH TIME ZONE,
			bounced_at TIMESTAMP WITH TIME ZONE,
			complained_at TIMESTAMP WITH TIME ZONE,
			unsubscribed_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_message_history_contact_email ON message_history(contact_email)`,
		`CREATE INDEX IF NOT EXISTS idx_message_history_broadcast_id ON message_history(broadcast_id)`,
		`CREATE INDEX IF NOT EXISTS idx_message_history_template_id ON message_history(template_id, template_version)`,
		`CREATE INDEX IF NOT EXISTS idx_message_history_created_at_id ON message_history(created_at DESC, id DESC)`,
		`CREATE TABLE IF NOT EXISTS transactional_notifications (
			id VARCHAR(32) NOT NULL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			description TEXT,
			channels JSONB NOT NULL,
			tracking_settings JSONB,
			metadata JSONB,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP WITH TIME ZONE
		)`,
		`CREATE TABLE IF NOT EXISTS webhook_events (
			id UUID PRIMARY KEY,
			type VARCHAR(50) NOT NULL,
			email_provider_kind VARCHAR(50) NOT NULL,
			integration_id VARCHAR(255) NOT NULL,
			recipient_email VARCHAR(255) NOT NULL,
			message_id VARCHAR(255) NOT NULL,
			timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
			raw_payload TEXT NOT NULL,
			bounce_type VARCHAR(100),
			bounce_category VARCHAR(100),
			bounce_diagnostic TEXT,
			complaint_feedback_type VARCHAR(100),
			created_at TIMESTAMP WITH TIME ZONE NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS webhook_events_message_id_idx ON webhook_events (message_id)`,
		`CREATE INDEX IF NOT EXISTS webhook_events_type_idx ON webhook_events (type)`,
		`CREATE INDEX IF NOT EXISTS webhook_events_timestamp_idx ON webhook_events (timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS webhook_events_recipient_email_idx ON webhook_events (recipient_email)`,
		`CREATE INDEX IF NOT EXISTS idx_broadcasts_status_testing ON broadcasts(status) WHERE status IN ('testing', 'test_completed', 'winner_selected')`,
	}

	// Run all table creation queries
	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to create workspace table: %w", err)
		}
	}

	return nil
}

// CleanDatabase drops all tables in reverse order
func CleanDatabase(db *sql.DB) error {
	// Drop tables in reverse order to handle dependencies
	for i := len(schema.TableNames) - 1; i >= 0; i-- {
		query := fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", schema.TableNames[i])
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to drop table %s: %w", schema.TableNames[i], err)
		}
	}

	// Drop the webhook_events table
	if _, err := db.Exec("DROP TABLE IF EXISTS webhook_events CASCADE"); err != nil {
		return fmt.Errorf("failed to drop webhook_events table: %w", err)
	}

	return nil
}
