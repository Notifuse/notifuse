package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/cache"
	"github.com/Notifuse/notifuse/pkg/logger"
)

const (
	// webBridgeMaxGoalsPerSessionPerBatch caps how many goals one session may
	// bridge in a single write. /track is public, and a single beat may carry up
	// to WebTrackMaxActions goals — without a cap, one hostile payload becomes
	// that many timeline rows, segment-queue entries and automation enrolments.
	//
	// It bounds one write, NOT a session's lifetime total: the counter lives in
	// EmitGoals and restarts on every batch, so a visit that keeps converting
	// across many flushes can exceed it overall. Sustained-rate protection is
	// the rate limiter's job; this only stops a single payload spiking.
	webBridgeMaxGoalsPerSessionPerBatch = 100

	// webBridgeMaxGoalAge refuses to bridge an old conversion. The SDK's offline
	// queue can replay a beat hours later, and automations trigger on the INSERT
	// regardless of how old occurred_at is — so without this a replayed backlog
	// sends "you just did X" emails for yesterday.
	webBridgeMaxGoalAge = 24 * time.Hour

	// webBridgeContactCacheTTL bounds how long a deleted contact keeps resolving
	// and how long a new one stays unrecognised.
	webBridgeContactCacheTTL = 60 * time.Second

	// webBridgeMaxGoalValue is the largest value custom_events.goal_value
	// (DECIMAL(15,2)) can hold.
	webBridgeMaxGoalValue = 9999999999999.99
)

// WebAnalyticsContactBridge turns verified web goals into custom_events, which
// the database triggers then surface as contact timeline entries, segment
// recomputation and automation enrolments.
//
// Goals only, never pageviews. A twelve-page session would otherwise mean twelve
// timeline rows and twelve segment recomputations per visitor, and would drown
// the contact drawer in noise. Goals are already the "meaningful action"
// abstraction on both sides.
type WebAnalyticsContactBridge struct {
	contactRepo  domain.ContactRepository
	eventRepo    domain.CustomEventRepository
	logger       logger.Logger
	nowFn        func() time.Time
	contactCache *cache.InMemoryCache
}

func NewWebAnalyticsContactBridge(
	contactRepo domain.ContactRepository,
	eventRepo domain.CustomEventRepository,
	log logger.Logger,
) *WebAnalyticsContactBridge {
	return &WebAnalyticsContactBridge{
		contactRepo:  contactRepo,
		eventRepo:    eventRepo,
		logger:       log,
		nowFn:        time.Now,
		contactCache: cache.NewInMemoryCache(5 * time.Minute),
	}
}

