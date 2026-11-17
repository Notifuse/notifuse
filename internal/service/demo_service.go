package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/google/uuid"
)

// DemoService handles demo workspace operations
type DemoService struct {
	logger                           logger.Logger
	config                           *config.Config
	workspaceService                 *WorkspaceService
	userService                      *UserService
	contactService                   *ContactService
	listService                      *ListService
	contactListService               *ContactListService
	templateService                  *TemplateService
	emailService                     *EmailService
	broadcastService                 *BroadcastService
	taskService                      *TaskService
	transactionalNotificationService *TransactionalNotificationService
	inboundWebhookEventService       *InboundWebhookEventService
	webhookRegistrationService       *WebhookRegistrationService
	messageHistoryService            *MessageHistoryService
	notificationCenterService        *NotificationCenterService
	segmentService                   domain.SegmentService
	workspaceRepo                    domain.WorkspaceRepository
	taskRepo                         domain.TaskRepository
	messageHistoryRepo               domain.MessageHistoryRepository
	inboundWebhookEventRepo          domain.InboundWebhookEventRepository
	broadcastRepo                    domain.BroadcastRepository
	customEventRepo                  domain.CustomEventRepository
	webAnalyticsRepo                 domain.WebAnalyticsRepository
	annotationRepo                   domain.AnnotationRepository
	webhookSubscriptionService       *WebhookSubscriptionService
	// Interface-typed, like segmentService above, so the seeded-automation guard tests can build a
	// DemoService with a generated mock. *AutomationService satisfies it, so app.go passes its own.
	automationService domain.AutomationService
	// Authenticates the seed for the demo workspace once; see seedContext.
	authService domain.AuthService
	// Holds the fixtures marker; see demoFixturesSettingKey.
	settingRepo domain.SettingRepository
	// Marks hard-bounced addresses as ProcessWebhook does; see generateDeliveredInboundWebhookEventsForBatch.
	contactRepo domain.ContactRepository
}

// demoFixturesWorkspaceID is the ID the fixtures workspace is always created under.
const demoFixturesWorkspaceID = "demo"

// demoFixturesSettingKey is the system setting that marks the workspace the fixtures
// seed created, by holding that workspace's creation time. SeedFixtures runs on any
// environment, production included, and "demo" is a plausible name for a real
// workspace on a self-hosted instance: the seed replaces a workspace only when the
// marker matches it, so it can never destroy one it did not create. The creation time
// rather than the ID, because a fixtures workspace deleted by hand and a real "demo"
// created afterwards share the ID and would otherwise inherit the marker.
//
// The marker is written twice. Before CreateWorkspace, with the time the creation
// starts: CreateWorkspace inserts the row and then runs steps that can still fail
// (owner membership, owner contact, default list) without removing it, so a marker
// written only afterwards would leave such a workspace unmarked and every later seed
// refused. After it, with the exact creation time. A workspace matches when its
// creation time is within demoFixturesMarkerWindow of the marker.
const demoFixturesSettingKey = "demo_fixtures_workspace_created_at"

// demoFixturesMarkerWindow bounds how long after the pending marker the workspace row
// may be created and still count as the seed's own. CreateWorkspace inserts within
// seconds of being called; a minute leaves room for a slow database without letting a
// workspace somebody creates by hand later match.
const demoFixturesMarkerWindow = time.Minute

// ErrDemoWorkspaceNotFixtures is returned by SeedFixtures when a workspace with the
// fixtures ID exists but was not created by the seed. Nothing has been deleted.
var ErrDemoWorkspaceNotFixtures = errors.New("a workspace with ID '" + demoFixturesWorkspaceID +
	"' exists and was not created by the demo fixtures seed; nothing was deleted")

// Sample data arrays for contact generation
var (
	firstNames = []string{
		"James", "Mary", "John", "Patricia", "Robert", "Jennifer", "Michael", "Linda",
		"William", "Elizabeth", "David", "Barbara", "Richard", "Susan", "Joseph", "Jessica",
		"Thomas", "Sarah", "Charles", "Karen", "Christopher", "Nancy", "Daniel", "Lisa",
		"Matthew", "Betty", "Anthony", "Margaret", "Mark", "Sandra", "Donald", "Ashley",
		"Steven", "Kimberly", "Paul", "Emily", "Andrew", "Donna", "Joshua", "Michelle",
		"Kenneth", "Dorothy", "Kevin", "Carol", "Brian", "Amanda", "George", "Melissa",
		"Edward", "Deborah", "Ronald", "Stephanie", "Timothy", "Rebecca", "Jason", "Sharon",
		"Jeffrey", "Laura", "Ryan", "Cynthia", "Jacob", "Kathleen", "Gary", "Amy",
		"Nicholas", "Angela", "Eric", "Shirley", "Jonathan", "Anna", "Stephen", "Ruth",
	}

	lastNames = []string{
		"Smith", "Johnson", "Williams", "Jones", "Brown", "Davis", "Miller", "Wilson",
		"Moore", "Taylor", "Anderson", "Thomas", "Jackson", "White", "Harris", "Martin",
		"Thompson", "Garcia", "Martinez", "Robinson", "Clark", "Rodriguez", "Lewis", "Lee",
		"Walker", "Hall", "Allen", "Young", "Hernandez", "King", "Wright", "Lopez",
		"Hill", "Scott", "Green", "Adams", "Baker", "Gonzalez", "Nelson", "Carter",
		"Mitchell", "Perez", "Roberts", "Turner", "Phillips", "Campbell", "Parker", "Evans",
		"Edwards", "Collins", "Stewart", "Sanchez", "Morris", "Rogers", "Reed", "Cook",
		"Morgan", "Bell", "Murphy", "Bailey", "Rivera", "Cooper", "Richardson", "Cox",
		"Howard", "Ward", "Torres", "Peterson", "Gray", "Ramirez", "James", "Watson",
	}

	emailDomains = []string{
		"gmail.com", "yahoo.com", "hotmail.com", "outlook.com", "icloud.com",
		"aol.com", "protonmail.com", "mail.com", "zoho.com", "example.com",
	}

	timezones = []string{
		"UTC", "America/New_York", "America/Los_Angeles", "America/Chicago",
		"Europe/London", "Europe/Paris", "Europe/Berlin", "Asia/Tokyo",
		"Asia/Shanghai", "Australia/Sydney",
	}

	languages = []string{
		"en", "es", "fr", "de", "it", "pt", "ru", "zh", "ja", "ko",
	}

	countries = []string{
		"US", "CA", "GB", "DE", "FR",
		"ES", "IT", "AU", "JP", "BR",
	}
)

// NewDemoService creates a new demo service instance
func NewDemoService(
	logger logger.Logger,
	config *config.Config,
	workspaceService *WorkspaceService,
	userService *UserService,
	contactService *ContactService,
	listService *ListService,
	contactListService *ContactListService,
	templateService *TemplateService,
	emailService *EmailService,
	broadcastService *BroadcastService,
	taskService *TaskService,
	transactionalNotificationService *TransactionalNotificationService,
	inboundWebhookEventService *InboundWebhookEventService,
	webhookRegistrationService *WebhookRegistrationService,
	messageHistoryService *MessageHistoryService,
	notificationCenterService *NotificationCenterService,
	segmentService domain.SegmentService,
	workspaceRepo domain.WorkspaceRepository,
	taskRepo domain.TaskRepository,
	messageHistoryRepo domain.MessageHistoryRepository,
	inboundWebhookEventRepo domain.InboundWebhookEventRepository,
	broadcastRepo domain.BroadcastRepository,
	customEventRepo domain.CustomEventRepository,
	webAnalyticsRepo domain.WebAnalyticsRepository,
	annotationRepo domain.AnnotationRepository,
	webhookSubscriptionService *WebhookSubscriptionService,
	automationService domain.AutomationService,
	authService domain.AuthService,
	settingRepo domain.SettingRepository,
) *DemoService {
	return &DemoService{
		logger:                           logger,
		config:                           config,
		workspaceService:                 workspaceService,
		userService:                      userService,
		contactService:                   contactService,
		listService:                      listService,
		contactListService:               contactListService,
		templateService:                  templateService,
		emailService:                     emailService,
		broadcastService:                 broadcastService,
		taskService:                      taskService,
		transactionalNotificationService: transactionalNotificationService,
		inboundWebhookEventService:       inboundWebhookEventService,
		webhookRegistrationService:       webhookRegistrationService,
		messageHistoryService:            messageHistoryService,
		notificationCenterService:        notificationCenterService,
		segmentService:                   segmentService,
		workspaceRepo:                    workspaceRepo,
		taskRepo:                         taskRepo,
		messageHistoryRepo:               messageHistoryRepo,
		inboundWebhookEventRepo:          inboundWebhookEventRepo,
		broadcastRepo:                    broadcastRepo,
		customEventRepo:                  customEventRepo,
		webAnalyticsRepo:                 webAnalyticsRepo,
		annotationRepo:                   annotationRepo,
		webhookSubscriptionService:       webhookSubscriptionService,
		automationService:                automationService,
		authService:                      authService,
		settingRepo:                      settingRepo,
		contactRepo:                      contactRepoOf(contactService),
	}
}

// contactRepoOf is the contact repository the contact service writes through.
func contactRepoOf(contactService *ContactService) domain.ContactRepository {
	if contactService == nil {
		return nil
	}
	return contactService.repo
}

// VerifyRootEmailHMAC verifies the HMAC of the root email
func (s *DemoService) VerifyRootEmailHMAC(providedHMAC string) bool {
	// Demo mode is single-root; use the primary configured root email.
	rootEmail := s.config.PrimaryRootEmail()
	if rootEmail == "" {
		s.logger.Error("Root email not configured")
		return false
	}

	// Use the domain function to verify HMAC with constant-time comparison
	return domain.VerifyEmailHMAC(rootEmail, providedHMAC, s.config.Security.SecretKey)
}

// ResetDemo deletes all existing workspaces and tasks, then creates a new demo workspace
func (s *DemoService) ResetDemo(ctx context.Context) error {
	s.logger.Info("Starting demo reset process")

	// Step 1: Delete all existing workspaces
	if err := s.deleteAllWorkspaces(ctx); err != nil {
		return fmt.Errorf("failed to delete existing workspaces: %w", err)
	}

	// Step 2: Create a new demo workspace
	if err := s.createDemoWorkspace(ctx); err != nil {
		return fmt.Errorf("failed to create demo workspace: %w", err)
	}

	s.logger.Info("Demo reset completed successfully")
	return nil
}

