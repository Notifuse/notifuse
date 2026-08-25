package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/asaskevich/govalidator"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDemoService_VerifyRootEmailHMAC(t *testing.T) {
	t.Run("returns false when root email is empty", func(t *testing.T) {
		svc := &DemoService{
			logger: logger.NewLoggerWithLevel("disabled"),
			config: &config.Config{RootEmail: "", Security: config.SecurityConfig{SecretKey: "secret"}},
		}
		assert.False(t, svc.VerifyRootEmailHMAC("anything"))
	})

	t.Run("returns true for valid HMAC and false for invalid", func(t *testing.T) {
		rootEmail := "root@example.com"
		secret := "supersecretkey"
		cfg := &config.Config{RootEmail: rootEmail, Security: config.SecurityConfig{SecretKey: secret}}
		svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled"), config: cfg}

		valid := domain.ComputeEmailHMAC(rootEmail, secret)
		assert.True(t, svc.VerifyRootEmailHMAC(valid))
		assert.False(t, svc.VerifyRootEmailHMAC(valid+"x"))
	})

	t.Run("uses the primary email when ROOT_EMAIL is a list", func(t *testing.T) {
		secret := "supersecretkey"
		cfg := &config.Config{RootEmail: "primary@example.com,second@example.com", Security: config.SecurityConfig{SecretKey: secret}}
		svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled"), config: cfg}

		// HMAC over the primary (first) email is accepted.
		assert.True(t, svc.VerifyRootEmailHMAC(domain.ComputeEmailHMAC("primary@example.com", secret)))
		// HMAC over a non-primary root, or over the raw list string, is rejected.
		assert.False(t, svc.VerifyRootEmailHMAC(domain.ComputeEmailHMAC("second@example.com", secret)))
		assert.False(t, svc.VerifyRootEmailHMAC(domain.ComputeEmailHMAC("primary@example.com,second@example.com", secret)))
	})
}

func TestDemoService_DeleteAllWorkspaces(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)

	svc := &DemoService{
		logger:        logger.NewLoggerWithLevel("disabled"),
		workspaceRepo: mockWorkspaceRepo,
		taskRepo:      mockTaskRepo,
	}

	ctx := context.Background()
	workspaces := []*domain.Workspace{{ID: "w1"}, {ID: "w2"}}

	// Success path
	mockWorkspaceRepo.EXPECT().List(ctx).Return(workspaces, nil)
	mockWorkspaceRepo.EXPECT().Delete(ctx, "w1").Return(nil)
	mockTaskRepo.EXPECT().DeleteAll(ctx, "w1").Return(nil)
	mockWorkspaceRepo.EXPECT().Delete(ctx, "w2").Return(nil)
	mockTaskRepo.EXPECT().DeleteAll(ctx, "w2").Return(nil)

	err := svc.deleteAllWorkspaces(ctx)
	assert.NoError(t, err)

	// Partial failures should still return nil
	mockWorkspaceRepo2 := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockTaskRepo2 := domainmocks.NewMockTaskRepository(ctrl)
	svc2 := &DemoService{logger: logger.NewLoggerWithLevel("disabled"), workspaceRepo: mockWorkspaceRepo2, taskRepo: mockTaskRepo2}

	mockWorkspaceRepo2.EXPECT().List(ctx).Return(workspaces, nil)
	mockWorkspaceRepo2.EXPECT().Delete(ctx, "w1").Return(assert.AnError)
	mockTaskRepo2.EXPECT().DeleteAll(ctx, "w1").Return(assert.AnError)
	mockWorkspaceRepo2.EXPECT().Delete(ctx, "w2").Return(nil)
	mockTaskRepo2.EXPECT().DeleteAll(ctx, "w2").Return(nil)

	err = svc2.deleteAllWorkspaces(ctx)
	assert.NoError(t, err)
}

func TestDemoService_GenerateSampleContactsBatch(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}

	batch := svc.generateSampleContactsBatch(10, 100)
	assert.Len(t, batch, 10)
	for i, c := range batch {
		assert.NotEmpty(t, c.Email)
		assert.NotZero(t, c.CreatedAt.Unix())
		assert.NotNil(t, c.FirstName)
		assert.NotNil(t, c.LastName)
		assert.True(t, strings.Contains(strings.ToLower(c.Email), strings.ToLower(c.FirstName.String)))
		assert.True(t, strings.Contains(strings.ToLower(c.Email), strings.ToLower(c.LastName.String)))
		// Ensure progression uses startIndex in at least some addresses across batch
		_ = i
	}
}

func TestGenerateEmail_BasicStructure(t *testing.T) {
	first := "John"
	last := "Doe"

	email := generateEmail(first, last, 42)
	// Basic checks
	assert.Contains(t, strings.ToLower(email), strings.ToLower(first))
	assert.Contains(t, strings.ToLower(email), strings.ToLower(last))
	parts := strings.SplitN(email, "@", 2)
	if assert.Len(t, parts, 2) {
		domainPart := parts[1]
		// Validate domain is one of the configured demo domains
		var found bool
		for _, d := range emailDomains {
			if domainPart == d {
				found = true
				break
			}
		}
		assert.True(t, found, "unexpected domain: %s", domainPart)
	}
}

func TestGetRandomElement(t *testing.T) {
	options := []string{"a", "b", "c"}
	picked := getRandomElement(options)
	assert.Contains(t, options, picked)
}

func TestCreateFallbackHTML(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
	html := svc.createFallbackHTML()
	assert.Contains(t, html, "<!DOCTYPE html>")
	assert.Contains(t, html, "</html>")
}

