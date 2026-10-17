package service

import (
	"context"
	"errors"
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
	repo               domain.ListRepository
	workspaceRepo      domain.WorkspaceRepository
	contactListRepo    domain.ContactListRepository
	contactRepo        domain.ContactRepository
	messageHistoryRepo domain.MessageHistoryRepository
	authService        domain.AuthService
	emailService       domain.EmailServiceInterface
	logger             logger.Logger
	apiEndpoint        string
	blogCache          cache.Cache
}

func NewListService(repo domain.ListRepository, workspaceRepo domain.WorkspaceRepository, contactListRepo domain.ContactListRepository, contactRepo domain.ContactRepository, messageHistoryRepo domain.MessageHistoryRepository, authService domain.AuthService, emailService domain.EmailServiceInterface, logger logger.Logger, apiEndpoint string, blogCache cache.Cache) *ListService {
	return &ListService{
		repo:               repo,
		workspaceRepo:      workspaceRepo,
		contactListRepo:    contactListRepo,
		contactRepo:        contactRepo,
		messageHistoryRepo: messageHistoryRepo,
		authService:        authService,
		emailService:       emailService,
		logger:             logger,
		apiEndpoint:        apiEndpoint,
		blogCache:          blogCache,
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

// SubscribeToLists satisfies domain.ListService for the callers that only need to
// know whether the subscription succeeded — the notification center, workspace
// onboarding and the demo seeder all discard the memberships.
func (s *ListService) SubscribeToLists(ctx context.Context, payload *domain.SubscribeToListsRequest, hasBearerToken bool) error {
	_, err := s.SubscribeToListsWithResults(ctx, payload, hasBearerToken)
	return err
}

// membershipResult copies a membership row for the subscribe response. A copy
// because the rows reported come from two places — the struct just written and the
// row read before it — and neither should be aliased into a slice that outlives the
// loop iteration. list_name is filled from the list being processed: contact_lists
// has no name column, so a stored row read by (email, list_id) carries none.
func membershipResult(contactList *domain.ContactList, listName string) *domain.ContactList {
	result := *contactList
	result.ListName = listName
	return &result
}

// SubscribeToListsWithResults subscribes a contact to lists and reports the
// membership each request left behind.
//
// The resulting status is the part a caller cannot predict: a list in double
// opt-in lands an unauthenticated subscriber on 'pending', a contact who had
// unsubscribed is forced back through double opt-in whatever the list's setting,
// and a bounced or complained address is refused outright, both even once the
// contact was removed from the list. A caller told only
// that the request succeeded cannot tell any of those apart from a contact who is
// now reachable.
//
// Every requested list gets exactly one entry, including the paths that write
// nothing — being already active, or bounced, is an outcome the caller has to be
// able to read.
//
// request can come from 3 different sources:
// 1. API
// 2. Frontend (authenticated with email and email_hmac)
// 3. Frontend (unauthenticated with email)
func (s *ListService) SubscribeToListsWithResults(ctx context.Context, payload *domain.SubscribeToListsRequest, hasBearerToken bool) ([]*domain.ContactList, error) {
	var err error

	// fail silently if the email is disposable
	if disposable_emails.IsDisposableEmail(payload.Contact.Email) {
		// Silently dropped, so there is no membership to report — an empty slice
		// rather than a nil one, so the response shape does not change with it.
		return []*domain.ContactList{}, nil
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, payload.WorkspaceID)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to get workspace: %v", err))
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	isAuthenticated := false

	var userWorkspace *domain.UserWorkspace

	if hasBearerToken {
		ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspace.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to authenticate user: %w", err)
		}

		// Check permission for writing lists
		if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeWrite) {
			return nil, domain.NewPermissionError(
				domain.PermissionResourceLists,
				domain.PermissionTypeWrite,
				"Insufficient permissions: write access to lists required",
			)
		}

		isAuthenticated = true
	} else if payload.Contact.EmailHMAC != "" {

		secretKey := workspace.Settings.SecretKey
		if !domain.VerifyEmailHMAC(payload.Contact.Email, payload.Contact.EmailHMAC, secretKey) {
			return nil, fmt.Errorf("invalid email verification")
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
			return nil, fmt.Errorf("failed to upsert contact: %w", err)
		}
	}

	// get the lists
	lists, err := s.repo.GetLists(ctx, workspace.ID)
	if err != nil {
		s.logger.WithField("list_ids", payload.ListIDs).Error(fmt.Sprintf("Failed to get lists: %v", err))
		return nil, fmt.Errorf("failed to get lists: %w", err)
	}

	// One entry per requested list, in the order they were asked for.
	results := make([]*domain.ContactList, 0, len(payload.ListIDs))

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
			// Typed, so the handler can answer 404 and name the list. A Zap or a
			// script outliving the list it points at is the ordinary way here, and
			// an untyped error made that an unexplained 500.
			return nil, &domain.ErrListNotFound{Message: fmt.Sprintf("list %s not found", listID)}
		}

		// reject if the list is not public and the request is not authenticated
		// (authenticated = bearer token OR valid HMAC from notification center)
		if !list.IsPublic && !isAuthenticated {
			return nil, fmt.Errorf("list is not public")
		}

		// Check existing subscription status to prevent overwriting. A contact removed from
		// the list is read too: the removal ends their membership, not the opt-out, bounce or
		// complaint on their row, which the guards below must still see.
		existingCL, err := s.contactListRepo.GetContactListIncludingRemoved(ctx, workspace.ID, payload.Contact.Email, listID)
		if err != nil {
			if _, ok := err.(*domain.ErrContactListNotFound); !ok {
				return nil, fmt.Errorf("failed to check existing subscription: %w", err)
			}
			// Not found — proceed as new subscription
		}
		// Removed while active or pending: not on the list, and nothing on the row to keep,
		// so this is a new subscription
		if existingCL != nil && existingCL.DeletedAt != nil &&
			(existingCL.Status == domain.ContactListStatusActive || existingCL.Status == domain.ContactListStatusPending) {
			existingCL = nil
		}

		// The workspace decides who is on a private list. A link's signature lets its contact
		// confirm a double opt-in the workspace started, but never join a private list,
		// nor rejoin one they left.
		if !list.IsPublic && !hasBearerToken &&
			(existingCL == nil || existingCL.Status == domain.ContactListStatusUnsubscribed) {
			return nil, fmt.Errorf("list is not public")
		}

		skipDBWrite := false
		forceDoubleOptin := false

		if existingCL != nil {
			switch existingCL.Status {
			case domain.ContactListStatusActive:
				// Idempotent no-op — already subscribed. Still reported: "already
				// active" is the outcome of this subscribe, and a caller that only
				// saw success would have no way to tell it apart from a new one.
				results = append(results, membershipResult(existingCL, list.Name))
				continue
			case domain.ContactListStatusPending:
				if !isAuthenticated {
					// Anonymous re-submit of the subscribe form:
					// skip DB write and resend DOI email below.
					skipDBWrite = true
				}
				// Authenticated request (valid email_hmac) is the confirmation-link click:
				// fall through so the upsert below transitions Pending → Active and the
				// DOI email logic is NOT re-triggered. (Issue #313)
			case domain.ContactListStatusBounced, domain.ContactListStatusComplained:
				// Hard block — do not send email to bounced/complained addresses.
				// Reported too, so the refusal is visible rather than silent: this
				// is the one path where a "successful" subscribe leaves the contact
				// permanently unmailable on that list.
				results = append(results, membershipResult(existingCL, list.Name))
				continue
			case domain.ContactListStatusUnsubscribed:
				// Allow re-subscribe — force double opt-in for compliance
				forceDoubleOptin = true
			}
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
		// also force double optin for re-subscribing contacts (compliance requirement)
		if (list.IsDoubleOptin && !isAuthenticated) || forceDoubleOptin {
			contactList.Status = domain.ContactListStatusPending
		}

		if !skipDBWrite {
			// Subscribe to the list
			err = s.contactListRepo.AddContactToList(ctx, workspace.ID, contactList)
			if err != nil {
				// codecov:ignore:start
				s.logger.WithField("email", contactList.Email).
					WithField("list_id", contactList.ListID).
					Error(fmt.Sprintf("Failed to subscribe to list: %v", err))
				// codecov:ignore:end
				return nil, fmt.Errorf("failed to subscribe to list: %w", err)
			}
		}

		// For pending resend: ensure status is pending so DOI email logic fires
		if skipDBWrite {
			contactList.Status = domain.ContactListStatusPending
		}

		// A pending resend wrote nothing, so the stored row — not the struct
		// assembled above, whose timestamps are this instant — is what the caller
		// should be told about.
		stored := contactList
		if skipDBWrite {
			stored = existingCL
		}
		results = append(results, membershipResult(stored, list.Name))

		marketingEmailProvider, integrationID, err := workspace.GetEmailProviderWithIntegrationID(true)
		if err != nil {
			s.logger.WithField("workspace_id", workspace.ID).Error(fmt.Sprintf("Failed to get marketing email provider: %v", err))
			return nil, fmt.Errorf("failed to get marketing email provider: %w", err)
		}

		// if the marketing email provider is not set, we don't need to send the welcome email
		if marketingEmailProvider == nil {
			continue
		}

		// get contact
		contact, err := s.contactRepo.GetContactByEmail(ctx, workspace.ID, contactList.Email)
		if err != nil {
			s.logger.WithField("email", contactList.Email).Error(fmt.Sprintf("Failed to get contact: %v", err))
			return nil, fmt.Errorf("failed to get contact: %w", err)
		}

		messageID := uuid.New().String()

		// Resolve the tracking/base endpoint: custom endpoint if set, else the API endpoint.
		endpoint := workspace.Settings.ResolveEndpoint(s.apiEndpoint)

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
			WorkspaceID:         workspace.ID,
			WorkspaceSecretKey:  workspace.Settings.SecretKey,
			WorkspaceWebsiteURL: workspace.Settings.WebsiteURL,
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
			return nil, fmt.Errorf("failed to build template data: %w", err)
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
				return nil, fmt.Errorf("failed to send double optin email: %w", err)
			}
		}
	}

	return results, nil
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

	// get the lists
	lists, err := s.repo.GetLists(ctx, workspace.ID)
	if err != nil {
		s.logger.WithField("list_ids", payload.ListIDs).Error(fmt.Sprintf("Failed to get lists: %v", err))
		return fmt.Errorf("failed to get lists: %w", err)
	}

	// Process each list for unsubscription
	recorded := false
	for _, listID := range payload.ListIDs {
		var list *domain.List
		for _, l := range lists {
			if l.ID == listID {
				list = l
				break
			}
		}

		// A list that doesn't exist, such as one deleted after its email went out, has no
		// subscription left to end. Answering with an error would only make the recipient's
		// mail provider retry.
		if list == nil {
			s.logger.WithField("list_id", listID).Warn("Unsubscribe skipped: list not found or deleted")
			continue
		}

		var listRecorded bool
		listRecorded, err = s.unsubscribeFromList(ctx, workspace.ID, payload.Email, listID, payload.MessageID)
		if err != nil {
			s.logger.WithField("email", payload.Email).
				WithField("list_id", listID).
				Error(fmt.Sprintf("Failed to unsubscribe from list: %v", err))
			return fmt.Errorf("failed to unsubscribe from list: %w", err)
		}
		recorded = recorded || listRecorded
	}

	// Update message history with unsubscribe event if MessageID is provided
	// This enables broadcast statistics to track unsubscribes via notification center.
	// Only once an opt-out was recorded: an erased contact's messages are kept with the
	// address redacted, and marking one would still fire its email.unsubscribed webhook.
	if payload.MessageID != "" && recorded {
		now := time.Now()
		updates := []domain.MessageEventUpdate{
			{
				ID:        payload.MessageID,
				Event:     domain.MessageEventUnsubscribed,
				Timestamp: now,
			},
		}
		if err := s.messageHistoryRepo.SetStatusesIfNotSet(ctx, workspace.ID, updates); err != nil {
			// Log but don't fail - the contact is already unsubscribed
			// Message history update is for analytics only
			s.logger.WithField("message_id", payload.MessageID).
				Warn(fmt.Sprintf("Failed to update message history for unsubscribe: %v", err))
		}
	}

	return nil
}

