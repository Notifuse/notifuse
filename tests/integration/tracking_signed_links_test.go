package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
	"github.com/Notifuse/notifuse/tests/testutil"
)

var deliveredClickLinkRe = regexp.MustCompile(`href="([^"]*/r/([A-Za-z0-9_-]+))"`)

// deliveredClickLink finds the tracked link of a delivered email whose destination starts
// with destPrefix, and returns it with its parsed token. The link is used as delivered.
func deliveredClickLink(t *testing.T, html, destPrefix string) (string, trackingtoken.Token) {
	t.Helper()
	for _, m := range deliveredClickLinkRe.FindAllStringSubmatch(html, -1) {
		tok, err := trackingtoken.Parse(m[2], trackingtoken.PurposeClick)
		require.NoError(t, err)
		if strings.HasPrefix(tok.URL, destPrefix) {
			return m[1], tok
		}
	}
	t.Fatalf("no tracked link to %s in the delivered email: %s", destPrefix, html)
	return "", trackingtoken.Token{}
}

func clickDelivered(t *testing.T, link string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, link, nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15")
	resp, err := noRedirectClient().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// assertDeliveredLinkIsSigned clicks the delivered link to an external destination and
// checks it redirects there directly: only a link signed with its workspace's key does.
// A same-host link would redirect even unsigned, so it proves nothing here.
func assertDeliveredLinkIsSigned(t *testing.T, html, workspaceID, destPrefix string) {
	t.Helper()
	link, tok := deliveredClickLink(t, html, destPrefix)
	assert.Equal(t, workspaceID, tok.WorkspaceID)
	assert.NotEmpty(t, tok.Tag, "the link must be signed")
	assertTrackedRedirect(t, clickDelivered(t, link), tok.URL)
}

// TestTrackedLinksAreSignedOnEverySendPath sends a tracked email through each send path
// and clicks the delivered link to another site: it must redirect without a confirmation
// page, which only a signed link does. A compile site that forgot the signing key would
// fail here. The scheduled broadcast path is covered by TestLinkClickMailpitE2E, the
// transactional API by TestWebAnalyticsEmailIdentify.
func TestTrackedLinksAreSignedOnEverySendPath(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(workerCtx))

	client := suite.APIClient
	factory := suite.DataFactory
	serverURL := suite.ServerManager.GetURL()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, factory.EnableEmailTracking(workspace.ID))
	integration, err := factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	// Absolute links, clicked exactly as delivered.
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, serverURL))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	const dest = "https://shop.example.com/signed-path"
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateSubject("Signed links "+uuid.New().String()[:8]),
		testutil.WithCodeModeTemplate(`<mjml><mj-body><mj-section><mj-column>`+
			`<mj-text><a href="`+dest+`?utm_source=test">Shop</a></mj-text>`+
			`</mj-column></mj-section></mj-body></mjml>`))
	require.NoError(t, err)

	require.NoError(t, testutil.ClearMailpitMessages(t))

	newEmail := func(name string) string {
		return fmt.Sprintf("signed-%s-%s@example.com", name, uuid.New().String()[:8])
	}
	postOK := func(t *testing.T, path string, body map[string]interface{}) {
		t.Helper()
		resp, err := client.Post(path, body)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", path, raw)
	}

	t.Run("transactional template test email", func(t *testing.T) {
		email := newEmail("test-template")
		postOK(t, "/api/transactional.testTemplate", map[string]interface{}{
			"workspace_id":    workspace.ID,
			"template_id":     template.ID,
			"integration_id":  integration.ID,
			"sender_id":       integration.EmailProvider.Senders[0].ID,
			"recipient_email": email,
		})
		msg, err := waitForEmailAndGetMessage(t, email, 20*time.Second)
		require.NoError(t, err)
		assertDeliveredLinkIsSigned(t, msg.HTML, workspace.ID, dest)
	})

	t.Run("broadcast test send", func(t *testing.T) {
		email := newEmail("send-to-individual")
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastAudience(domain.AudienceSettings{List: list.ID}))
		require.NoError(t, err)
		postOK(t, "/api/broadcasts.sendToIndividual", map[string]interface{}{
			"workspace_id":    workspace.ID,
			"broadcast_id":    broadcast.ID,
			"recipient_email": email,
			"template_id":     template.ID,
		})
		msg, err := waitForEmailAndGetMessage(t, email, 20*time.Second)
		require.NoError(t, err)
		assertDeliveredLinkIsSigned(t, msg.HTML, workspace.ID, dest)
	})

	t.Run("automation email node", func(t *testing.T) {
		email := newEmail("automation")
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		eventName := "signed_links_" + uuid.New().String()[:8]
		createActiveAutomation(t, client, workspace.ID, shortuuid.New(), list.ID, eventName, template.ID)
		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, map[string]interface{}{}))
		msg, err := waitForEmailAndGetMessage(t, email, 30*time.Second)
		require.NoError(t, err)
		assertDeliveredLinkIsSigned(t, msg.HTML, workspace.ID, dest)
	})

	t.Run("list double opt-in confirmation", func(t *testing.T) {
		email := newEmail("double-optin")
		list, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true), testutil.WithListDoubleOptin(true),
			func(l *domain.List) {
				l.DoubleOptInTemplate = &domain.TemplateReference{ID: template.ID, Version: template.Version}
			})
		require.NoError(t, err)
		body, err := json.Marshal(map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      map[string]interface{}{"email": email},
			"list_ids":     []string{list.ID},
		})
		require.NoError(t, err)
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Post(serverURL+"/subscribe", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "/subscribe: %s", raw)

		msg, err := waitForEmailAndGetMessage(t, email, 20*time.Second)
		require.NoError(t, err)
		assertDeliveredLinkIsSigned(t, msg.HTML, workspace.ID, dest)
	})

	t.Run("a delivered link of a deleted workspace shows the interstitial", func(t *testing.T) {
		// Its own workspace, so deleting it leaves the others alone.
		other, err := factory.CreateWorkspace()
		require.NoError(t, err)
		require.NoError(t, factory.AddUserToWorkspace(user.ID, other.ID, "owner"))
		require.NoError(t, factory.EnableEmailTracking(other.ID))
		otherIntegration, err := factory.SetupWorkspaceWithSMTPProvider(other.ID)
		require.NoError(t, err)
		require.NoError(t, factory.SetWorkspaceCustomEndpointURL(other.ID, serverURL))
		otherTemplate, err := factory.CreateTemplate(other.ID,
			testutil.WithTemplateSubject("Deleted workspace "+uuid.New().String()[:8]),
			testutil.WithCodeModeTemplate(`<mjml><mj-body><mj-section><mj-column>`+
				`<mj-text><a href="`+dest+`">Shop</a></mj-text>`+
				`</mj-column></mj-section></mj-body></mjml>`))
		require.NoError(t, err)

		email := newEmail("deleted-workspace")
		postOK(t, "/api/transactional.testTemplate", map[string]interface{}{
			"workspace_id":    other.ID,
			"template_id":     otherTemplate.ID,
			"integration_id":  otherIntegration.ID,
			"sender_id":       otherIntegration.EmailProvider.Senders[0].ID,
			"recipient_email": email,
		})
		msg, err := waitForEmailAndGetMessage(t, email, 20*time.Second)
		require.NoError(t, err)
		link, tok := deliveredClickLink(t, msg.HTML, dest)
		require.NotEmpty(t, tok.Tag)
		assertTrackedRedirect(t, clickDelivered(t, link), tok.URL)

		postOK(t, "/api/workspaces.delete", map[string]interface{}{"id": other.ID})

		// The click route caches a workspace for minutes; a restart drops the cache, as a
		// deletion followed by any deploy would.
		require.NoError(t, suite.ServerManager.Restart())
		link = strings.Replace(link, serverURL, suite.ServerManager.GetURL(), 1)
		assertTrackingInterstitial(t, clickDelivered(t, link), "shop.example.com")
	})
}

