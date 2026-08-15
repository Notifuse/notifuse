package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/domain"
)

// Demo annotations.
//
// Two kinds, and both describe something the generated data actually shows. An
// annotation is a claim about a chart: a vertical marker on a flat stretch of
// traffic does not teach the feature, it teaches the reader to distrust it. So
// there is exactly one editorial annotation here — the iPhone launch, the only
// narrative event the traffic generator models — and no invented outage,
// redesign or promo to go with it.
//
// The second kind is the automatic broadcast annotation, seeded by hand because
// the demo never takes the path that writes it. See seedBroadcastAnnotations.

const (
	demoLaunchAnnotationTitle = "iPhone 17 & iPhone Air launch"

	// One of the console's colour presets. Green reads as a good day, which is
	// what a 2.5x traffic spike is.
	demoLaunchAnnotationColor = "#22c55e"
)

// seedAnnotations writes the demo workspace's annotations.
//
// now is the seeding run's clock, passed in rather than read here so the launch
// annotation and the traffic spike are placed against the same day.
func (s *DemoService) seedAnnotations(ctx context.Context, workspaceID string, now time.Time) error {
	if s.annotationRepo == nil {
		return fmt.Errorf("annotation repository is not configured")
	}

	timezone := s.demoAnnotationTimezone(ctx, workspaceID)

	// Both halves run whatever the other does, and the errors are joined for the
	// caller to log: losing the launch marker is no reason to also lose the four
	// broadcast markers. The caller treats the result as non-fatal — a demo
	// without annotations is a smaller loss than a demo that failed to seed.
	var errs []error
	if err := s.seedLaunchAnnotation(ctx, workspaceID, timezone, now); err != nil {
		errs = append(errs, err)
	}
	if err := s.seedBroadcastAnnotations(ctx, workspaceID, timezone); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// seedLaunchAnnotation marks the one event the demo's traffic really contains.
func (s *DemoService) seedLaunchAnnotation(
	ctx context.Context,
	workspaceID string,
	timezone string,
	now time.Time,
) error {
	annotation := &domain.Annotation{
		ID:          strings.ReplaceAll(uuid.New().String(), "-", ""),
		AnnotatedAt: demoLaunchDay(now),
		Timezone:    timezone,
		Title:       demoLaunchAnnotationTitle,
		Description: demoLaunchAnnotationDescription(),
		Color:       demoLaunchAnnotationColor,
		// Typed-by-an-operator is the honest source: nothing on the platform
		// produced this row, and a manual row claims no source_id, so it can never
		// collide with an automatic one.
		Source:   domain.AnnotationSourceManual,
		SourceID: nil,
	}

	if err := annotation.Validate(); err != nil {
		return fmt.Errorf("invalid demo launch annotation: %w", err)
	}

	if err := s.annotationRepo.Create(ctx, workspaceID, annotation); err != nil {
		return fmt.Errorf("failed to create the demo launch annotation: %w", err)
	}

	return nil
}

// demoLaunchDay returns midnight UTC of the demo's launch day for a run whose
// clock reads now.
//
// The coupling to demoLaunchDaysAgo is the whole point. The traffic generator
// places its spike at day index Days-1-demoLaunchDaysAgo of a window that ends
// on today, which is the same instant this expression produces — so raising or
// lowering the constant moves the annotation with the spike. A hardcoded date,
// or a second arithmetic that happens to agree today, would drift the marker off
// the spike the first time the constant changed, silently and only on the chart.
// TestDemoLaunchAnnotation pins the two against each other.
func demoLaunchDay(now time.Time) time.Time {
	return now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -demoLaunchDaysAgo)
}

// demoLaunchAnnotationDescription says what the chart shows on that day. Both
// the multipliers and the page list are read from the generator's own constants
// rather than written out, so the text cannot go on claiming a 2.5x spike after
// somebody tunes it down, nor keep naming pages the launch no longer covers.
//
// It describes the acquisition mix by direction, not by percentage: the split
// falls out of demoNoUTMWeight, the campaign weights and two probabilities in
// pickAcquisition/pickReferrer, so any figure quoted here would be a fourth
// place to keep in step and the first to rot.
func demoLaunchAnnotationDescription() string {
	paths := make([]string, 0, len(demoLaunchPages))
	for _, page := range demoLaunchPages {
		paths = append(paths, page.Path)
	}
	sort.Strings(paths)

	return fmt.Sprintf(
		"Keynote day. Sessions run about %.1fx a normal day and stay near %.1fx for the following %d days. "+
			"The surge lands on the launch pages (%s), a large share of it carrying the iphone-launch-2024 "+
			"campaign, with tech-press referrals well above their usual share.",
		demoLaunchDayFactor, demoPostLaunchFactor, demoPostLaunchDays, strings.Join(paths, ", "),
	)
}

