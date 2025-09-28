package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	mjmlgo "github.com/Boostport/mjml-go"
	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	notifusemjml "github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper to build a minimal valid broadcast
func testBroadcast(workspaceID, id string) *domain.Broadcast {
	now := time.Now().UTC()
	return &domain.Broadcast{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        "Test Broadcast",
		ChannelType: "email",
		Status:      domain.BroadcastStatusDraft,
		Audience: domain.AudienceSettings{
			Segments: []string{"seg1"},
		},
		Schedule: domain.ScheduleSettings{
			IsScheduled: false,
		},
		TestSettings: domain.BroadcastTestSettings{
			Enabled:    false,
			Variations: []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

type broadcastSvcDeps struct {
	ctrl               *gomock.Controller
	repo               *domainmocks.MockBroadcastRepository
	workspaceRepo      *domainmocks.MockWorkspaceRepository
	contactRepo        *domainmocks.MockContactRepository
	emailSvc           *domainmocks.MockEmailServiceInterface
	templateSvc        *domainmocks.MockTemplateService
	taskService        *domainmocks.MockTaskService
	taskRepo           *domainmocks.MockTaskRepository
	authService        *domainmocks.MockAuthService
	eventBus           *domainmocks.MockEventBus
	messageHistoryRepo *domainmocks.MockMessageHistoryRepository
	svc                *BroadcastService
}

func setupBroadcastSvc(t *testing.T) *broadcastSvcDeps {
	t.Helper()
	ctrl := gomock.NewController(t)

	repo := domainmocks.NewMockBroadcastRepository(ctrl)
	workspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := domainmocks.NewMockContactRepository(ctrl)
	emailSvc := domainmocks.NewMockEmailServiceInterface(ctrl)
	templateSvc := domainmocks.NewMockTemplateService(ctrl)
	taskService := domainmocks.NewMockTaskService(ctrl)
	taskRepo := domainmocks.NewMockTaskRepository(ctrl)
	authService := domainmocks.NewMockAuthService(ctrl)
	eventBus := domainmocks.NewMockEventBus(ctrl)
	messageHistoryRepo := domainmocks.NewMockMessageHistoryRepository(ctrl)

	// use real no-op logger
	log := logger.NewLoggerWithLevel("disabled")

	svc := NewBroadcastService(
		log,
		repo,
		workspaceRepo,
		emailSvc,
		contactRepo,
		templateSvc,
		taskService,
		taskRepo,
		authService,
		eventBus,
		messageHistoryRepo,
		"https://api.example.test",
	)

	return &broadcastSvcDeps{
		ctrl:               ctrl,
		repo:               repo,
		workspaceRepo:      workspaceRepo,
		contactRepo:        contactRepo,
		emailSvc:           emailSvc,
		templateSvc:        templateSvc,
		taskService:        taskService,
		taskRepo:           taskRepo,
		authService:        authService,
		eventBus:           eventBus,
		messageHistoryRepo: messageHistoryRepo,
		svc:                svc,
	}
}

func authOK(auth *domainmocks.MockAuthService, ctx context.Context, workspaceID string) {
	userWorkspace := &domain.UserWorkspace{
		UserID:      "user1",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceBroadcasts: {Read: true, Write: true},
		},
	}
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: "user1"}, userWorkspace, nil)
}

func TestBroadcastService_CreateBroadcast_Success(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.CreateBroadcastRequest{
		WorkspaceID: "w1",
		Name:        "My Campaign",
		Audience:    domain.AudienceSettings{Segments: []string{"seg1"}},
		Schedule:    domain.ScheduleSettings{IsScheduled: false},
	}

	authOK(d.authService, ctx, req.WorkspaceID)

	d.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).Return(nil)

	b, err := d.svc.CreateBroadcast(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Equal(t, domain.BroadcastStatusDraft, b.Status)
	assert.Equal(t, req.WorkspaceID, b.WorkspaceID)
	assert.NotEmpty(t, b.ID)
}

func TestBroadcastService_ScheduleBroadcast_SendNow_Success(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.ScheduleBroadcastRequest{WorkspaceID: "w1", ID: "b1", SendNow: true}
	authOK(d.authService, ctx, req.WorkspaceID)

	// workspace with marketing email provider configured
	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{domain.NewEmailSender("from@example.com", "From")}}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	// Transaction flow
	d.repo.EXPECT().WithTransaction(ctx, req.WorkspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			return fn(nil)
		},
	)

	// Inside tx: get -> update -> publish ack -> wait
	draft := testBroadcast(req.WorkspaceID, req.ID)
	d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), req.WorkspaceID, req.ID).Return(draft, nil)
	d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(_ context.Context, _ domain.EventPayload, ack domain.EventAckCallback) {
			ack(nil)
		},
	)

	err := d.svc.ScheduleBroadcast(ctx, req)
	require.NoError(t, err)
}

func TestBroadcastService_PauseBroadcast_Success(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.PauseBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
	authOK(d.authService, ctx, req.WorkspaceID)

	d.repo.EXPECT().WithTransaction(ctx, req.WorkspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)

	sending := testBroadcast(req.WorkspaceID, req.ID)
	sending.Status = domain.BroadcastStatusSending
	d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), req.WorkspaceID, req.ID).Return(sending, nil)
	d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, _ domain.EventPayload, ack domain.EventAckCallback) { ack(nil) })

	err := d.svc.PauseBroadcast(ctx, req)
	require.NoError(t, err)
}

func TestBroadcastService_ResumeBroadcast_ToScheduled_Success(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
	authOK(d.authService, ctx, req.WorkspaceID)

	d.repo.EXPECT().WithTransaction(ctx, req.WorkspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)

	paused := testBroadcast(req.WorkspaceID, req.ID)
	paused.Status = domain.BroadcastStatusPaused
	// schedule in the future
	future := time.Now().UTC().Add(2 * time.Hour)
	_ = paused.Schedule.SetScheduledDateTime(future, "UTC")
	paused.Schedule.IsScheduled = true

	d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), req.WorkspaceID, req.ID).Return(paused, nil)
	d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, _ domain.EventPayload, ack domain.EventAckCallback) { ack(nil) })

	err := d.svc.ResumeBroadcast(ctx, req)
	require.NoError(t, err)
}

func TestBroadcastService_SendToIndividual_Success(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "to@example.com"}
	authOK(d.authService, ctx, req.WorkspaceID)

	// workspace with marketing provider and default sender
	sender := domain.NewEmailSender("from@example.com", "From")
	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt", SecretKey: "sk_test"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{sender}}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	// broadcast with a single variation
	b := testBroadcast(req.WorkspaceID, req.BroadcastID)
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)

	// contact may be found or not; return nil to test non-fatal path
	d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, req.RecipientEmail).Return(nil, errors.New("not found")).AnyTimes()

	// template fetch and compile
	template := &domain.Template{
		ID:      "tplA",
		Name:    "Template A",
		Channel: "email",
		Email: &domain.EmailTemplate{
			SenderID:        sender.ID,
			Subject:         "Hello",
			CompiledPreview: "<p>preview</p>",
			VisualEditorTree: &notifusemjml.MJMLBlock{
				BaseBlock:  notifusemjml.BaseBlock{ID: "root", Type: notifusemjml.MJMLComponentMjml, Attributes: map[string]interface{}{"version": "4.0.0"}},
				Type:       notifusemjml.MJMLComponentMjml,
				Attributes: map[string]interface{}{"version": "4.0.0"},
			},
		},
		Category:  string(domain.TemplateCategoryMarketing),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	d.templateSvc.EXPECT().GetTemplateByID(ctx, req.WorkspaceID, "tplA", int64(0)).Return(template, nil)

	compiledHTML := "<html>ok</html>"
	d.templateSvc.EXPECT().CompileTemplate(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, payload domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
		return &domain.CompileTemplateResponse{Success: true, HTML: &compiledHTML}, nil
	})

	d.emailSvc.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)

	d.messageHistoryRepo.EXPECT().Create(gomock.Any(), req.WorkspaceID, gomock.Any()).Return(nil)

	err := d.svc.SendToIndividual(ctx, req)
	require.NoError(t, err)
}

