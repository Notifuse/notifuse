package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
)

// MaxWebVersionTokenLength bounds the token read from a /v/ path before it is opened. A
// real token is a few hundred bytes; anything longer is refused without decrypting it.
const MaxWebVersionTokenLength = 2048

var (
	// ErrWebVersionNotFound covers every reason a web version is not shown: a token that
	// does not verify, a message that is not a broadcast or automation email, an erased
	// contact, a deleted template. One error, so the route answers them all alike.
	ErrWebVersionNotFound = errors.New("web version not found")
	// ErrWebVersionUnavailable is a transient failure: the email may well exist.
	ErrWebVersionUnavailable = errors.New("web version temporarily unavailable")
)

// WebVersionKeys is what the web version needs from the tracking workspace source: the
// key a workspace's links are signed with.
type WebVersionKeys interface {
	Lookup(ctx context.Context, workspaceID string) (trackingtoken.Workspace, trackingtoken.WorkspaceStatus)
	Refresh(ctx context.Context, workspaceID string) (trackingtoken.Workspace, trackingtoken.WorkspaceStatus)
}

// WebVersionRef names the message a verified web version token points to.
type WebVersionRef struct {
	WorkspaceID string
	MessageID   string
}

// WebVersionService renders the web version of a broadcast or automation email: the
// template version it was sent with, compiled again from the data recorded in message
// history. It writes nothing, and the page it returns tracks nothing.
type WebVersionService struct {
	keys          WebVersionKeys
	workspaceRepo domain.WorkspaceRepository
	messageRepo   domain.MessageHistoryRepository
	templateRepo  domain.TemplateRepository
	apiEndpoint   string
	apiHost       string
	logger        logger.Logger
	compile       func(notifuse_mjml.CompileTemplateRequest) (*notifuse_mjml.CompileTemplateResponse, error)

	now func() time.Time

	refusalMu     sync.Mutex
	refusalLogged map[string]time.Time

	endpointsMu sync.Mutex
	endpoints   map[string]string
	endpointsAt time.Time
}

// NewWebVersionService builds the service. apiEndpoint is the endpoint used when the
// workspace has no custom one.
func NewWebVersionService(
	keys WebVersionKeys,
	workspaceRepo domain.WorkspaceRepository,
	messageRepo domain.MessageHistoryRepository,
	templateRepo domain.TemplateRepository,
	apiEndpoint string,
	logger logger.Logger,
) *WebVersionService {
	return &WebVersionService{
		keys:          keys,
		workspaceRepo: workspaceRepo,
		messageRepo:   messageRepo,
		templateRepo:  templateRepo,
		apiEndpoint:   apiEndpoint,
		apiHost:       trackingtoken.HostOf(apiEndpoint),
		logger:        logger,
		compile:       notifuse_mjml.CompileTemplate,
		now:           time.Now,
		refusalLogged: make(map[string]time.Time),
	}
}

// Resolve opens and verifies a web version token requested on requestHost, the host the
// reader's browser asked for, port included or not. Only a token signed with its
// workspace's key for the view purpose resolves: there is no unsigned or legacy form.
//
// A web version shown under another workspace's custom domain would put one brand's
// email under another brand's address, which is what a phishing page needs. The token
// signs the host its link was written with, and the page opens on that host and on the
// API host only. When the workspace has since moved to another domain, the old one keeps
// its web versions only while no other workspace has claimed it.
func (s *WebVersionService) Resolve(ctx context.Context, token, requestHost string) (WebVersionRef, error) {
	if token == "" || len(token) > MaxWebVersionTokenLength {
		return WebVersionRef{}, ErrWebVersionNotFound
	}
	tok, err := trackingtoken.Parse(token, trackingtoken.PurposeView)
	if err != nil || tok.Tag == "" || !trackingtoken.ValidWorkspaceID(tok.WorkspaceID) {
		return WebVersionRef{}, ErrWebVersionNotFound
	}

	ws, status := s.keys.Lookup(ctx, tok.WorkspaceID)
	switch status {
	case trackingtoken.WorkspaceUnavailable:
		return WebVersionRef{}, ErrWebVersionUnavailable
	case trackingtoken.WorkspaceKnown:
	default:
		return WebVersionRef{}, ErrWebVersionNotFound
	}
	if !tok.Verify(ws.Key, trackingtoken.PurposeView) {
		// The cached key may be stale, for a workspace re-created under its ID.
		ws, status = s.keys.Refresh(ctx, tok.WorkspaceID)
		if status != trackingtoken.WorkspaceKnown || !tok.Verify(ws.Key, trackingtoken.PurposeView) {
			return WebVersionRef{}, ErrWebVersionNotFound
		}
	}

	ref := WebVersionRef{WorkspaceID: tok.WorkspaceID, MessageID: tok.MessageID}
	if trackingtoken.SameHost(requestHost, s.apiHost) {
		return ref, nil
	}
	if !trackingtoken.SameHost(requestHost, tok.Host) {
		s.logRefusal(tok, requestHost, "a host its link was not written with")
		return WebVersionRef{}, ErrWebVersionNotFound
	}
	if trackingtoken.SameHostname(tok.Host, ws.EndpointHost) {
		return ref, nil
	}
	// A domain the workspace has left: refused once another workspace holds it.
	endpoints, err := s.customEndpoints(ctx)
	if err != nil {
		return WebVersionRef{}, s.unavailable(ref, "read the custom domains", err)
	}
	for workspaceID, endpoint := range endpoints {
		if workspaceID != tok.WorkspaceID && trackingtoken.SameHostname(trackingtoken.HostOf(endpoint), tok.Host) {
			s.logRefusal(tok, requestHost, "a domain another workspace now holds")
			return WebVersionRef{}, ErrWebVersionNotFound
		}
	}
	return ref, nil
}

