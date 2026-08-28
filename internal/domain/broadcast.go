package domain

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

//go:generate mockgen -destination mocks/mock_broadcast_service.go -package mocks github.com/Notifuse/notifuse/internal/domain BroadcastService
//go:generate mockgen -destination mocks/mock_broadcast_sender.go -package mocks github.com/Notifuse/notifuse/internal/domain BroadcastSender
//go:generate mockgen -destination mocks/mock_broadcast_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain BroadcastRepository

// BroadcastStatus defines the current status of a broadcast
type BroadcastStatus string

const (
	BroadcastStatusDraft          BroadcastStatus = "draft"
	BroadcastStatusScheduled      BroadcastStatus = "scheduled"
	BroadcastStatusProcessing     BroadcastStatus = "processing" // Orchestrator is enqueueing emails
	BroadcastStatusPaused         BroadcastStatus = "paused"
	BroadcastStatusProcessed      BroadcastStatus = "processed" // Enqueueing complete
	BroadcastStatusCancelled      BroadcastStatus = "cancelled"
	BroadcastStatusFailed         BroadcastStatus = "failed"
	BroadcastStatusTesting        BroadcastStatus = "testing"         // A/B test in progress
	BroadcastStatusTestCompleted  BroadcastStatus = "test_completed"  // Test done, awaiting winner selection
	BroadcastStatusWinnerSelected BroadcastStatus = "winner_selected" // Winner chosen, enqueueing to remaining
)

// IsValid reports whether s is one of the known broadcast statuses.
func (s BroadcastStatus) IsValid() bool {
	switch s {
	case BroadcastStatusDraft, BroadcastStatusScheduled, BroadcastStatusProcessing,
		BroadcastStatusPaused, BroadcastStatusProcessed, BroadcastStatusCancelled,
		BroadcastStatusFailed, BroadcastStatusTesting, BroadcastStatusTestCompleted,
		BroadcastStatusWinnerSelected:
		return true
	default:
		return false
	}
}

// TestWinnerMetric defines the metric used to determine the winning A/B test variation
type TestWinnerMetric string

const (
	TestWinnerMetricOpenRate  TestWinnerMetric = "open_rate"
	TestWinnerMetricClickRate TestWinnerMetric = "click_rate"
)

// BroadcastTestSettings contains configuration for A/B testing
type BroadcastTestSettings struct {
	Enabled              bool                 `json:"enabled"`
	SamplePercentage     int                  `json:"sample_percentage"`
	AutoSendWinner       bool                 `json:"auto_send_winner"`
	AutoSendWinnerMetric TestWinnerMetric     `json:"auto_send_winner_metric,omitempty"`
	TestDurationHours    int                  `json:"test_duration_hours,omitempty"`
	Variations           []BroadcastVariation `json:"variations"`
}

// Value implements the driver.Valuer interface for database serialization
func (b BroadcastTestSettings) Value() (driver.Value, error) {
	return json.Marshal(b)
}

// MarshalJSON implements custom JSON marshaling to ensure Variations is never null
func (b BroadcastTestSettings) MarshalJSON() ([]byte, error) {
	type Alias BroadcastTestSettings
	// Ensure Variations is an empty array, not null
	if b.Variations == nil {
		b.Variations = []BroadcastVariation{}
	}
	return json.Marshal((*Alias)(&b))
}

// Scan implements the sql.Scanner interface for database deserialization
func (b *BroadcastTestSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	v, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(v)
	if err := json.Unmarshal(cloned, b); err != nil {
		return err
	}

	// Ensure Variations is never nil to prevent frontend crashes
	if b.Variations == nil {
		b.Variations = []BroadcastVariation{}
	}

	return nil
}

// BroadcastVariation represents a single variation in an A/B test
type BroadcastVariation struct {
	VariationName string            `json:"variation_name"`
	TemplateID    string            `json:"template_id"`
	Metrics       *VariationMetrics `json:"metrics,omitempty"`
	// joined servers-side
	Template *Template `json:"template,omitempty"`
}

// Value implements the driver.Valuer interface for database serialization
func (v BroadcastVariation) Value() (driver.Value, error) {
	return json.Marshal(v)
}

// Scan implements the sql.Scanner interface for database deserialization
func (v *BroadcastVariation) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(b)
	return json.Unmarshal(cloned, v)
}

// VariationMetrics contains performance metrics for a variation
type VariationMetrics struct {
	Recipients   int `json:"recipients"`
	Delivered    int `json:"delivered"`
	Opens        int `json:"opens"`
	Clicks       int `json:"clicks"`
	Bounced      int `json:"bounced"`
	Complained   int `json:"complained"`
	Unsubscribed int `json:"unsubscribed"`
}

// Value implements the driver.Valuer interface for database serialization
func (m VariationMetrics) Value() (driver.Value, error) {
	return json.Marshal(m)
}

// Scan implements the sql.Scanner interface for database deserialization
func (m *VariationMetrics) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(b)
	return json.Unmarshal(cloned, m)
}

// AudienceSettings defines how recipients are determined for a broadcast
type AudienceSettings struct {
	List                string   `json:"list,omitempty"`
	Segments            []string `json:"segments,omitempty"`
	ExcludeUnsubscribed bool     `json:"exclude_unsubscribed"`
}

