package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/mailer"
	"github.com/asaskevich/govalidator"
	"github.com/google/uuid"
)

type WorkspaceService struct {
	repo                   domain.WorkspaceRepository
	userRepo               domain.UserRepository
	taskRepo               domain.TaskRepository
	logger                 logger.Logger
	userService            domain.UserServiceInterface
	authService            domain.AuthService
	mailer                 mailer.Mailer
	config                 *config.Config
	contactService         domain.ContactService
	listService            domain.ListService
	contactListService     domain.ContactListService
	templateService        domain.TemplateService
	webhookRegService      domain.WebhookRegistrationService
	supabaseService        *SupabaseService
	secretKey              string
	dnsVerificationService *DNSVerificationService
}

func NewWorkspaceService(
	repo domain.WorkspaceRepository,
	userRepo domain.UserRepository,
	taskRepo domain.TaskRepository,
	logger logger.Logger,
	userService domain.UserServiceInterface,
	authService domain.AuthService,
	mailerInstance mailer.Mailer,
	config *config.Config,
	contactService domain.ContactService,
	listService domain.ListService,
	contactListService domain.ContactListService,
	templateService domain.TemplateService,
	webhookRegService domain.WebhookRegistrationService,
	secretKey string,
) *WorkspaceService {
	return &WorkspaceService{
		repo:               repo,
		userRepo:           userRepo,
		taskRepo:           taskRepo,
		logger:             logger,
		userService:        userService,
		authService:        authService,
		mailer:             mailerInstance,
		config:             config,
		contactService:     contactService,
		listService:        listService,
		contactListService: contactListService,
		templateService:    templateService,
		webhookRegService:  webhookRegService,
		secretKey:          secretKey,
	}
}

// SetSupabaseService sets the Supabase service (used to avoid circular dependencies)
func (s *WorkspaceService) SetSupabaseService(supabaseService *SupabaseService) {
	s.supabaseService = supabaseService
}

// SetDNSVerificationService sets the DNS verification service
func (s *WorkspaceService) SetDNSVerificationService(dnsVerificationService *DNSVerificationService) {
	s.dnsVerificationService = dnsVerificationService
}

// ListWorkspaces returns all workspaces for a user
func (s *WorkspaceService) ListWorkspaces(ctx context.Context) ([]*domain.Workspace, error) {
	user, err := s.authService.AuthenticateUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	userWorkspaces, err := s.repo.GetUserWorkspaces(ctx, user.ID)
	if err != nil {
		s.logger.WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to get user workspaces")
		return nil, err
	}

	// Return empty array if user has no workspaces
	if len(userWorkspaces) == 0 {
		return []*domain.Workspace{}, nil
	}

	workspaces := make([]*domain.Workspace, 0, len(userWorkspaces))
	for _, uw := range userWorkspaces {
		workspace, err := s.repo.GetByID(ctx, uw.WorkspaceID)
		if err != nil {
			s.logger.WithField("workspace_id", uw.WorkspaceID).WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to get workspace by ID")
			return nil, err
		}
		workspaces = append(workspaces, workspace)
	}

	return workspaces, nil
}

// GetWorkspace returns a workspace by ID if the user has access
func (s *WorkspaceService) GetWorkspace(ctx context.Context, id string) (*domain.Workspace, error) {
	// Check if this is a system call that should bypass authentication
	if ctx.Value(domain.SystemCallKey) == nil {
		// Validate user is a member of the workspace
		var user *domain.User
		var err error
		ctx, user, _, err = s.authService.AuthenticateUserForWorkspace(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to authenticate user: %w", err)
		}

		_, err = s.repo.GetUserWorkspace(ctx, user.ID, id)
		if err != nil {
			s.logger.WithField("workspace_id", id).WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to get user workspace")
			return nil, err
		}
	}

	workspace, err := s.repo.GetByID(ctx, id)
	if err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to get workspace by ID")
		return nil, err
	}

	return workspace, nil
}

