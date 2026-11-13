package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	svix "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var supabaseHandlerSecret = base64.StdEncoding.EncodeToString([]byte("supabase-handler-secret-0123456"))

type supabaseHandlerFixture struct {
	handler          *SupabaseWebhookHandler
	workspaceRepo    *mocks.MockWorkspaceRepository
	transactionalRep *mocks.MockTransactionalNotificationRepository
	transactionalSvc *mocks.MockTransactionalNotificationService
	webhookEventRepo *mocks.MockInboundWebhookEventRepository
}

// newSupabaseHandlerFixture wires the handler to a real Supabase service over strict
// mocks: the handler takes the concrete service.
func newSupabaseHandlerFixture(t *testing.T) *supabaseHandlerFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	f := &supabaseHandlerFixture{
		workspaceRepo:    mocks.NewMockWorkspaceRepository(ctrl),
		transactionalRep: mocks.NewMockTransactionalNotificationRepository(ctrl),
		transactionalSvc: mocks.NewMockTransactionalNotificationService(ctrl),
		webhookEventRepo: mocks.NewMockInboundWebhookEventRepository(ctrl),
	}
	log := logger.NewLoggerWithLevel("disabled")
	supabase := service.NewSupabaseService(f.workspaceRepo, nil, mocks.NewMockContactService(ctrl),
		mocks.NewMockListRepository(ctrl), mocks.NewMockContactListRepository(ctrl), nil, nil,
		f.transactionalRep, f.transactionalSvc, f.webhookEventRepo, log)
	f.handler = NewSupabaseWebhookHandler(supabase, log)
	return f
}

func (f *supabaseHandlerFixture) workspace(rejectDisposable bool) *domain.Workspace {
	return &domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{{
		ID:   "int-1",
		Type: domain.IntegrationTypeSupabase,
		SupabaseSettings: &domain.SupabaseIntegrationSettings{
			AuthEmailHook: domain.SupabaseAuthEmailHookSettings{SignatureKey: supabaseHandlerSecret},
			BeforeUserCreatedHook: domain.SupabaseUserCreatedHookSettings{
				SignatureKey:          supabaseHandlerSecret,
				RejectDisposableEmail: rejectDisposable,
			},
		},
	}}}
}

// signedSupabaseRequest builds a hook request signed as Supabase signs it.
func signedSupabaseRequest(t *testing.T, path string, payload interface{}) *http.Request {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	wh, err := svix.NewWebhook(supabaseHandlerSecret)
	require.NoError(t, err)
	now := time.Now()
	signature, err := wh.Sign("msg_1", now, body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, path+"?workspace_id=ws-1&integration_id=int-1", strings.NewReader(string(body)))
	req.Header.Set("webhook-id", "msg_1")
	req.Header.Set("webhook-timestamp", strconv.FormatInt(now.Unix(), 10))
	req.Header.Set("webhook-signature", signature)
	return req
}

// The auth email hook fails loudly: Supabase reports the error to the user signing in,
// where a silent success would leave them waiting for an email that never comes.
func TestSupabaseAuthEmailWebhook_Processing(t *testing.T) {
	payload := domain.SupabaseAuthEmailWebhook{
		User:      domain.SupabaseUser{ID: "user-1", Email: "jane@example.com"},
		EmailData: domain.SupabaseEmailData{EmailActionType: "signup", Token: "123456"},
	}

	t.Run("the email is sent", func(t *testing.T) {
		f := newSupabaseHandlerFixture(t)
		integrationID := "int-1"
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(f.workspace(false), nil)
		f.webhookEventRepo.EXPECT().StoreEvents(gomock.Any(), "ws-1", gomock.Any()).Return(nil)
		f.transactionalRep.EXPECT().List(gomock.Any(), "ws-1", gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]*domain.TransactionalNotification{{ID: "supabase_signup_confirm", IntegrationID: &integrationID}}, 1, nil)
		f.transactionalSvc.EXPECT().SendNotification(gomock.Any(), "ws-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, params domain.TransactionalNotificationSendParams) (string, error) {
				assert.Equal(t, "supabase_signup_confirm", params.ID)
				assert.Equal(t, "jane@example.com", params.Contact.Email)
				return "msg-1", nil
			})

		rec := httptest.NewRecorder()
		f.handler.handleAuthEmailWebhook(rec, signedSupabaseRequest(t, "/webhooks/supabase/auth-email", payload))

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"success": true}`, rec.Body.String())
	})

	// The hook URL names an integration the workspace no longer has.
	t.Run("a hook the workspace does not know is a 400", func(t *testing.T) {
		f := newSupabaseHandlerFixture(t)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(&domain.Workspace{ID: "ws-1"}, nil)

		rec := httptest.NewRecorder()
		f.handler.handleAuthEmailWebhook(rec, signedSupabaseRequest(t, "/webhooks/supabase/auth-email", payload))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to process webhook")
		assert.NotContains(t, rec.Body.String(), "int-1", "the internal error stays in the logs")
	})
}

// The before-user-created hook blocks a sign-up only for a disposable address; a
// failure on the Notifuse side lets the sign-up through.
func TestSupabaseUserCreatedWebhook_Processing(t *testing.T) {
	t.Run("a disposable address is refused in the hook's error format", func(t *testing.T) {
		f := newSupabaseHandlerFixture(t)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(f.workspace(true), nil)
		f.webhookEventRepo.EXPECT().StoreEvents(gomock.Any(), "ws-1", gomock.Any()).Return(nil)

		rec := httptest.NewRecorder()
		f.handler.handleUserCreatedWebhook(rec, signedSupabaseRequest(t, "/webhooks/supabase/before-user-created",
			domain.SupabaseBeforeUserCreatedWebhook{User: domain.SupabaseUser{ID: "user-2", Email: "x@guerrillamail.com"}}))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.JSONEq(t, `{"error": {"http_code": 400, "message": "disposable email addresses are not allowed"}}`, rec.Body.String())
	})

	t.Run("a Notifuse failure does not block the sign-up", func(t *testing.T) {
		f := newSupabaseHandlerFixture(t)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(nil, errors.New("db gone"))

		rec := httptest.NewRecorder()
		f.handler.handleUserCreatedWebhook(rec, signedSupabaseRequest(t, "/webhooks/supabase/before-user-created",
			domain.SupabaseBeforeUserCreatedWebhook{User: domain.SupabaseUser{ID: "user-3", Email: "jane@example.com"}}))

		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Empty(t, rec.Body.String())
	})
}
