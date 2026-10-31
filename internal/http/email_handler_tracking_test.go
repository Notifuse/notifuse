package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain/mocks"
	httpmocks "github.com/Notifuse/notifuse/internal/http/mocks"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
)

const (
	trackingTestUA        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0"
	trackingTestBotUA     = "Googlebot/2.1 (+http://www.google.com/bot.html)"
	trackingTestReqHost   = "track.notifuse.test"
	trackingTestMessageID = "message-123"
	trackingTestWorkspace = "workspace-123"
)

func trackingTestKey(t *testing.T, secret string) []byte {
	t.Helper()
	key, err := trackingtoken.DeriveKey(secret)
	require.NoError(t, err)
	return key
}

func oldTS() int64   { return time.Now().Add(-10 * time.Second).Unix() }
func freshTS() int64 { return time.Now().Unix() }

func signedClickToken(t *testing.T, key []byte, ts int64, dest string) string {
	t.Helper()
	token, err := trackingtoken.MintClick(key, trackingTestMessageID, trackingTestWorkspace, ts, dest)
	require.NoError(t, err)
	return token
}

func legacyClickToken(t *testing.T, ts int64, dest string) string {
	t.Helper()
	token, err := crypto.EncryptTrackingToken(trackingTestMessageID + "\n" + trackingTestWorkspace + "\n" + strconv.FormatInt(ts, 10) + "\n" + dest)
	require.NoError(t, err)
	return token
}

func clickRequest(handler *EmailHandler, token, host, userAgent string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/r/"+token, nil)
	req.Host = host
	req.Header.Set("User-Agent", userAgent)
	w := httptest.NewRecorder()
	handler.handleEncryptedClick(w, req.WithContext(context.Background()))
	return w
}

func visitRequest(handler *EmailHandler, query url.Values, host string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/visit?"+query.Encode(), nil)
	req.Host = host
	req.Header.Set("User-Agent", trackingTestUA)
	w := httptest.NewRecorder()
	handler.handleClickRedirection(w, req.WithContext(context.Background()))
	return w
}

func expectLookup(ws *httpmocks.MockTrackingWorkspaces, workspace trackingtoken.Workspace, status trackingtoken.WorkspaceStatus) {
	ws.EXPECT().Lookup(gomock.Any(), trackingTestWorkspace).Return(workspace, status)
}

func expectVisit(svc *mocks.MockEmailServiceInterface, clickedURL string) {
	svc.EXPECT().VisitLink(gomock.Any(), trackingTestMessageID, trackingTestWorkspace, clickedURL, gomock.Any()).Return(nil)
}

func assertInterstitial(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Location"))
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
}

func TestEncryptedClick_VerifiedTokenRedirectsToMintedDestination(t *testing.T) {
	key := trackingTestKey(t, "workspace-secret")
	for _, dest := range []string{
		"https://shop.example.com/product?a=1",
		"https://shop.com/50%off",
		"//shop.com/x",
		"myapp://x",
	} {
		t.Run(dest, func(t *testing.T) {
			svc, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
			expectLookup(ws, trackingtoken.Workspace{Key: key}, trackingtoken.WorkspaceKnown)
			expectVisit(svc, dest)

			w := clickRequest(handler, signedClickToken(t, key, oldTS(), dest), trackingTestReqHost, trackingTestUA)
			assert.Equal(t, http.StatusSeeOther, w.Code)
			assert.Equal(t, dest, w.Header().Get("Location"))
		})
	}

	t.Run("on another request host too, the tag binds no host", func(t *testing.T) {
		svc, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		expectLookup(ws, trackingtoken.Workspace{Key: key}, trackingtoken.WorkspaceKnown)
		expectVisit(svc, "https://shop.example.com/x")

		w := clickRequest(handler, signedClickToken(t, key, oldTS(), "https://shop.example.com/x"), "other-host.example:8080", trackingTestUA)
		assert.Equal(t, http.StatusSeeOther, w.Code)
		assert.Equal(t, "https://shop.example.com/x", w.Header().Get("Location"))
	})

	t.Run("an empty destination is still a 400", func(t *testing.T) {
		_, _, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		w := clickRequest(handler, signedClickToken(t, key, oldTS(), ""), trackingTestReqHost, trackingTestUA)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "Missing redirect URL\n", w.Body.String())
	})
}

