package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/cache"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Gap coverage for addSampleData's step ordering and for
// generateDeliveredInboundWebhookEventsForBatch's exact event rows.
// Fatal steps abort with the step's wrapped error and downstream steps
// never run; the web-analytics / automations / annotations steps are
// non-fatal by contract (pinned by the full-success test below).

type demoGapDeps struct {
	svc           *DemoService
	ctrl          *gomock.Controller
	templateRepo  *mocks.MockTemplateRepository
	listRepo      *mocks.MockListRepository
	contactRepo   *mocks.MockContactRepository
	segmentSvc    *mocks.MockSegmentService
	msgHistory    *mocks.MockMessageHistoryRepository
	createdTplIDs []string
}

// demoGapService builds the real Template/List/Contact services over mock
// repositories. Template+list auth succeeds as owner; contact auth is
// caller-supplied so tests can fail authentication or grant read-only
// access (read-only makes every UpsertContact fail its permission check
// without touching a repository, which is how the 1000-contact step
// completes cheaply).
func demoGapService(t *testing.T, contactAuth *mocks.MockAuthService) *demoGapDeps {
	t.Helper()
	ctrl := gomock.NewController(t)
	log := logger.NewLoggerWithLevel("disabled")
	d := &demoGapDeps{ctrl: ctrl}

	owner := &domain.UserWorkspace{UserID: "u1", WorkspaceID: "ws-gap", Role: "owner"}
	authOK := mocks.NewMockAuthService(ctrl)
	authOK.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		Return(context.Background(), &domain.User{ID: "u1"}, owner, nil).AnyTimes()

	wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
	wsRepo.EXPECT().GetByID(gomock.Any(), gomock.Any()).
		Return(&domain.Workspace{ID: "ws-gap", Settings: domain.WorkspaceSettings{Timezone: "UTC", SecretKey: "sk", Languages: []string{"en", "fr", "es"}, DefaultLanguage: "en"}}, nil).AnyTimes()

	d.templateRepo = mocks.NewMockTemplateRepository(ctrl)
	templateService := NewTemplateService(d.templateRepo, wsRepo, authOK, log, "", nil)

	d.listRepo = mocks.NewMockListRepository(ctrl)
	listService := NewListService(
		d.listRepo, wsRepo,
		mocks.NewMockContactListRepository(ctrl),
		mocks.NewMockContactRepository(ctrl),
		mocks.NewMockMessageHistoryRepository(ctrl),
		authOK, nil, log, "", cache.NewInMemoryCache(time.Hour),
	)

	d.contactRepo = mocks.NewMockContactRepository(ctrl)
	d.msgHistory = mocks.NewMockMessageHistoryRepository(ctrl)
	contactService := NewContactService(
		d.contactRepo, wsRepo, contactAuth,
		d.msgHistory,
		mocks.NewMockInboundWebhookEventRepository(ctrl),
		mocks.NewMockContactListRepository(ctrl),
		mocks.NewMockContactTimelineRepository(ctrl),
		nil,
		mocks.NewMockEmailQueueRepository(ctrl),
		mocks.NewMockCustomEventRepository(ctrl),
		mocks.NewMockSegmentRepository(ctrl),
		mocks.NewMockContactSegmentQueueRepository(ctrl),
		log,
	)

	d.segmentSvc = mocks.NewMockSegmentService(ctrl)

	d.svc = &DemoService{
		logger:          log,
		config:          &config.Config{},
		templateService: templateService,
		listService:     listService,
		contactService:  contactService,
		segmentService:  d.segmentSvc,
		// webAnalyticsRepo, automationService and annotationRepo stay nil:
		// steps 3b, 9 and 10 must degrade to logged warnings, never abort.
	}
	return d
}

func (d *demoGapDeps) expectTemplatesOK() {
	d.templateRepo.EXPECT().CreateTemplate(gomock.Any(), "ws-gap", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, tpl *domain.Template) error {
			d.createdTplIDs = append(d.createdTplIDs, tpl.ID)
			return nil
		}).AnyTimes()
}

