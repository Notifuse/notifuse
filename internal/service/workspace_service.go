package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
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
	blogService            *BlogService
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
	supabaseService *SupabaseService,
	dnsVerificationService *DNSVerificationService,
	blogService *BlogService,
) *WorkspaceService {
	return &WorkspaceService{
		repo:                   repo,
		userRepo:               userRepo,
		taskRepo:               taskRepo,
		logger:                 logger,
		userService:            userService,
		authService:            authService,
		mailer:                 mailerInstance,
		config:                 config,
		contactService:         contactService,
		listService:            listService,
		contactListService:     contactListService,
		templateService:        templateService,
		webhookRegService:      webhookRegService,
		secretKey:              secretKey,
		supabaseService:        supabaseService,
		dnsVerificationService: dnsVerificationService,
		blogService:            blogService,
	}
}

// ListWorkspaces returns all workspaces for a user
func (s *WorkspaceService) ListWorkspaces(ctx context.Context) ([]*domain.Workspace, error) {
	user, err := s.authService.AuthenticateUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Platform admins (ROOT_EMAIL) have owner access to every workspace, so they see them all.
	if s.config.IsRootEmail(user.Email) {
		return s.repo.List(ctx)
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
		// Validate user has access to the workspace. Platform admins (ROOT_EMAIL) get
		// owner access to every workspace via the AuthService override.
		var userWorkspace *domain.UserWorkspace
		var err error
		ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to authenticate user: %w", err)
		}

		// Read or write: the console derives access from read || write, so a member holding
		// only workspace:write reaches Settings and must be able to load the workspace it edits.
		if !userWorkspace.HasPermission(domain.PermissionResourceWorkspace, domain.PermissionTypeRead) &&
			!userWorkspace.HasPermission(domain.PermissionResourceWorkspace, domain.PermissionTypeWrite) {
			return nil, domain.NewPermissionError(
				domain.PermissionResourceWorkspace,
				domain.PermissionTypeRead,
				"Insufficient permissions: read access to workspace required",
			)
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
func (s *WorkspaceService) CreateWorkspace(ctx context.Context, id string, name string, websiteURL string, logoURL string, coverURL string, timezone string, fileManager domain.FileManagerSettings, defaultLanguage string, languages []string) (*domain.Workspace, error) {
	user, err := s.authService.AuthenticateUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Only allow root user to create workspaces
	if !s.config.IsRootEmail(user.Email) {
		s.logger.WithField("user_email", user.Email).WithField("root_email", s.config.RootEmail).Error("Non-root user attempted to create workspace")
		return nil, &domain.ErrUnauthorized{Message: "only root user can create workspaces"}
	}

	// Check workspace limit
	if s.config.MaxWorkspaces > 0 {
		count, err := s.repo.CountWorkspaces(ctx)
		if err != nil {
			s.logger.WithField("error", err.Error()).Error("Failed to count workspaces")
			return nil, err
		}
		if count >= s.config.MaxWorkspaces {
			return nil, &domain.ErrWorkspaceLimitReached{
				Limit:   s.config.MaxWorkspaces,
				Current: count,
			}
		}
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

	webAnalyticsDefaults := domain.DefaultWebFilters()
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
			DefaultLanguage:      defaultLanguage,
			Languages:            languages,
			WebAnalytics: &domain.WebAnalyticsSettings{
				Enabled:                false,
				BounceThresholdSeconds: domain.WebAnalyticsDefaultBounceThresholdSeconds,
				Filters:                webAnalyticsDefaults,
				FiltersVersion:         domain.ComputeWebFiltersVersion(webAnalyticsDefaults),
				GeoEnabled:             true,
				GeoStoreCity:           true,
				GeoStoreRegion:         true,
				GeoCoordsPrecision:     2,
			},
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

// preserveFileManagerSecret keeps the stored S3 credential when the caller's file
// manager block carries none.
//
// The sibling of preserveEmailProviderSecrets, and for the same reason: reads do
// not carry the secret to a machine caller (redactWorkspaceForCaller), so an API
// client echoing the settings object back has nothing to put in the field, and
// FileManagerSettings.Validate is happy without one — the wipe saved clean and was
// unrecoverable.
//
// An empty block is left alone: that is a file manager being switched off, and its
// credential goes with it.
func preserveFileManagerSecret(updated *domain.FileManagerSettings, stored domain.FileManagerSettings) {
	if updated.SecretKey != "" || updated.EncryptedSecretKey != "" {
		return // the caller is rotating it deliberately
	}
	if updated.Endpoint == "" && updated.Bucket == "" && updated.AccessKey == "" {
		return
	}
	updated.EncryptedSecretKey = stored.EncryptedSecretKey
}

// UpdateWorkspace updates a workspace if the user is an owner
func (s *WorkspaceService) UpdateWorkspace(ctx context.Context, id string, name string, settings domain.WorkspaceSettings) (*domain.Workspace, error) {
	// Check if user can access this workspace and is an owner. Platform admins (ROOT_EMAIL)
	// are synthesized as owners of every workspace by the AuthService override.
	var user *domain.User
	var userWorkspace *domain.UserWorkspace
	var err error
	ctx, user, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
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

	// Every field the assignment list below copies has a meaningful zero, so an
	// absent key and a deliberate blank arrive identical unless the decode said
	// which was which. Restore the stored value for each setting the body never
	// named, before anything reads settings.
	settings.PreserveOmitted(existingWorkspace.Settings)
	preserveFileManagerSecret(&settings.FileManager, existingWorkspace.Settings.FileManager)

	// This assignment list is an allowlist, and the omissions are deliberate. Blog
	// and web analytics settings are absent because each has its own endpoint gated
	// on that feature's write permission — see SetBlogSettings. Adding them back
	// here would let an owner saving general settings silently overwrite config a
	// delegated manager owns, since these forms resubmit the whole settings object.
	// Any new feature-scoped settings block belongs in its own setter, not here.
	existingWorkspace.Name = name
	existingWorkspace.Settings.WebsiteURL = settings.WebsiteURL
	existingWorkspace.Settings.LogoURL = settings.LogoURL
	existingWorkspace.Settings.CoverURL = settings.CoverURL
	existingWorkspace.Settings.Timezone = settings.Timezone
	existingWorkspace.Settings.FileManager = settings.FileManager
	existingWorkspace.Settings.TransactionalEmailProviderID = settings.TransactionalEmailProviderID
	// Reject a transactional-only provider (e.g. Mailjet) being NEWLY assigned as
	// the marketing provider. Only the transition is guarded: settings forms
	// resubmit the whole settings object, so an assignment that predates the
	// restriction must not block unrelated settings saves. Grandfathered
	// assignments are enforced at send time, in GetEmailProviderWithIntegrationID.
	if settings.MarketingEmailProviderID != "" &&
		settings.MarketingEmailProviderID != existingWorkspace.Settings.MarketingEmailProviderID {
		integration := existingWorkspace.GetIntegrationByID(settings.MarketingEmailProviderID)
		if integration != nil && integration.EmailProvider.Kind.IsTransactionalOnly() {
			return nil, fmt.Errorf("%s can only be used as a transactional email provider, not a marketing provider", integration.EmailProvider.Kind)
		}
	}

	existingWorkspace.Settings.MarketingEmailProviderID = settings.MarketingEmailProviderID
	existingWorkspace.Settings.EmailTrackingEnabled = settings.EmailTrackingEnabled

	// Verify DNS ownership if custom endpoint URL is being set or changed
	if settings.CustomEndpointURL != nil && *settings.CustomEndpointURL != "" {
		isDomainChanging := existingWorkspace.Settings.CustomEndpointURL == nil ||
			*existingWorkspace.Settings.CustomEndpointURL != *settings.CustomEndpointURL

		if isDomainChanging {
			// Verify DNS ownership
			if err := s.dnsVerificationService.VerifyDomainOwnership(ctx, *settings.CustomEndpointURL); err != nil {
				s.logger.
					WithField("workspace_id", id).
					WithField("domain", *settings.CustomEndpointURL).
					WithField("error", err.Error()).
					Warn("DNS verification failed")

				// In production, fail the request; in non-production, just log and continue
				if s.config.IsProduction() {
					// Return the validation error as-is without wrapping
					return nil, err
				}

				s.logger.
					WithField("workspace_id", id).
					WithField("domain", *settings.CustomEndpointURL).
					Info("DNS verification failed but continuing in non-production environment")
			} else {
				s.logger.
					WithField("workspace_id", id).
					WithField("domain", *settings.CustomEndpointURL).
					Info("DNS verification successful")
			}
		}
	}

	existingWorkspace.Settings.CustomEndpointURL = settings.CustomEndpointURL
	// Note: Custom field labels and blog settings are intentionally NOT updated here.
	// They are each managed exclusively via dedicated, permission-checked endpoints
	// (/api/workspaces.setCustomFieldLabels for labels, /api/workspaces.setBlogSettings
	// for the blog enable flag + config), which enforce granular permissions
	// (workspace:write and blog:write respectively) instead of requiring owner role.
	// This also prevents an owner's (possibly stale) settings save from clobbering
	// values set by a member. Existing labels and blog settings on existingWorkspace
	// are preserved as-is.
	existingWorkspace.Settings.DefaultLanguage = settings.DefaultLanguage
	existingWorkspace.Settings.Languages = settings.Languages

	// Handle template blocks - preserve existing blocks if not provided in update
	// Note: Template blocks should be managed via dedicated /api/templateBlocks.* endpoints
	// which support granular template permissions instead of requiring owner role.
	// This code is kept for backward compatibility.
	if settings.TemplateBlocks != nil {
		// Only update template blocks if explicitly provided in the request
		// Ensure they have proper timestamps and IDs
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
	}
	// If settings.TemplateBlocks is nil, preserve existing template blocks (don't overwrite)

	existingWorkspace.UpdatedAt = time.Now().UTC()

	if err := existingWorkspace.Validate(s.secretKey); err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to validate workspace")
		return nil, err
	}

	if err := s.repo.Update(ctx, existingWorkspace); err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to update workspace")
		return nil, err
	}

	// Blog themes are now created by the frontend when enabling the blog
	// No automatic theme creation in the backend

	return existingWorkspace, nil
}

// DeleteWorkspace deletes a workspace if the user is an owner
func (s *WorkspaceService) DeleteWorkspace(ctx context.Context, id string) error {
	// Check if user can access this workspace and is the owner. Platform admins (ROOT_EMAIL)
	// are synthesized as owners of every workspace by the AuthService override.
	var user *domain.User
	var userWorkspace *domain.UserWorkspace
	var err error
	ctx, user, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
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
	var requesterWorkspace *domain.UserWorkspace
	var err error
	ctx, user, requesterWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check if requester is an owner (platform admins are synthesized as owners)
	if requesterWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", userID).WithField("requester_id", user.ID).WithField("role", requesterWorkspace.Role).Error("Requester is not an owner of the workspace")
		return &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}

	if err := permissions.Validate(); err != nil {
		return err
	}

	// Check team member limit
	if s.config.MaxUsers > 0 {
		count, err := s.repo.CountWorkspaceMembersAndInvitations(ctx, workspaceID)
		if err != nil {
			s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to count workspace members")
			return err
		}
		if count >= s.config.MaxUsers {
			return &domain.ErrTeamMemberLimitReached{
				Limit:   s.config.MaxUsers,
				Current: count,
			}
		}
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
	// Check if requester is an owner (platform admins are synthesized as owners)
	var owner *domain.User
	var requesterWorkspace *domain.UserWorkspace
	var err error
	ctx, owner, requesterWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
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
	var inviterWorkspace *domain.UserWorkspace
	var err error
	ctx, inviter, inviterWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Owner-only (platform admins are synthesized as owners). An invitation carries an
	// arbitrary permission map, and for an email that already exists it seats the member
	// directly — so a scoped member, or a scoped API key, could otherwise mint an identity
	// with more access than its own.
	if inviterWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", workspaceID).WithField("requester_id", inviter.ID).WithField("role", inviterWorkspace.Role).Error("Requester is not an owner of the workspace")
		return nil, "", &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}

	// Validate email format
	if !govalidator.IsEmail(email) {
		return nil, "", fmt.Errorf("invalid email format")
	}

	if err := permissions.Validate(); err != nil {
		return nil, "", err
	}

	// Defense-in-depth: a configured platform admin (ROOT_EMAIL) already has owner access to
	// every workspace via the override, so inviting one is redundant. Rejecting it also closes a
	// theoretical path where inviting a not-yet-provisioned root address could create that root
	// identity through invitation acceptance.
	if s.config.IsRootEmail(email) {
		return nil, "", fmt.Errorf("cannot invite a platform admin email")
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

	// The inviter's access was already verified by AuthenticateUserForWorkspace and the
	// owner check above, so no extra membership check is needed.

	// Check team member limit
	if s.config.MaxUsers > 0 {
		count, err := s.repo.CountWorkspaceMembersAndInvitations(ctx, workspaceID)
		if err != nil {
			s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to count workspace members")
			return nil, "", err
		}
		if count >= s.config.MaxUsers {
			return nil, "", &domain.ErrTeamMemberLimitReached{
				Limit:   s.config.MaxUsers,
				Current: count,
			}
		}
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

	// Send invitation email in production mode.
	// This path only runs for invitees who do not yet have an account, so they
	// have no language preference of their own — the email is localized in the
	// inviter's language.
	if !s.config.IsDevelopment() {
		err = s.mailer.SendWorkspaceInvitation(email, workspace.Name, inviterName, token, inviterDetails.Language)
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
	var userWorkspace *domain.UserWorkspace
	var err error
	ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check if the current user is the owner of the workspace (platform admins are synthesized as owners)
	if userWorkspace.Role != "owner" {
		return &domain.ErrUnauthorized{Message: "only workspace owners can manage user permissions"}
	}

	if err := permissions.Validate(); err != nil {
		return err
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

	// Update the user's permissions. SetPermissions is a bare assignment, so store a copy:
	// a membership row must never share a map with the caller.
	targetUserWorkspace.SetPermissions(maps.Clone(permissions))
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

// SetCustomFieldLabels updates the custom field display labels for a workspace.
// Unlike most workspace settings (which are owner-only via UpdateWorkspace), this
// is the dedicated, granular-permission path: it requires write access to the
// workspace resource, so workspace owners and members with workspace:write (e.g.
// "full access") can manage custom field labels.
func (s *WorkspaceService) SetCustomFieldLabels(ctx context.Context, workspaceID string, labels map[string]string) error {
	var userWorkspace *domain.UserWorkspace
	var err error
	ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing workspace settings
	if !userWorkspace.HasPermission(domain.PermissionResourceWorkspace, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceWorkspace,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to workspace required",
		)
	}

	// Load the existing workspace and update only the custom field labels,
	// preserving all other settings.
	existingWorkspace, err := s.repo.GetByID(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get existing workspace")
		return err
	}

	existingWorkspace.Settings.CustomFieldLabels = labels

	// Canonical validation (covers non-console API consumers too)
	if err := existingWorkspace.Settings.ValidateCustomFieldLabels(); err != nil {
		return err
	}

	if err := s.repo.Update(ctx, existingWorkspace); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to update custom field labels")
		return err
	}

	return nil
}

// SetBlogSettings updates the workspace-level blog configuration (the enable flag
// plus title/SEO/pagination/feed settings). Unlike UpdateWorkspace (owner-only),
// this is gated on the granular blog:write permission so a delegated blog manager
// can manage blog config. It loads the workspace and mutates only the blog fields,
// preserving all other settings.
//
// It lives on WorkspaceService rather than BlogService because blog settings are
// fields of the WorkspaceSettings entity, and workspace-entity writes all go
// through this service's repository. Moving it to BlogService for feature cohesion
// would split those writes across two services; the permission gate is orthogonal
// and works from either home.
func (s *WorkspaceService) SetBlogSettings(ctx context.Context, workspaceID string, enabled *bool, settings *domain.BlogSettings, settingsSpecified bool) error {
	var userWorkspace *domain.UserWorkspace
	var err error
	ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Blog settings follow the blog feature's own permission, not workspace:write.
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to blog required",
		)
	}

	// Load the existing workspace and update only the blog fields, preserving all
	// other settings.
	existingWorkspace, err := s.repo.GetByID(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get existing workspace")
		return err
	}

	// A nil flag is a body that named no blog_enabled. Both of its values are
	// meaningful, so writing the zero one here would read "say nothing" as "turn the
	// blog off" — which is what the console's fallback exists to work around.
	if enabled != nil {
		existingWorkspace.Settings.BlogEnabled = *enabled
	}

	// The same reasoning one field down, with one twist: absence cannot be read off the
	// pointer here, because nil is already the deliberate "clear it". So a body that named no
	// blog_settings says so separately, and the stored title, SEO block, pagination and feed
	// configuration stand — which is what the console's disable button sends whenever the
	// settings fields are not on screen.
	if settingsSpecified {
		existingWorkspace.Settings.BlogSettings = settings
	}

	// Canonical validation (covers non-console API consumers too). Validate has a
	// nil-receiver guard, so a nil settings (disable/clear) is fine.
	if err := existingWorkspace.Settings.BlogSettings.Validate(); err != nil {
		return err
	}

	if err := s.repo.Update(ctx, existingWorkspace); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to update blog settings")
		return err
	}

	return nil
}

// SetWebAnalyticsSettings replaces the workspace's web analytics settings.
// Like blog settings, it is gated by the feature's own permission rather than
// workspace:write, and it preserves every other workspace setting.
func (s *WorkspaceService) SetWebAnalyticsSettings(ctx context.Context, workspaceID string, settings *domain.WebAnalyticsSettings) error {
	var userWorkspace *domain.UserWorkspace
	var err error
	ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	if !userWorkspace.HasPermission(domain.PermissionResourceWebAnalytics, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceWebAnalytics,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to web_analytics required",
		)
	}

	if err := settings.ValidateForSave(); err != nil {
		return err
	}

	// The filters version drives backfill staleness detection; never trust a
	// client-supplied hash.
	if settings != nil {
		settings.FiltersVersion = domain.ComputeWebFiltersVersion(settings.Filters)
	}

	existingWorkspace, err := s.repo.GetByID(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get existing workspace")
		return err
	}

	// No contacts:write gate here any more. It used to guard the two settings
	// that switched contact-timeline writing on; both are gone, because calling
	// identify() is now the opt-in and that decision is made in the customer's
	// own code with the workspace secret, not in this panel.

	existingWorkspace.Settings.WebAnalytics = settings

	if err := s.repo.Update(ctx, existingWorkspace); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to update web analytics settings")
		return err
	}

	return nil
}

