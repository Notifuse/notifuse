package domain

import "time"

const (
	// DKIMDowngradeThreshold is how many failed checks, counted at least
	// DKIMFailureSpacing apart, stop a published key from signing.
	DKIMDowngradeThreshold = 3
	// DKIMFailureSpacing is the least time between two counted failures, so that a
	// record removed for a few minutes, or a burst of manual checks, does not stop
	// signing.
	DKIMFailureSpacing = time.Hour
)

// EvaluateDKIMCheck applies one lookup of a key's record to the key. The manual check
// and the periodic one share it:
//   - ok publishes the key at once;
//   - dns_error changes nothing but the time and reason of the last check;
//   - not_found or mismatch moves a key that does not sign to pending or mismatch at
//     once, and stops a published key only at the DKIMDowngradeThreshold-th failure
//     counted DKIMFailureSpacing apart.
//
// transition is non-nil when the status changed. Revision is incremented by every
// call, so a concurrent writer holding the old revision is refused.
func EvaluateDKIMCheck(key DKIMKey, result DKIMCheckResult, detail string, now time.Time) (DKIMKey, *DKIMTransition) {
	updated := key
	updated.Revision++
	updated.LastCheckedAt = &now
	old := key.Status

	switch result {
	case DKIMCheckResultOK:
		updated.Status = DKIMKeyStatusPublished
		updated.VerifiedAt = &now
		updated.LastCheckError = ""
		updated.ConsecutiveFailures = 0
		updated.LastCountedFailureAt = nil
		updated.DowngradedAt = nil
		updated.DowngradeReason = ""

	case DKIMCheckResultNotFound, DKIMCheckResultMismatch:
		failed := DKIMKeyStatusPending
		if result == DKIMCheckResultMismatch {
			failed = DKIMKeyStatusMismatch
		}
		if detail == "" {
			detail = string(result)
		}
		updated.LastCheckError = truncateDKIMCheckError(detail)

		if key.Status != DKIMKeyStatusPublished {
			updated.Status = failed
			break
		}
		if key.LastCountedFailureAt == nil || now.Sub(*key.LastCountedFailureAt) >= DKIMFailureSpacing {
			updated.ConsecutiveFailures = key.ConsecutiveFailures + 1
			updated.LastCountedFailureAt = &now
		}
		if updated.ConsecutiveFailures >= DKIMDowngradeThreshold {
			updated.Status = failed
			updated.DowngradedAt = &now
			updated.DowngradeReason = updated.LastCheckError
			updated.ConsecutiveFailures = 0
			updated.LastCountedFailureAt = nil
		}

	default: // DKIMCheckResultDNSError: may pass, so it counts for nothing.
		if detail == "" {
			detail = string(DKIMCheckResultDNSError)
		}
		updated.LastCheckError = truncateDKIMCheckError(detail)
	}

	if updated.Status == old {
		return updated, nil
	}
	return updated, &DKIMTransition{
		Domain:     key.Domain,
		Selector:   key.Selector,
		OldStatus:  old,
		NewStatus:  updated.Status,
		Reason:     updated.LastCheckError,
		OccurredAt: now,
	}
}

// IsDowngrade says whether the transition stopped a key that signed.
func (t *DKIMTransition) IsDowngrade() bool {
	return t != nil && t.OldStatus == DKIMKeyStatusPublished && t.NewStatus != DKIMKeyStatusPublished
}
