package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type ContactService struct {
	repo                    domain.ContactRepository
	workspaceRepo           domain.WorkspaceRepository
	authService             domain.AuthService
	messageHistoryRepo      domain.MessageHistoryRepository
	inboundWebhookEventRepo domain.InboundWebhookEventRepository
	contactListRepo         domain.ContactListRepository
	contactTimelineRepo     domain.ContactTimelineRepository
	// webAnalyticsRepo is optional: installs without the feature wired still
	// delete contacts normally.
	webAnalyticsRepo domain.WebAnalyticsRepository
	// emailQueueRepo is nil only in test harnesses that do not exercise deletion;
	// app wiring always supplies it.
	emailQueueRepo          domain.EmailQueueRepository
	customEventRepo         domain.CustomEventRepository
	segmentRepo             domain.SegmentRepository
	contactSegmentQueueRepo domain.ContactSegmentQueueRepository
	logger                  logger.Logger
}

// NewContactService takes its dependencies positionally, which is defensible at
// this arity only because the repository types are distinct interfaces — swapping
// two of them does not compile.
//
// That stops being true if a fifth repository arrives, or if two of them ever
// share a method set. At that point move to an options struct: the failure mode
// otherwise is a purge that silently targets the wrong table, which no test would
// catch because every mock would still be satisfied.
func NewContactService(
	repo domain.ContactRepository,
	workspaceRepo domain.WorkspaceRepository,
	authService domain.AuthService,
	messageHistoryRepo domain.MessageHistoryRepository,
	inboundWebhookEventRepo domain.InboundWebhookEventRepository,
	contactListRepo domain.ContactListRepository,
	contactTimelineRepo domain.ContactTimelineRepository,
	webAnalyticsRepo domain.WebAnalyticsRepository,
	emailQueueRepo domain.EmailQueueRepository,
	customEventRepo domain.CustomEventRepository,
	segmentRepo domain.SegmentRepository,
	contactSegmentQueueRepo domain.ContactSegmentQueueRepository,
	logger logger.Logger,
) *ContactService {
	return &ContactService{
		repo:                    repo,
		workspaceRepo:           workspaceRepo,
		authService:             authService,
		messageHistoryRepo:      messageHistoryRepo,
		inboundWebhookEventRepo: inboundWebhookEventRepo,
		contactListRepo:         contactListRepo,
		contactTimelineRepo:     contactTimelineRepo,
		webAnalyticsRepo:        webAnalyticsRepo,
		emailQueueRepo:          emailQueueRepo,
		customEventRepo:         customEventRepo,
		segmentRepo:             segmentRepo,
		contactSegmentQueueRepo: contactSegmentQueueRepo,
		logger:                  logger,
	}
}

func (s *ContactService) GetContactByEmail(ctx context.Context, workspaceID string, email string) (*domain.Contact, error) {
	// Normalize email for consistent lookups
	email = domain.NormalizeEmail(email)

	// Check if this is a system call (e.g., from Supabase webhook)
	isSystemCall := ctx.Value(domain.SystemCallKey) != nil

	// Only authenticate and check permissions for non-system calls
	if !isSystemCall {
		var err error
		var userWorkspace *domain.UserWorkspace
		ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to authenticate user: %w", err)
		}

		// Check permission for reading contacts
		if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
			return nil, domain.NewPermissionError(
				domain.PermissionResourceContacts,
				domain.PermissionTypeRead,
				"Insufficient permissions: read access to contacts required",
			)
		}
	}

	contact, err := s.repo.GetContactByEmail(ctx, workspaceID, email)
	if err != nil {
		if strings.Contains(err.Error(), "contact not found") {
			return nil, err
		}
		s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to get contact by email: %v", err))
		return nil, fmt.Errorf("failed to get contact by email: %w", err)
	}

	return contact, nil
}

