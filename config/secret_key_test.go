package config

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/pkg/crypto"
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
func setKeyEnv(t *testing.T, secretKey, pasetoKey string) {
	t.Helper()
	t.Setenv("SECRET_KEY", secretKey)
	t.Setenv("PASETO_PRIVATE_KEY", pasetoKey)
}

func TestParseSecretKeys(t *testing.T) {
	newKey := freshBase64Key(t)
	oldKey := freshBase64Key(t)

	t.Run("one key without a comma is read byte for byte and is not a list", func(t *testing.T) {
		raw := ` "` + newKey + `" `

		keys, err := parseSecretKeys(raw, "SECRET_KEY")

		require.NoError(t, err)
		assert.Equal(t, raw, keys.current, "quotes and spaces stay part of a single key, as before")
		assert.Empty(t, keys.previous)
		assert.False(t, keys.isList)
	})

	t.Run("a list gives the first key and the previous ones with their quoted forms", func(t *testing.T) {
		keys, err := parseSecretKeys(newKey+","+oldKey, "SECRET_KEY")

		require.NoError(t, err)
		assert.Equal(t, newKey, keys.current)
		assert.Equal(t, []string{oldKey, `"` + oldKey + `"`, "'" + oldKey + "'"}, keys.previous)
		assert.True(t, keys.isList)
	})

	t.Run("quotes around the whole list are removed", func(t *testing.T) {
		keys, err := parseSecretKeys(`"`+newKey+","+oldKey+`"`, "SECRET_KEY")

		require.NoError(t, err)
		assert.Equal(t, newKey, keys.current)
		assert.Contains(t, keys.previous, oldKey)
	})

	t.Run("spaces around each key are trimmed", func(t *testing.T) {
		keys, err := parseSecretKeys(" "+newKey+" ,\t"+oldKey+" \n", "SECRET_KEY")

		require.NoError(t, err)
		assert.Equal(t, newKey, keys.current)
		assert.Contains(t, keys.previous, oldKey)
	})

	t.Run("two commas stand for one comma inside a key", func(t *testing.T) {
		keys, err := parseSecretKeys(newKey+",legacy,,pass,,phrase", "SECRET_KEY")

		require.NoError(t, err)
		assert.Equal(t, newKey, keys.current)
		assert.Contains(t, keys.previous, "legacy,pass,phrase")
	})

	t.Run("four commas stand for two commas inside a key", func(t *testing.T) {
		keys, err := parseSecretKeys(newKey+",legacy,,,,pass", "SECRET_KEY")

		require.NoError(t, err)
		assert.Contains(t, keys.previous, "legacy,,pass")
	})

	// <new>,,,legacy is <new>, then legacy, or <new> then ,legacy: a key that starts or
	// ends with a comma beside a separator cannot be told from the other reading.
	for name, raw := range map[string]string{
		"three commas after the first key": newKey + ",,,legacy-passphrase",
		"three commas between previous":    newKey + ",legacy,,," + oldKey,
		"five commas":                      newKey + ",,,,,legacy-passphrase",
		"three commas in a quoted list":    `"` + newKey + `,,,legacy-passphrase"`,
	} {
		t.Run("an odd run of three or more commas is refused, saying to quote the key: "+name, func(t *testing.T) {
			_, err := parseSecretKeys(raw, "SECRET_KEY")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "SECRET_KEY has three or more commas in a row")
			assert.Contains(t, err.Error(), `<new key>,",,<key>"`)
		})
	}

	t.Run("a previous key that starts with a comma is read with it when given in quotes", func(t *testing.T) {
		keys, err := parseSecretKeys(newKey+`,",,legacy-passphrase"`, "SECRET_KEY")

		require.NoError(t, err)
		assert.Equal(t, newKey, keys.current)
		assert.Contains(t, keys.previous, ",legacy-passphrase")
	})

	t.Run("a single key whose commas are doubled is a list of one", func(t *testing.T) {
		keys, err := parseSecretKeys(newKey+",,suffix", "SECRET_KEY")

		require.NoError(t, err)
		assert.Equal(t, newKey+",suffix", keys.current)
		assert.Empty(t, keys.previous)
		assert.True(t, keys.isList, "a list of one still gets the startup check")
	})

	t.Run("a previous key given in quotes is also tried without them", func(t *testing.T) {
		keys, err := parseSecretKeys(newKey+`,"`+oldKey+`"`, "SECRET_KEY")

		require.NoError(t, err)
		assert.Contains(t, keys.previous, `"`+oldKey+`"`)
		assert.Contains(t, keys.previous, oldKey)
		assert.Contains(t, keys.previous, "'"+oldKey+"'")
	})

	t.Run("a repeated key is listed once and the current key is never a previous one", func(t *testing.T) {
		keys, err := parseSecretKeys(newKey+","+oldKey+","+oldKey+","+newKey, "SECRET_KEY")

		require.NoError(t, err)
		assert.NotContains(t, keys.previous, newKey)
		assert.Contains(t, keys.previous, oldKey)
		seen := map[string]bool{}
		for _, key := range keys.previous {
			assert.False(t, seen[key], "%q listed twice", key)
			seen[key] = true
		}
	})

	// docker --env-file kept the quotes of SECRET_KEY="K", so data may be under "K" with
	// its quotes, while the same K now comes through without them.
	t.Run("the current key in quotes, listed after it, is tried as a previous key", func(t *testing.T) {
		keys, err := parseSecretKeys(newKey+`,"`+newKey+`"`, "SECRET_KEY")

		require.NoError(t, err)
		assert.Equal(t, newKey, keys.current)
		assert.Contains(t, keys.previous, `"`+newKey+`"`)
		assert.NotContains(t, keys.previous, newKey)
	})

	// What the startup error says to set when the data opens only with the key in quotes.
	t.Run("the current key listed twice is tried in quotes as a previous key", func(t *testing.T) {
		keys, err := parseSecretKeys(newKey+","+newKey, "SECRET_KEY")

		require.NoError(t, err)
		assert.True(t, keys.isList)
		assert.Equal(t, newKey, keys.current)
		assert.Contains(t, keys.previous, `"`+newKey+`"`)
		assert.Contains(t, keys.previous, "'"+newKey+"'")
		assert.NotContains(t, keys.previous, newKey)
	})

	// A key read from a file often ends with a newline, and its data was encrypted with
	// it: a previous key is also tried exactly as written.
	t.Run("a previous key is also tried with the spaces written around it", func(t *testing.T) {
		keys, err := parseSecretKeys(newKey+","+oldKey+"\n", "SECRET_KEY")

		require.NoError(t, err)
		assert.Equal(t, oldKey, keys.previous[0], "the trimmed key is tried first")
		assert.Contains(t, keys.previous, oldKey+"\n")

		keys, err = parseSecretKeys(newKey+", "+oldKey+" ,"+freshBase64Key(t), "SECRET_KEY")

		require.NoError(t, err)
		assert.Contains(t, keys.previous, " "+oldKey+" ")
	})

	for name, raw := range map[string]string{
		"an empty key between two others": newKey + ", ," + oldKey,
		"a trailing comma":                newKey + "," + oldKey + ",",
		"a leading comma":                 "," + newKey,
	} {
		t.Run(name+" is refused", func(t *testing.T) {
			_, err := parseSecretKeys(raw, "SECRET_KEY")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "empty key")
		})
	}

	for name, raw := range map[string]string{
		"each key in its own quotes": `"` + newKey + `","` + oldKey + `"`,
		"a stray quote":              newKey + `',` + oldKey,
	} {
		t.Run("a first key starting or ending with a quote is refused: "+name, func(t *testing.T) {
			_, err := parseSecretKeys(raw, "SECRET_KEY")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "quote")
		})
	}

	t.Run("a published first key is refused with the rotation to apply", func(t *testing.T) {
		for _, raw := range []string{formerComposeDefault, formerComposeDefault + "," + oldKey, formerEnvExamplePlaceholder} {
			_, err := parseSecretKeys(raw, "SECRET_KEY")

			require.Error(t, err, raw)
			assert.Contains(t, err.Error(), "published with Notifuse")
			assert.Contains(t, err.Error(), "openssl rand -base64 32")
			assert.Contains(t, err.Error(), "SECRET_KEY=<new key>,<current key>")
			assert.Contains(t, err.Error(), "API keys must be deleted and created again")
		}
	})

	t.Run("a published previous key is accepted: that is how an installation leaves it", func(t *testing.T) {
		keys, err := parseSecretKeys(newKey+","+formerComposeDefault, "SECRET_KEY")

		require.NoError(t, err)
		assert.Equal(t, newKey, keys.current)
		assert.Contains(t, keys.previous, formerComposeDefault)
	})

	t.Run("a first key under 32 bytes of key material is refused with the rotation to apply", func(t *testing.T) {
		for _, raw := range []string{"your-secret-key", "abc123#xyz789", strings.Repeat("k", 31), strings.Repeat("k", 31) + "," + oldKey} {
			_, err := parseSecretKeys(raw, "SECRET_KEY")

			require.Error(t, err, raw)
			assert.Contains(t, err.Error(), "at least 32 bytes")
			assert.Contains(t, err.Error(), "SECRET_KEY=<new key>,<current key>")
		}
	})

	t.Run("a first key of exactly 32 bytes is accepted", func(t *testing.T) {
		keys, err := parseSecretKeys(strings.Repeat("k", 32), "SECRET_KEY")

		require.NoError(t, err)
		assert.Equal(t, strings.Repeat("k", 32), keys.current)
	})

	t.Run("a legacy key containing a comma is refused, with how to write its commas", func(t *testing.T) {
		_, err := parseSecretKeys("my passphrase, with a comma, long enough overall", "SECRET_KEY")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "twice")
	})

	t.Run("errors name the variable the value came from", func(t *testing.T) {
		_, err := parseSecretKeys("short", "PASETO_PRIVATE_KEY")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "PASETO_PRIVATE_KEY=<new key>,<current key>")
	})
}

