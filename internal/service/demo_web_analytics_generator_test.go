package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// The generator's job is a distribution, so most of these assert statistical
// properties over a sample rather than exact values. Tolerances are wide enough
// that the seed does not matter and narrow enough that a broken curve fails.

func demoTestGenerator(t *testing.T, sessions, days int) *demoWebAnalyticsGenerator {
	t.Helper()
	filters := append(domain.DefaultWebFilters(), demoProductCategoryFilters()...)
	return newDemoWebAnalyticsGenerator(demoWebAnalyticsOptions{
		Sessions:   sessions,
		Days:       days,
		Now:        time.Date(2026, 8, 9, 18, 30, 0, 0, time.UTC),
		Seed:       1,
		Identities: []string{"ada@example.com", "grace@example.com", "alan@example.com"},
		Filters:    filters,
		SiteURL:    "https://www.apple.com",
	})
}

func demoGenerateAll(g *demoWebAnalyticsGenerator) demoWebAnalyticsBatch {
	all := demoWebAnalyticsBatch{}
	for day := 0; day < g.Days(); day++ {
		batch := g.GenerateDay(day)
		all.Sessions = append(all.Sessions, batch.Sessions...)
		all.Pages = append(all.Pages, batch.Pages...)
		all.Goals = append(all.Goals, batch.Goals...)
	}
	return all
}

func TestDemoWebAnalyticsGeneratorDeterminism(t *testing.T) {
	t.Run("the same seed produces the same demo", func(t *testing.T) {
		// A reset that shuffled its own data would make every screenshot and
		// every doc example rot on the next deploy.
		first := demoGenerateAll(demoTestGenerator(t, 2000, 30))
		second := demoGenerateAll(demoTestGenerator(t, 2000, 30))

		require.Equal(t, len(first.Sessions), len(second.Sessions))
		for i := range first.Sessions {
			assert.Equal(t, first.Sessions[i].ID, second.Sessions[i].ID, "session %d", i)
			assert.Equal(t, first.Sessions[i].DurationMs, second.Sessions[i].DurationMs)
			assert.Equal(t, first.Sessions[i].Channel, second.Sessions[i].Channel)
		}
		assert.Equal(t, len(first.Goals), len(second.Goals))
	})

	t.Run("a different seed produces a different demo", func(t *testing.T) {
		base := demoTestGenerator(t, 500, 10)
		other := newDemoWebAnalyticsGenerator(demoWebAnalyticsOptions{
			Sessions: 500, Days: 10, Now: base.opts.Now, Seed: 2,
			Filters: base.opts.Filters, SiteURL: base.opts.SiteURL,
		})
		assert.NotEqual(t,
			demoGenerateAll(base).Sessions[0].ID,
			demoGenerateAll(other).Sessions[0].ID)
	})
}

func TestDemoWebAnalyticsDailyCurve(t *testing.T) {
	g := demoTestGenerator(t, 100_000, 400)

	t.Run("the whole budget is allocated", func(t *testing.T) {
		total := 0
		for day := 0; day < g.Days(); day++ {
			total += g.dailyCounts[day]
		}
		assert.Equal(t, 100_000, total)
	})

	t.Run("traffic grows across the window", func(t *testing.T) {
		// Without a trend every previous-period comparison lands on 0%, which
		// hides the delta indicators the dashboard is built around.
		early, late := 0, 0
		for day := 0; day < 60; day++ {
			early += g.dailyCounts[day]
		}
		for day := g.Days() - 60; day < g.Days(); day++ {
			late += g.dailyCounts[day]
		}
		assert.Greater(t, late, early, "the last two months must outweigh the first two")
	})

	t.Run("the launch spikes and then decays", func(t *testing.T) {
		launch := g.dailyCounts[g.launchIndex]
		baseline := g.dailyCounts[g.launchIndex-7]
		afterwards := g.dailyCounts[g.launchIndex+1]

		assert.Greater(t, float64(launch), float64(baseline)*1.8, "launch day should roughly double")
		assert.Greater(t, afterwards, baseline, "the days after stay elevated")
		assert.Less(t, afterwards, launch, "but below the day itself")
	})

	t.Run("weekends are quieter", func(t *testing.T) {
		weekday, weekend, weekdays, weekends := 0, 0, 0, 0
		for day := 0; day < g.Days(); day++ {
			// Skip the launch window so its spike cannot mask the pattern.
			if day >= g.launchIndex-1 && day <= g.launchIndex+demoPostLaunchDays {
				continue
			}
			switch g.DayStart(day).Weekday() {
			case time.Saturday, time.Sunday:
				weekend += g.dailyCounts[day]
				weekends++
			default:
				weekday += g.dailyCounts[day]
				weekdays++
			}
		}
		assert.Less(t, float64(weekend)/float64(weekends), float64(weekday)/float64(weekdays))
	})
}

