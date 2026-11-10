package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	wvWorkspaceID = "ws1"
	wvMessageID   = "msg-1"
	wvSecret      = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	// wvAPIHost is the host of the fixture's API endpoint, https://api.example.com.
	wvAPIHost = "api.example.com"
	// wvLinkHost is the host wvToken's links are minted on, a workspace custom domain.
	wvLinkHost = "mail.brand-a.example"
)

// fakeWebVersionKeys answers Lookup and Refresh from fixed values, and counts the refreshes.
type fakeWebVersionKeys struct {
	lookup    trackingtoken.Workspace
	status    trackingtoken.WorkspaceStatus
	refreshed trackingtoken.Workspace
	refreshes int
}

func (f *fakeWebVersionKeys) Lookup(context.Context, string) (trackingtoken.Workspace, trackingtoken.WorkspaceStatus) {
	return f.lookup, f.status
}

func (f *fakeWebVersionKeys) Refresh(context.Context, string) (trackingtoken.Workspace, trackingtoken.WorkspaceStatus) {
	f.refreshes++
	if f.refreshed.Key != nil {
		return f.refreshed, trackingtoken.WorkspaceKnown
	}
	return f.lookup, f.status
}

type webVersionFixture struct {
	ctrl          *gomock.Controller
	keys          *fakeWebVersionKeys
	workspaceRepo *mocks.MockWorkspaceRepository
	messageRepo   *mocks.MockMessageHistoryRepository
	templateRepo  *mocks.MockTemplateRepository
	compiled      []notifuse_mjml.CompileTemplateRequest
	svc           *WebVersionService
}

func newWebVersionFixture(t *testing.T) *webVersionFixture {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()

	f := &webVersionFixture{
		ctrl:          ctrl,
		keys:          &fakeWebVersionKeys{lookup: trackingtoken.Workspace{Key: domain.TrackingSigningKey(wvSecret), EndpointHost: wvLinkHost}, status: trackingtoken.WorkspaceKnown},
		workspaceRepo: mocks.NewMockWorkspaceRepository(ctrl),
		messageRepo:   mocks.NewMockMessageHistoryRepository(ctrl),
		templateRepo:  mocks.NewMockTemplateRepository(ctrl),
	}
	f.svc = NewWebVersionService(f.keys, f.workspaceRepo, f.messageRepo, f.templateRepo, "https://api.example.com", logger)
	realCompile := f.svc.compile
	f.svc.compile = func(req notifuse_mjml.CompileTemplateRequest) (*notifuse_mjml.CompileTemplateResponse, error) {
		f.compiled = append(f.compiled, req)
		return realCompile(req)
	}
	return f
}

func wvToken(t *testing.T, secret, wid, mid string) string {
	t.Helper()
	token, err := trackingtoken.MintView(domain.TrackingSigningKey(secret), mid, wid, time.Now().Unix(), wvLinkHost)
	require.NoError(t, err)
	return token
}

func wvWorkspace() *domain.Workspace {
	return &domain.Workspace{ID: wvWorkspaceID, Settings: domain.WorkspaceSettings{SecretKey: wvSecret, DefaultLanguage: "en"}}
}

// wvLink is the web version link a template must hold for its web version to be shown.
const wvLink = `<a href="{{ view_in_browser_url }}">View in browser</a> `

// wvTemplate is a visual template whose text links to its web version, then holds content.
func wvTemplate(content string) *domain.Template {
	return &domain.Template{
		ID:      "tpl-1",
		Version: 3,
		Channel: "email",
		Email:   wvEmail("Hi", content),
	}
}

func wvEmail(subject, content string) *domain.EmailTemplate {
	return &domain.EmailTemplate{Subject: subject, VisualEditorTree: createValidMJMLTree(createTestTextBlock("txt1", wvLink+content))}
}

func wvBroadcastMessage(data map[string]interface{}) *domain.MessageHistory {
	broadcastID := "broadcast-1"
	return &domain.MessageHistory{ID: wvMessageID, BroadcastID: &broadcastID, TemplateID: "tpl-1", TemplateVersion: 3, MessageData: domain.MessageData{Data: data}}
}

