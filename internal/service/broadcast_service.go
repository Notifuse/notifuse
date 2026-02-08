package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service/broadcast"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/google/uuid"
)

// BroadcastService handles all broadcast-related operations
type BroadcastService struct {
	logger             logger.Logger
	repo               domain.BroadcastRepository
	workspaceRepo      domain.WorkspaceRepository
	contactRepo        domain.ContactRepository
	emailSvc           domain.EmailServiceInterface
	templateSvc        domain.TemplateService
	taskService        domain.TaskService
	taskRepo           domain.TaskRepository
	authService        domain.AuthService
	eventBus           domain.EventBus
	messageHistoryRepo domain.MessageHistoryRepository
	listService        domain.ListService
	dataFeedFetcher    broadcast.DataFeedFetcher
	apiEndpoint        string
}

// NewBroadcastService creates a new broadcast service
func NewBroadcastService(
	logger logger.Logger,
	repository domain.BroadcastRepository,
	workspaceRepository domain.WorkspaceRepository,
	emailService domain.EmailServiceInterface,
	contactRepository domain.ContactRepository,
	templateService domain.TemplateService,
	taskService domain.TaskService,
	taskRepository domain.TaskRepository,
	authService domain.AuthService,
	eventBus domain.EventBus,
	messageHistoryRepository domain.MessageHistoryRepository,
	listService domain.ListService,
	dataFeedFetcher broadcast.DataFeedFetcher,
	apiEndpoint string,
) *BroadcastService {
	return &BroadcastService{
		logger:             logger,
		repo:               repository,
		workspaceRepo:      workspaceRepository,
		emailSvc:           emailService,
		contactRepo:        contactRepository,
		templateSvc:        templateService,
		taskService:        taskService,
		taskRepo:           taskRepository,
		authService:        authService,
		eventBus:           eventBus,
		messageHistoryRepo: messageHistoryRepository,
		listService:        listService,
		dataFeedFetcher:    dataFeedFetcher,
		apiEndpoint:        apiEndpoint,
	}
}

// SetTaskService sets the task service (used to avoid circular dependencies)
func (s *BroadcastService) SetTaskService(taskService domain.TaskService) {
	s.taskService = taskService
}

// CreateBroadcast creates a new broadcast
func (s *BroadcastService) CreateBroadcast(ctx context.Context, request *domain.CreateBroadcastRequest) (*domain.Broadcast, error) {
	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		s.logger.Error("Failed to authenticate user for workspace")
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing broadcasts
	if !userWorkspace.HasPermission(domain.PermissionResourceBroadcasts, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBroadcasts,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to broadcasts required",
		)
	}

	// Validate the request
	broadcast, err := request.Validate()
	if err != nil {
		s.logger.Error("Failed to validate broadcast creation request")
		return nil, err
	}

	// Generate a unique ID for the broadcast if not provided
	if broadcast.ID == "" {
		// Create a random ID
		id := make([]byte, 16)
		_, err := rand.Read(id)
		if err != nil {
			return nil, fmt.Errorf("failed to generate ID: %w", err)
		}
		broadcast.ID = fmt.Sprintf("%x", id)[:32]
	}

	// Set default values
	broadcast.Status = domain.BroadcastStatusDraft
	now := time.Now().UTC()
	broadcast.CreatedAt = now
	broadcast.UpdatedAt = now

	// Set scheduled time if needed
	if broadcast.Schedule.IsScheduled && (broadcast.Schedule.ScheduledDate != "" && broadcast.Schedule.ScheduledTime != "") {
		// Set status to scheduled if the broadcast is scheduled
		broadcast.Status = domain.BroadcastStatusScheduled
	}

	// Persist the broadcast
	err = s.repo.CreateBroadcast(ctx, broadcast)
	if err != nil {
		s.logger.Error("Failed to create broadcast in repository")
		return nil, err
	}

	s.logger.Info("Broadcast created successfully")

	return broadcast, nil
}

// GetBroadcast retrieves a broadcast by ID
func (s *BroadcastService) GetBroadcast(ctx context.Context, workspaceID, broadcastID string) (*domain.Broadcast, error) {
	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("broadcast_id", broadcastID).Error("Failed to authenticate user for workspace")
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading broadcasts
	if !userWorkspace.HasPermission(domain.PermissionResourceBroadcasts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBroadcasts,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to broadcasts required",
		)
	}

	// Fetch the broadcast from the repository
	return s.repo.GetBroadcast(ctx, workspaceID, broadcastID)
}

// UpdateBroadcast updates an existing broadcast
func (s *BroadcastService) UpdateBroadcast(ctx context.Context, request *domain.UpdateBroadcastRequest) (*domain.Broadcast, error) {
	// Authenticate user for workspace
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		s.logger.WithField("broadcast_id", request.ID).Error("Failed to authenticate user for workspace")
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// First, get the existing broadcast
	existingBroadcast, err := s.repo.GetBroadcast(ctx, request.WorkspaceID, request.ID)
	if err != nil {
		s.logger.Error("Failed to get broadcast for update")
		return nil, err
	}

	// Validate and update broadcast fields
	updatedBroadcast, err := request.Validate(existingBroadcast)
	if err != nil {
		s.logger.Error("Failed to validate broadcast update request")
		return nil, err
	}

	// Set the updated time
	updatedBroadcast.UpdatedAt = time.Now().UTC()

	// Persist the changes
	err = s.repo.UpdateBroadcast(ctx, updatedBroadcast)
	if err != nil {
		s.logger.Error("Failed to update broadcast in repository")
		return nil, err
	}

	s.logger.Info("Broadcast updated successfully")

	return updatedBroadcast, nil
}

