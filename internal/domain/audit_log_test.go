package domain

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditRecord_IsSafeOnNil(t *testing.T) {
	var record *AuditRecord
	assert.NotPanics(t, func() {
		record.SetActorClaims("u1", "user")
		record.SetActor(&User{ID: "u1"}, nil, false)
		record.SetWorkspace("ws")
		record.SetTarget("template", "t1", "Welcome")
		record.SetChanges(map[string]AuditChange{"name": {Old: "a", New: "b"}})
		record.AddMetadata("k", "v")
		record.SetAuthMethod(AuditAuthSSO)
		record.Fail("reason")
		record.Skip()
		_, skipped := record.Finalize(200)
		assert.True(t, skipped)
	})
	assert.Nil(t, AuditFromContext(context.Background()))
	assert.Nil(t, AuditFromContext(nil)) //nolint:staticcheck // nil ctx is the documented no-op case
}

func TestAuditRecord_RoundTripsThroughContext(t *testing.T) {
	record := NewAuditRecord("203.0.113.9", "curl/8", "req-1")
	ctx := WithAuditRecord(context.Background(), record)
	assert.Same(t, record, AuditFromContext(ctx))

	// A derived context still finds it: the auth middleware and the services
	// run on children of the middleware's context.
	child := context.WithValue(ctx, UserIDKey, "u1")
	assert.Same(t, record, AuditFromContext(child))
}

func TestAuditRecord_ActorClaimsThenResolvedUser(t *testing.T) {
	record := NewAuditRecord("ip", "ua", "rid")
	record.SetActorClaims("u1", string(UserTypeUser))
	record.SetActor(&User{ID: "u1", Email: "ann@example.com", Name: "Ann", Type: UserTypeUser},
		&UserWorkspace{WorkspaceID: "ws1", Role: "member"}, false)

	event, skipped := record.Finalize(200)
	require.False(t, skipped)
	assert.Equal(t, AuditActorUser, event.ActorType)
	assert.Equal(t, "u1", event.ActorID)
	assert.Equal(t, "ann@example.com", event.ActorEmail)
	assert.Equal(t, "Ann", event.ActorName)
	assert.Equal(t, "member", event.ActorRole)
	assert.Equal(t, AuditAuthSession, event.AuthMethod)
	assert.Equal(t, "ws1", event.WorkspaceID, "the membership names the workspace when nothing else did")
	assert.Equal(t, "ip", event.IPAddress)
	assert.Equal(t, "ua", event.UserAgent)
	assert.Equal(t, "rid", event.RequestID)
	assert.Equal(t, 200, event.StatusCode)
	assert.Equal(t, AuditOutcomeSuccess, event.Outcome)
}

func TestAuditRecord_APIKeyAndRoot(t *testing.T) {
	t.Run("api key claims", func(t *testing.T) {
		record := NewAuditRecord("", "", "")
		record.SetActorClaims("k1", string(UserTypeAPIKey))
		event, _ := record.Finalize(200)
		assert.Equal(t, AuditActorAPIKey, event.ActorType)
		assert.Equal(t, AuditAuthAPIKey, event.AuthMethod)
	})
	t.Run("root outranks the membership row", func(t *testing.T) {
		record := NewAuditRecord("", "", "")
		record.SetActor(&User{ID: "r", Email: "root@example.com"}, &UserWorkspace{Role: "member"}, true)
		event, _ := record.Finalize(200)
		assert.Equal(t, AuditRoleRoot, event.ActorRole)
	})
	t.Run("a login method set by the service is kept", func(t *testing.T) {
		record := NewAuditRecord("", "", "")
		record.SetAuthMethod(AuditAuthMagicCode)
		record.SetActor(&User{ID: "u"}, nil, false)
		event, _ := record.Finalize(200)
		assert.Equal(t, AuditAuthMagicCode, event.AuthMethod)
	})
}

func TestAuditRecord_OutcomeFromStatusUnlessForced(t *testing.T) {
	cases := map[int]string{200: AuditOutcomeSuccess, 201: AuditOutcomeSuccess, 302: AuditOutcomeSuccess, 400: AuditOutcomeFailure,
		402: AuditOutcomeDenied, 403: AuditOutcomeDenied, 500: AuditOutcomeFailure}
	for status, want := range cases {
		event, _ := NewAuditRecord("", "", "").Finalize(status)
		assert.Equal(t, want, event.Outcome, "status %d", status)
	}

	// A sign-in for an unknown address answers 200 on purpose; the log still
	// says it failed, and why.
	record := NewAuditRecord("", "", "")
	record.Fail("unknown_email")
	event, _ := record.Finalize(200)
	assert.Equal(t, AuditOutcomeFailure, event.Outcome)
	assert.Equal(t, "unknown_email", event.Metadata["reason"])
}