func wvContactData(firstName, language string) map[string]interface{} {
	contact := map[string]interface{}{"email": "camille@example.com", "first_name": firstName}
	if language != "" {
		contact["language"] = language
	}
	return contact
}

// expectSource sets up the loads of a message that renders.
func (f *webVersionFixture) expectSource(message *domain.MessageHistory, template *domain.Template) {
	f.workspaceRepo.EXPECT().GetByID(gomock.Any(), wvWorkspaceID).Return(wvWorkspace(), nil)
	f.messageRepo.EXPECT().GetForWebVersion(gomock.Any(), wvWorkspaceID, wvSecret, wvMessageID).Return(message, nil)
	if template != nil {
		f.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), wvWorkspaceID, "tpl-1", int64(3)).Return(template, nil)
	}
}

func TestWebVersionService_RendersABroadcastEmailFromItsVersion(t *testing.T) {
	f := newWebVersionFixture(t)
	f.expectSource(wvBroadcastMessage(map[string]interface{}{"contact": wvContactData("Camille", "")}), wvTemplate("Hello {{ contact.first_name }}"))

	ref, err := f.svc.Resolve(context.Background(), wvToken(t, wvSecret, wvWorkspaceID, wvMessageID), wvAPIHost)
	require.NoError(t, err)
	assert.Equal(t, WebVersionRef{WorkspaceID: wvWorkspaceID, MessageID: wvMessageID}, ref)

	html, err := f.svc.Render(context.Background(), ref)
	require.NoError(t, err)
	assert.Contains(t, html, "Hello Camille")
}

func TestWebVersionService_RendersTheContactLanguage(t *testing.T) {
	f := newWebVersionFixture(t)
	template := wvTemplate("Hello")
	template.Translations = map[string]domain.TemplateTranslation{
		"fr": {Email: wvEmail("Salut", "Bonjour")},
	}
	f.expectSource(wvBroadcastMessage(map[string]interface{}{"contact": wvContactData("Camille", "fr")}), template)

	html, err := f.svc.Render(context.Background(), WebVersionRef{WorkspaceID: wvWorkspaceID, MessageID: wvMessageID})
	require.NoError(t, err)
	assert.Contains(t, html, "Bonjour")
	assert.NotContains(t, html, "Hello")
}

func TestWebVersionService_RendersWithoutAnyTracking(t *testing.T) {
	f := newWebVersionFixture(t)
	f.expectSource(wvBroadcastMessage(map[string]interface{}{
		"contact":            wvContactData("Camille", ""),
		"tracking_opens_url": "https://api.example.com/t/live-pixel",
		"utm_source":         "newsletter",
		"utm_medium":         "email",
		"utm_campaign":       "autumn",
		"utm_content":        "tpl-1",
		"utm_term":           "coffee",
	}), wvTemplate(`<a href="https://shop.example.com/sale">Shop</a><img src="{{ tracking_opens_url }}">`))

	html, err := f.svc.Render(context.Background(), WebVersionRef{WorkspaceID: wvWorkspaceID, MessageID: wvMessageID})
	require.NoError(t, err)

	require.Len(t, f.compiled, 1)
	ts := f.compiled[0].TrackingSettings
	assert.False(t, ts.EnableTracking)
	assert.True(t, ts.SuppressOpens)
	assert.True(t, ts.SuppressClicks)
	assert.True(t, ts.SuppressIdentify)
	assert.Empty(t, ts.IdentifyToken)
	assert.Empty(t, ts.SigningKey)
	assert.Equal(t, "https://api.example.com", ts.Endpoint)
	assert.Equal(t, "newsletter", ts.UTMSource)
	assert.Equal(t, "email", ts.UTMMedium)
	assert.Equal(t, "autumn", ts.UTMCampaign)
	assert.Equal(t, "tpl-1", ts.UTMContent)
	assert.Equal(t, "coffee", ts.UTMTerm)
	assert.Equal(t, "", f.compiled[0].TemplateData["tracking_opens_url"], "the stored pixel URL is live: never render it")

	assert.NotContains(t, html, "/t/", "no open pixel")
	assert.NotContains(t, html, "/r/", "no click redirect")
	assert.Contains(t, html, "https://shop.example.com/sale?utm_")
}

