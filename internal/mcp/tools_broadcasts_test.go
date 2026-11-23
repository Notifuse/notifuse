package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func broadcastsReadPermission() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceBroadcasts: {Read: true}}
}

func reportPermissions() domain.UserPermissions {
	return domain.UserPermissions{
		domain.PermissionResourceBroadcasts:     {Read: true},
		domain.PermissionResourceMessageHistory: {Read: true},
	}
}

var broadcastsEpoch = time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

// fullBroadcast carries every field of a broadcast, the secret ones included.
func fullBroadcast() *domain.Broadcast {
	started := broadcastsEpoch.Add(time.Hour)
	fetched := broadcastsEpoch.Add(30 * time.Minute)
	return &domain.Broadcast{
		ID: "bc1", WorkspaceID: testWorkspaceID, Name: "November newsletter", ChannelType: "email",
		Status:   domain.BroadcastStatusTestCompleted,
		Audience: domain.AudienceSettings{List: "newsletter", Segments: []string{"vip"}, ExcludeUnsubscribed: true},
		Schedule: domain.ScheduleSettings{IsScheduled: true, ScheduledDate: "2026-11-20", ScheduledTime: "09:00", Timezone: "Europe/Paris"},
		TestSettings: domain.BroadcastTestSettings{
			Enabled: true, SamplePercentage: 20, AutoSendWinnerMetric: domain.TestWinnerMetricOpenRate, TestDurationHours: 4,
			Variations: []domain.BroadcastVariation{
				{VariationName: "A", TemplateID: "tpl_a", Metrics: &domain.VariationMetrics{Recipients: 120, Delivered: 118, Opens: 40}},
				{VariationName: "B", TemplateID: "tpl_b", SenderID: "s2", Template: &domain.Template{ID: "tpl_b", Name: "Joined template"}},
			},
		},
		UTMParameters: &domain.UTMParameters{Source: "notifuse", Medium: "email", Campaign: "nov"},
		Metadata:      domain.MapOfAny{"owner": "marketing"},
		DataFeed: &domain.DataFeedSettings{
			GlobalFeed:          &domain.GlobalFeedSettings{Enabled: true, URL: "https://feed.example.com/x", Headers: []domain.DataFeedHeader{{Name: "Authorization", Value: "Bearer feed-secret"}}},
			GlobalFeedData:      domain.MapOfAny{"price": "fetched-payload"},
			GlobalFeedFetchedAt: &fetched,
			RecipientFeed:       &domain.RecipientFeedSettings{Enabled: false, Headers: []domain.DataFeedHeader{}},
		},
		TestSentAt: &started, TestPhaseRecipientCount: 240, EnqueuedCount: 240,
		CreatedAt: broadcastsEpoch, UpdatedAt: broadcastsEpoch, StartedAt: &started,
	}
}

func TestBroadcastTools_Definitions(t *testing.T) {
	broadcasts := []Pair{{Resource: "broadcasts", Verb: "read"}}
	assertReadTool(t, catalogueTool(t, "list_broadcasts"), "list_broadcasts", broadcasts, []string{"workspace_id"})
	assertReadTool(t, catalogueTool(t, "get_broadcast"), "get_broadcast", broadcasts, []string{"workspace_id", "broadcast_id"})
	assertReadTool(t, catalogueTool(t, "get_broadcast_report"), "get_broadcast_report",
		[]Pair{{Resource: "broadcasts", Verb: "read"}, {Resource: "message_history", Verb: "read"}}, []string{"workspace_id", "broadcast_id"})
}

func TestListBroadcasts_SendsTheFiltersAndProjects(t *testing.T) {
	f := newFixture(t)
	f.reachable(broadcastsReadPermission())
	startedAfter := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	f.broadcasts.EXPECT().ListBroadcasts(gomock.Any(), domain.ListBroadcastsParams{
		WorkspaceID:  testWorkspaceID,
		Statuses:     []domain.BroadcastStatus{"processed", "testing"},
		Search:       "news",
		Limit:        20,
		StartedAfter: &startedAfter,
	}).Return(&domain.BroadcastListResponse{Broadcasts: []*domain.Broadcast{fullBroadcast()}, TotalCount: 1}, nil)

	result := f.callTool("list_broadcasts", map[string]any{
		"workspace_id": testWorkspaceID, "statuses": []string{"testing", "processed", "testing"}, "search": "  news ",
		"started_after": "2026-11-01T01:00:00+01:00",
	})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"broadcasts":[{
			"id":"bc1","name":"November newsletter","status":"test_completed",
			"audience":{"list":"newsletter","segments":["vip"],"exclude_unsubscribed":true},
			"schedule":{"is_scheduled":true,"scheduled_date":"2026-11-20","scheduled_time":"09:00","timezone":"Europe/Paris","use_recipient_timezone":false},
			"ab_test":{"enabled":true},
			"template_ids":["tpl_a","tpl_b"],
			"tracking_mode":"inherit",
			"enqueued_count":240,
			"created_at":"2026-11-18T10:00:00Z","updated_at":"2026-11-18T10:00:00Z","started_at":"2026-11-18T11:00:00Z",
			"completed_at":null,"cancelled_at":null,"paused_at":null,"pause_reason":null
		}],
		"total_count":1
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"data_feed", "feed-secret", "fetched-payload", "metadata", "Joined template", "channel_type", "workspace_id"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

// broadcasts.list pages by offset on (created_at, id): the cursor carries the
// next offset, bound to the filters.
func TestListBroadcasts_Pages(t *testing.T) {
	f := newFixture(t)
	f.reachable(broadcastsReadPermission())
	page := func(ids ...string) []*domain.Broadcast {
		out := []*domain.Broadcast{}
		for _, id := range ids {
			out = append(out, &domain.Broadcast{ID: id, Name: id, CreatedAt: broadcastsEpoch})
		}
		return out
	}
	f.broadcasts.EXPECT().ListBroadcasts(gomock.Any(), domain.ListBroadcastsParams{WorkspaceID: testWorkspaceID, Search: "x", Limit: 2}).
		Return(&domain.BroadcastListResponse{Broadcasts: page("c", "b"), TotalCount: 3}, nil)
	f.broadcasts.EXPECT().ListBroadcasts(gomock.Any(), domain.ListBroadcastsParams{WorkspaceID: testWorkspaceID, Search: "x", Limit: 2, Offset: 2}).
		Return(&domain.BroadcastListResponse{Broadcasts: page("a"), TotalCount: 3}, nil)

	var first struct {
		Broadcasts []struct {
			ID string `json:"id"`
		} `json:"broadcasts"`
		NextCursor string `json:"next_cursor"`
	}
	f.callTool("list_broadcasts", map[string]any{"workspace_id": testWorkspaceID, "search": "x", "limit": 2}).decode(t, &first)
	require.Len(t, first.Broadcasts, 2)
	require.NotEmpty(t, first.NextCursor)

	var second struct {
		Broadcasts []struct {
			ID string `json:"id"`
		} `json:"broadcasts"`
		NextCursor string `json:"next_cursor"`
	}
	f.callTool("list_broadcasts", map[string]any{"workspace_id": testWorkspaceID, "search": "x", "limit": 2, "cursor": first.NextCursor}).decode(t, &second)
	require.Len(t, second.Broadcasts, 1)
	assert.Equal(t, "a", second.Broadcasts[0].ID)
	assert.Empty(t, second.NextCursor, "absent on the last page")

	toolErr := f.callTool("list_broadcasts", map[string]any{"workspace_id": testWorkspaceID, "search": "y", "limit": 2, "cursor": first.NextCursor}).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code, "a cursor is bound to its filters")
}

