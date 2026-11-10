package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

const (
	plainTextTestHTML = "<p>Hello <a href=\"https://e.com\">there</a></p>"
	plainTextTestText = "Hello there (https://e.com)\n"
)

// capturedRequest holds what a provider sent to its HTTP API.
type capturedRequest struct {
	contentType string
	body        []byte
}

// recordingHTTPClient answers every call with status/response and records the request.
func recordingHTTPClient(t *testing.T, status int, response string) (*mocks.MockHTTPClient, *capturedRequest) {
	t.Helper()
	ctrl := gomock.NewController(t)
	client := mocks.NewMockHTTPClient(ctrl)
	captured := &capturedRequest{}
	client.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		captured.contentType = req.Header.Get("Content-Type")
		captured.body = body
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(bytes.NewBufferString(response)),
			Header:     make(http.Header),
		}, nil
	}).Times(1)
	return client, captured
}

func plainTextTestLogger(t *testing.T) *pkgmocks.MockLogger {
	t.Helper()
	ctrl := gomock.NewController(t)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Debug(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	return log
}

func plainTextProviderRequest(provider *domain.EmailProvider, text string) domain.SendEmailProviderRequest {
	return domain.SendEmailProviderRequest{
		WorkspaceID:   "ws",
		IntegrationID: "integration",
		MessageID:     "msg-1",
		FromAddress:   "sender@example.com",
		FromName:      "Sender",
		To:            "to@example.com",
		Subject:       "Subject",
		Content:       plainTextTestHTML,
		TextContent:   text,
		Provider:      provider,
	}
}

func pdfAttachment() domain.Attachment {
	return domain.Attachment{
		Filename:    "invoice.pdf",
		Content:     base64.StdEncoding.EncodeToString([]byte("%PDF-1.4")),
		ContentType: "application/pdf",
		Disposition: "attachment",
	}
}

func decodeJSONBody(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &decoded))
	return decoded
}

func TestSendGridService_SendEmail_PlainTextVersion(t *testing.T) {
	provider := &domain.EmailProvider{
		Kind:     domain.EmailProviderKindSendGrid,
		SendGrid: &domain.SendGridSettings{APIKey: "SG.key"},
	}

	t.Run("text/plain comes first, then text/html", func(t *testing.T) {
		client, captured := recordingHTTPClient(t, http.StatusAccepted, `{}`)
		svc := NewSendGridService(client, nil, plainTextTestLogger(t))

		require.NoError(t, svc.SendEmail(context.Background(), plainTextProviderRequest(provider, plainTextTestText)))

		content := decodeJSONBody(t, captured.body)["content"].([]interface{})
		require.Len(t, content, 2)
		assert.Equal(t, map[string]interface{}{"type": "text/plain", "value": plainTextTestText}, content[0])
		assert.Equal(t, map[string]interface{}{"type": "text/html", "value": plainTextTestHTML}, content[1])
	})

	t.Run("no text sends html only", func(t *testing.T) {
		client, captured := recordingHTTPClient(t, http.StatusAccepted, `{}`)
		svc := NewSendGridService(client, nil, plainTextTestLogger(t))

		require.NoError(t, svc.SendEmail(context.Background(), plainTextProviderRequest(provider, "")))

		content := decodeJSONBody(t, captured.body)["content"].([]interface{})
		require.Len(t, content, 1)
		assert.Equal(t, map[string]interface{}{"type": "text/html", "value": plainTextTestHTML}, content[0])
	})
}

func TestMailgunService_SendEmail_PlainTextVersion(t *testing.T) {
	provider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindMailgun,
		Mailgun: &domain.MailgunSettings{Domain: "example.com", APIKey: "key", Region: "US"},
	}
	const ok = `{"id":"<id@example.com>","message":"Queued. Thank you."}`

	formFields := func(t *testing.T, captured *capturedRequest) url.Values {
		t.Helper()
		values, err := url.ParseQuery(string(captured.body))
		require.NoError(t, err)
		return values
	}
	multipartFields := func(t *testing.T, captured *capturedRequest) map[string][]string {
		t.Helper()
		mediaType, params, err := mime.ParseMediaType(captured.contentType)
		require.NoError(t, err)
		require.Equal(t, "multipart/form-data", mediaType)
		form, err := multipart.NewReader(bytes.NewReader(captured.body), params["boundary"]).ReadForm(1 << 20)
		require.NoError(t, err)
		return form.Value
	}

	t.Run("form path sends text beside html", func(t *testing.T) {
		client, captured := recordingHTTPClient(t, http.StatusOK, ok)
		svc := NewMailgunService(client, nil, plainTextTestLogger(t), "https://hooks.example.com")

		require.NoError(t, svc.SendEmail(context.Background(), plainTextProviderRequest(provider, plainTextTestText)))

		fields := formFields(t, captured)
		assert.Equal(t, []string{plainTextTestText}, fields["text"])
		assert.Equal(t, []string{plainTextTestHTML}, fields["html"])
	})

	t.Run("form path without text has no text field", func(t *testing.T) {
		client, captured := recordingHTTPClient(t, http.StatusOK, ok)
		svc := NewMailgunService(client, nil, plainTextTestLogger(t), "https://hooks.example.com")

		require.NoError(t, svc.SendEmail(context.Background(), plainTextProviderRequest(provider, "")))

		_, present := formFields(t, captured)["text"]
		assert.False(t, present)
	})

	t.Run("multipart path sends text beside html", func(t *testing.T) {
		client, captured := recordingHTTPClient(t, http.StatusOK, ok)
		svc := NewMailgunService(client, nil, plainTextTestLogger(t), "https://hooks.example.com")
		request := plainTextProviderRequest(provider, plainTextTestText)
		request.EmailOptions.Attachments = []domain.Attachment{pdfAttachment()}

		require.NoError(t, svc.SendEmail(context.Background(), request))

		fields := multipartFields(t, captured)
		assert.Equal(t, []string{plainTextTestText}, fields["text"])
		assert.Equal(t, []string{plainTextTestHTML}, fields["html"])
	})

	t.Run("multipart path without text has no text field", func(t *testing.T) {
		client, captured := recordingHTTPClient(t, http.StatusOK, ok)
		svc := NewMailgunService(client, nil, plainTextTestLogger(t), "https://hooks.example.com")
		request := plainTextProviderRequest(provider, "")
		request.EmailOptions.Attachments = []domain.Attachment{pdfAttachment()}

		require.NoError(t, svc.SendEmail(context.Background(), request))

		_, present := multipartFields(t, captured)["text"]
		assert.False(t, present)
	})
}