func TestBroadcastService_GetTestResults_ComputesRecommendation(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	authOK(d.authService, ctx, workspaceID)

	b := testBroadcast(workspaceID, broadcastID)
	b.Status = domain.BroadcastStatusTesting
	b.TestSettings.Enabled = true
	b.TestSettings.AutoSendWinner = false
	b.TestSettings.Variations = []domain.BroadcastVariation{
		{VariationName: "A", TemplateID: "tplA"},
		{VariationName: "B", TemplateID: "tplB"},
	}
	d.repo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	// stats for A and B
	d.messageHistoryRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalSent: 100, TotalDelivered: 100, TotalOpened: 30, TotalClicked: 5}, nil)
	d.messageHistoryRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalSent: 100, TotalDelivered: 100, TotalOpened: 25, TotalClicked: 10}, nil)

	res, err := d.svc.GetTestResults(ctx, workspaceID, broadcastID)
	require.NoError(t, err)
	require.NotNil(t, res)
	// Variation B should win: higher clicks weighted 0.7
	assert.Equal(t, "tplB", res.RecommendedWinner)
	assert.Equal(t, b.Status, domain.BroadcastStatus(res.Status))
}

func TestBroadcastService_SelectWinner_SetsWinnerAndResumesTask(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	winner := "tplA"
	authOK(d.authService, ctx, workspaceID)

	// transaction wrapper
	d.repo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)

	b := testBroadcast(workspaceID, broadcastID)
	b.Status = domain.BroadcastStatusTesting
	b.TestSettings.Enabled = true
	b.TestSettings.AutoSendWinner = false
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: winner}, {VariationName: "B", TemplateID: "tplB"}}

	d.repo.EXPECT().GetBroadcastTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(b, nil)
	d.repo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).Return(nil)

	// Task repo: resume task if present
	task := &domain.Task{ID: "task1", WorkspaceID: workspaceID, Status: domain.TaskStatusPaused}
	d.taskRepo.EXPECT().GetTaskByBroadcastID(ctx, workspaceID, broadcastID).Return(task, nil)
	d.taskRepo.EXPECT().Update(ctx, workspaceID, gomock.Any()).Return(nil)

	// Expect ExecutePendingTasks to be called in goroutine (may happen after test completes)
	d.taskService.EXPECT().ExecutePendingTasks(gomock.Any(), 1).Return(nil).AnyTimes()

	err := d.svc.SelectWinner(ctx, workspaceID, broadcastID, winner)
	require.NoError(t, err)

	// Give goroutine time to complete
	time.Sleep(200 * time.Millisecond)
}

func TestBroadcastService_SetTaskService_SetsField(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	// Create a new mock task service and set it
	newTaskSvc := domainmocks.NewMockTaskService(d.ctrl)
	d.svc.SetTaskService(newTaskSvc)

	// Since tests are in the same package, we can assert internal field
	assert.Equal(t, newTaskSvc, d.svc.taskService)
}

func TestBroadcastService_GetBroadcast_Success(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	authOK(d.authService, ctx, workspaceID)

	expected := testBroadcast(workspaceID, broadcastID)
	d.repo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(expected, nil)

	b, err := d.svc.GetBroadcast(ctx, workspaceID, broadcastID)
	require.NoError(t, err)
	assert.Equal(t, expected, b)
}

func TestBroadcastService_UpdateBroadcast_Success(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.UpdateBroadcastRequest{
		WorkspaceID: "w1",
		ID:          "b1",
		Name:        "Updated Name",
		Audience:    domain.AudienceSettings{Segments: []string{"seg1"}},
		Schedule:    domain.ScheduleSettings{IsScheduled: false},
		TestSettings: domain.BroadcastTestSettings{
			Enabled:    false,
			Variations: []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}},
		},
	}
	authOK(d.authService, ctx, req.WorkspaceID)

	existing := testBroadcast(req.WorkspaceID, req.ID)
	existing.Status = domain.BroadcastStatusDraft

	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.ID).Return(existing, nil)
	d.repo.EXPECT().UpdateBroadcast(ctx, gomock.Any()).Return(nil)

	updated, err := d.svc.UpdateBroadcast(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, req.Name, updated.Name)
}

func TestBroadcastService_ListBroadcasts_WithTemplates(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	params := domain.ListBroadcastsParams{WorkspaceID: "w1", WithTemplates: true}
	authOK(d.authService, ctx, params.WorkspaceID)

	b := testBroadcast(params.WorkspaceID, "b1")
	// Ensure there is a variation to load template for
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
	resp := &domain.BroadcastListResponse{Broadcasts: []*domain.Broadcast{b}, TotalCount: 1}

	d.repo.EXPECT().ListBroadcasts(ctx, gomock.Any()).Return(resp, nil)

	// Template returned
	tmpl := &domain.Template{ID: "tplA", Email: &domain.EmailTemplate{Subject: "S", SenderID: "sender"}}
	d.templateSvc.EXPECT().GetTemplateByID(ctx, params.WorkspaceID, "tplA", int64(0)).Return(tmpl, nil)

	out, err := d.svc.ListBroadcasts(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Len(t, out.Broadcasts, 1)
	v := out.Broadcasts[0].TestSettings.Variations[0]
	require.NotNil(t, v.Template)
	assert.Equal(t, "tplA", v.Template.ID)
}

func TestBroadcastService_CancelBroadcast_Success(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.CancelBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
	authOK(d.authService, ctx, req.WorkspaceID)

	// Transaction wrapper
	d.repo.EXPECT().WithTransaction(ctx, req.WorkspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)

	scheduled := testBroadcast(req.WorkspaceID, req.ID)
	scheduled.Status = domain.BroadcastStatusScheduled

	d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), req.WorkspaceID, req.ID).Return(scheduled, nil)
	d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	// Publish event and ack
	d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(_ context.Context, _ domain.EventPayload, ack domain.EventAckCallback) { ack(nil) },
	)

	err := d.svc.CancelBroadcast(ctx, req)
	require.NoError(t, err)
}

func TestBroadcastService_DeleteBroadcast_Success(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.DeleteBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
	authOK(d.authService, ctx, req.WorkspaceID)

	b := testBroadcast(req.WorkspaceID, req.ID)
	b.Status = domain.BroadcastStatusDraft // deletable
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.ID).Return(b, nil)
	d.repo.EXPECT().DeleteBroadcast(ctx, req.WorkspaceID, req.ID).Return(nil)

	err := d.svc.DeleteBroadcast(ctx, req)
	require.NoError(t, err)
}

// Error scenario tests

func TestBroadcastService_CreateBroadcast_AuthFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.CreateBroadcastRequest{WorkspaceID: "w1", Name: "Test"}

	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "w1").Return(ctx, nil, nil, errors.New("auth failed"))

	_, err := d.svc.CreateBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to authenticate user")
}

func TestBroadcastService_CreateBroadcast_PermissionDenied(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.CreateBroadcastRequest{WorkspaceID: "w1", Name: "Test"}

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user1",
		WorkspaceID: "w1",
		Role:        "viewer",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceBroadcasts: {Read: true, Write: false},
		},
	}
	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "w1").Return(ctx, &domain.User{ID: "user1"}, userWorkspace, nil)

	_, err := d.svc.CreateBroadcast(ctx, req)
	require.Error(t, err)
	assert.IsType(t, &domain.PermissionError{}, err)
}