func TestListBroadcasts_RefusesBadArgumentsBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(broadcastsReadPermission())
	negative, err := EncodeCursor("list_broadcasts", testWorkspaceID, map[string]any{}, -1)
	require.NoError(t, err)
	otherTool, err := EncodeCursor("list_lists", testWorkspaceID, nil, 2)
	require.NoError(t, err)

	for name, args := range map[string]map[string]any{
		"an unknown status":         {"statuses": []string{"sent"}},
		"a date that is not a time": {"started_after": "2026-11-01"},
		"limit above 50":            {"limit": 51},
		"garbage cursor":            {"cursor": "%%%"},
		"negative offset":           {"cursor": negative},
		"another tool's cursor":     {"cursor": otherTool},
		"with_templates":            {"with_templates": true},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("list_broadcasts", args).errorOf(t).Code)
		})
	}
}

func TestGetBroadcast_ProjectsWithoutFeedSecrets(t *testing.T) {
	f := newFixture(t)
	f.reachable(broadcastsReadPermission())
	f.broadcasts.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1", domain.GetBroadcastOptions{}).Return(fullBroadcast(), nil)

	result := f.callTool("get_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"broadcast":{
			"id":"bc1","name":"November newsletter","status":"test_completed",
			"audience":{"list":"newsletter","segments":["vip"],"exclude_unsubscribed":true},
			"schedule":{"is_scheduled":true,"scheduled_date":"2026-11-20","scheduled_time":"09:00","timezone":"Europe/Paris","use_recipient_timezone":false},
			"test_settings":{
				"enabled":true,"sample_percentage":20,"auto_send_winner":false,"auto_send_winner_metric":"open_rate","test_duration_hours":4,
				"variations":[
					{"variation_name":"A","template_id":"tpl_a","sender_id":"","metrics":{"recipients":120,"delivered":118,"opens":40,"clicks":0,"bounced":0,"complained":0,"unsubscribed":0}},
					{"variation_name":"B","template_id":"tpl_b","sender_id":"s2","metrics":null}
				]
			},
			"utm_parameters":{"source":"notifuse","medium":"email","campaign":"nov","term":"","content":""},
			"tracking_mode":"inherit",
			"data_feed":{
				"global_feed":{"enabled":true,"url":"https://feed.example.com/x","header_names":["Authorization"]},
				"recipient_feed":{"enabled":false,"url":"","header_names":[]},
				"global_feed_fetched_at":"2026-11-18T10:30:00Z"
			},
			"metadata":{"owner":"marketing"},
			"winning_template":null,"test_sent_at":"2026-11-18T11:00:00Z","winner_sent_at":null,
			"test_phase_recipient_count":240,"winner_phase_recipient_count":0,"test_paused_seconds":0,"enqueued_count":240,
			"created_at":"2026-11-18T10:00:00Z","updated_at":"2026-11-18T10:00:00Z","started_at":"2026-11-18T11:00:00Z",
			"completed_at":null,"cancelled_at":null,"paused_at":null,"pause_reason":null
		}
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"feed-secret", "fetched-payload", "global_feed_data", "Joined template", "channel_type"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

func TestGetBroadcast_CapsFreeText(t *testing.T) {
	f := newFixture(t)
	f.reachable(broadcastsReadPermission())
	broadcast := fullBroadcast()
	broadcast.PauseReason = strPtr(strings.Repeat("r", 2001))
	broadcast.DataFeed.GlobalFeed.URL = "https://feed.example.com/" + strings.Repeat("u", 2000)
	broadcast.Metadata = domain.MapOfAny{"blob": strings.Repeat("m", 2001)}
	f.broadcasts.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1", domain.GetBroadcastOptions{}).Return(broadcast, nil)

	var out struct {
		Broadcast struct {
			PauseReason     string          `json:"pause_reason"`
			Metadata        json.RawMessage `json:"metadata"`
			Truncated       bool            `json:"truncated"`
			TruncatedFields []string        `json:"truncated_fields"`
		} `json:"broadcast"`
	}
	f.callTool("get_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}).decode(t, &out)
	assert.Equal(t, strings.Repeat("r", 2000), out.Broadcast.PauseReason)
	assert.Equal(t, "null", string(out.Broadcast.Metadata))
	assert.True(t, out.Broadcast.Truncated)
	assert.ElementsMatch(t, []string{"pause_reason", "data_feed.global_feed.url", "metadata"}, out.Broadcast.TruncatedFields)
}

func TestGetBroadcast_Errors(t *testing.T) {
	t.Run("an unknown broadcast", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(broadcastsReadPermission())
		f.broadcasts.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "nope", domain.GetBroadcastOptions{}).Return(nil, &domain.ErrBroadcastNotFound{ID: "nope"})
		toolErr := f.callTool("get_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "nope"}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodeNotFound, Message: "Broadcast not found with ID: nope"}, toolErr)
	})

	t.Run("a key without broadcasts:read", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(domain.UserPermissions{domain.PermissionResourceBroadcasts: {Write: true}})
		toolErr := f.callTool("get_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the broadcasts read permission", Resource: "broadcasts", Permission: "read"}, toolErr)
	})

	t.Run("an empty broadcast_id", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(broadcastsReadPermission())
		assert.Equal(t, CodeValidation, f.callTool("get_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": ""}).errorOf(t).Code)
	})

	t.Run("a failed read", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(broadcastsReadPermission())
		f.broadcasts.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1", gomock.Any()).Return(nil, errors.New("pq: db-host-7"))
		assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"},
			f.callTool("get_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}).errorOf(t))
	})
}

