package mcp

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

const listBroadcastsName = "list_broadcasts"

// maxReportLinks is the most links get_broadcast_report returns, the most
// clicked first.
const maxReportLinks = 50

var broadcastStatuses = []string{
	string(domain.BroadcastStatusDraft),
	string(domain.BroadcastStatusScheduled),
	string(domain.BroadcastStatusProcessing),
	string(domain.BroadcastStatusPaused),
	string(domain.BroadcastStatusProcessed),
	string(domain.BroadcastStatusCancelled),
	string(domain.BroadcastStatusFailed),
	string(domain.BroadcastStatusTesting),
	string(domain.BroadcastStatusTestCompleted),
	string(domain.BroadcastStatusWinnerSelected),
}

var broadcastsRead = Pair{Resource: domain.PermissionResourceBroadcasts, Verb: domain.PermissionTypeRead}

// listBroadcastsTool pages through a workspace's broadcasts, newest first.
// broadcasts.list pages by offset on (created_at, id), so the cursor carries
// the offset.
func (s *Server) listBroadcastsTool() *Tool {
	return &Tool{
		Name:  listBroadcastsName,
		Title: "List broadcasts",
		Description: "Lists the broadcasts (email campaigns) of a workspace, newest first. " +
			"Each has its name, status, audience, schedule, whether an A/B test is on, the templates sent, and the send timestamps. " +
			"total_count counts every broadcast that matches the filters. " +
			"get_broadcast has the full settings, and get_broadcast_report has the delivery, open and click numbers. " +
			"To get the next page, pass next_cursor back as cursor with the same filters. " +
			"A broadcast created between two pages shifts the next page by one.",
		Class: ClassRead,
		Pairs: []Pair{broadcastsRead},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"statuses": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string", "enum": broadcastStatuses},
					"description": "Only broadcasts in one of these statuses.",
				},
				"search":        map[string]any{"type": "string", "description": "Part of the broadcast name to match, case-insensitive."},
				"started_after": map[string]any{"type": "string", "description": "RFC 3339 time. Only broadcasts that started sending at or after it. Broadcasts that never started are left out."},
				"limit":         map[string]any{"type": "integer", "description": "1 to 50, default 20."},
				"cursor":        map[string]any{"type": "string", "description": "next_cursor of the previous page, with the same filters."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.listBroadcasts,
	}
}

type listBroadcastsArgs struct {
	Statuses     []string `json:"statuses"`
	Search       string   `json:"search"`
	StartedAfter string   `json:"started_after"`
	Limit        *int     `json:"limit"`
	Cursor       string   `json:"cursor"`
}

type broadcastAudience struct {
	List                string   `json:"list"`
	Segments            []string `json:"segments"`
	ExcludeUnsubscribed bool     `json:"exclude_unsubscribed"`
}

type broadcastSchedule struct {
	IsScheduled          bool   `json:"is_scheduled"`
	ScheduledDate        string `json:"scheduled_date"`
	ScheduledTime        string `json:"scheduled_time"`
	Timezone             string `json:"timezone"`
	UseRecipientTimezone bool   `json:"use_recipient_timezone"`
}

type broadcastABTest struct {
	Enabled bool `json:"enabled"`
}

// broadcastTimes are the send timestamps every broadcast projection carries.
type broadcastTimes struct {
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	CancelledAt *time.Time `json:"cancelled_at"`
	PausedAt    *time.Time `json:"paused_at"`
	PauseReason *string    `json:"pause_reason"`
}

// broadcastSummary is the projection list_broadcasts returns. The data feed
// and the metadata are left out: get_broadcast has them.
type broadcastSummary struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Status        string            `json:"status"`
	Audience      broadcastAudience `json:"audience"`
	Schedule      broadcastSchedule `json:"schedule"`
	ABTest        broadcastABTest   `json:"ab_test"`
	TemplateIDs   []string          `json:"template_ids"`
	TrackingMode  string            `json:"tracking_mode"`
	EnqueuedCount int               `json:"enqueued_count"`
	broadcastTimes
	Truncation
}

type listBroadcastsOutput struct {
	Workspace  WorkspaceRef       `json:"workspace"`
	Broadcasts []broadcastSummary `json:"broadcasts"`
	TotalCount int                `json:"total_count"`
	NextCursor string             `json:"next_cursor,omitempty"`
}