// ListBroadcasts retrieves a list of broadcasts with pagination
func (s *BroadcastService) ListBroadcasts(ctx context.Context, params domain.ListBroadcastsParams) (*domain.BroadcastListResponse, error) {
	// Authenticate user for workspace
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, params.WorkspaceID)
	if err != nil {
		s.logger.Error("Failed to authenticate user for workspace")
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Apply default values for pagination if not provided
	if params.Limit <= 0 {
		params.Limit = 50 // Default limit
	}
	if params.Limit > 100 {
		params.Limit = 100 // Maximum limit
	}
	if params.Offset < 0 {
		params.Offset = 0 // Ensure offset is not negative
	}

	response, err := s.repo.ListBroadcasts(ctx, params)
	if err != nil {
		s.logger.Error("Failed to list broadcasts from repository")
		return nil, err
	}

	// If WithTemplates is true, fetch template details for each variation
	if params.WithTemplates {
		for _, broadcast := range response.Broadcasts {
			for i, variation := range broadcast.TestSettings.Variations {
				if variation.TemplateID != "" {
					// Fetch the template for this variation
					template, err := s.templateSvc.GetTemplateByID(ctx, params.WorkspaceID, variation.TemplateID, 0)
					if err != nil {
						s.logger.Error("Failed to fetch template for broadcast variation")
						// Continue with the next variation rather than failing the whole request
						continue
					}

					// Assign the template to the variation
					broadcast.SetTemplateForVariation(i, template)
				}
			}
		}
	}

	s.logger.Info("Broadcasts listed successfully")

	return response, nil
}

// ScheduleBroadcast schedules a broadcast for sending
func (s *BroadcastService) ScheduleBroadcast(ctx context.Context, request *domain.ScheduleBroadcastRequest) error {
	// Authenticate user for workspace
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		s.logger.WithField("broadcast_id", request.ID).Error("Failed to authenticate user for workspace")
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate schedule broadcast request")
		return err
	}

	// Get workspace to check for email provider configuration
	workspace, err := s.workspaceRepo.GetByID(ctx, request.WorkspaceID)
	if err != nil {
		s.logger.Error("Failed to get workspace for scheduling broadcast")
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	// Check if workspace has a marketing email provider configured
	emailProvider, err := workspace.GetEmailProvider(true) // true for marketing emails
	if err != nil {
		s.logger.Error("Failed to get email provider configuration")
		return fmt.Errorf("failed to get email provider: %w", err)
	}

	if emailProvider == nil {
		s.logger.Error("Cannot schedule broadcast: no marketing email provider configured for workspace")
		return fmt.Errorf("no marketing email provider configured for this workspace")
	}

	// Using a channel to wait for the event callback
	done := make(chan error, 1)

	// Use transaction to retrieve, update the broadcast, and publish the event
	err = s.repo.WithTransaction(ctx, request.WorkspaceID, func(tx *sql.Tx) error {
		// Retrieve the broadcast
		bcast, err := s.repo.GetBroadcastTx(ctx, tx, request.WorkspaceID, request.ID)
		if err != nil {
			s.logger.Error("Failed to get broadcast for scheduling")
			return err
		}

		// Only draft broadcasts can be scheduled
		if bcast.Status != domain.BroadcastStatusDraft {
			err := fmt.Errorf("only broadcasts with draft status can be scheduled, current status: %s", bcast.Status)
			s.logger.Error("Cannot schedule broadcast with non-draft status")
			return err
		}

		// Fetch global feed if configured
		if bcast.DataFeed != nil && bcast.DataFeed.GlobalFeed != nil && bcast.DataFeed.GlobalFeed.Enabled {
			// Get list information for the payload
			var listName string
			if bcast.Audience.List != "" {
				list, listErr := s.listService.GetListByID(ctx, request.WorkspaceID, bcast.Audience.List)
				if listErr != nil {
					s.logger.WithField("list_id", bcast.Audience.List).Warn("Failed to get list for global feed payload")
				} else if list != nil {
					listName = list.Name
				}
			}

			payload := &domain.GlobalFeedRequestPayload{
				Broadcast: domain.GlobalFeedBroadcast{
					ID:   bcast.ID,
					Name: bcast.Name,
				},
				List: domain.GlobalFeedList{
					ID:   bcast.Audience.List,
					Name: listName,
				},
				Workspace: domain.GlobalFeedWorkspace{
					ID:   workspace.ID,
					Name: workspace.Name,
				},
			}

			feedData, fetchErr := s.dataFeedFetcher.FetchGlobal(ctx, bcast.DataFeed.GlobalFeed, payload)
			if fetchErr != nil {
				s.logger.WithFields(map[string]interface{}{
					"broadcast_id": bcast.ID,
					"url":          bcast.DataFeed.GlobalFeed.URL,
					"error":        fetchErr.Error(),
				}).Error("Failed to fetch global feed")
				return fmt.Errorf("failed to fetch global feed: %w", fetchErr)
			}

			// If feedData is nil, the feed was disabled or not configured
			if feedData != nil {
				now := time.Now().UTC()
				bcast.DataFeed.GlobalFeedData = feedData
				bcast.DataFeed.GlobalFeedFetchedAt = &now

				s.logger.WithFields(map[string]interface{}{
					"broadcast_id": bcast.ID,
					"data_keys":    len(feedData),
				}).Info("Global feed data fetched successfully")
			}
		}

		// Update broadcast status and scheduling info
		bcast.Status = domain.BroadcastStatusScheduled
		bcast.UpdatedAt = time.Now().UTC()

		if request.SendNow {
			// If sending immediately, set status to sending
			bcast.Status = domain.BroadcastStatusProcessing
			now := time.Now().UTC()
			bcast.StartedAt = &now
		} else {
			// Update the schedule settings with the requested settings
			bcast.Schedule.IsScheduled = true
			bcast.Schedule.ScheduledDate = request.ScheduledDate
			bcast.Schedule.ScheduledTime = request.ScheduledTime
			bcast.Schedule.Timezone = request.Timezone
			bcast.Schedule.UseRecipientTimezone = request.UseRecipientTimezone
		}

		// Persist the changes
		err = s.repo.UpdateBroadcastTx(ctx, tx, bcast)
		if err != nil {
			s.logger.Error("Failed to update broadcast in repository")
			return err
		}

		// Create event payload with schedule information
		payloadData := map[string]interface{}{
			"broadcast_id": request.ID,
			"send_now":     request.SendNow,
			"status":       string(bcast.Status),
		}

		// Include actual scheduled time if broadcast is scheduled
		if !request.SendNow && bcast.Schedule.IsScheduled {
			scheduledTime, parseErr := bcast.Schedule.ParseScheduledDateTime()
			if parseErr == nil && !scheduledTime.IsZero() {
				payloadData["scheduled_time"] = scheduledTime.Format(time.RFC3339)
			}
		}

		eventPayload := domain.EventPayload{
			Type:        domain.EventBroadcastScheduled,
			WorkspaceID: request.WorkspaceID,
			EntityID:    request.ID,
			Data:        payloadData,
		}

		// Publish the event with callback within the transaction
		s.eventBus.PublishWithAck(ctx, eventPayload, func(eventErr error) {
			if eventErr != nil {
				// Event processing failed, log the error
				s.logger.WithFields(map[string]interface{}{
					"broadcast_id": request.ID,
					"workspace_id": request.WorkspaceID,
					"error":        eventErr.Error(),
				}).Error("Failed to process schedule broadcast event")

				// Since we're still in the same transaction, we don't need to rollback explicitly
				// The outer transaction will be rolled back when we return an error

				done <- fmt.Errorf("failed to process schedule event: %w", eventErr)
			} else {
				s.logger.WithField("broadcast_id", request.ID).Info("Schedule broadcast event processed successfully")
				done <- nil
			}
		})

		// Wait for the event processing to complete
		select {
		case eventErr := <-done:
			if eventErr != nil {
				// If the event processing failed, roll back the transaction by returning an error
				return eventErr
			}
			// If event processing succeeded, commit the transaction
			return nil
		case <-ctx.Done():
			// If context is cancelled, roll back transaction by returning an error
			return ctx.Err()
		}
	})

	return err
}