// GetWorkspaceMembersWithEmail returns all users with emails for a workspace, verifying the requester has access
func (s *WorkspaceService) GetWorkspaceMembersWithEmail(ctx context.Context, id string) ([]*domain.UserWorkspaceWithEmail, error) {
	// Check if user has access to the workspace (platform admins get owner access via the override)
	var requester *domain.User
	var requesterWorkspace *domain.UserWorkspace
	var err error
	ctx, requester, requesterWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// This endpoint is the console's only source of the signed-in user's own permission map:
	// every navigation entry is derived from it and any error is caught into an all-false
	// set, so denying it outright empties the console instead of narrowing it. A requester
	// without workspace:read is degraded to its own row below rather than rejected.
	// The nil check must DENY: a nil-guard that skipped the check would hand the whole
	// roster to a caller with no membership row at all.
	fullRoster := requesterWorkspace != nil &&
		requesterWorkspace.HasPermission(domain.PermissionResourceWorkspace, domain.PermissionTypeRead)

	// Get all workspace users with emails
	members, err := s.repo.GetWorkspaceUsersWithEmail(ctx, id)
	if err != nil {
		s.logger.WithField("workspace_id", id).WithField("error", err.Error()).Error("Failed to get workspace users with email")
		return nil, err
	}

	// Degraded response: the requester's own row only. No owner rows, no pending invitations
	// and no platform admins — a zero-scope key has no business enumerating the team.
	if !fullRoster {
		own := make([]*domain.UserWorkspaceWithEmail, 0, 1)
		for _, member := range members {
			if requester != nil && member.UserID == requester.ID {
				own = append(own, member)
			}
		}
		return own, nil
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

	// Surface platform admins (ROOT_EMAIL) as virtual owner entries for visibility — but only
	// to a workspace owner (platform admins themselves are synthesized as owners), so operator
	// identities are not exposed to ordinary members. They have owner access to every workspace
	// via the AuthService override but may not hold a membership row.
	if requesterWorkspace != nil && requesterWorkspace.Role == "owner" {
		existingEmails := make(map[string]struct{}, len(members))
		for _, m := range members {
			existingEmails[m.Email] = struct{}{}
		}
		for _, rootEmail := range s.config.RootEmails() {
			if _, ok := existingEmails[rootEmail]; ok {
				continue
			}
			rootUser, err := s.userService.GetUserByEmail(ctx, rootEmail)
			if err != nil || rootUser == nil {
				// Root account not provisioned yet (e.g. never signed in) — skip.
				continue
			}
			members = append(members, &domain.UserWorkspaceWithEmail{
				UserWorkspace: domain.UserWorkspace{
					UserID:      rootUser.ID,
					WorkspaceID: id,
					Role:        "owner",
					Permissions: domain.FullPermissions,
					CreatedAt:   now,
					UpdatedAt:   now,
				},
				Email: rootEmail,
				Type:  domain.UserTypeUser,
			})
			existingEmails[rootEmail] = struct{}{}
		}
	}

	return members, nil
}

// CreateAPIKey creates an API key for a workspace. A nil permissions map grants the key
// full access, which is the pre-scoping contract for callers that omit the field.
func (s *WorkspaceService) CreateAPIKey(ctx context.Context, workspaceID string, emailPrefix string, permissions domain.UserPermissions) (string, string, error) {
	// Validate user is a member of the workspace and has owner role
	var user *domain.User
	var userWorkspace *domain.UserWorkspace
	var err error
	ctx, user, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		// A non-member is an authorization failure, not a server error: the sentinel is a
		// plain error, so translate it into the typed error handlers map to 403.
		if errors.Is(err, domain.ErrUserNotInWorkspace) {
			return "", "", &domain.ErrUnauthorized{Message: "user is not a member of the workspace"}
		}
		return "", "", fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check if user is an owner (platform admins are synthesized as owners)
	if userWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", user.ID).WithField("role", userWorkspace.Role).Error("User is not an owner of the workspace")
		return "", "", &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}

	if err := permissions.Validate(); err != nil {
		return "", "", err
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

	// Defense-in-depth, mirroring InviteMember: a configured platform admin (ROOT_EMAIL) has
	// owner access to every workspace, so a prefix that lands on a root address would mint a
	// platform-admin key rather than a workspace-scoped one.
	if s.config.IsRootEmail(apiEmail) {
		return "", "", &domain.ErrUnauthorized{Message: "cannot create an API key for a platform admin email"}
	}

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
			return "", "", fmt.Errorf("api key email already in use: %w", userExistsErr)
		}
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", apiUser.ID).WithField("error", err.Error()).Error("Failed to create API user")
		return "", "", err
	}

	// Store a copy: domain.FullPermissions is a package-level map, and a membership row
	// holding a reference to it — or to the caller's map — would let a later mutation
	// rewrite the permissions of every key that shares it.
	keyPermissions := domain.NewFullPermissions()
	if permissions != nil {
		keyPermissions = maps.Clone(permissions)
	}

	newUserWorkspace := &domain.UserWorkspace{
		UserID:      apiUser.ID,
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: keyPermissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err = s.repo.AddUserToWorkspace(ctx, newUserWorkspace)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", apiUser.ID).WithField("error", err.Error()).Error("Failed to add API user to workspace")

		// The users row is already written and users.email is unique across the whole
		// deployment, so leaving it behind burns the address for every workspace on this
		// installation and for good: with no membership row the key never appears on
		// Settings → Team, whose roster joins user_workspaces, and RemoveMember can never
		// reach it. Detached from ctx, which may be the very thing that just failed.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		if deleteErr := s.userRepo.Delete(cleanupCtx, apiUser.ID); deleteErr != nil {
			s.logger.WithField("workspace_id", workspaceID).
				WithField("user_id", apiUser.ID).
				WithField("user_email", apiUser.Email).
				Error(fmt.Sprintf("failed to delete the API user after it could not be added to the workspace, its address is now unusable installation-wide: %v", deleteErr))
		}

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

	// Check team member limit before adding to workspace.
	// Subtract 1 because the invitation being accepted is still counted in the total
	// but will be deleted after the user is added — accepting converts an invitation
	// into a member (net-zero change), so it should not block acceptance.
	if s.config.MaxUsers > 0 {
		count, err := s.repo.CountWorkspaceMembersAndInvitations(ctx, workspaceID)
		if err != nil {
			s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to count workspace members")
			return nil, err
		}
		if count-1 >= s.config.MaxUsers {
			return nil, &domain.ErrTeamMemberLimitReached{
				Limit:   s.config.MaxUsers,
				Current: count,
			}
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

	// Check if the user is a member of the workspace that the invitation belongs to.
	// This method authenticates via context (not AuthenticateUserForWorkspace), so the
	// platform-admin override is applied explicitly here: ROOT_EMAIL users have access
	// to every workspace even without a membership row.
	if _, err = s.repo.GetUserWorkspace(ctx, user.ID, invitation.WorkspaceID); err != nil {
		// Bypass only the genuine "not a member" case for platform admins so real DB errors
		// still surface instead of being silently treated as authorized.
		rootBypass := errors.Is(err, domain.ErrUserNotInWorkspace) && s.config.IsRootEmail(user.Email)
		if !rootBypass {
			s.logger.WithField("workspace_id", invitation.WorkspaceID).WithField("user_id", user.ID).WithField("error", err.Error()).Error("User does not have access to this workspace")
			return &domain.ErrUnauthorized{Message: "You do not have access to this workspace"}
		}
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
	var requesterWorkspace *domain.UserWorkspace
	var err error
	ctx, requester, requesterWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check if requester is an owner (platform admins are synthesized as owners)
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

	// If it's an API key, delete the user completely. Its token lives ten years, carries no
	// jti and has no denylist, so this delete is the only thing that revokes the key —
	// reporting success on a failed delete would report a revocation that never happened.
	if userDetails.Type == domain.UserTypeAPIKey {
		if err := s.userRepo.Delete(ctx, userIDToRemove); err != nil {
			s.logger.WithField("user_id", userIDToRemove).WithField("error", err.Error()).Error("Failed to delete API key user")
			return err
		}
		s.logger.WithField("user_id", userIDToRemove).Info("API key user deleted successfully")
	}

	return nil
}

// addIntegration validates an integration and writes it onto the workspace row. It is the tail
// CreateIntegration and ConnectZapier share: repo.Update rewrites the workspace wholesale, so
// every integration write is a read-modify-write of the whole row rather than an insert.
//
// The caller has already authenticated and built the integration; this does no authorization of
// its own and must not be reached before an owner check.
func (s *WorkspaceService) addIntegration(ctx context.Context, workspaceID string, integration domain.Integration) error {
	workspace, err := s.repo.GetByID(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get workspace")
		return err
	}

	if err := integration.Validate(s.secretKey); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("integration_id", integration.ID).WithField("error", err.Error()).Error("Failed to validate integration")
		return err
	}

	workspace.AddIntegration(integration)

	if err := s.repo.Update(ctx, workspace); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("integration_id", integration.ID).WithField("error", err.Error()).Error("Failed to update workspace with new integration")
		return err
	}

	return nil
}

