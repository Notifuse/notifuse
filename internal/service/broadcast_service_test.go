package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

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
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: "user1"}, nil, nil)
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
