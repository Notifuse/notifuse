package domain

import (
	"fmt"
	"slices"
	"time"
)

//go:generate mockgen -destination mocks/mock_entitlement_provider.go -package mocks github.com/Notifuse/notifuse/internal/domain EntitlementProvider

// Licensing is decided per deployment — one Postgres database, whatever the number of API
// containers standing in front of it — and answered entirely offline from a signed key. No
// type in this file reaches the network, keeps state, or can fail: an installation with no
// key, with a key it cannot parse, or with a key that ran out is a Community installation
// holding exactly what CommunityEntitlements returns.
//
// Two rules govern everything here and everything that reads it:
//
// The absence of a licence only ever RESTRICTS. No code path may grant more when unlicensed
// than it grants when licensed — an installation that upgrades must never see a member's
// restricted permissions widen into full ones.
//
// Licensing never blocks the machines. It gates the creation of new configuration, never a
// send, never a read, never a deletion, never a login. HasPermission, the SES send path,
// endpoint and custom-domain resolution, the OIDC guards and magic-code login are all outside
// its reach, and the "never touch" list on EntitlementProvider names them one by one.

// Feature names one licence-gated capability.
//
// The set is a closed whitelist, never a blacklist: a key minted today must not silently
// unlock a capability invented three releases from now, so a feature string this build does
// not recognise is not a feature at all — IsValid rejects it and the gate stays shut.
//
// These values are the wire strings carried in a key's "feat" array and echoed to the console,
// so they are frozen. Renaming one un-licenses every key already issued, and there is no
// phone-home that could re-issue them.
type Feature string

const (
	// FeatureRBAC gates *writing* workspace permissions that differ from FullPermissions.
	// Permissions already stored stay enforced without it: licensing never touches
	// HasPermission, and removing a member is always allowed so an offboarding or a
	// compromised API key can still be revoked.
	FeatureRBAC Feature = "rbac"

	// FeatureSESTenant gates provisioning a new SES tenant. It stops at the discovery
	// service; the send path is untouched, so a tenant provisioned while licensed keeps
	// resolving forever after.
	FeatureSESTenant Feature = "ses_tenant"

	// FeatureSSO gates the console's *write* access while OIDC is enabled. It never gates
	// signing in: SSO login and magic-code login both keep working in every state, and an
	// unlicensed instance can always turn OIDC back off to lift the restriction.
	FeatureSSO Feature = "sso"

	// FeatureAuditLogs is minted into Enterprise keys from day one although the capability
	// does not exist yet. Adding it later to a feature list is free; re-minting every key
	// already in a customer's hands is not.
	FeatureAuditLogs Feature = "audit_logs"

	// FeatureTemplateI18n gates *authoring* a multilingual variant of a template: adding a
	// language, or editing the content of one already there. Nothing else about
	// translations is touched.
	//
	// Rendering is never gated. Template.ResolveEmailContent and ResolveWebContent pick a
	// translation on the send path and must stay ignorant of licensing, so every message
	// already going out in Dutch keeps going out in Dutch on a deployment whose key has
	// lapsed. Removing a language is never gated either: it is the free way back inside the
	// licence, and a gate that trapped a deployment in the state it is being refused for
	// would be the one failure this design refuses to have.
	//
	// It is the first gate that reaches a single-workspace, single-brand installation. The
	// other four are per-customer (workspaces, SES tenants) or governance (SSO, audit
	// logs), so a sender with one brand and three languages met none of them.
	FeatureTemplateI18n Feature = "template_i18n"
)

// IsValid reports whether the feature is one this build knows how to gate.
//
// A licence may legitimately carry a feature string a newer control plane understands and
// this binary does not. Such a string is dropped rather than honoured — the whitelist is the
// point — and IsValid is how a caller filters it out.
func (f Feature) IsValid() bool {
	switch f {
	case FeatureRBAC, FeatureSESTenant, FeatureSSO, FeatureAuditLogs, FeatureTemplateI18n:
		return true
	default:
		return false
	}
}