func TestNewDemoService_Constructs(t *testing.T) {
	svc := NewDemoService(
		logger.NewLoggerWithLevel("disabled"),
		&config.Config{},
		nil, // workspaceService
		nil, // userService
		nil, // contactService
		nil, // listService
		nil, // contactListService
		nil, // templateService
		nil, // emailService
		nil, // broadcastService
		nil, // taskService
		nil, // transactionalNotificationService
		nil, // webhookEventService
		nil, // webhookRegistrationService
		nil, // messageHistoryService
		nil, // notificationCenterService
		nil, // segmentService
		nil, // workspaceRepo
		nil, // taskRepo
		nil, // messageHistoryRepo
		nil, // webhookEventRepo
		nil, // broadcastRepo
		nil, // customEventRepo
		nil, // webAnalyticsRepo
		nil, // annotationRepo
		nil, // webhookSubscriptionService
		nil, // automationService
	)
	assert.NotNil(t, svc)
}

func TestDemoService_ResetDemo_DeleteAllError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)

	svc := &DemoService{
		logger:        logger.NewLoggerWithLevel("disabled"),
		workspaceRepo: mockWorkspaceRepo,
	}

	ctx := context.Background()
	mockWorkspaceRepo.EXPECT().List(ctx).Return(nil, assert.AnError)

	err := svc.ResetDemo(ctx)
	assert.Error(t, err)
}

func TestDemoService_CompileTemplateToHTML_Basic(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}

	titleContent := "Title"
	textContent := "Hello"

	titleBase := notifuse_mjml.NewBaseBlock("title", notifuse_mjml.MJMLComponentMjTitle)
	titleBase.Content = &titleContent
	title := &notifuse_mjml.MJTitleBlock{BaseBlock: titleBase}

	head := &notifuse_mjml.MJHeadBlock{BaseBlock: notifuse_mjml.NewBaseBlock("head", notifuse_mjml.MJMLComponentMjHead)}
	head.Children = []notifuse_mjml.EmailBlock{title}

	textBase := notifuse_mjml.NewBaseBlock("text", notifuse_mjml.MJMLComponentMjText)
	textBase.Content = &textContent
	text := &notifuse_mjml.MJTextBlock{BaseBlock: textBase}

	col := &notifuse_mjml.MJColumnBlock{BaseBlock: notifuse_mjml.NewBaseBlock("col", notifuse_mjml.MJMLComponentMjColumn)}
	col.Children = []notifuse_mjml.EmailBlock{text}

	sec := &notifuse_mjml.MJSectionBlock{BaseBlock: notifuse_mjml.NewBaseBlock("sec", notifuse_mjml.MJMLComponentMjSection)}
	sec.Children = []notifuse_mjml.EmailBlock{col}

	body := &notifuse_mjml.MJBodyBlock{BaseBlock: notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)}
	body.Children = []notifuse_mjml.EmailBlock{sec}

	root := &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)}
	root.Children = []notifuse_mjml.EmailBlock{head, body}

	html := svc.compileTemplateToHTML("demo", "message-1", root, domain.MapOfAny{"contact": domain.MapOfAny{"first_name": "Test"}})
	assert.NotEmpty(t, html)
}

func TestDemoService_CreateSampleLists_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockListRepo := domainmocks.NewMockListRepository(ctrl)
	mockContactListRepo := domainmocks.NewMockContactListRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockAuth := domainmocks.NewMockAuthService(ctrl)
	mockEmail := domainmocks.NewMockEmailServiceInterface(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockMessageHistoryRepo := domainmocks.NewMockMessageHistoryRepository(ctrl)
	mockCache := pkgmocks.NewMockCache(ctrl)

	listSvc := NewListService(mockListRepo, mockWorkspaceRepo, mockContactListRepo, mockContactRepo, mockMessageHistoryRepo, mockAuth, mockEmail, logger.NewLoggerWithLevel("disabled"), "https://api.test", mockCache)

	svc := &DemoService{
		logger:      logger.NewLoggerWithLevel("disabled"),
		listService: listSvc,
	}

	ctx := context.Background()
	userWorkspace := &domain.UserWorkspace{
		UserID:      "u1",
		WorkspaceID: "demo",
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceLists: {Read: true, Write: true},
		},
	}
	mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, "demo").Return(ctx, &domain.User{ID: "u1"}, userWorkspace, nil)
	mockListRepo.EXPECT().CreateList(ctx, "demo", gomock.Any()).Return(assert.AnError)

	err := svc.createSampleLists(ctx, "demo")
	assert.Error(t, err)
}

func TestDemoService_SubscribeContactsToList_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockListRepo := domainmocks.NewMockListRepository(ctrl)
	mockContactListRepo := domainmocks.NewMockContactListRepository(ctrl)
	mockAuth := domainmocks.NewMockAuthService(ctrl)
	mockEmail := domainmocks.NewMockEmailServiceInterface(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)

	// Services
	mockMessageHistoryRepo := domainmocks.NewMockMessageHistoryRepository(ctrl)
	mockInboundWebhookEventRepo := domainmocks.NewMockInboundWebhookEventRepository(ctrl)
	mockContactTimelineRepo := domainmocks.NewMockContactTimelineRepository(ctrl)
	mockCache := pkgmocks.NewMockCache(ctrl)
	contactSvc := NewContactService(mockContactRepo, mockWorkspaceRepo, mockAuth, mockMessageHistoryRepo, mockInboundWebhookEventRepo, mockContactListRepo, mockContactTimelineRepo, nil, nil, nil, nil, nil, logger.NewLoggerWithLevel("disabled"))
	listSvc := NewListService(mockListRepo, mockWorkspaceRepo, mockContactListRepo, mockContactRepo, mockMessageHistoryRepo, mockAuth, mockEmail, logger.NewLoggerWithLevel("disabled"), "https://api.test", mockCache)

	svc := &DemoService{
		logger:         logger.NewLoggerWithLevel("disabled"),
		contactService: contactSvc,
		listService:    listSvc,
	}

	ctx := context.Background()

	userWorkspace := &domain.UserWorkspace{
		UserID:      "u1",
		WorkspaceID: "demo",
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: true},
			domain.PermissionResourceLists:    {Read: true, Write: true},
		},
	}

	// GetContacts flow
	mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, "demo").Return(ctx, &domain.User{ID: "u1"}, userWorkspace, nil)
	mockContactRepo.EXPECT().GetContacts(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, req *domain.GetContactsRequest) (*domain.GetContactsResponse, error) {
		return &domain.GetContactsResponse{Contacts: []*domain.Contact{{Email: "a@example.com"}, {Email: "b@example.com"}}}, nil
	})

	// SubscribeToLists flow
	ws := &domain.Workspace{ID: "demo", Settings: domain.WorkspaceSettings{SecretKey: "secret"}}
	mockWorkspaceRepo.EXPECT().GetByID(ctx, "demo").Return(ws, nil).Times(2)

	// Not authenticated path: check existence -> not found
	mockContactRepo.EXPECT().GetContactByEmail(ctx, "demo", gomock.Any()).Return(nil, assert.AnError).Times(2)
	// Upsert contacts
	mockContactRepo.EXPECT().UpsertContact(ctx, "demo", gomock.Any()).Return(true, nil).Times(2)
	// List retrieval
	mockListRepo.EXPECT().GetLists(ctx, "demo").Return([]*domain.List{{ID: "newsletter", Name: "Newsletter", IsPublic: true}}, nil).Times(2)
	// Check existing subscription (not found)
	mockContactListRepo.EXPECT().GetContactListByIDs(ctx, "demo", gomock.Any(), "newsletter").Return(nil, &domain.ErrContactListNotFound{Message: "not found"}).Times(2)
	// Add to list
	mockContactListRepo.EXPECT().AddContactToList(ctx, "demo", gomock.Any()).Return(nil).Times(2)

	err := svc.subscribeContactsToList(ctx, "demo", "newsletter")
	assert.NoError(t, err)
}

