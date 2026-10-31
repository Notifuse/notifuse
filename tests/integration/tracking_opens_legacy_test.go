package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// legacyToken seals a plaintext the way 42.1 minted tokens: no signature, the timestamp
// written with %d.
func legacyToken(t *testing.T, fields ...string) string {
	t.Helper()
	token, err := crypto.EncryptTrackingToken(strings.Join(fields, "\n"))
	require.NoError(t, err)
	return token
}

func oldTimestamp() string {
	return strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)
}

// trackingPathOf returns the path and query of an absolute tracking URL found in an
// email, so it can be requested on the test server whatever host the email names.
func trackingPathOf(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u.RequestURI()
}

func fetchFromBrowser(t *testing.T, fullURL string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15")
	resp, err := noRedirectClient().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func messageEngagement(t *testing.T, db *sql.DB, messageID string) (opened, clicked bool) {
	t.Helper()
	var openedAt, clickedAt sql.NullTime
	err := db.QueryRowContext(context.Background(),
		`SELECT opened_at, clicked_at FROM message_history WHERE id = $1`, messageID).Scan(&openedAt, &clickedAt)
	require.NoError(t, err)
	return openedAt.Valid, clickedAt.Valid
}

// TestTrackingOpensAndLegacyLinksOnRealMessages covers what the click tests do not:
// opens from the pixel and from {{ tracking_opens_url }} of a delivered email, and links
// minted before signing (built here in the format 42.1 wrote) for real messages,
// including those leading to the workspace's custom domain.
func TestTrackingOpensAndLegacyLinksOnRealMessages(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory
	serverURL := suite.ServerManager.GetURL() // http://127.0.0.1:<port>
	port := serverURL[strings.LastIndexByte(serverURL, ':')+1:]
	// Another name for the same server, so the custom domain differs from the host the
	// test requests: a destination on it can only redirect through the custom-domain rule.
	customEndpoint := "http://localhost:" + port

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, factory.EnableEmailTracking(workspace.ID))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, customEndpoint))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateSubject("Opens "+uuid.New().String()[:8]),
		testutil.WithCodeModeTemplate(`<mjml><mj-body><mj-section><mj-column>`+
			`<mj-text><a href="https://shop.example.com/opens">Shop</a></mj-text>`+
			`<mj-raw><img alt="opens-variable" src="{{ tracking_opens_url }}"></mj-raw>`+
			`</mj-column></mj-section></mj-body></mjml>`))
	require.NoError(t, err)
	notification, err := factory.CreateTransactionalNotification(workspace.ID,
		testutil.WithTransactionalNotificationID("opens-"+uuid.New().String()[:8]),
		testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{TemplateID: template.ID, Settings: map[string]interface{}{}},
		}))
	require.NoError(t, err)
	require.NoError(t, testutil.ClearMailpitMessages(t))

	// send delivers the notification to a new address and returns its message ID and the
	// delivered HTML, once the message_history row exists.
	send := func(t *testing.T) (string, string) {
		t.Helper()
		email := fmt.Sprintf("opens-%s@example.com", uuid.New().String()[:8])
		resp, err := client.SendTransactionalNotification(map[string]interface{}{
			"id":       notification.ID,
			"contact":  map[string]interface{}{"email": email},
			"channels": []string{"email"},
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
		var result struct {
			MessageID string `json:"message_id"`
		}
		require.NoError(t, json.Unmarshal(body, &result))
		require.NotEmpty(t, result.MessageID)
		msg, err := waitForEmailAndGetMessage(t, email, 20*time.Second)
		require.NoError(t, err)
		testutil.WaitForCondition(t, func() bool {
			var n int
			_ = db.QueryRow(`SELECT COUNT(*) FROM message_history WHERE id = $1`, result.MessageID).Scan(&n)
			return n == 1
		}, 10*time.Second, "message_history row of "+result.MessageID)
		return result.MessageID, msg.HTML
	}

	pixelMessage, pixelHTML := send(t)
	variableMessage, variableHTML := send(t)

	pixelRe := regexp.MustCompile(`<td style="line-height:0px;font-size:0px;mso-line-height-rule:exactly;"><img src="([^"]+/t/[^"]+)"`)
	variableRe := regexp.MustCompile(`<img alt="opens-variable" src="([^"]+)"`)
	pixel := pixelRe.FindStringSubmatch(pixelHTML)
	require.NotNil(t, pixel, "the delivered email must carry the open pixel: %s", pixelHTML)
	variable := variableRe.FindStringSubmatch(variableHTML)
	require.NotNil(t, variable, "{{ tracking_opens_url }} must render: %s", variableHTML)
	assert.NotContains(t, variable[1], "/opens", "tracking_opens_url is no longer an /opens query URL")

	for name, c := range map[string]struct{ url, messageID string }{
		"pixel":              {pixel[1], pixelMessage},
		"tracking_opens_url": {variable[1], variableMessage},
	} {
		path := trackingPathOf(t, c.url)
		require.True(t, strings.HasPrefix(path, "/t/"), "%s: %s", name, path)
		tok, err := trackingtoken.Parse(strings.TrimPrefix(path, "/t/"), trackingtoken.PurposeOpen)
		require.NoError(t, err, name)
		assert.Equal(t, c.messageID, tok.MessageID, name)
		assert.Equal(t, workspace.ID, tok.WorkspaceID, name)
		assert.NotEmpty(t, tok.Tag, "%s must be signed", name)
	}

	// An open within 7 seconds of the send is taken for a prefetch, as before: the bot
	// check reads the timestamp before the signature. The last message sent is the
	// freshest.
	resp := fetchFromBrowser(t, serverURL+trackingPathOf(t, variable[1]))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	time.Sleep(500 * time.Millisecond)
	opened, _ := messageEngagement(t, db, variableMessage)
	assert.False(t, opened, "an open right after the send is not recorded")

	time.Sleep(8 * time.Second)

	t.Run("the delivered pixel records the open", func(t *testing.T) {
		resp := fetchFromBrowser(t, serverURL+trackingPathOf(t, pixel[1]))
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))
		testutil.WaitForCondition(t, func() bool {
			opened, _ := messageEngagement(t, db, pixelMessage)
			return opened
		}, 5*time.Second, "opened_at of the pixel message")
		_, clicked := messageEngagement(t, db, pixelMessage)
		assert.False(t, clicked, "an open is not a click")
	})

	t.Run("tracking_opens_url records the open", func(t *testing.T) {
		resp := fetchFromBrowser(t, serverURL+trackingPathOf(t, variable[1]))
		require.Equal(t, http.StatusOK, resp.StatusCode)
		testutil.WaitForCondition(t, func() bool {
			opened, _ := messageEngagement(t, db, variableMessage)
			return opened
		}, 5*time.Second, "opened_at of the tracking_opens_url message")
	})

	t.Run("a legacy open pixel of a real message records the open", func(t *testing.T) {
		messageID, _ := send(t)
		resp := fetchFromBrowser(t, serverURL+"/t/"+legacyToken(t, messageID, workspace.ID, oldTimestamp()))
		require.Equal(t, http.StatusOK, resp.StatusCode)
		testutil.WaitForCondition(t, func() bool {
			opened, _ := messageEngagement(t, db, messageID)
			return opened
		}, 5*time.Second, "opened_at after a legacy pixel")
	})

	t.Run("a legacy /opens pixel of a real message records the open", func(t *testing.T) {
		messageID, _ := send(t)
		resp := fetchFromBrowser(t, serverURL+"/opens?"+url.Values{
			"mid": {messageID}, "wid": {workspace.ID}, "ts": {oldTimestamp()},
		}.Encode())
		require.Equal(t, http.StatusOK, resp.StatusCode)
		testutil.WaitForCondition(t, func() bool {
			opened, _ := messageEngagement(t, db, messageID)
			return opened
		}, 5*time.Second, "opened_at after a legacy /opens pixel")
	})

	t.Run("legacy click links of a real message", func(t *testing.T) {
		cases := []struct {
			name, dest string
			redirect   bool
		}{
			{"to the workspace's custom domain", customEndpoint + "/notification-center?wid=" + workspace.ID, true},
			{"to the request host", serverURL + "/unsubscribe-oneclick?wid=" + workspace.ID, true},
			{"to another site", "https://shop.example.com/legacy", false},
			{"to the brand's website", "https://brand.example.com/", false},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				messageID, _ := send(t)
				resp := fetchFromBrowser(t, serverURL+"/r/"+legacyToken(t, messageID, workspace.ID, oldTimestamp(), c.dest))
				if c.redirect {
					assertTrackedRedirect(t, resp, c.dest)
				} else {
					assertTrackingInterstitial(t, resp, strings.Split(strings.TrimPrefix(c.dest, "https://"), "/")[0])
				}
				testutil.WaitForCondition(t, func() bool {
					_, clicked := messageEngagement(t, db, messageID)
					return clicked
				}, 5*time.Second, "clicked_at after a legacy link")
			})
		}
	})

	t.Run("the custom domain counts only for the token's own workspace", func(t *testing.T) {
		other, err := factory.CreateWorkspace()
		require.NoError(t, err)
		messageID := "msg-" + uuid.New().String()[:8]
		resp := fetchFromBrowser(t, serverURL+"/r/"+legacyToken(t, messageID, other.ID, oldTimestamp(),
			customEndpoint+"/notification-center"))
		assertTrackingInterstitial(t, resp, "localhost")
	})
}
