package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// captureRecorder keeps every row the middleware handed to it.
type captureRecorder struct {
	rows []*domain.AuditLog
}

func (c *captureRecorder) Record(_ context.Context, log *domain.AuditLog) {
	c.rows = append(c.rows, log)
}

func auditedRequest(method, path string, body string) *http.Request {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.RemoteAddr = "203.0.113.9:4321"
	req.Header.Set("User-Agent", "test-agent/1.0")
	return req
}

func serve(t *testing.T, recorder *captureRecorder, handler http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	NewAuditMiddleware(recorder)(handler).ServeHTTP(rec, req)
	return rec
}

func TestClientIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	assert.Equal(t, "10.0.0.1", ClientIP(req), "the port is stripped")

	req.Header.Set("X-Real-IP", "198.51.100.7")
	assert.Equal(t, "198.51.100.7", ClientIP(req))

	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.2")
	assert.Equal(t, "203.0.113.9", ClientIP(req), "the first forwarded address wins")
}

func TestAuditMiddleware_RequestID(t *testing.T) {
	recorder := &captureRecorder{}
	var seenInContext *domain.AuditRecord
	handler := func(w http.ResponseWriter, r *http.Request) {
		seenInContext = domain.AuditFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}

	rec := serve(t, recorder, handler, auditedRequest(http.MethodGet, "/api/contacts.list", ""))
	generated := rec.Header().Get(RequestIDHeader)
	assert.NotEmpty(t, generated, "a request id is minted when the client sent none")
	assert.NotNil(t, seenInContext, "every request carries a record, audited or not")

	req := auditedRequest(http.MethodGet, "/api/contacts.list", "")
	req.Header.Set(RequestIDHeader, "client-supplied")
	rec = serve(t, recorder, handler, req)
	assert.Equal(t, "client-supplied", rec.Header().Get(RequestIDHeader), "a client's id is echoed")

	req = auditedRequest(http.MethodGet, "/api/contacts.list", "")
	req.Header.Set(RequestIDHeader, strings.Repeat("x", 65))
	rec = serve(t, recorder, handler, req)
	assert.NotEqual(t, strings.Repeat("x", 65), rec.Header().Get(RequestIDHeader), "an oversized id is replaced")
	assert.Empty(t, recorder.rows, "a plain read is never recorded")
}

func TestAuditMiddleware_Classification(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

	t.Run("excluded and unknown routes are not recorded", func(t *testing.T) {
		recorder := &captureRecorder{}
		for _, req := range []*http.Request{
			auditedRequest(http.MethodPost, "/api/contacts.upsert", `{"workspace_id":"ws"}`),
			auditedRequest(http.MethodPost, "/api/transactional.send", `{"workspace_id":"ws"}`),
			auditedRequest(http.MethodGet, "/api/templates.list", ""),
			auditedRequest(http.MethodPost, "/api/nothing.here", `{}`),
			auditedRequest(http.MethodPost, "/na.js", ""),
			auditedRequest(http.MethodOptions, "/api/templates.update", ""),
			auditedRequest(http.MethodGet, "/api/templates.update", ""),
		} {
			serve(t, recorder, ok, req)
		}
		assert.Empty(t, recorder.rows)
	})

	t.Run("an audited POST is recorded with actor, context, target and outcome", func(t *testing.T) {
		recorder := &captureRecorder{}
		handler := func(w http.ResponseWriter, r *http.Request) {
			record := domain.AuditFromContext(r.Context())
			record.SetActorClaims("u1", "user")
			record.SetActor(&domain.User{ID: "u1", Email: "ann@example.com", Name: "Ann"}, &domain.UserWorkspace{WorkspaceID: "ws1", Role: "owner"}, false)
			record.SetTarget("", "", "Welcome")
			w.WriteHeader(http.StatusOK)
		}
		serve(t, recorder, handler, auditedRequest(http.MethodPost, "/api/templates.update", `{"workspace_id":"ws1","id":"tpl-1","name":"Welcome"}`))

		require.Len(t, recorder.rows, 1)
		row := recorder.rows[0]
		assert.Equal(t, "templates.update", row.Action)
		assert.Equal(t, domain.AuditCategoryTemplates, row.Category)
		assert.Equal(t, domain.AuditOutcomeSuccess, row.Outcome)
		assert.Equal(t, http.StatusOK, row.StatusCode)
		assert.Equal(t, "ws1", row.WorkspaceID)
		assert.Equal(t, "ann@example.com", row.ActorEmail)
		assert.Equal(t, "owner", row.ActorRole)
		assert.Equal(t, domain.AuditTargetTemplate, row.TargetType)
		assert.Equal(t, "tpl-1", row.TargetID, "the target id came from the body's id key")
		assert.Equal(t, "Welcome", row.TargetName, "the name came from the service")
		assert.Equal(t, "203.0.113.9", row.IPAddress)
		assert.Equal(t, "test-agent/1.0", row.UserAgent)
		assert.NotEmpty(t, row.RequestID)
	})

	t.Run("a logged read needs the export flag and no cursor", func(t *testing.T) {
		recorder := &captureRecorder{}
		serve(t, recorder, ok, auditedRequest(http.MethodGet, "/api/contacts.list?workspace_id=ws1", ""))
		serve(t, recorder, ok, auditedRequest(http.MethodGet, "/api/contacts.list?workspace_id=ws1&export=true&cursor=abc", ""))
		assert.Empty(t, recorder.rows)

		serve(t, recorder, ok, auditedRequest(http.MethodGet, "/api/contacts.list?workspace_id=ws1&export=true", ""))
		require.Len(t, recorder.rows, 1)
		assert.Equal(t, "contacts.export", recorder.rows[0].Action)
		assert.Equal(t, domain.AuditCategoryContacts, recorder.rows[0].Category)
	})

	t.Run("the OIDC callback is recorded whatever its query, and a redirect is a success", func(t *testing.T) {
		recorder := &captureRecorder{}
		redirect := func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/console", http.StatusFound) }
		serve(t, recorder, redirect, auditedRequest(http.MethodGet, "/api/user.oidc.callback?code=x&state=y", ""))
		require.Len(t, recorder.rows, 1)
		assert.Equal(t, "user.oidc.callback", recorder.rows[0].Action)
		assert.Equal(t, domain.AuditOutcomeSuccess, recorder.rows[0].Outcome)
	})
}

