package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	svix "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// These tests pin the two Supabase hook contracts: the auth-email hook fails
// loudly (a missed auth email is visible), while the before-user-created hook
// must never fail signup (every internal error returns nil) except for the
// deliberate disposable-email rejection.

var supabaseTestSecret = base64.StdEncoding.EncodeToString([]byte("supabase-test-secret-0123456789ab"))

func signSupabasePayload(t *testing.T, secret string, payload []byte) (webhookID, timestamp, signature string) {
	t.Helper()
	wh, err := svix.NewWebhook(secret)
	require.NoError(t, err)
	webhookID = "msg_test_1"
	now := time.Now()
	timestamp = strconv.FormatInt(now.Unix(), 10)
	signature, err = wh.Sign(webhookID, now, payload)
	require.NoError(t, err)
	return webhookID, timestamp, signature
}

type supabaseHookFixture struct {
	svc              *SupabaseService
	workspaceRepo    *mocks.MockWorkspaceRepository
	transactionalRep *mocks.MockTransactionalNotificationRepository
	transactionalSvc *mocks.MockTransactionalNotificationService
	contactService   *mocks.MockContactService
	listRepo         *mocks.MockListRepository
	contactListRepo  *mocks.MockContactListRepository
	webhookEventRepo *mocks.MockInboundWebhookEventRepository
}

func newSupabaseHookFixture(t *testing.T) *supabaseHookFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	f := &supabaseHookFixture{
		workspaceRepo:    mocks.NewMockWorkspaceRepository(ctrl),
		transactionalRep: mocks.NewMockTransactionalNotificationRepository(ctrl),
		transactionalSvc: mocks.NewMockTransactionalNotificationService(ctrl),
		contactService:   mocks.NewMockContactService(ctrl),
		listRepo:         mocks.NewMockListRepository(ctrl),
		contactListRepo:  mocks.NewMockContactListRepository(ctrl),
		webhookEventRepo: mocks.NewMockInboundWebhookEventRepository(ctrl),
	}
	f.svc = NewSupabaseService(
		f.workspaceRepo,
		nil, // emailService
		f.contactService,
		f.listRepo,
		f.contactListRepo,
		nil, // templateRepo
		nil, // templateService
		f.transactionalRep,
		f.transactionalSvc,
		f.webhookEventRepo,
		logger.NewLoggerWithLevel("disabled"),
	)
	return f
}

func supabaseWorkspace(secret string) *domain.Workspace {
	return &domain.Workspace{
		ID: "ws-1",
		Integrations: []domain.Integration{
			{
				ID:   "int-1",
				Type: domain.IntegrationTypeSupabase,
				SupabaseSettings: &domain.SupabaseIntegrationSettings{
					AuthEmailHook: domain.SupabaseAuthEmailHookSettings{SignatureKey: secret},
					BeforeUserCreatedHook: domain.SupabaseUserCreatedHookSettings{
						SignatureKey: secret,
					},
				},
			},
		},
	}
}

func authEmailPayload(t *testing.T, action string, mutate func(*domain.SupabaseAuthEmailWebhook)) []byte {
	t.Helper()
	webhook := domain.SupabaseAuthEmailWebhook{
		User: domain.SupabaseUser{
			ID: "user-1", Email: "old@example.com", EmailNew: "new@example.com",
		},
		EmailData: domain.SupabaseEmailData{
			Token: "tok-current", TokenHash: "hash-for-new", TokenNew: "tok-new", TokenHashNew: "hash-for-current",
			EmailActionType: action, RedirectTo: "https://app.example.com/callback", SiteURL: "https://app.example.com",
		},
	}
	if mutate != nil {
		mutate(&webhook)
	}
	payload, err := json.Marshal(webhook)
	require.NoError(t, err)
	return payload
}

func (f *supabaseHookFixture) expectWorkspace(ws *domain.Workspace) {
	f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(ws, nil).Times(1)
}

func (f *supabaseHookFixture) expectStoredEvent() {
	f.webhookEventRepo.EXPECT().StoreEvents(gomock.Any(), "ws-1", gomock.Any()).Return(nil).Times(1)
}