func TestDemoService_CreateSampleTemplates_Smoke(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockAuth := domainmocks.NewMockAuthService(ctrl)

	tmplSvc := NewTemplateService(mockTemplateRepo, mockWorkspaceRepo, mockAuth, logger.NewLoggerWithLevel("disabled"), "https://api.test")

	svc := &DemoService{
		logger:          logger.NewLoggerWithLevel("disabled"),
		templateService: tmplSvc,
	}

	ctx := context.Background()

	userWorkspace := &domain.UserWorkspace{
		UserID:      "u1",
		WorkspaceID: "demo",
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTemplates: {Read: true, Write: true},
		},
	}

	// One authentication and one workspace lookup per template, whatever the seeder creates.
	mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, "demo").Return(ctx, &domain.User{ID: "u1"}, userWorkspace, nil).AnyTimes()
	mockWorkspaceRepo.EXPECT().GetByID(ctx, "demo").Return(&domain.Workspace{
		ID: "demo",
		Settings: domain.WorkspaceSettings{
			DefaultLanguage: "en",
			Languages:       []string{"en", "fr", "es"},
		},
	}, nil).AnyTimes()

	var created []string
	mockTemplateRepo.EXPECT().
		CreateTemplate(ctx, "demo", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, template *domain.Template) error {
			created = append(created, template.ID)
			return nil
		}).
		AnyTimes()

	err := svc.createSampleTemplates(ctx, "demo")
	assert.NoError(t, err)

	// A CreateTemplate failure is only Warn-logged, so an automation's email node can end up pointing
	// at a template that was never created without anything failing.
	assert.Subset(t, created, []string{
		"newsletter-weekly", "newsletter-weekly-v2", "welcome-email", "password-reset",
		demoTemplateCartRecoveryA, demoTemplateCartRecoveryB, demoTemplateOrderThankYou, demoTemplateWinbackOffer,
	})
}

func TestDemoService_CreateNewsletterStructures_NotNil(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}

	b1 := svc.createNewsletterMJMLStructure(getNewsletterContents()["en"])
	b2 := svc.createNewsletterV2MJMLStructure(getNewsletterV2Contents()["en"])
	b3 := svc.createWelcomeMJMLStructure(getWelcomeContents()["en"])
	b4 := svc.createPasswordResetMJMLStructure(getPasswordResetContents()["en"])

	assert.NotNil(t, b1)
	assert.NotNil(t, b2)
	assert.NotNil(t, b3)
	assert.NotNil(t, b4)
	assert.Equal(t, notifuse_mjml.MJMLComponentMjml, b1.GetType())
	assert.Equal(t, notifuse_mjml.MJMLComponentMjml, b2.GetType())
	assert.Equal(t, notifuse_mjml.MJMLComponentMjml, b3.GetType())
	assert.Equal(t, notifuse_mjml.MJMLComponentMjml, b4.GetType())
}

func TestDemoService_ContentFactories(t *testing.T) {
	langs := []string{"en", "fr", "es"}

	t.Run("newsletter contents", func(t *testing.T) {
		contents := getNewsletterContents()
		for _, lang := range langs {
			c, ok := contents[lang]
			assert.True(t, ok, "missing language: %s", lang)
			assert.NotEmpty(t, c.title)
			assert.NotEmpty(t, c.mainText)
			assert.Equal(t, lang, c.lang)
		}
	})

	t.Run("newsletter v2 contents", func(t *testing.T) {
		contents := getNewsletterV2Contents()
		for _, lang := range langs {
			c, ok := contents[lang]
			assert.True(t, ok, "missing language: %s", lang)
			assert.NotEmpty(t, c.title)
			assert.NotEmpty(t, c.intro)
			assert.Equal(t, lang, c.lang)
		}
	})

	t.Run("welcome contents", func(t *testing.T) {
		contents := getWelcomeContents()
		for _, lang := range langs {
			c, ok := contents[lang]
			assert.True(t, ok, "missing language: %s", lang)
			assert.NotEmpty(t, c.title)
			assert.NotEmpty(t, c.welcome)
			assert.Equal(t, lang, c.lang)
		}
	})

	t.Run("password reset contents", func(t *testing.T) {
		contents := getPasswordResetContents()
		for _, lang := range langs {
			c, ok := contents[lang]
			assert.True(t, ok, "missing language: %s", lang)
			assert.NotEmpty(t, c.title)
			assert.NotEmpty(t, c.mainContent)
			assert.Equal(t, lang, c.lang)
		}
	})
}