func TestGetBroadcastReport_ReadsTheBroadcastThenItsStats(t *testing.T) {
	f := newFixture(t)
	f.reachable(reportPermissions())
	broadcast := fullBroadcast()
	gomock.InOrder(
		f.broadcasts.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1", domain.GetBroadcastOptions{}).Return(broadcast, nil),
		f.messages.EXPECT().GetBroadcastStats(gomock.Any(), testWorkspaceID, "bc1").Return(&domain.BroadcastDeliveryStats{
			Stats: &domain.MessageHistoryStatusSum{TotalSent: 1200, TotalDelivered: 1180, TotalBounced: 12, TotalComplained: 1, TotalFailed: 8,
				TotalOpened: 410, TotalClicked: 95, TotalUnsubscribed: 4, TotalOpensTracked: 1150, TotalClicksTracked: 1150},
			Queue: &domain.EmailQueueSourceCounts{FailedTerminal: 3},
		}, nil),
	)
	f.messages.EXPECT().GetBroadcastVariationStats(gomock.Any(), testWorkspaceID, "bc1", "tpl_a").
		Return(&domain.MessageHistoryStatusSum{TotalSent: 600, TotalOpened: 220}, nil)
	f.messages.EXPECT().GetBroadcastVariationStats(gomock.Any(), testWorkspaceID, "bc1", "tpl_b").
		Return(&domain.MessageHistoryStatusSum{TotalSent: 600, TotalOpened: 190}, nil)
	f.messages.EXPECT().GetBroadcastLinkStats(gomock.Any(), testWorkspaceID, "bc1", "").Return([]domain.LinkClickStats{
		{URL: "https://acme.com/b", TotalClicks: 10, UniqueClicks: 8},
		{URL: "https://acme.com/sale", TotalClicks: 130, UniqueClicks: 88},
		{URL: "https://acme.com/a", TotalClicks: 10, UniqueClicks: 9},
	}, nil)

	result := f.callTool("get_broadcast_report", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"broadcast":{"id":"bc1","name":"November newsletter","status":"test_completed","winning_template":null},
		"totals":{"sent":1200,"delivered":1180,"bounced":12,"complained":1,"failed":8,"opened":410,"clicked":95,"unsubscribed":4,"opens_tracked":1150,"clicks_tracked":1150},
		"queue":{"pending":0,"processing":0,"paused":0,"failed_retrying":0,"failed_terminal":3},
		"variations":[
			{"template_id":"tpl_a","variation_name":"A","sent":600,"delivered":0,"bounced":0,"complained":0,"failed":0,"opened":220,"clicked":0,"unsubscribed":0,"opens_tracked":0,"clicks_tracked":0},
			{"template_id":"tpl_b","variation_name":"B","sent":600,"delivered":0,"bounced":0,"complained":0,"failed":0,"opened":190,"clicked":0,"unsubscribed":0,"opens_tracked":0,"clicks_tracked":0}
		],
		"links":[
			{"url":"https://acme.com/sale","total_clicks":130,"unique_clicks":88},
			{"url":"https://acme.com/a","total_clicks":10,"unique_clicks":9},
			{"url":"https://acme.com/b","total_clicks":10,"unique_clicks":8}
		],
		"links_truncated":false
	}`, string(result.StructuredContent))
}

// The stats services check only message_history:read and answer zeros for a
// broadcast that does not exist: the broadcast read refuses first.
func TestGetBroadcastReport_AnUnknownBroadcastNeverReachesTheStats(t *testing.T) {
	f := newFixture(t)
	f.reachable(reportPermissions())
	f.broadcasts.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "nope", domain.GetBroadcastOptions{}).Return(nil, &domain.ErrBroadcastNotFound{ID: "nope"})

	toolErr := f.callTool("get_broadcast_report", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "nope"}).errorOf(t)
	assert.Equal(t, CodeNotFound, toolErr.Code)
}

func TestGetBroadcastReport_NeedsBothPairs(t *testing.T) {
	for name, permissions := range map[string]domain.UserPermissions{
		"broadcasts":      {domain.PermissionResourceMessageHistory: {Read: true}},
		"message_history": {domain.PermissionResourceBroadcasts: {Read: true}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(permissions)
			toolErr := f.callTool("get_broadcast_report", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}).errorOf(t)
			assert.Equal(t, CodePermissionDenied, toolErr.Code)
			assert.Equal(t, name, toolErr.Resource)
		})
	}
}

func TestGetBroadcastReport_OneVariationsLinksAndTheCap(t *testing.T) {
	f := newFixture(t)
	f.reachable(reportPermissions())
	broadcast := fullBroadcast()
	f.broadcasts.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1", gomock.Any()).Return(broadcast, nil)
	f.messages.EXPECT().GetBroadcastStats(gomock.Any(), testWorkspaceID, "bc1").Return(&domain.BroadcastDeliveryStats{Stats: &domain.MessageHistoryStatusSum{}}, nil)
	f.messages.EXPECT().GetBroadcastVariationStats(gomock.Any(), testWorkspaceID, "bc1", gomock.Any()).Return(&domain.MessageHistoryStatusSum{}, nil).Times(2)
	links := []domain.LinkClickStats{}
	for i := range 51 {
		links = append(links, domain.LinkClickStats{URL: fmt.Sprintf("https://acme.com/%02d", i), TotalClicks: int64(i)})
	}
	links[0].URL = "https://acme.com/" + strings.Repeat("p", 2000)
	links[0].TotalClicks = 1000
	f.messages.EXPECT().GetBroadcastLinkStats(gomock.Any(), testWorkspaceID, "bc1", "tpl_b").Return(links, nil)

	var out struct {
		Queue *json.RawMessage `json:"queue"`
		Links []struct {
			URL         string `json:"url"`
			TotalClicks int64  `json:"total_clicks"`
		} `json:"links"`
		LinksTruncated  bool     `json:"links_truncated"`
		TruncatedFields []string `json:"truncated_fields"`
	}
	f.callTool("get_broadcast_report", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "template_id": "tpl_b"}).decode(t, &out)
	assert.Nil(t, out.Queue, "absent when the queue could not be read")
	require.Len(t, out.Links, 50)
	assert.True(t, out.LinksTruncated)
	assert.Equal(t, int64(1000), out.Links[0].TotalClicks)
	assert.Equal(t, int64(2), out.Links[49].TotalClicks, "the least clicked link is the one cut")
	assert.Len(t, []rune(out.Links[0].URL), 2000)
	assert.Equal(t, []string{"links[0].url"}, out.TruncatedFields)
}

func TestGetBroadcastReport_ATemplateTheBroadcastDoesNotSend(t *testing.T) {
	f := newFixture(t)
	f.reachable(reportPermissions())
	f.broadcasts.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1", gomock.Any()).Return(fullBroadcast(), nil)
	toolErr := f.callTool("get_broadcast_report", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "template_id": "tpl_z"}).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code)
}

func TestGetBroadcastReport_NoVariationsWithoutAnABTest(t *testing.T) {
	f := newFixture(t)
	f.reachable(reportPermissions())
	broadcast := fullBroadcast()
	broadcast.TestSettings.Enabled = false
	f.broadcasts.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1", gomock.Any()).Return(broadcast, nil)
	f.messages.EXPECT().GetBroadcastStats(gomock.Any(), testWorkspaceID, "bc1").Return(&domain.BroadcastDeliveryStats{Stats: &domain.MessageHistoryStatusSum{TotalSent: 3}}, nil)
	f.messages.EXPECT().GetBroadcastLinkStats(gomock.Any(), testWorkspaceID, "bc1", "").Return(nil, nil)

	var out struct {
		Totals     map[string]int    `json:"totals"`
		Variations []json.RawMessage `json:"variations"`
		Links      []json.RawMessage `json:"links"`
	}
	f.callTool("get_broadcast_report", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}).decode(t, &out)
	assert.Equal(t, 3, out.Totals["sent"])
	assert.NotNil(t, out.Variations)
	assert.Empty(t, out.Variations)
	assert.NotNil(t, out.Links)
	assert.Empty(t, out.Links)
}

func TestGetBroadcastReport_AFailedStatsReadIsInternal(t *testing.T) {
	f := newFixture(t)
	f.reachable(reportPermissions())
	f.broadcasts.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1", gomock.Any()).Return(fullBroadcast(), nil)
	f.messages.EXPECT().GetBroadcastStats(gomock.Any(), testWorkspaceID, "bc1").Return(nil, errors.New("failed to get broadcast stats: pq"))
	assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"},
		f.callTool("get_broadcast_report", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}).errorOf(t))
}

// A broadcast's name, its variation names and its UTM parameters are free
// text, capped like the rest.
func TestBroadcastTools_CapTheNamesAndUTMParameters(t *testing.T) {
	long, capped := strings.Repeat("n", FreeTextLimit+1), strings.Repeat("n", FreeTextLimit)
	longBroadcast := func() *domain.Broadcast {
		broadcast := fullBroadcast()
		broadcast.Name = long
		broadcast.TestSettings.Variations[1].VariationName = long
		broadcast.UTMParameters = &domain.UTMParameters{Source: long, Medium: long, Campaign: long, Term: long, Content: long}
		return broadcast
	}

	t.Run("list_broadcasts", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(broadcastsReadPermission())
		f.broadcasts.EXPECT().ListBroadcasts(gomock.Any(), gomock.Any()).
			Return(&domain.BroadcastListResponse{Broadcasts: []*domain.Broadcast{longBroadcast()}, TotalCount: 1}, nil)
		var out struct {
			Broadcasts []struct {
				Name            string   `json:"name"`
				TruncatedFields []string `json:"truncated_fields"`
			} `json:"broadcasts"`
		}
		f.callTool("list_broadcasts", map[string]any{"workspace_id": testWorkspaceID}).decode(t, &out)
		require.Len(t, out.Broadcasts, 1)
		assert.Equal(t, capped, out.Broadcasts[0].Name)
		assert.Equal(t, []string{"name"}, out.Broadcasts[0].TruncatedFields)
	})

	t.Run("get_broadcast", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(broadcastsReadPermission())
		f.broadcasts.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1", domain.GetBroadcastOptions{}).Return(longBroadcast(), nil)
		var out struct {
			Broadcast struct {
				Name         string `json:"name"`
				TestSettings struct {
					Variations []struct {
						VariationName string `json:"variation_name"`
					} `json:"variations"`
				} `json:"test_settings"`
				UTMParameters   map[string]string `json:"utm_parameters"`
				TruncatedFields []string          `json:"truncated_fields"`
			} `json:"broadcast"`
		}
		f.callTool("get_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}).decode(t, &out)
		assert.Equal(t, capped, out.Broadcast.Name)
		require.Len(t, out.Broadcast.TestSettings.Variations, 2)
		assert.Equal(t, "A", out.Broadcast.TestSettings.Variations[0].VariationName)
		assert.Equal(t, capped, out.Broadcast.TestSettings.Variations[1].VariationName)
		for _, key := range []string{"source", "medium", "campaign", "term", "content"} {
			assert.Equal(t, capped, out.Broadcast.UTMParameters[key], key)
		}
		assert.ElementsMatch(t, []string{
			"name", "test_settings.variations[1].variation_name",
			"utm_parameters.source", "utm_parameters.medium", "utm_parameters.campaign", "utm_parameters.term", "utm_parameters.content",
		}, out.Broadcast.TruncatedFields)
	})

	t.Run("get_broadcast_report", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(reportPermissions())
		f.broadcasts.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1", gomock.Any()).Return(longBroadcast(), nil)
		f.messages.EXPECT().GetBroadcastStats(gomock.Any(), testWorkspaceID, "bc1").Return(nil, nil)
		f.messages.EXPECT().GetBroadcastVariationStats(gomock.Any(), testWorkspaceID, "bc1", gomock.Any()).Return(nil, nil).Times(2)
		f.messages.EXPECT().GetBroadcastLinkStats(gomock.Any(), testWorkspaceID, "bc1", "").Return(nil, nil)
		var out struct {
			Broadcast struct {
				Name string `json:"name"`
			} `json:"broadcast"`
			Variations []struct {
				VariationName string `json:"variation_name"`
			} `json:"variations"`
			TruncatedFields []string `json:"truncated_fields"`
		}
		f.callTool("get_broadcast_report", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}).decode(t, &out)
		assert.Equal(t, capped, out.Broadcast.Name)
		require.Len(t, out.Variations, 2)
		assert.Equal(t, capped, out.Variations[1].VariationName)
		assert.Equal(t, []string{"broadcast.name", "variations[1].variation_name"}, out.TruncatedFields)
	})
}

func broadcastsWritePermission() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceBroadcasts: {Write: true}}
}

// sendingWorkspace has a marketing and a transactional email integration.
func sendingWorkspace() *domain.Workspace {
	workspace := testWorkspace()
	workspace.Settings.DefaultLanguage = "en"
	workspace.Settings.MarketingEmailProviderID = "int-m"
	workspace.Settings.TransactionalEmailProviderID = "int-t"
	workspace.Integrations = []domain.Integration{
		{ID: "int-m", Name: "Marketing", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				{ID: "s-news", Email: "news@acme.test", IsDefault: true},
				{ID: "s-support", Email: "support@acme.test"},
			},
		}},
		{ID: "int-t", Name: "Transactional", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{
			Kind:    domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{{ID: "t-default", Email: "no-reply@acme.test", IsDefault: true}, {ID: "t-billing", Email: "billing@acme.test"}},
		}},
	}
	return workspace
}

// storedDraft is a draft broadcast with a schedule, segments, an A/B test
// whose variations chose senders, and the unsubscribed contacts kept.
func storedDraft() *domain.Broadcast {
	return &domain.Broadcast{
		ID: "bc1", WorkspaceID: testWorkspaceID, Name: "November newsletter", ChannelType: "email",
		Status:   domain.BroadcastStatusDraft,
		Audience: domain.AudienceSettings{List: "newsletter", Segments: []string{"vip"}, ExcludeUnsubscribed: false},
		Schedule: domain.ScheduleSettings{IsScheduled: true, ScheduledDate: "2026-12-01", ScheduledTime: "09:00", Timezone: "Europe/Paris"},
		TestSettings: domain.BroadcastTestSettings{
			Enabled: true, SamplePercentage: 20,
			Variations: []domain.BroadcastVariation{
				{VariationName: "A", TemplateID: "tpl_a", SenderID: "s-support"},
				{VariationName: "B", TemplateID: "tpl_b"},
			},
		},
		UTMParameters: &domain.UTMParameters{Source: "notifuse"},
		Metadata:      domain.MapOfAny{"owner": "marketing"},
		CreatedAt:     broadcastsEpoch, UpdatedAt: broadcastsEpoch,
	}
}

func TestBroadcastWriteTools_Definitions(t *testing.T) {
	write := []Pair{{Resource: "broadcasts", Verb: "write"}}
	id := []string{"workspace_id", "broadcast_id"}
	assertWriteTool(t, catalogueTool(t, "create_broadcast"), "create_broadcast", ClassWrite, writeHints{destructive: false, openWorld: false, idempotent: false},
		write, "broadcasts.create", []string{"workspace_id", "name", "list_id"})
	assertWriteTool(t, catalogueTool(t, "update_broadcast"), "update_broadcast", ClassWriteOpenWorld, writeHints{destructive: false, openWorld: true, idempotent: true},
		write, "broadcasts.update", id)
	assertWriteTool(t, catalogueTool(t, "send_broadcast_to_contact"), "send_broadcast_to_contact", ClassSend, writeHints{destructive: true, openWorld: true, idempotent: false},
		[]Pair{{Resource: "broadcasts", Verb: "write"}, {Resource: "templates", Verb: "read"}}, "broadcasts.sendToIndividual", []string{"workspace_id", "broadcast_id", "recipient_email"})
	assertWriteTool(t, catalogueTool(t, "schedule_broadcast"), "schedule_broadcast", ClassSend, writeHints{destructive: true, openWorld: true, idempotent: true},
		write, "broadcasts.schedule", []string{"workspace_id", "broadcast_id", "when"})
	assertWriteTool(t, catalogueTool(t, "select_broadcast_winner"), "select_broadcast_winner", ClassSend, writeHints{destructive: true, openWorld: true, idempotent: true},
		write, "broadcasts.selectWinner", []string{"workspace_id", "broadcast_id", "template_id"})
	assertWriteTool(t, catalogueTool(t, "pause_broadcast"), "pause_broadcast", ClassWrite, writeHints{destructive: false, openWorld: false, idempotent: true},
		write, "broadcasts.pause", id)
	assertWriteTool(t, catalogueTool(t, "resume_broadcast"), "resume_broadcast", ClassSend, writeHints{destructive: true, openWorld: true, idempotent: true},
		write, "broadcasts.resume", id)
	assertWriteTool(t, catalogueTool(t, "cancel_broadcast"), "cancel_broadcast", ClassDestructive, writeHints{destructive: true, openWorld: false, idempotent: true},
		write, "broadcasts.cancel", id)
	assertWriteTool(t, catalogueTool(t, "delete_broadcast"), "delete_broadcast", ClassDestructive, writeHints{destructive: true, openWorld: false, idempotent: true},
		write, "broadcasts.delete", id)

	// The send tools and the open-world update count in mcp_send.
	for name, counts := range map[string]bool{
		"create_broadcast": false, "update_broadcast": true, "send_broadcast_to_contact": true, "schedule_broadcast": true,
		"select_broadcast_winner": true, "pause_broadcast": false, "resume_broadcast": true, "cancel_broadcast": false, "delete_broadcast": false,
	} {
		assert.Equal(t, counts, catalogueTool(t, name).countsAsSend(), name)
	}
	// A variation's sender and the data feed are not offered.
	for _, name := range []string{"create_broadcast", "update_broadcast"} {
		properties := catalogueTool(t, name).InputSchema["properties"].(map[string]any)
		assert.NotContains(t, properties, "data_feed", name)
		assert.NotContains(t, properties, "schedule", name)
		variation := properties["ab_test"].(map[string]any)["properties"].(map[string]any)["variations"].(map[string]any)["items"].(map[string]any)
		assert.Equal(t, []string{"template_id", "variation_name"}, sortedKeys(variation["properties"].(map[string]any)), name)
	}
}

// broadcastsWriteFixture is a fixture whose key may write broadcasts, in a
// workspace with email integrations, with its audit rows captured.
func broadcastsWriteFixture(t *testing.T, permissions domain.UserPermissions) (*fixture, *auditLog) {
	f := newFixture(t)
	f.reachableIn(permissions, sendingWorkspace())
	return f, f.captureAudit()
}

// An omitted exclude_unsubscribed must not reach the service as false: that
// mails the list's unsubscribed contacts.
func TestCreateBroadcast_KeepsUnsubscribedContactsOutUnlessToldOtherwise(t *testing.T) {
	for name, tc := range map[string]struct {
		args    map[string]any
		exclude bool
	}{
		"omitted":        {args: map[string]any{}, exclude: true},
		"given as true":  {args: map[string]any{"exclude_unsubscribed": true}, exclude: true},
		"given as false": {args: map[string]any{"exclude_unsubscribed": false}, exclude: false},
	} {
		t.Run(name, func(t *testing.T) {
			f, _ := broadcastsWriteFixture(t, broadcastsWritePermission())
			var validated *domain.Broadcast
			f.broadcasts.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *domain.CreateBroadcastRequest) (*domain.Broadcast, error) {
				broadcast, err := req.Validate()
				require.NoError(t, err)
				validated = broadcast
				broadcast.ID = "bc-new"
				return broadcast, nil
			})
			args := map[string]any{"workspace_id": testWorkspaceID, "name": "Launch", "list_id": "newsletter", "template_id": "tpl_a"}
			for key, value := range tc.args {
				args[key] = value
			}
			require.False(t, f.callTool("create_broadcast", args).IsError)
			require.NotNil(t, validated)
			assert.Equal(t, tc.exclude, validated.Audience.ExcludeUnsubscribed)
		})
	}
}

func TestCreateBroadcast_BuildsTheRequest(t *testing.T) {
	f, audit := broadcastsWriteFixture(t, broadcastsWritePermission())
	var sent *domain.CreateBroadcastRequest
	f.broadcasts.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *domain.CreateBroadcastRequest) (*domain.Broadcast, error) {
		sent = req
		broadcast, err := req.Validate()
		require.NoError(t, err)
		broadcast.ID = "bc-new"
		broadcast.CreatedAt, broadcast.UpdatedAt = broadcastsEpoch, broadcastsEpoch
		return broadcast, nil
	})

	result := f.callTool("create_broadcast", map[string]any{
		"workspace_id": testWorkspaceID, "name": "Launch", "list_id": "newsletter", "segment_ids": []string{"vip"},
		"ab_test": map[string]any{
			"sample_percentage": 10, "auto_send_winner": true, "auto_send_winner_metric": "click_rate", "test_duration_hours": 6,
			"variations": []any{map[string]any{"template_id": "tpl_a"}, map[string]any{"template_id": "tpl_b", "variation_name": "Short"}},
		},
		"utm_parameters": map[string]any{"campaign": "launch"},
		"tracking_mode":  "inherit",
		"metadata":       map[string]any{"owner": "growth"},
	})
	require.False(t, result.IsError, string(result.StructuredContent))
	require.NotNil(t, sent)
	assert.Equal(t, testWorkspaceID, sent.WorkspaceID)
	assert.Equal(t, domain.AudienceSettings{List: "newsletter", Segments: []string{"vip"}, ExcludeUnsubscribed: true}, sent.Audience)
	assert.Equal(t, domain.BroadcastTestSettings{
		Enabled: true, SamplePercentage: 10, AutoSendWinner: true, AutoSendWinnerMetric: domain.TestWinnerMetricClickRate, TestDurationHours: 6,
		Variations: []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tpl_a"}, {VariationName: "Short", TemplateID: "tpl_b"}},
	}, sent.TestSettings)
	assert.Equal(t, &domain.UTMParameters{Campaign: "launch"}, sent.UTMParameters)
	assert.Equal(t, domain.MapOfAny{"owner": "growth"}, sent.Metadata)
	assert.Nil(t, sent.DataFeed)

	var out struct {
		Broadcast broadcastDetail `json:"broadcast"`
	}
	result.decode(t, &out)
	assert.Equal(t, "bc-new", out.Broadcast.ID)
	assert.Equal(t, "draft", out.Broadcast.Status)

	row := audit.last(t)
	assert.Equal(t, "broadcasts.create", row.Action)
	assert.Equal(t, domain.AuditCategoryBroadcasts, row.Category)
	assert.Equal(t, domain.AuditTargetBroadcast, row.TargetType)
	assert.Equal(t, "bc-new", row.TargetID)
	assert.Equal(t, "Launch", row.TargetName)
	assert.Equal(t, http.StatusOK, row.StatusCode)
}

func TestCreateBroadcast_OneTemplateSource(t *testing.T) {
	f, audit := broadcastsWriteFixture(t, broadcastsWritePermission())
	var created *domain.CreateBroadcastRequest
	f.broadcasts.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *domain.CreateBroadcastRequest) (*domain.Broadcast, error) {
		created = req
		broadcast, err := req.Validate()
		require.NoError(t, err)
		return broadcast, nil
	})
	require.False(t, f.callTool("create_broadcast", map[string]any{"workspace_id": testWorkspaceID, "name": "One", "list_id": "news", "template_id": "tpl_a"}).IsError)
	assert.Equal(t, domain.BroadcastTestSettings{Variations: []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tpl_a"}}}, created.TestSettings)

	abTest := map[string]any{"sample_percentage": 10, "variations": []any{map[string]any{"template_id": "a"}, map[string]any{"template_id": "b"}}}
	for name, args := range map[string]map[string]any{
		"no template":             {},
		"both":                    {"template_id": "tpl_a", "ab_test": abTest},
		"one variation":           {"ab_test": map[string]any{"sample_percentage": 10, "variations": []any{map[string]any{"template_id": "a"}}}},
		"sample above 100":        {"ab_test": map[string]any{"sample_percentage": 120, "variations": abTest["variations"]}},
		"winner without a metric": {"ab_test": map[string]any{"sample_percentage": 10, "auto_send_winner": true, "test_duration_hours": 2, "variations": abTest["variations"]}},
		"metadata above 32 KiB":   {"template_id": "tpl_a", "metadata": map[string]any{"blob": strings.Repeat("x", 33<<10)}},
		"a data feed":             {"template_id": "tpl_a", "data_feed": map[string]any{}},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"], args["name"], args["list_id"] = testWorkspaceID, "Refused", "news"
			assert.Equal(t, CodeValidation, f.callTool("create_broadcast", args).errorOf(t).Code)
			assert.Equal(t, http.StatusBadRequest, audit.last(t).StatusCode)
		})
	}
}

// update_broadcast sends only the keys it was given: the stored schedule,
// variations, segments and exclude_unsubscribed survive an update that
// names one field.
func TestUpdateBroadcast_KeepsWhatItIsNotGiven(t *testing.T) {
	f, audit := broadcastsWriteFixture(t, broadcastsWritePermission())
	f.bcastRows.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1").Return(storedDraft(), nil)
	var applied *domain.Broadcast
	f.broadcasts.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *domain.UpdateBroadcastRequest) (*domain.Broadcast, error) {
		// What the service does under its lock: apply the request to the row.
		broadcast, err := req.Validate(storedDraft())
		require.NoError(t, err)
		applied = broadcast
		return broadcast, nil
	})

	result := f.callTool("update_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "utm_parameters": map[string]any{"campaign": "dec"}})
	require.False(t, result.IsError, string(result.StructuredContent))
	require.NotNil(t, applied)
	stored := storedDraft()
	assert.Equal(t, stored.Name, applied.Name, "the name is required by the request, and kept")
	assert.Equal(t, stored.Schedule, applied.Schedule)
	assert.Equal(t, stored.TestSettings, applied.TestSettings)
	assert.Equal(t, stored.Audience, applied.Audience)
	assert.Equal(t, stored.Metadata, applied.Metadata)
	assert.Equal(t, &domain.UTMParameters{Campaign: "dec"}, applied.UTMParameters)

	row := audit.last(t)
	assert.Equal(t, "broadcasts.update", row.Action)
	assert.Equal(t, "bc1", row.TargetID)
	assert.Equal(t, "November newsletter", row.TargetName)
	assert.Equal(t, http.StatusOK, row.StatusCode)
}

func TestUpdateBroadcast_PatchesTheAudienceAndTemplatesKeyByKey(t *testing.T) {
	f, _ := broadcastsWriteFixture(t, broadcastsWritePermission())
	f.bcastRows.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1").Return(storedDraft(), nil)
	var applied *domain.Broadcast
	f.broadcasts.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *domain.UpdateBroadcastRequest) (*domain.Broadcast, error) {
		broadcast, err := req.Validate(storedDraft())
		require.NoError(t, err)
		applied = broadcast
		return broadcast, nil
	})

	require.False(t, f.callTool("update_broadcast", map[string]any{
		"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "name": "December", "list_id": "customers", "template_id": "tpl_a",
	}).IsError)
	require.NotNil(t, applied)
	assert.Equal(t, "December", applied.Name)
	assert.Equal(t, domain.AudienceSettings{List: "customers", Segments: []string{"vip"}, ExcludeUnsubscribed: false}, applied.Audience,
		"the segments and exclude_unsubscribed left out stay")
	assert.Equal(t, domain.BroadcastTestSettings{Variations: []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tpl_a", SenderID: "s-support"}}}, applied.TestSettings,
		"the sender the stored variation chose for the template is kept")
	assert.Equal(t, storedDraft().Schedule, applied.Schedule)
}

func TestUpdateBroadcast_RefusalsAreTyped(t *testing.T) {
	sending := storedDraft()
	sending.Status = domain.BroadcastStatusProcessing
	started := storedDraft()
	started.Status = domain.BroadcastStatusPaused
	startedAt := broadcastsEpoch
	started.StartedAt = &startedAt

	cases := map[string]struct {
		stored *domain.Broadcast
		err    error
		args   map[string]any
		want   ToolError
	}{
		"a broadcast that is sending": {stored: sending, args: map[string]any{"name": "X"},
			want: ToolError{Code: CodeInvalidState, Message: "cannot update broadcast bc1: its status is processing", Status: "processing"}},
		"exclude_unsubscribed once sending started": {stored: started, args: map[string]any{"exclude_unsubscribed": true},
			want: ToolError{Code: CodeValidation, Message: "exclude_unsubscribed can only be changed on a draft or scheduled broadcast"}},
		"a broadcast that does not exist": {err: &domain.ErrBroadcastNotFound{ID: "bc1"}, args: map[string]any{"name": "X"},
			want: ToolError{Code: CodeNotFound, Message: "Broadcast not found with ID: bc1"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f, audit := broadcastsWriteFixture(t, broadcastsWritePermission())
			f.bcastRows.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1").Return(tc.stored, tc.err)
			args := map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}
			for key, value := range tc.args {
				args[key] = value
			}
			assert.Equal(t, tc.want, f.callTool("update_broadcast", args).errorOf(t))
			assert.Equal(t, tc.want.auditStatus(), audit.last(t).StatusCode)
		})
	}

	t.Run("arguments refused before any read", func(t *testing.T) {
		f, _ := broadcastsWriteFixture(t, broadcastsWritePermission())
		for name, args := range map[string]map[string]any{
			"nothing to change": {},
			"an empty name":     {"name": ""},
			"both template sources": {"template_id": "a", "ab_test": map[string]any{"sample_percentage": 10, "variations": []any{
				map[string]any{"template_id": "a"}, map[string]any{"template_id": "b"},
			}}},
			"a schedule": {"schedule": map[string]any{}},
		} {
			args["workspace_id"], args["broadcast_id"] = testWorkspaceID, "bc1"
			assert.Equal(t, CodeValidation, f.callTool("update_broadcast", args).errorOf(t).Code, name)
		}
	})
}

func TestSendBroadcastToContact_SendsTheFirstTemplateByDefault(t *testing.T) {
	f, audit := broadcastsWriteFixture(t, domain.UserPermissions{
		domain.PermissionResourceBroadcasts: {Write: true}, domain.PermissionResourceTemplates: {Read: true},
	})
	f.bcastRows.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1").Return(storedDraft(), nil).Times(2)
	f.broadcasts.EXPECT().SendToIndividual(gomock.Any(), &domain.SendToIndividualRequest{
		WorkspaceID: testWorkspaceID, BroadcastID: "bc1", RecipientEmail: "jane@example.com", TemplateID: "tpl_a",
	}).Return(nil)
	f.broadcasts.EXPECT().SendToIndividual(gomock.Any(), &domain.SendToIndividualRequest{
		WorkspaceID: testWorkspaceID, BroadcastID: "bc1", RecipientEmail: "jane@example.com", TemplateID: "tpl_b",
	}).Return(nil)

	result := f.callTool("send_broadcast_to_contact", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "recipient_email": " Jane@Example.com "})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"sent":true,"broadcast_id":"bc1","template_id":"tpl_a","recipient_email":"jane@example.com"}`,
		string(result.StructuredContent))
	row := audit.last(t)
	assert.Equal(t, "broadcasts.sendToIndividual", row.Action)
	assert.Equal(t, domain.AuditTargetBroadcast, row.TargetType)
	assert.Equal(t, "bc1", row.TargetID)
	assert.Equal(t, map[string]any{"channel": "mcp", "mcp_tool": "send_broadcast_to_contact", "recipient_email": "jane@example.com", "template_id": "tpl_a"}, row.Metadata)

	require.False(t, f.callTool("send_broadcast_to_contact", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "recipient_email": "jane@example.com", "template_id": "tpl_b"}).IsError)
	assert.Equal(t, "tpl_b", audit.last(t).Metadata["template_id"])
}

