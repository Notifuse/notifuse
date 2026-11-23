package mcp

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"

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

var (
	broadcastsWrite = Pair{Resource: domain.PermissionResourceBroadcasts, Verb: domain.PermissionTypeWrite}
	readTemplates   = Pair{Resource: domain.PermissionResourceTemplates, Verb: domain.PermissionTypeRead}
)

// maxFreeFormBytes bounds a free-form object argument (metadata, template
// data), which no schema keyword can describe.
const maxFreeFormBytes = 32 << 10

// freeFormArgument refuses a free-form object larger than maxFreeFormBytes
// once serialized.
func freeFormArgument(name string, value map[string]any) *ToolError {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return validationError("%s is not valid JSON", name)
	}
	if len(data) > maxFreeFormBytes {
		return validationError("%s is limited to %d bytes of JSON", name, maxFreeFormBytes)
	}
	return nil
}

// recipientArgument normalizes an address argument and checks its form.
func recipientArgument(name, raw string) (string, *ToolError) {
	email := domain.NormalizeEmail(raw)
	if email == "" || !govalidator.IsEmail(email) {
		return "", validationError("%s must be a valid email address", name)
	}
	return email, nil
}

// broadcastTrackingModes are the tracking modes a broadcast takes.
var broadcastTrackingModes = []string{notifuse_mjml.TrackingModeInherit, notifuse_mjml.TrackingModeNoTracking}

func utmSchema(description string) map[string]any {
	text := map[string]any{"type": "string"}
	return map[string]any{
		"type":        "object",
		"description": description,
		"properties": map[string]any{
			"source": text, "medium": text, "campaign": text, "term": text, "content": text,
		},
		"additionalProperties": false,
	}
}

func abTestSchema(description string) map[string]any {
	return map[string]any{
		"type":        "object",
		"description": description,
		"properties": map[string]any{
			"sample_percentage": map[string]any{"type": "integer", "description": "Share of the audience that gets the test, 1 to 100."},
			"auto_send_winner": map[string]any{
				"type":        "boolean",
				"description": "Pick the winner on its own once test_duration_hours have passed, and send it to the rest of the audience. Without it, pick the winner with select_broadcast_winner.",
			},
			"auto_send_winner_metric": map[string]any{"type": "string", "enum": []string{string(domain.TestWinnerMetricOpenRate), string(domain.TestWinnerMetricClickRate)}, "description": "What picks the winner. Required with auto_send_winner."},
			"test_duration_hours":     map[string]any{"type": "integer", "description": "1 to 168. Required with auto_send_winner."},
			"variations": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"variation_name": map[string]any{"type": "string", "description": "Default: A, B, C and so on, in order."},
						"template_id":    map[string]any{"type": "string", "description": "Template ID from list_templates."},
					},
					"required":             []string{"template_id"},
					"additionalProperties": false,
				},
				"description": "2 to 8 variations, one template each.",
			},
		},
		"required":             []string{"sample_percentage", "variations"},
		"additionalProperties": false,
	}
}

// variationNames are the default names of A/B variations, by position.
const variationNames = "ABCDEFGH"

// testSettingsFromArgs turns template_id or ab_test into the test_settings
// object of a broadcast request. senders keeps the sender a stored variation
// chose for a template, which no argument sets.
func testSettingsFromArgs(args map[string]any, senders map[string]string) (map[string]any, bool) {
	variation := func(name, templateID string) map[string]any {
		out := map[string]any{"variation_name": name, "template_id": templateID}
		if sender := senders[templateID]; sender != "" {
			out["sender_id"] = sender
		}
		return out
	}
	if templateID, ok := args["template_id"].(string); ok {
		return map[string]any{"enabled": false, "variations": []any{variation("A", templateID)}}, true
	}
	abTest, ok := args["ab_test"].(map[string]any)
	if !ok {
		return nil, false
	}
	settings := map[string]any{"enabled": true}
	for _, key := range []string{"sample_percentage", "auto_send_winner", "auto_send_winner_metric", "test_duration_hours"} {
		if value, given := abTest[key]; given {
			settings[key] = value
		}
	}
	variations := []any{}
	items, _ := abTest["variations"].([]any)
	for i, item := range items {
		fields, _ := item.(map[string]any)
		templateID, _ := fields["template_id"].(string)
		name, _ := fields["variation_name"].(string)
		if name == "" && i < len(variationNames) {
			name = variationNames[i : i+1]
		}
		variations = append(variations, variation(name, templateID))
	}
	settings["variations"] = variations
	return settings, true
}

