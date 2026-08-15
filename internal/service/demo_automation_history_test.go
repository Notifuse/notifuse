package service

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// demoTestCohort builds a cohort with a mix of newsletter statuses, so both the ordinary paths and
// the subscription-guard exit are exercised.
func demoTestCohort(size int) []demoCohortMember {
	statuses := []string{"active", "active", "active", "active", "unsubscribed", "bounced", "complained"}
	cohort := make([]demoCohortMember, 0, size)
	for i := 0; i < size; i++ {
		cohort = append(cohort, demoCohortMember{
			Email:      fmt.Sprintf("contact%04d@example.com", i),
			ListStatus: statuses[i%len(statuses)],
		})
	}
	return cohort
}

func demoTestJourneys(t *testing.T, automation *domain.Automation, size int) []demoJourney {
	t.Helper()
	rng := rand.New(rand.NewSource(demoAutomationHistorySeed))
	journeys := buildDemoJourneys(automation, demoTestCohort(size), rng, time.Now().UTC())
	require.NotEmpty(t, journeys, "no journeys were built for %s", automation.ID)
	return journeys
}

// The scheduler claims a journey only when contact_automations.status = 'active' AND
// scheduled_at <= now AND the automation is live and not deleted. Seeded rows must fail that
// predicate on their own terms, not because the automation happens to be paused — Pause does not
// touch these rows, so a row left claimable would flood the queue the moment anyone un-pauses.
func TestDemoJourneys_AreUnclaimableByTheScheduler(t *testing.T) {
	for _, automation := range demoAutomations("ws1") {
		t.Run(automation.ID, func(t *testing.T) {
			for _, j := range demoTestJourneys(t, automation, 140) {
				// The predicate, restated as the claim query has it.
				claimable := j.Status == domain.ContactAutomationStatusActive
				assert.False(t, claimable, "journey for %s is claimable: status %q", j.Email, j.Status)

				// scheduled_at is never written for a seeded journey. Asserted through the insert
				// arguments rather than the struct, because that is what actually reaches Postgres.
				assert.Nil(t, demoScheduledAtArg(j), "journey for %s carries a scheduled_at", j.Email)
			}
		})
	}
}

// exit_reason is not free text: only the values the engine really writes may appear, or the demo
// shows a contact a reason no install of Notifuse can produce. "filter_rejected" in particular exists
// nowhere but a doc comment — a filter rejection with an empty exit branch simply completes.
func TestDemoJourneys_UseOnlyExitReasonsTheEngineWrites(t *testing.T) {
	allowed := map[string]bool{"unsubscribed": true, "bounced": true, "complained": true}

	for _, automation := range demoAutomations("ws1") {
		for _, j := range demoTestJourneys(t, automation, 140) {
			if j.ExitReason == nil {
				continue
			}
			assert.True(t, allowed[*j.ExitReason],
				"%s writes exit_reason %q, which no code path produces", automation.ID, *j.ExitReason)
			assert.Equal(t, domain.ContactAutomationStatusExited, j.Status,
				"%s sets an exit reason on a journey that did not exit", automation.ID)
		}
	}
}

// A completed or failed journey leaves exit_reason NULL — that is what the executor does — while the
// timeline entry still carries a human-readable reason. The two deliberately differ.
func TestDemoJourneys_LeaveExitReasonNullUnlessTheGuardStoppedThem(t *testing.T) {
	for _, automation := range demoAutomations("ws1") {
		for _, j := range demoTestJourneys(t, automation, 140) {
			if j.Status == domain.ContactAutomationStatusExited {
				continue
			}
			assert.Nil(t, j.ExitReason, "%s sets an exit reason on a %s journey", automation.ID, j.Status)
			assert.NotEmpty(t, j.TimelineReason, "%s writes a timeline entry with no reason", automation.ID)
		}
	}
}