func TestPostmarkService_SendEmail_PlainTextVersion(t *testing.T) {
	provider := &domain.EmailProvider{
		Kind:     domain.EmailProviderKindPostmark,
		Postmark: &domain.PostmarkSettings{ServerToken: "token"},
	}
	const ok = `{"ErrorCode":0,"Message":"OK","MessageID":"12345"}`

	t.Run("TextBody beside HtmlBody", func(t *testing.T) {
		client, captured := recordingHTTPClient(t, http.StatusOK, ok)
		svc := NewPostmarkService(client, nil, plainTextTestLogger(t))

		require.NoError(t, svc.SendEmail(context.Background(), plainTextProviderRequest(provider, plainTextTestText)))

		body := decodeJSONBody(t, captured.body)
		assert.Equal(t, plainTextTestText, body["TextBody"])
		assert.Equal(t, plainTextTestHTML, body["HtmlBody"])
	})

	t.Run("no text has no TextBody", func(t *testing.T) {
		client, captured := recordingHTTPClient(t, http.StatusOK, ok)
		svc := NewPostmarkService(client, nil, plainTextTestLogger(t))

		require.NoError(t, svc.SendEmail(context.Background(), plainTextProviderRequest(provider, "")))

		assert.NotContains(t, decodeJSONBody(t, captured.body), "TextBody")
	})
}

func TestMailjetService_SendEmail_PlainTextVersion(t *testing.T) {
	provider := &domain.EmailProvider{
		Kind:    domain.EmailProviderKindMailjet,
		Mailjet: &domain.MailjetSettings{APIKey: "key", SecretKey: "secret"},
	}
	const ok = `{"Messages":[{"Status":"success","To":[{"Email":"to@example.com","MessageID":1,"MessageUUID":"u"}]}]}`

	message := func(t *testing.T, captured *capturedRequest) map[string]interface{} {
		t.Helper()
		messages := decodeJSONBody(t, captured.body)["Messages"].([]interface{})
		require.Len(t, messages, 1)
		return messages[0].(map[string]interface{})
	}

	t.Run("TextPart beside HTMLPart", func(t *testing.T) {
		client, captured := recordingHTTPClient(t, http.StatusOK, ok)
		svc := NewMailjetService(client, nil, plainTextTestLogger(t))

		require.NoError(t, svc.SendEmail(context.Background(), plainTextProviderRequest(provider, plainTextTestText)))

		m := message(t, captured)
		assert.Equal(t, plainTextTestText, m["TextPart"])
		assert.Equal(t, plainTextTestHTML, m["HTMLPart"])
	})

	t.Run("no text has no TextPart", func(t *testing.T) {
		client, captured := recordingHTTPClient(t, http.StatusOK, ok)
		svc := NewMailjetService(client, nil, plainTextTestLogger(t))

		require.NoError(t, svc.SendEmail(context.Background(), plainTextProviderRequest(provider, "")))

		assert.NotContains(t, message(t, captured), "TextPart")
	})
}

func TestSparkPostService_SendEmail_PlainTextVersion(t *testing.T) {
	provider := &domain.EmailProvider{
		Kind:      domain.EmailProviderKindSparkPost,
		SparkPost: &domain.SparkPostSettings{Endpoint: "https://api.sparkpost.test", APIKey: "key"},
	}
	const ok = `{"results":{"id":"transmission"}}`

	content := func(t *testing.T, captured *capturedRequest) map[string]interface{} {
		t.Helper()
		return decodeJSONBody(t, captured.body)["content"].(map[string]interface{})
	}

	t.Run("content.text beside content.html", func(t *testing.T) {
		client, captured := recordingHTTPClient(t, http.StatusOK, ok)
		svc := NewSparkPostService(client, nil, plainTextTestLogger(t))

		require.NoError(t, svc.SendEmail(context.Background(), plainTextProviderRequest(provider, plainTextTestText)))

		c := content(t, captured)
		assert.Equal(t, plainTextTestText, c["text"])
		assert.Equal(t, plainTextTestHTML, c["html"])
	})

	t.Run("no text has no content.text", func(t *testing.T) {
		client, captured := recordingHTTPClient(t, http.StatusOK, ok)
		svc := NewSparkPostService(client, nil, plainTextTestLogger(t))

		require.NoError(t, svc.SendEmail(context.Background(), plainTextProviderRequest(provider, "")))

		assert.NotContains(t, content(t, captured), "text")
	})
}