func (s *ContactService) GetContactByExternalID(ctx context.Context, workspaceID string, externalID string) (*domain.Contact, error) {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading contacts
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to contacts required",
		)
	}

	contact, err := s.repo.GetContactByExternalID(ctx, workspaceID, externalID)
	if err != nil {
		if strings.Contains(err.Error(), "contact not found") {
			return nil, err
		}
		s.logger.WithField("external_id", externalID).Error(fmt.Sprintf("Failed to get contact by external ID: %v", err))
		return nil, fmt.Errorf("failed to get contact by external ID: %w", err)
	}

	return contact, nil
}

func (s *ContactService) GetContacts(ctx context.Context, req *domain.GetContactsRequest) (*domain.GetContactsResponse, error) {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading contacts
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to contacts required",
		)
	}

	response, err := s.repo.GetContacts(ctx, req)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to get contacts: %v", err))
		return nil, fmt.Errorf("failed to get contacts: %w", err)
	}

	return response, nil
}

func (s *ContactService) DeleteContact(ctx context.Context, workspaceID string, email string) error {
	// Normalize email for consistent lookups
	email = domain.NormalizeEmail(email)

	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing contacts
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to contacts required",
		)
	}

	// Queued mail goes first. Every other step below cleans up a row that already
	// exists; this is the only one that stops something from still happening, and
	// every moment it waits is a moment a worker can claim a row and send to the
	// address we were asked to erase.
	//
	// Fatal on error: reporting a contact as deleted while their mail is still on
	// its way to them is precisely the outcome this prevents.
	//
	// It does not close the window entirely. An entry already claimed by a worker
	// is held in memory and will still send — stopping that needs a contact check
	// in the send path itself, which costs a query per email.
	if s.emailQueueRepo != nil {
		if _, err := s.emailQueueRepo.DeleteForEmail(ctx, workspaceID, email); err != nil {
			s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to delete queued emails: %v", err))
			return fmt.Errorf("failed to delete queued emails: %w", err)
		}
	}

	// Delete related data first
	if err := s.messageHistoryRepo.DeleteForEmail(ctx, workspaceID, email); err != nil {
		s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to delete message history: %v", err))
		return fmt.Errorf("failed to delete message history: %w", err)
	}

	if err := s.inboundWebhookEventRepo.DeleteForEmail(ctx, workspaceID, email); err != nil {
		s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to delete webhook events: %v", err))
		return fmt.Errorf("failed to delete webhook events: %w", err)
	}

	if err := s.contactListRepo.DeleteForEmail(ctx, workspaceID, email); err != nil {
		s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to delete contact list relationships: %v", err))
		return fmt.Errorf("failed to delete contact list relationships: %w", err)
	}

	// The contact row goes first, and the order is load-bearing rather than
	// stylistic. Wrapping this in a transaction has been proposed and is WRONG:
	// inside one, the contact-row DELETE stays invisible to a concurrent buffered
	// beat until commit, so the projection's EXISTS guard would still pass and
	// re-insert timeline rows after the in-transaction purge had already run —
	// reintroducing precisely the bug described below.
	//
	// These are separate statements, not one transaction, and the web
	// analytics projection guards on EXISTS (SELECT 1 FROM contacts ...) — so
	// while the row survives, a beat buffered before the deletion can still flush
	// and re-insert the timeline rows just purged below, leaving a deleted
	// person's browsing history behind with no contact to reach it from.
	// Removing the row first makes that guard fail closed for anything in flight.
	if err := s.repo.DeleteContact(ctx, workspaceID, email); err != nil {
		// A contact row that is already gone must NOT abort the rest: the
		// dependent rows are deleted after it, so a run that removed the contact
		// and then failed part-way — a lock timeout, a dropped connection, an
		// evicted pod — can only be finished by retrying, and the repository
		// reports a zero-row delete as "contact not found". Short-circuiting here
		// would leave that contact's timeline and their address on the web
		// analytics rows with no supported way to remove either, since nothing
		// cascades from contacts.
		if !strings.Contains(err.Error(), "contact not found") {
			s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to delete contact: %v", err))
			return fmt.Errorf("failed to delete contact: %w", err)
		}
		s.logger.WithField("email", email).
			Info("Contact row already absent; continuing with dependent cleanup")
	}

	// Deleting these does not corrupt revenue reporting, which is worth stating
	// because the web analytics comment below makes the opposite argument for its
	// own table. custom_events is read only per-contact — repository lists, and the
	// EXISTS subquery for segment membership — while workspace revenue dashboards
	// aggregate web_goals, whose goal_value is untouched here. So this destroys
	// only the erased contact's own history, exactly as contact_timeline already does.
	//
	// custom_events carries the address in a NOT NULL column, so it cannot be
	// anonymised in place. Deleted rather than soft-deleted: the table has two
	// AFTER INSERT OR UPDATE triggers, so an UPDATE would re-insert timeline rows
	// for the address whose timeline is about to be purged, and fan the deleted
	// person's event out to webhook subscribers. A plain DELETE fires neither.
	if s.customEventRepo != nil {
		if err := s.customEventRepo.DeleteForEmail(ctx, workspaceID, email); err != nil {
			s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to delete custom events: %v", err))
			return fmt.Errorf("failed to delete custom events: %w", err)
		}
	}

	// ORDERING, and it is a two-hop cascade rather than a preference:
	//
	//   contact_segments DELETE
	//     -> contact_segment_changes_trigger INSERTs a segment.left row into
	//        contact_timeline carrying OLD.email
	//        -> contact_timeline_queue_trigger INSERTs that address into
	//           contact_segment_queue
	//
	// So this runs BEFORE the timeline purge below — after it, the purge would
	// have already run and the address would sit back on a timeline that reads as
	// cleared — and the queue purge runs AFTER, once the rows the cascade creates
	// actually exist.
	if s.segmentRepo != nil {
		if err := s.segmentRepo.DeleteForEmail(ctx, workspaceID, email); err != nil {
			s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to delete contact segments: %v", err))
			return fmt.Errorf("failed to delete contact segments: %w", err)
		}
	}

	if err := s.contactTimelineRepo.DeleteForEmail(ctx, workspaceID, email); err != nil {
		s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to delete contact timeline: %v", err))
		return fmt.Errorf("failed to delete contact timeline: %w", err)
	}

	// Last, for the cascade reason above. Also sweeps anything the queue
	// processor was mid-flight on.
	if s.contactSegmentQueueRepo != nil {
		if err := s.contactSegmentQueueRepo.RemoveFromQueue(ctx, workspaceID, email); err != nil {
			s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to remove contact from the segment queue: %v", err))
			return fmt.Errorf("failed to remove contact from the segment queue: %w", err)
		}
	}

	// Web analytics rows are anonymized rather than deleted: once the address is
	// gone they are ordinary anonymous traffic, and removing them would rewrite
	// historical session and pageview totals. Best-effort — the feature may not
	// be enabled, and a contact deletion must not fail because analytics did.
	if s.webAnalyticsRepo != nil {
		if err := s.webAnalyticsRepo.AnonymizeContact(ctx, workspaceID, email); err != nil {
			s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to anonymize web analytics rows: %v", err))
		}
	}

	return nil
}

