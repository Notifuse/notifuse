package service

import (
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/branding"
)

// BrandingService answers which identity the instance shows: its own brand while a licence covers
// FeatureBranding, Notifuse's otherwise.
//
// It holds the brand stored when the process started. A save restarts the process, so the
// snapshot never goes stale. The licence, on the other hand, is asked on every call: a key can
// expire while the process runs, and the next page load or email must already show Notifuse.
// Nothing here writes, so the stored brand is never altered by a lapse and comes back with a
// licence.
type BrandingService struct {
	stored       branding.Branding
	entitlements domain.EntitlementProvider
}

// NewBrandingService wraps the stored brand. A nil provider means unlicensed: the Notifuse
// identity is shown.
func NewBrandingService(stored branding.Branding, entitlements domain.EntitlementProvider) *BrandingService {
	return &BrandingService{stored: stored.Normalize(), entitlements: entitlements}
}

// Effective returns what every surface shows now. Its Revision is always that of the stored
// brand, whatever is shown, so the save-and-restart handshake can tell which snapshot a process
// serves. A nil receiver answers the Notifuse identity, so a caller wired before the service
// exists still gets an answer.
func (s *BrandingService) Effective() branding.Resolved {
	if s == nil {
		return branding.Resolve(branding.Branding{})
	}
	shown := branding.Branding{}
	if s.entitlements != nil && s.entitlements.Entitlements().Has(domain.FeatureBranding) {
		shown = s.stored
	}
	resolved := branding.Resolve(shown)
	resolved.Revision = s.stored.Revision()
	return resolved
}

// Name is the name to print where Notifuse's would be: the brand's in custom mode, "Notifuse"
// otherwise. The mailers receive it through mailer.Config.BrandName.
func (s *BrandingService) Name() string {
	return s.Effective().Name
}

// Revision identifies the stored brand this process serves.
func (s *BrandingService) Revision() string {
	if s == nil {
		return branding.Branding{}.Revision()
	}
	return s.stored.Revision()
}
