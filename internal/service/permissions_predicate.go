package service

import "github.com/Notifuse/notifuse/internal/domain"

// grantsFullPermissions reports whether a permission map grants read and write on every
// resource this build knows about.
//
// It lives in its own file because two unrelated concerns need exactly the same answer and
// must never be allowed to drift apart:
//
//   - the RBAC gate in workspace_service.go, where "not full" is precisely the assignment an
//     unlicensed deployment may not write;
//   - the rbac_custom telemetry flag in telemetry_service.go, which measures how many
//     installations hold such an assignment today, i.e. the blast radius of that same gate.
//
// A telemetry number that answered a slightly different question from the gate it is meant to
// size would be worse than no number at all, so there is one predicate and one definition of
// "full".
//
// It reads the map and nothing else, so nil and empty both answer false: stored on a
// membership row, either one denies every verb, which is a granular grant like any other.
// CreateAPIKey is the one caller for which nil means "full access", and it says so at its own
// gate rather than bending this predicate — nil means opposite things on the two paths, and a
// helper that picked one silently would hand out the other for free.
//
// The comparison runs over domain.AllPermissionResources rather than a literal, because that
// list grows — v39 added segments, webhook_subscriptions and webhook_events — and a hardcoded
// expectation would gate every deployment, and report every installation as customised, the
// day a resource is added.
//
// Unknown keys need no handling here — every write caller runs permissions.Validate() first,
// which rejects a resource this build does not know.
func grantsFullPermissions(permissions domain.UserPermissions) bool {
	for _, resource := range domain.AllPermissionResources {
		granted, ok := permissions[resource]
		if !ok || !granted.Read || !granted.Write {
			return false
		}
	}
	return true
}