// contact_automations has UNIQUE(automation_id, contact_email, entered_at). The generator must
// guarantee distinct timestamps rather than rely on luck.
func TestDemoJourneys_HaveDistinctEnrollmentTimes(t *testing.T) {
	for _, automation := range demoAutomations("ws1") {
		seen := map[string]bool{}
		for _, j := range demoTestJourneys(t, automation, 300) {
			key := j.Email + "|" + j.EnteredAt.Format(time.RFC3339Nano)
			assert.False(t, seen[key], "%s enrolls %s twice at the same instant", automation.ID, j.Email)
			seen[key] = true
		}
	}
}

// action='entered' is written by exactly one thing in the whole codebase: the enroll function, for
// the root node, with node_type hardcoded to 'trigger'. Putting it anywhere else would make the
// Flow Stats drawer show an "Inflight" number no real workspace can produce. And 'skipped' is never
// written at all, which is why the filter node's third tile is permanently zero in production.
func TestDemoJourneys_RecordTheActionsTheEngineRecords(t *testing.T) {
	for _, automation := range demoAutomations("ws1") {
		t.Run(automation.ID, func(t *testing.T) {
			for _, j := range demoTestJourneys(t, automation, 140) {
				enteredRows := 0
				for i, v := range j.Visits {
					assert.NotEqual(t, domain.NodeActionSkipped, v.Action, "a seeded row uses 'skipped', which nothing writes")
					assert.NotEqual(t, domain.NodeActionProcessing, v.Action, "a seeded row is left mid-flight as 'processing'")

					if v.Action == domain.NodeActionEntered {
						enteredRows++
						assert.Equal(t, 0, i, "the 'entered' row is not the first")
						assert.Equal(t, automation.RootNodeID, v.NodeID, "'entered' is recorded on a node other than the root")
						assert.Equal(t, domain.NodeTypeTrigger, v.NodeType, "the enrollment row must claim node_type 'trigger'")
					}
				}
				require.Equal(t, 1, enteredRows, "a journey must have exactly one enrollment row")
			}
		})
	}
}

// The engine's own error path hardcodes node_type 'trigger' on a failure row, which splits one node
// into two GROUP BY (node_id, node_type) groups and lets the console — which keys its map by node_id
// alone — silently overwrite one with the other. Seeded rows must not reproduce that.
func TestDemoJourneys_UseRealNodeTypesOnFailureRows(t *testing.T) {
	for _, automation := range demoAutomations("ws1") {
		for _, j := range demoTestJourneys(t, automation, 200) {
			for _, v := range j.Visits {
				if v.Action != domain.NodeActionFailed {
					continue
				}
				node := automation.GetNodeByID(v.NodeID)
				require.NotNil(t, node)
				assert.Equal(t, node.Type, v.NodeType,
					"%s writes a failure row for %s with node_type %q instead of %q",
					automation.ID, v.NodeID, v.NodeType, node.Type)
			}
		}
	}
}

func TestDemoJourneys_OnlyVisitNodesThatExist(t *testing.T) {
	for _, automation := range demoAutomations("ws1") {
		for _, j := range demoTestJourneys(t, automation, 140) {
			require.NotEmpty(t, j.Visits)
			for _, v := range j.Visits {
				node := automation.GetNodeByID(v.NodeID)
				assert.NotNil(t, node, "%s visits node %s, which does not exist", automation.ID, v.NodeID)
			}
			assert.NotNil(t, automation.GetNodeByID(j.CurrentNodeID),
				"%s parks a journey on node %s, which does not exist", automation.ID, j.CurrentNodeID)
		}
	}
}

