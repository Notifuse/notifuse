package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	"net/mail"
	"net/textproto"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// plainTextMJML greets the contact through Liquid and carries a button and a text link,
// so the text part shows both a rendered variable and the links' tracking.
const plainTextMJML = `<mjml><mj-head><mj-preview>Hidden preheader</mj-preview></mj-head><mj-body><mj-section><mj-column>
	<mj-text><h1>Hello {{ contact.first_name }}</h1><p>Read <a href="https://shop.example.com/new">what is new</a>.</p></mj-text>
	<mj-button href="https://shop.example.com/sale">Shop the sale</mj-button>
</mj-column></mj-section></mj-body></mjml>`

var plainTextURLRe = regexp.MustCompile(`https?://[^\s()]+`)

// deliveredParts is what a delivered message holds: its MIME shape and its two bodies,
// decoded. Mailpit's own Text field may be derived by Mailpit from the HTML, so the
// raw message is parsed here instead.
type deliveredParts struct {
	header textproto.MIMEHeader
	types  []string // media types of every part, depth first
	text   string
	html   string
}

func readDeliveredParts(t *testing.T, mailpitID string) deliveredParts {
	t.Helper()
	raw, err := testutil.GetMailpitMessageRaw(t, mailpitID)
	require.NoError(t, err)
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	require.NoError(t, err)
	body, err := io.ReadAll(msg.Body)
	require.NoError(t, err)
	parts := deliveredParts{header: textproto.MIMEHeader(msg.Header)}
	walkDeliveredPart(t, textproto.MIMEHeader(msg.Header), body, &parts)
	return parts
}

func walkDeliveredPart(t *testing.T, header textproto.MIMEHeader, body []byte, parts *deliveredParts) {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	require.NoError(t, err)
	parts.types = append(parts.types, mediaType)
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			child, err := reader.NextRawPart()
			if err == io.EOF {
				return
			}
			require.NoError(t, err)
			childBody, err := io.ReadAll(child)
			require.NoError(t, err)
			walkDeliveredPart(t, child.Header, childBody, parts)
		}
	}
	switch strings.ToLower(header.Get("Content-Transfer-Encoding")) {
	case "quoted-printable":
		body, err = io.ReadAll(quotedprintable.NewReader(bytes.NewReader(body)))
		require.NoError(t, err)
	case "base64":
		body, err = base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(body)), ""))
		require.NoError(t, err)
	}
	decoded := strings.ReplaceAll(string(body), "\r\n", "\n")
	switch mediaType {
	case "text/plain":
		parts.text = decoded
	case "text/html":
		parts.html = decoded
	}
}