func (s *Server) listBroadcasts(ctx context.Context, call *Call) (any, error) {
	var args listBroadcastsArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	limit, toolErr := PageLimit(args.Limit)
	if toolErr != nil {
		return nil, toolErr
	}
	params := domain.ListBroadcastsParams{
		WorkspaceID: call.WorkspaceID,
		Search:      strings.TrimSpace(args.Search),
		Limit:       limit,
	}
	filters := map[string]any{}
	if params.Search != "" {
		filters["search"] = params.Search
	}
	if len(args.Statuses) > 0 {
		statuses := slices.Clone(args.Statuses)
		slices.Sort(statuses)
		statuses = slices.Compact(statuses)
		for _, status := range statuses {
			params.Statuses = append(params.Statuses, domain.BroadcastStatus(status))
		}
		filters["statuses"] = statuses
	}
	if args.StartedAfter != "" {
		startedAfter, err := time.Parse(time.RFC3339, args.StartedAfter)
		if err != nil {
			return nil, validationError("started_after must be an RFC 3339 time, such as 2026-11-01T00:00:00Z")
		}
		startedAfter = startedAfter.UTC()
		params.StartedAfter = &startedAfter
		filters["started_after"] = startedAfter.Format(time.RFC3339Nano)
	}
	if args.Cursor != "" {
		if toolErr := DecodeCursor(args.Cursor, listBroadcastsName, call.WorkspaceID, filters, &params.Offset); toolErr != nil {
			return nil, toolErr
		}
		if params.Offset < 0 {
			return nil, validationError("cursor is not valid for this call: drop it to start from the first page")
		}
	}

	resp, err := s.deps.Broadcasts.ListBroadcasts(ctx, params)
	if err != nil {
		return nil, err
	}

	out := listBroadcastsOutput{Workspace: call.WorkspaceRef(), Broadcasts: []broadcastSummary{}, TotalCount: resp.TotalCount}
	for _, broadcast := range resp.Broadcasts {
		if broadcast != nil {
			out.Broadcasts = append(out.Broadcasts, summarizeBroadcast(broadcast))
		}
	}
	if next := params.Offset + len(resp.Broadcasts); len(resp.Broadcasts) > 0 && next < resp.TotalCount {
		cursor, err := EncodeCursor(listBroadcastsName, call.WorkspaceID, filters, next)
		if err != nil {
			return nil, err
		}
		out.NextCursor = cursor
	}
	return out, nil
}

func summarizeBroadcast(broadcast *domain.Broadcast) broadcastSummary {
	out := broadcastSummary{
		ID:            broadcast.ID,
		Status:        string(broadcast.Status),
		Audience:      projectAudience(broadcast.Audience),
		Schedule:      projectSchedule(broadcast.Schedule),
		ABTest:        broadcastABTest{Enabled: broadcast.TestSettings.Enabled},
		TemplateIDs:   []string{},
		TrackingMode:  broadcastTrackingMode(broadcast.TrackingMode),
		EnqueuedCount: broadcast.EnqueuedCount,
	}
	out.Name = out.Text("name", broadcast.Name)
	for _, variation := range broadcast.TestSettings.Variations {
		if variation.TemplateID != "" && !slices.Contains(out.TemplateIDs, variation.TemplateID) {
			out.TemplateIDs = append(out.TemplateIDs, variation.TemplateID)
		}
	}
	out.broadcastTimes = projectBroadcastTimes(broadcast, &out.Truncation)
	return out
}

func projectAudience(audience domain.AudienceSettings) broadcastAudience {
	segments := audience.Segments
	if segments == nil {
		segments = []string{}
	}
	return broadcastAudience{List: audience.List, Segments: segments, ExcludeUnsubscribed: audience.ExcludeUnsubscribed}
}

func projectSchedule(schedule domain.ScheduleSettings) broadcastSchedule {
	return broadcastSchedule{
		IsScheduled:          schedule.IsScheduled,
		ScheduledDate:        schedule.ScheduledDate,
		ScheduledTime:        schedule.ScheduledTime,
		Timezone:             schedule.Timezone,
		UseRecipientTimezone: schedule.UseRecipientTimezone,
	}
}

