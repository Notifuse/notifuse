package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// OAuth2 SMTP must survive an edit that does not re-supply the credential.
//
// Workspaces no longer serve decrypted credentials, so the console cannot echo an
// integration's OAuth2 client secret back on save. Every other provider's
// Validate already tolerates that — they encrypt only `if plaintext != ""` — but
// validateOAuth2 hard-required the plaintext, and it runs in
// UpdateIntegrationRequest.Validate at the HTTP handler, upstream of the
// service's preserve step. So an owner renaming an OAuth2 integration got
// `oauth2_client_secret is required` and could not save at all.
//
// The second half matters as much as the first: the encryption call below the
// check was unconditional, so merely relaxing the check would encrypt "" over the
// stored ciphertext and destroy the credential — a silent failure that only shows
// up at the next send.

func oauth2SMTPProvider(mutate func(*SMTPSettings)) EmailProvider {
	smtp := &SMTPSettings{
		Host:           "smtp.office365.com",
		Port:           587,
		AuthType:       "oauth2",
		OAuth2Provider: "microsoft",
		OAuth2ClientID: "client-id",
		OAuth2TenantID: "tenant-id",
	}
	mutate(smtp)
	return EmailProvider{
		Kind:               EmailProviderKindSMTP,
		RateLimitPerMinute: 25,
		Senders:            []EmailSender{NewEmailSender("sender@example.com", "Sender")},
		SMTP:               smtp,
	}
}

func TestOAuth2SMTPValidatesWithoutResupplyingTheSecret(t *testing.T) {
	p := oauth2SMTPProvider(func(s *SMTPSettings) {
		s.EncryptedOAuth2ClientSecret = "stored-cipher"
	})

	require.NoError(t, p.Validate("passphrase"),
		"an edit that does not mention the credential must not be rejected")
	assert.Equal(t, "stored-cipher", p.SMTP.EncryptedOAuth2ClientSecret,
		"the stored credential must survive untouched — encrypting the blank would destroy it")
	assert.Empty(t, p.SMTP.OAuth2ClientSecret)
}

func TestOAuth2SMTPStillRequiresASecretWhenNoneIsStored(t *testing.T) {
	// Creation, where nothing is stored yet: the requirement still holds.
	p := oauth2SMTPProvider(func(s *SMTPSettings) {})

	err := p.Validate("passphrase")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oauth2_client_secret is required")
}

func TestOAuth2SMTPRotatesWhenASecretIsSupplied(t *testing.T) {
	p := oauth2SMTPProvider(func(s *SMTPSettings) {
		s.OAuth2ClientSecret = "a-new-secret"
		s.EncryptedOAuth2ClientSecret = "stored-cipher"
	})

	require.NoError(t, p.Validate("passphrase"))
	assert.NotEqual(t, "stored-cipher", p.SMTP.EncryptedOAuth2ClientSecret,
		"a typed-in secret must actually replace the stored one")
	assert.NotEmpty(t, p.SMTP.EncryptedOAuth2ClientSecret)
}

// Google additionally requires a refresh token, which is the more painful one to
// re-obtain: it takes re-running the OAuth flow, not a copy-paste.
func TestOAuth2GoogleRefreshTokenSurvivesAnEditThatOmitsIt(t *testing.T) {
	p := oauth2SMTPProvider(func(s *SMTPSettings) {
		s.OAuth2Provider = "google"
		s.OAuth2TenantID = ""
		s.EncryptedOAuth2ClientSecret = "stored-secret-cipher"
		s.EncryptedOAuth2RefreshToken = "stored-token-cipher"
	})

	require.NoError(t, p.Validate("passphrase"))
	assert.Equal(t, "stored-secret-cipher", p.SMTP.EncryptedOAuth2ClientSecret)
	assert.Equal(t, "stored-token-cipher", p.SMTP.EncryptedOAuth2RefreshToken)
}

func TestOAuth2GoogleStillRequiresARefreshTokenWhenNoneIsStored(t *testing.T) {
	p := oauth2SMTPProvider(func(s *SMTPSettings) {
		s.OAuth2Provider = "google"
		s.OAuth2TenantID = ""
		s.EncryptedOAuth2ClientSecret = "stored-cipher"
	})

	err := p.Validate("passphrase")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oauth2_refresh_token is required")
}

// The end-to-end shape of the regression: redact, serialise, echo back, validate.
// This is the path the console actually takes, and the one the service-level test
// for preserveEmailProviderSecrets could not see, because that helper runs
// downstream of this validation.
func TestOAuth2SMTPSurvivesRedactAndEcho(t *testing.T) {
	w := &Workspace{Integrations: Integrations{{
		ID: "int-1", Name: "Microsoft 365", Type: IntegrationTypeEmail,
		EmailProvider: oauth2SMTPProvider(func(s *SMTPSettings) {
			s.OAuth2ClientSecret = "SENTINEL-oauth2-secret"
			s.EncryptedOAuth2ClientSecret = "stored-cipher"
		}),
	}}}

	w.Redact()

	raw, err := json.Marshal(w.Integrations[0].EmailProvider)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "SENTINEL-oauth2-secret", "the credential must not be served")

	var echoed EmailProvider
	require.NoError(t, json.Unmarshal(raw, &echoed))

	require.NoError(t, echoed.Validate("passphrase"),
		"the console echoes back what it was given; rejecting it makes the integration uneditable")
	assert.Equal(t, "stored-cipher", echoed.SMTP.EncryptedOAuth2ClientSecret)
}
