package domain

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Sentinels chosen so a substring search over the marshalled JSON is conclusive.
const (
	secretSMTPPassword       = "SENTINEL-smtp-password"
	secretSMTPOAuthSecret    = "SENTINEL-smtp-oauth-client-secret"
	secretSMTPOAuthRefresh   = "SENTINEL-smtp-oauth-refresh-token"
	secretSESSecretKey       = "SENTINEL-ses-secret-key"
	secretSparkPostAPIKey    = "SENTINEL-sparkpost-api-key"
	secretPostmarkToken      = "SENTINEL-postmark-server-token"
	secretMailgunAPIKey      = "SENTINEL-mailgun-api-key"
	secretMailjetAPIKey      = "SENTINEL-mailjet-api-key"
	secretMailjetSecretKey   = "SENTINEL-mailjet-secret-key"
	secretSendGridAPIKey     = "SENTINEL-sendgrid-api-key"
	secretAnthropicAPIKey    = "SENTINEL-anthropic-api-key"
	secretOpenAIAPIKey       = "SENTINEL-openai-api-key"
	secretGeminiAPIKey       = "SENTINEL-gemini-api-key"
	secretFirecrawlAPIKey    = "SENTINEL-firecrawl-api-key"
	secretSupabaseAuthKey    = "SENTINEL-supabase-auth-signature-key"
	secretSupabaseCreatedKey = "SENTINEL-supabase-created-signature-key"
	secretFileManagerKey     = "SENTINEL-file-manager-secret-key"
)

// workspaceWithEverySecret builds a workspace carrying every plaintext credential
// the decrypt-on-load path can populate, so the test fails if any one of them
// survives into a response body.
func workspaceWithEverySecret() *Workspace {
	return &Workspace{
		ID:   "ws-1",
		Name: "Acme",
		Settings: WorkspaceSettings{
			FileManager: FileManagerSettings{
				AccessKey: "AKIA-not-a-secret",
				SecretKey: secretFileManagerKey,
			},
		},
		Integrations: Integrations{
			{
				ID:   "int-smtp",
				Name: "SMTP",
				Type: IntegrationTypeEmail,
				EmailProvider: EmailProvider{
					Kind: EmailProviderKindSMTP,
					SMTP: &SMTPSettings{
						Host:               "smtp.example.com",
						Port:               587,
						Username:           "postmaster@acme.com",
						Password:           secretSMTPPassword,
						OAuth2ClientSecret: secretSMTPOAuthSecret,
						OAuth2RefreshToken: secretSMTPOAuthRefresh,
					},
					SES:       &AmazonSESSettings{AccessKey: "AKIA-not-a-secret", SecretKey: secretSESSecretKey},
					SparkPost: &SparkPostSettings{APIKey: secretSparkPostAPIKey},
					Postmark:  &PostmarkSettings{ServerToken: secretPostmarkToken},
					Mailgun:   &MailgunSettings{APIKey: secretMailgunAPIKey},
					Mailjet:   &MailjetSettings{APIKey: secretMailjetAPIKey, SecretKey: secretMailjetSecretKey},
					SendGrid:  &SendGridSettings{APIKey: secretSendGridAPIKey},
				},
			},
			{
				ID:   "int-llm",
				Name: "LLM",
				LLMProvider: &LLMProvider{
					Anthropic: &AnthropicSettings{APIKey: secretAnthropicAPIKey},
					OpenAI:    &OpenAISettings{APIKey: secretOpenAIAPIKey},
					Gemini:    &GeminiSettings{APIKey: secretGeminiAPIKey},
				},
				FirecrawlSettings: &FirecrawlSettings{APIKey: secretFirecrawlAPIKey},
				SupabaseSettings: &SupabaseIntegrationSettings{
					AuthEmailHook:         SupabaseAuthEmailHookSettings{SignatureKey: secretSupabaseAuthKey},
					BeforeUserCreatedHook: SupabaseUserCreatedHookSettings{SignatureKey: secretSupabaseCreatedKey},
				},
			},
		},
	}
}

