package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createSampleAutomations only logs a warning when an automation is rejected, and addSampleData
// treats the whole step as non-fatal, so a malformed flow leaves the demo silently missing the
// feature it exists to show. Worse, a filter or branch node's conditions tree is never validated
// anywhere — not by the node, not by the parser — so a malformed one would ship and only fail years
// later, at execution, on someone else's install.
func TestDemoAutomations_AreUsable(t *testing.T) {
	automations := demoAutomations("ws1")
	require.NotEmpty(t, automations)

	qb := NewQueryBuilder()
	seededTemplates := demoSeededTemplateIDs()
	seededLists := map[string]bool{demoListNewsletter: true, demoListVIPClub: true}

	for _, automation := range automations {
		t.Run(automation.ID, func(t *testing.T) {
			require.NoError(t, automation.Validate(), "automation does not validate")
			require.NotNil(t, automation.Trigger)
			require.NoError(t, automation.Trigger.Validate(), "trigger does not validate")
			assert.Contains(t, domain.ValidEventKinds, automation.Trigger.EventKind)

			// Conditions are left nil on purpose: they compile to correlated subqueries that a trigger
			// WHEN clause cannot hold, and none of these flows needs one.
			assert.Nil(t, automation.Trigger.Conditions)

			byID := map[string]*domain.AutomationNode{}
			for _, node := range automation.Nodes {
				require.NoError(t, node.Validate(), "node %s does not validate", node.ID)
				require.NoError(t, node.ValidateForAutomation(automation), "node %s is not valid for its automation", node.ID)

				// Both have been forgotten in drafts of this seeder, and both are rejected only at
				// save time — where the error is swallowed.
				assert.Equal(t, automation.ID, node.AutomationID, "node %s carries the wrong automation_id", node.ID)
				assert.NotNil(t, node.Config, "node %s has a nil config", node.ID)

				// branch has no editor component, no add-menu entry and no config form: seeding one
				// would show a prospect a node they cannot open.
				assert.NotEqual(t, domain.NodeTypeBranch, node.Type, "node %s is a branch", node.ID)

				byID[node.ID] = node
			}

			require.NotEmpty(t, automation.RootNodeID)
			require.NotNil(t, automation.GetNodeByID(automation.RootNodeID), "root node does not exist")

			for _, node := range automation.Nodes {
				assertDemoNodeConfigValidates(t, node)

				if node.NextNodeID != nil {
					assert.NotNil(t, byID[*node.NextNodeID], "node %s points at missing node %s", node.ID, *node.NextNodeID)
				}
				for _, target := range demoNodeTargets(node) {
					assert.NotNil(t, byID[target], "node %s points at missing node %s", node.ID, target)
				}
				if listID := demoNodeListID(node); listID != "" {
					assert.True(t, seededLists[listID], "node %s references list %q, which the demo does not seed", node.ID, listID)
				}
				if templateID := demoNodeTemplateID(node); templateID != "" {
					assert.True(t, seededTemplates[templateID], "node %s references template %q, which the demo does not seed", node.ID, templateID)
				}
				if tree := demoNodeConditions(node); tree != nil {
					require.NoError(t, tree.Validate(), "node %s has a conditions tree that does not validate", node.ID)
					_, _, err := qb.BuildSQL(tree)
					require.NoError(t, err, "node %s has a conditions tree that does not compile", node.ID)
				}
			}

			// Every node has to be reachable from the root, or the editor draws an island.
			reachable := demoReachableNodes(automation)
			for _, node := range automation.Nodes {
				assert.True(t, reachable[node.ID], "node %s is unreachable from the root", node.ID)
			}

			// Identical positions open the editable flows as a pile of overlapping boxes: the editor
			// renders stored positions verbatim and only re-layouts on an explicit user action.
			seen := map[string]bool{}
			for _, node := range automation.Nodes {
				key := fmt.Sprintf("%.0f:%.0f", node.Position.X, node.Position.Y)
				assert.False(t, seen[key], "node %s shares a position with another node", node.ID)
				seen[key] = true
			}
		})
	}
}

// An automation carrying an email node cannot be activated unless the automation itself has a list,
// and the check runs at Activate — after the row is already written.
func TestDemoAutomations_EmailFlowsCarryAList(t *testing.T) {
	for _, automation := range demoAutomations("ws1") {
		hasEmail := false
		for _, node := range automation.Nodes {
			if node.Type == domain.NodeTypeEmail {
				hasEmail = true
			}
		}
		if hasEmail {
			assert.False(t, automation.HasEmailNodeRestriction(),
				"%s has an email node but no list, so it can never be activated", automation.ID)
		}
	}
}