// copyArgs copies the given keys of args into fields, renamed by the map.
func copyArgs(args map[string]any, fields map[string]any, names map[string]string) {
	for arg, field := range names {
		if value, ok := args[arg]; ok {
			fields[field] = value
		}
	}
}

// broadcastOutput is what create_broadcast and update_broadcast return.
type broadcastOutput struct {
	Workspace WorkspaceRef    `json:"workspace"`
	Broadcast broadcastDetail `json:"broadcast"`
}

func (s *Server) createBroadcastTool() *Tool {
	return &Tool{
		Name:  "create_broadcast",
		Title: "Create broadcast",
		Description: "Creates a draft broadcast (an email campaign) for a list. It sends nothing: schedule_broadcast sends it. " +
			"Give template_id to send one template, or ab_test to test several templates on part of the audience first. " +
			"By default the list's unsubscribed contacts are left out. Set exclude_unsubscribed to false only for a notice every member must get, such as a security or legal notice. " +
			"The list, segments and templates are not checked here: a wrong ID only shows when the broadcast sends. " +
			"The broadcast gets a new ID on every call, so calling it again creates a second draft.",
		Class:       ClassWrite,
		Idempotent:  boolPtr(false),
		Pairs:       []Pair{broadcastsWrite},
		AuditAction: "broadcasts.create",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"name":         map[string]any{"type": "string", "description": "1 to 255 bytes."},
				"list_id":      map[string]any{"type": "string", "description": "List ID from list_lists: the audience."},
				"segment_ids":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Only the list's contacts in these segments. Default: the whole list."},
				"exclude_unsubscribed": map[string]any{
					"type":        "boolean",
					"description": "Default true. false also sends to the list's unsubscribed contacts and leaves out the one-click unsubscribe header. Use false only for a notice every member must get.",
				},
				"template_id":    map[string]any{"type": "string", "description": "Template ID from list_templates, sent to the whole audience. Give template_id or ab_test, not both."},
				"ab_test":        abTestSchema("A/B test of several templates. Give template_id or ab_test, not both."),
				"utm_parameters": utmSchema("UTM parameters added to the links of the email."),
				"tracking_mode":  map[string]any{"type": "string", "enum": broadcastTrackingModes, "description": "inherit (default) follows the workspace and each contact's consent. no_tracking adds no open pixel and no click redirect."},
				"metadata":       map[string]any{"type": "object", "description": "Your own key and value pairs stored on the broadcast, at most 32 KiB."},
			},
			"required":             []string{"workspace_id", "name", "list_id"},
			"additionalProperties": false,
		},
		Handler: s.createBroadcast,
	}
}