// A list reads its first key without the quotes and spaces around it, where a single
// key keeps them: unquoted gives the form a list would read.
func TestUnquoted(t *testing.T) {
	for raw, want := range map[string]string{
		"key":              "key",
		" key\n":           "key",
		`"key"`:            "key",
		"'key'":            "key",
		` " key " ` + "\n": "key",
		`"key`:             `"key`,
	} {
		assert.Equal(t, want, unquoted(raw), "%q", raw)
	}
}

func TestNewSecurityConfig(t *testing.T) {
	newKey, oldKey := "a-new-raw-key-giving-32-bytes-of-material", freshBase64Key(t)

	t.Run("signs and encrypts with the first key of a list, and decrypts with all of them", func(t *testing.T) {
		security, err := NewSecurityConfig(`"`+newKey+","+oldKey+`"`, "SECRET_KEY")
		require.NoError(t, err)
		assert.Equal(t, newKey, security.SecretKey)
		assert.Equal(t, []byte(newKey), security.JWTSecret)
		assert.True(t, security.SecretKeyIsList)
		assert.Contains(t, security.PreviousSecretKeys, oldKey)
		assert.Equal(t, security.SecretKey, security.DecryptionKeys()[0])
		assert.Equal(t, security.PreviousSecretKeys, security.DecryptionKeys()[1:])
		assert.Equal(t, "SECRET_KEY", security.KeyVariable)
	})

	t.Run("records the variable the keys came from", func(t *testing.T) {
		security, err := NewSecurityConfig(newKey+","+oldKey, "PASETO_PRIVATE_KEY")
		require.NoError(t, err)
		assert.Equal(t, "PASETO_PRIVATE_KEY", security.KeyVariable)
	})

	t.Run("a single key is used byte for byte", func(t *testing.T) {
		security, err := NewSecurityConfig(" "+newKey, "SECRET_KEY")
		require.NoError(t, err)
		assert.Equal(t, " "+newKey, security.SecretKey)
		assert.Equal(t, []string{" " + newKey}, security.DecryptionKeys())
		assert.False(t, security.SecretKeyIsList)
	})

	t.Run("refuses what the server refuses, naming the variable", func(t *testing.T) {
		_, err := NewSecurityConfig(formerComposeDefault, "PASETO_PRIVATE_KEY")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "PASETO_PRIVATE_KEY")
	})
}

