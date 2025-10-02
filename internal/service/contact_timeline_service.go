package service

import (
	"context"

	"github.com/Notifuse/notifuse/internal/domain"
)

// ContactTimelineService implements domain.ContactTimelineService
type ContactTimelineService struct {
	repo domain.ContactTimelineRepository
}

// NewContactTimelineService creates a new contact timeline service
func NewContactTimelineService(repo domain.ContactTimelineRepository) *ContactTimelineService {
	return &ContactTimelineService{
		repo: repo,
	}
}

// List retrieves timeline entries for a contact with pagination
func (s *ContactTimelineService) List(ctx context.Context, workspaceID string, email string, limit int, cursor *string) ([]*domain.ContactTimelineEntry, *string, error) {
	return s.repo.List(ctx, workspaceID, email, limit, cursor)
}