func demoGapBroadcastExpectations(d *broadcastSvcDeps) {
	uw := &domain.UserWorkspace{
		UserID: "user1", WorkspaceID: "ws-gap", Role: "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceBroadcasts: {Read: true, Write: true},
		},
	}
	d.authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-gap").
		Return(context.Background(), &domain.User{ID: "user1"}, uw, nil).AnyTimes()
	d.workspaceRepo.EXPECT().GetByID(gomock.Any(), gomock.Any()).
		Return(&domain.Workspace{ID: "ws-gap", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}, nil).AnyTimes()
	d.templateSvc.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&domain.Template{ID: "tpl"}, nil).AnyTimes()
}

func TestDemoService_AddSampleData_GapListsFailureAborts(t *testing.T) {
	ctrl := gomock.NewController(t)
	contactAuth := mocks.NewMockAuthService(ctrl)
	d := demoGapService(t, contactAuth)
	d.expectTemplatesOK()

	sentinel := errors.New("lists table unavailable")
	// Only the newsletter list is attempted: its failure must abort before
	// the VIP list, before contacts and before segments.
	d.listRepo.EXPECT().CreateList(gomock.Any(), "ws-gap", gomock.Any()).Return(sentinel).Times(1)
	d.segmentSvc.EXPECT().CreateSegment(gomock.Any(), gomock.Any()).Times(0)

	err := d.svc.addSampleData(context.Background(), "ws-gap")
	require.EqualError(t, err, "failed to create newsletter list: failed to create list: lists table unavailable")
	assert.ErrorIs(t, err, sentinel)
	assert.Contains(t, d.createdTplIDs, "newsletter-weekly",
		"templates (step 1) run before lists (step 2)")
}

func TestDemoService_AddSampleData_GapSubscribeFailureAborts(t *testing.T) {
	ctrl := gomock.NewController(t)
	sentinel := errors.New("no session")
	contactAuth := mocks.NewMockAuthService(ctrl)
	// Every contact call fails authentication: step 3 logs and continues,
	// step 4's GetContacts surfaces the failure and aborts the seed.
	contactAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		Return(context.Background(), nil, nil, sentinel).AnyTimes()

	d := demoGapService(t, contactAuth)
	d.expectTemplatesOK()
	d.listRepo.EXPECT().CreateList(gomock.Any(), "ws-gap", gomock.Any()).Return(nil).Times(2)
	d.segmentSvc.EXPECT().CreateSegment(gomock.Any(), gomock.Any()).Times(0)
	// Proof broadcasts/message-history never start: no repository is wired
	// for them, so reaching step 5+ would panic on the nil broadcast service.

	err := d.svc.addSampleData(context.Background(), "ws-gap")
	require.EqualError(t, err, "failed to get contacts: failed to authenticate user: no session")
	assert.ErrorIs(t, err, sentinel)
}

func TestDemoService_AddSampleData_GapBroadcastFailureAborts(t *testing.T) {
	ctrl := gomock.NewController(t)
	member := &domain.UserWorkspace{
		UserID: "u1", WorkspaceID: "ws-gap", Role: "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: false},
		},
	}
	contactAuth := mocks.NewMockAuthService(ctrl)
	contactAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		Return(context.Background(), &domain.User{ID: "u1"}, member, nil).AnyTimes()

	d := demoGapService(t, contactAuth)
	d.expectTemplatesOK()
	d.listRepo.EXPECT().CreateList(gomock.Any(), "ws-gap", gomock.Any()).Return(nil).Times(2)
	// No contacts to subscribe: step 4 succeeds trivially.
	d.contactRepo.EXPECT().GetContacts(gomock.Any(), gomock.Any()).
		Return(&domain.GetContactsResponse{Contacts: nil}, nil).Times(1)
	d.segmentSvc.EXPECT().CreateSegment(gomock.Any(), gomock.Any()).Times(0)

	// Real broadcast service over mocks, every campaign refused.
	svcB, deps, _ := demoBroadcastSvc(t)
	defer deps.ctrl.Finish()
	demoGapBroadcastExpectations(deps)
	deps.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).
		Return(errors.New("template deleted")).Times(4)
	d.svc.broadcastService = svcB.broadcastService

	err := d.svc.addSampleData(context.Background(), "ws-gap")
	require.EqualError(t, err, "failed to create any sample broadcasts")
}

