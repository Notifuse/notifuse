package service

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
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