// PauseBroadcast pauses a sending broadcast
func (s *BroadcastService) PauseBroadcast(ctx context.Context, request *domain.PauseBroadcastRequest) error {
	// Authenticate user for workspace
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		s.logger.WithField("broadcast_id", request.ID).Error("Failed to authenticate user for workspace")
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate pause broadcast request")
		return err
	}

	// Using a channel to wait for the event callback
	done := make(chan error, 1)

	// Use transaction to retrieve, update the broadcast, and publish the event
	err = s.repo.WithTransaction(ctx, request.WorkspaceID, func(tx *sql.Tx) error {
		// Retrieve the broadcast
		broadcast, err := s.repo.GetBroadcastTx(ctx, tx, request.WorkspaceID, request.ID)
		if err != nil {
			s.logger.Error("Failed to get broadcast for pausing")
			return err
		}

		// Only sending broadcasts can be paused
		if broadcast.Status != domain.BroadcastStatusProcessing {
			err := fmt.Errorf("only broadcasts with sending status can be paused, current status: %s", broadcast.Status)
			s.logger.Error("Cannot pause broadcast with non-sending status")
			return err
		}

		// Update broadcast status and pause info
		broadcast.Status = domain.BroadcastStatusPaused
		now := time.Now().UTC()
		broadcast.PausedAt = &now
		broadcast.UpdatedAt = now

		// Persist the changes
		err = s.repo.UpdateBroadcastTx(ctx, tx, broadcast)
		if err != nil {
			s.logger.Error("Failed to update broadcast in repository")
			return err
		}

		s.logger.Info("Broadcast paused successfully")

		// Create an event with acknowledgment callback
		eventPayload := domain.EventPayload{
			Type:        domain.EventBroadcastPaused,
			WorkspaceID: request.WorkspaceID,
			EntityID:    request.ID,
			Data: map[string]interface{}{
				"broadcast_id": request.ID,
			},
		}

		// Publish the event with callback within the transaction
		s.eventBus.PublishWithAck(ctx, eventPayload, func(eventErr error) {
			if eventErr != nil {
				// Event processing failed, log the error
				s.logger.WithFields(map[string]interface{}{
					"broadcast_id": request.ID,
					"workspace_id": request.WorkspaceID,
					"error":        eventErr.Error(),
				}).Error("Failed to process pause broadcast event")

				// Since we're still in the same transaction, we don't need to rollback explicitly
				// The outer transaction will be rolled back when we return an error

				done <- fmt.Errorf("failed to process pause event: %w", eventErr)
			} else {
				s.logger.WithField("broadcast_id", request.ID).Info("Pause broadcast event processed successfully")
				done <- nil
			}
		})

		// Wait for the event processing to complete
		select {
		case eventErr := <-done:
			if eventErr != nil {
				// If the event processing failed, roll back the transaction by returning an error
				return eventErr
			}
			// If event processing succeeded, commit the transaction
			return nil
		case <-ctx.Done():
			// If context is cancelled, roll back transaction by returning an error
			return ctx.Err()
		}
	})

	return err
}