func TestAuditMiddleware_Outcomes(t *testing.T) {
	status := func(code int, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		}
	}
	post := func() *http.Request {
		return auditedRequest(http.MethodPost, "/api/workspaces.inviteMember", `{"workspace_id":"ws1","email":"bob@example.com"}`)
	}

	t.Run("403 is denied and names the resource", func(t *testing.T) {
		recorder := &captureRecorder{}
		serve(t, recorder, status(http.StatusForbidden, `{"error":"no","resource":"workspace","permission":"write"}`), post())
		require.Len(t, recorder.rows, 1)
		row := recorder.rows[0]
		assert.Equal(t, domain.AuditOutcomeDenied, row.Outcome)
		assert.Equal(t, http.StatusForbidden, row.StatusCode)
		assert.Equal(t, "workspace", row.Metadata["resource"])
		assert.Equal(t, "write", row.Metadata["permission"])
		assert.Equal(t, "bob@example.com", row.TargetID, "the target still comes from the body")
		_, licence := row.Metadata["licence_refused"]
		assert.False(t, licence)
	})

	t.Run("402 is denied and names the feature", func(t *testing.T) {
		recorder := &captureRecorder{}
		serve(t, recorder, status(http.StatusPaymentRequired, `{"error":"license_required","feature":"rbac","required_tier":"Studio"}`), post())
		require.Len(t, recorder.rows, 1)
		row := recorder.rows[0]
		assert.Equal(t, domain.AuditOutcomeDenied, row.Outcome)
		assert.Equal(t, true, row.Metadata["licence_refused"])
		assert.Equal(t, "rbac", row.Metadata["feature"])
		assert.Equal(t, "Studio", row.Metadata["required_tier"])
	})

	t.Run("401 is not recorded", func(t *testing.T) {
		recorder := &captureRecorder{}
		serve(t, recorder, status(http.StatusUnauthorized, `{"error":"Unauthorized"}`), post())
		assert.Empty(t, recorder.rows)
	})

	t.Run("other errors are failures with the status", func(t *testing.T) {
		recorder := &captureRecorder{}
		serve(t, recorder, status(http.StatusBadRequest, `{"error":"bad"}`), post())
		serve(t, recorder, status(http.StatusInternalServerError, `{"error":"boom"}`), post())
		require.Len(t, recorder.rows, 2)
		assert.Equal(t, domain.AuditOutcomeFailure, recorder.rows[0].Outcome)
		assert.Equal(t, http.StatusBadRequest, recorder.rows[0].StatusCode)
		assert.Equal(t, http.StatusInternalServerError, recorder.rows[1].StatusCode)
	})

	t.Run("a handler that never calls WriteHeader is a 200", func(t *testing.T) {
		recorder := &captureRecorder{}
		serve(t, recorder, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }, post())
		require.Len(t, recorder.rows, 1)
		assert.Equal(t, http.StatusOK, recorder.rows[0].StatusCode)
	})

	t.Run("Fail from a service wins over a 200", func(t *testing.T) {
		recorder := &captureRecorder{}
		handler := func(w http.ResponseWriter, r *http.Request) {
			domain.AuditFromContext(r.Context()).Fail("unknown_email")
			w.WriteHeader(http.StatusOK)
		}
		serve(t, recorder, handler, auditedRequest(http.MethodPost, "/api/user.signin", `{"email":"nobody@example.com"}`))
		require.Len(t, recorder.rows, 1)
		assert.Equal(t, domain.AuditOutcomeFailure, recorder.rows[0].Outcome)
		assert.Equal(t, "unknown_email", recorder.rows[0].Metadata["reason"])
		assert.Equal(t, "nobody@example.com", recorder.rows[0].TargetID)
	})

	t.Run("Skip suppresses the row", func(t *testing.T) {
		recorder := &captureRecorder{}
		handler := func(w http.ResponseWriter, r *http.Request) {
			domain.AuditFromContext(r.Context()).Skip()
			w.WriteHeader(http.StatusOK)
		}
		serve(t, recorder, handler, post())
		assert.Empty(t, recorder.rows)
	})
}

