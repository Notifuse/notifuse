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
	webhookEventService              *WebhookEventService
	webhookRegistrationService       *WebhookRegistrationService
	messageHistoryService            *MessageHistoryService
	notificationCenterService        *NotificationCenterService
	segmentService                   domain.SegmentService
	workspaceRepo                    domain.WorkspaceRepository
	taskRepo                         domain.TaskRepository
	messageHistoryRepo               domain.MessageHistoryRepository
	webhookEventRepo                 domain.WebhookEventRepository
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
	webhookEventService *WebhookEventService,
	webhookRegistrationService *WebhookRegistrationService,
	messageHistoryService *MessageHistoryService,
	notificationCenterService *NotificationCenterService,
	segmentService domain.SegmentService,
	workspaceRepo domain.WorkspaceRepository,
	taskRepo domain.TaskRepository,
	messageHistoryRepo domain.MessageHistoryRepository,
	webhookEventRepo domain.WebhookEventRepository,
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
		webhookEventService:              webhookEventService,
		webhookRegistrationService:       webhookRegistrationService,
		messageHistoryService:            messageHistoryService,
		notificationCenterService:        notificationCenterService,
		segmentService:                   segmentService,
		workspaceRepo:                    workspaceRepo,
		taskRepo:                         taskRepo,
		messageHistoryRepo:               messageHistoryRepo,
		webhookEventRepo:                 webhookEventRepo,
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
		"https://demo.notifuse.com/logo.png",
		"https://demo.notifuse.com/cover.png",
		"UTC",
		fileManagerSettings,
	)
	if err != nil {
		return fmt.Errorf("failed to create demo workspace: %w", err)
	}

	s.logger.WithField("workspace_id", workspace.ID).Info("Demo workspace created successfully")

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

	// Step 4: Subscribe all contacts to the newsletter list
	if err := s.subscribeContactsToList(ctx, workspaceID, "newsletter"); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to subscribe contacts to newsletter list")
		return err
	}

	// Step 5: Create sample broadcast campaign
	if err := s.createSampleBroadcast(ctx, workspaceID); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create sample broadcast")
		return err
	}

	// Step 6: Create sample transactional notifications
	if err := s.createSampleTransactionalNotifications(ctx, workspaceID); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create sample transactional notifications")
		return err
	}

	// Step 7: Generate sample message history with realistic engagement rates
	if err := s.generateSampleMessageHistory(ctx, workspaceID); err != nil {
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
	}

	// Create the integration
	integrationID, err := s.workspaceService.CreateIntegration(
		ctx,
		workspaceID,
		"Demo SMTP Integration",
		domain.IntegrationTypeEmail,
		smtpProvider,
	)
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

		// Add some custom fields for e-commerce demo data
		if rand.Float32() < 0.7 { // 70% of contacts have purchase history
			contact.LifetimeValue = &domain.NullableFloat64{Float64: rand.Float64() * 1000, IsNull: false}
			contact.OrdersCount = &domain.NullableFloat64{Float64: float64(rand.Intn(20)), IsNull: false}
			contact.LastOrderAt = &domain.NullableTime{Time: createdAt.Add(time.Duration(rand.Intn(30)) * 24 * time.Hour), IsNull: false}
		}

		contacts[i] = contact
	}

	return contacts
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
	headerText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "header-text",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "24px",
				"font-weight": "bold",
				"align":       "center",
				"color":       "#2c3e50",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &titleContent,
	}

	// Create main text block
	mainText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "main-text",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "16px",
				"line-height": "1.6",
				"color":       "#34495e",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &textContent,
	}

	// Create divider
	divider := &notifuse_mjml.MJDividerBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "divider",
			Type: notifuse_mjml.MJMLComponentMjDivider,
			Attributes: map[string]interface{}{
				"border-width": "1px",
				"border-color": "#ecf0f1",
			},
		},
		Type: notifuse_mjml.MJMLComponentMjDivider,
	}

	// Create highlights title
	highlightsText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "highlights-title",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "18px",
				"font-weight": "bold",
				"color":       "#2c3e50",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &highlightsContent,
	}

	// Create highlights list
	highlightsList := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "highlights-list",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "14px",
				"line-height": "1.6",
				"color":       "#34495e",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &listContent,
	}

	// Create button
	button := &notifuse_mjml.MJButtonBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "cta-button",
			Type: notifuse_mjml.MJMLComponentMjButton,
			Attributes: map[string]interface{}{
				"background-color": "#3498db",
				"color":            "#ffffff",
				"font-size":        "16px",
				"padding":          "12px 24px",
				"border-radius":    "6px",
				"href":             "https://demo.notifuse.com/newsletter?utm_source={{utm_source}}&utm_medium={{utm_medium}}&utm_campaign={{utm_campaign}}",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjButton,
		Content: &buttonContent,
	}

	// Create title and preview blocks
	title := &notifuse_mjml.MJTitleBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "title",
			Type: notifuse_mjml.MJMLComponentMjTitle,
		},
		Type:    notifuse_mjml.MJMLComponentMjTitle,
		Content: &titleContent,
	}

	preview := &notifuse_mjml.MJPreviewBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "preview",
			Type: notifuse_mjml.MJMLComponentMjPreview,
		},
		Type:    notifuse_mjml.MJMLComponentMjPreview,
		Content: &previewContent,
	}

	// Create footer text
	footerContent := "You received this email because you're subscribed to our newsletter.<br><a href=\"{{unsubscribe_url}}\">Unsubscribe</a> | <a href=\"https://demo.notifuse.com\">Visit our website</a>"
	footerText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "footer-text",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size": "12px",
				"color":     "#7f8c8d",
				"align":     "center",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &footerContent,
	}

	// Create columns for layout
	headerColumn := &notifuse_mjml.MJColumnBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "header-column",
			Type:     notifuse_mjml.MJMLComponentMjColumn,
			Children: []interface{}{headerText},
		},
		Type:     notifuse_mjml.MJMLComponentMjColumn,
		Children: []notifuse_mjml.EmailBlock{headerText},
	}

	contentColumn := &notifuse_mjml.MJColumnBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "content-column",
			Type:     notifuse_mjml.MJMLComponentMjColumn,
			Children: []interface{}{mainText, divider, highlightsText, highlightsList, button},
		},
		Type:     notifuse_mjml.MJMLComponentMjColumn,
		Children: []notifuse_mjml.EmailBlock{mainText, divider, highlightsText, highlightsList, button},
	}

	footerColumn := &notifuse_mjml.MJColumnBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "footer-column",
			Type:     notifuse_mjml.MJMLComponentMjColumn,
			Children: []interface{}{footerText},
		},
		Type:     notifuse_mjml.MJMLComponentMjColumn,
		Children: []notifuse_mjml.EmailBlock{footerText},
	}

	// Create sections
	headerSection := &notifuse_mjml.MJSectionBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "header-section",
			Type:     notifuse_mjml.MJMLComponentMjSection,
			Children: []interface{}{headerColumn},
			Attributes: map[string]interface{}{
				"background-color": "#f8f9fa",
				"padding":          "20px 0",
			},
		},
		Type:     notifuse_mjml.MJMLComponentMjSection,
		Children: []notifuse_mjml.EmailBlock{headerColumn},
	}

	contentSection := &notifuse_mjml.MJSectionBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "content-section",
			Type:     notifuse_mjml.MJMLComponentMjSection,
			Children: []interface{}{contentColumn},
			Attributes: map[string]interface{}{
				"background-color": "#ffffff",
				"padding":          "20px",
			},
		},
		Type:     notifuse_mjml.MJMLComponentMjSection,
		Children: []notifuse_mjml.EmailBlock{contentColumn},
	}

	footerSection := &notifuse_mjml.MJSectionBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "footer-section",
			Type:     notifuse_mjml.MJMLComponentMjSection,
			Children: []interface{}{footerColumn},
			Attributes: map[string]interface{}{
				"background-color": "#f8f9fa",
				"padding":          "20px",
			},
		},
		Type:     notifuse_mjml.MJMLComponentMjSection,
		Children: []notifuse_mjml.EmailBlock{footerColumn},
	}

	// Create head and body
	head := &notifuse_mjml.MJHeadBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "head",
			Type:     notifuse_mjml.MJMLComponentMjHead,
			Children: []interface{}{title, preview},
		},
		Type:     notifuse_mjml.MJMLComponentMjHead,
		Children: []notifuse_mjml.EmailBlock{title, preview},
	}

	body := &notifuse_mjml.MJBodyBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "body",
			Type:     notifuse_mjml.MJMLComponentMjBody,
			Children: []interface{}{headerSection, contentSection, footerSection},
		},
		Type:     notifuse_mjml.MJMLComponentMjBody,
		Children: []notifuse_mjml.EmailBlock{headerSection, contentSection, footerSection},
	}

	// Create root MJML block
	return &notifuse_mjml.MJMLBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "mjml-root",
			Type: notifuse_mjml.MJMLComponentMjml,
			Attributes: map[string]interface{}{
				"lang": "en",
			},
			Children: []interface{}{head, body},
		},
		Type: notifuse_mjml.MJMLComponentMjml,
		Attributes: map[string]interface{}{
			"lang": "en",
		},
		Children: []notifuse_mjml.EmailBlock{head, body},
	}
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
	title := &notifuse_mjml.MJTitleBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "title",
			Type: notifuse_mjml.MJMLComponentMjTitle,
		},
		Type:    notifuse_mjml.MJMLComponentMjTitle,
		Content: &titleContent,
	}

	preview := &notifuse_mjml.MJPreviewBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "preview",
			Type: notifuse_mjml.MJMLComponentMjPreview,
		},
		Type:    notifuse_mjml.MJMLComponentMjPreview,
		Content: &previewContent,
	}

	// Create hero section
	heroText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "hero-text",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "32px",
				"font-weight": "bold",
				"align":       "center",
				"color":       "#1a202c",
				"padding":     "20px 0",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &heroContent,
	}

	introText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "intro-text",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "16px",
				"line-height": "1.6",
				"color":       "#4a5568",
				"align":       "center",
				"padding":     "0 20px 30px",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &introContent,
	}

	// Create feature cards
	feature1TitleText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "feature1-title",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "18px",
				"font-weight": "bold",
				"color":       "#2d3748",
				"padding":     "10px 20px 5px",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &feature1Title,
	}

	feature1ContentText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "feature1-content",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "14px",
				"line-height": "1.5",
				"color":       "#4a5568",
				"padding":     "0 20px 20px",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &feature1Content,
	}

	feature2TitleText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "feature2-title",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "18px",
				"font-weight": "bold",
				"color":       "#2d3748",
				"padding":     "10px 20px 5px",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &feature2Title,
	}

	feature2ContentText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "feature2-content",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "14px",
				"line-height": "1.5",
				"color":       "#4a5568",
				"padding":     "0 20px 20px",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &feature2Content,
	}

	feature3TitleText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "feature3-title",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "18px",
				"font-weight": "bold",
				"color":       "#2d3748",
				"padding":     "10px 20px 5px",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &feature3Title,
	}

	feature3ContentText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "feature3-content",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "14px",
				"line-height": "1.5",
				"color":       "#4a5568",
				"padding":     "0 20px 20px",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &feature3Content,
	}

	// Create CTA button
	button := &notifuse_mjml.MJButtonBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "cta-button",
			Type: notifuse_mjml.MJMLComponentMjButton,
			Attributes: map[string]interface{}{
				"background-color": "#667eea",
				"color":            "#ffffff",
				"font-size":        "16px",
				"font-weight":      "bold",
				"padding":          "15px 30px",
				"border-radius":    "8px",
				"href":             "https://demo.notifuse.com/weekly-digest?utm_source={{utm_source}}&utm_medium={{utm_medium}}&utm_campaign={{utm_campaign}}",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjButton,
		Content: &buttonContent,
	}

	// Create footer
	footerContent := "You're receiving this because you subscribed to our weekly digest.<br><a href=\"{{unsubscribe_url}}\">Unsubscribe</a> | <a href=\"https://demo.notifuse.com/preferences\">Manage Preferences</a>"
	footerText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "footer-text",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size": "12px",
				"color":     "#a0aec0",
				"align":     "center",
				"padding":   "20px",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &footerContent,
	}

	// Create columns and sections
	heroColumn := &notifuse_mjml.MJColumnBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "hero-column",
			Type:     notifuse_mjml.MJMLComponentMjColumn,
			Children: []interface{}{heroText, introText},
		},
		Type:     notifuse_mjml.MJMLComponentMjColumn,
		Children: []notifuse_mjml.EmailBlock{heroText, introText},
	}

	// Create feature columns (side by side layout)
	feature1Column := &notifuse_mjml.MJColumnBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "feature1-column",
			Type:     notifuse_mjml.MJMLComponentMjColumn,
			Children: []interface{}{feature1TitleText, feature1ContentText},
			Attributes: map[string]interface{}{
				"width":            "50%",
				"background-color": "#f7fafc",
				"border-radius":    "8px",
				"padding":          "15px",
			},
		},
		Type:     notifuse_mjml.MJMLComponentMjColumn,
		Children: []notifuse_mjml.EmailBlock{feature1TitleText, feature1ContentText},
	}

	feature2Column := &notifuse_mjml.MJColumnBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "feature2-column",
			Type:     notifuse_mjml.MJMLComponentMjColumn,
			Children: []interface{}{feature2TitleText, feature2ContentText},
			Attributes: map[string]interface{}{
				"width":            "50%",
				"background-color": "#f7fafc",
				"border-radius":    "8px",
				"padding":          "15px",
			},
		},
		Type:     notifuse_mjml.MJMLComponentMjColumn,
		Children: []notifuse_mjml.EmailBlock{feature2TitleText, feature2ContentText},
	}

	feature3Column := &notifuse_mjml.MJColumnBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "feature3-column",
			Type:     notifuse_mjml.MJMLComponentMjColumn,
			Children: []interface{}{feature3TitleText, feature3ContentText},
			Attributes: map[string]interface{}{
				"background-color": "#f7fafc",
				"border-radius":    "8px",
				"padding":          "15px",
			},
		},
		Type:     notifuse_mjml.MJMLComponentMjColumn,
		Children: []notifuse_mjml.EmailBlock{feature3TitleText, feature3ContentText},
	}

	ctaColumn := &notifuse_mjml.MJColumnBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "cta-column",
			Type:     notifuse_mjml.MJMLComponentMjColumn,
			Children: []interface{}{button},
		},
		Type:     notifuse_mjml.MJMLComponentMjColumn,
		Children: []notifuse_mjml.EmailBlock{button},
	}

	footerColumn := &notifuse_mjml.MJColumnBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "footer-column",
			Type:     notifuse_mjml.MJMLComponentMjColumn,
			Children: []interface{}{footerText},
		},
		Type:     notifuse_mjml.MJMLComponentMjColumn,
		Children: []notifuse_mjml.EmailBlock{footerText},
	}

	// Create sections
	heroSection := &notifuse_mjml.MJSectionBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "hero-section",
			Type:     notifuse_mjml.MJMLComponentMjSection,
			Children: []interface{}{heroColumn},
			Attributes: map[string]interface{}{
				"background-color": "#ffffff",
				"padding":          "30px 20px",
			},
		},
		Type:     notifuse_mjml.MJMLComponentMjSection,
		Children: []notifuse_mjml.EmailBlock{heroColumn},
	}

	featuresSection := &notifuse_mjml.MJSectionBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "features-section",
			Type:     notifuse_mjml.MJMLComponentMjSection,
			Children: []interface{}{feature1Column, feature2Column},
			Attributes: map[string]interface{}{
				"background-color": "#ffffff",
				"padding":          "20px",
			},
		},
		Type:     notifuse_mjml.MJMLComponentMjSection,
		Children: []notifuse_mjml.EmailBlock{feature1Column, feature2Column},
	}

	feature3Section := &notifuse_mjml.MJSectionBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "feature3-section",
			Type:     notifuse_mjml.MJMLComponentMjSection,
			Children: []interface{}{feature3Column},
			Attributes: map[string]interface{}{
				"background-color": "#ffffff",
				"padding":          "20px",
			},
		},
		Type:     notifuse_mjml.MJMLComponentMjSection,
		Children: []notifuse_mjml.EmailBlock{feature3Column},
	}

	ctaSection := &notifuse_mjml.MJSectionBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "cta-section",
			Type:     notifuse_mjml.MJMLComponentMjSection,
			Children: []interface{}{ctaColumn},
			Attributes: map[string]interface{}{
				"background-color": "#ffffff",
				"padding":          "30px 20px",
			},
		},
		Type:     notifuse_mjml.MJMLComponentMjSection,
		Children: []notifuse_mjml.EmailBlock{ctaColumn},
	}

	footerSection := &notifuse_mjml.MJSectionBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "footer-section",
			Type:     notifuse_mjml.MJMLComponentMjSection,
			Children: []interface{}{footerColumn},
			Attributes: map[string]interface{}{
				"background-color": "#edf2f7",
				"padding":          "20px",
			},
		},
		Type:     notifuse_mjml.MJMLComponentMjSection,
		Children: []notifuse_mjml.EmailBlock{footerColumn},
	}

	// Create head and body
	head := &notifuse_mjml.MJHeadBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "head",
			Type:     notifuse_mjml.MJMLComponentMjHead,
			Children: []interface{}{title, preview},
		},
		Type:     notifuse_mjml.MJMLComponentMjHead,
		Children: []notifuse_mjml.EmailBlock{title, preview},
	}

	body := &notifuse_mjml.MJBodyBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "body",
			Type:     notifuse_mjml.MJMLComponentMjBody,
			Children: []interface{}{heroSection, featuresSection, feature3Section, ctaSection, footerSection},
		},
		Type:     notifuse_mjml.MJMLComponentMjBody,
		Children: []notifuse_mjml.EmailBlock{heroSection, featuresSection, feature3Section, ctaSection, footerSection},
	}

	// Create root MJML block
	return &notifuse_mjml.MJMLBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "mjml-root",
			Type: notifuse_mjml.MJMLComponentMjml,
			Attributes: map[string]interface{}{
				"lang": "en",
			},
			Children: []interface{}{head, body},
		},
		Type: notifuse_mjml.MJMLComponentMjml,
		Attributes: map[string]interface{}{
			"lang": "en",
		},
		Children: []notifuse_mjml.EmailBlock{head, body},
	}
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
	title := &notifuse_mjml.MJTitleBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "title",
			Type: notifuse_mjml.MJMLComponentMjTitle,
		},
		Type:    notifuse_mjml.MJMLComponentMjTitle,
		Content: &titleContent,
	}

	preview := &notifuse_mjml.MJPreviewBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "preview",
			Type: notifuse_mjml.MJMLComponentMjPreview,
		},
		Type:    notifuse_mjml.MJMLComponentMjPreview,
		Content: &previewContent,
	}

	welcomeText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "welcome-text",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "28px",
				"font-weight": "bold",
				"align":       "center",
				"color":       "#2c3e50",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &welcomeContent,
	}

	mainText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "main-text",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "16px",
				"line-height": "1.6",
				"color":       "#34495e",
				"align":       "center",
				"padding":     "20px 0",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &mainContent,
	}

	button := &notifuse_mjml.MJButtonBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "get-started-button",
			Type: notifuse_mjml.MJMLComponentMjButton,
			Attributes: map[string]interface{}{
				"background-color": "#27ae60",
				"color":            "#ffffff",
				"font-size":        "16px",
				"padding":          "12px 24px",
				"border-radius":    "6px",
				"href":             "https://demo.notifuse.com/getting-started?utm_source={{utm_source}}&utm_medium={{utm_medium}}&utm_campaign={{utm_campaign}}",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjButton,
		Content: &buttonContent,
	}

	divider := &notifuse_mjml.MJDividerBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "divider",
			Type: notifuse_mjml.MJMLComponentMjDivider,
			Attributes: map[string]interface{}{
				"border-width": "1px",
				"border-color": "#ecf0f1",
				"padding":      "20px 0",
			},
		},
		Type: notifuse_mjml.MJMLComponentMjDivider,
	}

	footerText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "footer-text",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size": "14px",
				"color":     "#7f8c8d",
				"align":     "center",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &footerContent,
	}

	column := &notifuse_mjml.MJColumnBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "main-column",
			Type:     notifuse_mjml.MJMLComponentMjColumn,
			Children: []interface{}{welcomeText, mainText, button, divider, footerText},
		},
		Type:     notifuse_mjml.MJMLComponentMjColumn,
		Children: []notifuse_mjml.EmailBlock{welcomeText, mainText, button, divider, footerText},
	}

	section := &notifuse_mjml.MJSectionBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "main-section",
			Type:     notifuse_mjml.MJMLComponentMjSection,
			Children: []interface{}{column},
			Attributes: map[string]interface{}{
				"background-color": "#ffffff",
				"padding":          "40px 20px",
			},
		},
		Type:     notifuse_mjml.MJMLComponentMjSection,
		Children: []notifuse_mjml.EmailBlock{column},
	}

	head := &notifuse_mjml.MJHeadBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "head",
			Type:     notifuse_mjml.MJMLComponentMjHead,
			Children: []interface{}{title, preview},
		},
		Type:     notifuse_mjml.MJMLComponentMjHead,
		Children: []notifuse_mjml.EmailBlock{title, preview},
	}

	body := &notifuse_mjml.MJBodyBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "body",
			Type:     notifuse_mjml.MJMLComponentMjBody,
			Children: []interface{}{section},
		},
		Type:     notifuse_mjml.MJMLComponentMjBody,
		Children: []notifuse_mjml.EmailBlock{section},
	}

	return &notifuse_mjml.MJMLBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "mjml-root",
			Type: notifuse_mjml.MJMLComponentMjml,
			Attributes: map[string]interface{}{
				"lang": "en",
			},
			Children: []interface{}{head, body},
		},
		Type: notifuse_mjml.MJMLComponentMjml,
		Attributes: map[string]interface{}{
			"lang": "en",
		},
		Children: []notifuse_mjml.EmailBlock{head, body},
	}
}

