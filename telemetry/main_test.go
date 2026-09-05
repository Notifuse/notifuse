package telemetry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// jsonTagNames lists the JSON field names a struct serialises, in declaration
// order, skipping anything explicitly excluded with `json:"-"`.
func jsonTagNames(t *testing.T, value interface{}) []string {
	t.Helper()

	structType := reflect.TypeOf(value)
	names := make([]string, 0, structType.NumField())
	for i := 0; i < structType.NumField(); i++ {
		tag := structType.Field(i).Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			t.Fatalf("field %s has no json tag; it would be logged under its Go name", structType.Field(i).Name)
		}
		names = append(names, name)
	}
	return names
}

// bigQueryColumnNames reads the column names out of the deployed table schema.
func bigQueryColumnNames(t *testing.T) []string {
	t.Helper()

	raw, err := os.ReadFile("bigquery_schema.json")
	if err != nil {
		t.Fatalf("failed to read bigquery_schema.json: %v", err)
	}

	var columns []struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(raw, &columns); err != nil {
		t.Fatalf("bigquery_schema.json is not valid JSON: %v", err)
	}

	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(names, column.Name)
	}
	return names
}

// TestLogEntryMatchesBigQuerySchema is the anti-drift test this module was
// missing.
//
// A telemetry field has to survive four separate declarations to reach an
// analyst: the sender's struct, this receiver's TelemetryMetrics, this
// receiver's LogEntry, and a column in bigquery_schema.json. Nothing connects
// them at compile time. A field that is added to the first two and forgotten in
// either of the last two is dropped in perfect silence — the sender gets its 200,
// the function logs no error, and the column simply is not there. That is exactly
// what happened to ses_tenant, rbac_custom, version, oidc_enabled and
// license_tier, which were serialised by the platform for a full release and
// never reached BigQuery.
//
// The check runs both ways on purpose. A column with no field is dead weight in
// the table; a field with no column is data thrown away.
func TestLogEntryMatchesBigQuerySchema(t *testing.T) {
	entryFields := jsonTagNames(t, LogEntry{})
	columns := bigQueryColumnNames(t)

	columnSet := make(map[string]bool, len(columns))
	for _, column := range columns {
		columnSet[column] = true
	}
	fieldSet := make(map[string]bool, len(entryFields))
	for _, field := range entryFields {
		fieldSet[field] = true
	}

	t.Run("every logged field has a bigquery column", func(t *testing.T) {
		for _, field := range entryFields {
			if !columnSet[field] {
				t.Errorf("LogEntry field %q has no column in bigquery_schema.json; it is being thrown away", field)
			}
		}
	})

	t.Run("every bigquery column has a logged field", func(t *testing.T) {
		for _, column := range columns {
			if !fieldSet[column] {
				t.Errorf("bigquery_schema.json column %q is never written by LogEntry", column)
			}
		}
	})
}

