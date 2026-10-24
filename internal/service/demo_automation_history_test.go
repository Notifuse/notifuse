package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
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

func TestDemoAutomationCohortSQL(t *testing.T) {
	t.Run("welcome draws from the newsletter list itself", func(t *testing.T) {
		query, args := demoAutomationCohortSQL(demoAutomationWelcome)
		assert.Contains(t, query, "FROM contact_lists")
		assert.Contains(t, query, "status = 'active'")
		assert.Equal(t, []interface{}{demoListNewsletter}, args)
	})

	t.Run("goal automations draw from custom events with their own goal", func(t *testing.T) {
		_, cartArgs := demoAutomationCohortSQL(demoAutomationCartRecovery)
		assert.Equal(t, []interface{}{demoGoalAddToCart}, cartArgs)

		query, purchaseArgs := demoAutomationCohortSQL(demoAutomationPostPurchase)
		assert.Contains(t, query, "FROM custom_events")
		assert.Equal(t, []interface{}{demoGoalPurchase}, purchaseArgs)

		_, vipArgs := demoAutomationCohortSQL(demoAutomationVIPConcierge)
		assert.Equal(t, []interface{}{demoGoalPurchase}, vipArgs)
	})

	t.Run("winback requires the last purchase to be stale", func(t *testing.T) {
		query, args := demoAutomationCohortSQL(demoAutomationWinback)
		assert.Contains(t, query, "HAVING MAX(occurred_at) < NOW() - INTERVAL '90 days'")
		assert.Equal(t, []interface{}{demoGoalPurchase}, args)
	})

	t.Run("unknown automation has no cohort query", func(t *testing.T) {
		query, args := demoAutomationCohortSQL("no-such-automation")
		assert.Equal(t, "", query)
		assert.Nil(t, args)
	})
}

func TestDemoAutomationOutcomesAndShares(t *testing.T) {
	ids := []string{
		demoAutomationWelcome, demoAutomationCartRecovery, demoAutomationPostPurchase,
		demoAutomationVIPConcierge, demoAutomationWinback,
	}
	t.Run("weights are percentages summing to 100 per automation", func(t *testing.T) {
		for _, id := range ids {
			total := 0
			for _, o := range demoAutomationOutcomes(id) {
				total += o.Weight
				assert.NotEmpty(t, o.Path, "%s has an empty journey path", id)
				assert.True(t, o.Status == domain.ContactAutomationStatusCompleted ||
					o.Status == domain.ContactAutomationStatusFailed,
					"%s outcome has non-terminal status %s", id, o.Status)
			}
			assert.Equal(t, 100, total, "%s outcome weights must sum to 100", id)
		}
		assert.Nil(t, demoAutomationOutcomes("no-such-automation"))
	})

	t.Run("only flows with a guarded marketing email have an exit path", func(t *testing.T) {
		assert.NotNil(t, demoAutomationExitPath(demoAutomationWelcome))
		assert.NotNil(t, demoAutomationExitPath(demoAutomationCartRecovery))
		assert.NotNil(t, demoAutomationExitPath(demoAutomationWinback))
		// Transactional thank-you and the no-email concierge flow can never
		// produce 'exited'; seeding exits there would fabricate a counter
		// a real workspace cannot have.
		assert.Nil(t, demoAutomationExitPath(demoAutomationPostPurchase))
		assert.Nil(t, demoAutomationExitPath(demoAutomationVIPConcierge))
	})

	t.Run("cohort shares cap enrolment except for goal-driven flows", func(t *testing.T) {
		assert.Equal(t, 0.42, demoAutomationCohortShare(demoAutomationWelcome))
		assert.Equal(t, 0.55, demoAutomationCohortShare(demoAutomationVIPConcierge))
		assert.Equal(t, 1.0, demoAutomationCohortShare(demoAutomationCartRecovery))
		assert.Equal(t, 1.0, demoAutomationCohortShare("no-such-automation"))
	})
}

func TestDemoDelayDuration(t *testing.T) {
	assert.Equal(t, 15*time.Minute, demoDelayDuration(map[string]interface{}{"duration": 15, "unit": "minutes"}))
	assert.Equal(t, 4*time.Hour, demoDelayDuration(map[string]interface{}{"duration": 4, "unit": "hours"}))
	assert.Equal(t, 3*24*time.Hour, demoDelayDuration(map[string]interface{}{"duration": 3, "unit": "days"}))
	// Missing or malformed config falls back to a minute, never to zero
	// (a zero-length delay would put later nodes before their trigger).
	assert.Equal(t, time.Minute, demoDelayDuration(map[string]interface{}{}))
	assert.Equal(t, time.Minute, demoDelayDuration(map[string]interface{}{"duration": 0, "unit": "days"}))
	assert.Equal(t, 5*time.Minute, demoDelayDuration(map[string]interface{}{"duration": 5, "unit": "fortnights"}))
}