func TestDemoService_AddSampleData_GapMessageHistoryFailureAborts(t *testing.T) {
	ctrl := gomock.NewController(t)
	member := &domain.UserWorkspace{
		UserID: "u1", WorkspaceID: "ws-gap", Role: "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: false},
		},
	}
	contactAuth := mocks.NewMockAuthService(ctrl)
	contactAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		Return(context.Background(), &domain.User{ID: "u1"}, member, nil).AnyTimes()

	d := demoGapService(t, contactAuth)
	d.expectTemplatesOK()
	d.listRepo.EXPECT().CreateList(gomock.Any(), "ws-gap", gomock.Any()).Return(nil).Times(2)
	d.contactRepo.EXPECT().GetContacts(gomock.Any(), gomock.Any()).
		Return(&domain.GetContactsResponse{Contacts: nil}, nil).Times(1)
	d.segmentSvc.EXPECT().CreateSegment(gomock.Any(), gomock.Any()).Times(0)

	svcB, deps, broadcastRepo := demoBroadcastSvc(t)
	defer deps.ctrl.Finish()
	demoGapBroadcastExpectations(deps)
	deps.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).Return(nil).Times(4)
	broadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Times(0) // step 7 aborts before they are marked as sent
	d.svc.broadcastService = svcB.broadcastService
	d.svc.broadcastRepo = broadcastRepo

	// Transactional notifications (step 6) succeed.
	txRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	txRepo.EXPECT().Create(gomock.Any(), "ws-gap", gomock.Any()).Return(nil).Times(2) // password reset and welcome
	txAuth := mocks.NewMockAuthService(ctrl)
	owner := &domain.UserWorkspace{UserID: "u1", WorkspaceID: "ws-gap", Role: "owner"}
	txAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		Return(context.Background(), &domain.User{ID: "u1"}, owner, nil).AnyTimes()
	templateSvc := mocks.NewMockTemplateService(ctrl)
	templateSvc.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&domain.Template{ID: "password-reset"}, nil).AnyTimes()
	d.svc.transactionalNotificationService = NewTransactionalNotificationService(
		txRepo, mocks.NewMockMessageHistoryRepository(ctrl), templateSvc,
		mocks.NewMockContactService(ctrl), mocks.NewMockEmailServiceInterface(ctrl),
		txAuth, logger.NewLoggerWithLevel("disabled"),
		mocks.NewMockWorkspaceRepository(ctrl), mocks.NewMockListRepository(ctrl), "",
	)

	// Step 7 cannot load the workspace: exact wrapped error, segments skipped.
	sentinel := errors.New("workspace gone")
	wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
	wsRepo.EXPECT().GetByID(gomock.Any(), "ws-gap").Return(nil, sentinel).Times(1)
	d.svc.workspaceRepo = wsRepo

	err := d.svc.addSampleData(context.Background(), "ws-gap")
	require.EqualError(t, err, "failed to get workspace: workspace gone")
	assert.ErrorIs(t, err, sentinel)
}