// EmitGoals writes one custom_events row per goal.
//
// Callers must pass only goals they have not passed before — the buffer's
// per-writer cursor does that. The repository's insert-if-absent is the durable
// backstop for everything the cursor cannot cover (a restart, an eviction, a
// second replica), not a substitute for it: without the cursor every flush would
// re-attempt every goal of the session.
// The returned set contains exactly the goals that reached the database. The
// caller advances its cursor from it: anything absent — anonymous at this beat,
// contact not found, or an insert that failed — must be retried on a later
// flush, because the SDK re-sends its whole cumulative action list and a goal
// fired before the visitor identified would otherwise never be recorded.
func (b *WebAnalyticsContactBridge) EmitGoals(ctx context.Context, workspaceID string, goals []*domain.WebGoal) map[*domain.WebGoal]bool {
	written := map[*domain.WebGoal]bool{}
	if b == nil || b.eventRepo == nil || len(goals) == 0 {
		return written
	}

	now := b.nowFn()
	events := make([]*domain.CustomEvent, 0, len(goals))
	accepted := make([]*domain.WebGoal, 0, len(goals))
	skipped := 0

	// Keyed per SESSION rather than applied to the batch as a whole: a batch
	// spanning many sessions can carry more than the cap legitimately, and
	// cutting it off there would silently drop other people's conversions — and,
	// since they are never marked written, retry them on every flush forever.
	//
	// Not persisted, so it restarts on each call — see the constant.
	perSession := map[string]int{}

	for _, goal := range goals {
		if perSession[goal.SessionID] >= webBridgeMaxGoalsPerSessionPerBatch {
			skipped++
			written[goal] = true // a hostile session must not be retried forever
			continue
		}
		if goal.ContactEmail == nil || *goal.ContactEmail == "" {
			continue // anonymous: nothing to attach it to
		}
		// Too old, or a name with nothing usable in it, can never become
		// bridgeable — mark them so they are not re-examined on every flush.
		//
		// Checked against occurredAt, the value actually stored: GoalAt is
		// skew-corrected and drifts per beat, so guarding on one while writing
		// the other let a goal pass the window and still land with an absurd
		// timestamp — which is what automations and timeline ordering key on.
		occurredAt := time.UnixMilli(goal.ClientTsMs).UTC()
		if now.Sub(occurredAt) > webBridgeMaxGoalAge || occurredAt.After(now.Add(time.Hour)) {
			skipped++
			written[goal] = true
			continue
		}
		eventName := normalizeWebGoalEventName(goal.GoalName)
		if eventName == "" {
			skipped++
			written[goal] = true
			continue
		}
		if !b.contactExists(ctx, workspaceID, *goal.ContactEmail) {
			continue // decision: link only, never create
		}

		events = append(events, buildWebGoalCustomEvent(goal, eventName, occurredAt))
		accepted = append(accepted, goal)
		perSession[goal.SessionID]++
	}

	if skipped > 0 {
		b.logger.WithField("workspace_id", workspaceID).WithField("skipped", skipped).
			Warn("Web analytics goals not bridged to the contact timeline")
	}
	if len(events) == 0 {
		return written
	}

	// Deliberately its own transaction, run after the analytics flush has
	// committed. Inside it, the timeline trigger's cascade into
	// contact_segment_queue would hold row locks for the duration of a
	// multi-session, multi-contact batch — the shape that has deadlocked this
	// queue before.
	if err := b.eventRepo.BatchInsertNew(ctx, workspaceID, events); err != nil {
		b.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).
			Error("Failed to bridge web analytics goals into the contact timeline")
		// Only the goals that could have been written are retried. `written`
		// already carries the ones rejected above — too old, unusable name, over
		// the cap — and those can never succeed, so re-examining them on every
		// later flush would be pure waste.
		return written
	}
	for _, goal := range accepted {
		written[goal] = true
	}
	return written
}

