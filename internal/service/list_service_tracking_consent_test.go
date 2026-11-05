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

type listConsentMocks struct {
	repo          *mocks.MockListRepository
	auth          *mocks.MockAuthService
	workspaceRepo *mocks.MockWorkspaceRepository
	contactList   *mocks.MockContactListRepository
	contactRepo   *mocks.MockContactRepository
	email         *mocks.MockEmailServiceInterface
}

func newListConsentService(ctrl *gomock.Controller) (*ListService, listConsentMocks) {
	m := listConsentMocks{
		repo:          mocks.NewMockListRepository(ctrl),
		auth:          mocks.NewMockAuthService(ctrl),
		workspaceRepo: mocks.NewMockWorkspaceRepository(ctrl),
		contactList:   mocks.NewMockContactListRepository(ctrl),
		contactRepo:   mocks.NewMockContactRepository(ctrl),
		email:         mocks.NewMockEmailServiceInterface(ctrl),
	}
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	svc := NewListService(m.repo, m.workspaceRepo, m.contactList, m.contactRepo, mocks.NewMockMessageHistoryRepository(ctrl), m.auth, m.email, logger, "https://api.example.com", pkgmocks.NewMockCache(ctrl))
	return svc, m
}

func TestListService_SubscribeToLists_TrackingConsentSource(t *testing.T) {
	const workspaceID = "workspace123"
	const email = "new@example.com"
	backdated := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name        string
		bearer      bool
		userType    domain.UserType
		source      *domain.NullableString
		wantSource  string
		wantDateNil bool
	}{
		{name: "public form imposes subscribe_form and drops the date", source: &domain.NullableString{String: domain.TrackingConsentSourceConsole}, wantSource: domain.TrackingConsentSourceSubscribeForm, wantDateNil: true},
		{name: "API key defaults to api", bearer: true, userType: domain.UserTypeAPIKey, wantSource: domain.TrackingConsentSourceAPI},
		{name: "console user defaults to console", bearer: true, userType: domain.UserTypeUser, wantSource: domain.TrackingConsentSourceConsole},
		{name: "authenticated caller keeps its own source and date", bearer: true, userType: domain.UserTypeAPIKey, source: &domain.NullableString{String: "legacy_crm"}, wantSource: "legacy_crm"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			svc, m := newListConsentService(ctrl)

			m.workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{ID: workspaceID, Settings: domain.WorkspaceSettings{SecretKey: "k"}}, nil)
			if tc.bearer {
				m.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(context.Background(), &domain.User{ID: "u", Type: tc.userType}, &domain.UserWorkspace{
					UserID: "u", WorkspaceID: workspaceID, Role: "member",
					Permissions: domain.UserPermissions{domain.PermissionResourceLists: {Read: true, Write: true}},
				}, nil)
			} else {
				m.contactRepo.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, email).Return(nil, nil)
			}

			var upserted *domain.Contact
			m.contactRepo.EXPECT().UpsertContact(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, c *domain.Contact) (bool, error) {
				copied := *c
				upserted = &copied
				return true, nil
			})
			m.repo.EXPECT().GetLists(gomock.Any(), workspaceID).Return([]*domain.List{{ID: "list1", Name: "News", IsPublic: true}}, nil)
			m.contactList.EXPECT().GetContactListIncludingRemoved(gomock.Any(), workspaceID, email, "list1").Return(nil, &domain.ErrContactListNotFound{Message: "not found"})
			m.contactList.EXPECT().AddContactToList(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

			err := svc.SubscribeToLists(context.Background(), &domain.SubscribeToListsRequest{
				WorkspaceID: workspaceID,
				Contact: domain.Contact{
					Email:                    email,
					TrackingClicksConsent:    &domain.NullableString{String: domain.TrackingConsentDenied},
					TrackingConsentSource:    tc.source,
					TrackingConsentUpdatedAt: &domain.NullableTime{Time: backdated},
				},
				ListIDs: []string{"list1"},
			}, tc.bearer)
			require.NoError(t, err)

			require.NotNil(t, upserted)
			require.NotNil(t, upserted.TrackingClicksConsent)
			assert.Equal(t, domain.TrackingConsentDenied, upserted.TrackingClicksConsent.String)
			require.NotNil(t, upserted.TrackingConsentSource)
			assert.Equal(t, tc.wantSource, upserted.TrackingConsentSource.String)
			if tc.wantDateNil {
				assert.Nil(t, upserted.TrackingConsentUpdatedAt)
			} else {
				require.NotNil(t, upserted.TrackingConsentUpdatedAt)
				assert.True(t, upserted.TrackingConsentUpdatedAt.Time.Equal(backdated))
			}
		})
	}
}

