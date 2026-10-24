package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- blog.go: UpdateBlogThemeRequest.UnmarshalJSON + FilesFor ---

func storedFiles() BlogThemeFiles {
	return BlogThemeFiles{
		HomeLiquid: "stored-home", CategoryLiquid: "stored-cat", PostLiquid: "stored-post",
		HeaderLiquid: "stored-header", FooterLiquid: "stored-footer", SharedLiquid: "stored-shared",
		StylesCSS: "stored-css", ScriptsJS: "stored-js",
	}
}

func TestCoverage_UpdateBlogThemeUnmarshalAndFilesFor(t *testing.T) {
	stored := storedFiles()

	t.Run("partial body replaces only named files", func(t *testing.T) {
		var req UpdateBlogThemeRequest
		require.NoError(t, json.Unmarshal([]byte(`{"version":3,"files":{"home.liquid":"new-home"}}`), &req))
		assert.Equal(t, 3, req.Version)
		assert.Equal(t, "new-home", req.Files.HomeLiquid)
		got, err := req.FilesFor(stored)
		require.NoError(t, err)
		want := stored
		want.HomeLiquid = "new-home"
		assert.Equal(t, want, got)
	})

	t.Run("explicit empty string clears exactly that file", func(t *testing.T) {
		var req UpdateBlogThemeRequest
		require.NoError(t, json.Unmarshal([]byte(`{"version":1,"files":{"styles.css":""}}`), &req))
		got, err := req.FilesFor(stored)
		require.NoError(t, err)
		want := stored
		want.StylesCSS = ""
		assert.Equal(t, want, got)
	})

	t.Run("body without files key preserves everything", func(t *testing.T) {
		var req UpdateBlogThemeRequest
		require.NoError(t, json.Unmarshal([]byte(`{"version":3}`), &req))
		got, err := req.FilesFor(stored)
		require.NoError(t, err)
		assert.Equal(t, stored, got)
	})

	t.Run("null files is a preserving no-op", func(t *testing.T) {
		var req UpdateBlogThemeRequest
		require.NoError(t, json.Unmarshal([]byte(`{"version":2,"files":null}`), &req))
		got, err := req.FilesFor(stored)
		require.NoError(t, err)
		assert.Equal(t, stored, got)
	})

	t.Run("full body replaces every file and keeps version and notes", func(t *testing.T) {
		body := `{"version":7,"notes":"refresh","files":{"home.liquid":"h","category.liquid":"c","post.liquid":"p","header.liquid":"hd","footer.liquid":"f","shared.liquid":"s","styles.css":"css","scripts.js":"js"}}`
		var req UpdateBlogThemeRequest
		require.NoError(t, json.Unmarshal([]byte(body), &req))
		assert.Equal(t, 7, req.Version)
		require.NotNil(t, req.Notes)
		assert.Equal(t, "refresh", *req.Notes)
		got, err := req.FilesFor(stored)
		require.NoError(t, err)
		assert.Equal(t, BlogThemeFiles{
			HomeLiquid: "h", CategoryLiquid: "c", PostLiquid: "p", HeaderLiquid: "hd",
			FooterLiquid: "f", SharedLiquid: "s", StylesCSS: "css", ScriptsJS: "js",
		}, got)
	})

	t.Run("invalid JSON and invalid files payload are exact errors", func(t *testing.T) {
		var req UpdateBlogThemeRequest
		assert.Error(t, json.Unmarshal([]byte(`{"version":`), &req))

		// A mistyped file value already fails while decoding the body.
		var req2 UpdateBlogThemeRequest
		err := json.Unmarshal([]byte(`{"version":1,"files":{"home.liquid":123}}`), &req2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot unmarshal number into Go struct field")

		// FilesFor's own guard covers a raw payload that bypassed that decode.
		req3 := &UpdateBlogThemeRequest{Version: 1}
		req3.rawFiles = []byte(`{"home.liquid":123}`)
		got, err := req3.FilesFor(stored)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid files:")
		assert.Equal(t, stored, got, "a bad payload must leave the stored files untouched")
	})

	t.Run("Go-built request with filled Files is the complete set", func(t *testing.T) {
		filled := BlogThemeFiles{HomeLiquid: "only-home"}
		req := &UpdateBlogThemeRequest{Version: 1, Files: filled}
		got, err := req.FilesFor(stored)
		require.NoError(t, err)
		assert.Equal(t, filled, got)
	})

	t.Run("Go-built request with zero Files changes nothing", func(t *testing.T) {
		req := &UpdateBlogThemeRequest{Version: 1}
		got, err := req.FilesFor(stored)
		require.NoError(t, err)
		assert.Equal(t, stored, got)
	})

	t.Run("empty stored plus partial body yields only named files", func(t *testing.T) {
		var req UpdateBlogThemeRequest
		require.NoError(t, json.Unmarshal([]byte(`{"version":1,"files":{"scripts.js":"x=1"}}`), &req))
		got, err := req.FilesFor(BlogThemeFiles{})
		require.NoError(t, err)
		assert.Equal(t, BlogThemeFiles{ScriptsJS: "x=1"}, got)
	})
}

// --- audit_log.go: SetWorkspace / SetChanges / Noop recorder ---

func TestCoverage_AuditRecordSetWorkspaceAndSetChanges(t *testing.T) {
	t.Run("SetWorkspace sets then overrides, empty keeps", func(t *testing.T) {
		r := NewAuditRecord("ip", "ua", "rid")
		r.SetWorkspace("ws-1")
		event, skipped := r.Finalize(200)
		require.False(t, skipped)
		assert.Equal(t, "ws-1", event.WorkspaceID)

		r2 := NewAuditRecord("", "", "")
		r2.SetWorkspace("ws-1")
		r2.SetWorkspace("")
		r2.SetWorkspace("ws-2")
		event2, _ := r2.Finalize(200)
		assert.Equal(t, "ws-2", event2.WorkspaceID)
	})

	t.Run("SetWorkspace after Finalize is ignored", func(t *testing.T) {
		r := NewAuditRecord("", "", "")
		r.SetWorkspace("before")
		event, _ := r.Finalize(200)
		r.SetWorkspace("after")
		again, _ := r.Finalize(200)
		assert.Equal(t, "before", event.WorkspaceID)
		assert.Equal(t, "before", again.WorkspaceID)
	})

	t.Run("SetChanges attaches the exact map, empty and nil keep prior", func(t *testing.T) {
		changes := map[string]AuditChange{"name": {Old: "a", New: "b"}}
		r := NewAuditRecord("", "", "")
		r.SetChanges(changes)
		event, _ := r.Finalize(200)
		assert.Equal(t, changes, event.Changes)

		r2 := NewAuditRecord("", "", "")
		r2.SetChanges(changes)
		r2.SetChanges(map[string]AuditChange{})
		r2.SetChanges(nil)
		event2, _ := r2.Finalize(200)
		assert.Equal(t, changes, event2.Changes, "empty/nil SetChanges must not clear")

		r3 := NewAuditRecord("", "", "")
		r3.SetChanges(nil)
		event3, _ := r3.Finalize(200)
		assert.Nil(t, event3.Changes)
	})

	t.Run("SetChanges after Finalize is ignored", func(t *testing.T) {
		r := NewAuditRecord("", "", "")
		changes := map[string]AuditChange{"name": {Old: "a", New: "b"}}
		r.SetChanges(changes)
		event, _ := r.Finalize(200)
		r.SetChanges(map[string]AuditChange{"name": {Old: "x", New: "y"}})
		again, _ := r.Finalize(200)
		assert.Equal(t, changes, event.Changes)
		assert.Equal(t, changes, again.Changes)
	})

	t.Run("nil record SetWorkspace and SetChanges are no-ops", func(t *testing.T) {
		var r *AuditRecord
		assert.NotPanics(t, func() {
			r.SetWorkspace("ws")
			r.SetChanges(map[string]AuditChange{"a": {Old: "1", New: "2"}})
		})
	})

	t.Run("Noop recorder accepts any log including nil", func(t *testing.T) {
		var recorder AuditLogRecorder = NoopAuditLogRecorder{}
		assert.NotPanics(t, func() {
			recorder.Record(context.Background(), &AuditLog{Action: "anything"})
			recorder.Record(context.Background(), nil)
		})
	})
}

// --- custom_event.go: Validate branches + IsDeleted ---

func validCustomEvent() CustomEvent {
	return CustomEvent{
		ExternalID: "ext-1", Email: "user@example.com", EventName: "orders/fulfilled",
		OccurredAt: time.Now(),
	}
}

func covStrPtr(s string) *string   { return &s }
func covF64Ptr(f float64) *float64 { return &f }

func TestCoverage_CustomEventValidateGoalBranches(t *testing.T) {
	t.Run("external_id length boundary 255 ok 256 fails", func(t *testing.T) {
		e := validCustomEvent()
		e.ExternalID = strings.Repeat("x", 255)
		assert.NoError(t, e.Validate())
		e.ExternalID = strings.Repeat("x", 256)
		err := e.Validate()
		require.Error(t, err)
		assert.Equal(t, "external_id must be 255 characters or less", err.Error())
	})

	t.Run("event_name length boundary 100 ok 101 fails", func(t *testing.T) {
		e := validCustomEvent()
		e.EventName = strings.Repeat("a", 100)
		assert.NoError(t, e.Validate())
		e.EventName = strings.Repeat("a", 101)
		err := e.Validate()
		require.Error(t, err)
		assert.Equal(t, "event_name must be 100 characters or less", err.Error())
	})

	t.Run("each requiring goal type without value fails with exact message", func(t *testing.T) {
		for _, gt := range []string{GoalTypePurchase, GoalTypeSubscription} {
			e := validCustomEvent()
			e.GoalType = covStrPtr(gt)
			err := e.Validate()
			require.Error(t, err, gt)
			assert.Equal(t, fmt.Sprintf("goal_value is required for goal_type '%s'", gt), err.Error())
		}
	})

	t.Run("optional goal types pass without value, all pass with value", func(t *testing.T) {
		for _, gt := range []string{GoalTypeLead, GoalTypeSignup, GoalTypeBooking, GoalTypeTrial, GoalTypeOther} {
			e := validCustomEvent()
			e.GoalType = covStrPtr(gt)
			assert.NoError(t, e.Validate(), gt)
		}
		for _, gt := range ValidGoalTypes {
			e := validCustomEvent()
			e.GoalType = covStrPtr(gt)
			e.GoalValue = covF64Ptr(9.99)
			assert.NoError(t, e.Validate(), gt)
		}
	})

	t.Run("negative goal value is allowed for refunds", func(t *testing.T) {
		e := validCustomEvent()
		e.GoalType = covStrPtr(GoalTypePurchase)
		e.GoalValue = covF64Ptr(-50.25)
		assert.NoError(t, e.Validate())
	})

	t.Run("invalid and empty goal types fail", func(t *testing.T) {
		for _, gt := range []string{"Purchase", "PURCHASE", "", "refund"} {
			e := validCustomEvent()
			e.GoalType = covStrPtr(gt)
			err := e.Validate()
			require.Error(t, err, gt)
			assert.Contains(t, err.Error(), "goal_type must be one of:")
		}
	})

	t.Run("goal_name boundary 100 ok 101 fails", func(t *testing.T) {
		e := validCustomEvent()
		e.GoalName = covStrPtr(strings.Repeat("n", 100))
		assert.NoError(t, e.Validate())
		e.GoalName = covStrPtr(strings.Repeat("n", 101))
		err := e.Validate()
		require.Error(t, err)
		assert.Equal(t, "goal_name must be 100 characters or less", err.Error())
	})

	t.Run("IsDeleted reflects DeletedAt exactly", func(t *testing.T) {
		e := validCustomEvent()
		assert.False(t, e.IsDeleted())
		now := time.Now()
		e.DeletedAt = &now
		assert.True(t, e.IsDeleted())
	})
}

func TestCoverage_UpsertCustomEventValidateGoalBranches(t *testing.T) {
	base := func() UpsertCustomEventRequest {
		return UpsertCustomEventRequest{
			WorkspaceID: "ws1", Email: "user@example.com",
			EventName: "orders/fulfilled", ExternalID: "ext-1",
		}
	}
	t.Run("external_id boundary and goal rules mirror CustomEvent", func(t *testing.T) {
		r := base()
		r.ExternalID = strings.Repeat("x", 255)
		assert.NoError(t, r.Validate())
		r.ExternalID = strings.Repeat("x", 256)
		err := r.Validate()
		require.Error(t, err)
		assert.Equal(t, "external_id must be 255 characters or less", err.Error())

		r = base()
		r.GoalType = covStrPtr(GoalTypeSubscription)
		err = r.Validate()
		require.Error(t, err)
		assert.Equal(t, "goal_value is required for goal_type 'subscription'", err.Error())

		r.GoalValue = covF64Ptr(0)
		assert.NoError(t, r.Validate(), "zero is a supplied value, distinct from nil")

		r = base()
		r.GoalType = covStrPtr("bogus")
		require.Error(t, r.Validate())

		r = base()
		r.GoalName = covStrPtr(strings.Repeat("n", 101))
		err = r.Validate()
		require.Error(t, err)
		assert.Equal(t, "goal_name must be 100 characters or less", err.Error())
	})
}

// --- workspace.go: SetWebAnalyticsSettingsRequest.Validate (line ~1812) ---

func TestCoverage_SetWebAnalyticsSettingsRequestValidate(t *testing.T) {
	t.Run("empty workspace_id exact error and empty returns", func(t *testing.T) {
		wsID, settings, err := (&SetWebAnalyticsSettingsRequest{}).Validate()
		assert.Equal(t, "", wsID)
		assert.Nil(t, settings)
		require.Error(t, err)
		assert.Equal(t, "invalid set web analytics settings request: workspace_id is required", err.Error())
	})

	t.Run("non-alphanumeric workspace_id exact error", func(t *testing.T) {
		for _, id := range []string{"ws 1", "ws-1", "ws_1", "ws.1"} {
			_, _, err := (&SetWebAnalyticsSettingsRequest{WorkspaceID: id}).Validate()
			require.Error(t, err, id)
			assert.Equal(t, "invalid set web analytics settings request: workspace_id must be alphanumeric", err.Error(), id)
		}
	})

	t.Run("length boundary 32 ok 33 fails", func(t *testing.T) {
		id32 := strings.Repeat("a", 32)
		wsID, settings, err := (&SetWebAnalyticsSettingsRequest{WorkspaceID: id32}).Validate()
		require.NoError(t, err)
		assert.Equal(t, id32, wsID)
		assert.Nil(t, settings, "nil settings clear the stored configuration and must validate")

		_, _, err = (&SetWebAnalyticsSettingsRequest{WorkspaceID: strings.Repeat("a", 33)}).Validate()
		require.Error(t, err)
		assert.Equal(t, "invalid set web analytics settings request: workspace_id length must be between 1 and 32", err.Error())
	})

	t.Run("settings validation errors propagate unchanged", func(t *testing.T) {
		_, _, err := (&SetWebAnalyticsSettingsRequest{
			WorkspaceID: "ws1",
			Settings:    &WebAnalyticsSettings{BounceThresholdSeconds: -1},
		}).Validate()
		require.Error(t, err)
		assert.Equal(t, "bounce_threshold_seconds must be >= 0", err.Error())

		_, _, err = (&SetWebAnalyticsSettingsRequest{
			WorkspaceID: "ws1",
			Settings:    &WebAnalyticsSettings{Enabled: true},
		}).Validate()
		require.Error(t, err)
		assert.Equal(t, "allowed_domains must list at least one domain to enable web analytics", err.Error())
	})

	t.Run("valid request returns the same settings pointer", func(t *testing.T) {
		settings := &WebAnalyticsSettings{Enabled: true, AllowedDomains: []string{"example.com"}}
		wsID, got, err := (&SetWebAnalyticsSettingsRequest{WorkspaceID: "ws1", Settings: settings}).Validate()
		require.NoError(t, err)
		assert.Equal(t, "ws1", wsID)
		assert.True(t, got == settings)

		disabled := &WebAnalyticsSettings{Enabled: false}
		_, got2, err := (&SetWebAnalyticsSettingsRequest{WorkspaceID: "ws1", Settings: disabled}).Validate()
		require.NoError(t, err)
		assert.True(t, got2 == disabled)
	})
}

// --- errors.go: Error() strings ---

func TestCoverage_ErrorsErrorStrings(t *testing.T) {
	assert.Equal(t, "task already running [task-9]", (&ErrTaskAlreadyRunning{TaskID: "task-9"}).Error())
	assert.Equal(t, "task already running []", (&ErrTaskAlreadyRunning{}).Error())

	assert.Equal(t, "validation error: field is broken", NewValidationError("field is broken").Error())
	assert.Equal(t, "validation error: ", ValidationError{}.Error())
	var ve ValidationError
	assert.True(t, errors.As(NewValidationError("x"), &ve))
	assert.Equal(t, "x", ve.Message)

	assert.Equal(t, "task timed out [t1] after 0 seconds", (&ErrTaskTimeout{TaskID: "t1"}).Error())
	assert.Equal(t, " not found with ID: ", (&ErrNotFound{}).Error())

	noWrap := &ErrTaskExecution{TaskID: "t2", Reason: "boom"}
	assert.Equal(t, "task execution failed [t2]: boom", noWrap.Error())
	assert.Nil(t, noWrap.Unwrap())
	assert.False(t, errors.Is(noWrap, errors.New("boom")))
}

// --- web_analytics.go: CanIdentifyFromEmailLinks ---

func TestCoverage_CanIdentifyFromEmailLinks(t *testing.T) {
	var nilSettings *WebAnalyticsSettings
	assert.False(t, nilSettings.CanIdentifyFromEmailLinks())
	assert.False(t, (&WebAnalyticsSettings{}).CanIdentifyFromEmailLinks())
	assert.False(t, (&WebAnalyticsSettings{Enabled: true, IdentifyFromEmailLinks: true}).CanIdentifyFromEmailLinks(),
		"no allowed domains means no identification")
	assert.False(t, (&WebAnalyticsSettings{Enabled: true, IdentifyFromEmailLinks: true, AllowedDomains: []string{}}).CanIdentifyFromEmailLinks())
	assert.False(t, (&WebAnalyticsSettings{Enabled: false, IdentifyFromEmailLinks: true, AllowedDomains: []string{"example.com"}}).CanIdentifyFromEmailLinks())
	assert.False(t, (&WebAnalyticsSettings{Enabled: true, IdentifyFromEmailLinks: false, AllowedDomains: []string{"example.com"}}).CanIdentifyFromEmailLinks())
	assert.True(t, (&WebAnalyticsSettings{Enabled: true, IdentifyFromEmailLinks: true, AllowedDomains: []string{"example.com"}}).CanIdentifyFromEmailLinks())
	assert.True(t, (&WebAnalyticsSettings{Enabled: true, IdentifyFromEmailLinks: true, AllowedDomains: []string{"a.com", "b.com"}}).CanIdentifyFromEmailLinks())
}

// --- email_queue.go: InFlight ---

func TestCoverage_EmailQueueSourceCountsInFlight(t *testing.T) {
	assert.Equal(t, int64(0), (&EmailQueueSourceCounts{}).InFlight())
	assert.Equal(t, int64(0), (&EmailQueueSourceCounts{FailedTerminal: 7}).InFlight(),
		"terminal failures are not in flight")
	assert.Equal(t, int64(10), (&EmailQueueSourceCounts{
		Pending: 1, Processing: 2, Paused: 3, FailedRetrying: 4, FailedTerminal: 100,
	}).InFlight())
	assert.Equal(t, int64(5), (&EmailQueueSourceCounts{Pending: 5}).InFlight())
	assert.Equal(t, int64(5), (&EmailQueueSourceCounts{Processing: 5}).InFlight())
	assert.Equal(t, int64(5), (&EmailQueueSourceCounts{Paused: 5}).InFlight())
	assert.Equal(t, int64(5), (&EmailQueueSourceCounts{FailedRetrying: 5}).InFlight())
}