func (s *Server) createBroadcast(ctx context.Context, call *Call) (any, error) {
	args := call.Args
	name, _ := args["name"].(string)
	call.Audit.SetTarget(domain.AuditTargetBroadcast, "", name)
	_, withTemplate := args["template_id"]
	_, withTest := args["ab_test"]
	if withTemplate == withTest {
		return nil, validationError("give exactly one of template_id and ab_test")
	}
	metadata, _ := args["metadata"].(map[string]any)
	if toolErr := freeFormArgument("metadata", metadata); toolErr != nil {
		return nil, toolErr
	}

	// exclude_unsubscribed is always written: an omitted key mails the list's
	// unsubscribed contacts in a request filled field by field.
	excludeUnsubscribed := true
	if value, ok := args["exclude_unsubscribed"].(bool); ok {
		excludeUnsubscribed = value
	}
	audience := map[string]any{"list": args["list_id"], "exclude_unsubscribed": excludeUnsubscribed}
	copyArgs(args, audience, map[string]string{"segment_ids": "segments"})
	testSettings, _ := testSettingsFromArgs(args, nil)
	fields := map[string]any{
		"workspace_id":  call.WorkspaceID,
		"name":          name,
		"audience":      audience,
		"test_settings": testSettings,
	}
	copyArgs(args, fields, map[string]string{"utm_parameters": "utm_parameters", "tracking_mode": "tracking_mode", "metadata": "metadata"})

	var req domain.CreateBroadcastRequest
	if err := requestFromJSON(fields, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	// broadcasts.create validates in its handler; the service's own errors
	// for the same checks are untyped.
	if _, err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	broadcast, err := s.deps.Broadcasts.CreateBroadcast(ctx, &req)
	if err != nil {
		return nil, err
	}
	call.Audit.SetTarget("", broadcast.ID, broadcast.Name)
	return broadcastOutput{Workspace: call.WorkspaceRef(), Broadcast: projectBroadcast(broadcast)}, nil
}

func (s *Server) updateBroadcastTool() *Tool {
	return &Tool{
		Name:  "update_broadcast",
		Title: "Update broadcast",
		Description: "Changes the fields you give on a broadcast that is a draft, scheduled or paused, and keeps the others: the schedule, the audience keys and the templates you leave out stay as they are. " +
			"A scheduled or paused broadcast sends the new settings, so confirm the change with the user first. " +
			"template_id replaces the templates with that one and turns any A/B test off; ab_test replaces the whole test. " +
			"exclude_unsubscribed and tracking_mode can change only before the broadcast starts sending. " +
			"The schedule is not changed here: use schedule_broadcast. " +
			"The broadcast is read then written, so a change saved by someone else in between can be overwritten. " +
			"Calling it again with the same arguments changes nothing more.",
		Class:       ClassWriteOpenWorld,
		Idempotent:  boolPtr(true),
		Pairs:       []Pair{broadcastsWrite},
		AuditAction: "broadcasts.update",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"broadcast_id": map[string]any{"type": "string", "description": "Broadcast ID from list_broadcasts."},
				"name":         map[string]any{"type": "string", "description": "1 to 255 bytes."},
				"list_id":      map[string]any{"type": "string", "description": "List ID from list_lists: the new audience. The segments stay unless you give segment_ids."},
				"segment_ids":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Replaces the segments. An empty array sends to the whole list."},
				"exclude_unsubscribed": map[string]any{
					"type":        "boolean",
					"description": "false also sends to the list's unsubscribed contacts and leaves out the one-click unsubscribe header. Use false only for a notice every member must get.",
				},
				"template_id":    map[string]any{"type": "string", "description": "Template ID from list_templates. Replaces the templates and turns the A/B test off. Give template_id or ab_test, not both."},
				"ab_test":        abTestSchema("Replaces the templates and the A/B test settings. Give template_id or ab_test, not both."),
				"utm_parameters": utmSchema("Replaces all UTM parameters. Keys you leave out are cleared."),
				"tracking_mode":  map[string]any{"type": "string", "enum": broadcastTrackingModes, "description": "inherit follows the workspace and each contact's consent. no_tracking adds no open pixel and no click redirect."},
				"metadata":       map[string]any{"type": "object", "description": "Replaces the stored key and value pairs, at most 32 KiB."},
			},
			"required":             []string{"workspace_id", "broadcast_id"},
			"additionalProperties": false,
		},
		Handler: s.updateBroadcast,
	}
}

// updatableBroadcastStatuses are the statuses broadcasts.update accepts.
var updatableBroadcastStatuses = []domain.BroadcastStatus{
	domain.BroadcastStatusDraft, domain.BroadcastStatusScheduled, domain.BroadcastStatusPaused,
}