func projectBroadcastTimes(broadcast *domain.Broadcast, truncation *Truncation) broadcastTimes {
	out := broadcastTimes{
		CreatedAt:   broadcast.CreatedAt,
		UpdatedAt:   broadcast.UpdatedAt,
		StartedAt:   broadcast.StartedAt,
		CompletedAt: broadcast.CompletedAt,
		CancelledAt: broadcast.CancelledAt,
		PausedAt:    broadcast.PausedAt,
	}
	if broadcast.PauseReason != nil {
		reason := truncation.Text("pause_reason", *broadcast.PauseReason)
		out.PauseReason = &reason
	}
	return out
}

// broadcastTrackingMode names the stored "" as the console does: the
// broadcast follows the workspace and each contact's consent.
func broadcastTrackingMode(mode string) string {
	if mode == "" {
		return notifuse_mjml.TrackingModeInherit
	}
	return mode
}

// getBroadcastTool reads one broadcast's settings.
func (s *Server) getBroadcastTool() *Tool {
	return &Tool{
		Name:  "get_broadcast",
		Title: "Get broadcast",
		Description: "Reads one broadcast: " +
			"status, audience, schedule, A/B test settings with each variation's template and metrics, UTM parameters, tracking mode, data feed settings, metadata and send timestamps. " +
			"Use get_template to read a template's content. Data feed header values and fetched feed data are never returned, only the header names. " +
			"Long text is cut at 2000 characters and named in truncated_fields. Metadata longer than that is null.",
		Class: ClassRead,
		Pairs: []Pair{broadcastsRead},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"broadcast_id": map[string]any{"type": "string", "description": "Broadcast ID from list_broadcasts."},
			},
			"required":             []string{"workspace_id", "broadcast_id"},
			"additionalProperties": false,
		},
		Handler: s.getBroadcast,
	}
}

type broadcastIDArgs struct {
	BroadcastID string `json:"broadcast_id"`
	TemplateID  string `json:"template_id"`
}

type variationMetrics struct {
	Recipients   int `json:"recipients"`
	Delivered    int `json:"delivered"`
	Opens        int `json:"opens"`
	Clicks       int `json:"clicks"`
	Bounced      int `json:"bounced"`
	Complained   int `json:"complained"`
	Unsubscribed int `json:"unsubscribed"`
}

type broadcastVariation struct {
	VariationName string            `json:"variation_name"`
	TemplateID    string            `json:"template_id"`
	SenderID      string            `json:"sender_id"`
	Metrics       *variationMetrics `json:"metrics"`
}

type broadcastTestSettings struct {
	Enabled              bool                 `json:"enabled"`
	SamplePercentage     int                  `json:"sample_percentage"`
	AutoSendWinner       bool                 `json:"auto_send_winner"`
	AutoSendWinnerMetric string               `json:"auto_send_winner_metric"`
	TestDurationHours    int                  `json:"test_duration_hours"`
	Variations           []broadcastVariation `json:"variations"`
}

type broadcastUTM struct {
	Source   string `json:"source"`
	Medium   string `json:"medium"`
	Campaign string `json:"campaign"`
	Term     string `json:"term"`
	Content  string `json:"content"`
}

// feedSettings is a data feed without its header values, which hold
// credentials such as an Authorization header.
type feedSettings struct {
	Enabled     bool     `json:"enabled"`
	URL         string   `json:"url"`
	HeaderNames []string `json:"header_names"`
}

// broadcastDataFeed leaves out the fetched global feed data: it is unbounded
// and can hold anything the feed answered.
type broadcastDataFeed struct {
	GlobalFeed          *feedSettings `json:"global_feed"`
	RecipientFeed       *feedSettings `json:"recipient_feed"`
	GlobalFeedFetchedAt *time.Time    `json:"global_feed_fetched_at"`
}