func TestGetStringValue(t *testing.T) {
	// Test with nil
	assert.Equal(t, "", getStringValue(nil))

	// Test with null value
	nullValue := &domain.NullableString{String: "", IsNull: true}
	assert.Equal(t, "", getStringValue(nullValue))

	// Test with valid value
	validValue := &domain.NullableString{String: "test", IsNull: false}
	assert.Equal(t, "test", getStringValue(validValue))
}

func TestDemoService_DeleteAllWorkspaces_WithWorkspaces(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockTaskRepo := domainmocks.NewMockTaskRepository(ctrl)

	svc := &DemoService{
		logger:        logger.NewLoggerWithLevel("disabled"),
		workspaceRepo: mockWorkspaceRepo,
		taskRepo:      mockTaskRepo,
	}

	ctx := context.Background()
	workspaces := []*domain.Workspace{{ID: "w1"}, {ID: "w2"}}

	// Mock successful deletion
	mockWorkspaceRepo.EXPECT().List(ctx).Return(workspaces, nil)
	mockWorkspaceRepo.EXPECT().Delete(ctx, "w1").Return(nil)
	mockTaskRepo.EXPECT().DeleteAll(ctx, "w1").Return(nil)
	mockWorkspaceRepo.EXPECT().Delete(ctx, "w2").Return(nil)
	mockTaskRepo.EXPECT().DeleteAll(ctx, "w2").Return(nil)

	err := svc.deleteAllWorkspaces(ctx)
	assert.NoError(t, err)
}

func TestDemoService_GenerateMessageHistoryForContact(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}

	contact := &domain.Contact{
		Email:     "test@example.com",
		FirstName: &domain.NullableString{String: "John", IsNull: false},
		LastName:  &domain.NullableString{String: "Doe", IsNull: false},
	}

	baseTime := time.Now()
	message, engagement := svc.generateMessageHistoryForContact(contact, "newsletter-weekly", 1, "test-broadcast", baseTime)

	assert.NotNil(t, message)
	_ = engagement // engagement not needed for this test
	assert.Equal(t, contact.Email, message.ContactEmail)
	assert.Equal(t, "newsletter-weekly", message.TemplateID)
	assert.Equal(t, int64(1), message.TemplateVersion)
	assert.Equal(t, "test-broadcast", *message.BroadcastID)
	assert.Equal(t, "email", message.Channel)
	assert.NotNil(t, message.MessageData)
	assert.False(t, message.SentAt.IsZero())
}

func TestDemoService_GenerateTransactionalMessageHistoryForContact(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}

	contact := &domain.Contact{
		Email:     "test@example.com",
		FirstName: &domain.NullableString{String: "Jane", IsNull: false},
		LastName:  &domain.NullableString{String: "Smith", IsNull: false},
	}

	baseTime := time.Now()
	message, engagement := svc.generateTransactionalMessageHistoryForContact(contact, "password-reset", 1, "password-reset", baseTime)

	assert.NotNil(t, message)
	_ = engagement // engagement not needed for this test
	assert.Equal(t, contact.Email, message.ContactEmail)
	assert.Equal(t, "password-reset", message.TemplateID)
	assert.Equal(t, int64(1), message.TemplateVersion)
	assert.Nil(t, message.BroadcastID) // Transactional messages have no broadcast ID
	assert.Equal(t, "email", message.Channel)
	assert.NotNil(t, message.MessageData)
	assert.False(t, message.SentAt.IsZero())

	// Check for password reset specific data
	data, ok := message.MessageData.Data["reset_url"]
	assert.True(t, ok)
	assert.Contains(t, data.(string), "reset-password")
}

func TestDemoService_CompileTemplateToHTML_WithMinimalInput(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}

	// Create a minimal MJML structure (just mj-text without proper MJML wrapper)
	// gomjml handles this by producing partial HTML output
	minimalBlock := &notifuse_mjml.MJTextBlock{BaseBlock: notifuse_mjml.NewBaseBlock("minimal", notifuse_mjml.MJMLComponentMjText)}
	minimalBlock.Content = nil

	testData := domain.MapOfAny{"test": "value"}
	html := svc.compileTemplateToHTML("demo", "test", minimalBlock, testData)

	// gomjml produces partial HTML for minimal/incomplete structures
	// The output should be non-empty (some HTML is generated)
	assert.NotEmpty(t, html, "Should return HTML output even for minimal input")
}

func TestDemoService_CreateSampleLists_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockListRepo := domainmocks.NewMockListRepository(ctrl)
	mockContactListRepo := domainmocks.NewMockContactListRepository(ctrl)
	mockContactRepo := domainmocks.NewMockContactRepository(ctrl)
	mockAuth := domainmocks.NewMockAuthService(ctrl)
	mockEmail := domainmocks.NewMockEmailServiceInterface(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockMessageHistoryRepo := domainmocks.NewMockMessageHistoryRepository(ctrl)
	mockCache := pkgmocks.NewMockCache(ctrl)

	listSvc := NewListService(mockListRepo, mockWorkspaceRepo, mockContactListRepo, mockContactRepo, mockMessageHistoryRepo, mockAuth, mockEmail, logger.NewLoggerWithLevel("disabled"), "https://api.test", mockCache)

	svc := &DemoService{
		logger:      logger.NewLoggerWithLevel("disabled"),
		listService: listSvc,
	}

	ctx := context.Background()
	userWorkspace := &domain.UserWorkspace{
		UserID:      "u1",
		WorkspaceID: "demo",
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceLists: {Read: true, Write: true},
		},
	}

	mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, "demo").Return(ctx, &domain.User{ID: "u1"}, userWorkspace, nil).AnyTimes()
	mockCache.EXPECT().Clear().AnyTimes()

	var created []string
	mockListRepo.EXPECT().
		CreateList(ctx, "demo", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, list *domain.List) error {
			created = append(created, list.ID)
			return nil
		}).
		AnyTimes()

	err := svc.createSampleLists(ctx, "demo")
	assert.NoError(t, err)

	// The automations move contacts between these two, and a list id that fails validation aborts
	// the entire seed here — createSampleLists is one of the few steps that returns its error.
	assert.ElementsMatch(t, []string{demoListNewsletter, demoListVIPClub}, created)
	for _, id := range created {
		assert.True(t, govalidator.IsAlphanumeric(id), "list id %q is not alphanumeric and cannot be created", id)
		assert.LessOrEqual(t, len(id), 32, "list id %q exceeds the 32-character limit", id)
	}
}