func (s *Server) updateBroadcast(ctx context.Context, call *Call) (any, error) {
	args := call.Args
	id, _ := args["broadcast_id"].(string)
	call.Audit.SetTarget(domain.AuditTargetBroadcast, id, "")
	changes := 0
	for key := range args {
		if key != "workspace_id" && key != "broadcast_id" {
			changes++
		}
	}
	if changes == 0 {
		return nil, validationError("give at least one field to change besides broadcast_id")
	}
	_, withTemplate := args["template_id"]
	_, withTest := args["ab_test"]
	if withTemplate && withTest {
		return nil, validationError("give template_id or ab_test, not both")
	}
	if name, ok := args["name"].(string); ok && name == "" {
		return nil, validationError("name cannot be empty")
	}
	metadata, _ := args["metadata"].(map[string]any)
	if toolErr := freeFormArgument("metadata", metadata); toolErr != nil {
		return nil, toolErr
	}

	// The service checks the request against the broadcast under its lock, but
	// answers its refusals untyped. The same checks run here first on the row
	// as it stands, so a refusal comes back as invalid_state or validation. The
	// row also gives the name, which the request requires even when unchanged,
	// and the senders the stored variations chose.
	stored, err := s.deps.BroadcastRows.GetBroadcast(ctx, call.WorkspaceID, id)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, &domain.ErrBroadcastNotFound{ID: id}
	}
	call.Audit.SetTarget("", "", stored.Name)
	if !slices.Contains(updatableBroadcastStatuses, stored.Status) {
		return nil, domain.NewInvalidStateError("broadcast", stored.ID, "update", string(stored.Status))
	}

	fields := map[string]any{"workspace_id": call.WorkspaceID, "id": id, "name": stored.Name}
	copyArgs(args, fields, map[string]string{"name": "name", "utm_parameters": "utm_parameters", "tracking_mode": "tracking_mode", "metadata": "metadata"})
	// Only the audience keys given are sent: the request keeps the others.
	audience := map[string]any{}
	copyArgs(args, audience, map[string]string{"list_id": "list", "segment_ids": "segments", "exclude_unsubscribed": "exclude_unsubscribed"})
	if len(audience) > 0 {
		fields["audience"] = audience
	}
	senders := map[string]string{}
	for _, variation := range stored.TestSettings.Variations {
		if variation.SenderID != "" {
			senders[variation.TemplateID] = variation.SenderID
		}
	}
	if testSettings, ok := testSettingsFromArgs(args, senders); ok {
		fields["test_settings"] = testSettings
	}
	// The schedule is never sent: an empty one would clear it.

	var req domain.UpdateBroadcastRequest
	if err := requestFromJSON(fields, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	check := *stored
	if _, err := req.Validate(&check); err != nil {
		return nil, validationError("%v", err)
	}
	broadcast, err := s.deps.Broadcasts.UpdateBroadcast(ctx, &req)
	if err != nil {
		return nil, err
	}
	call.Audit.SetTarget("", "", broadcast.Name)
	return broadcastOutput{Workspace: call.WorkspaceRef(), Broadcast: projectBroadcast(broadcast)}, nil
}

// broadcastIDSchema is the input schema of a tool that takes a broadcast ID
// only.
func broadcastIDSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
			"broadcast_id": map[string]any{"type": "string", "description": "Broadcast ID from list_broadcasts."},
		},
		"required":             []string{"workspace_id", "broadcast_id"},
		"additionalProperties": false,
	}
}

// broadcastIDArgument reads broadcast_id and names it in the audit row.
func broadcastIDArgument(call *Call) (string, *ToolError) {
	id, _ := call.Args["broadcast_id"].(string)
	if id == "" {
		return "", validationError("broadcast_id is required")
	}
	call.Audit.SetTarget(domain.AuditTargetBroadcast, id, "")
	return id, nil
}

// requireMarketingProvider refuses a broadcast send when the workspace has no
// marketing email integration, which the services answer untyped.
func requireMarketingProvider(workspace *domain.Workspace) *ToolError {
	provider, err := workspace.GetEmailProvider(true)
	if err != nil {
		return validationError("the workspace's marketing email integration cannot send: %v", err)
	}
	if provider == nil {
		return validationError("the workspace has no marketing email integration: set one in the console first")
	}
	return nil
}

// auditBroadcastAudience reads a broadcast once a send tool has acted on it,
// and records its audience and templates in the audit row. The read is the
// state the call left; it is nil when the read fails, which the call's
// outcome does not depend on.
func (s *Server) auditBroadcastAudience(ctx context.Context, call *Call, id string) *domain.Broadcast {
	broadcast, err := s.deps.BroadcastRows.GetBroadcast(ctx, call.WorkspaceID, id)
	if err != nil || broadcast == nil {
		return nil
	}
	call.Audit.SetTarget("", "", broadcast.Name)
	call.Audit.AddMetadata("list_id", broadcast.Audience.List)
	segments := broadcast.Audience.Segments
	if segments == nil {
		segments = []string{}
	}
	call.Audit.AddMetadata("segment_ids", segments)
	call.Audit.AddMetadata("template_ids", summarizeBroadcast(broadcast).TemplateIDs)
	return broadcast
}

func (s *Server) sendBroadcastToContactTool() *Tool {
	return &Tool{
		Name:  "send_broadcast_to_contact",
		Title: "Send broadcast to one address",
		Description: "Sends a broadcast's email to one address now, through the marketing email integration, as a test or for one person. " +
			"The address need not be a contact; a contact with that address gets its own data in the email. " +
			"It does not change the broadcast or its audience. " +
			"Calling it again sends the email again, so confirm the address with the user first.",
		Class:       ClassSend,
		Pairs:       []Pair{broadcastsWrite, readTemplates},
		AuditAction: "broadcasts.sendToIndividual",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":    map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"broadcast_id":    map[string]any{"type": "string", "description": "Broadcast ID from list_broadcasts."},
				"recipient_email": map[string]any{"type": "string", "description": "The address that gets the email."},
				"template_id":     map[string]any{"type": "string", "description": "Template of the A/B variation to send. Default: the first one."},
			},
			"required":             []string{"workspace_id", "broadcast_id", "recipient_email"},
			"additionalProperties": false,
		},
		Handler: s.sendBroadcastToContact,
	}
}