type broadcastDetail struct {
	ID                        string                `json:"id"`
	Name                      string                `json:"name"`
	Status                    string                `json:"status"`
	Audience                  broadcastAudience     `json:"audience"`
	Schedule                  broadcastSchedule     `json:"schedule"`
	TestSettings              broadcastTestSettings `json:"test_settings"`
	UTMParameters             *broadcastUTM         `json:"utm_parameters"`
	TrackingMode              string                `json:"tracking_mode"`
	DataFeed                  *broadcastDataFeed    `json:"data_feed"`
	Metadata                  any                   `json:"metadata"`
	WinningTemplate           *string               `json:"winning_template"`
	TestSentAt                *time.Time            `json:"test_sent_at"`
	WinnerSentAt              *time.Time            `json:"winner_sent_at"`
	TestPhaseRecipientCount   int                   `json:"test_phase_recipient_count"`
	WinnerPhaseRecipientCount int                   `json:"winner_phase_recipient_count"`
	TestPausedSeconds         int                   `json:"test_paused_seconds"`
	EnqueuedCount             int                   `json:"enqueued_count"`
	broadcastTimes
	Truncation
}

type getBroadcastOutput struct {
	Workspace WorkspaceRef    `json:"workspace"`
	Broadcast broadcastDetail `json:"broadcast"`
}

func (s *Server) getBroadcast(ctx context.Context, call *Call) (any, error) {
	var args broadcastIDArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if args.BroadcastID == "" {
		return nil, validationError("broadcast_id is required")
	}
	// Variation templates are never asked for: get_template reads them.
	broadcast, err := s.deps.Broadcasts.GetBroadcast(ctx, call.WorkspaceID, args.BroadcastID, domain.GetBroadcastOptions{})
	if err != nil {
		return nil, err
	}
	if broadcast == nil {
		return nil, &domain.ErrBroadcastNotFound{ID: args.BroadcastID}
	}
	return getBroadcastOutput{Workspace: call.WorkspaceRef(), Broadcast: projectBroadcast(broadcast)}, nil
}

func projectBroadcast(broadcast *domain.Broadcast) broadcastDetail {
	settings := broadcast.TestSettings
	out := broadcastDetail{
		ID:       broadcast.ID,
		Status:   string(broadcast.Status),
		Audience: projectAudience(broadcast.Audience),
		Schedule: projectSchedule(broadcast.Schedule),
		TestSettings: broadcastTestSettings{
			Enabled:              settings.Enabled,
			SamplePercentage:     settings.SamplePercentage,
			AutoSendWinner:       settings.AutoSendWinner,
			AutoSendWinnerMetric: string(settings.AutoSendWinnerMetric),
			TestDurationHours:    settings.TestDurationHours,
			Variations:           []broadcastVariation{},
		},
		TrackingMode:              broadcastTrackingMode(broadcast.TrackingMode),
		WinningTemplate:           broadcast.WinningTemplate,
		TestSentAt:                broadcast.TestSentAt,
		WinnerSentAt:              broadcast.WinnerSentAt,
		TestPhaseRecipientCount:   broadcast.TestPhaseRecipientCount,
		WinnerPhaseRecipientCount: broadcast.WinnerPhaseRecipientCount,
		TestPausedSeconds:         broadcast.TestPausedSeconds,
		EnqueuedCount:             broadcast.EnqueuedCount,
	}
	out.Name = out.Text("name", broadcast.Name)
	for i, variation := range settings.Variations {
		projected := broadcastVariation{
			VariationName: out.Text(fmt.Sprintf("test_settings.variations[%d].variation_name", i), variation.VariationName),
			TemplateID:    variation.TemplateID,
			SenderID:      variation.SenderID,
		}
		if m := variation.Metrics; m != nil {
			projected.Metrics = &variationMetrics{
				Recipients: m.Recipients, Delivered: m.Delivered, Opens: m.Opens, Clicks: m.Clicks,
				Bounced: m.Bounced, Complained: m.Complained, Unsubscribed: m.Unsubscribed,
			}
		}
		out.TestSettings.Variations = append(out.TestSettings.Variations, projected)
	}
	if utm := broadcast.UTMParameters; utm != nil {
		out.UTMParameters = &broadcastUTM{
			Source:   out.Text("utm_parameters.source", utm.Source),
			Medium:   out.Text("utm_parameters.medium", utm.Medium),
			Campaign: out.Text("utm_parameters.campaign", utm.Campaign),
			Term:     out.Text("utm_parameters.term", utm.Term),
			Content:  out.Text("utm_parameters.content", utm.Content),
		}
	}
	if feed := broadcast.DataFeed; feed != nil {
		out.DataFeed = &broadcastDataFeed{GlobalFeedFetchedAt: feed.GlobalFeedFetchedAt}
		if global := feed.GlobalFeed; global != nil {
			out.DataFeed.GlobalFeed = projectFeed(&out.Truncation, "data_feed.global_feed.url", global.Enabled, global.URL, global.Headers)
		}
		if recipient := feed.RecipientFeed; recipient != nil {
			out.DataFeed.RecipientFeed = projectFeed(&out.Truncation, "data_feed.recipient_feed.url", recipient.Enabled, recipient.URL, recipient.Headers)
		}
	}
	if len(broadcast.Metadata) > 0 {
		out.Metadata = out.JSON("metadata", map[string]any(broadcast.Metadata))
	}
	out.broadcastTimes = projectBroadcastTimes(broadcast, &out.Truncation)
	return out
}

