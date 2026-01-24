package domain

import (
	"context"

	"github.com/Notifuse/notifuse/pkg/analytics"
)

//go:generate mockgen -destination mocks/mock_analytics_service.go -package mocks github.com/Notifuse/notifuse/internal/domain AnalyticsService
//go:generate mockgen -destination mocks/mock_analytics_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain AnalyticsRepository

// PredefinedSchemas contains all available analytics schemas for Notifuse
var PredefinedSchemas = map[string]analytics.SchemaDefinition{
	"message_history": {
		Name: "message_history",
		Measures: map[string]analytics.MeasureDefinition{
			"count": {
				Type:        "count",
				Title:       "Total Messages",
				SQL:         "COUNT(*)",
				Description: "Total number of message history records",
			},
			"count_sent": {
				Type:        "count",
				Title:       "Sent",
				SQL:         "*",
				Description: "Total number of sent messages",
				Filters: []analytics.MeasureFilter{
					{SQL: "sent_at IS NOT NULL"},
				},
			},
			"count_delivered": {
				Type:        "count",
				Title:       "Delivered",
				SQL:         "*",
				Description: "Total number of delivered messages",
				Filters: []analytics.MeasureFilter{
					{SQL: "delivered_at IS NOT NULL"},
				},
			},
			"count_bounced": {
				Type:        "count",
				Title:       "Bounced",
				SQL:         "*",
				Description: "Total number of bounced messages",
				Filters: []analytics.MeasureFilter{
					{SQL: "bounced_at IS NOT NULL"},
				},
			},
			"count_complained": {
				Type:        "count",
				Title:       "Complaints",
				SQL:         "*",
				Description: "Total number of complained messages",
				Filters: []analytics.MeasureFilter{
					{SQL: "complained_at IS NOT NULL"},
				},
			},
			"count_opened": {
				Type:        "count",
				Title:       "Opens",
				SQL:         "*",
				Description: "Total number of opened messages",
				Filters: []analytics.MeasureFilter{
					{SQL: "opened_at IS NOT NULL"},
				},
			},
			"count_clicked": {
				Type:        "count",
				Title:       "Clicks",
				SQL:         "*",
				Description: "Total number of clicked messages",
				Filters: []analytics.MeasureFilter{
					{SQL: "clicked_at IS NOT NULL"},
				},
			},
			"count_unsubscribed": {
				Type:        "count",
				Title:       "Unsubscribes",
				SQL:         "*",
				Description: "Total number of unsubscribed messages",
				Filters: []analytics.MeasureFilter{
					{SQL: "unsubscribed_at IS NOT NULL"},
				},
			},
			"count_failed": {
				Type:        "count",
				Title:       "Failed",
				SQL:         "*",
				Description: "Total number of failed messages",
				Filters: []analytics.MeasureFilter{
					{SQL: "failed_at IS NOT NULL"},
				},
			},
			"count_sent_emails": {
				Type:        "count",
				Title:       "Sent Emails",
				SQL:         "*",
				Description: "Total number of sent email messages",
				Filters: []analytics.MeasureFilter{
					{SQL: "sent_at IS NOT NULL"},
					{SQL: "channel = 'email'"},
				},
			},
			"count_delivered_emails": {
				Type:        "count",
				Title:       "Delivered Emails",
				SQL:         "*",
				Description: "Total number of delivered email messages",
				Filters: []analytics.MeasureFilter{
					{SQL: "delivered_at IS NOT NULL"},
					{SQL: "channel = 'email'"},
				},
			},
			"count_broadcast_messages": {
				Type:        "count",
				Title:       "Broadcast Messages",
				SQL:         "*",
				Description: "Total number of broadcast messages",
				Filters: []analytics.MeasureFilter{
					{SQL: "broadcast_id IS NOT NULL"},
				},
			},
			"count_transactional_messages": {
				Type:        "count",
				Title:       "Transactional Messages",
				SQL:         "*",
				Description: "Total number of transactional messages",
				Filters: []analytics.MeasureFilter{
					{SQL: "broadcast_id IS NULL"},
				},
			},
			"count_recent_messages": {
				Type:        "count",
				Title:       "Recent Messages",
				SQL:         "*",
				Description: "Messages from the last 30 days",
				Filters: []analytics.MeasureFilter{
					{SQL: "created_at >= NOW() - INTERVAL '30 days'"},
				},
			},
			"count_successful_deliveries": {
				Type:        "count",
				Title:       "Successful Deliveries",
				SQL:         "*",
				Description: "Successfully delivered messages (not bounced or failed)",
				Filters: []analytics.MeasureFilter{
					{SQL: "delivered_at IS NOT NULL"},
					{SQL: "bounced_at IS NULL"},
					{SQL: "failed_at IS NULL"},
				},
			},
		},
		Dimensions: map[string]analytics.DimensionDefinition{
			"created_at": {
				Type:        "time",
				Title:       "Created At",
				SQL:         "created_at",
				Description: "Message creation timestamp",
			},
			"sent_at": {
				Type:        "time",
				Title:       "Sent At",
				SQL:         "sent_at",
				Description: "Message sent timestamp",
			},
			"contact_email": {
				Type:        "string",
				Title:       "Contact Email",
				SQL:         "contact_email",
				Description: "Recipient email address",
			},
			"broadcast_id": {
				Type:        "string",
				Title:       "Broadcast ID",
				SQL:         "broadcast_id",
				Description: "Associated broadcast ID",
			},
			"channel": {
				Type:        "string",
				Title:       "Channel",
				SQL:         "channel",
				Description: "Message channel (email, sms, etc.)",
			},
			"template_id": {
				Type:        "string",
				Title:       "Template ID",
				SQL:         "template_id",
				Description: "Template identifier",
			},
			"external_id": {
				Type:        "string",
				Title:       "External ID",
				SQL:         "external_id",
				Description: "Client-provided transactional notification identifier",
			},
		},
	},
	"contacts": {
		Name: "contacts",
		Measures: map[string]analytics.MeasureDefinition{
			"count": {
				Type:        "count",
				Title:       "Total Contacts",
				SQL:         "*",
				Description: "Total number of contacts",
			},
			"count_active": {
				Type:        "count",
				Title:       "Active Contacts",
				SQL:         "*",
				Description: "Total number of active contacts",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'active'"},
				},
			},
			"count_unsubscribed": {
				Type:        "count",
				Title:       "Unsubscribed Contacts",
				SQL:         "*",
				Description: "Total number of unsubscribed contacts",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'unsubscribed'"},
				},
			},
			"count_bounced": {
				Type:        "count",
				Title:       "Bounced Contacts",
				SQL:         "*",
				Description: "Total number of bounced contacts",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'bounced'"},
				},
			},
			"count_recent_contacts": {
				Type:        "count",
				Title:       "Recent Contacts",
				SQL:         "*",
				Description: "Contacts created in the last 30 days",
				Filters: []analytics.MeasureFilter{
					{SQL: "created_at >= NOW() - INTERVAL '30 days'"},
				},
			},
			"count_with_source": {
				Type:        "count",
				Title:       "Contacts with Source",
				SQL:         "*",
				Description: "Contacts with a known source",
				Filters: []analytics.MeasureFilter{
					{SQL: "source IS NOT NULL"},
					{SQL: "source != ''"},
				},
			},
			"avg_created_days_ago": {
				Type:        "avg",
				Title:       "Average Days Since Creation",
				SQL:         "EXTRACT(EPOCH FROM (NOW() - created_at)) / 86400",
				Description: "Average days since contact creation",
			},
		},
		Dimensions: map[string]analytics.DimensionDefinition{
			"created_at": {
				Type:        "time",
				Title:       "Created At",
				SQL:         "created_at",
				Description: "Contact creation timestamp",
			},
			"email": {
				Type:        "string",
				Title:       "Email",
				SQL:         "email",
				Description: "Contact email address",
			},
			"first_name": {
				Type:        "string",
				Title:       "First Name",
				SQL:         "first_name",
				Description: "Contact first name",
			},
			"last_name": {
				Type:        "string",
				Title:       "Last Name",
				SQL:         "last_name",
				Description: "Contact last name",
			},
			"external_id": {
				Type:        "string",
				Title:       "External ID",
				SQL:         "external_id",
				Description: "External contact identifier",
			},
			"timezone": {
				Type:        "string",
				Title:       "Timezone",
				SQL:         "timezone",
				Description: "Contact timezone",
			},
			"country": {
				Type:        "string",
				Title:       "Country",
				SQL:         "country",
				Description: "Contact country",
			},
		},
	},
	"broadcasts": {
		Name: "broadcasts",
		Measures: map[string]analytics.MeasureDefinition{
			"count": {
				Type:        "count",
				Title:       "Total Broadcasts",
				SQL:         "*",
				Description: "Total number of broadcasts",
			},
			"count_draft": {
				Type:        "count",
				Title:       "Draft Broadcasts",
				SQL:         "*",
				Description: "Total number of draft broadcasts",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'draft'"},
				},
			},
			"count_scheduled": {
				Type:        "count",
				Title:       "Scheduled Broadcasts",
				SQL:         "*",
				Description: "Total number of scheduled broadcasts",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'scheduled'"},
				},
			},
			"count_sending": {
				Type:        "count",
				Title:       "Sending Broadcasts",
				SQL:         "*",
				Description: "Total number of broadcasts currently sending",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'sending'"},
				},
			},
			"count_recent": {
				Type:        "count",
				Title:       "Recent Broadcasts",
				SQL:         "*",
				Description: "Broadcasts created in the last 30 days",
				Filters: []analytics.MeasureFilter{
					{SQL: "created_at >= NOW() - INTERVAL '30 days'"},
				},
			},
			"avg_recipients": {
				Type:        "avg",
				Title:       "Average Recipients",
				SQL:         "recipient_count",
				Description: "Average number of recipients per broadcast",
			},
			"sum_recipients": {
				Type:        "sum",
				Title:       "Total Recipients",
				SQL:         "recipient_count",
				Description: "Total number of recipients across all broadcasts",
			},
			"max_recipients": {
				Type:        "max",
				Title:       "Max Recipients",
				SQL:         "recipient_count",
				Description: "Maximum recipients in a single broadcast",
			},
			"min_recipients": {
				Type:        "min",
				Title:       "Min Recipients",
				SQL:         "recipient_count",
				Description: "Minimum recipients in a single broadcast",
			},
			"test_recipients": {
				Type:        "sum",
				Title:       "Test Recipients",
				SQL:         "test_phase_recipient_count",
				Description: "Total test phase recipients",
			},
			"completed_broadcasts_count": {
				Type:        "count",
				Title:       "Completed Broadcasts",
				SQL:         "*",
				Description: "Total number of completed broadcasts",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'completed'"},
				},
			},
			"avg_recipients_completed": {
				Type:        "avg",
				SQL:         "recipient_count",
				Description: "Average recipients for completed broadcasts only",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'completed'"},
				},
			},
			"winner_recipients": {
				Type:        "sum",
				SQL:         "winner_phase_recipient_count",
				Description: "Total winner phase recipients",
			},
			"avg_test_recipients": {
				Type:        "avg",
				SQL:         "test_phase_recipient_count",
				Description: "Average test phase recipients per broadcast",
				Filters: []analytics.MeasureFilter{
					{SQL: "test_phase_recipient_count > 0"},
				},
			},
			"sum_recipients_completed": {
				Type:        "sum",
				SQL:         "recipient_count",
				Description: "Total recipients for completed broadcasts",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'completed'"},
				},
			},
			"count_with_ab_test": {
				Type:        "count",
				SQL:         "*",
				Description: "Broadcasts with A/B testing enabled",
				Filters: []analytics.MeasureFilter{
					{SQL: "test_phase_recipient_count > 0"},
				},
			},
			"count_large_broadcasts": {
				Type:        "count",
				SQL:         "*",
				Description: "Broadcasts with more than 1000 recipients",
				Filters: []analytics.MeasureFilter{
					{SQL: "recipient_count > 1000"},
				},
			},
		},
		Dimensions: map[string]analytics.DimensionDefinition{
			"id": {
				Type:        "string",
				SQL:         "id",
				Description: "Broadcast identifier",
			},
			"name": {
				Type:        "string",
				SQL:         "name",
				Description: "Broadcast name",
			},
			"status": {
				Type:        "string",
				SQL:         "status",
				Description: "Broadcast status",
			},
			"created_at": {
				Type:        "time",
				SQL:         "created_at",
				Description: "Broadcast creation timestamp",
			},
			"started_at": {
				Type:        "time",
				SQL:         "started_at",
				Description: "Broadcast start timestamp",
			},
			"completed_at": {
				Type:        "time",
				SQL:         "completed_at",
				Description: "Broadcast completion timestamp",
			},
			"workspace_id": {
				Type:        "string",
				SQL:         "workspace_id",
				Description: "Associated workspace ID",
			},
		},
	},
	"webhook_deliveries": {
		Name: "webhook_deliveries",
		Measures: map[string]analytics.MeasureDefinition{
			"count": {
				Type:        "count",
				Title:       "Total Deliveries",
				SQL:         "*",
				Description: "Total number of webhook deliveries",
			},
			"count_delivered": {
				Type:        "count",
				Title:       "Delivered",
				SQL:         "*",
				Description: "Successfully delivered webhooks",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'delivered'"},
				},
			},
			"count_failed": {
				Type:        "count",
				Title:       "Failed",
				SQL:         "*",
				Description: "Failed webhook deliveries",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'failed'"},
				},
			},
		},
		Dimensions: map[string]analytics.DimensionDefinition{
			"created_at": {
				Type:        "time",
				Title:       "Created At",
				SQL:         "created_at",
				Description: "Delivery creation timestamp",
			},
			"subscription_id": {
				Type:        "string",
				Title:       "Subscription ID",
				SQL:         "subscription_id",
				Description: "Associated webhook subscription",
			},
			"status": {
				Type:        "string",
				Title:       "Status",
				SQL:         "status",
				Description: "Delivery status",
			},
		},
	},
	"email_queue": {
		Name: "email_queue",
		Measures: map[string]analytics.MeasureDefinition{
			"count": {
				Type:        "count",
				Title:       "Total Entries",
				SQL:         "*",
				Description: "Total queue entries",
			},
			"count_pending": {
				Type:        "count",
				Title:       "Pending",
				SQL:         "*",
				Description: "Emails waiting to be sent",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'pending'"},
				},
			},
			"count_processing": {
				Type:        "count",
				Title:       "Processing",
				SQL:         "*",
				Description: "Emails currently being processed",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'processing'"},
				},
			},
			"count_failed": {
				Type:        "count",
				Title:       "Failed",
				SQL:         "*",
				Description: "Emails that failed and awaiting retry",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'failed'"},
				},
			},
			"count_broadcast": {
				Type:        "count",
				Title:       "Broadcast Emails",
				SQL:         "*",
				Description: "Emails from broadcasts",
				Filters: []analytics.MeasureFilter{
					{SQL: "source_type = 'broadcast'"},
				},
			},
			"count_automation": {
				Type:        "count",
				Title:       "Automation Emails",
				SQL:         "*",
				Description: "Emails from automations",
				Filters: []analytics.MeasureFilter{
					{SQL: "source_type = 'automation'"},
				},
			},
			"avg_attempts": {
				Type:        "avg",
				Title:       "Average Attempts",
				SQL:         "attempts",
				Description: "Average number of send attempts",
			},
			"max_attempts": {
				Type:        "max",
				Title:       "Max Attempts",
				SQL:         "attempts",
				Description: "Maximum number of send attempts",
			},
			"count_retryable": {
				Type:        "count",
				Title:       "Retryable",
				SQL:         "*",
				Description: "Failed emails that can still be retried",
				Filters: []analytics.MeasureFilter{
					{SQL: "status = 'failed'"},
					{SQL: "attempts < max_attempts"},
				},
			},
		},
		Dimensions: map[string]analytics.DimensionDefinition{
			"created_at": {
				Type:        "time",
				Title:       "Created At",
				SQL:         "created_at",
				Description: "When the email was queued",
			},
			"updated_at": {
				Type:        "time",
				Title:       "Updated At",
				SQL:         "updated_at",
				Description: "Last update timestamp",
			},
			"next_retry_at": {
				Type:        "time",
				Title:       "Next Retry At",
				SQL:         "next_retry_at",
				Description: "Scheduled retry time for failed emails",
			},
			"status": {
				Type:        "string",
				Title:       "Status",
				SQL:         "status",
				Description: "Queue entry status (pending, processing, failed)",
			},
			"source_type": {
				Type:        "string",
				Title:       "Source Type",
				SQL:         "source_type",
				Description: "Origin type (broadcast, automation)",
			},
			"source_id": {
				Type:        "string",
				Title:       "Source ID",
				SQL:         "source_id",
				Description: "Broadcast or automation ID",
			},
			"integration_id": {
				Type:        "string",
				Title:       "Integration ID",
				SQL:         "integration_id",
				Description: "Email provider integration ID",
			},
			"provider_kind": {
				Type:        "string",
				Title:       "Provider Kind",
				SQL:         "provider_kind",
				Description: "Email provider type (ses, smtp, etc.)",
			},
			"priority": {
				Type:        "number",
				Title:       "Priority",
				SQL:         "priority",
				Description: "Queue priority (lower = higher priority)",
			},
			"template_id": {
				Type:        "string",
				Title:       "Template ID",
				SQL:         "template_id",
				Description: "Email template identifier",
			},
			"contact_email": {
				Type:        "string",
				Title:       "Contact Email",
				SQL:         "contact_email",
				Description: "Recipient email address",
			},
		},
	},
	"automation_node_executions": {
		Name: "automation_node_executions",
		Measures: map[string]analytics.MeasureDefinition{
			"count": {
				Type:        "count",
				Title:       "Total Executions",
				SQL:         "*",
				Description: "Total node executions",
			},
			"count_entered": {
				Type:        "count",
				Title:       "Entered",
				SQL:         "*",
				Description: "Contacts that entered nodes",
				Filters: []analytics.MeasureFilter{
					{SQL: "action = 'entered'"},
				},
			},
			"count_completed": {
				Type:        "count",
				Title:       "Completed",
				SQL:         "*",
				Description: "Successfully completed executions",
				Filters: []analytics.MeasureFilter{
					{SQL: "action = 'completed'"},
				},
			},
			"count_failed": {
				Type:        "count",
				Title:       "Failed",
				SQL:         "*",
				Description: "Failed executions",
				Filters: []analytics.MeasureFilter{
					{SQL: "action = 'failed'"},
				},
			},
			"count_skipped": {
				Type:        "count",
				Title:       "Skipped",
				SQL:         "*",
				Description: "Skipped executions",
				Filters: []analytics.MeasureFilter{
					{SQL: "action = 'skipped'"},
				},
			},
		},
		Dimensions: map[string]analytics.DimensionDefinition{
			"automation_id": {
				Type:        "string",
				Title:       "Automation ID",
				SQL:         "automation_id",
				Description: "Automation identifier",
			},
			"node_id": {
				Type:        "string",
				Title:       "Node ID",
				SQL:         "node_id",
				Description: "Node identifier",
			},
			"node_type": {
				Type:        "string",
				Title:       "Node Type",
				SQL:         "node_type",
				Description: "Type of node",
			},
			"action": {
				Type:        "string",
				Title:       "Action",
				SQL:         "action",
				Description: "Execution action",
			},
			"entered_at": {
				Type:        "time",
				Title:       "Entered At",
				SQL:         "entered_at",
				Description: "When node was entered",
			},
		},
	},
}

// AnalyticsService defines the analytics business logic interface
type AnalyticsService interface {
	Query(ctx context.Context, workspaceID string, query analytics.Query) (*analytics.Response, error)
	GetSchemas(ctx context.Context, workspaceID string) (map[string]analytics.SchemaDefinition, error)
}

// AnalyticsRepository defines the analytics data access interface
type AnalyticsRepository interface {
	Query(ctx context.Context, workspaceID string, query analytics.Query) (*analytics.Response, error)
	GetSchemas(ctx context.Context, workspaceID string) (map[string]analytics.SchemaDefinition, error)
}