// webVersionEndpointsTTL is how long the list of custom domains is reused. Anyone holding
// a link sent with a domain its workspace has left can request it at will, so the list
// is not read for each request; a domain another workspace takes is refused this much
// later at most, on top of the tracking cache's own delay.
const webVersionEndpointsTTL = 30 * time.Second

// customEndpoints returns every workspace's custom endpoint, read at most once per TTL.
// Concurrent readers wait for the one read in flight.
func (s *WebVersionService) customEndpoints(ctx context.Context) (map[string]string, error) {
	s.endpointsMu.Lock()
	defer s.endpointsMu.Unlock()
	if s.endpoints != nil && s.now().Sub(s.endpointsAt) < webVersionEndpointsTTL {
		return s.endpoints, nil
	}
	endpoints, err := s.workspaceRepo.ListCustomEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	if endpoints == nil {
		endpoints = map[string]string{}
	}
	s.endpoints, s.endpointsAt = endpoints, s.now()
	return endpoints, nil
}

// webVersionRefusalLogInterval spaces the refusal lines of one workspace: each comes from
// a request, so a stream of requests must not become a stream of lines.
const webVersionRefusalLogInterval = time.Minute

// maxLoggedHostLength cuts the hosts a refusal logs to the longest a DNS name can be.
const maxLoggedHostLength = 255

func (s *WebVersionService) logRefusal(tok trackingtoken.Token, requestHost, reason string) {
	now := s.now()
	s.refusalMu.Lock()
	if last, ok := s.refusalLogged[tok.WorkspaceID]; ok && now.Sub(last) < webVersionRefusalLogInterval {
		s.refusalMu.Unlock()
		return
	}
	if len(s.refusalLogged) >= maxRefusalLogEntries {
		// Only workspaces whose key verified reach here, so the map stays small; clearing
		// it bounds it anyway.
		s.refusalLogged = make(map[string]time.Time)
	}
	s.refusalLogged[tok.WorkspaceID] = now
	s.refusalMu.Unlock()

	s.logger.WithFields(map[string]interface{}{
		"workspace_id": tok.WorkspaceID,
		"message_id":   tok.MessageID,
		"link_host":    truncateHost(tok.Host),
		"request_host": truncateHost(requestHost),
	}).Info("Web version refused on " + reason)
}

const maxRefusalLogEntries = 10000

func truncateHost(host string) string {
	if len(host) > maxLoggedHostLength {
		return host[:maxLoggedHostLength]
	}
	return host
}

