package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/logging"
	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

// TelemetryMetrics represents the metrics data sent to the telemetry endpoint
type TelemetryMetrics struct {
	WorkspaceIDSHA1    string `json:"workspace_id_sha1"`
	WorkspaceCreatedAt string `json:"workspace_created_at"`
	WorkspaceUpdatedAt string `json:"workspace_updated_at"`
	LastMessageAt      string `json:"last_message_at"`
	ContactsCount      int    `json:"contacts_count"`
	BroadcastsCount    int    `json:"broadcasts_count"`
	TransactionalCount int    `json:"transactional_count"`
	MessagesCount      int    `json:"messages_count"`
	ListsCount         int    `json:"lists_count"`
	SegmentsCount      int    `json:"segments_count"`
	UsersCount         int    `json:"users_count"`
	BlogPostsCount     int    `json:"blog_posts_count"`
	APIEndpoint        string `json:"api_endpoint"`

	// Integration flags - boolean for each email provider
	Mailgun   bool `json:"mailgun"`
	AmazonSES bool `json:"amazonses"`
	Mailjet   bool `json:"mailjet"`
	SparkPost bool `json:"sparkpost"`
	Postmark  bool `json:"postmark"`
	SMTP      bool `json:"smtp"`
	S3        bool `json:"s3"`

	// Non-email integrations; the LLM one is reported per provider.
	Anthropic bool `json:"anthropic"`
	OpenAI    bool `json:"openai"`
	Gemini    bool `json:"gemini"`
	Supabase  bool `json:"supabase"`
	Firecrawl bool `json:"firecrawl"`

	// WebAnalytics reports whether the workspace recorded a web analytics
	// session recently, i.e. that it is collecting traffic — not merely that the
	// feature is switched on.
	WebAnalytics bool `json:"web_analytics"`

	// The five fields below are POINTERS because they arrived with v40 and every pre-v40
	// sender omits them. As plain values they would decode to false/"" and be written as
	// false/"" — see LogEntry for why that ruins the one measurement they exist for.

	// SESTenant reports whether any SES integration in the workspace has
	// Notifuse-managed tenant isolation switched on. Sender-side intent, not what
	// AWS has provisioned; see the field of the same name in
	// internal/service/telemetry_service.go.
	SESTenant *bool `json:"ses_tenant"`

	// RBACCustom reports whether any member or API key of the workspace holds
	// permissions other than the full set — the blast radius of the RBAC licence
	// gate. Only the boolean travels: never who is restricted, nor on what.
	RBACCustom *bool `json:"rbac_custom"`

	// The three fields below describe the installation rather than the workspace,
	// so they repeat identically on every row an instance sends, exactly as
	// api_endpoint already does.

	// Version is the release the sending binary was built as.
	Version *string `json:"version"`

	// OIDCEnabled is the sender's RESOLVED single sign-on setting, which may come
	// from its database rather than from an environment variable. No issuer,
	// client id or provider name is sent.
	OIDCEnabled *bool `json:"oidc_enabled"`

	// LicenseTier is the plan the sending deployment is licensed for, or an empty
	// string when it is not licensed. No key, licence id, organisation or billing
	// contact is sent.
	LicenseTier *string `json:"license_tier"`
}

