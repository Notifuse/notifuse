package service

import (
	"context"

	"github.com/Notifuse/notifuse/internal/domain"
)

// The demo workspace's showcase automations.
//
// Five flows that between them use every node kind the console can actually open — branch is left
// out deliberately, because NodeConfigPanel has no form for it and renders "Configuration for branch
// is not available in Phase 2", so seeding one would show a prospect a node they cannot inspect.
//
// Two of them ship live and three paused. The split is not caution for its own sake: the automation
// scheduler is disabled whenever ENVIRONMENT=demo, so nothing ever executes, and the enrollment
// trigger is AFTER INSERT on contact_timeline, so an automation activated at the end of the seed
// cannot see any of the history written before it. That leaves the trigger event as the only thing
// that decides whether a live automation enrolls anyone after the reset:
//
//   - cart-recovery and post-purchase trigger on custom_event. The demo's goal events are all
//     written during the seed, so their triggers stay quiet afterwards. Safe to activate.
//   - welcome-series, vip-concierge and winback-sunset trigger on list.subscribed / segment.joined,
//     which keep landing after the seed returns — segment membership is built asynchronously by the
//     task scheduler, which is not demo-gated. Activating those would enroll a burst of contacts who
//     would then sit at the trigger node forever, so they stay paused.
//
// Paused also means Edit and Delete stay on the card, so a prospect can open those three in the flow
// editor. The numbers on all five come from seeded journey history, not from execution — see
// demo_automation_history.go.
const (
	demoAutomationWelcome      = "welcome-series"
	demoAutomationCartRecovery = "cart-recovery"
	demoAutomationPostPurchase = "post-purchase"
	demoAutomationVIPConcierge = "vip-concierge"
	demoAutomationWinback      = "winback-sunset"
)

// The demo's lists and conversion goals, named here because the automations reference them and a
// typo in a list_id fails silently: contact_lists has no foreign key, so the executor would happily
// write an orphan membership and the flow would render entirely green.
const (
	demoListNewsletter = "newsletter"
	demoListVIPClub    = "vipclub"

	demoGoalAddToCart = "add_to_cart"
	demoGoalPurchase  = "purchase"

	// Templates created by createAutomationTemplates. An email node pointing at a template that does
	// not exist is not rejected at save time, and CreateTemplate failures are only Warn-logged, so
	// the guard test checks these against what the template seeder really creates.
	demoTemplateCartRecoveryA = "cart-recovery-a"
	demoTemplateCartRecoveryB = "cart-recovery-b"
	demoTemplateOrderThankYou = "order-thank-you"
	demoTemplateWinbackOffer  = "winback-offer"
)

// demoLiveAutomations are the automations activated on the demo host. Activation installs a real
// PostgreSQL trigger, so this list must only ever hold automations whose trigger event does not keep
// firing after the seed.
var demoLiveAutomations = []string{demoAutomationCartRecovery, demoAutomationPostPurchase}

// demoIntendedStatus is the status a seeded automation should come to rest in.
//
// The ones destined to go live are created draft and then activated, which is the only order that
// installs a trigger. The rest come to rest paused — the same inert state as draft, but the honest
// badge for a card carrying hundreds of finished journeys, and the one that keeps Edit and Delete on
// the card so a prospect can open the flow.
//
// Getting them there takes a direct write. AutomationService.Create deliberately overwrites the
// status with draft, because a row created live would show a Live badge with nothing listening on
// contact_timeline. Paused carries none of that risk — it installs no trigger either — so the
// journey-history transaction sets it alongside the stats. See writeDemoJourneys.
func demoIntendedStatus(automationID string) domain.AutomationStatus {
	for _, id := range demoLiveAutomations {
		if id == automationID {
			return domain.AutomationStatusDraft
		}
	}
	return domain.AutomationStatusPaused
}

// Node layout geometry. The Flow Stats drawer re-runs layoutNodes so it looks right whatever is
// stored, but the editor renders stored positions verbatim — automationToFlow maps them straight
// through and only the user's explicit "reorganize" action re-layouts. Identical or zero positions
// therefore open the three editable flows as a pile of overlapping boxes. These values mirror
// layoutNodes' own defaults so the two views agree.
const (
	demoNodeStartX     = 400.0
	demoNodeStartY     = 50.0
	demoNodeRowHeight  = 200.0
	demoNodeSiblingGap = 150.0
)