// CreateWorkspace creates a new workspace and adds the creator as owner
func (s *WorkspaceService) CreateWorkspace(ctx context.Context, id string, name string, websiteURL string, logoURL string, coverURL string, timezone string, fileManager domain.FileManagerSettings) (*domain.Workspace, error) {
	user, err := s.authService.AuthenticateUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Only allow root user to create workspaces
	if user.Email != s.config.RootEmail {
		s.logger.WithField("user_email", user.Email).WithField("root_email", s.config.RootEmail).Error("Non-root user attempted to create workspace")
		return nil, &domain.ErrUnauthorized{Message: "only root user can create workspaces"}
	}

	randomSecretKey, err := GenerateSecureKey(32) // 32 bytes = 256 bits
	if err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to generate secure key")
		return nil, err
	}

	// For development environments, use a fixed secret key
	if s.config.IsDevelopment() {
		randomSecretKey = "secret_key_for_dev_env"
	}

	workspace := &domain.Workspace{
		ID:   id,
		Name: name,
		Settings: domain.WorkspaceSettings{
			WebsiteURL:           websiteURL,
			LogoURL:              logoURL,
			CoverURL:             coverURL,
			Timezone:             timezone,
			FileManager:          fileManager,
			SecretKey:            randomSecretKey,
			EmailTrackingEnabled: true,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := workspace.Validate(s.secretKey); err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to validate workspace")
		return nil, err
	}

	// check if workspace already exists
	if existingWorkspace, _ := s.repo.GetByID(ctx, id); existingWorkspace != nil {
		s.logger.WithField("workspace_id", id).Error("Workspace already exists")
		return nil, fmt.Errorf("workspace already exists")
	}

	if err := s.repo.Create(ctx, workspace); err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to create workspace")
		return nil, err
	}

	// Add the creator as owner
	userWorkspace := &domain.UserWorkspace{
		UserID:      user.ID,
		WorkspaceID: id,
		Role:        "owner",
		Permissions: domain.FullPermissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if err := userWorkspace.Validate(); err != nil {
		s.logger.WithField("workspace_id", id).WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to validate user workspace")
		return nil, err
	}

	if err := s.repo.AddUserToWorkspace(ctx, userWorkspace); err != nil {
		s.logger.WithField("workspace_id", id).WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to add user to workspace")
		return nil, err
	}

	// Get user details to create contact
	userDetails, err := s.userService.GetUserByID(ctx, user.ID)
	if err != nil {
		s.logger.WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to get user details for contact creation")
		return nil, err
	}

	// Create contact for the owner
	contact := &domain.Contact{
		Email:     userDetails.Email,
		FirstName: &domain.NullableString{String: userDetails.Name, IsNull: false},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := contact.Validate(); err != nil {
		s.logger.WithField("workspace_id", id).WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to validate contact")
		return nil, err
	}

	operation := s.contactService.UpsertContact(ctx, id, contact)
	if operation.Action == domain.UpsertContactOperationError {
		s.logger.WithField("workspace_id", id).WithField("user_id", user.ID).WithField("error", operation.Error).Error("Failed to create contact for owner")
		return nil, fmt.Errorf("%s", operation.Error)
	}

	// create a default list for the workspace
	list := &domain.List{
		ID:            "test",
		Name:          "Test List",
		IsDoubleOptin: false,
		IsPublic:      false,
		Description:   "This is a test list",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	err = s.listService.CreateList(ctx, id, list)
	if err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to create default list for workspace")
		return nil, err
	}

	err = s.listService.SubscribeToLists(ctx, &domain.SubscribeToListsRequest{
		WorkspaceID: id,
		Contact: domain.Contact{
			Email: userDetails.Email,
		},
		ListIDs: []string{list.ID},
	}, true)
	if err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to create default contact list for workspace")
		return nil, err
	}

	// Create permanent contact segment queue processing task for this workspace
	if err := EnsureContactSegmentQueueProcessingTask(ctx, s.taskRepo, id); err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to create contact segment queue processing task")
		// Don't fail workspace creation if task creation fails - it can be created later
	}

	// Create permanent segment recompute checking task for this workspace
	if err := EnsureSegmentRecomputeTask(ctx, s.taskRepo, id); err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to create segment recompute task")
		// Don't fail workspace creation if task creation fails - it can be created later
	}

	return workspace, nil
}

// UpdateWorkspace updates a workspace if the user is an owner
func (s *WorkspaceService) UpdateWorkspace(ctx context.Context, id string, name string, settings domain.WorkspaceSettings) (*domain.Workspace, error) {
	// Check if user can access this workspace
	var user *domain.User
	var err error
	ctx, user, _, err = s.authService.AuthenticateUserForWorkspace(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check if user is an owner
	userWorkspace, err := s.repo.GetUserWorkspace(ctx, user.ID, id)
	if err != nil {
		s.logger.WithField("workspace_id", id).WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to get user workspace")
		return nil, err
	}

	if userWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", id).WithField("user_id", user.ID).WithField("role", userWorkspace.Role).Error("User is not an owner of the workspace")
		return nil, &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}

	// Get the existing workspace to preserve integrations and other fields
	existingWorkspace, err := s.repo.GetByID(ctx, id)
	if err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to get existing workspace")
		return nil, err
	}

	existingWorkspace.Name = name
	existingWorkspace.Settings.WebsiteURL = settings.WebsiteURL
	existingWorkspace.Settings.LogoURL = settings.LogoURL
	existingWorkspace.Settings.CoverURL = settings.CoverURL
	existingWorkspace.Settings.Timezone = settings.Timezone
	existingWorkspace.Settings.FileManager = settings.FileManager
	existingWorkspace.Settings.TransactionalEmailProviderID = settings.TransactionalEmailProviderID
	existingWorkspace.Settings.MarketingEmailProviderID = settings.MarketingEmailProviderID
	existingWorkspace.Settings.EmailTrackingEnabled = settings.EmailTrackingEnabled

	// Verify DNS ownership if custom endpoint URL is being set or changed
	if settings.CustomEndpointURL != nil && *settings.CustomEndpointURL != "" {
		isDomainChanging := existingWorkspace.Settings.CustomEndpointURL == nil ||
			*existingWorkspace.Settings.CustomEndpointURL != *settings.CustomEndpointURL

		if isDomainChanging && s.dnsVerificationService != nil {
			// Verify DNS ownership
			if err := s.dnsVerificationService.VerifyDomainOwnership(ctx, *settings.CustomEndpointURL); err != nil {
				s.logger.
					WithField("workspace_id", id).
					WithField("domain", *settings.CustomEndpointURL).
					WithField("error", err.Error()).
					Warn("DNS verification failed")
				return nil, fmt.Errorf("domain verification failed: %w", err)
			}

			s.logger.
				WithField("workspace_id", id).
				WithField("domain", *settings.CustomEndpointURL).
				Info("DNS verification successful")
		}
	}

	existingWorkspace.Settings.CustomEndpointURL = settings.CustomEndpointURL
	existingWorkspace.Settings.CustomFieldLabels = settings.CustomFieldLabels

	// Handle template blocks - ensure they have proper timestamps and IDs
	for i := range settings.TemplateBlocks {
		block := &settings.TemplateBlocks[i]

		// If this is a new block (no ID), generate one and set created time
		if block.ID == "" {
			block.ID = uuid.New().String()
			block.Created = time.Now().UTC()
		}

		// Always update the Updated timestamp
		block.Updated = time.Now().UTC()
	}
	existingWorkspace.Settings.TemplateBlocks = settings.TemplateBlocks

	existingWorkspace.UpdatedAt = time.Now().UTC()

	if err := existingWorkspace.Validate(s.secretKey); err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to validate workspace")
		return nil, err
	}

	if err := s.repo.Update(ctx, existingWorkspace); err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to update workspace")
		return nil, err
	}

	return existingWorkspace, nil
}