func TestAuditMiddleware_BodyPeek(t *testing.T) {
	t.Run("the handler receives every byte", func(t *testing.T) {
		recorder := &captureRecorder{}
		body := `{"workspace_id":"ws1","id":"tpl","padding":"` + strings.Repeat("p", 200*1024) + `"}`
		var received []byte
		handler := func(w http.ResponseWriter, r *http.Request) {
			var err error
			received, err = io.ReadAll(r.Body)
			require.NoError(t, err)
			w.WriteHeader(http.StatusOK)
		}
		serve(t, recorder, handler, auditedRequest(http.MethodPost, "/api/templates.update", body))
		assert.Equal(t, body, string(received))
		require.Len(t, recorder.rows, 1)
		assert.Equal(t, "ws1", recorder.rows[0].WorkspaceID)
		assert.Equal(t, "tpl", recorder.rows[0].TargetID)
	})

	t.Run("a body above the limit is replayed whole but not decoded", func(t *testing.T) {
		recorder := &captureRecorder{}
		body := `{"workspace_id":"ws1","padding":"` + strings.Repeat("p", auditBodyPeekLimit+1024) + `"}`
		var received int
		handler := func(w http.ResponseWriter, r *http.Request) {
			data, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			received = len(data)
			w.WriteHeader(http.StatusOK)
		}
		serve(t, recorder, handler, auditedRequest(http.MethodPost, "/api/contacts.import", body))
		assert.Equal(t, len(body), received)
		require.Len(t, recorder.rows, 1)
		assert.Empty(t, recorder.rows[0].WorkspaceID, "too large to peek; a service may still name it")
	})

	t.Run("a non-JSON body is left alone", func(t *testing.T) {
		recorder := &captureRecorder{}
		req := auditedRequest(http.MethodPost, "/api/templates.update", "")
		req.Body = io.NopCloser(bytes.NewBufferString("--boundary\r\nfile"))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
		var received string
		handler := func(w http.ResponseWriter, r *http.Request) {
			data, _ := io.ReadAll(r.Body)
			received = string(data)
			w.WriteHeader(http.StatusOK)
		}
		serve(t, recorder, handler, req)
		assert.Equal(t, "--boundary\r\nfile", received)
	})

	t.Run("a workspace target names the workspace too", func(t *testing.T) {
		recorder := &captureRecorder{}
		serve(t, recorder, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) },
			auditedRequest(http.MethodPost, "/api/workspaces.delete", `{"id":"ws-gone"}`))
		require.Len(t, recorder.rows, 1)
		assert.Equal(t, "ws-gone", recorder.rows[0].WorkspaceID)
		assert.Equal(t, "ws-gone", recorder.rows[0].TargetID)
		assert.Equal(t, domain.AuditTargetWorkspace, recorder.rows[0].TargetType)
	})

	t.Run("numeric ids are rendered as text", func(t *testing.T) {
		assert.Equal(t, "42", jsonScalar(json.RawMessage(`42`)))
		assert.Equal(t, "abc", jsonScalar(json.RawMessage(`"abc"`)))
		assert.Empty(t, jsonScalar(json.RawMessage(`{"a":1}`)))
		assert.Empty(t, jsonScalar(nil))
	})
}