func TestLoad_SecretKeys(t *testing.T) {
	t.Run("former compose default as SECRET_KEY is refused with the rotation to apply", func(t *testing.T) {
		setKeyEnv(t, formerComposeDefault, "")

		_, err := LoadWithOptions(LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SECRET_KEY=<new key>,<current key>")
	})

	t.Run("former compose default through PASETO_PRIVATE_KEY names that variable", func(t *testing.T) {
		setKeyEnv(t, "", formerComposeDefault)

		_, err := LoadWithOptions(LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "PASETO_PRIVATE_KEY=<new key>,<current key>")
	})

	t.Run("former compose default with surrounding whitespace or in quotes is refused", func(t *testing.T) {
		for _, raw := range []string{"  " + formerComposeDefault + "\n", `"` + formerComposeDefault + `"`} {
			setKeyEnv(t, raw, "")

			_, err := LoadWithOptions(LoadOptions{})
			require.Error(t, err, raw)
			assert.Contains(t, err.Error(), "published with Notifuse")
		}
	})

	t.Run("rotating off the former compose default loads, and only the new key signs", func(t *testing.T) {
		newKey := freshBase64Key(t)
		setKeyEnv(t, newKey+","+formerComposeDefault, "")

		cfg, err := LoadWithOptions(LoadOptions{})
		require.NoError(t, err)
		assert.Equal(t, newKey, cfg.Security.SecretKey)
		assert.Contains(t, cfg.Security.PreviousSecretKeys, formerComposeDefault)
		assert.True(t, cfg.Security.SecretKeyIsList)
		decoded, err := base64.StdEncoding.DecodeString(newKey)
		require.NoError(t, err)
		assert.Equal(t, decoded, cfg.Security.JWTSecret)
	})

	t.Run("a raw SECRET_KEY signs exactly as before", func(t *testing.T) {
		secretKey := " raw-secret-key-0123456789abcdefghij "
		setKeyEnv(t, secretKey, "")

		cfg, err := LoadWithOptions(LoadOptions{})
		require.NoError(t, err)
		assert.Equal(t, secretKey, cfg.Security.SecretKey)
		assert.Empty(t, cfg.Security.PreviousSecretKeys)
		assert.False(t, cfg.Security.SecretKeyIsList)
		assert.Equal(t, []byte(secretKey), cfg.Security.JWTSecret)
	})

	t.Run("a base64 SECRET_KEY signs exactly as before", func(t *testing.T) {
		secretKey := freshBase64Key(t)
		setKeyEnv(t, secretKey, "")

		cfg, err := LoadWithOptions(LoadOptions{})
		require.NoError(t, err)
		decoded, err := base64.StdEncoding.DecodeString(secretKey)
		require.NoError(t, err)
		assert.Equal(t, decoded, cfg.Security.JWTSecret)
	})

	t.Run("a short SECRET_KEY is refused", func(t *testing.T) {
		setKeyEnv(t, "your-secret-key", "")

		_, err := LoadWithOptions(LoadOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least 32 bytes")
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
		// docker --env-file keeps quotes as part of the value.
		"double-quoted":                  `"` + formerComposeDefault + `"`,
		"single-quoted":                  "'" + formerComposeDefault + "'",
		"quoted with whitespace":         ` " ` + formerComposeDefault + ` " `,
		"quoted env.example placeholder": `"` + formerEnvExamplePlaceholder + `"`,
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
		"mismatched quotes":     `"` + formerComposeDefault + "'",
		"lone quote":            `"`,
	}
	for name, key := range others {
		t.Run("ignores "+name, func(t *testing.T) {
			assert.False(t, isPublishedKey(key))
		})
	}
}

func TestLoadSystemSettings_OpensSettingsUnderAPreviousKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	newKey, oldKey := freshBase64Key(t), freshBase64Key(t)
	password, err := crypto.EncryptString("smtp-password", oldKey)
	require.NoError(t, err)
	username, err := crypto.EncryptString("smtp-user", newKey)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT key, value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
		AddRow("is_installed", "true").
		AddRow("encrypted_smtp_password", password).
		AddRow("encrypted_smtp_username", username))

	settings, err := loadSystemSettings(db, []string{newKey, oldKey})

	require.NoError(t, err)
	assert.Equal(t, "smtp-password", settings.SMTPPassword)
	assert.Equal(t, "smtp-user", settings.SMTPUsername)
}

func TestLoadSystemSettings_NamesTheSettingsNoKeyOpens(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	key, other := freshBase64Key(t), freshBase64Key(t)
	password, err := crypto.EncryptString("smtp-password", key)
	require.NoError(t, err)
	secret, err := crypto.EncryptString("oidc-secret", other)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT key, value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
		AddRow("is_installed", "true").
		AddRow("encrypted_smtp_password", password).
		AddRow("encrypted_oidc_client_secret", secret).
		AddRow("encrypted_smtp_bridge_tls_key_base64", "not-a-ciphertext"))

	settings, err := loadSystemSettings(db, []string{key})

	require.NoError(t, err)
	assert.Equal(t, "smtp-password", settings.SMTPPassword)
	assert.Empty(t, settings.OIDCClientSecret)
	assert.ElementsMatch(t, []string{"encrypted_oidc_client_secret", "encrypted_smtp_bridge_tls_key_base64"}, settings.UnopenedSettings)
}