func TestWebVersionService_RendersWithTheWorkspaceEndpoint(t *testing.T) {
	f := newWebVersionFixture(t)
	custom := "https://mail.example.com/"
	ws := wvWorkspace()
	ws.Settings.CustomEndpointURL = &custom
	f.workspaceRepo.EXPECT().GetByID(gomock.Any(), wvWorkspaceID).Return(ws, nil)
	f.messageRepo.EXPECT().GetForWebVersion(gomock.Any(), wvWorkspaceID, wvSecret, wvMessageID).
		Return(wvBroadcastMessage(map[string]interface{}{"contact": wvContactData("Camille", "")}), nil)
	f.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), wvWorkspaceID, "tpl-1", int64(3)).Return(wvTemplate("Hi"), nil)

	_, err := f.svc.Render(context.Background(), WebVersionRef{WorkspaceID: wvWorkspaceID, MessageID: wvMessageID})
	require.NoError(t, err)
	require.Len(t, f.compiled, 1)
	assert.Equal(t, "https://mail.example.com", f.compiled[0].TrackingSettings.Endpoint)
}

func TestWebVersionService_RendersAnAutomationEmailWithItsUTMs(t *testing.T) {
	f := newWebVersionFixture(t)
	automationID := "auto-1"
	f.expectSource(&domain.MessageHistory{
		ID: wvMessageID, AutomationID: &automationID, TemplateID: "tpl-1", TemplateVersion: 3,
		MessageData: domain.MessageData{Data: map[string]interface{}{"contact": wvContactData("Camille", ""), "automation_name": "Welcome"}},
	}, wvTemplate(`<a href="https://shop.example.com/">Shop</a>`))

	_, err := f.svc.Render(context.Background(), WebVersionRef{WorkspaceID: wvWorkspaceID, MessageID: wvMessageID})
	require.NoError(t, err)
	require.Len(t, f.compiled, 1)
	ts := f.compiled[0].TrackingSettings
	assert.Equal(t, "automation", ts.UTMSource)
	assert.Equal(t, "email", ts.UTMMedium)
	assert.Equal(t, "Welcome", ts.UTMCampaign)
	assert.Equal(t, "tpl-1", ts.UTMContent)
}

func TestWebVersionService_RendersACodeModeTemplateThatLinksToIt(t *testing.T) {
	f := newWebVersionFixture(t)
	source := `<mjml><mj-body><mj-section><mj-column><mj-text><a href="{{view_in_browser_url}}">View</a> Hello {{ contact.first_name }}</mj-text></mj-column></mj-section></mj-body></mjml>`
	template := &domain.Template{ID: "tpl-1", Version: 3, Channel: "email", Email: &domain.EmailTemplate{Subject: "Hi", EditorMode: domain.EditorModeCode, MjmlSource: &source}}
	f.expectSource(wvBroadcastMessage(map[string]interface{}{"contact": wvContactData("Camille", "")}), template)

	html, err := f.svc.Render(context.Background(), WebVersionRef{WorkspaceID: wvWorkspaceID, MessageID: wvMessageID})
	require.NoError(t, err)
	assert.Contains(t, html, "Hello Camille")
}