type sendBroadcastToContactOutput struct {
	Workspace      WorkspaceRef `json:"workspace"`
	Sent           bool         `json:"sent"`
	BroadcastID    string       `json:"broadcast_id"`
	TemplateID     string       `json:"template_id"`
	RecipientEmail string       `json:"recipient_email"`
}

func (s *Server) sendBroadcastToContact(ctx context.Context, call *Call) (any, error) {
	id, toolErr := broadcastIDArgument(call)
	if toolErr != nil {
		return nil, toolErr
	}
	raw, _ := call.Args["recipient_email"].(string)
	recipient, toolErr := recipientArgument("recipient_email", raw)
	if toolErr != nil {
		return nil, toolErr
	}
	call.Audit.AddMetadata("recipient_email", recipient)
	if toolErr := requireMarketingProvider(call.Workspace); toolErr != nil {
		return nil, toolErr
	}

	// The broadcast is read to name the template sent, which the service
	// picks without saying, and to refuse a broadcast without one, which it
	// answers untyped.
	broadcast, err := s.deps.BroadcastRows.GetBroadcast(ctx, call.WorkspaceID, id)
	if err != nil {
		return nil, err
	}
	if broadcast == nil {
		return nil, &domain.ErrBroadcastNotFound{ID: id}
	}
	call.Audit.SetTarget("", "", broadcast.Name)
	templateID, _ := call.Args["template_id"].(string)
	if templateID == "" {
		if len(broadcast.TestSettings.Variations) == 0 || broadcast.TestSettings.Variations[0].TemplateID == "" {
			return nil, validationError("broadcast %s has no template to send", id)
		}
		templateID = broadcast.TestSettings.Variations[0].TemplateID
	}
	call.Audit.AddMetadata("template_id", templateID)

	var req domain.SendToIndividualRequest
	if err := requestFromJSON(map[string]any{
		"workspace_id": call.WorkspaceID, "broadcast_id": id, "recipient_email": recipient, "template_id": templateID,
	}, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	if err := s.deps.Broadcasts.SendToIndividual(ctx, &req); err != nil {
		return nil, err
	}
	return sendBroadcastToContactOutput{
		Workspace: call.WorkspaceRef(), Sent: true, BroadcastID: id, TemplateID: templateID, RecipientEmail: recipient,
	}, nil
}

func (s *Server) scheduleBroadcastTool() *Tool {
	return &Tool{
		Name:  "schedule_broadcast",
		Title: "Schedule broadcast",
		Description: "Sends a draft broadcast to its audience, now or at a set time, through the marketing email integration. " +
			"This emails every contact of the audience, so confirm the broadcast, its audience and the time with the user first. " +
			"when is \"now\" or an RFC 3339 time in the future, kept to the minute: the seconds are dropped, and the schedule is stored in UTC. " +
			"Sending at each contact's own local time (use_recipient_timezone in the API) is not offered. " +
			"Only a draft can be scheduled: calling it again answers invalid_state with the broadcast's status, and sends nothing more. " +
			"To stop a scheduled or sending broadcast, use pause_broadcast or cancel_broadcast.",
		Class:       ClassSend,
		Idempotent:  boolPtr(true),
		Pairs:       []Pair{broadcastsWrite},
		AuditAction: "broadcasts.schedule",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"broadcast_id": map[string]any{"type": "string", "description": "Broadcast ID from list_broadcasts."},
				"when":         map[string]any{"type": "string", "description": "\"now\", or an RFC 3339 time in the future, such as 2026-12-01T09:00:00+01:00."},
			},
			"required":             []string{"workspace_id", "broadcast_id", "when"},
			"additionalProperties": false,
		},
		Handler: s.scheduleBroadcast,
	}
}

type scheduleBroadcastOutput struct {
	Workspace   WorkspaceRef `json:"workspace"`
	BroadcastID string       `json:"broadcast_id"`
	Status      string       `json:"status"`
	ScheduledAt *time.Time   `json:"scheduled_at"`
}

// nowFunc is the clock schedule_broadcast compares a time with.
var nowFunc = time.Now