func TestProcessAuthEmailHook_Guards(t *testing.T) {
	ctx := context.Background()

	t.Run("workspace lookup failure is wrapped", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		sentinel := errors.New("db gone")
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(nil, sentinel).Times(1)
		err := f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-1", []byte("{}"), "id", "ts", "sig")
		require.ErrorContains(t, err, "failed to get workspace:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("unknown integration id is named in the error", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		err := f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-missing", []byte("{}"), "id", "ts", "sig")
		require.EqualError(t, err, "integration not found: int-missing")
	})

	t.Run("a non-Supabase integration is rejected", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		ws := supabaseWorkspace(supabaseTestSecret)
		ws.Integrations[0].Type = "zapier"
		f.expectWorkspace(ws)
		err := f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-1", []byte("{}"), "id", "ts", "sig")
		require.EqualError(t, err, "integration is not a Supabase integration")
	})

	t.Run("missing supabase settings are rejected", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		ws := supabaseWorkspace(supabaseTestSecret)
		ws.Integrations[0].SupabaseSettings = nil
		f.expectWorkspace(ws)
		err := f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-1", []byte("{}"), "id", "ts", "sig")
		require.EqualError(t, err, "supabase settings not found")
	})

	t.Run("a bad signature is rejected and no send happens", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		payload := authEmailPayload(t, "signup", nil)
		_, ts, _ := signSupabasePayload(t, supabaseTestSecret, payload)
		err := f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-1", payload, "msg_test_1", ts, "v1,invalidsignature")
		require.ErrorContains(t, err, "invalid webhook signature:")
	})

	t.Run("a tampered payload fails signature validation", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		payload := authEmailPayload(t, "signup", nil)
		id, ts, sig := signSupabasePayload(t, supabaseTestSecret, payload)
		tampered := authEmailPayload(t, "signup", func(w *domain.SupabaseAuthEmailWebhook) {
			w.User.Email = "attacker@example.com"
		})
		err := f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-1", tampered, id, ts, sig)
		require.ErrorContains(t, err, "invalid webhook signature:")
	})

	t.Run("valid signature but malformed JSON is a parse error", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		payload := []byte("{not json")
		id, ts, sig := signSupabasePayload(t, supabaseTestSecret, payload)
		err := f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-1", payload, id, ts, sig)
		require.ErrorContains(t, err, "failed to parse webhook payload:")
	})
}