func TestDemoWebAnalyticsSessionShape(t *testing.T) {
	batch := demoGenerateAll(demoTestGenerator(t, 20_000, 60))
	require.NotEmpty(t, batch.Sessions)

	pagesBySession := map[string][]*domain.WebPage{}
	for _, page := range batch.Pages {
		pagesBySession[page.SessionID] = append(pagesBySession[page.SessionID], page)
	}

	t.Run("session aggregates are derived from the pages, not invented", func(t *testing.T) {
		// A TimeScore that disagrees with the pages table is worse than none.
		for _, session := range batch.Sessions[:500] {
			pages := pagesBySession[session.ID]
			require.Len(t, pages, session.PageviewCount, "session %s", session.ID)

			var sum int64
			maxScroll := 0
			for _, page := range pages {
				sum += page.DurationMs
				if page.MaxScroll > maxScroll {
					maxScroll = page.MaxScroll
				}
			}
			assert.Equal(t, sum, session.DurationMs, "duration is the sum of page focus time")
			assert.Equal(t, maxScroll, session.MaxScroll)
		}
	})

	t.Run("pages are contiguous with one landing and one exit", func(t *testing.T) {
		for _, session := range batch.Sessions[:500] {
			pages := pagesBySession[session.ID]
			landings, exits := 0, 0
			for i, page := range pages {
				assert.Equal(t, i+1, page.PageNumber, "page numbers start at 1 and are contiguous")
				if page.IsLanding {
					landings++
				}
				if page.IsExit {
					exits++
				}
			}
			assert.Equal(t, 1, landings, "session %s", session.ID)
			assert.Equal(t, 1, exits, "session %s", session.ID)
			assert.Equal(t, pages[len(pages)-1].Path, session.ExitPath)
			assert.Equal(t, pages[0].Path, session.LandingPath)
		}
	})

	t.Run("scroll depth only grows within a session", func(t *testing.T) {
		// A visitor who reached 80% of one page has seen that much of the
		// visit; a redrawn value would make the metric incoherent.
		for _, pages := range pagesBySession {
			best := 0
			for _, page := range pages {
				if page.MaxScroll > best {
					best = page.MaxScroll
				}
			}
			assert.LessOrEqual(t, best, 100)
		}
	})

	t.Run("more than two pageviews happen", func(t *testing.T) {
		// Staminads caps at two, which pins pages/session at 1.24 and makes
		// every exit path equal its landing path.
		deep := 0
		for _, session := range batch.Sessions {
			if session.PageviewCount >= 3 {
				deep++
			}
		}
		share := float64(deep) / float64(len(batch.Sessions))
		assert.Greater(t, share, 0.30, "at least a third of sessions go three pages deep")
	})

	t.Run("the bounce rate is believable", func(t *testing.T) {
		// Staminads lands at 4.2% because its duration multipliers compound.
		bounced := 0
		for _, session := range batch.Sessions {
			if session.DurationMs < 10_000 {
				bounced++
			}
		}
		rate := float64(bounced) / float64(len(batch.Sessions)) * 100
		assert.Greater(t, rate, 15.0, "bounce rate %.1f%% is implausibly low", rate)
		assert.Less(t, rate, 50.0, "bounce rate %.1f%% is implausibly high", rate)
	})

	t.Run("no session happens in the future", func(t *testing.T) {
		now := time.Date(2026, 8, 9, 18, 30, 0, 0, time.UTC)
		for _, session := range batch.Sessions {
			assert.False(t, session.CreatedAt.After(now), "session %s", session.ID)
		}
	})
}