// Writing status=live directly inserts a row that displays as Live with no trigger behind it, and
// Activate then refuses because it is already live — an automation that can never fire and cannot be
// repaired. The two flows destined to go live are created draft and activated; the rest are created
// paused, which is as inert as draft and is the honest badge for a card carrying journey history.
func TestDemoAutomations_ShipInTheRightStatus(t *testing.T) {
	live := map[string]bool{}
	for _, id := range demoLiveAutomations {
		live[id] = true
	}

	for _, automation := range demoAutomations("ws1") {
		assert.NotEqual(t, domain.AutomationStatusLive, automation.Status,
			"%s is created live, which installs no trigger and cannot be activated afterwards", automation.ID)

		if live[automation.ID] {
			assert.Equal(t, domain.AutomationStatusDraft, automation.Status,
				"%s is activated later, so it must be created draft", automation.ID)
		} else {
			assert.Equal(t, domain.AutomationStatusPaused, automation.Status,
				"%s is never activated, so it must be created paused", automation.ID)
		}
	}
}

// Only automations whose trigger event stops firing once the seed is finished may go live. The
// others would enroll a burst of contacts who then sit at the trigger node forever, because the
// scheduler is disabled in demo.
func TestDemoAutomations_OnlyCustomEventFlowsGoLive(t *testing.T) {
	byID := map[string]*domain.Automation{}
	for _, automation := range demoAutomations("ws1") {
		byID[automation.ID] = automation
	}

	require.NotEmpty(t, demoLiveAutomations)
	for _, id := range demoLiveAutomations {
		automation, ok := byID[id]
		require.True(t, ok, "%s is marked live but is not seeded", id)
		assert.Equal(t, "custom_event", automation.Trigger.EventKind,
			"%s triggers on %s, which keeps firing after the seed returns", id, automation.Trigger.EventKind)
	}
}

// The list ordering is created_at DESC with no secondary key and the page renders in response order,
// so creating them in narrative order would tell the story backwards.
func TestDemoAutomations_AreCreatedInReverseDisplayOrder(t *testing.T) {
	automations := demoAutomations("ws1")
	require.NotEmpty(t, automations)
	assert.Equal(t, demoAutomationWelcome, automations[len(automations)-1].ID,
		"the welcome flow is created last so it lists first")
}

func TestCreateSampleAutomations(t *testing.T) {
	newService := func(t *testing.T, environment string) (*DemoService, *domainmocks.MockAutomationService) {
		t.Helper()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockAutomationService := domainmocks.NewMockAutomationService(ctrl)
		mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
		// No workspace database in a unit test, so history seeding bails out early and loudly enough
		// for the log, which is exactly what it does in production when the connection is gone.
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("no database in this test")).
			AnyTimes()

		return &DemoService{
			logger:            logger.NewLoggerWithLevel("disabled"),
			config:            &config.Config{Environment: environment},
			automationService: mockAutomationService,
			workspaceRepo:     mockWorkspaceRepo,
		}, mockAutomationService
	}

	t.Run("creates every automation and activates only the live ones in demo mode", func(t *testing.T) {
		svc, mockAutomationService := newService(t, "demo")

		var created []*domain.Automation
		mockAutomationService.EXPECT().
			Create(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, a *domain.Automation) error {
				created = append(created, a)
				return nil
			}).
			AnyTimes()

		var activated []string
		mockAutomationService.EXPECT().
			Activate(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, id string) error {
				activated = append(activated, id)
				return nil
			}).
			AnyTimes()

		require.NoError(t, svc.createSampleAutomations(context.Background(), "ws1"))

		require.Len(t, created, len(demoAutomations("ws1")))
		assert.ElementsMatch(t, demoLiveAutomations, activated)
	})

	// The demo reset endpoint is registered on any non-production environment, but the scheduler and
	// the email queue worker only stop when ENVIRONMENT=demo. A live automation on a development
	// instance would enroll, advance and enqueue real sends.
	t.Run("activates nothing outside demo mode, but still creates the flows", func(t *testing.T) {
		svc, mockAutomationService := newService(t, "development")

		created := 0
		mockAutomationService.EXPECT().
			Create(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ *domain.Automation) error {
				created++
				return nil
			}).
			AnyTimes()

		mockAutomationService.EXPECT().Activate(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		require.NoError(t, svc.createSampleAutomations(context.Background(), "ws1"))
		assert.Equal(t, len(demoAutomations("ws1")), created)
	})

	t.Run("a failed automation is skipped and never activated", func(t *testing.T) {
		svc, mockAutomationService := newService(t, "demo")

		// The first live automation fails to create; history and activation must skip it, because
		// contact_automations.automation_id is a NOT NULL foreign key onto a row that does not exist.
		failing := demoLiveAutomations[0]
		created := 0
		mockAutomationService.EXPECT().
			Create(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, a *domain.Automation) error {
				if a.ID == failing {
					return errors.New("rejected")
				}
				created++
				return nil
			}).
			AnyTimes()

		var activated []string
		mockAutomationService.EXPECT().
			Activate(gomock.Any(), "ws1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, id string) error {
				activated = append(activated, id)
				return nil
			}).
			AnyTimes()

		require.NoError(t, svc.createSampleAutomations(context.Background(), "ws1"))

		assert.Equal(t, len(demoAutomations("ws1"))-1, created, "the other automations are still created")
		assert.NotContains(t, activated, failing)
	})

	t.Run("does nothing when no automation service is wired", func(t *testing.T) {
		svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled"), config: &config.Config{Environment: "demo"}}
		require.NoError(t, svc.createSampleAutomations(context.Background(), "ws1"))
	})
}