func TestEncryptedClick_UnverifiableTokensShowInterstitial(t *testing.T) {
	key := trackingTestKey(t, "workspace-secret")
	const dest = "https://evil.example/landing"

	t.Run("wrong tag, with exactly one refresh", func(t *testing.T) {
		svc, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		expectLookup(ws, trackingtoken.Workspace{Key: key}, trackingtoken.WorkspaceKnown)
		ws.EXPECT().Refresh(gomock.Any(), trackingTestWorkspace).Return(trackingtoken.Workspace{Key: key}, trackingtoken.WorkspaceKnown).Times(1)
		expectVisit(svc, dest)

		forged := signedClickToken(t, trackingTestKey(t, "attacker-key"), oldTS(), dest)
		assertInterstitial(t, clickRequest(handler, forged, trackingTestReqHost, trackingTestUA))
	})

	t.Run("refresh picks up a re-created workspace's key", func(t *testing.T) {
		svc, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		newKey := trackingTestKey(t, "recreated-secret")
		expectLookup(ws, trackingtoken.Workspace{Key: key}, trackingtoken.WorkspaceKnown)
		ws.EXPECT().Refresh(gomock.Any(), trackingTestWorkspace).Return(trackingtoken.Workspace{Key: newKey}, trackingtoken.WorkspaceKnown)
		expectVisit(svc, dest)

		w := clickRequest(handler, signedClickToken(t, newKey, oldTS(), dest), trackingTestReqHost, trackingTestUA)
		assert.Equal(t, http.StatusSeeOther, w.Code)
		assert.Equal(t, dest, w.Header().Get("Location"))
	})

	t.Run("unsigned", func(t *testing.T) {
		svc, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		expectLookup(ws, trackingtoken.Workspace{Key: key}, trackingtoken.WorkspaceKnown)
		expectVisit(svc, dest)
		assertInterstitial(t, clickRequest(handler, signedClickToken(t, nil, oldTS(), dest), trackingTestReqHost, trackingTestUA))
	})

	t.Run("legacy", func(t *testing.T) {
		svc, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		expectLookup(ws, trackingtoken.Workspace{Key: key}, trackingtoken.WorkspaceKnown)
		expectVisit(svc, dest)
		assertInterstitial(t, clickRequest(handler, legacyClickToken(t, oldTS(), dest), trackingTestReqHost, trackingTestUA))
	})

	t.Run("unknown workspace records nothing", func(t *testing.T) {
		for name, token := range map[string]string{
			"signed": signedClickToken(t, key, oldTS(), dest),
			"legacy": legacyClickToken(t, oldTS(), dest),
		} {
			_, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
			expectLookup(ws, trackingtoken.Workspace{}, trackingtoken.WorkspaceUnknown)
			// No VisitLink expectation: gomock fails the test on a call.
			w := clickRequest(handler, token, trackingTestReqHost, trackingTestUA)
			assert.Equal(t, http.StatusOK, w.Code, name)
			assert.Empty(t, w.Header().Get("Location"), name)
		}
	})

	t.Run("workspace unavailable still records", func(t *testing.T) {
		svc, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		expectLookup(ws, trackingtoken.Workspace{}, trackingtoken.WorkspaceUnavailable)
		expectVisit(svc, dest)
		assertInterstitial(t, clickRequest(handler, signedClickToken(t, key, oldTS(), dest), trackingTestReqHost, trackingTestUA))
	})
}