// LicenseState is where an installation sits in the licence lifecycle.
//
// It is derived at read time by comparing the key's expiry against the current clock, never
// stored and never refreshed by a ticker: a stored state has a moment where it is stale, and
// a ticker is a goroutine that can die silently. Deriving it means the answer is correct the
// instant it is asked for.
type LicenseState string

const (
	// LicenseStateNone means no key is installed, or the installed key failed to parse or
	// verify. Every failure lands here: licence handling degrades, it never errors out.
	LicenseStateNone LicenseState = "none"

	// LicenseStateActive means the key verified and has not reached its expiry.
	LicenseStateActive LicenseState = "active"

	// LicenseStateGrace means the key expired but is still inside the grace period.
	// Everything the key grants keeps working; only the console says so. Grace exists
	// because a renewal payment can fail and be retried for two weeks, and a customer who
	// is being dunned must not lose capabilities they are in the middle of paying for.
	LicenseStateGrace LicenseState = "grace"

	// LicenseStateExpired means the key expired and the grace period is over. Entitlements
	// are identical to LicenseStateNone — there is no intermediate frozen tier.
	LicenseStateExpired LicenseState = "expired"
)

const (
	// CommunityMaxWorkspaces is how many workspaces an unlicensed installation may create.
	// Checked only at creation: an installation already holding more keeps every one of
	// them, and nothing is deleted, hidden, disabled or made read-only.
	CommunityMaxWorkspaces = 3

	// UnlimitedWorkspaces is the MaxWorkspaces value meaning "no ceiling". A negative
	// sentinel rather than zero, because zero is the value a struct has when nobody filled
	// it in, and reading an unfilled struct as "unlimited" is exactly the mistake that
	// would hand an unlicensed installation more than a licensed one.
	UnlimitedWorkspaces = -1
)

// Entitlements is the resolved answer to "what may this deployment do", already reconciled
// against the clock. Everything downstream reads this and nothing downstream parses a key.
//
// A zero Entitlements is NOT a valid answer: MaxWorkspaces would be 0, and a quota check
// written as `if limit > 0` reads 0 as unlimited. Anything handing out Entitlements returns
// CommunityEntitlements() when it has nothing better to say — never Entitlements{}.
type Entitlements struct {
	// Tier is the plan name printed on the key ("studio", "agency", "enterprise", …).
	//
	// DISPLAY ONLY. Never branch on it, in Go or in TypeScript. Every decision reads
	// Features and MaxWorkspaces, so a Custom deal is an arbitrary (features, quota) pair
	// that needs no code at all — and a tier renamed on the pricing page cannot change what
	// any installation is allowed to do.
	Tier string `json:"tier"`

	// Org and Sub are the licensee's organisation and billing contact, shown in the console
	// as "Licensed to: ACME SAS — billing@acme.com". Display only, and deliberately so: the
	// deterrent against passing a key around is social, not cryptographic.
	Org string `json:"org"`
	Sub string `json:"sub"`

	// MaxWorkspaces is the ceiling enforced when a workspace is created, and only then.
	// UnlimitedWorkspaces (-1) means no ceiling.
	MaxWorkspaces int `json:"max_workspaces"`

	// Features are the capabilities this key grants. Read it through Has, never by ranging
	// over it at a call site.
	//
	// A provider must not report features it is not entitled to serve: in LicenseStateNone
	// and LicenseStateExpired this is empty, because those two states are the same tier.
	Features []Feature `json:"features"`

	// State is the lifecycle position, for the console's banner and the startup log line.
	// Gates read Features and MaxWorkspaces; they do not switch on State.
	State LicenseState `json:"state"`

	// ExpiresAt is the key's expiry, zero when unlicensed. The console uses it to say how
	// long a grace period has left.
	ExpiresAt time.Time `json:"expires_at"`
}