func TestDemoService_AddSampleData_GapFullSuccessNonFatalStepsDegrade(t *testing.T) {
	ctrl := gomock.NewController(t)
	member := &domain.UserWorkspace{
		UserID: "u1", WorkspaceID: "ws-gap", Role: "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: false},
		},
	}
	contactAuth := mocks.NewMockAuthService(ctrl)
	contactAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		Return(context.Background(), &domain.User{ID: "u1"}, member, nil).AnyTimes()

	d := demoGapService(t, contactAuth)
	d.expectTemplatesOK()
	d.listRepo.EXPECT().CreateList(gomock.Any(), "ws-gap", gomock.Any()).Return(nil).Times(2)
	// Subscribe (step 4) and message history (step 7) both list contacts;
	// an empty book makes both succeed without any sends.
	d.contactRepo.EXPECT().GetContacts(gomock.Any(), gomock.Any()).
		Return(&domain.GetContactsResponse{Contacts: nil}, nil).Times(2)

	var segmentIDs []string
	d.segmentSvc.EXPECT().CreateSegment(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *domain.CreateSegmentRequest) (*domain.Segment, error) {
			segmentIDs = append(segmentIDs, req.ID)
			return &domain.Segment{}, nil
		}).AnyTimes()

	svcB, deps, broadcastRepo := demoBroadcastSvc(t)
	defer deps.ctrl.Finish()
	demoGapBroadcastExpectations(deps)
	deps.repo.EXPECT().CreateBroadcast(gomock.Any(), gomock.Any()).Return(nil).Times(4)
	broadcastRepo.EXPECT().UpdateBroadcast(gomock.Any(), gomock.Any()).Return(nil).Times(4)
	d.svc.broadcastService = svcB.broadcastService
	d.svc.broadcastRepo = broadcastRepo

	txRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	txRepo.EXPECT().Create(gomock.Any(), "ws-gap", gomock.Any()).Return(nil).Times(2) // password reset and welcome
	txAuth := mocks.NewMockAuthService(ctrl)
	owner := &domain.UserWorkspace{UserID: "u1", WorkspaceID: "ws-gap", Role: "owner"}
	txAuth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		Return(context.Background(), &domain.User{ID: "u1"}, owner, nil).AnyTimes()
	templateSvc := mocks.NewMockTemplateService(ctrl)
	templateSvc.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&domain.Template{ID: "password-reset"}, nil).AnyTimes()
	d.svc.transactionalNotificationService = NewTransactionalNotificationService(
		txRepo, mocks.NewMockMessageHistoryRepository(ctrl), templateSvc,
		mocks.NewMockContactService(ctrl), mocks.NewMockEmailServiceInterface(ctrl),
		txAuth, logger.NewLoggerWithLevel("disabled"),
		mocks.NewMockWorkspaceRepository(ctrl), mocks.NewMockListRepository(ctrl), "",
	)

	wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
	wsRepo.EXPECT().GetByID(gomock.Any(), "ws-gap").
		Return(&domain.Workspace{ID: "ws-gap", Settings: domain.WorkspaceSettings{SecretKey: "sk"}}, nil).Times(1)
	// Step 7b dates the broadcasts it marked as sent.
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	dbMock.ExpectExec("UPDATE broadcasts").WillReturnResult(sqlmock.NewResult(0, 4))
	wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-gap").Return(db, nil).Times(1)
	d.svc.workspaceRepo = wsRepo

	// Steps 3b (nil webAnalyticsRepo), 9 (nil automationService) and 10
	// (nil annotationRepo) all fail/degrade internally; the seed must
	// still report success and reach the segments.
	require.NoError(t, d.svc.addSampleData(context.Background(), "ws-gap"))
	assert.Contains(t, segmentIDs, "vip_customers")
	assert.Contains(t, segmentIDs, "cart_abandoners")
}

// --- generateDeliveredInboundWebhookEventsForBatch ---

