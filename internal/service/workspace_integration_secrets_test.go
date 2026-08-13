package service

import (
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
)

// Credential preservation on integration update.
//
// Workspaces do not serve decrypted credentials (domain.Workspace.Redact), so a
// client cannot echo an integration's password back on save. The email branch of
// UpdateIntegration assigns req.Provider wholesale, so without preservation an
// edit that does not mention a credential would wipe the stored one.
//
// Supabase and LLM integrations preserve their keys the same way.

func TestPreserveEmailProviderSecrets_KeepsStoredCredentialWhenClientSendsNone(t *testing.T) {
	cases := []struct {
		name     string
		existing domain.EmailProvider
		incoming domain.EmailProvider
		assert   func(*testing.T, domain.EmailProvider)
	}{
		{
			name: "smtp password",
			existing: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
				SMTP: &domain.SMTPSettings{Host: "old.example.com", EncryptedPassword: "stored-cipher"},
			},
			incoming: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
				SMTP: &domain.SMTPSettings{Host: "new.example.com"},
			},
			assert: func(t *testing.T, got domain.EmailProvider) {
				assert.Equal(t, "stored-cipher", got.SMTP.EncryptedPassword, "credential must survive an edit that does not mention it")
				assert.Equal(t, "new.example.com", got.SMTP.Host, "the edit itself must still apply")
			},
		},
		{
			name: "smtp oauth2 secret and refresh token",
			existing: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
				SMTP: &domain.SMTPSettings{
					EncryptedOAuth2ClientSecret: "stored-client-secret",
					EncryptedOAuth2RefreshToken: "stored-refresh-token",
				},
			},
			incoming: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, SMTP: &domain.SMTPSettings{}},
			assert: func(t *testing.T, got domain.EmailProvider) {
				assert.Equal(t, "stored-client-secret", got.SMTP.EncryptedOAuth2ClientSecret)
				assert.Equal(t, "stored-refresh-token", got.SMTP.EncryptedOAuth2RefreshToken)
			},
		},
		{
			name: "ses secret key",
			existing: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES:  &domain.AmazonSESSettings{EncryptedSecretKey: "stored-cipher"},
			},
			incoming: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES:  &domain.AmazonSESSettings{AccessKey: "AKIA-new"},
			},
			assert: func(t *testing.T, got domain.EmailProvider) {
				assert.Equal(t, "stored-cipher", got.SES.EncryptedSecretKey)
				assert.Equal(t, "AKIA-new", got.SES.AccessKey)
			},
		},
		{
			name: "sparkpost api key",
			existing: domain.EmailProvider{
				Kind:      domain.EmailProviderKindSparkPost,
				SparkPost: &domain.SparkPostSettings{EncryptedAPIKey: "stored-cipher"},
			},
			incoming: domain.EmailProvider{Kind: domain.EmailProviderKindSparkPost, SparkPost: &domain.SparkPostSettings{}},
			assert: func(t *testing.T, got domain.EmailProvider) {
				assert.Equal(t, "stored-cipher", got.SparkPost.EncryptedAPIKey)
			},
		},
		{
			name: "postmark server token",
			existing: domain.EmailProvider{
				Kind:     domain.EmailProviderKindPostmark,
				Postmark: &domain.PostmarkSettings{EncryptedServerToken: "stored-cipher"},
			},
			incoming: domain.EmailProvider{Kind: domain.EmailProviderKindPostmark, Postmark: &domain.PostmarkSettings{}},
			assert: func(t *testing.T, got domain.EmailProvider) {
				assert.Equal(t, "stored-cipher", got.Postmark.EncryptedServerToken)
			},
		},
		{
			name: "mailgun api key",
			existing: domain.EmailProvider{
				Kind:    domain.EmailProviderKindMailgun,
				Mailgun: &domain.MailgunSettings{EncryptedAPIKey: "stored-cipher"},
			},
			incoming: domain.EmailProvider{Kind: domain.EmailProviderKindMailgun, Mailgun: &domain.MailgunSettings{}},
			assert: func(t *testing.T, got domain.EmailProvider) {
				assert.Equal(t, "stored-cipher", got.Mailgun.EncryptedAPIKey)
			},
		},
		{
			name: "mailjet api key and secret key",
			existing: domain.EmailProvider{
				Kind:    domain.EmailProviderKindMailjet,
				Mailjet: &domain.MailjetSettings{EncryptedAPIKey: "stored-api", EncryptedSecretKey: "stored-secret"},
			},
			incoming: domain.EmailProvider{Kind: domain.EmailProviderKindMailjet, Mailjet: &domain.MailjetSettings{}},
			assert: func(t *testing.T, got domain.EmailProvider) {
				assert.Equal(t, "stored-api", got.Mailjet.EncryptedAPIKey)
				assert.Equal(t, "stored-secret", got.Mailjet.EncryptedSecretKey)
			},
		},
		{
			name: "sendgrid api key",
			existing: domain.EmailProvider{
				Kind:     domain.EmailProviderKindSendGrid,
				SendGrid: &domain.SendGridSettings{EncryptedAPIKey: "stored-cipher"},
			},
			incoming: domain.EmailProvider{Kind: domain.EmailProviderKindSendGrid, SendGrid: &domain.SendGridSettings{}},
			assert: func(t *testing.T, got domain.EmailProvider) {
				assert.Equal(t, "stored-cipher", got.SendGrid.EncryptedAPIKey)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updated := &domain.Integration{EmailProvider: tc.incoming}
			existing := &domain.Integration{EmailProvider: tc.existing}

			preserveEmailProviderSecrets(updated, existing)

			tc.assert(t, updated.EmailProvider)
		})
	}
}