// CreateIntegration creates a new integration for a workspace
func (s *WorkspaceService) CreateIntegration(ctx context.Context, req domain.CreateIntegrationRequest) (string, error) {
	// Authenticate user and verify they are an owner of the workspace
	// (platform admins are synthesized as owners).
	ctx, user, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("failed to authenticate user: %w", err)
	}

	if userWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", req.WorkspaceID).WithField("user_id", user.ID).WithField("role", userWorkspace.Role).Error("User is not an owner of the workspace")
		return "", &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
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

	// No zapier case, deliberately. The settings hold an address only ConnectZapier can mint,
	// so leaving the type unmatched here leaves ZapierSettings nil and Integration.Validate
	// rejects the record below. That second layer is what closes the service-level callers,
	// such as the demo seeder, which never reach CreateIntegrationRequest.Validate at all.
	switch req.Type {
	case domain.IntegrationTypeEmail:
		integration.EmailProvider = req.Provider
	case domain.IntegrationTypeSupabase:
		integration.SupabaseSettings = req.SupabaseSettings
	case domain.IntegrationTypeLLM:
		integration.LLMProvider = req.LLMProvider
	case domain.IntegrationTypeFirecrawl:
		integration.FirecrawlSettings = req.FirecrawlSettings
	}

	if err := s.addIntegration(ctx, req.WorkspaceID, integration); err != nil {
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

	return integrationID, nil
}

