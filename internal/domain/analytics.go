package domain

import (
	"context"
	"strings"

	"github.com/Notifuse/notifuse/pkg/analytics"
)

//go:generate mockgen -destination mocks/mock_analytics_service.go -package mocks github.com/Notifuse/notifuse/internal/domain AnalyticsService
//go:generate mockgen -destination mocks/mock_analytics_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain AnalyticsRepository

// broadcastSendingStatuses are the statuses of a broadcast whose task is queueing its
// emails: processing, and an A/B test's test phase (testing) and winner phase
// (winner_selected). A paused broadcast queues nothing until it is resumed, and an A/B test
// waiting for its winner (test_completed) has nothing to queue until one is picked. A
// processed broadcast is complete: its task queued every email.
var broadcastSendingStatuses = []string{
	string(BroadcastStatusProcessing),
	string(BroadcastStatusTesting),
	string(BroadcastStatusWinnerSelected),
}

// broadcastProcessedFilter keeps the processed broadcasts, the only ones whose
// enqueued_count is written.
var broadcastProcessedFilter = []analytics.MeasureFilter{
	{SQL: valueFilter("status", string(BroadcastStatusProcessed))},
}

// broadcastRecipientsFilter keeps the processed broadcasts that recorded how many emails
// they queued. v21 (21.0) added enqueued_count with a default of 0 and renamed the sent
// broadcasts processed, so a broadcast sent before 21.0 is processed with 0: counting it
// would read as a broadcast sent to nobody. A processed broadcast that queued nothing
// (an empty audience) is left out with it, and it sent nothing either. Rebuilding the old
// counts from message_history was not done: it reads the whole table, and a message
// history pruned by data retention, or a send that failed to queue, would give a count
// the broadcast never had.
var broadcastRecipientsFilter = []analytics.MeasureFilter{
	{SQL: valueFilter("status", string(BroadcastStatusProcessed))},
	{SQL: "enqueued_count > 0"},
}

// automationStepSkippedSQL is true on the row of an email step that skipped its
// contact: the executor records it as completed, with skipped in its output. The
// skipped action has no writer.
const automationStepSkippedSQL = "(output->>'skipped' = 'true')"

