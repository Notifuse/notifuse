package service

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/crypto"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationCenterService_GetContactPreferences_TrackingState(t *testing.T) {
	const secretKey = "test-secret-key"
	const email = "user@example.com"
	hmac := crypto.ComputeHMAC256([]byte(email), secretKey)

	cases := []struct {
		name     string
		tracking *domain.EmailTrackingSettings
		contact  *domain.Contact
		want     domain.ContactTrackingState
	}{
		{
			name:     "unknown contact under track is tracked",
			tracking: domain.DefaultEmailTrackingSettings(),
			contact:  &domain.Contact{Email: email},
			want:     domain.ContactTrackingState{OpensEnabled: true, ClicksEnabled: true, OpensTracked: true, ClicksTracked: true},
		},
		{
			name:     "unknown contact under no_track is not tracked",
			tracking: &domain.EmailTrackingSettings{OpensEnabled: true, ClicksEnabled: true, UnknownConsent: domain.UnknownConsentNoTrack},
			contact:  &domain.Contact{Email: email},
			want:     domain.ContactTrackingState{OpensEnabled: true, ClicksEnabled: true},
		},
		{
			name:     "workspace tracks clicks only, contact denied clicks",
			tracking: &domain.EmailTrackingSettings{ClicksEnabled: true, UnknownConsent: domain.UnknownConsentTrack},
			contact:  &domain.Contact{Email: email, TrackingClicksConsent: &domain.NullableString{String: domain.TrackingConsentDenied}},
			want:     domain.ContactTrackingState{ClicksEnabled: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			contactRepo := mocks.NewMockContactRepository(ctrl)
			workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			listRepo := mocks.NewMockListRepository(ctrl)
			logger := pkgmocks.NewMockLogger(ctrl)

			workspaceRepo.EXPECT().GetByID(gomock.Any(), "ws").Return(&domain.Workspace{ID: "ws", Settings: domain.WorkspaceSettings{
				SecretKey:            secretKey,
				EmailTrackingEnabled: tc.tracking.OpensEnabled || tc.tracking.ClicksEnabled,
				EmailTracking:        tc.tracking,
			}}, nil)
			contactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws", email).Return(tc.contact, nil)
			listRepo.EXPECT().GetLists(gomock.Any(), "ws").Return(nil, nil)

			svc := NewNotificationCenterService(contactRepo, workspaceRepo, listRepo, logger)
			resp, err := svc.GetContactPreferences(context.Background(), "ws", email, hmac)
			require.NoError(t, err)
			require.NotNil(t, resp.Tracking)
			assert.Equal(t, tc.want, *resp.Tracking)
		})
	}
}
