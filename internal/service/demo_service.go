package service

import (
	"context"
	"fmt"
	"math/rand"
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
	webhookSubscriptionService       *WebhookSubscriptionService
}

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
	webhookSubscriptionService *WebhookSubscriptionService,
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
		webhookSubscriptionService:       webhookSubscriptionService,
	}
}

// VerifyRootEmailHMAC verifies the HMAC of the root email
func (s *DemoService) VerifyRootEmailHMAC(providedHMAC string) bool {
	if s.config.RootEmail == "" {
		s.logger.Error("Root email not configured")
		return false
	}

	// Use the domain function to verify HMAC with constant-time comparison
	return domain.VerifyEmailHMAC(s.config.RootEmail, providedHMAC, s.config.Security.SecretKey)
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

// createDemoWorkspace creates a new demo workspace with sample data
func (s *DemoService) createDemoWorkspace(ctx context.Context) error {
	s.logger.Info("Creating demo workspace")

	// Get the root user to create the workspace
	s.logger.WithField("root_email", s.config.RootEmail).Info("Looking up root user for demo workspace creation")

	rootUser, err := s.userService.GetUserByEmail(ctx, s.config.RootEmail)
	if err != nil {
		s.logger.WithField("root_email", s.config.RootEmail).WithField("error", err.Error()).Error("Failed to get root user")
		return fmt.Errorf("failed to get root user with email '%s': %w", s.config.RootEmail, err)
	}

	s.logger.WithField("root_user_id", rootUser.ID).WithField("root_user_type", rootUser.Type).Info("Found root user for demo workspace creation")

	// Create authenticated context with root user
	// For UserTypeUser, we need to create a temporary session or use API key approach
	authenticatedCtx := context.WithValue(ctx, domain.UserIDKey, rootUser.ID)
	if rootUser.Type == domain.UserTypeUser {
		// For demo purposes, treat root user as API key to avoid session complexity
		authenticatedCtx = context.WithValue(authenticatedCtx, domain.UserTypeKey, string(domain.UserTypeAPIKey))
	} else {
		authenticatedCtx = context.WithValue(authenticatedCtx, domain.UserTypeKey, string(rootUser.Type))
	}

	// Use hardcoded demo workspace ID
	workspaceID := "demo"

	// Create workspace settings with readonly demo bucket
	fileManagerSettings := domain.FileManagerSettings{
		Endpoint:  s.config.Demo.FileManagerEndpoint,
		Bucket:    s.config.Demo.FileManagerBucket,
		AccessKey: s.config.Demo.FileManagerAccessKey,
		SecretKey: s.config.Demo.FileManagerSecretKey,
	}

	// Create the demo workspace
	workspace, err := s.workspaceService.CreateWorkspace(
		authenticatedCtx,
		workspaceID,
		"Demo Workspace",
		"https://demo.notifuse.com",
		"https://www.notifuse.com/apple-touch-icon.png",
		"https://demo.notifuse.com/cover.png",
		"UTC",
		fileManagerSettings,
	)
	if err != nil {
		return fmt.Errorf("failed to create demo workspace: %w", err)
	}

	s.logger.WithField("workspace_id", workspace.ID).Info("Demo workspace created successfully")

	// Create webhook subscription to generate webhook deliveries for demo data
	// Must be created BEFORE sample data so DB triggers can fire
	_, err = s.webhookSubscriptionService.Create(
		authenticatedCtx,
		workspace.ID,
		"Demo Webhook",
		"https://webhook.site/demo",
		domain.WebhookEventTypes, // Subscribe to all event types
		nil,
	)
	if err != nil {
		s.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Warn("Failed to create demo webhook subscription")
		// Non-fatal - continue with demo setup
	} else {
		s.logger.WithField("workspace_id", workspace.ID).Info("Demo webhook subscription created")
	}

	// Create SMTP integration for demo emails
	if err := s.createDemoSMTPIntegration(authenticatedCtx, workspace.ID); err != nil {
		s.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Warn("Failed to create SMTP integration")
		// Don't fail the entire operation if SMTP integration creation fails
	}

	// Add comprehensive sample data to the workspace
	if err := s.addSampleData(authenticatedCtx, workspace.ID); err != nil {
		s.logger.WithField("workspace_id", workspace.ID).WithField("error", err.Error()).Warn("Failed to add sample data to demo workspace")
		// Don't fail the entire operation if sample data creation fails
	}

	return nil
}

// addSampleData adds comprehensive sample data including 1000 contacts, templates, and broadcasts
func (s *DemoService) addSampleData(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Adding comprehensive sample data to demo workspace")

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

	// Step 3b: Generate sample custom events to simulate e-commerce transactions
	if err := s.generateSampleCustomEvents(ctx, workspaceID); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create sample custom events")
		// Don't fail the entire operation if custom events creation fails
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

	// Step 8: Create sample segments
	if err := s.createSampleSegments(ctx, workspaceID); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create sample segments")
		return err
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

		// Add some randomness to creation times (spread over last 6 months)
		createdAt := time.Now().AddDate(0, -6, 0).Add(time.Duration(rand.Intn(180*24)) * time.Hour)

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

// generateSampleCustomEvents creates sample custom events to simulate e-commerce transactions
func (s *DemoService) generateSampleCustomEvents(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Generating sample custom events for e-commerce simulation")

	// Get all contacts to create events for
	contactsReq := &domain.GetContactsRequest{
		WorkspaceID: workspaceID,
		Limit:       1000,
	}

	contactsResp, err := s.contactService.GetContacts(ctx, contactsReq)
	if err != nil {
		return fmt.Errorf("failed to get contacts for custom events: %w", err)
	}

	if len(contactsResp.Contacts) == 0 {
		s.logger.WithField("workspace_id", workspaceID).Info("No contacts found, skipping custom events generation")
		return nil
	}

	// Sample product names for realistic demo data
	productNames := []string{
		"Premium Subscription", "Basic Plan", "Pro License", "Enterprise Package",
		"Widget Pack", "Service Credit", "Annual Membership", "Monthly Bundle",
		"Starter Kit", "Professional Tools", "Digital Download", "Consulting Hour",
	}

	totalEvents := 0

	// 70% of contacts have purchase history
	for _, contact := range contactsResp.Contacts {
		if rand.Float32() >= 0.7 {
			continue // Skip 30% of contacts
		}

		// Generate 1-5 purchase events per contact
		numPurchases := 1 + rand.Intn(5)
		contactCreatedAt := contact.CreatedAt

		for j := 0; j < numPurchases; j++ {
			// Spread purchases over time after contact creation
			daysAfterCreation := rand.Intn(180) // Up to 6 months after signup
			purchaseTime := contactCreatedAt.Add(time.Duration(daysAfterCreation*24) * time.Hour)

			// Don't create events in the future
			if purchaseTime.After(time.Now()) {
				purchaseTime = time.Now().Add(-time.Duration(rand.Intn(30*24)) * time.Hour)
			}

			// Generate purchase value (between $10 and $500)
			purchaseValue := 10.0 + rand.Float64()*490.0
			purchaseValue = float64(int(purchaseValue*100)) / 100 // Round to 2 decimal places

			goalType := "purchase"
			goalName := "E-commerce Purchase"

			customEvent := &domain.CustomEvent{
				ExternalID: fmt.Sprintf("demo_purchase_%s_%d_%d", contact.Email, j, purchaseTime.Unix()),
				Email:      contact.Email,
				EventName:  "purchase",
				Properties: map[string]interface{}{
					"product_name": getRandomElement(productNames),
					"quantity":     1 + rand.Intn(3),
					"currency":     "USD",
					"order_id":     fmt.Sprintf("ORD-%d", 10000+rand.Intn(90000)),
				},
				OccurredAt: purchaseTime,
				Source:     "demo",
				GoalName:   &goalName,
				GoalType:   &goalType,
				GoalValue:  &purchaseValue,
				CreatedAt:  purchaseTime,
				UpdatedAt:  purchaseTime,
			}

			if err := s.customEventRepo.Upsert(ctx, workspaceID, customEvent); err != nil {
				s.logger.WithField("email", contact.Email).WithField("error", err.Error()).Debug("Failed to create custom event")
				continue
			}

			totalEvents++
		}
	}

	s.logger.WithField("workspace_id", workspaceID).WithField("total_events", totalEvents).Info("Sample custom events generation completed")
	return nil
}

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
func (s *DemoService) createSampleTemplates(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating sample templates")

	// Create newsletter template
	newsletterMJML := s.createNewsletterMJMLStructure()
	newsletterTestData := domain.MapOfAny{
		"contact": domain.MapOfAny{
			"first_name": "John",
			"last_name":  "Doe",
			"email":      "john.doe@example.com",
		},
	}

	// Compile MJML to HTML
	newsletterHTML := s.compileTemplateToHTML(workspaceID, "newsletter-preview", newsletterMJML, newsletterTestData)

	newsletterTemplate := &domain.Template{
		ID:       "newsletter-weekly",
		Name:     "Weekly Newsletter",
		Version:  1,
		Channel:  "email",
		Category: string(domain.TemplateCategoryMarketing),
		Email: &domain.EmailTemplate{
			Subject:          "{{contact.first_name}}, Your Weekly Update is Here! 📧",
			CompiledPreview:  newsletterHTML,
			VisualEditorTree: newsletterMJML,
		},
		TestData:  newsletterTestData,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.templateService.CreateTemplate(ctx, workspaceID, newsletterTemplate); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create newsletter template")
	}

	// Create newsletter template v2
	newsletterV2MJML := s.createNewsletterV2MJMLStructure()
	newsletterV2TestData := domain.MapOfAny{
		"contact": domain.MapOfAny{
			"first_name": "Sarah",
			"last_name":  "Wilson",
			"email":      "sarah.wilson@example.com",
		},
	}

	// Compile MJML to HTML
	newsletterV2HTML := s.compileTemplateToHTML(workspaceID, "newsletter-v2-preview", newsletterV2MJML, newsletterV2TestData)

	newsletterV2Template := &domain.Template{
		ID:       "newsletter-weekly-v2",
		Name:     "Weekly Newsletter v2",
		Version:  1,
		Channel:  "email",
		Category: string(domain.TemplateCategoryMarketing),
		Email: &domain.EmailTemplate{
			Subject:          "🚀 {{contact.first_name}}, This Week's Top Stories & Updates!",
			CompiledPreview:  newsletterV2HTML,
			VisualEditorTree: newsletterV2MJML,
		},
		TestData:  newsletterV2TestData,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.templateService.CreateTemplate(ctx, workspaceID, newsletterV2Template); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create newsletter v2 template")
	}

	// Create welcome email template
	welcomeMJML := s.createWelcomeMJMLStructure()
	welcomeTestData := domain.MapOfAny{
		"contact": domain.MapOfAny{
			"first_name": "Jane",
			"last_name":  "Smith",
			"email":      "jane.smith@example.com",
		},
	}

	// Compile MJML to HTML
	welcomeHTML := s.compileTemplateToHTML(workspaceID, "welcome-preview", welcomeMJML, welcomeTestData)

	welcomeTemplate := &domain.Template{
		ID:       "welcome-email",
		Name:     "Welcome Email",
		Version:  1,
		Channel:  "email",
		Category: string(domain.TemplateCategoryWelcome),
		Email: &domain.EmailTemplate{
			Subject:          "Welcome to our community, {{contact.first_name}}! 🎉",
			CompiledPreview:  welcomeHTML,
			VisualEditorTree: welcomeMJML,
		},
		TestData:  welcomeTestData,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.templateService.CreateTemplate(ctx, workspaceID, welcomeTemplate); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create welcome template")
	}

	// Create password reset template
	passwordResetMJML := s.createPasswordResetMJMLStructure()
	passwordResetTestData := domain.MapOfAny{
		"contact": domain.MapOfAny{
			"first_name": "Alex",
			"last_name":  "Johnson",
			"email":      "alex.johnson@example.com",
		},
		"reset_url": "https://demo.notifuse.com/reset-password?token=demo_token_123",
	}

	// Compile MJML to HTML
	passwordResetHTML := s.compileTemplateToHTML(workspaceID, "password-reset-preview", passwordResetMJML, passwordResetTestData)

	passwordResetTemplate := &domain.Template{
		ID:       "password-reset",
		Name:     "Password Reset",
		Version:  1,
		Channel:  "email",
		Category: string(domain.TemplateCategoryTransactional),
		Email: &domain.EmailTemplate{
			Subject:          "Reset your password, {{contact.first_name}}",
			CompiledPreview:  passwordResetHTML,
			VisualEditorTree: passwordResetMJML,
		},
		TestData:  passwordResetTestData,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.templateService.CreateTemplate(ctx, workspaceID, passwordResetTemplate); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create password reset template")
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

// createNewsletterMJMLStructure creates the MJML structure for the newsletter template
func (s *DemoService) createNewsletterMJMLStructure() notifuse_mjml.EmailBlock {
	// Create the text content block
	textContent := "Hi {{contact.first_name}},<br><br>Welcome to this week's newsletter! Here are the latest updates and insights we thought you'd find interesting."
	highlightsContent := "📈 This Week's Highlights"
	listContent := "• New feature releases and improvements<br>• Industry insights and trends<br>• Community highlights and success stories"
	buttonContent := "Read Full Newsletter"
	titleContent := "Weekly Newsletter"
	previewContent := "Your weekly dose of updates and insights"

	// Create header text block
	headerTextBase := notifuse_mjml.NewBaseBlock("header-text", notifuse_mjml.MJMLComponentMjText)
	headerTextBase.Content = &titleContent
	headerText := &notifuse_mjml.MJTextBlock{BaseBlock: headerTextBase}

	// Create main text block
	mainTextBase := notifuse_mjml.NewBaseBlock("main-text", notifuse_mjml.MJMLComponentMjText)
	mainTextBase.Content = &textContent
	mainText := &notifuse_mjml.MJTextBlock{BaseBlock: mainTextBase}

	// Create divider
	divider := &notifuse_mjml.MJDividerBlock{
		BaseBlock: notifuse_mjml.NewBaseBlock("divider", notifuse_mjml.MJMLComponentMjDivider),
	}

	// Create highlights title
	highlightsTextBase := notifuse_mjml.NewBaseBlock("highlights-title", notifuse_mjml.MJMLComponentMjText)
	highlightsTextBase.Content = &highlightsContent
	highlightsText := &notifuse_mjml.MJTextBlock{BaseBlock: highlightsTextBase}

	// Create highlights list
	highlightsListBase := notifuse_mjml.NewBaseBlock("highlights-list", notifuse_mjml.MJMLComponentMjText)
	highlightsListBase.Content = &listContent
	highlightsList := &notifuse_mjml.MJTextBlock{BaseBlock: highlightsListBase}

	// Create button
	buttonBase := notifuse_mjml.NewBaseBlock("cta-button", notifuse_mjml.MJMLComponentMjButton)
	buttonBase.Attributes["background-color"] = "#3498db"
	buttonBase.Attributes["color"] = "#ffffff"
	buttonBase.Attributes["font-size"] = "16px"
	buttonBase.Attributes["padding"] = "12px 24px"
	buttonBase.Attributes["border-radius"] = "6px"
	buttonBase.Attributes["href"] = "https://demo.notifuse.com/newsletter?utm_source={{utm_source}}&utm_medium={{utm_medium}}&utm_campaign={{utm_campaign}}"
	buttonBase.Content = &buttonContent
	button := &notifuse_mjml.MJButtonBlock{BaseBlock: buttonBase}

	// Create title and preview blocks
	titleBase := notifuse_mjml.NewBaseBlock("title", notifuse_mjml.MJMLComponentMjTitle)
	titleBase.Content = &titleContent
	title := &notifuse_mjml.MJTitleBlock{BaseBlock: titleBase}

	previewBase := notifuse_mjml.NewBaseBlock("preview", notifuse_mjml.MJMLComponentMjPreview)
	previewBase.Content = &previewContent
	preview := &notifuse_mjml.MJPreviewBlock{BaseBlock: previewBase}

	// Create footer text
	footerContent := "You received this email because you're subscribed to our newsletter.<br><a href=\"{{unsubscribe_url}}\">Unsubscribe</a> | <a href=\"https://demo.notifuse.com\">Visit our website</a>"
	footerTextBase := notifuse_mjml.NewBaseBlock("footer-text", notifuse_mjml.MJMLComponentMjText)
	footerTextBase.Content = &footerContent
	footerText := &notifuse_mjml.MJTextBlock{BaseBlock: footerTextBase}

	// Create columns for layout
	headerColumnBase := notifuse_mjml.NewBaseBlock("header-column", notifuse_mjml.MJMLComponentMjColumn)
	headerColumnBase.Children = []notifuse_mjml.EmailBlock{headerText}
	headerColumn := &notifuse_mjml.MJColumnBlock{BaseBlock: headerColumnBase}

	contentColumnBase := notifuse_mjml.NewBaseBlock("content-column", notifuse_mjml.MJMLComponentMjColumn)
	contentColumnBase.Children = []notifuse_mjml.EmailBlock{mainText, divider, highlightsText, highlightsList, button}
	contentColumn := &notifuse_mjml.MJColumnBlock{BaseBlock: contentColumnBase}

	footerColumnBase := notifuse_mjml.NewBaseBlock("footer-column", notifuse_mjml.MJMLComponentMjColumn)
	footerColumnBase.Children = []notifuse_mjml.EmailBlock{footerText}
	footerColumn := &notifuse_mjml.MJColumnBlock{BaseBlock: footerColumnBase}

	// Create sections
	headerSectionBase := notifuse_mjml.NewBaseBlock("header-section", notifuse_mjml.MJMLComponentMjSection)
	headerSectionBase.Children = []notifuse_mjml.EmailBlock{headerColumn}
	headerSection := &notifuse_mjml.MJSectionBlock{BaseBlock: headerSectionBase}

	contentSectionBase := notifuse_mjml.NewBaseBlock("content-section", notifuse_mjml.MJMLComponentMjSection)
	contentSectionBase.Children = []notifuse_mjml.EmailBlock{contentColumn}
	contentSection := &notifuse_mjml.MJSectionBlock{BaseBlock: contentSectionBase}

	footerSectionBase := notifuse_mjml.NewBaseBlock("footer-section", notifuse_mjml.MJMLComponentMjSection)
	footerSectionBase.Children = []notifuse_mjml.EmailBlock{footerColumn}
	footerSection := &notifuse_mjml.MJSectionBlock{BaseBlock: footerSectionBase}

	// Create head and body
	headBase := notifuse_mjml.NewBaseBlock("head", notifuse_mjml.MJMLComponentMjHead)
	headBase.Children = []notifuse_mjml.EmailBlock{title, preview}
	head := &notifuse_mjml.MJHeadBlock{BaseBlock: headBase}

	bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
	bodyBase.Children = []notifuse_mjml.EmailBlock{headerSection, contentSection, footerSection}
	body := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}

	// Create root MJML block
	rootBase := notifuse_mjml.NewBaseBlock("mjml-root", notifuse_mjml.MJMLComponentMjml)
	rootBase.Attributes["lang"] = "en"
	rootBase.Children = []notifuse_mjml.EmailBlock{head, body}
	return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
}

// createNewsletterV2MJMLStructure creates the MJML structure for the newsletter v2 template (modern card-based design)
func (s *DemoService) createNewsletterV2MJMLStructure() notifuse_mjml.EmailBlock {
	// Create the text content blocks with different styling and content
	titleContent := "Weekly Digest"
	previewContent := "Your personalized weekly roundup of insights and updates"
	heroContent := "Stay Ahead of the Curve 📈"
	introContent := "Hey {{contact.first_name}},<br><br>Here's your curated weekly digest packed with the latest trends, insights, and updates tailored just for you."

	// Feature stories content
	feature1Title := "🎯 Featured Story"
	feature1Content := "Breaking: New industry standards are reshaping how we approach digital transformation. Here's what you need to know."

	feature2Title := "💡 Quick Tips"
	feature2Content := "5 productivity hacks that successful professionals swear by. Simple changes, big impact."

	feature3Title := "🔥 Trending Now"
	feature3Content := "The tools and strategies everyone's talking about this week. Don't miss out on the conversation."

	buttonContent := "Explore More"

	// Create title and preview blocks
	titleBase := notifuse_mjml.NewBaseBlock("title", notifuse_mjml.MJMLComponentMjTitle)
	titleBase.Content = &titleContent
	title := &notifuse_mjml.MJTitleBlock{BaseBlock: titleBase}

	previewBase := notifuse_mjml.NewBaseBlock("preview", notifuse_mjml.MJMLComponentMjPreview)
	previewBase.Content = &previewContent
	preview := &notifuse_mjml.MJPreviewBlock{BaseBlock: previewBase}

	// Create hero section
	heroTextBase := notifuse_mjml.NewBaseBlock("hero-text", notifuse_mjml.MJMLComponentMjText)
	heroTextBase.Content = &heroContent
	heroText := &notifuse_mjml.MJTextBlock{BaseBlock: heroTextBase}

	introTextBase := notifuse_mjml.NewBaseBlock("intro-text", notifuse_mjml.MJMLComponentMjText)
	introTextBase.Content = &introContent
	introText := &notifuse_mjml.MJTextBlock{BaseBlock: introTextBase}

	// Create feature cards
	feature1TitleTextBase := notifuse_mjml.NewBaseBlock("feature1-title", notifuse_mjml.MJMLComponentMjText)
	feature1TitleTextBase.Content = &feature1Title
	feature1TitleText := &notifuse_mjml.MJTextBlock{BaseBlock: feature1TitleTextBase}

	feature1ContentTextBase := notifuse_mjml.NewBaseBlock("feature1-content", notifuse_mjml.MJMLComponentMjText)
	feature1ContentTextBase.Content = &feature1Content
	feature1ContentText := &notifuse_mjml.MJTextBlock{BaseBlock: feature1ContentTextBase}

	feature2TitleTextBase := notifuse_mjml.NewBaseBlock("feature2-title", notifuse_mjml.MJMLComponentMjText)
	feature2TitleTextBase.Content = &feature2Title
	feature2TitleText := &notifuse_mjml.MJTextBlock{BaseBlock: feature2TitleTextBase}

	feature2ContentTextBase := notifuse_mjml.NewBaseBlock("feature2-content", notifuse_mjml.MJMLComponentMjText)
	feature2ContentTextBase.Content = &feature2Content
	feature2ContentText := &notifuse_mjml.MJTextBlock{BaseBlock: feature2ContentTextBase}

	feature3TitleTextBase := notifuse_mjml.NewBaseBlock("feature3-title", notifuse_mjml.MJMLComponentMjText)
	feature3TitleTextBase.Content = &feature3Title
	feature3TitleText := &notifuse_mjml.MJTextBlock{BaseBlock: feature3TitleTextBase}

	feature3ContentTextBase := notifuse_mjml.NewBaseBlock("feature3-content", notifuse_mjml.MJMLComponentMjText)
	feature3ContentTextBase.Content = &feature3Content
	feature3ContentText := &notifuse_mjml.MJTextBlock{BaseBlock: feature3ContentTextBase}

	// Create CTA button
	buttonBase2 := notifuse_mjml.NewBaseBlock("cta-button", notifuse_mjml.MJMLComponentMjButton)
	buttonBase2.Attributes["background-color"] = "#667eea"
	buttonBase2.Attributes["color"] = "#ffffff"
	buttonBase2.Attributes["font-size"] = "16px"
	buttonBase2.Attributes["font-weight"] = "bold"
	buttonBase2.Attributes["padding"] = "15px 30px"
	buttonBase2.Attributes["border-radius"] = "8px"
	buttonBase2.Attributes["href"] = "https://demo.notifuse.com/weekly-digest?utm_source={{utm_source}}&utm_medium={{utm_medium}}&utm_campaign={{utm_campaign}}"
	buttonBase2.Content = &buttonContent
	button := &notifuse_mjml.MJButtonBlock{BaseBlock: buttonBase2}

	// Create footer
	footerContent := "You're receiving this because you subscribed to our weekly digest.<br><a href=\"{{unsubscribe_url}}\">Unsubscribe</a> | <a href=\"https://demo.notifuse.com/preferences\">Manage Preferences</a>"
	footerTextBase := notifuse_mjml.NewBaseBlock("footer-text", notifuse_mjml.MJMLComponentMjText)
	footerTextBase.Content = &footerContent
	footerText := &notifuse_mjml.MJTextBlock{BaseBlock: footerTextBase}

	// Create columns and sections
	heroColumnBase := notifuse_mjml.NewBaseBlock("hero-column", notifuse_mjml.MJMLComponentMjColumn)
	heroColumnBase.Children = []notifuse_mjml.EmailBlock{heroText, introText}
	heroColumn := &notifuse_mjml.MJColumnBlock{BaseBlock: heroColumnBase}

	// Create feature columns (side by side layout)
	feature1ColumnBase := notifuse_mjml.NewBaseBlock("feature1-column", notifuse_mjml.MJMLComponentMjColumn)
	feature1ColumnBase.Children = []notifuse_mjml.EmailBlock{feature1TitleText, feature1ContentText}
	feature1Column := &notifuse_mjml.MJColumnBlock{BaseBlock: feature1ColumnBase}

	feature2ColumnBase := notifuse_mjml.NewBaseBlock("feature2-column", notifuse_mjml.MJMLComponentMjColumn)
	feature2ColumnBase.Children = []notifuse_mjml.EmailBlock{feature2TitleText, feature2ContentText}
	feature2Column := &notifuse_mjml.MJColumnBlock{BaseBlock: feature2ColumnBase}

	feature3ColumnBase := notifuse_mjml.NewBaseBlock("feature3-column", notifuse_mjml.MJMLComponentMjColumn)
	feature3ColumnBase.Children = []notifuse_mjml.EmailBlock{feature3TitleText, feature3ContentText}
	feature3Column := &notifuse_mjml.MJColumnBlock{BaseBlock: feature3ColumnBase}

	ctaColumnBase := notifuse_mjml.NewBaseBlock("cta-column", notifuse_mjml.MJMLComponentMjColumn)
	ctaColumnBase.Children = []notifuse_mjml.EmailBlock{button}
	ctaColumn := &notifuse_mjml.MJColumnBlock{BaseBlock: ctaColumnBase}

	footerColumnBase := notifuse_mjml.NewBaseBlock("footer-column", notifuse_mjml.MJMLComponentMjColumn)
	footerColumnBase.Children = []notifuse_mjml.EmailBlock{footerText}
	footerColumn := &notifuse_mjml.MJColumnBlock{BaseBlock: footerColumnBase}

	// Create sections
	heroSectionBase := notifuse_mjml.NewBaseBlock("hero-section", notifuse_mjml.MJMLComponentMjSection)
	heroSectionBase.Children = []notifuse_mjml.EmailBlock{heroColumn}
	heroSection := &notifuse_mjml.MJSectionBlock{BaseBlock: heroSectionBase}

	featuresSectionBase := notifuse_mjml.NewBaseBlock("features-section", notifuse_mjml.MJMLComponentMjSection)
	featuresSectionBase.Children = []notifuse_mjml.EmailBlock{feature1Column, feature2Column}
	featuresSection := &notifuse_mjml.MJSectionBlock{BaseBlock: featuresSectionBase}

	feature3SectionBase := notifuse_mjml.NewBaseBlock("feature3-section", notifuse_mjml.MJMLComponentMjSection)
	feature3SectionBase.Children = []notifuse_mjml.EmailBlock{feature3Column}
	feature3Section := &notifuse_mjml.MJSectionBlock{BaseBlock: feature3SectionBase}

	ctaSectionBase := notifuse_mjml.NewBaseBlock("cta-section", notifuse_mjml.MJMLComponentMjSection)
	ctaSectionBase.Children = []notifuse_mjml.EmailBlock{ctaColumn}
	ctaSection := &notifuse_mjml.MJSectionBlock{BaseBlock: ctaSectionBase}

	footerSectionBase := notifuse_mjml.NewBaseBlock("footer-section", notifuse_mjml.MJMLComponentMjSection)
	footerSectionBase.Children = []notifuse_mjml.EmailBlock{footerColumn}
	footerSection := &notifuse_mjml.MJSectionBlock{BaseBlock: footerSectionBase}

	// Create head and body
	headBase := notifuse_mjml.NewBaseBlock("head", notifuse_mjml.MJMLComponentMjHead)
	headBase.Children = []notifuse_mjml.EmailBlock{title, preview}
	head := &notifuse_mjml.MJHeadBlock{BaseBlock: headBase}

	bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
	bodyBase.Children = []notifuse_mjml.EmailBlock{heroSection, featuresSection, feature3Section, ctaSection, footerSection}
	body := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}

	// Create root MJML block
	rootBase := notifuse_mjml.NewBaseBlock("mjml-root", notifuse_mjml.MJMLComponentMjml)
	rootBase.Attributes["lang"] = "en"
	rootBase.Children = []notifuse_mjml.EmailBlock{head, body}
	return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
}

// createWelcomeMJMLStructure creates the MJML structure for the welcome template
func (s *DemoService) createWelcomeMJMLStructure() notifuse_mjml.EmailBlock {
	// Create content strings
	titleContent := "Welcome to our community!"
	previewContent := "Thank you for joining us, {{contact.first_name}}!"
	welcomeContent := "Welcome, {{contact.first_name}}! 🎉"
	mainContent := "Thank you for joining our community! We're excited to have you on board and can't wait to share amazing content with you."
	buttonContent := "Get Started"
	footerContent := "If you have any questions, feel free to reach out to our support team.<br><br>Best regards,<br>The Demo Team"

	// Create blocks using concrete types
	titleBase := notifuse_mjml.NewBaseBlock("title", notifuse_mjml.MJMLComponentMjTitle)
	titleBase.Content = &titleContent
	title := &notifuse_mjml.MJTitleBlock{BaseBlock: titleBase}

	previewBase := notifuse_mjml.NewBaseBlock("preview", notifuse_mjml.MJMLComponentMjPreview)
	previewBase.Content = &previewContent
	preview := &notifuse_mjml.MJPreviewBlock{BaseBlock: previewBase}

	welcomeTextBase := notifuse_mjml.NewBaseBlock("welcome-text", notifuse_mjml.MJMLComponentMjText)
	welcomeTextBase.Content = &welcomeContent
	welcomeText := &notifuse_mjml.MJTextBlock{BaseBlock: welcomeTextBase}

	mainTextBase := notifuse_mjml.NewBaseBlock("main-text", notifuse_mjml.MJMLComponentMjText)
	mainTextBase.Content = &mainContent
	mainText := &notifuse_mjml.MJTextBlock{BaseBlock: mainTextBase}

	buttonBase3 := notifuse_mjml.NewBaseBlock("get-started-button", notifuse_mjml.MJMLComponentMjButton)
	buttonBase3.Attributes["background-color"] = "#27ae60"
	buttonBase3.Attributes["color"] = "#ffffff"
	buttonBase3.Attributes["font-size"] = "16px"
	buttonBase3.Attributes["padding"] = "12px 24px"
	buttonBase3.Attributes["border-radius"] = "6px"
	buttonBase3.Attributes["href"] = "https://demo.notifuse.com/getting-started?utm_source={{utm_source}}&utm_medium={{utm_medium}}&utm_campaign={{utm_campaign}}"
	buttonBase3.Content = &buttonContent
	button := &notifuse_mjml.MJButtonBlock{BaseBlock: buttonBase3}

	divider := &notifuse_mjml.MJDividerBlock{
		BaseBlock: notifuse_mjml.NewBaseBlock("divider", notifuse_mjml.MJMLComponentMjDivider),
	}

	footerTextBase := notifuse_mjml.NewBaseBlock("footer-text", notifuse_mjml.MJMLComponentMjText)
	footerTextBase.Content = &footerContent
	footerText := &notifuse_mjml.MJTextBlock{BaseBlock: footerTextBase}

	columnBase := notifuse_mjml.NewBaseBlock("main-column", notifuse_mjml.MJMLComponentMjColumn)
	columnBase.Children = []notifuse_mjml.EmailBlock{welcomeText, mainText, button, divider, footerText}
	column := &notifuse_mjml.MJColumnBlock{BaseBlock: columnBase}

	sectionBase := notifuse_mjml.NewBaseBlock("main-section", notifuse_mjml.MJMLComponentMjSection)
	sectionBase.Children = []notifuse_mjml.EmailBlock{column}
	section := &notifuse_mjml.MJSectionBlock{BaseBlock: sectionBase}

	headBase := notifuse_mjml.NewBaseBlock("head", notifuse_mjml.MJMLComponentMjHead)
	headBase.Children = []notifuse_mjml.EmailBlock{title, preview}
	head := &notifuse_mjml.MJHeadBlock{BaseBlock: headBase}

	bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
	bodyBase.Children = []notifuse_mjml.EmailBlock{section}
	body := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}

	rootBase7 := notifuse_mjml.NewBaseBlock("mjml-root", notifuse_mjml.MJMLComponentMjml)
	rootBase7.Attributes["lang"] = "en"
	rootBase7.Children = []notifuse_mjml.EmailBlock{head, body}
	return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase7}
}

