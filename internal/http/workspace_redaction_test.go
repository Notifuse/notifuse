package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Credential redaction is enforced at the HTTP boundary, which means it is only
// as good as the call sites.
//
// domain.Workspace.Redact() has thorough unit tests, but they prove the function
// works — not that anyone calls it. Deleting `workspace.Redact()` from a handler
// left every one of those tests green, and the whole fix is those six lines.
// These tests assert on the RESPONSE BODY for exactly that reason: they fail if a
// call site is removed, reordered after serialisation, or forgotten on a new
// endpoint that returns a workspace.

const (
	redactSMTPPassword    = "SENTINEL-smtp-password"
	redactSESSecretKey    = "SENTINEL-ses-secret-key"
	redactSendGridAPIKey  = "SENTINEL-sendgrid-api-key"
	redactAnthropicAPIKey = "SENTINEL-anthropic-api-key"
)

// workspaceWithLiveCredentials mirrors what the repository hands back: AfterLoad
// has already decrypted every credential into its plain field.
func workspaceWithLiveCredentials(id string) *domain.Workspace {
	return &domain.Workspace{
		ID:   id,
		Name: "Acme",
		Settings: domain.WorkspaceSettings{
			Timezone: "UTC",
		},
		Integrations: domain.Integrations{
			{
				ID:   "int-email",
				Name: "Email",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSMTP,
					SMTP: &domain.SMTPSettings{
						Host:              "smtp.example.com",
						Port:              587,
						Username:          "postmaster@acme.com",
						Password:          redactSMTPPassword,
						EncryptedPassword: "ciphertext",
					},
					SES:      &domain.AmazonSESSettings{AccessKey: "AKIA-public", SecretKey: redactSESSecretKey},
					SendGrid: &domain.SendGridSettings{APIKey: redactSendGridAPIKey},
				},
			},
			{
				ID:          "int-llm",
				Name:        "LLM",
				LLMProvider: &domain.LLMProvider{Anthropic: &domain.AnthropicSettings{APIKey: redactAnthropicAPIKey}},
			},
		},
	}
}

func allRedactionSentinels() []string {
	return []string{redactSMTPPassword, redactSESSecretKey, redactSendGridAPIKey, redactAnthropicAPIKey}
}

// assertNoCredential is deliberately a substring search over the raw body rather
// than a field-by-field check: a new provider added later is covered without
// anyone remembering to extend this test.
func assertNoCredential(t *testing.T, body string) {
	t.Helper()
	for _, secret := range allRedactionSentinels() {
		assert.NotContains(t, body, secret, "a decrypted credential reached the response body")
	}
}