// DeleteWorkspace deletes a workspace if the user is an owner
func (s *WorkspaceService) DeleteWorkspace(ctx context.Context, id string) error {
	// Check if user can access this workspace and is the owner
	var user *domain.User
	var err error
	ctx, user, _, err = s.authService.AuthenticateUserForWorkspace(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	userWorkspace, err := s.repo.GetUserWorkspace(ctx, user.ID, id)
	if err != nil {
		s.logger.WithField("workspace_id", id).WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to get user workspace")
		return err
	}

	if userWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", id).WithField("user_id", user.ID).WithField("role", userWorkspace.Role).Error("User is not an owner of the workspace")
		return &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}

	// Get the workspace to retrieve all integrations
	workspace, err := s.repo.GetByID(ctx, id)
	if err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to get workspace")
		return err
	}

	// Delete all integrations before deleting the workspace
	for _, integration := range workspace.Integrations {
		err = s.DeleteIntegration(ctx, id, integration.ID)
		if err != nil {
			s.logger.WithField("workspace_id", id).WithField("integration_id", integration.ID).WithField("error", err.Error()).Warn("Failed to delete integration during workspace deletion")
			// Continue with other integrations even if one fails
		}
	}

	// Delete all tasks for this workspace (including the queue processing task)
	if err := s.taskRepo.DeleteAll(ctx, id); err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Warn("Failed to delete tasks during workspace deletion")
		// Continue with workspace deletion even if task deletion fails
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to delete workspace")
		return err
	}

	return nil
}

// AddUserToWorkspace adds a user to a workspace if the requester is an owner
func (s *WorkspaceService) AddUserToWorkspace(ctx context.Context, workspaceID string, userID string, role string, permissions domain.UserPermissions) error {
	var user *domain.User
	var err error
	ctx, user, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check if requester is an owner
	requesterWorkspace, err := s.repo.GetUserWorkspace(ctx, user.ID, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", userID).WithField("requester_id", user.ID).WithField("error", err.Error()).Error("Failed to get requester workspace")
		return err
	}

	if requesterWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", userID).WithField("requester_id", user.ID).WithField("role", requesterWorkspace.Role).Error("Requester is not an owner of the workspace")
		return &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}

	// Use the permissions passed as parameter

	userWorkspace := &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        role,
		Permissions: permissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if err := userWorkspace.Validate(); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", userID).WithField("error", err.Error()).Error("Failed to validate user workspace")
		return err
	}

	if err := s.repo.AddUserToWorkspace(ctx, userWorkspace); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", userID).WithField("error", err.Error()).Error("Failed to add user to workspace")
		return err
	}

	return nil
}

// RemoveUserFromWorkspace removes a user from a workspace if the requester is an owner
func (s *WorkspaceService) RemoveUserFromWorkspace(ctx context.Context, workspaceID string, userID string) error {
	// Check if requester is an owner
	var owner *domain.User
	var err error
	ctx, owner, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	requesterWorkspace, err := s.repo.GetUserWorkspace(ctx, owner.ID, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", userID).WithField("requester_id", owner.ID).WithField("error", err.Error()).Error("Failed to get requester workspace")
		return err
	}

	if requesterWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", userID).WithField("requester_id", owner.ID).WithField("role", requesterWorkspace.Role).Error("Requester is not an owner of the workspace")
		return &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}

	// Prevent users from removing themselves
	if userID == owner.ID {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", userID).Error("Cannot remove self from workspace")
		return fmt.Errorf("cannot remove yourself from the workspace")
	}

	if err := s.repo.RemoveUserFromWorkspace(ctx, userID, workspaceID); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", userID).WithField("error", err.Error()).Error("Failed to remove user from workspace")
		return err
	}

	return nil
}

// TransferOwnership transfers the ownership of a workspace from the current owner to a member
func (s *WorkspaceService) TransferOwnership(ctx context.Context, workspaceID string, newOwnerID string, currentOwnerID string) error {
	// Authenticate the user
	var err error
	ctx, _, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check if current owner is actually an owner
	currentOwnerWorkspace, err := s.repo.GetUserWorkspace(ctx, currentOwnerID, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("current_owner_id", currentOwnerID).WithField("new_owner_id", newOwnerID).WithField("error", err.Error()).Error("Failed to get current owner workspace")
		return err
	}

	if currentOwnerWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", workspaceID).WithField("current_owner_id", currentOwnerID).WithField("role", currentOwnerWorkspace.Role).Error("Current owner is not an owner of the workspace")
		return &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}

	// Check if new owner exists and is a member
	newOwnerWorkspace, err := s.repo.GetUserWorkspace(ctx, newOwnerID, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("new_owner_id", newOwnerID).WithField("error", err.Error()).Error("Failed to get new owner workspace")
		return err
	}

	if newOwnerWorkspace.Role != "member" {
		s.logger.WithField("workspace_id", workspaceID).WithField("new_owner_id", newOwnerID).WithField("role", newOwnerWorkspace.Role).Error("New owner must be a current member of the workspace")
		return fmt.Errorf("new owner must be a current member of the workspace")
	}

	// Update new owner's role to owner
	newOwnerWorkspace.Role = "owner"
	newOwnerWorkspace.Permissions = domain.FullPermissions
	newOwnerWorkspace.UpdatedAt = time.Now().UTC()

	if err := s.repo.AddUserToWorkspace(ctx, newOwnerWorkspace); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("new_owner_id", newOwnerID).WithField("error", err.Error()).Error("Failed to update new owner's role")
		return err
	}

	// Update current owner's role to member
	currentOwnerWorkspace.Role = "member"
	currentOwnerWorkspace.UpdatedAt = time.Now().UTC()
	if err := s.repo.AddUserToWorkspace(ctx, currentOwnerWorkspace); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("current_owner_id", currentOwnerID).WithField("error", err.Error()).Error("Failed to update current owner's role")
		return err
	}

	return nil
}