// On another workspace's custom domain, a web version would put one brand's email under
// another brand's address, which is what a phishing page needs. The token signs the host
// its link was minted on, and the page opens there and on the API host only; on a
// domain the workspace has since left, only while no other workspace has taken it.
func TestWebVersionService_ResolveChecksTheSignedHost(t *testing.T) {
	token := func(t *testing.T) string { return wvToken(t, wvSecret, wvWorkspaceID, wvMessageID) }

	t.Run("on the workspace's current domain, without reading the others", func(t *testing.T) {
		for _, host := range []string{"mail.brand-a.example", "Mail.Brand-A.example:443", "mail.brand-a.example."} {
			f := newWebVersionFixture(t)
			ref, err := f.svc.Resolve(context.Background(), token(t), host)
			require.NoError(t, err, host)
			assert.Equal(t, wvMessageID, ref.MessageID)
		}
	})

	t.Run("on the API host", func(t *testing.T) {
		f := newWebVersionFixture(t)
		_, err := f.svc.Resolve(context.Background(), token(t), "API.example.com:8443")
		require.NoError(t, err)
	})

	t.Run("not on any other host", func(t *testing.T) {
		for _, host := range []string{"mail.brand-b.example", "old.brand-a.example", "mail.brand-a.example.evil.example", "notifuse:8080", ""} {
			f := newWebVersionFixture(t)
			_, err := f.svc.Resolve(context.Background(), token(t), host)
			assert.ErrorIs(t, err, ErrWebVersionNotFound, host)
		}
	})

	t.Run("on a domain the workspace left, while no other workspace has it", func(t *testing.T) {
		f := newWebVersionFixture(t)
		f.keys.lookup.EndpointHost = "new.brand-a.example"
		f.workspaceRepo.EXPECT().ListCustomEndpoints(gomock.Any()).Return(map[string]string{
			wvWorkspaceID: "https://new.brand-a.example",
		}, nil)
		_, err := f.svc.Resolve(context.Background(), token(t), "mail.brand-a.example")
		require.NoError(t, err)
	})

	t.Run("not on a domain the workspace left once another workspace has it", func(t *testing.T) {
		f := newWebVersionFixture(t)
		f.keys.lookup.EndpointHost = "new.brand-a.example"
		f.workspaceRepo.EXPECT().ListCustomEndpoints(gomock.Any()).Return(map[string]string{
			wvWorkspaceID: "https://new.brand-a.example",
			"brand_b":     "https://MAIL.brand-a.example:443",
		}, nil)
		_, err := f.svc.Resolve(context.Background(), token(t), "mail.brand-a.example")
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})

	t.Run("a failure to read the other domains is transient", func(t *testing.T) {
		f := newWebVersionFixture(t)
		f.keys.lookup.EndpointHost = ""
		f.workspaceRepo.EXPECT().ListCustomEndpoints(gomock.Any()).Return(nil, errors.New("db down"))
		_, err := f.svc.Resolve(context.Background(), token(t), "mail.brand-a.example")
		assert.ErrorIs(t, err, ErrWebVersionUnavailable)
	})

	t.Run("a link minted without an endpoint host opens on the API host only", func(t *testing.T) {
		f := newWebVersionFixture(t)
		bare, err := trackingtoken.MintView(domain.TrackingSigningKey(wvSecret), wvMessageID, wvWorkspaceID, time.Now().Unix(), "")
		require.NoError(t, err)
		_, err = f.svc.Resolve(context.Background(), bare, wvAPIHost)
		require.NoError(t, err)
		_, err = f.svc.Resolve(context.Background(), bare, "mail.brand-b.example")
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})
}

func TestWebVersionService_ReadsTheCustomDomainsOncePerTTL(t *testing.T) {
	// Anyone holding a link sent with a domain the workspace left can request it at will:
	// the domains are not read again for each request.
	f := newWebVersionFixture(t)
	f.keys.lookup.EndpointHost = "new.brand-a.example"
	clock := time.Unix(1_800_000_000, 0)
	f.svc.now = func() time.Time { return clock }
	f.workspaceRepo.EXPECT().ListCustomEndpoints(gomock.Any()).Return(map[string]string{
		wvWorkspaceID: "https://new.brand-a.example",
	}, nil).Times(2)

	token := wvToken(t, wvSecret, wvWorkspaceID, wvMessageID)
	for i := 0; i < 5; i++ {
		_, err := f.svc.Resolve(context.Background(), token, wvLinkHost)
		require.NoError(t, err)
	}
	clock = clock.Add(webVersionEndpointsTTL)
	_, err := f.svc.Resolve(context.Background(), token, wvLinkHost)
	require.NoError(t, err)
}