func TestAuditMiddleware_ServicesEnrichAfterAuth(t *testing.T) {
	// The record set by the middleware is the one the auth middleware and the
	// services find on a child context.
	recorder := &captureRecorder{}
	handler := func(w http.ResponseWriter, r *http.Request) {
		child := context.WithValue(r.Context(), domain.UserIDKey, "u1")
		domain.AuditFromContext(child).SetActorClaims("u1", string(domain.UserTypeAPIKey))
		w.WriteHeader(http.StatusOK)
	}
	serve(t, recorder, handler, auditedRequest(http.MethodPost, "/api/lists.create", `{"workspace_id":"ws1"}`))
	require.Len(t, recorder.rows, 1)
	assert.Equal(t, domain.AuditActorAPIKey, recorder.rows[0].ActorType)
	assert.Equal(t, "u1", recorder.rows[0].ActorID)
}

func TestAuditMiddleware_LateEnrichmentIsIgnored(t *testing.T) {
	recorder := &captureRecorder{}
	var record *domain.AuditRecord
	handler := func(w http.ResponseWriter, r *http.Request) {
		record = domain.AuditFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}
	serve(t, recorder, handler, auditedRequest(http.MethodPost, "/api/lists.create", `{"workspace_id":"ws1"}`))
	record.SetTarget("list", "late", "")
	require.Len(t, recorder.rows, 1)
	assert.Empty(t, recorder.rows[0].TargetID)
}

func TestAuditMiddleware_RecorderPanicDoesNotBreakTheResponse(t *testing.T) {
	// The real recorder recovers its own panics; the middleware is also fine
	// with one that does not, because the response is already written.
	panicking := recorderFunc(func(context.Context, *domain.AuditLog) { panic("boom") })
	rec := httptest.NewRecorder()
	assert.Panics(t, func() {
		NewAuditMiddleware(panicking)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})).ServeHTTP(rec, auditedRequest(http.MethodPost, "/api/lists.create", `{}`))
	})
	assert.Equal(t, http.StatusCreated, rec.Code, "the client had its answer before the recorder ran")
}

type recorderFunc func(context.Context, *domain.AuditLog)

func (f recorderFunc) Record(ctx context.Context, log *domain.AuditLog) { f(ctx, log) }

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushes int
}

func (f *flushRecorder) Flush() { f.flushes++ }

func TestAuditResponseWriter_ForwardsFlush(t *testing.T) {
	inner := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler := func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "streaming handlers need the Flusher to survive the wrapper")
		_, _ = w.Write([]byte("chunk"))
		flusher.Flush()
	}
	NewAuditMiddleware(&captureRecorder{})(http.HandlerFunc(handler)).ServeHTTP(inner, auditedRequest(http.MethodPost, "/api/auditLogs.export", `{"workspace_id":"ws1"}`))
	assert.Equal(t, 1, inner.flushes)
	assert.Equal(t, "chunk", inner.Body.String())
}