// webHrefs returns the distinct http(s) hrefs of the anchors in an HTML body, sorted.
func webHrefs(t *testing.T, body string) []string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	require.NoError(t, err)
	var hrefs []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				lower := strings.ToLower(a.Val)
				if a.Key == "href" && (strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")) {
					hrefs = append(hrefs, a.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return distinctSorted(hrefs)
}

func distinctSorted(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// assertTextMirrorsHTML checks the message is multipart/alternative with the text first,
// and that the text links to exactly what the HTML links to: a glued or dropped URL fails.
func assertTextMirrorsHTML(t *testing.T, parts deliveredParts) {
	t.Helper()
	require.Contains(t, parts.types, "multipart/alternative")
	textAt, htmlAt := -1, -1
	for i, mt := range parts.types {
		if mt == "text/plain" && textAt < 0 {
			textAt = i
		}
		if mt == "text/html" && htmlAt < 0 {
			htmlAt = i
		}
	}
	require.True(t, textAt >= 0 && htmlAt > textAt, "text/plain must come before text/html, got %v", parts.types)
	assert.Equal(t, webHrefs(t, parts.html), distinctSorted(plainTextURLRe.FindAllString(parts.text, -1)),
		"the text links to the same URLs as the HTML")
	assert.NotContains(t, parts.text, "Hidden preheader")
}

func TestPlainTextAlternative_E2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	// Tracked links are absolute only with an endpoint, and the test server has no API
	// endpoint: the workspace's custom endpoint stands in for it.
	linkEndpoint := "https://links.example.com"
	workspace, err := factory.CreateWorkspace(
		withEmailTracking(*domain.DefaultEmailTrackingSettings()),
		func(w *domain.Workspace) { w.Settings.CustomEndpointURL = &linkEndpoint },
	)
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID, smtpToMailpit("Plain Text E2E"))
	require.NoError(t, err)

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(workerCtx))

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	require.NoError(t, testutil.ClearMailpitMessages(t))

	tag := uuid.New().String()[:8]
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Plain Text E2E Template"),
		testutil.WithTemplateSubject("Plain text e2e "+tag),
		testutil.WithCodeModeTemplate(plainTextMJML))
	require.NoError(t, err)

	t.Run("a transactional email carries the rendered text with its links", func(t *testing.T) {
		notification, err := factory.CreateTransactionalNotification(workspace.ID,
			testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
				domain.TransactionalChannelEmail: {TemplateID: template.ID, Settings: map[string]interface{}{}},
			}))
		require.NoError(t, err)

		recipient := fmt.Sprintf("tx-text-%s@example.com", tag)
		resp, err := client.SendTransactionalNotification(map[string]interface{}{
			"id":       notification.ID,
			"contact":  map[string]interface{}{"email": recipient, "first_name": "Camille"},
			"channels": []string{"email"},
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		email, err := waitForEmailByRecipientAddr(t, recipient, 30*time.Second)
		require.NoError(t, err)
		parts := readDeliveredParts(t, email.ID)

		assertTextMirrorsHTML(t, parts)
		assert.Contains(t, parts.text, "Hello Camille")
		assert.Contains(t, parts.text, "Shop the sale ("+linkEndpoint+"/r/")
		assert.Contains(t, parts.html, "Hello Camille")
	})

	t.Run("a broadcast email carries the text, tracked as each recipient consented", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID, testutil.WithListName("Plain Text E2E List"))
		require.NoError(t, err)
		granted := fmt.Sprintf("bc-granted-%s@example.com", tag)
		refused := fmt.Sprintf("bc-refused-%s@example.com", tag)
		for email, consent := range map[string]string{granted: "granted", refused: "denied"} {
			status, body := postStatus(t, client, "/api/contacts.upsert", map[string]interface{}{
				"workspace_id": workspace.ID,
				"contact": map[string]interface{}{
					"email": email, "first_name": "Ada", "tracking_clicks_consent": consent,
				},
			})
			require.Equal(t, http.StatusOK, status, body)
			_, err = factory.CreateContactList(workspace.ID,
				testutil.WithContactListEmail(email),
				testutil.WithContactListListID(list.ID),
				testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
		}

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Plain Text E2E Broadcast"),
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastAudience(domain.AudienceSettings{List: list.ID, ExcludeUnsubscribed: true}))
		require.NoError(t, err)
		resp, err := client.ScheduleBroadcast(map[string]interface{}{"workspace_id": workspace.ID, "id": broadcast.ID, "send_now": true})
		require.NoError(t, err)
		_ = resp.Body.Close()
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID, []string{"processed", "completed", "sent"}, 60*time.Second)
		require.NoError(t, err)

		grantedEmail, err := waitForEmailByRecipientAddr(t, granted, 30*time.Second)
		require.NoError(t, err)
		grantedParts := readDeliveredParts(t, grantedEmail.ID)
		assertTextMirrorsHTML(t, grantedParts)
		assert.Contains(t, grantedParts.text, "Hello Ada")
		assert.Contains(t, grantedParts.text, linkEndpoint+"/r/", "a consenting recipient's text links are tracked")
		assert.NotEmpty(t, grantedParts.header.Get("List-Unsubscribe"), "the unsubscribe header is still set")

		refusedEmail, err := waitForEmailByRecipientAddr(t, refused, 30*time.Second)
		require.NoError(t, err)
		refusedParts := readDeliveredParts(t, refusedEmail.ID)
		assertTextMirrorsHTML(t, refusedParts)
		assert.NotContains(t, refusedParts.text, "/r/", "a refusing recipient's text links go straight to their destination")
		assert.Contains(t, refusedParts.text, "https://shop.example.com/sale?")
	})

	t.Run("templates.compile returns the text version", func(t *testing.T) {
		resp, err := client.Post("/api/templates.compile", map[string]interface{}{
			"workspace_id": workspace.ID,
			"message_id":   "preview",
			"mjml_source":  plainTextMJML,
			"test_data":    map[string]interface{}{"contact": map[string]interface{}{"first_name": "Lou"}},
			"channel":      "email",
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result struct {
			HTML string  `json:"html"`
			Text *string `json:"text"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		require.NotNil(t, result.Text)
		assert.Contains(t, *result.Text, "Hello Lou")
		assert.Equal(t, webHrefs(t, result.HTML), distinctSorted(plainTextURLRe.FindAllString(*result.Text, -1)))
	})
}