// ResumeBroadcast resumes a paused broadcast
func (s *BroadcastService) ResumeBroadcast(ctx context.Context, request *domain.ResumeBroadcastRequest) error {
	// Authenticate user for workspace
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		s.logger.WithField("broadcast_id", request.ID).Error("Failed to authenticate user for workspace")
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate resume broadcast request")
		return err
	}

	// Using a channel to wait for the event callback
	done := make(chan error, 1)

	// Use transaction to retrieve, update the broadcast, and publish the event
	err = s.repo.WithTransaction(ctx, request.WorkspaceID, func(tx *sql.Tx) error {
		// Retrieve the broadcast
		broadcast, err := s.repo.GetBroadcastTx(ctx, tx, request.WorkspaceID, request.ID)
		if err != nil {
			s.logger.Error("Failed to get broadcast for resuming")
			return err
		}

		// Only paused broadcasts can be resumed
		if broadcast.Status != domain.BroadcastStatusPaused {
			err := fmt.Errorf("only broadcasts with paused status can be resumed, current status: %s", broadcast.Status)
			s.logger.Error("Cannot resume broadcast with invalid status")
			return err
		}

		// Update broadcast status
		now := time.Now().UTC()
		broadcast.UpdatedAt = now

		// Determine the new status based on scheduling
		startNow := false

		// If broadcast was originally scheduled and scheduled time is in the future
		if broadcast.Schedule.IsScheduled {
			scheduledTime, err := broadcast.Schedule.ParseScheduledDateTime()
			isScheduledInFuture := err == nil && scheduledTime.After(now) && broadcast.StartedAt == nil

			if isScheduledInFuture {
				broadcast.Status = domain.BroadcastStatusScheduled
				s.logger.Info("Broadcast resumed to scheduled status")
			} else {
				// If scheduled time has passed or there was an error parsing it
				broadcast.Status = domain.BroadcastStatusProcessing
				startNow = true
				if broadcast.StartedAt == nil {
					broadcast.StartedAt = &now
				}
				s.logger.Info("Broadcast resumed to sending status")
			}
		} else {
			// If broadcast wasn't scheduled, resume sending
			broadcast.Status = domain.BroadcastStatusProcessing
			startNow = true
			if broadcast.StartedAt == nil {
				broadcast.StartedAt = &now
			}
			s.logger.Info("Broadcast resumed to sending status")
		}

		// Clear the paused timestamp and reason
		broadcast.PausedAt = nil
		broadcast.PauseReason = nil

		// Persist the changes
		err = s.repo.UpdateBroadcastTx(ctx, tx, broadcast)
		if err != nil {
			s.logger.Error("Failed to update broadcast in repository")
			return err
		}

		s.logger.Info("Broadcast resumed successfully")

		// Create an event with acknowledgment callback
		eventPayload := domain.EventPayload{
			Type:        domain.EventBroadcastResumed,
			WorkspaceID: request.WorkspaceID,
			EntityID:    request.ID,
			Data: map[string]interface{}{
				"broadcast_id": request.ID,
				"start_now":    startNow,
			},
		}

		// Publish the event with callback within the transaction
		s.eventBus.PublishWithAck(ctx, eventPayload, func(eventErr error) {
			if eventErr != nil {
				// Event processing failed, log the error
				s.logger.WithFields(map[string]interface{}{
					"broadcast_id": request.ID,
					"workspace_id": request.WorkspaceID,
					"error":        eventErr.Error(),
				}).Error("Failed to process resume broadcast event")

				// Since we're still in the same transaction, we don't need to rollback explicitly
				// The outer transaction will be rolled back when we return an error

				done <- fmt.Errorf("failed to process resume event: %w", eventErr)
			} else {
				s.logger.WithField("broadcast_id", request.ID).Info("Resume broadcast event processed successfully")
				done <- nil
			}
		})

		// Wait for the event processing to complete
		select {
		case eventErr := <-done:
			if eventErr != nil {
				// If the event processing failed, roll back the transaction by returning an error
				return eventErr
			}
			// If event processing succeeded, commit the transaction
			return nil
		case <-ctx.Done():
			// If context is cancelled, roll back transaction by returning an error
			return ctx.Err()
		}
	})

	return err
}

// CancelBroadcast cancels a scheduled broadcast
func (s *BroadcastService) CancelBroadcast(ctx context.Context, request *domain.CancelBroadcastRequest) error {
	// Authenticate user for workspace
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		s.logger.WithField("broadcast_id", request.ID).Error("Failed to authenticate user for workspace")
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate cancel broadcast request")
		return err
	}

	// Using a channel to wait for the event callback
	done := make(chan error, 1)

	// Use transaction to retrieve, update the broadcast, and publish the event
	err = s.repo.WithTransaction(ctx, request.WorkspaceID, func(tx *sql.Tx) error {
		// Retrieve the broadcast
		broadcast, err := s.repo.GetBroadcastTx(ctx, tx, request.WorkspaceID, request.ID)
		if err != nil {
			s.logger.Error("Failed to get broadcast for cancellation")
			return err
		}

		// Only scheduled or paused broadcasts can be cancelled
		if broadcast.Status != domain.BroadcastStatusScheduled && broadcast.Status != domain.BroadcastStatusPaused {
			err := fmt.Errorf("only broadcasts with scheduled or paused status can be cancelled, current status: %s", broadcast.Status)
			s.logger.Error("Cannot cancel broadcast with invalid status")
			return err
		}

		// Update broadcast status and cancellation info
		broadcast.Status = domain.BroadcastStatusCancelled
		now := time.Now().UTC()
		broadcast.CancelledAt = &now
		broadcast.UpdatedAt = now

		// Persist the changes
		err = s.repo.UpdateBroadcastTx(ctx, tx, broadcast)
		if err != nil {
			s.logger.Error("Failed to update broadcast in repository")
			return err
		}

		s.logger.Info("Broadcast cancelled successfully")

		// Create an event with acknowledgment callback
		eventPayload := domain.EventPayload{
			Type:        domain.EventBroadcastCancelled,
			WorkspaceID: request.WorkspaceID,
			EntityID:    request.ID,
			Data: map[string]interface{}{
				"broadcast_id": request.ID,
			},
		}

		// Publish the event with callback within the transaction
		s.eventBus.PublishWithAck(ctx, eventPayload, func(eventErr error) {
			if eventErr != nil {
				// Event processing failed, log the error
				s.logger.WithFields(map[string]interface{}{
					"broadcast_id": request.ID,
					"workspace_id": request.WorkspaceID,
					"error":        eventErr.Error(),
				}).Error("Failed to process cancel broadcast event")

				// Since we're still in the same transaction, we don't need to rollback explicitly
				// The outer transaction will be rolled back when we return an error

				done <- fmt.Errorf("failed to process cancel event: %w", eventErr)
			} else {
				s.logger.WithField("broadcast_id", request.ID).Info("Cancel broadcast event processed successfully")
				done <- nil
			}
		})

		// Wait for the event processing to complete
		select {
		case eventErr := <-done:
			if eventErr != nil {
				// If the event processing failed, roll back the transaction by returning an error
				return eventErr
			}
			// If event processing succeeded, commit the transaction
			return nil
		case <-ctx.Done():
			// If context is cancelled, roll back transaction by returning an error
			return ctx.Err()
		}
	})

	return err
}