func (s *ContactService) BatchImportContacts(ctx context.Context, workspaceID string, contacts []*domain.Contact, listIDs []string) *domain.BatchImportContactsResponse {
	response := &domain.BatchImportContactsResponse{
		Operations: make([]*domain.UpsertContactOperation, 0, len(contacts)),
	}

	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		// Err as well as Error: the handler matches the typed error to pick a status
		// code, and a revoked key, a non-member and an unknown workspace are all
		// indistinguishable once flattened to prose. Setting only the string made
		// every authentication failure answer with the handler's generic fallback
		// status, so an integration whose key was revoked never saw the 401 that
		// tells it to re-authenticate.
		response.Error = fmt.Sprintf("failed to authenticate user: %v", err)
		response.Err = err
		return response
	}

	// Check permission for writing contacts
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeWrite) {
		permErr := domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to contacts required",
		)
		response.Error = permErr.Error()
		response.Err = permErr
		return response
	}

	// If listIDs are provided, also check permission for writing lists
	if len(listIDs) > 0 {
		if !userWorkspace.HasPermission(domain.PermissionResourceLists, domain.PermissionTypeWrite) {
			permErr := domain.NewPermissionError(
				domain.PermissionResourceLists,
				domain.PermissionTypeWrite,
				"Insufficient permissions: write access to lists required",
			)
			response.Error = permErr.Error()
			response.Err = permErr
			return response
		}
	}

	// Pre-validate all contacts and separate valid from invalid
	// This allows us to provide immediate feedback on validation errors
	// while still processing valid contacts in bulk
	validContacts := make([]*domain.Contact, 0, len(contacts))
	validContactIndices := make([]int, 0, len(contacts))

	for i, contact := range contacts {
		// CreatedAt and UpdatedAt are optional - if not provided, DB will use CURRENT_TIMESTAMP
		// If provided, the values will be used (allows historical imports)

		if err := contact.Validate(); err != nil {
			// Record validation error
			operation := &domain.UpsertContactOperation{
				Email:  contact.Email,
				Action: domain.UpsertContactOperationError,
				Error:  fmt.Sprintf("invalid contact at index %d: %v", i, err),
			}
			response.Operations = append(response.Operations, operation)
		} else {
			// Add to valid contacts for bulk processing
			validContacts = append(validContacts, contact)
			validContactIndices = append(validContactIndices, i)
		}
	}

	// Deduplicate contacts by email - keep the last occurrence
	// This prevents PostgreSQL "ON CONFLICT DO UPDATE cannot affect row a second time" error
	// when the same email appears multiple times in a single batch
	if len(validContacts) > 1 {
		lastIndex := make(map[string]int, len(validContacts))
		for i, c := range validContacts {
			lastIndex[c.Email] = i
		}

		if len(lastIndex) < len(validContacts) {
			deduplicatedContacts := make([]*domain.Contact, 0, len(lastIndex))
			deduplicatedIndices := make([]int, 0, len(lastIndex))
			for i, c := range validContacts {
				if lastIndex[c.Email] == i {
					deduplicatedContacts = append(deduplicatedContacts, c)
					deduplicatedIndices = append(deduplicatedIndices, validContactIndices[i])
				}
			}
			validContacts = deduplicatedContacts
			validContactIndices = deduplicatedIndices
		}
	}

	// If there are valid contacts, perform bulk upsert in chunks
	if len(validContacts) > 0 {
		allResults := make([]domain.BulkUpsertResult, 0, len(validContacts))

		for chunkStart := 0; chunkStart < len(validContacts); chunkStart += domain.BulkImportChunkSize {
			chunkEnd := chunkStart + domain.BulkImportChunkSize
			if chunkEnd > len(validContacts) {
				chunkEnd = len(validContacts)
			}
			chunk := validContacts[chunkStart:chunkEnd]

			bulkResults, err := s.repo.BulkUpsertContacts(ctx, workspaceID, chunk)
			if err != nil {
				s.logger.Error(fmt.Sprintf("Bulk upsert failed for chunk %d-%d: %v", chunkStart, chunkEnd-1, err))
				for i := chunkStart; i < chunkEnd; i++ {
					operation := &domain.UpsertContactOperation{
						Email:  validContacts[i].Email,
						Action: domain.UpsertContactOperationError,
						Error:  fmt.Sprintf("failed to upsert contact at index %d: %v", validContactIndices[i], err),
					}
					response.Operations = append(response.Operations, operation)
				}
				continue
			}

			for _, result := range bulkResults {
				action := domain.UpsertContactOperationCreate
				if !result.IsNew {
					action = domain.UpsertContactOperationUpdate
				}
				operation := &domain.UpsertContactOperation{
					Email:  result.Email,
					Action: action,
				}
				response.Operations = append(response.Operations, operation)
			}
			allResults = append(allResults, bulkResults...)
		}

		// If listIDs were provided, bulk subscribe successfully upserted contacts to lists
		if len(listIDs) > 0 && len(allResults) > 0 {
			emails := make([]string, len(allResults))
			for i, result := range allResults {
				emails[i] = result.Email
			}

			err := s.contactListRepo.BulkAddContactsToLists(ctx, workspaceID, emails, listIDs, domain.ContactListStatusActive)
			if err != nil {
				s.logger.Error(fmt.Sprintf("Failed to bulk add contacts to lists: %v", err))
			}
		}
	}

	return response
}

