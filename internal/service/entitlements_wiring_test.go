package service

import (
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
)

// fullyLicensedProvider is the EntitlementProvider handed to the construction sites that
// predate licensing — the ones whose subject is something else entirely (listing workspaces,
// verifying DNS, blank integration credentials) and which must keep asserting the behaviour
// they always asserted.
//
// It answers an active Enterprise grant, because that is what those tests used to run under:
// before the gates existed every capability was ungated, and a provider granting everything is
// the only substitution that leaves their expectations unchanged. Handing them
// CommunityEntitlements would make them assert the gated behaviour by accident, which is the
// licence gates' own tests' job — see the licence-gate section of workspace_service_test.go,
// which drives its own providers.
//
// It is a generated mock rather than a hand-written stub so that a gate consulting the
// provider is visible to gomock like every other collaborator. AnyTimes, because these tests
// pin behaviour, not how often a gate asks.
func fullyLicensedProvider(ctrl *gomock.Controller) domain.EntitlementProvider {
	provider := mocks.NewMockEntitlementProvider(ctrl)
	provider.EXPECT().Entitlements().Return(domain.Entitlements{
		Tier:          "enterprise",
		Org:           "Test Fixture SAS",
		MaxWorkspaces: domain.UnlimitedWorkspaces,
		Features: []domain.Feature{
			domain.FeatureRBAC,
			domain.FeatureSESTenant,
			domain.FeatureSSO,
			domain.FeatureAuditLogs,
			domain.FeatureTemplateI18n,
		},
		State:     domain.LicenseStateActive,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}).AnyTimes()
	return provider
}