// createSampleBroadcasts creates multiple sample broadcast campaigns and returns their IDs
func (s *DemoService) createSampleBroadcasts(ctx context.Context, workspaceID string) ([]string, error) {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating sample broadcasts")

	var broadcastIDs []string

	// Create 4 newsletter broadcasts to simulate recent campaigns
	broadcasts := []struct {
		name     string
		campaign string
	}{
		{"Weekly Newsletter #1", "weekly_newsletter_1"},
		{"Weekly Newsletter #2", "weekly_newsletter_2"},
		{"Weekly Newsletter #3", "weekly_newsletter_3"},
		{"Weekly Newsletter #4 - A/B Test", "weekly_newsletter_4"},
	}

	for i, bc := range broadcasts {
		var variations []domain.BroadcastVariation

		// Last broadcast has A/B test enabled
		if i == len(broadcasts)-1 {
			variations = []domain.BroadcastVariation{
				{
					VariationName: "variation-a",
					TemplateID:    "newsletter-weekly",
				},
				{
					VariationName: "variation-b",
					TemplateID:    "newsletter-weekly-v2",
				},
			}
		} else {
			// Alternate between templates for other broadcasts
			templateID := "newsletter-weekly"
			if i%2 == 1 {
				templateID = "newsletter-weekly-v2"
			}
			variations = []domain.BroadcastVariation{
				{
					VariationName: "variation-a",
					TemplateID:    templateID,
				},
			}
		}

		broadcastReq := &domain.CreateBroadcastRequest{
			WorkspaceID: workspaceID,
			Name:        bc.name,
			Audience: domain.AudienceSettings{
				List:                "newsletter",
				Segments:            []string{},
				ExcludeUnsubscribed: true,
			},
			TestSettings: domain.BroadcastTestSettings{
				Enabled:          i == len(broadcasts)-1, // Only enable A/B test for last broadcast
				SamplePercentage: 10,
				AutoSendWinner:   false,
				Variations:       variations,
			},
			TrackingEnabled: true,
			UTMParameters: &domain.UTMParameters{
				Source:   "demo.notifuse.com",
				Medium:   "email",
				Campaign: bc.campaign,
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

		// Update the broadcast in the repository to reflect processed status
		if err := s.broadcastRepo.UpdateBroadcast(ctx, broadcast); err != nil {
			s.logger.WithField("broadcast_id", broadcast.ID).WithField("error", err.Error()).Warn("Failed to update broadcast status to processed")
			// Continue anyway - the broadcast was created, just not marked as processed
		}

		broadcastIDs = append(broadcastIDs, broadcast.ID)
		s.logger.WithField("broadcast_id", broadcast.ID).WithField("name", bc.name).WithField("status", "sent").Info("Sample broadcast created and marked as sent")
	}

	if len(broadcastIDs) == 0 {
		return nil, fmt.Errorf("failed to create any sample broadcasts")
	}

	s.logger.WithField("workspace_id", workspaceID).WithField("count", len(broadcastIDs)).Info("Sample broadcasts created successfully")
	return broadcastIDs, nil
}

// createPasswordResetMJMLStructure creates the MJML structure for the password reset template
func (s *DemoService) createPasswordResetMJMLStructure() notifuse_mjml.EmailBlock {
	// Create content strings
	titleContent := "Reset Your Password"
	previewContent := "You requested a password reset for your account"
	headerContent := "Reset Your Password 🔐"
	mainContent := "Hi {{contact.first_name}},<br><br>We received a request to reset the password for your account. If you made this request, click the button below to set a new password:"
	buttonContent := "Reset Password"
	expireContent := "This link will expire in 24 hours for security reasons."
	footerContent := "If you didn't request a password reset, you can safely ignore this email. Your password will remain unchanged.<br><br>If you're having trouble with the button above, copy and paste the URL below into your web browser:<br>{{reset_url}}"

	// Create blocks using concrete types
	titleBase := notifuse_mjml.NewBaseBlock("title", notifuse_mjml.MJMLComponentMjTitle)
	titleBase.Content = &titleContent
	title := &notifuse_mjml.MJTitleBlock{BaseBlock: titleBase}

	previewBase := notifuse_mjml.NewBaseBlock("preview", notifuse_mjml.MJMLComponentMjPreview)
	previewBase.Content = &previewContent
	preview := &notifuse_mjml.MJPreviewBlock{BaseBlock: previewBase}

	headerTextBase := notifuse_mjml.NewBaseBlock("header-text", notifuse_mjml.MJMLComponentMjText)
	headerTextBase.Content = &headerContent
	headerText := &notifuse_mjml.MJTextBlock{BaseBlock: headerTextBase}

	mainTextBase := notifuse_mjml.NewBaseBlock("main-text", notifuse_mjml.MJMLComponentMjText)
	mainTextBase.Content = &mainContent
	mainText := &notifuse_mjml.MJTextBlock{BaseBlock: mainTextBase}

	buttonBase4 := notifuse_mjml.NewBaseBlock("reset-button", notifuse_mjml.MJMLComponentMjButton)
	buttonBase4.Attributes["background-color"] = "#e74c3c"
	buttonBase4.Attributes["color"] = "#ffffff"
	buttonBase4.Attributes["font-size"] = "16px"
	buttonBase4.Attributes["font-weight"] = "bold"
	buttonBase4.Attributes["padding"] = "15px 30px"
	buttonBase4.Attributes["border-radius"] = "6px"
	buttonBase4.Attributes["href"] = "{{reset_url}}"
	buttonBase4.Content = &buttonContent
	button := &notifuse_mjml.MJButtonBlock{BaseBlock: buttonBase4}

	expireTextBase := notifuse_mjml.NewBaseBlock("expire-text", notifuse_mjml.MJMLComponentMjText)
	expireTextBase.Content = &expireContent
	expireText := &notifuse_mjml.MJTextBlock{BaseBlock: expireTextBase}

	divider := &notifuse_mjml.MJDividerBlock{
		BaseBlock: notifuse_mjml.NewBaseBlock("divider", notifuse_mjml.MJMLComponentMjDivider),
	}

	footerTextBase := notifuse_mjml.NewBaseBlock("footer-text", notifuse_mjml.MJMLComponentMjText)
	footerTextBase.Content = &footerContent
	footerText := &notifuse_mjml.MJTextBlock{BaseBlock: footerTextBase}

	columnBase := notifuse_mjml.NewBaseBlock("main-column", notifuse_mjml.MJMLComponentMjColumn)
	columnBase.Children = []notifuse_mjml.EmailBlock{headerText, mainText, button, expireText, divider, footerText}
	column := &notifuse_mjml.MJColumnBlock{BaseBlock: columnBase}

	sectionBase := notifuse_mjml.NewBaseBlock("main-section", notifuse_mjml.MJMLComponentMjSection)
	sectionBase.Children = []notifuse_mjml.EmailBlock{column}
	section := &notifuse_mjml.MJSectionBlock{BaseBlock: sectionBase}

	headBase := notifuse_mjml.NewBaseBlock("head", notifuse_mjml.MJMLComponentMjHead)
	headBase.Children = []notifuse_mjml.EmailBlock{title, preview}
	head := &notifuse_mjml.MJHeadBlock{BaseBlock: headBase}

	bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
	bodyBase.Children = []notifuse_mjml.EmailBlock{section}
	body := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}

	rootBase6 := notifuse_mjml.NewBaseBlock("mjml-root", notifuse_mjml.MJMLComponentMjml)
	rootBase6.Attributes["lang"] = "en"
	rootBase6.Children = []notifuse_mjml.EmailBlock{head, body}
	return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase6}
}

