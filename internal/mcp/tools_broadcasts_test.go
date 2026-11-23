package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
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