func TestAuditRecord_SkipAndFinalize(t *testing.T) {
	record := NewAuditRecord("", "", "")
	record.Skip()
	_, skipped := record.Finalize(200)
	assert.True(t, skipped)

	// Enrichment after Finalize is ignored: a handler goroutine that outlives
	// the response cannot change a row that was already written.
	record = NewAuditRecord("", "", "")
	record.SetTarget("template", "t1", "")
	event, _ := record.Finalize(200)
	record.SetTarget("template", "t2", "")
	record.AddMetadata("late", true)
	assert.Equal(t, "t1", event.TargetID)
	assert.Nil(t, event.Metadata)
	again, _ := record.Finalize(200)
	assert.Equal(t, "t1", again.TargetID)
}

func TestAuditRecord_TargetKeepsKnownFields(t *testing.T) {
	record := NewAuditRecord("", "", "")
	record.SetTarget("template", "t1", "")
	record.SetTarget("", "", "Welcome")
	event, _ := record.Finalize(200)
	assert.Equal(t, "template", event.TargetType)
	assert.Equal(t, "t1", event.TargetID)
	assert.Equal(t, "Welcome", event.TargetName)
}

func TestAuditRecord_ConcurrentMetadata(t *testing.T) {
	record := NewAuditRecord("", "", "")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			record.AddMetadata("k", i)
			record.SetTarget("t", "id", "")
		}(i)
	}
	wg.Wait()
	event, _ := record.Finalize(200)
	assert.Contains(t, event.Metadata, "k")
}

type auditDiffFixture struct {
	Name         string         `json:"name"`
	SMTPPassword string         `json:"smtp_password"`
	Secret       string         `json:"-"`
	Nested       map[string]any `json:"nested,omitempty"`
	UpdatedAt    string         `json:"updated_at"`
}

func TestAuditDiff(t *testing.T) {
	t.Run("reports only what changed and redacts secrets", func(t *testing.T) {
		before := auditDiffFixture{Name: "A", SMTPPassword: "old", Secret: "s1", UpdatedAt: "1"}
		after := auditDiffFixture{Name: "B", SMTPPassword: "new", Secret: "s2", UpdatedAt: "2"}

		changes := AuditDiff(before, after)
		require.Len(t, changes, 2)
		assert.Equal(t, AuditChange{Old: "A", New: "B"}, changes["name"])
		assert.Equal(t, AuditChange{Old: AuditRedactedValue, New: AuditRedactedValue}, changes["smtp_password"],
			"the change is kept, the values are not")
		_, hasUpdatedAt := changes["updated_at"]
		assert.False(t, hasUpdatedAt, "bookkeeping fields are ignored")
		encoded, _ := json.Marshal(changes)
		assert.NotContains(t, string(encoded), "s1")
		assert.NotContains(t, string(encoded), "s2")
	})

	t.Run("nil when nothing changed", func(t *testing.T) {
		fixture := auditDiffFixture{Name: "A"}
		assert.Nil(t, AuditDiff(fixture, fixture))
		assert.Nil(t, AuditDiff(nil, nil))
		var nilPointer *auditDiffFixture
		assert.Nil(t, AuditDiff(nilPointer, nilPointer))
	})

	t.Run("nested objects are reported whole with inner secrets redacted", func(t *testing.T) {
		before := auditDiffFixture{Nested: map[string]any{"host": "a", "api_key": "k1"}}
		after := auditDiffFixture{Nested: map[string]any{"host": "b", "api_key": "k2"}}
		changes := AuditDiff(before, after)
		require.Contains(t, changes, "nested")
		newNested := changes["nested"].New.(map[string]any)
		assert.Equal(t, "b", newNested["host"])
		assert.Equal(t, AuditRedactedValue, newNested["api_key"])
	})

	t.Run("added and removed keys", func(t *testing.T) {
		changes := AuditDiff(map[string]any{"a": 1}, map[string]any{"b": 2})
		assert.Equal(t, AuditChange{Old: float64(1), New: nil}, changes["a"])
		assert.Equal(t, AuditChange{Old: nil, New: float64(2)}, changes["b"])
	})

	t.Run("large values become a size marker", func(t *testing.T) {
		big := strings.Repeat("x", AuditMaxValueBytes+10)
		changes := AuditDiff(map[string]any{"body": "small"}, map[string]any{"body": big})
		assert.Equal(t, "small", changes["body"].Old)
		assert.Contains(t, changes["body"].New, "[changed, ")
	})
}