// zapierConnectAttempts caps how many addresses ConnectZapier tries before it gives up. Each
// attempt draws fresh randomness, so a collision on users.email costs a retry rather than the
// connection; five is far past the point where a collision stops being a coincidence.
const zapierConnectAttempts = 5

// ConnectZapier mints an API key for a Zapier connection and records it on the workspace as a
// zapier integration. The token is returned once and never stored anywhere; the address is the
// only thing the integration persists.
//
// It cannot delegate to CreateIntegration, and both reasons are deliberate rather than
// oversights: that path has no zapier case, so a record made through it arrives settings-less
// and Integration.Validate rejects it, and CreateIntegrationRequest carries no zapier settings
// field because the address is derived here, from randomness the client never sees. Together
// they are what stops a caller from inventing a connection whose address belongs to no key.
// What the two paths do share is addIntegration, the read-modify-write tail.
func (s *WorkspaceService) ConnectZapier(ctx context.Context, workspaceID string, label string) (string, string, string, error) {
	// Owner gate modelled on CreateAPIKey, not on CreateIntegration: that one wraps
	// ErrUserNotInWorkspace in a bare fmt.Errorf and a non-member comes back as a 500. Minting
	// a credential is CreateAPIKey's job, so it maps the same way here.
	var user *domain.User
	var userWorkspace *domain.UserWorkspace
	var err error
	// The enriched context is threaded into CreateAPIKey below on purpose: it carries the
	// authenticated user, and passing the original would make every attempt re-run the queries
	// behind AuthenticateUserForWorkspace instead of hitting its context cache.
	ctx, user, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotInWorkspace) {
			return "", "", "", &domain.ErrUnauthorized{Message: "user is not a member of the workspace"}
		}
		return "", "", "", fmt.Errorf("failed to authenticate user: %w", err)
	}

	if userWorkspace.Role != "owner" {
		s.logger.WithField("workspace_id", workspaceID).WithField("user_id", user.ID).WithField("role", userWorkspace.Role).Error("User is not an owner of the workspace")
		return "", "", "", &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}

	var token, email string
	for attempt := 1; ; attempt++ {
		randomHex, genErr := GenerateSecureKey(4)
		if genErr != nil {
			return "", "", "", fmt.Errorf("failed to generate zapier api key suffix: %w", genErr)
		}

		prefix, prefixErr := domain.ZapierKeyPrefix(label, randomHex)
		if prefixErr != nil {
			return "", "", "", prefixErr
		}

		var createErr error
		token, email, createErr = s.CreateAPIKey(ctx, workspaceID, prefix, domain.ZapierKeyPermissions())
		if createErr == nil {
			break
		}

		// CreateAPIKey wraps ErrUserExists rather than returning it, and ErrUserExists is a
		// struct type, not a sentinel: a retry written as errors.Is against a sentinel would
		// match nothing and never fire, without ever saying so.
		var userExistsErr *domain.ErrUserExists
		if !errors.As(createErr, &userExistsErr) {
			return "", "", "", createErr
		}

		if attempt == zapierConnectAttempts {
			// Still wrapping *ErrUserExists, so the handler maps an exhausted retry the same
			// way it maps the first collision.
			return "", "", "", fmt.Errorf("failed to mint a unique zapier api key address after %d attempts: %w", zapierConnectAttempts, createErr)
		}

		s.logger.WithField("workspace_id", workspaceID).
			WithField("api_key_prefix", prefix).
			Warn("Zapier API key address is already in use, retrying with fresh randomness")
	}

	integrationID := uuid.New().String()
	integration := domain.Integration{
		ID:   integrationID,
		Name: label,
		Type: domain.IntegrationTypeZapier,
		ZapierSettings: &domain.ZapierSettings{
			APIKeyEmail: email,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.addIntegration(ctx, workspaceID, integration); err != nil {
		// The key is live and the card that would let anyone find it again was never written.
		// Detached from ctx: a client disconnect is one of the ways the write above fails, and
		// the compensation would then be cancelled by the very thing it exists to compensate.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		if revokeErr := s.revokeZapierAPIKey(cleanupCtx, workspaceID, email); revokeErr != nil {
			s.logger.WithField("workspace_id", workspaceID).
				WithField("user_email", email).
				Error(fmt.Sprintf("failed to revoke the Zapier API key after its integration could not be saved, the key is live with nothing in the console pointing at it: %v", revokeErr))
		}

		return "", "", "", err
	}

	return token, email, integrationID, nil
}