// valueFilter is the measure filter keeping the rows whose column holds one of values.
// Measures build their filters from the Go constants of the column's values, so that a
// value that does not exist cannot be typed in: a filter on one matches no row and the
// measure silently counts zero.
func valueFilter(column string, values ...string) string {
	if len(values) == 1 {
		return column + " = '" + values[0] + "'"
	}
	return column + " IN ('" + strings.Join(values, "', '") + "')"
}

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
			// Email is the only channel written to message_history today
			// (TransactionalChannelEmail), so a measure without a channel filter counts
			// email messages all the same: count_sent and count_sent_emails count the
			// same rows. The measures that filter on channel would stay email-only if
			// another channel were added; the others would then count every channel.
			"count_sent": {
				Type:        "count",
				Title:       "Sent",
				SQL:         "*",
				Description: "Total number of messages accepted by the email provider",
				Filters: []analytics.MeasureFilter{
					{SQL: "sent_at IS NOT NULL"},
					// A message the provider refused is not a message that was sent.
					// sent_at is stamped on the first attempt whatever its outcome, so
					// without this a permanently failed recipient counted as both sent
					// and failed, and every rate below used an inflated denominator.
					// failed_at is written only by our own send paths and is cleared by
					// a later success, so a bounce stays counted as sent — it was sent,
					// then it bounced.
					{SQL: "failed_at IS NULL"},
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
				Description: "Total number of email messages accepted by the email provider",
				Filters: []analytics.MeasureFilter{
					{SQL: "sent_at IS NOT NULL"},
					{SQL: "failed_at IS NULL"},
					{SQL: valueFilter("channel", string(TransactionalChannelEmail))},
				},
			},
			"count_delivered_emails": {
				Type:        "count",
				Title:       "Delivered Emails",
				SQL:         "*",
				Description: "Total number of delivered email messages",
				Filters: []analytics.MeasureFilter{
					{SQL: "delivered_at IS NOT NULL"},
					{SQL: valueFilter("channel", string(TransactionalChannelEmail))},
				},
			},
			// The denominators of the open and click rates: emails sent with the
			// open pixel, and with the click redirect. A message sent before the
			// flags existed has them NULL and counts as tracked.
			"count_opens_tracked": {
				Type:        "count",
				Title:       "Sent Emails Tracked for Opens",
				SQL:         "*",
				Description: "Email messages accepted by the email provider that carried the open pixel",
				Filters: []analytics.MeasureFilter{
					{SQL: "sent_at IS NOT NULL"},
					{SQL: "failed_at IS NULL"},
					{SQL: valueFilter("channel", string(TransactionalChannelEmail))},
					{SQL: "opens_tracked IS NOT FALSE"},
				},
			},
			"count_clicks_tracked": {
				Type:        "count",
				Title:       "Sent Emails Tracked for Clicks",
				SQL:         "*",
				Description: "Email messages accepted by the email provider whose links went through the click redirect",
				Filters: []analytics.MeasureFilter{
					{SQL: "sent_at IS NOT NULL"},
					{SQL: "failed_at IS NULL"},
					{SQL: valueFilter("channel", string(TransactionalChannelEmail))},
					{SQL: "clicks_tracked IS NOT FALSE"},
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
				Description: "Messages sent by a transactional notification, what the origin dimension calls transactional: not automation emails, template tests or double opt-in confirmations",
				Filters: []analytics.MeasureFilter{
					{SQL: "transactional_notification_id IS NOT NULL"},
					{SQL: "automation_id IS NULL"},
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
			"last_sent_at": {
				Type:        "max",
				Title:       "Last Sent At",
				SQL:         "sent_at",
				Description: "When the most recent message accepted by the email provider was sent: for a transactional message, just before the provider call; for a broadcast or automation message, the time its email was queued",
				Filters: []analytics.MeasureFilter{
					// sent_at is stamped whatever the outcome (see count_sent), and a retry
					// upsert does not move it. A transactional message's sent_at is taken just
					// before the provider call; a broadcast or automation message's is the time
					// its email was queued, which can be hours before it left under a rate limit.
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
			// No index on either column: the console always filters on broadcast_id too.
			"opened_at": {
				Type:        "time",
				Title:       "Opened At",
				SQL:         "opened_at",
				Description: "When the message was first opened; a click counts as an open when opens are tracked",
			},
			"clicked_at": {
				Type:        "time",
				Title:       "Clicked At",
				SQL:         "clicked_at",
				Description: "When a link in the message was first clicked",
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
			"automation_id": {
				Type:        "string",
				Title:       "Automation ID",
				SQL:         "automation_id",
				Description: "Automation that sent the message",
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
			"transactional_notification_id": {
				Type:        "string",
				Title:       "Transactional Notification ID",
				SQL:         "transactional_notification_id",
				Description: "Transactional notification identifier",
			},
			// Parenthesised: the query builder copies a dimension's SQL into SELECT, GROUP BY
			// and filters. The values and their order are those of the messages.list origin
			// filter: an automation email that also carries a notification ID is the
			// automation's. A broadcast's test send carries its broadcast_id; template tests and
			// double opt-in confirmations carry no ID and count as other.
			"origin": {
				Type:        "string",
				Title:       "Origin",
				SQL:         "(CASE WHEN broadcast_id IS NOT NULL THEN 'broadcast' WHEN automation_id IS NOT NULL THEN 'automation' WHEN transactional_notification_id IS NOT NULL THEN 'transactional' ELSE 'other' END)",
				Description: "What sent the message: broadcast, automation, transactional, or other (none of the three, such as template tests and double opt-in confirmations)",
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
			// No status measures: a contact has no status of its own, only one per list
			// (contact_lists), and the contacts table has no status column to filter on.
			"count_recent_contacts": {
				Type:        "count",
				Title:       "Recent Contacts",
				SQL:         "*",
				Description: "Contacts created in the last 30 days",
				Filters: []analytics.MeasureFilter{
					{SQL: "created_at >= NOW() - INTERVAL '30 days'"},
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
					{SQL: valueFilter("status", string(BroadcastStatusDraft))},
				},
			},
			"count_scheduled": {
				Type:        "count",
				Title:       "Scheduled Broadcasts",
				SQL:         "*",
				Description: "Total number of scheduled broadcasts",
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("status", string(BroadcastStatusScheduled))},
				},
			},
			"count_sending": {
				Type:        "count",
				Title:       "Sending Broadcasts",
				SQL:         "*",
				Description: "Broadcasts queueing their emails: processing, or an A/B test sending its test or its winner. A paused broadcast and an A/B test waiting for its winner are not sending.",
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("status", broadcastSendingStatuses...)},
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
			// A broadcast's recipients are the emails its task queued, enqueued_count: the
			// table keeps no other recipient count (an A/B test adds its winner phase to
			// its test phase there). The task writes it once, when the broadcast is
			// processed; until then, and for a broadcast paused, cancelled or waiting for
			// its winner, it holds 0. Every recipient measure therefore reads processed
			// broadcasts with a count only (broadcastRecipientsFilter): a draft's 0, or
			// the 0 of a broadcast sent before 21.0, is not a broadcast sent to nobody.
			"avg_recipients": {
				Type:        "avg",
				Title:       "Average Recipients",
				SQL:         "enqueued_count",
				Description: "Average emails queued per processed broadcast. Counted when a broadcast finishes; others are left out. Broadcasts with a count of 0, such as those sent before version 21.0, which recorded none, are left out.",
				Filters:     broadcastRecipientsFilter,
			},
			"sum_recipients": {
				Type:        "sum",
				Title:       "Total Recipients",
				SQL:         "enqueued_count",
				Description: "Emails queued by processed broadcasts. Counted when a broadcast finishes; others are left out. Broadcasts with a count of 0, such as those sent before version 21.0, which recorded none, are left out.",
				Filters:     broadcastRecipientsFilter,
			},
			"max_recipients": {
				Type:        "max",
				Title:       "Max Recipients",
				SQL:         "enqueued_count",
				Description: "Most emails queued by one processed broadcast. Counted when a broadcast finishes; others are left out. Broadcasts with a count of 0, such as those sent before version 21.0, which recorded none, are left out.",
				Filters:     broadcastRecipientsFilter,
			},
			"min_recipients": {
				Type:        "min",
				Title:       "Min Recipients",
				SQL:         "enqueued_count",
				Description: "Fewest emails queued by one processed broadcast. Counted when a broadcast finishes; others are left out. Broadcasts with a count of 0, such as those sent before version 21.0, which recorded none, are left out.",
				Filters:     broadcastRecipientsFilter,
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
				Description: "Broadcasts that finished queueing their emails (processed)",
				Filters:     broadcastProcessedFilter,
			},
			"avg_recipients_completed": {
				Type:        "avg",
				Title:       "Average Recipients (Completed)",
				SQL:         "enqueued_count",
				Description: "Average emails queued per processed broadcast. Same as avg_recipients.",
				Filters:     broadcastRecipientsFilter,
			},
			"winner_recipients": {
				Type:        "sum",
				Title:       "Winner Recipients",
				SQL:         "winner_phase_recipient_count",
				Description: "Contacts processed in the winner phase of A/B tests whose winner was sent, including any whose email failed to queue",
				// An A/B test that ends its test phase writes the winner phase it plans;
				// once the winner phase finishes, the count is the contacts it went
				// through (RecipientOffset - WinnerPhaseStartOffset), queued or not.
				Filters: []analytics.MeasureFilter{
					{SQL: "winner_sent_at IS NOT NULL"},
				},
			},
			"avg_test_recipients": {
				Type:        "avg",
				Title:       "Average Test Recipients",
				SQL:         "test_phase_recipient_count",
				Description: "Average test phase recipients per broadcast",
				Filters: []analytics.MeasureFilter{
					{SQL: "test_phase_recipient_count > 0"},
				},
			},
			"sum_recipients_completed": {
				Type:        "sum",
				Title:       "Total Recipients (Completed)",
				SQL:         "enqueued_count",
				Description: "Emails queued by processed broadcasts. Same as sum_recipients.",
				Filters:     broadcastRecipientsFilter,
			},
			"count_with_ab_test": {
				Type:        "count",
				Title:       "A/B Tested Broadcasts",
				SQL:         "*",
				Description: "Broadcasts with A/B testing enabled, whatever their status",
				// The switch itself: test_phase_recipient_count is written only once the
				// test phase ends, so it misses drafts and tests still running.
				Filters: []analytics.MeasureFilter{
					{SQL: "(test_settings->>'enabled') = 'true'"},
				},
			},
			"count_large_broadcasts": {
				Type:        "count",
				Title:       "Large Broadcasts",
				SQL:         "*",
				Description: "Processed broadcasts that queued more than 1000 emails. Broadcasts with a count of 0, such as those sent before version 21.0, which recorded none, are left out.",
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("status", string(BroadcastStatusProcessed))},
					{SQL: "enqueued_count > 1000"},
				},
			},
		},
		Dimensions: map[string]analytics.DimensionDefinition{
			"id": {
				Type:        "string",
				Title:       "Broadcast ID",
				SQL:         "id",
				Description: "Broadcast identifier",
			},
			"name": {
				Type:        "string",
				Title:       "Name",
				SQL:         "name",
				Description: "Broadcast name",
			},
			"status": {
				Type:        "string",
				Title:       "Status",
				SQL:         "status",
				Description: "Broadcast status",
			},
			"created_at": {
				Type:        "time",
				Title:       "Created At",
				SQL:         "created_at",
				Description: "Broadcast creation timestamp",
			},
			"started_at": {
				Type:        "time",
				Title:       "Started At",
				SQL:         "started_at",
				Description: "Broadcast start timestamp",
			},
			"completed_at": {
				Type:        "time",
				Title:       "Completed At",
				SQL:         "completed_at",
				Description: "Broadcast completion timestamp",
			},
			// Each workspace has a database of its own, so this column holds the same
			// value on every row the query reads: grouping or filtering by it changes
			// nothing. It stays for callers that already name it.
			"workspace_id": {
				Type:        "string",
				Title:       "Workspace ID",
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
					{SQL: valueFilter("status", WebhookDeliveryStatusDelivered)},
				},
			},
			"count_failed": {
				Type:        "count",
				Title:       "Failed",
				SQL:         "*",
				Description: "Failed webhook deliveries",
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("status", WebhookDeliveryStatusFailed)},
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
					{SQL: valueFilter("status", string(EmailQueueStatusPending))},
				},
			},
			"count_processing": {
				Type:        "count",
				Title:       "Processing",
				SQL:         "*",
				Description: "Emails currently being processed",
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("status", string(EmailQueueStatusProcessing))},
				},
			},
			"count_failed": {
				Type:        "count",
				Title:       "Failed",
				SQL:         "*",
				Description: "Emails in failed status, whether waiting for a retry or given up (see count_retryable)",
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("status", string(EmailQueueStatusFailed))},
				},
			},
			"count_paused": {
				Type:        "count",
				Title:       "Paused",
				SQL:         "*",
				Description: "Emails held while their broadcast is paused",
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("status", string(EmailQueueStatusPaused))},
				},
			},
			"count_broadcast": {
				Type:        "count",
				Title:       "Broadcast Emails",
				SQL:         "*",
				Description: "Emails from broadcasts",
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("source_type", string(EmailQueueSourceBroadcast))},
				},
			},
			"count_automation": {
				Type:        "count",
				Title:       "Automation Emails",
				SQL:         "*",
				Description: "Emails from automations",
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("source_type", string(EmailQueueSourceAutomation))},
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
				Description: "Most send attempts made so far by one email (not the retry limit)",
			},
			"count_retryable": {
				Type:        "count",
				Title:       "Retryable",
				SQL:         "*",
				Description: "Failed emails with a retry scheduled",
				// A failure given up on has no retry date, whatever its attempt count: a
				// rejection not worth repeating (a mailbox that does not exist) is given up
				// on at its first attempt. The worker retries only rows with a date.
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("status", string(EmailQueueStatusFailed))},
					{SQL: "next_retry_at IS NOT NULL"},
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
				Description: "Queue entry status (pending, processing, failed, paused)",
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
			// The executor writes one row per step it runs ('processing', then
			// 'completed' or 'failed'); the enrollment writes 'entered' on the trigger
			// node. A skipped email is a step that ran: 'completed' with output.skipped.
			// count_completed, count_skipped and count_errors split the 'completed' rows
			// between them: the node stats (nodeStatsVisitsSQL) count the first two
			// together as completed, and the last as errors. count_failed counts as the
			// node stats' failed attempts do.
			"count_entered": {
				Type:        "count",
				Title:       "Enrolled",
				SQL:         "*",
				Description: "Contacts enrolled in the automation: one row per enrollment, on its trigger node",
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("action", string(NodeActionEntered))},
				},
			},
			"count_completed": {
				Type:        "count",
				Title:       "Completed",
				SQL:         "*",
				Description: "Steps that ran and succeeded. Skipped emails (count_skipped) and steps that recorded an error (count_errors) are left out; the automation's step statistics count count_completed and count_skipped together as completed.",
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("action", string(NodeActionCompleted))},
					{SQL: automationStepSkippedSQL + " IS NOT TRUE"},
					{SQL: "output->'error' IS NULL"},
				},
			},
			"count_failed": {
				Type:        "count",
				Title:       "Failed",
				SQL:         "*",
				Description: "Failed attempts at a step, one per attempt. A step cut short by a shutdown is not a failure.",
				// A failed attempt writes two rows: the step's own row closed as failed
				// (completed_at set) and the failure record (no completed_at). A step cut
				// short writes only the first. Counting the second counts each attempt once.
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("action", string(NodeActionFailed))},
					{SQL: "completed_at IS NULL"},
				},
			},
			"count_errors": {
				Type:        "count",
				Title:       "Completed with Errors",
				SQL:         "*",
				Description: "Steps that completed but recorded an error, such as an add-to-list or remove-from-list step that failed to write the list before version 44.0. The automation's step statistics count these as errors.",
				// output ? 'error' in the node stats: the builder's placeholders would
				// rewrite a ?, and -> gives the same answer, a JSON null included.
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("action", string(NodeActionCompleted))},
					{SQL: "output->'error' IS NOT NULL"},
				},
			},
			"count_skipped": {
				Type:        "count",
				Title:       "Skipped",
				SQL:         "*",
				Description: "Email steps that sent nothing because the contact had unsubscribed, bounced or complained",
				Filters: []analytics.MeasureFilter{
					{SQL: valueFilter("action", string(NodeActionCompleted))},
					{SQL: automationStepSkippedSQL},
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

// analyticsSchemaResources maps a predefined schema to the permission resource
// that owns the rows it exposes. A cube query is a read of the underlying table,
// so it must cost the same grant as reading that table anywhere else.
//
// The three non-obvious ones:
//   - webhook_deliveries is the delivery log of the outbound webhook
//     subscriptions, keyed by subscription_id — the same data the subscription
//     service serves under webhook_subscriptions:read.
//   - email_queue rows carry contact_email, template_id and the originating
//     broadcast or automation: the outbound message log before it is sent, which
//     is what message_history holds after.
//   - automation_node_executions is per-node automation telemetry, keyed by
//     automation_id.
//
// Web analytics schemas are not listed here — they are per-workspace and
// resolved through IsWebAnalyticsSchema in AnalyticsSchemaResource.
var analyticsSchemaResources = map[string]PermissionResource{
	"message_history":            PermissionResourceMessageHistory,
	"contacts":                   PermissionResourceContacts,
	"broadcasts":                 PermissionResourceBroadcasts,
	"webhook_deliveries":         PermissionResourceWebhookSubscriptions,
	"email_queue":                PermissionResourceMessageHistory,
	"automation_node_executions": PermissionResourceAutomations,
}

// AnalyticsSchemaResource returns the resource whose read grant gates a schema,
// and false when the schema has no mapping. Callers must treat the false case as
// a denial: a schema added to PredefinedSchemas without an entry here would
// otherwise be readable by any workspace member, silently making the analytics
// endpoint a bypass for every resource permission.
func AnalyticsSchemaResource(schema string) (PermissionResource, bool) {
	if IsWebAnalyticsSchema(schema) {
		return PermissionResourceWebAnalytics, true
	}
	resource, ok := analyticsSchemaResources[schema]
	return resource, ok
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
