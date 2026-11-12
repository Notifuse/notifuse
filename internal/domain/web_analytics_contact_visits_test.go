package domain

import (
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContactVisitsRequest_FromQuery(t *testing.T) {
	parse := func(extra url.Values) (ContactVisitsRequest, error) {
		q := url.Values{"workspace_id": {"ws1"}, "email": {"Ann@Example.com "}}
		for k, v := range extra {
			q[k] = v
		}
		var req ContactVisitsRequest
		err := req.FromQuery(q)
		return req, err
	}

	t.Run("defaults: filter all, limit 20, email normalized", func(t *testing.T) {
		req, err := parse(nil)
		require.NoError(t, err)
		assert.Equal(t, "ann@example.com", req.Email)
		assert.Equal(t, ContactVisitsFilterAll, req.Filter)
		assert.Equal(t, 20, req.Limit)
	})

	t.Run("email required", func(t *testing.T) {
		_, err := parse(url.Values{"email": {"  "}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "email")
	})

	// PostgreSQL refuses a NUL byte, and invalid UTF-8 would be rewritten to U+FFFD and
	// match nobody: both are the caller's error, answered 400, not 500 or an empty page.
	t.Run("an email with a NUL byte or invalid UTF-8 refused", func(t *testing.T) {
		for _, bad := range []string{"a\x00b@example.com", "a\xffb@example.com"} {
			_, err := parse(url.Values{"email": {bad}})
			require.Error(t, err, "%q", bad)
			assert.Contains(t, err.Error(), "email")
		}
	})

	t.Run("workspace_id required", func(t *testing.T) {
		_, err := parse(url.Values{"workspace_id": {""}})
		require.Error(t, err)
	})

	t.Run("goal and email filters accepted, an unknown one refused", func(t *testing.T) {
		for _, f := range []string{"goal", "email", "all"} {
			req, err := parse(url.Values{"filter": {f}})
			require.NoError(t, err, f)
			assert.Equal(t, f, req.Filter)
		}
		_, err := parse(url.Values{"filter": {"bogus"}})
		require.Error(t, err)
	})

	t.Run("limit 1 and 50 accepted, 0, 51 and text refused", func(t *testing.T) {
		for _, l := range []string{"1", "50"} {
			_, err := parse(url.Values{"limit": {l}})
			require.NoError(t, err, l)
		}
		for _, l := range []string{"0", "51", "ten"} {
			_, err := parse(url.Values{"limit": {l}})
			require.Error(t, err, l)
		}
	})

	t.Run("a cursor made by the encoder accepted, garbage refused", func(t *testing.T) {
		cursor := EncodeContactVisitsCursor(time.Now(), "0190d5a4-1c2b-7000-8000-000000000001")
		_, err := parse(url.Values{"cursor": {cursor}})
		require.NoError(t, err)

		for _, bad := range []string{"%%%", EncodeContactVisitsCursor(time.Now(), "not-a-uuid"), "bm9waXBl"} {
			_, err := parse(url.Values{"cursor": {bad}})
			require.Error(t, err, bad)
		}
	})
}

func TestContactVisitsCursor_RoundTrip(t *testing.T) {
	at := time.Date(2026, 11, 3, 9, 15, 30, 123456789, time.FixedZone("CET", 3600))
	gotAt, gotID, err := DecodeContactVisitsCursor(EncodeContactVisitsCursor(at, "0190D5A4-1C2B-7000-8000-000000000001"))
	require.NoError(t, err)
	assert.True(t, gotAt.Equal(at), "nanoseconds kept")
	assert.Equal(t, "0190d5a4-1c2b-7000-8000-000000000001", gotID)
}

func TestContactSessionPagesRequest_FromQuery(t *testing.T) {
	valid := url.Values{
		"workspace_id": {"ws1"},
		"email":        {"ANN@example.com"},
		"session_id":   {"0190D5A4-1C2B-7000-8000-000000000001"},
		"session_date": {"2026-11-03"},
	}

	var req ContactSessionPagesRequest
	require.NoError(t, req.FromQuery(valid))
	assert.Equal(t, "ann@example.com", req.Email)
	assert.Equal(t, "0190d5a4-1c2b-7000-8000-000000000001", req.SessionID)
	assert.Equal(t, "2026-11-03", req.SessionDate)

	for _, missing := range []string{"workspace_id", "email", "session_id", "session_date"} {
		q := url.Values{}
		for k, v := range valid {
			if k != missing {
				q[k] = v
			}
		}
		var r ContactSessionPagesRequest
		err := r.FromQuery(q)
		require.Error(t, err, missing)
		assert.Contains(t, err.Error(), missing)
	}

	for _, bad := range []struct{ field, value string }{
		{"session_id", "42"},
		{"session_date", "03/11/2026"},
		// Go reads year 0, PostgreSQL refuses it.
		{"session_date", "0000-01-01"},
		{"email", "a\x00b@example.com"},
		{"email", "a\xffb@example.com"},
	} {
		q := url.Values{}
		for k, v := range valid {
			q[k] = v
		}
		q.Set(bad.field, bad.value)
		var r ContactSessionPagesRequest
		err := r.FromQuery(q)
		require.Error(t, err, "%s=%q", bad.field, bad.value)
		assert.Contains(t, err.Error(), bad.field)
	}

	q := url.Values{}
	for k, v := range valid {
		q[k] = v
	}
	q.Set("session_date", "0001-01-01")
	var first ContactSessionPagesRequest
	require.NoError(t, first.FromQuery(q), "year 1 is the first PostgreSQL accepts")
}

func TestContactVisitsResponse_JSON(t *testing.T) {
	data, err := json.Marshal(ContactVisitsResponse{Sessions: []ContactVisitSession{{ID: "s1", SessionDate: "2026-11-03"}}})
	require.NoError(t, err)
	assert.NotContains(t, string(data), "summary", "no summary after the first page")
	assert.NotContains(t, string(data), "next_cursor")
	assert.Contains(t, string(data), `"first_goal_name":null`)
	assert.Contains(t, string(data), `"session_date":"2026-11-03"`)

	data, err = json.Marshal(ContactVisitsSummary{Goals: []ContactVisitGoal{}, TopPages: []ContactVisitPage{}})
	require.NoError(t, err)
	assert.Contains(t, string(data), `"first_session":null`)
	assert.Contains(t, string(data), `"goals":[]`)
}