func TestIsAuditSecretKey(t *testing.T) {
	for _, key := range []string{"password", "smtp_password", "client_secret", "OIDCClientSecret", "access_key_id", "Token", "api_key", "apiKey", "credentials", "private_key", "signature"} {
		assert.True(t, IsAuditSecretKey(key), key)
	}
	for _, key := range []string{"name", "host", "port", "from_email", "permissions", "retention_days"} {
		assert.False(t, IsAuditSecretKey(key), key)
	}
}

func TestRedactAuditChangesAndMetadata(t *testing.T) {
	changes := RedactAuditChanges(map[string]AuditChange{
		"token": {Old: "a", New: "b"},
		"name":  {Old: "a", New: "b"},
	})
	assert.Equal(t, AuditRedactedValue, changes["token"].New)
	assert.Equal(t, "b", changes["name"].New)

	metadata := RedactAuditMetadata(map[string]any{"scope": "full", "api_key": "x"})
	assert.Equal(t, "full", metadata["scope"])
	assert.Equal(t, AuditRedactedValue, metadata["api_key"])
	assert.Nil(t, RedactAuditMetadata(nil))
}

func TestAuditLogFilter_FromURLParams(t *testing.T) {
	t.Run("parses every filter", func(t *testing.T) {
		values := url.Values{}
		values.Set("workspace_id", "ws1")
		values.Set("actions", "templates.update, lists.create,")
		values.Set("categories", "templates")
		values.Set("actor_email", "ann@example.com")
		values.Set("actor_type", "api_key")
		values.Set("outcome", "denied")
		values.Set("target_type", "template")
		values.Set("target_id", "t1")
		values.Set("ip_address", "203.0.113.9")
		values.Set("from", "2026-01-01T00:00:00Z")
		values.Set("to", "2026-02-01T00:00:00Z")
		values.Set("limit", "20")

		var filter AuditLogFilter
		require.NoError(t, filter.FromURLParams(values))
		assert.Equal(t, AuditScopeWorkspace, filter.Scope)
		assert.Equal(t, []string{"templates.update", "lists.create"}, filter.Actions)
		assert.Equal(t, []string{"templates"}, filter.Categories)
		assert.Equal(t, "api_key", filter.ActorType)
		assert.Equal(t, "denied", filter.Outcome)
		assert.Equal(t, 20, filter.Limit)
		require.NotNil(t, filter.From)
		assert.Equal(t, 2026, filter.From.Year())
	})

	t.Run("rejects what it cannot query", func(t *testing.T) {
		for name, values := range map[string]url.Values{
			"no workspace":   {},
			"bad from":       {"workspace_id": {"ws"}, "from": {"yesterday"}},
			"bad to":         {"workspace_id": {"ws"}, "to": {"1"}},
			"bad limit":      {"workspace_id": {"ws"}, "limit": {"many"}},
			"bad outcome":    {"workspace_id": {"ws"}, "outcome": {"maybe"}},
			"bad actor type": {"workspace_id": {"ws"}, "actor_type": {"robot"}},
			"bad scope":      {"scope": {"galaxy"}},
			"bad cursor":     {"workspace_id": {"ws"}, "cursor": {"!!"}},
			"to before from": {"workspace_id": {"ws"}, "from": {"2026-02-01T00:00:00Z"}, "to": {"2026-01-01T00:00:00Z"}},
		} {
			var filter AuditLogFilter
			assert.Error(t, filter.FromURLParams(values), name)
		}
	})

	t.Run("clamps the limit and defaults the scope", func(t *testing.T) {
		filter := AuditLogFilter{WorkspaceID: "ws"}
		require.NoError(t, filter.Validate())
		assert.Equal(t, AuditLogDefaultListLimit, filter.Limit)

		filter = AuditLogFilter{WorkspaceID: "ws", Limit: 10_000}
		require.NoError(t, filter.Validate())
		assert.Equal(t, AuditLogMaxListLimit, filter.Limit)
	})

	t.Run("deployment scope needs no workspace", func(t *testing.T) {
		filter := AuditLogFilter{Scope: AuditScopeDeployment}
		assert.NoError(t, filter.Validate())
	})
}

func TestAuditCursor_RoundTrip(t *testing.T) {
	at := time.Date(2026, 9, 5, 10, 30, 0, 123456000, time.UTC)
	cursor := EncodeAuditCursor(at, "abc")
	decodedAt, id, err := DecodeAuditCursor(cursor)
	require.NoError(t, err)
	assert.True(t, at.Equal(decodedAt))
	assert.Equal(t, "abc", id)

	filter := AuditLogFilter{WorkspaceID: "ws", Cursor: cursor}
	assert.NoError(t, filter.Validate())

	_, _, err = DecodeAuditCursor("bm90LWEtY3Vyc29y") // "not-a-cursor"
	assert.Error(t, err)
}