func TestWebVersionService_BacksOffAfterAFailedReadOfTheCustomDomains(t *testing.T) {
	f := newWebVersionFixture(t)
	f.keys.lookup.EndpointHost = "new.brand-a.example"
	clock := time.Unix(1_800_000_000, 0)
	f.svc.now = func() time.Time { return clock }
	gomock.InOrder(
		f.workspaceRepo.EXPECT().ListCustomEndpoints(gomock.Any()).Return(nil, errors.New("db down")),
		f.workspaceRepo.EXPECT().ListCustomEndpoints(gomock.Any()).Return(map[string]string{}, nil),
	)

	token := wvToken(t, wvSecret, wvWorkspaceID, wvMessageID)
	for i := 0; i < 3; i++ {
		_, err := f.svc.Resolve(context.Background(), token, wvLinkHost)
		assert.ErrorIs(t, err, ErrWebVersionUnavailable)
	}
	clock = clock.Add(webVersionEndpointsBackoff)
	_, err := f.svc.Resolve(context.Background(), token, wvLinkHost)
	require.NoError(t, err)
}

func TestWebVersionService_StopsWaitingForTheCustomDomainsWhenTheRequestEnds(t *testing.T) {
	f := newWebVersionFixture(t)
	f.keys.lookup.EndpointHost = "new.brand-a.example"
	started, release := make(chan struct{}), make(chan struct{})
	f.workspaceRepo.EXPECT().ListCustomEndpoints(gomock.Any()).DoAndReturn(func(ctx context.Context) (map[string]string, error) {
		close(started)
		<-release
		return map[string]string{}, ctx.Err()
	})
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := f.svc.Resolve(ctx, wvToken(t, wvSecret, wvWorkspaceID, wvMessageID), wvLinkHost)
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, ErrWebVersionUnavailable)
	case <-time.After(2 * time.Second):
		t.Fatal("Resolve kept waiting for the read after its request ended")
	}
}

func TestWebVersionService_RefusalLogIsBounded(t *testing.T) {
	// A refused request writes one line a minute per workspace, with the request's host
	// cut short: requests must not size the log.
	f := newWebVersionFixture(t)
	logged := 0
	quiet := pkgmocks.NewMockLogger(f.ctrl)
	quiet.EXPECT().WithFields(gomock.Any()).DoAndReturn(func(fields map[string]interface{}) logger.Logger {
		assert.LessOrEqual(t, len(fields["request_host"].(string)), 255)
		return quiet
	}).AnyTimes()
	quiet.EXPECT().Info(gomock.Any()).Do(func(string) { logged++ }).AnyTimes()
	f.svc.logger = quiet

	long := strings.Repeat("a", 10000) + ".example"
	for i := 0; i < 5; i++ {
		_, err := f.svc.Resolve(context.Background(), wvToken(t, wvSecret, wvWorkspaceID, wvMessageID), long)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	}
	assert.Equal(t, 1, logged)
}