func allSecretSentinels() []string {
	return []string{
		secretSMTPPassword, secretSMTPOAuthSecret, secretSMTPOAuthRefresh,
		secretSESSecretKey, secretSparkPostAPIKey, secretPostmarkToken,
		secretMailgunAPIKey, secretMailjetAPIKey, secretMailjetSecretKey,
		secretSendGridAPIKey, secretAnthropicAPIKey, secretOpenAIAPIKey,
		secretGeminiAPIKey, secretFirecrawlAPIKey, secretSupabaseAuthKey,
		secretSupabaseCreatedKey,
		// secretFileManagerKey is deliberately absent — see
		// TestWorkspaceRedact_KeepsFileManagerSecret.
	}
}

// TestWorkspaceRedact_RemovesEveryPlaintextCredential is the load-bearing test.
//
// It asserts on the marshalled bytes rather than on fields, because the defect it
// guards against is a credential reaching a response body. A field-by-field
// assertion would still pass if a new provider were added and left unredacted;
// this fails the moment any sentinel survives serialisation.
func TestWorkspaceRedact_RemovesEveryPlaintextCredential(t *testing.T) {
	w := workspaceWithEverySecret()

	// Sanity: without redaction these really are in the payload. If this half
	// ever stops holding, the test below is passing vacuously.
	before, err := json.Marshal(w)
	require.NoError(t, err)
	for _, secret := range allSecretSentinels() {
		require.Contains(t, string(before), secret,
			"fixture no longer carries %s — the redaction assertion would be vacuous", secret)
	}

	w.Redact()

	after, err := json.Marshal(w)
	require.NoError(t, err)
	for _, secret := range allSecretSentinels() {
		assert.NotContains(t, string(after), secret, "%s survived redaction", secret)
	}
}

// TestWorkspaceRedact_KeepsNonSecretContext pins what redaction must NOT remove.
// The console shows these to say which account an integration is connected as; a
// redaction that blanked them would look like a broken settings page.
func TestWorkspaceRedact_KeepsNonSecretContext(t *testing.T) {
	w := workspaceWithEverySecret()
	w.Redact()

	out, err := json.Marshal(w)
	require.NoError(t, err)
	s := string(out)

	assert.Contains(t, s, "smtp.example.com", "host is not a secret")
	assert.Contains(t, s, "postmaster@acme.com", "username identifies the account, and the password is what protects it")
	assert.Contains(t, s, "AKIA-not-a-secret", "an access key id is useless without its secret")
	assert.Contains(t, s, "int-smtp", "integration identity must survive")
}

// TestWorkspaceRedact_IsSafeOnEmptyAndNil guards the boundary call sites, which
// run on every workspace including ones with no integrations at all.
func TestWorkspaceRedact_IsSafeOnEmptyAndNil(t *testing.T) {
	assert.NotPanics(t, func() { (*Workspace)(nil).Redact() })
	assert.NotPanics(t, func() { (&Workspace{}).Redact() })
	assert.NotPanics(t, func() {
		(&Workspace{Integrations: Integrations{{ID: "bare"}}}).Redact()
	})
}

// TestWorkspaceRedact_LeavesCiphertext documents a deliberate limit: the
// encrypted forms still go out, because the console uses their presence to show
// whether a credential is configured. They are useless without the instance
// secret key, which is never served.
func TestWorkspaceRedact_LeavesCiphertext(t *testing.T) {
	w := &Workspace{
		Integrations: Integrations{{
			ID: "int-smtp",
			EmailProvider: EmailProvider{
				Kind: EmailProviderKindSMTP,
				SMTP: &SMTPSettings{
					Password:          secretSMTPPassword,
					EncryptedPassword: "ciphertext-blob",
				},
			},
		}},
	}
	w.Redact()

	out, err := json.Marshal(w)
	require.NoError(t, err)
	assert.NotContains(t, string(out), secretSMTPPassword)
	assert.True(t, strings.Contains(string(out), "ciphertext-blob"),
		"the console needs a way to tell a configured credential from an unset one")
}

// --- Masked hints -----------------------------------------------------------
//
// Redaction removes a credential entirely, which leaves an owner unable to tell
// the production key from the staging one. The hint is the last few characters,
// enough to recognise a key you already know and useless to anyone else.

