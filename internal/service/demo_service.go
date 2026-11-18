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
	"github.com/lib/pq"
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

	// Only the domains RFC 2606 reserves for examples. The demo's contacts show on the public
	// demo, on preview instances and in the documentation's screenshots, where an address at a
	// real mailbox provider can be somebody's.
	emailDomains = []string{"example.com", "example.org", "example.net"}

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

	// Step 5: Create sample broadcast campaigns, which date and template their messages
	broadcasts, err := s.createSampleBroadcasts(ctx, workspaceID, now)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create sample broadcasts")
		return err
	}

	// Step 6: Create sample transactional notifications
	if err := s.createSampleTransactionalNotifications(ctx, workspaceID); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create sample transactional notifications")
		return err
	}

	// Step 7: Generate sample message history with realistic engagement rates, each broadcast's
	// dated and templated as that broadcast went out
	if err := s.generateSampleMessageHistory(ctx, workspaceID, broadcasts, now); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to generate sample message history")
		return err
	}

	// Step 7b: Mark the broadcasts as sent, with the counters their message history gave them.
	// Fatal: a broadcast left a draft with a full history behind it could be sent for real.
	if err := s.saveSampleBroadcasts(ctx, workspaceID, broadcasts); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to mark sample broadcasts as sent")
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
	// row the rest of the seed writes: with every_time frequency and repeat buyers, a journey for
	// every purchase of the demo's conversion history, on top of the history the seed already
	// writes for them.
	if err := s.createSampleAutomations(ctx, workspaceID); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create sample automations")
		// Non-fatal: the rest of the demo is still usable without them.
	}

	// Step 10: Annotate the charts. It has to run after the web analytics (step 3b),
	// which is what puts the launch spike on the chart the manual annotation points
	// at, and after the broadcasts are marked as sent (step 7b), because one annotation per
	// broadcast is read back from the send date on its row.
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

	// Create contacts in batches to avoid overwhelming the system. The addresses are unique
	// across the batches: UpsertContact would merge two contacts given the same one.
	batchSize := 100
	used := map[string]bool{}
	for i := 0; i < count; i += batchSize {
		remaining := count - i
		currentBatchSize := batchSize
		if remaining < batchSize {
			currentBatchSize = remaining
		}

		batch := s.generateSampleContactsBatch(currentBatchSize, i, used)

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

// generateSampleContactsBatch creates a batch of sample contacts, at addresses not in used,
// which it adds them to.
func (s *DemoService) generateSampleContactsBatch(count int, startIndex int, used map[string]bool) []*domain.Contact {
	contacts := make([]*domain.Contact, count)

	for i := 0; i < count; i++ {
		firstName := getRandomElement(firstNames)
		lastName := getRandomElement(lastNames)
		email := uniqueDemoEmail(generateEmail(firstName, lastName, startIndex+i), used)

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

// uniqueDemoEmail is email if no other contact of the seed has it, else email with a number
// after its local part ("ann.lee-2@example.com"), and records the address it returns as used.
// Three domains and a few name formats give the same address to two contacts often enough to
// matter: UpsertContact would merge them into one.
func uniqueDemoEmail(email string, used map[string]bool) string {
	candidate := email
	if at := strings.LastIndex(email, "@"); at > 0 {
		for n := 2; used[candidate]; n++ {
			candidate = fmt.Sprintf("%s-%d%s", email[:at], n, email[at:])
		}
	}
	used[candidate] = true
	return candidate
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

// How a sample broadcast went out, which dates both the broadcast row and its messages.
const (
	// demoBroadcastDraftLead is the time from a broadcast's created_at to its started_at: it
	// was written the day before it went out.
	demoBroadcastDraftLead = 24 * time.Hour
	// demoBroadcastSendWindow is the time from a broadcast's started_at to its completed_at,
	// over which its emails leave.
	demoBroadcastSendWindow = 2 * time.Hour
	// demoABTestSendWindow is the time over which the A/B test sample leaves after test_sent_at.
	demoABTestSendWindow = 15 * time.Minute
	// demoABWinnerDelay is the time from the A/B test's started_at to winner_sent_at: the test
	// ran, a winner was picked, and the rest of the audience was sent the winning template.
	demoABWinnerDelay = time.Hour
)

// createSampleBroadcasts creates the sample broadcast campaigns and returns them marked, in
// memory, as sent before now: their messages are dated and templated from them
// (demoBroadcastSend). They are stored as drafts until saveSampleBroadcasts writes them, once
// their message history has given them their counters.
func (s *DemoService) createSampleBroadcasts(ctx context.Context, workspaceID string, now time.Time) ([]*domain.Broadcast, error) {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating sample broadcasts")

	var created []*domain.Broadcast

	for i, bc := range demoBroadcastCampaigns {
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

		// Processed in the past, a broadcast every two days, the first ten days ago and the
		// last four. The message history is dated from these timestamps, never on its own.
		daysAgo := 10 - (i * 2)
		if daysAgo < 1 {
			daysAgo = 1
		}
		demoMarkBroadcastSent(broadcast, now.AddDate(0, 0, -daysAgo))

		created = append(created, broadcast)
		s.logger.WithField("broadcast_id", broadcast.ID).WithField("name", bc.Name).Info("Sample broadcast created")
	}

	if len(created) == 0 {
		return nil, fmt.Errorf("failed to create any sample broadcasts")
	}

	s.logger.WithField("workspace_id", workspaceID).WithField("count", len(created)).Info("Sample broadcasts created successfully")
	return created, nil
}

// demoMarkBroadcastSent marks a sample broadcast as processed, written the day before
// startedAt and sent from startedAt over demoBroadcastSendWindow.
//
// The A/B test (demoIsABTest) is marked with its last variation as the winner: its sample was
// sent the variations from the start, and the rest of the audience the winner once it was
// picked, so the Variations table shows both variations' sends and highlights the winner.
// WinningTemplate and the *SentAt fields are columns UpdateBroadcast writes.
func demoMarkBroadcastSent(broadcast *domain.Broadcast, startedAt time.Time) {
	completedAt := startedAt.Add(demoBroadcastSendWindow)
	broadcast.Status = domain.BroadcastStatusProcessed
	broadcast.CreatedAt = startedAt.Add(-demoBroadcastDraftLead)
	broadcast.StartedAt = &startedAt
	broadcast.CompletedAt = &completedAt
	broadcast.UpdatedAt = completedAt

	if demoIsABTest(broadcast) {
		variations := broadcast.TestSettings.Variations
		winner := variations[len(variations)-1].TemplateID
		testSentAt := startedAt
		winnerSentAt := startedAt.Add(demoABWinnerDelay)
		broadcast.WinningTemplate = &winner
		broadcast.TestSentAt = &testSentAt
		broadcast.WinnerSentAt = &winnerSentAt
	}
}

// demoIsABTest says whether a sample broadcast is the A/B test: a test with more than one
// variation. It is the one predicate both the broadcast's dates (demoMarkBroadcastSent) and
// its messages (demoBroadcastSend) go by.
func demoIsABTest(broadcast *domain.Broadcast) bool {
	return broadcast.TestSettings.Enabled && len(broadcast.TestSettings.Variations) > 1
}

// demoBroadcastSend draws how one recipient's email of a sample broadcast went out: the
// template and the moment it left, never later than now. The broadcast is one
// demoBroadcastCampaign accepts.
//
// A broadcast's emails leave from its started_at until its completed_at. The A/B test's leave
// in its two phases, as the orchestrator sends them: the sample, its percentage of the
// recipients, right after test_sent_at, each recipient sent one of the variations at random as
// the email queue picks among the templates it holds; then everyone else, sent the winning
// template from winner_sent_at.
func demoBroadcastSend(broadcast *domain.Broadcast, now time.Time) (templateID string, sentAt time.Time) {
	if demoIsABTest(broadcast) {
		if rand.Intn(100) < broadcast.TestSettings.SamplePercentage {
			variations := broadcast.TestSettings.Variations
			return variations[rand.Intn(len(variations))].TemplateID, demoEventAfter(now, *broadcast.TestSentAt, demoABTestSendWindow)
		}
		return *broadcast.WinningTemplate, demoEventAfter(now, *broadcast.WinnerSentAt, broadcast.CompletedAt.Sub(*broadcast.WinnerSentAt))
	}
	return broadcast.TestSettings.Variations[0].TemplateID, demoEventAfter(now, *broadcast.StartedAt, broadcast.CompletedAt.Sub(*broadcast.StartedAt))
}

// demoInTestPhase says whether an email of the A/B test was part of its sample: the sample
// leaves within demoABTestSendWindow of test_sent_at, well before winner_sent_at.
func demoInTestPhase(broadcast *domain.Broadcast, sentAt time.Time) bool {
	return demoIsABTest(broadcast) && sentAt.Before(*broadcast.WinnerSentAt)
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
func (s *DemoService) generateSampleMessageHistory(ctx context.Context, workspaceID string, broadcasts []*domain.Broadcast, now time.Time) error {
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
	totalMessages, err := s.generateMessagesPerContact(ctx, workspaceID, workspace.Settings.SecretKey, contactsResp.Contacts, broadcasts, now)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to generate message history")
		return err
	}

	// Without it every event on the contacts' timelines is dated at the seed.
	if err := s.backdateDemoMessageTimeline(ctx, workspaceID); err != nil {
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
// delivered it, by now.
func demoBounceOrDeliver(engagement *messageEngagement, now, sentTime time.Time, bounceRate float64) {
	if rand.Float64() < bounceRate {
		engagement.shouldBounce = true
		engagement.bouncedTime = demoEventAfter(now, sentTime.Add(time.Second), 300*time.Second) // Within 5 minutes
		return
	}
	engagement.shouldDeliver = true
	engagement.deliveredTime = demoEventAfter(now, sentTime, 1800*time.Second) // Within 30 minutes
}

// demoEventAfter is a random moment within `within` after `after`, and never later than now, the
// seed's one clock: an event of a recent email (an open of yesterday's newsletter, a complaint)
// falls in the time that has passed since, rather than in the future or piled up at the moment
// of the seed.
func demoEventAfter(now, after time.Time, within time.Duration) time.Time {
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
// and returns those created with what becomes of them by now. A message create refuses is left
// out.
func (s *DemoService) demoContactMessages(contact *domain.Contact, campaigns []demoCampaignTemplate, now time.Time, create func(*domain.MessageHistory) error) []messageEngagementData {
	numEmails := 2 + rand.Intn(3) // 2, 3, or 4 emails
	if numEmails > len(campaigns) {
		numEmails = len(campaigns)
	}
	selected := make([]demoSend, numEmails)
	for j, idx := range rand.Perm(len(campaigns))[:numEmails] {
		selected[j] = campaigns[idx].draw(now)
	}
	// In the order they left, so what an email's outcome changes for the contact holds for the
	// emails after it: see demoStillReachable. A broadcast's emails leave over hours, so the
	// order is the moment each one left, not the day its campaign went out.
	sort.SliceStable(selected, func(a, b int) bool { return selected[a].sentAt.Before(selected[b].sentAt) })

	var sent []messageEngagementData
	var bounced, complained bool
	for _, send := range selected {
		campaign := send.campaign
		if !demoStillReachable(campaign, bounced, complained) {
			continue
		}
		var message *domain.MessageHistory
		var engagement messageEngagement
		if campaign.broadcast != nil {
			message, engagement = s.generateMessageHistoryForContact(contact, send.templateID, campaign.templateVersion, campaign.broadcast, send.sentAt, now)
		} else {
			message, engagement = s.generateTransactionalMessageHistoryForContact(contact, send.templateID, campaign.templateVersion, campaign.messageType, send.sentAt, now)
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
	return !(complained && campaign.broadcast != nil)
}

// demoCampaignTemplate is one email the demo may send a contact: a sample broadcast, whose row
// gives the template and the moment of each email (demoBroadcastSend), or a transactional
// email of templateID, sent daysAgo.
type demoCampaignTemplate struct {
	templateID      string // transactional only
	templateVersion int64
	broadcast       *domain.Broadcast // nil for transactional
	messageType     string            // "newsletter", "welcome", "password-reset"
	daysAgo         int               // transactional only
}

// demoSend is one email of a campaign drawn for a contact: its template and when it left.
type demoSend struct {
	campaign   demoCampaignTemplate
	templateID string
	sentAt     time.Time
}

// draw picks the template and the moment of one email of the campaign. A transactional email
// leaves within the hour of its day.
func (c demoCampaignTemplate) draw(now time.Time) demoSend {
	if c.broadcast != nil {
		templateID, sentAt := demoBroadcastSend(c.broadcast, now)
		return demoSend{campaign: c, templateID: templateID, sentAt: sentAt}
	}
	return demoSend{campaign: c, templateID: c.templateID, sentAt: demoEventAfter(now, now.AddDate(0, 0, -c.daysAgo), time.Hour)}
}

// demoTransactionalCampaigns are the demo's transactional emails over the last 10 days. Each
// message type has its notification in demoNotificationIDs.
var demoTransactionalCampaigns = []demoCampaignTemplate{
	{templateID: "welcome-email", templateVersion: 1, messageType: "welcome", daysAgo: 2},
	{templateID: "welcome-email", templateVersion: 1, messageType: "welcome", daysAgo: 5},
	{templateID: "password-reset", templateVersion: 1, messageType: "password-reset", daysAgo: 3},
	{templateID: "password-reset", templateVersion: 1, messageType: "password-reset", daysAgo: 8},
}

// demoBroadcastCampaign is the campaign of a sample broadcast, or false for a broadcast its
// messages cannot be dated from: one never marked as sent (demoMarkBroadcastSent), or with no
// template.
func demoBroadcastCampaign(broadcast *domain.Broadcast) (demoCampaignTemplate, bool) {
	if broadcast == nil || broadcast.StartedAt == nil || broadcast.CompletedAt == nil || len(broadcast.TestSettings.Variations) == 0 {
		return demoCampaignTemplate{}, false
	}
	if demoIsABTest(broadcast) && (broadcast.WinningTemplate == nil || broadcast.TestSentAt == nil || broadcast.WinnerSentAt == nil) {
		return demoCampaignTemplate{}, false
	}
	return demoCampaignTemplate{templateVersion: 1, broadcast: broadcast, messageType: "newsletter"}, true
}

// demoBroadcastCounts are the emails of a sample broadcast the seed created.
type demoBroadcastCounts struct {
	total     int
	testPhase int // the A/B test's sample
}

// generateMessagesPerContact creates message history by assigning 2-4 emails to each contact,
// with what became of them by now.
func (s *DemoService) generateMessagesPerContact(ctx context.Context, workspaceID string, secretKey string, contacts []*domain.Contact, broadcasts []*domain.Broadcast, now time.Time) (int, error) {
	s.logger.WithField("workspace_id", workspaceID).Info("Generating messages per contact")

	campaigns := append([]demoCampaignTemplate{}, demoTransactionalCampaigns...)
	byID := map[string]*domain.Broadcast{}
	for _, broadcast := range broadcasts {
		if campaign, ok := demoBroadcastCampaign(broadcast); ok {
			campaigns = append(campaigns, campaign)
			byID[broadcast.ID] = broadcast
		}
	}

	totalMessages := 0
	batchSize := 50
	// The emails created for each broadcast, which its counters are made of.
	counts := map[string]*demoBroadcastCounts{}

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
			for _, data := range s.demoContactMessages(contact, campaigns, now, create) {
				messagesWithEngagement = append(messagesWithEngagement, data)
				totalMessages++
				if data.message.BroadcastID == nil {
					continue
				}
				id := *data.message.BroadcastID
				if counts[id] == nil {
					counts[id] = &demoBroadcastCounts{}
				}
				counts[id].total++
				if demoInTestPhase(byID[id], data.message.SentAt) {
					counts[id].testPhase++
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

	setDemoBroadcastCounts(broadcasts, counts)

	s.logger.WithField("workspace_id", workspaceID).WithField("total_messages", totalMessages).Info("Messages per contact generation completed")
	return totalMessages, nil
}

// setDemoBroadcastCounts gives each sample broadcast the counters a real send leaves on it,
// made of the emails the seed created for it. Every broadcast has its emails as enqueued_count,
// as the orchestrator records when the send finishes. The A/B test also has the recipients of
// its two phases, its sample and everyone else; a broadcast with one template keeps 0 for both,
// as setPhaseRecipientCounts leaves it.
func setDemoBroadcastCounts(broadcasts []*domain.Broadcast, counts map[string]*demoBroadcastCounts) {
	for _, broadcast := range broadcasts {
		count := counts[broadcast.ID]
		if count == nil {
			continue
		}
		broadcast.EnqueuedCount = count.total
		if demoIsABTest(broadcast) {
			broadcast.TestPhaseRecipientCount = count.testPhase
			broadcast.WinnerPhaseRecipientCount = count.total - count.testPhase
		}
	}
}

// saveSampleBroadcasts writes the sample broadcasts as sent, with their counters, in one update
// each as the orchestrator's last write of a send. It runs once their message history is
// created: the repository refuses to update a processed broadcast, so counters written after
// the status are dropped (which left the A/B test with no phase counts).
//
// The repository stamps updated_at with the moment it writes, and created_at was the moment the
// broadcast was created, so both are then set on the rows to the dates the send was given
// (demoMarkBroadcastSent): left at the seed, the broadcast reads as created after it started.
func (s *DemoService) saveSampleBroadcasts(ctx context.Context, workspaceID string, broadcasts []*domain.Broadcast) error {
	ids := make([]string, len(broadcasts))
	createdAt := make([]string, len(broadcasts))
	updatedAt := make([]string, len(broadcasts))
	for i, broadcast := range broadcasts {
		// UpdateBroadcast overwrites the in-memory UpdatedAt too.
		ids[i] = broadcast.ID
		createdAt[i] = broadcast.CreatedAt.UTC().Format(time.RFC3339Nano)
		updatedAt[i] = broadcast.UpdatedAt.UTC().Format(time.RFC3339Nano)
		if err := s.broadcastRepo.UpdateBroadcast(ctx, broadcast); err != nil {
			return fmt.Errorf("failed to mark sample broadcast %s as sent: %w", broadcast.ID, err)
		}
	}

	db, err := s.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}
	if _, err := db.ExecContext(ctx, demoBackdateBroadcastsQuery, pq.Array(ids), pq.Array(createdAt), pq.Array(updatedAt)); err != nil {
		return fmt.Errorf("failed to date the sample broadcasts: %w", err)
	}
	return nil
}

// demoBackdateBroadcastsQuery sets each broadcast's created_at and updated_at.
const demoBackdateBroadcastsQuery = `
	UPDATE broadcasts AS b
	SET created_at = v.created_at, updated_at = v.updated_at
	FROM unnest($1::text[], $2::timestamptz[], $3::timestamptz[]) AS v(id, created_at, updated_at)
	WHERE b.id = v.id`

// backdateDemoMessageTimeline dates the timeline entries of the seeded emails' events at the
// event. The timeline trigger dates an entry at the message's updated_at, which the repository
// methods the seed records events with (SetStatusesIfNotSet, SetOpened, SetClicked) set to the
// moment they run: right for a live event, but it put every delivery, open and click of the
// seeded history at the moment of the seed. The sent entries are dated right already, from the
// message's own updated_at.
func (s *DemoService) backdateDemoMessageTimeline(ctx context.Context, workspaceID string) error {
	db, err := s.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}
	if _, err := db.ExecContext(ctx, demoBackdateMessageTimelineQuery); err != nil {
		return fmt.Errorf("failed to date the message timeline entries: %w", err)
	}
	return nil
}

// demoBackdateMessageTimelineQuery moves each event entry of a message to that event's time on
// the message. An email.updated entry is a delivery, or the status of a hard bounce.
const demoBackdateMessageTimelineQuery = `
	UPDATE contact_timeline t
	SET created_at = COALESCE(CASE t.kind
		WHEN 'email.opened' THEN m.opened_at
		WHEN 'email.clicked' THEN m.clicked_at
		WHEN 'email.link_clicked' THEN m.clicked_at
		WHEN 'email.bounced' THEN m.bounced_at
		WHEN 'email.complained' THEN m.complained_at
		WHEN 'email.updated' THEN CASE WHEN t.changes ? 'delivered_at' THEN m.delivered_at ELSE m.bounced_at END
	END, t.created_at)
	FROM message_history m
	WHERE t.entity_type = 'message_history'
		AND t.entity_id = m.id
		AND t.kind IN ('email.opened', 'email.clicked', 'email.link_clicked', 'email.bounced',
			'email.complained', 'email.updated')`

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
	hardBounced := map[string]time.Time{} // address → its bounce
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
			hardBounced[data.message.ContactEmail] = engagement.bouncedTime
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
	// At the bounce, as ProcessWebhook marks it when SES reports it, rather than at the seed: the
	// list change shows on the contact's timeline. Nothing reaches an address after its bounce,
	// so each address bounces once.
	if len(hardBounced) > 0 && s.contactRepo != nil {
		if err := s.contactRepo.MarkEmailsAsBouncedAt(ctx, workspaceID, hardBounced); err != nil {
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
// Engagement rates: 0.5% hard bounces, the rest delivered, 60% open rate, 20% click rate.
// The message is sent at sentTime, and its events follow it, by now.
func (s *DemoService) generateTransactionalMessageHistoryForContact(contact *domain.Contact, templateID string, templateVersion int64, messageType string, sentTime, now time.Time) (*domain.MessageHistory, messageEngagement) {
	messageID := fmt.Sprintf("demo_%s_%s_%d", contact.Email, messageType, sentTime.Unix())

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
	demoBounceOrDeliver(&engagement, now, sentTime, demoTransactionalBounceRate)
	if engagement.shouldBounce {
		return message, engagement
	}
	deliveredTime := engagement.deliveredTime

	// 60% open rate
	if rand.Float64() < 0.60 {
		openedTime := demoEventAfter(now, deliveredTime, 24*time.Hour) // Within 24 hours
		engagement.shouldOpen = true
		engagement.openedTime = openedTime

		// 20% click rate (of all messages, so 20/60 = 33.33% of opened messages)
		if rand.Float64() < 0.20/0.60 {
			clickedTime := demoEventAfter(now, openedTime, 30*time.Minute) // Within 30 minutes of opening
			engagement.shouldClick = true
			engagement.clickedTime = clickedTime
		}

		// Very low unsubscribe rates for transactional messages
		if rand.Float64() < 0.001 {
			unsubscribeTime := demoEventAfter(now, openedTime, time.Hour) // Within 1 hour of opening
			message.UnsubscribedAt = &unsubscribeTime
		}
	}

	return message, engagement
}

// generateMessageHistoryForContact creates a realistic message history record for a contact
// with the specified engagement rates: 1.5% hard bounces, the rest delivered, 0.2% spam
// complaints, 60% open rate, 20% click rate. The message is sent at sentTime, which its
// broadcast decides (demoBroadcastSend), and its events follow it, by now.
func (s *DemoService) generateMessageHistoryForContact(contact *domain.Contact, templateID string, templateVersion int64, broadcast *domain.Broadcast, sentTime, now time.Time) (*domain.MessageHistory, messageEngagement) {
	broadcastID := broadcast.ID
	messageID := fmt.Sprintf("demo_%s_%s_%d", contact.Email, broadcastID, sentTime.Unix())

	// The UTM parameters the broadcast tags its links with: the same campaign name as the
	// seeded web sessions (demoBroadcastCampaigns).
	campaign := ""
	if broadcast.UTMParameters != nil {
		campaign = broadcast.UTMParameters.Campaign
	}
	messageData := domain.MessageData{
		Data: map[string]interface{}{
			"contact": map[string]interface{}{
				"email":      contact.Email,
				"first_name": getStringValue(contact.FirstName),
				"last_name":  getStringValue(contact.LastName),
			},
			"utm_source":   demoBroadcastUTMSource,
			"utm_medium":   "email",
			"utm_campaign": campaign,
		},
		Metadata: map[string]interface{}{
			"demo_generated": true,
			"campaign_type":  "newsletter",
		},
	}

	// Base message with sent status
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
	demoBounceOrDeliver(&engagement, now, sentTime, demoNewsletterBounceRate)
	if engagement.shouldBounce {
		return message, engagement
	}
	deliveredTime := engagement.deliveredTime

	// A few recipients mark the newsletter as spam once delivered.
	if rand.Float64() < demoNewsletterComplaintRate {
		engagement.shouldComplain = true
		engagement.complainedTime = demoEventAfter(now, deliveredTime, 48*time.Hour) // Within 48 hours
	}

	// 60% open rate
	if rand.Float64() < 0.60 {
		openedTime := demoEventAfter(now, deliveredTime, 7*24*time.Hour) // Within 7 days
		engagement.shouldOpen = true
		engagement.openedTime = openedTime

		// 20% click rate (of all messages, so 20/60 = 33.33% of opened messages)
		if rand.Float64() < 0.20/0.60 {
			clickedTime := demoEventAfter(now, openedTime, time.Hour) // Within 1 hour of opening
			engagement.shouldClick = true
			engagement.clickedTime = clickedTime
		}

		// 1% unsubscribed (of all messages, not just opened)
		if rand.Float64() < 0.01/0.60 {
			unsubscribeTime := demoEventAfter(now, openedTime, 30*time.Minute) // Within 30 minutes of opening
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