// A visit's timestamps have to move forward, and a delay node has to account for its configured wait
// rather than the milliseconds a processing step takes.
func TestDemoJourneys_HaveCoherentTimestamps(t *testing.T) {
	for _, automation := range demoAutomations("ws1") {
		for _, j := range demoTestJourneys(t, automation, 100) {
			previous := j.EnteredAt
			for _, v := range j.Visits {
				if v.Action == domain.NodeActionEntered {
					continue
				}
				assert.False(t, v.EnteredAt.Before(previous), "%s runs a node before the one that precedes it", automation.ID)
				assert.True(t, v.CompletedAt.After(v.EnteredAt), "%s completes node %s before entering it", automation.ID, v.NodeID)
				assert.Positive(t, v.DurationMs, "%s records node %s with no duration", automation.ID, v.NodeID)

				if node := automation.GetNodeByID(v.NodeID); node != nil && node.Type == domain.NodeTypeDelay {
					want := demoDelayDuration(node.Config)
					assert.Equal(t, int(want/time.Millisecond), v.DurationMs,
						"%s does not wait its configured delay at %s", automation.ID, v.NodeID)
				}
				previous = v.CompletedAt
			}
			assert.Equal(t, j.Visits[len(j.Visits)-1].CompletedAt, j.EndedAt)
		}
	}
}

// The card's four numbers come from automations.stats, the funnel from a different table. They are
// written in one transaction precisely so they cannot disagree.
func TestDemoAutomationStats_MatchTheJourneysBehindThem(t *testing.T) {
	for _, automation := range demoAutomations("ws1") {
		journeys := demoTestJourneys(t, automation, 200)
		stats := demoAutomationStats(journeys)

		assert.Equal(t, int64(len(journeys)), stats.Enrolled)
		assert.Equal(t, stats.Enrolled, stats.Completed+stats.Exited+stats.Failed,
			"%s has journeys in no terminal bucket", automation.ID)
		assert.Positive(t, stats.Completed, "%s completes nothing", automation.ID)
	}
}

// A fixed seed is worth nothing if a map iteration or a timestamp slips in.
func TestDemoJourneys_AreDeterministic(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	cohort := demoTestCohort(120)

	for _, automation := range demoAutomations("ws1") {
		first := buildDemoJourneys(automation, cohort, rand.New(rand.NewSource(demoAutomationHistorySeed)), now)
		second := buildDemoJourneys(automation, cohort, rand.New(rand.NewSource(demoAutomationHistorySeed)), now)

		require.Len(t, second, len(first))
		for i := range first {
			// The row id is a fresh uuid by design; everything that lands in a column is not.
			assert.Equal(t, first[i].Email, second[i].Email)
			assert.Equal(t, first[i].Status, second[i].Status)
			assert.Equal(t, first[i].EnteredAt, second[i].EnteredAt)
			assert.Equal(t, first[i].CurrentNodeID, second[i].CurrentNodeID)
			assert.Len(t, second[i].Visits, len(first[i].Visits))
		}
	}
}

// The renderer and this seeder are coupled through something invisible to the compiler: changes
// carries a {field: {new: value}} envelope. Flatten it and the entry still renders — just blank.
func TestDemoJourneyTimelineEntries(t *testing.T) {
	for _, automation := range demoAutomations("ws1") {
		t.Run(automation.ID, func(t *testing.T) {
			journeys := demoTestJourneys(t, automation, 60)
			entries, err := demoJourneyTimelineEntries(automation, journeys)
			require.NoError(t, err)
			require.Len(t, entries, len(journeys)*2)

			starts, ends := 0, 0
			for _, e := range entries {
				var changes map[string]map[string]interface{}
				require.NoError(t, json.Unmarshal(e.Changes, &changes), "changes are not a {key:{new:...}} envelope")
				require.Contains(t, changes, "automation_id")
				require.Contains(t, changes["automation_id"], "new")

				switch e.Kind {
				case "automation.start":
					starts++
					assert.Equal(t, "insert", e.Operation)
					assert.Contains(t, changes, "root_node_id")
				case "automation.end":
					ends++
					assert.Equal(t, "update", e.Operation)
					assert.Contains(t, changes, "exit_reason")
					assert.Contains(t, changes, "status")
					assert.NotEmpty(t, changes["exit_reason"]["new"])
				default:
					t.Fatalf("unexpected timeline kind %q", e.Kind)
				}
			}
			assert.Equal(t, len(journeys), starts)
			assert.Equal(t, len(journeys), ends)

			// Ordered by email, so concurrent inserters take the contact_segment_queue row locks in
			// the same order and cannot deadlock against each other.
			for i := 1; i < len(entries); i++ {
				assert.LessOrEqual(t, entries[i-1].Email, entries[i].Email, "timeline entries are not ordered by email")
			}
		})
	}
}

