package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/aws/smithy-go"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// gapHTTPClient is a fake domain.HTTPClient for Mailgun: its API host is
// hard-coded, so requests are captured and answered in-process.
type gapHTTPClient struct {
	calls   int
	lastReq *http.Request
	body    []byte
	handler func(req *http.Request) (*http.Response, error)
}

func (c *gapHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.calls++
	c.lastReq = req
	if req.Body != nil {
		c.body, _ = io.ReadAll(req.Body)
	}
	return c.handler(req)
}

func gapLogger(t *testing.T) *pkgmocks.MockLogger {
	t.Helper()
	ctrl := gomock.NewController(t)
	l := pkgmocks.NewMockLogger(ctrl)
	l.EXPECT().Error(gomock.Any()).AnyTimes()
	l.EXPECT().Info(gomock.Any()).AnyTimes()
	l.EXPECT().Warn(gomock.Any()).AnyTimes()
	l.EXPECT().Debug(gomock.Any()).AnyTimes()
	l.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(l).AnyTimes()
	l.EXPECT().WithFields(gomock.Any()).Return(l).AnyTimes()
	return l
}

func gapResp(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func gapMailgunSvc(client *gapHTTPClient, t *testing.T) *MailgunService {
	ctrl := gomock.NewController(t)
	return NewMailgunService(client, mocks.NewMockAuthService(ctrl), gapLogger(t), "https://hooks.example.com")
}

func TestProviderGapMailgunCreateWebhookEmptyEventsNoHTTP(t *testing.T) {
	client := &gapHTTPClient{handler: func(req *http.Request) (*http.Response, error) {
		t.Fatal("HTTP must not be called when events are empty")
		return nil, nil
	}}
	svc := gapMailgunSvc(client, t)
	_, err := svc.CreateWebhook(context.Background(), domain.MailgunSettings{Domain: "mg.example.com", APIKey: "k"}, domain.MailgunWebhook{URL: "https://x.example/h"})
	require.EqualError(t, err, "at least one event type is required")
	assert.Equal(t, 0, client.calls)
}

func TestProviderGapMailgunCreateWebhookEURequestAndNonOK(t *testing.T) {
	client := &gapHTTPClient{handler: func(req *http.Request) (*http.Response, error) {
		return gapResp(http.StatusForbidden, `{"message":"denied"}`), nil
	}}
	svc := gapMailgunSvc(client, t)
	cfg := domain.MailgunSettings{Domain: "mg.example.com", APIKey: "secret-key", Region: "EU"}
	_, err := svc.CreateWebhook(context.Background(), cfg, domain.MailgunWebhook{URL: "https://cb.example/hook", Events: []string{"delivered", "complained"}})
	require.EqualError(t, err, "API returned non-OK status code 403")
	require.NotNil(t, client.lastReq)
	assert.Equal(t, http.MethodPost, client.lastReq.Method)
	assert.Equal(t, "https://api.eu.mailgun.net/v3/domains/mg.example.com/webhooks", client.lastReq.URL.String())
	user, pass, ok := client.lastReq.BasicAuth()
	require.True(t, ok)
	assert.Equal(t, "api", user)
	assert.Equal(t, "secret-key", pass)
	assert.Equal(t, "application/x-www-form-urlencoded", client.lastReq.Header.Get("Content-Type"))
	// Only the first event is sent per Mailgun's one-call-per-event API.
	assert.Equal(t, "id=delivered&url=https%3A%2F%2Fcb.example%2Fhook", string(client.body))
}

func TestProviderGapMailgunGetWebhookDecodeAndStatusErrors(t *testing.T) {
	cfg := domain.MailgunSettings{Domain: "example.com", APIKey: "k", Region: "US"}

	client := &gapHTTPClient{handler: func(req *http.Request) (*http.Response, error) {
		return gapResp(http.StatusOK, `not-json`), nil
	}}
	svc := gapMailgunSvc(client, t)
	_, err := svc.GetWebhook(context.Background(), cfg, "delivered")
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to decode response:"), "got %q", err.Error())
	assert.Equal(t, "https://api.mailgun.net/v3/domains/example.com/webhooks/delivered", client.lastReq.URL.String())

	client2 := &gapHTTPClient{handler: func(req *http.Request) (*http.Response, error) {
		return gapResp(http.StatusNotFound, `{"message":"missing"}`), nil
	}}
	svc2 := gapMailgunSvc(client2, t)
	_, err = svc2.GetWebhook(context.Background(), cfg, "delivered")
	require.EqualError(t, err, "API returned non-OK status code 404")
}

func TestProviderGapMailgunDeleteWebhookAndTestWebhook(t *testing.T) {
	cfg := domain.MailgunSettings{Domain: "example.com", APIKey: "k"}
	client := &gapHTTPClient{handler: func(req *http.Request) (*http.Response, error) {
		return gapResp(http.StatusNoContent, ""), nil
	}}
	svc := gapMailgunSvc(client, t)
	require.NoError(t, svc.DeleteWebhook(context.Background(), cfg, "complained"))
	assert.Equal(t, http.MethodDelete, client.lastReq.Method)
	assert.Equal(t, "https://api.mailgun.net/v3/domains/example.com/webhooks/complained", client.lastReq.URL.String())

	client2 := &gapHTTPClient{handler: func(req *http.Request) (*http.Response, error) {
		return gapResp(http.StatusInternalServerError, "boom"), nil
	}}
	svc2 := gapMailgunSvc(client2, t)
	require.EqualError(t, svc2.DeleteWebhook(context.Background(), cfg, "complained"), "API returned non-OK status code 500")

	callsBefore := client2.calls
	err := svc2.TestWebhook(context.Background(), cfg, "complained", "delivered")
	require.EqualError(t, err, "testing webhooks is not supported by the Mailgun API")
	assert.Equal(t, callsBefore, client2.calls, "TestWebhook must not call HTTP")
}

func TestProviderGapMailgunSendEmailGuardsNoHTTP(t *testing.T) {
	client := &gapHTTPClient{handler: func(req *http.Request) (*http.Response, error) {
		t.Fatal("HTTP must not be called after a guard fails")
		return nil, nil
	}}
	svc := gapMailgunSvc(client, t)
	base := domain.SendEmailProviderRequest{
		WorkspaceID: "ws", IntegrationID: "int", MessageID: "m1", FromAddress: "a@example.com",
		FromName: "A", Subject: "s", Content: "c",
		Provider: &domain.EmailProvider{Mailgun: &domain.MailgunSettings{Domain: "d.example", APIKey: "k"}},
	}
	err := svc.SendEmail(context.Background(), base)
	require.EqualError(t, err, "invalid request: to address is required")

	withTo := base
	withTo.To = "to@example.com"
	withTo.Provider = &domain.EmailProvider{}
	err = svc.SendEmail(context.Background(), withTo)
	require.EqualError(t, err, "mailgun provider is not configured")
	assert.Equal(t, 0, client.calls)
}

func TestProviderGapMailgunInboundRouteOwnership(t *testing.T) {
	owned := []string{`forward("https://app.example/webhooks/email/inbound?workspace_id=ws1&integration_id=int1")`, "stop()"}
	assert.True(t, isOwnedInboundReplyRoute(owned, "ws1", "int1"))
	assert.False(t, isOwnedInboundReplyRoute(owned, "ws1", "other"))
	assert.False(t, isOwnedInboundReplyRoute(owned, "other", "int1"))
	assert.False(t, isOwnedInboundReplyRoute([]string{`forward("https://app.example/other?workspace_id=ws1&integration_id=int1")`}, "ws1", "int1"))
	assert.False(t, isOwnedInboundReplyRoute(nil, "ws1", "int1"))
	assert.False(t, isOwnedInboundReplyRoute([]string{}, "ws1", "int1"))
}

func TestProviderGapMailgunDeleteRouteErrorIncludesBody(t *testing.T) {
	client := &gapHTTPClient{handler: func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://api.mailgun.net/v3/routes/route-9", req.URL.String())
		return gapResp(http.StatusInternalServerError, "kaboom"), nil
	}}
	svc := gapMailgunSvc(client, t)
	err := svc.deleteRoute(context.Background(), domain.MailgunSettings{Domain: "d.example", APIKey: "k"}, "route-9")
	// Real contract: deleteRoute (unlike the webhook methods) appends the body.
	require.EqualError(t, err, "API returned non-OK status code 500: kaboom")
}

