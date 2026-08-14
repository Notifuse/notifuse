package service

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// The showcase segments are only a showcase if they have members, and the demo's
// purchase history now comes entirely from the web funnel — so the tier
// constants, the identified conversion factor and the funnel rates together
// decide whether "Cart Abandoners" opens on 160 contacts or on an empty table.
// Nothing else in the suite would notice: every distribution assertion would
// still pass with a workspace where nobody ever bought anything.
//
// The bounds are ranges rather than exact counts. The point is not the number,
// it is that no audience is empty and none has swallowed the workspace.
func TestDemoWebAnalyticsAudiences(t *testing.T) {
	const contacts = 1000

	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

	// The demo's own contact population: a thousand addresses whose signup dates
	// are spread across the same window the traffic covers
	// (generateSampleContactsBatch). Modelled rather than mocked, because the
	// signup curve is what decides how much of the traffic can be identified at
	// all — a test using contacts that all predate the window would measure a
	// workspace the demo never produces.
	identities := make([]demoIdentity, 0, contacts)
	for i := 0; i < contacts; i++ {
		// The same square-root curve generateSampleContactsBatch draws from,
		// walked deterministically instead of sampled.
		age := math.Sqrt((float64(i)+0.5)/contacts) * float64(demoWebAnalyticsDays) * 24
		identities = append(identities, demoIdentity{
			Email:      fmt.Sprintf("contact%d@example.com", i),
			KnownSince: now.Add(-time.Duration(age) * time.Hour),
		})
	}

	filters := append(domain.DefaultWebFilters(), demoChannelFilters()...)
	filters = append(filters, demoProductCategoryFilters()...)

	batch := demoGenerateAll(newDemoWebAnalyticsGenerator(demoWebAnalyticsOptions{
		Sessions:   demoWebAnalyticsSessions,
		Days:       demoWebAnalyticsDays,
		Now:        now,
		Seed:       demoWebAnalyticsSeed,
		Identities: identities,
		Filters:    filters,
		SiteURL:    demoWebAnalyticsSite,
	}))

	// The window the segments use, and the one the navigation projection uses.
	// They are the same 90 days on purpose: a contact in Cart Abandoners should
	// have the visit that put them there visible on their timeline.
	since := now.AddDate(0, 0, -demoWebAnalyticsTimelineDays)

	cartRecently := map[string]bool{}
	checkoutRecently := map[string]bool{}
	boughtRecently := map[string]bool{}
	purchaseCount := map[string]int{}
	purchaseValue := map[string]float64{}

	for _, goal := range batch.Goals {
		if goal.ContactEmail == nil {
			continue
		}
		email := *goal.ContactEmail
		recent := !goal.GoalAt.Before(since)
		switch goal.GoalName {
		case "add_to_cart":
			if recent {
				cartRecently[email] = true
			}
		case "checkout_start":
			if recent {
				checkoutRecently[email] = true
			}
		case "purchase":
			if recent {
				boughtRecently[email] = true
			}
			purchaseCount[email]++
			purchaseValue[email] += goal.GoalValue
		}
	}

	countAbandoners := func(intent map[string]bool) int {
		abandoners := 0
		for email := range intent {
			if !boughtRecently[email] {
				abandoners++
			}
		}
		return abandoners
	}

	t.Run("Cart Abandoners is a usable audience", func(t *testing.T) {
		size := countAbandoners(cartRecently)
		assert.Greater(t, size, 40, "only %d contacts carted without buying", size)
		assert.Less(t, size, contacts/2, "%d contacts is most of the workspace", size)
	})

	t.Run("Checkout Abandoners is smaller but not empty", func(t *testing.T) {
		// It is a strictly deeper step of the same funnel, so it must be a
		// smaller audience — if the two ever match in size, the event names are
		// not selecting what they claim to.
		cart := countAbandoners(cartRecently)
		checkout := countAbandoners(checkoutRecently)
		assert.Greater(t, checkout, 15, "only %d contacts reached checkout without buying", checkout)
		assert.Less(t, checkout, cart, "checkout %d should be narrower than cart %d", checkout, cart)
	})

	t.Run("VIP Customers has members at the thresholds the demo ships", func(t *testing.T) {
		// Mirrors the segment in createSampleSegments: three purchases and $800
		// of lifetime value, both "anytime".
		vips := 0
		for email, count := range purchaseCount {
			if count >= 3 && purchaseValue[email] >= 800 {
				vips++
			}
		}
		assert.Greater(t, vips, 10, "only %d contacts qualify as VIP", vips)
	})

	t.Run("Win-back Opportunities is lapsed buyers, not the whole list", func(t *testing.T) {
		// The reason the segment gained its "bought at some point" leaf: the
		// negation alone matches everyone who never bought, which here is most
		// of the workspace.
		lapsed := 0
		for email := range purchaseCount {
			if !boughtRecently[email] {
				lapsed++
			}
		}
		assert.Greater(t, lapsed, 20, "only %d lapsed buyers", lapsed)
		assert.Less(t, lapsed, contacts/2, "%d is most of the workspace", lapsed)
		assert.Greater(t, len(purchaseCount), lapsed,
			"every buyer is lapsed, so nobody bought inside the window")
	})

	t.Run("the timeline window is populated for a large share of contacts", func(t *testing.T) {
		// What the contact drawer will actually show. A window that only reaches
		// a handful of contacts would mean most of the demo's contacts open on an
		// email-only timeline.
		visited := map[string]int{}
		rows := 0
		for _, session := range batch.Sessions {
			if session.ContactEmail == nil || session.CreatedAt.Before(since) {
				continue
			}
			visited[*session.ContactEmail]++
			rows += 1 + session.PageviewCount // the visit summary plus its pages
		}

		require.NotEmpty(t, visited)
		// Four in five, not "most": a contact can only be attributed visits made
		// after they signed up, so the newest members of the list legitimately
		// have nothing here. Below this, the drawer is empty too often for the
		// feature to be worth opening the demo for.
		assert.Greater(t, len(visited), contacts*3/4,
			"only %d of %d contacts have a recent visit", len(visited), contacts)
		assert.Less(t, rows/len(visited), 40,
			"%d timeline rows per contact would bury the email history", rows/len(visited))

		t.Logf("contacts with a recent visit: %d/%d, %d timeline rows, %d per visited contact",
			len(visited), contacts, rows, rows/len(visited))
	})

	// Not an assertion: the numbers the bounds above are drawn from, printed so
	// that tuning a constant does not mean re-deriving them from scratch.
	t.Run("summary", func(t *testing.T) {
		identified := 0
		for _, session := range batch.Sessions {
			if session.ContactEmail != nil {
				identified++
			}
		}
		byName := map[string]int{}
		for _, goal := range batch.Goals {
			if goal.ContactEmail != nil {
				byName[goal.GoalName]++
			}
		}
		t.Logf("sessions %d (%d identified, %.1f%%), identified goals %v, buyers %d",
			len(batch.Sessions), identified,
			100*float64(identified)/float64(len(batch.Sessions)), byName, len(purchaseCount))
		t.Logf("audiences: cart %d, checkout %d, buyers %d",
			countAbandoners(cartRecently), countAbandoners(checkoutRecently), len(purchaseCount))
	})
}
