package domain

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageSigningKey(t *testing.T) {
	t.Run("is derived from SECRET_KEY, never equal to it", func(t *testing.T) {
		secret := "s3cr3t-key-value"
		key := UsageSigningKey(secret)

		require.Len(t, key, 32, "HMAC-SHA256 output")
		// The point of deriving: SECRET_KEY also signs JWTs and encrypts the
		// database, so a leaked usage key must not be that value.
		assert.NotEqual(t, secret, string(key))
		assert.NotContains(t, string(key), secret)
	})

	t.Run("is stable, so both sides derive the same key with nothing to distribute", func(t *testing.T) {
		assert.Equal(t, UsageSigningKey("abc"), UsageSigningKey("abc"))
	})

	t.Run("differs per installation", func(t *testing.T) {
		assert.NotEqual(t, UsageSigningKey("abc"), UsageSigningKey("abd"))
	})
}

// TestUsageSignatureGoldenVector pins the wire format against fixed inputs.
//
// The control plane is a separate Go module and cannot import this package, so
// it reimplements this scheme. Nothing else would catch the two copies drifting:
// a change here would keep every test in this repository green while silently
// locking out every caller. The manager carries the identical vector, so
// whichever side is edited fails first.
//
// Changing these constants means changing the wire format. Bump the version
// prefix and keep the old branch instead.
func TestUsageSignatureGoldenVector(t *testing.T) {
	const (
		secret       = "golden-vector-secret"
		keyHex       = "9d1c6fc70542f7901782f00e82e6210d7ddacef8d392d0854a88c0b4d2f6d104"
		timestamp    = int64(1755345600)
		path         = "/api/usage.get"
		expectedSign = "v1,CBjjN9MyWfv41F0BemwrgCfZhx2UIrIn4QpRFThXOcU="
	)

	key := UsageSigningKey(secret)
	assert.Equal(t, keyHex, hex.EncodeToString(key), "the key derivation is part of the contract")
	assert.Equal(t, expectedSign, SignUsageRequest(key, timestamp, path))
}

func TestVerifyUsageSignature(t *testing.T) {
	key := UsageSigningKey("installation-secret")
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	const path = "/api/usage.get"

	t.Run("accepts a fresh signature", func(t *testing.T) {
		ts := now.Unix()
		require.NoError(t, VerifyUsageSignature(key, ts, path, SignUsageRequest(key, ts, path), now))
	})

	t.Run("rejects a signature made with another installation's key", func(t *testing.T) {
		ts := now.Unix()
		other := UsageSigningKey("someone-elses-secret")
		assert.Error(t, VerifyUsageSignature(key, ts, path, SignUsageRequest(other, ts, path), now))
	})

	t.Run("rejects a replay outside the skew window", func(t *testing.T) {
		ts := now.Add(-UsageSignatureMaxSkew - time.Second).Unix()
		sig := SignUsageRequest(key, ts, path)
		// The signature itself is perfectly valid — it is age alone that rejects it,
		// which is what stops a captured request being replayed forever.
		assert.Error(t, VerifyUsageSignature(key, ts, path, sig, now))
		assert.NoError(t, VerifyUsageSignature(key, ts, path, sig, time.Unix(ts, 0).UTC()))
	})

	t.Run("rejects a clock too far ahead as well as too far behind", func(t *testing.T) {
		ts := now.Add(UsageSignatureMaxSkew + time.Second).Unix()
		assert.Error(t, VerifyUsageSignature(key, ts, path, SignUsageRequest(key, ts, path), now))
	})

	t.Run("the timestamp is inside the signed content", func(t *testing.T) {
		// Moving a captured request's timestamp forward to keep it alive must
		// invalidate it, or the skew window buys nothing.
		signedAt := now.Add(-UsageSignatureMaxSkew - time.Minute).Unix()
		sig := SignUsageRequest(key, signedAt, path)
		assert.Error(t, VerifyUsageSignature(key, now.Unix(), path, sig, now))
	})

	t.Run("the path is inside the signed content", func(t *testing.T) {
		ts := now.Unix()
		sig := SignUsageRequest(key, ts, path)
		assert.Error(t, VerifyUsageSignature(key, ts, "/api/usage.other", sig, now))
	})

	t.Run("rejects an empty or malformed signature", func(t *testing.T) {
		ts := now.Unix()
		assert.Error(t, VerifyUsageSignature(key, ts, path, "", now))
		assert.Error(t, VerifyUsageSignature(key, ts, path, "v1,", now))
		assert.Error(t, VerifyUsageSignature(key, ts, path, "not-a-signature", now))
	})

	t.Run("the version prefix is part of the comparison", func(t *testing.T) {
		ts := now.Unix()
		sig := SignUsageRequest(key, ts, path)
		require.True(t, strings.HasPrefix(sig, UsageSignatureVersion+","))
		// A future scheme's signature must not be accepted by this verifier just
		// because the digest happens to match.
		assert.Error(t, VerifyUsageSignature(key, ts, path, strings.Replace(sig, "v1,", "v2,", 1), now))
	})

	t.Run("failures are indistinguishable to the caller", func(t *testing.T) {
		ts := now.Unix()
		stale := VerifyUsageSignature(key, now.Add(-time.Hour).Unix(), path, "v1,x", now)
		wrong := VerifyUsageSignature(key, ts, path, "v1,x", now)
		require.Error(t, stale)
		require.Error(t, wrong)
		// Telling a prober which half it got wrong is free information.
		assert.Equal(t, stale.Error(), wrong.Error())
	})
}

func TestParseUsageTimestamp(t *testing.T) {
	ts, err := ParseUsageTimestamp(" 1755345600 ")
	require.NoError(t, err)
	assert.Equal(t, int64(1755345600), ts)

	for _, bad := range []string{"", "abc", "1.5", "9999999999999999999999"} {
		_, err := ParseUsageTimestamp(bad)
		assert.Error(t, err, "must reject %q", bad)
	}
}
