package crypto

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	reencryptCurrentKey = "current-key-for-reencrypt-tests-32+"
	reencryptOldKey     = "old-key-for-reencrypt-tests-32-bytes"
	reencryptOlderKey   = "older-key-for-reencrypt-tests-32bytes"
)

func mustEncrypt(t *testing.T, plaintext, key string) string {
	t.Helper()
	enc, err := EncryptString(plaintext, key)
	require.NoError(t, err)
	return enc
}

// opensWith returns the plaintext under key, or "" and false.
func opensWith(value, key string) (string, bool) {
	plain, err := DecryptFromHexString(value, key)
	return plain, err == nil
}

func TestDecryptWithKeys(t *testing.T) {
	t.Run("returns the plaintext and the position of the key that opens the value", func(t *testing.T) {
		value := mustEncrypt(t, "secret", reencryptOldKey)

		plaintext, index, err := DecryptWithKeys(value, []string{reencryptCurrentKey, reencryptOldKey})

		require.NoError(t, err)
		assert.Equal(t, "secret", plaintext)
		assert.Equal(t, 1, index)
	})

	t.Run("refuses a value none of the keys opens", func(t *testing.T) {
		value := mustEncrypt(t, "secret", reencryptOlderKey)

		_, _, err := DecryptWithKeys(value, []string{reencryptCurrentKey, reencryptOldKey})

		assert.True(t, errors.Is(err, ErrUndecryptable))
	})
}

func TestReencryptValue(t *testing.T) {
	t.Run("a value under the current key is left as it is", func(t *testing.T) {
		value := mustEncrypt(t, "secret", reencryptCurrentKey)

		got, changed, err := ReencryptValue(value, reencryptCurrentKey, []string{reencryptOldKey})

		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, value, got)
	})

	t.Run("a value under a previous key comes back under the current key only", func(t *testing.T) {
		value := mustEncrypt(t, "secret", reencryptOlderKey)

		got, changed, err := ReencryptValue(value, reencryptCurrentKey, []string{reencryptOldKey, reencryptOlderKey})

		require.NoError(t, err)
		assert.True(t, changed)
		plain, ok := opensWith(got, reencryptCurrentKey)
		require.True(t, ok)
		assert.Equal(t, "secret", plain)
		_, ok = opensWith(got, reencryptOlderKey)
		assert.False(t, ok)
	})

	t.Run("a value no key opens is refused", func(t *testing.T) {
		value := mustEncrypt(t, "secret", "some-other-key-nobody-listed-32bytes")

		_, _, err := ReencryptValue(value, reencryptCurrentKey, []string{reencryptOldKey})

		assert.True(t, errors.Is(err, ErrUndecryptable))
	})

	t.Run("without previous keys, a value under another key is refused", func(t *testing.T) {
		value := mustEncrypt(t, "secret", reencryptOldKey)

		_, _, err := ReencryptValue(value, reencryptCurrentKey, nil)

		assert.True(t, errors.Is(err, ErrUndecryptable))
	})

	t.Run("a value that is no ciphertext is refused", func(t *testing.T) {
		_, _, err := ReencryptValue("not-hex-at-all", reencryptCurrentKey, []string{reencryptOldKey})

		assert.True(t, errors.Is(err, ErrUndecryptable))
	})
}

// The paths a test document's encrypted values sit at, as the domain gives them.
var (
	settingsTestPaths     = []string{"encrypted_secret_key", "file_manager.encrypted_secret_key"}
	integrationsTestPaths = []string{
		"[].email_provider.smtp.encrypted_username",
		"[].email_provider.smtp.encrypted_password",
		"[].llm_provider.anthropic.encrypted_api_key",
	}
)