func TestWebVersionService_Resolve(t *testing.T) {
	t.Run("a token of another purpose is not found", func(t *testing.T) {
		f := newWebVersionFixture(t)
		openToken, err := trackingtoken.MintOpen(domain.TrackingSigningKey(wvSecret), wvMessageID, wvWorkspaceID, time.Now().Unix())
		require.NoError(t, err)
		_, err = f.svc.Resolve(context.Background(), openToken, wvAPIHost)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})

	t.Run("a token signed with another key is not found", func(t *testing.T) {
		f := newWebVersionFixture(t)
		_, err := f.svc.Resolve(context.Background(), wvToken(t, "another-secret", wvWorkspaceID, wvMessageID), wvAPIHost)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
		assert.Equal(t, 1, f.keys.refreshes, "a failed signature re-reads the key once")
	})

	t.Run("a stale cached key is refreshed", func(t *testing.T) {
		f := newWebVersionFixture(t)
		f.keys.lookup = trackingtoken.Workspace{Key: domain.TrackingSigningKey("old-secret")}
		f.keys.refreshed = trackingtoken.Workspace{Key: domain.TrackingSigningKey(wvSecret)}
		ref, err := f.svc.Resolve(context.Background(), wvToken(t, wvSecret, wvWorkspaceID, wvMessageID), wvAPIHost)
		require.NoError(t, err)
		assert.Equal(t, wvMessageID, ref.MessageID)
	})

	t.Run("an unknown workspace is not found", func(t *testing.T) {
		f := newWebVersionFixture(t)
		f.keys.status = trackingtoken.WorkspaceUnknown
		_, err := f.svc.Resolve(context.Background(), wvToken(t, wvSecret, wvWorkspaceID, wvMessageID), wvAPIHost)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})

	t.Run("an unavailable workspace lookup is transient", func(t *testing.T) {
		f := newWebVersionFixture(t)
		f.keys.status = trackingtoken.WorkspaceUnavailable
		_, err := f.svc.Resolve(context.Background(), wvToken(t, wvSecret, wvWorkspaceID, wvMessageID), wvAPIHost)
		assert.ErrorIs(t, err, ErrWebVersionUnavailable)
	})

	t.Run("garbage is not found", func(t *testing.T) {
		f := newWebVersionFixture(t)
		_, err := f.svc.Resolve(context.Background(), "not-a-token", wvAPIHost)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})

	t.Run("an over-long token is not found without being opened", func(t *testing.T) {
		f := newWebVersionFixture(t)
		_, err := f.svc.Resolve(context.Background(), strings.Repeat("a", MaxWebVersionTokenLength+1), wvAPIHost)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})
}