// LogEntry represents the structured log entry for Google Cloud Logging
type LogEntry struct {
	Timestamp          time.Time `json:"timestamp"`
	WorkspaceIDSHA1    string    `json:"workspace_id_sha1"`
	WorkspaceCreatedAt string    `json:"workspace_created_at"`
	WorkspaceUpdatedAt string    `json:"workspace_updated_at"`
	LastMessageAt      string    `json:"last_message_at"`
	ContactsCount      int       `json:"contacts_count"`
	BroadcastsCount    int       `json:"broadcasts_count"`
	TransactionalCount int       `json:"transactional_count"`
	MessagesCount      int       `json:"messages_count"`
	ListsCount         int       `json:"lists_count"`
	SegmentsCount      int       `json:"segments_count"`
	UsersCount         int       `json:"users_count"`
	BlogPostsCount     int       `json:"blog_posts_count"`
	APIEndpoint        string    `json:"api_endpoint"`
	Source             string    `json:"source"`
	EventType          string    `json:"event_type"`

	// Integration flags - boolean for each email provider
	Mailgun   bool `json:"mailgun"`
	AmazonSES bool `json:"amazonses"`
	Mailjet   bool `json:"mailjet"`
	SparkPost bool `json:"sparkpost"`
	Postmark  bool `json:"postmark"`
	SMTP      bool `json:"smtp"`
	S3        bool `json:"s3"`

	Anthropic bool `json:"anthropic"`
	OpenAI    bool `json:"openai"`
	Gemini    bool `json:"gemini"`
	Supabase  bool `json:"supabase"`
	Firecrawl bool `json:"firecrawl"`

	WebAnalytics bool `json:"web_analytics"`

	// Pointers, and omitempty, so that "the sender did not report this" is a value the
	// column can hold. All five arrived with v40; every pre-v40 sender omits them, and as
	// plain bool/string they decoded to false/"" and were WRITTEN as false/"" —
	// indistinguishable in BigQuery from an upgraded instance that genuinely has SSO off
	// and no custom RBAC. The blast-radius measurement that decides an irreversible licence
	// change is taken over exactly these columns, and it would have counted every
	// un-upgraded instance in the `false` bucket.
	//
	// The sender never omits them (internal/service/telemetry_service.go carries no
	// omitempty), so a non-nil pointer to false or "" still travels and still lands as
	// false or "". Absent means absent, and only that.
	SESTenant   *bool   `json:"ses_tenant,omitempty"`
	RBACCustom  *bool   `json:"rbac_custom,omitempty"`
	Version     *string `json:"version,omitempty"`
	OIDCEnabled *bool   `json:"oidc_enabled,omitempty"`
	LicenseTier *string `json:"license_tier,omitempty"`
}

var (
	loggingClient  *logging.Client
	logger         *logging.Logger
	loggingOnce    sync.Once
	loggingInitErr error
)

func init() {
	// Register the HTTP function
	functions.HTTP("ReceiveTelemetry", receiveTelemetry)
}

// getLogger builds the Cloud Logging client on first use and returns it.
//
// This used to live in init() and log.Fatalf on failure. That crash-looped the
// whole instance on a misconfiguration, and — the reason it moved — made the
// package impossible to load in a test, because init() runs before any test
// does. There is therefore no unit test in this module today for the payload
// mapping or for the BigQuery schema, which is precisely how five fields came to
// be sent by the platform and dropped on the floor here.
//
// Deferring it costs one sync.Once and turns a cold-start crash into a 500 on
// the request that could not be logged: the same telemetry is lost either way,
// and this way the reason appears in the function log next to the request.
func getLogger() (*logging.Logger, error) {
	loggingOnce.Do(func() {
		projectID := os.Getenv("GCP_PROJECT")
		if projectID == "" {
			loggingInitErr = errors.New("GCP_PROJECT environment variable is not set")
			return
		}

		client, err := logging.NewClient(context.Background(), projectID)
		if err != nil {
			loggingInitErr = fmt.Errorf("failed to create logging client: %w", err)
			return
		}

		loggingClient = client
		logger = client.Logger("telemetry")
	})

	return logger, loggingInitErr
}