func TestWorkspaceRedact_ExposesLastFourCharactersAsHint(t *testing.T) {
	w := workspaceWithEverySecret()
	w.Redact()

	emailInt := w.Integrations[0]
	otherInt := w.Integrations[1]

	cases := []struct {
		integration Integration
		key         string
		secret      string
	}{
		{emailInt, "smtp.password", secretSMTPPassword},
		{emailInt, "smtp.oauth2_client_secret", secretSMTPOAuthSecret},
		{emailInt, "smtp.oauth2_refresh_token", secretSMTPOAuthRefresh},
		{emailInt, "ses.secret_key", secretSESSecretKey},
		{emailInt, "sparkpost.api_key", secretSparkPostAPIKey},
		{emailInt, "postmark.server_token", secretPostmarkToken},
		{emailInt, "mailgun.api_key", secretMailgunAPIKey},
		{emailInt, "mailjet.api_key", secretMailjetAPIKey},
		{emailInt, "mailjet.secret_key", secretMailjetSecretKey},
		{emailInt, "sendgrid.api_key", secretSendGridAPIKey},
		{otherInt, "anthropic.api_key", secretAnthropicAPIKey},
		{otherInt, "openai.api_key", secretOpenAIAPIKey},
		{otherInt, "gemini.api_key", secretGeminiAPIKey},
		{otherInt, "firecrawl.api_key", secretFirecrawlAPIKey},
		{otherInt, "supabase.auth_email_hook.signature_key", secretSupabaseAuthKey},
		{otherInt, "supabase.before_user_created_hook.signature_key", secretSupabaseCreatedKey},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			got, ok := tc.integration.CredentialHints[tc.key]
			require.True(t, ok, "no hint recorded for %s", tc.key)
			assert.Equal(t, tc.secret[len(tc.secret)-4:], got)
			assert.Len(t, got, 4)
		})
	}

}

// TestWorkspaceRedact_KeepsFileManagerSecret pins the one exception, so nobody
// "completes" the redaction later and breaks file uploads.
//
// The console builds an S3Client in the browser from this field and talks to the
// bucket directly. Blanking it hardens nothing and breaks the file manager for
// every user; the real fix is presigned URLs, tracked separately.
func TestWorkspaceRedact_KeepsFileManagerSecret(t *testing.T) {
	w := workspaceWithEverySecret()
	w.Redact()

	assert.Equal(t, secretFileManagerKey, w.Settings.FileManager.SecretKey,
		"the browser-side S3 client needs this; redacting it is a breakage, not a fix")
}

// A hint must never be a usable fraction of a short secret.
func TestWorkspaceRedact_NoHintForShortSecrets(t *testing.T) {
	for _, secret := range []string{"a", "abc", "1234", "1234567"} {
		t.Run(secret, func(t *testing.T) {
			w := &Workspace{Integrations: Integrations{{
				ID: "i", Type: IntegrationTypeEmail,
				EmailProvider: EmailProvider{
					Kind: EmailProviderKindSendGrid,
					SendGrid: &SendGridSettings{
						APIKey:          secret,
						EncryptedAPIKey: "cipher",
					},
				},
			}}}
			w.Redact()

			_, ok := w.Integrations[0].CredentialHints["sendgrid.api_key"]
			assert.False(t, ok, "four characters of a %d-character secret gives away too much", len(secret))
		})
	}
}

func TestWorkspaceRedact_NoHintWhenCredentialUnset(t *testing.T) {
	w := &Workspace{Integrations: Integrations{{
		ID: "i", Type: IntegrationTypeEmail,
		EmailProvider: EmailProvider{Kind: EmailProviderKindSMTP, SMTP: &SMTPSettings{Host: "h"}},
	}}}
	w.Redact()

	assert.Empty(t, w.Integrations[0].CredentialHints,
		"an unset credential must look unset, not configured-but-hidden")
}

// The hint must never let the whole secret back out, however it is assembled.
func TestWorkspaceRedact_HintIsNeverTheSecret(t *testing.T) {
	w := workspaceWithEverySecret()
	w.Redact()

	out, err := json.Marshal(w)
	require.NoError(t, err)
	for _, secret := range allSecretSentinels() {
		assert.NotContains(t, string(out), secret)
	}
}