func demoGapWebhookSvc(t *testing.T, wsRepo *mocks.MockWorkspaceRepository, events *mocks.MockInboundWebhookEventRepository, withWorkspaceService bool) *DemoService {
	t.Helper()
	ctrl := gomock.NewController(t)
	log := logger.NewLoggerWithLevel("disabled")
	svc := &DemoService{logger: log}
	if events != nil {
		svc.inboundWebhookEventRepo = events
	}
	if withWorkspaceService {
		svc.workspaceService = NewWorkspaceService(
			wsRepo, nil, nil, log,
			mocks.NewMockUserServiceInterface(ctrl),
			mocks.NewMockAuthService(ctrl),
			nil, &config.Config{},
			mocks.NewMockContactService(ctrl),
			nil, nil, nil, nil, "", nil, nil, nil, nil,
		)
	}
	return svc
}

func demoGapSystemCtx() context.Context {
	return context.WithValue(context.Background(), domain.SystemCallKey, true)
}

var demoGapDeliveredAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func demoGapMsg(id, email string, deliver bool) messageEngagementData {
	return messageEngagementData{
		message:    &domain.MessageHistory{ID: id, ContactEmail: email},
		engagement: messageEngagement{shouldDeliver: deliver, deliveredTime: demoGapDeliveredAt},
	}
}

func TestDemoService_DeliveredWebhookGap_NilGuardsSkip(t *testing.T) {
	ctrl := gomock.NewController(t)
	events := mocks.NewMockInboundWebhookEventRepository(ctrl)
	// No StoreEvents expectation: any call fails the test.
	batch := []messageEngagementData{demoGapMsg("m1", "a@example.com", true)}

	noWS := demoGapWebhookSvc(t, mocks.NewMockWorkspaceRepository(ctrl), events, false)
	assert.NoError(t, noWS.generateDeliveredInboundWebhookEventsForBatch(demoGapSystemCtx(), "ws-gap", batch))

	noRepo := demoGapWebhookSvc(t, mocks.NewMockWorkspaceRepository(ctrl), nil, true)
	assert.NoError(t, noRepo.generateDeliveredInboundWebhookEventsForBatch(demoGapSystemCtx(), "ws-gap", batch))
}

func TestDemoService_DeliveredWebhookGap_WorkspaceFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	events := mocks.NewMockInboundWebhookEventRepository(ctrl)
	wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
	sentinel := errors.New("db down")
	wsRepo.EXPECT().GetByID(gomock.Any(), "ws-gap").Return(nil, sentinel).Times(1)

	svc := demoGapWebhookSvc(t, wsRepo, events, true)
	err := svc.generateDeliveredInboundWebhookEventsForBatch(
		demoGapSystemCtx(), "ws-gap", []messageEngagementData{demoGapMsg("m1", "a@example.com", true)})
	require.EqualError(t, err, "failed to get workspace: db down")
	assert.ErrorIs(t, err, sentinel)
}

func TestDemoService_DeliveredWebhookGap_NoProviderOrNothingDelivered(t *testing.T) {
	ctrl := gomock.NewController(t)
	events := mocks.NewMockInboundWebhookEventRepository(ctrl)

	t.Run("empty provider id skips storage", func(t *testing.T) {
		wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
		wsRepo.EXPECT().GetByID(gomock.Any(), "ws-gap").
			Return(&domain.Workspace{ID: "ws-gap"}, nil).Times(1)
		svc := demoGapWebhookSvc(t, wsRepo, events, true)
		err := svc.generateDeliveredInboundWebhookEventsForBatch(
			demoGapSystemCtx(), "ws-gap", []messageEngagementData{demoGapMsg("m1", "a@example.com", true)})
		assert.NoError(t, err)
	})

	t.Run("no delivered messages stores nothing", func(t *testing.T) {
		wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
		wsRepo.EXPECT().GetByID(gomock.Any(), "ws-gap").
			Return(&domain.Workspace{ID: "ws-gap", Settings: domain.WorkspaceSettings{TransactionalEmailProviderID: "int-1"}}, nil).Times(2)
		svc := demoGapWebhookSvc(t, wsRepo, events, true)
		batch := []messageEngagementData{
			demoGapMsg("m1", "a@example.com", false),
			demoGapMsg("m2", "b@example.com", false),
		}
		assert.NoError(t, svc.generateDeliveredInboundWebhookEventsForBatch(demoGapSystemCtx(), "ws-gap", batch))
		assert.NoError(t, svc.generateDeliveredInboundWebhookEventsForBatch(demoGapSystemCtx(), "ws-gap", nil))
	})
}