func TestAuditResponseWriter_KeepsOnlyRefusalBodies(t *testing.T) {
	w := &auditResponseWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"big":"body"}`))
	assert.Zero(t, w.errorBody.Len(), "a success body is not kept")

	w = &auditResponseWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write(bytes.Repeat([]byte("x"), auditErrorBodyLimit+100))
	assert.Equal(t, auditErrorBodyLimit, w.errorBody.Len(), "and a refusal body is capped")
	w.WriteHeader(http.StatusOK)
	assert.Equal(t, http.StatusForbidden, w.status, "the first status wins")
}

func TestAuditMiddleware_401IsKeptWhenAServiceMarkedTheFailure(t *testing.T) {
	recorder := &captureRecorder{}
	handler := func(w http.ResponseWriter, r *http.Request) {
		domain.AuditFromContext(r.Context()).Fail("invalid_code")
		w.WriteHeader(http.StatusUnauthorized)
	}
	serve(t, recorder, handler, auditedRequest(http.MethodPost, "/api/user.verify", `{"email":"ann@example.com","code":"000000"}`))
	require.Len(t, recorder.rows, 1)
	assert.Equal(t, domain.AuditOutcomeFailure, recorder.rows[0].Outcome)
	assert.Equal(t, "invalid_code", recorder.rows[0].Metadata["reason"])
	assert.Equal(t, "ann@example.com", recorder.rows[0].TargetID)
	assert.Equal(t, http.StatusUnauthorized, recorder.rows[0].StatusCode)
}

func TestClientIP_IgnoresHeadersThatAreNotAddresses(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	req.Header.Set("X-Forwarded-For", strings.Repeat("a", 80))
	assert.Equal(t, "10.0.0.1", ClientIP(req), "a padded header cannot become the stored address")

	req.Header.Set("X-Forwarded-For", "203.0.113.9:8443, 10.0.0.2")
	assert.Equal(t, "203.0.113.9", ClientIP(req), "a port is stripped")

	req.Header.Set("X-Forwarded-For", "[2001:db8::1]:443")
	assert.Equal(t, "2001:db8::1", ClientIP(req))

	req.Header.Del("X-Forwarded-For")
	req.Header.Set("X-Real-IP", "not-an-ip")
	assert.Equal(t, "10.0.0.1", ClientIP(req))
}

func TestAuditMiddleware_TargetFromResponseAndNestedRequest(t *testing.T) {
	t.Run("a create that mints its id names it from the response", func(t *testing.T) {
		recorder := &captureRecorder{}
		handler := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"broadcast":{"id":"bc-1","name":"Launch"}}`))
		}
		serve(t, recorder, handler, auditedRequest(http.MethodPost, "/api/broadcasts.create", `{"workspace_id":"ws1","name":"Launch"}`))
		require.Len(t, recorder.rows, 1)
		assert.Equal(t, domain.AuditTargetBroadcast, recorder.rows[0].TargetType)
		assert.Equal(t, "bc-1", recorder.rows[0].TargetID)
	})

	t.Run("a service that named the target wins over the response", func(t *testing.T) {
		recorder := &captureRecorder{}
		handler := func(w http.ResponseWriter, r *http.Request) {
			domain.AuditFromContext(r.Context()).SetTarget(domain.AuditTargetBroadcast, "from-service", "Launch")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"broadcast":{"id":"bc-1"}}`))
		}
		serve(t, recorder, handler, auditedRequest(http.MethodPost, "/api/broadcasts.create", `{"workspace_id":"ws1"}`))
		require.Len(t, recorder.rows, 1)
		assert.Equal(t, "from-service", recorder.rows[0].TargetID)
	})

	t.Run("a failed create names nothing from its error body", func(t *testing.T) {
		recorder := &captureRecorder{}
		handler := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"broadcast":{"id":"should-not-be-read"}}`))
		}
		serve(t, recorder, handler, auditedRequest(http.MethodPost, "/api/broadcasts.create", `{"workspace_id":"ws1"}`))
		require.Len(t, recorder.rows, 1)
		assert.Empty(t, recorder.rows[0].TargetID)
	})

	t.Run("a nested request key is followed", func(t *testing.T) {
		recorder := &captureRecorder{}
		serve(t, recorder, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) },
			auditedRequest(http.MethodPost, "/api/automations.update", `{"workspace_id":"ws1","automation":{"id":"auto-1","name":"Welcome"}}`))
		require.Len(t, recorder.rows, 1)
		assert.Equal(t, "auto-1", recorder.rows[0].TargetID)
		assert.Equal(t, domain.AuditTargetAutomation, recorder.rows[0].TargetType)
	})
}

func TestLookupJSONPath(t *testing.T) {
	body := []byte(`{"a":{"b":{"c":"deep"}},"n":7,"arr":[1],"obj":{"x":1}}`)
	assert.Equal(t, "deep", lookupJSONPath(body, "a.b.c"))
	assert.Equal(t, "7", lookupJSONPath(body, "n"))
	assert.Empty(t, lookupJSONPath(body, "a.b"), "an object is not an id")
	assert.Empty(t, lookupJSONPath(body, "arr.0"))
	assert.Empty(t, lookupJSONPath(body, "missing.key"))
	assert.Empty(t, lookupJSONPath(body, ""))
	assert.Empty(t, lookupJSONPath([]byte(`not json`), "a"))
	assert.Empty(t, lookupJSONPath(nil, "a"))
}