// InviteMember creates an invitation for a user to join a workspace
func (s *WorkspaceService) InviteMember(ctx context.Context, workspaceID, email string, permissions domain.UserPermissions) (*domain.WorkspaceInvitation, string, error) {
	var inviter *domain.User
	var err error
	ctx, inviter, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Validate email format
	if !govalidator.IsEmail(email) {
		return nil, "", fmt.Errorf("invalid email format")
	}

	// Check if workspace exists
	workspace, err := s.repo.GetByID(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get workspace for invitation")
		return nil, "", err
	}
	if workspace == nil {
		return nil, "", fmt.Errorf("workspace not found")
	}

	// Check if the inviter has permission to invite members (is a member of the workspace)
	isMember, err := s.repo.IsUserWorkspaceMember(ctx, inviter.ID, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("inviter_id", inviter.ID).WithField("error", err.Error()).Error("Failed to check if inviter is a member")
		return nil, "", err
	}
	if !isMember {
		return nil, "", fmt.Errorf("inviter is not a member of the workspace")
	}

	// Get inviter user details for the email
	inviterDetails, err := s.userService.GetUserByID(ctx, inviter.ID)
	if err != nil {
		s.logger.WithField("inviter_id", inviter.ID).WithField("error", err.Error()).Error("Failed to get inviter details")
		return nil, "", err
	}
	inviterName := inviterDetails.Name
	if inviterName == "" {
		inviterName = inviterDetails.Email
	}

	// Check if user already exists with this email
	existingUser, err := s.userService.GetUserByEmail(ctx, email)
	if err == nil && existingUser != nil {
		// User exists, check if they're already a member
		isMember, err := s.repo.IsUserWorkspaceMember(ctx, existingUser.ID, workspaceID)
		if err != nil {
			s.logger.WithField("workspace_id", workspaceID).WithField("user_id", existingUser.ID).WithField("error", err.Error()).Error("Failed to check if user is already a member")
			return nil, "", err
		}
		if isMember {
			return nil, "", fmt.Errorf("user is already a member of the workspace")
		}

		// User exists but is not a member, add them as a member
		userWorkspace := &domain.UserWorkspace{
			UserID:      existingUser.ID,
			WorkspaceID: workspaceID,
			Role:        "member", // Always set invited users as members
			Permissions: permissions,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}
		err = s.repo.AddUserToWorkspace(ctx, userWorkspace)
		if err != nil {
			s.logger.WithField("workspace_id", workspaceID).WithField("user_id", existingUser.ID).WithField("error", err.Error()).Error("Failed to add user to workspace")
			return nil, "", err
		}

		// Return nil invitation since user was directly added
		return nil, "", nil
	}

	// User doesn't exist or there was an error (treat as user doesn't exist for security)
	// Create an invitation
	invitationID := uuid.New().String()
	expiresAt := time.Now().UTC().Add(15 * 24 * time.Hour) // 15 days

	invitation := &domain.WorkspaceInvitation{
		ID:          invitationID,
		WorkspaceID: workspaceID,
		InviterID:   inviter.ID,
		Email:       email,
		Permissions: permissions,
		ExpiresAt:   expiresAt,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	err = s.repo.CreateInvitation(ctx, invitation)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("email", email).WithField("error", err.Error()).Error("Failed to create workspace invitation")
		return nil, "", err
	}

	// Generate a JWT token with the invitation details
	token := s.authService.GenerateInvitationToken(invitation)

	// Send invitation email in production mode
	if !s.config.IsDevelopment() {
		err = s.mailer.SendWorkspaceInvitation(email, workspace.Name, inviterName, token)
		if err != nil {
			s.logger.WithField("workspace_id", workspaceID).WithField("email", email).WithField("error", err.Error()).Error("Failed to send invitation email")
			// Continue even if email sending fails
		}

		// Only return the token in development mode
		return invitation, "", nil
	}

	// In development mode, return the token
	return invitation, token, nil
}

// SetUserPermissions sets the permissions for a user in a workspace
func (s *WorkspaceService) SetUserPermissions(ctx context.Context, workspaceID, userID string, permissions domain.UserPermissions) error {
	var currentUser *domain.User
	var err error
	ctx, currentUser, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check if the current user is the owner of the workspace
	userWorkspace, err := s.repo.GetUserWorkspace(ctx, currentUser.ID, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", currentUser.ID).WithField("error", err.Error()).Error("Failed to get user workspace for permission check")
		return fmt.Errorf("failed to verify workspace membership: %w", err)
	}

	if userWorkspace.Role != "owner" {
		return fmt.Errorf("only workspace owners can manage user permissions")
	}

	// Check if the target user exists in the workspace
	targetUserWorkspace, err := s.repo.GetUserWorkspace(ctx, userID, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("target_user_id", userID).WithField("error", err.Error()).Error("Failed to get target user workspace")
		return fmt.Errorf("user is not a member of the workspace")
	}

	// Prevent owners from modifying their own permissions or other owners' permissions
	if targetUserWorkspace.Role == "owner" {
		return fmt.Errorf("cannot modify permissions for workspace owners")
	}

	// Update the user's permissions
	targetUserWorkspace.SetPermissions(permissions)
	targetUserWorkspace.UpdatedAt = time.Now().UTC()

	err = s.repo.UpdateUserWorkspacePermissions(ctx, targetUserWorkspace)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("target_user_id", userID).WithField("error", err.Error()).Error("Failed to update user permissions")
		return fmt.Errorf("failed to update user permissions: %w", err)
	}

	// Invalidate all sessions for the user whose permissions were changed
	// This ensures they can't continue using old sessions with outdated permissions
	sessions, err := s.userRepo.GetSessionsByUserID(ctx, userID)
	if err != nil {
		s.logger.WithField("target_user_id", userID).WithField("error", err.Error()).Warn("Failed to get user sessions for invalidation")
		// Don't fail the entire operation if we can't get sessions
	} else {
		for _, session := range sessions {
			err = s.userRepo.DeleteSession(ctx, session.ID)
			if err != nil {
				s.logger.WithField("target_user_id", userID).WithField("session_id", session.ID).WithField("error", err.Error()).Warn("Failed to delete user session")
				// Continue with other sessions even if one fails
			}
		}

		if len(sessions) > 0 {
			s.logger.WithField("target_user_id", userID).WithField("sessions_invalidated", len(sessions)).Info("Invalidated user sessions after permission change")
		}
	}

	return nil
}

