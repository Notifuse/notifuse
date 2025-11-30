package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/cache"
	"github.com/Notifuse/notifuse/pkg/disposable_emails"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/google/uuid"
)

type ListService struct {
	repo            domain.ListRepository
	workspaceRepo   domain.WorkspaceRepository
	contactListRepo domain.ContactListRepository
	contactRepo     domain.ContactRepository
	authService     domain.AuthService
	emailService    domain.EmailServiceInterface
	logger          logger.Logger
	apiEndpoint     string
	blogCache       cache.Cache
}

func NewListService(repo domain.ListRepository, workspaceRepo domain.WorkspaceRepository, contactListRepo domain.ContactListRepository, contactRepo domain.ContactRepository, authService domain.AuthService, emailService domain.EmailServiceInterface, logger logger.Logger, apiEndpoint string, blogCache cache.Cache) *ListService {
	return &ListService{
		repo:            repo,
		workspaceRepo:   workspaceRepo,
		contactListRepo: contactListRepo,
		contactRepo:     contactRepo,
		authService:     authService,
		emailService:    emailService,
		logger:          logger,
		apiEndpoint:     apiEndpoint,
		blogCache:       blogCache,
	}
}

func (s *ListService) CreateList(ctx context.Context, workspaceID string, list *domain.List) error {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing lists
	if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceLists,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to lists required",
		)
	}

	now := time.Now().UTC()
	list.CreatedAt = now
	list.UpdatedAt = now

	if err := list.Validate(); err != nil {
		return fmt.Errorf("invalid list: %w", err)
	}

	if err := s.repo.CreateList(ctx, workspaceID, list); err != nil {
		s.logger.WithField("list_id", list.ID).Error(fmt.Sprintf("Failed to create list: %v", err))
		return fmt.Errorf("failed to create list: %w", err)
	}

	// Clear blog cache since public lists may be displayed on blog pages
	s.blogCache.Clear()

	return nil
}

func (s *ListService) GetListByID(ctx context.Context, workspaceID string, id string) (*domain.List, error) {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading lists
	if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceLists,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to lists required",
		)
	}

	list, err := s.repo.GetListByID(ctx, workspaceID, id)
	if err != nil {
		if _, ok := err.(*domain.ErrListNotFound); ok {
			return nil, err
		}
		s.logger.WithField("list_id", id).Error(fmt.Sprintf("Failed to get list: %v", err))
		return nil, fmt.Errorf("failed to get list: %w", err)
	}

	return list, nil
}

func (s *ListService) GetLists(ctx context.Context, workspaceID string) ([]*domain.List, error) {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading lists
	if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceLists,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to lists required",
		)
	}

	lists, err := s.repo.GetLists(ctx, workspaceID)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to get lists: %v", err))
		return nil, fmt.Errorf("failed to get lists: %w", err)
	}

	return lists, nil
}

func (s *ListService) UpdateList(ctx context.Context, workspaceID string, list *domain.List) error {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing lists
	if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceLists,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to lists required",
		)
	}

	list.UpdatedAt = time.Now().UTC()

	if err := list.Validate(); err != nil {
		return fmt.Errorf("invalid list: %w", err)
	}

	if err := s.repo.UpdateList(ctx, workspaceID, list); err != nil {
		s.logger.WithField("list_id", list.ID).Error(fmt.Sprintf("Failed to update list: %v", err))
		return fmt.Errorf("failed to update list: %w", err)
	}

	// Clear blog cache since public lists may be displayed on blog pages
	s.blogCache.Clear()

	return nil
}

func (s *ListService) DeleteList(ctx context.Context, workspaceID string, id string) error {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing lists
	if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceLists,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to lists required",
		)
	}

	if err := s.repo.DeleteList(ctx, workspaceID, id); err != nil {
		s.logger.WithField("list_id", id).Error(fmt.Sprintf("Failed to delete list: %v", err))
		return fmt.Errorf("failed to delete list: %w", err)
	}

	// Clear blog cache since public lists may be displayed on blog pages
	s.blogCache.Clear()

	return nil
}

func (s *ListService) GetListStats(ctx context.Context, workspaceID string, id string) (*domain.ListStats, error) {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading lists
	if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceLists,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to lists required",
		)
	}

	stats, err := s.repo.GetListStats(ctx, workspaceID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get list stats: %w", err)
	}

	return stats, nil
}