// A feature that is on and needs a stored setting no key opens would otherwise fail on
// its own terms, or, for the SMTP bridge outside production, start without TLS.
func TestCheckUnopenedSettings(t *testing.T) {
	unopened := &SystemSettings{UnopenedSettings: []string{
		"encrypted_oidc_client_secret", "encrypted_smtp_bridge_tls_cert_base64", "encrypted_smtp_bridge_tls_key_base64",
	}}
	secretKey := SecurityConfig{SecretKey: freshBase64Key(t), KeyVariable: "SECRET_KEY"}

	t.Run("single sign-on without its stored client secret is refused, naming it", func(t *testing.T) {
		err := checkUnopenedSettings(unopened, OIDCConfig{Enabled: true}, SMTPBridgeConfig{},
			SecurityConfig{SecretKey: freshBase64Key(t), KeyVariable: "PASETO_PRIVATE_KEY"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "encrypted_oidc_client_secret")
		assert.Contains(t, err.Error(), "PASETO_PRIVATE_KEY")
		assert.Contains(t, err.Error(), "OIDC_CLIENT_SECRET")
		assert.Contains(t, err.Error(), "former compose.yaml default")
		assert.NotContains(t, err.Error(), "smtp_bridge")
	})

	t.Run("the SMTP bridge without its stored TLS certificate and key is refused, naming them", func(t *testing.T) {
		err := checkUnopenedSettings(unopened, OIDCConfig{}, SMTPBridgeConfig{Enabled: true}, secretKey)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "encrypted_smtp_bridge_tls_cert_base64, encrypted_smtp_bridge_tls_key_base64")
		assert.Contains(t, err.Error(), "SMTP_BRIDGE_TLS=off")
	})

	t.Run("a part the environment gives is not named", func(t *testing.T) {
		err := checkUnopenedSettings(unopened, OIDCConfig{}, SMTPBridgeConfig{Enabled: true, TLSCertBase64: "cert"}, secretKey)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "encrypted_smtp_bridge_tls_key_base64")
		assert.NotContains(t, err.Error(), "encrypted_smtp_bridge_tls_cert_base64")
	})

	t.Run("values given in the environment need nothing stored", func(t *testing.T) {
		require.NoError(t, checkUnopenedSettings(unopened,
			OIDCConfig{Enabled: true, ClientSecret: "from-env"},
			SMTPBridgeConfig{Enabled: true, TLSCertBase64: "cert", TLSKeyBase64: "key"}, secretKey))
	})

	t.Run("an SMTP bridge with TLS set off needs no certificate", func(t *testing.T) {
		require.NoError(t, checkUnopenedSettings(unopened, OIDCConfig{}, SMTPBridgeConfig{Enabled: true, TLSMode: "off"}, secretKey))
	})

	t.Run("an SMTP bridge TLS mode that is not valid is left to its own check", func(t *testing.T) {
		require.NoError(t, checkUnopenedSettings(unopened, OIDCConfig{}, SMTPBridgeConfig{Enabled: true, TLSMode: "OFF"}, secretKey))
		require.Error(t, checkUnopenedSettings(unopened, OIDCConfig{}, SMTPBridgeConfig{Enabled: true, TLSMode: "starttls"}, secretKey),
			"a valid mode that needs TLS still needs the stored certificate and key")
	})

	t.Run("features that are off need nothing", func(t *testing.T) {
		require.NoError(t, checkUnopenedSettings(unopened, OIDCConfig{}, SMTPBridgeConfig{}, secretKey))
	})

	t.Run("settings that opened stop nothing", func(t *testing.T) {
		require.NoError(t, checkUnopenedSettings(&SystemSettings{}, OIDCConfig{Enabled: true}, SMTPBridgeConfig{Enabled: true}, secretKey))
		require.NoError(t, checkUnopenedSettings(nil, OIDCConfig{Enabled: true}, SMTPBridgeConfig{Enabled: true}, secretKey))
	})

	// Loading the settings ran before the startup check that gives these hints.
	t.Run("a single key whose stored setting opens with it in quotes says so", func(t *testing.T) {
		key := freshBase64Key(t)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		secret, err := crypto.EncryptString("oidc-secret", `"`+key+`"`)
		require.NoError(t, err)
		mock.ExpectQuery("SELECT key, value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
			AddRow("is_installed", "true").
			AddRow("encrypted_oidc_client_secret", secret))
		settings, err := loadSystemSettings(db, []string{key})
		require.NoError(t, err)

		err = checkUnopenedSettings(settings, OIDCConfig{Enabled: true}, SMTPBridgeConfig{},
			SecurityConfig{SecretKey: key, KeyVariable: "SECRET_KEY"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "SECRET_KEY opens none of the stored settings encrypted_oidc_client_secret")
		assert.Contains(t, err.Error(), "in quotes")
		assert.Contains(t, err.Error(), "SECRET_KEY=<key>,<key>")

		listed := SecurityConfig{SecretKey: key, KeyVariable: "SECRET_KEY", SecretKeyIsList: true}
		err = checkUnopenedSettings(settings, OIDCConfig{Enabled: true}, SMTPBridgeConfig{}, listed)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "in quotes", "a list already tries its keys in quotes")
	})
}