func TestDemoService_DeliveredWebhookGap_ExactBatchRows(t *testing.T) {
	// The demo reads as sent through SES: each email's delivery, hard bounce or spam complaint is
	// the SNS notification SES posts, which the real parser reads back as the same event, and the
	// message carries the same event. It once stored "SMTP · Delivered" webhooks beside messages
	// that stayed "Sent".
	ctrl := gomock.NewController(t)
	events := mocks.NewMockInboundWebhookEventRepository(ctrl)
	wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
	wsRepo.EXPECT().GetByID(gomock.Any(), "ws-gap").
		Return(&domain.Workspace{ID: "ws-gap", Settings: domain.WorkspaceSettings{
			TransactionalEmailProviderID: "int-42",
			MarketingEmailProviderID:     "int-7",
		}}, nil).Times(1)

	var stored []*domain.InboundWebhookEvent
	var storedWS string
	events.EXPECT().StoreEvents(gomock.Any(), "ws-gap", gomock.Any()).
		DoAndReturn(func(_ context.Context, ws string, evts []*domain.InboundWebhookEvent) error {
			storedWS = ws
			stored = evts
			return nil
		}).Times(1)

	msgHistory := mocks.NewMockMessageHistoryRepository(ctrl)
	var recorded []domain.MessageEventUpdate
	msgHistory.EXPECT().SetStatusesIfNotSet(gomock.Any(), "ws-gap", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, updates []domain.MessageEventUpdate) error {
			recorded = updates
			return nil
		}).Times(1)

	// As ProcessWebhook does, the hard-bounced address is bounced on every list it is on.
	contacts := mocks.NewMockContactRepository(ctrl)
	// At the bounce, not at the seed: the list change shows on the contact's timeline.
	contacts.EXPECT().MarkEmailsAsBouncedAt(gomock.Any(), "ws-gap", map[string]time.Time{"d@example.com": demoGapDeliveredAt.Add(-time.Minute)}).Return(nil).Times(1)

	svc := demoGapWebhookSvc(t, wsRepo, events, true)
	svc.messageHistoryRepo = msgHistory
	svc.contactRepo = contacts
	bouncedAt := demoGapDeliveredAt.Add(-time.Minute)
	complainedAt := demoGapDeliveredAt.Add(time.Hour)
	newsletter := "bc-1"
	bounced := demoGapMsg("msg-4", "d@example.com", false)
	bounced.engagement.shouldBounce = true
	bounced.engagement.bouncedTime = bouncedAt
	complained := demoGapMsg("msg-3", "c@example.com", true)
	complained.message.BroadcastID = &newsletter
	complained.engagement.shouldComplain = true
	complained.engagement.complainedTime = complainedAt
	batch := []messageEngagementData{
		demoGapMsg("msg-1", "a@example.com", true),
		demoGapMsg("msg-2", "b@example.com", false), // nothing reported yet: no event
		complained,
		bounced,
	}
	require.NoError(t, svc.generateDeliveredInboundWebhookEventsForBatch(demoGapSystemCtx(), "ws-gap", batch))

	assert.Equal(t, "ws-gap", storedWS)
	reason := demoSESBounceStatusInfo()
	assert.Equal(t, []domain.MessageEventUpdate{
		{ID: "msg-1", Event: domain.MessageEventDelivered, Timestamp: demoGapDeliveredAt},
		{ID: "msg-3", Event: domain.MessageEventDelivered, Timestamp: demoGapDeliveredAt},
		{ID: "msg-3", Event: domain.MessageEventComplained, Timestamp: complainedAt},
		{ID: "msg-4", Event: domain.MessageEventBounced, Timestamp: bouncedAt, StatusInfo: &reason},
	}, recorded)

	want := []struct {
		id, email, integration string
		eventType              domain.EmailEventType
		at                     time.Time
	}{
		{"msg-1", "a@example.com", "int-42", domain.EmailEventDelivered, demoGapDeliveredAt},
		// A newsletter went through the marketing provider.
		{"msg-3", "c@example.com", "int-7", domain.EmailEventDelivered, demoGapDeliveredAt},
		{"msg-3", "c@example.com", "int-7", domain.EmailEventComplaint, complainedAt},
		{"msg-4", "d@example.com", "int-42", domain.EmailEventBounce, bouncedAt},
	}
	require.Len(t, stored, len(want))
	parser := &InboundWebhookEventService{logger: logger.NewLoggerWithLevel("disabled")}
	seenIDs := map[string]bool{}
	for i, evt := range stored {
		assert.Equal(t, want[i].eventType, evt.Type)
		assert.Equal(t, domain.WebhookSourceSES, evt.Source)
		assert.Equal(t, want[i].integration, evt.IntegrationID)
		assert.Equal(t, want[i].email, evt.RecipientEmail)
		require.NotNil(t, evt.MessageID)
		assert.Equal(t, want[i].id, *evt.MessageID)
		assert.Equal(t, want[i].at, evt.Timestamp)
		assert.Equal(t, want[i].at, evt.CreatedAt)
		assert.NotEmpty(t, evt.ID)
		assert.False(t, seenIDs[evt.ID], "event IDs must be unique")
		seenIDs[evt.ID] = true

		// What SES would have posted: the real parser reads the same event out of it.
		parsed, err := parser.processSESWebhook(evt.IntegrationID, []byte(evt.RawPayload))
		require.NoError(t, err)
		require.Len(t, parsed, 1, "event %d", i)
		assert.Equal(t, evt.Type, parsed[0].Type)
		assert.Equal(t, domain.WebhookSourceSES, parsed[0].Source)
		assert.Equal(t, evt.RecipientEmail, parsed[0].RecipientEmail)
		assert.Equal(t, *evt.MessageID, *parsed[0].MessageID)
		assert.True(t, evt.Timestamp.Equal(parsed[0].Timestamp))
		assert.Equal(t, evt.BounceType, parsed[0].BounceType)
		assert.Equal(t, evt.BounceCategory, parsed[0].BounceCategory)
		assert.Equal(t, evt.BounceDiagnostic, parsed[0].BounceDiagnostic)
		assert.Equal(t, evt.ComplaintFeedbackType, parsed[0].ComplaintFeedbackType)
	}
	// The bounce reads as hard, as ProcessWebhook classifies it.
	assert.Equal(t, domain.BounceClassificationHard, domain.ClassifyBounce(domain.BounceInput{
		Provider: domain.EmailProviderKindSES, Type: stored[3].BounceType, Subtype: stored[3].BounceCategory, Diagnostic: stored[3].BounceDiagnostic,
	}))
}

