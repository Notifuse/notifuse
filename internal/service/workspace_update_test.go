package service

import (
	"context"

	"errors"
	notifuse_mjml "github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// UpdateWorkspace is owner-only and merges through an allowlist: fields with
// their own endpoints (blog, web analytics) must survive a general settings
// save untouched, and a transactional-only provider must never be newly
// assigned as the marketing provider — while a pre-existing assignment is
// grandfathered so unrelated saves keep working.

func validTemplateBlock() notifuse_mjml.EmailBlock {
	blk, err := notifuse_mjml.UnmarshalEmailBlock(
		[]byte(`{"id":"b1","type":"mj-text","content":"Hello"}`))
	if err != nil {
		panic(err)
	}
	return blk
}

func existingWorkspaceForUpdate() *domain.Workspace {
	return &domain.Workspace{
		ID:   "ws1",
		Name: "Old Name",
		Settings: domain.WorkspaceSettings{
			WebsiteURL: "https://old.example.com",
			SecretKey:  "workspace-secret",
			TemplateBlocks: []domain.TemplateBlock{
				{ID: "block-1", Name: "Header", Block: validTemplateBlock()},
			},
		},
		Integrations: []domain.Integration{
			{
				ID:   "int-mailjet",
				Name: "Mailjet",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:               domain.EmailProviderKindMailjet,
					RateLimitPerMinute: 25,
					Senders: []domain.EmailSender{
						{ID: "sender-1", Email: "demo@notifuse.com", Name: "Demo", IsDefault: true},
					},
					Mailjet: &domain.MailjetSettings{APIKey: "mj-key", SecretKey: "mj-secret"},
				},
			},
		},
	}
}

func TestWorkspaceService_UpdateWorkspace_Guards(t *testing.T) {
	ctx := context.Background()

	t.Run("auth failure is wrapped", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		sentinel := errors.New("token expired")
		deps.auth.EXPECT().AuthenticateUserForWorkspace(ctx, "ws1").
			Return(nil, nil, nil, sentinel).Times(1)
		_, err := svc.UpdateWorkspace(ctx, "ws1", "New", domain.WorkspaceSettings{})
		require.ErrorContains(t, err, "failed to authenticate user:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("members cannot update, owners can", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		deps.expectMember(ctx, "ws1", "user-1")
		_, err := svc.UpdateWorkspace(ctx, "ws1", "New", domain.WorkspaceSettings{})
		require.Error(t, err)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.Equal(t, "user is not an owner of the workspace", unauth.Message)
	})

	t.Run("a missing workspace surfaces the repo error unwrapped", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		deps.expectOwner(ctx, "ws1", "user-1")
		sentinel := errors.New("workspace gone")
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(nil, sentinel).Times(1)
		_, err := svc.UpdateWorkspace(ctx, "ws1", "New", domain.WorkspaceSettings{})
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestWorkspaceService_UpdateWorkspace_MarketingProviderGuard(t *testing.T) {
	ctx := context.Background()

	t.Run("newly assigning a transactional-only provider as marketing is refused", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		deps.expectOwner(ctx, "ws1", "user-1")
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").
			Return(existingWorkspaceForUpdate(), nil).Times(1)
		// No repo.Update expectation: the refusal happens before persisting.

		settings := domain.WorkspaceSettings{MarketingEmailProviderID: "int-mailjet"}
		_, err := svc.UpdateWorkspace(ctx, "ws1", "Old Name", settings)
		require.EqualError(t, err,
			"mailjet can only be used as a transactional email provider, not a marketing provider")
	})

	t.Run("the same assignment already stored is grandfathered", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		deps.expectOwner(ctx, "ws1", "user-1")
		existing := existingWorkspaceForUpdate()
		existing.Settings.MarketingEmailProviderID = "int-mailjet"
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(existing, nil).Times(1)
		deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil).Times(1)

		settings := domain.WorkspaceSettings{MarketingEmailProviderID: "int-mailjet", Timezone: "UTC", DefaultLanguage: "en", Languages: []string{"en"}}
		_, err := svc.UpdateWorkspace(ctx, "ws1", "Old Name", settings)
		require.NoError(t, err, "an unchanged assignment must not block the save")
	})
}

func TestWorkspaceService_UpdateWorkspace_Merge(t *testing.T) {
	ctx := context.Background()

	t.Run("allowlisted fields merge, the rest of the workspace survives", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		deps.expectOwner(ctx, "ws1", "user-1")
		existing := existingWorkspaceForUpdate()
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(existing, nil).Times(1)
		deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
				assert.Equal(t, "New Name", ws.Name)
				assert.Equal(t, "https://new.example.com", ws.Settings.WebsiteURL)
				assert.Equal(t, "workspace-secret", ws.Settings.SecretKey,
					"the workspace secret is not part of the update allowlist")
				require.Len(t, ws.Integrations, 1, "integrations are preserved, not replaced")
				assert.Equal(t, "int-mailjet", ws.Integrations[0].ID)
				require.Len(t, ws.Settings.TemplateBlocks, 1,
					"nil template blocks in the request preserve the stored ones")
				assert.Equal(t, "block-1", ws.Settings.TemplateBlocks[0].ID)
				return nil
			}).Times(1)

		got, err := svc.UpdateWorkspace(ctx, "ws1", "New Name", domain.WorkspaceSettings{
			WebsiteURL:      "https://new.example.com",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
		})
		require.NoError(t, err)
		assert.Equal(t, "New Name", got.Name)
	})

	t.Run("new template blocks get ids and timestamps", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		deps.expectOwner(ctx, "ws1", "user-1")
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").
			Return(existingWorkspaceForUpdate(), nil).Times(1)
		deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
				require.Len(t, ws.Settings.TemplateBlocks, 1)
				assert.NotEmpty(t, ws.Settings.TemplateBlocks[0].ID)
				assert.False(t, ws.Settings.TemplateBlocks[0].Created.IsZero())
				assert.False(t, ws.Settings.TemplateBlocks[0].Updated.IsZero())
				return nil
			}).Times(1)

		_, err := svc.UpdateWorkspace(ctx, "ws1", "Old Name", domain.WorkspaceSettings{
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Languages:       []string{"en"},
			TemplateBlocks:  []domain.TemplateBlock{{Name: "Promo", Block: validTemplateBlock()}},
		})
		require.NoError(t, err)
	})

	t.Run("a persist failure surfaces the repo error", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		deps.expectOwner(ctx, "ws1", "user-1")
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").
			Return(existingWorkspaceForUpdate(), nil).Times(1)
		sentinel := errors.New("write failed")
		deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(sentinel).Times(1)

		_, err := svc.UpdateWorkspace(ctx, "ws1", "Old Name", domain.WorkspaceSettings{Timezone: "UTC", DefaultLanguage: "en", Languages: []string{"en"}})
		assert.ErrorIs(t, err, sentinel)
	})
}