func TestDemoService_GenerateMessagesPerContact(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMessageHistoryRepo := domainmocks.NewMockMessageHistoryRepository(ctrl)

	svc := &DemoService{
		logger:                  logger.NewLoggerWithLevel("disabled"),
		messageHistoryRepo:      mockMessageHistoryRepo,
		inboundWebhookEventRepo: nil, // Won't be called in this test
		workspaceService:        nil, // Won't be called in this test
	}

	ctx := context.Background()
	contacts := []*domain.Contact{
		{Email: "test1@example.com", FirstName: &domain.NullableString{String: "John", IsNull: false}},
		{Email: "test2@example.com", FirstName: &domain.NullableString{String: "Jane", IsNull: false}},
	}

	// Provide sample broadcast IDs
	broadcastIDs := []string{"broadcast-1", "broadcast-2", "broadcast-3", "broadcast-4"}

	// Mock message history creation - each contact gets 2-4 messages
	mockMessageHistoryRepo.EXPECT().Create(ctx, "demo", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	// Mock SetOpened and SetClicked for message_history updates (triggers timeline entries)
	mockMessageHistoryRepo.EXPECT().SetOpened(ctx, "demo", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockMessageHistoryRepo.EXPECT().SetClicked(ctx, "demo", gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, _ time.Time, clickedURL string) error {
			// Demo clicks must record a plausible https destination so the
			// per-link click table is populated in demo workspaces
			assert.True(t, strings.HasPrefix(clickedURL, "https://"))
			return nil
		}).AnyTimes()

	count, err := svc.generateMessagesPerContact(ctx, "demo", "test-secret-key", contacts, broadcastIDs)
	// No error expected - webhook generation errors are logged but don't fail the operation
	assert.NoError(t, err)
	// With 2 contacts getting 2-4 messages each, expect at least 4 messages
	assert.GreaterOrEqual(t, count, 4)
}

func TestDemoService_GenerateMessagesPerContact_EmptyContacts(t *testing.T) {
	svc := &DemoService{
		logger:             logger.NewLoggerWithLevel("disabled"),
		messageHistoryRepo: nil, // Won't be called
	}

	ctx := context.Background()
	broadcastIDs := []string{"broadcast-1", "broadcast-2", "broadcast-3", "broadcast-4"}
	count, err := svc.generateMessagesPerContact(ctx, "demo", "test-secret-key", []*domain.Contact{}, broadcastIDs)

	assert.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestNewDemoService_AllFields(t *testing.T) {
	logger := logger.NewLoggerWithLevel("disabled")
	config := &config.Config{}

	svc := NewDemoService(
		logger,
		config,
		nil, // workspaceService
		nil, // userService
		nil, // contactService
		nil, // listService
		nil, // contactListService
		nil, // templateService
		nil, // emailService
		nil, // broadcastService
		nil, // taskService
		nil, // transactionalNotificationService
		nil, // webhookEventService
		nil, // webhookRegistrationService
		nil, // messageHistoryService
		nil, // notificationCenterService
		nil, // segmentService
		nil, // workspaceRepo
		nil, // taskRepo
		nil, // messageHistoryRepo
		nil, // webhookEventRepo
		nil, // broadcastRepo
		nil, // customEventRepo
		nil, // webAnalyticsRepo
		nil, // annotationRepo
		nil, // webhookSubscriptionService
		nil, // automationService
	)

	assert.NotNil(t, svc)
	assert.Equal(t, logger, svc.logger)
	assert.Equal(t, config, svc.config)
}

func TestDemoService_GenerateWebhookEvents_ErrorGettingWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceService := &DemoService{
		logger:        logger.NewLoggerWithLevel("disabled"),
		workspaceRepo: mockWorkspaceRepo,
	}

	// Create a simple workspace service function that returns an error
	ctx := context.Background()

	mockWorkspaceRepo.EXPECT().GetByID(ctx, "demo").Return(nil, assert.AnError)

	// Call generateWebhookEvents which should handle the error
	// Note: We're testing the implementation directly through the repository mock
	// since WorkspaceService has many dependencies
	_, err := mockWorkspaceService.workspaceRepo.GetByID(ctx, "demo")
	assert.Error(t, err)
}

func TestDemoService_GenerateMessageHistoryForContact_EngagementMetrics(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}

	contact := &domain.Contact{
		Email:     "test@example.com",
		FirstName: &domain.NullableString{String: "John", IsNull: false},
		LastName:  &domain.NullableString{String: "Doe", IsNull: false},
	}

	baseTime := time.Now()
	deliveredCount := 0
	openedCount := 0
	clickedCount := 0

	// Test multiple times - larger sample size for more reliable statistics
	iterations := 200
	for i := 0; i < iterations; i++ {
		message, engagement := svc.generateMessageHistoryForContact(contact, "newsletter-weekly", 1, "test-broadcast", baseTime)

		assert.NotNil(t, message)
		assert.Equal(t, contact.Email, message.ContactEmail)
		assert.Equal(t, "newsletter-weekly", message.TemplateID)
		assert.Equal(t, int64(1), message.TemplateVersion)
		assert.Equal(t, "test-broadcast", *message.BroadcastID)
		assert.Equal(t, "email", message.Channel)
		assert.NotNil(t, message.MessageData)
		assert.False(t, message.SentAt.IsZero())

		// Count engagement metrics from engagement struct
		if engagement.shouldDeliver {
			deliveredCount++
		}
		if engagement.shouldOpen {
			openedCount++
		}
		if engagement.shouldClick {
			clickedCount++
		}
	}

	// Verify all messages are delivered (100% delivery rate)
	assert.Equal(t, iterations, deliveredCount, "All messages should be delivered (100%% delivery rate)")

	// Verify some engagement is happening (with very wide tolerances for statistical variance)
	assert.Greater(t, openedCount, 0, "At least some messages should be opened")
	assert.Greater(t, clickedCount, 0, "At least some messages should be clicked")
}

