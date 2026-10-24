package domain

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduleBroadcastRequest_Validate(t *testing.T) {
	t.Run("missing workspace and id are rejected in order", func(t *testing.T) {
		require.EqualError(t, (&ScheduleBroadcastRequest{}).Validate(), "workspace_id is required")
		require.EqualError(t, (&ScheduleBroadcastRequest{WorkspaceID: "ws-1"}).Validate(), "broadcast id is required")
	})

	t.Run("send-now needs no schedule fields", func(t *testing.T) {
		require.NoError(t, (&ScheduleBroadcastRequest{
			WorkspaceID: "ws-1", ID: "b-1", SendNow: true,
		}).Validate())
	})

	t.Run("scheduled send requires both date and time", func(t *testing.T) {
		base := ScheduleBroadcastRequest{WorkspaceID: "ws-1", ID: "b-1"}
		require.EqualError(t, base.Validate(),
			"scheduled_date and scheduled_time are required when not sending immediately")

		base.ScheduledDate = "2026-11-01"
		require.EqualError(t, base.Validate(),
			"scheduled_date and scheduled_time are required when not sending immediately")
	})

	t.Run("date must keep the YYYY-MM-DD shape", func(t *testing.T) {
		for _, date := range []string{"2026/11/01", "2026-1-01", "01-11-2026", "20261101"} {
			req := &ScheduleBroadcastRequest{
				WorkspaceID: "ws-1", ID: "b-1",
				ScheduledDate: date, ScheduledTime: "09:30",
			}
			assert.EqualError(t, req.Validate(), "scheduled date must be in YYYY-MM-DD format", "date %q", date)
		}
	})

	t.Run("time must keep the HH:MM shape", func(t *testing.T) {
		for _, tm := range []string{"9:30", "09-30", "0930", "09:30:00"} {
			req := &ScheduleBroadcastRequest{
				WorkspaceID: "ws-1", ID: "b-1",
				ScheduledDate: "2026-11-01", ScheduledTime: tm,
			}
			assert.EqualError(t, req.Validate(), "scheduled time must be in HH:MM format", "time %q", tm)
		}
	})

	t.Run("an unknown timezone is rejected, a real one passes", func(t *testing.T) {
		req := &ScheduleBroadcastRequest{
			WorkspaceID: "ws-1", ID: "b-1",
			ScheduledDate: "2026-11-01", ScheduledTime: "09:30", Timezone: "Mars/Olympus",
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid timezone:")

		req.Timezone = "Europe/Paris"
		require.NoError(t, req.Validate())
	})

	t.Run("a well-formed schedule passes", func(t *testing.T) {
		require.NoError(t, (&ScheduleBroadcastRequest{
			WorkspaceID: "ws-1", ID: "b-1",
			ScheduledDate: "2026-11-01", ScheduledTime: "09:30",
		}).Validate())
	})
}

func TestBroadcastIDOnlyRequests_Validate(t *testing.T) {
	cases := []struct {
		name     string
		validate func(workspaceID, id string) error
	}{
		{"pause", func(ws, id string) error { return (&PauseBroadcastRequest{WorkspaceID: ws, ID: id}).Validate() }},
		{"retry", func(ws, id string) error { return (&RetryFailedBroadcastRequest{WorkspaceID: ws, ID: id}).Validate() }},
		{"resume", func(ws, id string) error { return (&ResumeBroadcastRequest{WorkspaceID: ws, ID: id}).Validate() }},
		{"cancel", func(ws, id string) error { return (&CancelBroadcastRequest{WorkspaceID: ws, ID: id}).Validate() }},
		{"delete", func(ws, id string) error { return (&DeleteBroadcastRequest{WorkspaceID: ws, ID: id}).Validate() }},
	}
	for _, tc := range cases {
		t.Run(tc.name+" requires workspace_id", func(t *testing.T) {
			require.EqualError(t, tc.validate("", "b-1"), "workspace_id is required")
		})
		t.Run(tc.name+" requires broadcast id", func(t *testing.T) {
			require.EqualError(t, tc.validate("ws-1", ""), "broadcast id is required")
		})
		t.Run(tc.name+" accepts both ids", func(t *testing.T) {
			require.NoError(t, tc.validate("ws-1", "b-1"))
		})
	}
}

func TestSendToIndividualRequest_Validate(t *testing.T) {
	require.EqualError(t, (&SendToIndividualRequest{}).Validate(), "workspace_id is required")
	require.EqualError(t, (&SendToIndividualRequest{WorkspaceID: "ws-1"}).Validate(), "broadcast_id is required")
	require.EqualError(t, (&SendToIndividualRequest{WorkspaceID: "ws-1", BroadcastID: "b-1"}).Validate(),
		"recipient_email is required")
	require.NoError(t, (&SendToIndividualRequest{
		WorkspaceID: "ws-1", BroadcastID: "b-1", RecipientEmail: "a@example.com",
	}).Validate())
}

func TestSelectWinnerAndGetTestResults_Validate(t *testing.T) {
	require.EqualError(t, (&SelectWinnerRequest{}).Validate(), "workspace_id is required")
	require.EqualError(t, (&SelectWinnerRequest{WorkspaceID: "ws-1"}).Validate(), "broadcast id is required")
	require.EqualError(t, (&SelectWinnerRequest{WorkspaceID: "ws-1", ID: "b-1"}).Validate(), "template_id is required")
	require.NoError(t, (&SelectWinnerRequest{WorkspaceID: "ws-1", ID: "b-1", TemplateID: "t-1"}).Validate())

	require.EqualError(t, (&GetTestResultsRequest{}).Validate(), "workspace_id is required")
	require.EqualError(t, (&GetTestResultsRequest{WorkspaceID: "ws-1"}).Validate(), "broadcast id is required")
	require.NoError(t, (&GetTestResultsRequest{WorkspaceID: "ws-1", ID: "b-1"}).Validate())
}

func TestGetBroadcastRequest_FromURLParams(t *testing.T) {
	t.Run("parses ids and with_templates", func(t *testing.T) {
		var req GetBroadcastRequest
		require.NoError(t, req.FromURLParams(url.Values{
			"workspace_id":   {"ws-1"},
			"id":             {"b-1"},
			"with_templates": {"true"},
		}))
		assert.Equal(t, "ws-1", req.WorkspaceID)
		assert.Equal(t, "b-1", req.ID)
		assert.True(t, req.WithTemplates)
	})

	t.Run("with_templates defaults to false when absent", func(t *testing.T) {
		var req GetBroadcastRequest
		require.NoError(t, req.FromURLParams(url.Values{
			"workspace_id": {"ws-1"}, "id": {"b-1"},
		}))
		assert.False(t, req.WithTemplates)
	})

	t.Run("missing ids fail in order", func(t *testing.T) {
		var req GetBroadcastRequest
		require.EqualError(t, req.FromURLParams(url.Values{}), "workspace_id is required")
		req = GetBroadcastRequest{}
		require.EqualError(t, req.FromURLParams(url.Values{"workspace_id": {"ws-1"}}), "id is required")
	})

	t.Run("a garbage with_templates is an error, not silently false", func(t *testing.T) {
		var req GetBroadcastRequest
		err := req.FromURLParams(url.Values{
			"workspace_id": {"ws-1"}, "id": {"b-1"}, "with_templates": {"maybe"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid with_templates parameter:")
	})
}

func TestGetBroadcastsRequest_FromURLParams(t *testing.T) {
	t.Run("single status fills both Status and Statuses", func(t *testing.T) {
		var req GetBroadcastsRequest
		require.NoError(t, req.FromURLParams(url.Values{
			"workspace_id": {"ws-1"}, "status": {"draft"},
		}))
		assert.Equal(t, "draft", req.Status)
		assert.Equal(t, []string{"draft"}, req.Statuses)
	})

	t.Run("comma-separated statuses are trimmed and split", func(t *testing.T) {
		var req GetBroadcastsRequest
		require.NoError(t, req.FromURLParams(url.Values{
			"workspace_id": {"ws-1"}, "status": {"draft, paused ,failed"},
		}))
		assert.Equal(t, []string{"draft", "paused", "failed"}, req.Statuses)
		// With several statuses, Status must stay empty: it is the legacy
		// single-value field and must never hold a comma-joined string.
		assert.Equal(t, "", req.Status)
	})

	t.Run("an unknown status is rejected by name", func(t *testing.T) {
		var req GetBroadcastsRequest
		err := req.FromURLParams(url.Values{
			"workspace_id": {"ws-1"}, "status": {"draft,drafty"},
		})
		require.EqualError(t, err, "invalid status: drafty")
	})

	t.Run("a degenerate status list is rejected", func(t *testing.T) {
		var req GetBroadcastsRequest
		err := req.FromURLParams(url.Values{
			"workspace_id": {"ws-1"}, "status": {",,"},
		})
		require.EqualError(t, err, "invalid status parameter: ,,")
	})

	t.Run("search is trimmed, pagination is parsed", func(t *testing.T) {
		var req GetBroadcastsRequest
		require.NoError(t, req.FromURLParams(url.Values{
			"workspace_id": {"ws-1"}, "search": {"  Promo "},
			"limit": {"10"}, "offset": {"30"}, "with_templates": {"true"},
		}))
		assert.Equal(t, "Promo", req.Search)
		assert.Equal(t, 10, req.Limit)
		assert.Equal(t, 30, req.Offset)
		assert.True(t, req.WithTemplates)
	})

	t.Run("non-numeric limit and offset are errors", func(t *testing.T) {
		var req GetBroadcastsRequest
		err := req.FromURLParams(url.Values{"workspace_id": {"ws-1"}, "limit": {"ten"}})
		require.ErrorContains(t, err, "invalid limit parameter:")

		req = GetBroadcastsRequest{}
		err = req.FromURLParams(url.Values{"workspace_id": {"ws-1"}, "offset": {"x"}})
		require.ErrorContains(t, err, "invalid offset parameter:")
	})

	t.Run("missing workspace_id fails before anything else", func(t *testing.T) {
		var req GetBroadcastsRequest
		require.EqualError(t, req.FromURLParams(url.Values{"status": {"draft"}}), "workspace_id is required")
	})
}

func TestGetTestResultsRequest_FromURLParams(t *testing.T) {
	var req GetTestResultsRequest
	require.NoError(t, req.FromURLParams(url.Values{
		"workspace_id": {"ws-1"}, "id": {"b-1"},
	}))
	assert.Equal(t, "ws-1", req.WorkspaceID)
	assert.Equal(t, "b-1", req.ID)
}

func TestFeedRequests_Validate(t *testing.T) {
	t.Run("refresh requires workspace, broadcast and url in order", func(t *testing.T) {
		require.EqualError(t, (&RefreshGlobalFeedRequest{}).Validate(), "workspace_id is required")
		require.EqualError(t, (&RefreshGlobalFeedRequest{WorkspaceID: "ws-1"}).Validate(), "broadcast_id is required")
		require.EqualError(t, (&RefreshGlobalFeedRequest{WorkspaceID: "ws-1", BroadcastID: "b-1"}).Validate(),
			"url is required")
	})

	t.Run("refresh accepts http but rejects other schemes and hostless urls", func(t *testing.T) {
		base := RefreshGlobalFeedRequest{WorkspaceID: "ws-1", BroadcastID: "b-1"}
		base.URL = "http://example.com/feed.json"
		require.NoError(t, base.Validate())

		base.URL = "ftp://example.com/feed.json"
		require.EqualError(t, base.Validate(), "url: URL must use http or https scheme")

		base.URL = "https://"
		require.EqualError(t, base.Validate(), "url: URL must have a host")
	})

	t.Run("recipient feed test requires https, unlike the global refresh", func(t *testing.T) {
		// This asymmetry is deliberate (recipient feeds render per-contact
		// data), so pin it: accepting http here would be a security regression.
		req := &TestRecipientFeedRequest{WorkspaceID: "ws-1", BroadcastID: "b-1", URL: "http://example.com/feed.json"}
		require.EqualError(t, req.Validate(), "url must use https scheme")

		req.URL = "https://example.com/feed.json"
		require.NoError(t, req.Validate())
	})

	t.Run("a header without name or value fails the whole request", func(t *testing.T) {
		req := &RefreshGlobalFeedRequest{
			WorkspaceID: "ws-1", BroadcastID: "b-1", URL: "https://example.com/feed.json",
			Headers: []DataFeedHeader{{Name: "", Value: "v"}},
		}
		require.EqualError(t, req.Validate(), "header name is required")

		req.Headers = []DataFeedHeader{{Name: "X-Key", Value: ""}}
		require.EqualError(t, req.Validate(), "header value is required")

		req.Headers = []DataFeedHeader{{Name: "X-Key", Value: "v"}}
		require.NoError(t, req.Validate())
	})
}

func TestBroadcastErrorTypes_Error(t *testing.T) {
	// ErrBroadcastRetryNotAllowed must surface its reason verbatim: the
	// handler shows it to the user as the 400 body.
	assert.Equal(t, "broadcast is not in a failed state",
		(&ErrBroadcastRetryNotAllowed{Reason: "broadcast is not in a failed state"}).Error())
	assert.Equal(t, "Contact not found with email: a@example.com",
		(&ErrContactNotFoundForFeed{Email: "a@example.com"}).Error())
}

func TestParseIntAndBoolParam(t *testing.T) {
	v, err := ParseIntParam("42")
	require.NoError(t, err)
	assert.Equal(t, 42, v)

	_, err = ParseIntParam("abc")
	require.Error(t, err)

	// The whole string must be the number. fmt.Sscanf("%d") silently accepted
	// "10x" as 10 (a typo'd limit paged wrong instead of failing); these are
	// the regression cases for that bug and must all error.
	for _, s := range []string{"10x", "10 ", "1.5", "1e3", ""} {
		_, err := ParseIntParam(s)
		assert.Error(t, err, "ParseIntParam(%q) must reject trailing garbage", s)
	}
	v, err = ParseIntParam("-3")
	require.NoError(t, err)
	assert.Equal(t, -3, v)

	b, err := ParseBoolParam("true")
	require.NoError(t, err)
	assert.True(t, b)

	// fmt.Sscanf("%t") scans ANY token as false with no error, so every one of
	// these silently became false before the fix; they must all error now.
	for _, s := range []string{"notabool", "maybe", "yes", "2", ""} {
		_, err := ParseBoolParam(s)
		assert.Error(t, err, "ParseBoolParam(%q) must be rejected", s)
	}
	// strconv.ParseBool spellings keep working.
	for s, want := range map[string]bool{"1": true, "0": false, "TRUE": true, "False": false} {
		b, err := ParseBoolParam(s)
		require.NoError(t, err, "ParseBoolParam(%q)", s)
		assert.Equal(t, want, b)
	}
}