func gapSparkPost(t *testing.T, handler http.HandlerFunc) (*SparkPostService, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	ctrl := gomock.NewController(t)
	svc := NewSparkPostService(srv.Client(), mocks.NewMockAuthService(ctrl), gapLogger(t))
	return svc, srv
}

func TestProviderGapSparkPostCreateWebhookHTTPContract(t *testing.T) {
	var gotPath, gotAuth, gotCT string
	var gotBody map[string]interface{}
	svc, srv := gapSparkPost(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"results":{"id":"wh-42","name":"Notifuse-int","target":"https://cb.example/hook","events":["delivery"],"active":true}}`))
	})
	cfg := domain.SparkPostSettings{Endpoint: srv.URL, APIKey: "sp-key"}
	got, err := svc.CreateWebhook(context.Background(), cfg, domain.SparkPostWebhook{Name: "Notifuse-int", Target: "https://cb.example/hook", Events: []string{"delivery"}, Active: true, AuthType: "none"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "wh-42", got.Results.ID)
	assert.Equal(t, "/api/v1/webhooks", gotPath)
	assert.Equal(t, "Bearer sp-key", gotAuth)
	assert.Equal(t, "application/json", gotCT)
	assert.Equal(t, "Notifuse-int", gotBody["name"])
	assert.Equal(t, "https://cb.example/hook", gotBody["target"])
	assert.Equal(t, []interface{}{"delivery"}, gotBody["events"])
}

func TestProviderGapSparkPostErrorMapping(t *testing.T) {
	svc, srv := gapSparkPost(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"message":"bad"}]}`))
	})
	cfg := domain.SparkPostSettings{Endpoint: srv.URL, APIKey: "sp-key"}
	_, err := svc.CreateWebhook(context.Background(), cfg, domain.SparkPostWebhook{Name: "n", Target: "https://cb.example/h"})
	require.EqualError(t, err, "API returned non-OK status code 400")

	svc2, srv2 := gapSparkPost(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/webhooks/wh-7", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"message":"missing"}]}`))
	})
	cfg2 := domain.SparkPostSettings{Endpoint: srv2.URL, APIKey: "sp-key"}
	_, err = svc2.GetWebhook(context.Background(), cfg2, "wh-7")
	require.EqualError(t, err, "API returned non-OK status code 404")
}