func TestSendBroadcastToContact_Refusals(t *testing.T) {
	both := domain.UserPermissions{domain.PermissionResourceBroadcasts: {Write: true}, domain.PermissionResourceTemplates: {Read: true}}
	t.Run("an address that is not one", func(t *testing.T) {
		f, audit := broadcastsWriteFixture(t, both)
		toolErr := f.callTool("send_broadcast_to_contact", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "recipient_email": "jane"}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodeValidation, Message: "recipient_email must be a valid email address"}, toolErr)
		assert.Equal(t, http.StatusBadRequest, audit.last(t).StatusCode)
	})
	t.Run("a key without templates:read", func(t *testing.T) {
		f, audit := broadcastsWriteFixture(t, broadcastsWritePermission())
		toolErr := f.callTool("send_broadcast_to_contact", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "recipient_email": "jane@example.com"}).errorOf(t)
		assert.Equal(t, CodePermissionDenied, toolErr.Code)
		assert.Equal(t, "templates", toolErr.Resource)
		assert.Equal(t, http.StatusForbidden, audit.last(t).StatusCode)
	})
	t.Run("no marketing integration", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(both)
		f.captureAudit()
		toolErr := f.callTool("send_broadcast_to_contact", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "recipient_email": "jane@example.com"}).errorOf(t)
		assert.Equal(t, CodeValidation, toolErr.Code)
		assert.Contains(t, toolErr.Message, "no marketing email integration")
	})
	t.Run("a broadcast without a template", func(t *testing.T) {
		f, _ := broadcastsWriteFixture(t, both)
		empty := storedDraft()
		empty.TestSettings = domain.BroadcastTestSettings{}
		f.bcastRows.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1").Return(empty, nil)
		assert.Equal(t, CodeValidation, f.callTool("send_broadcast_to_contact", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "recipient_email": "jane@example.com"}).errorOf(t).Code)
	})
	t.Run("a template the broadcast does not send", func(t *testing.T) {
		f, _ := broadcastsWriteFixture(t, both)
		f.bcastRows.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1").Return(storedDraft(), nil)
		f.broadcasts.EXPECT().SendToIndividual(gomock.Any(), gomock.Any()).Return(&domain.ErrNotFound{Entity: "broadcast_variation", ID: "tpl_x"})
		assert.Equal(t, CodeNotFound, f.callTool("send_broadcast_to_contact", map[string]any{
			"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "recipient_email": "jane@example.com", "template_id": "tpl_x",
		}).errorOf(t).Code)
	})
}