func TestProcessAuthEmailHook_SignupFlow(t *testing.T) {
	ctx := context.Background()

	t.Run("sends the notification registered for the action and integration", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		payload := authEmailPayload(t, "signup", nil)
		id, ts, sig := signSupabasePayload(t, supabaseTestSecret, payload)
		f.expectStoredEvent()
		f.transactionalRep.EXPECT().List(gomock.Any(), "ws-1", gomock.Any(), 1000, 0).
			Return([]*domain.TransactionalNotification{
				{ID: "supabase_signup_main", IntegrationID: strPtr("int-1")},
				{ID: "supabase_signup_other_ws", IntegrationID: strPtr("int-other")},
			}, 2, nil).Times(1)
		f.transactionalSvc.EXPECT().SendNotification(gomock.Any(), "ws-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, params domain.TransactionalNotificationSendParams) (string, error) {
				assert.Equal(t, "supabase_signup_main", params.ID)
				require.NotNil(t, params.Contact)
				assert.Equal(t, "old@example.com", params.Contact.Email)
				return "msg-1", nil
			}).Times(1)
		require.NoError(t, f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-1", payload, id, ts, sig))
	})

	t.Run("a notification for another integration is not used", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		payload := authEmailPayload(t, "signup", nil)
		id, ts, sig := signSupabasePayload(t, supabaseTestSecret, payload)
		f.expectStoredEvent()
		f.transactionalRep.EXPECT().List(gomock.Any(), "ws-1", gomock.Any(), 1000, 0).
			Return([]*domain.TransactionalNotification{
				{ID: "supabase_signup_main", IntegrationID: strPtr("int-other")},
			}, 1, nil).Times(1)
		err := f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-1", payload, id, ts, sig)
		require.ErrorContains(t, err, "failed to get notification for action signup:")
		assert.Contains(t, err.Error(), "notification not found for action type: signup")
	})

	t.Run("notification list failure is wrapped", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		payload := authEmailPayload(t, "recovery", nil)
		id, ts, sig := signSupabasePayload(t, supabaseTestSecret, payload)
		f.expectStoredEvent()
		sentinel := errors.New("list exploded")
		f.transactionalRep.EXPECT().List(gomock.Any(), "ws-1", gomock.Any(), 1000, 0).
			Return(nil, 0, sentinel).Times(1)
		err := f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-1", payload, id, ts, sig)
		require.ErrorContains(t, err, "failed to get notification for action recovery:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("send failure reaches the caller", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		payload := authEmailPayload(t, "signup", nil)
		id, ts, sig := signSupabasePayload(t, supabaseTestSecret, payload)
		f.expectStoredEvent()
		f.transactionalRep.EXPECT().List(gomock.Any(), "ws-1", gomock.Any(), 1000, 0).
			Return([]*domain.TransactionalNotification{
				{ID: "supabase_signup_main", IntegrationID: strPtr("int-1")},
			}, 1, nil).Times(1)
		sentinel := errors.New("smtp down")
		f.transactionalSvc.EXPECT().SendNotification(gomock.Any(), "ws-1", gomock.Any()).
			Return("", sentinel).Times(1)
		err := f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-1", payload, id, ts, sig)
		require.ErrorContains(t, err, "failed to send transactional notification:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestProcessAuthEmailHook_EmailChange(t *testing.T) {
	ctx := context.Background()

	expectEmailChangeNotification := func(f *supabaseHookFixture) {
		f.transactionalRep.EXPECT().List(gomock.Any(), "ws-1", gomock.Any(), 1000, 0).
			Return([]*domain.TransactionalNotification{
				{ID: "supabase_email_change_main", IntegrationID: strPtr("int-1")},
			}, 1, nil).Times(1)
	}

	t.Run("secure mode emails both addresses with their own token hashes", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		payload := authEmailPayload(t, "email_change", nil)
		id, ts, sig := signSupabasePayload(t, supabaseTestSecret, payload)
		f.expectStoredEvent()
		expectEmailChangeNotification(f)

		var sent []domain.TransactionalNotificationSendParams
		f.transactionalSvc.EXPECT().SendNotification(gomock.Any(), "ws-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, params domain.TransactionalNotificationSendParams) (string, error) {
				sent = append(sent, params)
				return "msg", nil
			}).Times(2)

		require.NoError(t, f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-1", payload, id, ts, sig))
		require.Len(t, sent, 2)
		// Current address first, with token_hash_new (Supabase's counterintuitive naming).
		assert.Equal(t, "old@example.com", sent[0].Contact.Email)
		assert.Equal(t, "hash-for-current", sent[0].Data["token_hash"])
		assert.Equal(t, "tok-current", sent[0].Data["token"])
		// New address second, with the plain token_hash.
		assert.Equal(t, "new@example.com", sent[1].Contact.Email)
		assert.Equal(t, "hash-for-new", sent[1].Data["token_hash"])
		assert.Equal(t, "tok-new", sent[1].Data["token"])
	})

	t.Run("a failed send to the current address still emails the new address", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		payload := authEmailPayload(t, "email_change", nil)
		id, ts, sig := signSupabasePayload(t, supabaseTestSecret, payload)
		f.expectStoredEvent()
		expectEmailChangeNotification(f)

		gomock.InOrder(
			f.transactionalSvc.EXPECT().SendNotification(gomock.Any(), "ws-1", gomock.Any()).
				Return("", errors.New("current inbox bounced")),
			f.transactionalSvc.EXPECT().SendNotification(gomock.Any(), "ws-1", gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, params domain.TransactionalNotificationSendParams) (string, error) {
					assert.Equal(t, "new@example.com", params.Contact.Email)
					return "msg", nil
				}),
		)
		require.NoError(t, f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-1", payload, id, ts, sig),
			"the second send's success decides the outcome")
	})

	t.Run("a failed send to the new address is the returned error", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		payload := authEmailPayload(t, "email_change", nil)
		id, ts, sig := signSupabasePayload(t, supabaseTestSecret, payload)
		f.expectStoredEvent()
		expectEmailChangeNotification(f)

		sentinel := errors.New("new inbox unreachable")
		gomock.InOrder(
			f.transactionalSvc.EXPECT().SendNotification(gomock.Any(), "ws-1", gomock.Any()).Return("msg", nil),
			f.transactionalSvc.EXPECT().SendNotification(gomock.Any(), "ws-1", gomock.Any()).Return("", sentinel),
		)
		err := f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-1", payload, id, ts, sig)
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("single-email mode only emails the new address, once", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		payload := authEmailPayload(t, "email_change", func(w *domain.SupabaseAuthEmailWebhook) {
			w.EmailData.TokenHashNew = "" // no secure dual-send
		})
		id, ts, sig := signSupabasePayload(t, supabaseTestSecret, payload)
		f.expectStoredEvent()
		expectEmailChangeNotification(f)
		f.transactionalSvc.EXPECT().SendNotification(gomock.Any(), "ws-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, params domain.TransactionalNotificationSendParams) (string, error) {
				assert.Equal(t, "new@example.com", params.Contact.Email)
				return "msg", nil
			}).Times(1)
		require.NoError(t, f.svc.ProcessAuthEmailHook(ctx, "ws-1", "int-1", payload, id, ts, sig))
	})
}