// SeedFixtures replaces the fixtures workspace and leaves every other workspace alone.
//
// Unlike ResetDemo, which empties the instance, it is safe on a working instance (a
// preview that keeps its own workspaces) and is therefore served on every environment.
// A workspace with the fixtures ID that the seed did not create is refused with
// ErrDemoWorkspaceNotFixtures; see demoFixturesSettingKey.
func (s *DemoService) SeedFixtures(ctx context.Context) error {
	s.logger.Info("Starting demo fixtures seed")

	existing, err := s.workspaceRepo.GetByID(ctx, demoFixturesWorkspaceID)
	var notFound *domain.ErrWorkspaceNotFound
	switch {
	case errors.As(err, &notFound):
		// Nothing to replace.
	case err != nil:
		return fmt.Errorf("failed to look up the fixtures workspace: %w", err)
	default:
		marked, err := s.isFixturesWorkspace(ctx, existing)
		if err != nil {
			return err
		}
		if !marked {
			return ErrDemoWorkspaceNotFixtures
		}
		if err := s.deletePreviousFixtures(ctx); err != nil {
			return fmt.Errorf("failed to delete the previous fixtures workspace: %w", err)
		}
	}

	// createDemoWorkspace names the step that failed.
	if err := s.createDemoWorkspace(ctx); err != nil {
		return err
	}

	s.logger.Info("Demo fixtures seed completed successfully")
	return nil
}

// isFixturesWorkspace reports whether the fixtures marker matches workspace.
func (s *DemoService) isFixturesWorkspace(ctx context.Context, workspace *domain.Workspace) (bool, error) {
	setting, err := s.settingRepo.Get(ctx, demoFixturesSettingKey)
	var notFound *domain.ErrSettingNotFound
	if errors.As(err, &notFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read the demo fixtures marker: %w", err)
	}
	markedAt, err := time.Parse(time.RFC3339Nano, setting.Value)
	if err != nil {
		// Not a marker this code wrote: refuse rather than guess.
		return false, nil
	}
	createdAt := demoFixturesMarkerTime(workspace.CreatedAt)
	return !createdAt.Before(markedAt) && createdAt.Sub(markedAt) <= demoFixturesMarkerWindow, nil
}

// restoreFixturesMarker puts back the marker that was there before a failed creation,
// or removes the pending one when there was none.
func (s *DemoService) restoreFixturesMarker(ctx context.Context, previous *domain.Setting) {
	var err error
	if previous == nil {
		err = s.settingRepo.Delete(ctx, demoFixturesSettingKey)
	} else {
		err = s.settingRepo.Set(ctx, demoFixturesSettingKey, previous.Value)
	}
	if err != nil {
		// The creation already failed and is reported; a pending marker left behind
		// matches only a workspace created within demoFixturesMarkerWindow of it.
		s.logger.WithField("error", err.Error()).Error("Failed to restore the demo fixtures marker")
	}
}

// demoFixturesMarkerTime is the creation time as the marker stores it. The repository
// truncates created_at to the microsecond PostgreSQL keeps, so both sides of the
// comparison are brought there.
func demoFixturesMarkerTime(createdAt time.Time) time.Time {
	return createdAt.UTC().Truncate(time.Microsecond)
}

// deletePreviousFixtures deletes the fixtures workspace the way an owner does, as
// root: integrations first, then tasks, then the workspace and its database. The
// seed runs on working instances with their task scheduler on, so tasks go before
// the workspace, and an integration somebody connected to the fixtures workspace is
// unregistered from its provider rather than left pointing at a dropped database.
func (s *DemoService) deletePreviousFixtures(ctx context.Context) error {
	rootUser, err := s.lookupRootUser(ctx)
	if err != nil {
		return err
	}
	rootCtx, endRootSession, err := s.rootContext(ctx, rootUser)
	if err != nil {
		return err
	}
	defer endRootSession()
	return s.workspaceService.DeleteWorkspace(rootCtx, demoFixturesWorkspaceID)
}

// lookupRootUser returns the user the demo workspace is created and deleted as.
func (s *DemoService) lookupRootUser(ctx context.Context) (*domain.User, error) {
	// Demo mode is single-root: the primary configured root email.
	rootEmail := s.config.PrimaryRootEmail()
	s.logger.WithField("root_email", rootEmail).Info("Looking up root user for demo workspace creation")

	rootUser, err := s.userService.GetUserByEmail(ctx, rootEmail)
	if err != nil {
		s.logger.WithField("root_email", rootEmail).WithField("error", err.Error()).Error("Failed to get root user")
		return nil, fmt.Errorf("failed to get root user with email '%s': %w", rootEmail, err)
	}

	s.logger.WithField("root_user_id", rootUser.ID).WithField("root_user_type", rootUser.Type).Info("Found root user for demo workspace creation")
	return rootUser, nil
}

// deleteAllWorkspaces deletes all workspaces from the system
func (s *DemoService) deleteAllWorkspaces(ctx context.Context) error {
	s.logger.Info("Deleting all existing workspaces")

	// Get all workspaces
	workspaces, err := s.workspaceRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list workspaces: %w", err)
	}

	// Delete each workspace
	for _, workspace := range workspaces {
		s.logger.WithField("workspace_id", workspace.ID).Info("Deleting workspace")
		if err := s.workspaceRepo.Delete(ctx, workspace.ID); err != nil {
			s.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Error("Failed to delete workspace")
			// Continue with other workspaces even if one fails
		}
		if err := s.taskRepo.DeleteAll(ctx, workspace.ID); err != nil {
			s.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Error("Failed to delete tasks")
			// Continue with other workspaces even if one fails
		}
	}

	s.logger.WithField("count", len(workspaces)).Info("Finished deleting workspaces")
	return nil
}

// demoSessionExpiry bounds the session the demo seed acts through. The seed finishes
// well within it, sample data included, and nobody ever holds a token for it.
const demoSessionExpiry = 24 * time.Hour

// rootContext authenticates the demo seed as the root account. Root is a person, so it
// acts through a session of its own, as after a sign-in: a context without a session
// only ever authenticates an API key account. The caller runs end once the seed is
// done, which closes that session.
func (s *DemoService) rootContext(ctx context.Context, rootUser *domain.User) (authenticatedCtx context.Context, end func(), err error) {
	authenticatedCtx = context.WithValue(ctx, domain.UserIDKey, rootUser.ID)
	if rootUser.Type != domain.UserTypeUser {
		return context.WithValue(authenticatedCtx, domain.UserTypeKey, string(rootUser.Type)), func() {}, nil
	}

	session, err := s.userService.startSession(ctx, rootUser.ID, demoSessionExpiry)
	if err != nil {
		s.logger.WithField("root_user_id", rootUser.ID).WithField("error", err.Error()).Error("Failed to open a session for the demo seed")
		return nil, nil, err
	}
	end = func() {
		if err := s.userService.endSession(context.WithoutCancel(ctx), session.ID); err != nil {
			// It expires on its own; nobody holds a token for it.
			s.logger.WithField("root_user_id", rootUser.ID).WithField("error", err.Error()).Warn("Failed to close the demo seed session")
		}
	}
	authenticatedCtx = context.WithValue(authenticatedCtx, domain.UserTypeKey, string(domain.UserTypeUser))
	return context.WithValue(authenticatedCtx, domain.SessionIDKey, session.ID), end, nil
}

// seedContext authenticates the seed for the demo workspace once, as a request is,
// and returns the context the rest of the seed acts through. Later checks read that
// result rather than the session, so the seed does not depend on the session staying
// open: on the demo every visitor shares the root account, and one of them signing
// out ends all of its sessions.
func (s *DemoService) seedContext(ctx context.Context, workspaceID string) (context.Context, error) {
	seedCtx, _, _, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate the demo seed: %w", err)
	}
	return seedCtx, nil
}

// The demo workspace's brand and languages. The templates built from the gallery carry the
// name and the logo (demo_gallery/*.json), so a change here means regenerating them.
const (
	demoWorkspaceName       = "Apple"
	demoWorkspaceWebsiteURL = "https://www.apple.com"
	demoWorkspaceLogoURL    = "https://www.apple.com/ac/structured-data/images/knowledge_graph_logo.png"
)

var demoWorkspaceLanguages = []string{"en", "fr", "es"}