// Hints are computed for display and must never reach the database. repo.Update
// calls Workspace.BeforeSave before marshalling, so clearing them there is what
// guarantees it.
func TestWorkspaceBeforeSave_ClearsCredentialHints(t *testing.T) {
	w := &Workspace{
		ID:   "ws-1",
		Name: "Acme",
		Settings: WorkspaceSettings{
			SecretKey: "workspace-secret", // BeforeSave refuses to save without one
		},
		Integrations: Integrations{{
			ID:              "i",
			Name:            "n",
			Type:            IntegrationTypeEmail,
			CredentialHints: map[string]string{"smtp.password": "abc3"},
			EmailProvider:   EmailProvider{Kind: EmailProviderKindSMTP, SMTP: &SMTPSettings{Host: "h", Port: 587}},
		}},
	}

	require.NoError(t, w.BeforeSave("passphrase"))

	assert.Nil(t, w.Integrations[0].CredentialHints, "a hint must not be persisted")

	out, err := json.Marshal(w.Integrations)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "credential_hints")
}

// --- The public surface ------------------------------------------------------
//
// Redact() is calibrated for a workspace MEMBER: it keeps the encrypted forms so
// the console can tell a configured credential from an unset one, keeps the last
// few characters as a hint, and keeps the S3 secret because the browser's file
// manager genuinely needs it.
//
// None of that reasoning survives on /api/workspaces.verifyInvitationToken, which
// is registered with NO authentication at all. The caller there is not a member —
// they hold an invitation token and may never accept it. The page shows them one
// thing, the workspace's name, so everything else is gratuitous.

func TestWorkspaceRedactForPublic_KeepsOnlyWhatAnInviteeNeeds(t *testing.T) {
	w := workspaceWithEverySecret()
	w.RedactForPublic()

	out, err := json.Marshal(w)
	require.NoError(t, err)
	s := string(out)

	// Everything Redact() deliberately keeps for a member must be gone here.
	assert.NotContains(t, s, secretFileManagerKey,
		"the file manager exemption is for a member's browser, not an unauthenticated invitee")
	assert.NotContains(t, s, "credential_hints",
		"a hint is a fragment of a credential; an invitee has no claim to one")
	assert.NotContains(t, s, "ciphertext")
	assert.NotContains(t, s, "encrypted_")

	for _, secret := range append(allSecretSentinels(), secretFileManagerKey) {
		assert.NotContains(t, s, secret)
	}

	// Integrations as a whole: an invitee has no business knowing which email
	// provider or LLM the workspace uses before they have even accepted.
	assert.Empty(t, w.Integrations)

	// What the invitation page actually renders must survive.
	assert.Contains(t, s, "Acme", "the invitee is told which workspace invited them")
	assert.Contains(t, s, "ws-1")
}

func TestWorkspaceRedactForPublic_IsSafeOnNilAndEmpty(t *testing.T) {
	assert.NotPanics(t, func() { (*Workspace)(nil).RedactForPublic() })
	assert.NotPanics(t, func() { (&Workspace{}).RedactForPublic() })
}

// The minted address is display-only and the integrations screen prints it on the card, so
// Redact leaves it whole and records no hint for it. A hint would be pointless anyway: the
// address is not a credential, and the token it belongs to is never persisted to hint at.
func TestWorkspaceRedact_KeepsZapierAPIKeyEmail(t *testing.T) {
	const address = "zapier-marketing-3f9a1c02@v3.notifuse.com"
	w := &Workspace{Integrations: Integrations{{
		ID: "i", Name: "Marketing", Type: IntegrationTypeZapier,
		ZapierSettings: &ZapierSettings{APIKeyEmail: address},
	}}}
	w.Redact()

	assert.Equal(t, address, w.Integrations[0].ZapierSettings.APIKeyEmail)
	assert.Empty(t, w.Integrations[0].CredentialHints)

	out, err := json.Marshal(w)
	require.NoError(t, err)
	assert.Contains(t, string(out), address)
}