func TestScheduleBroadcast_MapsWhen(t *testing.T) {
	defer func(previous func() time.Time) { nowFunc = previous }(nowFunc)
	nowFunc = func() time.Time { return time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC) }

	for name, tc := range map[string]struct {
		when string
		want domain.ScheduleBroadcastRequest
		out  string
	}{
		"now": {when: "now", want: domain.ScheduleBroadcastRequest{WorkspaceID: testWorkspaceID, ID: "bc1", SendNow: true},
			out: `{"workspace":{"id":"ws1","name":"Acme"},"broadcast_id":"bc1","status":"processing","scheduled_at":null}`},
		"a time with an offset, to the minute": {when: "2026-12-01T09:30:45+01:00",
			want: domain.ScheduleBroadcastRequest{WorkspaceID: testWorkspaceID, ID: "bc1", ScheduledDate: "2026-12-01", ScheduledTime: "08:30", Timezone: "UTC"},
			out:  `{"workspace":{"id":"ws1","name":"Acme"},"broadcast_id":"bc1","status":"scheduled","scheduled_at":"2026-12-01T08:30:00Z"}`},
		"a time across midnight in UTC": {when: "2026-12-01T00:15:00+02:00",
			want: domain.ScheduleBroadcastRequest{WorkspaceID: testWorkspaceID, ID: "bc1", ScheduledDate: "2026-11-30", ScheduledTime: "22:15", Timezone: "UTC"},
			out:  `{"workspace":{"id":"ws1","name":"Acme"},"broadcast_id":"bc1","status":"scheduled","scheduled_at":"2026-11-30T22:15:00Z"}`},
	} {
		t.Run(name, func(t *testing.T) {
			f, audit := broadcastsWriteFixture(t, broadcastsWritePermission())
			f.broadcasts.EXPECT().ScheduleBroadcast(gomock.Any(), &tc.want).Return(nil)
			f.bcastRows.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1").Return(storedDraft(), nil)
			result := f.callTool("schedule_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "when": tc.when})
			require.False(t, result.IsError, string(result.StructuredContent))
			assert.JSONEq(t, tc.out, string(result.StructuredContent))

			row := audit.last(t)
			assert.Equal(t, "broadcasts.schedule", row.Action)
			assert.Equal(t, "bc1", row.TargetID)
			assert.Equal(t, "November newsletter", row.TargetName)
			assert.Equal(t, map[string]any{
				"channel": "mcp", "mcp_tool": "schedule_broadcast", "when": tc.when,
				"list_id": "newsletter", "segment_ids": []string{"vip"}, "template_ids": []string{"tpl_a", "tpl_b"},
			}, row.Metadata)
		})
	}
}