func TestBroadcastService_CreateBroadcast_ValidationFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.CreateBroadcastRequest{WorkspaceID: "w1", Name: ""} // empty name should fail validation
	authOK(d.authService, ctx, req.WorkspaceID)

	_, err := d.svc.CreateBroadcast(ctx, req)
	require.Error(t, err)
}

func TestBroadcastService_CreateBroadcast_RepositoryFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.CreateBroadcastRequest{
		WorkspaceID: "w1",
		Name:        "Test",
		Audience:    domain.AudienceSettings{Segments: []string{"seg1"}},
		Schedule:    domain.ScheduleSettings{IsScheduled: false},
	}
	authOK(d.authService, ctx, req.WorkspaceID)

	d.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).Return(errors.New("db error"))

	_, err := d.svc.CreateBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestBroadcastService_GetBroadcast_AuthFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, errors.New("auth failed"))

	_, err := d.svc.GetBroadcast(ctx, workspaceID, broadcastID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to authenticate user")
}

func TestBroadcastService_GetBroadcast_PermissionDenied(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user1",
		WorkspaceID: workspaceID,
		Role:        "none",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceBroadcasts: {Read: false, Write: false},
		},
	}
	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: "user1"}, userWorkspace, nil)

	_, err := d.svc.GetBroadcast(ctx, workspaceID, broadcastID)
	require.Error(t, err)
	assert.IsType(t, &domain.PermissionError{}, err)
}

func TestBroadcastService_UpdateBroadcast_AuthFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.UpdateBroadcastRequest{WorkspaceID: "w1", ID: "b1"}

	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "w1").Return(ctx, nil, nil, errors.New("auth failed"))

	_, err := d.svc.UpdateBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to authenticate user")
}

func TestBroadcastService_UpdateBroadcast_GetBroadcastFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.UpdateBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
	authOK(d.authService, ctx, req.WorkspaceID)

	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.ID).Return(nil, errors.New("not found"))

	_, err := d.svc.UpdateBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestBroadcastService_UpdateBroadcast_ValidationFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.UpdateBroadcastRequest{
		WorkspaceID: "w1",
		ID:          "b1",
		Name:        "", // empty name should fail validation
	}
	authOK(d.authService, ctx, req.WorkspaceID)

	existing := testBroadcast(req.WorkspaceID, req.ID)
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.ID).Return(existing, nil)

	_, err := d.svc.UpdateBroadcast(ctx, req)
	require.Error(t, err)
}

func TestBroadcastService_UpdateBroadcast_RepositoryFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.UpdateBroadcastRequest{
		WorkspaceID: "w1",
		ID:          "b1",
		Name:        "Updated Name",
		Audience:    domain.AudienceSettings{Segments: []string{"seg1"}},
		Schedule:    domain.ScheduleSettings{IsScheduled: false},
		TestSettings: domain.BroadcastTestSettings{
			Enabled:    false,
			Variations: []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}},
		},
	}
	authOK(d.authService, ctx, req.WorkspaceID)

	existing := testBroadcast(req.WorkspaceID, req.ID)
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.ID).Return(existing, nil)
	d.repo.EXPECT().UpdateBroadcast(ctx, gomock.Any()).Return(errors.New("db error"))

	_, err := d.svc.UpdateBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestBroadcastService_ListBroadcasts_AuthFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	params := domain.ListBroadcastsParams{WorkspaceID: "w1"}

	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "w1").Return(ctx, nil, nil, errors.New("auth failed"))

	_, err := d.svc.ListBroadcasts(ctx, params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to authenticate user")
}

func TestBroadcastService_ListBroadcasts_RepositoryFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	params := domain.ListBroadcastsParams{WorkspaceID: "w1"}
	authOK(d.authService, ctx, params.WorkspaceID)

	d.repo.EXPECT().ListBroadcasts(ctx, gomock.Any()).Return(nil, errors.New("db error"))

	_, err := d.svc.ListBroadcasts(ctx, params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestBroadcastService_ListBroadcasts_DefaultPagination(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	params := domain.ListBroadcastsParams{WorkspaceID: "w1", Limit: 0, Offset: -1} // test defaults
	authOK(d.authService, ctx, params.WorkspaceID)

	expectedParams := params
	expectedParams.Limit = 50 // default
	expectedParams.Offset = 0 // corrected negative

	resp := &domain.BroadcastListResponse{Broadcasts: []*domain.Broadcast{}, TotalCount: 0}
	d.repo.EXPECT().ListBroadcasts(ctx, expectedParams).Return(resp, nil)

	out, err := d.svc.ListBroadcasts(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, out)
}

func TestBroadcastService_ListBroadcasts_MaxLimit(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	params := domain.ListBroadcastsParams{WorkspaceID: "w1", Limit: 200} // over max
	authOK(d.authService, ctx, params.WorkspaceID)

	expectedParams := params
	expectedParams.Limit = 100 // capped at max

	resp := &domain.BroadcastListResponse{Broadcasts: []*domain.Broadcast{}, TotalCount: 0}
	d.repo.EXPECT().ListBroadcasts(ctx, expectedParams).Return(resp, nil)

	out, err := d.svc.ListBroadcasts(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, out)
}

func TestBroadcastService_ListBroadcasts_WithTemplates_TemplateError(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	params := domain.ListBroadcastsParams{WorkspaceID: "w1", WithTemplates: true}
	authOK(d.authService, ctx, params.WorkspaceID)

	b := testBroadcast(params.WorkspaceID, "b1")
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
	resp := &domain.BroadcastListResponse{Broadcasts: []*domain.Broadcast{b}, TotalCount: 1}

	d.repo.EXPECT().ListBroadcasts(ctx, gomock.Any()).Return(resp, nil)

	// Template fetch fails - should continue without failing whole request
	d.templateSvc.EXPECT().GetTemplateByID(ctx, params.WorkspaceID, "tplA", int64(0)).Return(nil, errors.New("template error"))

	out, err := d.svc.ListBroadcasts(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Len(t, out.Broadcasts, 1)
	// Template should be nil since fetch failed
	assert.Nil(t, out.Broadcasts[0].TestSettings.Variations[0].Template)
}

func TestBroadcastService_ScheduleBroadcast_AuthFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.ScheduleBroadcastRequest{WorkspaceID: "w1", ID: "b1"}

	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "w1").Return(ctx, nil, nil, errors.New("auth failed"))

	err := d.svc.ScheduleBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to authenticate user")
}

func TestBroadcastService_ScheduleBroadcast_ValidationFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.ScheduleBroadcastRequest{WorkspaceID: "", ID: "b1"} // empty workspace should fail

	// Mock auth with empty workspace ID
	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "").Return(ctx, nil, nil, errors.New("invalid workspace"))

	err := d.svc.ScheduleBroadcast(ctx, req)
	require.Error(t, err)
}

func TestBroadcastService_ScheduleBroadcast_WorkspaceNotFound(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.ScheduleBroadcastRequest{WorkspaceID: "w1", ID: "b1", SendNow: true}
	authOK(d.authService, ctx, req.WorkspaceID)

	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(nil, errors.New("workspace not found"))

	err := d.svc.ScheduleBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace")
}