func (s *ContactService) UpsertContact(ctx context.Context, workspaceID string, contact *domain.Contact) domain.UpsertContactOperation {
	operation := domain.UpsertContactOperation{
		Email:  contact.Email,
		Action: domain.UpsertContactOperationCreate,
	}

	// Check if this is a system call (e.g., from Supabase webhook)
	isSystemCall := ctx.Value(domain.SystemCallKey) != nil

	// Only authenticate and check permissions for non-system calls
	if !isSystemCall {
		var err error
		var userWorkspace *domain.UserWorkspace
		ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
		if err != nil {
			operation.Action = domain.UpsertContactOperationError
			operation.Error = err.Error()
			// See BatchImportContacts: the string alone cannot be matched by
			// errors.Is/As, so the handler fell through to its catch-all status and
			// reported a revoked key as a bad request.
			operation.Err = err
			s.logger.WithField("email", contact.Email).Error(fmt.Sprintf("Failed to authenticate user: %v", err))
			return operation
		}

		// Check permission for writing contacts
		if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeWrite) {
			permErr := domain.NewPermissionError(
				domain.PermissionResourceContacts,
				domain.PermissionTypeWrite,
				"Insufficient permissions: write access to contacts required",
			)
			operation.Action = domain.UpsertContactOperationError
			operation.Error = permErr.Error()
			operation.Err = permErr
			s.logger.WithField("email", contact.Email).Error(permErr.Error())
			return operation
		}
	}

	if err := contact.Validate(); err != nil {
		operation.Action = domain.UpsertContactOperationError
		operation.Error = err.Error()
		s.logger.WithField("email", contact.Email).Error(fmt.Sprintf("Invalid contact: %v", err))
		return operation
	}

	// CreatedAt and UpdatedAt are optional - if not provided, DB will use CURRENT_TIMESTAMP
	// If provided, the values will be used (allows historical imports)

	isNew, err := s.repo.UpsertContact(ctx, workspaceID, contact)
	if err != nil {
		operation.Action = domain.UpsertContactOperationError
		operation.Error = err.Error()
		s.logger.WithField("email", contact.Email).Error(fmt.Sprintf("Failed to upsert contact: %v", err))
		return operation
	}

	if !isNew {
		operation.Action = domain.UpsertContactOperationUpdate
	}

	// Read the row back so the caller learns what was actually stored. The
	// repository merges an update field by field and the database fills in the
	// timestamps, so the struct passed in describes the request, not the result.
	// Best effort on purpose: the write is committed by now, and turning a failed
	// read into an error would report a successful upsert as a failure and invite
	// the caller to retry it.
	stored, err := s.repo.GetContactByEmail(ctx, workspaceID, contact.Email)
	if err != nil {
		s.logger.WithField("email", contact.Email).Error(fmt.Sprintf("Failed to read back upserted contact: %v", err))
	} else {
		operation.Contact = stored
	}

	return operation
}

func (s *ContactService) CountContacts(ctx context.Context, workspaceID string) (int, error) {
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading contacts
	if !userWorkspace.HasPermission(domain.PermissionResourceContacts, domain.PermissionTypeRead) {
		return 0, domain.NewPermissionError(
			domain.PermissionResourceContacts,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to contacts required",
		)
	}

	count, err := s.repo.Count(ctx, workspaceID)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to count contacts: %v", err))
		return 0, fmt.Errorf("failed to count contacts: %w", err)
	}

	return count, nil
}