// TestTrackingRoutesForUnknownWorkspaceIDs names workspaces that do not exist on the
// public routes: they answer as before, and a workspace created right after its ID was
// probed is usable at once.
func TestTrackingRoutesForUnknownWorkspaceIDs(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	baseURL := suite.ServerManager.GetURL()
	factory := suite.DataFactory
	browser := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0"
	get := func(t *testing.T, path string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", browser)
		resp, err := noRedirectClient().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	t.Run("routes answer for random workspace IDs", func(t *testing.T) {
		for i := range 3 {
			wid := fmt.Sprintf("ghost%d%s", i, uuid.New().String()[:8])

			token, err := trackingtoken.MintOpen(nil, "msg-1", wid, time.Now().Add(-time.Minute).Unix())
			require.NoError(t, err)
			resp := get(t, "/t/"+token)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))

			resp = get(t, "/visit?"+url.Values{"url": {"https://example.com/"}, "mid": {"msg-1"}, "wid": {wid}}.Encode())
			assertTrackingInterstitial(t, resp, "example.com")

			// These two answer 500 for an unknown workspace, as they did before the
			// connection manager refused unknown IDs: both load the workspace before
			// any connection. Pinned so the guard is seen not to change them.
			resp = get(t, "/preferences?"+url.Values{"workspace_id": {wid}, "email": {"a@example.com"}, "email_hmac": {"x"}}.Encode())
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "GET /preferences for an unknown workspace")

			form := url.Values{"List-Unsubscribe": {"One-Click"}}
			oneClick := "/unsubscribe-oneclick?" + url.Values{"wid": {wid}, "email": {"a@example.com"}, "email_hmac": {"x"}, "lids": {"l1"}}.Encode()
			resp2, err := http.Post(baseURL+oneClick, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
			require.NoError(t, err)
			_ = resp2.Body.Close()
			assert.Equal(t, http.StatusInternalServerError, resp2.StatusCode, "one-click unsubscribe for an unknown workspace")
		}
	})

	t.Run("a workspace created right after its ID was probed is usable at once", func(t *testing.T) {
		wid := "probed" + uuid.New().String()[:8]
		token, err := trackingtoken.MintOpen(nil, "msg-1", wid, time.Now().Add(-time.Minute).Unix())
		require.NoError(t, err)
		// Each of these reaches the connection manager, which caches that wid has no row.
		_ = get(t, "/t/"+token)
		_ = get(t, "/preferences?"+url.Values{"workspace_id": {wid}, "email": {"a@example.com"}, "email_hmac": {"x"}}.Encode())

		_, err = factory.CreateWorkspace(func(w *domain.Workspace) { w.ID = wid })
		require.NoError(t, err)
		// Within the not-found cache's lifetime: works only if creation forgot the miss.
		_, err = factory.CreateContact(wid)
		require.NoError(t, err, "the new workspace's database must be reachable at once")
	})
}

