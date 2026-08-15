package domain

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validAnnotation is a fully resolved row, as the service hands one to the
// repository: colour and timezone already defaulted.
func validAnnotation() *Annotation {
	return &Annotation{
		ID:          "3f1c2a9b4d5e6f708192a3b4c5d6e7f8",
		AnnotatedAt: time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC),
		Timezone:    "Asia/Tokyo",
		Title:       "Product launch",
		Description: "Landing page went live",
		Color:       AnnotationDefaultColor,
		Source:      AnnotationSourceManual,
	}
}

func TestAnnotation_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Annotation)
		wantErr string
	}{
		{name: "happy path", mutate: func(*Annotation) {}},
		{name: "empty title", mutate: func(a *Annotation) { a.Title = "" }, wantErr: "title is required"},
		{
			name:    "title over the rune limit",
			mutate:  func(a *Annotation) { a.Title = strings.Repeat("a", AnnotationMaxTitleLength+1) },
			wantErr: "title must be 100 characters or less",
		},
		{
			name:    "description over the rune limit",
			mutate:  func(a *Annotation) { a.Description = strings.Repeat("a", AnnotationMaxDescriptionLength+1) },
			wantErr: "description must be 500 characters or less",
		},
		{name: "colour by name", mutate: func(a *Annotation) { a.Color = "red" }, wantErr: "hex color"},
		{name: "colour shorthand", mutate: func(a *Annotation) { a.Color = "#ABC" }, wantErr: "hex color"},
		{name: "colour non-hex digits", mutate: func(a *Annotation) { a.Color = "#GGGGGG" }, wantErr: "hex color"},
		{name: "colour missing hash", mutate: func(a *Annotation) { a.Color = "3b82f6" }, wantErr: "hex color"},
		{name: "empty colour", mutate: func(a *Annotation) { a.Color = "" }, wantErr: "hex color"},
		// time.LoadLocation would accept both of these; IsValidTimezone must not.
		{name: "empty timezone", mutate: func(a *Annotation) { a.Timezone = "" }, wantErr: "timezone is required"},
		{name: "Local timezone", mutate: func(a *Annotation) { a.Timezone = "Local" }, wantErr: "invalid timezone"},
		{name: "unknown timezone", mutate: func(a *Annotation) { a.Timezone = "Mars/Phobos" }, wantErr: "invalid timezone"},
		{name: "zero annotated_at", mutate: func(a *Annotation) { a.AnnotatedAt = time.Time{} }, wantErr: "annotated_at is required"},
		{name: "unknown source", mutate: func(a *Annotation) { a.Source = "api" }, wantErr: "source must be one of"},
		{name: "empty source", mutate: func(a *Annotation) { a.Source = "" }, wantErr: "source must be one of"},
		{name: "broadcast source", mutate: func(a *Annotation) { a.Source = AnnotationSourceBroadcast }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := validAnnotation()
			tt.mutate(a)

			err := a.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestAnnotation_Validate_CountsRunesNotBytes(t *testing.T) {
	// annotations.title is VARCHAR(100) — 100 characters, whichever they are. A
	// byte-counted bound would refuse a title Postgres stores happily.
	a := validAnnotation()
	a.Title = strings.Repeat("記", AnnotationMaxTitleLength)
	require.Greater(t, len(a.Title), AnnotationMaxTitleLength, "the fixture must overshoot a byte bound")
	assert.NoError(t, a.Validate())

	a.Title = strings.Repeat("記", AnnotationMaxTitleLength+1)
	assert.ErrorContains(t, a.Validate(), "title must be 100 characters or less")

	a.Title = "ok"
	a.Description = strings.Repeat("記", AnnotationMaxDescriptionLength)
	assert.NoError(t, a.Validate())

	a.Description = strings.Repeat("記", AnnotationMaxDescriptionLength+1)
	assert.ErrorContains(t, a.Validate(), "description must be 500 characters or less")
}

func TestAnnotation_Validate_AcceptsDeprecatedTimezoneAlias(t *testing.T) {
	// Rows written before the rename must stay editable.
	a := validAnnotation()
	a.Timezone = "Europe/Kiev"
	assert.NoError(t, a.Validate())
}

func TestAnnotation_IsSystem(t *testing.T) {
	manual := &Annotation{Source: AnnotationSourceManual}
	assert.False(t, manual.IsSystem())

	broadcast := &Annotation{Source: AnnotationSourceBroadcast}
	assert.True(t, broadcast.IsSystem())
}

func TestCreateAnnotationRequest_Validate(t *testing.T) {
	// Raw JSON, not struct literals: only a body can express a field that is
	// absent rather than zero, which is how a caller actually gets this wrong.
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "happy path",
			body: `{"workspace_id":"ws1","annotated_at":"2026-08-15T09:00:00Z","title":"Launch"}`,
		},
		{
			name: "colour and timezone may be omitted, the service defaults them",
			body: `{"workspace_id":"ws1","annotated_at":"2026-08-15T09:00:00Z","title":"Launch"}`,
		},
		{name: "empty body", body: `{}`, wantErr: "workspace_id is required"},
		{name: "workspace only", body: `{"workspace_id":"ws1"}`, wantErr: "title is required"},
		{
			name:    "null annotated_at",
			body:    `{"workspace_id":"ws1","title":"Launch","annotated_at":null}`,
			wantErr: "annotated_at is required",
		},
		{
			name:    "missing annotated_at",
			body:    `{"workspace_id":"ws1","title":"Launch"}`,
			wantErr: "annotated_at is required",
		},
		{
			name:    "bad colour",
			body:    `{"workspace_id":"ws1","annotated_at":"2026-08-15T09:00:00Z","title":"Launch","color":"red"}`,
			wantErr: "hex color",
		},
		{
			name:    "bad timezone",
			body:    `{"workspace_id":"ws1","annotated_at":"2026-08-15T09:00:00Z","title":"Launch","timezone":"Local"}`,
			wantErr: "invalid timezone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req CreateAnnotationRequest
			require.NoError(t, json.Unmarshal([]byte(tt.body), &req))

			err := req.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestCreateAnnotationRequest_Validate_RejectsSystemSource(t *testing.T) {
	var req CreateAnnotationRequest
	require.NoError(t, json.Unmarshal([]byte(
		`{"workspace_id":"ws1","annotated_at":"2026-08-15T09:00:00Z","title":"Launch","source":"broadcast"}`,
	), &req))
	assert.ErrorContains(t, req.Validate(), `source must be "manual"`)

	// An explicit "manual" and an absent source are both fine.
	require.NoError(t, json.Unmarshal([]byte(
		`{"workspace_id":"ws1","annotated_at":"2026-08-15T09:00:00Z","title":"Launch","source":"manual"}`,
	), &req))
	assert.NoError(t, req.Validate())
}

func TestUpdateAnnotationRequest_Validate(t *testing.T) {
	var req UpdateAnnotationRequest
	require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"ws1"}`), &req))
	assert.ErrorContains(t, req.Validate(), "id is required")

	require.NoError(t, json.Unmarshal([]byte(
		`{"workspace_id":"ws1","id":"a1","annotated_at":"2026-08-15T09:00:00Z","title":"Launch"}`,
	), &req))
	assert.NoError(t, req.Validate())
}

func TestDeleteAnnotationRequest_Validate(t *testing.T) {
	var req DeleteAnnotationRequest
	require.NoError(t, json.Unmarshal([]byte(`{}`), &req))
	assert.ErrorContains(t, req.Validate(), "workspace_id is required")

	require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"ws1"}`), &req))
	assert.ErrorContains(t, req.Validate(), "id is required")

	require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"ws1","id":"a1"}`), &req))
	assert.NoError(t, req.Validate())
}

func TestListAnnotationsRequest_Validate(t *testing.T) {
	start := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	t.Run("workspace_id is required", func(t *testing.T) {
		req := &ListAnnotationsRequest{}
		assert.ErrorContains(t, req.Validate(), "workspace_id is required")
	})

	t.Run("end before start", func(t *testing.T) {
		req := &ListAnnotationsRequest{WorkspaceID: "ws1", Start: &end, End: &start}
		assert.ErrorContains(t, req.Validate(), "end must not be before start")
	})

	t.Run("an open-ended range is fine", func(t *testing.T) {
		req := &ListAnnotationsRequest{WorkspaceID: "ws1", Start: &start}
		assert.NoError(t, req.Validate())
	})

	t.Run("unknown source", func(t *testing.T) {
		req := &ListAnnotationsRequest{WorkspaceID: "ws1", Sources: []string{"manual", "api"}}
		assert.ErrorContains(t, req.Validate(), "source must be one of")
	})

	t.Run("limit defaults when unset", func(t *testing.T) {
		req := &ListAnnotationsRequest{WorkspaceID: "ws1"}
		require.NoError(t, req.Validate())
		assert.Equal(t, AnnotationDefaultListLimit, req.Limit)
	})

	t.Run("limit is clamped, not rejected", func(t *testing.T) {
		req := &ListAnnotationsRequest{WorkspaceID: "ws1", Limit: AnnotationMaxListLimit + 1}
		require.NoError(t, req.Validate())
		assert.Equal(t, AnnotationMaxListLimit, req.Limit)
	})

	t.Run("a negative limit falls back to the default", func(t *testing.T) {
		req := &ListAnnotationsRequest{WorkspaceID: "ws1", Limit: -5}
		require.NoError(t, req.Validate())
		assert.Equal(t, AnnotationDefaultListLimit, req.Limit)
	})
}

func TestListAnnotationsRequest_FromURLParams(t *testing.T) {
	t.Run("parses a full query", func(t *testing.T) {
		var req ListAnnotationsRequest
		require.NoError(t, req.FromURLParams(url.Values{
			"workspace_id": {"ws1"},
			"start":        {"2026-08-15T00:00:00Z"},
			"end":          {"2026-08-16T00:00:00Z"},
			"sources":      {"manual, broadcast"},
			"limit":        {"25"},
		}))

		assert.Equal(t, "ws1", req.WorkspaceID)
		require.NotNil(t, req.Start)
		assert.True(t, req.Start.Equal(time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)))
		require.NotNil(t, req.End)
		assert.True(t, req.End.Equal(time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)))
		assert.Equal(t, []string{"manual", "broadcast"}, req.Sources)
		assert.Equal(t, 25, req.Limit)
	})

	t.Run("absent filters stay nil", func(t *testing.T) {
		var req ListAnnotationsRequest
		require.NoError(t, req.FromURLParams(url.Values{"workspace_id": {"ws1"}}))
		assert.Nil(t, req.Start)
		assert.Nil(t, req.End)
		assert.Empty(t, req.Sources)
		assert.Zero(t, req.Limit)
	})

	t.Run("workspace_id is required", func(t *testing.T) {
		var req ListAnnotationsRequest
		assert.ErrorContains(t, req.FromURLParams(url.Values{}), "workspace_id is required")
	})

	t.Run("a malformed timestamp is an error, not an ignored filter", func(t *testing.T) {
		var req ListAnnotationsRequest
		err := req.FromURLParams(url.Values{"workspace_id": {"ws1"}, "start": {"yesterday"}})
		require.ErrorContains(t, err, "start must be an RFC3339 timestamp")
		assert.Nil(t, req.Start, "a rejected value must not become a zero-time filter")

		err = req.FromURLParams(url.Values{"workspace_id": {"ws1"}, "end": {"2026-08-16"}})
		assert.ErrorContains(t, err, "end must be an RFC3339 timestamp")
	})

	t.Run("a malformed limit is an error", func(t *testing.T) {
		var req ListAnnotationsRequest
		err := req.FromURLParams(url.Values{"workspace_id": {"ws1"}, "limit": {"all"}})
		assert.ErrorContains(t, err, "limit must be an integer")
	})
}

func TestGetAnnotationRequest_FromURLParams(t *testing.T) {
	t.Run("parses both params", func(t *testing.T) {
		var req GetAnnotationRequest
		require.NoError(t, req.FromURLParams(url.Values{
			"workspace_id": {"ws1"},
			"id":           {"3f1c2a9b4d5e6f708192a3b4c5d6e7f8"},
		}))
		assert.Equal(t, "ws1", req.WorkspaceID)
		assert.Equal(t, "3f1c2a9b4d5e6f708192a3b4c5d6e7f8", req.ID)
	})

	t.Run("workspace_id is required", func(t *testing.T) {
		var req GetAnnotationRequest
		assert.ErrorContains(t, req.FromURLParams(url.Values{"id": {"a1"}}), "workspace_id is required")
	})

	t.Run("id is required", func(t *testing.T) {
		var req GetAnnotationRequest
		assert.ErrorContains(t, req.FromURLParams(url.Values{"workspace_id": {"ws1"}}), "id is required")
	})
}

func TestGetAnnotationRequest_Validate(t *testing.T) {
	var req GetAnnotationRequest
	require.NoError(t, json.Unmarshal([]byte(`{}`), &req))
	assert.ErrorContains(t, req.Validate(), "workspace_id is required")

	require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"ws1"}`), &req))
	assert.ErrorContains(t, req.Validate(), "id is required")

	require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"ws1","id":"a1"}`), &req))
	assert.NoError(t, req.Validate())
}

func TestAnnotation_JSONRoundTrip(t *testing.T) {
	sourceID := "broadcast-123"
	original := validAnnotation()
	original.Source = AnnotationSourceBroadcast
	original.SourceID = &sourceID

	raw, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Annotation
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, original.ID, decoded.ID)
	assert.True(t, original.AnnotatedAt.Equal(decoded.AnnotatedAt))
	assert.Equal(t, original.Timezone, decoded.Timezone)
	require.NotNil(t, decoded.SourceID)
	assert.Equal(t, sourceID, *decoded.SourceID)

	// A manual row must not ship a source_id key at all — the console keys its
	// "Broadcast" tag off source, but the API contract says absent, not null.
	manual, err := json.Marshal(validAnnotation())
	require.NoError(t, err)
	assert.NotContains(t, string(manual), "source_id")
}