func TestCheckUnopenedSettings_KeyWrittenWithQuotes(t *testing.T) {
	key := freshBase64Key(t)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	secret, err := crypto.EncryptString("oidc-secret", key)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT key, value FROM settings").WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
		AddRow("is_installed", "true").
		AddRow("encrypted_oidc_client_secret", secret))
	quoted := `"` + key + `"`
	settings, err := loadSystemSettings(db, []string{quoted})
	require.NoError(t, err)

	err = checkUnopenedSettings(settings, OIDCConfig{Enabled: true}, SMTPBridgeConfig{},
		SecurityConfig{SecretKey: quoted, KeyVariable: "SECRET_KEY"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "SECRET_KEY opens none of the stored settings encrypted_oidc_client_secret, "+
		"which single sign-on (OIDC) needs, but does without the quotes or spaces around it: remove them from SECRET_KEY")
}

func TestKeyHint(t *testing.T) {
	const key = "the-key-as-the-environment-gives-it"
	opensOnly := func(want string) func(string) bool {
		return func(candidate string) bool { return candidate == want }
	}

	t.Run("a key in quotes that opens without them says to remove them", func(t *testing.T) {
		hint := KeyHint(`"`+key+"\"\n", "SECRET_KEY", opensOnly(key))
		assert.Contains(t, hint, "remove them from SECRET_KEY")
	})

	for name, quoted := range map[string]string{"double": `"` + key + `"`, "single": "'" + key + "'"} {
		t.Run("a key whose data is under it in "+name+" quotes says to list it twice", func(t *testing.T) {
			hint := KeyHint(key, "PASETO_PRIVATE_KEY", opensOnly(quoted))
			assert.Contains(t, hint, "the same key in quotes does")
			assert.Contains(t, hint, "PASETO_PRIVATE_KEY=<key>,<key>")
		})
	}

	t.Run("no hint when no form opens", func(t *testing.T) {
		assert.Empty(t, KeyHint(key, "SECRET_KEY", opensOnly("another-key")))
		assert.Empty(t, KeyHint(`"`+key+`"`, "SECRET_KEY", opensOnly("another-key")))
	})
}