// revokeZapierAPIKey deletes the API key a Zapier connection was minted with, found by the
// address the integration stores, which is the only handle on it: the user id is never persisted.
//
// Both halves run, in this order, exactly as RemoveMember does them. The token lives ten years
// and has no denylist, so deleting the user is the whole of the revocation, while a membership
// row left pointing at a deleted user is invisible in the console and permanently unremovable,
// since RemoveMember looks the user up first and errors on precisely those rows.
//
// Neither half being there already is an error, and for the same reason in both directions:
// nothing ties the two writes together, so an attempt that stops between them leaves one row
// behind, and every later attempt starts over from the top. A key that is already gone is the
// familiar case — the address outlives the user row whenever an owner revokes the key from
// Settings → Team. A membership that is already gone is the rarer one, and refusing it would
// wedge the card, and the live key it points at, for good.
func (s *WorkspaceService) revokeZapierAPIKey(ctx context.Context, workspaceID string, apiKeyEmail string) error {
	apiUser, err := s.userRepo.GetUserByEmail(ctx, apiKeyEmail)
	if err != nil {
		var notFoundErr *domain.ErrUserNotFound
		if errors.As(err, &notFoundErr) {
			s.logger.WithField("workspace_id", workspaceID).
				WithField("user_email", apiKeyEmail).
				Info("Zapier API key user no longer exists, nothing to revoke")
			return nil
		}
		return fmt.Errorf("failed to find the Zapier API key user: %w", err)
	}

	if err := s.repo.RemoveUserFromWorkspace(ctx, apiUser.ID, workspaceID); err != nil {
		// The removal reports a row it did not match the same way it reports a database that
		// is down, so ask which one this was rather than tolerate both. Only an absence we
		// can confirm is passed over; anything else still aborts, because the delete below
		// is the revocation the caller was promised.
		stillMember, memberErr := s.repo.IsUserWorkspaceMember(ctx, apiUser.ID, workspaceID)
		if memberErr != nil || stillMember {
			return fmt.Errorf("failed to remove the Zapier API key user from the workspace: %w", err)
		}

		s.logger.WithField("workspace_id", workspaceID).
			WithField("user_email", apiKeyEmail).
			Info("Zapier API key user has no membership left to remove, finishing the revocation")
	}

	if err := s.userRepo.Delete(ctx, apiUser.ID); err != nil {
		return fmt.Errorf("failed to delete the Zapier API key user: %w", err)
	}

	return nil
}

