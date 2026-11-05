package service

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContactService_UpsertContact_TrackingConsentSource(t *testing.T) {
	const workspaceID = "workspace123"
	userWorkspace := &domain.UserWorkspace{
		UserID: "user123", WorkspaceID: workspaceID, Role: "member",
		Permissions: domain.UserPermissions{domain.PermissionResourceContacts: {Read: true, Write: true}},
	}

	cases := []struct {
		name       string
		user       *domain.User
		source     *domain.NullableString
		consent    *domain.NullableString
		systemCall bool
		wantSource *string
	}{
		{name: "API key", user: &domain.User{Type: domain.UserTypeAPIKey}, consent: &domain.NullableString{String: domain.TrackingConsentGranted}, wantSource: strPtr(domain.TrackingConsentSourceAPI)},
		{name: "console user", user: &domain.User{Type: domain.UserTypeUser}, consent: &domain.NullableString{String: domain.TrackingConsentDenied}, wantSource: strPtr(domain.TrackingConsentSourceConsole)},
		{name: "explicit null is a change too", user: &domain.User{Type: domain.UserTypeUser}, consent: &domain.NullableString{IsNull: true}, wantSource: strPtr(domain.TrackingConsentSourceConsole)},
		{name: "caller's own source kept", user: &domain.User{Type: domain.UserTypeAPIKey}, consent: &domain.NullableString{String: domain.TrackingConsentGranted}, source: &domain.NullableString{String: "legacy_crm"}, wantSource: strPtr("legacy_crm")},
		{name: "no consent, no source", user: &domain.User{Type: domain.UserTypeAPIKey}},
		{name: "system call attributes nothing", systemCall: true, consent: &domain.NullableString{String: domain.TrackingConsentGranted}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			service, mockRepo, _, mockAuthService, _, _, _, _, _, _, _, _, _ := createContactServiceWithMocks(ctrl)

			ctx := context.Background()
			if tc.systemCall {
				ctx = context.WithValue(ctx, domain.SystemCallKey, true)
			} else {
				mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, tc.user, userWorkspace, nil)
			}

			contact := &domain.Contact{Email: "a@example.com", TrackingOpensConsent: tc.consent, TrackingConsentSource: tc.source}
			var written *domain.Contact
			mockRepo.EXPECT().UpsertContact(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, c *domain.Contact) (bool, error) {
				written = c
				return true, nil
			})
			mockRepo.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, contact.Email).Return(contact, nil)

			result := service.UpsertContact(ctx, workspaceID, contact)
			require.Empty(t, result.Error)

			require.NotNil(t, written)
			if tc.wantSource == nil {
				assert.Nil(t, written.TrackingConsentSource)
			} else {
				require.NotNil(t, written.TrackingConsentSource)
				assert.Equal(t, *tc.wantSource, written.TrackingConsentSource.String)
			}
		})
	}
}

func TestContactService_BatchImportContacts_TrackingConsentSource(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	service, mockRepo, _, mockAuthService, _, _, _, _, _, _, _, _, _ := createContactServiceWithMocks(ctrl)

	const workspaceID = "workspace123"
	ctx := context.Background()
	userWorkspace := &domain.UserWorkspace{
		UserID: "user123", WorkspaceID: workspaceID, Role: "member",
		Permissions: domain.UserPermissions{domain.PermissionResourceContacts: {Read: true, Write: true}},
	}
	mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{Type: domain.UserTypeAPIKey}, userWorkspace, nil)

	granted := &domain.NullableString{String: domain.TrackingConsentGranted}
	contacts := []*domain.Contact{
		{Email: "a@example.com", TrackingClicksConsent: granted},
		{Email: "b@example.com", TrackingClicksConsent: granted, TrackingConsentSource: &domain.NullableString{String: "legacy_crm"}},
		{Email: "c@example.com"},
	}
	var written []*domain.Contact
	mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, cs []*domain.Contact) ([]domain.BulkUpsertResult, error) {
		written = cs
		results := make([]domain.BulkUpsertResult, 0, len(cs))
		for _, c := range cs {
			results = append(results, domain.BulkUpsertResult{Email: c.Email, IsNew: true})
		}
		return results, nil
	})

	response := service.BatchImportContacts(ctx, workspaceID, contacts, nil)
	require.Empty(t, response.Error)

	byEmail := map[string]*domain.Contact{}
	for _, c := range written {
		byEmail[c.Email] = c
	}
	require.Len(t, byEmail, len(contacts))
	assert.Equal(t, domain.TrackingConsentSourceImport, byEmail["a@example.com"].TrackingConsentSource.String)
	assert.Equal(t, "legacy_crm", byEmail["b@example.com"].TrackingConsentSource.String)
	assert.Nil(t, byEmail["c@example.com"].TrackingConsentSource)
}