func TestExportAuditLogsRequest_Validate(t *testing.T) {
	req := ExportAuditLogsRequest{AuditLogFilter: AuditLogFilter{WorkspaceID: "ws", Cursor: "ignored"}}
	require.NoError(t, req.Validate())
	assert.Equal(t, AuditExportFormatCSV, req.Format, "csv is the default")
	assert.Empty(t, req.Cursor, "an export always starts at the top")

	req = ExportAuditLogsRequest{AuditLogFilter: AuditLogFilter{WorkspaceID: "ws"}, Format: "xlsx"}
	assert.Error(t, req.Validate())
}

func TestAuditLogSettings(t *testing.T) {
	days := func(n int) *int { return &n }

	t.Run("validation", func(t *testing.T) {
		var nilSettings *AuditLogSettings
		assert.NoError(t, nilSettings.ValidateForSave())
		assert.NoError(t, (&AuditLogSettings{}).ValidateForSave())
		assert.NoError(t, (&AuditLogSettings{RetentionDays: days(0)}).ValidateForSave(), "0 keeps forever")
		assert.NoError(t, (&AuditLogSettings{RetentionDays: days(30)}).ValidateForSave())
		assert.NoError(t, (&AuditLogSettings{RetentionDays: days(3650)}).ValidateForSave())
		assert.Error(t, (&AuditLogSettings{RetentionDays: days(29)}).ValidateForSave())
		assert.Error(t, (&AuditLogSettings{RetentionDays: days(3651)}).ValidateForSave())
		assert.Error(t, (&AuditLogSettings{RetentionDays: days(-1)}).ValidateForSave())
	})

	t.Run("effective retention", func(t *testing.T) {
		var nilSettings *AuditLogSettings
		assert.Equal(t, 365, nilSettings.EffectiveRetentionDays(365))
		assert.Equal(t, 365, (&AuditLogSettings{}).EffectiveRetentionDays(365))
		assert.Equal(t, 0, (&AuditLogSettings{RetentionDays: days(0)}).EffectiveRetentionDays(365))
		assert.Equal(t, 90, (&AuditLogSettings{RetentionDays: days(90)}).EffectiveRetentionDays(365))
	})

	t.Run("request", func(t *testing.T) {
		_, _, err := (&SetAuditLogSettingsRequest{}).Validate()
		assert.Error(t, err)
		wsID, settings, err := (&SetAuditLogSettingsRequest{WorkspaceID: "ws", Settings: AuditLogSettings{RetentionDays: days(90)}}).Validate()
		require.NoError(t, err)
		assert.Equal(t, "ws", wsID)
		assert.Equal(t, 90, *settings.RetentionDays)
	})

	t.Run("round-trips inside workspace settings", func(t *testing.T) {
		settings := WorkspaceSettings{Timezone: "UTC", AuditLogs: &AuditLogSettings{RetentionDays: days(90)}}
		encoded, err := json.Marshal(settings)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"audit_logs":{"retention_days":90}`)

		var decoded WorkspaceSettings
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		require.NotNil(t, decoded.AuditLogs)
		assert.Equal(t, 90, *decoded.AuditLogs.RetentionDays)

		encoded, _ = json.Marshal(WorkspaceSettings{Timezone: "UTC"})
		assert.NotContains(t, string(encoded), "audit_logs")
	})
}

func TestNoopAuditLogRecorder(t *testing.T) {
	var recorder AuditLogRecorder = NoopAuditLogRecorder{}
	assert.NotPanics(t, func() { recorder.Record(context.Background(), &AuditLog{Action: "x"}) })
}

func TestAuditRecord_FailedAndHasTargetID(t *testing.T) {
	var nilRecord *AuditRecord
	assert.False(t, nilRecord.Failed())
	assert.False(t, nilRecord.HasTargetID())

	record := NewAuditRecord("", "", "")
	assert.False(t, record.Failed())
	assert.False(t, record.HasTargetID())

	record.Fail("invalid_code")
	assert.True(t, record.Failed(), "a service marked the request failed, whatever the status will be")

	record.SetTarget("template", "t1", "")
	assert.True(t, record.HasTargetID())
}

func TestIsAuditSecretKey_AnchorsKey(t *testing.T) {
	// "key" is anchored to the end: an address that happens to mention the key
	// is not a secret.
	assert.False(t, IsAuditSecretKey("api_key_email"))
	assert.False(t, IsAuditSecretKey("key_count"))
	for _, key := range []string{"api_key", "apiKey", "SecretKey", "service_role_key", "private_key", "token", "access_key_id"} {
		assert.True(t, IsAuditSecretKey(key), key)
	}
}
