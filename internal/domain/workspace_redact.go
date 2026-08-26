package domain

import "unicode/utf8"

// Redaction of decrypted credentials before a workspace leaves the process.
//
// INVARIANT: a Workspace must never be serialised to a client as-loaded.
//
// Every repository read path calls Workspace.AfterLoad, which decrypts each
// integration's credentials into plain JSON-tagged fields, because that is what
// the sending code needs. Those fields are for internal use only.
//
// Redact belongs at the HTTP boundary, not in the service or repository, because
// the same loaded workspace is what the email path signs and sends with. Any new
// endpoint that returns a Workspace must call it.
//
// The encrypted forms are deliberately left in place: the console distinguishes
// "configured" from "not set" by their presence, and they are inert without the
// instance secret key, which is never served. Non-secret context — hostnames,
// usernames, access key ids — is left alone too, so the settings UI can still say
// which account an integration is connected as.

// credentialHintMinLength is the shortest secret that earns a hint. Four
// characters of a six-character secret is most of it; below this threshold the
// console still knows the credential is set, from the encrypted field.
const credentialHintMinLength = 8

// credentialHintLength is how much of the tail is shown.
const credentialHintLength = 4

// credentialHint returns the last few characters of a secret, or "" when the
// secret is unset or too short to give any away safely.
//
// Counted in runes, not bytes, so a multi-byte character cannot be sliced in
// half into invalid UTF-8.
func credentialHint(secret string) string {
	if utf8.RuneCountInString(secret) < credentialHintMinLength {
		return ""
	}
	r := []rune(secret)
	return string(r[len(r)-credentialHintLength:])
}

// RedactForPublic strips a workspace down to what an UNAUTHENTICATED caller may
// see, for /api/workspaces.verifyInvitationToken.
//
// Redact() is calibrated for a member: it keeps the encrypted forms so the
// console can show "configured", keeps a few characters of each credential as a
// recognition hint, and keeps the S3 secret because the browser's file manager
// genuinely needs it. Every one of those justifications assumes the caller is
// inside the workspace. On the invitation route they are not — they hold a token
// and may never accept it — so none of it applies.
//
// The invitation page renders the workspace's name and nothing else, so the
// integrations go entirely: which email provider or LLM a workspace uses is not
// an invitee's business before they have joined.
func (w *Workspace) RedactForPublic() {
	if w == nil {
		return
	}
	w.Redact()
	w.Integrations = nil
	w.RedactFileManagerSecret()
}

// RedactFileManagerSecret blanks the S3 credential Redact deliberately keeps.
//
// Separate from Redact because the two answer different questions. Redact asks
// "may this leave the process at all", and for the S3 secret the answer is yes:
// the console's file manager talks to the bucket from the browser with it.
// This asks "does THIS response need it", and for a listing that exists to tell
// a caller which workspaces it can reach, the answer is no.
//
// Safe on a nil receiver so boundary call sites need no guard.
func (w *Workspace) RedactFileManagerSecret() {
	if w == nil {
		return
	}
	w.Settings.FileManager.SecretKey = ""
	w.Settings.FileManager.EncryptedSecretKey = ""
}

// Redact removes every decrypted credential from the workspace, in place.
//
// Safe on a nil receiver so boundary call sites need no guard.
func (w *Workspace) Redact() {
	if w == nil {
		return
	}

	// Settings.FileManager.SecretKey is deliberately NOT redacted, and it is the
	// one exception here.
	//
	// Integration credentials are used server-side only — the backend does the
	// sending — so no client ever needs them. The S3 secret is different: the
	// console builds an S3Client in the browser from this exact field
	// (console/src/components/file_manager/fileManager.tsx) and talks to the
	// bucket directly, for listing, upload and delete. Blanking it here does not
	// harden anything, it just breaks the file manager for every user.
	//
	// Fixing it properly means the browser never holds the key at all: presigned
	// URLs minted by the backend, or proxying the operations through it. That is
	// an architectural change, not a redaction, and it is tracked separately.

	for i := range w.Integrations {
		w.Integrations[i].Redact()
	}
}