// An entry stamped with the reset time would sort above the cart event that caused it — the same
// defect the web-activity seeding had to fix.
func TestDemoJourneyTimelineEntries_AreDatedWhenTheJourneyHappened(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

	for _, automation := range demoAutomations("ws1") {
		journeys := buildDemoJourneys(automation, demoTestCohort(60), rand.New(rand.NewSource(demoAutomationHistorySeed)), now)
		entries, err := demoJourneyTimelineEntries(automation, journeys)
		require.NoError(t, err)

		byEmail := map[string]demoJourney{}
		for _, j := range journeys {
			byEmail[j.Email] = j
		}

		for _, e := range entries {
			j, ok := byEmail[e.Email]
			require.True(t, ok)
			assert.False(t, e.CreatedAt.After(time.Now().UTC()), "a timeline entry is dated in the future")
			if e.Kind == "automation.start" {
				assert.Equal(t, j.EnteredAt, e.CreatedAt)
			} else {
				assert.Equal(t, j.EndedAt, e.CreatedAt)
			}
		}
	}
}

// Nothing executes add_to_list or remove_from_list in demo, so without these writes the VIP Club
// list ships empty while post-purchase's card claims journeys completed through an add_to_list node.
func TestDemoJourneyListChanges(t *testing.T) {
	byID := map[string]*domain.Automation{}
	for _, automation := range demoAutomations("ws1") {
		byID[automation.ID] = automation
	}

	t.Run("a completed post-purchase journey joins the VIP list", func(t *testing.T) {
		journeys := demoTestJourneys(t, byID[demoAutomationPostPurchase], 120)
		adds, removals := demoJourneyListChanges(journeys)

		assert.NotEmpty(t, adds, "nobody is promoted to the VIP list")
		assert.Empty(t, removals, "the thank-you flow removes nobody from anything")

		for i := 1; i < len(adds); i++ {
			assert.LessOrEqual(t, adds[i-1].Email, adds[i].Email, "list changes are not ordered by email")
		}
	})

	t.Run("a sunset journey leaves the newsletter", func(t *testing.T) {
		journeys := demoTestJourneys(t, byID[demoAutomationWinback], 200)
		adds, removals := demoJourneyListChanges(journeys)

		assert.Empty(t, adds)
		assert.NotEmpty(t, removals, "nobody is sunset, so the removal node's numbers are a fiction")
	})

	t.Run("a journey that never reached the node changes nothing", func(t *testing.T) {
		// A failure at the add_to_list node itself means the membership was never written.
		journey := demoJourney{
			Email: "someone@example.com",
			Visits: []demoNodeVisit{
				{NodeID: "pp-trigger", NodeType: domain.NodeTypeTrigger, Action: domain.NodeActionEntered},
				{NodeID: "pp-add-vip", NodeType: domain.NodeTypeAddToList, Action: domain.NodeActionFailed},
			},
		}
		adds, removals := demoJourneyListChanges([]demoJourney{journey})
		assert.Empty(t, adds)
		assert.Empty(t, removals)
	})
}
// The insert has to fill every column it names, or Postgres rejects the statement outright.
func TestDemoContactAutomationArgs_MatchTheColumnList(t *testing.T) {
	journeys := demoTestJourneys(t, demoAutomations("ws1")[0], 10)
	assert.Len(t, demoContactAutomationArgs(journeys[0]), demoContactAutomationColumns)
}
