package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// InitializeDatabase creates all necessary database tables if they don't exist
func InitializeDatabase(db *sql.DB, rootEmail string) error {
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
			full_name VARCHAR(255),
			phone VARCHAR(50),
			address_line_1 VARCHAR(255),
			address_line_2 VARCHAR(255),
			country VARCHAR(100),
			postcode VARCHAR(20),
			state VARCHAR(100),
			job_title VARCHAR(255),
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
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			db_created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			db_updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_external_id ON contacts(external_id)`,
		`CREATE TABLE IF NOT EXISTS lists (
			id VARCHAR(32) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			is_double_optin BOOLEAN NOT NULL DEFAULT FALSE,
			is_public BOOLEAN NOT NULL DEFAULT FALSE,
			description TEXT,
			double_optin_template JSONB,
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
		`CREATE INDEX IF NOT EXISTS idx_contact_lists_list_id ON contact_lists(list_id)`,
		`CREATE TABLE IF NOT EXISTS templates (
			id VARCHAR(32) NOT NULL,
			name VARCHAR(255) NOT NULL,
			version INTEGER NOT NULL,
			channel VARCHAR(20) NOT NULL,
			email JSONB,
			web JSONB,
			category VARCHAR(20) NOT NULL,
			template_macro_id VARCHAR(32),
			integration_id VARCHAR(255),
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
			enqueued_count INTEGER DEFAULT 0,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
			started_at TIMESTAMP WITH TIME ZONE,
			completed_at TIMESTAMP WITH TIME ZONE,
			cancelled_at TIMESTAMP WITH TIME ZONE,
			paused_at TIMESTAMP WITH TIME ZONE,
			pause_reason TEXT,
			data_feed JSONB,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE IF NOT EXISTS message_history (
			id VARCHAR(255) NOT NULL PRIMARY KEY,
			contact_email VARCHAR(255) NOT NULL,
			external_id VARCHAR(255),
			broadcast_id VARCHAR(255),
			automation_id VARCHAR(36),
			list_id VARCHAR(32),
			template_id VARCHAR(32) NOT NULL,
			template_version INTEGER NOT NULL,
			channel VARCHAR(20) NOT NULL,
			status_info VARCHAR(255),
			message_data JSONB NOT NULL,
			channel_options JSONB,
			attachments JSONB,
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
		`CREATE INDEX IF NOT EXISTS idx_message_history_broadcast_id ON message_history(broadcast_id) WHERE broadcast_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_message_history_automation_id ON message_history(automation_id) WHERE automation_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_message_history_template_id ON message_history(template_id, template_version)`,
		`CREATE INDEX IF NOT EXISTS idx_message_history_created_at_id ON message_history(created_at DESC, id DESC)`,
		`CREATE TABLE IF NOT EXISTS transactional_notifications (
			id VARCHAR(32) NOT NULL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			description TEXT,
			channels JSONB NOT NULL,
			tracking_settings JSONB,
			metadata JSONB,
			integration_id VARCHAR(255),
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP WITH TIME ZONE
		)`,
		`CREATE TABLE IF NOT EXISTS inbound_webhook_events (
			id UUID PRIMARY KEY,
			type VARCHAR(50) NOT NULL,
			source VARCHAR(50) NOT NULL,
			integration_id VARCHAR(255) NOT NULL,
			recipient_email VARCHAR(255) NOT NULL,
			message_id VARCHAR(255),
			timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
			raw_payload TEXT NOT NULL,
			bounce_type VARCHAR(100),
			bounce_category VARCHAR(100),
			bounce_diagnostic TEXT,
			complaint_feedback_type VARCHAR(100),
			created_at TIMESTAMP WITH TIME ZONE NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS inbound_webhook_events_message_id_idx ON inbound_webhook_events (message_id)`,
		`CREATE INDEX IF NOT EXISTS inbound_webhook_events_type_idx ON inbound_webhook_events (type)`,
		`CREATE INDEX IF NOT EXISTS inbound_webhook_events_timestamp_idx ON inbound_webhook_events (timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS inbound_webhook_events_recipient_email_idx ON inbound_webhook_events (recipient_email)`,
		`CREATE INDEX IF NOT EXISTS idx_broadcasts_status_testing ON broadcasts(status) WHERE status IN ('testing', 'test_completed', 'winner_selected')`,
		`CREATE TABLE IF NOT EXISTS contact_timeline (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email VARCHAR(255) NOT NULL,
			operation VARCHAR(20) NOT NULL,
			entity_type VARCHAR(50) NOT NULL,
			kind VARCHAR(50) NOT NULL DEFAULT '',
			changes JSONB,
			entity_id VARCHAR(255),
			created_at TIMESTAMP WITH TIME ZONE NOT NULL,
			db_created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_timeline_email_created_at ON contact_timeline(email, created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_timeline_kind ON contact_timeline(kind)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_timeline_entity_id ON contact_timeline(entity_id) WHERE entity_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS segments (
			id VARCHAR(32) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			color VARCHAR(50) NOT NULL,
			tree JSONB NOT NULL,
			timezone VARCHAR(100) NOT NULL,
			version INTEGER NOT NULL,
			status VARCHAR(20) NOT NULL,
			generated_sql TEXT,
			generated_args JSONB,
			recompute_after TIMESTAMP WITH TIME ZONE,
			db_created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			db_updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_segments_status ON segments(status)`,
		`CREATE TABLE IF NOT EXISTS contact_segments (
			email VARCHAR(255) NOT NULL,
			segment_id VARCHAR(32) NOT NULL,
			version INTEGER NOT NULL,
			matched_at TIMESTAMP WITH TIME ZONE NOT NULL,
			computed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (email, segment_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_segments_segment_id ON contact_segments(segment_id)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_segments_version ON contact_segments(segment_id, version)`,
		`CREATE TABLE IF NOT EXISTS contact_segment_queue (
			email VARCHAR(255) PRIMARY KEY,
			queued_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_segment_queue_queued_at ON contact_segment_queue(queued_at ASC)`,
		`CREATE TABLE IF NOT EXISTS message_attachments (
			checksum VARCHAR(64) PRIMARY KEY,
			content BYTEA NOT NULL,
			content_type VARCHAR(255),
			size_bytes BIGINT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS blog_categories (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			slug VARCHAR(100) NOT NULL UNIQUE,
			settings JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_blog_categories_slug ON blog_categories(slug) WHERE deleted_at IS NULL`,
		`CREATE TABLE IF NOT EXISTS blog_posts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			category_id UUID,
			slug VARCHAR(100) NOT NULL UNIQUE,
			settings JSONB NOT NULL DEFAULT '{}',
			published_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_blog_posts_published ON blog_posts(published_at DESC) WHERE deleted_at IS NULL AND published_at IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_blog_posts_category ON blog_posts(category_id) WHERE deleted_at IS NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_blog_posts_slug ON blog_posts(slug) WHERE deleted_at IS NULL`,
		`CREATE TABLE IF NOT EXISTS blog_themes (
			version INTEGER NOT NULL PRIMARY KEY,
			published_at TIMESTAMP,
			published_by_user_id TEXT,
			files JSONB NOT NULL DEFAULT '{}',
			notes TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_blog_themes_published ON blog_themes(version) WHERE published_at IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_blog_themes_version ON blog_themes(version DESC)`,
		`CREATE TABLE IF NOT EXISTS custom_events (
			event_name VARCHAR(100) NOT NULL,
			external_id VARCHAR(255) NOT NULL,
			email VARCHAR(255) NOT NULL,
			properties JSONB NOT NULL DEFAULT '{}'::jsonb,
			occurred_at TIMESTAMPTZ NOT NULL,
			source VARCHAR(50) NOT NULL DEFAULT 'api',
			integration_id VARCHAR(32),
			-- Goal tracking fields
			goal_name VARCHAR(100) DEFAULT NULL,
			goal_type VARCHAR(20) DEFAULT NULL,
			goal_value DECIMAL(15,2) DEFAULT NULL,
			-- Soft delete
			deleted_at TIMESTAMPTZ DEFAULT NULL,
			-- Timestamps
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (event_name, external_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_events_email ON custom_events(email, occurred_at DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_custom_events_external_id ON custom_events(external_id)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_events_integration_id ON custom_events(integration_id) WHERE integration_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_custom_events_properties ON custom_events USING GIN (properties jsonb_path_ops)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_events_goal_type ON custom_events(email, goal_type, occurred_at DESC) WHERE goal_type IS NOT NULL AND deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_custom_events_purchases ON custom_events(email, goal_value, occurred_at) WHERE goal_type = 'purchase' AND deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_custom_events_deleted ON custom_events(deleted_at) WHERE deleted_at IS NOT NULL`,
		// Webhook subscriptions table
		`CREATE TABLE IF NOT EXISTS webhook_subscriptions (
			id VARCHAR(32) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			url TEXT NOT NULL,
			secret VARCHAR(64) NOT NULL,
			settings JSONB NOT NULL DEFAULT '{}'::jsonb,
			enabled BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			last_delivery_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled ON webhook_subscriptions(enabled) WHERE enabled = true`,
		// Webhook deliveries table
		`CREATE TABLE IF NOT EXISTS webhook_deliveries (
			id VARCHAR(36) PRIMARY KEY,
			subscription_id VARCHAR(32) NOT NULL,
			event_type VARCHAR(100) NOT NULL,
			payload JSONB NOT NULL,
			status VARCHAR(20) DEFAULT 'pending',
			attempts INT DEFAULT 0,
			max_attempts INT DEFAULT 10,
			next_attempt_at TIMESTAMPTZ DEFAULT NOW(),
			last_attempt_at TIMESTAMPTZ,
			delivered_at TIMESTAMPTZ,
			last_response_status INT,
			last_response_body TEXT,
			last_error TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending ON webhook_deliveries(next_attempt_at) WHERE status IN ('pending', 'failed') AND attempts < max_attempts`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription ON webhook_deliveries(subscription_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status ON webhook_deliveries(status)`,
		// Automation tables (nodes embedded as JSONB in automations)
		`CREATE TABLE IF NOT EXISTS automations (
			id VARCHAR(36) PRIMARY KEY,
			workspace_id VARCHAR(36) NOT NULL,
			name VARCHAR(255) NOT NULL,
			status VARCHAR(20) DEFAULT 'draft',
			list_id VARCHAR(36),
			trigger_config JSONB NOT NULL,
			trigger_sql TEXT,
			root_node_id VARCHAR(36),
			nodes JSONB DEFAULT '[]',
			stats JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_automations_workspace_status ON automations(workspace_id, status) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_automations_list ON automations(list_id, status)`,
		`CREATE TABLE IF NOT EXISTS contact_automations (
			id VARCHAR(36) PRIMARY KEY,
			automation_id VARCHAR(36) NOT NULL REFERENCES automations(id),
			contact_email VARCHAR(255) NOT NULL,
			current_node_id VARCHAR(36),
			status VARCHAR(20) DEFAULT 'active',
			exit_reason VARCHAR(50),
			entered_at TIMESTAMPTZ DEFAULT NOW(),
			scheduled_at TIMESTAMPTZ,
			context JSONB DEFAULT '{}',
			retry_count INTEGER DEFAULT 0,
			last_error TEXT,
			last_retry_at TIMESTAMPTZ,
			max_retries INTEGER DEFAULT 3,
			UNIQUE(automation_id, contact_email, entered_at)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_automations_scheduled ON contact_automations(scheduled_at) WHERE status = 'active' AND scheduled_at IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_contact_automations_automation ON contact_automations(automation_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_automations_email ON contact_automations(contact_email, status)`,
		`CREATE TABLE IF NOT EXISTS automation_node_executions (
			id VARCHAR(36) PRIMARY KEY,
			contact_automation_id VARCHAR(36) NOT NULL REFERENCES contact_automations(id) ON DELETE CASCADE,
			automation_id VARCHAR(36),
			node_id VARCHAR(36) NOT NULL,
			node_type VARCHAR(50) NOT NULL,
			action VARCHAR(20) NOT NULL,
			entered_at TIMESTAMPTZ DEFAULT NOW(),
			completed_at TIMESTAMPTZ,
			duration_ms INTEGER,
			output JSONB DEFAULT '{}',
			error TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_node_executions_contact_automation ON automation_node_executions(contact_automation_id, entered_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_node_executions_automation ON automation_node_executions(automation_id, node_id, action)`,
		`CREATE TABLE IF NOT EXISTS automation_trigger_log (
			id VARCHAR(36) PRIMARY KEY,
			automation_id VARCHAR(36) NOT NULL REFERENCES automations(id),
			contact_email VARCHAR(255) NOT NULL,
			triggered_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(automation_id, contact_email)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_trigger_log_automation ON automation_trigger_log(automation_id, triggered_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_trigger_log_contact ON automation_trigger_log(contact_email, automation_id)`,
		// Email queue tables (V21 migration)
		`CREATE TABLE IF NOT EXISTS email_queue (
			id VARCHAR(36) PRIMARY KEY,
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			priority INTEGER NOT NULL DEFAULT 5,
			source_type VARCHAR(20) NOT NULL,
			source_id VARCHAR(36) NOT NULL,
			integration_id VARCHAR(36) NOT NULL,
			provider_kind VARCHAR(20) NOT NULL,
			contact_email VARCHAR(255) NOT NULL,
			message_id VARCHAR(100) NOT NULL,
			template_id VARCHAR(36) NOT NULL,
			payload JSONB NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 3,
			last_error TEXT,
			next_retry_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			processed_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_email_queue_pending ON email_queue(priority ASC, created_at ASC) WHERE status = 'pending'`,
		`CREATE INDEX IF NOT EXISTS idx_email_queue_next_retry ON email_queue(next_retry_at) WHERE status = 'pending' AND next_retry_at IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_email_queue_retry ON email_queue(next_retry_at) WHERE status = 'failed' AND attempts < max_attempts`,
		`CREATE INDEX IF NOT EXISTS idx_email_queue_source ON email_queue(source_type, source_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_email_queue_integration ON email_queue(integration_id, status)`,
	}

	// Run all table creation queries
	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to create workspace table: %w", err)
		}
	}

	// Create trigger functions and triggers for contact timeline
	triggerQueries := []string{
		// Contact changes trigger function
		`CREATE OR REPLACE FUNCTION track_contact_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			op VARCHAR(20);
		BEGIN
			IF TG_OP = 'INSERT' THEN
				op := 'insert';
				changes_json := NULL;
			ELSIF TG_OP = 'UPDATE' THEN
				op := 'update';
				IF OLD.external_id IS DISTINCT FROM NEW.external_id THEN changes_json := changes_json || jsonb_build_object('external_id', jsonb_build_object('old', OLD.external_id, 'new', NEW.external_id)); END IF;
				IF OLD.timezone IS DISTINCT FROM NEW.timezone THEN changes_json := changes_json || jsonb_build_object('timezone', jsonb_build_object('old', OLD.timezone, 'new', NEW.timezone)); END IF;
				IF OLD.language IS DISTINCT FROM NEW.language THEN changes_json := changes_json || jsonb_build_object('language', jsonb_build_object('old', OLD.language, 'new', NEW.language)); END IF;
				IF OLD.first_name IS DISTINCT FROM NEW.first_name THEN changes_json := changes_json || jsonb_build_object('first_name', jsonb_build_object('old', OLD.first_name, 'new', NEW.first_name)); END IF;
				IF OLD.last_name IS DISTINCT FROM NEW.last_name THEN changes_json := changes_json || jsonb_build_object('last_name', jsonb_build_object('old', OLD.last_name, 'new', NEW.last_name)); END IF;
				IF OLD.full_name IS DISTINCT FROM NEW.full_name THEN changes_json := changes_json || jsonb_build_object('full_name', jsonb_build_object('old', OLD.full_name, 'new', NEW.full_name)); END IF;
				IF OLD.phone IS DISTINCT FROM NEW.phone THEN changes_json := changes_json || jsonb_build_object('phone', jsonb_build_object('old', OLD.phone, 'new', NEW.phone)); END IF;
				IF OLD.address_line_1 IS DISTINCT FROM NEW.address_line_1 THEN changes_json := changes_json || jsonb_build_object('address_line_1', jsonb_build_object('old', OLD.address_line_1, 'new', NEW.address_line_1)); END IF;
				IF OLD.address_line_2 IS DISTINCT FROM NEW.address_line_2 THEN changes_json := changes_json || jsonb_build_object('address_line_2', jsonb_build_object('old', OLD.address_line_2, 'new', NEW.address_line_2)); END IF;
				IF OLD.country IS DISTINCT FROM NEW.country THEN changes_json := changes_json || jsonb_build_object('country', jsonb_build_object('old', OLD.country, 'new', NEW.country)); END IF;
				IF OLD.postcode IS DISTINCT FROM NEW.postcode THEN changes_json := changes_json || jsonb_build_object('postcode', jsonb_build_object('old', OLD.postcode, 'new', NEW.postcode)); END IF;
				IF OLD.state IS DISTINCT FROM NEW.state THEN changes_json := changes_json || jsonb_build_object('state', jsonb_build_object('old', OLD.state, 'new', NEW.state)); END IF;
				IF OLD.job_title IS DISTINCT FROM NEW.job_title THEN changes_json := changes_json || jsonb_build_object('job_title', jsonb_build_object('old', OLD.job_title, 'new', NEW.job_title)); END IF;
				IF OLD.custom_string_1 IS DISTINCT FROM NEW.custom_string_1 THEN changes_json := changes_json || jsonb_build_object('custom_string_1', jsonb_build_object('old', OLD.custom_string_1, 'new', NEW.custom_string_1)); END IF;
				IF OLD.custom_string_2 IS DISTINCT FROM NEW.custom_string_2 THEN changes_json := changes_json || jsonb_build_object('custom_string_2', jsonb_build_object('old', OLD.custom_string_2, 'new', NEW.custom_string_2)); END IF;
				IF OLD.custom_string_3 IS DISTINCT FROM NEW.custom_string_3 THEN changes_json := changes_json || jsonb_build_object('custom_string_3', jsonb_build_object('old', OLD.custom_string_3, 'new', NEW.custom_string_3)); END IF;
				IF OLD.custom_string_4 IS DISTINCT FROM NEW.custom_string_4 THEN changes_json := changes_json || jsonb_build_object('custom_string_4', jsonb_build_object('old', OLD.custom_string_4, 'new', NEW.custom_string_4)); END IF;
				IF OLD.custom_string_5 IS DISTINCT FROM NEW.custom_string_5 THEN changes_json := changes_json || jsonb_build_object('custom_string_5', jsonb_build_object('old', OLD.custom_string_5, 'new', NEW.custom_string_5)); END IF;
				IF OLD.custom_number_1 IS DISTINCT FROM NEW.custom_number_1 THEN changes_json := changes_json || jsonb_build_object('custom_number_1', jsonb_build_object('old', OLD.custom_number_1, 'new', NEW.custom_number_1)); END IF;
				IF OLD.custom_number_2 IS DISTINCT FROM NEW.custom_number_2 THEN changes_json := changes_json || jsonb_build_object('custom_number_2', jsonb_build_object('old', OLD.custom_number_2, 'new', NEW.custom_number_2)); END IF;
				IF OLD.custom_number_3 IS DISTINCT FROM NEW.custom_number_3 THEN changes_json := changes_json || jsonb_build_object('custom_number_3', jsonb_build_object('old', OLD.custom_number_3, 'new', NEW.custom_number_3)); END IF;
				IF OLD.custom_number_4 IS DISTINCT FROM NEW.custom_number_4 THEN changes_json := changes_json || jsonb_build_object('custom_number_4', jsonb_build_object('old', OLD.custom_number_4, 'new', NEW.custom_number_4)); END IF;
				IF OLD.custom_number_5 IS DISTINCT FROM NEW.custom_number_5 THEN changes_json := changes_json || jsonb_build_object('custom_number_5', jsonb_build_object('old', OLD.custom_number_5, 'new', NEW.custom_number_5)); END IF;
				IF OLD.custom_datetime_1 IS DISTINCT FROM NEW.custom_datetime_1 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_1', jsonb_build_object('old', OLD.custom_datetime_1, 'new', NEW.custom_datetime_1)); END IF;
				IF OLD.custom_datetime_2 IS DISTINCT FROM NEW.custom_datetime_2 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_2', jsonb_build_object('old', OLD.custom_datetime_2, 'new', NEW.custom_datetime_2)); END IF;
				IF OLD.custom_datetime_3 IS DISTINCT FROM NEW.custom_datetime_3 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_3', jsonb_build_object('old', OLD.custom_datetime_3, 'new', NEW.custom_datetime_3)); END IF;
				IF OLD.custom_datetime_4 IS DISTINCT FROM NEW.custom_datetime_4 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_4', jsonb_build_object('old', OLD.custom_datetime_4, 'new', NEW.custom_datetime_4)); END IF;
				IF OLD.custom_datetime_5 IS DISTINCT FROM NEW.custom_datetime_5 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_5', jsonb_build_object('old', OLD.custom_datetime_5, 'new', NEW.custom_datetime_5)); END IF;
				IF OLD.custom_json_1 IS DISTINCT FROM NEW.custom_json_1 THEN changes_json := changes_json || jsonb_build_object('custom_json_1', jsonb_build_object('old', OLD.custom_json_1, 'new', NEW.custom_json_1)); END IF;
				IF OLD.custom_json_2 IS DISTINCT FROM NEW.custom_json_2 THEN changes_json := changes_json || jsonb_build_object('custom_json_2', jsonb_build_object('old', OLD.custom_json_2, 'new', NEW.custom_json_2)); END IF;
				IF OLD.custom_json_3 IS DISTINCT FROM NEW.custom_json_3 THEN changes_json := changes_json || jsonb_build_object('custom_json_3', jsonb_build_object('old', OLD.custom_json_3, 'new', NEW.custom_json_3)); END IF;
				IF OLD.custom_json_4 IS DISTINCT FROM NEW.custom_json_4 THEN changes_json := changes_json || jsonb_build_object('custom_json_4', jsonb_build_object('old', OLD.custom_json_4, 'new', NEW.custom_json_4)); END IF;
				IF OLD.custom_json_5 IS DISTINCT FROM NEW.custom_json_5 THEN changes_json := changes_json || jsonb_build_object('custom_json_5', jsonb_build_object('old', OLD.custom_json_5, 'new', NEW.custom_json_5)); END IF;
				IF changes_json = '{}'::jsonb THEN RETURN NEW; END IF;
			END IF;
		IF TG_OP = 'INSERT' THEN
			INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at)
			VALUES (NEW.email, op, 'contact', 'contact.created', changes_json, NEW.created_at);
		ELSE
			INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at)
			VALUES (NEW.email, op, 'contact', 'contact.updated', changes_json, NEW.updated_at);
		END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;`,
		// Contact list changes trigger function - uses semantic event kinds (list.subscribed, list.confirmed, etc.)
		`CREATE OR REPLACE FUNCTION track_contact_list_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			op VARCHAR(20);
			kind_value VARCHAR(50);
		BEGIN
			IF TG_OP = 'INSERT' THEN
				op := 'insert';

				-- Map initial status to semantic event kind (dotted format)
				kind_value := CASE NEW.status
					WHEN 'active' THEN 'list.subscribed'
					WHEN 'pending' THEN 'list.pending'
					WHEN 'unsubscribed' THEN 'list.unsubscribed'
					WHEN 'bounced' THEN 'list.bounced'
					WHEN 'complained' THEN 'list.complained'
					ELSE 'list.subscribed'
				END;

				changes_json := jsonb_build_object(
					'list_id', jsonb_build_object('new', NEW.list_id),
					'status', jsonb_build_object('new', NEW.status)
				);

			ELSIF TG_OP = 'UPDATE' THEN
				op := 'update';

				-- Handle soft delete
				IF OLD.deleted_at IS DISTINCT FROM NEW.deleted_at AND NEW.deleted_at IS NOT NULL THEN
					kind_value := 'list.removed';
					changes_json := jsonb_build_object(
						'deleted_at', jsonb_build_object('old', OLD.deleted_at, 'new', NEW.deleted_at)
					);

				-- Handle status transitions
				ELSIF OLD.status IS DISTINCT FROM NEW.status THEN
					kind_value := CASE
						WHEN OLD.status = 'pending' AND NEW.status = 'active' THEN 'list.confirmed'
						WHEN OLD.status IN ('unsubscribed', 'bounced', 'complained') AND NEW.status = 'active' THEN 'list.resubscribed'
						WHEN NEW.status = 'unsubscribed' THEN 'list.unsubscribed'
						WHEN NEW.status = 'bounced' THEN 'list.bounced'
						WHEN NEW.status = 'complained' THEN 'list.complained'
						WHEN NEW.status = 'pending' THEN 'list.pending'
						WHEN NEW.status = 'active' THEN 'list.subscribed'
						ELSE 'list.status_changed'
					END;

					changes_json := jsonb_build_object(
						'status', jsonb_build_object('old', OLD.status, 'new', NEW.status)
					);
				ELSE
					RETURN NEW;
				END IF;
			END IF;

			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
			VALUES (NEW.email, op, 'contact_list', kind_value, NEW.list_id, changes_json, CURRENT_TIMESTAMP);

			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;`,
		// Message history changes trigger function
		`CREATE OR REPLACE FUNCTION track_message_history_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			op VARCHAR(20);
			kind_value VARCHAR(50);
		BEGIN
			IF TG_OP = 'INSERT' THEN
				op := 'insert';
				changes_json := jsonb_build_object('template_id', jsonb_build_object('new', NEW.template_id), 'template_version', jsonb_build_object('new', NEW.template_version), 'channel', jsonb_build_object('new', NEW.channel), 'broadcast_id', jsonb_build_object('new', NEW.broadcast_id), 'sent_at', jsonb_build_object('new', NEW.sent_at));
				INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
				VALUES (NEW.contact_email, op, 'message_history', 'insert_message_history', NEW.id, changes_json, NEW.updated_at);
			ELSIF TG_OP = 'UPDATE' THEN
				op := 'update';
				-- Handle engagement events separately with specific kinds
				IF OLD.opened_at IS DISTINCT FROM NEW.opened_at AND NEW.opened_at IS NOT NULL THEN
					changes_json := jsonb_build_object('opened_at', jsonb_build_object('old', OLD.opened_at, 'new', NEW.opened_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
					VALUES (NEW.contact_email, op, 'message_history', 'open_' || NEW.channel, NEW.id, changes_json, NEW.updated_at);
				END IF;
				IF OLD.clicked_at IS DISTINCT FROM NEW.clicked_at AND NEW.clicked_at IS NOT NULL THEN
					changes_json := jsonb_build_object('clicked_at', jsonb_build_object('old', OLD.clicked_at, 'new', NEW.clicked_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
					VALUES (NEW.contact_email, op, 'message_history', 'click_' || NEW.channel, NEW.id, changes_json, NEW.updated_at);
				END IF;
				IF OLD.bounced_at IS DISTINCT FROM NEW.bounced_at AND NEW.bounced_at IS NOT NULL THEN
					changes_json := jsonb_build_object('bounced_at', jsonb_build_object('old', OLD.bounced_at, 'new', NEW.bounced_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
					VALUES (NEW.contact_email, op, 'message_history', 'bounce_' || NEW.channel, NEW.id, changes_json, NEW.updated_at);
				END IF;
				IF OLD.complained_at IS DISTINCT FROM NEW.complained_at AND NEW.complained_at IS NOT NULL THEN
					changes_json := jsonb_build_object('complained_at', jsonb_build_object('old', OLD.complained_at, 'new', NEW.complained_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
					VALUES (NEW.contact_email, op, 'message_history', 'complain_' || NEW.channel, NEW.id, changes_json, NEW.updated_at);
				END IF;
				IF OLD.unsubscribed_at IS DISTINCT FROM NEW.unsubscribed_at AND NEW.unsubscribed_at IS NOT NULL THEN
					changes_json := jsonb_build_object('unsubscribed_at', jsonb_build_object('old', OLD.unsubscribed_at, 'new', NEW.unsubscribed_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
					VALUES (NEW.contact_email, op, 'message_history', 'unsubscribe_' || NEW.channel, NEW.id, changes_json, NEW.updated_at);
				END IF;
				-- Handle other updates (delivered, failed, status_info) as generic updates
				changes_json := '{}'::jsonb;
				IF OLD.delivered_at IS DISTINCT FROM NEW.delivered_at THEN changes_json := changes_json || jsonb_build_object('delivered_at', jsonb_build_object('old', OLD.delivered_at, 'new', NEW.delivered_at)); END IF;
				IF OLD.failed_at IS DISTINCT FROM NEW.failed_at THEN changes_json := changes_json || jsonb_build_object('failed_at', jsonb_build_object('old', OLD.failed_at, 'new', NEW.failed_at)); END IF;
				IF OLD.status_info IS DISTINCT FROM NEW.status_info THEN changes_json := changes_json || jsonb_build_object('status_info', jsonb_build_object('old', OLD.status_info, 'new', NEW.status_info)); END IF;
				IF changes_json != '{}'::jsonb THEN
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
					VALUES (NEW.contact_email, op, 'message_history', 'update_message_history', NEW.id, changes_json, NEW.updated_at);
				END IF;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;`,
		// Inbound webhook event changes trigger function
		`CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			entity_id_value VARCHAR(255);
		BEGIN
			-- Use message_id if available, otherwise use inbound webhook event id
			entity_id_value := COALESCE(NEW.message_id, NEW.id::text);

			changes_json := jsonb_build_object('type', jsonb_build_object('new', NEW.type), 'source', jsonb_build_object('new', NEW.source));
			IF NEW.bounce_type IS NOT NULL AND NEW.bounce_type != '' THEN changes_json := changes_json || jsonb_build_object('bounce_type', jsonb_build_object('new', NEW.bounce_type)); END IF;
			IF NEW.bounce_category IS NOT NULL AND NEW.bounce_category != '' THEN changes_json := changes_json || jsonb_build_object('bounce_category', jsonb_build_object('new', NEW.bounce_category)); END IF;
			IF NEW.bounce_diagnostic IS NOT NULL AND NEW.bounce_diagnostic != '' THEN changes_json := changes_json || jsonb_build_object('bounce_diagnostic', jsonb_build_object('new', NEW.bounce_diagnostic)); END IF;
			IF NEW.complaint_feedback_type IS NOT NULL AND NEW.complaint_feedback_type != '' THEN changes_json := changes_json || jsonb_build_object('complaint_feedback_type', jsonb_build_object('new', NEW.complaint_feedback_type)); END IF;
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
			VALUES (NEW.recipient_email, 'insert', 'inbound_webhook_event', 'insert_inbound_webhook_event', entity_id_value, changes_json, CURRENT_TIMESTAMP);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;`,
		// Contact segment changes trigger function
		`CREATE OR REPLACE FUNCTION track_contact_segment_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			op VARCHAR(20);
			kind_value VARCHAR(50);
		BEGIN
			IF TG_OP = 'INSERT' THEN
				op := 'insert';
				kind_value := 'segment.joined';
				changes_json := jsonb_build_object('segment_id', jsonb_build_object('new', NEW.segment_id), 'version', jsonb_build_object('new', NEW.version), 'matched_at', jsonb_build_object('new', NEW.matched_at));
			ELSIF TG_OP = 'DELETE' THEN
				op := 'delete';
				kind_value := 'segment.left';
				changes_json := jsonb_build_object('segment_id', jsonb_build_object('old', OLD.segment_id), 'version', jsonb_build_object('old', OLD.version));
				INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
				VALUES (OLD.email, op, 'contact_segment', kind_value, OLD.segment_id, changes_json, CURRENT_TIMESTAMP);
				RETURN OLD;
			END IF;
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
			VALUES (NEW.email, op, 'contact_segment', kind_value, NEW.segment_id, changes_json, CURRENT_TIMESTAMP);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;`,
		// Contact timeline queue trigger function
		`CREATE OR REPLACE FUNCTION queue_contact_for_segment_recomputation()
		RETURNS TRIGGER AS $$
		BEGIN
			-- Queue the contact for segment recomputation
			INSERT INTO contact_segment_queue (email, queued_at)
			VALUES (NEW.email, CURRENT_TIMESTAMP)
			ON CONFLICT (email) DO UPDATE SET queued_at = EXCLUDED.queued_at;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;`,
		// Contact list status update on bounce/complaint trigger function
		`CREATE OR REPLACE FUNCTION update_contact_lists_on_status_change()
		RETURNS TRIGGER AS $$
		BEGIN
			-- Handle complaint events (worst status - can upgrade from any status)
			IF NEW.complained_at IS NOT NULL AND OLD.complained_at IS NULL THEN
				IF NEW.list_id IS NOT NULL THEN
					UPDATE contact_lists
					SET status = 'complained',
						updated_at = NEW.complained_at
					WHERE email = NEW.contact_email
					AND list_id = NEW.list_id
					AND status != 'complained';
				END IF;
			END IF;

			-- Handle bounce events (ONLY HARD BOUNCES - can only update if not already complained or bounced)
			-- Note: Application layer should only set bounced_at for hard/permanent bounces
			IF NEW.bounced_at IS NOT NULL AND OLD.bounced_at IS NULL THEN
				IF NEW.list_id IS NOT NULL THEN
					UPDATE contact_lists
					SET status = 'bounced',
						updated_at = NEW.bounced_at
					WHERE email = NEW.contact_email
					AND list_id = NEW.list_id
					AND status NOT IN ('complained', 'bounced');
				END IF;
			END IF;

			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;`,
		// Custom event changes trigger function
		`CREATE OR REPLACE FUNCTION track_custom_event_timeline()
		RETURNS TRIGGER AS $$
		DECLARE
			timeline_operation TEXT;
			changes_json JSONB;
			property_key TEXT;
			property_diff JSONB;
			kind_value TEXT;
		BEGIN
			IF TG_OP = 'INSERT' THEN
				timeline_operation := 'insert';
				kind_value := 'custom_event.' || NEW.event_name;
				changes_json := jsonb_build_object(
					'event_name', jsonb_build_object('new', NEW.event_name),
					'external_id', jsonb_build_object('new', NEW.external_id)
				);
				-- Add goal fields if present
				IF NEW.goal_type IS NOT NULL THEN
					changes_json := changes_json || jsonb_build_object('goal_type', jsonb_build_object('new', NEW.goal_type));
				END IF;
				IF NEW.goal_value IS NOT NULL THEN
					changes_json := changes_json || jsonb_build_object('goal_value', jsonb_build_object('new', NEW.goal_value));
				END IF;
				IF NEW.goal_name IS NOT NULL THEN
					changes_json := changes_json || jsonb_build_object('goal_name', jsonb_build_object('new', NEW.goal_name));
				END IF;
			ELSIF TG_OP = 'UPDATE' THEN
				timeline_operation := 'update';
				kind_value := 'custom_event.' || NEW.event_name;
				property_diff := '{}'::jsonb;
				FOR property_key IN
					SELECT DISTINCT key
					FROM (
						SELECT key FROM jsonb_object_keys(OLD.properties) AS key
						UNION
						SELECT key FROM jsonb_object_keys(NEW.properties) AS key
					) AS all_keys
				LOOP
					IF (OLD.properties->property_key) IS DISTINCT FROM (NEW.properties->property_key) THEN
						property_diff := property_diff || jsonb_build_object(
							property_key,
							jsonb_build_object(
								'old', OLD.properties->property_key,
								'new', NEW.properties->property_key
							)
						);
					END IF;
				END LOOP;
				changes_json := jsonb_build_object(
					'properties', property_diff,
					'occurred_at', jsonb_build_object(
						'old', OLD.occurred_at,
						'new', NEW.occurred_at
					)
				);
				-- Add goal fields if changed
				IF OLD.goal_type IS DISTINCT FROM NEW.goal_type THEN
					changes_json := changes_json || jsonb_build_object('goal_type', jsonb_build_object('old', OLD.goal_type, 'new', NEW.goal_type));
				END IF;
				IF OLD.goal_value IS DISTINCT FROM NEW.goal_value THEN
					changes_json := changes_json || jsonb_build_object('goal_value', jsonb_build_object('old', OLD.goal_value, 'new', NEW.goal_value));
				END IF;
				IF OLD.goal_name IS DISTINCT FROM NEW.goal_name THEN
					changes_json := changes_json || jsonb_build_object('goal_name', jsonb_build_object('old', OLD.goal_name, 'new', NEW.goal_name));
				END IF;
			END IF;
			INSERT INTO contact_timeline (
				email, operation, entity_type, kind, entity_id, changes, created_at
			) VALUES (
				NEW.email, timeline_operation, 'custom_event', kind_value,
				NEW.external_id, changes_json, NEW.occurred_at
			);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;`,
		// Create triggers
		`DROP TRIGGER IF EXISTS contact_changes_trigger ON contacts`,
		`CREATE TRIGGER contact_changes_trigger AFTER INSERT OR UPDATE ON contacts FOR EACH ROW EXECUTE FUNCTION track_contact_changes()`,
		`DROP TRIGGER IF EXISTS contact_list_changes_trigger ON contact_lists`,
		`CREATE TRIGGER contact_list_changes_trigger AFTER INSERT OR UPDATE ON contact_lists FOR EACH ROW EXECUTE FUNCTION track_contact_list_changes()`,
		`DROP TRIGGER IF EXISTS message_history_changes_trigger ON message_history`,
		`CREATE TRIGGER message_history_changes_trigger AFTER INSERT OR UPDATE ON message_history FOR EACH ROW EXECUTE FUNCTION track_message_history_changes()`,
		`DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger ON inbound_webhook_events`,
		`CREATE TRIGGER inbound_webhook_event_changes_trigger AFTER INSERT ON inbound_webhook_events FOR EACH ROW EXECUTE FUNCTION track_inbound_webhook_event_changes()`,
		`DROP TRIGGER IF EXISTS contact_segment_changes_trigger ON contact_segments`,
		`CREATE TRIGGER contact_segment_changes_trigger AFTER INSERT OR DELETE ON contact_segments FOR EACH ROW EXECUTE FUNCTION track_contact_segment_changes()`,
		`DROP TRIGGER IF EXISTS contact_timeline_queue_trigger ON contact_timeline`,
		`CREATE TRIGGER contact_timeline_queue_trigger AFTER INSERT ON contact_timeline FOR EACH ROW EXECUTE FUNCTION queue_contact_for_segment_recomputation()`,
		`DROP TRIGGER IF EXISTS message_history_status_trigger ON message_history`,
		`CREATE TRIGGER message_history_status_trigger AFTER UPDATE ON message_history FOR EACH ROW EXECUTE FUNCTION update_contact_lists_on_status_change()`,
		`DROP TRIGGER IF EXISTS custom_event_timeline_trigger ON custom_events`,
		`CREATE TRIGGER custom_event_timeline_trigger AFTER INSERT OR UPDATE ON custom_events FOR EACH ROW EXECUTE FUNCTION track_custom_event_timeline()`,
		// Webhook trigger functions for outgoing webhooks
		// Trigger 1: contacts table - contact.created, contact.updated, contact.deleted
		`CREATE OR REPLACE FUNCTION webhook_contacts_trigger()
		RETURNS TRIGGER AS $$
		DECLARE
			sub RECORD;
			event_kind VARCHAR(50);
			payload JSONB;
			contact_record RECORD;
		BEGIN
			-- Determine event kind and which record to use
			IF TG_OP = 'INSERT' THEN
				event_kind := 'contact.created';
				contact_record := NEW;
			ELSIF TG_OP = 'UPDATE' THEN
				event_kind := 'contact.updated';
				contact_record := NEW;
				-- Skip if nothing changed (compare all relevant fields)
				IF NEW.external_id IS NOT DISTINCT FROM OLD.external_id AND
				   NEW.timezone IS NOT DISTINCT FROM OLD.timezone AND
				   NEW.language IS NOT DISTINCT FROM OLD.language AND
				   NEW.first_name IS NOT DISTINCT FROM OLD.first_name AND
				   NEW.last_name IS NOT DISTINCT FROM OLD.last_name AND
				   NEW.full_name IS NOT DISTINCT FROM OLD.full_name AND
				   NEW.phone IS NOT DISTINCT FROM OLD.phone AND
				   NEW.address_line_1 IS NOT DISTINCT FROM OLD.address_line_1 AND
				   NEW.address_line_2 IS NOT DISTINCT FROM OLD.address_line_2 AND
				   NEW.country IS NOT DISTINCT FROM OLD.country AND
				   NEW.postcode IS NOT DISTINCT FROM OLD.postcode AND
				   NEW.state IS NOT DISTINCT FROM OLD.state AND
				   NEW.job_title IS NOT DISTINCT FROM OLD.job_title AND
				   NEW.custom_string_1 IS NOT DISTINCT FROM OLD.custom_string_1 AND
				   NEW.custom_string_2 IS NOT DISTINCT FROM OLD.custom_string_2 AND
				   NEW.custom_string_3 IS NOT DISTINCT FROM OLD.custom_string_3 AND
				   NEW.custom_string_4 IS NOT DISTINCT FROM OLD.custom_string_4 AND
				   NEW.custom_string_5 IS NOT DISTINCT FROM OLD.custom_string_5 AND
				   NEW.custom_number_1 IS NOT DISTINCT FROM OLD.custom_number_1 AND
				   NEW.custom_number_2 IS NOT DISTINCT FROM OLD.custom_number_2 AND
				   NEW.custom_number_3 IS NOT DISTINCT FROM OLD.custom_number_3 AND
				   NEW.custom_number_4 IS NOT DISTINCT FROM OLD.custom_number_4 AND
				   NEW.custom_number_5 IS NOT DISTINCT FROM OLD.custom_number_5 AND
				   NEW.custom_datetime_1 IS NOT DISTINCT FROM OLD.custom_datetime_1 AND
				   NEW.custom_datetime_2 IS NOT DISTINCT FROM OLD.custom_datetime_2 AND
				   NEW.custom_datetime_3 IS NOT DISTINCT FROM OLD.custom_datetime_3 AND
				   NEW.custom_datetime_4 IS NOT DISTINCT FROM OLD.custom_datetime_4 AND
				   NEW.custom_datetime_5 IS NOT DISTINCT FROM OLD.custom_datetime_5 AND
				   NEW.custom_json_1 IS NOT DISTINCT FROM OLD.custom_json_1 AND
				   NEW.custom_json_2 IS NOT DISTINCT FROM OLD.custom_json_2 AND
				   NEW.custom_json_3 IS NOT DISTINCT FROM OLD.custom_json_3 AND
				   NEW.custom_json_4 IS NOT DISTINCT FROM OLD.custom_json_4 AND
				   NEW.custom_json_5 IS NOT DISTINCT FROM OLD.custom_json_5 THEN
					RETURN NEW;
				END IF;
			ELSIF TG_OP = 'DELETE' THEN
				event_kind := 'contact.deleted';
				contact_record := OLD;
			ELSE
				RETURN COALESCE(NEW, OLD);
			END IF;

			-- Build payload with full contact object
			payload := jsonb_build_object(
				'contact', to_jsonb(contact_record)
			);

			-- Insert webhook deliveries for matching subscriptions
			FOR sub IN
				SELECT id FROM webhook_subscriptions
				WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
			LOOP
				INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
				VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
			END LOOP;
			RETURN COALESCE(NEW, OLD);
		END;
		$$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS webhook_contacts ON contacts`,
		`CREATE TRIGGER webhook_contacts AFTER INSERT OR UPDATE OR DELETE ON contacts FOR EACH ROW EXECUTE FUNCTION webhook_contacts_trigger()`,
		// Trigger 2: contact_lists table - list events
		`CREATE OR REPLACE FUNCTION webhook_contact_lists_trigger()
		RETURNS TRIGGER AS $$
		DECLARE
			sub RECORD;
			event_kind VARCHAR(50);
			payload JSONB;
			list_name VARCHAR(255);
		BEGIN
			-- Get list name for payload enrichment
			SELECT name INTO list_name FROM lists WHERE id = NEW.list_id;

			-- Determine event kind based on status transitions
			IF TG_OP = 'INSERT' THEN
				CASE NEW.status
					WHEN 'active' THEN event_kind := 'list.subscribed';
					WHEN 'pending' THEN event_kind := 'list.pending';
					WHEN 'unsubscribed' THEN event_kind := 'list.unsubscribed';
					WHEN 'bounced' THEN event_kind := 'list.bounced';
					WHEN 'complained' THEN event_kind := 'list.complained';
					ELSE RETURN NEW;
				END CASE;
			ELSIF TG_OP = 'UPDATE' THEN
				-- Detect status transitions
				IF NEW.status IS DISTINCT FROM OLD.status THEN
					IF OLD.status = 'pending' AND NEW.status = 'active' THEN
						event_kind := 'list.confirmed';
					ELSIF OLD.status IN ('unsubscribed', 'bounced', 'complained') AND NEW.status = 'active' THEN
						event_kind := 'list.resubscribed';
					ELSIF NEW.status = 'unsubscribed' THEN
						event_kind := 'list.unsubscribed';
					ELSIF NEW.status = 'bounced' THEN
						event_kind := 'list.bounced';
					ELSIF NEW.status = 'complained' THEN
						event_kind := 'list.complained';
					ELSE
						RETURN NEW;
					END IF;
				ELSIF NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL THEN
					event_kind := 'list.removed';
				ELSE
					RETURN NEW;
				END IF;
			ELSE
				RETURN NEW;
			END IF;

			-- Build payload
			payload := jsonb_build_object(
				'email', NEW.email,
				'list_id', NEW.list_id,
				'list_name', list_name,
				'status', NEW.status,
				'previous_status', CASE WHEN TG_OP = 'UPDATE' THEN OLD.status ELSE NULL END
			);

			-- Insert webhook deliveries for matching subscriptions
			FOR sub IN
				SELECT id FROM webhook_subscriptions
				WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
			LOOP
				INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
				VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
			END LOOP;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS webhook_contact_lists ON contact_lists`,
		`CREATE TRIGGER webhook_contact_lists AFTER INSERT OR UPDATE ON contact_lists FOR EACH ROW EXECUTE FUNCTION webhook_contact_lists_trigger()`,
		// Trigger 3: contact_segments table - segment events
		`CREATE OR REPLACE FUNCTION webhook_contact_segments_trigger()
		RETURNS TRIGGER AS $$
		DECLARE
			sub RECORD;
			event_kind VARCHAR(50);
			payload JSONB;
			segment_name VARCHAR(255);
			contact_email VARCHAR(255);
		BEGIN
			-- Get segment name for payload
			SELECT name INTO segment_name FROM segments WHERE id = COALESCE(NEW.segment_id, OLD.segment_id);
			-- contact_segments uses email directly as the key
			contact_email := COALESCE(NEW.email, OLD.email);

			-- Determine event kind
			IF TG_OP = 'INSERT' THEN
				event_kind := 'segment.joined';
			ELSIF TG_OP = 'DELETE' THEN
				event_kind := 'segment.left';
			ELSE
				RETURN NEW;
			END IF;

			-- Build payload
			payload := jsonb_build_object(
				'email', contact_email,
				'segment_id', COALESCE(NEW.segment_id, OLD.segment_id),
				'segment_name', segment_name
			);

			-- Insert webhook deliveries for matching subscriptions
			FOR sub IN
				SELECT id FROM webhook_subscriptions
				WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
			LOOP
				INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
				VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
			END LOOP;

			IF TG_OP = 'DELETE' THEN
				RETURN OLD;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS webhook_contact_segments ON contact_segments`,
		`CREATE TRIGGER webhook_contact_segments AFTER INSERT OR DELETE ON contact_segments FOR EACH ROW EXECUTE FUNCTION webhook_contact_segments_trigger()`,
		// Trigger 4: message_history table - email events
		`CREATE OR REPLACE FUNCTION webhook_message_history_trigger()
		RETURNS TRIGGER AS $$
		DECLARE
			sub RECORD;
			event_kind VARCHAR(50);
			event_timestamp TIMESTAMPTZ;
			payload JSONB;
		BEGIN
			-- Detect which email event occurred
			IF TG_OP = 'INSERT' THEN
				event_kind := 'email.sent';
				event_timestamp := NEW.sent_at;
			ELSIF TG_OP = 'UPDATE' THEN
				IF NEW.delivered_at IS NOT NULL AND OLD.delivered_at IS NULL THEN
					event_kind := 'email.delivered';
					event_timestamp := NEW.delivered_at;
				ELSIF NEW.opened_at IS NOT NULL AND OLD.opened_at IS NULL THEN
					event_kind := 'email.opened';
					event_timestamp := NEW.opened_at;
				ELSIF NEW.clicked_at IS NOT NULL AND OLD.clicked_at IS NULL THEN
					event_kind := 'email.clicked';
					event_timestamp := NEW.clicked_at;
				ELSIF NEW.bounced_at IS NOT NULL AND OLD.bounced_at IS NULL THEN
					event_kind := 'email.bounced';
					event_timestamp := NEW.bounced_at;
				ELSIF NEW.complained_at IS NOT NULL AND OLD.complained_at IS NULL THEN
					event_kind := 'email.complained';
					event_timestamp := NEW.complained_at;
				ELSIF NEW.unsubscribed_at IS NOT NULL AND OLD.unsubscribed_at IS NULL THEN
					event_kind := 'email.unsubscribed';
					event_timestamp := NEW.unsubscribed_at;
				ELSE
					RETURN NEW;
				END IF;
			ELSE
				RETURN NEW;
			END IF;

			-- Build rich payload with full message context
			payload := jsonb_build_object(
				'email', NEW.contact_email,
				'message_id', NEW.id,
				'template_id', NEW.template_id,
				'broadcast_id', NEW.broadcast_id,
				'list_id', NEW.list_id,
				'channel', NEW.channel,
				'event_timestamp', event_timestamp
			);

			-- Insert webhook deliveries for matching subscriptions
			FOR sub IN
				SELECT id FROM webhook_subscriptions
				WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
			LOOP
				INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
				VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
			END LOOP;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS webhook_message_history ON message_history`,
		`CREATE TRIGGER webhook_message_history AFTER INSERT OR UPDATE ON message_history FOR EACH ROW EXECUTE FUNCTION webhook_message_history_trigger()`,
		// Trigger 5: custom_events table - custom events with filtering
		`CREATE OR REPLACE FUNCTION webhook_custom_events_trigger()
		RETURNS TRIGGER AS $$
		DECLARE
			sub RECORD;
			custom_filters JSONB;
			should_deliver BOOLEAN;
			payload JSONB;
			event_kind VARCHAR(50);
			subscribed_event_type VARCHAR(50);
		BEGIN
			-- Determine event kind based on operation and soft-delete status
			IF TG_OP = 'INSERT' THEN
				-- New record - check if it's being created as deleted
				IF NEW.deleted_at IS NOT NULL THEN
					event_kind := 'custom_event.deleted';
					subscribed_event_type := 'custom_event.deleted';
				ELSE
					event_kind := 'custom_event.created';
					subscribed_event_type := 'custom_event.created';
				END IF;
			ELSIF TG_OP = 'UPDATE' THEN
				-- Check for soft-delete: was not deleted, now is deleted
				IF (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL) THEN
					event_kind := 'custom_event.deleted';
					subscribed_event_type := 'custom_event.deleted';
				-- Check for restore: was deleted, now is not deleted
				ELSIF (OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL) THEN
					event_kind := 'custom_event.created';
					subscribed_event_type := 'custom_event.created';
				-- Regular update (skip if record is deleted)
				ELSIF NEW.deleted_at IS NULL THEN
					event_kind := 'custom_event.updated';
					subscribed_event_type := 'custom_event.updated';
				ELSE
					-- Record is deleted and staying deleted, skip
					RETURN NEW;
				END IF;
			ELSE
				RETURN NEW;
			END IF;

			-- Build payload with full custom_event object
			payload := jsonb_build_object('custom_event', to_jsonb(NEW));

			-- Find matching subscriptions with the correct event type
			FOR sub IN
				SELECT id, settings FROM webhook_subscriptions
				WHERE enabled = true AND subscribed_event_type = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
			LOOP
				should_deliver := true;
				custom_filters := sub.settings->'custom_event_filters';

				-- Apply goal_types filter if specified
				IF custom_filters IS NOT NULL AND custom_filters ? 'goal_types'
				   AND jsonb_array_length(custom_filters->'goal_types') > 0 THEN
					IF NEW.goal_type IS NULL OR NOT (NEW.goal_type = ANY(
						SELECT jsonb_array_elements_text(custom_filters->'goal_types')
					)) THEN
						should_deliver := false;
					END IF;
				END IF;

				-- Apply event_names filter if specified
				IF should_deliver AND custom_filters IS NOT NULL AND custom_filters ? 'event_names'
				   AND jsonb_array_length(custom_filters->'event_names') > 0 THEN
					IF NOT (NEW.event_name = ANY(
						SELECT jsonb_array_elements_text(custom_filters->'event_names')
					)) THEN
						should_deliver := false;
					END IF;
				END IF;

				IF should_deliver THEN
					INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
					VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
				END IF;
			END LOOP;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS webhook_custom_events ON custom_events`,
		`CREATE TRIGGER webhook_custom_events AFTER INSERT OR UPDATE ON custom_events FOR EACH ROW EXECUTE FUNCTION webhook_custom_events_trigger()`,
		// Automation enroll contact function
		`CREATE OR REPLACE FUNCTION automation_enroll_contact(
			p_automation_id VARCHAR(36),
			p_contact_email VARCHAR(255),
			p_root_node_id VARCHAR(36),
			p_frequency VARCHAR(20)
		) RETURNS VOID AS $$
		DECLARE
			v_already_triggered BOOLEAN;
			v_new_id VARCHAR(36);
		BEGIN
			-- 1. For "once" frequency, check if already triggered
			IF p_frequency = 'once' THEN
				SELECT EXISTS(
					SELECT 1 FROM automation_trigger_log
					WHERE automation_id = p_automation_id
					AND contact_email = p_contact_email
				) INTO v_already_triggered;

				IF v_already_triggered THEN
					RETURN;  -- Already triggered for this contact, skip
				END IF;

				-- Record trigger for deduplication
				INSERT INTO automation_trigger_log (id, automation_id, contact_email, triggered_at)
				VALUES (gen_random_uuid()::text, p_automation_id, p_contact_email, NOW())
				ON CONFLICT (automation_id, contact_email) DO NOTHING;
			END IF;

			-- 2. Generate new ID for contact_automation
			v_new_id := gen_random_uuid()::text;

			-- 3. Enroll contact in automation
			INSERT INTO contact_automations (
				id, automation_id, contact_email, current_node_id,
				status, entered_at, scheduled_at
			) VALUES (
				v_new_id,
				p_automation_id,
				p_contact_email,
				p_root_node_id,
				'active',
				NOW(),
				NOW()
			);

			-- 4. Increment enrolled stat
			UPDATE automations
			SET stats = jsonb_set(
				COALESCE(stats, '{}'::jsonb),
				'{enrolled}',
				to_jsonb(COALESCE((stats->>'enrolled')::int, 0) + 1)
			),
			updated_at = NOW()
			WHERE id = p_automation_id;

			-- 5. Log node execution entry
			INSERT INTO automation_node_executions (
				id, contact_automation_id, automation_id, node_id, node_type, action, entered_at, output
			) VALUES (
				gen_random_uuid()::text,
				v_new_id,
				p_automation_id,
				p_root_node_id,
				'trigger',
				'entered',
				NOW(),
				'{}'::jsonb
			);

			-- 6. Create automation.start timeline event
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
			VALUES (
				p_contact_email,
				'insert',
				'automation',
				'automation.start',
				p_automation_id,
				jsonb_build_object(
					'automation_id', jsonb_build_object('new', p_automation_id),
					'root_node_id', jsonb_build_object('new', p_root_node_id)
				),
				NOW()
			);

		END;
		$$ LANGUAGE plpgsql`,
	}

	for _, query := range triggerQueries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to create workspace triggers: %w", err)
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

	// Drop the inbound_webhook_events table
	if _, err := db.Exec("DROP TABLE IF EXISTS inbound_webhook_events CASCADE"); err != nil {
		return fmt.Errorf("failed to drop inbound_webhook_events table: %w", err)
	}

	return nil
}
