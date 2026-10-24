package domain

import (
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integrations.Scan must reject non-[]byte driver values (a driver returning a
// string would otherwise silently store garbage), map NULL to an empty list,
// and copy the driver buffer so later mutation cannot corrupt stored JSON.
func TestIntegrationsScanContract(t *testing.T) {
	t.Run("NULL becomes an empty, non-nil list", func(t *testing.T) {
		var ints Integrations
		require.NoError(t, ints.Scan(nil))
		assert.NotNil(t, ints)
		assert.Len(t, ints, 0)
	})

	t.Run("non-byte driver values are rejected", func(t *testing.T) {
		var ints Integrations
		err := ints.Scan("not bytes")
		require.EqualError(t, err, "type assertion to []byte failed")
	})

	t.Run("invalid JSON is an error, not a silent empty list", func(t *testing.T) {
		var ints Integrations
		assert.Error(t, ints.Scan([]byte("{oops")))
	})

	t.Run("valid JSON round-trips through Value", func(t *testing.T) {
		original := Integrations{{ID: "i1", Name: "Mail", Type: IntegrationTypeEmail}}
		v, err := original.Value()
		require.NoError(t, err)
		raw, ok := v.([]byte)
		require.True(t, ok, "Value must produce the []byte the driver stores")
		var back Integrations
		require.NoError(t, back.Scan(raw))
		require.Len(t, back, 1)
		assert.Equal(t, "i1", back[0].ID)
		assert.Equal(t, IntegrationTypeEmail, back[0].Type)
	})
}

// BeforeSave must never persist plaintext secrets: after it runs, the decoded
// fields are cleared and only encrypted forms remain — yet AfterLoad with the
// same passphrase restores them, and the wrong passphrase fails closed.
func TestIntegrationSecretLifecycle(t *testing.T) {
	const passphrase = "test-passphrase"
	newSMTPIntegration := func() *Integration {
		return &Integration{
			ID: "int-1", Name: "SMTP", Type: IntegrationTypeEmail,
			CredentialHints: map[string]string{"password": "cret"},
			EmailProvider: EmailProvider{
				Kind: EmailProviderKindSMTP,
				SMTP: &SMTPSettings{Host: "smtp.example.com", Port: 587, Username: "u", Password: "s3cret-pw"},
			},
		}
	}

	t.Run("BeforeSave clears plaintext and credential hints", func(t *testing.T) {
		i := newSMTPIntegration()
		require.NoError(t, i.BeforeSave(passphrase))
		assert.Empty(t, i.EmailProvider.SMTP.Password, "plaintext password must be wiped before storage")
		assert.NotEmpty(t, i.EmailProvider.SMTP.EncryptedPassword)
		assert.NotContains(t, i.EmailProvider.SMTP.EncryptedPassword, "s3cret-pw")
		assert.Nil(t, i.CredentialHints, "display-only hints must never be persisted")
	})

	t.Run("AfterLoad restores the secret with the right passphrase only", func(t *testing.T) {
		i := newSMTPIntegration()
		require.NoError(t, i.BeforeSave(passphrase))
		require.NoError(t, i.AfterLoad(passphrase))
		assert.Equal(t, "s3cret-pw", i.EmailProvider.SMTP.Password)

		j := newSMTPIntegration()
		require.NoError(t, j.BeforeSave(passphrase))
		err := j.AfterLoad("wrong-passphrase")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt integration provider secrets")
		assert.Empty(t, j.EmailProvider.SMTP.Password, "a failed decrypt must not leave a partial secret behind")
	})

	t.Run("Scan preserves the encrypted form through JSON storage", func(t *testing.T) {
		i := newSMTPIntegration()
		require.NoError(t, i.BeforeSave(passphrase))
		raw, err := json.Marshal(i)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "s3cret-pw")
		var back Integration
		require.NoError(t, back.Scan(driver.Value(raw)))
		require.NoError(t, back.AfterLoad(passphrase))
		assert.Equal(t, "s3cret-pw", back.EmailProvider.SMTP.Password)
	})

	t.Run("an email integration with no provider settings saves cleanly", func(t *testing.T) {
		i := &Integration{ID: "x", Name: "n", Type: IntegrationTypeEmail}
		require.NoError(t, i.BeforeSave(passphrase))
		require.NoError(t, i.AfterLoad(passphrase))
	})
}

// Integration.Validate dispatches per type and wraps provider errors with the
// integration kind; unknown types are rejected by name.
func TestIntegrationValidateDispatch(t *testing.T) {
	cases := []struct {
		name string
		i    Integration
		want string
	}{
		{"missing id", Integration{Name: "n", Type: IntegrationTypeEmail}, "integration id is required"},
		{"missing name", Integration{ID: "1", Type: IntegrationTypeEmail}, "integration name is required"},
		{"missing type", Integration{ID: "1", Name: "n"}, "integration type is required"},
		{"supabase without settings", Integration{ID: "1", Name: "n", Type: IntegrationTypeSupabase}, "supabase settings are required for supabase integration"},
		{"llm without settings", Integration{ID: "1", Name: "n", Type: IntegrationTypeLLM}, "llm provider settings are required for llm integration"},
		{"firecrawl without settings", Integration{ID: "1", Name: "n", Type: IntegrationTypeFirecrawl}, "firecrawl settings are required for firecrawl integration"},
		{"zapier without settings", Integration{ID: "1", Name: "n", Type: IntegrationTypeZapier}, "zapier settings are required for zapier integration"},
		{"unknown type", Integration{ID: "1", Name: "n", Type: "carrier-pigeon"}, "unsupported integration type: carrier-pigeon"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.EqualError(t, tc.i.Validate("pw"), tc.want)
		})
	}
}