// DeleteBroadcast deletes a broadcast
func (s *BroadcastService) DeleteBroadcast(ctx context.Context, request *domain.DeleteBroadcastRequest) error {
	// Authenticate user for workspace
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		s.logger.WithField("broadcast_id", request.ID).Error("Failed to authenticate user for workspace")
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate delete broadcast request")
		return err
	}

	// Retrieve the broadcast to check its status
	broadcast, err := s.repo.GetBroadcast(ctx, request.WorkspaceID, request.ID)
	if err != nil {
		s.logger.Error("Failed to get broadcast for deletion")
		return err
	}

	// Broadcasts in 'sending' status cannot be deleted
	if broadcast.Status == domain.BroadcastStatusProcessing {
		err := fmt.Errorf("broadcasts in 'sending' status cannot be deleted")
		s.logger.Error("Cannot delete broadcast with sending status")
		return err
	}

	// Delete the broadcast
	err = s.repo.DeleteBroadcast(ctx, request.WorkspaceID, request.ID)
	if err != nil {
		s.logger.Error("Failed to delete broadcast from repository")
		return err
	}

	s.logger.Info("Broadcast deleted successfully")

	return nil
}

// SendToIndividual sends a broadcast to an individual recipient
func (s *BroadcastService) SendToIndividual(ctx context.Context, request *domain.SendToIndividualRequest) error {
	// Authenticate user for workspace
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		s.logger.WithField("broadcast_id", request.BroadcastID).Error("Failed to authenticate user for workspace")
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate send to individual request")
		return err
	}

	// get workspace
	workspace, err := s.workspaceRepo.GetByID(ctx, request.WorkspaceID)
	if err != nil {
		s.logger.Error("Failed to get workspace for individual sending")
		return err
	}

	// Check if workspace has a marketing email provider configured
	emailProvider, integrationID, err := workspace.GetEmailProviderWithIntegrationID(true) // true for marketing emails
	if err != nil {
		s.logger.Error("Failed to get email provider configuration")
		return fmt.Errorf("failed to get email provider: %w", err)
	}

	if emailProvider == nil {
		s.logger.Error("Cannot send broadcast: no marketing email provider configured for workspace")
		return fmt.Errorf("no marketing email provider configured for this workspace")
	}

	// Retrieve the broadcast
	broadcast, err := s.repo.GetBroadcast(ctx, request.WorkspaceID, request.BroadcastID)
	if err != nil {
		s.logger.Error("Failed to get broadcast for individual sending")
		return err
	}

	// Determine which variation to use
	templateID := request.TemplateID
	if templateID == "" && len(broadcast.TestSettings.Variations) > 0 {
		// If no variation ID specified, use the first one
		templateID = broadcast.TestSettings.Variations[0].TemplateID
		s.logger.Debug("No variation specified, using first variation")
	} else if templateID == "" {
		err := fmt.Errorf("broadcast has no variations")
		s.logger.Error("Cannot send broadcast with no variations")
		return err
	}

	// Find the specified variation
	var variation *domain.BroadcastVariation
	for _, v := range broadcast.TestSettings.Variations {
		if v.TemplateID == templateID {
			variation = &v
			break
		}
	}

	if variation == nil {
		err := fmt.Errorf("variation with ID %s not found in broadcast", templateID)
		s.logger.Error("Variation not found in broadcast")
		return err
	}

	// Fetch the contact if it exists, but don't fail if not found
	contact, contactErr := s.contactRepo.GetContactByEmail(ctx, request.WorkspaceID, request.RecipientEmail)
	if contactErr != nil {
		// Just log the error, don't return it
		s.logger.Info("Contact not found, using email address only")
	}

	// Fetch the template with latest version
	template, err := s.templateSvc.GetTemplateByID(ctx, request.WorkspaceID, variation.TemplateID, 0)
	if err != nil {
		s.logger.Error("Failed to fetch template for broadcast")
		return err
	}

	emailSender := emailProvider.GetSender(template.Email.SenderID)

	if emailSender == nil {
		s.logger.Error("Failed to get sender for broadcast")
		return fmt.Errorf("failed to get sender for broadcast")
	}

	messageID := uuid.New().String()

	// Use workspace CustomEndpointURL if provided, otherwise use the default API endpoint
	endpoint := s.apiEndpoint
	if workspace.Settings.CustomEndpointURL != nil && *workspace.Settings.CustomEndpointURL != "" {
		endpoint = *workspace.Settings.CustomEndpointURL
	}

	trackingSettings := notifuse_mjml.TrackingSettings{
		Endpoint:       endpoint,
		EnableTracking: workspace.Settings.EmailTrackingEnabled,
		WorkspaceID:    request.WorkspaceID,
		MessageID:      messageID,
	}

	// Add UTM parameters if available
	if broadcast.UTMParameters != nil {
		trackingSettings.UTMSource = broadcast.UTMParameters.Source
		trackingSettings.UTMMedium = broadcast.UTMParameters.Medium
		trackingSettings.UTMCampaign = broadcast.UTMParameters.Campaign
		trackingSettings.UTMContent = broadcast.UTMParameters.Content
		trackingSettings.UTMTerm = broadcast.UTMParameters.Term
	}

	req := domain.TemplateDataRequest{
		WorkspaceID:        request.WorkspaceID,
		WorkspaceSecretKey: workspace.Settings.SecretKey,
		ContactWithList: domain.ContactWithList{
			Contact:  contact,
			ListID:   broadcast.Audience.List, // Use list from broadcast audience for unsubscribe URL
			ListName: "",
		},
		MessageID:        messageID,
		TrackingSettings: trackingSettings,
		Broadcast:        broadcast,
	}
	templateData, err := domain.BuildTemplateData(req)
	if err != nil {
		s.logger.Error("Failed to build template data for broadcast")
		return err
	}

	// Add contact data if available
	if contact != nil {
		contactData, err := contact.ToMapOfAny()
		if err == nil {
			templateData["contact"] = contactData
		}
	}

	// Compile the template
	compiledTemplate, err := s.templateSvc.CompileTemplate(ctx, domain.CompileTemplateRequest{
		WorkspaceID:      request.WorkspaceID,
		MessageID:        messageID,
		VisualEditorTree: template.Email.VisualEditorTree,
		TemplateData:     notifuse_mjml.MapOfAny(templateData),
		TrackingSettings: trackingSettings,
	})
	if err != nil {
		s.logger.Error("Failed to compile template for broadcast")
		return err
	}

	if !compiledTemplate.Success || compiledTemplate.HTML == nil {
		errMsg := "Template compilation failed"
		if compiledTemplate.Error != nil {
			errMsg = compiledTemplate.Error.Message
		}
		s.logger.Error("Failed to generate HTML from template")
		return fmt.Errorf("template compilation failed: %s", errMsg)
	}

	// Create SendEmailProviderRequest
	emailRequest := domain.SendEmailProviderRequest{
		WorkspaceID:   request.WorkspaceID,
		IntegrationID: integrationID,
		MessageID:     messageID,
		FromAddress:   emailSender.Email,
		FromName:      emailSender.Name,
		To:            request.RecipientEmail,
		Subject:       template.Email.Subject,
		Content:       *compiledTemplate.HTML,
		Provider:      emailProvider,
		EmailOptions: domain.EmailOptions{
			ReplyTo: template.Email.ReplyTo,
		},
	}

	// Extract List-Unsubscribe URL from template data for RFC-8058 compliance
	if unsubscribeURL, ok := templateData["oneclick_unsubscribe_url"].(string); ok && unsubscribeURL != "" {
		emailRequest.EmailOptions.ListUnsubscribeURL = unsubscribeURL
	}

	// Send the email
	err = s.emailSvc.SendEmail(ctx, emailRequest, true)
	if err != nil {
		s.logger.Error("Failed to send message")
		return err
	}

	now := time.Now().UTC()
	listID := broadcast.Audience.List
	message := &domain.MessageHistory{
		ID:              messageID,
		ContactEmail:    request.RecipientEmail,
		BroadcastID:     &request.BroadcastID,
		ListID:          &listID,
		TemplateID:      template.ID,
		TemplateVersion: template.Version,
		Channel:         "email",
		MessageData: domain.MessageData{
			Data: templateData,
		},
		SentAt:    now,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Record message in history
	return s.messageHistoryRepo.Create(ctx, request.WorkspaceID, workspace.Settings.SecretKey, message)
}