func TestListService_SubscribeToLists_DoubleOptInFollowsTrackingConsent(t *testing.T) {
	const workspaceID = "workspace123"
	const email = "new@example.com"

	cases := []struct {
		name           string
		consent        *domain.NullableString
		wantClicks     bool
		wantUTMContent bool
	}{
		{name: "clicks denied: no redirect, no per-recipient utm_content", consent: &domain.NullableString{String: domain.TrackingConsentDenied}},
		{name: "unknown under track: tracked, message ID in utm_content", wantClicks: true, wantUTMContent: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			svc, m := newListConsentService(ctrl)

			m.workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
				ID: workspaceID,
				Settings: domain.WorkspaceSettings{
					SecretKey:                "k",
					MarketingEmailProviderID: "mkt",
					EmailTrackingEnabled:     true,
					EmailTracking:            domain.DefaultEmailTrackingSettings(),
				},
				Integrations: domain.Integrations{{
					ID:   "mkt",
					Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind:      domain.EmailProviderKindSparkPost,
						Senders:   []domain.EmailSender{domain.NewEmailSender("from@example.com", "From")},
						SparkPost: &domain.SparkPostSettings{APIKey: "k"},
					},
				}},
			}, nil)
			gomock.InOrder(
				m.contactRepo.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, email).Return(nil, nil),
				m.contactRepo.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, email).Return(&domain.Contact{Email: email, TrackingClicksConsent: tc.consent}, nil),
			)
			m.contactRepo.EXPECT().UpsertContact(gomock.Any(), workspaceID, gomock.Any()).Return(true, nil)
			m.repo.EXPECT().GetLists(gomock.Any(), workspaceID).Return([]*domain.List{{
				ID: "list1", Name: "News", IsPublic: true, IsDoubleOptin: true,
				DoubleOptInTemplate: &domain.TemplateReference{ID: "doi", Version: 1},
			}}, nil)
			m.contactList.EXPECT().GetContactListIncludingRemoved(gomock.Any(), workspaceID, email, "list1").Return(nil, &domain.ErrContactListNotFound{Message: "not found"})
			m.contactList.EXPECT().AddContactToList(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

			var sent domain.SendEmailRequest
			m.email.EXPECT().SendEmailForTemplate(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req domain.SendEmailRequest) error {
				sent = req
				return nil
			})

			require.NoError(t, svc.SubscribeToLists(context.Background(), &domain.SubscribeToListsRequest{
				WorkspaceID: workspaceID,
				Contact:     domain.Contact{Email: email},
				ListIDs:     []string{"list1"},
			}, false))

			assert.Equal(t, !tc.wantClicks, sent.TrackingSettings.SuppressClicks)
			assert.Equal(t, !tc.wantClicks, sent.TrackingSettings.SuppressIdentify)
			assert.False(t, sent.TrackingSettings.SuppressOpens)
			if tc.wantUTMContent {
				assert.Equal(t, sent.MessageID, sent.TrackingSettings.UTMContent)
			} else {
				assert.Empty(t, sent.TrackingSettings.UTMContent)
			}
		})
	}
}