func TestScheduleBroadcast_Refusals(t *testing.T) {
	defer func(previous func() time.Time) { nowFunc = previous }(nowFunc)
	nowFunc = func() time.Time { return time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC) }

	t.Run("a time that is not one, or past", func(t *testing.T) {
		f, _ := broadcastsWriteFixture(t, broadcastsWritePermission())
		for _, when := range []string{"tomorrow", "2026-12-01 09:00", "2026-11-23T11:59:00Z", "2026-11-23T12:00:00Z"} {
			assert.Equal(t, CodeValidation, f.callTool("schedule_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "when": when}).errorOf(t).Code, when)
		}
	})
	// The seconds are dropped before the comparison: a time later in the
	// current minute lands on the minute's start, which is past.
	t.Run("a time later in the current minute", func(t *testing.T) {
		defer func(previous func() time.Time) { nowFunc = previous }(nowFunc)
		nowFunc = func() time.Time { return time.Date(2026, 11, 23, 12, 0, 20, 0, time.UTC) }
		f, _ := broadcastsWriteFixture(t, broadcastsWritePermission())
		// No ScheduleBroadcast expectation: a call would fail the mock.
		toolErr := f.callTool("schedule_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "when": "2026-11-23T12:00:45Z"}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodeValidation, Message: `when must be in a later minute than now, since the seconds are dropped: give a later time, or "now"`}, toolErr)
	})
	t.Run("no marketing integration", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(broadcastsWritePermission())
		f.captureAudit()
		assert.Equal(t, CodeValidation, f.callTool("schedule_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "when": "now"}).errorOf(t).Code)
	})
	// A replay meets a broadcast that is no longer a draft.
	t.Run("a replay", func(t *testing.T) {
		f, audit := broadcastsWriteFixture(t, broadcastsWritePermission())
		f.broadcasts.EXPECT().ScheduleBroadcast(gomock.Any(), gomock.Any()).Return(domain.NewInvalidStateError("broadcast", "bc1", "schedule", "scheduled"))
		toolErr := f.callTool("schedule_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "when": "now"}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodeInvalidState, Message: "cannot schedule broadcast bc1: its status is scheduled", Status: "scheduled"}, toolErr)
		assert.Equal(t, http.StatusConflict, audit.last(t).StatusCode)
	})
}

func TestSelectBroadcastWinner(t *testing.T) {
	f, audit := broadcastsWriteFixture(t, broadcastsWritePermission())
	// The second call is a replay with the same template: the service answers
	// success and changes nothing.
	f.broadcasts.EXPECT().SelectWinner(gomock.Any(), testWorkspaceID, "bc1", "tpl_b").Return(nil).Times(2)
	f.bcastRows.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1").Return(storedDraft(), nil).Times(2)
	for range 2 {
		result := f.callTool("select_broadcast_winner", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "template_id": "tpl_b"})
		assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"broadcast_id":"bc1","winning_template":"tpl_b","status":"winner_selected"}`, string(result.StructuredContent))
	}
	row := audit.last(t)
	assert.Equal(t, "broadcasts.selectWinner", row.Action)
	assert.Equal(t, "tpl_b", row.Metadata["template_id"])
	assert.Equal(t, "newsletter", row.Metadata["list_id"])

	f.broadcasts.EXPECT().SelectWinner(gomock.Any(), testWorkspaceID, "bc1", "tpl_a").Return(domain.NewInvalidStateError("broadcast", "bc1", "select_winner", "winner_selected"))
	toolErr := f.callTool("select_broadcast_winner", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "template_id": "tpl_a"}).errorOf(t)
	assert.Equal(t, CodeInvalidState, toolErr.Code)
	assert.Equal(t, "winner_selected", toolErr.Status)
}

// A pause or a cancel replayed on a broadcast already in that state answers
// success, read from the refusal the service makes under the row's lock.
func TestPauseAndCancelBroadcast_AlreadyInTheTargetState(t *testing.T) {
	for _, tc := range []struct {
		tool, action, target string
		call                 func(f *fixture) *gomock.Call
	}{
		{"pause_broadcast", "broadcasts.pause", "paused", func(f *fixture) *gomock.Call {
			return f.broadcasts.EXPECT().PauseBroadcast(gomock.Any(), &domain.PauseBroadcastRequest{WorkspaceID: testWorkspaceID, ID: "bc1"})
		}},
		{"cancel_broadcast", "broadcasts.cancel", "cancelled", func(f *fixture) *gomock.Call {
			return f.broadcasts.EXPECT().CancelBroadcast(gomock.Any(), &domain.CancelBroadcastRequest{WorkspaceID: testWorkspaceID, ID: "bc1"})
		}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			f, audit := broadcastsWriteFixture(t, broadcastsWritePermission())
			args := map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}

			tc.call(f).Return(nil)
			result := f.callTool(tc.tool, args)
			assert.JSONEq(t, fmt.Sprintf(`{"workspace":{"id":"ws1","name":"Acme"},"broadcast_id":"bc1","status":%q,"already":false}`, tc.target), string(result.StructuredContent))
			row := audit.last(t)
			assert.Equal(t, tc.action, row.Action)
			assert.Equal(t, "bc1", row.TargetID)
			assert.Equal(t, http.StatusOK, row.StatusCode)

			tc.call(f).Return(fmt.Errorf("x: %w", domain.NewInvalidStateError("broadcast", "bc1", "y", tc.target)))
			result = f.callTool(tc.tool, args)
			assert.JSONEq(t, fmt.Sprintf(`{"workspace":{"id":"ws1","name":"Acme"},"broadcast_id":"bc1","status":%q,"already":true}`, tc.target), string(result.StructuredContent))
			row = audit.last(t)
			assert.Equal(t, http.StatusOK, row.StatusCode)
			assert.Equal(t, true, row.Metadata["already"])

			tc.call(f).Return(domain.NewInvalidStateError("broadcast", "bc1", "y", "draft"))
			toolErr := f.callTool(tc.tool, args).errorOf(t)
			assert.Equal(t, CodeInvalidState, toolErr.Code)
			assert.Equal(t, "draft", toolErr.Status)
			assert.Equal(t, http.StatusConflict, audit.last(t).StatusCode)
		})
	}
}

func TestResumeBroadcast_ReportsWhereItWentBack(t *testing.T) {
	f, audit := broadcastsWriteFixture(t, broadcastsWritePermission())
	f.broadcasts.EXPECT().ResumeBroadcast(gomock.Any(), &domain.ResumeBroadcastRequest{WorkspaceID: testWorkspaceID, ID: "bc1"}).Return(nil)
	resumed := storedDraft()
	resumed.Status = domain.BroadcastStatusTesting
	f.bcastRows.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1").Return(resumed, nil)

	result := f.callTool("resume_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"})
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"broadcast_id":"bc1","status":"testing","already":false}`, string(result.StructuredContent))
	row := audit.last(t)
	assert.Equal(t, "broadcasts.resume", row.Action)
	assert.Equal(t, "newsletter", row.Metadata["list_id"])

	// A replay meets a broadcast that is no longer paused.
	f.broadcasts.EXPECT().ResumeBroadcast(gomock.Any(), gomock.Any()).Return(domain.NewInvalidStateError("broadcast", "bc1", "resume", "testing"))
	toolErr := f.callTool("resume_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodeInvalidState, Message: "cannot resume broadcast bc1: its status is testing", Status: "testing"}, toolErr)
}

func TestDeleteBroadcast_RefusesWhatCouldStillSend(t *testing.T) {
	for _, status := range []domain.BroadcastStatus{"processing", "paused", "testing", "test_completed", "winner_selected"} {
		t.Run(string(status), func(t *testing.T) {
			f, audit := broadcastsWriteFixture(t, broadcastsWritePermission())
			stored := storedDraft()
			stored.Status = status
			f.bcastRows.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1").Return(stored, nil)
			toolErr := f.callTool("delete_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}).errorOf(t)
			assert.Equal(t, CodeInvalidState, toolErr.Code)
			assert.Equal(t, string(status), toolErr.Status)
			assert.Contains(t, toolErr.Message, "cancel_broadcast")
			assert.Equal(t, http.StatusConflict, audit.last(t).StatusCode)
		})
	}

	t.Run("a queue that still holds emails", func(t *testing.T) {
		f, _ := broadcastsWriteFixture(t, broadcastsWritePermission())
		stored := storedDraft()
		stored.Status = domain.BroadcastStatusProcessed
		f.bcastRows.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1").Return(stored, nil)
		f.queue.EXPECT().GetSourceCounts(gomock.Any(), testWorkspaceID, domain.EmailQueueSourceBroadcast, "bc1").
			Return(&domain.EmailQueueSourceCounts{Pending: 3, FailedRetrying: 1, FailedTerminal: 9}, nil)
		toolErr := f.callTool("delete_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}).errorOf(t)
		assert.Equal(t, ToolError{
			Code:    CodeInvalidState,
			Message: "broadcast bc1 still has 4 emails in its queue: cancel it with cancel_broadcast, or wait until they are sent, before deleting it",
			Status:  "processed",
		}, toolErr)
	})
}

func TestDeleteBroadcast_DeletesThenAnswersNotFound(t *testing.T) {
	for _, status := range []domain.BroadcastStatus{"draft", "scheduled", "processed", "failed", "cancelled"} {
		t.Run(string(status), func(t *testing.T) {
			f, audit := broadcastsWriteFixture(t, broadcastsWritePermission())
			stored := storedDraft()
			stored.Status = status
			f.bcastRows.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1").Return(stored, nil)
			// Rows given up on stay in the queue until its retention sweep, and send nothing.
			f.queue.EXPECT().GetSourceCounts(gomock.Any(), testWorkspaceID, domain.EmailQueueSourceBroadcast, "bc1").
				Return(&domain.EmailQueueSourceCounts{FailedTerminal: 2}, nil)
			f.broadcasts.EXPECT().DeleteBroadcast(gomock.Any(), &domain.DeleteBroadcastRequest{WorkspaceID: testWorkspaceID, ID: "bc1"}).Return(nil)

			result := f.callTool("delete_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"})
			assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"deleted":true,"broadcast_id":"bc1"}`, string(result.StructuredContent))
			row := audit.last(t)
			assert.Equal(t, "broadcasts.delete", row.Action)
			assert.Equal(t, domain.AuditTargetBroadcast, row.TargetType)
			assert.Equal(t, "bc1", row.TargetID)
			assert.Equal(t, "November newsletter", row.TargetName)
			assert.Equal(t, http.StatusOK, row.StatusCode)

			f.bcastRows.EXPECT().GetBroadcast(gomock.Any(), testWorkspaceID, "bc1").Return(nil, &domain.ErrBroadcastNotFound{ID: "bc1"})
			assert.Equal(t, CodeNotFound, f.callTool("delete_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}).errorOf(t).Code)
			assert.Equal(t, http.StatusNotFound, audit.last(t).StatusCode)
		})
	}
}

func TestBroadcastWriteTools_ServiceErrorsAreMapped(t *testing.T) {
	f, _ := broadcastsWriteFixture(t, broadcastsWritePermission())
	f.broadcasts.EXPECT().PauseBroadcast(gomock.Any(), gomock.Any()).Return(&domain.ErrBroadcastNotFound{ID: "bc1"})
	assert.Equal(t, CodeNotFound, f.callTool("pause_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}).errorOf(t).Code)
	f.broadcasts.EXPECT().CancelBroadcast(gomock.Any(), gomock.Any()).Return(errors.New("pq: connection reset"))
	assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"}, f.callTool("cancel_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1"}).errorOf(t))
	f.broadcasts.EXPECT().ScheduleBroadcast(gomock.Any(), gomock.Any()).Return(domain.NewValidationError("sender s9 is not a sender of the marketing email integration"))
	assert.Equal(t, CodeValidation, f.callTool("schedule_broadcast", map[string]any{"workspace_id": testWorkspaceID, "broadcast_id": "bc1", "when": "now"}).errorOf(t).Code)
}