// GetTestResults retrieves A/B test results for a broadcast
func (s *BroadcastService) GetTestResults(ctx context.Context, workspaceID, broadcastID string) (*domain.TestResultsResponse, error) {
	// Authenticate user
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	// Get broadcast
	broadcast, err := s.repo.GetBroadcast(ctx, workspaceID, broadcastID)
	if err != nil {
		return nil, err
	}

	// Validate status - allow viewing results during active sending and completed states
	if broadcast.Status != domain.BroadcastStatusTestCompleted &&
		broadcast.Status != domain.BroadcastStatusWinnerSelected &&
		broadcast.Status != domain.BroadcastStatusProcessed &&
		broadcast.Status != domain.BroadcastStatusProcessing &&
		broadcast.Status != domain.BroadcastStatusTesting {
		return nil, fmt.Errorf("broadcast test results not available for status: %s", broadcast.Status)
	}

	// Calculate metrics for each variation using message_history aggregation
	variationResults := make(map[string]*domain.VariationResult)
	var recommendedWinner string
	bestScore := -1.0

	for _, variation := range broadcast.TestSettings.Variations {
		// Use existing message history repository method with TemplateID
		stats, err := s.messageHistoryRepo.GetBroadcastVariationStats(ctx, workspaceID, broadcastID, variation.TemplateID)
		if err != nil {
			s.logger.WithFields(map[string]interface{}{
				"template_id": variation.TemplateID,
				"error":       err.Error(),
			}).Warn("Failed to get variation stats")
			continue // Skip failed variations
		}

		// Calculate rates (avoid division by zero)
		openRate := 0.0
		clickRate := 0.0
		if stats.TotalSent > 0 {
			openRate = float64(stats.TotalOpened) / float64(stats.TotalSent)
			clickRate = float64(stats.TotalClicked) / float64(stats.TotalSent)
		}

		variationResults[variation.TemplateID] = &domain.VariationResult{
			TemplateID:   variation.TemplateID,
			TemplateName: "Template " + variation.TemplateID, // Could fetch actual template name
			Recipients:   stats.TotalSent,                    // Use sent as recipients to match rate calculation denominator
			Delivered:    stats.TotalDelivered,
			Opens:        stats.TotalOpened,
			Clicks:       stats.TotalClicked,
			OpenRate:     openRate,
			ClickRate:    clickRate,
		}

		// Calculate score for recommendation (if not auto-send winner mode)
		if !broadcast.TestSettings.AutoSendWinner && broadcast.WinningTemplate == nil {
			score := (clickRate * 0.7) + (openRate * 0.3)
			if score > bestScore {
				bestScore = score
				recommendedWinner = variation.TemplateID
			}
		}
	}

	// Get winning template as string for response
	winningTemplate := ""
	if broadcast.WinningTemplate != nil {
		winningTemplate = *broadcast.WinningTemplate
	}

	return &domain.TestResultsResponse{
		BroadcastID:       broadcastID,
		Status:            string(broadcast.Status),
		TestStartedAt:     broadcast.StartedAt,
		TestCompletedAt:   broadcast.TestSentAt,
		VariationResults:  variationResults,
		RecommendedWinner: recommendedWinner,
		WinningTemplate:   winningTemplate, // Include actual winner if selected
		IsAutoSendWinner:  broadcast.TestSettings.AutoSendWinner,
	}, nil
}