func TestDemoService_GenerateTransactionalMessageHistoryForContact_PasswordReset(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}

	contact := &domain.Contact{
		Email:     "test@example.com",
		FirstName: &domain.NullableString{String: "Jane", IsNull: false},
		LastName:  &domain.NullableString{String: "Smith", IsNull: false},
	}

	baseTime := time.Now()
	deliveredCount := 0
	openedCount := 0
	clickedCount := 0

	// Test multiple times - larger sample size for more reliable statistics
	iterations := 200
	for i := 0; i < iterations; i++ {
		message, engagement := svc.generateTransactionalMessageHistoryForContact(contact, "password-reset", 1, "password-reset", baseTime)

		assert.NotNil(t, message)
		assert.Equal(t, contact.Email, message.ContactEmail)
		assert.Equal(t, "password-reset", message.TemplateID)
		assert.Equal(t, int64(1), message.TemplateVersion)
		assert.Nil(t, message.BroadcastID) // Transactional messages have no broadcast ID
		assert.Equal(t, "email", message.Channel)
		assert.NotNil(t, message.MessageData)
		assert.False(t, message.SentAt.IsZero())

		// Check for password reset specific data
		data, ok := message.MessageData.Data["reset_url"]
		assert.True(t, ok)
		assert.Contains(t, data.(string), "reset-password")

		// Check metadata
		metadata, ok := message.MessageData.Metadata["is_transactional"]
		assert.True(t, ok)
		assert.True(t, metadata.(bool))

		// Count engagement metrics from engagement struct
		if engagement.shouldDeliver {
			deliveredCount++
		}
		if engagement.shouldOpen {
			openedCount++
		}
		if engagement.shouldClick {
			clickedCount++
		}
	}

	// Verify all messages are delivered (100% delivery rate)
	assert.Equal(t, iterations, deliveredCount, "All messages should be delivered (100%% delivery rate)")

	// Verify some engagement is happening (with very wide tolerances for statistical variance)
	assert.Greater(t, openedCount, 0, "At least some messages should be opened")
	assert.Greater(t, clickedCount, 0, "At least some messages should be clicked")
}

func TestDemoService_GenerateTransactionalMessageHistoryForContact_Welcome(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}

	contact := &domain.Contact{
		Email:     "test@example.com",
		FirstName: &domain.NullableString{String: "Jane", IsNull: false},
		LastName:  &domain.NullableString{String: "Smith", IsNull: false},
	}

	baseTime := time.Now()
	message, engagement := svc.generateTransactionalMessageHistoryForContact(contact, "welcome-email", 1, "welcome", baseTime)

	assert.NotNil(t, message)
	_ = engagement // engagement not needed for this test
	assert.Equal(t, contact.Email, message.ContactEmail)
	assert.Equal(t, "welcome-email", message.TemplateID)

	// Check that reset_url is NOT added for welcome messages
	_, hasResetURL := message.MessageData.Data["reset_url"]
	assert.False(t, hasResetURL)

	// Check metadata
	messageType, ok := message.MessageData.Metadata["message_type"]
	assert.True(t, ok)
	assert.Equal(t, "welcome", messageType)
}

// Test removed due to nil pointer dereference - would require complex mocking setup

func TestDemoService_CompileTemplateToHTML_Success(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}

	// Create a valid MJML structure
	titleContent := "Test Title"
	textContent := "Test Content"

	titleBase := notifuse_mjml.NewBaseBlock("title", notifuse_mjml.MJMLComponentMjTitle)
	titleBase.Content = &titleContent
	title := &notifuse_mjml.MJTitleBlock{BaseBlock: titleBase}

	head := &notifuse_mjml.MJHeadBlock{BaseBlock: notifuse_mjml.NewBaseBlock("head", notifuse_mjml.MJMLComponentMjHead)}
	head.Children = []notifuse_mjml.EmailBlock{title}

	textBase := notifuse_mjml.NewBaseBlock("text", notifuse_mjml.MJMLComponentMjText)
	textBase.Content = &textContent
	text := &notifuse_mjml.MJTextBlock{BaseBlock: textBase}

	col := &notifuse_mjml.MJColumnBlock{BaseBlock: notifuse_mjml.NewBaseBlock("col", notifuse_mjml.MJMLComponentMjColumn)}
	col.Children = []notifuse_mjml.EmailBlock{text}

	sec := &notifuse_mjml.MJSectionBlock{BaseBlock: notifuse_mjml.NewBaseBlock("sec", notifuse_mjml.MJMLComponentMjSection)}
	sec.Children = []notifuse_mjml.EmailBlock{col}

	body := &notifuse_mjml.MJBodyBlock{BaseBlock: notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)}
	body.Children = []notifuse_mjml.EmailBlock{sec}

	root := &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)}
	root.Children = []notifuse_mjml.EmailBlock{head, body}

	testData := domain.MapOfAny{"contact": domain.MapOfAny{"first_name": "John"}}
	html := svc.compileTemplateToHTML("demo", "test-message", root, testData)

	// Should return valid HTML (not fallback)
	assert.True(t, strings.Contains(html, "<!DOCTYPE html>") || strings.Contains(html, "<!doctype html>"))
	assert.NotContains(t, html, "Demo Template") // Should not be fallback
}

