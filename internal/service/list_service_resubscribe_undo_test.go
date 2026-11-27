package service

import (
	"context"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type atomicUndoRepoStub struct {
	domain.ContactListRepository
	unsubscribeActivePendingFn func(ctx context.Context, workspaceID, email, listID string) (bool, error)
	restoreFn                  func(ctx context.Context, workspaceID, email, listID string) (bool, error)
}

func (s *atomicUndoRepoStub) UnsubscribeActiveOrPendingContactList(ctx context.Context, workspaceID, email, listID string) (bool, error) {
	return s.unsubscribeActivePendingFn(ctx, workspaceID, email, listID)
}

func (s *atomicUndoRepoStub) RestoreUnsubscribedContactList(ctx context.Context, workspaceID, email, listID string) (bool, error) {
	return s.restoreFn(ctx, workspaceID, email, listID)
}

func TestListService_UnsubscribeFromListsWithResults_UndoTokens(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws123"
	secretKey := "test-secret-key"
	email := "alice@example.com"
	hmacSig := domain.ComputeEmailHMAC(email, secretKey)
	privateList := &domain.List{ID: "vip", Name: "VIP Members", IsPublic: false}
	workspace := &domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{SecretKey: secretKey},
	}

	newFixture := func(t *testing.T, atomicTransition bool) (*ListService, *mocks.MockContactListRepository, *mocks.MockContactRepository, *mocks.MockMessageHistoryRepository) {
		ctrl := gomock.NewController(t)
		listRepo := mocks.NewMockListRepository(ctrl)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		baseCLRepo := mocks.NewMockContactListRepository(ctrl)
		atomicRepo := &atomicUndoRepoStub{
			ContactListRepository: baseCLRepo,
			unsubscribeActivePendingFn: func(_ context.Context, wsID, em, lID string) (bool, error) {
				assert.Equal(t, workspaceID, wsID)
				assert.Equal(t, email, em)
				assert.Equal(t, "vip", lID)
				return atomicTransition, nil
			},
		}
		contactRepo := mocks.NewMockContactRepository(ctrl)
		messageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		logger := pkgmocks.NewMockLogger(ctrl)

		svc := NewListService(listRepo, workspaceRepo, atomicRepo, contactRepo, messageHistoryRepo,
			mocks.NewMockAuthService(ctrl), mocks.NewMockEmailServiceInterface(ctrl), logger, "https://api.example.com", pkgmocks.NewMockCache(ctrl))

		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		listRepo.EXPECT().GetLists(gomock.Any(), workspaceID).Return([]*domain.List{privateList}, nil)
		return svc, baseCLRepo, contactRepo, messageHistoryRepo
	}

	t.Run("mints undo token and returns list name only when atomic UPDATE transitions active/pending row", func(t *testing.T) {
		svc, _, _, messageHistoryRepo := newFixture(t, true)
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

		results, err := svc.UnsubscribeFromListsWithResults(ctx, &domain.UnsubscribeFromListsRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EmailHMAC:   hmacSig,
			ListIDs:     []string{"vip"},
			MessageID:   "msg-1",
		}, false)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "vip", results[0].ID)
		assert.Equal(t, "VIP Members", results[0].Name)
		require.NotEmpty(t, results[0].ResubscribeToken)
		assert.NoError(t, domain.VerifyResubscribeToken(secretKey, workspaceID, email, "vip", results[0].ResubscribeToken))
	})

	t.Run("omits private list name and undo token when row was already unsubscribed or removed", func(t *testing.T) {
		svc, baseCLRepo, _, _ := newFixture(t, false)
		baseCLRepo.EXPECT().MarkContactListUnsubscribed(gomock.Any(), workspaceID, email, "vip", time.Time{}).Return(true, nil)

		results, err := svc.UnsubscribeFromListsWithResults(ctx, &domain.UnsubscribeFromListsRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EmailHMAC:   hmacSig,
			ListIDs:     []string{"vip"},
		}, false)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("omits undo token for non-member targeted by automation", func(t *testing.T) {
		svc, baseCLRepo, contactRepo, messageHistoryRepo := newFixture(t, false)
		noRow := &domain.ErrContactListNotFound{Message: "not found"}
		baseCLRepo.EXPECT().MarkContactListUnsubscribed(gomock.Any(), workspaceID, email, "vip", time.Time{}).Return(false, nil)
		baseCLRepo.EXPECT().GetContactListIncludingRemoved(gomock.Any(), workspaceID, email, "vip").Return(nil, noRow)
		contactRepo.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, email).Return(&domain.Contact{Email: email}, nil)
		messageHistoryRepo.EXPECT().WasSentForList(gomock.Any(), workspaceID, "msg-1", email, "vip").Return(true, nil)
		baseCLRepo.EXPECT().AddContactToList(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
		messageHistoryRepo.EXPECT().SetStatusesIfNotSet(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

		results, err := svc.UnsubscribeFromListsWithResults(ctx, &domain.UnsubscribeFromListsRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EmailHMAC:   hmacSig,
			ListIDs:     []string{"vip"},
			MessageID:   "msg-1",
		}, false)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	for _, status := range []domain.ContactListStatus{domain.ContactListStatusBounced, domain.ContactListStatusComplained} {
		t.Run("omits undo token when row is "+string(status), func(t *testing.T) {
			svc, baseCLRepo, _, _ := newFixture(t, false)
			baseCLRepo.EXPECT().MarkContactListUnsubscribed(gomock.Any(), workspaceID, email, "vip", time.Time{}).Return(false, nil)
			baseCLRepo.EXPECT().GetContactListIncludingRemoved(gomock.Any(), workspaceID, email, "vip").
				Return(&domain.ContactList{Email: email, ListID: "vip", Status: status}, nil)

			results, err := svc.UnsubscribeFromListsWithResults(ctx, &domain.UnsubscribeFromListsRequest{
				WorkspaceID: workspaceID,
				Email:       email,
				EmailHMAC:   hmacSig,
				ListIDs:     []string{"vip"},
			}, false)
			require.NoError(t, err)
			assert.Empty(t, results)
		})
	}
}

func TestListService_ResubscribeWithToken(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws123"
	secretKey := "test-secret-key"
	email := "alice@example.com"
	hmacSig := domain.ComputeEmailHMAC(email, secretKey)
	privateList := &domain.List{ID: "vip", Name: "VIP Members", IsPublic: false}
	workspace := &domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{SecretKey: secretKey},
	}

	validToken, err := domain.MintResubscribeToken(secretKey, workspaceID, email, "vip")
	require.NoError(t, err)

	expiredToken, err := domain.MintResubscribeTokenAt(secretKey, workspaceID, email, "vip", time.Now().UTC().Add(-2*time.Hour))
	require.NoError(t, err)

	setup := func(t *testing.T, restoreResult bool) (*ListService, *mocks.MockListRepository, *mocks.MockContactListRepository, *gomock.Controller) {
		ctrl := gomock.NewController(t)
		listRepo := mocks.NewMockListRepository(ctrl)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		baseCLRepo := mocks.NewMockContactListRepository(ctrl)
		atomicRepo := &atomicUndoRepoStub{
			ContactListRepository: baseCLRepo,
			restoreFn: func(_ context.Context, wsID, em, lID string) (bool, error) {
				assert.Equal(t, workspaceID, wsID)
				assert.Equal(t, email, em)
				assert.Equal(t, "vip", lID)
				return restoreResult, nil
			},
		}
		svc := NewListService(listRepo, workspaceRepo, atomicRepo, mocks.NewMockContactRepository(ctrl),
			mocks.NewMockMessageHistoryRepository(ctrl), mocks.NewMockAuthService(ctrl),
			mocks.NewMockEmailServiceInterface(ctrl), pkgmocks.NewMockLogger(ctrl), "https://api.example.com", pkgmocks.NewMockCache(ctrl))
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		return svc, listRepo, baseCLRepo, ctrl
	}

	t.Run("restores private list membership atomically with valid token and records engagement", func(t *testing.T) {
		svc, listRepo, baseCLRepo, ctrl := setup(t, true)
		engagementRepo := mocks.NewMockContactEngagementRepository(ctrl)
		svc.SetEngagementRecorder(NewEngagementRecorder(engagementRepo, pkgmocks.NewMockLogger(ctrl)))

		listRepo.EXPECT().GetListByID(gomock.Any(), workspaceID, "vip").Return(privateList, nil)
		baseCLRepo.EXPECT().GetContactListIncludingRemoved(gomock.Any(), workspaceID, email, "vip").
			Return(&domain.ContactList{Email: email, ListID: "vip", Status: domain.ContactListStatusUnsubscribed}, nil)
		engagementRepo.EXPECT().Touch(gomock.Any(), workspaceID, email, gomock.Any()).Return(nil)

		cl, err := svc.ResubscribeWithToken(ctx, &domain.ResubscribeRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EmailHMAC:   hmacSig,
			ListID:      "vip",
			Token:       validToken,
		})
		require.NoError(t, err)
		assert.Equal(t, "vip", cl.ListID)
		assert.Equal(t, "VIP Members", cl.ListName)
		assert.Equal(t, domain.ContactListStatusActive, cl.Status)
	})

	t.Run("fails closed when repository does not support atomic undo restoration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		listRepo := mocks.NewMockListRepository(ctrl)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		plainCLRepo := mocks.NewMockContactListRepository(ctrl)
		svc := NewListService(listRepo, workspaceRepo, plainCLRepo, mocks.NewMockContactRepository(ctrl),
			mocks.NewMockMessageHistoryRepository(ctrl), mocks.NewMockAuthService(ctrl),
			mocks.NewMockEmailServiceInterface(ctrl), pkgmocks.NewMockLogger(ctrl), "https://api.example.com", pkgmocks.NewMockCache(ctrl))

		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		listRepo.EXPECT().GetListByID(gomock.Any(), workspaceID, "vip").Return(privateList, nil)
		plainCLRepo.EXPECT().GetContactListIncludingRemoved(gomock.Any(), workspaceID, email, "vip").
			Return(&domain.ContactList{Email: email, ListID: "vip", Status: domain.ContactListStatusUnsubscribed}, nil)
		// Crucially, ResubscribeContactToList must NOT be called as an unsafe fallback.

		_, err := svc.ResubscribeWithToken(ctx, &domain.ResubscribeRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EmailHMAC:   hmacSig,
			ListID:      "vip",
			Token:       validToken,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "atomic")
	})

	t.Run("already active is idempotent and does not touch engagement", func(t *testing.T) {
		svc, listRepo, baseCLRepo, ctrl := setup(t, true)
		engagementRepo := mocks.NewMockContactEngagementRepository(ctrl)
		svc.SetEngagementRecorder(NewEngagementRecorder(engagementRepo, pkgmocks.NewMockLogger(ctrl)))

		listRepo.EXPECT().GetListByID(gomock.Any(), workspaceID, "vip").Return(privateList, nil)
		baseCLRepo.EXPECT().GetContactListIncludingRemoved(gomock.Any(), workspaceID, email, "vip").
			Return(&domain.ContactList{Email: email, ListID: "vip", Status: domain.ContactListStatusActive}, nil)

		cl, err := svc.ResubscribeWithToken(ctx, &domain.ResubscribeRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EmailHMAC:   hmacSig,
			ListID:      "vip",
			Token:       validToken,
		})
		require.NoError(t, err)
		assert.Equal(t, domain.ContactListStatusActive, cl.Status)
	})

	t.Run("refuses missing row or pending row", func(t *testing.T) {
		svc, listRepo, baseCLRepo, _ := setup(t, true)
		listRepo.EXPECT().GetListByID(gomock.Any(), workspaceID, "vip").Return(privateList, nil)
		baseCLRepo.EXPECT().GetContactListIncludingRemoved(gomock.Any(), workspaceID, email, "vip").
			Return(nil, &domain.ErrContactListNotFound{Message: "not found"})

		_, err := svc.ResubscribeWithToken(ctx, &domain.ResubscribeRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EmailHMAC:   hmacSig,
			ListID:      "vip",
			Token:       validToken,
		})
		assert.ErrorIs(t, err, domain.ErrResubscribeNotAllowed)

		svc2, listRepo2, baseCLRepo2, _ := setup(t, true)
		listRepo2.EXPECT().GetListByID(gomock.Any(), workspaceID, "vip").Return(privateList, nil)
		baseCLRepo2.EXPECT().GetContactListIncludingRemoved(gomock.Any(), workspaceID, email, "vip").
			Return(&domain.ContactList{Email: email, ListID: "vip", Status: domain.ContactListStatusPending}, nil)

		_, err = svc2.ResubscribeWithToken(ctx, &domain.ResubscribeRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EmailHMAC:   hmacSig,
			ListID:      "vip",
			Token:       validToken,
		})
		assert.ErrorIs(t, err, domain.ErrResubscribeNotAllowed)
	})

	t.Run("refuses when atomic restorer reports 0 rows (concurrent removal or bounce)", func(t *testing.T) {
		svc, listRepo, baseCLRepo, _ := setup(t, false)
		listRepo.EXPECT().GetListByID(gomock.Any(), workspaceID, "vip").Return(privateList, nil)
		baseCLRepo.EXPECT().GetContactListIncludingRemoved(gomock.Any(), workspaceID, email, "vip").
			Return(&domain.ContactList{Email: email, ListID: "vip", Status: domain.ContactListStatusUnsubscribed}, nil)

		_, err := svc.ResubscribeWithToken(ctx, &domain.ResubscribeRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EmailHMAC:   hmacSig,
			ListID:      "vip",
			Token:       validToken,
		})
		assert.ErrorIs(t, err, domain.ErrResubscribeNotAllowed)
	})

	t.Run("rejects invalid email_hmac", func(t *testing.T) {
		svc, _, _, _ := setup(t, true)
		_, err := svc.ResubscribeWithToken(ctx, &domain.ResubscribeRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EmailHMAC:   "wrong-hmac",
			ListID:      "vip",
			Token:       validToken,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid email verification")
	})

	t.Run("rejects expired token", func(t *testing.T) {
		svc, _, _, _ := setup(t, true)
		_, err := svc.ResubscribeWithToken(ctx, &domain.ResubscribeRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EmailHMAC:   hmacSig,
			ListID:      "vip",
			Token:       expiredToken,
		})
		assert.ErrorIs(t, err, domain.ErrResubscribeTokenExpired)
	})

	t.Run("rejects token used for a different list", func(t *testing.T) {
		svc, _, _, _ := setup(t, true)
		_, err := svc.ResubscribeWithToken(ctx, &domain.ResubscribeRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EmailHMAC:   hmacSig,
			ListID:      "other-list",
			Token:       validToken,
		})
		assert.ErrorIs(t, err, domain.ErrResubscribeTokenInvalid)
	})

	t.Run("refuses when row was removed by workspace", func(t *testing.T) {
		svc, listRepo, baseCLRepo, _ := setup(t, true)
		removedAt := time.Now().UTC()
		listRepo.EXPECT().GetListByID(gomock.Any(), workspaceID, "vip").Return(privateList, nil)
		baseCLRepo.EXPECT().GetContactListIncludingRemoved(gomock.Any(), workspaceID, email, "vip").
			Return(&domain.ContactList{Email: email, ListID: "vip", Status: domain.ContactListStatusUnsubscribed, DeletedAt: &removedAt}, nil)

		_, err := svc.ResubscribeWithToken(ctx, &domain.ResubscribeRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EmailHMAC:   hmacSig,
			ListID:      "vip",
			Token:       validToken,
		})
		assert.ErrorIs(t, err, domain.ErrResubscribeNotAllowed)
	})

	for _, status := range []domain.ContactListStatus{domain.ContactListStatusBounced, domain.ContactListStatusComplained} {
		t.Run("refuses "+string(status)+" row", func(t *testing.T) {
			svc, listRepo, baseCLRepo, _ := setup(t, true)
			listRepo.EXPECT().GetListByID(gomock.Any(), workspaceID, "vip").Return(privateList, nil)
			baseCLRepo.EXPECT().GetContactListIncludingRemoved(gomock.Any(), workspaceID, email, "vip").
				Return(&domain.ContactList{Email: email, ListID: "vip", Status: status}, nil)

			_, err := svc.ResubscribeWithToken(ctx, &domain.ResubscribeRequest{
				WorkspaceID: workspaceID,
				Email:       email,
				EmailHMAC:   hmacSig,
				ListID:      "vip",
				Token:       validToken,
			})
			assert.ErrorIs(t, err, domain.ErrResubscribeNotAllowed)
		})
	}
}
