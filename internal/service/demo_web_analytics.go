package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// Demo web analytics seeding.
//
// The volume is deliberate: 100 000 sessions over 400 days. The span is what
// the console's date picker needs — Year to date and Previous 12 months are
// empty below thirteen months — and the density is what makes an Explore
// drill-down three levels deep survive its default ten-session threshold.

const (
	demoWebAnalyticsSessions = 100_000
	demoWebAnalyticsDays     = 400

	// A fixed seed, so two resets produce the same demo and a screenshot taken
	// today still matches the data next month.
	demoWebAnalyticsSeed = 20260809

	demoWebAnalyticsSite = "https://www.apple.com"
)

// seedWebAnalytics fills the demo workspace with web analytics history.
//
// Sessions are generated with their channel left blank and classified by the
// workspace's own rules, so the Filters tab governs real data: editing a rule
// and running a backfill visibly changes the dashboards.
func (s *DemoService) seedWebAnalytics(ctx context.Context, workspaceID string) error {
	if s.webAnalyticsRepo == nil {
		return fmt.Errorf("web analytics repository is not configured")
	}

	filters, err := s.enableDemoWebAnalytics(ctx, workspaceID)
	if err != nil {
		return err
	}

	identities, err := s.demoContactEmails(ctx, workspaceID)
	if err != nil {
		// Identity linking is a nice-to-have; an empty list simply means every
		// visitor stays anonymous.
		s.logger.WithField("error", err.Error()).
			Warn("Failed to load demo contacts for web analytics identities")
	}

	now := time.Now().UTC()
	generator := newDemoWebAnalyticsGenerator(demoWebAnalyticsOptions{
		Sessions:   demoWebAnalyticsSessions,
		Days:       demoWebAnalyticsDays,
		Now:        now,
		Seed:       demoWebAnalyticsSeed,
		Identities: identities,
		Filters:    filters,
		SiteURL:    demoWebAnalyticsSite,
	})

	if err := s.webAnalyticsRepo.EnsureMonthlyPartitions(
		ctx, workspaceID, demoMonthsCovering(generator),
	); err != nil {
		return fmt.Errorf("failed to create demo web analytics partitions: %w", err)
	}

	started := time.Now()
	sessions, pages, goals, err := s.flushDemoWebAnalytics(ctx, workspaceID, generator)
	if err != nil {
		return err
	}

	// Fresh partitions have no statistics, so the first dashboard load would
	// otherwise plan every query against an empty table.
	s.analyzeDemoWebAnalytics(ctx, workspaceID)

	s.logger.WithField("workspace_id", workspaceID).
		WithField("sessions", sessions).
		WithField("pages", pages).
		WithField("goals", goals).
		WithField("duration", time.Since(started).String()).
		Info("Demo web analytics generated")
	return nil
}

// flushDemoWebAnalytics writes the generated rows a month at a time, newest
// first. Per-month batches keep both the transaction and peak memory bounded;
// newest first means the ranges a visitor is most likely to open are populated
// before the older history lands.
func (s *DemoService) flushDemoWebAnalytics(
	ctx context.Context,
	workspaceID string,
	generator *demoWebAnalyticsGenerator,
) (sessions, pages, goals int, err error) {
	byMonth := map[time.Time][]int{}
	for day := 0; day < generator.Days(); day++ {
		month := demoMonthOf(generator.DayStart(day))
		byMonth[month] = append(byMonth[month], day)
	}

	months := make([]time.Time, 0, len(byMonth))
	for month := range byMonth {
		months = append(months, month)
	}
	sortTimesDescending(months)

	for _, month := range months {
		batch := demoWebAnalyticsBatch{}
		for _, day := range byMonth[month] {
			day := generator.GenerateDay(day)
			batch.Sessions = append(batch.Sessions, day.Sessions...)
			batch.Pages = append(batch.Pages, day.Pages...)
			batch.Goals = append(batch.Goals, day.Goals...)
		}

		if err := s.webAnalyticsRepo.FlushBatch(
			ctx, workspaceID, batch.Sessions, batch.Pages, batch.Goals,
		); err != nil {
			return sessions, pages, goals, fmt.Errorf(
				"failed to write demo web analytics for %s: %w", month.Format("2006-01"), err)
		}

		sessions += len(batch.Sessions)
		pages += len(batch.Pages)
		goals += len(batch.Goals)
	}

	return sessions, pages, goals, nil
}