// createDemoWorkspace creates a new demo workspace with sample data
func (s *DemoService) createDemoWorkspace(ctx context.Context) error {
	s.logger.Info("Creating demo workspace")

	rootUser, err := s.lookupRootUser(ctx)
	if err != nil {
		return err
	}

	authenticatedCtx, endRootSession, err := s.rootContext(ctx, rootUser)
	if err != nil {
		return err
	}
	defer endRootSession()

	workspaceID := demoFixturesWorkspaceID

	// Create workspace settings with readonly demo bucket
	fileManagerSettings := domain.FileManagerSettings{
		Endpoint:  s.config.Demo.FileManagerEndpoint,
		Bucket:    s.config.Demo.FileManagerBucket,
		AccessKey: s.config.Demo.FileManagerAccessKey,
		SecretKey: s.config.Demo.FileManagerSecretKey,
	}

	// The pending marker; see demoFixturesSettingKey. Detached like CreateWorkspace
	// itself, so a caller giving up cannot leave a created workspace unmarked.
	markCtx := context.WithoutCancel(ctx)
	previousMarker, err := s.settingRepo.Get(markCtx, demoFixturesSettingKey)
	var noMarker *domain.ErrSettingNotFound
	if errors.As(err, &noMarker) {
		previousMarker, err = nil, nil
	}
	if err != nil {
		return fmt.Errorf("failed to read the demo fixtures marker: %w", err)
	}
	pending := demoFixturesMarkerTime(time.Now()).Format(time.RFC3339Nano)
	if err := s.settingRepo.Set(markCtx, demoFixturesSettingKey, pending); err != nil {
		return fmt.Errorf("failed to mark the demo workspace as fixtures: %w", err)
	}

	// Create the demo workspace
	workspace, err := s.workspaceService.CreateWorkspace(
		authenticatedCtx,
		workspaceID,
		demoWorkspaceName,
		demoWorkspaceWebsiteURL,
		demoWorkspaceLogoURL,
		"https://demo.notifuse.com/cover.png",
		"UTC",
		fileManagerSettings,
		domain.DefaultLanguageCode, demoWorkspaceLanguages,
	)
	if err != nil {
		if errors.Is(err, domain.ErrWorkspaceAlreadyExists) {
			// The workspace in the way is not the one this call started, so the
			// pending marker must not claim it: put back what was there.
			s.restoreFixturesMarker(markCtx, previousMarker)
		}
		return fmt.Errorf("failed to create demo workspace: %w", err)
	}

	s.logger.WithField("workspace_id", workspace.ID).Info("Demo workspace created successfully")

	// The exact creation time, before any sample data: a seed that fails from here
	// on leaves a workspace the next SeedFixtures may replace.
	marker := demoFixturesMarkerTime(workspace.CreatedAt).Format(time.RFC3339Nano)
	if err := s.settingRepo.Set(markCtx, demoFixturesSettingKey, marker); err != nil {
		return fmt.Errorf("failed to mark the demo workspace as fixtures: %w", err)
	}

	authenticatedCtx, err = s.seedContext(authenticatedCtx, workspace.ID)
	if err != nil {
		return err
	}

	// Create SMTP integration for demo emails
	if err := s.createDemoSMTPIntegration(authenticatedCtx, workspace.ID); err != nil {
		s.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Warn("Failed to create SMTP integration")
		// Don't fail the entire operation if SMTP integration creation fails
	}

	// Add comprehensive sample data to the workspace.
	//
	// Detached from the caller's connection for its whole length, not just for
	// the web analytics step. A reset takes tens of seconds and the server sets
	// no request timeouts, so the only way this context dies is the operator
	// closing the tab or a proxy giving up — and every step here aborts the rest
	// on error. Half a demo, silently, is the worst outcome available; the reset
	// is manually triggered, HMAC-gated and behind a five-minute rate limit, so
	// there is nobody for cancellation to protect.
	//
	// A failure here fails the reset or seed. It used to be a warning with a 200,
	// so a scheduled run that left a workspace without templates, broadcasts or
	// history was reported as a success and nobody looked. The workspace stays
	// as far as it got, and the next run replaces it.
	if err := s.addSampleData(context.WithoutCancel(authenticatedCtx), workspace.ID); err != nil {
		return fmt.Errorf("failed to add sample data to the demo workspace: %w", err)
	}

	// Create webhook subscription AFTER sample data so DB triggers don't fire
	// for all the seed data, avoiding thousands of unnecessary webhook deliveries
	if err := s.createDemoWebhookSubscription(authenticatedCtx, workspace.ID); err != nil {
		s.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Warn("Failed to create demo webhook subscription")
		// Non-fatal - continue with demo setup
	} else {
		s.logger.WithField("workspace_id", workspace.ID).Info("Demo webhook subscription created")
	}

	return nil
}

// createDemoWebhookSubscription adds the demo's webhook subscription, switched off.
//
// It exists so the console has one to show — subscribing to every event type, so
// the event list is worth reading — but it must not deliver. The trigger
// functions fan out to `enabled = true` subscriptions only, so an enabled one
// would write a webhook_deliveries row for every contact, event and message the
// demo produces, and the delivery worker would then retry each of them up to ten
// times against a placeholder URL that answers nothing. Anyone who wants to watch
// it work can point it at their own endpoint and toggle it on.
//
// Created enabled and then switched off, because Create has no say in it. If the
// switch-off fails the subscription is removed rather than left running: shipping
// an enabled firehose is the one outcome this function exists to prevent, and a
// demo without a webhook example is a much smaller loss.
func (s *DemoService) createDemoWebhookSubscription(ctx context.Context, workspaceID string) error {
	const (
		name = "Demo Webhook"
		url  = "https://webhook.site/demo"
	)

	subscription, err := s.webhookSubscriptionService.Create(
		ctx, workspaceID, name, url,
		domain.WebhookEventTypes, // Subscribe to all event types
		nil,
		// User-created, and unfiltered on both lists and segments: the demo
		// subscription exists to show the console what the full event surface
		// looks like, so narrowing it would defeat the point.
		domain.WebhookSubscriptionSourceUser, nil, nil,
	)
	if err != nil {
		return err
	}

	disabled := false
	if _, err := s.webhookSubscriptionService.Update(
		ctx, workspaceID, subscription.ID, name, url, domain.WebhookEventTypes, nil, &disabled, nil, nil,
	); err != nil {
		if deleteErr := s.webhookSubscriptionService.Delete(ctx, workspaceID, subscription.ID); deleteErr != nil {
			s.logger.WithField("workspace_id", workspaceID).
				WithField("subscription_id", subscription.ID).
				WithField("error", deleteErr.Error()).
				Error("Demo webhook subscription is enabled and could not be removed")
		}
		return fmt.Errorf("failed to disable the demo webhook subscription: %w", err)
	}

	return nil
}

// addSampleData adds comprehensive sample data including 1000 contacts, templates, and broadcasts
func (s *DemoService) addSampleData(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Adding comprehensive sample data to demo workspace")

	// One clock for the whole run. The traffic window (step 3b) and the launch
	// annotation that marks its spike (step 10) are separated by the broadcasts,
	// a thousand contacts' message history, the segments and the automations —
	// long enough for a reset started just before UTC midnight to read two
	// different days and leave the marker a day off the peak it explains.
	now := time.Now().UTC()

	// Step 1: Create sample templates first
	if err := s.createSampleTemplates(ctx, workspaceID); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create sample templates")
		return err
	}

	// Step 2: Create sample lists
	if err := s.createSampleLists(ctx, workspaceID); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create sample lists")
		return err
	}

	// Step 3: Generate and add 1000 sample contacts
	if err := s.generateAndAddSampleContacts(ctx, workspaceID, 1000); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create sample contacts")
		return err
	}

	// Step 3b: Generate the web analytics history, which is also where the demo's
	// e-commerce transactions come from — every purchase belongs to a session, on
	// a product page, at the end of a funnel the visitor actually walked.
	//
	// Runs after contacts because a share of the sessions are attributed to them,
	// and before segments (step 8) because a segment built against an empty event
	// table is only populated later, asynchronously, by the recomputation queue —
	// which shows up as a demo whose audiences are empty for the first minute
	// after a reset.
	//
	// It stays synchronous deliberately. At this volume the write is seconds, not
	// minutes, and a goroutine whose only purpose is to let a manually-triggered
	// admin endpoint return sooner is complexity with no user behind it. The point
	// where that changes is a measured one: if seeding on the demo host ever runs
	// past roughly 45 seconds, split it rather than backgrounding it wholesale —
	// write the most recent ~35 days synchronously, since the default view is the
	// last 7 days and the demo is complete the moment the response returns, and
	// finish the older months behind the reset mutex.
	if err := s.seedWebAnalytics(ctx, workspaceID, now); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to generate demo web analytics")
		// Non-fatal: the rest of the demo is still usable without it.
	}

	// Step 4: Subscribe all contacts to the newsletter list
	if err := s.subscribeContactsToList(ctx, workspaceID, "newsletter"); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to subscribe contacts to newsletter list")
		return err
	}

	// Step 5: Create sample broadcast campaigns and get their IDs
	broadcastIDs, err := s.createSampleBroadcasts(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create sample broadcasts")
		return err
	}

	// Step 6: Create sample transactional notifications
	if err := s.createSampleTransactionalNotifications(ctx, workspaceID); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create sample transactional notifications")
		return err
	}

	// Step 7: Generate sample message history with realistic engagement rates using real broadcast IDs
	if err := s.generateSampleMessageHistory(ctx, workspaceID, broadcastIDs); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to generate sample message history")
		return err
	}

	// Step 8: Create sample segments, last, so every one of them is built against
	// data that is already in place.
	if err := s.createSampleSegments(ctx, workspaceID); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create sample segments")
		return err
	}

	// Step 9: Create the showcase automations, last of all. Two reasons for the position, and both
	// are load-bearing.
	//
	// After segments (step 8), because two of the five trigger on segment ids that do not exist
	// until then. And after everything else, because activating an automation installs an AFTER
	// INSERT trigger on contact_timeline: installed any earlier it would enroll a contact for every
	// row the rest of the seed writes, and with every_time frequency and repeat buyers that burst
	// collides with contact_automations' UNIQUE(automation_id, contact_email, entered_at) inside the
	// custom-events batch transaction — which would abort it and take the demo's whole conversion
	// history with it.
	if err := s.createSampleAutomations(ctx, workspaceID); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create sample automations")
		// Non-fatal: the rest of the demo is still usable without them.
	}

	// Step 10: Annotate the charts. It has to run after the web analytics (step 3b),
	// which is what puts the launch spike on the chart the manual annotation points
	// at, and after the broadcasts (step 5), because one annotation per broadcast is
	// read back from the broadcast rows.
	if err := s.seedAnnotations(ctx, workspaceID, now); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to seed demo annotations")
		// Non-fatal: the rest of the demo is still usable without them.
	}

	s.logger.WithField("workspace_id", workspaceID).Info("Comprehensive sample data added successfully")
	return nil
}

// generateAndAddSampleContacts creates realistic sample contacts
func (s *DemoService) generateAndAddSampleContacts(ctx context.Context, workspaceID string, count int) error {
	s.logger.WithField("workspace_id", workspaceID).WithField("count", count).Info("Generating sample contacts")

	// Create contacts in batches to avoid overwhelming the system
	batchSize := 100
	for i := 0; i < count; i += batchSize {
		remaining := count - i
		currentBatchSize := batchSize
		if remaining < batchSize {
			currentBatchSize = remaining
		}

		batch := s.generateSampleContactsBatch(currentBatchSize, i)

		// Add batch to workspace
		for _, contact := range batch {
			operation := s.contactService.UpsertContact(ctx, workspaceID, contact)
			if operation.Action == domain.UpsertContactOperationError {
				s.logger.WithField("email", contact.Email).WithField("error", operation.Error).Debug("Failed to create sample contact")
			}
		}

		s.logger.WithField("batch", i/batchSize+1).WithField("processed", i+currentBatchSize).Info("Processed contact batch")
	}

	s.logger.WithField("workspace_id", workspaceID).WithField("total_contacts", count).Info("Sample contacts generation completed")
	return nil
}