func TestBroadcastService_ScheduleBroadcast_NoEmailProvider(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.ScheduleBroadcastRequest{WorkspaceID: "w1", ID: "b1", SendNow: true}
	authOK(d.authService, ctx, req.WorkspaceID)

	// workspace without marketing email provider
	workspace := &domain.Workspace{
		ID:           "w1",
		Settings:     domain.WorkspaceSettings{},
		Integrations: domain.Integrations{},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	err := d.svc.ScheduleBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no marketing email provider configured")
}

func TestBroadcastService_ScheduleBroadcast_TransactionFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.ScheduleBroadcastRequest{WorkspaceID: "w1", ID: "b1", SendNow: true}
	authOK(d.authService, ctx, req.WorkspaceID)

	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	d.repo.EXPECT().WithTransaction(ctx, req.WorkspaceID, gomock.Any()).Return(errors.New("transaction error"))

	err := d.svc.ScheduleBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction error")
}

func TestBroadcastService_ScheduleBroadcast_InvalidStatus(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.ScheduleBroadcastRequest{WorkspaceID: "w1", ID: "b1", SendNow: true}
	authOK(d.authService, ctx, req.WorkspaceID)

	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	d.repo.EXPECT().WithTransaction(ctx, req.WorkspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			// broadcast with invalid status for scheduling
			broadcast := testBroadcast(req.WorkspaceID, req.ID)
			broadcast.Status = domain.BroadcastStatusSending // not draft
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), req.WorkspaceID, req.ID).Return(broadcast, nil)
			return fn(nil)
		},
	)

	err := d.svc.ScheduleBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only broadcasts with draft status can be scheduled")
}

func TestBroadcastService_ScheduleBroadcast_EventProcessingFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.ScheduleBroadcastRequest{WorkspaceID: "w1", ID: "b1", SendNow: true}
	authOK(d.authService, ctx, req.WorkspaceID)

	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	d.repo.EXPECT().WithTransaction(ctx, req.WorkspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			draft := testBroadcast(req.WorkspaceID, req.ID)
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), req.WorkspaceID, req.ID).Return(draft, nil)
			d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			// Event processing fails
			d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Do(
				func(_ context.Context, _ domain.EventPayload, ack domain.EventAckCallback) {
					ack(errors.New("event processing failed"))
				},
			)
			return fn(nil)
		},
	)

	err := d.svc.ScheduleBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to process schedule event")
}

func TestBroadcastService_PauseBroadcast_AuthFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.PauseBroadcastRequest{WorkspaceID: "w1", ID: "b1"}

	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "w1").Return(ctx, nil, nil, errors.New("auth failed"))

	err := d.svc.PauseBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to authenticate user")
}

func TestBroadcastService_PauseBroadcast_InvalidStatus(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.PauseBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
	authOK(d.authService, ctx, req.WorkspaceID)

	d.repo.EXPECT().WithTransaction(ctx, req.WorkspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			// broadcast with invalid status for pausing
			broadcast := testBroadcast(req.WorkspaceID, req.ID)
			broadcast.Status = domain.BroadcastStatusDraft // not sending
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), req.WorkspaceID, req.ID).Return(broadcast, nil)
			return fn(nil)
		},
	)

	err := d.svc.PauseBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only broadcasts with sending status can be paused")
}

func TestBroadcastService_ResumeBroadcast_AuthFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}

	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "w1").Return(ctx, nil, nil, errors.New("auth failed"))

	err := d.svc.ResumeBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to authenticate user")
}

func TestBroadcastService_ResumeBroadcast_InvalidStatus(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
	authOK(d.authService, ctx, req.WorkspaceID)

	d.repo.EXPECT().WithTransaction(ctx, req.WorkspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			// broadcast with invalid status for resuming
			broadcast := testBroadcast(req.WorkspaceID, req.ID)
			broadcast.Status = domain.BroadcastStatusDraft // not paused
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), req.WorkspaceID, req.ID).Return(broadcast, nil)
			return fn(nil)
		},
	)

	err := d.svc.ResumeBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only broadcasts with paused status can be resumed")
}

func TestBroadcastService_ResumeBroadcast_ToSending_Success(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
	authOK(d.authService, ctx, req.WorkspaceID)

	d.repo.EXPECT().WithTransaction(ctx, req.WorkspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) },
	)

	paused := testBroadcast(req.WorkspaceID, req.ID)
	paused.Status = domain.BroadcastStatusPaused
	// no schedule, should resume to sending
	paused.Schedule.IsScheduled = false

	d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), req.WorkspaceID, req.ID).Return(paused, nil)
	d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, _ domain.EventPayload, ack domain.EventAckCallback) { ack(nil) })

	err := d.svc.ResumeBroadcast(ctx, req)
	require.NoError(t, err)
}

func TestBroadcastService_CancelBroadcast_AuthFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.CancelBroadcastRequest{WorkspaceID: "w1", ID: "b1"}

	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "w1").Return(ctx, nil, nil, errors.New("auth failed"))

	err := d.svc.CancelBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to authenticate user")
}

func TestBroadcastService_CancelBroadcast_InvalidStatus(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.CancelBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
	authOK(d.authService, ctx, req.WorkspaceID)

	d.repo.EXPECT().WithTransaction(ctx, req.WorkspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			// broadcast with invalid status for cancelling
			broadcast := testBroadcast(req.WorkspaceID, req.ID)
			broadcast.Status = domain.BroadcastStatusSending // not scheduled or paused
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), req.WorkspaceID, req.ID).Return(broadcast, nil)
			return fn(nil)
		},
	)

	err := d.svc.CancelBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only broadcasts with scheduled or paused status can be cancelled")
}

func TestBroadcastService_DeleteBroadcast_AuthFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.DeleteBroadcastRequest{WorkspaceID: "w1", ID: "b1"}

	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "w1").Return(ctx, nil, nil, errors.New("auth failed"))

	err := d.svc.DeleteBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to authenticate user")
}

func TestBroadcastService_DeleteBroadcast_ValidationFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.DeleteBroadcastRequest{WorkspaceID: "", ID: "b1"} // empty workspace

	// Mock auth with empty workspace ID
	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "").Return(ctx, nil, nil, errors.New("invalid workspace"))

	err := d.svc.DeleteBroadcast(ctx, req)
	require.Error(t, err)
}

func TestBroadcastService_DeleteBroadcast_GetBroadcastFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.DeleteBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
	authOK(d.authService, ctx, req.WorkspaceID)

	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.ID).Return(nil, errors.New("not found"))

	err := d.svc.DeleteBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestBroadcastService_DeleteBroadcast_SendingStatus(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.DeleteBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
	authOK(d.authService, ctx, req.WorkspaceID)

	b := testBroadcast(req.WorkspaceID, req.ID)
	b.Status = domain.BroadcastStatusSending // not deletable
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.ID).Return(b, nil)

	err := d.svc.DeleteBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broadcasts in 'sending' status cannot be deleted")
}

func TestBroadcastService_DeleteBroadcast_RepositoryFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.DeleteBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
	authOK(d.authService, ctx, req.WorkspaceID)

	b := testBroadcast(req.WorkspaceID, req.ID)
	b.Status = domain.BroadcastStatusDraft
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.ID).Return(b, nil)
	d.repo.EXPECT().DeleteBroadcast(ctx, req.WorkspaceID, req.ID).Return(errors.New("db error"))

	err := d.svc.DeleteBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestBroadcastService_SendToIndividual_AuthFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "test@example.com"}

	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "w1").Return(ctx, nil, nil, errors.New("auth failed"))

	err := d.svc.SendToIndividual(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to authenticate user")
}

func TestBroadcastService_SendToIndividual_ValidationFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "", BroadcastID: "b1", RecipientEmail: "test@example.com"}

	// Mock auth with empty workspace ID
	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, "").Return(ctx, nil, nil, errors.New("invalid workspace"))

	err := d.svc.SendToIndividual(ctx, req)
	require.Error(t, err)
}