// UpdateIntegration updates an existing integration in a workspace
// preserveDerivedSESFields copies server-owned SES state from the stored integration onto the
// updated one. These fields are written by webhook registration and tenant provisioning, never by
// a client, so a client's value (or absence of one) must not win.
// hydrateEmailProviderCredentials fills blank credentials on a client-supplied
// provider from the stored one.
//
// Sibling of preserveEmailProviderSecrets, and the difference is which form it
// restores. That one runs on the SAVE path and restores the ciphertext, because
// the value is on its way back to the database. This one runs on paths that
// USE the provider immediately — testing an integration — where the provider
// services sign with the plaintext, so the ciphertext would be no help.
//
// The stored provider comes from a workspace the repository loaded, so AfterLoad
// has already decrypted it.
//
// Only a provider block the caller actually sent is touched, so switching kind
// and testing before saving cannot borrow the previous provider's credential, and
// a credential the caller typed always wins — otherwise a wrong new key would
// appear to work.
func hydrateEmailProviderCredentials(incoming *domain.EmailProvider, stored *domain.EmailProvider) {
	if incoming == nil || stored == nil {
		return
	}
	fill := func(target *string, value string) {
		if *target == "" {
			*target = value
		}
	}

	if incoming.SES != nil && stored.SES != nil {
		fill(&incoming.SES.SecretKey, stored.SES.SecretKey)
	}
	if incoming.SMTP != nil && stored.SMTP != nil {
		fill(&incoming.SMTP.Password, stored.SMTP.Password)
		fill(&incoming.SMTP.Username, stored.SMTP.Username)
		fill(&incoming.SMTP.OAuth2ClientSecret, stored.SMTP.OAuth2ClientSecret)
		fill(&incoming.SMTP.OAuth2RefreshToken, stored.SMTP.OAuth2RefreshToken)
	}
	if incoming.SparkPost != nil && stored.SparkPost != nil {
		fill(&incoming.SparkPost.APIKey, stored.SparkPost.APIKey)
	}
	if incoming.Postmark != nil && stored.Postmark != nil {
		fill(&incoming.Postmark.ServerToken, stored.Postmark.ServerToken)
	}
	if incoming.Mailgun != nil && stored.Mailgun != nil {
		fill(&incoming.Mailgun.APIKey, stored.Mailgun.APIKey)
	}
	if incoming.Mailjet != nil && stored.Mailjet != nil {
		fill(&incoming.Mailjet.APIKey, stored.Mailjet.APIKey)
		fill(&incoming.Mailjet.SecretKey, stored.Mailjet.SecretKey)
	}
	if incoming.SendGrid != nil && stored.SendGrid != nil {
		fill(&incoming.SendGrid.APIKey, stored.SendGrid.APIKey)
	}
}