// createSampleTransactionalNotifications creates sample transactional notifications
func (s *DemoService) createSampleTransactionalNotifications(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating sample transactional notifications")

	// Create password reset transactional notification
	passwordResetNotification := domain.TransactionalNotificationCreateParams{
		ID:          "password_reset",
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
	shouldDeliver bool
	shouldOpen    bool
	shouldClick   bool
	deliveredTime time.Time
	openedTime    time.Time
	clickedTime   time.Time
}

// generateMessagesPerContact creates message history by assigning 2-4 emails to each contact
func (s *DemoService) generateMessagesPerContact(ctx context.Context, workspaceID string, secretKey string, contacts []*domain.Contact, broadcastIDs []string) (int, error) {
	s.logger.WithField("workspace_id", workspaceID).Info("Generating messages per contact")

	// Define available campaign/message templates over the last 10 days
	type campaignTemplate struct {
		templateID      string
		templateVersion int64
		broadcastID     *string // nil for transactional
		messageType     string  // "newsletter", "welcome", "password-reset"
		daysAgo         int
	}

	campaigns := []campaignTemplate{
		// Transactional messages
		{templateID: "welcome-email", templateVersion: 1, broadcastID: nil, messageType: "welcome", daysAgo: 2},
		{templateID: "welcome-email", templateVersion: 1, broadcastID: nil, messageType: "welcome", daysAgo: 5},
		{templateID: "password-reset", templateVersion: 1, broadcastID: nil, messageType: "password-reset", daysAgo: 3},
		{templateID: "password-reset", templateVersion: 1, broadcastID: nil, messageType: "password-reset", daysAgo: 8},
	}

	// Add newsletter campaigns using real broadcast IDs
	if len(broadcastIDs) >= 4 {
		campaigns = append(campaigns,
			campaignTemplate{templateID: "newsletter-weekly", templateVersion: 1, broadcastID: &broadcastIDs[0], messageType: "newsletter", daysAgo: 1},
			campaignTemplate{templateID: "newsletter-weekly-v2", templateVersion: 1, broadcastID: &broadcastIDs[1], messageType: "newsletter", daysAgo: 4},
			campaignTemplate{templateID: "newsletter-weekly", templateVersion: 1, broadcastID: &broadcastIDs[2], messageType: "newsletter", daysAgo: 7},
			campaignTemplate{templateID: "newsletter-weekly-v2", templateVersion: 1, broadcastID: &broadcastIDs[3], messageType: "newsletter", daysAgo: 10},
		)
	}

	totalMessages := 0
	batchSize := 50

	for i := 0; i < len(contacts); i += batchSize {
		end := i + batchSize
		if end > len(contacts) {
			end = len(contacts)
		}
		batch := contacts[i:end]

		// Collect engagement data for sequential processing
		var messagesWithEngagement []messageEngagementData

		for _, contact := range batch {
			// Each contact gets 2-4 emails
			numEmails := 2 + rand.Intn(3) // 2, 3, or 4 emails

			// Randomly select campaigns for this contact
			selectedCampaigns := make([]campaignTemplate, numEmails)
			selectedIndexes := rand.Perm(len(campaigns))[:numEmails]
			for j, idx := range selectedIndexes {
				selectedCampaigns[j] = campaigns[idx]
			}

			// Create message history for each selected campaign
			for _, campaign := range selectedCampaigns {
				campaignTime := time.Now().AddDate(0, 0, -campaign.daysAgo)

				var message *domain.MessageHistory
				var engagement messageEngagement
				if campaign.broadcastID != nil {
					// Newsletter/broadcast message
					message, engagement = s.generateMessageHistoryForContact(contact, campaign.templateID, campaign.templateVersion, *campaign.broadcastID, campaignTime)
				} else {
					// Transactional message
					message, engagement = s.generateTransactionalMessageHistoryForContact(contact, campaign.templateID, campaign.templateVersion, campaign.messageType, campaignTime)
				}

				if err := s.messageHistoryRepo.Create(ctx, workspaceID, secretKey, message); err != nil {
					s.logger.WithField("contact_email", contact.Email).WithField("error", err.Error()).Debug("Failed to create message history record")
					continue
				}

				messagesWithEngagement = append(messagesWithEngagement, messageEngagementData{
					message:    message,
					engagement: engagement,
				})

				totalMessages++
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

	s.logger.WithField("workspace_id", workspaceID).WithField("total_messages", totalMessages).Info("Messages per contact generation completed")
	return totalMessages, nil
}

// messageEngagementData holds a message and its engagement info
type messageEngagementData struct {
	message    *domain.MessageHistory
	engagement messageEngagement
}

// generateDeliveredInboundWebhookEventsForBatch creates inbound webhook events for delivered messages
func (s *DemoService) generateDeliveredInboundWebhookEventsForBatch(ctx context.Context, workspaceID string, messagesData []messageEngagementData) error {
	// Skip if workspace service or inbound webhook event repo is not available
	if s.workspaceService == nil || s.inboundWebhookEventRepo == nil {
		s.logger.Debug("Workspace service or inbound webhook event repo not available, skipping inbound webhook events")
		return nil
	}

	// Get the integration ID from the workspace
	workspace, err := s.workspaceService.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	integrationID := workspace.Settings.TransactionalEmailProviderID
	if integrationID == "" {
		s.logger.WithField("workspace_id", workspaceID).Debug("No transactional email provider configured, skipping inbound webhook events")
		return nil
	}

	// Collect inbound webhook events for delivered messages
	var inboundWebhookEvents []*domain.InboundWebhookEvent
	for _, data := range messagesData {
		if !data.engagement.shouldDeliver {
			continue
		}

		inboundWebhookEventID := uuid.New().String()
		rawPayload := fmt.Sprintf(`{"event":"delivered","message_id":"%s","recipient":"%s","timestamp":"%s"}`,
			data.message.ID, data.message.ContactEmail, data.engagement.deliveredTime.Format(time.RFC3339))

		inboundWebhookEvent := &domain.InboundWebhookEvent{
			ID:             inboundWebhookEventID,
			Type:           domain.EmailEventDelivered,
			Source:         domain.WebhookSourceSMTP,
			IntegrationID:  integrationID,
			RecipientEmail: data.message.ContactEmail,
			MessageID:      &data.message.ID,
			Timestamp:      data.engagement.deliveredTime,
			RawPayload:     rawPayload,
			CreatedAt:      data.engagement.deliveredTime,
		}

		inboundWebhookEvents = append(inboundWebhookEvents, inboundWebhookEvent)
	}

	// Store inbound webhook events
	if len(inboundWebhookEvents) > 0 {
		if err := s.inboundWebhookEventRepo.StoreEvents(ctx, workspaceID, inboundWebhookEvents); err != nil {
			return fmt.Errorf("failed to store inbound webhook events: %w", err)
		}
		s.logger.WithField("count", len(inboundWebhookEvents)).Debug("Generated delivered inbound webhook events")
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

// updateClickedMessagesForBatch updates message_history records with clicked_at timestamps
func (s *DemoService) updateClickedMessagesForBatch(ctx context.Context, workspaceID string, messagesData []messageEngagementData) error {
	for _, data := range messagesData {
		if !data.engagement.shouldClick {
			continue
		}

		// Use SetClicked to update the message (triggers timeline entry)
		// SetClicked also sets opened_at if not already set
		if err := s.messageHistoryRepo.SetClicked(ctx, workspaceID, data.message.ID, data.engagement.clickedTime); err != nil {
			s.logger.WithField("message_id", data.message.ID).WithField("error", err.Error()).Debug("Failed to set clicked status")
		}
	}

	return nil
}

// generateTransactionalMessageHistoryForContact creates a realistic transactional message history record for a contact
// Transactional messages have no broadcast ID and different engagement patterns
// Engagement rates: 100% delivered, 60% open rate, 20% click rate
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
		ID:              messageID,
		ContactEmail:    contact.Email,
		BroadcastID:     nil, // Transactional messages have no broadcast ID
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

	// 100% delivery rate - all messages delivered successfully
	deliveredTime := sentTime.Add(time.Duration(rand.Intn(1800)) * time.Second) // Within 30 minutes
	engagement.shouldDeliver = true
	engagement.deliveredTime = deliveredTime

	// 60% open rate
	if rand.Float64() < 0.60 {
		openedTime := deliveredTime.Add(time.Duration(rand.Intn(24*3600)) * time.Second) // Within 24 hours
		engagement.shouldOpen = true
		engagement.openedTime = openedTime

		// 20% click rate (of all messages, so 20/60 = 33.33% of opened messages)
		if rand.Float64() < 0.20/0.60 {
			clickedTime := openedTime.Add(time.Duration(rand.Intn(1800)) * time.Second) // Within 30 minutes of opening
			engagement.shouldClick = true
			engagement.clickedTime = clickedTime
		}

		// Very low unsubscribe rates for transactional messages
		if rand.Float64() < 0.001 {
			unsubscribeTime := openedTime.Add(time.Duration(rand.Intn(3600)) * time.Second) // Within 1 hour of opening
			message.UnsubscribedAt = &unsubscribeTime
		}
	}

	return message, engagement
}

// generateMessageHistoryForContact creates a realistic message history record for a contact
// with the specified engagement rates: 100% delivered, 60% open rate, 20% click rate
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
	message := &domain.MessageHistory{
		ID:              messageID,
		ContactEmail:    contact.Email,
		BroadcastID:     &broadcastID,
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

	// 100% delivery rate - all messages delivered successfully
	deliveredTime := sentTime.Add(time.Duration(rand.Intn(1800)) * time.Second) // Within 30 minutes
	engagement.shouldDeliver = true
	engagement.deliveredTime = deliveredTime

	// 60% open rate
	if rand.Float64() < 0.60 {
		openedTime := deliveredTime.Add(time.Duration(rand.Intn(7*24*3600)) * time.Second) // Within 7 days
		engagement.shouldOpen = true
		engagement.openedTime = openedTime

		// 20% click rate (of all messages, so 20/60 = 33.33% of opened messages)
		if rand.Float64() < 0.20/0.60 {
			clickedTime := openedTime.Add(time.Duration(rand.Intn(3600)) * time.Second) // Within 1 hour of opening
			engagement.shouldClick = true
			engagement.clickedTime = clickedTime
		}

		// 1% unsubscribed (of all messages, not just opened)
		if rand.Float64() < 0.01/0.60 {
			unsubscribeTime := openedTime.Add(time.Duration(rand.Intn(1800)) * time.Second) // Within 30 minutes of opening
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
								Kind:              "open_email",
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

	s.logger.WithField("workspace_id", workspaceID).Info("Sample segments created successfully")
	return nil
}