// Value implements the driver.Valuer interface for database serialization
func (a AudienceSettings) Value() (driver.Value, error) {
	return json.Marshal(a)
}

// Scan implements the sql.Scanner interface for database deserialization
func (a *AudienceSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(b)
	return json.Unmarshal(cloned, a)
}

// ScheduleSettings defines when a broadcast will be sent
type ScheduleSettings struct {
	IsScheduled          bool   `json:"is_scheduled"`
	ScheduledDate        string `json:"scheduled_date,omitempty"` // Format: YYYY-MM-dd
	ScheduledTime        string `json:"scheduled_time,omitempty"` // Format: HH:mm
	Timezone             string `json:"timezone,omitempty"`       // IANA timezone format, e.g. "America/New_York"
	UseRecipientTimezone bool   `json:"use_recipient_timezone"`
}

// Value implements the driver.Valuer interface for database serialization
func (s ScheduleSettings) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan implements the sql.Scanner interface for database deserialization
func (s *ScheduleSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(b)
	return json.Unmarshal(cloned, s)
}

// ParseScheduledDateTime parses the ScheduledDate and ScheduledTime fields and returns a time.Time
func (s *ScheduleSettings) ParseScheduledDateTime() (time.Time, error) {
	if s.ScheduledDate == "" || s.ScheduledTime == "" {
		return time.Time{}, nil
	}

	datetime := fmt.Sprintf("%s %s", s.ScheduledDate, s.ScheduledTime)
	var t time.Time
	var err error

	if s.Timezone == "" {
		t, err = time.Parse("2006-01-02 15:04", datetime)
		if err != nil {
			return time.Time{}, err
		}
	} else {
		loc, err := time.LoadLocation(s.Timezone)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid timezone: %s", err)
		}

		t, err = time.ParseInLocation("2006-01-02 15:04", datetime, loc)
		if err != nil {
			return time.Time{}, err
		}
	}

	return t, nil
}

// SetScheduledDateTime formats a time.Time as ScheduledDate and ScheduledTime strings
func (s *ScheduleSettings) SetScheduledDateTime(t time.Time, timezone string) error {
	if t.IsZero() {
		s.ScheduledDate = ""
		s.ScheduledTime = ""
		s.Timezone = ""
		return nil
	}

	// If timezone is provided, convert time to that timezone
	if timezone != "" {
		loc, err := time.LoadLocation(timezone)
		if err != nil {
			return fmt.Errorf("invalid timezone: %s", err)
		}
		t = t.In(loc)
		s.Timezone = timezone
	}

	s.ScheduledDate = t.Format("2006-01-02")
	s.ScheduledTime = t.Format("15:04")
	return nil
}

// Broadcast represents a broadcast message campaign
type Broadcast struct {
	ID                        string                `json:"id"`
	WorkspaceID               string                `json:"workspace_id"`
	Name                      string                `json:"name"`
	ChannelType               string                `json:"channel_type"` // email, sms, push, etc.
	Status                    BroadcastStatus       `json:"status"`       // pending, sending, completed, failed
	Audience                  AudienceSettings      `json:"audience"`
	Schedule                  ScheduleSettings      `json:"schedule"`
	TestSettings              BroadcastTestSettings `json:"test_settings"`
	UTMParameters             *UTMParameters        `json:"utm_parameters,omitempty"`
	Metadata                  MapOfAny              `json:"metadata,omitempty"`
	WinningTemplate           *string               `json:"winning_template,omitempty"`
	TestSentAt                *time.Time            `json:"test_sent_at,omitempty"`
	WinnerSentAt              *time.Time            `json:"winner_sent_at,omitempty"`
	TestPhaseRecipientCount   int                   `json:"test_phase_recipient_count"`
	WinnerPhaseRecipientCount int                   `json:"winner_phase_recipient_count"`
	EnqueuedCount             int                   `json:"enqueued_count"` // Emails added to queue
	CreatedAt                 time.Time             `json:"created_at"`
	UpdatedAt                 time.Time             `json:"updated_at"`
	StartedAt                 *time.Time            `json:"started_at,omitempty"`
	CompletedAt               *time.Time            `json:"completed_at,omitempty"`
	CancelledAt               *time.Time            `json:"cancelled_at,omitempty"`
	PausedAt                  *time.Time            `json:"paused_at,omitempty"`
	PauseReason               *string               `json:"pause_reason,omitempty"`

	// Data feed settings (global and recipient feeds)
	DataFeed *DataFeedSettings `json:"data_feed,omitempty"`
}

// UTMParameters contains UTM tracking parameters for the broadcast
type UTMParameters struct {
	Source   string `json:"source,omitempty"`
	Medium   string `json:"medium,omitempty"`
	Campaign string `json:"campaign,omitempty"`
	Term     string `json:"term,omitempty"`
	Content  string `json:"content,omitempty"`
}

// Value implements the driver.Valuer interface for database serialization
func (u UTMParameters) Value() (driver.Value, error) {
	return json.Marshal(u)
}

// Scan implements the sql.Scanner interface for database deserialization
func (u *UTMParameters) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(b)
	return json.Unmarshal(cloned, u)
}