// preserveEmailProviderSecrets keeps a stored credential when the caller's payload
// carries none for that provider.
//
// Workspaces do not serve decrypted credentials (domain.Workspace.Redact), so a
// client cannot echo an integration's password back on save. Without this, the
// wholesale `updatedIntegration.EmailProvider = req.Provider` below would wipe the
// stored credential on any edit that does not mention it — changing a sender name
// would silently break sending.
//
// Only a provider block the caller actually sent is touched, so switching kinds
// cannot drag the previous provider's secret across. A caller that supplies either
// a new plaintext value or its own ciphertext is rotating deliberately and wins.
//
// The Supabase and LLM branches of UpdateIntegration do the same.
func preserveEmailProviderSecrets(updated *domain.Integration, existing *domain.Integration) {
	if updated == nil || existing == nil {
		return
	}
	keep := func(newPlain, newCipher *string, oldCipher string) {
		if *newPlain == "" && *newCipher == "" {
			*newCipher = oldCipher
		}
	}

	u, e := &updated.EmailProvider, &existing.EmailProvider

	if u.SES != nil && e.SES != nil {
		keep(&u.SES.SecretKey, &u.SES.EncryptedSecretKey, e.SES.EncryptedSecretKey)
	}
	if u.SMTP != nil && e.SMTP != nil {
		keep(&u.SMTP.Password, &u.SMTP.EncryptedPassword, e.SMTP.EncryptedPassword)
		keep(&u.SMTP.OAuth2ClientSecret, &u.SMTP.EncryptedOAuth2ClientSecret, e.SMTP.EncryptedOAuth2ClientSecret)
		keep(&u.SMTP.OAuth2RefreshToken, &u.SMTP.EncryptedOAuth2RefreshToken, e.SMTP.EncryptedOAuth2RefreshToken)
	}
	if u.SparkPost != nil && e.SparkPost != nil {
		keep(&u.SparkPost.APIKey, &u.SparkPost.EncryptedAPIKey, e.SparkPost.EncryptedAPIKey)
	}
	if u.Postmark != nil && e.Postmark != nil {
		keep(&u.Postmark.ServerToken, &u.Postmark.EncryptedServerToken, e.Postmark.EncryptedServerToken)
	}
	if u.Mailgun != nil && e.Mailgun != nil {
		keep(&u.Mailgun.APIKey, &u.Mailgun.EncryptedAPIKey, e.Mailgun.EncryptedAPIKey)
	}
	if u.Mailjet != nil && e.Mailjet != nil {
		keep(&u.Mailjet.APIKey, &u.Mailjet.EncryptedAPIKey, e.Mailjet.EncryptedAPIKey)
		keep(&u.Mailjet.SecretKey, &u.Mailjet.EncryptedSecretKey, e.Mailjet.EncryptedSecretKey)
	}
	if u.SendGrid != nil && e.SendGrid != nil {
		keep(&u.SendGrid.APIKey, &u.SendGrid.EncryptedAPIKey, e.SendGrid.EncryptedAPIKey)
	}
}

func preserveDerivedSESFields(updated *domain.Integration, existing *domain.Integration) {
	if existing == nil || existing.EmailProvider.SES == nil {
		return
	}
	if updated.EmailProvider.SES == nil {
		return
	}

	updated.EmailProvider.SES.ManagedConfigurationSet = existing.EmailProvider.SES.ManagedConfigurationSet
	updated.EmailProvider.SES.ManagedTenantName = existing.EmailProvider.SES.ManagedTenantName

	// InboundTopicARN predates this change and had the same exposure.
	if updated.EmailProvider.SES.InboundTopicARN == "" {
		updated.EmailProvider.SES.InboundTopicARN = existing.EmailProvider.SES.InboundTopicARN
	}
}

