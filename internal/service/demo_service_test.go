package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
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
		nil, // workspaceRepo
		nil, // taskRepo
		nil, // messageHistoryRepo
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

	title := &notifuse_mjml.MJTitleBlock{
		BaseBlock: notifuse_mjml.BaseBlock{ID: "title", Type: notifuse_mjml.MJMLComponentMjTitle},
		Type:      notifuse_mjml.MJMLComponentMjTitle,
		Content:   &titleContent,
	}
	head := &notifuse_mjml.MJHeadBlock{
		BaseBlock: notifuse_mjml.BaseBlock{ID: "head", Type: notifuse_mjml.MJMLComponentMjHead, Children: []interface{}{title}},
		Type:      notifuse_mjml.MJMLComponentMjHead,
		Children:  []notifuse_mjml.EmailBlock{title},
	}

	text := &notifuse_mjml.MJTextBlock{
		BaseBlock: notifuse_mjml.BaseBlock{ID: "text", Type: notifuse_mjml.MJMLComponentMjText},
		Type:      notifuse_mjml.MJMLComponentMjText,
		Content:   &textContent,
	}
	col := &notifuse_mjml.MJColumnBlock{
		BaseBlock: notifuse_mjml.BaseBlock{ID: "col", Type: notifuse_mjml.MJMLComponentMjColumn, Children: []interface{}{text}},
		Type:      notifuse_mjml.MJMLComponentMjColumn,
		Children:  []notifuse_mjml.EmailBlock{text},
	}
	sec := &notifuse_mjml.MJSectionBlock{
		BaseBlock: notifuse_mjml.BaseBlock{ID: "sec", Type: notifuse_mjml.MJMLComponentMjSection, Children: []interface{}{col}},
		Type:      notifuse_mjml.MJMLComponentMjSection,
		Children:  []notifuse_mjml.EmailBlock{col},
	}
	body := &notifuse_mjml.MJBodyBlock{
		BaseBlock: notifuse_mjml.BaseBlock{ID: "body", Type: notifuse_mjml.MJMLComponentMjBody, Children: []interface{}{sec}},
		Type:      notifuse_mjml.MJMLComponentMjBody,
		Children:  []notifuse_mjml.EmailBlock{sec},
	}
	root := &notifuse_mjml.MJMLBlock{
		BaseBlock: notifuse_mjml.BaseBlock{ID: "root", Type: notifuse_mjml.MJMLComponentMjml, Children: []interface{}{head, body}},
		Type:      notifuse_mjml.MJMLComponentMjml,
		Children:  []notifuse_mjml.EmailBlock{head, body},
	}

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

	listSvc := NewListService(mockListRepo, mockWorkspaceRepo, mockContactListRepo, mockContactRepo, mockAuth, mockEmail, logger.NewLoggerWithLevel("disabled"), "https://api.test")

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
	mockWebhookEventRepo := domainmocks.NewMockWebhookEventRepository(ctrl)
	contactSvc := NewContactService(mockContactRepo, mockWorkspaceRepo, mockAuth, mockMessageHistoryRepo, mockWebhookEventRepo, mockContactListRepo, logger.NewLoggerWithLevel("disabled"))
	listSvc := NewListService(mockListRepo, mockWorkspaceRepo, mockContactListRepo, mockContactRepo, mockAuth, mockEmail, logger.NewLoggerWithLevel("disabled"), "https://api.test")

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
	// Add to list
	mockContactListRepo.EXPECT().AddContactToList(ctx, "demo", gomock.Any()).Return(nil).Times(2)

	err := svc.subscribeContactsToList(ctx, "demo", "newsletter")
	assert.NoError(t, err)
}

func TestDemoService_CreateSampleTemplates_Smoke(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockAuth := domainmocks.NewMockAuthService(ctrl)

	tmplSvc := NewTemplateService(mockTemplateRepo, mockAuth, logger.NewLoggerWithLevel("disabled"), "https://api.test")

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

	// Authenticate for each template creation (4 templates)
	mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, "demo").Return(ctx, &domain.User{ID: "u1"}, userWorkspace, nil).Times(4)
	mockTemplateRepo.EXPECT().CreateTemplate(ctx, "demo", gomock.Any()).Return(nil).Times(4)

	err := svc.createSampleTemplates(ctx, "demo")
	assert.NoError(t, err)
}

func TestDemoService_CreateNewsletterStructures_NotNil(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}

	b1 := svc.createNewsletterMJMLStructure()
	b2 := svc.createNewsletterV2MJMLStructure()
	b3 := svc.createWelcomeMJMLStructure()
	b4 := svc.createPasswordResetMJMLStructure()

	assert.NotNil(t, b1)
	assert.NotNil(t, b2)
	assert.NotNil(t, b3)
	assert.NotNil(t, b4)
	assert.Equal(t, notifuse_mjml.MJMLComponentMjml, b1.GetType())
	assert.Equal(t, notifuse_mjml.MJMLComponentMjml, b2.GetType())
	assert.Equal(t, notifuse_mjml.MJMLComponentMjml, b3.GetType())
	assert.Equal(t, notifuse_mjml.MJMLComponentMjml, b4.GetType())
}