// Redact removes every decrypted credential from a single integration.
func (i *Integration) Redact() {
	if i == nil {
		return
	}

	hints := map[string]string{}
	record := func(key, secret string) {
		if h := credentialHint(secret); h != "" {
			hints[key] = h
		}
	}

	if e := &i.EmailProvider; true {
		if e.SES != nil {
			record("ses.secret_key", e.SES.SecretKey)
		}
		if e.SMTP != nil {
			record("smtp.password", e.SMTP.Password)
			record("smtp.oauth2_client_secret", e.SMTP.OAuth2ClientSecret)
			record("smtp.oauth2_refresh_token", e.SMTP.OAuth2RefreshToken)
		}
		if e.SparkPost != nil {
			record("sparkpost.api_key", e.SparkPost.APIKey)
		}
		if e.Postmark != nil {
			record("postmark.server_token", e.Postmark.ServerToken)
		}
		if e.Mailgun != nil {
			record("mailgun.api_key", e.Mailgun.APIKey)
		}
		if e.Mailjet != nil {
			record("mailjet.api_key", e.Mailjet.APIKey)
			record("mailjet.secret_key", e.Mailjet.SecretKey)
		}
		if e.SendGrid != nil {
			record("sendgrid.api_key", e.SendGrid.APIKey)
		}
	}
	if i.LLMProvider != nil {
		if i.LLMProvider.Anthropic != nil {
			record("anthropic.api_key", i.LLMProvider.Anthropic.APIKey)
		}
		if i.LLMProvider.OpenAI != nil {
			record("openai.api_key", i.LLMProvider.OpenAI.APIKey)
		}
		if i.LLMProvider.Gemini != nil {
			record("gemini.api_key", i.LLMProvider.Gemini.APIKey)
		}
	}
	if i.FirecrawlSettings != nil {
		record("firecrawl.api_key", i.FirecrawlSettings.APIKey)
	}
	if i.SupabaseSettings != nil {
		record("supabase.auth_email_hook.signature_key", i.SupabaseSettings.AuthEmailHook.SignatureKey)
		record("supabase.before_user_created_hook.signature_key", i.SupabaseSettings.BeforeUserCreatedHook.SignatureKey)
	}

	// nil rather than an empty map, so "no credentials configured" serialises as
	// an absent field instead of {}.
	if len(hints) > 0 {
		i.CredentialHints = hints
	} else {
		i.CredentialHints = nil
	}

	i.EmailProvider.Redact()

	if i.LLMProvider != nil {
		if i.LLMProvider.Anthropic != nil {
			i.LLMProvider.Anthropic.APIKey = ""
		}
		if i.LLMProvider.OpenAI != nil {
			i.LLMProvider.OpenAI.APIKey = ""
		}
		if i.LLMProvider.Gemini != nil {
			i.LLMProvider.Gemini.APIKey = ""
		}
	}

	if i.FirecrawlSettings != nil {
		i.FirecrawlSettings.APIKey = ""
	}

	if i.SupabaseSettings != nil {
		i.SupabaseSettings.AuthEmailHook.SignatureKey = ""
		i.SupabaseSettings.BeforeUserCreatedHook.SignatureKey = ""
	}
}

// Redact removes every decrypted credential from an email provider.
//
// Deliberately NOT gated on Kind, unlike EncryptSecretKeys. Kind says which
// provider is active, not which structs hold data: switching a workspace from
// SES to SMTP leaves the SES block populated, and a Kind-gated redaction would
// serve those stale credentials to every member.
func (e *EmailProvider) Redact() {
	if e == nil {
		return
	}

	if e.SES != nil {
		e.SES.SecretKey = ""
	}
	if e.SMTP != nil {
		e.SMTP.Password = ""
		e.SMTP.OAuth2ClientSecret = ""
		e.SMTP.OAuth2RefreshToken = ""
	}
	if e.SparkPost != nil {
		e.SparkPost.APIKey = ""
	}
	if e.Postmark != nil {
		e.Postmark.ServerToken = ""
	}
	if e.Mailgun != nil {
		e.Mailgun.APIKey = ""
	}
	if e.Mailjet != nil {
		e.Mailjet.APIKey = ""
		e.Mailjet.SecretKey = ""
	}
	if e.SendGrid != nil {
		e.SendGrid.APIKey = ""
	}
}