func TestReencryptFields(t *testing.T) {
	t.Run("a document whose values open with the current key comes back byte for byte", func(t *testing.T) {
		doc := []byte(fmt.Sprintf(`{"name": "x",  "encrypted_secret_key": %q}`,
			mustEncrypt(t, "workspace-key", reencryptCurrentKey)))

		got, changed, err := ReencryptFields(doc, settingsTestPaths, reencryptCurrentKey, []string{reencryptOldKey})

		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, doc, got)
	})

	t.Run("values under a previous key are re-encrypted at every listed path", func(t *testing.T) {
		doc := []byte(fmt.Sprintf(`[
			{"id": "smtp", "email_provider": {"kind": "smtp", "smtp": {
				"host": "smtp.example.com",
				"encrypted_username": %q,
				"encrypted_password": %q}}},
			{"id": "llm", "llm_provider": {"anthropic": {"encrypted_api_key": %q}}}
		]`,
			mustEncrypt(t, "user", reencryptOldKey),
			mustEncrypt(t, "pass", reencryptOldKey),
			mustEncrypt(t, "sk-ant", reencryptCurrentKey)))

		got, changed, err := ReencryptFields(doc, integrationsTestPaths, reencryptCurrentKey, []string{reencryptOldKey})

		require.NoError(t, err)
		assert.True(t, changed)
		var out []map[string]any
		require.NoError(t, json.Unmarshal(got, &out))
		smtp := out[0]["email_provider"].(map[string]any)["smtp"].(map[string]any)
		assert.Equal(t, "smtp.example.com", smtp["host"])
		for field, want := range map[string]string{"encrypted_username": "user", "encrypted_password": "pass"} {
			plain, ok := opensWith(smtp[field].(string), reencryptCurrentKey)
			require.True(t, ok, field)
			assert.Equal(t, want, plain, field)
			_, ok = opensWith(smtp[field].(string), reencryptOldKey)
			assert.False(t, ok, field)
		}
		apiKey := out[1]["llm_provider"].(map[string]any)["anthropic"].(map[string]any)["encrypted_api_key"].(string)
		plain, ok := opensWith(apiKey, reencryptCurrentKey)
		require.True(t, ok)
		assert.Equal(t, "sk-ant", plain)
	})

	t.Run("an array inside an object is walked element by element", func(t *testing.T) {
		doc := []byte(fmt.Sprintf(`{"items": [{"encrypted_api_key": %q}, {"encrypted_api_key": ""}]}`,
			mustEncrypt(t, "k", reencryptOldKey)))

		got, changed, err := ReencryptFields(doc, []string{"items.[].encrypted_api_key"}, reencryptCurrentKey, []string{reencryptOldKey})

		require.NoError(t, err)
		assert.True(t, changed)
		var out struct {
			Items []map[string]string `json:"items"`
		}
		require.NoError(t, json.Unmarshal(got, &out))
		plain, ok := opensWith(out.Items[0]["encrypted_api_key"], reencryptCurrentKey)
		require.True(t, ok)
		assert.Equal(t, "k", plain)
		assert.Equal(t, "", out.Items[1]["encrypted_api_key"])
	})

	t.Run("values off the listed paths are left alone, whatever their key is named", func(t *testing.T) {
		doc := []byte(fmt.Sprintf(`{
			"encrypted_secret_key": %q,
			"custom_field_labels": {"encrypted_note": "My label"},
			"template_blocks": [{"block": {"attributes": {"encrypted_ref": "abc"}}}],
			"file_manager": {"bucket": "b", "encrypted_other": "not ours"}
		}`, mustEncrypt(t, "k", reencryptCurrentKey)))

		got, changed, err := ReencryptFields(doc, settingsTestPaths, reencryptCurrentKey, nil)

		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, doc, got)
	})

	t.Run("empty strings and values that are not strings are left alone", func(t *testing.T) {
		doc := []byte(fmt.Sprintf(`{
			"encrypted_secret_key": "",
			"file_manager": {"encrypted_secret_key": %q}
		}`, mustEncrypt(t, "k", reencryptOldKey)))

		got, changed, err := ReencryptFields(doc, settingsTestPaths, reencryptCurrentKey, []string{reencryptOldKey})

		require.NoError(t, err)
		assert.True(t, changed)
		var out map[string]any
		require.NoError(t, json.Unmarshal(got, &out))
		assert.Equal(t, "", out["encrypted_secret_key"])

		for _, doc := range []string{`{"encrypted_secret_key": true}`, `{"encrypted_secret_key": 3}`, `{"file_manager": null}`} {
			got, changed, err := ReencryptFields([]byte(doc), settingsTestPaths, reencryptCurrentKey, nil)
			require.NoError(t, err, doc)
			assert.False(t, changed, doc)
			assert.Equal(t, []byte(doc), got, doc)
		}
	})

	t.Run("numbers are kept as written", func(t *testing.T) {
		doc := []byte(fmt.Sprintf(`{"big": 12345678901234567890, "ratio": 0.1, "encrypted_secret_key": %q}`,
			mustEncrypt(t, "k", reencryptOldKey)))

		got, changed, err := ReencryptFields(doc, settingsTestPaths, reencryptCurrentKey, []string{reencryptOldKey})

		require.NoError(t, err)
		assert.True(t, changed)
		assert.Contains(t, string(got), `"big":12345678901234567890`)
		assert.Contains(t, string(got), `"ratio":0.1`)
	})

	t.Run("a value no key opens fails with its path and changes nothing", func(t *testing.T) {
		doc := []byte(fmt.Sprintf(`[{"email_provider": {"smtp": {"encrypted_password": %q}}}]`,
			mustEncrypt(t, "pass", "a-key-nobody-listed-for-this-test-32b")))

		got, _, err := ReencryptFields(doc, integrationsTestPaths, reencryptCurrentKey, []string{reencryptOldKey})

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUndecryptable))
		assert.Contains(t, err.Error(), "[0].email_provider.smtp.encrypted_password")
		assert.Nil(t, got)
	})

	t.Run("without previous keys it only checks: a value under another key fails", func(t *testing.T) {
		doc := []byte(fmt.Sprintf(`{"encrypted_secret_key": %q}`, mustEncrypt(t, "k", reencryptOldKey)))

		_, _, err := ReencryptFields(doc, settingsTestPaths, reencryptCurrentKey, nil)

		assert.True(t, errors.Is(err, ErrUndecryptable))
	})

	t.Run("an empty or null document is left alone", func(t *testing.T) {
		for _, doc := range [][]byte{nil, []byte("null"), []byte("[]")} {
			got, changed, err := ReencryptFields(doc, integrationsTestPaths, reencryptCurrentKey, []string{reencryptOldKey})

			require.NoError(t, err, string(doc))
			assert.False(t, changed, string(doc))
			assert.Equal(t, doc, got, string(doc))
		}
	})

	t.Run("a document that is not JSON is refused", func(t *testing.T) {
		_, _, err := ReencryptFields([]byte("{not json"), settingsTestPaths, reencryptCurrentKey, nil)

		assert.Error(t, err)
	})
}
