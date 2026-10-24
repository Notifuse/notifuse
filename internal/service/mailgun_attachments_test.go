package service

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// The Mailgun multipart body is the wire contract: field names, the
// List-Unsubscribe pair, inline-part naming, and per-attachment content
// types are all things Mailgun (and the reply-matching Message-ID) depend
// on, and none of them is visible in the function's return value — so the
// request body itself is parsed and asserted.

type parsedPart struct {
	name        string
	filename    string
	contentType string
	body        string
}

func captureMailgunRequest(t *testing.T, httpClient *mocks.MockHTTPClient, status int, respBody string) *[]parsedPart {
	t.Helper()
	var parts []parsedPart
	httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		mediaType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(mediaType, "multipart/"), "request must be multipart")
		reader := multipart.NewReader(req.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			body, err := io.ReadAll(part)
			require.NoError(t, err)
			parts = append(parts, parsedPart{
				name:        part.FormName(),
				filename:    part.FileName(),
				contentType: part.Header.Get("Content-Type"),
				body:        string(body),
			})
		}
		user, pass, ok := req.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "api", user)
		assert.Equal(t, "mg-key", pass)
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(respBody)),
			Header:     make(http.Header),
		}, nil
	}).Times(1)
	return &parts
}

func mailgunAttachmentRequest() domain.SendEmailProviderRequest {
	png := base64.StdEncoding.EncodeToString([]byte("PNG-BYTES"))
	return domain.SendEmailProviderRequest{
		MessageID:   "msg-42",
		FromAddress: "sender@example.com",
		FromName:    "Sender",
		To:          "rcpt@example.com",
		Subject:     "Hello",
		Content:     "<p>Hi</p>",
		Provider: &domain.EmailProvider{
			Mailgun: &domain.MailgunSettings{APIKey: "mg-key", Domain: "example.com"},
		},
		EmailOptions: domain.EmailOptions{
			CC:                 []string{"cc@example.com", ""},
			BCC:                []string{"bcc@example.com"},
			ReplyTo:            "reply@example.com",
			ListUnsubscribeURL: "https://example.com/unsub",
			Attachments: []domain.Attachment{
				{Filename: "report.pdf", Content: base64.StdEncoding.EncodeToString([]byte("PDF-BYTES")), ContentType: "application/pdf"},
				{Filename: "logo.png", Content: png, Disposition: "inline", ContentID: "logo1", ContentType: "image/png"},
			},
		},
	}
}

func newMailgunServiceForAttachments(t *testing.T) (*MailgunService, *mocks.MockHTTPClient) {
	t.Helper()
	ctrl := gomock.NewController(t)
	httpClient := mocks.NewMockHTTPClient(ctrl)
	svc := NewMailgunService(httpClient, nil, logger.NewLoggerWithLevel("disabled"), "")
	return svc, httpClient
}

func TestMailgunSendEmailWithAttachments_BodyContract(t *testing.T) {
	svc, httpClient := newMailgunServiceForAttachments(t)
	parts := captureMailgunRequest(t, httpClient, http.StatusOK, `{"id":"<1>","message":"Queued"}`)

	err := svc.sendEmailWithAttachments(context.Background(),
		"https://api.mailgun.net/v3/example.com/messages", mailgunAttachmentRequest())
	require.NoError(t, err)

	fields := map[string]string{}
	var files []parsedPart
	for _, p := range *parts {
		if p.filename == "" {
			fields[p.name] = p.body
		} else {
			files = append(files, p)
		}
	}

	assert.Equal(t, "Sender <sender@example.com>", fields["from"])
	assert.Equal(t, "rcpt@example.com", fields["to"])
	assert.Equal(t, "Hello", fields["subject"])
	assert.Equal(t, "<p>Hi</p>", fields["html"])
	assert.Equal(t, "cc@example.com", fields["cc"], "empty CC entries are skipped")
	assert.Equal(t, "bcc@example.com", fields["bcc"])
	assert.Equal(t, "reply@example.com", fields["h:Reply-To"])
	// RFC 8058 one-click unsubscribe needs both headers, the URL in <>.
	assert.Equal(t, "<https://example.com/unsub>", fields["h:List-Unsubscribe"])
	assert.Equal(t, "List-Unsubscribe=One-Click", fields["h:List-Unsubscribe-Post"])
	assert.Equal(t, "msg-42", fields["v:notifuse_message_id"])
	assert.Equal(t, domain.BuildRFCMessageID("msg-42", "sender@example.com"), fields["h:Message-Id"])

	require.Len(t, files, 2)
	assert.Equal(t, "attachment", files[0].name)
	assert.Equal(t, "report.pdf", files[0].filename)
	assert.Equal(t, "application/pdf", files[0].contentType)
	assert.Equal(t, "PDF-BYTES", files[0].body)

	// Inline parts are named by Content-ID so cid:logo1 resolves in the HTML.
	assert.Equal(t, "inline", files[1].name)
	assert.Equal(t, "logo1", files[1].filename)
	assert.Equal(t, "image/png", files[1].contentType)
	assert.Equal(t, "PNG-BYTES", files[1].body)
}

func TestMailgunSendEmailWithAttachments_Errors(t *testing.T) {
	ctx := context.Background()
	url := "https://api.mailgun.net/v3/example.com/messages"

	t.Run("non-200 surfaces the status code", func(t *testing.T) {
		svc, httpClient := newMailgunServiceForAttachments(t)
		captureMailgunRequest(t, httpClient, http.StatusForbidden, `{"message":"Forbidden"}`)
		err := svc.sendEmailWithAttachments(ctx, url, mailgunAttachmentRequest())
		require.EqualError(t, err, "API returned non-OK status code 403")
	})

	t.Run("transport failure is wrapped", func(t *testing.T) {
		svc, httpClient := newMailgunServiceForAttachments(t)
		sentinel := errors.New("connection refused")
		httpClient.EXPECT().Do(gomock.Any()).Return(nil, sentinel).Times(1)
		err := svc.sendEmailWithAttachments(ctx, url, mailgunAttachmentRequest())
		require.ErrorContains(t, err, "failed to execute request:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("an undecodable attachment fails before any HTTP call, naming its index", func(t *testing.T) {
		svc, _ := newMailgunServiceForAttachments(t)
		req := mailgunAttachmentRequest()
		req.EmailOptions.Attachments[1].Content = "!!! not base64 !!!"
		err := svc.sendEmailWithAttachments(ctx, url, req)
		require.ErrorContains(t, err, "attachment 1: failed to decode content:")
	})

	t.Run("an attachment without a content type defaults to octet-stream", func(t *testing.T) {
		svc, httpClient := newMailgunServiceForAttachments(t)
		parts := captureMailgunRequest(t, httpClient, http.StatusOK, `{}`)
		req := mailgunAttachmentRequest()
		req.EmailOptions.Attachments = []domain.Attachment{
			{Filename: "data.bin", Content: base64.StdEncoding.EncodeToString([]byte("X"))},
		}
		require.NoError(t, svc.sendEmailWithAttachments(ctx, url, req))
		for _, p := range *parts {
			if p.filename == "data.bin" {
				assert.Equal(t, "application/octet-stream", p.contentType)
				return
			}
		}
		t.Fatal("attachment part not found")
	})
}
