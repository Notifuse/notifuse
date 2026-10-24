package app

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/internal/service/queue"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCoverage_AppGettersReturnExactFields pins every trivial getter to its
// own field: a getter rewired to a sibling field must fail here. Distinct
// sentinel values per field make a swap observable.
func TestCoverage_AppGettersReturnExactFields(t *testing.T) {
	ctrl := gomock.NewController(t)

	webAnalyticsRepo := mocks.NewMockWebAnalyticsRepository(ctrl)
	customEventRepo := mocks.NewMockCustomEventRepository(ctrl)
	emailQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	taskRepo := mocks.NewMockTaskRepository(ctrl)
	userRepo := mocks.NewMockUserRepository(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	contactRepo := mocks.NewMockContactRepository(ctrl)
	listRepo := mocks.NewMockListRepository(ctrl)
	templateRepo := mocks.NewMockTemplateRepository(ctrl)
	broadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	contactListRepo := mocks.NewMockContactListRepository(ctrl)
	transactionalRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	telemetryRepo := mocks.NewMockTelemetryRepository(ctrl)

	emailQueueWorker := &queue.EmailQueueWorker{}
	emailService := &service.EmailService{}
	automationScheduler := &service.AutomationScheduler{}
	taskScheduler := &service.TaskScheduler{}
	auditLogService := &service.AuditLogService{}
	webAnalyticsBuffer := &service.WebAnalyticsBuffer{}
	authService := &service.AuthService{}
	transactionalService := &service.TransactionalNotificationService{}

	cfg := &config.Config{Environment: "coverage-sentinel"}
	mux := http.NewServeMux()
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	logger := pkgmocks.NewMockLogger(ctrl)
	mailer := pkgmocks.NewMockMailer(ctrl)

	app := &App{
		config: cfg, mux: mux, db: db, logger: logger, mailer: mailer,
		webAnalyticsRepo: webAnalyticsRepo, customEventRepo: customEventRepo,
		emailQueueRepo: emailQueueRepo, taskRepo: taskRepo,
		userRepo: userRepo, workspaceRepo: workspaceRepo, contactRepo: contactRepo,
		listRepo: listRepo, templateRepo: templateRepo, broadcastRepo: broadcastRepo,
		messageHistoryRepo: messageHistoryRepo, contactListRepo: contactListRepo,
		transactionalNotificationRepo: transactionalRepo, telemetryRepo: telemetryRepo,
		emailQueueWorker: emailQueueWorker, emailService: emailService,
		automationScheduler: automationScheduler, taskScheduler: taskScheduler,
		auditLogService: auditLogService, webAnalyticsBuffer: webAnalyticsBuffer,
		authService: authService, transactionalNotificationService: transactionalService,
	}

	// Repository getters: interface identity, not just non-nil.
	assert.True(t, app.GetWebAnalyticsRepository() == webAnalyticsRepo)
	assert.True(t, app.GetCustomEventRepository() == customEventRepo)
	assert.True(t, app.GetEmailQueueRepository() == emailQueueRepo)
	assert.True(t, app.GetTaskRepository() == taskRepo)
	assert.True(t, app.GetUserRepository() == userRepo)
	assert.True(t, app.GetWorkspaceRepository() == workspaceRepo)
	assert.True(t, app.GetContactRepository() == contactRepo)
	assert.True(t, app.GetListRepository() == listRepo)
	assert.True(t, app.GetTemplateRepository() == templateRepo)
	assert.True(t, app.GetBroadcastRepository() == broadcastRepo)
	assert.True(t, app.GetMessageHistoryRepository() == messageHistoryRepo)
	assert.True(t, app.GetContactListRepository() == contactListRepo)
	assert.True(t, app.GetTransactionalNotificationRepository() == transactionalRepo)
	assert.True(t, app.GetTelemetryRepository() == telemetryRepo)

	// Pointer getters: exact sentinel pointer.
	assert.True(t, app.GetEmailQueueWorker() == emailQueueWorker)
	assert.True(t, app.GetAutomationScheduler() == automationScheduler)
	assert.True(t, app.GetTaskScheduler() == taskScheduler)
	assert.True(t, app.GetAuditLogService() == auditLogService)
	assert.True(t, app.GetWebAnalyticsBuffer() == webAnalyticsBuffer)

	// The email service is stored as a concrete *service.EmailService and
	// exposed as domain.EmailServiceInterface: identity must survive.
	gotEmail, ok := app.GetEmailService().(*service.EmailService)
	require.True(t, ok, "GetEmailService must return the stored *service.EmailService")
	assert.True(t, gotEmail == emailService)

	gotTransactional, ok := app.GetTransactionalNotificationService().(*service.TransactionalNotificationService)
	require.True(t, ok)
	assert.True(t, gotTransactional == transactionalService)
	assert.True(t, app.GetAuthService() == authService)

	// Scalar/pointer infrastructure getters.
	assert.True(t, app.GetConfig() == cfg)
	assert.Equal(t, "coverage-sentinel", app.GetConfig().Environment)
	assert.True(t, app.GetMux() == mux)
	assert.True(t, app.GetDB() == (*sql.DB)(db))
	assert.True(t, app.GetLogger() == logger)
	assert.True(t, app.GetMailer() == mailer)
}

func TestCoverage_AppGettersZeroValueAreNil(t *testing.T) {
	app := &App{}
	assert.Nil(t, app.GetWebAnalyticsRepository())
	assert.Nil(t, app.GetCustomEventRepository())
	assert.Nil(t, app.GetEmailQueueRepository())
	assert.Nil(t, app.GetTaskRepository())
	assert.Nil(t, app.GetEmailQueueWorker())
	assert.Nil(t, app.GetEmailService())
	assert.Nil(t, app.GetAutomationScheduler())
	assert.Nil(t, app.GetAuditLogService())
	assert.Nil(t, app.GetWebAnalyticsBuffer())
	assert.Nil(t, app.GetUserRepository())
	assert.Nil(t, app.GetWorkspaceRepository())
	assert.Nil(t, app.GetContactRepository())
	assert.Nil(t, app.GetConfig())
	assert.Nil(t, app.GetDB())
	assert.Nil(t, app.GetMux())
}

func TestCoverage_AppGettersAreNotSwapped(t *testing.T) {
	ctrl := gomock.NewController(t)
	first := mocks.NewMockTaskRepository(ctrl)
	second := mocks.NewMockTaskRepository(ctrl)
	require.True(t, first != second)
	app := &App{taskRepo: first, emailQueueRepo: mocks.NewMockEmailQueueRepository(ctrl)}
	assert.True(t, app.GetTaskRepository() == first)
	assert.False(t, app.GetTaskRepository() == second)

	workerA := &queue.EmailQueueWorker{}
	workerB := &queue.EmailQueueWorker{}
	app2 := &App{emailQueueWorker: workerA}
	assert.True(t, app2.GetEmailQueueWorker() == workerA)
	assert.False(t, app2.GetEmailQueueWorker() == workerB)
}