// createSampleBroadcast creates a sample broadcast campaign
func (s *DemoService) createSampleBroadcast(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating sample broadcast")

	// Create a draft broadcast campaign
	broadcastReq := &domain.CreateBroadcastRequest{
		WorkspaceID: workspaceID,
		Name:        "Weekly Newsletter - Demo Campaign",
		Audience: domain.AudienceSettings{
			Lists:               []string{"newsletter"},
			Segments:            []string{},
			ExcludeUnsubscribed: true,
			SkipDuplicateEmails: true,
			RateLimitPerMinute:  0,
		},
		Schedule: domain.ScheduleSettings{
			IsScheduled:   false,
			ScheduledDate: "",
			ScheduledTime: "",
			Timezone:      "UTC",
		},
		TestSettings: domain.BroadcastTestSettings{
			Enabled:          true,
			SamplePercentage: 10,
			AutoSendWinner:   false,
			Variations: []domain.BroadcastVariation{
				{
					VariationName: "variation-a",
					TemplateID:    "newsletter-weekly",
				},
				{
					VariationName: "variation-b",
					TemplateID:    "newsletter-weekly-v2",
				},
			},
		},
		TrackingEnabled: true,
		UTMParameters: &domain.UTMParameters{
			Source:   "demo.notifuse.com",
			Medium:   "email",
			Campaign: "weekly_newsletter_demo_campaign",
			Term:     "",
			Content:  "",
		},
	}

	broadcast, err := s.broadcastService.CreateBroadcast(ctx, broadcastReq)
	if err != nil {
		return fmt.Errorf("failed to create sample broadcast: %w", err)
	}

	s.logger.WithField("broadcast_id", broadcast.ID).WithField("workspace_id", workspaceID).Info("Sample broadcast created successfully")
	return nil
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
	title := &notifuse_mjml.MJTitleBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "title",
			Type: notifuse_mjml.MJMLComponentMjTitle,
		},
		Type:    notifuse_mjml.MJMLComponentMjTitle,
		Content: &titleContent,
	}

	preview := &notifuse_mjml.MJPreviewBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "preview",
			Type: notifuse_mjml.MJMLComponentMjPreview,
		},
		Type:    notifuse_mjml.MJMLComponentMjPreview,
		Content: &previewContent,
	}

	headerText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "header-text",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "28px",
				"font-weight": "bold",
				"align":       "center",
				"color":       "#e74c3c",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &headerContent,
	}

	mainText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "main-text",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size":   "16px",
				"line-height": "1.6",
				"color":       "#34495e",
				"padding":     "20px 0",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &mainContent,
	}

	button := &notifuse_mjml.MJButtonBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "reset-button",
			Type: notifuse_mjml.MJMLComponentMjButton,
			Attributes: map[string]interface{}{
				"background-color": "#e74c3c",
				"color":            "#ffffff",
				"font-size":        "16px",
				"font-weight":      "bold",
				"padding":          "15px 30px",
				"border-radius":    "6px",
				"href":             "{{reset_url}}",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjButton,
		Content: &buttonContent,
	}

	expireText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "expire-text",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size": "14px",
				"color":     "#95a5a6",
				"align":     "center",
				"padding":   "10px 0",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &expireContent,
	}

	divider := &notifuse_mjml.MJDividerBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "divider",
			Type: notifuse_mjml.MJMLComponentMjDivider,
			Attributes: map[string]interface{}{
				"border-width": "1px",
				"border-color": "#ecf0f1",
				"padding":      "20px 0",
			},
		},
		Type: notifuse_mjml.MJMLComponentMjDivider,
	}

	footerText := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "footer-text",
			Type: notifuse_mjml.MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"font-size": "14px",
				"color":     "#7f8c8d",
				"align":     "center",
			},
		},
		Type:    notifuse_mjml.MJMLComponentMjText,
		Content: &footerContent,
	}

	column := &notifuse_mjml.MJColumnBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "main-column",
			Type:     notifuse_mjml.MJMLComponentMjColumn,
			Children: []interface{}{headerText, mainText, button, expireText, divider, footerText},
		},
		Type:     notifuse_mjml.MJMLComponentMjColumn,
		Children: []notifuse_mjml.EmailBlock{headerText, mainText, button, expireText, divider, footerText},
	}

	section := &notifuse_mjml.MJSectionBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "main-section",
			Type:     notifuse_mjml.MJMLComponentMjSection,
			Children: []interface{}{column},
			Attributes: map[string]interface{}{
				"background-color": "#ffffff",
				"padding":          "40px 20px",
			},
		},
		Type:     notifuse_mjml.MJMLComponentMjSection,
		Children: []notifuse_mjml.EmailBlock{column},
	}

	head := &notifuse_mjml.MJHeadBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "head",
			Type:     notifuse_mjml.MJMLComponentMjHead,
			Children: []interface{}{title, preview},
		},
		Type:     notifuse_mjml.MJMLComponentMjHead,
		Children: []notifuse_mjml.EmailBlock{title, preview},
	}

	body := &notifuse_mjml.MJBodyBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:       "body",
			Type:     notifuse_mjml.MJMLComponentMjBody,
			Children: []interface{}{section},
		},
		Type:     notifuse_mjml.MJMLComponentMjBody,
		Children: []notifuse_mjml.EmailBlock{section},
	}

	return &notifuse_mjml.MJMLBlock{
		BaseBlock: notifuse_mjml.BaseBlock{
			ID:   "mjml-root",
			Type: notifuse_mjml.MJMLComponentMjml,
			Attributes: map[string]interface{}{
				"lang": "en",
			},
			Children: []interface{}{head, body},
		},
		Type: notifuse_mjml.MJMLComponentMjml,
		Attributes: map[string]interface{}{
			"lang": "en",
		},
		Children: []notifuse_mjml.EmailBlock{head, body},
	}
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
func (s *DemoService) generateSampleMessageHistory(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Generating sample message history with ~3 emails per contact")

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
	totalMessages, err := s.generateMessagesPerContact(ctx, workspaceID, contactsResp.Contacts)
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
func (s *DemoService) generateMessagesPerContact(ctx context.Context, workspaceID string, contacts []*domain.Contact) (int, error) {
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
		// Newsletter campaigns over last 10 days
		{templateID: "newsletter-weekly", templateVersion: 1, broadcastID: stringPtr("newsletter-broadcast-1"), messageType: "newsletter", daysAgo: 1},
		{templateID: "newsletter-weekly-v2", templateVersion: 1, broadcastID: stringPtr("newsletter-broadcast-2"), messageType: "newsletter", daysAgo: 4},
		{templateID: "newsletter-weekly", templateVersion: 1, broadcastID: stringPtr("newsletter-broadcast-3"), messageType: "newsletter", daysAgo: 7},
		{templateID: "newsletter-weekly-v2", templateVersion: 1, broadcastID: stringPtr("newsletter-broadcast-4"), messageType: "newsletter", daysAgo: 10},
		// Transactional messages
		{templateID: "welcome-email", templateVersion: 1, broadcastID: nil, messageType: "welcome", daysAgo: 2},
		{templateID: "welcome-email", templateVersion: 1, broadcastID: nil, messageType: "welcome", daysAgo: 5},
		{templateID: "password-reset", templateVersion: 1, broadcastID: nil, messageType: "password-reset", daysAgo: 3},
		{templateID: "password-reset", templateVersion: 1, broadcastID: nil, messageType: "password-reset", daysAgo: 8},
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

				if err := s.messageHistoryRepo.Create(ctx, workspaceID, message); err != nil {
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
		// 1. First, generate webhook events for delivered messages
		if err := s.generateDeliveredWebhookEventsForBatch(ctx, workspaceID, messagesWithEngagement); err != nil {
			s.logger.WithField("error", err.Error()).Debug("Failed to generate delivered webhook events")
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

// stringPtr returns a pointer to a string
func stringPtr(s string) *string {
	return &s
}

// messageEngagementData holds a message and its engagement info
type messageEngagementData struct {
	message    *domain.MessageHistory
	engagement messageEngagement
}

// generateDeliveredWebhookEventsForBatch creates webhook events for delivered messages
func (s *DemoService) generateDeliveredWebhookEventsForBatch(ctx context.Context, workspaceID string, messagesData []messageEngagementData) error {
	// Skip if workspace service or webhook event repo is not available
	if s.workspaceService == nil || s.webhookEventRepo == nil {
		s.logger.Debug("Workspace service or webhook event repo not available, skipping webhook events")
		return nil
	}

	// Get the integration ID from the workspace
	workspace, err := s.workspaceService.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	integrationID := workspace.Settings.TransactionalEmailProviderID
	if integrationID == "" {
		s.logger.WithField("workspace_id", workspaceID).Debug("No transactional email provider configured, skipping webhook events")
		return nil
	}

	// Collect webhook events for delivered messages
	var webhookEvents []*domain.WebhookEvent
	for _, data := range messagesData {
		if !data.engagement.shouldDeliver {
			continue
		}

		webhookEventID := uuid.New().String()
		rawPayload := fmt.Sprintf(`{"event":"delivered","message_id":"%s","recipient":"%s","timestamp":"%s"}`,
			data.message.ID, data.message.ContactEmail, data.engagement.deliveredTime.Format(time.RFC3339))

		webhookEvent := &domain.WebhookEvent{
			ID:                webhookEventID,
			Type:              domain.EmailEventDelivered,
			EmailProviderKind: domain.EmailProviderKindSMTP,
			IntegrationID:     integrationID,
			RecipientEmail:    data.message.ContactEmail,
			MessageID:         data.message.ID,
			Timestamp:         data.engagement.deliveredTime,
			RawPayload:        rawPayload,
			CreatedAt:         data.engagement.deliveredTime,
		}

		webhookEvents = append(webhookEvents, webhookEvent)
	}

	// Store webhook events
	if len(webhookEvents) > 0 {
		if err := s.webhookEventRepo.StoreEvents(ctx, workspaceID, webhookEvents); err != nil {
			return fmt.Errorf("failed to store webhook events: %w", err)
		}
		s.logger.WithField("count", len(webhookEvents)).Debug("Generated delivered webhook events")
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
	var utmMedium string = "email"

	if strings.Contains(broadcastID, "transactional") {
		campaignType = "transactional"
		if templateID == "password-reset" {
			campaignType = "password_reset"
		} else if templateID == "welcome-email" {
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

// Helper function to get a random pointer to string from slice
func getRandomPointer(slice []string) *string {
	if len(slice) == 0 {
		return nil
	}
	value := slice[rand.Intn(len(slice))]
	return &value
}

// createSampleSegments creates demo segments for showcasing the segmentation feature
func (s *DemoService) createSampleSegments(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating sample segments")

	// Segment 1: VIP Customers (high lifetime value and orders)
	vipSegment := &domain.CreateSegmentRequest{
		WorkspaceID: workspaceID,
		ID:          "vipcustomers",
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
							Table: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "lifetime_value",
										FieldType:    "number",
										Operator:     "gte",
										NumberValues: []float64{800.0},
									},
								},
							},
						},
					},
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Table: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "orders_count",
										FieldType:    "number",
										Operator:     "gte",
										NumberValues: []float64{3.0},
									},
								},
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

	// Segment 2: US Customers
	usSegment := &domain.CreateSegmentRequest{
		WorkspaceID: workspaceID,
		ID:          "uscustomers",
		Name:        "US Customers",
		Color:       "blue",
		Timezone:    "UTC",
		Tree: &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "and",
				Leaves: []*domain.TreeNode{
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Table: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "country",
										FieldType:    "string",
										Operator:     "equals",
										StringValues: []string{"US"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if _, err := s.segmentService.CreateSegment(ctx, usSegment); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create US Customers segment")
	} else {
		s.logger.Info("Created US Customers segment")
	}

	// Segment 3: European Market (complex OR logic)
	europeSegment := &domain.CreateSegmentRequest{
		WorkspaceID: workspaceID,
		ID:          "european-market",
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
							Table: "contacts",
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
							Table: "contacts",
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
							Table: "contacts",
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
							Table: "contacts",
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
							Table: "contacts",
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

	// Segment 4: Engaged Users (behavioral - email opens)
	engagedSegment := &domain.CreateSegmentRequest{
		WorkspaceID: workspaceID,
		ID:          "engaged-users",
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
							Table: "contact_timeline",
							ContactTimeline: &domain.ContactTimelineCondition{
								Kind:          "open_email",
								CountOperator: "at_least",
								CountValue:    5,
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

	// Segment 5: At-Risk Customers (no recent orders)
	atRiskSegment := &domain.CreateSegmentRequest{
		WorkspaceID: workspaceID,
		ID:          "at-risk-customers",
		Name:        "At-Risk Customers",
		Color:       "orange",
		Timezone:    "UTC",
		Tree: &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "and",
				Leaves: []*domain.TreeNode{
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Table: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "lifetime_value",
										FieldType:    "number",
										Operator:     "gte",
										NumberValues: []float64{500.0},
									},
								},
							},
						},
					},
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Table: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "last_order_at",
										FieldType:    "time",
										Operator:     "before_date",
										StringValues: []string{"2024-01-01"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if _, err := s.segmentService.CreateSegment(ctx, atRiskSegment); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create At-Risk Customers segment")
	} else {
		s.logger.Info("Created At-Risk Customers segment")
	}

	// Segment 6: Newsletter Subscribers (list-based)
	newsletterSegment := &domain.CreateSegmentRequest{
		WorkspaceID: workspaceID,
		ID:          "newsletter-subscribers",
		Name:        "Newsletter Subscribers",
		Color:       "cyan",
		Timezone:    "UTC",
		Tree: &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "and",
				Leaves: []*domain.TreeNode{
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Table: "contact_lists",
							ContactList: &domain.ContactListCondition{
								Operator: "in",
								ListID:   "newsletter",
							},
						},
					},
				},
			},
		},
	}

	if _, err := s.segmentService.CreateSegment(ctx, newsletterSegment); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create Newsletter Subscribers segment")
	} else {
		s.logger.Info("Created Newsletter Subscribers segment")
	}

	// Segment 7: High-Value North America (complex AND/OR logic)
	highValueNASegment := &domain.CreateSegmentRequest{
		WorkspaceID: workspaceID,
		ID:          "high-value-north-america",
		Name:        "High-Value North America",
		Color:       "purple",
		Timezone:    "UTC",
		Tree: &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "and",
				Leaves: []*domain.TreeNode{
					{
						Kind: "branch",
						Branch: &domain.TreeNodeBranch{
							Operator: "or",
							Leaves: []*domain.TreeNode{
								{
									Kind: "leaf",
									Leaf: &domain.TreeNodeLeaf{
										Table: "contacts",
										Contact: &domain.ContactCondition{
											Filters: []*domain.DimensionFilter{
												{
													FieldName:    "country",
													FieldType:    "string",
													Operator:     "equals",
													StringValues: []string{"US"},
												},
											},
										},
									},
								},
								{
									Kind: "leaf",
									Leaf: &domain.TreeNodeLeaf{
										Table: "contacts",
										Contact: &domain.ContactCondition{
											Filters: []*domain.DimensionFilter{
												{
													FieldName:    "country",
													FieldType:    "string",
													Operator:     "equals",
													StringValues: []string{"CA"},
												},
											},
										},
									},
								},
							},
						},
					},
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Table: "contacts",
							Contact: &domain.ContactCondition{
								Filters: []*domain.DimensionFilter{
									{
										FieldName:    "lifetime_value",
										FieldType:    "number",
										Operator:     "gte",
										NumberValues: []float64{2000.0},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if _, err := s.segmentService.CreateSegment(ctx, highValueNASegment); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create High-Value North America segment")
	} else {
		s.logger.Info("Created High-Value North America segment")
	}

	// Segment 8: Inactive Users (no email opens)
	inactiveSegment := &domain.CreateSegmentRequest{
		WorkspaceID: workspaceID,
		ID:          "inactive-users",
		Name:        "Inactive Users",
		Color:       "default",
		Timezone:    "UTC",
		Tree: &domain.TreeNode{
			Kind: "branch",
			Branch: &domain.TreeNodeBranch{
				Operator: "and",
				Leaves: []*domain.TreeNode{
					{
						Kind: "leaf",
						Leaf: &domain.TreeNodeLeaf{
							Table: "contact_timeline",
							ContactTimeline: &domain.ContactTimelineCondition{
								Kind:          "open_email",
								CountOperator: "equals",
								CountValue:    0,
							},
						},
					},
				},
			},
		},
	}

	if _, err := s.segmentService.CreateSegment(ctx, inactiveSegment); err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to create Inactive Users segment")
	} else {
		s.logger.Info("Created Inactive Users segment")
	}

	s.logger.WithField("workspace_id", workspaceID).Info("Sample segments created successfully")
	return nil
}