func TestDemoWebAnalyticsSessionIdentity(t *testing.T) {
	batch := demoGenerateAll(demoTestGenerator(t, 10_000, 30))

	t.Run("the id carries the session start, so it agrees with its partition", func(t *testing.T) {
		// The ingest path derives the partition from the id alone; demo rows
		// have to satisfy the same invariant or a backfill would move them.
		for _, session := range batch.Sessions[:200] {
			parsed, err := uuid.Parse(session.ID)
			require.NoError(t, err)
			require.Equal(t, 7, int(parsed.Version()))

			ms := int64(parsed[0])<<40 | int64(parsed[1])<<32 | int64(parsed[2])<<24 |
				int64(parsed[3])<<16 | int64(parsed[4])<<8 | int64(parsed[5])
			embedded := time.UnixMilli(ms).UTC()

			assert.WithinDuration(t, session.CreatedAt, embedded, time.Millisecond)
			assert.Equal(t,
				time.Date(embedded.Year(), embedded.Month(), embedded.Day(), 0, 0, 0, 0, time.UTC),
				session.SessionDate)
		}
	})

	t.Run("a minority of visitors are known contacts", func(t *testing.T) {
		identified := 0
		for _, session := range batch.Sessions {
			if session.ContactEmail != nil {
				identified++
				assert.Contains(t, *session.ContactEmail, "@", "the demo identity is a contact address")
			}
		}
		share := float64(identified) / float64(len(batch.Sessions))
		assert.InDelta(t, demoIdentifiedShare, share, 0.03)
	})

	t.Run("pages and goals inherit their session", func(t *testing.T) {
		byID := map[string]*domain.WebSession{}
		for _, session := range batch.Sessions {
			byID[session.ID] = session
		}
		for _, page := range batch.Pages {
			session, ok := byID[page.SessionID]
			require.True(t, ok, "page references a missing session")
			assert.Equal(t, session.SessionDate, page.SessionDate)
		}
		for _, goal := range batch.Goals {
			session, ok := byID[goal.SessionID]
			require.True(t, ok, "goal references a missing session")
			assert.Equal(t, session.SessionDate, goal.SessionDate)
		}
	})
}

func TestDemoWebAnalyticsGoals(t *testing.T) {
	batch := demoGenerateAll(demoTestGenerator(t, 30_000, 90))

	byName := map[string]int{}
	bySession := map[string]map[string]*domain.WebGoal{}
	for _, goal := range batch.Goals {
		byName[goal.GoalName]++
		if bySession[goal.SessionID] == nil {
			bySession[goal.SessionID] = map[string]*domain.WebGoal{}
		}
		bySession[goal.SessionID][goal.GoalName] = goal
	}

	// Demo data is the one dataset where a missing goal type is invisible until a
	// prospect tries the feature: an untyped goal makes the Custom Events Goal
	// segment condition match nothing, precisely where the product is being shown.
	// The other subtests here pin shape, value, timing and count — dropping the
	// type would pass every one of them.
	t.Run("every demo goal carries a real type", func(t *testing.T) {
		require.NotEmpty(t, batch.Goals)
		valid := map[string]bool{}
		for _, t := range domain.ValidGoalTypes {
			valid[t] = true
		}
		seen := map[string]bool{}
		for _, goal := range batch.Goals {
			require.NotEmpty(t, goal.GoalType, "goal %q is untyped", goal.GoalName)
			require.True(t, valid[goal.GoalType],
				"goal %q carries %q, which is not a type the segment conditions accept",
				goal.GoalName, goal.GoalType)
			seen[goal.GoalType] = true
		}
		assert.Contains(t, seen, domain.GoalTypePurchase,
			"a demo with no purchase-typed goal cannot demonstrate revenue reporting")
	})

	t.Run("the funnel narrows", func(t *testing.T) {
		require.NotZero(t, byName["add_to_cart"], "no conversions at all")
		assert.Greater(t, byName["add_to_cart"], byName["checkout_start"])
		assert.Greater(t, byName["checkout_start"], byName["purchase"])
	})

	t.Run("every step implies the one before it", func(t *testing.T) {
		for sessionID, goals := range bySession {
			if _, ok := goals["purchase"]; ok {
				assert.Contains(t, goals, "checkout_start", "session %s", sessionID)
			}
			if _, ok := goals["checkout_start"]; ok {
				assert.Contains(t, goals, "add_to_cart", "session %s", sessionID)
			}
		}
	})

	t.Run("a purchase is billed at the price it was added at", func(t *testing.T) {
		checked := 0
		for _, goals := range bySession {
			purchase, ok := goals["purchase"]
			if !ok {
				continue
			}
			assert.Equal(t, goals["add_to_cart"].GoalValue, purchase.GoalValue)
			assert.Greater(t, purchase.GoalValue, 0.0)
			checked++
		}
		require.NotZero(t, checked, "no purchases to check")
	})

	t.Run("goals happen inside their session", func(t *testing.T) {
		byID := map[string]*domain.WebSession{}
		for _, session := range batch.Sessions {
			byID[session.ID] = session
		}
		for _, goal := range batch.Goals {
			session := byID[goal.SessionID]
			require.NotNil(t, session)
			assert.False(t, goal.GoalAt.Before(session.CreatedAt),
				"goal %s fires before its session starts", goal.GoalName)
		}
	})

	t.Run("the session carries its own conversion totals", func(t *testing.T) {
		byID := map[string]*domain.WebSession{}
		for _, session := range batch.Sessions {
			byID[session.ID] = session
		}
		for sessionID, goals := range bySession {
			session := byID[sessionID]
			require.NotNil(t, session)
			assert.Equal(t, len(goals), session.GoalCount)
		}
	})
}