// buildLogEntry maps a received payload onto the row shape BigQuery ingests.
//
// It is a separate function so it can be tested without a Cloud Logging client.
// Every field the sender serialises has to be copied here AND declared in
// bigquery_schema.json; a field missing from either one is silently discarded,
// with no error anywhere and a perfectly healthy 200 going back to the sender.
// TestLogEntryMatchesBigQuerySchema exists to make that omission a red test
// rather than a column of NULLs discovered months later.
func buildLogEntry(metrics TelemetryMetrics, receivedAt time.Time) LogEntry {
	return LogEntry{
		Timestamp:          receivedAt,
		WorkspaceIDSHA1:    metrics.WorkspaceIDSHA1,
		WorkspaceCreatedAt: metrics.WorkspaceCreatedAt,
		WorkspaceUpdatedAt: metrics.WorkspaceUpdatedAt,
		LastMessageAt:      metrics.LastMessageAt,
		ContactsCount:      metrics.ContactsCount,
		BroadcastsCount:    metrics.BroadcastsCount,
		TransactionalCount: metrics.TransactionalCount,
		MessagesCount:      metrics.MessagesCount,
		ListsCount:         metrics.ListsCount,
		SegmentsCount:      metrics.SegmentsCount,
		UsersCount:         metrics.UsersCount,
		BlogPostsCount:     metrics.BlogPostsCount,
		APIEndpoint:        metrics.APIEndpoint,
		Source:             "notifuse-platform",
		EventType:          "telemetry_metrics",
		Mailgun:            metrics.Mailgun,
		AmazonSES:          metrics.AmazonSES,
		Mailjet:            metrics.Mailjet,
		SparkPost:          metrics.SparkPost,
		Postmark:           metrics.Postmark,
		SMTP:               metrics.SMTP,
		S3:                 metrics.S3,
		Anthropic:          metrics.Anthropic,
		OpenAI:             metrics.OpenAI,
		Gemini:             metrics.Gemini,
		Supabase:           metrics.Supabase,
		Firecrawl:          metrics.Firecrawl,
		WebAnalytics:       metrics.WebAnalytics,
		SESTenant:          metrics.SESTenant,
		RBACCustom:         metrics.RBACCustom,
		Version:            metrics.Version,
		OIDCEnabled:        metrics.OIDCEnabled,
		LicenseTier:        metrics.LicenseTier,
	}
}

// receiveTelemetry is the main HTTP function handler
func receiveTelemetry(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers to allow requests from any origin
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, User-Agent")

	// Handle preflight OPTIONS request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only accept POST requests
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// check if user agent contains "Notifuse-Telemetry"
	userAgent := r.Header.Get("User-Agent")
	if !strings.Contains(userAgent, "Notifuse-Telemetry") {
		// Fail silently - return success but don't process the request
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := map[string]interface{}{
			"status":    "success",
			"message":   "Request received",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Parse JSON payload
	var metrics TelemetryMetrics
	if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
		log.Printf("Failed to decode JSON payload: %v", err)
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Create structured log entry
	logEntry := buildLogEntry(metrics, time.Now())

	// Log to Google Cloud Logging with structured data
	cloudLogger, err := getLogger()
	if err != nil {
		// Loud rather than silent: a receiver that cannot log is dropping every
		// payload it accepts, and a 200 here would hide that from both sides.
		log.Printf("Cloud Logging is unavailable: %v", err)
		http.Error(w, "Telemetry sink unavailable", http.StatusInternalServerError)
		return
	}

	cloudLogger.Log(logging.Entry{
		Severity: logging.Info,
		Payload:  logEntry,
		Labels: map[string]string{
			"workspace_id_sha1": metrics.WorkspaceIDSHA1,
			"event_type":        "telemetry_metrics",
			"source":            "notifuse-platform",
		},
	})

	// Print the complete logEntry JSON for debugging
	logEntryJSON, err := json.MarshalIndent(logEntry, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal logEntry to JSON: %v", err)
	} else {
		log.Printf("LogEntry JSON:\n%s", string(logEntryJSON))
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"status":    "success",
		"message":   "Telemetry data received and logged",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	json.NewEncoder(w).Encode(response)
}

// Cleanup function to close the logging client (called automatically by Cloud Functions runtime)
func cleanup() {
	if loggingClient != nil {
		loggingClient.Close()
	}
}