func TestProcessUserCreatedHook_NeverBlocksSignup(t *testing.T) {
	ctx := context.Background()
	payload, err := json.Marshal(domain.SupabaseBeforeUserCreatedWebhook{
		User: domain.SupabaseUser{ID: "user-1", Email: "person@example.com"},
	})
	require.NoError(t, err)

	t.Run("workspace failure returns nil", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws-1").Return(nil, errors.New("db gone")).Times(1)
		assert.NoError(t, f.svc.ProcessUserCreatedHook(ctx, "ws-1", "int-1", payload, "id", "ts", "sig"))
	})

	t.Run("unknown integration returns nil", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		assert.NoError(t, f.svc.ProcessUserCreatedHook(ctx, "ws-1", "int-missing", payload, "id", "ts", "sig"))
	})

	t.Run("bad signature returns nil and creates no contact", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		_, ts, _ := signSupabasePayload(t, supabaseTestSecret, payload)
		assert.NoError(t, f.svc.ProcessUserCreatedHook(ctx, "ws-1", "int-1", payload, "msg_test_1", ts, "v1,wrong"))
	})

	t.Run("disposable email is the one hard rejection", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		ws := supabaseWorkspace(supabaseTestSecret)
		ws.Integrations[0].SupabaseSettings.BeforeUserCreatedHook.RejectDisposableEmail = true
		f.expectWorkspace(ws)
		disposablePayload, err := json.Marshal(domain.SupabaseBeforeUserCreatedWebhook{
			User: domain.SupabaseUser{ID: "user-2", Email: "x@guerrillamail.com"},
		})
		require.NoError(t, err)
		id, ts, sig := signSupabasePayload(t, supabaseTestSecret, disposablePayload)
		f.expectStoredEvent()
		err = f.svc.ProcessUserCreatedHook(ctx, "ws-1", "int-1", disposablePayload, id, ts, sig)
		require.EqualError(t, err, "disposable email addresses are not allowed")
	})

	t.Run("a normal email passes the disposable check and upserts a contact", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		ws := supabaseWorkspace(supabaseTestSecret)
		ws.Integrations[0].SupabaseSettings.BeforeUserCreatedHook.RejectDisposableEmail = true
		f.expectWorkspace(ws)
		id, ts, sig := signSupabasePayload(t, supabaseTestSecret, payload)
		f.expectStoredEvent()
		f.contactService.EXPECT().UpsertContact(gomock.Any(), "ws-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, contact *domain.Contact) domain.UpsertContactOperation {
				assert.Equal(t, "person@example.com", contact.Email)
				return domain.UpsertContactOperation{Action: domain.UpsertContactOperationCreate}
			}).Times(1)
		assert.NoError(t, f.svc.ProcessUserCreatedHook(ctx, "ws-1", "int-1", payload, id, ts, sig))
	})

	t.Run("an upsert failure still returns nil", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		f.expectWorkspace(supabaseWorkspace(supabaseTestSecret))
		id, ts, sig := signSupabasePayload(t, supabaseTestSecret, payload)
		f.expectStoredEvent()
		f.contactService.EXPECT().UpsertContact(gomock.Any(), "ws-1", gomock.Any()).
			Return(domain.UpsertContactOperation{
				Action: domain.UpsertContactOperationError, Error: "insert failed",
			}).Times(1)
		assert.NoError(t, f.svc.ProcessUserCreatedHook(ctx, "ws-1", "int-1", payload, id, ts, sig))
	})

	t.Run("configured lists are joined, missing lists are skipped", func(t *testing.T) {
		f := newSupabaseHookFixture(t)
		ws := supabaseWorkspace(supabaseTestSecret)
		ws.Integrations[0].SupabaseSettings.BeforeUserCreatedHook.AddUserToLists = []string{"list-ok", "list-gone"}
		f.expectWorkspace(ws)
		id, ts, sig := signSupabasePayload(t, supabaseTestSecret, payload)
		f.expectStoredEvent()
		f.contactService.EXPECT().UpsertContact(gomock.Any(), "ws-1", gomock.Any()).
			Return(domain.UpsertContactOperation{Action: domain.UpsertContactOperationCreate}).Times(1)
		f.listRepo.EXPECT().GetListByID(gomock.Any(), "ws-1", "list-ok").
			Return(&domain.List{ID: "list-ok", Name: "Newsletter"}, nil).Times(1)
		f.contactListRepo.EXPECT().AddContactToList(gomock.Any(), "ws-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, cl *domain.ContactList) error {
				assert.Equal(t, "person@example.com", cl.Email)
				assert.Equal(t, "Newsletter", cl.ListName)
				assert.Equal(t, domain.ContactListStatusActive, cl.Status)
				return nil
			}).Times(1)
		f.listRepo.EXPECT().GetListByID(gomock.Any(), "ws-1", "list-gone").
			Return(nil, errors.New("not found")).Times(1)
		assert.NoError(t, f.svc.ProcessUserCreatedHook(ctx, "ws-1", "int-1", payload, id, ts, sig))
	})
}