func TestDemoBounceOrDeliver(t *testing.T) {
	sent := demoGapDeliveredAt
	now := sent.AddDate(0, 0, 1)
	var bounced messageEngagement
	demoBounceOrDeliver(&bounced, now, sent, 1)
	assert.True(t, bounced.shouldBounce)
	assert.False(t, bounced.shouldDeliver, "a bounced email is never delivered")
	assert.True(t, bounced.bouncedTime.After(sent))

	var delivered messageEngagement
	demoBounceOrDeliver(&delivered, now, sent, 0)
	assert.True(t, delivered.shouldDeliver)
	assert.False(t, delivered.shouldBounce)
	assert.False(t, delivered.deliveredTime.Before(sent))
}

func TestDemoService_StoreDemoDeliveredSESEvents(t *testing.T) {
	// The automation history writes delivered_at itself: SES reported each of those deliveries.
	ctrl := gomock.NewController(t)
	events := mocks.NewMockInboundWebhookEventRepository(ctrl)
	wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
	wsRepo.EXPECT().GetByID(gomock.Any(), "ws-gap").
		Return(&domain.Workspace{ID: "ws-gap", Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "int-7"}}, nil).Times(1)
	var stored []*domain.InboundWebhookEvent
	events.EXPECT().StoreEvents(gomock.Any(), "ws-gap", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, evts []*domain.InboundWebhookEvent) error {
			stored = evts
			return nil
		}).Times(1)

	automation := "auto-1"
	at := demoGapDeliveredAt
	svc := demoGapWebhookSvc(t, wsRepo, events, true)
	require.NoError(t, svc.storeDemoDeliveredSESEvents(demoGapSystemCtx(), "ws-gap", []*domain.MessageHistory{
		{ID: "m-1", ContactEmail: "a@example.com", AutomationID: &automation, DeliveredAt: &at},
		{ID: "m-2", ContactEmail: "b@example.com", AutomationID: &automation}, // not delivered: no event
	}))
	require.Len(t, stored, 1)
	assert.Equal(t, domain.EmailEventDelivered, stored[0].Type)
	assert.Equal(t, domain.WebhookSourceSES, stored[0].Source)
	assert.Equal(t, "int-7", stored[0].IntegrationID)
	assert.Equal(t, "m-1", *stored[0].MessageID)
	assert.Equal(t, at, stored[0].Timestamp)
}