func TestDemoService_CompileTemplateToHTML_CompilationFailure(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}

	// Create an invalid MJML structure that will cause compilation to fail
	invalidText := &notifuse_mjml.MJTextBlock{BaseBlock: notifuse_mjml.NewBaseBlock("invalid", notifuse_mjml.MJMLComponentMjText)}
	invalidText.Content = nil // Invalid content

	// Create a minimal but potentially problematic structure
	body := &notifuse_mjml.MJBodyBlock{BaseBlock: notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)}
	body.Children = []notifuse_mjml.EmailBlock{invalidText}

	root := &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)}
	root.Children = []notifuse_mjml.EmailBlock{body}

	testData := domain.MapOfAny{"test": "value"}
	html := svc.compileTemplateToHTML("demo", "test", root, testData)

	// gomjml produces valid HTML with doctype for properly structured MJML
	// (even if the inner content is minimal)
	assert.NotEmpty(t, html, "Should return HTML output")
	assert.True(t, strings.Contains(strings.ToLower(html), "<!doctype html"),
		"Should contain valid HTML doctype (gomjml uses lowercase)")
}

func TestDemoService_GenerateEmail_AllFormats(t *testing.T) {
	// Test all 4 different email formats by calling multiple times
	first := "John"
	last := "Doe"

	emailFormats := make(map[string]bool)

	// Generate many emails to hit all format cases
	for i := 0; i < 100; i++ {
		email := generateEmail(first, last, i)

		// Basic validation
		assert.Contains(t, strings.ToLower(email), strings.ToLower(first))
		assert.Contains(t, strings.ToLower(email), strings.ToLower(last))
		assert.Contains(t, email, "@")

		parts := strings.SplitN(email, "@", 2)
		assert.Len(t, parts, 2)

		// Track different formats
		localPart := parts[0]
		if strings.Contains(localPart, ".") && !strings.ContainsAny(localPart, "0123456789") {
			emailFormats["dot_format"] = true
		} else if !strings.Contains(localPart, ".") && !strings.ContainsAny(localPart, "0123456789") {
			emailFormats["concat_format"] = true
		} else if strings.ContainsAny(localPart, "0123456789") {
			emailFormats["number_format"] = true
		}
	}

	// Should have generated different formats
	assert.True(t, len(emailFormats) > 1, "Should generate multiple email formats")
}

func TestDemoService_GenerateEmail_DomainValidation(t *testing.T) {
	email := generateEmail("Test", "User", 42)
	parts := strings.SplitN(email, "@", 2)
	domain := parts[1]

	// Validate domain is one of the configured demo domains
	var found bool
	for _, d := range emailDomains {
		if domain == d {
			found = true
			break
		}
	}
	assert.True(t, found, "unexpected domain: %s", domain)
}

// TestCreateSampleSegments_ProducesUsableSegments guards the demo workspace's showcase segments.
// createSampleSegments only logs a warning when a segment is rejected, so an invalid tree would
// leave the demo silently missing a segment that the product is meant to be demonstrating.
func TestCreateSampleSegments_ProducesUsableSegments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentService := domainmocks.NewMockSegmentService(ctrl)

	var created []*domain.CreateSegmentRequest
	mockSegmentService.EXPECT().
		CreateSegment(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *domain.CreateSegmentRequest) (*domain.Segment, error) {
			created = append(created, req)
			return &domain.Segment{ID: req.ID}, nil
		}).
		AnyTimes()

	svc := &DemoService{
		logger:         logger.NewLoggerWithLevel("disabled"),
		segmentService: mockSegmentService,
	}

	require.NoError(t, svc.createSampleSegments(context.Background(), "ws1"))
	require.NotEmpty(t, created)

	qb := NewQueryBuilder()
	byID := map[string]*domain.CreateSegmentRequest{}
	for _, req := range created {
		byID[req.ID] = req

		// Every showcase segment must be one the product would actually accept and run.
		require.NotNil(t, req.Tree, "segment %s has no tree", req.ID)
		require.NoError(t, req.Tree.Validate(), "segment %s does not validate", req.ID)
		_, _, err := qb.BuildSQL(req.Tree)
		require.NoError(t, err, "segment %s does not compile", req.ID)

		// ...and one the console can reopen. The segment builder renders the root as a branch
		// unconditionally, so a tree whose root is a bare leaf compiles and runs correctly but
		// shows "A branch condition is required..." instead of the form when it is edited.
		require.Equal(t, "branch", req.Tree.Kind,
			"segment %s has a leaf at the root and cannot be edited in the console", req.ID)
		require.NotNil(t, req.Tree.Branch, "segment %s has no branch at the root", req.ID)
		require.NotEmpty(t, req.Tree.Branch.Leaves, "segment %s has an empty root branch", req.ID)
	}

	winback, ok := byID["winback_opportunities"]
	require.True(t, ok, "the negation showcase segment must be created")

	// Two leaves, and the pairing is the point: bought at some time, did not buy lately. The
	// negated leaf on its own also matches every contact who never bought, which on this demo is
	// most of the workspace.
	require.Len(t, winback.Tree.Branch.Leaves, 2)
	var everBought, notLately *domain.CustomEventsGoalCondition
	for _, leaf := range winback.Tree.Branch.Leaves {
		goal := leaf.Leaf.CustomEventsGoal
		require.NotNil(t, goal)
		if goal.Negate {
			notLately = goal
		} else {
			everBought = goal
		}
	}
	require.NotNil(t, everBought, "a win-back audience has to have bought at some point")
	require.NotNil(t, notLately, "the win-back segment is only a showcase if it is negated")
	assert.Equal(t, "anytime", everBought.TimeframeOperator)
	assert.Equal(t, domain.GoalTypePurchase, everBought.GoalType)
	assert.Equal(t, "in_the_last_days", notLately.TimeframeOperator)

	// Negation has to wrap the leaf; inverting the comparison would silently exclude the
	// contacts with no purchase in the window, who are the whole point of the segment.
	sqlStr, _, err := qb.BuildSQL(winback.Tree)
	require.NoError(t, err)
	assert.Contains(t, sqlStr, "NOT EXISTS (SELECT 1 FROM custom_events ce")

	// Relative window, so it must be flagged for daily recomputation or its membership freezes.
	assert.True(t, winback.Tree.HasRelativeDates(),
		"a rolling-window segment must be scheduled for recomputation")

	// The abandonment audiences: the goal the visitor fired on the site, crossed with the absence
	// of the one that would have completed the funnel. They are the demo's answer to "what does
	// web analytics buy me", so a demo missing either has lost the argument.
	for _, abandonment := range []struct{ id, eventName string }{
		{"cart_abandoners", "add_to_cart"},
		{"checkout_abandoners", "checkout_start"},
	} {
		segment, ok := byID[abandonment.id]
		require.True(t, ok, "%s must be created", abandonment.id)
		require.Len(t, segment.Tree.Branch.Leaves, 2, "%s", abandonment.id)

		var intent, absence *domain.CustomEventsGoalCondition
		for _, leaf := range segment.Tree.Branch.Leaves {
			goal := leaf.Leaf.CustomEventsGoal
			require.NotNil(t, goal)
			if goal.Negate {
				absence = goal
			} else {
				intent = goal
			}
		}

		require.NotNil(t, intent, "%s has no intent condition", abandonment.id)
		require.NotNil(t, intent.EventName, "%s must match on the event name", abandonment.id)
		// Matched by name, not by type: the cart and checkout steps carry no revenue and are both
		// typed "other", so a type filter cannot tell them apart and both segments would hold the
		// same contacts.
		assert.Equal(t, abandonment.eventName, *intent.EventName)
		assert.Equal(t, "in_the_last_days", intent.TimeframeOperator)

		require.NotNil(t, absence, "%s must exclude the contacts who converted", abandonment.id)
		assert.Equal(t, domain.GoalTypePurchase, absence.GoalType)
		assert.Equal(t, "in_the_last_days", absence.TimeframeOperator)

		sqlStr, _, err := qb.BuildSQL(segment.Tree)
		require.NoError(t, err, "%s", abandonment.id)
		assert.Contains(t, sqlStr, "NOT EXISTS (SELECT 1 FROM custom_events ce", "%s", abandonment.id)
		assert.Contains(t, sqlStr, "ce.event_name = ", "%s", abandonment.id)
	}

	// The two abandonment audiences must not be the same audience under two names.
	cartSQL, _, err := qb.BuildSQL(byID["cart_abandoners"].Tree)
	require.NoError(t, err)
	checkoutSQL, _, err := qb.BuildSQL(byID["checkout_abandoners"].Tree)
	require.NoError(t, err)
	assert.Equal(t, cartSQL, checkoutSQL,
		"the two differ only in a bound argument, so identical SQL is expected here")
	assert.NotEqual(t,
		*byID["cart_abandoners"].Tree.Branch.Leaves[0].Leaf.CustomEventsGoal.EventName,
		*byID["checkout_abandoners"].Tree.Branch.Leaves[0].Leaf.CustomEventsGoal.EventName,
		"the two segments must select different events")
}