// demoNodeRow returns the position of a node at the given depth on the main spine.
func demoNodeRow(depth int) domain.NodePosition {
	return domain.NodePosition{X: demoNodeStartX, Y: demoNodeStartY + demoNodeRowHeight*float64(depth)}
}

// demoNodeRowOffset returns the position of a sibling at the given depth, offset horizontally.
func demoNodeRowOffset(depth int, offset float64) domain.NodePosition {
	pos := demoNodeRow(depth)
	pos.X += offset
	return pos
}

func demoStr(s string) *string { return &s }

// demoNode builds an automation node. AutomationID is set from the automation the node belongs to:
// AutomationNode.Validate rejects an empty one, and Automation.Validate runs it over every node, so
// forgetting it makes the whole automation unsaveable. Config must be non-nil for the same reason —
// an empty map passes, nil does not.
func demoNode(automationID, id string, nodeType domain.NodeType, config map[string]interface{}, next *string, pos domain.NodePosition) *domain.AutomationNode {
	if config == nil {
		config = map[string]interface{}{}
	}
	return &domain.AutomationNode{
		ID:           id,
		AutomationID: automationID,
		Type:         nodeType,
		Config:       config,
		NextNodeID:   next,
		Position:     pos,
	}
}

// DemoAutomations returns the showcase automation definitions for a workspace.
//
// Exported as a seam for the integration suite, which needs the real definitions to write real rows
// against a real database — the service-layer tests run on mocks and cannot see a column name that
// does not exist.
func DemoAutomations(workspaceID string) []*domain.Automation {
	return demoAutomations(workspaceID)
}

// SeedAutomationHistory writes journey history for automations that already exist in the workspace.
//
// The counterpart seam to DemoAutomations: the cohort queries and the batched inserts are the part of
// this feature that only a real Postgres can validate.
func (s *DemoService) SeedAutomationHistory(ctx context.Context, workspaceID string, automations []*domain.Automation) error {
	return s.seedDemoAutomationHistory(ctx, workspaceID, automations)
}

// demoAutomations returns the five showcase automations, in creation order.
//
// Creation order is the reverse of reading order on purpose: the list endpoint orders by created_at
// DESC with no secondary key and the page renders in response order, so creating them in narrative
// order would show the story backwards.
func demoAutomations(workspaceID string) []*domain.Automation {
	return []*domain.Automation{
		demoWinbackAutomation(workspaceID),
		demoVIPConciergeAutomation(workspaceID),
		demoPostPurchaseAutomation(workspaceID),
		demoCartRecoveryAutomation(workspaceID),
		demoWelcomeAutomation(workspaceID),
	}
}

// demoWelcomeAutomation is the onboarding flow: two touches, with a subscription check between them
// so the second only goes to people still on the list.
func demoWelcomeAutomation(workspaceID string) *domain.Automation {
	id := demoAutomationWelcome
	return &domain.Automation{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        "Welcome Series",
		Status:      demoIntendedStatus(id),
		ListID:      demoListNewsletter,
		Trigger: &domain.TimelineTriggerConfig{
			EventKind: "list.subscribed",
			ListID:    demoStr(demoListNewsletter),
			Frequency: domain.TriggerFrequencyOnce,
		},
		RootNodeID: "ws-trigger",
		Nodes: []*domain.AutomationNode{
			demoNode(id, "ws-trigger", domain.NodeTypeTrigger, nil, demoStr("ws-delay-15m"), demoNodeRow(0)),
			demoNode(id, "ws-delay-15m", domain.NodeTypeDelay, map[string]interface{}{
				"duration": 15, "unit": "minutes",
			}, demoStr("ws-email-welcome"), demoNodeRow(1)),
			demoNode(id, "ws-email-welcome", domain.NodeTypeEmail, map[string]interface{}{
				"template_id": "welcome-email",
			}, demoStr("ws-delay-3d"), demoNodeRow(2)),
			demoNode(id, "ws-delay-3d", domain.NodeTypeDelay, map[string]interface{}{
				"duration": 3, "unit": "days",
			}, demoStr("ws-status-check"), demoNodeRow(3)),
			// Two of the three branches are wired. One would render as a single edge straight down
			// and the fork this flow exists to show would never appear — the viewer emits an edge
			// only for a non-empty target.
			demoNode(id, "ws-status-check", domain.NodeTypeListStatusBranch, map[string]interface{}{
				"list_id":             demoListNewsletter,
				"active_node_id":      "ws-email-digest",
				"non_active_node_id":  "ws-email-reengage",
				"not_in_list_node_id": "",
			}, nil, demoNodeRow(4)),
			demoNode(id, "ws-email-digest", domain.NodeTypeEmail, map[string]interface{}{
				"template_id": "newsletter-weekly",
			}, nil, demoNodeRowOffset(5, -demoNodeSiblingGap)),
			demoNode(id, "ws-email-reengage", domain.NodeTypeEmail, map[string]interface{}{
				"template_id": demoTemplateWinbackOffer,
			}, nil, demoNodeRowOffset(5, demoNodeSiblingGap)),
		},
	}
}