// GetWorkspaceMembersWithEmail returns all users with emails for a workspace, verifying the requester has access
func (s *WorkspaceService) GetWorkspaceMembersWithEmail(ctx context.Context, id string) ([]*domain.UserWorkspaceWithEmail, error) {
	// Check if user has access to the workspace
	var user *domain.User
	var err error
	ctx, user, _, err = s.authService.AuthenticateUserForWorkspace(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	_, err = s.repo.GetUserWorkspace(ctx, user.ID, id)
	if err != nil {
		s.logger.WithField("workspace_id", id).WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to get user workspace")
		return nil, &domain.ErrUnauthorized{Message: "You do not have access to this workspace"}
	}

	// Get all workspace users with emails
	members, err := s.repo.GetWorkspaceUsersWithEmail(ctx, id)
	if err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to get workspace users with email")
		return nil, err
	}

	// force all permissions to owners
	for _, member := range members {
		if member.Role == "owner" {
			member.Permissions = domain.FullPermissions
		}
	}

	// Get all workspace invitations
	invitations, err := s.repo.GetWorkspaceInvitations(ctx, id)
	if err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to get workspace invitations")
		return nil, err
	}

	// Convert invitations to UserWorkspaceWithEmail format
	now := time.Now().UTC()
	for _, invitation := range invitations {
		// Skip expired invitations
		if invitation.ExpiresAt.Before(now) {
			continue
		}

		// Create a UserWorkspaceWithEmail entry for the invitation
		invitationMember := &domain.UserWorkspaceWithEmail{
			UserWorkspace: domain.UserWorkspace{
				UserID:      "", // Empty for invitations as user doesn't exist yet
				WorkspaceID: invitation.WorkspaceID,
				Role:        "member",               // Invitations are typically for members
				Permissions: invitation.Permissions, // Include permissions from invitation
				CreatedAt:   invitation.CreatedAt,
				UpdatedAt:   invitation.UpdatedAt,
			},
			Email:               invitation.Email,
			Type:                domain.UserTypeUser, // Assume invited users are regular users
			InvitationExpiresAt: &invitation.ExpiresAt,
			InvitationID:        invitation.ID,
		}
		members = append(members, invitationMember)
	}

	return members, nil
}

// CreateAPIKey creates an API key for a workspace
func (s *WorkspaceService) CreateAPIKey(ctx context.Context, workspaceID string, emailPrefix string) (string, string, error) {
	// Validate user is a member of the workspace and has owner role
	var user *domain.User
	var err error
	ctx, user, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return "", "", fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check if user is an owner
	userWorkspace, err := s.repo.GetUserWorkspace(ctx, user.ID, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to get user workspace")
		return "", "", err
	}

	if userWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", user.ID).WithField("role", userWorkspace.Role).Error("User is not an owner of the workspace")
		return "", "", &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}

	// Generate an API email using the prefix
	// Extract domainName from API endpoint by removing any protocol prefix and path suffix
	domainName := s.config.APIEndpoint
	if strings.HasPrefix(domainName, "http://") {
		domainName = strings.TrimPrefix(domainName, "http://")
	} else if strings.HasPrefix(domainName, "https://") {
		domainName = strings.TrimPrefix(domainName, "https://")
	}
	if idx := strings.Index(domainName, "/"); idx != -1 {
		domainName = domainName[:idx]
	}
	apiEmail := emailPrefix + "@" + domainName

	// Create a user object for the API key
	apiUser := &domain.User{
		ID:        uuid.New().String(),
		Email:     apiEmail,
		Type:      domain.UserTypeAPIKey,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	err = s.userRepo.CreateUser(ctx, apiUser)
	if err != nil {
		// Check if this is a duplicate user error
		var userExistsErr *domain.ErrUserExists
		if errors.As(err, &userExistsErr) {
			s.logger.WithField("workspace_id", workspaceID).WithField("user_email", apiUser.Email).Error("API user already exists")
			return "", "", fmt.Errorf("this user already exists")
		}
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", apiUser.ID).WithField("error", err.Error()).Error("Failed to create API user")
		return "", "", err
	}

	// Create full permissions for API key

	newUserWorkspace := &domain.UserWorkspace{
		UserID:      apiUser.ID,
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.FullPermissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err = s.repo.AddUserToWorkspace(ctx, newUserWorkspace)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", apiUser.ID).WithField("error", err.Error()).Error("Failed to add API user to workspace")
		return "", "", err
	}

	// Generate the token using the auth service
	token := s.authService.GenerateAPIAuthToken(apiUser)

	return token, apiEmail, nil
}

// GetInvitationByID retrieves a workspace invitation by its ID
func (s *WorkspaceService) GetInvitationByID(ctx context.Context, invitationID string) (*domain.WorkspaceInvitation, error) {
	return s.repo.GetInvitationByID(ctx, invitationID)
}