// createDemoSMTPIntegration creates the demo SMTP integration
func (s *DemoService) createDemoSMTPIntegration(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating demo SMTP integration")

	// Create SMTP provider configuration
	smtpProvider := domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{
			Host:     "mailpit.notifuse.com",
			Port:     1025,
			Username: "admin",
			Password: "", // No password needed for demo Mailpit
			UseTLS:   false,
		},
		Senders: []domain.EmailSender{
			{
				ID:        uuid.New().String(),
				Email:     "demo@notifuse.com",
				Name:      "Notifuse Demo",
				IsDefault: true,
			},
		},
		RateLimitPerMinute: 25,
	}

	// Create the integration
	integrationID, err := s.workspaceService.CreateIntegration(ctx, domain.CreateIntegrationRequest{
		WorkspaceID: workspaceID,
		Name:        "Demo SMTP Integration",
		Type:        domain.IntegrationTypeEmail,
		Provider:    smtpProvider,
	})
	if err != nil {
		return fmt.Errorf("failed to create SMTP integration: %w", err)
	}

	// Get current workspace to update settings
	workspace, err := s.workspaceService.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace for settings update: %w", err)
	}

	// Update workspace settings to use this integration for both transactional and marketing emails
	workspace.Settings.TransactionalEmailProviderID = integrationID
	workspace.Settings.MarketingEmailProviderID = integrationID

	// Update the workspace with the new settings
	_, err = s.workspaceService.UpdateWorkspace(ctx, workspaceID, workspace.Name, workspace.Settings)
	if err != nil {
		return fmt.Errorf("failed to update workspace settings with email provider IDs: %w", err)
	}

	s.logger.WithField("workspace_id", workspaceID).WithField("integration_id", integrationID).Info("Demo SMTP integration created and set as transactional and marketing email provider")
	return nil
}

// generateSampleContactsBatch creates a batch of sample contacts
func (s *DemoService) generateSampleContactsBatch(count int, startIndex int) []*domain.Contact {
	contacts := make([]*domain.Contact, count)

	for i := 0; i < count; i++ {
		firstName := getRandomElement(firstNames)
		lastName := getRandomElement(lastNames)
		email := generateEmail(firstName, lastName, startIndex+i)

		// Spread over the same window the web analytics history covers. Six
		// months used to be enough because the purchase history was invented
		// alongside the contact; now it comes from the site's own traffic, and a
		// list that is younger than the traffic leaves contacts whose first
		// recorded visit predates their own "Contact created" entry.
		//
		// Weighted towards the older end (the square root of a uniform draw), so
		// the list looks like one that grew fast and then settled rather than one
		// where a third of the members joined last month. It also decides how
		// much of the demo is worth opening: a contact can only be attributed
		// visits made after they signed up, so a list skewed recent would leave
		// half the workspace with an empty browsing history.
		age := math.Sqrt(rand.Float64()) * float64(demoWebAnalyticsDays) * 24
		createdAt := time.Now().Add(-time.Duration(age) * time.Hour)

		contact := &domain.Contact{
			Email:     email,
			FirstName: &domain.NullableString{String: firstName, IsNull: false},
			LastName:  &domain.NullableString{String: lastName, IsNull: false},
			Timezone:  &domain.NullableString{String: getRandomElement(timezones), IsNull: false},
			Language:  &domain.NullableString{String: getRandomElement(languages), IsNull: false},
			Country:   &domain.NullableString{String: getRandomElement(countries), IsNull: false},
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		}

		contacts[i] = contact
	}

	return contacts
}

// The demo used to invent its purchase history here: a loop that gave 70% of
// contacts one to five orders with a random product and a random date. It was
// deleted when the web analytics funnel started producing real ones. An order
// with no session, no product page and no cart behind it cannot appear on a
// timeline next to the visit that produced it, and it made the abandoned-cart
// audiences meaningless — every contact already had a purchase, so nobody had
// abandoned anything. See seedDemoWebGoalEvents in demo_web_analytics.go.

// generateEmail creates a realistic email address
func generateEmail(firstName, lastName string, index int) string {
	domain := getRandomElement(emailDomains)

	// Various email formats to make it realistic
	switch rand.Intn(4) {
	case 0:
		return fmt.Sprintf("%s.%s@%s", strings.ToLower(firstName), strings.ToLower(lastName), domain)
	case 1:
		return fmt.Sprintf("%s%s@%s", strings.ToLower(firstName), strings.ToLower(lastName), domain)
	case 2:
		return fmt.Sprintf("%s%s%d@%s", strings.ToLower(firstName), strings.ToLower(lastName), rand.Intn(100), domain)
	default:
		return fmt.Sprintf("%s.%s%d@%s", strings.ToLower(firstName), strings.ToLower(lastName), index, domain)
	}
}

// getRandomElement returns a random element from a string slice
func getRandomElement(slice []string) string {
	return slice[rand.Intn(len(slice))]
}

// createSampleLists creates the demo lists
func (s *DemoService) createSampleLists(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating sample lists")

	// Create the main newsletter list that will contain all 1000 contacts
	newsletterList := &domain.List{
		ID:            "newsletter",
		Name:          "Newsletter",
		IsDoubleOptin: false, // Disable double opt-in for demo to simplify
		IsPublic:      true,
		Description:   "Weekly newsletter subscription list - Demo data with 1000 subscribers",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.listService.CreateList(ctx, workspaceID, newsletterList); err != nil {
		return fmt.Errorf("failed to create newsletter list: %w", err)
	}

	// A second list, so the automations that move contacts between audiences have somewhere to move
	// them. With one list, add_to_list, remove_from_list and list_status_branch are all either no-ops
	// or self-referential.
	//
	// The ID has to be alphanumeric — no hyphens, no underscores (domain/list.go, govalidator
	// .IsAlphanumeric). "vip-club" fails validation here, and because this function returns its error
	// that would abort the whole seed at step 2: no contacts, no analytics, no segments.
	vipList := &domain.List{
		ID:            "vipclub",
		Name:          "VIP Club",
		IsDoubleOptin: false,
		IsPublic:      false,
		Description:   "Top customers, promoted automatically after their first purchase",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.listService.CreateList(ctx, workspaceID, vipList); err != nil {
		return fmt.Errorf("failed to create VIP club list: %w", err)
	}

	s.logger.WithField("workspace_id", workspaceID).Info("Sample lists created successfully")
	return nil
}

// subscribeContactsToList subscribes all contacts to the specified list
func (s *DemoService) subscribeContactsToList(ctx context.Context, workspaceID, listID string) error {
	s.logger.WithField("workspace_id", workspaceID).WithField("list_id", listID).Info("Subscribing contacts to list")

	// Get all contacts (this is simplified - in production you'd paginate)
	contactsReq := &domain.GetContactsRequest{
		WorkspaceID: workspaceID,
		Limit:       1000,
	}

	contactsResp, err := s.contactService.GetContacts(ctx, contactsReq)
	if err != nil {
		return fmt.Errorf("failed to get contacts: %w", err)
	}

	// Subscribe each contact to the list
	for _, contact := range contactsResp.Contacts {
		subscribeReq := &domain.SubscribeToListsRequest{
			WorkspaceID: workspaceID,
			Contact: domain.Contact{
				Email: contact.Email,
			},
			ListIDs: []string{listID},
		}

		if err := s.listService.SubscribeToLists(ctx, subscribeReq, false); err != nil {
			s.logger.WithField("email", contact.Email).WithField("error", err.Error()).Debug("Failed to subscribe contact to list")
		}
	}

	s.logger.WithField("workspace_id", workspaceID).WithField("list_id", listID).WithField("count", len(contactsResp.Contacts)).Info("Contacts subscribed to list successfully")
	return nil
}

// createSampleTemplates creates the demo email templates
// A refused template is an error, not a warning. The demo must show everything, including
// translations, so a demo deployment runs LICENSED — and a demo that is not licensed must say
// so loudly rather than seed broadcasts, notifications and message history against
// template IDs that were never created. That is what the previous Warn-and-continue did.
func (s *DemoService) createSampleTemplates(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating sample templates")

	// Built from the template gallery's models (demo_gallery_templates.go).
	for _, spec := range demoGallerySampleTemplates {
		template, err := s.buildDemoGalleryTemplate(workspaceID, spec)
		if err != nil {
			return fmt.Errorf("demo: %s template: %w", spec.ID, err)
		}
		if err := s.templateService.CreateTemplate(ctx, workspaceID, withSubjectPreviews(template)); err != nil {
			return fmt.Errorf("demo: %s template: %w", spec.ID, err)
		}
	}

	// The templates the showcase automations send (demo_automation_templates.go).
	if err := s.createAutomationTemplates(ctx, workspaceID); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create automation templates")
	}

	s.logger.WithField("workspace_id", workspaceID).Info("Sample templates created successfully")
	return nil
}

// compileTemplateToHTML compiles an MJML structure to HTML using the notifuse_mjml package
func (s *DemoService) compileTemplateToHTML(workspaceID, messageID string, mjmlStructure notifuse_mjml.EmailBlock, testData domain.MapOfAny) string {
	// Convert domain.MapOfAny to notifuse_mjml.MapOfAny
	mjmlTestData := make(notifuse_mjml.MapOfAny)
	for k, v := range testData {
		mjmlTestData[k] = v
	}

	// Create compile request
	compileReq := notifuse_mjml.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		MessageID:        messageID,
		VisualEditorTree: mjmlStructure,
		TemplateData:     mjmlTestData,
		TrackingSettings: notifuse_mjml.TrackingSettings{
			EnableTracking: false, // Disable tracking for demo templates
		},
	}

	// Compile the template
	resp, err := notifuse_mjml.CompileTemplate(compileReq)
	if err != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to compile MJML template")
		return s.createFallbackHTML() // Return fallback HTML on error
	}

	if !resp.Success || resp.HTML == nil {
		errorMsg := "Unknown compilation error"
		if resp.Error != nil {
			errorMsg = resp.Error.Message
		}
		s.logger.WithField("error", errorMsg).Error("MJML compilation failed")
		return s.createFallbackHTML() // Return fallback HTML on error
	}

	return *resp.HTML
}