func TestEncryptedClick_LegacyToOwnHostsRedirects(t *testing.T) {
	cases := map[string]struct {
		dest     string
		endpoint string
		want     string
	}{
		"request host":           {dest: "https://" + trackingTestReqHost + "/unsubscribe-oneclick?x=1", want: "https://" + trackingTestReqHost + "/unsubscribe-oneclick?x=1"},
		"API_ENDPOINT host":      {dest: "https://api.notifuse.test/notification-center?x=1", want: "https://api.notifuse.test/notification-center?x=1"},
		"custom endpoint host":   {dest: "https://links.brand.com/u", endpoint: "links.brand.com", want: "https://links.brand.com/u"},
		"surrounding whitespace": {dest: " https://" + trackingTestReqHost + "/u\n", want: "https://" + trackingTestReqHost + "/u"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
			expectLookup(ws, trackingtoken.Workspace{Key: trackingTestKey(t, "s"), EndpointHost: tc.endpoint}, trackingtoken.WorkspaceKnown)
			expectVisit(svc, tc.dest)

			w := clickRequest(handler, legacyClickToken(t, oldTS(), tc.dest), trackingTestReqHost, trackingTestUA)
			assert.Equal(t, http.StatusSeeOther, w.Code)
			assert.Equal(t, tc.want, w.Header().Get("Location"))
		})
	}
}

func TestEncryptedClick_RejectedDestinationsRecordNothing(t *testing.T) {
	for _, dest := range []string{"javascript:alert(1)", `/\evil.com`, "?x=1"} {
		t.Run(dest, func(t *testing.T) {
			_, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
			expectLookup(ws, trackingtoken.Workspace{Key: trackingTestKey(t, "s")}, trackingtoken.WorkspaceKnown)
			w := clickRequest(handler, legacyClickToken(t, oldTS(), dest), trackingTestReqHost, trackingTestUA)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Empty(t, w.Header().Get("Location"))
		})
	}
}

func TestEncryptedClick_InvalidTokens(t *testing.T) {
	_, _, _, handler := setupEmailHandlerTestWithWorkspaces(t)
	assert.Equal(t, http.StatusBadRequest, clickRequest(handler, "", trackingTestReqHost, trackingTestUA).Code)
	assert.Equal(t, http.StatusBadRequest, clickRequest(handler, "invalid-token", trackingTestReqHost, trackingTestUA).Code)

	threeFields, err := crypto.EncryptTrackingToken("message-123\nworkspace-123\n1730000000")
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, clickRequest(handler, threeFields, trackingTestReqHost, trackingTestUA).Code)
}

func TestEncryptedClick_BotChecks(t *testing.T) {
	key := trackingTestKey(t, "workspace-secret")
	const dest = "https://shop.example.com/x"
	tokens := map[string]func(ts int64) string{
		"signed": func(ts int64) string { return signedClickToken(t, key, ts, dest) },
		"legacy": func(ts int64) string { return legacyClickToken(t, ts, dest) },
	}
	for name, mint := range tokens {
		t.Run(name+" too fast records nothing", func(t *testing.T) {
			_, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
			ws.EXPECT().Lookup(gomock.Any(), trackingTestWorkspace).Return(trackingtoken.Workspace{Key: key}, trackingtoken.WorkspaceKnown).AnyTimes()
			ws.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return(trackingtoken.Workspace{Key: key}, trackingtoken.WorkspaceKnown).AnyTimes()
			clickRequest(handler, mint(freshTS()), trackingTestReqHost, trackingTestUA)
		})
		t.Run(name+" bot user agent records nothing", func(t *testing.T) {
			_, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
			ws.EXPECT().Lookup(gomock.Any(), trackingTestWorkspace).Return(trackingtoken.Workspace{Key: key}, trackingtoken.WorkspaceKnown).AnyTimes()
			clickRequest(handler, mint(oldTS()), trackingTestReqHost, trackingTestBotUA)
		})
		t.Run(name+" old enough records", func(t *testing.T) {
			svc, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
			ws.EXPECT().Lookup(gomock.Any(), trackingTestWorkspace).Return(trackingtoken.Workspace{Key: key}, trackingtoken.WorkspaceKnown).AnyTimes()
			expectVisit(svc, dest)
			clickRequest(handler, mint(oldTS()), trackingTestReqHost, trackingTestUA)
		})
	}
}