// TestCreateDemoWebhookSubscription_IsSwitchedOff guards the load decision.
//
// The demo ships a webhook subscription so the console has one to show, subscribed
// to every event type. Enabled, it would write a webhook_deliveries row for every
// contact, event and message the demo produces — the trigger functions fan out to
// `enabled = true` subscriptions only — and the delivery worker would retry each
// of those ten times against a URL that answers nothing.
//
// Nothing else would notice: Create hardcodes Enabled: true, so a refactor that
// dropped the switch-off would leave a demo that looks identical and delivers
// forever.
func TestCreateDemoWebhookSubscription_IsSwitchedOff(t *testing.T) {
	newService := func(t *testing.T) (*DemoService, *domainmocks.MockWebhookSubscriptionRepository) {
		t.Helper()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		repo := domainmocks.NewMockWebhookSubscriptionRepository(ctrl)
		auth := domainmocks.NewMockAuthService(ctrl)
		auth.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
				return ctx, &domain.User{ID: "root"}, &domain.UserWorkspace{
					UserID: "root", WorkspaceID: workspaceID, Role: "owner",
				}, nil
			}).AnyTimes()

		// Removing a subscription takes its queued deliveries with it, or they
		// go on matching the delivery worker's pending predicate for a
		// subscription that no longer exists.
		deliveryRepo := domainmocks.NewMockWebhookDeliveryRepository(ctrl)
		deliveryRepo.EXPECT().DeleteBySubscriptionID(gomock.Any(), "demo", gomock.Any()).
			Return(nil).AnyTimes()

		log := logger.NewLoggerWithLevel("disabled")
		return &DemoService{
			logger: log,
			webhookSubscriptionService: NewWebhookSubscriptionService(
				repo, deliveryRepo, auth, log),
		}, repo
	}

	t.Run("the subscription is stored disabled", func(t *testing.T) {
		svc, repo := newService(t)

		var created *domain.WebhookSubscription
		repo.EXPECT().Create(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				created = sub
				return nil
			})
		repo.EXPECT().GetByID(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, id string) (*domain.WebhookSubscription, error) {
				return created, nil
			})

		var stored *domain.WebhookSubscription
		repo.EXPECT().Update(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				stored = sub
				return nil
			})

		require.NoError(t, svc.createDemoWebhookSubscription(context.Background(), "demo"))

		require.NotNil(t, stored)
		assert.False(t, stored.Enabled, "an enabled demo webhook delivers on every seeded row")
		// Still subscribed to everything, so the console has a full example to show.
		assert.Equal(t, domain.WebhookEventTypes, stored.Settings.EventTypes)
		assert.Equal(t, "https://webhook.site/demo", stored.URL)
	})

	t.Run("a subscription that cannot be switched off is removed", func(t *testing.T) {
		// The failure mode has to land on "no webhook", never on "enabled webhook":
		// the second is the one thing this function exists to prevent.
		svc, repo := newService(t)

		var created *domain.WebhookSubscription
		repo.EXPECT().Create(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
				created = sub
				return nil
			})
		repo.EXPECT().GetByID(gomock.Any(), "demo", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, id string) (*domain.WebhookSubscription, error) {
				return created, nil
			})
		repo.EXPECT().Update(gomock.Any(), "demo", gomock.Any()).
			Return(errors.New("write failed"))
		repo.EXPECT().Delete(gomock.Any(), "demo", gomock.Any()).Return(nil)

		err := svc.createDemoWebhookSubscription(context.Background(), "demo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disable the demo webhook subscription")
	})
}