func (s *WorkspaceService) UpdateIntegration(ctx context.Context, req domain.UpdateIntegrationRequest) error {
	// Authenticate user and verify they are an owner of the workspace
	// (platform admins are synthesized as owners).
	ctx, user, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
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
		if !req.ProviderSpecified() {
			// The same preserve-on-absence the three branches below get for free from
			// their pointers. Neither helper below can stand in for it: both key off the
			// incoming provider's own blocks, and a body that sent no provider has none.
			// The wholesale assignment therefore used to store an empty provider, which
			// validates clean — an empty Kind reads as "not configured" — and takes the
			// senders, the rate limit and the encrypted credential with it.
			updatedIntegration.EmailProvider = existingIntegration.EmailProvider
			break
		}
		updatedIntegration.EmailProvider = req.Provider
		preserveEmailProviderSecrets(&updatedIntegration, existingIntegration)
		// Derived state belongs to the server, not the client. Without this, any caller whose
		// payload omits these fields — which is every API client that isn't our console —
		// silently wipes them: the SES tenant and configuration set stop being sent, and
		// stop-on-reply breaks because the inbound topic ARN is gone.
		preserveDerivedSESFields(&updatedIntegration, existingIntegration)
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
	case domain.IntegrationTypeLLM:
		// Preserve existing encrypted API key if new key is not provided
		if req.LLMProvider != nil {
			updatedIntegration.LLMProvider = req.LLMProvider

			// Preserve Anthropic encrypted API key if not provided in update
			if req.LLMProvider.Anthropic != nil &&
				req.LLMProvider.Anthropic.APIKey == "" &&
				req.LLMProvider.Anthropic.EncryptedAPIKey == "" &&
				existingIntegration.LLMProvider != nil &&
				existingIntegration.LLMProvider.Anthropic != nil {
				updatedIntegration.LLMProvider.Anthropic.EncryptedAPIKey =
					existingIntegration.LLMProvider.Anthropic.EncryptedAPIKey
			}

			// Preserve OpenAI encrypted API key if not provided in update.
			// NOTE: only the secret (API key) is preserved here. Non-secret fields like
			// model/base_url/reasoning_effort are taken verbatim from req (the whole
			// OpenAI struct above is overwritten), so the frontend must always resend
			// them or they reset to their zero value. Any future SECRET field needs its
			// own preserve-on-blank branch like this one.
			if req.LLMProvider.OpenAI != nil &&
				req.LLMProvider.OpenAI.APIKey == "" &&
				req.LLMProvider.OpenAI.EncryptedAPIKey == "" &&
				existingIntegration.LLMProvider != nil &&
				existingIntegration.LLMProvider.OpenAI != nil {
				updatedIntegration.LLMProvider.OpenAI.EncryptedAPIKey =
					existingIntegration.LLMProvider.OpenAI.EncryptedAPIKey
			}

			// Preserve Gemini encrypted API key if not provided in update
			if req.LLMProvider.Gemini != nil &&
				req.LLMProvider.Gemini.APIKey == "" &&
				req.LLMProvider.Gemini.EncryptedAPIKey == "" &&
				existingIntegration.LLMProvider != nil &&
				existingIntegration.LLMProvider.Gemini != nil {
				updatedIntegration.LLMProvider.Gemini.EncryptedAPIKey =
					existingIntegration.LLMProvider.Gemini.EncryptedAPIKey
			}
		} else {
			// If no settings provided, preserve existing
			updatedIntegration.LLMProvider = existingIntegration.LLMProvider
		}
	case domain.IntegrationTypeFirecrawl:
		// Preserve existing encrypted API key if new key is not provided
		if req.FirecrawlSettings != nil {
			updatedIntegration.FirecrawlSettings = req.FirecrawlSettings

			// Preserve encrypted API key if not provided in update
			if req.FirecrawlSettings.APIKey == "" &&
				req.FirecrawlSettings.EncryptedAPIKey == "" &&
				existingIntegration.FirecrawlSettings != nil {
				updatedIntegration.FirecrawlSettings.EncryptedAPIKey =
					existingIntegration.FirecrawlSettings.EncryptedAPIKey
			}
		} else {
			// If no settings provided, preserve existing
			updatedIntegration.FirecrawlSettings = existingIntegration.FirecrawlSettings
		}
	case domain.IntegrationTypeZapier:
		// Server-owned, the same category as preserveDerivedSESFields above: the address was
		// minted by ConnectZapier and UpdateIntegrationRequest has no field that could carry
		// it, so the stored value is the only source there is. Without this line every rename
		// would leave the record settings-less and Validate would reject it.
		//
		// A rename therefore changes the label alone: the address is immutable, so a card
		// renamed to "Marketing" may keep zapier-support-3f9a1c02@host, which is why the
		// console shows both.
		updatedIntegration.ZapierSettings = existingIntegration.ZapierSettings
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
	// (platform admins are synthesized as owners).
	ctx, user, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
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
		// Tear down the provider-side resources this integration owns (except SMTP, which has
		// none). This is deliberately unconditional: it used to run only when webhooks were
		// registered, but SES resources now outlive webhook registration on purpose — an
		// integration whose webhooks were unregistered still owns a configuration set and,
		// when isolation is enabled, a tenant that AWS bills monthly. Gating on registration
		// would strand both with no way to remove them.
		if s.webhookRegService != nil && integration.EmailProvider.Kind != domain.EmailProviderKindSMTP {
			s.logger.WithField("workspace_id", workspaceID).
				WithField("integration_id", integrationID).
				Info("Removing provider resources for integration that is being deleted")

			if err := s.webhookRegService.DeleteIntegrationResources(ctx, workspaceID, integrationID); err != nil {
				s.logger.WithField("workspace_id", workspaceID).
					WithField("integration_id", integrationID).
					WithField("error", err.Error()).
					Warn("Failed to remove provider resources during integration deletion, continuing with deletion anyway")
			}
		}

	case domain.IntegrationTypeSupabase:
		// Delete all templates and transactional notifications associated with this integration
		err := s.deleteSupabaseIntegrationResources(ctx, workspaceID, integrationID)
		if err != nil {
			s.logger.WithField("workspace_id", workspaceID).
				WithField("integration_id", integrationID).
				WithField("error", err.Error()).
				Warn("Failed to delete Supabase integration resources, continuing with deletion anyway")
		}

	case domain.IntegrationTypeZapier:
		// Revoke the key this card minted, on the same principle as the two cases above:
		// deleting a connector tears down what it owns. Leaving the key alive would strand a
		// live credential whose only remaining trace anywhere is a row on Settings → Team.
		// DeleteWorkspace funnels every integration through here, so deleting a workspace
		// revokes its Zapier keys too.
		//
		// Unlike its neighbours this one aborts the deletion rather than warning past it. The
		// confirmation the user answered says the key is revoked; removing the card anyway
		// would report a revocation that did not happen and leave nothing to retry from.
		if integration.ZapierSettings != nil {
			if err := s.revokeZapierAPIKey(ctx, workspaceID, integration.ZapierSettings.APIKeyEmail); err != nil {
				s.logger.WithField("workspace_id", workspaceID).
					WithField("integration_id", integrationID).
					WithField("error", err.Error()).
					Error("Failed to revoke the Zapier API key, leaving the integration in place")
				return err
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