// buildEmailTranslations builds translation entries for fr and es languages
func (s *DemoService) buildEmailTranslations(
	workspaceID, messageIDPrefix string,
	subjects map[string]string,
	mjmlStructures map[string]notifuse_mjml.EmailBlock,
	testData domain.MapOfAny,
) map[string]domain.TemplateTranslation {
	translations := make(map[string]domain.TemplateTranslation)
	for lang, mjml := range mjmlStructures {
		html := s.compileTemplateToHTML(workspaceID, messageIDPrefix+"-"+lang, mjml, testData)
		translations[lang] = domain.TemplateTranslation{
			Email: &domain.EmailTemplate{
				Subject:          subjects[lang],
				CompiledPreview:  html,
				VisualEditorTree: mjml,
			},
		}
	}
	return translations
}

// createFallbackHTML creates a simple fallback HTML when MJML compilation fails
func (s *DemoService) createFallbackHTML() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Demo Template</title>
</head>
<body style="margin: 0; padding: 20px; font-family: Arial, sans-serif; background-color: #f8f9fa;">
    <div style="max-width: 600px; margin: 0 auto; background-color: #ffffff; padding: 20px; border-radius: 8px;">
        <h1 style="color: #2c3e50; text-align: center;">Demo Template</h1>
        <p style="color: #34495e; line-height: 1.6;">This is a demo email template.</p>
    </div>