func TestBuildDemoNodeVisits(t *testing.T) {
	automations := DemoAutomations("ws-demo")
	var welcome *domain.Automation
	for _, a := range automations {
		if a.ID == demoAutomationWelcome {
			welcome = a
		}
	}
	require.NotNil(t, welcome)
	rng := rand.New(rand.NewSource(1))
	entered := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("a path through a missing node yields nothing rather than partial rows", func(t *testing.T) {
		assert.Nil(t, buildDemoNodeVisits(welcome,
			[]string{"ws-trigger", "ghost-node"}, entered, false,
			domain.ContactAutomationStatusCompleted, rng))
	})

	t.Run("delay nodes take their configured duration", func(t *testing.T) {
		visits := buildDemoNodeVisits(welcome,
			[]string{"ws-trigger", "ws-delay-15m"}, entered, false,
			domain.ContactAutomationStatusCompleted, rng)
		require.Len(t, visits, 3) // entered + trigger visit + delay visit
		delay := visits[2]
		assert.Equal(t, "ws-delay-15m", delay.NodeID)
		assert.Equal(t, 15*time.Minute, delay.CompletedAt.Sub(delay.EnteredAt),
			"a delay node's elapsed time is the delay itself, not processing time")
		assert.Equal(t, 15*60*1000, delay.DurationMs)
		// Visits are contiguous: each node starts when the previous finished.
		assert.Equal(t, visits[1].CompletedAt, delay.EnteredAt)
		assert.Equal(t, entered, visits[0].EnteredAt)
	})

	t.Run("only the last visit can fail", func(t *testing.T) {
		visits := buildDemoNodeVisits(welcome,
			[]string{"ws-trigger", "ws-delay-15m", "ws-email-welcome"}, entered, true,
			domain.ContactAutomationStatusFailed, rng)
		require.Len(t, visits, 4)
		for _, v := range visits[:len(visits)-1] {
			assert.NotEqual(t, domain.NodeActionFailed, v.Action)
		}
		assert.Equal(t, domain.NodeActionFailed, visits[len(visits)-1].Action)
		assert.Equal(t, "ws-email-welcome", visits[len(visits)-1].NodeID)
	})
}

func TestDemoAutomationStats(t *testing.T) {
	stats := demoAutomationStats([]demoJourney{
		{Status: domain.ContactAutomationStatusCompleted},
		{Status: domain.ContactAutomationStatusCompleted},
		{Status: domain.ContactAutomationStatusFailed},
		{Status: domain.ContactAutomationStatusExited},
	})
	assert.Equal(t, domain.AutomationStats{
		Enrolled: 4, Completed: 2, Exited: 1, Failed: 1,
	}, stats)
}

func TestSeedDemoAutomationHistory_Orchestration(t *testing.T) {
	ctx := context.Background()
	newSvc := func(wsRepo *mocks.MockWorkspaceRepository) *DemoService {
		return &DemoService{
			logger:        logger.NewLoggerWithLevel("disabled"),
			workspaceRepo: wsRepo,
		}
	}

	t.Run("no automations means no database work", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := newSvc(mocks.NewMockWorkspaceRepository(ctrl))
		require.NoError(t, svc.seedDemoAutomationHistory(ctx, "ws-1", nil))
	})

	t.Run("connection failure is wrapped", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
		sentinel := errors.New("no pool")
		wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(nil, sentinel).Times(1)
		err := newSvc(wsRepo).seedDemoAutomationHistory(ctx, "ws-1", DemoAutomations("ws-1"))
		require.ErrorContains(t, err, "failed to get workspace connection:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("a failing cohort query skips that automation, not the whole seed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil).Times(1)
		mock.ExpectQuery(regexp.QuoteMeta("FROM contact_lists")).
			WillReturnError(errors.New("table missing"))

		welcomeOnly := []*domain.Automation{{ID: demoAutomationWelcome}}
		require.NoError(t, newSvc(wsRepo).seedDemoAutomationHistory(ctx, "ws-1", welcomeOnly),
			"cohort failures are per-automation warnings")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("loadDemoCohort scans members in order", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(regexp.QuoteMeta("FROM contact_lists")).
			WillReturnRows(sqlmock.NewRows([]string{"email", "status"}).
				AddRow("a@example.com", "active").
				AddRow("b@example.com", "unsubscribed"))

		svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
		cohort, err := svc.loadDemoCohort(ctx, db, demoAutomationWelcome)
		require.NoError(t, err)
		assert.Equal(t, []demoCohortMember{
			{Email: "a@example.com", ListStatus: "active"},
			{Email: "b@example.com", ListStatus: "unsubscribed"},
		}, cohort)
	})
}
