package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestSegmentGoalNegation exercises "has not done X in the last N days" through the FULL segment
// service (/api/segments.preview: auth -> validation -> SQL build -> execution against the
// workspace DB), for the two shapes issue #399 asked for: negated goal conditions and the
// not_in_the_last_days operator on a contact datetime property.
//
// It also pins down why negation had to wrap the leaf: the pre-existing "count <= 0" spelling is
// asserted to match nobody, because the aggregation is an EXISTS subquery grouped by email and a
// contact with no matching events produces no group at all.
func TestSegmentGoalNegation(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceID := workspace.ID

	user, err := factory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspaceID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspaceID)

	workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)
	ctx := context.Background()

	// Three audiences that behave differently under negation:
	//   recent  — purchased 3 days ago
	//   lapsed  — purchased 100 days ago, the promo target
	//   never   — no events at all, also a promo target and the one an EXISTS can never see
	for _, email := range []string{"recent@goal.com", "lapsed@goal.com", "never@goal.com"} {
		_, err = factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
		require.NoError(t, err)
	}

	seedPurchase := func(email string, daysAgo int, sku string, amount float64) {
		_, err := workspaceDB.ExecContext(ctx, `
			INSERT INTO custom_events
				(event_name, external_id, email, properties, occurred_at, source,
				 goal_name, goal_type, goal_value)
			VALUES ($1, $2, $3, $4, $5, 'test', 'checkout', 'purchase', $6)`,
			"shopify.order",
			fmt.Sprintf("ext_%s_%d", strings.Split(email, "@")[0], daysAgo),
			email,
			fmt.Sprintf(`{"sku": %q}`, sku),
			time.Now().UTC().AddDate(0, 0, -daysAgo),
			amount,
		)
		require.NoError(t, err)
	}

	seedPurchase("recent@goal.com", 3, "A-1", 50)
	seedPurchase("lapsed@goal.com", 100, "B-2", 80)

	// previewEmails runs a leaf through the real segment service and returns the contacts it
	// matches. The preview endpoint withholds the email list for privacy and reports only a
	// count, so identity is recovered by executing the very SQL the service compiled — which
	// still proves the whole chain (validation -> BuildSQL -> execution) and, unlike a bare
	// count, distinguishes "lapsed + never" from any other pair.
	previewEmails := func(t *testing.T, leaf map[string]interface{}) []string {
		resp, err := client.Post("/api/segments.preview", map[string]interface{}{
			"workspace_id": workspaceID,
			"tree":         map[string]interface{}{"kind": "leaf", "leaf": leaf},
			"limit":        100,
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode, "preview must succeed")

		var result struct {
			TotalCount   int           `json:"total_count"`
			GeneratedSQL string        `json:"generated_sql"`
			SQLArgs      []interface{} `json:"sql_args"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

		rows, err := workspaceDB.QueryContext(ctx, result.GeneratedSQL, result.SQLArgs...)
		require.NoError(t, err, "the compiled query must execute: %s", result.GeneratedSQL)
		defer func() { _ = rows.Close() }()

		emails := []string{}
		for rows.Next() {
			var email string
			require.NoError(t, rows.Scan(&email))
			emails = append(emails, email)
		}
		require.NoError(t, rows.Err())
		require.Len(t, emails, result.TotalCount, "the service count must agree with its own query")
		return emails
	}

	previewGoal := func(t *testing.T, goal map[string]interface{}) []string {
		return previewEmails(t, map[string]interface{}{
			"source": "custom_events_goals", "custom_events_goal": goal,
		})
	}

	previewContactEmails := func(t *testing.T, filter map[string]interface{}) []string {
		return previewEmails(t, map[string]interface{}{
			"source":  "contacts",
			"contact": map[string]interface{}{"filters": []interface{}{filter}},
		})
	}

	purchasedInLast30 := func() map[string]interface{} {
		return map[string]interface{}{
			"goal_type":          "purchase",
			"aggregate_operator": "count",
			"operator":           "gte",
			"value":              1,
			"timeframe_operator": "in_the_last_days",
			"timeframe_values":   []string{"30"},
		}
	}

	t.Run("positive condition is unchanged", func(t *testing.T) {
		require.ElementsMatch(t, []string{"recent@goal.com"}, previewGoal(t, purchasedInLast30()))
	})

	t.Run("negated condition matches lapsed AND never-purchased contacts", func(t *testing.T) {
		goal := purchasedInLast30()
		goal["negate"] = true

		// never@goal.com is the case that motivated wrapping the leaf: no custom_events row at
		// all, so no group, so no EXISTS — only NOT EXISTS reaches them.
		require.ElementsMatch(t,
			[]string{"lapsed@goal.com", "never@goal.com"},
			previewGoal(t, goal))
	})

	t.Run("count <= 0 still matches nobody, which is why negate exists", func(t *testing.T) {
		goal := purchasedInLast30()
		goal["operator"] = "lte"
		goal["value"] = 0

		require.Empty(t, previewGoal(t, goal),
			"an aggregate grouped by email can never observe zero rows")
	})

	t.Run("negation composes with a property filter", func(t *testing.T) {
		goal := map[string]interface{}{
			"goal_type":          "purchase",
			"aggregate_operator": "count",
			"operator":           "gte",
			"value":              1,
			"timeframe_operator": "anytime",
			"negate":             true,
			"filters": []interface{}{
				map[string]interface{}{
					"field_name":    "sku",
					"field_type":    "string",
					"operator":      "equals",
					"string_values": []string{"A-1"},
				},
			},
		}

		// Only recent bought A-1, so everyone else "has not bought A-1".
		require.ElementsMatch(t,
			[]string{"lapsed@goal.com", "never@goal.com"},
			previewGoal(t, goal))
	})

	t.Run("event_name narrows beyond the goal type", func(t *testing.T) {
		goal := map[string]interface{}{
			"goal_type":          "purchase",
			"event_name":         "stripe.payment",
			"aggregate_operator": "count",
			"operator":           "gte",
			"value":              1,
			"timeframe_operator": "anytime",
		}

		require.Empty(t, previewGoal(t, goal), "nobody has a stripe.payment event")
	})

	t.Run("a timeline change key cannot inject SQL", func(t *testing.T) {
		// This is the path that actually shipped interpolated: contact_timeline filters spliced
		// field_name into the SQL text. The event-property equivalent below is new code that was
		// never vulnerable, so covering only that would leave the real hole unexercised.
		emails := previewEmails(t, map[string]interface{}{
			"source": "contact_timeline",
			"contact_timeline": map[string]interface{}{
				"kind":           "custom_event.shopify.order",
				"count_operator": "at_least",
				"count_value":    1,
				"filters": []interface{}{
					map[string]interface{}{
						"field_name":    `goal_type'->>'new' = (SELECT email FROM contacts LIMIT 1) -- `,
						"field_type":    "string",
						"operator":      "equals",
						"string_values": []string{"purchase"},
					},
				},
			},
		})

		// Bound as a value, the crafted key just names a change nobody recorded. Spliced into the
		// SQL text it parsed as a comparison against a sub-select, which the preview count then
		// leaked one boolean at a time.
		require.Empty(t, emails)
	})

	t.Run("an off-type property value does not abort the query", func(t *testing.T) {
		// custom_events.properties is an arbitrary caller payload, so nothing stops one event
		// from storing a non-date under a key others store dates under. An unguarded cast takes
		// the whole segment down, and only when the plan happens to scan that row — an
		// intermittent failure surfacing long after the event was written.
		_, err := workspaceDB.ExecContext(ctx, `
			INSERT INTO custom_events
				(event_name, external_id, email, properties, occurred_at, source, goal_type, goal_value)
			VALUES ('support.ticket', 'offtype', $1, '{"renewed_at": "soon"}', NOW(), 'test', 'lead', 1)`,
			"never@goal.com")
		require.NoError(t, err)

		emails := previewGoal(t, map[string]interface{}{
			"goal_type":          "*",
			"aggregate_operator": "count",
			"operator":           "gte",
			"value":              1,
			"timeframe_operator": "anytime",
			"filters": []interface{}{
				map[string]interface{}{
					"field_name":    "renewed_at",
					"field_type":    "time",
					"operator":      "not_in_the_last_days",
					"string_values": []string{"30"},
				},
			},
		})

		// "soon" is unusable as a date, so it reads as unset — which the NULL-inclusive operator
		// matches. What matters is that the query returned at all.
		require.Contains(t, emails, "never@goal.com")
	})

	t.Run("negation composes inside a branch", func(t *testing.T) {
		resp, err := client.Post("/api/segments.preview", map[string]interface{}{
			"workspace_id": workspaceID,
			"tree": map[string]interface{}{
				"kind": "branch",
				"branch": map[string]interface{}{
					"operator": "and",
					"leaves": []interface{}{
						map[string]interface{}{
							"kind": "leaf",
							"leaf": map[string]interface{}{
								"source": "custom_events_goals",
								"custom_events_goal": map[string]interface{}{
									"goal_type": "purchase", "aggregate_operator": "count",
									"operator": "gte", "value": 1,
									"timeframe_operator": "in_the_last_days",
									"timeframe_values":   []string{"30"},
									"negate":             true,
								},
							},
						},
						map[string]interface{}{
							"kind": "leaf",
							"leaf": map[string]interface{}{
								"source": "contacts",
								"contact": map[string]interface{}{
									"filters": []interface{}{map[string]interface{}{
										"field_name": "email", "field_type": "string",
										"operator": "contains", "string_values": []string{"lapsed"},
									}},
								},
							},
						},
					},
				},
			},
			"limit": 100,
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result struct {
			TotalCount int `json:"total_count"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		assert.Equal(t, 1, result.TotalCount,
			"only lapsed satisfies both the negated goal and the email filter")
	})

	t.Run("a soft-deleted event counts as not having happened", func(t *testing.T) {
		_, err := workspaceDB.ExecContext(ctx,
			`UPDATE custom_events SET deleted_at = NOW() WHERE email = $1`, "recent@goal.com")
		require.NoError(t, err)
		defer func() {
			_, err := workspaceDB.ExecContext(ctx,
				`UPDATE custom_events SET deleted_at = NULL WHERE email = $1`, "recent@goal.com")
			require.NoError(t, err)
		}()

		goal := purchasedInLast30()
		goal["negate"] = true

		// The subquery already excludes soft-deleted rows, so negating it must treat the recent
		// buyer as someone who has not purchased.
		require.Contains(t, previewGoal(t, goal), "recent@goal.com")
	})

	t.Run("a property key cannot inject SQL", func(t *testing.T) {
		goal := map[string]interface{}{
			"goal_type":          "purchase",
			"aggregate_operator": "count",
			"operator":           "gte",
			"value":              1,
			"timeframe_operator": "anytime",
			"filters": []interface{}{
				map[string]interface{}{
					"field_name":    `sku'->>'x' = (SELECT email FROM contacts LIMIT 1) -- `,
					"field_type":    "string",
					"operator":      "equals",
					"string_values": []string{"A-1"},
				},
			},
		}

		// The crafted key is bound as a value, so it simply names a property nobody has.
		// Before the fix this executed as SQL.
		require.Empty(t, previewGoal(t, goal))
	})

	t.Run("not_in_the_last_days includes contacts whose date was never set", func(t *testing.T) {
		_, err := workspaceDB.ExecContext(ctx,
			`UPDATE contacts SET custom_datetime_1 = $1 WHERE email = $2`,
			time.Now().UTC().AddDate(0, 0, -2), "recent@goal.com")
		require.NoError(t, err)
		_, err = workspaceDB.ExecContext(ctx,
			`UPDATE contacts SET custom_datetime_1 = $1 WHERE email = $2`,
			time.Now().UTC().AddDate(0, 0, -100), "lapsed@goal.com")
		require.NoError(t, err)
		// never@goal.com keeps custom_datetime_1 NULL on purpose.

		require.ElementsMatch(t,
			[]string{"lapsed@goal.com", "never@goal.com"},
			previewContactEmails(t, map[string]interface{}{
				"field_name":    "custom_datetime_1",
				"field_type":    "time",
				"operator":      "not_in_the_last_days",
				"string_values": []string{"30"},
			}),
			"a contact who never set the date has not set it in the last 30 days either")

		require.ElementsMatch(t,
			[]string{"recent@goal.com"},
			previewContactEmails(t, map[string]interface{}{
				"field_name":    "custom_datetime_1",
				"field_type":    "time",
				"operator":      "in_the_last_days",
				"string_values": []string{"30"},
			}))
	})
}