func TestDemoService_DeliveredWebhookGap_StoreFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	events := mocks.NewMockInboundWebhookEventRepository(ctrl)
	wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
	wsRepo.EXPECT().GetByID(gomock.Any(), "ws-gap").
		Return(&domain.Workspace{ID: "ws-gap", Settings: domain.WorkspaceSettings{TransactionalEmailProviderID: "int-42"}}, nil).Times(1)
	sentinel := errors.New("insert failed")
	events.EXPECT().StoreEvents(gomock.Any(), "ws-gap", gomock.Any()).Return(sentinel).Times(1)

	svc := demoGapWebhookSvc(t, wsRepo, events, true)
	err := svc.generateDeliveredInboundWebhookEventsForBatch(
		demoGapSystemCtx(), "ws-gap", []messageEngagementData{demoGapMsg("m1", "a@example.com", true)})
	require.EqualError(t, err, "failed to store inbound webhook events: insert failed")
	assert.ErrorIs(t, err, sentinel)
}

func TestDemoService_DeliveredWebhookGap_RecordFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	events := mocks.NewMockInboundWebhookEventRepository(ctrl)
	wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
	wsRepo.EXPECT().GetByID(gomock.Any(), "ws-gap").
		Return(&domain.Workspace{ID: "ws-gap", Settings: domain.WorkspaceSettings{TransactionalEmailProviderID: "int-42"}}, nil).Times(1)
	events.EXPECT().StoreEvents(gomock.Any(), "ws-gap", gomock.Any()).Return(nil).Times(1)
	msgHistory := mocks.NewMockMessageHistoryRepository(ctrl)
	sentinel := errors.New("update failed")
	msgHistory.EXPECT().SetStatusesIfNotSet(gomock.Any(), "ws-gap", gomock.Any()).Return(sentinel).Times(1)

	svc := demoGapWebhookSvc(t, wsRepo, events, true)
	svc.messageHistoryRepo = msgHistory
	err := svc.generateDeliveredInboundWebhookEventsForBatch(
		demoGapSystemCtx(), "ws-gap", []messageEngagementData{demoGapMsg("m1", "a@example.com", true)})
	require.EqualError(t, err, "failed to record deliveries: update failed")
	assert.ErrorIs(t, err, sentinel)
}