func TestEncryptedOpen_NeverConsultsWorkspaces(t *testing.T) {
	key := trackingTestKey(t, "workspace-secret")
	mintOpen := func(t *testing.T, k []byte, ts int64) string {
		token, err := trackingtoken.MintOpen(k, trackingTestMessageID, trackingTestWorkspace, ts)
		require.NoError(t, err)
		return token
	}
	tokens := map[string]func(ts int64) string{
		"legacy":    func(ts int64) string { return mintOpen(t, nil, ts) },
		"signed":    func(ts int64) string { return mintOpen(t, key, ts) },
		"wrong tag": func(ts int64) string { return mintOpen(t, trackingTestKey(t, "attacker-key"), ts) },
	}
	for name, mint := range tokens {
		t.Run(name+" records", func(t *testing.T) {
			// setupEmailHandlerTest's workspace source has no expectations.
			svc, _, handler, _ := setupEmailHandlerTest(t)
			svc.EXPECT().OpenEmail(gomock.Any(), trackingTestMessageID, trackingTestWorkspace).Return(nil)
			assertTrackingPixel(t, coverageEncryptedOpenRequest(t, handler, "/t/"+mint(oldTS()), trackingTestUA))
		})
		t.Run(name+" too fast records nothing", func(t *testing.T) {
			_, _, handler, _ := setupEmailHandlerTest(t)
			assertTrackingPixel(t, coverageEncryptedOpenRequest(t, handler, "/t/"+mint(freshTS()), trackingTestUA))
		})
	}
}