// TestCustomEventLongNameIsRecorded covers the v37 widening of contact_timeline.kind. The timeline
// trigger writes 'custom_event.' || event_name into that column, so before the widening any event
// name longer than 37 characters aborted the custom_events insert outright — the event could not
// be recorded at all.
func TestCustomEventLongNameIsRecorded(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail("long@event.com"))
	require.NoError(t, err)

	// 100 characters, the maximum domain.CustomEvent.Validate accepts.
	longName := strings.Repeat("a", 100)
	require.NoError(t,
		factory.CreateCustomEvent(workspace.ID, "long@event.com", longName, map[string]interface{}{"k": "v"}),
		"an event name at the documented maximum must be insertable")

	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	var kind string
	require.NoError(t, workspaceDB.QueryRowContext(context.Background(),
		`SELECT kind FROM contact_timeline WHERE email = $1 AND entity_type = 'custom_event'`,
		"long@event.com").Scan(&kind))

	require.Equal(t, "custom_event."+longName, kind, "the kind must be stored whole, not truncated")
}

// TestAutomationFilterNodeNegatedGoal runs the REAL FilterNodeExecutor — the production code
// path for an automation filter node — against a real workspace database. #399 asks for
// negation in automations as well as segments, and the automation path is a different SQL
// shape: it wraps the segment query as `SELECT EXISTS (<query> AND email = $n)`, so a leaf
// that compiles to `NOT EXISTS (...)` ends up nested inside an `EXISTS`.
func TestAutomationFilterNodeNegatedGoal(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	ctx := context.Background()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	for _, email := range []string{"buyer@auto.com", "lapsed@auto.com", "never@auto.com"} {
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
	}

	seed := func(email string, daysAgo int) {
		_, err := workspaceDB.ExecContext(ctx, `
			INSERT INTO custom_events
				(event_name, external_id, email, properties, occurred_at, source, goal_type, goal_value)
			VALUES ('shopify.order', $1, $2, '{}', $3, 'test', 'purchase', 10)`,
			fmt.Sprintf("auto_%s_%d", strings.Split(email, "@")[0], daysAgo),
			email, time.Now().UTC().AddDate(0, 0, -daysAgo))
		require.NoError(t, err)
	}
	seed("buyer@auto.com", 3)
	seed("lapsed@auto.com", 100)

	executor := service.NewFilterNodeExecutor(
		service.NewQueryBuilder(),
		suite.ServerManager.GetApp().GetWorkspaceRepository(),
	)

	continueID := "pass-node"
	exitID := "fail-node"

	// "has NOT purchased in the last 30 days"
	node := &domain.AutomationNode{
		ID:   "filter-node",
		Type: domain.NodeTypeFilter,
		Config: map[string]interface{}{
			"conditions": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "custom_events_goals",
					"custom_events_goal": map[string]interface{}{
						"goal_type":          "purchase",
						"aggregate_operator": "count",
						"operator":           "gte",
						"value":              1,
						"timeframe_operator": "in_the_last_days",
						"timeframe_values":   []string{"30"},
						"negate":             true,
					},
				},
			},
			"continue_node_id": continueID,
			"exit_node_id":     exitID,
		},
	}

	passed := func(t *testing.T, email string) bool {
		result, err := executor.Execute(ctx, service.NodeExecutionParams{
			WorkspaceID: workspace.ID,
			Node:        node,
			ContactData: &domain.Contact{Email: email},
		})
		require.NoError(t, err)
		require.NotNil(t, result.NextNodeID)
		return *result.NextNodeID == continueID
	}

	t.Run("recent buyer fails the negated filter", func(t *testing.T) {
		require.False(t, passed(t, "buyer@auto.com"))
	})

	t.Run("lapsed buyer passes", func(t *testing.T) {
		require.True(t, passed(t, "lapsed@auto.com"))
	})

	t.Run("contact with no events at all passes", func(t *testing.T) {
		// The case an EXISTS can never reach, nested inside the automation's own EXISTS wrapper.
		require.True(t, passed(t, "never@auto.com"))
	})
}