// TestReceivedPayloadReachesTheLogEntry walks a payload the way a real request
// does — decode, then map — and asserts nothing is lost on the way.
//
// It compares the two structs field by field rather than listing them by hand,
// so a field added to TelemetryMetrics and forgotten in buildLogEntry fails here
// instead of arriving in BigQuery as a NULL.
func TestReceivedPayloadReachesTheLogEntry(t *testing.T) {
	// Every field set to a value distinguishable from its zero, so a field that
	// buildLogEntry forgets to copy shows up as the zero value.
	payload := TelemetryMetrics{
		WorkspaceIDSHA1:    "a1b2c3",
		WorkspaceCreatedAt: "2023-01-15T10:30:00Z",
		WorkspaceUpdatedAt: "2025-08-15T14:22:30Z",
		LastMessageAt:      "2025-08-20T09:45:12Z",
		ContactsCount:      1250,
		BroadcastsCount:    18,
		TransactionalCount: 87,
		MessagesCount:      3420,
		ListsCount:         7,
		SegmentsCount:      14,
		UsersCount:         5,
		BlogPostsCount:     12,
		APIEndpoint:        "https://api.example.com",
		Mailgun:            true,
		AmazonSES:          true,
		Mailjet:            true,
		SparkPost:          true,
		Postmark:           true,
		SMTP:               true,
		S3:                 true,
		Anthropic:          true,
		OpenAI:             true,
		Gemini:             true,
		Supabase:           true,
		Firecrawl:          true,
		WebAnalytics:       true,
		SESTenant:          ptr(true),
		RBACCustom:         ptr(true),
		Version:            ptr("40.0"),
		OIDCEnabled:        ptr(true),
		LicenseTier:        ptr("agency"),
	}

	receivedAt := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	entry := buildLogEntry(payload, receivedAt)

	entryValue := reflect.ValueOf(entry)
	entryType := entryValue.Type()
	metricsValue := reflect.ValueOf(payload)
	metricsType := metricsValue.Type()

	for i := 0; i < metricsType.NumField(); i++ {
		name := metricsType.Field(i).Name
		target, ok := entryType.FieldByName(name)
		if !ok {
			t.Errorf("TelemetryMetrics field %s has no counterpart on LogEntry", name)
			continue
		}
		got := entryValue.FieldByIndex(target.Index).Interface()
		want := metricsValue.Field(i).Interface()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("field %s was not carried into the log entry: got %v, want %v", name, got, want)
		}
	}

	// The three fields the receiver adds itself.
	if entry.Timestamp != receivedAt {
		t.Errorf("timestamp: got %v, want %v", entry.Timestamp, receivedAt)
	}
	if entry.Source != "notifuse-platform" {
		t.Errorf("source: got %q", entry.Source)
	}
	if entry.EventType != "telemetry_metrics" {
		t.Errorf("event type: got %q", entry.EventType)
	}
}

// TestTestPayloadDecodes keeps the checked-in sample honest: it is what a human
// curls at the deployed function, and a sample missing the newest fields tests
// the old shape.
func TestTestPayloadDecodes(t *testing.T) {
	raw, err := os.ReadFile("test_payload.json")
	if err != nil {
		t.Fatalf("failed to read test_payload.json: %v", err)
	}

	var metrics TelemetryMetrics
	if err := json.Unmarshal(raw, &metrics); err != nil {
		t.Fatalf("test_payload.json does not decode into TelemetryMetrics: %v", err)
	}

	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("test_payload.json is not a JSON object: %v", err)
	}

	for _, name := range jsonTagNames(t, TelemetryMetrics{}) {
		if _, ok := asMap[name]; !ok {
			t.Errorf("test_payload.json does not exercise the %q field", name)
		}
	}
}

