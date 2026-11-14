package service

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	svcmocks "github.com/Notifuse/notifuse/internal/service/mocks"
)

// A custom domain belongs to one workspace: one that saved another workspace's domain
// could show its own emails, web versions included, under that workspace's brand.
func TestWorkspaceService_UpdateWorkspace_CustomDomainIsNotShared(t *testing.T) {
	ctx := context.Background()
	settingsWith := func(endpoint string) domain.WorkspaceSettings {
		return domain.WorkspaceSettings{Timezone: "UTC", DefaultLanguage: "en", Languages: []string{"en"}, CustomEndpointURL: &endpoint}
	}

	t.Run("refuses a domain another workspace uses, however it is written", func(t *testing.T) {
		for _, endpoint := range []string{
			"https://mail.brand-b.example",
			"https://MAIL.Brand-B.example:443/",
			"https://mail.brand-b.example.",
		} {
			svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
			deps.expectOwner(ctx, "ws1", "user-1")
			deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(existingWorkspaceForUpdate(), nil)
			deps.repo.EXPECT().ListCustomEndpoints(gomock.Any()).Return(map[string]string{
				"ws1":     "https://old.brand-a.example",
				"brand_b": "https://mail.brand-b.example",
			}, nil)
			deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Times(0)

			_, err := svc.UpdateWorkspace(ctx, "ws1", "Old Name", settingsWith(endpoint))
			var validation domain.ValidationError
			require.ErrorAs(t, err, &validation, endpoint)
			assert.Contains(t, validation.Message, "another workspace")
		}
	})

	t.Run("checks a move from one domain to another", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		deps.expectOwner(ctx, "ws1", "user-1")
		existing := existingWorkspaceForUpdate()
		previous := "https://mail.brand-a.example"
		existing.Settings.CustomEndpointURL = &previous
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(existing, nil)
		deps.repo.EXPECT().ListCustomEndpoints(gomock.Any()).Return(map[string]string{
			"ws1":     previous,
			"brand_b": "https://mail.brand-b.example",
		}, nil)
		deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Times(0)

		_, err := svc.UpdateWorkspace(ctx, "ws1", "Old Name", settingsWith("https://mail.brand-b.example"))
		var validation domain.ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Contains(t, validation.Message, "another workspace")
	})

	t.Run("keeps a domain the workspace already has without looking it up", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		deps.expectOwner(ctx, "ws1", "user-1")
		existing := existingWorkspaceForUpdate()
		endpoint := "https://mail.brand-a.example"
		existing.Settings.CustomEndpointURL = &endpoint
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(existing, nil)
		deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)

		_, err := svc.UpdateWorkspace(ctx, "ws1", "Old Name", settingsWith(endpoint))
		require.NoError(t, err)
	})

	t.Run("lets a workspace edit the URL of a domain it shares, the host unchanged", func(t *testing.T) {
		// Workspaces that shared a domain before the rule keep it; rewriting its URL
		// is not taking another workspace's domain. The new URL is still checked in DNS.
		ctrl := gomock.NewController(t)
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		verifier := svcmocks.NewMockCustomDomainVerifier(ctrl)
		svc.dnsVerificationService = verifier
		deps.expectOwner(ctx, "ws1", "user-1")
		existing := existingWorkspaceForUpdate()
		previous := "http://mail.shared.example"
		existing.Settings.CustomEndpointURL = &previous
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(existing, nil)
		deps.repo.EXPECT().ListCustomEndpoints(gomock.Any()).Times(0)
		verifier.EXPECT().VerifyDomainOwnership(gomock.Any(), "https://MAIL.shared.example/").Return(nil).Times(1)
		deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil).Times(1)

		_, err := svc.UpdateWorkspace(ctx, "ws1", "Old Name", settingsWith("https://MAIL.shared.example/"))
		require.NoError(t, err)
	})

	t.Run("a URL edit that keeps the host still has to pass the DNS check in production", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		svc.config.Environment = "production"
		verifier := svcmocks.NewMockCustomDomainVerifier(ctrl)
		svc.dnsVerificationService = verifier
		deps.expectOwner(ctx, "ws1", "user-1")
		existing := existingWorkspaceForUpdate()
		previous := "https://mail.brand-a.example"
		existing.Settings.CustomEndpointURL = &previous
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(existing, nil)
		refused := domain.ValidationError{Message: "the domain does not point to this instance"}
		verifier.EXPECT().VerifyDomainOwnership(gomock.Any(), "https://mail.brand-a.example/links").Return(refused)
		deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Times(0)

		_, err := svc.UpdateWorkspace(ctx, "ws1", "Old Name", settingsWith("https://mail.brand-a.example/links"))
		assert.ErrorIs(t, err, refused)
	})

	t.Run("refuses a new custom endpoint whose host cannot be read", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		deps.expectOwner(ctx, "ws1", "user-1")
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(existingWorkspaceForUpdate(), nil)
		deps.repo.EXPECT().ListCustomEndpoints(gomock.Any()).Times(0)
		deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Times(0)

		_, err := svc.UpdateWorkspace(ctx, "ws1", "Old Name", settingsWith("https://%FF.example"))
		var validation domain.ValidationError
		require.ErrorAs(t, err, &validation)
	})

	t.Run("a failure to read the other domains refuses the save", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		deps.expectOwner(ctx, "ws1", "user-1")
		deps.repo.EXPECT().GetByID(gomock.Any(), "ws1").Return(existingWorkspaceForUpdate(), nil)
		deps.repo.EXPECT().ListCustomEndpoints(gomock.Any()).Return(nil, errors.New("db down"))
		deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Times(0)

		_, err := svc.UpdateWorkspace(ctx, "ws1", "Old Name", settingsWith("https://mail.brand-a.example"))
		assert.Error(t, err)
	})
}

func TestWorkspaceService_EnsureCustomDomainIsFree(t *testing.T) {
	ctx := context.Background()
	inUse := map[string]string{
		"ws1":     "https://mail.brand-a.example",
		"brand_b": "https://[2001:db8::1]:8443",
	}

	t.Run("accepts a domain no other workspace uses, and the workspace's own", func(t *testing.T) {
		for _, endpoint := range []string{"https://new.brand-a.example", "https://MAIL.brand-a.example:443", "https://[2001:db8::2]"} {
			svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
			deps.repo.EXPECT().ListCustomEndpoints(gomock.Any()).Return(inUse, nil)
			assert.NoError(t, svc.ensureCustomDomainIsFree(ctx, "ws1", endpoint), endpoint)
		}
	})

	t.Run("refuses another workspace's IPv6 literal", func(t *testing.T) {
		svc, deps := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		deps.repo.EXPECT().ListCustomEndpoints(gomock.Any()).Return(inUse, nil)
		var validation domain.ValidationError
		assert.ErrorAs(t, svc.ensureCustomDomainIsFree(ctx, "ws1", "https://[2001:DB8::1]"), &validation)
	})

	t.Run("an endpoint without a host is left to the URL validation", func(t *testing.T) {
		svc, _ := covWorkspaceService(gomock.NewController(t), nil, nil, nil)
		assert.NoError(t, svc.ensureCustomDomainIsFree(ctx, "ws1", ""))
	})
}