func TestVisit_Classification(t *testing.T) {
	t.Run("without mid and wid: interstitial, nothing recorded, no lookup", func(t *testing.T) {
		_, _, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		w := visitRequest(handler, url.Values{"url": {"https://evil.example"}}, trackingTestReqHost)
		assertInterstitial(t, w)
	})

	t.Run("same host, known workspace: 303 and aggregate click", func(t *testing.T) {
		svc, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		expectLookup(ws, trackingtoken.Workspace{Key: trackingTestKey(t, "s")}, trackingtoken.WorkspaceKnown)
		expectVisit(svc, "")
		dest := "https://" + trackingTestReqHost + "/notification-center?x=1"
		w := visitRequest(handler, url.Values{
			"url": {dest}, "mid": {trackingTestMessageID}, "wid": {trackingTestWorkspace},
			"ts": {strconv.FormatInt(oldTS(), 10)},
		}, trackingTestReqHost)
		assert.Equal(t, http.StatusSeeOther, w.Code)
		assert.Equal(t, dest, w.Header().Get("Location"))
	})

	t.Run("external host, known workspace: interstitial and aggregate click", func(t *testing.T) {
		svc, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		expectLookup(ws, trackingtoken.Workspace{Key: trackingTestKey(t, "s")}, trackingtoken.WorkspaceKnown)
		expectVisit(svc, "")
		assertInterstitial(t, visitRequest(handler, url.Values{
			"url": {"https://evil.example"}, "mid": {trackingTestMessageID}, "wid": {trackingTestWorkspace},
		}, trackingTestReqHost))
	})

	t.Run("unknown workspace records nothing", func(t *testing.T) {
		_, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		expectLookup(ws, trackingtoken.Workspace{}, trackingtoken.WorkspaceUnknown)
		assertInterstitial(t, visitRequest(handler, url.Values{
			"url": {"https://evil.example"}, "mid": {trackingTestMessageID}, "wid": {trackingTestWorkspace},
		}, trackingTestReqHost))
	})

	t.Run("too fast records nothing", func(t *testing.T) {
		_, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		expectLookup(ws, trackingtoken.Workspace{Key: trackingTestKey(t, "s")}, trackingtoken.WorkspaceKnown)
		assertInterstitial(t, visitRequest(handler, url.Values{
			"url": {"https://evil.example"}, "mid": {trackingTestMessageID}, "wid": {trackingTestWorkspace},
			"ts": {strconv.FormatInt(freshTS(), 10)},
		}, trackingTestReqHost))
	})

	t.Run("missing url is a 400", func(t *testing.T) {
		_, _, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		w := visitRequest(handler, url.Values{"mid": {trackingTestMessageID}, "wid": {trackingTestWorkspace}}, trackingTestReqHost)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "Missing redirect URL\n", w.Body.String())
	})

	t.Run("javascript destination is a 400", func(t *testing.T) {
		_, _, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		w := visitRequest(handler, url.Values{"url": {"javascript:alert(1)"}}, trackingTestReqHost)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("relative destination is a 400, never resolved", func(t *testing.T) {
		_, _, _, handler := setupEmailHandlerTestWithWorkspaces(t)
		w := visitRequest(handler, url.Values{"url": {`/\evil.com`}}, trackingTestReqHost)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.False(t, strings.Contains(w.Header().Get("Location"), "evil"))
	})
}

func TestEncryptedClick_VerifiedTokenToBlockedSchemeIsRefused(t *testing.T) {
	key := trackingTestKey(t, "workspace-secret")
	for _, dest := range []string{" javascript:alert(1)", "java\tscript:alert(1)", "vbscript:x", "data:text/html,x", "JavaScript:x"} {
		t.Run(dest, func(t *testing.T) {
			_, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
			expectLookup(ws, trackingtoken.Workspace{Key: key}, trackingtoken.WorkspaceKnown)
			// No VisitLink expectation: nothing is recorded.
			w := clickRequest(handler, signedClickToken(t, key, oldTS(), dest), trackingTestReqHost, trackingTestUA)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Empty(t, w.Header().Get("Location"))
			assert.NotContains(t, w.Body.String(), "script")
		})
	}
}

func TestVisit_RefusedDestinationCostsNoLookup(t *testing.T) {
	// No Lookup expectation: gomock fails the test if the workspace is looked up.
	_, _, _, handler := setupEmailHandlerTestWithWorkspaces(t)
	w := visitRequest(handler, url.Values{
		"url": {"javascript:alert(1)"}, "mid": {trackingTestMessageID}, "wid": {trackingTestWorkspace},
	}, trackingTestReqHost)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestVisit_WorkspaceCustomDomainRedirects(t *testing.T) {
	svc, ws, _, handler := setupEmailHandlerTestWithWorkspaces(t)
	expectLookup(ws, trackingtoken.Workspace{Key: trackingTestKey(t, "s"), EndpointHost: "links.brand.com"}, trackingtoken.WorkspaceKnown)
	expectVisit(svc, "")
	w := visitRequest(handler, url.Values{
		"url": {"https://links.brand.com/notification-center?x=1"}, "mid": {trackingTestMessageID}, "wid": {trackingTestWorkspace},
	}, trackingTestReqHost)
	assert.Equal(t, http.StatusSeeOther, w.Code)
	assert.Equal(t, "https://links.brand.com/notification-center?x=1", w.Header().Get("Location"))
}

func TestNewEmailHandler_SchemelessAPIEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	handler := NewEmailHandler(mocks.NewMockEmailServiceInterface(ctrl), nil, nil, "", httpmocks.NewMockTrackingWorkspaces(ctrl), "api.example.com")
	assert.Equal(t, "api.example.com", handler.apiHost)

	// A legacy link to the API host then still redirects behind a proxy that rewrites Host.
	decision, target, _, _ := classifyTrackedDestination("https://api.example.com/notification-center", "internal:8080", handler.apiHost, "")
	assert.Equal(t, decisionRedirect, decision)
	assert.Equal(t, "https://api.example.com/notification-center", target)
}

func TestLegacyOpens_BotChecks(t *testing.T) {
	open := func(handler *EmailHandler, ts int64, ua string) {
		q := url.Values{"mid": {trackingTestMessageID}, "wid": {trackingTestWorkspace}, "ts": {strconv.FormatInt(ts, 10)}}
		req := httptest.NewRequest(http.MethodGet, "/opens?"+q.Encode(), nil)
		req.Header.Set("User-Agent", ua)
		handler.handleOpens(httptest.NewRecorder(), req)
	}
	t.Run("old enough records", func(t *testing.T) {
		svc, _, handler, _ := setupEmailHandlerTest(t)
		svc.EXPECT().OpenEmail(gomock.Any(), trackingTestMessageID, trackingTestWorkspace).Return(nil)
		open(handler, oldTS(), trackingTestUA)
	})
	t.Run("too fast records nothing", func(t *testing.T) {
		_, _, handler, _ := setupEmailHandlerTest(t)
		open(handler, freshTS(), trackingTestUA)
	})
	t.Run("bot records nothing", func(t *testing.T) {
		_, _, handler, _ := setupEmailHandlerTest(t)
		open(handler, oldTS(), trackingTestBotUA)
	})
}
