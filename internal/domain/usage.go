package domain

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

//go:generate mockgen -destination mocks/mock_usage_service.go -package mocks github.com/Notifuse/notifuse/internal/domain UsageService

// Usage metering is read by the control plane, which pulls it from each
// installation rather than being pushed to. Pulling is what keeps a self-hosted
// installation from ever initiating an outbound call: there is no code path here
// that phones anywhere, only an endpoint that answers when asked with a valid
// signature.

const (
	// usageKeyLabel domain-separates the usage key from every other use of
	// SECRET_KEY.
	usageKeyLabel = "notifuse.usage.v1"

	// UsageSignatureMaxSkew bounds how long a signed request stays valid. Short,
	// because the caller and this server are both machines with clocks, and a
	// captured request must not stay replayable.
	UsageSignatureMaxSkew = 5 * time.Minute

	// UsageSignatureVersion prefixes the signature, so the scheme can change
	// without the verifier having to guess which one it is looking at.
	UsageSignatureVersion = "v1"
)

// UsageSigningKey derives the key that authenticates usage reads from the
// installation's SECRET_KEY.
//
// Derived, rather than a separate configured secret, because the control plane
// already holds every tenant's SECRET_KEY: there is nothing new to generate,
// distribute, store or rotate, and both sides recompute the same key from what
// they already have. Derived, rather than SECRET_KEY used directly, because
// SECRET_KEY also signs JWTs and encrypts the database — a usage key that leaked
// would otherwise be a full compromise, whereas HMAC is one-way, so this key
// reveals nothing about the one it came from.
func UsageSigningKey(secretKey string) []byte {
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(usageKeyLabel))
	return h.Sum(nil)
}

// SignUsageRequest returns the signature for a usage read.
//
// The signed content is `{timestamp}.{path}`, and the output is
// `v1,{base64(HMAC-SHA256)}` — the same shape signPayload already uses for
// outbound webhooks, so there is one signature convention in this codebase
// rather than two. The timestamp is inside the signed content, which is what
// stops it being edited to extend a captured request's life.
func SignUsageRequest(key []byte, timestamp int64, path string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(strconv.FormatInt(timestamp, 10)))
	h.Write([]byte("."))
	h.Write([]byte(path))
	return UsageSignatureVersion + "," + base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// VerifyUsageSignature checks a usage read's signature and freshness.
//
// Both failures return the same opaque error: telling a caller whether it got
// the timestamp or the signature wrong is free information for someone probing
// the endpoint, and neither is actionable for a legitimate caller.
func VerifyUsageSignature(key []byte, timestamp int64, path, signature string, now time.Time) error {
	if signature == "" {
		return fmt.Errorf("invalid usage signature")
	}

	// Freshness first, so a replayed request is rejected without the constant-time
	// comparison having to run at all.
	skew := now.UTC().Sub(time.Unix(timestamp, 0).UTC())
	if skew < 0 {
		skew = -skew
	}
	if skew > UsageSignatureMaxSkew {
		return fmt.Errorf("invalid usage signature")
	}

	expected := SignUsageRequest(key, timestamp, path)
	// Compared as whole strings, version prefix included, so a signature computed
	// under a future scheme can never be accepted by this one.
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("invalid usage signature")
	}
	return nil
}

// ParseUsageTimestamp reads the timestamp header. Separate from verification so
// a malformed header is one error rather than a panic in the caller.
func ParseUsageTimestamp(raw string) (int64, error) {
	ts, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid usage timestamp")
	}
	return ts, nil
}

// InstanceUsage is what this installation reports for one month, summed across
// every workspace it holds.
//
// The quota is instance-wide because PLAN_MAX_* arrives as a process
// environment variable while a tenant may hold several workspaces, so the sum is
// the number a quota is actually measured against.
type InstanceUsage struct {
	// PeriodMonth is the first day of the UTC month, at midnight UTC.
	PeriodMonth     time.Time `json:"period_month"`
	Pageviews       int64     `json:"pageviews"`
	TimelineEntries int64     `json:"timeline_entries"`

	// Workspaces is how many workspaces contributed a snapshot for this month.
	Workspaces int `json:"workspaces"`

	// ComputedAt is the OLDEST snapshot time among those workspaces, so a reader
	// sees the true staleness of the total rather than the freshest part of it.
	// Zero when no workspace has been metered for this month yet.
	ComputedAt time.Time `json:"computed_at"`
}

// UsageReport is the response to a usage read.
type UsageReport struct {
	Months []*InstanceUsage `json:"months"`

	// WorkspaceCount is every workspace on this installation, whether or not it
	// has been metered yet — the denominator for reading Workspaces above.
	WorkspaceCount int       `json:"workspace_count"`
	GeneratedAt    time.Time `json:"generated_at"`
}

// UsageService reports metered usage for the whole installation.
type UsageService interface {
	// GetUsage sums the stored monthly snapshots across every workspace.
	//
	// It fails rather than returning a partial total when a workspace cannot be
	// read. A total that silently omits a workspace is a wrong number the control
	// plane would act on, whereas an error means no usage was reported at all,
	// and the documented rule for missing usage is to skip and log.
	GetUsage(ctx context.Context, months []time.Time) (*UsageReport, error)
}
