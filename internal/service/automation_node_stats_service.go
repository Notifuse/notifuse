package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// readAutomationsAs authenticates the caller and checks they may read automations.
func (s *AutomationService) readAutomationsAs(ctx context.Context, workspaceID string) (context.Context, *domain.UserWorkspace, error) {
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to authenticate: %w", err)
	}
	if !userWorkspace.HasPermission(domain.PermissionResourceAutomations, domain.PermissionTypeRead) {
		return nil, nil, domain.NewPermissionError(
			domain.PermissionResourceAutomations,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to automations required",
		)
	}
	return ctx, userWorkspace, nil
}

// CountJourneys counts the journeys of each automation by status, with zeros for an
// automation without journey.
func (s *AutomationService) CountJourneys(ctx context.Context, workspaceID string, automationIDs []string) (map[string]domain.JourneyCounts, error) {
	ctx, _, err := s.readAutomationsAs(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]domain.JourneyCounts, len(automationIDs))
	if len(automationIDs) == 0 {
		return counts, nil
	}

	stored, err := s.repo.CountJourneysByStatus(ctx, workspaceID, automationIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to count journeys: %w", err)
	}
	for _, id := range automationIDs {
		counts[id] = stored[id]
	}
	return counts, nil
}

// GetNodeStats returns the results of every step of the automation's graph. A step never
// reached is in the response with zeros; a step of the executions that is no longer in the
// graph is left out. The email counters need the message history read permission: without
// it they are neither read nor returned.
func (s *AutomationService) GetNodeStats(ctx context.Context, workspaceID, automationID string, since, until *time.Time) (*domain.NodeStatsResponse, error) {
	ctx, userWorkspace, err := s.readAutomationsAs(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	// A not found error is kept as such (%w) for the handler's 404.
	automation, err := s.repo.GetByID(ctx, workspaceID, automationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get automation: %w", err)
	}

	withEmail := userWorkspace.HasPermission(domain.PermissionResourceMessageHistory, domain.PermissionTypeRead)
	raw, err := s.repo.GetNodeStats(ctx, workspaceID, automationID, since, until, withEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to get node stats: %w", err)
	}

	response := domain.NewNodeStatsResponse(automationID, since, until)
	for _, node := range automation.Nodes {
		if node == nil {
			continue
		}
		stats := domain.NewNodeStats(node.Type)
		if read, ok := raw.Nodes[node.ID]; ok && read != nil {
			stats = read
			// The executions' node_type can be wrong (an error row says trigger): the graph's wins.
			stats.NodeType = node.Type
			if stats.SkipReasons == nil {
				stats.SkipReasons = map[string]int64{}
			}
			if stats.Branches == nil {
				stats.Branches = map[string]int64{}
			}
		}
		switch {
		case node.Type != domain.NodeTypeEmail || !withEmail:
			stats.Email = nil
		case stats.Email == nil:
			stats.Email = &domain.NodeEmailStats{}
		}
		response.Nodes[node.ID] = stats
	}
	return response, nil
}