// TestReceiveTelemetryRejections covers the paths that answer before any Cloud
// Logging client is needed. The success path needs credentials and belongs to
// the deployed function, not to a unit test.
func TestReceiveTelemetryRejections(t *testing.T) {
	cases := []struct {
		name       string
		method     string
		userAgent  string
		body       string
		wantStatus int
	}{
		{
			name:       "a preflight request is answered without a payload",
			method:     http.MethodOptions,
			wantStatus: http.StatusOK,
		},
		{
			name:       "a get request is refused",
			method:     http.MethodGet,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "a post from an unknown agent is accepted and ignored",
			method:     http.MethodPost,
			userAgent:  "curl/8.0",
			body:       `{"workspace_id_sha1":"abc"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "a malformed payload from the platform is a bad request",
			method:     http.MethodPost,
			userAgent:  "Notifuse-Telemetry/1.0",
			body:       `{"contacts_count":`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(testCase.method, "/", strings.NewReader(testCase.body))
			if testCase.userAgent != "" {
				request.Header.Set("User-Agent", testCase.userAgent)
			}
			recorder := httptest.NewRecorder()

			receiveTelemetry(recorder, request)

			if recorder.Code != testCase.wantStatus {
				t.Errorf("status: got %d, want %d", recorder.Code, testCase.wantStatus)
			}
		})
	}
}

// ptr is the shortest way to write an optional telemetry field in a fixture.
func ptr[T any](v T) *T { return &v }

// A payload from a sender that predates these fields must leave them NULL, not false.
//
// The five arrived with v40, and every pre-v40 sender omits them. As plain bool/string they
// decoded to false/"" and were written as false/"" — indistinguishable, in the table, from an
// upgraded instance that genuinely has SSO off and no custom RBAC. The measurement these
// columns exist for is the blast radius of an irreversible licence change, taken while most
// senders are still on the old version, so every un-upgraded instance — including every one
// running SSO — would have been counted in the `false` bucket and the number read as small.
func TestPreV40PayloadLeavesTheNewColumnsNull(t *testing.T) {
	// Exactly what a v39 sender puts on the wire: no ses_tenant, no rbac_custom, no
	// version, no oidc_enabled, no license_tier.
	const v39 = `{
		"workspace_id": "ws-1",
		"workspace_created_at": "2026-01-01T00:00:00Z",
		"contacts_count": 10,
		"api_endpoint": "https://api.example.com"
	}`

	var payload TelemetryMetrics
	if err := json.Unmarshal([]byte(v39), &payload); err != nil {
		t.Fatalf("decoding a v39 payload failed: %v", err)
	}

	entry := buildLogEntry(payload, time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC))

	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("encoding the row failed: %v", err)
	}

	var row map[string]interface{}
	if err := json.Unmarshal(encoded, &row); err != nil {
		t.Fatalf("re-decoding the row failed: %v", err)
	}

	for _, column := range []string{"ses_tenant", "rbac_custom", "version", "oidc_enabled", "license_tier"} {
		if _, present := row[column]; present {
			t.Errorf("%s is present in the row for a sender that never reported it (value %v); "+
				"BigQuery stores that as a value, not NULL, and every query over this column "+
				"then counts un-upgraded instances as if they had answered", column, row[column])
		}
	}

	// The columns the old sender DID report still travel, or the fix has thrown the
	// payload away rather than made one field optional.
	if row["contacts_count"] != float64(10) {
		t.Errorf("contacts_count did not survive: %v", row["contacts_count"])
	}
}

// The mirror, and the reason omitempty on a pointer is the right tool: a v40 sender that is
// unlicensed and has SSO off reports false and "" explicitly, and those must land as false
// and "" rather than being swept away with the absent ones.
func TestExplicitFalseFromAV40SenderIsNotDroppedAsAbsent(t *testing.T) {
	const v40 = `{
		"workspace_id": "ws-1",
		"workspace_created_at": "2026-01-01T00:00:00Z",
		"ses_tenant": false,
		"rbac_custom": false,
		"version": "40.0",
		"oidc_enabled": false,
		"license_tier": ""
	}`

	var payload TelemetryMetrics
	if err := json.Unmarshal([]byte(v40), &payload); err != nil {
		t.Fatalf("decoding a v40 payload failed: %v", err)
	}

	encoded, err := json.Marshal(buildLogEntry(payload, time.Now()))
	if err != nil {
		t.Fatalf("encoding the row failed: %v", err)
	}

	var row map[string]interface{}
	if err := json.Unmarshal(encoded, &row); err != nil {
		t.Fatalf("re-decoding the row failed: %v", err)
	}

	for column, want := range map[string]interface{}{
		"ses_tenant":   false,
		"rbac_custom":  false,
		"oidc_enabled": false,
		"license_tier": "",
		"version":      "40.0",
	} {
		got, present := row[column]
		if !present {
			t.Errorf("%s was dropped: an unlicensed v40 instance that answered must not be "+
				"indistinguishable from one that never spoke", column)
			continue
		}
		if got != want {
			t.Errorf("%s: got %v, want %v", column, got, want)
		}
	}
}