// this method is used to subscribe a contact to a list
// request can come from 3 different sources:
// 1. API
// 2. Frontend (authenticated with email and email_hmac)
// 3. Frontend (unauthenticated with email)
func (s *ListService) SubscribeToLists(ctx context.Context, payload *domain.SubscribeToListsRequest, hasBearerToken bool) error {
	var err error

	// fail silently if the email is disposable
	if disposable_emails.IsDisposableEmail(payload.Contact.Email) {
		return nil
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, payload.WorkspaceID)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to get workspace: %v", err))
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	isAuthenticated := false

	var userWorkspace *domain.UserWorkspace

	if hasBearerToken {
		ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspace.ID)
		if err != nil {
			return fmt.Errorf("failed to authenticate user: %w", err)
		}

		// Check permission for writing lists
		if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeWrite) {
			return domain.NewPermissionError(
				domain.PermissionResourceLists,
				domain.PermissionTypeWrite,
				"Insufficient permissions: write access to lists required",
			)
		}

		isAuthenticated = true
	} else if payload.Contact.EmailHMAC != "" {

		secretKey := workspace.Settings.SecretKey
		if !domain.VerifyEmailHMAC(payload.Contact.Email, payload.Contact.EmailHMAC, secretKey) {
			return fmt.Errorf("invalid email verification")
		}

		isAuthenticated = true
	}

	// if the contact is not authenticated we only allow inserting the contact to avoid public frontend injections
	canUpsert := true
	if !isAuthenticated {
		// check if the contact already exists
		if existingContact, _ := s.contactRepo.GetContactByEmail(ctx, workspace.ID, payload.Contact.Email); existingContact != nil {
			canUpsert = false
		}
	}

	if canUpsert {
		// upsert the contact
		_, err = s.contactRepo.UpsertContact(ctx, workspace.ID, &payload.Contact)
		if err != nil {
			s.logger.WithField("email", payload.Contact.Email).Error(fmt.Sprintf("Failed to upsert contact: %v", err))
			return fmt.Errorf("failed to upsert contact: %w", err)
		}
	}

	// get the lists
	lists, err := s.repo.GetLists(ctx, workspace.ID)
	if err != nil {
		s.logger.WithField("list_ids", payload.ListIDs).Error(fmt.Sprintf("Failed to get lists: %v", err))
		return fmt.Errorf("failed to get lists: %w", err)
	}

	// get the list
	for _, listID := range payload.ListIDs {

		var list *domain.List
		for _, l := range lists {
			if l.ID == listID {
				list = l
				break
			}
		}

		if list == nil {
			s.logger.WithField("list_id", listID).Error("List not found")
			return fmt.Errorf("list not found")
		}

		// reject if the list is not public and the request is not authenticated
		// (authenticated = bearer token OR valid HMAC from notification center)
		if !list.IsPublic && !isAuthenticated {
			return fmt.Errorf("list is not public")
		}

		contactList := &domain.ContactList{
			Email:     payload.Contact.Email,
			ListID:    listID,
			ListName:  list.Name,
			Status:    domain.ContactListStatusActive,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}

		// if the list is double optin and the contact is not authenticated, set the status to pending
		if list.IsDoubleOptin && !isAuthenticated {
			contactList.Status = domain.ContactListStatusPending
		}

		// Subscribe to the list
		err = s.contactListRepo.AddContactToList(ctx, workspace.ID, contactList)
		if err != nil {
			// codecov:ignore:start
			s.logger.WithField("email", contactList.Email).
				WithField("list_id", contactList.ListID).
				Error(fmt.Sprintf("Failed to subscribe to list: %v", err))
			// codecov:ignore:end
			return fmt.Errorf("failed to subscribe to list: %w", err)
		}

		marketingEmailProvider, integrationID, err := workspace.GetEmailProviderWithIntegrationID(true)
		if err != nil {
			s.logger.WithField("workspace_id", workspace.ID).Error(fmt.Sprintf("Failed to get marketing email provider: %v", err))
			return fmt.Errorf("failed to get marketing email provider: %w", err)
		}

		// if the marketing email provider is not set, we don't need to send the welcome email
		if marketingEmailProvider == nil {
			continue
		}

		// get contact
		contact, err := s.contactRepo.GetContactByEmail(ctx, workspace.ID, contactList.Email)
		if err != nil {
			s.logger.WithField("email", contactList.Email).Error(fmt.Sprintf("Failed to get contact: %v", err))
			return fmt.Errorf("failed to get contact: %w", err)
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
			UTMSource:      workspace.Settings.WebsiteURL,
			UTMMedium:      "email",
			UTMCampaign:    list.Name,
			UTMContent:     messageID,
			WorkspaceID:    workspace.ID,
			MessageID:      messageID,
		}

		req := domain.TemplateDataRequest{
			WorkspaceID:        workspace.ID,
			WorkspaceSecretKey: workspace.Settings.SecretKey,
			ContactWithList: domain.ContactWithList{
				Contact:  contact,
				ListID:   listID,
				ListName: list.Name,
			},
			MessageID:        messageID,
			TrackingSettings: trackingSettings,
			Broadcast:        nil,
		}
		templateData, err := domain.BuildTemplateData(req)

		if err != nil {
			s.logger.WithField("email", contactList.Email).Error(fmt.Sprintf("Failed to build template data: %v", err))
			return fmt.Errorf("failed to build template data: %w", err)
		}

		// send welcome email
		if contactList.Status == domain.ContactListStatusActive && list.WelcomeTemplate != nil {

			request := domain.SendEmailRequest{
				WorkspaceID:      workspace.ID,
				IntegrationID:    integrationID,
				MessageID:        messageID,
				ExternalID:       nil,
				Contact:          contact,
				TemplateConfig:   domain.ChannelTemplate{TemplateID: list.WelcomeTemplate.ID},
				MessageData:      domain.MessageData{Data: templateData},
				TrackingSettings: trackingSettings,
				EmailProvider:    marketingEmailProvider,
				EmailOptions:     domain.EmailOptions{},
			}
			err = s.emailService.SendEmailForTemplate(ctx, request)

			if err != nil {
				s.logger.WithField("email", contactList.Email).Error(fmt.Sprintf("Failed to send welcome email: %v", err))
				return fmt.Errorf("failed to send welcome email: %w", err)
			}
		}

		// double optin
		if contactList.Status == domain.ContactListStatusPending && list.DoubleOptInTemplate != nil {

			request := domain.SendEmailRequest{
				WorkspaceID:      workspace.ID,
				IntegrationID:    integrationID,
				MessageID:        messageID,
				ExternalID:       nil,
				Contact:          contact,
				TemplateConfig:   domain.ChannelTemplate{TemplateID: list.DoubleOptInTemplate.ID},
				MessageData:      domain.MessageData{Data: templateData},
				TrackingSettings: trackingSettings,
				EmailProvider:    marketingEmailProvider,
				EmailOptions:     domain.EmailOptions{},
			}
			err = s.emailService.SendEmailForTemplate(ctx, request)

			if err != nil {
				s.logger.WithField("email", contactList.Email).Error(fmt.Sprintf("Failed to send double optin email: %v", err))
				return fmt.Errorf("failed to send double optin email: %w", err)
			}
		}
	}

	return nil
}

