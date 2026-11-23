package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dkimT0 = time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

func TestEvaluateDKIMCheck_OKPublishesAtOnce(t *testing.T) {
	downgraded := dkimT0.Add(-time.Hour)
	key := DKIMKey{Domain: "acme.io", Selector: "s", Status: DKIMKeyStatusMismatch, Revision: 4,
		LastCheckError: "old", DowngradedAt: &downgraded, DowngradeReason: "gone"}

	got, tr := EvaluateDKIMCheck(key, DKIMCheckResultOK, "", dkimT0)

	assert.Equal(t, DKIMKeyStatusPublished, got.Status)
	assert.Equal(t, int64(5), got.Revision)
	assert.Equal(t, dkimT0, *got.VerifiedAt)
	assert.Equal(t, dkimT0, *got.LastCheckedAt)
	assert.Empty(t, got.LastCheckError)
	assert.Nil(t, got.DowngradedAt)
	assert.Empty(t, got.DowngradeReason)
	require.NotNil(t, tr)
	assert.Equal(t, DKIMKeyStatusMismatch, tr.OldStatus)
	assert.Equal(t, DKIMKeyStatusPublished, tr.NewStatus)
	assert.False(t, tr.IsDowngrade())

	again, tr := EvaluateDKIMCheck(got, DKIMCheckResultOK, "", dkimT0.Add(time.Hour))
	assert.Nil(t, tr, "no transition when the status does not change")
	assert.Equal(t, int64(6), again.Revision)
}

func TestEvaluateDKIMCheck_FailureOfAKeyThatDoesNotSign(t *testing.T) {
	key := DKIMKey{Status: DKIMKeyStatusPending}

	got, tr := EvaluateDKIMCheck(key, DKIMCheckResultMismatch, "another key", dkimT0)
	assert.Equal(t, DKIMKeyStatusMismatch, got.Status)
	assert.Equal(t, "another key", got.LastCheckError)
	assert.Zero(t, got.ConsecutiveFailures)
	require.NotNil(t, tr)
	assert.False(t, tr.IsDowngrade())

	got, tr = EvaluateDKIMCheck(got, DKIMCheckResultNotFound, "", dkimT0)
	assert.Equal(t, DKIMKeyStatusPending, got.Status)
	assert.Equal(t, "not_found", got.LastCheckError)
	assert.NotNil(t, tr)
}

func TestEvaluateDKIMCheck_DNSErrorIsNeutral(t *testing.T) {
	counted := dkimT0.Add(-2 * time.Hour)
	key := DKIMKey{Status: DKIMKeyStatusPublished, ConsecutiveFailures: 2, LastCountedFailureAt: &counted, Revision: 1}

	got, tr := EvaluateDKIMCheck(key, DKIMCheckResultDNSError, "DNS lookup failed: timeout", dkimT0)

	assert.Nil(t, tr)
	assert.Equal(t, DKIMKeyStatusPublished, got.Status)
	assert.Equal(t, 2, got.ConsecutiveFailures)
	assert.Equal(t, counted, *got.LastCountedFailureAt)
	assert.Equal(t, "DNS lookup failed: timeout", got.LastCheckError)
	assert.Equal(t, dkimT0, *got.LastCheckedAt)
	assert.Equal(t, int64(2), got.Revision)
}

func TestEvaluateDKIMCheck_PublishedKeyStopsAtTheThirdSpacedFailure(t *testing.T) {
	key := DKIMKey{Domain: "acme.io", Selector: "s", Status: DKIMKeyStatusPublished}
	now := dkimT0
	var transitions []*DKIMTransition

	step := func(result DKIMCheckResult, after time.Duration) {
		now = now.Add(after)
		var tr *DKIMTransition
		key, tr = EvaluateDKIMCheck(key, result, "record removed", now)
		if tr != nil {
			transitions = append(transitions, tr)
		}
	}

	step(DKIMCheckResultNotFound, 0)
	assert.Equal(t, 1, key.ConsecutiveFailures)
	assert.Equal(t, DKIMKeyStatusPublished, key.Status, "1/3, still signing")

	step(DKIMCheckResultNotFound, 10*time.Minute)
	step(DKIMCheckResultMismatch, 44*time.Minute)
	assert.Equal(t, 1, key.ConsecutiveFailures, "failures well under an hour after the counted one are not counted")

	step(DKIMCheckResultNotFound, time.Minute) // 55 minutes after: an hourly check a little early
	assert.Equal(t, 2, key.ConsecutiveFailures)
	step(DKIMCheckResultDNSError, 2*time.Hour)
	assert.Equal(t, 2, key.ConsecutiveFailures)
	assert.Equal(t, DKIMKeyStatusPublished, key.Status)
	assert.Empty(t, transitions)

	step(DKIMCheckResultNotFound, 0) // 2h after the second counted one
	assert.Equal(t, DKIMKeyStatusPending, key.Status)
	assert.Equal(t, now, *key.DowngradedAt)
	assert.Equal(t, "record removed", key.DowngradeReason)
	assert.Zero(t, key.ConsecutiveFailures)
	require.Len(t, transitions, 1)
	assert.True(t, transitions[0].IsDowngrade())
	assert.Equal(t, DKIMKeyStatusPublished, transitions[0].OldStatus)
	assert.Equal(t, DKIMKeyStatusPending, transitions[0].NewStatus)

	step(DKIMCheckResultNotFound, 3*time.Hour)
	assert.Len(t, transitions, 1, "the downgrade is reported once")
	assert.NotNil(t, key.DowngradedAt, "the downgrade stays shown")
}

func TestEvaluateDKIMCheck_MismatchDowngradesToMismatch(t *testing.T) {
	key := DKIMKey{Status: DKIMKeyStatusPublished}
	for i := 0; i < DKIMDowngradeThreshold; i++ {
		key, _ = EvaluateDKIMCheck(key, DKIMCheckResultMismatch, "another key", dkimT0.Add(time.Duration(i)*time.Hour))
	}
	assert.Equal(t, DKIMKeyStatusMismatch, key.Status)
}

func TestEvaluateDKIMCheck_OKResetsTheCount(t *testing.T) {
	key := DKIMKey{Status: DKIMKeyStatusPublished}
	key, _ = EvaluateDKIMCheck(key, DKIMCheckResultNotFound, "", dkimT0)
	key, _ = EvaluateDKIMCheck(key, DKIMCheckResultOK, "", dkimT0.Add(time.Hour))
	assert.Zero(t, key.ConsecutiveFailures)
	assert.Nil(t, key.LastCountedFailureAt)
}

func TestEvaluateDKIMCheck_BoundsTheReason(t *testing.T) {
	got, _ := EvaluateDKIMCheck(DKIMKey{}, DKIMCheckResultMismatch, strings.Repeat("é", 300), dkimT0)
	assert.LessOrEqual(t, len(got.LastCheckError), 255)
	assert.True(t, strings.HasPrefix(got.LastCheckError, "éé"))
}