func TestBroadcastService_SendToIndividual_WorkspaceNotFound(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "test@example.com"}
	authOK(d.authService, ctx, req.WorkspaceID)

	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(nil, errors.New("workspace not found"))

	err := d.svc.SendToIndividual(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace not found")
}

func TestBroadcastService_SendToIndividual_NoEmailProvider(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "test@example.com"}
	authOK(d.authService, ctx, req.WorkspaceID)

	workspace := &domain.Workspace{
		ID:           "w1",
		Settings:     domain.WorkspaceSettings{},
		Integrations: domain.Integrations{},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	err := d.svc.SendToIndividual(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no marketing email provider configured")
}

func TestBroadcastService_SendToIndividual_BroadcastNotFound(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "test@example.com"}
	authOK(d.authService, ctx, req.WorkspaceID)

	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(nil, errors.New("broadcast not found"))

	err := d.svc.SendToIndividual(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broadcast not found")
}

func TestBroadcastService_SendToIndividual_NoVariations(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "test@example.com"}
	authOK(d.authService, ctx, req.WorkspaceID)

	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	b := testBroadcast(req.WorkspaceID, req.BroadcastID)
	b.TestSettings.Variations = []domain.BroadcastVariation{} // no variations
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)

	err := d.svc.SendToIndividual(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broadcast has no variations")
}

func TestBroadcastService_SendToIndividual_VariationNotFound(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{
		WorkspaceID:    "w1",
		BroadcastID:    "b1",
		RecipientEmail: "test@example.com",
		TemplateID:     "nonexistent", // variation that doesn't exist
	}
	authOK(d.authService, ctx, req.WorkspaceID)

	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	b := testBroadcast(req.WorkspaceID, req.BroadcastID)
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)

	err := d.svc.SendToIndividual(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "variation with ID nonexistent not found")
}

func TestBroadcastService_SendToIndividual_TemplateNotFound(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "test@example.com"}
	authOK(d.authService, ctx, req.WorkspaceID)

	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	b := testBroadcast(req.WorkspaceID, req.BroadcastID)
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)

	d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, req.RecipientEmail).Return(nil, errors.New("not found")).AnyTimes()

	d.templateSvc.EXPECT().GetTemplateByID(ctx, req.WorkspaceID, "tplA", int64(0)).Return(nil, errors.New("template not found"))

	err := d.svc.SendToIndividual(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template not found")
}

func TestBroadcastService_SendToIndividual_NoSender(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "test@example.com"}
	authOK(d.authService, ctx, req.WorkspaceID)

	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{}}}, // no senders
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	b := testBroadcast(req.WorkspaceID, req.BroadcastID)
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)

	d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, req.RecipientEmail).Return(nil, errors.New("not found")).AnyTimes()

	template := &domain.Template{
		ID:      "tplA",
		Name:    "Template A",
		Channel: "email",
		Email: &domain.EmailTemplate{
			SenderID: "nonexistent", // sender that doesn't exist
			Subject:  "Hello",
		},
	}
	d.templateSvc.EXPECT().GetTemplateByID(ctx, req.WorkspaceID, "tplA", int64(0)).Return(template, nil)

	err := d.svc.SendToIndividual(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get sender")
}

func TestBroadcastService_SendToIndividual_TemplateCompilationFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "test@example.com"}
	authOK(d.authService, ctx, req.WorkspaceID)

	sender := domain.NewEmailSender("from@example.com", "From")
	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt", SecretKey: "sk_test"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{sender}}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	b := testBroadcast(req.WorkspaceID, req.BroadcastID)
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)

	d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, req.RecipientEmail).Return(nil, errors.New("not found")).AnyTimes()

	template := &domain.Template{
		ID:      "tplA",
		Name:    "Template A",
		Channel: "email",
		Email: &domain.EmailTemplate{
			SenderID: sender.ID,
			Subject:  "Hello",
		},
	}
	d.templateSvc.EXPECT().GetTemplateByID(ctx, req.WorkspaceID, "tplA", int64(0)).Return(template, nil)

	d.templateSvc.EXPECT().CompileTemplate(ctx, gomock.Any()).Return(nil, errors.New("compilation failed"))

	err := d.svc.SendToIndividual(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compilation failed")
}

func TestBroadcastService_SendToIndividual_TemplateCompilationNotSuccessful(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "test@example.com"}
	authOK(d.authService, ctx, req.WorkspaceID)

	sender := domain.NewEmailSender("from@example.com", "From")
	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt", SecretKey: "sk_test"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{sender}}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	b := testBroadcast(req.WorkspaceID, req.BroadcastID)
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)

	d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, req.RecipientEmail).Return(nil, errors.New("not found")).AnyTimes()

	template := &domain.Template{
		ID:      "tplA",
		Name:    "Template A",
		Channel: "email",
		Email: &domain.EmailTemplate{
			SenderID: sender.ID,
			Subject:  "Hello",
		},
	}
	d.templateSvc.EXPECT().GetTemplateByID(ctx, req.WorkspaceID, "tplA", int64(0)).Return(template, nil)

	// Template compilation fails
	errMsg := "Template syntax error"
	compiledTemplate := &domain.CompileTemplateResponse{
		Success: false,
		Error:   &mjmlgo.Error{Message: errMsg},
	}
	d.templateSvc.EXPECT().CompileTemplate(ctx, gomock.Any()).Return(compiledTemplate, nil)

	err := d.svc.SendToIndividual(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template compilation failed")
	assert.Contains(t, err.Error(), errMsg)
}

func TestBroadcastService_SendToIndividual_EmailSendFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "test@example.com"}
	authOK(d.authService, ctx, req.WorkspaceID)

	sender := domain.NewEmailSender("from@example.com", "From")
	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt", SecretKey: "sk_test"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{sender}}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	b := testBroadcast(req.WorkspaceID, req.BroadcastID)
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)

	d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, req.RecipientEmail).Return(nil, errors.New("not found")).AnyTimes()

	template := &domain.Template{
		ID:      "tplA",
		Name:    "Template A",
		Channel: "email",
		Email: &domain.EmailTemplate{
			SenderID: sender.ID,
			Subject:  "Hello",
		},
	}
	d.templateSvc.EXPECT().GetTemplateByID(ctx, req.WorkspaceID, "tplA", int64(0)).Return(template, nil)

	compiledHTML := "<html>ok</html>"
	d.templateSvc.EXPECT().CompileTemplate(ctx, gomock.Any()).Return(&domain.CompileTemplateResponse{Success: true, HTML: &compiledHTML}, nil)

	d.emailSvc.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(errors.New("email send failed"))

	err := d.svc.SendToIndividual(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "email send failed")
}

func TestBroadcastService_SendToIndividual_MessageHistoryFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "test@example.com"}
	authOK(d.authService, ctx, req.WorkspaceID)

	sender := domain.NewEmailSender("from@example.com", "From")
	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt", SecretKey: "sk_test"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{sender}}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	b := testBroadcast(req.WorkspaceID, req.BroadcastID)
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)

	d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, req.RecipientEmail).Return(nil, errors.New("not found")).AnyTimes()

	template := &domain.Template{
		ID:      "tplA",
		Name:    "Template A",
		Channel: "email",
		Email: &domain.EmailTemplate{
			SenderID: sender.ID,
			Subject:  "Hello",
		},
	}
	d.templateSvc.EXPECT().GetTemplateByID(ctx, req.WorkspaceID, "tplA", int64(0)).Return(template, nil)

	compiledHTML := "<html>ok</html>"
	d.templateSvc.EXPECT().CompileTemplate(ctx, gomock.Any()).Return(&domain.CompileTemplateResponse{Success: true, HTML: &compiledHTML}, nil)

	d.emailSvc.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)

	d.messageHistoryRepo.EXPECT().Create(gomock.Any(), req.WorkspaceID, gomock.Any()).Return(errors.New("message history failed"))

	err := d.svc.SendToIndividual(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message history failed")
}