func TestWorkspaceHandlerRedactsCredentials(t *testing.T) {
	t.Run("workspaces.get", func(t *testing.T) {
		_, workspaceSvc, mux, secretKey, _ := setupTest(t)
		workspaceSvc.EXPECT().
			GetWorkspace(gomock.Any(), "ws1").
			Return(workspaceWithLiveCredentials("ws1"), nil)

		req := httptest.NewRequest(http.MethodGet, "/api/workspaces.get?id=ws1", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assertNoCredential(t, body)

		// The response must still be usable: identity and non-secret context stay.
		assert.Contains(t, body, "smtp.example.com", "the host is not a secret")
		assert.Contains(t, body, "postmaster@acme.com", "the username says which account is connected")
		assert.Contains(t, body, "ciphertext", "the console tells configured from unset by the encrypted field")
	})

	t.Run("workspaces.list", func(t *testing.T) {
		_, workspaceSvc, mux, secretKey, _ := setupTest(t)
		workspaceSvc.EXPECT().
			ListWorkspaces(gomock.Any()).
			Return([]*domain.Workspace{
				workspaceWithLiveCredentials("ws1"),
				workspaceWithLiveCredentials("ws2"),
			}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/workspaces.list", nil)
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assertNoCredential(t, body)

		// Every element, not just the first: the loop is the easy thing to get
		// wrong when the payload is a list.
		assert.Equal(t, 2, strings.Count(body, `"id":"ws`), "both workspaces were returned")
	})
}

// The invitation-verification route is PUBLIC — no authentication at all — and it
// returns the workspace. It is the call site that matters most and the one least
// likely to be thought of, because it is not reached from the console's own
// settings screens.
func TestVerifyInvitationTokenRedactsCredentials(t *testing.T) {
	handler, workspaceSvc, _, _, authSvc := setupTest(t)

	const invitationID, workspaceID, email = "inv-1", "ws1", "invited@example.com"

	authSvc.EXPECT().
		ValidateInvitationToken("valid-token").
		Return(invitationID, workspaceID, email, nil)
	workspaceSvc.EXPECT().
		GetInvitationByID(gomock.Any(), invitationID).
		Return(&domain.WorkspaceInvitation{
			ID: invitationID, WorkspaceID: workspaceID, Email: email,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}, nil)
	workspaceSvc.EXPECT().
		GetWorkspace(gomock.Any(), workspaceID).
		Return(workspaceWithLiveCredentials(workspaceID), nil)

	body, err := json.Marshal(VerifyInvitationTokenRequest{Token: "valid-token"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces.verifyInvitationToken", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.handleVerifyInvitationToken(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	respBody := w.Body.String()
	assertNoCredential(t, respBody)

	// This route redacts harder than the member-facing ones. None of the things
	// Redact() deliberately keeps for a member are defensible for a caller who is
	// not authenticated at all and may never accept the invitation.
	assert.NotContains(t, respBody, "credential_hints", "a hint is a fragment of a credential")
	assert.NotContains(t, respBody, "ciphertext", "not even the encrypted form")
	assert.NotContains(t, respBody, "int-email", "an invitee has no business knowing which providers are configured")
	assert.Contains(t, respBody, "Acme", "but they are still told which workspace invited them")
}

// A guard against the fixture rotting: if the workspace stops carrying the
// sentinels, every assertion above passes while proving nothing.
func TestRedactionFixtureActuallyCarriesCredentials(t *testing.T) {
	raw, err := json.Marshal(workspaceWithLiveCredentials("ws1"))
	require.NoError(t, err)

	for _, secret := range allRedactionSentinels() {
		require.Contains(t, string(raw), secret,
			"the fixture no longer carries %s, so the redaction tests are vacuous", secret)
	}
}

// handleCreate and handleUpdate return the workspace they just persisted. They
// matter for a second reason beyond disclosure: redaction MUTATES in place, so a
// call placed before the save would blank the credentials in the database. These
// assert the response, which is only reachable once the save has happened.
func TestWorkspaceHandlerRedactsOnWriteEndpoints(t *testing.T) {
	t.Run("workspaces.create", func(t *testing.T) {
		_, workspaceSvc, mux, secretKey, _ := setupTest(t)
		workspaceSvc.EXPECT().
			CreateWorkspace(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(workspaceWithLiveCredentials("ws1"), nil).
			AnyTimes()

		body, err := json.Marshal(domain.CreateWorkspaceRequest{
			ID:   "ws1",
			Name: "Acme",
			Settings: domain.WorkspaceSettings{
				WebsiteURL:      "https://example.com",
				Timezone:        "UTC",
				DefaultLanguage: "en",
				Languages:       []string{"en"},
			},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.create", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		assertNoCredential(t, w.Body.String())
	})

	t.Run("workspaces.update", func(t *testing.T) {
		_, workspaceSvc, mux, secretKey, _ := setupTest(t)
		workspaceSvc.EXPECT().
			UpdateWorkspace(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(workspaceWithLiveCredentials("ws1"), nil).
			AnyTimes()

		body, err := json.Marshal(map[string]interface{}{
			"id":   "ws1",
			"name": "Acme",
			"settings": map[string]interface{}{
				"website_url":      "https://example.com",
				"timezone":         "UTC",
				"default_language": "en",
				"languages":        []string{"en"},
			},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.update", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+createTestToken(t, secretKey, "test-user"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assertNoCredential(t, w.Body.String())
	})
}