// A deliberate rotation must not be silently undone by the preservation above.
func TestPreserveEmailProviderSecrets_DoesNotOverrideAnIntentionalRotation(t *testing.T) {
	t.Run("new plaintext wins", func(t *testing.T) {
		updated := &domain.Integration{EmailProvider: domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{Password: "brand-new-password"},
		}}
		existing := &domain.Integration{EmailProvider: domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{EncryptedPassword: "stored-cipher"},
		}}

		preserveEmailProviderSecrets(updated, existing)

		assert.Equal(t, "brand-new-password", updated.EmailProvider.SMTP.Password)
		assert.Empty(t, updated.EmailProvider.SMTP.EncryptedPassword,
			"the stored ciphertext must not shadow a rotation; encryption happens downstream")
	})

	t.Run("new ciphertext wins", func(t *testing.T) {
		updated := &domain.Integration{EmailProvider: domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{EncryptedPassword: "caller-supplied-cipher"},
		}}
		existing := &domain.Integration{EmailProvider: domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{EncryptedPassword: "stored-cipher"},
		}}

		preserveEmailProviderSecrets(updated, existing)

		assert.Equal(t, "caller-supplied-cipher", updated.EmailProvider.SMTP.EncryptedPassword)
	})
}

// Switching provider kind must not drag the old provider's credential across.
func TestPreserveEmailProviderSecrets_DoesNotResurrectAcrossProviders(t *testing.T) {
	updated := &domain.Integration{EmailProvider: domain.EmailProvider{
		Kind:     domain.EmailProviderKindSendGrid,
		SendGrid: &domain.SendGridSettings{APIKey: "new-sendgrid-key"},
	}}
	existing := &domain.Integration{EmailProvider: domain.EmailProvider{
		Kind: domain.EmailProviderKindSMTP,
		SMTP: &domain.SMTPSettings{EncryptedPassword: "stored-smtp-cipher"},
	}}

	preserveEmailProviderSecrets(updated, existing)

	assert.Nil(t, updated.EmailProvider.SMTP, "a provider the caller did not send must not be conjured up")
	assert.Equal(t, "new-sendgrid-key", updated.EmailProvider.SendGrid.APIKey)
}

func TestPreserveEmailProviderSecrets_IsSafeOnNils(t *testing.T) {
	assert.NotPanics(t, func() { preserveEmailProviderSecrets(nil, nil) })
	assert.NotPanics(t, func() { preserveEmailProviderSecrets(&domain.Integration{}, nil) })
	assert.NotPanics(t, func() { preserveEmailProviderSecrets(&domain.Integration{}, &domain.Integration{}) })
}