// unsubscribeFromList records a contact's opt-out from one list and reports whether it
// recorded anything. messageID is the email the request came from.
func (s *ListService) unsubscribeFromList(ctx context.Context, workspaceID, email, listID, messageID string) (bool, error) {
	// A bounced or complained address is never mailed, and its status says more than an
	// opt-out, so it is kept. People who report spam often unsubscribe too: overwriting
	// would turn the complaint into a plain "unsubscribed", which a broadcast set to
	// include unsubscribed contacts mails again. The update keeps it in its own WHERE, so
	// a complaint saved while this request runs is kept too. A contact removed from the
	// list stays removed: an unsubscribe link from an email sent before the removal must
	// not put them back on the list.
	updated, err := s.contactListRepo.MarkContactListUnsubscribed(ctx, workspaceID, email, listID)
	if err != nil || updated {
		return updated, err
	}

	// Nothing updated: either the row is bounced or complained and was kept, or the
	// contact has no row on the list. A row in any other status was committed by a
	// subscribe after the update ran, and takes the opt-out.
	row, err := s.contactListRepo.GetContactListIncludingRemoved(ctx, workspaceID, email, listID)
	if err == nil {
		if row.Status == domain.ContactListStatusBounced || row.Status == domain.ContactListStatusComplained {
			return true, nil
		}
		return s.contactListRepo.MarkContactListUnsubscribed(ctx, workspaceID, email, listID)
	}
	var notOnList *domain.ErrContactListNotFound
	if !errors.As(err, &notOnList) {
		return false, err
	}

	// No row on the list: automations email contacts who aren't on their list, and those
	// emails carry the list's unsubscribe links. Nothing is written for a contact that no
	// longer exists, so a link from before an erasure can't store the address again.
	if _, err := s.contactRepo.GetContactByEmail(ctx, workspaceID, email); err != nil {
		if errors.Is(err, domain.ErrContactNotFound) {
			s.logger.WithField("list_id", listID).Info("Unsubscribe skipped: contact no longer exists")
			return false, nil
		}
		return false, fmt.Errorf("failed to get contact: %w", err)
	}

	// The link's signature covers the address, not the list, so the list must be the one
	// the email was sent for. Otherwise any link could put its contact on any list,
	// private ones included, which the preferences page would then show by name.
	sent := false
	if messageID != "" {
		sent, err = s.messageHistoryRepo.WasSentForList(ctx, workspaceID, messageID, email, listID)
		if err != nil {
			return false, err
		}
	}
	if !sent {
		s.logger.WithField("list_id", listID).WithField("message_id", messageID).
			Warn("Unsubscribe skipped: the email was not sent for this list")
		return false, nil
	}

	// AddContactToList never overwrites a bounced or complained row
	err = s.contactListRepo.AddContactToList(ctx, workspaceID, &domain.ContactList{
		Email:  email,
		ListID: listID,
		Status: domain.ContactListStatusUnsubscribed,
	})
	return err == nil, err
}