// seedBroadcastAnnotations writes the annotation every demo broadcast would
// already have if it had been sent for real.
//
// A live broadcast is annotated by the annotation service, from the
// broadcast.sending_started event. The demo never publishes that event: it
// creates its broadcasts and then writes them straight to Processed with a
// backdated started_at, so nothing on the platform ever observes a send. Without
// this deliberate second insert the demo would show the manual launch marker and
// none of the automatic ones — exactly the half of the feature a prospect is
// least likely to guess at.
//
// It writes through CreateFromSource, like the real handler, so the rows carry
// the same (source, source_id) idempotency: re-running the seed against a
// workspace that already has them adds nothing.
func (s *DemoService) seedBroadcastAnnotations(ctx context.Context, workspaceID, timezone string) error {
	if s.broadcastRepo == nil {
		return fmt.Errorf("broadcast repository is not configured")
	}

	response, err := s.broadcastRepo.ListBroadcasts(ctx, domain.ListBroadcastsParams{
		WorkspaceID: workspaceID,
		Limit:       demoBroadcastAnnotationLimit,
	})
	if err != nil {
		return fmt.Errorf("failed to list demo broadcasts for annotations: %w", err)
	}
	if response == nil {
		return nil
	}

	for _, broadcast := range response.Broadcasts {
		if broadcast == nil {
			continue
		}
		// A broadcast that never started has no instant to mark. The real handler
		// falls back to the event time; here there is no event, and inventing "now"
		// would put a send marker on a day nothing was sent.
		if broadcast.StartedAt == nil {
			continue
		}

		sourceID := broadcast.ID
		annotation := &domain.Annotation{
			ID:          strings.ReplaceAll(uuid.New().String(), "-", ""),
			AnnotatedAt: *broadcast.StartedAt,
			Timezone:    timezone,
			Title:       demoAnnotationTitle(broadcast.Name),
			Color:       domain.AnnotationBroadcastColor,
			Source:      domain.AnnotationSourceBroadcast,
			SourceID:    &sourceID,
		}

		if err := annotation.Validate(); err != nil {
			s.logger.WithField("workspace_id", workspaceID).
				WithField("broadcast_id", broadcast.ID).
				WithField("error", err.Error()).
				Warn("Skipped demo broadcast annotation: invalid annotation")
			continue
		}

		// Logged and skipped rather than returned: one unannotated broadcast must
		// not cost the demo the other three.
		if _, err := s.annotationRepo.CreateFromSource(ctx, workspaceID, annotation); err != nil {
			s.logger.WithField("workspace_id", workspaceID).
				WithField("broadcast_id", broadcast.ID).
				WithField("error", err.Error()).
				Warn("Failed to annotate demo broadcast")
		}
	}

	return nil
}

// demoBroadcastAnnotationLimit bounds the broadcast listing. The demo creates a
// handful; the limit is only here so a workspace that somehow holds more does
// not turn a seeding step into an unbounded read.
const demoBroadcastAnnotationLimit = 100

// demoAnnotationTitle fits a broadcast name into the title column.
//
// Runes, not bytes: title is VARCHAR(100) characters, so a byte slice would both
// over-truncate a non-ASCII name and be able to cut a multi-byte rune in half.
func demoAnnotationTitle(name string) string {
	if name == "" {
		// Same fallback as the live handler: an unnamed broadcast still deserves its
		// vertical, and a blank title would fail validation and lose the row.
		return "Broadcast"
	}
	if runes := []rune(name); len(runes) > domain.AnnotationMaxTitleLength {
		return string(runes[:domain.AnnotationMaxTitleLength])
	}
	return name
}

// demoAnnotationTimezone resolves the display timezone the same way the
// annotation service does: the workspace's own, else UTC. A lookup failure is
// not fatal — timezone is display intent only, and an annotation stored as UTC
// is worth far more than no annotation.
func (s *DemoService) demoAnnotationTimezone(ctx context.Context, workspaceID string) string {
	if s.workspaceRepo == nil {
		return "UTC"
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil || workspace == nil {
		return "UTC"
	}
	if !domain.IsValidTimezone(workspace.Settings.Timezone) {
		return "UTC"
	}

	return workspace.Settings.Timezone
}
