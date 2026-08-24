package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskExecuteSigningKey(t *testing.T) {
	t.Run("is not the secret key itself", func(t *testing.T) {
		key := TaskExecuteSigningKey("installation-secret")
		assert.NotEqual(t, []byte("installation-secret"), key)
		assert.Len(t, key, sha256.Size)
	})

	t.Run("is domain-separated from the usage key", func(t *testing.T) {
		// Both derive from SECRET_KEY. A leaked usage key must not sign dispatches.
		assert.NotEqual(t, UsageSigningKey("installation-secret"), TaskExecuteSigningKey("installation-secret"))
	})

	t.Run("differs per installation", func(t *testing.T) {
		assert.NotEqual(t, TaskExecuteSigningKey("a"), TaskExecuteSigningKey("b"))
	})
}

func TestSignTaskExecuteRequest(t *testing.T) {
	key := TaskExecuteSigningKey("installation-secret")
	body := []byte(`{"workspace_id":"ws-1","id":"task-1"}`)

	t.Run("signs timestamp, path and body digest", func(t *testing.T) {
		signature := SignTaskExecuteRequest(key, 1700000000, "/api/tasks.execute", body)

		version, encoded, found := strings.Cut(signature, ",")
		require.True(t, found)
		assert.Equal(t, "v1", version)

		raw, err := base64.StdEncoding.DecodeString(encoded)
		require.NoError(t, err)
		assert.Len(t, raw, sha256.Size)
	})

	t.Run("the body digest is what changes with the task id", func(t *testing.T) {
		other := []byte(`{"workspace_id":"ws-1","id":"task-2"}`)
		assert.NotEqual(t,
			SignTaskExecuteRequest(key, 1700000000, "/api/tasks.execute", body),
			SignTaskExecuteRequest(key, 1700000000, "/api/tasks.execute", other))
	})

	t.Run("the signed content is {timestamp}.{path}.{sha256(body)}", func(t *testing.T) {
		digest := sha256.Sum256(body)

		// The usage signer signs `{timestamp}.{path}`. Handing it the path with
		// the body digest appended reproduces this signature byte for byte —
		// same shape, one more field in the content.
		assert.Equal(t,
			SignUsageRequest(key, 1700000000, "/api/tasks.execute."+hex.EncodeToString(digest[:])),
			SignTaskExecuteRequest(key, 1700000000, "/api/tasks.execute", body))
	})
}

func TestVerifyTaskExecuteSignature(t *testing.T) {
	key := TaskExecuteSigningKey("installation-secret")
	body := []byte(`{"workspace_id":"ws-1","id":"task-1"}`)
	now := time.Unix(1700000000, 0).UTC()
	signature := SignTaskExecuteRequest(key, now.Unix(), "/api/tasks.execute", body)

	t.Run("accepts a fresh, matching signature", func(t *testing.T) {
		assert.NoError(t, VerifyTaskExecuteSignature(key, now.Unix(), "/api/tasks.execute", body, signature, now))
	})

	t.Run("accepts clock skew inside the window, in both directions", func(t *testing.T) {
		assert.NoError(t, VerifyTaskExecuteSignature(key, now.Unix(), "/api/tasks.execute", body, signature,
			now.Add(TaskExecuteSignatureMaxSkew-time.Second)))
		assert.NoError(t, VerifyTaskExecuteSignature(key, now.Unix(), "/api/tasks.execute", body, signature,
			now.Add(-TaskExecuteSignatureMaxSkew+time.Second)))
	})

	t.Run("rejects a stale timestamp", func(t *testing.T) {
		assert.Error(t, VerifyTaskExecuteSignature(key, now.Unix(), "/api/tasks.execute", body, signature,
			now.Add(TaskExecuteSignatureMaxSkew+time.Second)))
	})

	t.Run("rejects a timestamp edited to extend the window", func(t *testing.T) {
		later := now.Add(TaskExecuteSignatureMaxSkew + time.Hour)
		assert.Error(t, VerifyTaskExecuteSignature(key, later.Unix(), "/api/tasks.execute", body, signature, later))
	})

	t.Run("rejects a signature captured for another body", func(t *testing.T) {
		// The failure this shape exists for: with a bare-path signature, this
		// would be accepted for the whole skew window, for any task id.
		other := []byte(`{"workspace_id":"ws-1","id":"task-2"}`)
		assert.Error(t, VerifyTaskExecuteSignature(key, now.Unix(), "/api/tasks.execute", other, signature, now))
	})

	t.Run("rejects a signature made for another path", func(t *testing.T) {
		assert.Error(t, VerifyTaskExecuteSignature(key, now.Unix(), "/api/cron", body, signature, now))
	})

	t.Run("rejects another installation's key", func(t *testing.T) {
		assert.Error(t, VerifyTaskExecuteSignature(TaskExecuteSigningKey("other-secret"),
			now.Unix(), "/api/tasks.execute", body, signature, now))
	})

	t.Run("rejects an empty signature", func(t *testing.T) {
		assert.Error(t, VerifyTaskExecuteSignature(key, now.Unix(), "/api/tasks.execute", body, "", now))
	})

	t.Run("rejects a signature under a different version prefix", func(t *testing.T) {
		_, encoded, _ := strings.Cut(signature, ",")
		assert.Error(t, VerifyTaskExecuteSignature(key, now.Unix(), "/api/tasks.execute", body, "v2,"+encoded, now))
	})

	t.Run("does not distinguish its failures", func(t *testing.T) {
		stale := VerifyTaskExecuteSignature(key, now.Unix(), "/api/tasks.execute", body, signature,
			now.Add(TaskExecuteSignatureMaxSkew+time.Second))
		wrong := VerifyTaskExecuteSignature(key, now.Unix(), "/api/tasks.execute", body, "v1,bogus", now)
		require.Error(t, stale)
		require.Error(t, wrong)
		assert.Equal(t, stale.Error(), wrong.Error())
	})
}

func TestParseTaskExecuteTimestamp(t *testing.T) {
	t.Run("reads a Unix seconds header", func(t *testing.T) {
		ts, err := ParseTaskExecuteTimestamp(" 1700000000 ")
		assert.NoError(t, err)
		assert.Equal(t, int64(1700000000), ts)
	})

	t.Run("rejects a malformed header", func(t *testing.T) {
		for _, raw := range []string{"", "not-a-number", "1700000000.5"} {
			_, err := ParseTaskExecuteTimestamp(raw)
			assert.Error(t, err, raw)
		}
	})
}