// demoSeededTemplateIDs is the set of template IDs the demo really creates: the four automation
// templates plus the four the general template seeder writes.
func demoSeededTemplateIDs() map[string]bool {
	ids := map[string]bool{
		"newsletter-weekly":    true,
		"newsletter-weekly-v2": true,
		"welcome-email":        true,
		"password-reset":       true,
	}
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
	for _, template := range svc.demoAutomationTemplates("ws1") {
		ids[template.ID] = true
	}
	return ids
}

func assertDemoNodeConfigValidates(t *testing.T, node *domain.AutomationNode) {
	t.Helper()

	switch node.Type {
	case domain.NodeTypeDelay:
		cfg := demoDecodeNodeConfig[domain.DelayNodeConfig](t, node)
		require.NoError(t, cfg.Validate(), "node %s", node.ID)
	case domain.NodeTypeEmail:
		cfg := demoDecodeNodeConfig[domain.EmailNodeConfig](t, node)
		require.NoError(t, cfg.Validate(), "node %s", node.ID)
	case domain.NodeTypeAddToList:
		cfg := demoDecodeNodeConfig[domain.AddToListNodeConfig](t, node)
		require.NoError(t, cfg.Validate(), "node %s", node.ID)
	case domain.NodeTypeRemoveFromList:
		cfg := demoDecodeNodeConfig[domain.RemoveFromListNodeConfig](t, node)
		require.NoError(t, cfg.Validate(), "node %s", node.ID)
	case domain.NodeTypeABTest:
		cfg := demoDecodeNodeConfig[domain.ABTestNodeConfig](t, node)
		require.NoError(t, cfg.Validate(), "node %s", node.ID)
	case domain.NodeTypeListStatusBranch:
		cfg := demoDecodeNodeConfig[domain.ListStatusBranchNodeConfig](t, node)
		require.NoError(t, cfg.Validate(), "node %s", node.ID)
	case domain.NodeTypeWebhook:
		cfg := demoDecodeNodeConfig[domain.WebhookNodeConfig](t, node)
		require.NoError(t, cfg.Validate(), "node %s", node.ID)
	}
}

// demoDecodeNodeConfig round-trips a node's untyped config through JSON, which is exactly how the
// executor reads it — so a config that decodes here decodes there.
func demoDecodeNodeConfig[T any](t *testing.T, node *domain.AutomationNode) T {
	t.Helper()
	raw, err := json.Marshal(node.Config)
	require.NoError(t, err, "node %s config does not marshal", node.ID)
	var cfg T
	require.NoError(t, json.Unmarshal(raw, &cfg), "node %s config does not decode", node.ID)
	return cfg
}

// demoNodeTargets returns the node ids a node routes to through its config rather than NextNodeID.
func demoNodeTargets(node *domain.AutomationNode) []string {
	var targets []string
	add := func(key string) {
		if value, ok := node.Config[key].(string); ok && value != "" {
			targets = append(targets, value)
		}
	}
	switch node.Type {
	case domain.NodeTypeFilter:
		add("continue_node_id")
		add("exit_node_id")
	case domain.NodeTypeListStatusBranch:
		add("active_node_id")
		add("non_active_node_id")
		add("not_in_list_node_id")
	case domain.NodeTypeABTest:
		if variants, ok := node.Config["variants"].([]domain.ABTestVariant); ok {
			for _, variant := range variants {
				if variant.NextNodeID != "" {
					targets = append(targets, variant.NextNodeID)
				}
			}
		}
	}
	return targets
}

func demoNodeListID(node *domain.AutomationNode) string {
	switch node.Type {
	case domain.NodeTypeAddToList, domain.NodeTypeRemoveFromList, domain.NodeTypeListStatusBranch:
		listID, _ := node.Config["list_id"].(string)
		return listID
	}
	return ""
}

func demoNodeTemplateID(node *domain.AutomationNode) string {
	if node.Type != domain.NodeTypeEmail {
		return ""
	}
	templateID, _ := node.Config["template_id"].(string)
	return templateID
}

func demoNodeConditions(node *domain.AutomationNode) *domain.TreeNode {
	if node.Type != domain.NodeTypeFilter && node.Type != domain.NodeTypeBranch {
		return nil
	}
	tree, _ := node.Config["conditions"].(*domain.TreeNode)
	return tree
}

func demoReachableNodes(automation *domain.Automation) map[string]bool {
	reachable := map[string]bool{}
	queue := []string{automation.RootNodeID}

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if id == "" || reachable[id] {
			continue
		}
		node := automation.GetNodeByID(id)
		if node == nil {
			continue
		}
		reachable[id] = true

		if node.NextNodeID != nil {
			queue = append(queue, *node.NextNodeID)
		}
		queue = append(queue, demoNodeTargets(node)...)
	}
	return reachable
}