// Testing a saved integration must not authenticate with a blank credential.
//
// The console tests an integration by posting the provider object it holds — and
// that object now comes from a redacted workspace, so every secret field is
// empty. Nothing on the server decrypted or merged anything, so the test send
// went out with `Authorization: Bearer ` and failed, for an integration that
// sends perfectly well in production. The failure is the worst kind: it accuses
// working configuration.
//
// Hydration takes the PLAINTEXT from the stored integration, not the ciphertext:
// the provider services sign with the plaintext, so restoring the encrypted form
// the way the update path does would not help here.
func TestHydrateEmailProviderCredentials(t *testing.T) {
	cases := []struct {
		name     string
		stored   domain.EmailProvider
		incoming domain.EmailProvider
		assert   func(*testing.T, domain.EmailProvider)
	}{
		{
			name:     "smtp password",
			stored:   domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, SMTP: &domain.SMTPSettings{Password: "stored-pw"}},
			incoming: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, SMTP: &domain.SMTPSettings{Host: "smtp.example.com"}},
			assert: func(t *testing.T, got domain.EmailProvider) {
				assert.Equal(t, "stored-pw", got.SMTP.Password)
				assert.Equal(t, "smtp.example.com", got.SMTP.Host, "non-credential edits still apply")
			},
		},
		{
			name: "smtp oauth2 pair",
			stored: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, SMTP: &domain.SMTPSettings{
				OAuth2ClientSecret: "stored-secret", OAuth2RefreshToken: "stored-token",
			}},
			incoming: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, SMTP: &domain.SMTPSettings{}},
			assert: func(t *testing.T, got domain.EmailProvider) {
				assert.Equal(t, "stored-secret", got.SMTP.OAuth2ClientSecret)
				assert.Equal(t, "stored-token", got.SMTP.OAuth2RefreshToken)
			},
		},
		{
			name:     "ses secret key",
			stored:   domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: &domain.AmazonSESSettings{SecretKey: "stored"}},
			incoming: domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: &domain.AmazonSESSettings{AccessKey: "AKIA"}},
			assert: func(t *testing.T, got domain.EmailProvider) {
				assert.Equal(t, "stored", got.SES.SecretKey)
			},
		},
		{
			name:     "sendgrid api key",
			stored:   domain.EmailProvider{Kind: domain.EmailProviderKindSendGrid, SendGrid: &domain.SendGridSettings{APIKey: "stored"}},
			incoming: domain.EmailProvider{Kind: domain.EmailProviderKindSendGrid, SendGrid: &domain.SendGridSettings{}},
			assert: func(t *testing.T, got domain.EmailProvider) {
				assert.Equal(t, "stored", got.SendGrid.APIKey)
			},
		},
		{
			name:     "mailjet pair",
			stored:   domain.EmailProvider{Kind: domain.EmailProviderKindMailjet, Mailjet: &domain.MailjetSettings{APIKey: "k", SecretKey: "s"}},
			incoming: domain.EmailProvider{Kind: domain.EmailProviderKindMailjet, Mailjet: &domain.MailjetSettings{}},
			assert: func(t *testing.T, got domain.EmailProvider) {
				assert.Equal(t, "k", got.Mailjet.APIKey)
				assert.Equal(t, "s", got.Mailjet.SecretKey)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			incoming := tc.incoming
			hydrateEmailProviderCredentials(&incoming, &tc.stored)
			tc.assert(t, incoming)
		})
	}
}

// Someone testing a credential they just typed is testing THAT credential, not
// the stored one — otherwise a wrong new key would appear to work.
func TestHydrateEmailProviderCredentialsNeverOverwritesASuppliedOne(t *testing.T) {
	stored := domain.EmailProvider{Kind: domain.EmailProviderKindSendGrid, SendGrid: &domain.SendGridSettings{APIKey: "stored"}}
	incoming := domain.EmailProvider{Kind: domain.EmailProviderKindSendGrid, SendGrid: &domain.SendGridSettings{APIKey: "typed-in"}}

	hydrateEmailProviderCredentials(&incoming, &stored)
	assert.Equal(t, "typed-in", incoming.SendGrid.APIKey)
}

// Testing a provider of a different kind than the stored one — the "change
// provider and test before saving" flow — must not borrow the old credential.
func TestHydrateEmailProviderCredentialsDoesNotCrossProviders(t *testing.T) {
	stored := domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, SMTP: &domain.SMTPSettings{Password: "stored"}}
	incoming := domain.EmailProvider{Kind: domain.EmailProviderKindSendGrid, SendGrid: &domain.SendGridSettings{APIKey: "new"}}

	hydrateEmailProviderCredentials(&incoming, &stored)
	assert.Nil(t, incoming.SMTP, "a provider block the caller did not send must not be conjured up")
	assert.Equal(t, "new", incoming.SendGrid.APIKey)
}

func TestHydrateEmailProviderCredentialsIsSafeOnNils(t *testing.T) {
	assert.NotPanics(t, func() { hydrateEmailProviderCredentials(nil, nil) })
	assert.NotPanics(t, func() { hydrateEmailProviderCredentials(&domain.EmailProvider{}, nil) })
	assert.NotPanics(t, func() { hydrateEmailProviderCredentials(&domain.EmailProvider{}, &domain.EmailProvider{}) })
}