// Has reports whether this deployment is licensed for the feature. It is the only predicate
// a gate should ever use.
func (e Entitlements) Has(feature Feature) bool {
	return slices.Contains(e.Features, feature)
}

// Licensed reports whether a valid key is installed, counting the grace period as licensed.
//
// It answers the console's "is there a key here" question — the banner, the "Licensed to"
// line — and must not be used as a gate: a gate asks Has, because an active Studio key is
// licensed and still has no right to SSO.
func (e Entitlements) Licensed() bool {
	return e.State == LicenseStateActive || e.State == LicenseStateGrace
}

// CommunityEntitlements returns what an installation with no valid licence may do. It is the
// answer for LicenseStateNone and for LicenseStateExpired alike, and it is the value every
// error path in licence handling degrades to.
//
// It builds a fresh value on every call, and Features is a fresh empty slice rather than a
// package-level one. FullPermissions is the cautionary tale: a package-level collection handed
// out by reference was mutated by a caller and corrupted the value for the whole process.
// Empty rather than nil so it marshals to [] for the console, which reads this off
// /api/user.me.
func CommunityEntitlements() Entitlements {
	return Entitlements{
		MaxWorkspaces: CommunityMaxWorkspaces,
		Features:      []Feature{},
		State:         LicenseStateNone,
	}
}

// The plan names quoted back to a buyer in a 402 body. Marketing labels shown to a human, not
// something to branch on — the same display-only rule as Entitlements.Tier, and unexported so
// no gate can reach for them.
const (
	tierStudio     = "Studio"
	tierEnterprise = "Enterprise"
)

// ErrFeatureNotLicensed is returned when a caller asked for a capability this deployment has
// no licence for.
//
// internal/http/utils.go maps it to HTTP 402 Payment Required, ahead of the PermissionError
// branch, so every handler that already funnels through writeServiceError/writePermissionError
// gets the mapping without an edit. 402 and not 403: the user's permissions are fine, the
// deployment simply has not bought this — and the console keys one purchase component off the
// status code, so the distinction has to live in the status, not in the body.
//
// A pointer struct rather than a sentinel because the console needs the feature name and the
// tier to sell: it renders "SES tenant isolation — Studio". Match it the way the rest of the
// package's struct errors are matched, through the wraps services add on the way up:
//
//	var notLicensed *domain.ErrFeatureNotLicensed
//	if errors.As(err, &notLicensed) { … }
type ErrFeatureNotLicensed struct {
	Feature      Feature `json:"feature"`
	RequiredTier string  `json:"required_tier"`
	Message      string  `json:"message"`
}

func (e *ErrFeatureNotLicensed) Error() string { return e.Message }

// NewFeatureNotLicensedError builds the refusal for a feature, filling in the plan a buyer
// needs and the sentence the console shows.
//
// A feature this build does not know quotes the most expensive plan rather than the cheapest.
// Guessing low would advertise a licence that does not actually carry the capability, and the
// refusal has to stay a refusal: unknown never resolves to a cheaper yes.
func NewFeatureNotLicensedError(f Feature) *ErrFeatureNotLicensed {
	switch f {
	case FeatureRBAC:
		return &ErrFeatureNotLicensed{
			Feature:      f,
			RequiredTier: tierStudio,
			Message:      "Custom permissions require a Notifuse licence (Studio or above).",
		}
	case FeatureSESTenant:
		return &ErrFeatureNotLicensed{
			Feature:      f,
			RequiredTier: tierStudio,
			Message:      "SES tenant isolation requires a Notifuse licence (Studio or above).",
		}
	case FeatureSSO:
		return &ErrFeatureNotLicensed{
			Feature:      f,
			RequiredTier: tierEnterprise,
			Message:      "Single sign-on requires a Notifuse Enterprise licence.",
		}
	case FeatureAuditLogs:
		return &ErrFeatureNotLicensed{
			Feature:      f,
			RequiredTier: tierEnterprise,
			Message:      "Audit logs require a Notifuse Enterprise licence.",
		}
	case FeatureTemplateI18n:
		return &ErrFeatureNotLicensed{
			Feature:      f,
			RequiredTier: tierStudio,
			Message:      "Template translations require a Notifuse licence (Studio or above). Translations already saved keep being sent, and removing one is always allowed.",
		}
	default:
		return &ErrFeatureNotLicensed{
			Feature:      f,
			RequiredTier: tierEnterprise,
			Message:      "This capability requires a Notifuse licence.",
		}
	}
}