// TestSegmentRecomputeSchedulingForRelativeOperators covers the failure mode that would make the
// new operator look like it works and then quietly stop: membership of a "not in the last N days"
// segment moves with the clock, not with any event, so nothing re-evaluates it unless the segment
// is flagged as relative and given a recompute_after. A segment that misses that flag simply
// freezes at the day it was saved, with no error anywhere.
func TestSegmentRecomputeSchedulingForRelativeOperators(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	user, err := factory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	ctx := context.Background()

	createSegment := func(t *testing.T, id string, leaf map[string]interface{}) {
		resp, err := client.Post("/api/segments.create", map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           id,
			"name":         "Segment " + id,
			"color":        "#FF5733",
			"timezone":     "UTC",
			"tree":         map[string]interface{}{"kind": "leaf", "leaf": leaf},
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	recomputeAfter := func(t *testing.T, id string) sql.NullTime {
		var at sql.NullTime
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT recompute_after FROM segments WHERE id = $1`, id).Scan(&at))
		return at
	}

	t.Run("a not_in_the_last_days contact filter is scheduled", func(t *testing.T) {
		createSegment(t, "relnotlast", map[string]interface{}{
			"source": "contacts",
			"contact": map[string]interface{}{
				"filters": []map[string]interface{}{{
					"field_name":    "custom_datetime_1",
					"field_type":    "time",
					"operator":      "not_in_the_last_days",
					"string_values": []string{"30"},
				}},
			},
		})

		at := recomputeAfter(t, "relnotlast")
		require.True(t, at.Valid, "a relative-date segment must carry a recompute schedule")
		assert.True(t, at.Time.After(time.Now()), "the schedule must be in the future")
	})

	t.Run("a relative operator inside goal property filters is scheduled", func(t *testing.T) {
		createSegment(t, "relgoalprop", map[string]interface{}{
			"source": "custom_events_goals",
			"custom_events_goal": map[string]interface{}{
				"goal_type":          "purchase",
				"aggregate_operator": "count",
				"operator":           "gte",
				"value":              1,
				"timeframe_operator": "anytime",
				"negate":             true,
				"filters": []map[string]interface{}{{
					"field_name":    "renewed_at",
					"field_type":    "time",
					"operator":      "not_in_the_last_days",
					"string_values": []string{"7"},
				}},
			},
		})

		require.True(t, recomputeAfter(t, "relgoalprop").Valid,
			"a goal property filter on a rolling window is just as time-dependent")
	})

	t.Run("an absolute date filter is not scheduled", func(t *testing.T) {
		createSegment(t, "absdate", map[string]interface{}{
			"source": "contacts",
			"contact": map[string]interface{}{
				"filters": []map[string]interface{}{{
					"field_name":    "custom_datetime_1",
					"field_type":    "time",
					"operator":      "before_date",
					"string_values": []string{"2026-01-01T00:00:00Z"},
				}},
			},
		})

		assert.False(t, recomputeAfter(t, "absdate").Valid,
			"a fixed date never changes meaning, so daily recomputation would be waste")
	})
}

// TestNegatedSegmentBuildsMembership closes the last gap in the negation path: everything else
// exercises preview (a COUNT over the compiled query) or the incremental queue. This drives the
// segment BUILD processor, which is what actually writes contact_segments — and which reaches the
// stored query differently, appending "AND email = ANY($n)" per batch rather than wrapping it.
func TestNegatedSegmentBuildsMembership(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient
	ctx := context.Background()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	user, err := factory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	for _, email := range []string{"recent@build.com", "lapsed@build.com", "never@build.com"} {
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
	}
	for _, seed := range []struct {
		email   string
		daysAgo int
	}{{"recent@build.com", 3}, {"lapsed@build.com", 100}} {
		_, err = workspaceDB.ExecContext(ctx, `
			INSERT INTO custom_events
				(event_name, external_id, email, properties, occurred_at, source, goal_type, goal_value)
			VALUES ('shopify.order', $1, $2, '{}', $3, 'test', 'purchase', 10)`,
			"build_"+seed.email, seed.email, time.Now().UTC().AddDate(0, 0, -seed.daysAgo))
		require.NoError(t, err)
	}

	segmentID := "notpurchased"
	resp, err := client.Post("/api/segments.create", map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           segmentID,
		"name":         "Has not purchased in 30 days",
		"color":        "#FF5733",
		"timezone":     "UTC",
		"tree": map[string]interface{}{
			"kind": "leaf",
			"leaf": map[string]interface{}{
				"source": "custom_events_goals",
				"custom_events_goal": map[string]interface{}{
					"goal_type":          "purchase",
					"aggregate_operator": "count",
					"operator":           "gte",
					"value":              1,
					"timeframe_operator": "in_the_last_days",
					"timeframe_values":   []string{"30"},
					"negate":             true,
				},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	rebuildResp, err := client.Post("/api/segments.rebuild", map[string]interface{}{
		"workspace_id": workspace.ID,
		"segment_id":   segmentID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rebuildResp.StatusCode)
	_ = rebuildResp.Body.Close()

	execResp, err := client.ExecuteTask(map[string]interface{}{"limit": 10})
	require.NoError(t, err)
	_ = execResp.Body.Close()

	_, err = testutil.WaitForSegmentBuilt(t, client, workspace.ID, segmentID, 30*time.Second)
	require.NoError(t, err)

	rows, err := workspaceDB.QueryContext(ctx,
		`SELECT email FROM contact_segments WHERE segment_id = $1 ORDER BY email`, segmentID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var members []string
	for rows.Next() {
		var email string
		require.NoError(t, rows.Scan(&email))
		members = append(members, email)
	}
	require.NoError(t, rows.Err())

	// Materialized membership, not a preview count: the lapsed buyer and the contact with no
	// events at all, and not the recent buyer.
	assert.ElementsMatch(t, []string{"lapsed@build.com", "never@build.com"}, members)
}

// TestNegatedSegmentPartitionsContacts pins the invariant that makes a negated goal condition
// self-checking: the segment and its mirror image (identical, un-negated) must partition the whole
// contact list. No contact can satisfy both, and none can escape both — including contacts with no
// matching events at all, which are exactly the ones the aggregation alone cannot see.
//
// This is the property the demo workspace's "Win-back Opportunities" segment relies on to be an
// honest showcase, so a drift here would quietly turn that demo into a lie.
func TestNegatedSegmentPartitionsContacts(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient
	ctx := context.Background()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	user, err := factory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// A spread mirroring the demo generator: some recent buyers, some lapsed, and a share with no
	// purchase history at all (the demo skips ~30% of contacts).
	seed := []struct {
		email   string
		daysAgo int // -1 means no purchase event
	}{
		{"p1@part.com", 5}, {"p2@part.com", 40}, {"p3@part.com", 120},
		{"p4@part.com", 200}, {"p5@part.com", -1}, {"p6@part.com", -1},
		{"p7@part.com", 89}, {"p8@part.com", 91},
	}
	for _, s := range seed {
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(s.email))
		require.NoError(t, err)
		if s.daysAgo < 0 {
			continue
		}
		_, err = workspaceDB.ExecContext(ctx, `
			INSERT INTO custom_events
				(event_name, external_id, email, properties, occurred_at, source, goal_type, goal_value)
			VALUES ('purchase', $1, $2, '{}', $3, 'demo', 'purchase', 25)`,
			"part_"+s.email, s.email, time.Now().UTC().AddDate(0, 0, -s.daysAgo))
		require.NoError(t, err)
	}

	var totalContacts int
	require.NoError(t, workspaceDB.QueryRowContext(ctx, `SELECT count(*) FROM contacts`).Scan(&totalContacts))

	previewCount := func(t *testing.T, negate bool) int {
		resp, err := client.Post("/api/segments.preview", map[string]interface{}{
			"workspace_id": workspace.ID,
			"tree": map[string]interface{}{
				"kind": "leaf",
				"leaf": map[string]interface{}{
					"source": "custom_events_goals",
					"custom_events_goal": map[string]interface{}{
						"goal_type":          "purchase",
						"aggregate_operator": "count",
						"operator":           "gte",
						"value":              1,
						"timeframe_operator": "in_the_last_days",
						"timeframe_values":   []string{"90"},
						"negate":             negate,
					},
				},
			},
			"limit": 100,
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result struct {
			TotalCount int `json:"total_count"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		return result.TotalCount
	}

	purchased := previewCount(t, false)
	winback := previewCount(t, true)

	// 5, 40, 89 days ago are inside the window; 91, 120, 200 and the two with no events are not.
	assert.Equal(t, 3, purchased)
	assert.Equal(t, 5, winback)
	assert.Equal(t, totalContacts, purchased+winback,
		"the negated segment and its mirror must cover every contact exactly once")
}