// enableDemoWebAnalytics turns the feature on and appends the product-category
// rules to the defaults the workspace was created with, returning the full rule
// set for the generator to classify against.
func (s *DemoService) enableDemoWebAnalytics(
	ctx context.Context,
	workspaceID string,
) ([]domain.WebFilter, error) {
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to load demo workspace: %w", err)
	}

	settings := workspace.Settings.WebAnalytics
	if settings == nil {
		settings = &domain.WebAnalyticsSettings{
			BounceThresholdSeconds: domain.WebAnalyticsDefaultBounceThresholdSeconds,
			Filters:                domain.DefaultWebFilters(),
		}
	}

	settings.Enabled = true
	settings.AllowedDomains = []string{"*.apple.com"}
	settings.GeoEnabled = true
	settings.GeoStoreCity = true
	settings.GeoStoreRegion = true
	settings.GeoCoordsPrecision = 2
	settings.CustomDimensionLabels = map[string]string{
		"custom_1": "Product line",
		"custom_2": "Auth state",
		"custom_3": "Experiment",
	}

	// The defaults classify most channels; these cover the ones this demo's
	// traffic needs and the defaults miss. The product rules then derive the
	// product line from the landing path, which is what gives custom_1 meaning.
	settings.Filters = append(settings.Filters, demoChannelFilters()...)
	settings.Filters = append(settings.Filters, demoProductCategoryFilters()...)
	settings.FiltersVersion = domain.ComputeWebFiltersVersion(settings.Filters)

	workspace.Settings.WebAnalytics = settings
	if err := s.workspaceRepo.Update(ctx, workspace); err != nil {
		return nil, fmt.Errorf("failed to enable demo web analytics: %w", err)
	}

	return settings.Filters, nil
}

// demoContactEmails returns the seeded contacts' emails, used as visitor ids so
// the analytics and the email side of the demo describe the same people.
func (s *DemoService) demoContactEmails(ctx context.Context, workspaceID string) ([]string, error) {
	response, err := s.contactService.GetContacts(ctx, &domain.GetContactsRequest{
		WorkspaceID: workspaceID,
		Limit:       1000,
	})
	if err != nil {
		return nil, err
	}

	emails := make([]string, 0, len(response.Contacts))
	for _, contact := range response.Contacts {
		if contact.Email != "" {
			emails = append(emails, contact.Email)
		}
	}
	return emails, nil
}

func (s *DemoService) analyzeDemoWebAnalytics(ctx context.Context, workspaceID string) {
	for _, table := range []string{"web_sessions", "web_pages", "web_goals"} {
		partitions, err := s.webAnalyticsRepo.ListPartitions(ctx, workspaceID, table)
		if err != nil {
			s.logger.WithField("table", table).WithField("error", err.Error()).
				Warn("Failed to list demo web analytics partitions")
			continue
		}
		if err := s.webAnalyticsRepo.AnalyzePartitions(ctx, workspaceID, partitions); err != nil {
			s.logger.WithField("table", table).WithField("error", err.Error()).
				Warn("Failed to analyze demo web analytics partitions")
		}
	}
}

func demoMonthOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func demoMonthsCovering(generator *demoWebAnalyticsGenerator) []time.Time {
	seen := map[time.Time]bool{}
	months := []time.Time{}
	for day := 0; day < generator.Days(); day++ {
		month := demoMonthOf(generator.DayStart(day))
		if !seen[month] {
			seen[month] = true
			months = append(months, month)
		}
	}
	// The current month's successor, so a session generated moments after the
	// reset still has somewhere to land.
	next := demoMonthOf(time.Now().UTC()).AddDate(0, 1, 0)
	if !seen[next] {
		months = append(months, next)
	}
	return months
}