func projectFeed(truncation *Truncation, urlPath string, enabled bool, url string, headers []domain.DataFeedHeader) *feedSettings {
	out := &feedSettings{Enabled: enabled, URL: truncation.Text(urlPath, url), HeaderNames: []string{}}
	for i, header := range headers {
		out.HeaderNames = append(out.HeaderNames, truncation.Text(fmt.Sprintf("%s.header_names[%d]", strings.TrimSuffix(urlPath, ".url"), i), header.Name))
	}
	return out
}

// getBroadcastReportTool reads how a broadcast performed. The stats services
// check only message_history:read and answer zeros for a broadcast that does
// not exist, so the broadcast is read first: that checks broadcasts:read and
// answers not_found.
func (s *Server) getBroadcastReportTool() *Tool {
	return &Tool{
		Name:  "get_broadcast_report",
		Title: "Get broadcast report",
		Description: "Reports how a broadcast performed. " +
			"It counts messages sent, delivered, bounced, complained, failed, opened, clicked and unsubscribed, and how many were tracked for opens and clicks (the denominators of the open and click rates). " +
			"It shows what the broadcast's queue still holds, which is absent when the queue could not be read. " +
			"It gives the same numbers for each A/B variation. It lists the clicked links, at most 50, most clicked first. " +
			"links_truncated is true when more links exist. template_id limits the links to one variation.",
		Class: ClassRead,
		Pairs: []Pair{broadcastsRead, {Resource: domain.PermissionResourceMessageHistory, Verb: domain.PermissionTypeRead}},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"broadcast_id": map[string]any{"type": "string", "description": "Broadcast ID from list_broadcasts."},
				"template_id":  map[string]any{"type": "string", "description": "Only the links of the A/B variation sent with this template."},
			},
			"required":             []string{"workspace_id", "broadcast_id"},
			"additionalProperties": false,
		},
		Handler: s.getBroadcastReport,
	}
}

type reportBroadcast struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	WinningTemplate *string `json:"winning_template"`
}

type reportTotals struct {
	Sent          int `json:"sent"`
	Delivered     int `json:"delivered"`
	Bounced       int `json:"bounced"`
	Complained    int `json:"complained"`
	Failed        int `json:"failed"`
	Opened        int `json:"opened"`
	Clicked       int `json:"clicked"`
	Unsubscribed  int `json:"unsubscribed"`
	OpensTracked  int `json:"opens_tracked"`
	ClicksTracked int `json:"clicks_tracked"`
}

type reportQueue struct {
	Pending        int64 `json:"pending"`
	Processing     int64 `json:"processing"`
	Paused         int64 `json:"paused"`
	FailedRetrying int64 `json:"failed_retrying"`
	FailedTerminal int64 `json:"failed_terminal"`
}

type reportVariation struct {
	TemplateID    string `json:"template_id"`
	VariationName string `json:"variation_name"`
	reportTotals
}

type reportLink struct {
	URL          string `json:"url"`
	TotalClicks  int64  `json:"total_clicks"`
	UniqueClicks int64  `json:"unique_clicks"`
}