func (s *Server) scheduleBroadcast(ctx context.Context, call *Call) (any, error) {
	id, toolErr := broadcastIDArgument(call)
	if toolErr != nil {
		return nil, toolErr
	}
	when, _ := call.Args["when"].(string)
	fields := map[string]any{"workspace_id": call.WorkspaceID, "id": id}
	var scheduledAt *time.Time
	if when == "now" {
		fields["send_now"] = true
	} else {
		at, err := time.Parse(time.RFC3339, when)
		if err != nil {
			return nil, validationError("when must be \"now\" or an RFC 3339 time, such as 2026-12-01T09:00:00Z")
		}
		// The request takes a local date, a minute and an IANA zone; an offset
		// names no zone, so the time is sent in UTC. The seconds are dropped
		// before the comparison, so a time later in the current minute is past.
		at = at.UTC().Truncate(time.Minute)
		if !at.After(nowFunc()) {
			return nil, validationError("when must be in a later minute than now, since the seconds are dropped: give a later time, or \"now\"")
		}
		scheduledAt = &at
		fields["send_now"] = false
		fields["scheduled_date"] = at.Format("2006-01-02")
		fields["scheduled_time"] = at.Format("15:04")
		fields["timezone"] = "UTC"
	}
	call.Audit.AddMetadata("when", when)
	if toolErr := requireMarketingProvider(call.Workspace); toolErr != nil {
		return nil, toolErr
	}
	var req domain.ScheduleBroadcastRequest
	if err := requestFromJSON(fields, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	if err := s.deps.Broadcasts.ScheduleBroadcast(ctx, &req); err != nil {
		return nil, err
	}
	s.auditBroadcastAudience(ctx, call, id)
	status := domain.BroadcastStatusScheduled
	if req.SendNow {
		status = domain.BroadcastStatusProcessing
	}
	return scheduleBroadcastOutput{Workspace: call.WorkspaceRef(), BroadcastID: id, Status: string(status), ScheduledAt: scheduledAt}, nil
}

func (s *Server) selectBroadcastWinnerTool() *Tool {
	schema := broadcastIDSchema()
	schema["properties"].(map[string]any)["template_id"] = map[string]any{
		"type": "string", "description": "Template of the winning variation, as get_broadcast_report shows it.",
	}
	schema["required"] = []string{"workspace_id", "broadcast_id", "template_id"}
	return &Tool{
		Name:  "select_broadcast_winner",
		Title: "Select A/B test winner",
		Description: "Picks the winning template of a broadcast's A/B test and sends it to the rest of the audience. " +
			"This emails every contact who did not get the test, so confirm the winner with the user first; get_broadcast_report compares the variations. " +
			"It works once the test is over, or while it runs when the winner is not picked automatically. " +
			"Calling it again with the same template changes nothing more; another template after a winner answers invalid_state.",
		Class:       ClassSend,
		Idempotent:  boolPtr(true),
		Pairs:       []Pair{broadcastsWrite},
		AuditAction: "broadcasts.selectWinner",
		InputSchema: schema,
		Handler:     s.selectBroadcastWinner,
	}
}

type selectBroadcastWinnerOutput struct {
	Workspace       WorkspaceRef `json:"workspace"`
	BroadcastID     string       `json:"broadcast_id"`
	WinningTemplate string       `json:"winning_template"`
	Status          string       `json:"status"`
}

func (s *Server) selectBroadcastWinner(ctx context.Context, call *Call) (any, error) {
	id, toolErr := broadcastIDArgument(call)
	if toolErr != nil {
		return nil, toolErr
	}
	templateID, _ := call.Args["template_id"].(string)
	call.Audit.AddMetadata("template_id", templateID)
	req := domain.SelectWinnerRequest{WorkspaceID: call.WorkspaceID, ID: id, TemplateID: templateID}
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	if err := s.deps.Broadcasts.SelectWinner(ctx, call.WorkspaceID, id, templateID); err != nil {
		return nil, err
	}
	s.auditBroadcastAudience(ctx, call, id)
	return selectBroadcastWinnerOutput{
		Workspace: call.WorkspaceRef(), BroadcastID: id, WinningTemplate: templateID, Status: string(domain.BroadcastStatusWinnerSelected),
	}, nil
}

// broadcastStateOutput is what pause_broadcast, resume_broadcast and
// cancel_broadcast return. Already is true when the broadcast was in the
// target state before the call.
type broadcastStateOutput struct {
	Workspace   WorkspaceRef `json:"workspace"`
	BroadcastID string       `json:"broadcast_id"`
	Status      string       `json:"status"`
	Already     bool         `json:"already"`
}

// alreadyIn reports whether err refuses an action because the broadcast is
// already in status: the status the service read under the row's lock.
func alreadyIn(err error, status domain.BroadcastStatus) bool {
	var invalid *domain.InvalidStateError
	return errors.As(err, &invalid) && invalid.Status == string(status)
}

func (s *Server) pauseBroadcastTool() *Tool {
	return &Tool{
		Name:  "pause_broadcast",
		Title: "Pause broadcast",
		Description: "Pauses a broadcast that is sending: emails not yet sent wait until resume_broadcast. " +
			"Emails already sent are not recalled. " +
			"A broadcast already paused answers success with already set to true, so calling it again is safe. " +
			"A broadcast that is not sending, such as a draft or a scheduled one, answers invalid_state: use cancel_broadcast for a scheduled one.",
		Class:       ClassWrite,
		Idempotent:  boolPtr(true),
		Pairs:       []Pair{broadcastsWrite},
		AuditAction: "broadcasts.pause",
		InputSchema: broadcastIDSchema(),
		Handler:     s.pauseBroadcast,
	}
}

func (s *Server) pauseBroadcast(ctx context.Context, call *Call) (any, error) {
	id, toolErr := broadcastIDArgument(call)
	if toolErr != nil {
		return nil, toolErr
	}
	var req domain.PauseBroadcastRequest
	if err := requestFromJSON(map[string]any{"workspace_id": call.WorkspaceID, "id": id}, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	out := broadcastStateOutput{Workspace: call.WorkspaceRef(), BroadcastID: id, Status: string(domain.BroadcastStatusPaused)}
	err := s.deps.Broadcasts.PauseBroadcast(ctx, &req)
	switch {
	case alreadyIn(err, domain.BroadcastStatusPaused):
		out.Already = true
		call.Audit.AddMetadata("already", true)
	case err != nil:
		return nil, err
	}
	return out, nil
}

func (s *Server) resumeBroadcastTool() *Tool {
	return &Tool{
		Name:  "resume_broadcast",
		Title: "Resume broadcast",
		Description: "Resumes a paused broadcast: it sends the emails it had left to its audience, or waits for its scheduled time when that is still ahead. " +
			"This emails contacts, so confirm with the user first. " +
			"status is where the broadcast went back to. " +
			"Only a paused broadcast can be resumed: calling it again answers invalid_state with the broadcast's status, and sends nothing more.",
		Class:       ClassSend,
		Idempotent:  boolPtr(true),
		Pairs:       []Pair{broadcastsWrite},
		AuditAction: "broadcasts.resume",
		InputSchema: broadcastIDSchema(),
		Handler:     s.resumeBroadcast,
	}
}

func (s *Server) resumeBroadcast(ctx context.Context, call *Call) (any, error) {
	id, toolErr := broadcastIDArgument(call)
	if toolErr != nil {
		return nil, toolErr
	}
	if toolErr := requireMarketingProvider(call.Workspace); toolErr != nil {
		return nil, toolErr
	}
	var req domain.ResumeBroadcastRequest
	if err := requestFromJSON(map[string]any{"workspace_id": call.WorkspaceID, "id": id}, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if err := s.deps.Broadcasts.ResumeBroadcast(ctx, &req); err != nil {
		return nil, err
	}
	// The service returns no status: the broadcast is read again for it.
	out := broadcastStateOutput{Workspace: call.WorkspaceRef(), BroadcastID: id}
	if broadcast := s.auditBroadcastAudience(ctx, call, id); broadcast != nil {
		out.Status = string(broadcast.Status)
	}
	return out, nil
}

func (s *Server) cancelBroadcastTool() *Tool {
	return &Tool{
		Name:  "cancel_broadcast",
		Title: "Cancel broadcast",
		Description: "Cancels a scheduled, paused or sending broadcast for good: emails not yet sent are dropped, and it cannot be resumed or scheduled again. " +
			"Emails already sent are not recalled. This cannot be undone, so confirm with the user first. " +
			"A broadcast already cancelled answers success with already set to true, so calling it again is safe. " +
			"A draft cannot be cancelled: it answers invalid_state.",
		Class:       ClassDestructive,
		Pairs:       []Pair{broadcastsWrite},
		AuditAction: "broadcasts.cancel",
		InputSchema: broadcastIDSchema(),
		Handler:     s.cancelBroadcast,
	}
}

func (s *Server) cancelBroadcast(ctx context.Context, call *Call) (any, error) {
	id, toolErr := broadcastIDArgument(call)
	if toolErr != nil {
		return nil, toolErr
	}
	var req domain.CancelBroadcastRequest
	if err := requestFromJSON(map[string]any{"workspace_id": call.WorkspaceID, "id": id}, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	out := broadcastStateOutput{Workspace: call.WorkspaceRef(), BroadcastID: id, Status: string(domain.BroadcastStatusCancelled)}
	err := s.deps.Broadcasts.CancelBroadcast(ctx, &req)
	switch {
	case alreadyIn(err, domain.BroadcastStatusCancelled):
		out.Already = true
		call.Audit.AddMetadata("already", true)
	case err != nil:
		return nil, err
	}
	return out, nil
}

// undeletableBroadcastStatuses are the statuses of a broadcast whose task can
// still queue emails or whose queued emails wait for a resume: deleting it
// touches neither, so emails could still go out for a broadcast nobody can
// see, or wait forever. Cancelling it first drops them.
var undeletableBroadcastStatuses = []domain.BroadcastStatus{
	domain.BroadcastStatusProcessing,
	domain.BroadcastStatusPaused,
	domain.BroadcastStatusTesting,
	domain.BroadcastStatusTestCompleted,
	domain.BroadcastStatusWinnerSelected,
}

func (s *Server) deleteBroadcastTool() *Tool {
	return &Tool{
		Name:  "delete_broadcast",
		Title: "Delete broadcast",
		Description: "Deletes a broadcast for good. Messages already sent keep their history. This cannot be undone, so confirm with the user first. " +
			"A broadcast that is sending, paused or in an A/B test answers invalid_state, and so does one whose queue still holds emails to send: cancel it with cancel_broadcast first. " +
			"Returns not_found for a broadcast that does not exist, which is also what a repeated call answers.",
		Class:       ClassDestructive,
		Pairs:       []Pair{broadcastsWrite},
		AuditAction: "broadcasts.delete",
		InputSchema: broadcastIDSchema(),
		Handler:     s.deleteBroadcast,
	}
}

type deleteBroadcastOutput struct {
	Workspace   WorkspaceRef `json:"workspace"`
	Deleted     bool         `json:"deleted"`
	BroadcastID string       `json:"broadcast_id"`
}

func (s *Server) deleteBroadcast(ctx context.Context, call *Call) (any, error) {
	id, toolErr := broadcastIDArgument(call)
	if toolErr != nil {
		return nil, toolErr
	}
	broadcast, err := s.deps.BroadcastRows.GetBroadcast(ctx, call.WorkspaceID, id)
	if err != nil {
		return nil, err
	}
	if broadcast == nil {
		return nil, &domain.ErrBroadcastNotFound{ID: id}
	}
	call.Audit.SetTarget("", "", broadcast.Name)
	status := string(broadcast.Status)
	if slices.Contains(undeletableBroadcastStatuses, broadcast.Status) {
		return nil, &ToolError{
			Code:    CodeInvalidState,
			Message: fmt.Sprintf("broadcast %s is %s: cancel it with cancel_broadcast before deleting it", id, status),
			Status:  status,
		}
	}
	// A broadcast past its sending, or one that failed, can still have emails
	// in its queue, which the worker would send after the broadcast is gone.
	counts, err := s.deps.Queue.GetSourceCounts(ctx, call.WorkspaceID, domain.EmailQueueSourceBroadcast, id)
	if err != nil {
		return nil, err
	}
	if counts == nil {
		counts = &domain.EmailQueueSourceCounts{}
	}
	if waiting := counts.Pending + counts.Processing + counts.Paused + counts.FailedRetrying; waiting > 0 {
		return nil, &ToolError{
			Code:    CodeInvalidState,
			Message: fmt.Sprintf("broadcast %s still has %d emails in its queue: cancel it with cancel_broadcast, or wait until they are sent, before deleting it", id, waiting),
			Status:  status,
		}
	}

	var req domain.DeleteBroadcastRequest
	if err := requestFromJSON(map[string]any{"workspace_id": call.WorkspaceID, "id": id}, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if err := s.deps.Broadcasts.DeleteBroadcast(ctx, &req); err != nil {
		return nil, err
	}
	return deleteBroadcastOutput{Workspace: call.WorkspaceRef(), Deleted: true, BroadcastID: id}, nil
}
