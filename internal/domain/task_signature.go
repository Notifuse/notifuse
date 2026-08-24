package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// /api/tasks.execute is the scheduler's own dispatch target: a machine calling a
// machine, with no user session to authenticate against. It is authenticated by
// signature instead, derived from SECRET_KEY, which both sides already hold — the
// same arrangement the usage read uses (see usage.go).
//
// It matters that this endpoint is authenticated at all: MarkAsRunningTx accepts a
// task in status pending OR paused, so an unsigned call runs a scheduled-but-not-due
// — or deliberately paused — task now. That is the effect of tasks.trigger, which
// the handler gates on the owning resource's write permission.

const (
	// taskExecuteKeyLabel domain-separates the dispatch key from every other use
	// of SECRET_KEY, including the usage key.
	taskExecuteKeyLabel = "notifuse.task-execute.v1"

	// TaskExecuteSignatureMaxSkew bounds how long a signed dispatch stays valid.
	TaskExecuteSignatureMaxSkew = 5 * time.Minute

	// TaskExecuteSignatureVersion prefixes the signature, so the scheme can change
	// without the verifier having to guess which one it is looking at.
	TaskExecuteSignatureVersion = "v1"

	// TaskExecuteTimestampHeader carries the Unix seconds the dispatch was signed at.
	TaskExecuteTimestampHeader = "X-Notifuse-Timestamp"
	// TaskExecuteSignatureHeader carries `v1,{base64(HMAC-SHA256)}`.
	TaskExecuteSignatureHeader = "X-Notifuse-Signature"
)

// TaskExecuteSigningKey derives the key that authenticates task dispatches from
// the installation's SECRET_KEY. HMAC is one-way, so this key reveals nothing
// about the one it came from — which also signs JWTs and encrypts the database.
func TaskExecuteSigningKey(secretKey string) []byte {
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(taskExecuteKeyLabel))
	return h.Sum(nil)
}

// SignTaskExecuteRequest returns the signature for one dispatch.
//
// The signed content is `{timestamp}.{path}.{sha256(body)}` and the output is
// `v1,{base64(HMAC-SHA256)}`. The body digest is what distinguishes this from the
// usage read's bare-path form: every dispatch hits the same path, so a signature
// over the path alone would be a five-minute wildcard for any task id in any
// workspace once captured. The timestamp is inside the signed content, which is
// what stops it being edited to extend a captured dispatch's life.
func SignTaskExecuteRequest(key []byte, timestamp int64, path string, body []byte) string {
	digest := sha256.Sum256(body)

	h := hmac.New(sha256.New, key)
	h.Write([]byte(strconv.FormatInt(timestamp, 10)))
	h.Write([]byte("."))
	h.Write([]byte(path))
	h.Write([]byte("."))
	h.Write([]byte(hex.EncodeToString(digest[:])))
	return TaskExecuteSignatureVersion + "," + base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// VerifyTaskExecuteSignature checks a dispatch's signature and freshness.
//
// Both failures return the same opaque error: telling a caller whether it got the
// timestamp or the signature wrong is free information for someone probing the
// endpoint, and neither is actionable for a legitimate caller. The handler logs
// the distinguishing detail on its own line instead.
func VerifyTaskExecuteSignature(key []byte, timestamp int64, path string, body []byte, signature string, now time.Time) error {
	if signature == "" {
		return fmt.Errorf("invalid task execution signature")
	}

	// Freshness first, so a replayed dispatch is rejected without the
	// constant-time comparison having to run at all.
	skew := now.UTC().Sub(time.Unix(timestamp, 0).UTC())
	if skew < 0 {
		skew = -skew
	}
	if skew > TaskExecuteSignatureMaxSkew {
		return fmt.Errorf("invalid task execution signature")
	}

	expected := SignTaskExecuteRequest(key, timestamp, path, body)
	// Compared as whole strings, version prefix included, so a signature computed
	// under a future scheme can never be accepted by this one.
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("invalid task execution signature")
	}
	return nil
}

// ParseTaskExecuteTimestamp reads the timestamp header. Separate from
// verification so a malformed header is one error rather than a panic in the
// caller.
func ParseTaskExecuteTimestamp(raw string) (int64, error) {
	ts, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid task execution timestamp")
	}
	return ts, nil
}