// AcceptInvitation processes an invitation acceptance by creating a user if needed and adding them to the workspace
func (s *WorkspaceService) AcceptInvitation(ctx context.Context, invitationID, workspaceID, email string) (*domain.AuthResponse, error) {
	// Get the invitation to retrieve permissions
	invitation, err := s.repo.GetInvitationByID(ctx, invitationID)
	if err != nil {
		s.logger.WithField("invitation_id", invitationID).WithField("error", err.Error()).Error("Failed to get invitation")
		return nil, fmt.Errorf("invitation not found: %w", err)
	}

	// Check if user already exists
	existingUser, err := s.userService.GetUserByEmail(ctx, email)
	var user *domain.User

	if err != nil {
		// User doesn't exist, create a new one
		user = &domain.User{
			ID:        uuid.New().String(),
			Email:     email,
			Name:      "", // User can update this later
			Type:      domain.UserTypeUser,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}

		err = s.userRepo.CreateUser(ctx, user)
		if err != nil {
			s.logger.WithField("email", email).WithField("error", err.Error()).Error("Failed to create user for invitation acceptance")
			return nil, fmt.Errorf("failed to create user: %w", err)
		}

		s.logger.WithField("user_id", user.ID).WithField("email", email).Info("Created new user from invitation acceptance")
	} else {
		user = existingUser

		// Check if user is already a member of the workspace
		isMember, err := s.repo.IsUserWorkspaceMember(ctx, user.ID, workspaceID)
		if err != nil {
			s.logger.WithField("user_id", user.ID).WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to check if user is already a member")
			return nil, fmt.Errorf("failed to check workspace membership: %w", err)
		}

		if isMember {
			s.logger.WithField("user_id", user.ID).WithField("workspace_id", workspaceID).Info("User is already a member of the workspace")
			// Delete the invitation since it's no longer needed
			if err := s.repo.DeleteInvitation(ctx, invitationID); err != nil {
				s.logger.WithField("invitation_id", invitationID).WithField("error", err.Error()).Warn("Failed to delete invitation after finding user is already a member")
			}
			return nil, fmt.Errorf("user is already a member of the workspace")
		}
	}

	// Add user to workspace as a member with permissions from invitation
	userWorkspace := &domain.UserWorkspace{
		UserID:      user.ID,
		WorkspaceID: workspaceID,
		Role:        "member",               // Always set invited users as members
		Permissions: invitation.Permissions, // Use permissions from invitation
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	err = s.repo.AddUserToWorkspace(ctx, userWorkspace)
	if err != nil {
		s.logger.WithField("user_id", user.ID).WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to add user to workspace")
		return nil, fmt.Errorf("failed to add user to workspace: %w", err)
	}

	// Create a new session for the user
	sessionExpiry := time.Now().Add(24 * time.Hour * 30) // 30 days
	session := &domain.Session{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		ExpiresAt: sessionExpiry,
		CreatedAt: time.Now().UTC(),
	}

	err = s.userRepo.CreateSession(ctx, session)
	if err != nil {
		s.logger.WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to create session for invitation acceptance")
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Generate authentication token
	token := s.authService.GenerateUserAuthToken(user, session.ID, session.ExpiresAt)

	// Delete the invitation after successful acceptance
	err = s.repo.DeleteInvitation(ctx, invitationID)
	if err != nil {
		s.logger.WithField("invitation_id", invitationID).WithField("error", err.Error()).Warn("Failed to delete invitation after successful acceptance")
		// Don't return error here as the main operation succeeded
	}

	s.logger.WithField("user_id", user.ID).WithField("workspace_id", workspaceID).WithField("invitation_id", invitationID).Info("Successfully accepted invitation and created session")

	return &domain.AuthResponse{
		Token:     token,
		User:      *user,
		ExpiresAt: session.ExpiresAt,
	}, nil
}

// DeleteInvitation deletes a workspace invitation by its ID
func (s *WorkspaceService) DeleteInvitation(ctx context.Context, invitationID string) error {
	// Check if user has access to perform this action
	user, err := s.authService.AuthenticateUserFromContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Get the invitation to verify it exists and get the workspace ID
	invitation, err := s.repo.GetInvitationByID(ctx, invitationID)
	if err != nil {
		s.logger.WithField("invitation_id", invitationID).WithField("error", err.Error()).Error("Failed to get invitation")
		return fmt.Errorf("invitation not found: %w", err)
	}

	// Check if the user is a member of the workspace that the invitation belongs to
	_, err = s.repo.GetUserWorkspace(ctx, user.ID, invitation.WorkspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", invitation.WorkspaceID).WithField("user_id", user.ID).WithField("error", err.Error()).Error("User does not have access to this workspace")
		return &domain.ErrUnauthorized{Message: "You do not have access to this workspace"}
	}

	// Delete the invitation
	if err := s.repo.DeleteInvitation(ctx, invitationID); err != nil {
		s.logger.WithField("invitation_id", invitationID).WithField("error", err.Error()).Error("Failed to delete invitation")
		return fmt.Errorf("failed to delete invitation: %w", err)
	}

	s.logger.WithField("invitation_id", invitationID).WithField("email", invitation.Email).Info("Successfully deleted invitation")
	return nil
}

// RemoveMember removes a member from a workspace and deletes the user if it's an API key
func (s *WorkspaceService) RemoveMember(ctx context.Context, workspaceID string, userIDToRemove string) error {
	// Authenticate the user making the request
	var requester *domain.User
	var err error
	ctx, requester, _, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check if requester is an owner
	requesterWorkspace, err := s.repo.GetUserWorkspace(ctx, requester.ID, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", userIDToRemove).WithField("requester_id", requester.ID).WithField("error", err.Error()).Error("Failed to get requester workspace")
		return err
	}

	if requesterWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", userIDToRemove).WithField("requester_id", requester.ID).WithField("role", requesterWorkspace.Role).Error("Requester is not an owner of the workspace")
		return &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}

	// Prevent owners from removing themselves
	if userIDToRemove == requester.ID {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", userIDToRemove).Error("Cannot remove self from workspace")
		return fmt.Errorf("cannot remove yourself from the workspace")
	}

	// Get the complete user to check its type
	userDetails, err := s.userService.GetUserByID(ctx, userIDToRemove)
	if err != nil {
		s.logger.WithField("user_id", userIDToRemove).WithField("error", err.Error()).Error("Failed to get user details")
		return err
	}

	// Remove user from workspace
	if err := s.repo.RemoveUserFromWorkspace(ctx, userIDToRemove, workspaceID); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", userIDToRemove).WithField("error", err.Error()).Error("Failed to remove user from workspace")
		return err
	}

	// If it's an API key, delete the user completely
	if userDetails.Type == domain.UserTypeAPIKey {
		if err := s.userRepo.Delete(ctx, userIDToRemove); err != nil {
			s.logger.WithField("user_id", userIDToRemove).WithField("error", err.Error()).Error("Failed to delete API key user")
			// Continue even if delete fails - the user is already removed from workspace
		} else {
			s.logger.WithField("user_id", userIDToRemove).Info("API key user deleted successfully")
		}
	}

	return nil
}