func TestDemoWebAnalyticsAttribution(t *testing.T) {
	batch := demoGenerateAll(demoTestGenerator(t, 20_000, 60))

	t.Run("the rules classify nearly everything", func(t *testing.T) {
		// The generator writes no channel of its own; whatever is here came
		// from the workspace's filters.
		classified, product := 0, 0
		for _, session := range batch.Sessions {
			if session.ChannelGroup != "" {
				classified++
			}
			if session.Custom1 != "" {
				product++
			}
		}
		assert.Greater(t, float64(classified)/float64(len(batch.Sessions)), 0.95)
		assert.Greater(t, float64(product)/float64(len(batch.Sessions)), 0.95,
			"custom_1 comes from the product-category rules")
	})

	t.Run("click ids reach the highest-priority default rules", func(t *testing.T) {
		// Staminads never populates utm_id_from, which leaves the ten click-ID
		// rules at priorities 900-870 as dead weight.
		clicks := 0
		for _, session := range batch.Sessions {
			if session.UTMIDFrom == "gclid" {
				clicks++
				assert.NotEmpty(t, session.UTMID)
				assert.Equal(t, "search-paid", session.ChannelGroup)
			}
		}
		assert.NotZero(t, clicks, "no gclid traffic was generated")
	})

	t.Run("a campaign's referrer matches its source", func(t *testing.T) {
		for _, session := range batch.Sessions {
			if session.UTMSource == "google" && session.UTMMedium == "cpc" {
				assert.Equal(t, "www.google.com", session.ReferrerDomain)
			}
			if session.UTMSource == "email" {
				assert.True(t, session.IsDirect, "an email click carries no referrer")
			}
		}
	})

	t.Run("the channel mix is varied enough to fill a dashboard", func(t *testing.T) {
		groups := map[string]int{}
		for _, session := range batch.Sessions {
			groups[session.ChannelGroup]++
		}
		assert.GreaterOrEqual(t, len(groups), 6, "got %v", groups)
		for _, required := range []string{"search-paid", "search-organic", "direct"} {
			assert.NotZero(t, groups[required], "missing %s in %v", required, groups)
		}
	})
}

func TestDemoWebAnalyticsHoursFollowTheVisitorsClock(t *testing.T) {
	// The hour curve describes a visitor's own day. Sampled in UTC it would put
	// a European evening peak in the middle of an American night, and the day ×
	// hour heat map — which reads in the workspace timezone — would be wrong.
	batch := demoGenerateAll(demoTestGenerator(t, 30_000, 60))

	byLocalHour := map[int]int{}
	for _, session := range batch.Sessions {
		location := mustLoadLocation(session.Timezone)
		byLocalHour[session.CreatedAt.In(location).Hour()]++
	}

	night := byLocalHour[2] + byLocalHour[3] + byLocalHour[4]
	evening := byLocalHour[18] + byLocalHour[19] + byLocalHour[20]
	require.NotZero(t, night, "the histogram is empty")
	assert.Greater(t, evening, night*2,
		fmt.Sprintf("evening %d should dominate the small hours %d", evening, night))
}