// ErrWorkspaceQuotaReached is returned when creating a workspace would exceed the licence's
// workspace ceiling.
//
// It maps to HTTP 402, and that is the whole reason it exists next to ErrWorkspaceLimitReached
// rather than replacing it: the older error is the operator's own PLAN_MAX_WORKSPACES ceiling
// and maps to 403 in workspace_handler.go, where nothing is for sale. This one says "buy a
// bigger plan", so it needs the status the console's purchase component keys off.
//
// Enforced at creation only. No existing workspace is deleted, disabled, hidden or made
// read-only when a licence lapses — an installation holding eight workspaces keeps all eight
// and simply cannot create a ninth.
type ErrWorkspaceQuotaReached struct {
	Limit   int `json:"limit"`
	Current int `json:"current"`
}

func (e *ErrWorkspaceQuotaReached) Error() string {
	return fmt.Sprintf("workspace quota reached: %d workspaces exist (limit: %d)", e.Current, e.Limit)
}

// EntitlementProvider answers what this deployment is licensed for. It is the read-only port
// every gate consumes, and the only licence-shaped dependency any service takes.
//
// Entitlements takes no context and returns no error on purpose. A gate must be able to ask
// this question on any path without acquiring a cancellation, and there is no failure mode to
// report: an implementation that cannot read or verify a key returns CommunityEntitlements().
// Failing safe here means failing *closed on features* and *open on operation* — the
// deployment loses paid capabilities, never the ability to run.
//
// Implementations must never panic, never block on anything unbounded, and never return the
// zero Entitlements. Claims are parsed once and the lifecycle state is derived from the clock
// at read time, so the ordinary call is a mutex and a comparison. The single exception is an
// implementation retrying a stored key it could not read at all — that read is bounded by a
// timeout, rate-limited by a backoff, and taken by at most one goroutine at a time, because
// caching "I could not find out" as "there is no licence" would cost a paying customer every
// paid capability until somebody restarted the process.
//
// # Call sites
//
// There is no ee/ directory to grep, so every consumer of this interface is enumerated here
// instead. This list is the compensating control the plan owes for that absence, and the
// plan promises it is complete — which makes a stale entry worse than no list at all,
// because a reader who trusts it stops looking.
//
// So it is not prose. TestEntitlementProviderCallSitesAreListed in license_test.go walks the
// tree for consumers of this interface and fails when one of them is missing from the list
// below. If that test is red, the fix is to add the file here, not to loosen the test.
//
// Gates — the places where the absence of a licence actually refuses something. One entry is
// an inversion of that and is filed under Widenings below, on its own, because a list of
// refusals is exactly where a widening disappears:
//
//   - internal/service/workspace_service.go, CreateWorkspace — MaxWorkspaces (G1), refused
//     with *ErrWorkspaceQuotaReached.
//   - internal/service/workspace_service.go, SetUserPermissions — FeatureRBAC (G3).
//   - internal/service/workspace_service.go, InviteMember — FeatureRBAC (G3), only when the
//     invitation carries permissions other than FullPermissions. Accepting an invitation is
//     never gated: a colleague invited last week must still be able to walk in.
//   - internal/service/workspace_service.go, CreateAPIKey — FeatureRBAC (G3), only for a
//     scoped key. AddUserToWorkspace is gated the same way as defence in depth.
//   - internal/service/ses_discovery_service.go, EnableTenantIsolation — FeatureSESTenant
//     (G4), after the integration authorization check.
//   - internal/service/template_service.go, CreateTemplate and UpdateTemplate —
//     FeatureTemplateI18n (G5), only when the save ADDS a language or CHANGES the content of
//     one already stored (TranslationsWiden). Removing a language passes, an unrelated edit
//     to a template that happens to carry translations passes, and the send path is not
//     touched at all.
//   - internal/service/oidc_service.go, IsEnabled — FeatureSSO (G2). It answers "switched on
//     AND licensed for", and ensureProvider and ExchangeCode call it rather than restating
//     it, so every path that reaches the IdP is covered by one expression. This is the
//     narrowest gate of the four: it removes the sign-in button and nothing else. Magic-code
//     login keeps working for every user of the deployment — an SSO account always has a
//     verified email address — sessions already minted stay valid, and no console is walled.
//     internal/http/root_handler.go reads the same IsEnabled through a func() bool so that
//     the OIDC_ENABLED flag in config.js cannot disagree with the endpoint the button leads
//     to; it does not consult this interface itself.
//
// Readers — consumers that report the grant without refusing anything. They are listed
// because "where is this consulted" is the question the list exists to answer, and an
// unlisted reader is exactly as invisible as an unlisted gate:
//
//   - internal/service/telemetry_service.go, licenseTier — puts the plan name in the daily
//     telemetry payload. It gates nothing, it must never gate anything, and it treats a nil
//     provider and an expired key alike as unlicensed.
//   - internal/http/user_handler.go, GetCurrentUser — serves the grant on /api/user.me to
//     every console session. It is the only way a non-root user ever learns what the
//     deployment is licensed for, and therefore the single value the console keys off.
//   - internal/http/settings_handler.go, licenseResponse — serves the same grant on the
//     root-only /api/licence.get, and on the response to /api/licence.set so the console
//     repaints from the round trip that installed the key.
//
// Those two reach the grant through a port they declare themselves (LicenseStateReader,
// LicenseServiceInterface) rather than through this interface. That is deliberate house
// style, and it made both invisible to the honesty test below until it was taught to look
// for the CALL as well as the type. If a third such reader is added, the test sees it.
//
// # Widenings
//
// None. There is no place in the codebase where the absence of a licence grants more.
//
// There was one, and the heading stays so that its absence is a claim a reader can check
// rather than a list that forgot. ConnectZapier used to widen the Zapier key to
// FullPermissions on an unlicensed deployment, because the RBAC gate sat inside
// CreateAPIKey and would have refused the narrow scope. The gate now applies only to a scope
// the CUSTOMER chose (workspace_service.go, apiKeyScope); Zapier's scope is a constant of the
// product, minted through a path the API cannot reach, and is the same five resources in
// every licence state. A test asserts the widening has not returned.
//
// # Never
//
// Nothing outside the three lists above asks. In particular this interface must never be
// consulted by: HasPermission or any of its call sites; AmazonSESSettings.ResolveTenant or
// applySESSendingContext, or anything else on the send path; WorkspaceSettings.ResolveEndpoint,
// GetWorkspaceByCustomDomain or any other read that resolves a live tracking, unsubscribe or
// blog URL; magic-code login, which stays available unconditionally and is what makes gating
// SSO survivable; PauseBroadcast or PauseForCircuitBreaker; the MaxUsers seat check, which is
// never licensed; or any code path that deletes data.
//
// The OIDC service was on this list until the SSO gate replaced the console read-only wall.
// The wall is what the exclusion existed to protect: refusing SSO itself was rejected while
// the alternative was walling a whole deployment, because a login that stops working locks
// operators out of the very settings page where they would fix it. That reasoning does not
// survive the move — magic-code login is unconditional, every SSO account has a verified
// email address, and an existing session is a session, so nobody is locked out of anything.
// What replaced it is the rule directly above: the gate may take the button, never the login.
type EntitlementProvider interface {
	// Entitlements returns the current resolved grant. It is cheap enough to call inline on
	// a request path and always returns a usable value.
	Entitlements() Entitlements
}