// CreateIntegration creates a new integration for a workspace
func (s *WorkspaceService) CreateIntegration(ctx context.Context, req domain.CreateIntegrationRequest) (string, error) {
	// Authenticate user and verify they are an owner of the workspace
	ctx, user, _, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check if user is an owner
	userWorkspace, err := s.repo.GetUserWorkspace(ctx, user.ID, req.WorkspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", req.WorkspaceID).WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to get user workspace")
		return "", err
	}

	if userWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", req.WorkspaceID).WithField("user_id", user.ID).WithField("role", userWorkspace.Role).Error("User is not an owner of the workspace")
		return "", &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}

	// Get the workspace
	workspace, err := s.repo.GetByID(ctx, req.WorkspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", req.WorkspaceID).WithField("error", err.Error()).Error("Failed to get workspace")
		return "", err
	}

	// Create a unique ID for the integration
	integrationID := uuid.New().String()

	// Create the integration based on type
	integration := domain.Integration{
		ID:        integrationID,
		Name:      req.Name,
		Type:      req.Type,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	switch req.Type {
	case domain.IntegrationTypeEmail:
		integration.EmailProvider = req.Provider
	case domain.IntegrationTypeSupabase:
		integration.SupabaseSettings = req.SupabaseSettings
	}

	// Validate the integration
	if err := integration.Validate(s.secretKey); err != nil {
		s.logger.WithField("workspace_id", req.WorkspaceID).WithField("integration_id", integrationID).WithField("error", err.Error()).Error("Failed to validate integration")
		return "", err
	}

	// Add the integration to the workspace
	workspace.AddIntegration(integration)

	// Save the updated workspace
	if err := s.repo.Update(ctx, workspace); err != nil {
		s.logger.WithField("workspace_id", req.WorkspaceID).WithField("integration_id", integrationID).WithField("error", err.Error()).Error("Failed to update workspace with new integration")
		return "", err
	}

	// Handle type-specific post-creation tasks
	switch req.Type {
	case domain.IntegrationTypeEmail:
		// Register webhooks for email integrations (except SMTP)
		if s.webhookRegService != nil && req.Provider.Kind != domain.EmailProviderKindSMTP {
			eventTypes := []domain.EmailEventType{
				domain.EmailEventDelivered,
				domain.EmailEventBounce,
				domain.EmailEventComplaint,
			}

			webhookConfig := &domain.WebhookRegistrationConfig{
				IntegrationID: integrationID,
				EventTypes:    eventTypes,
			}

			_, err := s.webhookRegService.RegisterWebhooks(ctx, req.WorkspaceID, webhookConfig)
			if err != nil {
				s.logger.WithField("workspace_id", req.WorkspaceID).
					WithField("integration_id", integrationID).
					WithField("error", err.Error()).
					Warn("Failed to register webhooks for new integration, but integration was created successfully")
			}
		}

	case domain.IntegrationTypeSupabase:
		// Create default templates and transactional notifications for Supabase integration
		if s.supabaseService != nil {
			// Create templates first
			mappings, err := s.supabaseService.CreateDefaultSupabaseTemplates(ctx, req.WorkspaceID, integrationID)
			if err != nil {
				s.logger.WithField("workspace_id", req.WorkspaceID).
					WithField("integration_id", integrationID).
					WithField("error", err.Error()).
					Error("Failed to create default Supabase templates")
				// Don't fail the integration creation, templates can be created manually
			} else {
				// Create transactional notifications that reference the templates
				err = s.supabaseService.CreateDefaultSupabaseNotifications(ctx, req.WorkspaceID, integrationID, mappings)
				if err != nil {
					s.logger.WithField("workspace_id", req.WorkspaceID).
						WithField("integration_id", integrationID).
						WithField("error", err.Error()).
						Error("Failed to create default Supabase notifications")
					// Don't fail the integration creation
				}
			}
		}
	}

	return integrationID, nil
}

// UpdateIntegration updates an existing integration in a workspace
func (s *WorkspaceService) UpdateIntegration(ctx context.Context, req domain.UpdateIntegrationRequest) error {
	// Authenticate user and verify they are an owner of the workspace
	ctx, user, _, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check if user is an owner
	userWorkspace, err := s.repo.GetUserWorkspace(ctx, user.ID, req.WorkspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", req.WorkspaceID).WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to get user workspace")
		return err
	}

	if userWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", req.WorkspaceID).WithField("user_id", user.ID).WithField("role", userWorkspace.Role).Error("User is not an owner of the workspace")
		return &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}

	// Get the workspace
	workspace, err := s.repo.GetByID(ctx, req.WorkspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", req.WorkspaceID).WithField("error", err.Error()).Error("Failed to get workspace")
		return err
	}

	// Find the existing integration
	existingIntegration := workspace.GetIntegrationByID(req.IntegrationID)
	if existingIntegration == nil {
		s.logger.WithField("workspace_id", req.WorkspaceID).WithField("integration_id", req.IntegrationID).Error("Integration not found")
		return fmt.Errorf("integration not found")
	}

	// Update the integration
	updatedIntegration := domain.Integration{
		ID:        req.IntegrationID,
		Name:      req.Name,
		Type:      existingIntegration.Type, // Type cannot be changed
		CreatedAt: existingIntegration.CreatedAt,
		UpdatedAt: time.Now().UTC(),
	}

	// Update type-specific settings
	switch existingIntegration.Type {
	case domain.IntegrationTypeEmail:
		updatedIntegration.EmailProvider = req.Provider
	case domain.IntegrationTypeSupabase:
		// Preserve existing encrypted keys if new keys are not provided
		if req.SupabaseSettings != nil {
			// Start with the new settings
			updatedIntegration.SupabaseSettings = req.SupabaseSettings

			// If auth email hook signature key is not provided, preserve the existing one
			if req.SupabaseSettings.AuthEmailHook.SignatureKey == "" &&
				req.SupabaseSettings.AuthEmailHook.EncryptedSignatureKey == "" &&
				existingIntegration.SupabaseSettings != nil {
				updatedIntegration.SupabaseSettings.AuthEmailHook.EncryptedSignatureKey =
					existingIntegration.SupabaseSettings.AuthEmailHook.EncryptedSignatureKey
			}

			// If before user created hook signature key is not provided, preserve the existing one
			if req.SupabaseSettings.BeforeUserCreatedHook.SignatureKey == "" &&
				req.SupabaseSettings.BeforeUserCreatedHook.EncryptedSignatureKey == "" &&
				existingIntegration.SupabaseSettings != nil {
				updatedIntegration.SupabaseSettings.BeforeUserCreatedHook.EncryptedSignatureKey =
					existingIntegration.SupabaseSettings.BeforeUserCreatedHook.EncryptedSignatureKey
			}
		} else {
			// If no settings provided, preserve existing
			updatedIntegration.SupabaseSettings = existingIntegration.SupabaseSettings
		}
	}

	// Validate the updated integration
	if err := updatedIntegration.Validate(s.secretKey); err != nil {
		s.logger.WithField("workspace_id", req.WorkspaceID).WithField("integration_id", req.IntegrationID).WithField("error", err.Error()).Error("Failed to validate updated integration")
		return err
	}

	// Update the integration in the workspace
	workspace.AddIntegration(updatedIntegration) // This will replace the existing one

	// Save the updated workspace
	if err := s.repo.Update(ctx, workspace); err != nil {
		s.logger.WithField("workspace_id", req.WorkspaceID).WithField("integration_id", req.IntegrationID).WithField("error", err.Error()).Error("Failed to update workspace with updated integration")
		return err
	}

	return nil
}