// SelectWinner manually selects the winning variation for an A/B test
func (s *BroadcastService) SelectWinner(ctx context.Context, workspaceID, broadcastID, templateID string) error {
	// Authenticate user
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}

	return s.repo.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		// Get broadcast
		broadcast, err := s.repo.GetBroadcastTx(ctx, tx, workspaceID, broadcastID)
		if err != nil {
			return err
		}

		// Validate status - allow winner selection during testing phase if auto_send_winner is false
		validForWinnerSelection := false
		if broadcast.Status == domain.BroadcastStatusTestCompleted {
			validForWinnerSelection = true
		} else if broadcast.Status == domain.BroadcastStatusTesting && !broadcast.TestSettings.AutoSendWinner {
			// Allow manual winner selection during test phase when auto_send_winner is false
			validForWinnerSelection = true
		} else if broadcast.Status == domain.BroadcastStatusProcessing && broadcast.TestSettings.Enabled && !broadcast.TestSettings.AutoSendWinner {
			// Allow manual winner selection during sending phase for A/B tests when auto_send_winner is false
			// This handles the case where the test phase is very short and the status hasn't been updated to "testing" yet
			validForWinnerSelection = true
		}

		if !validForWinnerSelection {
			return fmt.Errorf("broadcast is not in test completed state")
		}

		// Validate template ID
		validTemplate := false
		for _, variation := range broadcast.TestSettings.Variations {
			if variation.TemplateID == templateID {
				validTemplate = true
				break
			}
		}
		if !validTemplate {
			return fmt.Errorf("invalid template ID")
		}

		// Update broadcast with winning template
		broadcast.WinningTemplate = &templateID // Store the winning TemplateID
		broadcast.Status = domain.BroadcastStatusWinnerSelected
		broadcast.UpdatedAt = time.Now().UTC()

		if err := s.repo.UpdateBroadcastTx(ctx, tx, broadcast); err != nil {
			return err
		}

		// Resume the associated task by finding it and updating its status
		task, err := s.taskRepo.GetTaskByBroadcastID(ctx, workspaceID, broadcastID)
		if err != nil {
			s.logger.WithField("broadcast_id", broadcastID).Debug("No task found for broadcast")
			return nil // Not an error if no task exists
		}

		// Resume the task
		nextRunAfter := time.Now().UTC()
		task.NextRunAfter = &nextRunAfter
		task.Status = domain.TaskStatusPending

		if updateErr := s.taskRepo.Update(ctx, workspaceID, task); updateErr != nil {
			return updateErr
		}

		// Immediately trigger task execution after winner selection (if auto-execution is enabled)
		// Note: We always trigger here since winner selection should immediately resume sending
		// In tests, this is disabled to prevent race conditions
		if s.taskService.IsAutoExecuteEnabled() {
			go func() {
				// Small delay to ensure transaction is committed
				time.Sleep(100 * time.Millisecond)
				if execErr := s.taskService.ExecutePendingTasks(context.Background(), 1); execErr != nil {
					s.logger.WithFields(map[string]interface{}{
						"broadcast_id": broadcastID,
						"task_id":      task.ID,
						"error":        execErr.Error(),
					}).Error("Failed to trigger immediate task execution after winner selection")
				}
			}()
		}

		return nil
	})
}

// RefreshGlobalFeed refreshes the global feed data for a broadcast
func (s *BroadcastService) RefreshGlobalFeed(ctx context.Context, request *domain.RefreshGlobalFeedRequest) (*domain.RefreshGlobalFeedResponse, error) {
	// Validate the request
	if err := request.Validate(); err != nil {
		return nil, err
	}

	// Authenticate user for workspace
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		s.logger.WithField("broadcast_id", request.BroadcastID).Error("Failed to authenticate user for workspace")
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Get the broadcast (needed for payload: broadcast name, audience list)
	broadcast, err := s.repo.GetBroadcast(ctx, request.WorkspaceID, request.BroadcastID)
	if err != nil {
		return nil, err
	}

	// Build feed settings from request
	feedSettings := &domain.GlobalFeedSettings{
		Enabled: true,
		URL:     request.URL,
		Headers: request.Headers,
	}

	// Get workspace and list information for the payload
	workspace, err := s.workspaceRepo.GetByID(ctx, request.WorkspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", request.WorkspaceID).Error("Failed to get workspace for global feed refresh")
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	var listName string
	if broadcast.Audience.List != "" {
		list, listErr := s.listService.GetListByID(ctx, request.WorkspaceID, broadcast.Audience.List)
		if listErr != nil {
			s.logger.WithField("list_id", broadcast.Audience.List).Warn("Failed to get list for global feed payload")
		} else if list != nil {
			listName = list.Name
		}
	}

	// Build the payload
	payload := &domain.GlobalFeedRequestPayload{
		Broadcast: domain.GlobalFeedBroadcast{
			ID:   broadcast.ID,
			Name: broadcast.Name,
		},
		List: domain.GlobalFeedList{
			ID:   broadcast.Audience.List,
			Name: listName,
		},
		Workspace: domain.GlobalFeedWorkspace{
			ID:   workspace.ID,
			Name: workspace.Name,
		},
	}

	// Fetch the global feed data
	feedData, fetchErr := s.dataFeedFetcher.FetchGlobal(ctx, feedSettings, payload)
	if fetchErr != nil {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcast.ID,
			"url":          request.URL,
			"error":        fetchErr.Error(),
		}).Error("Failed to fetch global feed")

		return &domain.RefreshGlobalFeedResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to fetch global feed: %s", fetchErr.Error()),
		}, nil
	}

	now := time.Now().UTC()

	s.logger.WithFields(map[string]interface{}{
		"broadcast_id": broadcast.ID,
		"data_keys":    len(feedData),
	}).Info("Global feed data fetched successfully")

	return &domain.RefreshGlobalFeedResponse{
		Success:   true,
		Data:      feedData,
		FetchedAt: &now,
	}, nil
}