func sortTimesDescending(times []time.Time) {
	for i := 0; i < len(times); i++ {
		for j := i + 1; j < len(times); j++ {
			if times[j].After(times[i]) {
				times[i], times[j] = times[j], times[i]
			}
		}
	}
}

// demoChannelFilters covers the traffic this demo's fixtures generate that the
// out-of-the-box rules do not classify. Two gaps matter:
//
//   - The campaigns use utm_medium "social", while the shipped Instagram and
//     Facebook rules require cpc/paid/paidsocial. Without these, paid social
//     lands in "not-mapped" and the channel report reads as broken.
//   - The default set has no rules for tech publishers, retailers, or the
//     site's own domain — traffic a consumer-electronics site actually gets.
//
// Priorities sit in the gaps of the default ladder so a default always wins
// where it applies. News aggregators are deliberately left unclassified: a demo
// where every session is already mapped gives the Filters tab nothing to
// demonstrate.
func demoChannelFilters() []domain.WebFilter {
	now := time.Now().UTC().Format(time.RFC3339)
	order := 200

	rule := func(id, name string, conditions []domain.WebFilterCondition, group, channel string, priority int) domain.WebFilter {
		order++
		return domain.WebFilter{
			ID:         "demo-channel-" + id,
			Name:       name,
			Priority:   priority,
			Order:      order,
			Tags:       []string{"channel"},
			Conditions: conditions,
			Operations: []domain.WebFilterOperation{
				{Dimension: "channel_group", Action: domain.WebFilterActionSetValue, Value: group},
				{Dimension: "channel", Action: domain.WebFilterActionSetValue, Value: channel},
			},
			Enabled:   true,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	utm := func(source, medium string) []domain.WebFilterCondition {
		return []domain.WebFilterCondition{
			{Field: "utm_source", Operator: domain.WebFilterOpRegex, Value: "^" + source + "$"},
			{Field: "utm_medium", Operator: domain.WebFilterOpRegex, Value: "^" + medium + "$"},
		}
	}
	referrer := func(host string) []domain.WebFilterCondition {
		return []domain.WebFilterCondition{
			{Field: "referrer_domain", Operator: domain.WebFilterOpContains, Value: host},
		}
	}

	return []domain.WebFilter{
		rule("instagram-social", "Instagram Ads (demo)", utm("instagram", "social"), "social-paid", "instagram-ads", 760),
		rule("facebook-social", "Facebook Ads (demo)", utm("facebook", "social"), "social-paid", "facebook-ads", 755),
		rule("twitter-social", "Twitter (demo)", utm("twitter", "social"), "social-organic", "twitter", 750),
		rule("display", "Display (demo)", utm("display", "display"), "display-banner", "display", 745),
		// 742, not 740: an exact tie with the shipped "Direct Traffic" rule
		// resolves by list order, which is not a property worth depending on.
		rule("affiliate", "Affiliate (demo)", utm("affiliate", "referral"), "referral", "affiliate", 742),

		rule("macrumors", "MacRumors", referrer("macrumors.com"), "tech-news", "macrumors", 500),
		rule("9to5mac", "9to5Mac", referrer("9to5mac.com"), "tech-news", "9to5mac", 495),
		rule("theverge", "The Verge", referrer("theverge.com"), "tech-news", "theverge", 490),
		rule("cnet", "CNET", referrer("cnet.com"), "tech-news", "cnet", 485),
		rule("techcrunch", "TechCrunch", referrer("techcrunch.com"), "tech-news", "techcrunch", 480),
		rule("engadget", "Engadget", referrer("engadget.com"), "tech-news", "engadget", 475),
		rule("wired", "Wired", referrer("wired.com"), "tech-news", "wired", 470),

		rule("amazon", "Amazon", referrer("amazon.com"), "referral", "amazon", 460),
		rule("bestbuy", "Best Buy", referrer("bestbuy.com"), "referral", "bestbuy", 455),
		rule("target", "Target", referrer("target.com"), "referral", "target", 450),
		rule("walmart", "Walmart", referrer("walmart.com"), "referral", "walmart", 445),

		rule("internal", "Own domain", referrer("apple.com"), "direct", "direct", 400),
	}
}