// Validate validates the broadcast struct
func (b *Broadcast) Validate() error {
	if b.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if b.Name == "" {
		return fmt.Errorf("name is required")
	}

	if len(b.Name) > 255 {
		return fmt.Errorf("name must be less than 255 characters")
	}

	// Validate status
	if !b.Status.IsValid() {
		return fmt.Errorf("invalid broadcast status: %s", b.Status)
	}

	// Validate test settings if enabled
	if b.TestSettings.Enabled {
		if b.TestSettings.SamplePercentage <= 0 || b.TestSettings.SamplePercentage > 100 {
			return fmt.Errorf("test sample percentage must be between 1 and 100")
		}

		if len(b.TestSettings.Variations) < 2 {
			return fmt.Errorf("at least 2 variations are required for A/B testing")
		}

		if len(b.TestSettings.Variations) > 8 {
			return fmt.Errorf("maximum 8 variations are allowed for A/B testing")
		}

		if b.TestSettings.AutoSendWinner {
			if b.TestSettings.TestDurationHours <= 0 {
				return fmt.Errorf("test duration must be greater than 0 hours when auto winner is enabled")
			}

			if b.TestSettings.TestDurationHours > 168 { // 7 days max
				return fmt.Errorf("test duration cannot exceed 168 hours (7 days)")
			}

			// Validate that winner metric is set
			if b.TestSettings.AutoSendWinnerMetric == "" {
				return fmt.Errorf("auto send winner metric must be specified when auto winner is enabled")
			}

			switch b.TestSettings.AutoSendWinnerMetric {
			case TestWinnerMetricOpenRate, TestWinnerMetricClickRate:
				// Valid metric
			default:
				return fmt.Errorf("invalid test winner metric: %s", b.TestSettings.AutoSendWinnerMetric)
			}
		}

		// Validate variations
		for i, variation := range b.TestSettings.Variations {
			if variation.TemplateID == "" {
				return fmt.Errorf("template_id is required for variation %d", i+1)
			}
		}
	}

	// Validate audience settings
	// CHANGED: List is required (for all broadcasts, not just web)
	if b.Audience.List == "" {
		return fmt.Errorf("list is required")
	}

	// Validate schedule settings
	if b.Schedule.IsScheduled && (b.Schedule.ScheduledDate == "" || b.Schedule.ScheduledTime == "") {
		return fmt.Errorf("scheduled date and time are required when not sending immediately")
	}

	if b.Schedule.IsScheduled {
		// Validate date format (YYYY-MM-DD)
		if len(b.Schedule.ScheduledDate) != 10 || b.Schedule.ScheduledDate[4] != '-' || b.Schedule.ScheduledDate[7] != '-' {
			return fmt.Errorf("scheduled date must be in YYYY-MM-DD format")
		}

		// Validate time format (HH:MM)
		if len(b.Schedule.ScheduledTime) != 5 || b.Schedule.ScheduledTime[2] != ':' {
			return fmt.Errorf("scheduled time must be in HH:MM format")
		}

		// If a timezone is specified, validate it
		if b.Schedule.Timezone != "" {
			_, err := time.LoadLocation(b.Schedule.Timezone)
			if err != nil {
				return fmt.Errorf("invalid timezone: %s", err)
			}
		}
	}

	// Validate data feed settings if present
	if b.DataFeed != nil {
		if err := b.DataFeed.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// CreateBroadcastRequest defines the request to create a new broadcast.
// Note: Scheduling must be done via the ScheduleBroadcastRequest after creation.
type CreateBroadcastRequest struct {
	WorkspaceID   string                `json:"workspace_id"`
	Name          string                `json:"name"`
	Audience      AudienceSettings      `json:"audience"`
	TestSettings  BroadcastTestSettings `json:"test_settings"`
	UTMParameters *UTMParameters        `json:"utm_parameters,omitempty"`
	Metadata      MapOfAny              `json:"metadata,omitempty"`
	DataFeed      *DataFeedSettings     `json:"data_feed,omitempty"`
}

// Validate validates the create broadcast request
func (r *CreateBroadcastRequest) Validate() (*Broadcast, error) {
	broadcast := &Broadcast{
		WorkspaceID:   r.WorkspaceID,
		Name:          r.Name,
		Status:        BroadcastStatusDraft,
		Audience:      r.Audience,
		Schedule:      ScheduleSettings{}, // Empty schedule - must use broadcasts.schedule endpoint
		TestSettings:  r.TestSettings,
		UTMParameters: r.UTMParameters,
		Metadata:      r.Metadata,
		DataFeed:      r.DataFeed,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	if err := broadcast.Validate(); err != nil {
		return nil, err
	}

	return broadcast, nil
}

// UpdateBroadcastRequest defines the request to update an existing broadcast.
//
// The endpoint is a patch: a body that omits a key leaves the stored value alone. That
// matters because every optional field here has a meaningful zero value. An absent
// test_settings would unhook the variations that carry the broadcast's templates, and
// Broadcast.Validate only inspects variations when the A/B test is enabled, so nothing
// would object. An absent schedule would clear is_scheduled, which a resume reads to
// choose between restoring the schedule and sending right now.
//
// The audience is a patch too, key by key rather than as a whole. Broadcast.Validate
// looks only at its list, so a body naming just that one — a supported way to retarget a
// broadcast — passes while dropping the stored segments and resetting
// exclude_unsubscribed, which widens the next send to the whole list, unsubscribed
// contacts included.
//
// A nil pointer or map is already a faithful "absent" for utm_parameters and metadata,
// but AudienceSettings, ScheduleSettings and BroadcastTestSettings are plain structs
// whose zero value is itself a legitimate payload, so their presence is recorded at
// decode time instead.
type UpdateBroadcastRequest struct {
	WorkspaceID   string                `json:"workspace_id"`
	ID            string                `json:"id"`
	Name          string                `json:"name"`
	Audience      AudienceSettings      `json:"audience"`
	Schedule      ScheduleSettings      `json:"schedule"`
	TestSettings  BroadcastTestSettings `json:"test_settings"`
	UTMParameters *UTMParameters        `json:"utm_parameters,omitempty"`
	Metadata      MapOfAny              `json:"metadata,omitempty"`
	DataFeed      *DataFeedSettings     `json:"data_feed,omitempty"`

	// Written by UnmarshalJSON only. A request assembled in Go has no wire body to read
	// presence from, so these stay false and every field is applied — what a caller
	// filling the struct in by hand means.
	scheduleOmitted     bool
	testSettingsOmitted bool

	// The audience is recorded one key at a time: its own keys are what a client patches,
	// and each carries a separate decision about who the send reaches.
	audienceListOmitted                bool
	audienceSegmentsOmitted            bool
	audienceExcludeUnsubscribedOmitted bool
}

// UnmarshalJSON decodes the request and records which patch keys the body actually
// carried, so Validate can tell an omitted schedule, test_settings or audience key from
// one the client deliberately sent as zero.
//
// A null counts as omitted throughout: there is no schedule, no set of test settings and
// no audience key a null could have meant, so it can only be a serializer writing out an
// absent optional.
func (r *UpdateBroadcastRequest) UnmarshalJSON(data []byte) error {
	// The alias drops the methods, so this does not recurse.
	type alias UpdateBroadcastRequest
	if err := json.Unmarshal(data, (*alias)(r)); err != nil {
		return err
	}

	var sent map[string]json.RawMessage
	if err := json.Unmarshal(data, &sent); err != nil {
		return err
	}

	r.scheduleOmitted = jsonKeyOmitted(sent, "schedule")
	r.testSettingsOmitted = jsonKeyOmitted(sent, "test_settings")

	// An absent or null audience leaves no keys behind, so every one of them reads as
	// omitted and the stored audience stands whole. The decode above has already accepted
	// the value as an object or a null, so nothing else can be in there.
	var audience map[string]json.RawMessage
	if !jsonKeyOmitted(sent, "audience") {
		if err := json.Unmarshal(sent["audience"], &audience); err != nil {
			return err
		}
	}

	r.audienceListOmitted = jsonKeyOmitted(audience, "list")
	r.audienceSegmentsOmitted = jsonKeyOmitted(audience, "segments")
	r.audienceExcludeUnsubscribedOmitted = jsonKeyOmitted(audience, "exclude_unsubscribed")

	return nil
}

// Validate validates the update broadcast request
func (r *UpdateBroadcastRequest) Validate(existingBroadcast *Broadcast) (*Broadcast, error) {
	if r.WorkspaceID != existingBroadcast.WorkspaceID {
		return nil, fmt.Errorf("workspace_id cannot be changed")
	}

	if r.ID != existingBroadcast.ID {
		return nil, fmt.Errorf("broadcast id cannot be changed")
	}

	// Cannot update a broadcast that is not in draft or scheduled status
	if existingBroadcast.Status != BroadcastStatusDraft &&
		existingBroadcast.Status != BroadcastStatusScheduled &&
		existingBroadcast.Status != BroadcastStatusPaused {
		return nil, fmt.Errorf("cannot update broadcast with status: %s", existingBroadcast.Status)
	}

	// Update the existing broadcast. Name is required by Broadcast.Validate below, so an
	// omitted one is rejected rather than written through; the rest are optional and keep
	// their stored value when the request left them out — see the type comment for what
	// each zero value would otherwise destroy.
	existingBroadcast.Name = r.Name

	// The audience is merged a key at a time, because each of its keys is a separate
	// decision about who the next send reaches and only the list is one Validate would
	// object to losing. Every key stays writable: a list retargets, an empty segments
	// array widens back to the whole list, and an explicit false switches the
	// unsubscribed exclusion off.
	if !r.audienceListOmitted {
		existingBroadcast.Audience.List = r.Audience.List
	}
	if !r.audienceSegmentsOmitted {
		existingBroadcast.Audience.Segments = r.Audience.Segments
	}
	if !r.audienceExcludeUnsubscribedOmitted {
		existingBroadcast.Audience.ExcludeUnsubscribed = r.Audience.ExcludeUnsubscribed
	}

	if !r.scheduleOmitted {
		existingBroadcast.Schedule = r.Schedule
	}

	if !r.testSettingsOmitted {
		existingBroadcast.TestSettings = r.TestSettings
	}

	if r.UTMParameters != nil {
		existingBroadcast.UTMParameters = r.UTMParameters
	}

	if r.Metadata != nil {
		existingBroadcast.Metadata = r.Metadata
	}

	// Handle data_feed update - preserve fetched data if only updating settings
	if r.DataFeed != nil {
		if existingBroadcast.DataFeed == nil {
			existingBroadcast.DataFeed = r.DataFeed
		} else {
			// Preserve GlobalFeedData and GlobalFeedFetchedAt from existing broadcast:
			// the payload belongs to the broadcast, and a client editing feed settings
			// never sends it back.
			existingGlobalFeedData := existingBroadcast.DataFeed.GlobalFeedData
			existingGlobalFeedFetchedAt := existingBroadcast.DataFeed.GlobalFeedFetchedAt

			// Except on the save that switches a live feed off. A disabled feed carrying
			// data is a supported shape — that is how a client supplies its own payload
			// instead of asking for a fetch — so the send path injects the payload on the
			// strength of the data alone, and no further fetch will ever replace it. This
			// transition is the only moment the response left behind can be dropped;
			// keying on the incoming Enabled alone would instead wipe the payload of a
			// broadcast that was already in supply-your-own-data mode.
			globalFeedWasEnabled := existingBroadcast.DataFeed.GlobalFeed != nil &&
				existingBroadcast.DataFeed.GlobalFeed.Enabled
			if globalFeedWasEnabled && r.DataFeed.GlobalFeed != nil && !r.DataFeed.GlobalFeed.Enabled {
				existingGlobalFeedData = nil
				existingGlobalFeedFetchedAt = nil
			}

			// Update feed settings from request
			if r.DataFeed.GlobalFeed != nil {
				existingBroadcast.DataFeed.GlobalFeed = r.DataFeed.GlobalFeed
			}
			if r.DataFeed.RecipientFeed != nil {
				existingBroadcast.DataFeed.RecipientFeed = r.DataFeed.RecipientFeed
			}

			// Restore preserved data
			existingBroadcast.DataFeed.GlobalFeedData = existingGlobalFeedData
			existingBroadcast.DataFeed.GlobalFeedFetchedAt = existingGlobalFeedFetchedAt
		}
	}

	existingBroadcast.UpdatedAt = time.Now().UTC()

	if err := existingBroadcast.Validate(); err != nil {
		return nil, err
	}

	return existingBroadcast, nil
}

// ScheduleBroadcastRequest defines the request to schedule a broadcast
type ScheduleBroadcastRequest struct {
	WorkspaceID          string `json:"workspace_id"`
	ID                   string `json:"id"`
	SendNow              bool   `json:"send_now"`
	ScheduledDate        string `json:"scheduled_date,omitempty"`
	ScheduledTime        string `json:"scheduled_time,omitempty"`
	Timezone             string `json:"timezone,omitempty"`
	UseRecipientTimezone bool   `json:"use_recipient_timezone"`
}

// Validate validates the schedule broadcast request
func (r *ScheduleBroadcastRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.ID == "" {
		return fmt.Errorf("broadcast id is required")
	}

	if !r.SendNow {
		// If not sending now, we need scheduled date and time
		if r.ScheduledDate == "" || r.ScheduledTime == "" {
			return fmt.Errorf("scheduled_date and scheduled_time are required when not sending immediately")
		}

		// Validate date format (YYYY-MM-DD)
		if len(r.ScheduledDate) != 10 || r.ScheduledDate[4] != '-' || r.ScheduledDate[7] != '-' {
			return fmt.Errorf("scheduled date must be in YYYY-MM-DD format")
		}

		// Validate time format (HH:MM)
		if len(r.ScheduledTime) != 5 || r.ScheduledTime[2] != ':' {
			return fmt.Errorf("scheduled time must be in HH:MM format")
		}

		// If a timezone is specified, validate it
		if r.Timezone != "" {
			_, err := time.LoadLocation(r.Timezone)
			if err != nil {
				return fmt.Errorf("invalid timezone: %s", err)
			}
		}
	}

	return nil
}

// PauseBroadcastRequest defines the request to pause a sending broadcast
type PauseBroadcastRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

// Validate validates the pause broadcast request
func (r *PauseBroadcastRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.ID == "" {
		return fmt.Errorf("broadcast id is required")
	}

	return nil
}

// ResumeBroadcastRequest defines the request to resume a paused broadcast
type ResumeBroadcastRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

// Validate validates the resume broadcast request
func (r *ResumeBroadcastRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.ID == "" {
		return fmt.Errorf("broadcast id is required")
	}

	return nil
}

// CancelBroadcastRequest defines the request to cancel a scheduled broadcast
type CancelBroadcastRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

// Validate validates the cancel broadcast request
func (r *CancelBroadcastRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.ID == "" {
		return fmt.Errorf("broadcast id is required")
	}

	return nil
}

// DeleteBroadcastRequest defines the request to delete a broadcast
type DeleteBroadcastRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

// Validate validates the delete broadcast request
func (r *DeleteBroadcastRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.ID == "" {
		return fmt.Errorf("broadcast id is required")
	}

	return nil
}

// ListBroadcastsParams defines parameters for listing broadcasts with pagination
type ListBroadcastsParams struct {
	WorkspaceID string
	// Status filters by a single status. Deprecated: prefer Statuses for
	// multi-status filtering. Kept for backward compatibility.
	Status BroadcastStatus
	// Statuses filters broadcasts whose status is any of the provided values.
	// When non-empty it takes precedence over Status.
	Statuses []BroadcastStatus
	// Search is a case-insensitive substring match on the broadcast name.
	Search        string
	Limit         int
	Offset        int
	WithTemplates bool // Whether to fetch and include template details for each variation
}

// BroadcastListResponse defines the response for listing broadcasts
type BroadcastListResponse struct {
	Broadcasts []*Broadcast `json:"broadcasts"`
	TotalCount int          `json:"total_count"`
}

// SendToIndividualRequest defines the request to send a broadcast to an individual
type SendToIndividualRequest struct {
	WorkspaceID    string `json:"workspace_id"`
	BroadcastID    string `json:"broadcast_id"`
	RecipientEmail string `json:"recipient_email"`
	TemplateID     string `json:"template_id,omitempty"`
}

// Validate validates the send to individual request
func (r *SendToIndividualRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.BroadcastID == "" {
		return fmt.Errorf("broadcast_id is required")
	}

	if r.RecipientEmail == "" {
		return fmt.Errorf("recipient_email is required")
	}

	return nil
}

// GetBroadcastsRequest is used to extract query parameters for listing broadcasts
type GetBroadcastsRequest struct {
	WorkspaceID string `json:"workspace_id"`
	// Status holds the raw "status" query parameter. A single status keeps
	// backward compatibility; a comma-separated list is parsed into Statuses.
	Status string `json:"status,omitempty"`
	// Statuses is the parsed list of statuses to filter by.
	Statuses []string `json:"statuses,omitempty"`
	// Search is a case-insensitive substring match on the broadcast name.
	Search        string `json:"search,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	Offset        int    `json:"offset,omitempty"`
	WithTemplates bool   `json:"with_templates,omitempty"`
}

// FromURLParams parses URL query parameters into the request
func (r *GetBroadcastRequest) FromURLParams(values url.Values) error {
	r.WorkspaceID = values.Get("workspace_id")
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	r.ID = values.Get("id")
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}

	if withTemplatesStr := values.Get("with_templates"); withTemplatesStr != "" {
		var err error
		r.WithTemplates, err = ParseBoolParam(withTemplatesStr)
		if err != nil {
			return fmt.Errorf("invalid with_templates parameter: %w", err)
		}
	}

	return nil
}

// FromURLParams parses URL query parameters into the request
func (r *GetBroadcastsRequest) FromURLParams(values url.Values) error {
	r.WorkspaceID = values.Get("workspace_id")
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	// Support filtering by one or more statuses (comma-separated). Each value is
	// validated against the known statuses so a typo or degenerate value (e.g.
	// "?status=,," or "?status=drafty") is rejected rather than silently
	// returning an empty or unfiltered list.
	if statusParam := values.Get("status"); statusParam != "" {
		for _, s := range strings.Split(statusParam, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if !BroadcastStatus(s).IsValid() {
				return fmt.Errorf("invalid status: %s", s)
			}
			r.Statuses = append(r.Statuses, s)
		}
		if len(r.Statuses) == 0 {
			return fmt.Errorf("invalid status parameter: %s", statusParam)
		}
	}

	// Status retains the single-value form for backward compatibility; for a
	// multi-status request it stays empty so it never holds a malformed,
	// comma-joined value (Statuses carries the filter in that case).
	if len(r.Statuses) == 1 {
		r.Status = r.Statuses[0]
	}

	r.Search = strings.TrimSpace(values.Get("search"))

	if limitStr := values.Get("limit"); limitStr != "" {
		var err error
		r.Limit, err = ParseIntParam(limitStr)
		if err != nil {
			return fmt.Errorf("invalid limit parameter: %w", err)
		}
	}

	if offsetStr := values.Get("offset"); offsetStr != "" {
		var err error
		r.Offset, err = ParseIntParam(offsetStr)
		if err != nil {
			return fmt.Errorf("invalid offset parameter: %w", err)
		}
	}

	if withTemplatesStr := values.Get("with_templates"); withTemplatesStr != "" {
		var err error
		r.WithTemplates, err = ParseBoolParam(withTemplatesStr)
		if err != nil {
			return fmt.Errorf("invalid with_templates parameter: %w", err)
		}
	}

	return nil
}

// parseIntParam parses a string to an integer
func ParseIntParam(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	if err != nil {
		return 0, err
	}
	return result, nil
}

// parseBoolParam parses a string to a boolean
func ParseBoolParam(s string) (bool, error) {
	var result bool
	_, err := fmt.Sscanf(s, "%t", &result)
	if err != nil {
		return false, err
	}
	return result, nil
}

// GetBroadcastRequest represents the request to get a single broadcast
type GetBroadcastRequest struct {
	WorkspaceID   string `json:"workspace_id"`
	ID            string `json:"id"`
	WithTemplates bool   `json:"with_templates,omitempty"`
}

// SelectWinnerRequest represents the request to select a winning variation
type SelectWinnerRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
	TemplateID  string `json:"template_id"`
}

// Validate validates the select winner request
func (r *SelectWinnerRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.ID == "" {
		return fmt.Errorf("broadcast id is required")
	}
	if r.TemplateID == "" {
		return fmt.Errorf("template_id is required")
	}
	return nil
}

// GetTestResultsRequest represents the request to get A/B test results
type GetTestResultsRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

// Validate validates the get test results request
func (r *GetTestResultsRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.ID == "" {
		return fmt.Errorf("broadcast id is required")
	}
	return nil
}

// FromURLParams parses URL parameters into the request
func (r *GetTestResultsRequest) FromURLParams(values url.Values) error {
	r.WorkspaceID = values.Get("workspace_id")
	r.ID = values.Get("id")
	return nil
}

// VariationResult represents the results for a single A/B test variation
type VariationResult struct {
	TemplateID   string  `json:"template_id"`
	TemplateName string  `json:"template_name"`
	Recipients   int     `json:"recipients"` // Total sent emails (used as denominator for rate calculations)
	Delivered    int     `json:"delivered"`  // Total delivered emails (ESP-dependent, may not be available)
	Opens        int     `json:"opens"`
	Clicks       int     `json:"clicks"`
	OpenRate     float64 `json:"open_rate"`  // Opens / Recipients
	ClickRate    float64 `json:"click_rate"` // Clicks / Recipients
}

// TestResultsResponse represents the response for A/B test results
type TestResultsResponse struct {
	BroadcastID       string                      `json:"broadcast_id"`
	Status            string                      `json:"status"`
	TestStartedAt     *time.Time                  `json:"test_started_at,omitempty"`
	TestCompletedAt   *time.Time                  `json:"test_completed_at,omitempty"`
	VariationResults  map[string]*VariationResult `json:"variation_results"`
	RecommendedWinner string                      `json:"recommended_winner,omitempty"`
	WinningTemplate   string                      `json:"winning_template,omitempty"`
	IsAutoSendWinner  bool                        `json:"is_auto_send_winner"`
}

// RefreshGlobalFeedRequest defines the request to refresh global feed data
type RefreshGlobalFeedRequest struct {
	WorkspaceID string           `json:"workspace_id"`
	BroadcastID string           `json:"broadcast_id"`
	URL         string           `json:"url"`
	Headers     []DataFeedHeader `json:"headers"`
}

// Validate validates the refresh global feed request
func (r *RefreshGlobalFeedRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.BroadcastID == "" {
		return fmt.Errorf("broadcast_id is required")
	}

	if r.URL == "" {
		return fmt.Errorf("url is required")
	}

	// Basic URL format check. SSRF protection (blocking private/loopback/link-local
	// targets and re-validating redirects) is enforced at fetch time by the data-feed
	// HTTP client; see broadcast.NewDataFeedFetcher and pkg/safehttpclient.
	parsedURL, err := url.Parse(r.URL)
	if err != nil {
		return fmt.Errorf("url: invalid URL: %s", err.Error())
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("url: URL must use http or https scheme")
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("url: URL must have a host")
	}

	for _, header := range r.Headers {
		if err := header.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// RefreshGlobalFeedResponse defines the response for refresh global feed
type RefreshGlobalFeedResponse struct {
	Success   bool                   `json:"success"`
	Data      map[string]interface{} `json:"data,omitempty"`
	FetchedAt *time.Time             `json:"fetched_at,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// ErrContactNotFoundForFeed is returned when a specified contact cannot be found for feed testing
type ErrContactNotFoundForFeed struct {
	Email string
}

func (e *ErrContactNotFoundForFeed) Error() string {
	return fmt.Sprintf("Contact not found with email: %s", e.Email)
}

// TestRecipientFeedRequest defines the request to test recipient feed
type TestRecipientFeedRequest struct {
	WorkspaceID  string           `json:"workspace_id"`
	BroadcastID  string           `json:"broadcast_id"`
	ContactEmail string           `json:"contact_email,omitempty"`
	URL          string           `json:"url"`
	Headers      []DataFeedHeader `json:"headers"`
}

// Validate validates the test recipient feed request
func (r *TestRecipientFeedRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}

	if r.BroadcastID == "" {
		return fmt.Errorf("broadcast_id is required")
	}

	if r.URL == "" {
		return fmt.Errorf("url is required")
	}

	// Basic URL format check. SSRF protection (blocking private/loopback/link-local
	// targets and re-validating redirects) is enforced at fetch time by the data-feed
	// HTTP client; see broadcast.NewDataFeedFetcher and pkg/safehttpclient.
	parsedURL, err := url.Parse(r.URL)
	if err != nil {
		return fmt.Errorf("url: invalid URL: %s", err.Error())
	}
	if parsedURL.Scheme != "https" {
		return fmt.Errorf("url must use https scheme")
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("url: URL must have a host")
	}

	for _, header := range r.Headers {
		if err := header.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// TestRecipientFeedResponse defines the response for test recipient feed
type TestRecipientFeedResponse struct {
	Success   bool                   `json:"success"`
	Data      map[string]interface{} `json:"data,omitempty"`
	FetchedAt *time.Time             `json:"fetched_at,omitempty"`
	Error     string                 `json:"error,omitempty"`
	// Contact used for the test
	ContactEmail string `json:"contact_email,omitempty"`
}

// BroadcastService defines the interface for broadcast operations
type BroadcastService interface {
	// CreateBroadcast creates a new broadcast
	CreateBroadcast(ctx context.Context, request *CreateBroadcastRequest) (*Broadcast, error)

	// GetBroadcast retrieves a broadcast by ID
	GetBroadcast(ctx context.Context, workspaceID, id string) (*Broadcast, error)

	// UpdateBroadcast updates an existing broadcast
	UpdateBroadcast(ctx context.Context, request *UpdateBroadcastRequest) (*Broadcast, error)

	// ListBroadcasts retrieves a list of broadcasts with pagination
	ListBroadcasts(ctx context.Context, params ListBroadcastsParams) (*BroadcastListResponse, error)

	// ScheduleBroadcast schedules a broadcast for sending
	ScheduleBroadcast(ctx context.Context, request *ScheduleBroadcastRequest) error

	// PauseBroadcast pauses a sending broadcast
	PauseBroadcast(ctx context.Context, request *PauseBroadcastRequest) error

	// ResumeBroadcast resumes a paused broadcast
	ResumeBroadcast(ctx context.Context, request *ResumeBroadcastRequest) error

	// CancelBroadcast cancels a scheduled broadcast
	CancelBroadcast(ctx context.Context, request *CancelBroadcastRequest) error

	// DeleteBroadcast deletes a broadcast
	DeleteBroadcast(ctx context.Context, request *DeleteBroadcastRequest) error

	// SendToIndividual sends a broadcast to an individual recipient
	SendToIndividual(ctx context.Context, request *SendToIndividualRequest) error

	// GetTestResults retrieves A/B test results for a broadcast
	GetTestResults(ctx context.Context, workspaceID, broadcastID string) (*TestResultsResponse, error)

	// SelectWinner manually selects the winning variation for an A/B test
	SelectWinner(ctx context.Context, workspaceID, broadcastID, templateID string) error

	// RefreshGlobalFeed refreshes the global feed data for a broadcast
	RefreshGlobalFeed(ctx context.Context, request *RefreshGlobalFeedRequest) (*RefreshGlobalFeedResponse, error)

	// TestRecipientFeed tests the recipient feed configuration with a sample or specified contact
	TestRecipientFeed(ctx context.Context, request *TestRecipientFeedRequest) (*TestRecipientFeedResponse, error)
}

// BroadcastSender is a minimal interface needed for sending broadcasts,
// used by task processors to avoid circular dependencies
type BroadcastSender interface {
	GetBroadcast(ctx context.Context, workspaceID, broadcastID string) (*Broadcast, error)
	GetTemplateByID(ctx context.Context, workspaceID, templateID string) (*Template, error)

	// Message history tracking methods
	RecordMessageSent(ctx context.Context, workspaceID string, message *MessageHistory) error
	UpdateMessageStatus(ctx context.Context, workspaceID string, messageID string, event MessageEvent, timestamp time.Time) error
}

// BroadcastRepository defines the data access layer for broadcasts
type BroadcastRepository interface {
	CreateBroadcast(ctx context.Context, broadcast *Broadcast) error
	GetBroadcast(ctx context.Context, workspaceID, broadcastID string) (*Broadcast, error)
	UpdateBroadcast(ctx context.Context, broadcast *Broadcast) error
	DeleteBroadcast(ctx context.Context, workspaceID, broadcastID string) error
	ListBroadcasts(ctx context.Context, params ListBroadcastsParams) (*BroadcastListResponse, error)

	// Transaction management
	WithTransaction(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error

	// Transaction-aware methods
	CreateBroadcastTx(ctx context.Context, tx *sql.Tx, broadcast *Broadcast) error
	GetBroadcastTx(ctx context.Context, tx *sql.Tx, workspaceID, broadcastID string) (*Broadcast, error)
	UpdateBroadcastTx(ctx context.Context, tx *sql.Tx, broadcast *Broadcast) error
	// UpdateBroadcastStatusTx updates only status-lifecycle fields (status,
	// timestamps, pause_reason). Unlike UpdateBroadcastTx, it does not reject
	// already-terminal states — used by pause/resume/cancel flows that need to
	// transition a Processed broadcast. The service layer enforces allowed
	// transitions.
	UpdateBroadcastStatusTx(ctx context.Context, tx *sql.Tx, broadcast *Broadcast) error
	DeleteBroadcastTx(ctx context.Context, tx *sql.Tx, workspaceID, broadcastID string) error
	ListBroadcastsTx(ctx context.Context, tx *sql.Tx, params ListBroadcastsParams) (*BroadcastListResponse, error)
}

// ErrBroadcastNotFound is an error type for when a broadcast is not found
type ErrBroadcastNotFound struct {
	ID string
}

// Error returns the error message
func (e *ErrBroadcastNotFound) Error() string {
	return fmt.Sprintf("Broadcast not found with ID: %s", e.ID)
}

// SetTemplateForVariation assigns a template to a specific variation
func (b *Broadcast) SetTemplateForVariation(variationIndex int, template *Template) {
	if b == nil || variationIndex < 0 || variationIndex >= len(b.TestSettings.Variations) {
		return
	}

	b.TestSettings.Variations[variationIndex].Template = template
}