// demoCartRecoveryAutomation ties the web-analytics goals to lifecycle messaging: someone added to
// cart, four hours later they still have not bought, so they get one of two recovery emails.
func demoCartRecoveryAutomation(workspaceID string) *domain.Automation {
	id := demoAutomationCartRecovery
	return &domain.Automation{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        "Abandoned Cart Recovery",
		Status:      demoIntendedStatus(id),
		ListID:      demoListNewsletter,
		Trigger: &domain.TimelineTriggerConfig{
			EventKind:       "custom_event",
			CustomEventName: demoStr(demoGoalAddToCart),
			Frequency:       domain.TriggerFrequencyEveryTime,
		},
		RootNodeID: "cr-trigger",
		Nodes: []*domain.AutomationNode{
			demoNode(id, "cr-trigger", domain.NodeTypeTrigger, nil, demoStr("cr-delay-4h"), demoNodeRow(0)),
			demoNode(id, "cr-delay-4h", domain.NodeTypeDelay, map[string]interface{}{
				"duration": 4, "unit": "hours",
			}, demoStr("cr-filter"), demoNodeRow(1)),
			// The condition is the same leaf the Cart Abandoners segment hangs on, shared so the two
			// cannot drift apart. The description says what that leaf actually does rather than what
			// the flow's name suggests: it is a 90-day window, so a customer who bought three weeks
			// ago and abandons a cart today is filtered out.
			demoNode(id, "cr-filter", domain.NodeTypeFilter, map[string]interface{}{
				"description":      "No purchase in the last 90 days",
				"conditions":       demoSegmentAllOf(demoNoPurchaseInLast90Days()),
				"continue_node_id": "cr-abtest",
				"exit_node_id":     "",
			}, nil, demoNodeRow(2)),
			demoNode(id, "cr-abtest", domain.NodeTypeABTest, map[string]interface{}{
				"variants": []domain.ABTestVariant{
					{ID: "a", Name: "Product-led", Weight: 50, NextNodeID: "cr-email-a"},
					{ID: "b", Name: "Discount-led", Weight: 50, NextNodeID: "cr-email-b"},
				},
			}, nil, demoNodeRow(3)),
			demoNode(id, "cr-email-a", domain.NodeTypeEmail, map[string]interface{}{
				"template_id": demoTemplateCartRecoveryA,
			}, nil, demoNodeRowOffset(4, -demoNodeSiblingGap)),
			demoNode(id, "cr-email-b", domain.NodeTypeEmail, map[string]interface{}{
				"template_id": demoTemplateCartRecoveryB,
			}, nil, demoNodeRowOffset(4, demoNodeSiblingGap)),
		},
	}
}

