package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/tracing"
)

// MessageHistoryService implements domain.MessageHistoryService interface
type MessageHistoryService struct {
	repo        domain.MessageHistoryRepository
	logger      logger.Logger
	authService domain.AuthService
}

// NewMessageHistoryService creates a new message history service
func NewMessageHistoryService(repo domain.MessageHistoryRepository, logger logger.Logger, authService domain.AuthService) *MessageHistoryService {
	return &MessageHistoryService{
		repo:        repo,
		logger:      logger,
		authService: authService,
	}
}

// ListMessages retrieves messages for a workspace with cursor-based pagination and filters
func (s *MessageHistoryService) ListMessages(ctx context.Context, workspaceID string, params domain.MessageListParams) (*domain.MessageListResult, error) {
	// codecov:ignore:start

	ctx, span := tracing.StartServiceSpan(ctx, "MessageHistoryService", "ListMessages")
	defer tracing.EndSpan(span, nil)
	tracing.AddAttribute(ctx, "workspaceID", workspaceID)
	// codecov:ignore:end

	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Call repository method with pagination and filtering parameters
	messages, nextCursor, err := s.repo.ListMessages(ctx, workspaceID, params)

	// codecov:ignore:start
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to list messages: %v", err))
		tracing.MarkSpanError(ctx, err)
		return nil, err
	}
	// codecov:ignore:end

	return &domain.MessageListResult{
		Messages:   messages,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}, nil
}

func (s *MessageHistoryService) GetBroadcastStats(ctx context.Context, workspaceID string, id string) (*domain.MessageHistoryStatusSum, error) {
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	stats, err := s.repo.GetBroadcastStats(ctx, workspaceID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get broadcast stats: %w", err)
	}

	return stats, nil
}

// GetBroadcastVariationStats retrieves statistics for a specific variation of a broadcast
func (s *MessageHistoryService) GetBroadcastVariationStats(ctx context.Context, workspaceID, broadcastID, templateID string) (*domain.MessageHistoryStatusSum, error) {
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Validate input parameters
	if templateID == "" {
		return nil, errors.New("template ID cannot be empty")
	}

	stats, err := s.repo.GetBroadcastVariationStats(ctx, workspaceID, broadcastID, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get broadcast variation stats: %w", err)
	}

	return stats, nil
}