func TestProviderGapSparkPostDeleteAndTestWebhook(t *testing.T) {
	var paths []string
	svc, srv := gapSparkPost(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/validate") {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"errors":[{"message":"invalid"}]}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	cfg := domain.SparkPostSettings{Endpoint: srv.URL, APIKey: "sp-key"}
	require.NoError(t, svc.DeleteWebhook(context.Background(), cfg, "wh-7"))
	require.EqualError(t, svc.TestWebhook(context.Background(), cfg, "wh-7"), "API returned non-OK status code 422")
	assert.Equal(t, []string{"DELETE /api/v1/webhooks/wh-7", "POST /api/v1/webhooks/wh-7/validate"}, paths)
}

func TestProviderGapSparkPostValidateWebhook(t *testing.T) {
	var gotBody map[string]string
	svc, srv := gapSparkPost(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		assert.Equal(t, "/api/v1/webhooks/validate", r.URL.Path)
		_, _ = w.Write([]byte(`{"results":{"valid":true}}`))
	})
	cfg := domain.SparkPostSettings{Endpoint: srv.URL, APIKey: "sp-key"}
	valid, err := svc.ValidateWebhook(context.Background(), cfg, domain.SparkPostWebhook{Target: "https://cb.example/hook"})
	require.NoError(t, err)
	assert.True(t, valid)
	assert.Equal(t, map[string]string{"target": "https://cb.example/hook"}, gotBody)

	// Real contract: ValidateWebhook does not check the status code; a decodable
	// body wins even on an error status.
	svc2, srv2 := gapSparkPost(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"results":{"valid":false}}`))
	})
	cfg2 := domain.SparkPostSettings{Endpoint: srv2.URL, APIKey: "sp-key"}
	valid, err = svc2.ValidateWebhook(context.Background(), cfg2, domain.SparkPostWebhook{Target: "https://cb.example/hook"})
	require.NoError(t, err)
	assert.False(t, valid)

	svc3, srv3 := gapSparkPost(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	})
	cfg3 := domain.SparkPostSettings{Endpoint: srv3.URL, APIKey: "sp-key"}
	_, err = svc3.ValidateWebhook(context.Background(), cfg3, domain.SparkPostWebhook{Target: "https://cb.example/hook"})
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to decode validation response:"), "got %q", err.Error())
}

func TestProviderGapSESAddressEncoding(t *testing.T) {
	got, err := encodeEmailAddress("plain@example.com")
	require.NoError(t, err)
	assert.Equal(t, "plain@example.com", got)

	// No "@" is returned as-is (invalid format passthrough).
	got, err = encodeEmailAddress("not-an-email")
	require.NoError(t, err)
	assert.Equal(t, "not-an-email", got)

	// International domain is punycode-encoded.
	got, err = encodeEmailAddress("user@münchen.de")
	require.NoError(t, err)
	assert.Equal(t, "user@xn--mnchen-3ya.de", got)

	// Non-ASCII local part is RFC 2047 B-encoded (mime.BEncoding emits lowercase "b").
	got, err = encodeEmailAddress("tüser@example.com")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(got, "=?UTF-8?b?"), "got %q", got)
	assert.True(t, strings.HasSuffix(got, "?=@example.com"), "got %q", got)
}

func TestProviderGapSESFormatFromHeader(t *testing.T) {
	got, err := formatFromHeader("", "sender@example.com")
	require.NoError(t, err)
	assert.Equal(t, "sender@example.com", got)

	got, err = formatFromHeader("Alice", "alice@münchen.de")
	require.NoError(t, err)
	assert.Equal(t, "Alice <alice@xn--mnchen-3ya.de>", got)

	got, err = formatFromHeader("Jürg", "j@example.com")
	require.NoError(t, err)
	// mime.BEncoding emits lowercase "b" in the encoded-word marker.
	assert.True(t, strings.HasPrefix(got, "=?UTF-8?b?"), "got %q", got)
	assert.True(t, strings.HasSuffix(got, " <j@example.com>"), "got %q", got)
}

func TestProviderGapSESInboundIDsFromURL(t *testing.T) {
	ws, integ := inboundIDsFromURL("https://app.example/webhooks/email/inbound?workspace_id=ws9&integration_id=int9")
	assert.Equal(t, "ws9", ws)
	assert.Equal(t, "int9", integ)

	ws, integ = inboundIDsFromURL("https://app.example/webhooks/email/inbound")
	assert.Equal(t, "", ws)
	assert.Equal(t, "", integ)

	ws, integ = inboundIDsFromURL("%zz")
	assert.Equal(t, "", ws)
	assert.Equal(t, "", integ)
}

func TestProviderGapSESPublishTopicPolicy(t *testing.T) {
	policy, err := sesPublishTopicPolicy("arn:aws:sns:us-east-1:123456789012:topic", "123456789012")
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(policy), &decoded))
	assert.Equal(t, "2012-10-17", decoded["Version"])
	stmts, ok := decoded["Statement"].([]interface{})
	require.True(t, ok)
	require.Len(t, stmts, 1)
	stmt := stmts[0].(map[string]interface{})
	assert.Equal(t, "AllowSESPublish", stmt["Sid"])
	assert.Equal(t, "arn:aws:sns:us-east-1:123456789012:topic", stmt["Resource"])
	cond := stmt["Condition"].(map[string]interface{})
	assert.Equal(t, map[string]interface{}{"StringEquals": map[string]interface{}{"AWS:SourceAccount": "123456789012"}}, cond)

	// Empty account: no Condition is attached.
	policy, err = sesPublishTopicPolicy("arn:aws:sns:us-east-1:123456789012:topic", "")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(policy), &decoded))
	stmt = decoded["Statement"].([]interface{})[0].(map[string]interface{})
	_, hasCond := stmt["Condition"]
	assert.False(t, hasCond)
}

func TestProviderGapSESAccountAndAWSErrHelpers(t *testing.T) {
	assert.Equal(t, "123456789012", accountIDFromARN("arn:aws:sns:us-east-1:123456789012:topic"))
	assert.Equal(t, "", accountIDFromARN("arn:aws:sns"))
	assert.Equal(t, "", accountIDFromARN(""))

	assert.False(t, isAWSErrCode(nil, "AlreadyExists"))
	assert.False(t, isAWSErrCode(errors.New("boom"), "AlreadyExists"))

	wrapped := wrapSESError(fmt.Errorf("boom"), "send email")
	assert.Equal(t, "failed to send email: boom", wrapped.Error())

	apiErr := wrapSESError(&smithy.GenericAPIError{Code: "Throttling", Message: "slow down"}, "send email")
	assert.Equal(t, "SES error: api error Throttling: slow down", apiErr.Error())
}

func TestProviderGapSESBuildDestinationSkipsEmpty(t *testing.T) {
	dest, err := buildSESDestination("to@example.com", []string{"", "cc@münchen.de"}, []string{"bcc@example.com", ""})
	require.NoError(t, err)
	assert.Equal(t, []string{"to@example.com"}, dest.ToAddresses)
	assert.Equal(t, []string{"cc@xn--mnchen-3ya.de"}, dest.CcAddresses)
	assert.Equal(t, []string{"bcc@example.com"}, dest.BccAddresses)

	dest, err = buildSESDestination("to@example.com", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"to@example.com"}, dest.ToAddresses)
	assert.Empty(t, dest.CcAddresses)
	assert.Empty(t, dest.BccAddresses)
}

func TestProviderGapSESCachedConfigurationSetExpiry(t *testing.T) {
	svc, _, _, _, _ := createMockSESService(t)
	svc.configSetCache.Store("int|us-east-1", configSetCacheEntry{name: "notifuse-int"})
	name, ok := svc.cachedConfigurationSet("int|us-east-1")
	assert.True(t, ok)
	assert.Equal(t, "notifuse-int", name)

	svc.configSetCache.Store("int2|us-east-1", configSetCacheEntry{name: "", expiresAt: time.Now().Add(-time.Minute)})
	_, ok = svc.cachedConfigurationSet("int2|us-east-1")
	assert.False(t, ok, "expired negative entry must be a miss")

	_, ok = svc.cachedConfigurationSet("absent|us-east-1")
	assert.False(t, ok)
}

func TestProviderGapSESSendEmailGuardsNoV2Call(t *testing.T) {
	svc, _, _, _, _, v2 := createMockSESServiceWithV2(t)
	// No v2 expectation is set: any SendEmail call on the mock fails the test.
	_ = v2
	provider := &domain.EmailProvider{SES: &domain.AmazonSESSettings{Region: "us-east-1"}}
	req := domain.SendEmailProviderRequest{
		WorkspaceID: "ws", IntegrationID: "int", MessageID: "m", FromAddress: "a@example.com",
		FromName: "A", To: "to@example.com", Subject: "s", Content: "c", Provider: provider,
	}
	err := svc.SendEmail(context.Background(), req)
	require.ErrorIs(t, err, ErrInvalidAWSCredentials)

	noSES := req
	noSES.Provider = &domain.EmailProvider{}
	err = svc.SendEmail(context.Background(), noSES)
	require.EqualError(t, err, "SES provider is not configured")

	invalid := req
	invalid.To = ""
	invalid.Provider = provider
	err = svc.SendEmail(context.Background(), invalid)
	require.EqualError(t, err, "invalid request: to address is required")
}

func TestProviderGapSESEnsureInboundMissingIntegrationNoDownstream(t *testing.T) {
	svc, mockSES, mockSNS, _, _ := createMockSESService(t)
	// Neither CreateTopic nor any SES call is expected: the guard must fail first.
	_ = mockSES
	_ = mockSNS
	provider := &domain.EmailProvider{SES: &domain.AmazonSESSettings{AccessKey: "ak", SecretKey: "sk", Region: "us-east-1"}}
	err := svc.EnsureInboundRoute(context.Background(), provider, "https://app.example/webhooks/email/inbound?workspace_id=ws1")
	require.EqualError(t, err, `inbound URL is missing integration_id: "https://app.example/webhooks/email/inbound?workspace_id=ws1"`)

	// Unsupported region soft-skips with nil and no downstream call.
	provider2 := &domain.EmailProvider{SES: &domain.AmazonSESSettings{AccessKey: "ak", SecretKey: "sk", Region: "eu-south-2"}}
	require.NoError(t, svc.EnsureInboundRoute(context.Background(), provider2, "https://app.example/x?integration_id=int1"))
	assert.False(t, svc.isInboundRegistered(context.Background(), *provider2.SES, "int1"))
}

func TestProviderGapSESInboundRecipientsFailsClosed(t *testing.T) {
	svc, mockSES, _, _, _ := createMockSESService(t)
	ctx := context.Background()

	mockSES.EXPECT().ListIdentitiesWithContext(gomock.Any(), gomock.Any()).Return(nil, errors.New("list down")).Times(1)
	_, err := svc.inboundRecipients(ctx, mockSES)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to list SES Domain identities:"), "got %q", err.Error())

	mockSES.EXPECT().ListIdentitiesWithContext(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *ses.ListIdentitiesInput, _ ...interface{}) (*ses.ListIdentitiesOutput, error) {
			return &ses.ListIdentitiesOutput{}, nil
		}).Times(2)
	rule, err := svc.buildInboundReceiptRule(ctx, mockSES, "notifuse-inbound-int1", "arn:aws:sns:us-east-1:123456789012:t")
	require.Nil(t, rule)
	require.EqualError(t, err, "no verified SES identities found; refusing to provision an account-wide inbound receipt rule")
}