// demoPostPurchaseAutomation is the revenue-side counterpart: a purchase promotes the buyer onto the
// VIP list and sends a thank-you, showing that an automation can change audience membership and not
// only send mail.
func demoPostPurchaseAutomation(workspaceID string) *domain.Automation {
	id := demoAutomationPostPurchase
	return &domain.Automation{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        "Post-Purchase Thank You",
		Status:      demoIntendedStatus(id),
		ListID:      demoListNewsletter,
		Trigger: &domain.TimelineTriggerConfig{
			EventKind:       "custom_event",
			CustomEventName: demoStr(demoGoalPurchase),
			Frequency:       domain.TriggerFrequencyEveryTime,
		},
		RootNodeID: "pp-trigger",
		Nodes: []*domain.AutomationNode{
			demoNode(id, "pp-trigger", domain.NodeTypeTrigger, nil, demoStr("pp-delay-1h"), demoNodeRow(0)),
			demoNode(id, "pp-delay-1h", domain.NodeTypeDelay, map[string]interface{}{
				"duration": 1, "unit": "hours",
			}, demoStr("pp-add-vip"), demoNodeRow(1)),
			// status has to be exactly "active" or "pending"; anything else is rejected at save time.
			// This is the same trap migration v35 had to repair for add_to_list in segments.
			demoNode(id, "pp-add-vip", domain.NodeTypeAddToList, map[string]interface{}{
				"list_id": demoListVIPClub,
				"status":  "active",
			}, demoStr("pp-email-thanks"), demoNodeRow(2)),
			demoNode(id, "pp-email-thanks", domain.NodeTypeEmail, map[string]interface{}{
				"template_id": demoTemplateOrderThankYou,
			}, nil, demoNodeRow(3)),
		},
	}
}

// demoVIPConciergeAutomation shows a segment trigger and an outbound integration, with no email
// anywhere in the flow.
func demoVIPConciergeAutomation(workspaceID string) *domain.Automation {
	id := demoAutomationVIPConcierge
	return &domain.Automation{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        "VIP Concierge Handoff",
		Status:      demoIntendedStatus(id),
		ListID:      demoListVIPClub,
		Trigger: &domain.TimelineTriggerConfig{
			EventKind: "segment.joined",
			SegmentID: demoStr("vip_customers"),
			Frequency: domain.TriggerFrequencyOnce,
		},
		RootNodeID: "vc-trigger",
		Nodes: []*domain.AutomationNode{
			demoNode(id, "vc-trigger", domain.NodeTypeTrigger, nil, demoStr("vc-add-vip"), demoNodeRow(0)),
			demoNode(id, "vc-add-vip", domain.NodeTypeAddToList, map[string]interface{}{
				"list_id": demoListVIPClub,
				"status":  "active",
			}, demoStr("vc-webhook"), demoNodeRow(1)),
			// example.com, not a notifuse.com path: an unknown non-/api path there answers with a 307
			// to /console, which the webhook client follows with the POST body intact and stores the
			// console's HTML as a successful response. A reserved domain cannot fake a success.
			//
			// No secret: it would sit in cleartext inside the automations.nodes JSONB, readable by
			// anyone with read access to the workspace.
			demoNode(id, "vc-webhook", domain.NodeTypeWebhook, map[string]interface{}{
				"url": "https://example.com/hooks/crm/vip",
			}, nil, demoNodeRow(2)),
		},
	}
}

// demoWinbackAutomation is list hygiene as an automation: one last offer, then a week later either
// they re-engaged or they come off the list.
func demoWinbackAutomation(workspaceID string) *domain.Automation {
	id := demoAutomationWinback
	timeframe := "in_the_last_days"
	return &domain.Automation{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        "Win-back and Sunset",
		Status:      demoIntendedStatus(id),
		ListID:      demoListNewsletter,
		Trigger: &domain.TimelineTriggerConfig{
			EventKind: "segment.joined",
			SegmentID: demoStr("winback_opportunities"),
			Frequency: domain.TriggerFrequencyOnce,
		},
		RootNodeID: "wb-trigger",
		Nodes: []*domain.AutomationNode{
			demoNode(id, "wb-trigger", domain.NodeTypeTrigger, nil, demoStr("wb-email-offer"), demoNodeRow(0)),
			demoNode(id, "wb-email-offer", domain.NodeTypeEmail, map[string]interface{}{
				"template_id": demoTemplateWinbackOffer,
			}, demoStr("wb-delay-7d"), demoNodeRow(1)),
			demoNode(id, "wb-delay-7d", domain.NodeTypeDelay, map[string]interface{}{
				"duration": 7, "unit": "days",
			}, demoStr("wb-filter"), demoNodeRow(2)),
			// The mirror image of cart-recovery's filter: here the wired branch is the rejection one,
			// so between the two flows both filter shapes are on show.
			demoNode(id, "wb-filter", domain.NodeTypeFilter, map[string]interface{}{
				"description": "Opened an email in the last 7 days",
				"conditions": demoSegmentAllOf(&domain.TreeNode{
					Kind: "leaf",
					Leaf: &domain.TreeNodeLeaf{
						Source: "contact_timeline",
						ContactTimeline: &domain.ContactTimelineCondition{
							Kind:              "email.opened",
							CountOperator:     "at_least",
							CountValue:        1,
							TimeframeOperator: &timeframe,
							TimeframeValues:   []string{"7"},
							Filters:           []*domain.DimensionFilter{},
						},
					},
				}),
				"continue_node_id": "",
				"exit_node_id":     "wb-remove",
			}, nil, demoNodeRow(3)),
			demoNode(id, "wb-remove", domain.NodeTypeRemoveFromList, map[string]interface{}{
				"list_id": demoListNewsletter,
			}, nil, demoNodeRowOffset(4, demoNodeSiblingGap)),
		},
	}
}