// this method is used to unsubscribe a contact from a list
// request can come from 2 different sources:
// 1. API
// 2. Frontend (authenticated with email and email_hmac)
func (s *ListService) UnsubscribeFromLists(ctx context.Context, payload *domain.UnsubscribeFromListsRequest, hasBearerToken bool) error {
	var err error

	workspace, err := s.workspaceRepo.GetByID(ctx, payload.WorkspaceID)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to get workspace: %v", err))
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	var userWorkspace *domain.UserWorkspace
	if hasBearerToken {
		ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspace.ID)
		if err != nil {
			return fmt.Errorf("failed to authenticate user: %w", err)
		}

		// Check permission for writing lists
		if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeWrite) {
			return domain.NewPermissionError(
				domain.PermissionResourceLists,
				domain.PermissionTypeWrite,
				"Insufficient permissions: write access to lists required",
			)
		}
	} else {
		// verify contact hmac
		if payload.EmailHMAC == "" {
			return fmt.Errorf("email_hmac is required")
		}

		secretKey := workspace.Settings.SecretKey
		if !domain.VerifyEmailHMAC(payload.Email, payload.EmailHMAC, secretKey) {
			return fmt.Errorf("invalid email verification")
		}
	}

	// Get contact
	contact, err := s.contactRepo.GetContactByEmail(ctx, workspace.ID, payload.Email)
	if err != nil {
		s.logger.WithField("email", payload.Email).Error(fmt.Sprintf("Failed to get contact: %v", err))
		return fmt.Errorf("failed to get contact: %w", err)
	}

	// Get email provider for sending confirmation emails
	marketingEmailProvider, integrationID, err := workspace.GetEmailProviderWithIntegrationID(true)
	if err != nil {
		s.logger.WithField("workspace_id", workspace.ID).Error(fmt.Sprintf("Failed to get marketing email provider: %v", err))
		// We'll continue even if we can't get the email provider
		// This allows unsubscribe to work even if we can't send confirmation emails
	}

	// get the lists
	lists, err := s.repo.GetLists(ctx, workspace.ID)
	if err != nil {
		s.logger.WithField("list_ids", payload.ListIDs).Error(fmt.Sprintf("Failed to get lists: %v", err))
		return fmt.Errorf("failed to get lists: %w", err)
	}

	// Process each list for unsubscription
	for _, listID := range payload.ListIDs {
		var list *domain.List
		for _, l := range lists {
			if l.ID == listID {
				list = l
				break
			}
		}

		if list == nil {
			s.logger.WithField("list_id", listID).Error("List not found")
			return fmt.Errorf("list not found")
		}

		// Update contact's status to unsubscribed for this list
		err = s.contactListRepo.UpdateContactListStatus(ctx, workspace.ID, payload.Email, listID, domain.ContactListStatusUnsubscribed)
		if err != nil {
			s.logger.WithField("email", payload.Email).
				WithField("list_id", listID).
				Error(fmt.Sprintf("Failed to unsubscribe from list: %v", err))
			return fmt.Errorf("failed to unsubscribe from list: %w", err)
		}

		// Send unsubscribe confirmation email if template is set and email provider exists
		if list.UnsubscribeTemplate != nil && marketingEmailProvider != nil {
			messageID := uuid.New().String()

			// Use workspace CustomEndpointURL if provided, otherwise use the default API endpoint
			endpoint := s.apiEndpoint
			if workspace.Settings.CustomEndpointURL != nil && *workspace.Settings.CustomEndpointURL != "" {
				endpoint = *workspace.Settings.CustomEndpointURL
			}

			trackingSettings := notifuse_mjml.TrackingSettings{
				Endpoint:       endpoint,
				EnableTracking: workspace.Settings.EmailTrackingEnabled,
				UTMSource:      workspace.Settings.WebsiteURL,
				UTMMedium:      "email",
				UTMCampaign:    list.Name,
				UTMContent:     messageID,
				WorkspaceID:    workspace.ID,
				MessageID:      messageID,
			}

			req := domain.TemplateDataRequest{
				WorkspaceID:        workspace.ID,
				WorkspaceSecretKey: workspace.Settings.SecretKey,
				ContactWithList: domain.ContactWithList{
					Contact:  contact,
					ListID:   listID,
					ListName: list.Name,
				},
				MessageID:        messageID,
				TrackingSettings: trackingSettings,
				Broadcast:        nil,
			}
			templateData, err := domain.BuildTemplateData(req)

			if err != nil {
				s.logger.WithField("email", payload.Email).Error(fmt.Sprintf("Failed to build template data: %v", err))
				return fmt.Errorf("failed to build template data: %w", err)
			}

			request := domain.SendEmailRequest{
				WorkspaceID:      workspace.ID,
				IntegrationID:    integrationID,
				MessageID:        messageID,
				ExternalID:       nil,
				Contact:          contact,
				TemplateConfig:   domain.ChannelTemplate{TemplateID: list.UnsubscribeTemplate.ID},
				MessageData:      domain.MessageData{Data: templateData},
				TrackingSettings: trackingSettings,
				EmailProvider:    marketingEmailProvider,
				EmailOptions:     domain.EmailOptions{},
			}
			err = s.emailService.SendEmailForTemplate(ctx, request)

			if err != nil {
				s.logger.WithField("email", payload.Email).Error(fmt.Sprintf("Failed to send unsubscribe confirmation email: %v", err))
				return fmt.Errorf("failed to send unsubscribe confirmation email: %w", err)
			}
		}
	}
	return nil
}