// TestTemplateCompileLinksAreUnsignedAndPinned compiles through the API, as a client
// would, with tracking settings naming another workspace and a signing key: the links
// are minted for the caller's workspace and unsigned, so an external one shows the
// interstitial.
func TestTemplateCompileLinksAreUnsignedAndPinned(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory
	serverURL := suite.ServerManager.GetURL()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	victim, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	resp, err := client.Post("/api/templates.compile", map[string]interface{}{
		"workspace_id": workspace.ID,
		"message_id":   "preview",
		"mjml_source": `<mjml><mj-body><mj-section><mj-column>` +
			`<mj-text><a href="https://evil.example/phish">Shop</a></mj-text>` +
			`</mj-column></mj-section></mj-body></mjml>`,
		"tracking_settings": map[string]interface{}{
			"enable_tracking": true,
			"endpoint":        serverURL,
			"workspace_id":    victim.ID,
			"signing_key":     "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		},
	})
	require.NoError(t, err)
	raw, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", raw)

	var compiled struct {
		HTML string `json:"html"`
	}
	require.NoError(t, json.Unmarshal(raw, &compiled))
	link, tok := deliveredClickLink(t, compiled.HTML, "https://evil.example/")
	assert.Equal(t, workspace.ID, tok.WorkspaceID, "links record against the caller's workspace")
	assert.Empty(t, tok.Tag, "preview links are never signed")
	assertTrackingInterstitial(t, clickDelivered(t, link), "evil.example")
}