// DeleteIntegration deletes an integration from a workspace
func (s *WorkspaceService) DeleteIntegration(ctx context.Context, workspaceID, integrationID string) error {
	// Authenticate user and verify they are an owner of the workspace
	ctx, user, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check if user is an owner
	userWorkspace, err := s.repo.GetUserWorkspace(ctx, user.ID, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", user.ID).WithField("error", err.Error()).Error("Failed to get user workspace")
		return err
	}

	if userWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", user.ID).WithField("role", userWorkspace.Role).Error("User is not an owner of the workspace")
		return &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}

	// Get the workspace
	workspace, err := s.repo.GetByID(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get workspace")
		return err
	}

	// Find the integration to get its type before removal
	integration := workspace.GetIntegrationByID(integrationID)
	if integration == nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("integration_id", integrationID).Error("Integration not found")
		return fmt.Errorf("integration not found")
	}

	// Handle type-specific cleanup before removing the integration
	switch integration.Type {
	case domain.IntegrationTypeEmail:
		// Attempt to unregister webhooks for email integrations (except SMTP which doesn't support webhooks)
		if s.webhookRegService != nil && integration.EmailProvider.Kind != domain.EmailProviderKindSMTP {
			// Try to get webhook status to check what's registered
			status, err := s.webhookRegService.GetWebhookStatus(ctx, workspaceID, integrationID)
			if err != nil {
				// Just log the error, don't prevent deletion
				s.logger.WithField("workspace_id", workspaceID).
					WithField("integration_id", integrationID).
					WithField("error", err.Error()).
					Warn("Failed to get webhook status during integration deletion")
			} else if status != nil && status.IsRegistered {
				// Log that we're removing webhooks
				s.logger.WithField("workspace_id", workspaceID).
					WithField("integration_id", integrationID).
					Info("Unregistering webhooks for integration that is being deleted")

				// Use the dedicated method to unregister webhooks
				err := s.webhookRegService.UnregisterWebhooks(ctx, workspaceID, integrationID)
				if err != nil {
					s.logger.WithField("workspace_id", workspaceID).
						WithField("integration_id", integrationID).
						WithField("error", err.Error()).
						Warn("Failed to unregister webhooks during integration deletion, continuing with deletion anyway")
				}
			}
		}

	case domain.IntegrationTypeSupabase:
		// Delete all templates and transactional notifications associated with this integration
		if s.supabaseService != nil {
			err := s.deleteSupabaseIntegrationResources(ctx, workspaceID, integrationID)
			if err != nil {
				s.logger.WithField("workspace_id", workspaceID).
					WithField("integration_id", integrationID).
					WithField("error", err.Error()).
					Warn("Failed to delete Supabase integration resources, continuing with deletion anyway")
			}
		}
	}

	// Attempt to remove the integration
	if !workspace.RemoveIntegration(integrationID) {
		s.logger.WithField("workspace_id", workspaceID).WithField("integration_id", integrationID).Error("Integration not found")
		return fmt.Errorf("integration not found")
	}

	// Check if the integration is referenced in workspace settings
	if workspace.Settings.TransactionalEmailProviderID == integrationID {
		workspace.Settings.TransactionalEmailProviderID = ""
	}
	if workspace.Settings.MarketingEmailProviderID == integrationID {
		workspace.Settings.MarketingEmailProviderID = ""
	}

	// Save the updated workspace
	if err := s.repo.Update(ctx, workspace); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("integration_id", integrationID).WithField("error", err.Error()).Error("Failed to update workspace after integration deletion")
		return err
	}

	return nil
}

// deleteSupabaseIntegrationResources deletes all templates and transactional notifications associated with a Supabase integration
func (s *WorkspaceService) deleteSupabaseIntegrationResources(ctx context.Context, workspaceID, integrationID string) error {
	// Delegate to the Supabase service which has access to all necessary repositories
	if s.supabaseService == nil {
		return fmt.Errorf("supabase service not available")
	}

	return s.supabaseService.DeleteIntegrationResources(ctx, workspaceID, integrationID)
}

// GenerateSecureKey generates a cryptographically secure random key
// with the specified byte length and returns it as a hex-encoded string
func GenerateSecureKey(byteLength int) (string, error) {
	key := make([]byte, byteLength)
	_, err := rand.Read(key)
	if err != nil {
		return "", fmt.Errorf("failed to generate secure key: %w", err)
	}
	return hex.EncodeToString(key), nil
}
