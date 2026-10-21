package config

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// formerComposeDefault is the SECRET_KEY value compose.yaml shipped as its default
// before 42.0, and formerEnvExamplePlaceholder the one env.example carried. Notifuse
// must refuse to sign with either.
const (
	formerComposeDefault        = "d04zCk3Fa45oOjDWHpAvc1AZxnLdGffOnNWK+Jt2yXf37+FTfuMMHb8flcfPMqLluRR3rvhbr555r6j1DEigrA=="
	formerEnvExamplePlaceholder = "your-secret-key-here"
)

func freshBase64Key(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(b)
}

// setKeyEnv sets the key variables for one test; an empty value leaves a variable
// effectively unset.
func setKeyEnv(t *testing.T, secretKey, pasetoKey, signingKey string) {
	t.Helper()
	t.Setenv("SECRET_KEY", secretKey)
	t.Setenv("PASETO_PRIVATE_KEY", pasetoKey)
	t.Setenv("SIGNING_KEY", signingKey)
}

func TestLoad_SigningKey(t *testing.T) {
	t.Run("former compose default as SECRET_KEY without SIGNING_KEY is refused", func(t *testing.T) {
		setKeyEnv(t, formerComposeDefault, "", "")

		_, err := LoadWithOptions(LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SIGNING_KEY")
		assert.Contains(t, err.Error(), "openssl rand -base64 32")
		assert.Contains(t, err.Error(), "keep SECRET_KEY")
		assert.Contains(t, err.Error(), "pass it to the container as SIGNING_KEY")
	})

	t.Run("former compose default through PASETO_PRIVATE_KEY is refused", func(t *testing.T) {
		setKeyEnv(t, "", formerComposeDefault, "")

		_, err := LoadWithOptions(LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SIGNING_KEY must be set")
		// Names the variable the operator actually set.
		assert.Contains(t, err.Error(), "PASETO_PRIVATE_KEY is a value published")
		assert.Contains(t, err.Error(), "keep PASETO_PRIVATE_KEY unchanged")
	})

	t.Run("former compose default with surrounding whitespace is refused", func(t *testing.T) {
		setKeyEnv(t, "  "+formerComposeDefault+"\n", "", "")

		_, err := LoadWithOptions(LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SIGNING_KEY")
	})

	t.Run("former env.example placeholder as SECRET_KEY without SIGNING_KEY is refused", func(t *testing.T) {
		setKeyEnv(t, formerEnvExamplePlaceholder, "", "")

		_, err := LoadWithOptions(LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SIGNING_KEY must be set")
	})

	t.Run("former compose default as SIGNING_KEY is refused", func(t *testing.T) {
		setKeyEnv(t, "another-secret-key-0123456789abcdef", "", formerComposeDefault)

		_, err := LoadWithOptions(LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SIGNING_KEY")
		assert.Contains(t, err.Error(), "openssl rand -base64 32")
	})

	t.Run("former compose default as SECRET_KEY with a fresh SIGNING_KEY loads", func(t *testing.T) {
		signingKey := freshBase64Key(t)
		setKeyEnv(t, formerComposeDefault, "", signingKey)

		cfg, err := LoadWithOptions(LoadOptions{})
		require.NoError(t, err)

		// SECRET_KEY still decrypts stored data, so it is kept exactly as given.
		assert.Equal(t, formerComposeDefault, cfg.Security.SecretKey)
		assert.Equal(t, signingKey, cfg.Security.SigningKey)
		decoded, err := base64.StdEncoding.DecodeString(signingKey)
		require.NoError(t, err)
		assert.Equal(t, decoded, cfg.Security.JWTSecret)
	})

	t.Run("SIGNING_KEY is trimmed", func(t *testing.T) {
		signingKey := freshBase64Key(t)
		setKeyEnv(t, "another-secret-key-0123456789abcdef", "", " "+signingKey+"\n")

		cfg, err := LoadWithOptions(LoadOptions{})
		require.NoError(t, err)
		assert.Equal(t, signingKey, cfg.Security.SigningKey)
	})

	t.Run("whitespace-only SIGNING_KEY counts as unset", func(t *testing.T) {
		setKeyEnv(t, "another-secret-key-0123456789abcdef", "", " \n\t")

		cfg, err := LoadWithOptions(LoadOptions{})
		require.NoError(t, err)
		assert.Equal(t, "another-secret-key-0123456789abcdef", cfg.Security.SigningKey)
	})

	t.Run("without SIGNING_KEY a raw SECRET_KEY signs exactly as before", func(t *testing.T) {
		secretKey := " raw-secret-key-0123456789abcdefghij "
		setKeyEnv(t, secretKey, "", "")

		cfg, err := LoadWithOptions(LoadOptions{})
		require.NoError(t, err)
		assert.Equal(t, secretKey, cfg.Security.SecretKey)
		assert.Equal(t, secretKey, cfg.Security.SigningKey, "an unset SIGNING_KEY must reuse SECRET_KEY byte for byte")
		assert.Equal(t, []byte(secretKey), cfg.Security.JWTSecret)
	})

	t.Run("without SIGNING_KEY a base64 SECRET_KEY signs exactly as before", func(t *testing.T) {
		secretKey := freshBase64Key(t)
		setKeyEnv(t, secretKey, "", "")

		cfg, err := LoadWithOptions(LoadOptions{})
		require.NoError(t, err)
		assert.Equal(t, secretKey, cfg.Security.SigningKey)
		decoded, err := base64.StdEncoding.DecodeString(secretKey)
		require.NoError(t, err)
		assert.Equal(t, decoded, cfg.Security.JWTSecret)
	})
}

func TestIsPublishedKey(t *testing.T) {
	decoded, err := base64.StdEncoding.DecodeString(formerComposeDefault)
	require.NoError(t, err)

	wrapped := formerComposeDefault[:40] + "\n" + formerComposeDefault[40:76] + "\r\n" + formerComposeDefault[76:]

	recognised := map[string]string{
		"as shipped":              formerComposeDefault,
		"surrounding whitespace":  "\t" + formerComposeDefault + " \n",
		"without padding":         strings.TrimRight(formerComposeDefault, "="),
		"url alphabet":            base64.URLEncoding.EncodeToString(decoded),
		"url alphabet unpadded":   base64.RawURLEncoding.EncodeToString(decoded),
		"wrapped over lines":      wrapped,
		"raw decoded bytes":       string(decoded),
		"env.example placeholder": formerEnvExamplePlaceholder,
	}
	for name, key := range recognised {
		t.Run("recognises "+name, func(t *testing.T) {
			assert.True(t, isPublishedKey(key))
		})
	}

	// One character changed decodes to different bytes.
	altered := "e" + formerComposeDefault[1:]

	others := map[string]string{
		"empty":                 "",
		"fresh base64 key":      freshBase64Key(t),
		"one character changed": altered,
		"raw string key":        "another-secret-key-0123456789abcdef",
	}
	for name, key := range others {
		t.Run("ignores "+name, func(t *testing.T) {
			assert.False(t, isPublishedKey(key))
		})
	}
}