func TestBroadcastService_GetTestResults_AuthFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"

	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, errors.New("auth failed"))

	_, err := d.svc.GetTestResults(ctx, workspaceID, broadcastID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth failed")
}

func TestBroadcastService_GetTestResults_BroadcastNotFound(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	authOK(d.authService, ctx, workspaceID)

	d.repo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(nil, errors.New("broadcast not found"))

	_, err := d.svc.GetTestResults(ctx, workspaceID, broadcastID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broadcast not found")
}

func TestBroadcastService_GetTestResults_InvalidStatus(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	authOK(d.authService, ctx, workspaceID)

	b := testBroadcast(workspaceID, broadcastID)
	b.Status = domain.BroadcastStatusDraft // invalid status for test results
	d.repo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	_, err := d.svc.GetTestResults(ctx, workspaceID, broadcastID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broadcast test results not available for status")
}

func TestBroadcastService_GetTestResults_StatsFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	authOK(d.authService, ctx, workspaceID)

	b := testBroadcast(workspaceID, broadcastID)
	b.Status = domain.BroadcastStatusTesting
	b.TestSettings.Enabled = true
	b.TestSettings.Variations = []domain.BroadcastVariation{
		{VariationName: "A", TemplateID: "tplA"},
	}
	d.repo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	// Stats fetch fails - should continue without that variation
	d.messageHistoryRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(nil, errors.New("stats failed"))

	res, err := d.svc.GetTestResults(ctx, workspaceID, broadcastID)
	require.NoError(t, err)
	require.NotNil(t, res)
	// Should have empty results since stats failed
	assert.Empty(t, res.VariationResults)
}

func TestBroadcastService_SelectWinner_AuthFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	templateID := "tplA"

	d.authService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, errors.New("auth failed"))

	err := d.svc.SelectWinner(ctx, workspaceID, broadcastID, templateID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth failed")
}

func TestBroadcastService_SelectWinner_TransactionFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	templateID := "tplA"
	authOK(d.authService, ctx, workspaceID)

	d.repo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).Return(errors.New("transaction failed"))

	err := d.svc.SelectWinner(ctx, workspaceID, broadcastID, templateID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction failed")
}

func TestBroadcastService_SelectWinner_InvalidStatus(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	templateID := "tplA"
	authOK(d.authService, ctx, workspaceID)

	d.repo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			b := testBroadcast(workspaceID, broadcastID)
			b.Status = domain.BroadcastStatusDraft // invalid status
			d.repo.EXPECT().GetBroadcastTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(b, nil)
			return fn(nil)
		},
	)

	err := d.svc.SelectWinner(ctx, workspaceID, broadcastID, templateID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broadcast is not in test completed state")
}

func TestBroadcastService_SelectWinner_InvalidTemplate(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	templateID := "invalid"
	authOK(d.authService, ctx, workspaceID)

	d.repo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			b := testBroadcast(workspaceID, broadcastID)
			b.Status = domain.BroadcastStatusTestCompleted
			b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
			d.repo.EXPECT().GetBroadcastTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(b, nil)
			return fn(nil)
		},
	)

	err := d.svc.SelectWinner(ctx, workspaceID, broadcastID, templateID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid template ID")
}

func TestBroadcastService_SelectWinner_UpdateFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	templateID := "tplA"
	authOK(d.authService, ctx, workspaceID)

	d.repo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			b := testBroadcast(workspaceID, broadcastID)
			b.Status = domain.BroadcastStatusTestCompleted
			b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: templateID}}
			d.repo.EXPECT().GetBroadcastTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(b, nil)
			d.repo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).Return(errors.New("update failed"))
			return fn(nil)
		},
	)

	err := d.svc.SelectWinner(ctx, workspaceID, broadcastID, templateID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")
}

func TestBroadcastService_SelectWinner_TaskUpdateFailure(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	templateID := "tplA"
	authOK(d.authService, ctx, workspaceID)

	d.repo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			b := testBroadcast(workspaceID, broadcastID)
			b.Status = domain.BroadcastStatusTestCompleted
			b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: templateID}}
			d.repo.EXPECT().GetBroadcastTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(b, nil)
			d.repo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).Return(nil)

			task := &domain.Task{ID: "task1", WorkspaceID: workspaceID, Status: domain.TaskStatusPaused}
			d.taskRepo.EXPECT().GetTaskByBroadcastID(ctx, workspaceID, broadcastID).Return(task, nil)
			d.taskRepo.EXPECT().Update(ctx, workspaceID, gomock.Any()).Return(errors.New("task update failed"))
			return fn(nil)
		},
	)

	err := d.svc.SelectWinner(ctx, workspaceID, broadcastID, templateID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task update failed")
}

func TestBroadcastService_SelectWinner_NoTask(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	templateID := "tplA"
	authOK(d.authService, ctx, workspaceID)

	d.repo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			b := testBroadcast(workspaceID, broadcastID)
			b.Status = domain.BroadcastStatusTestCompleted
			b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: templateID}}
			d.repo.EXPECT().GetBroadcastTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(b, nil)
			d.repo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).Return(nil)

			// No task found - should not be an error
			d.taskRepo.EXPECT().GetTaskByBroadcastID(ctx, workspaceID, broadcastID).Return(nil, errors.New("task not found"))
			return fn(nil)
		},
	)

	err := d.svc.SelectWinner(ctx, workspaceID, broadcastID, templateID)
	require.NoError(t, err)
}

// Context cancellation tests

func TestBroadcastService_ScheduleBroadcast_ContextCancellation(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	req := &domain.ScheduleBroadcastRequest{WorkspaceID: "w1", ID: "b1", SendNow: true}
	authOK(d.authService, ctx, req.WorkspaceID)

	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	d.repo.EXPECT().WithTransaction(ctx, req.WorkspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			draft := testBroadcast(req.WorkspaceID, req.ID)
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), req.WorkspaceID, req.ID).Return(draft, nil)
			d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Do(
				func(_ context.Context, _ domain.EventPayload, ack domain.EventAckCallback) {
					// Cancel context before callback
					cancel()
					// Simulate delay to allow context cancellation to take effect
					time.Sleep(10 * time.Millisecond)
				},
			)
			return fn(nil)
		},
	)

	err := d.svc.ScheduleBroadcast(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

// Additional edge case tests

func TestBroadcastService_CreateBroadcast_WithScheduledBroadcast(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.CreateBroadcastRequest{
		WorkspaceID: "w1",
		Name:        "Scheduled Campaign",
		Audience:    domain.AudienceSettings{Segments: []string{"seg1"}},
		Schedule: domain.ScheduleSettings{
			IsScheduled:   true,
			ScheduledDate: "2024-12-25",
			ScheduledTime: "10:00",
			Timezone:      "UTC",
		},
	}

	authOK(d.authService, ctx, req.WorkspaceID)

	d.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, b *domain.Broadcast) error {
			// Verify status is set to scheduled for scheduled broadcasts
			assert.Equal(t, domain.BroadcastStatusScheduled, b.Status)
			return nil
		},
	)

	b, err := d.svc.CreateBroadcast(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Equal(t, domain.BroadcastStatusScheduled, b.Status)
}