// Render compiles the web version of a resolved message.
func (s *WebVersionService) Render(ctx context.Context, ref WebVersionRef) (string, error) {
	workspace, err := s.workspaceRepo.GetByID(ctx, ref.WorkspaceID)
	if err != nil {
		var notFound *domain.ErrWorkspaceNotFound
		if errors.As(err, &notFound) {
			return "", ErrWebVersionNotFound
		}
		return "", s.unavailable(ref, "load the workspace", err)
	}

	message, err := s.messageRepo.GetForWebVersion(ctx, ref.WorkspaceID, workspace.Settings.SecretKey, ref.MessageID)
	if err != nil {
		return "", s.unavailable(ref, "load the message", err)
	}
	// Broadcasts and automations only. A message recorded without its template version
	// is never rendered from the latest one, and an erased contact's message has no data.
	if message == nil || (message.BroadcastID == nil && message.AutomationID == nil) ||
		message.TemplateVersion <= 0 || len(message.MessageData.Data) == 0 {
		return "", ErrWebVersionNotFound
	}

	template, err := s.templateRepo.GetTemplateByID(ctx, ref.WorkspaceID, message.TemplateID, message.TemplateVersion)
	if err != nil {
		var notFound *domain.ErrTemplateNotFound
		if errors.As(err, &notFound) {
			return "", ErrWebVersionNotFound
		}
		return "", s.unavailable(ref, "load the template", err)
	}
	// Deleting a template takes its web versions down: it is how an operator withdraws one.
	if template == nil || template.DeletedAt != nil {
		return "", ErrWebVersionNotFound
	}

	data := make(map[string]interface{}, len(message.MessageData.Data))
	for key, value := range message.MessageData.Data {
		data[key] = value
	}
	// The recorded pixel URL is live and signed: a template that uses it must not record
	// an open each time the page is viewed.
	data["tracking_opens_url"] = ""

	emailContent := template.ResolveEmailContent(contactLanguage(data), workspace.Settings.DefaultLanguage)
	// The variable in the template is what turns the web version on. Every broadcast and
	// automation email carries a link in its data, but only one whose email shows it has
	// a web version.
	if emailContent == nil || !linksToWebVersion(emailContent) {
		return "", ErrWebVersionNotFound
	}

	compileReq := notifuse_mjml.CompileTemplateRequest{
		WorkspaceID:      ref.WorkspaceID,
		MessageID:        ref.MessageID,
		TemplateData:     notifuse_mjml.MapOfAny(data),
		TrackingSettings: webVersionTrackingSettings(workspace.Settings.ResolveEndpoint(s.apiEndpoint), message, data),
	}
	emailContent.ApplyToCompileRequest(&compileReq, nil)
	compiled, err := s.compile(compileReq)
	if err != nil || compiled == nil || !compiled.Success || compiled.HTML == nil {
		reason := "compilation failed"
		if err != nil {
			reason = err.Error()
		} else if compiled != nil && compiled.Error != nil {
			reason = compiled.Error.Message
		}
		s.logger.WithFields(map[string]interface{}{
			"workspace_id": ref.WorkspaceID,
			"message_id":   ref.MessageID,
			"template_id":  message.TemplateID,
			"error":        reason,
		}).Warn("Web version could not be compiled")
		return "", ErrWebVersionNotFound
	}
	return *compiled.HTML, nil
}

func (s *WebVersionService) unavailable(ref WebVersionRef, step string, err error) error {
	log := s.logger.WithFields(map[string]interface{}{
		"workspace_id": ref.WorkspaceID,
		"message_id":   ref.MessageID,
		"error":        err.Error(),
	})
	if errors.Is(err, context.Canceled) {
		// The reader closed the page, or a link scanner gave up: nothing failed.
		log.Debug("Web version request abandoned while trying to " + step)
	} else {
		log.Error("Web version failed to " + step)
	}
	return fmt.Errorf("%w: %s", ErrWebVersionUnavailable, step)
}

// webVersionTrackingSettings tracks nothing (no pixel, no click redirect, no identity
// token), so viewing or clicking in the web version records nothing and cannot track a
// contact who refused tracking after the send. Links keep the UTM parameters of the
// send: a broadcast's are in its recorded data, an automation's follow the rule the
// automation executor applies.
func webVersionTrackingSettings(endpoint string, message *domain.MessageHistory, data map[string]interface{}) notifuse_mjml.TrackingSettings {
	ts := notifuse_mjml.TrackingSettings{
		Endpoint:         endpoint,
		EnableTracking:   false,
		SuppressOpens:    true,
		SuppressClicks:   true,
		SuppressIdentify: true,
	}
	if message.BroadcastID != nil {
		ts.UTMSource = stringValue(data["utm_source"])
		ts.UTMMedium = stringValue(data["utm_medium"])
		ts.UTMCampaign = stringValue(data["utm_campaign"])
		ts.UTMContent = stringValue(data["utm_content"])
		ts.UTMTerm = stringValue(data["utm_term"])
	} else {
		ts.UTMSource = "automation"
		ts.UTMMedium = "email"
		ts.UTMCampaign = stringValue(data["automation_name"])
		ts.UTMContent = message.TemplateID
	}
	return ts
}

// linksToWebVersion reports whether the email content uses the web version variable: the
// MJML source of a code mode template, the visual tree otherwise.
func linksToWebVersion(content *domain.EmailTemplate) bool {
	if source := content.GetCodeModeMjmlSource(); source != nil {
		return strings.Contains(*source, webVersionVariable)
	}
	tree, err := json.Marshal(content.VisualEditorTree)
	return err == nil && strings.Contains(string(tree), webVersionVariable)
}

const webVersionVariable = "view_in_browser_url"

// contactLanguage reads the recorded contact's language, "" when there is none.
func contactLanguage(data map[string]interface{}) string {
	contact, ok := data["contact"].(map[string]interface{})
	if !ok {
		return ""
	}
	return stringValue(contact["language"])
}

func stringValue(v interface{}) string {
	s, _ := v.(string)
	return s
}
