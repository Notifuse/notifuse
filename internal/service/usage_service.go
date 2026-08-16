package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// UsageService sums the stored monthly usage snapshots across every workspace on
// this installation.
//
// It only reads snapshots — it never recounts. The recount is the maintenance
// worker's job, and keeping it out of the read path is deliberate: this service
// answers a request that reaches the public mux, and a read that triggered a
// COUNT(*) per workspace would turn every call into work proportional to a
// month of traffic. The snapshot's ComputedAt is reported instead, so a caller
// can see how stale the number is rather than paying to refresh it.
type UsageService struct {
	workspaceRepo    domain.WorkspaceRepository
	webAnalyticsRepo domain.WebAnalyticsRepository
	logger           logger.Logger
}

// NewUsageService creates the usage service.
func NewUsageService(
	workspaceRepo domain.WorkspaceRepository,
	webAnalyticsRepo domain.WebAnalyticsRepository,
	log logger.Logger,
) *UsageService {
	return &UsageService{
		workspaceRepo:    workspaceRepo,
		webAnalyticsRepo: webAnalyticsRepo,
		logger:           log,
	}
}

// GetUsage sums the stored snapshots for the given UTC months.
func (s *UsageService) GetUsage(ctx context.Context, months []time.Time) (*domain.UsageReport, error) {
	if len(months) == 0 {
		return nil, fmt.Errorf("no months requested")
	}

	workspaces, err := s.workspaceRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	// Keyed by the month's UTC instant so workspaces land in the same bucket
	// regardless of the order they come back in.
	totals := make(map[time.Time]*domain.InstanceUsage, len(months))
	for _, m := range months {
		month := time.Date(m.UTC().Year(), m.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
		totals[month] = &domain.InstanceUsage{PeriodMonth: month}
	}

	for _, workspace := range workspaces {
		usage, err := s.webAnalyticsRepo.GetUsage(ctx, workspace.ID, months)
		if err != nil {
			// Deliberately fatal, and deliberately not the "if err == nil { assign }"
			// pattern the telemetry repository uses: swallowing this would report a
			// total missing one workspace's traffic as though it were the whole
			// installation's, and the control plane cannot tell an under-report from
			// a quiet month. Failing means no usage is reported, which is the case
			// the control plane already knows to skip.
			return nil, fmt.Errorf("failed to read usage for workspace %s: %w", workspace.ID, err)
		}

		for _, u := range usage {
			total, ok := totals[u.PeriodMonth.UTC()]
			if !ok {
				// A month the caller did not ask for; the repository filters by month,
				// so this only happens if the two disagree about normalisation.
				continue
			}
			total.Pageviews += u.Pageviews
			total.TimelineEntries += u.TimelineEntries
			total.Workspaces++
			// Oldest snapshot wins: the total is only as fresh as its stalest part.
			if total.ComputedAt.IsZero() || u.ComputedAt.Before(total.ComputedAt) {
				total.ComputedAt = u.ComputedAt
			}
		}
	}

	// Returned in the order the caller asked for, so a caller reading months[0]
	// gets the month it put first.
	report := &domain.UsageReport{
		Months:         make([]*domain.InstanceUsage, 0, len(months)),
		WorkspaceCount: len(workspaces),
		GeneratedAt:    time.Now().UTC(),
	}
	for _, m := range months {
		month := time.Date(m.UTC().Year(), m.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
		if total, ok := totals[month]; ok {
			report.Months = append(report.Months, total)
		}
	}

	return report, nil
}