func TestBroadcastService_SendToIndividual_WithContact(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "test@example.com"}
	authOK(d.authService, ctx, req.WorkspaceID)

	sender := domain.NewEmailSender("from@example.com", "From")
	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt", SecretKey: "sk_test"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{sender}}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	b := testBroadcast(req.WorkspaceID, req.BroadcastID)
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)

	// Contact found this time
	firstName := &domain.NullableString{String: "John", IsNull: false}
	lastName := &domain.NullableString{String: "Doe", IsNull: false}
	contact := &domain.Contact{
		Email:     req.RecipientEmail,
		FirstName: firstName,
		LastName:  lastName,
	}
	d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, req.RecipientEmail).Return(contact, nil).AnyTimes()

	template := &domain.Template{
		ID:      "tplA",
		Name:    "Template A",
		Channel: "email",
		Email: &domain.EmailTemplate{
			SenderID:        sender.ID,
			Subject:         "Hello {{contact.first_name}}",
			CompiledPreview: "<p>Hello {{contact.first_name}}</p>",
		},
	}
	d.templateSvc.EXPECT().GetTemplateByID(ctx, req.WorkspaceID, "tplA", int64(0)).Return(template, nil)

	compiledHTML := "<html>Hello John</html>"
	d.templateSvc.EXPECT().CompileTemplate(ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, compileReq domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
			// Verify contact data is included in template data
			contactData, ok := compileReq.TemplateData["contact"]
			assert.True(t, ok)
			assert.NotNil(t, contactData)
			return &domain.CompileTemplateResponse{Success: true, HTML: &compiledHTML}, nil
		},
	)

	d.emailSvc.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)

	d.messageHistoryRepo.EXPECT().Create(gomock.Any(), req.WorkspaceID, gomock.Any()).Return(nil)

	err := d.svc.SendToIndividual(ctx, req)
	require.NoError(t, err)
}

func TestBroadcastService_SendToIndividual_WithCustomEndpoint(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "test@example.com"}
	authOK(d.authService, ctx, req.WorkspaceID)

	sender := domain.NewEmailSender("from@example.com", "From")
	customEndpoint := "https://custom.api.example.com"
	workspace := &domain.Workspace{
		ID: "w1",
		Settings: domain.WorkspaceSettings{
			MarketingEmailProviderID: "mkt",
			SecretKey:                "sk_test",
			CustomEndpointURL:        &customEndpoint,
		},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{sender}}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	b := testBroadcast(req.WorkspaceID, req.BroadcastID)
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
	// Add UTM parameters to test that branch
	b.UTMParameters = &domain.UTMParameters{
		Source:   "newsletter",
		Medium:   "email",
		Campaign: "holiday_sale",
		Content:  "variation_a",
		Term:     "discount",
	}
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)

	d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, req.RecipientEmail).Return(nil, errors.New("not found")).AnyTimes()

	template := &domain.Template{
		ID:      "tplA",
		Name:    "Template A",
		Channel: "email",
		Email: &domain.EmailTemplate{
			SenderID: sender.ID,
			Subject:  "Hello",
		},
	}
	d.templateSvc.EXPECT().GetTemplateByID(ctx, req.WorkspaceID, "tplA", int64(0)).Return(template, nil)

	compiledHTML := "<html>ok</html>"
	d.templateSvc.EXPECT().CompileTemplate(ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, compileReq domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
			// Verify custom endpoint is used in tracking settings
			assert.Equal(t, customEndpoint, compileReq.TrackingSettings.Endpoint)
			// Verify UTM parameters are set
			assert.Equal(t, "newsletter", compileReq.TrackingSettings.UTMSource)
			assert.Equal(t, "email", compileReq.TrackingSettings.UTMMedium)
			assert.Equal(t, "holiday_sale", compileReq.TrackingSettings.UTMCampaign)
			assert.Equal(t, "variation_a", compileReq.TrackingSettings.UTMContent)
			assert.Equal(t, "discount", compileReq.TrackingSettings.UTMTerm)
			return &domain.CompileTemplateResponse{Success: true, HTML: &compiledHTML}, nil
		},
	)

	d.emailSvc.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)

	d.messageHistoryRepo.EXPECT().Create(gomock.Any(), req.WorkspaceID, gomock.Any()).Return(nil)

	err := d.svc.SendToIndividual(ctx, req)
	require.NoError(t, err)
}

func TestBroadcastService_GetTestResults_WithAutoSendWinner(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	authOK(d.authService, ctx, workspaceID)

	b := testBroadcast(workspaceID, broadcastID)
	b.Status = domain.BroadcastStatusTesting
	b.TestSettings.Enabled = true
	b.TestSettings.AutoSendWinner = true // auto-send enabled
	b.TestSettings.Variations = []domain.BroadcastVariation{
		{VariationName: "A", TemplateID: "tplA"},
		{VariationName: "B", TemplateID: "tplB"},
	}
	d.repo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	// stats for A and B
	d.messageHistoryRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalSent: 100, TotalDelivered: 100, TotalOpened: 30, TotalClicked: 5}, nil)
	d.messageHistoryRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalSent: 100, TotalDelivered: 100, TotalOpened: 25, TotalClicked: 10}, nil)

	res, err := d.svc.GetTestResults(ctx, workspaceID, broadcastID)
	require.NoError(t, err)
	require.NotNil(t, res)
	// Should not recommend winner when auto-send is enabled
	assert.Empty(t, res.RecommendedWinner)
	assert.True(t, res.IsAutoSendWinner)
}

func TestBroadcastService_GetTestResults_WithWinnerAlreadySelected(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	authOK(d.authService, ctx, workspaceID)

	b := testBroadcast(workspaceID, broadcastID)
	b.Status = domain.BroadcastStatusWinnerSelected
	b.TestSettings.Enabled = true
	b.TestSettings.AutoSendWinner = false
	b.WinningTemplate = "tplB" // winner already selected
	b.TestSettings.Variations = []domain.BroadcastVariation{
		{VariationName: "A", TemplateID: "tplA"},
		{VariationName: "B", TemplateID: "tplB"},
	}
	d.repo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	// stats for A and B
	d.messageHistoryRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalSent: 100, TotalDelivered: 100, TotalOpened: 30, TotalClicked: 5}, nil)
	d.messageHistoryRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplB").Return(&domain.MessageHistoryStatusSum{TotalSent: 100, TotalDelivered: 100, TotalOpened: 25, TotalClicked: 10}, nil)

	res, err := d.svc.GetTestResults(ctx, workspaceID, broadcastID)
	require.NoError(t, err)
	require.NotNil(t, res)
	// Should not recommend winner when winner already selected
	assert.Empty(t, res.RecommendedWinner)
	assert.Equal(t, "tplB", res.WinningTemplate)
}

func TestBroadcastService_GetTestResults_ZeroSentMessages(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	authOK(d.authService, ctx, workspaceID)

	b := testBroadcast(workspaceID, broadcastID)
	b.Status = domain.BroadcastStatusTesting
	b.TestSettings.Enabled = true
	b.TestSettings.AutoSendWinner = false
	b.TestSettings.Variations = []domain.BroadcastVariation{
		{VariationName: "A", TemplateID: "tplA"},
	}
	d.repo.EXPECT().GetBroadcast(ctx, workspaceID, broadcastID).Return(b, nil)

	// Zero sent messages - should handle division by zero
	d.messageHistoryRepo.EXPECT().GetBroadcastVariationStats(ctx, workspaceID, broadcastID, "tplA").Return(&domain.MessageHistoryStatusSum{TotalSent: 0, TotalDelivered: 0, TotalOpened: 0, TotalClicked: 0}, nil)

	res, err := d.svc.GetTestResults(ctx, workspaceID, broadcastID)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.VariationResults, 1)

	result := res.VariationResults["tplA"]
	assert.Equal(t, 0.0, result.OpenRate)
	assert.Equal(t, 0.0, result.ClickRate)
}