// buildWebGoalCustomEvent turns one web goal into the custom event that
// represents it.
//
// Package-level rather than a method because the demo seeder writes the same
// rows for its synthetic history, and it cannot go through the bridge: the
// bridge hangs off the ingest buffer by design (internal/app/app.go), and its
// staleness guard rejects anything older than a day, which is every row the
// demo generates. Two copies of this payload would drift, and the drift would
// only show up as a segment that matches on live traffic and not on the demo.
func buildWebGoalCustomEvent(goal *domain.WebGoal, eventName string, occurredAt time.Time) *domain.CustomEvent {
	// Client properties first, server context second: a visitor must not be able
	// to forge session_id, path or the attribution keys by sending them as goal
	// properties. Spreading them the other way round let them.
	properties := map[string]interface{}{}
	for key, value := range goal.Properties {
		properties[key] = value
	}
	for key, value := range map[string]interface{}{
		"session_id":   goal.SessionID,
		"path":         goal.Path,
		"landing_path": goal.LandingPath,
		"utm_source":   goal.UTMSource,
		"utm_medium":   goal.UTMMedium,
		"utm_campaign": goal.UTMCampaign,
		"device":       goal.Device,
		"country":      goal.Country,
	} {
		properties[key] = value
	}
	// channel, channel_group and custom_N are deliberately absent: the
	// attribution backfill rewrites those on historical rows when a workspace
	// edits its rules, and nothing would re-emit this frozen copy. Segment
	// authors would end up comparing a stale snapshot against a live dashboard.

	event := &domain.CustomEvent{
		// The web_goals primary key, so a replay is naturally idempotent.
		ExternalID: fmt.Sprintf("%s:%d:%s:%d", goal.SessionID, goal.TabID, goal.GoalName, goal.ClientTsMs),
		Email:      *goal.ContactEmail,
		EventName:  eventName,
		Properties: properties,
		// The client's own timestamp, not the skew-corrected GoalAt: skew is
		// recomputed per beat, so GoalAt drifts and would make a replay look
		// like a newer event. Bounded by the caller's staleness check.
		OccurredAt: occurredAt,
		Source:     "web_analytics",
		GoalName:   &eventName,
	}
	if goal.GoalValue > 0 {
		// web_goals.goal_value is REAL (up to ~3.4e38) but custom_events.goal_value
		// is DECIMAL(15,2), which overflows past ~1e13 and would abort the whole
		// bridge batch — losing every other contact's conversions with it.
		value := math.Min(float64(goal.GoalValue), webBridgeMaxGoalValue)
		event.GoalValue = &value
	}
	// The type comes from the site, the only party that knows whether a
	// conversion was a purchase or a lead. It was left nil until the SDK began
	// requiring one; guessing server-side would have corrupted the revenue
	// reports that key on it.
	//
	// Stamped outside the goal-value guard above on purpose: a lead has a type
	// but usually no value, and typing only the conversions with money attached
	// would leave exactly the wrong half of the funnel unsegmentable.
	goalType := goal.GoalType
	if goalType == "" {
		// Buffered before this shipped, or sent by a client that omitted it.
		goalType = domain.GoalTypeOther
	}
	event.GoalType = &goalType
	return event
}

func (b *WebAnalyticsContactBridge) contactExists(ctx context.Context, workspaceID, email string) bool {
	return webContactExists(ctx, b.contactCache, b.contactRepo, "bridge:", workspaceID, email)
}

// webContactExists answers "is this address a contact?" with a short-lived
// cache, and is deliberately not written with cache.GetOrSet.
//
// GetOrSet runs its loader while holding the cache's process-wide write lock, so
// a workspace database that is merely slow would stall identity resolution for
// every workspace on the box. It also has no way to distinguish "looked up,
// absent" from "could not look up": returning false for a transient error would
// cache that answer, dropping every identity for that address for the whole TTL
// over one blip. Only a definitive answer is cached; an error costs the identity
// on this beat alone.
func webContactExists(
	ctx context.Context,
	c *cache.InMemoryCache,
	repo domain.ContactRepository,
	keyPrefix, workspaceID, email string,
) bool {
	if repo == nil || c == nil {
		return false
	}
	key := keyPrefix + workspaceID + ":" + email
	if cached, found := c.Get(key); found {
		exists, _ := cached.(bool)
		return exists
	}

	// Outside any cache lock.
	contact, err := repo.GetContactByEmail(ctx, workspaceID, email)
	definitivelyAbsent := errors.Is(err, domain.ErrContactNotFound) || errors.Is(err, sql.ErrNoRows)
	if err != nil && !definitivelyAbsent {
		return false // transient (or a cancelled beat): do not remember it
	}
	exists := err == nil && contact != nil
	c.Set(key, exists, webBridgeContactCacheTTL)
	return exists
}

// normalizeWebGoalEventName maps an arbitrary client string onto the charset
// custom_events accepts (lowercase letters, digits, underscore, dot, slash).
//
// Returns "" when nothing usable survives, which the caller treats as a skip.
// The 100-character cap is not cosmetic: contact_timeline.kind is VARCHAR(150)
// and the trigger prefixes "custom_event.", leaving exactly 100 with no room.
func normalizeWebGoalEventName(name string) string {
	var out strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '/':
			out.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore && out.Len() > 0 {
				out.WriteRune('_')
				lastUnderscore = true
			}
		}
		if out.Len() >= domain.WebTrackMaxGoalNameLength {
			break
		}
	}
	return strings.Trim(out.String(), "_")
}