func TestWebVersionService_RenderRefusals(t *testing.T) {
	ref := WebVersionRef{WorkspaceID: wvWorkspaceID, MessageID: wvMessageID}
	data := map[string]interface{}{"contact": wvContactData("Camille", "")}

	t.Run("no such message", func(t *testing.T) {
		f := newWebVersionFixture(t)
		f.expectSource(nil, nil)
		_, err := f.svc.Render(context.Background(), ref)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})

	t.Run("a transactional message", func(t *testing.T) {
		f := newWebVersionFixture(t)
		f.expectSource(&domain.MessageHistory{ID: wvMessageID, TemplateID: "tpl-1", TemplateVersion: 3, MessageData: domain.MessageData{Data: data}}, nil)
		_, err := f.svc.Render(context.Background(), ref)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})

	t.Run("a message recorded without its template version", func(t *testing.T) {
		f := newWebVersionFixture(t)
		message := wvBroadcastMessage(data)
		message.TemplateVersion = 0
		f.expectSource(message, nil)
		_, err := f.svc.Render(context.Background(), ref)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})

	t.Run("a message whose contact was erased", func(t *testing.T) {
		f := newWebVersionFixture(t)
		f.expectSource(wvBroadcastMessage(nil), nil)
		_, err := f.svc.Render(context.Background(), ref)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})

	t.Run("a missing template version", func(t *testing.T) {
		f := newWebVersionFixture(t)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), wvWorkspaceID).Return(wvWorkspace(), nil)
		f.messageRepo.EXPECT().GetForWebVersion(gomock.Any(), wvWorkspaceID, wvSecret, wvMessageID).Return(wvBroadcastMessage(data), nil)
		f.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), wvWorkspaceID, "tpl-1", int64(3)).Return(nil, &domain.ErrTemplateNotFound{Message: "not found"})
		_, err := f.svc.Render(context.Background(), ref)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})

	t.Run("a deleted template", func(t *testing.T) {
		f := newWebVersionFixture(t)
		template := wvTemplate("Hi")
		deletedAt := time.Now()
		template.DeletedAt = &deletedAt
		f.expectSource(wvBroadcastMessage(data), template)
		_, err := f.svc.Render(context.Background(), ref)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
		assert.Empty(t, f.compiled)
	})

	t.Run("a template without email content", func(t *testing.T) {
		f := newWebVersionFixture(t)
		f.expectSource(wvBroadcastMessage(data), &domain.Template{ID: "tpl-1", Version: 3})
		_, err := f.svc.Render(context.Background(), ref)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})

	// The variable in the template is what turns the web version on: a send mints a link
	// for every broadcast and automation email, but only one whose email shows it is served.
	t.Run("a template that does not link to its web version", func(t *testing.T) {
		f := newWebVersionFixture(t)
		template := wvTemplate("Hi")
		template.Email.VisualEditorTree = createValidMJMLTree(createTestTextBlock("txt1", "Hi {{ contact.first_name }}"))
		f.expectSource(wvBroadcastMessage(data), template)
		_, err := f.svc.Render(context.Background(), ref)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
		assert.Empty(t, f.compiled)
	})

	t.Run("a code mode template that does not link to its web version", func(t *testing.T) {
		f := newWebVersionFixture(t)
		source := `<mjml><mj-body><mj-section><mj-column><mj-text>Hi</mj-text></mj-column></mj-section></mj-body></mjml>`
		// The visual tree a code mode template keeps from before is not what it sends.
		template := wvTemplate("Hi")
		template.Email.EditorMode = domain.EditorModeCode
		template.Email.MjmlSource = &source
		f.expectSource(wvBroadcastMessage(data), template)
		_, err := f.svc.Render(context.Background(), ref)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})

	t.Run("a translation that does not link to its web version", func(t *testing.T) {
		f := newWebVersionFixture(t)
		template := wvTemplate("Hello")
		template.Translations = map[string]domain.TemplateTranslation{
			"fr": {Email: &domain.EmailTemplate{Subject: "Salut", VisualEditorTree: createValidMJMLTree(createTestTextBlock("txt1", "Bonjour"))}},
		}
		f.expectSource(wvBroadcastMessage(map[string]interface{}{"contact": wvContactData("Camille", "fr")}), template)
		_, err := f.svc.Render(context.Background(), ref)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})

	t.Run("a template that does not compile", func(t *testing.T) {
		f := newWebVersionFixture(t)
		f.expectSource(wvBroadcastMessage(data), wvTemplate("{% if %}"))
		_, err := f.svc.Render(context.Background(), ref)
		assert.ErrorIs(t, err, ErrWebVersionNotFound)
	})

	t.Run("a message history read failure is transient", func(t *testing.T) {
		f := newWebVersionFixture(t)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), wvWorkspaceID).Return(wvWorkspace(), nil)
		f.messageRepo.EXPECT().GetForWebVersion(gomock.Any(), wvWorkspaceID, wvSecret, wvMessageID).Return(nil, errors.New("db down"))
		_, err := f.svc.Render(context.Background(), ref)
		assert.ErrorIs(t, err, ErrWebVersionUnavailable)
	})

	t.Run("a request its reader abandoned is not logged as an error", func(t *testing.T) {
		f := newWebVersionFixture(t)
		quiet := pkgmocks.NewMockLogger(f.ctrl)
		quiet.EXPECT().WithFields(gomock.Any()).Return(quiet).AnyTimes()
		quiet.EXPECT().Debug(gomock.Any()).AnyTimes()
		f.svc.logger = quiet
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), wvWorkspaceID).Return(nil, fmt.Errorf("query: %w", context.Canceled))
		_, err := f.svc.Render(context.Background(), ref)
		assert.ErrorIs(t, err, ErrWebVersionUnavailable)
	})

	t.Run("a workspace read failure is transient", func(t *testing.T) {
		f := newWebVersionFixture(t)
		f.workspaceRepo.EXPECT().GetByID(gomock.Any(), wvWorkspaceID).Return(nil, errors.New("db down"))
		_, err := f.svc.Render(context.Background(), ref)
		assert.ErrorIs(t, err, ErrWebVersionUnavailable)
	})
}