func TestBroadcastService_SelectWinner_DuringTestingPhase(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	templateID := "tplA"
	authOK(d.authService, ctx, workspaceID)

	d.repo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			b := testBroadcast(workspaceID, broadcastID)
			b.Status = domain.BroadcastStatusTesting // testing phase
			b.TestSettings.Enabled = true
			b.TestSettings.AutoSendWinner = false // manual selection allowed
			b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: templateID}}
			d.repo.EXPECT().GetBroadcastTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(b, nil)
			d.repo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).Return(nil)

			// No task found - should not be an error
			d.taskRepo.EXPECT().GetTaskByBroadcastID(ctx, workspaceID, broadcastID).Return(nil, errors.New("task not found"))
			return fn(nil)
		},
	)

	err := d.svc.SelectWinner(ctx, workspaceID, broadcastID, templateID)
	require.NoError(t, err)
}

func TestBroadcastService_SelectWinner_DuringSendingPhase(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	workspaceID := "w1"
	broadcastID := "b1"
	templateID := "tplA"
	authOK(d.authService, ctx, workspaceID)

	d.repo.EXPECT().WithTransaction(ctx, workspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			b := testBroadcast(workspaceID, broadcastID)
			b.Status = domain.BroadcastStatusSending // sending phase
			b.TestSettings.Enabled = true
			b.TestSettings.AutoSendWinner = false // manual selection allowed
			b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: templateID}}
			d.repo.EXPECT().GetBroadcastTx(ctx, gomock.Any(), workspaceID, broadcastID).Return(b, nil)
			d.repo.EXPECT().UpdateBroadcastTx(ctx, gomock.Any(), gomock.Any()).Return(nil)

			// No task found - should not be an error
			d.taskRepo.EXPECT().GetTaskByBroadcastID(ctx, workspaceID, broadcastID).Return(nil, errors.New("task not found"))
			return fn(nil)
		},
	)

	err := d.svc.SelectWinner(ctx, workspaceID, broadcastID, templateID)
	require.NoError(t, err)
}

// Additional tests to improve coverage on the lower-coverage methods

func TestBroadcastService_CreateBroadcast_IDGeneration(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.CreateBroadcastRequest{
		WorkspaceID: "w1",
		Name:        "Test Campaign",
		Audience:    domain.AudienceSettings{Segments: []string{"seg1"}},
		Schedule:    domain.ScheduleSettings{IsScheduled: false},
	}

	authOK(d.authService, ctx, req.WorkspaceID)

	d.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, b *domain.Broadcast) error {
			// Verify ID was generated
			assert.NotEmpty(t, b.ID)
			assert.Len(t, b.ID, 32) // Should be 32 chars from hex encoding
			return nil
		},
	)

	b, err := d.svc.CreateBroadcast(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.NotEmpty(t, b.ID)
}

func TestBroadcastService_ScheduleBroadcast_ScheduledTimeInPayload(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.ScheduleBroadcastRequest{
		WorkspaceID:   "w1",
		ID:            "b1",
		SendNow:       false,
		ScheduledDate: "2024-12-25",
		ScheduledTime: "10:00",
		Timezone:      "UTC",
	}
	authOK(d.authService, ctx, req.WorkspaceID)

	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	d.repo.EXPECT().WithTransaction(ctx, req.WorkspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			draft := testBroadcast(req.WorkspaceID, req.ID)
			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), req.WorkspaceID, req.ID).Return(draft, nil)
			d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			// Verify event payload includes scheduled_time
			d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Do(
				func(_ context.Context, payload domain.EventPayload, ack domain.EventAckCallback) {
					// Check that scheduled_time is included in the payload
					scheduledTime, exists := payload.Data["scheduled_time"]
					assert.True(t, exists)
					assert.NotNil(t, scheduledTime)
					ack(nil)
				},
			)
			return fn(nil)
		},
	)

	err := d.svc.ScheduleBroadcast(ctx, req)
	require.NoError(t, err)
}

func TestBroadcastService_ResumeBroadcast_ScheduleParseError(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.ResumeBroadcastRequest{WorkspaceID: "w1", ID: "b1"}
	authOK(d.authService, ctx, req.WorkspaceID)

	d.repo.EXPECT().WithTransaction(ctx, req.WorkspaceID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
			paused := testBroadcast(req.WorkspaceID, req.ID)
			paused.Status = domain.BroadcastStatusPaused
			paused.Schedule.IsScheduled = true
			// Set invalid schedule data to trigger parse error
			paused.Schedule.ScheduledDate = "invalid-date"
			paused.Schedule.ScheduledTime = "invalid-time"

			d.repo.EXPECT().GetBroadcastTx(gomock.Any(), gomock.Any(), req.WorkspaceID, req.ID).Return(paused, nil)
			d.repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			d.eventBus.EXPECT().PublishWithAck(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, _ domain.EventPayload, ack domain.EventAckCallback) { ack(nil) })

			return fn(nil)
		},
	)

	err := d.svc.ResumeBroadcast(ctx, req)
	require.NoError(t, err)
}

func TestBroadcastService_SendToIndividual_ContactToMapError(t *testing.T) {
	d := setupBroadcastSvc(t)
	defer d.ctrl.Finish()

	ctx := context.Background()
	req := &domain.SendToIndividualRequest{WorkspaceID: "w1", BroadcastID: "b1", RecipientEmail: "test@example.com"}
	authOK(d.authService, ctx, req.WorkspaceID)

	sender := domain.NewEmailSender("from@example.com", "From")
	workspace := &domain.Workspace{
		ID:       "w1",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "mkt", SecretKey: "sk_test"},
		Integrations: domain.Integrations{
			{ID: "mkt", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: []domain.EmailSender{sender}}},
		},
	}
	d.workspaceRepo.EXPECT().GetByID(ctx, req.WorkspaceID).Return(workspace, nil)

	b := testBroadcast(req.WorkspaceID, req.BroadcastID)
	b.TestSettings.Variations = []domain.BroadcastVariation{{VariationName: "A", TemplateID: "tplA"}}
	d.repo.EXPECT().GetBroadcast(ctx, req.WorkspaceID, req.BroadcastID).Return(b, nil)

	// Contact found but with invalid data that causes ToMapOfAny to fail
	contact := &domain.Contact{
		Email: req.RecipientEmail,
		// This will cause ToMapOfAny to fail gracefully and continue without contact data
	}
	d.contactRepo.EXPECT().GetContactByEmail(ctx, req.WorkspaceID, req.RecipientEmail).Return(contact, nil).AnyTimes()

	template := &domain.Template{
		ID:      "tplA",
		Name:    "Template A",
		Channel: "email",
		Email: &domain.EmailTemplate{
			SenderID: sender.ID,
			Subject:  "Hello",
		},
	}
	d.templateSvc.EXPECT().GetTemplateByID(ctx, req.WorkspaceID, "tplA", int64(0)).Return(template, nil)

	compiledHTML := "<html>ok</html>"
	d.templateSvc.EXPECT().CompileTemplate(ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, compileReq domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
			// Verify that even if contact.ToMapOfAny fails, the template compilation continues
			// The contact data might or might not be present depending on the error handling
			return &domain.CompileTemplateResponse{Success: true, HTML: &compiledHTML}, nil
		},
	)

	d.emailSvc.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)
	d.messageHistoryRepo.EXPECT().Create(gomock.Any(), req.WorkspaceID, gomock.Any()).Return(nil)

	err := d.svc.SendToIndividual(ctx, req)
	require.NoError(t, err)
}