type getBroadcastReportOutput struct {
	Workspace      WorkspaceRef      `json:"workspace"`
	Broadcast      reportBroadcast   `json:"broadcast"`
	Totals         reportTotals      `json:"totals"`
	Queue          *reportQueue      `json:"queue,omitempty"`
	Variations     []reportVariation `json:"variations"`
	Links          []reportLink      `json:"links"`
	LinksTruncated bool              `json:"links_truncated"`
	Truncation
}

func (s *Server) getBroadcastReport(ctx context.Context, call *Call) (any, error) {
	var args broadcastIDArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if args.BroadcastID == "" {
		return nil, validationError("broadcast_id is required")
	}
	broadcast, err := s.deps.Broadcasts.GetBroadcast(ctx, call.WorkspaceID, args.BroadcastID, domain.GetBroadcastOptions{})
	if err != nil {
		return nil, err
	}
	if broadcast == nil {
		return nil, &domain.ErrBroadcastNotFound{ID: args.BroadcastID}
	}
	if args.TemplateID != "" && !slices.ContainsFunc(broadcast.TestSettings.Variations, func(v domain.BroadcastVariation) bool {
		return v.TemplateID == args.TemplateID
	}) {
		return nil, validationError("template_id %q is not a template this broadcast sends", args.TemplateID)
	}

	stats, err := s.deps.Messages.GetBroadcastStats(ctx, call.WorkspaceID, broadcast.ID)
	if err != nil {
		return nil, err
	}
	out := getBroadcastReportOutput{
		Workspace:  call.WorkspaceRef(),
		Broadcast:  reportBroadcast{ID: broadcast.ID, Status: string(broadcast.Status), WinningTemplate: broadcast.WinningTemplate},
		Variations: []reportVariation{},
		Links:      []reportLink{},
	}
	out.Broadcast.Name = out.Text("broadcast.name", broadcast.Name)
	if stats != nil {
		out.Totals = projectTotals(stats.Stats)
		if queue := stats.Queue; queue != nil {
			out.Queue = &reportQueue{
				Pending: queue.Pending, Processing: queue.Processing, Paused: queue.Paused,
				FailedRetrying: queue.FailedRetrying, FailedTerminal: queue.FailedTerminal,
			}
		}
	}

	if broadcast.TestSettings.Enabled {
		for _, variation := range broadcast.TestSettings.Variations {
			if variation.TemplateID == "" {
				continue
			}
			sum, err := s.deps.Messages.GetBroadcastVariationStats(ctx, call.WorkspaceID, broadcast.ID, variation.TemplateID)
			if err != nil {
				return nil, err
			}
			out.Variations = append(out.Variations, reportVariation{
				TemplateID:    variation.TemplateID,
				VariationName: out.Text(fmt.Sprintf("variations[%d].variation_name", len(out.Variations)), variation.VariationName),
				reportTotals:  projectTotals(sum),
			})
		}
	}

	links, err := s.deps.Messages.GetBroadcastLinkStats(ctx, call.WorkspaceID, broadcast.ID, args.TemplateID)
	if err != nil {
		return nil, err
	}
	links = slices.Clone(links)
	slices.SortFunc(links, func(a, b domain.LinkClickStats) int {
		if c := cmp.Compare(b.TotalClicks, a.TotalClicks); c != 0 {
			return c
		}
		return cmp.Compare(a.URL, b.URL)
	})
	if len(links) > maxReportLinks {
		links = links[:maxReportLinks]
		out.LinksTruncated = true
	}
	for i, link := range links {
		out.Links = append(out.Links, reportLink{
			URL:          out.Text(fmt.Sprintf("links[%d].url", i), link.URL),
			TotalClicks:  link.TotalClicks,
			UniqueClicks: link.UniqueClicks,
		})
	}
	return out, nil
}

func projectTotals(sum *domain.MessageHistoryStatusSum) reportTotals {
	if sum == nil {
		return reportTotals{}
	}
	return reportTotals{
		Sent: sum.TotalSent, Delivered: sum.TotalDelivered, Bounced: sum.TotalBounced, Complained: sum.TotalComplained,
		Failed: sum.TotalFailed, Opened: sum.TotalOpened, Clicked: sum.TotalClicked, Unsubscribed: sum.TotalUnsubscribed,
		OpensTracked: sum.TotalOpensTracked, ClicksTracked: sum.TotalClicksTracked,
	}
}