</body>
</html>`
}

// demoBroadcastUTMSource is the utm_source the sample broadcasts tag their
// links with.
const demoBroadcastUTMSource = "demo.notifuse.com"

// demoBroadcastCampaigns are the sample broadcasts, the utm_campaign each one
// tags its links with, and the template behind each of its variations.
//
// The web analytics seeder emits sessions under these same campaign names, one
// per variation carrying the template id as utm_content, exactly as a real send
// stamps it (demo_web_analytics_catalog.go). The two seeders ran independently
// and each picked its own names, which left every broadcast pointing at an
// empty report.
var demoBroadcastCampaigns = []struct {
	Name     string
	Campaign string
	// One template id per variation. A send writes utm_content from it, so
	// these are what a per-variation report filters on.
	Templates []string
}{
	{"Weekly Newsletter #1", "weekly_newsletter_1", []string{"newsletter-weekly"}},
	{"Weekly Newsletter #2", "weekly_newsletter_2", []string{"newsletter-weekly-v2"}},
	{"Weekly Newsletter #3", "weekly_newsletter_3", []string{"newsletter-weekly"}},
	{"Weekly Newsletter #4 - A/B Test", "weekly_newsletter_4",
		[]string{"newsletter-weekly", "newsletter-weekly-v2"}},
}

// createSampleBroadcasts creates multiple sample broadcast campaigns and returns their IDs
func (s *DemoService) createSampleBroadcasts(ctx context.Context, workspaceID string) ([]string, error) {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating sample broadcasts")

	var broadcastIDs []string

	broadcasts := demoBroadcastCampaigns

	for i, bc := range broadcasts {
		// Variations follow the templates declared alongside the campaign, so the
		// utm_content a send stamps stays in step with the seeded sessions.
		variations := make([]domain.BroadcastVariation, 0, len(bc.Templates))
		for v, templateID := range bc.Templates {
			variations = append(variations, domain.BroadcastVariation{
				VariationName: fmt.Sprintf("variation-%c", 'a'+rune(v)),
				TemplateID:    templateID,
			})
		}

		broadcastReq := &domain.CreateBroadcastRequest{
			WorkspaceID: workspaceID,
			Name:        bc.Name,
			Audience: domain.AudienceSettings{
				List:                "newsletter",
				Segments:            []string{},
				ExcludeUnsubscribed: true,
			},
			TestSettings: domain.BroadcastTestSettings{
				Enabled:          len(bc.Templates) > 1, // A broadcast with more than one template is the A/B test
				SamplePercentage: 10,
				AutoSendWinner:   false,
				Variations:       variations,
			},
			UTMParameters: &domain.UTMParameters{
				Source:   demoBroadcastUTMSource,
				Medium:   "email",
				Campaign: bc.Campaign,
				Term:     "",
				Content:  "",
			},
		}

		broadcast, err := s.broadcastService.CreateBroadcast(ctx, broadcastReq)
		if err != nil {
			s.logger.WithField("error", err.Error()).Warn("Failed to create sample broadcast")
			continue
		}

		// Update broadcast status to "processed" since we're generating message history for it
		// Set timestamps to simulate that it was processed in the past (10-1 days ago based on campaign)
		daysAgo := 10 - (i * 2) // Spread broadcasts over last 10 days
		if daysAgo < 1 {
			daysAgo = 1
		}
		sentTime := time.Now().AddDate(0, 0, -daysAgo)
		completedTime := sentTime.Add(2 * time.Hour) // Completed 2 hours after processing started

		broadcast.Status = domain.BroadcastStatusProcessed
		broadcast.StartedAt = &sentTime
		broadcast.CompletedAt = &completedTime
		broadcast.UpdatedAt = completedTime

		// The last broadcast is the A/B test; record its final variation as the selected
		// winner (all of its messages were sent with that variation) so the Variations
		// table highlights it. Derive the template id from the variation rather than
		// hardcoding it, and set the test/winner phase timestamps so the record matches a
		// genuine winner selection. WinningTemplate/*SentAt are top-level columns
		// persisted by UpdateBroadcast below.
		if len(variations) > 1 {
			winner := variations[len(variations)-1].TemplateID
			winnerSentTime := sentTime.Add(1 * time.Hour)
			broadcast.WinningTemplate = &winner
			broadcast.TestSentAt = &sentTime
			broadcast.WinnerSentAt = &winnerSentTime
		}

		// Update the broadcast in the repository to reflect processed status
		if err := s.broadcastRepo.UpdateBroadcast(ctx, broadcast); err != nil {
			s.logger.WithField("broadcast_id", broadcast.ID).WithField("error", err.Error()).Warn("Failed to update broadcast status to processed")
			// Continue anyway - the broadcast was created, just not marked as processed
		}

		broadcastIDs = append(broadcastIDs, broadcast.ID)
		s.logger.WithField("broadcast_id", broadcast.ID).WithField("name", bc.Name).WithField("status", "sent").Info("Sample broadcast created and marked as sent")
	}

	if len(broadcastIDs) == 0 {
		return nil, fmt.Errorf("failed to create any sample broadcasts")
	}

	s.logger.WithField("workspace_id", workspaceID).WithField("count", len(broadcastIDs)).Info("Sample broadcasts created successfully")
	return broadcastIDs, nil
}

// demoNotificationIDs gives, by message type, the transactional notification that sends the
// demo's transactional emails: their messages carry its ID, as real ones do.
var demoNotificationIDs = map[string]string{
	"password-reset": "password_reset",
	"welcome":        "welcome",
}

// createSampleTransactionalNotifications creates sample transactional notifications
func (s *DemoService) createSampleTransactionalNotifications(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating sample transactional notifications")

	// Create password reset transactional notification
	passwordResetNotification := domain.TransactionalNotificationCreateParams{
		ID:          demoNotificationIDs["password-reset"],
		Name:        "Password Reset Email",
		Description: "Sent when a user requests to reset their password",
		Channels: domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{
				TemplateID: "password-reset",
			},
		},
		TrackingSettings: notifuse_mjml.TrackingSettings{
			EnableTracking: true,
		},
		Metadata: domain.MapOfAny{
			"category": "security",
			"priority": "high",
		},
	}

	_, err := s.transactionalNotificationService.CreateNotification(ctx, workspaceID, passwordResetNotification)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create password reset transactional notification")
	}

	// The welcome emails the demo seeds are sent by this notification.
	welcomeNotification := domain.TransactionalNotificationCreateParams{
		ID:          demoNotificationIDs["welcome"],
		Name:        "Welcome Email",
		Description: "Sent when a user signs up",
		Channels: domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{
				TemplateID: "welcome-email",
			},
		},
		TrackingSettings: notifuse_mjml.TrackingSettings{
			EnableTracking: true,
		},
		Metadata: domain.MapOfAny{
			"category": "onboarding",
		},
	}
	if _, err := s.transactionalNotificationService.CreateNotification(ctx, workspaceID, welcomeNotification); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create welcome transactional notification")
	}

	s.logger.WithField("workspace_id", workspaceID).Info("Sample transactional notifications created successfully")
	return nil
}

// generateSampleMessageHistory creates realistic message history with specified engagement rates:
// 90% delivered, 5% failed, 5% bounce, 20% opened, 10% click, 1% unsubscribed
// Each contact receives approximately 3 emails (2-4 range)
func (s *DemoService) generateSampleMessageHistory(ctx context.Context, workspaceID string, broadcastIDs []string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Generating sample message history with ~3 emails per contact")

	// Get workspace to retrieve secret key
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	// Get all contacts to create message history for
	contactsReq := &domain.GetContactsRequest{
		WorkspaceID: workspaceID,
		Limit:       1000,
	}

	contactsResp, err := s.contactService.GetContacts(ctx, contactsReq)
	if err != nil {
		return fmt.Errorf("failed to get contacts for message history: %w", err)
	}

	if len(contactsResp.Contacts) == 0 {
		s.logger.WithField("workspace_id", workspaceID).Info("No contacts found, skipping message history generation")
		return nil
	}

	// Generate messages per contact (2-4 emails each)
	// This also generates webhook events and updates for engagement (delivered, opened, clicked)
	totalMessages, err := s.generateMessagesPerContact(ctx, workspaceID, workspace.Settings.SecretKey, contactsResp.Contacts, broadcastIDs)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to generate message history")
		return err
	}

	s.logger.WithField("workspace_id", workspaceID).WithField("total_messages", totalMessages).WithField("contacts", len(contactsResp.Contacts)).WithField("avg_per_contact", float64(totalMessages)/float64(len(contactsResp.Contacts))).Info("Sample message history generation completed")
	return nil
}

// messageEngagement holds engagement timestamps for a message
type messageEngagement struct {
	shouldDeliver  bool
	shouldOpen     bool
	shouldClick    bool
	shouldBounce   bool
	shouldComplain bool
	deliveredTime  time.Time
	openedTime     time.Time
	clickedTime    time.Time
	bouncedTime    time.Time
	complainedTime time.Time
}

// The demo's SES history: a share of the emails hard-bounce (reported instead of a delivery,
// and never opened), and a few newsletters are marked as spam after their delivery.
const (
	demoNewsletterBounceRate    = 0.015
	demoTransactionalBounceRate = 0.005
	demoNewsletterComplaintRate = 0.002
)

// demoBounceOrDeliver decides, from the moment an email left, whether SES bounced it or
// delivered it.
func demoBounceOrDeliver(engagement *messageEngagement, sentTime time.Time, bounceRate float64) {
	if rand.Float64() < bounceRate {
		engagement.shouldBounce = true
		engagement.bouncedTime = demoEventAfter(sentTime.Add(time.Second), 300*time.Second) // Within 5 minutes
		return
	}
	engagement.shouldDeliver = true
	engagement.deliveredTime = demoEventAfter(sentTime, 1800*time.Second) // Within 30 minutes
}

// demoEventAfter is a random moment within `within` after `after`, and never later than now: an
// event of a recent email (an open of yesterday's newsletter, a complaint) falls in the time
// that has passed since, rather than in the future or piled up at the moment of the seed.
func demoEventAfter(after time.Time, within time.Duration) time.Time {
	now := time.Now()
	if !after.Before(now) {
		return now
	}
	if elapsed := now.Sub(after); elapsed < within {
		within = elapsed
	}
	if within <= 0 {
		return after
	}
	return after.Add(time.Duration(rand.Int63n(int64(within))))
}

// demoContactMessages sends a contact 2 to 4 of the campaigns, oldest first, each through create,
// and returns those created with what becomes of them. A message create refuses is left out.
func (s *DemoService) demoContactMessages(contact *domain.Contact, campaigns []demoCampaignTemplate, create func(*domain.MessageHistory) error) []messageEngagementData {
	numEmails := 2 + rand.Intn(3) // 2, 3, or 4 emails
	if numEmails > len(campaigns) {
		numEmails = len(campaigns)
	}
	selected := make([]demoCampaignTemplate, numEmails)
	for j, idx := range rand.Perm(len(campaigns))[:numEmails] {
		selected[j] = campaigns[idx]
	}
	// Oldest first, so what an email's outcome changes for the contact holds for the emails
	// after it: see demoStillReachable.
	sort.SliceStable(selected, func(a, b int) bool { return selected[a].daysAgo > selected[b].daysAgo })

	var sent []messageEngagementData
	var bounced, complained bool
	for _, campaign := range selected {
		if !demoStillReachable(campaign, bounced, complained) {
			continue
		}
		campaignTime := time.Now().AddDate(0, 0, -campaign.daysAgo)
		var message *domain.MessageHistory
		var engagement messageEngagement
		if campaign.broadcastID != nil {
			message, engagement = s.generateMessageHistoryForContact(contact, campaign.templateID, campaign.templateVersion, *campaign.broadcastID, campaignTime)
		} else {
			message, engagement = s.generateTransactionalMessageHistoryForContact(contact, campaign.templateID, campaign.templateVersion, campaign.messageType, campaignTime)
		}
		if create(message) != nil {
			continue
		}
		sent = append(sent, messageEngagementData{message: message, engagement: engagement})
		bounced = bounced || engagement.shouldBounce
		complained = complained || engagement.shouldComplain
	}
	return sent
}

// demoStillReachable says whether a contact is sent a campaign after the emails before it. A
// hard bounce leaves the address bounced on every list and nothing more reaches it; a spam
// complaint leaves it complained on the newsletter, which the later newsletters skip.
func demoStillReachable(campaign demoCampaignTemplate, bounced, complained bool) bool {
	if bounced {
		return false
	}
	return !(complained && campaign.broadcastID != nil)
}

// demoCampaignTemplate is one email the demo may send a contact.
type demoCampaignTemplate struct {
	templateID      string
	templateVersion int64
	broadcastID     *string // nil for transactional
	messageType     string  // "newsletter", "welcome", "password-reset"
	daysAgo         int
}

// demoTransactionalCampaigns are the demo's transactional emails over the last 10 days. Each
// message type has its notification in demoNotificationIDs.
var demoTransactionalCampaigns = []demoCampaignTemplate{
	{templateID: "welcome-email", templateVersion: 1, messageType: "welcome", daysAgo: 2},
	{templateID: "welcome-email", templateVersion: 1, messageType: "welcome", daysAgo: 5},
	{templateID: "password-reset", templateVersion: 1, messageType: "password-reset", daysAgo: 3},
	{templateID: "password-reset", templateVersion: 1, messageType: "password-reset", daysAgo: 8},
}

// generateMessagesPerContact creates message history by assigning 2-4 emails to each contact
func (s *DemoService) generateMessagesPerContact(ctx context.Context, workspaceID string, secretKey string, contacts []*domain.Contact, broadcastIDs []string) (int, error) {
	s.logger.WithField("workspace_id", workspaceID).Info("Generating messages per contact")

	campaigns := append([]demoCampaignTemplate{}, demoTransactionalCampaigns...)

	// Add newsletter campaigns using real broadcast IDs
	if len(broadcastIDs) >= 4 {
		campaigns = append(campaigns,
			demoCampaignTemplate{templateID: "newsletter-weekly", templateVersion: 1, broadcastID: &broadcastIDs[0], messageType: "newsletter", daysAgo: 1},
			demoCampaignTemplate{templateID: "newsletter-weekly-v2", templateVersion: 1, broadcastID: &broadcastIDs[1], messageType: "newsletter", daysAgo: 4},
			demoCampaignTemplate{templateID: "newsletter-weekly", templateVersion: 1, broadcastID: &broadcastIDs[2], messageType: "newsletter", daysAgo: 7},
			demoCampaignTemplate{templateID: "newsletter-weekly-v2", templateVersion: 1, broadcastID: &broadcastIDs[3], messageType: "newsletter", daysAgo: 10},
		)
	}

	totalMessages := 0
	batchSize := 50
	// The recipients drawn for each broadcast, which its A/B counters are made of.
	broadcastRecipients := map[string]int{}

	for i := 0; i < len(contacts); i += batchSize {
		end := i + batchSize
		if end > len(contacts) {
			end = len(contacts)
		}
		batch := contacts[i:end]

		// Collect engagement data for sequential processing
		var messagesWithEngagement []messageEngagementData

		for _, contact := range batch {
			create := func(message *domain.MessageHistory) error {
				err := s.messageHistoryRepo.Create(ctx, workspaceID, secretKey, message)
				if err != nil {
					s.logger.WithField("contact_email", contact.Email).WithField("error", err.Error()).Debug("Failed to create message history record")
				}
				return err
			}
			for _, data := range s.demoContactMessages(contact, campaigns, create) {
				messagesWithEngagement = append(messagesWithEngagement, data)
				totalMessages++
				if data.message.BroadcastID != nil {
					broadcastRecipients[*data.message.BroadcastID]++
				}
			}
		}

		// Apply engagement events sequentially to simulate realistic event flow
		// 1. First, generate inbound webhook events for delivered messages
		if err := s.generateDeliveredInboundWebhookEventsForBatch(ctx, workspaceID, messagesWithEngagement); err != nil {
			s.logger.WithField("error", err.Error()).Debug("Failed to generate delivered inbound webhook events")
		}

		// 2. Then, update message_history for opened messages (triggers timeline entries)
		if err := s.updateOpenedMessagesForBatch(ctx, workspaceID, messagesWithEngagement); err != nil {
			s.logger.WithField("error", err.Error()).Debug("Failed to update opened messages")
		}

		// 3. Finally, update message_history for clicked messages (triggers timeline entries)
		if err := s.updateClickedMessagesForBatch(ctx, workspaceID, messagesWithEngagement); err != nil {
			s.logger.WithField("error", err.Error()).Debug("Failed to update clicked messages")
		}
	}

	s.recordDemoABTestCounts(ctx, workspaceID, broadcastIDs, broadcastRecipients)

	s.logger.WithField("workspace_id", workspaceID).WithField("total_messages", totalMessages).Info("Messages per contact generation completed")
	return totalMessages, nil
}

// recordDemoABTestCounts gives each A/B broadcast of the demo the recipients of its test and
// winner phases, as a real send records them: its sample percentage of the recipients drawn
// went to the test, the rest to the winner.
func (s *DemoService) recordDemoABTestCounts(ctx context.Context, workspaceID string, broadcastIDs []string, recipients map[string]int) {
	for _, broadcastID := range broadcastIDs {
		total := recipients[broadcastID]
		if total == 0 {
			continue
		}
		broadcast, err := s.broadcastRepo.GetBroadcast(ctx, workspaceID, broadcastID)
		if err != nil {
			s.logger.WithField("broadcast_id", broadcastID).WithField("error", err.Error()).Warn("Failed to read sample broadcast for its A/B counts")
			continue
		}
		if !broadcast.TestSettings.Enabled {
			continue
		}
		broadcast.TestPhaseRecipientCount = total * broadcast.TestSettings.SamplePercentage / 100
		broadcast.WinnerPhaseRecipientCount = total - broadcast.TestPhaseRecipientCount
		if err := s.broadcastRepo.UpdateBroadcast(ctx, broadcast); err != nil {
			s.logger.WithField("broadcast_id", broadcastID).WithField("error", err.Error()).Warn("Failed to record sample broadcast A/B counts")
		}
	}
}

// messageEngagementData holds a message and its engagement info
type messageEngagementData struct {
	message    *domain.MessageHistory
	engagement messageEngagement
}

// generateDeliveredInboundWebhookEventsForBatch records what SES reported about each email of the
// batch: its delivery, its hard bounce or its spam complaint, as the SNS notification posted to the
// webhook (demo_ses_events.go), and the same event on the message, as ProcessWebhook writes it.
// Without the second half the demo showed a "Delivered" webhook beside a message still "Sent".
func (s *DemoService) generateDeliveredInboundWebhookEventsForBatch(ctx context.Context, workspaceID string, messagesData []messageEngagementData) error {
	// Skip if workspace service or inbound webhook event repo is not available
	if s.workspaceService == nil || s.inboundWebhookEventRepo == nil {
		s.logger.Debug("Workspace service or inbound webhook event repo not available, skipping inbound webhook events")
		return nil
	}

	workspace, err := s.workspaceService.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}
	providers := demoProvidersOf(workspace)
	if providers.transactional == "" {
		s.logger.WithField("workspace_id", workspaceID).Debug("No email provider configured, skipping inbound webhook events")
		return nil
	}

	var inboundWebhookEvents []*domain.InboundWebhookEvent
	var updates []domain.MessageEventUpdate
	var hardBounced []string
	record := func(data messageEngagementData, eventType domain.EmailEventType, at time.Time, update domain.MessageEventUpdate) error {
		integrationID, sender := providers.of(data.message)
		event, err := demoSESEvent(eventType, integrationID, sender, data.message, at)
		if err != nil {
			return err
		}
		inboundWebhookEvents = append(inboundWebhookEvents, event)
		updates = append(updates, update)
		return nil
	}
	for _, data := range messagesData {
		engagement := data.engagement
		if engagement.shouldBounce {
			reason := demoSESBounceStatusInfo()
			if err := record(data, domain.EmailEventBounce, engagement.bouncedTime, domain.MessageEventUpdate{
				ID: data.message.ID, Event: domain.MessageEventBounced, Timestamp: engagement.bouncedTime, StatusInfo: &reason,
			}); err != nil {
				return err
			}
			hardBounced = append(hardBounced, data.message.ContactEmail)
			continue
		}
		if engagement.shouldDeliver {
			if err := record(data, domain.EmailEventDelivered, engagement.deliveredTime, domain.MessageEventUpdate{
				ID: data.message.ID, Event: domain.MessageEventDelivered, Timestamp: engagement.deliveredTime,
			}); err != nil {
				return err
			}
		}
		if engagement.shouldComplain {
			if err := record(data, domain.EmailEventComplaint, engagement.complainedTime, domain.MessageEventUpdate{
				ID: data.message.ID, Event: domain.MessageEventComplained, Timestamp: engagement.complainedTime,
			}); err != nil {
				return err
			}
		}
	}

	if len(inboundWebhookEvents) > 0 {
		if err := s.inboundWebhookEventRepo.StoreEvents(ctx, workspaceID, inboundWebhookEvents); err != nil {
			return fmt.Errorf("failed to store inbound webhook events: %w", err)
		}
		s.logger.WithField("count", len(inboundWebhookEvents)).Debug("Generated SES inbound webhook events")
	}

	if len(updates) > 0 && s.messageHistoryRepo != nil {
		if err := s.messageHistoryRepo.SetStatusesIfNotSet(ctx, workspaceID, updates); err != nil {
			return fmt.Errorf("failed to record deliveries: %w", err)
		}
	}

	// As ProcessWebhook does for a hard bounce: the address is bounced on every list it is on,
	// not only on the list of the email that bounced (which the message's own trigger covers).
	if len(hardBounced) > 0 && s.contactRepo != nil {
		if err := s.contactRepo.MarkEmailsAsBounced(ctx, workspaceID, dedupeStrings(hardBounced), time.Now().UTC()); err != nil {
			return fmt.Errorf("failed to mark emails as bounced: %w", err)
		}
	}

	return nil
}

// updateOpenedMessagesForBatch updates message_history records with opened_at timestamps
func (s *DemoService) updateOpenedMessagesForBatch(ctx context.Context, workspaceID string, messagesData []messageEngagementData) error {
	for _, data := range messagesData {
		if !data.engagement.shouldOpen {
			continue
		}

		// Use SetOpened to update the message (triggers timeline entry)
		if err := s.messageHistoryRepo.SetOpened(ctx, workspaceID, data.message.ID, data.engagement.openedTime); err != nil {
			s.logger.WithField("message_id", data.message.ID).WithField("error", err.Error()).Debug("Failed to set opened status")
		}
	}

	return nil
}

// demoClickedURLs are deterministic destination URLs recorded with demo clicks
// so demo workspaces show a populated per-link click table
var demoClickedURLs = []string{
	"https://example.com/products/new-arrivals",
	"https://example.com/blog/getting-started",
	"https://example.com/pricing",
}

// updateClickedMessagesForBatch updates message_history records with clicked_at timestamps
func (s *DemoService) updateClickedMessagesForBatch(ctx context.Context, workspaceID string, messagesData []messageEngagementData) error {
	for i, data := range messagesData {
		if !data.engagement.shouldClick {
			continue
		}

		// Use SetClicked to update the message (triggers timeline entry)
		// SetClicked also sets opened_at if not already set
		clickedURL := demoClickedURLs[i%len(demoClickedURLs)]
		// Verified, as a click through a signed link is: the demo timeline shows which link
		// each contact clicked, and link-click automations can be tried on it.
		if err := s.messageHistoryRepo.SetClicked(ctx, workspaceID, data.message.ID, data.engagement.clickedTime, clickedURL, true); err != nil {
			s.logger.WithField("message_id", data.message.ID).WithField("error", err.Error()).Debug("Failed to set clicked status")
		}
	}

	return nil
}

// notificationIDOf is the ID of the demo notification that sends a message type, nil for none.
func notificationIDOf(messageType string) *string {
	id, ok := demoNotificationIDs[messageType]
	if !ok {
		return nil
	}
	return &id
}

// generateTransactionalMessageHistoryForContact creates a realistic transactional message history record for a contact
// Transactional messages have no broadcast ID and different engagement patterns
// Engagement rates: 0.5% hard bounces, the rest delivered, 60% open rate, 20% click rate
func (s *DemoService) generateTransactionalMessageHistoryForContact(contact *domain.Contact, templateID string, templateVersion int64, messageType string, baseTime time.Time) (*domain.MessageHistory, messageEngagement) {
	messageID := fmt.Sprintf("demo_%s_%s_%d", contact.Email, messageType, baseTime.Unix())

	// Create message data for transactional message
	messageData := domain.MessageData{
		Data: map[string]interface{}{
			"contact": map[string]interface{}{
				"email":      contact.Email,
				"first_name": getStringValue(contact.FirstName),
				"last_name":  getStringValue(contact.LastName),
			},
		},
		Metadata: map[string]interface{}{
			"demo_generated":   true,
			"message_type":     messageType,
			"is_transactional": true,
		},
	}

	// Add specific data for password reset messages
	if messageType == "password-reset" {
		messageData.Data["reset_url"] = "https://demo.notifuse.com/reset-password?token=demo_token_123"
	}

	// Base transactional message with sent status
	sentTime := baseTime.Add(time.Duration(rand.Intn(3600)) * time.Second) // Random time within first hour
	message := &domain.MessageHistory{
		ID:                          messageID,
		ContactEmail:                contact.Email,
		BroadcastID:                 nil, // Transactional messages have no broadcast ID
		TemplateID:                  templateID,
		TransactionalNotificationID: notificationIDOf(messageType),
		TemplateVersion:             templateVersion,
		Channel:                     "email",
		MessageData:                 messageData,
		SentAt:                      sentTime,
		CreatedAt:                   sentTime,
		UpdatedAt:                   sentTime,
	}

	// Initialize engagement
	engagement := messageEngagement{}

	// Delivered, or hard-bounced and gone no further.
	demoBounceOrDeliver(&engagement, sentTime, demoTransactionalBounceRate)
	if engagement.shouldBounce {
		return message, engagement
	}
	deliveredTime := engagement.deliveredTime

	// 60% open rate
	if rand.Float64() < 0.60 {
		openedTime := demoEventAfter(deliveredTime, 24*time.Hour) // Within 24 hours
		engagement.shouldOpen = true
		engagement.openedTime = openedTime

		// 20% click rate (of all messages, so 20/60 = 33.33% of opened messages)
		if rand.Float64() < 0.20/0.60 {
			clickedTime := demoEventAfter(openedTime, 30*time.Minute) // Within 30 minutes of opening
			engagement.shouldClick = true
			engagement.clickedTime = clickedTime
		}

		// Very low unsubscribe rates for transactional messages
		if rand.Float64() < 0.001 {
			unsubscribeTime := demoEventAfter(openedTime, time.Hour) // Within 1 hour of opening
			message.UnsubscribedAt = &unsubscribeTime
		}
	}

	return message, engagement
}

// generateMessageHistoryForContact creates a realistic message history record for a contact
// with the specified engagement rates: 1.5% hard bounces, the rest delivered, 0.2% spam
// complaints, 60% open rate, 20% click rate
func (s *DemoService) generateMessageHistoryForContact(contact *domain.Contact, templateID string, templateVersion int64, broadcastID string, baseTime time.Time) (*domain.MessageHistory, messageEngagement) {
	messageID := fmt.Sprintf("demo_%s_%s_%d", contact.Email, broadcastID, baseTime.Unix())

	// Determine campaign type based on broadcastID
	var campaignType string
	utmMedium := "email"

	if strings.Contains(broadcastID, "transactional") {
		campaignType = "transactional"
		switch templateID {
		case "password-reset":
			campaignType = "password_reset"
		case "welcome-email":
			campaignType = "welcome"
		}
	} else {
		campaignType = "newsletter"
	}

	// Create base message data
	messageData := domain.MessageData{
		Data: map[string]interface{}{
			"contact": map[string]interface{}{
				"email":      contact.Email,
				"first_name": getStringValue(contact.FirstName),
				"last_name":  getStringValue(contact.LastName),
			},
			"utm_source":   "demo.notifuse.com",
			"utm_medium":   utmMedium,
			"utm_campaign": broadcastID,
		},
		Metadata: map[string]interface{}{
			"demo_generated": true,
			"campaign_type":  campaignType,
		},
	}

	// Base message with sent status
	sentTime := baseTime.Add(time.Duration(rand.Intn(3600)) * time.Second) // Random time within first hour
	// The list the demo's broadcasts go to, which a broadcast's message stores: a complaint or a
	// hard bounce on it sets the contact's status on that list (apply_message_status_to_contact_lists).
	listID := demoListNewsletter
	message := &domain.MessageHistory{
		ID:              messageID,
		ContactEmail:    contact.Email,
		BroadcastID:     &broadcastID,
		ListID:          &listID,
		TemplateID:      templateID,
		TemplateVersion: templateVersion,
		Channel:         "email",
		MessageData:     messageData,
		SentAt:          sentTime,
		CreatedAt:       sentTime,
		UpdatedAt:       sentTime,
	}

	// Initialize engagement
	engagement := messageEngagement{}

	// Delivered, or hard-bounced and gone no further.
	demoBounceOrDeliver(&engagement, sentTime, demoNewsletterBounceRate)
	if engagement.shouldBounce {
		return message, engagement
	}
	deliveredTime := engagement.deliveredTime

	// A few recipients mark the newsletter as spam once delivered.
	if rand.Float64() < demoNewsletterComplaintRate {
		engagement.shouldComplain = true
		engagement.complainedTime = demoEventAfter(deliveredTime, 48*time.Hour) // Within 48 hours
	}

	// 60% open rate
	if rand.Float64() < 0.60 {
		openedTime := demoEventAfter(deliveredTime, 7*24*time.Hour) // Within 7 days
		engagement.shouldOpen = true
		engagement.openedTime = openedTime

		// 20% click rate (of all messages, so 20/60 = 33.33% of opened messages)
		if rand.Float64() < 0.20/0.60 {
			clickedTime := demoEventAfter(openedTime, time.Hour) // Within 1 hour of opening
			engagement.shouldClick = true
			engagement.clickedTime = clickedTime
		}

		// 1% unsubscribed (of all messages, not just opened)
		if rand.Float64() < 0.01/0.60 {
			unsubscribeTime := demoEventAfter(openedTime, 30*time.Minute) // Within 30 minutes of opening
			message.UnsubscribedAt = &unsubscribeTime
		}
	}

	return message, engagement
}

// Helper function to get string value from NullableString
func getStringValue(ns *domain.NullableString) string {
	if ns != nil && !ns.IsNull {
		return ns.String
	}
	return ""
}

// createSampleSegments creates demo segments for showcasing the segmentation feature
func (s *DemoService) createSampleSegments(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating sample segments")

	// Segment 1: VIP Customers (high lifetime value and orders) - demonstrates AND logic with custom events goals
	vipSegment := &domain.CreateSegmentRequest{
		WorkspaceID: workspaceID,
		ID:          "vip_customers",
		Name:        "VIP Customers",
		Color:       "gold",
		Timezone:    "UTC",
		Tree: &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "and",
				Leaves: []*domain.TreeNode{
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Source: "custom_events_goals",
							CustomEventsGoal: &domain.CustomEventsGoalCondition{
								GoalType:          "purchase",
								AggregateOperator: "sum",
								Operator:          "gte",
								Value:             800.0,
								TimeframeOperator: "anytime",
							},
						},
					},
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Source: "custom_events_goals",
							CustomEventsGoal: &domain.CustomEventsGoalCondition{
								GoalType:          "purchase",
								AggregateOperator: "count",
								Operator:          "gte",
								Value:             3.0,
								TimeframeOperator: "anytime",
							},
						},
					},
				},
			},
		},
	}

	if _, err := s.segmentService.CreateSegment(ctx, vipSegment); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create VIP Customers segment")
	} else {
		s.logger.Info("Created VIP Customers segment")
	}

	// Segment 2: European Market (complex OR logic) - demonstrates OR logic
	europeSegment := &domain.CreateSegmentRequest{
		WorkspaceID: workspaceID,
		ID:          "european_market",
		Name:        "European Market",
		Color:       "geekblue",
		Timezone:    "UTC",
		Tree: &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "or",
				Leaves: []*domain.TreeNode{
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Source: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "country",
										FieldType:    "string",
										Operator:     "equals",
										StringValues: []string{"GB"},
									},
								},
							},
						},
					},
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Source: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "country",
										FieldType:    "string",
										Operator:     "equals",
										StringValues: []string{"FR"},
									},
								},
							},
						},
					},
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Source: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "country",
										FieldType:    "string",
										Operator:     "equals",
										StringValues: []string{"DE"},
									},
								},
							},
						},
					},
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Source: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "country",
										FieldType:    "string",
										Operator:     "equals",
										StringValues: []string{"ES"},
									},
								},
							},
						},
					},
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Source: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "country",
										FieldType:    "string",
										Operator:     "equals",
										StringValues: []string{"IT"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if _, err := s.segmentService.CreateSegment(ctx, europeSegment); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create European Market segment")
	} else {
		s.logger.Info("Created European Market segment")
	}

	// Segment 3: Engaged Users (behavioral - email opens) - demonstrates timeline-based filtering
	timeframeOperator := "anytime"
	engagedSegment := &domain.CreateSegmentRequest{
		WorkspaceID: workspaceID,
		ID:          "engaged_users",
		Name:        "Engaged Users",
		Color:       "green",
		Timezone:    "UTC",
		Tree: &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "and",
				Leaves: []*domain.TreeNode{
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Source: "contact_timeline",
							ContactTimeline: &domain.ContactTimelineCondition{
								Kind:              "email.opened",
								CountOperator:     "at_least",
								CountValue:        3,
								TimeframeOperator: &timeframeOperator,
								TimeframeValues:   []string{},
								Filters:           []*domain.DimensionFilter{},
							},
						},
					},
				},
			},
		},
	}

	if _, err := s.segmentService.CreateSegment(ctx, engagedSegment); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create Engaged Users segment")
	} else {
		s.logger.Info("Created Engaged Users segment")
	}

	// Segment 4: Win-back Opportunities (negated goal) - demonstrates "has not done X in the last
	// N days", combined with the positive condition that makes it a win-back rather than a
	// prospecting list: someone who bought at some point, and has not bought since.
	//
	// The negated leaf carries the interesting half. Its aggregation compiles to a subquery grouped
	// by contact, so a contact with zero matching events produces no group and can never satisfy
	// the comparison however it is written — NOT EXISTS is the only form that also matches the
	// people who never converted, which is what "has not purchased" has to mean.
	//
	// The "anytime" leaf then excludes those never-buyers again. Without it the segment is every
	// contact who is not a recent customer, which on this demo is over 90% of the workspace: the
	// purchase history now comes from the web funnel (see seedDemoWebGoalEvents), where a few
	// hundred contacts have ever bought rather than the 700 the old invented order rows gave
	// everybody.
	winbackSegment := &domain.CreateSegmentRequest{
		WorkspaceID: workspaceID,
		ID:          "winback_opportunities",
		Name:        "Win-back Opportunities",
		Color:       "volcano",
		Timezone:    "UTC",
		Tree: demoSegmentAllOf(
			// Bought at least once, ever.
			demoGoalLeaf(&domain.CustomEventsGoalCondition{
				GoalType:          domain.GoalTypePurchase,
				AggregateOperator: "count",
				Operator:          "gte",
				Value:             1.0,
				TimeframeOperator: "anytime",
			}),
			demoNoPurchaseInLast90Days(),
		),
	}

	if _, err := s.segmentService.CreateSegment(ctx, winbackSegment); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create Win-back Opportunities segment")
	} else {
		s.logger.Info("Created Win-back Opportunities segment")
	}

	// Segments 5 and 6: the abandonment audiences, the pair that shows what web analytics is for.
	// Both are goals the visitor fired on the site — recorded as custom events by the same bridge
	// that records live traffic — crossed with the absence of the goal that would have completed
	// the funnel.
	//
	// Matched on event_name rather than goal_type: add_to_cart and checkout_start are both typed
	// "other" (only a purchase carries revenue), so the type alone cannot tell a cart from a
	// checkout.
	abandonment := []struct {
		id, name, color, eventName string
	}{
		{"cart_abandoners", "Cart Abandoners", "orange", "add_to_cart"},
		{"checkout_abandoners", "Checkout Abandoners", "magenta", "checkout_start"},
	}

	for _, segment := range abandonment {
		eventName := segment.eventName
		request := &domain.CreateSegmentRequest{
			WorkspaceID: workspaceID,
			ID:          segment.id,
			Name:        segment.name,
			Color:       segment.color,
			Timezone:    "UTC",
			Tree: demoSegmentAllOf(
				demoGoalLeaf(&domain.CustomEventsGoalCondition{
					// Any type: the cart and checkout steps carry no revenue and so are
					// typed "other", which "*" covers without naming it.
					GoalType:          "*",
					EventName:         &eventName,
					AggregateOperator: "count",
					Operator:          "gte",
					Value:             1.0,
					TimeframeOperator: "in_the_last_days",
					TimeframeValues:   []string{"90"},
				}),
				demoNoPurchaseInLast90Days(),
			),
		}

		if _, err := s.segmentService.CreateSegment(ctx, request); err != nil {
			s.logger.WithField("segment", segment.id).WithField("error", err.Error()).
				Warn("Failed to create abandonment segment")
		} else {
			s.logger.WithField("segment", segment.id).Info("Created abandonment segment")
		}
	}

	s.logger.WithField("workspace_id", workspaceID).Info("Sample segments created successfully")
	return nil
}

// demoNoPurchaseInLast90Days is the leaf every abandonment audience hangs on, and the one the
// win-back segment reuses. Shared so the three cannot drift apart: an audience of people who did
// not buy is only meaningful next to the others if they all agree on what "did not buy" means.
func demoNoPurchaseInLast90Days() *domain.TreeNode {
	return demoGoalLeaf(&domain.CustomEventsGoalCondition{
		GoalType:          domain.GoalTypePurchase,
		AggregateOperator: "count",
		Operator:          "gte",
		Value:             1.0,
		TimeframeOperator: "in_the_last_days",
		TimeframeValues:   []string{"90"},
		Negate:            true,
	})
}

func demoGoalLeaf(condition *domain.CustomEventsGoalCondition) *domain.TreeNode {
	return &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source:           "custom_events_goals",
			CustomEventsGoal: condition,
		},
	}
}

// demoSegmentAllOf wraps conditions in an AND branch. The root has to be a branch even for a single
// condition: the console's segment builder renders the root as a branch unconditionally, so a bare
// leaf compiles and runs correctly but cannot be opened for editing.
func demoSegmentAllOf(leaves ...*domain.TreeNode) *domain.TreeNode {
	return &domain.TreeNode{
		Kind: "branch",
		Branch: &domain.TreeNodeBranch{
			Operator: "and",
			Leaves:   leaves,
		},
	}
}