// createSampleAutomations creates the five showcase automations, seeds their journey history, and
// activates the two that are safe to run live.
//
// Individual failures are logged and skipped rather than aborting, matching createSampleSegments —
// a missing automation should not cost the demo its contacts. The consequence is that a broken
// automation is invisible in the seed's return value, which is what the guard tests are for.
func (s *DemoService) createSampleAutomations(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating sample automations")

	if s.automationService == nil {
		s.logger.Warn("No automation service configured, skipping demo automations")
		return nil
	}

	// Only the automations that were really created get history: contact_automations.automation_id is
	// NOT NULL REFERENCES automations(id), so a journey row for an automation that failed to create
	// raises a foreign-key violation and takes its whole transaction with it.
	created := make([]*domain.Automation, 0, len(demoAutomations(workspaceID)))
	for _, automation := range demoAutomations(workspaceID) {
		if err := s.automationService.Create(ctx, workspaceID, automation); err != nil {
			s.logger.WithFields(map[string]interface{}{
				"automation_id": automation.ID,
				"error":         err.Error(),
			}).Warn("Failed to create demo automation")
			continue
		}
		created = append(created, automation)
	}

	if err := s.seedDemoAutomationHistory(ctx, workspaceID, created); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to seed demo automation history")
	}

	s.activateDemoAutomations(ctx, workspaceID, created)

	s.logger.WithField("workspace_id", workspaceID).Info("Sample automations created successfully")
	return nil
}

// activateDemoAutomations makes the live automations live, but only on the demo host.
//
// The demo reset endpoint is registered on any non-production environment, while the automation
// scheduler and the email queue worker only stop when ENVIRONMENT=demo. So on a development or
// staging instance a live automation with an email node would enroll, advance and enqueue real sends
// through the demo's own SMTP integration. Gating on the workers alone is not enough; the gate has to
// be here.
//
// Note this gates the seeding process, not the data: activation leaves durable DDL behind, so once a
// demo database carries these triggers a visitor's own events keep enrolling genuinely active
// journeys. The seeded rows stay inert regardless — see demo_automation_history.go.
func (s *DemoService) activateDemoAutomations(ctx context.Context, workspaceID string, created []*domain.Automation) {
	if !s.config.IsDemo() {
		s.logger.Info("Not running in demo mode, leaving demo automations inactive")
		return
	}

	live := make(map[string]bool, len(demoLiveAutomations))
	for _, id := range demoLiveAutomations {
		live[id] = true
	}

	for _, automation := range created {
		if !live[automation.ID] {
			continue
		}
		// Create wrote it as draft on purpose. Writing status=live directly would insert a row that
		// displays as Live with no trigger behind it, and Activate then refuses because it is already
		// live — an automation that can never fire and cannot be repaired.
		if err := s.automationService.Activate(ctx, workspaceID, automation.ID); err != nil {
			s.logger.WithFields(map[string]interface{}{
				"automation_id": automation.ID,
				"error":         err.Error(),
			}).Warn("Failed to activate demo automation")
		}
	}
}