// TestRecipientFeed tests the recipient feed configuration with a sample or specified contact
func (s *BroadcastService) TestRecipientFeed(ctx context.Context, request *domain.TestRecipientFeedRequest) (*domain.TestRecipientFeedResponse, error) {
	// Validate the request
	if err := request.Validate(); err != nil {
		return nil, err
	}

	// Authenticate user for workspace
	ctx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		s.logger.WithField("broadcast_id", request.BroadcastID).Error("Failed to authenticate user for workspace")
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Get the broadcast (needed for payload: broadcast name, audience list)
	broadcast, err := s.repo.GetBroadcast(ctx, request.WorkspaceID, request.BroadcastID)
	if err != nil {
		return nil, err
	}

	// Build feed settings from request
	feedSettings := &domain.RecipientFeedSettings{
		Enabled: true,
		URL:     request.URL,
		Headers: request.Headers,
	}

	// Get or create a sample contact for testing
	var contact *domain.Contact
	var contactEmail string

	if request.ContactEmail != "" {
		// Use the specified contact
		contact, err = s.contactRepo.GetContactByEmail(ctx, request.WorkspaceID, request.ContactEmail)
		if err != nil {
			if err == domain.ErrContactNotFound {
				return nil, &domain.ErrContactNotFoundForFeed{Email: request.ContactEmail}
			}
			s.logger.WithFields(map[string]interface{}{
				"workspace_id": request.WorkspaceID,
				"email":        request.ContactEmail,
				"error":        err.Error(),
			}).Error("Failed to get contact for recipient feed test")
			return nil, fmt.Errorf("failed to get contact: %w", err)
		}
		contactEmail = contact.Email
	} else {
		// Create a sample contact for testing (not persisted)
		contact = &domain.Contact{
			Email:     "sample@example.com",
			FirstName: &domain.NullableString{String: "Sample", IsNull: false},
			LastName:  &domain.NullableString{String: "Contact", IsNull: false},
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		contactEmail = contact.Email
	}

	// Get workspace and list information for the payload
	workspace, err := s.workspaceRepo.GetByID(ctx, request.WorkspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", request.WorkspaceID).Error("Failed to get workspace for recipient feed test")
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	var listName string
	if broadcast.Audience.List != "" {
		list, listErr := s.listService.GetListByID(ctx, request.WorkspaceID, broadcast.Audience.List)
		if listErr != nil {
			s.logger.WithField("list_id", broadcast.Audience.List).Warn("Failed to get list for recipient feed payload")
		} else if list != nil {
			listName = list.Name
		}
	}

	// Build the recipient feed payload
	payload := &domain.RecipientFeedRequestPayload{
		Contact: domain.BuildRecipientFeedContact(contact),
		Broadcast: domain.RecipientFeedBroadcast{
			ID:   broadcast.ID,
			Name: broadcast.Name,
		},
		List: domain.RecipientFeedList{
			ID:   broadcast.Audience.List,
			Name: listName,
		},
		Workspace: domain.RecipientFeedWorkspace{
			ID: workspace.ID,
		},
	}

	// Fetch the recipient feed data
	feedData, fetchErr := s.dataFeedFetcher.FetchRecipient(ctx, feedSettings, payload)
	if fetchErr != nil {
		s.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcast.ID,
			"url":          request.URL,
			"error":        fetchErr.Error(),
		}).Warn("Failed to fetch recipient feed during test")

		// Return a response with the error (not a hard error, since we want to communicate the fetch error)
		return &domain.TestRecipientFeedResponse{
			Success:      false,
			Error:        fmt.Sprintf("failed to fetch recipient feed: %s", fetchErr.Error()),
			ContactEmail: contactEmail,
		}, nil
	}

	now := time.Now().UTC()
	s.logger.WithFields(map[string]interface{}{
		"broadcast_id":  broadcast.ID,
		"contact_email": contactEmail,
		"data_keys":     len(feedData),
	}).Info("Recipient feed test completed successfully")

	return &domain.TestRecipientFeedResponse{
		Success:      true,
		Data:         feedData,
		FetchedAt:    &now,
		ContactEmail: contactEmail,
	}, nil
}

// ValidateSlug checks if slug is valid format (no nanoid - clean slugs)
func ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug cannot be empty")
	}

	if len(slug) > 100 {
		return fmt.Errorf("slug too long (max 100 characters)")
	}

	// Check format: lowercase letters, numbers, and hyphens only
	for _, r := range slug {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return fmt.Errorf("slug must contain only lowercase letters, numbers, and hyphens")
		}
	}

	return nil
}
