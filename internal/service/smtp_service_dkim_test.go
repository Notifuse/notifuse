package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"testing"

	msgdkim "github.com/emersion/go-msgauth/dkim"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/dkim"
)

const dkimTestSelector = "notifuse202611a3f29c1b04e87d21"

func dkimSigningKey(t *testing.T, domainName string) *domain.DKIMSigningKey {
	t.Helper()
	privPEM, _ := dkimServiceKey(t)
	key, err := dkim.ParsePrivateKey(privPEM)
	require.NoError(t, err)
	return &domain.DKIMSigningKey{Domain: domainName, Selector: dkimTestSelector, Key: key}
}

// unstuff turns the DATA section as the server received it back into the message.
func unstuff(data []byte) []byte {
	lines := strings.Split(string(data), "\r\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "..") {
			lines[i] = l[1:]
		}
	}
	return []byte(strings.Join(lines, "\r\n"))
}

func verifyDKIM(t *testing.T, message []byte, domainName string) []*msgdkim.Verification {
	t.Helper()
	_, pub := dkimServiceKey(t)
	verifications, err := msgdkim.VerifyWithOptions(bytes.NewReader(message), &msgdkim.VerifyOptions{
		LookupTXT: func(name string) ([]string, error) {
			if name == dkim.RecordName(dkimTestSelector, domainName) {
				return []string{dkim.FormatDNSValue(pub)}, nil
			}
			return nil, errors.New("unexpected lookup " + name)
		},
	})
	require.NoError(t, err)
	return verifications
}

func dkimSMTPRequest(port int) domain.SendEmailProviderRequest {
	return domain.SendEmailProviderRequest{
		WorkspaceID:   "workspace-123",
		IntegrationID: "integration-123",
		MessageID:     "message-123",
		FromAddress:   "news@acme.io",
		FromName:      "Acme Été",
		To:            "zoe@example.com",
		Subject:       "Événement d'été à Zürich",
		Content:       "<p>Bonjour Zoé, à bientôt.</p>\n.<b>une ligne qui commence par un point</b>",
		TextContent:   "Bonjour Zoé,\n.une ligne qui commence par un point\nÀ bientôt",
		Provider: &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{Host: "127.0.0.1", Port: port},
		},
		EmailOptions: domain.EmailOptions{
			ReplyTo:            "support@acme.io",
			ListUnsubscribeURL: "https://acme.io/unsubscribe/abc",
			Attachments: []domain.Attachment{{
				Filename:    "report.bin",
				Content:     base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 0xff, 0xfe, '\n', '.', '\n'}),
				ContentType: "application/octet-stream",
			}},
		},
	}
}

func TestSMTPService_SendEmail_SignsWithDKIM(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	request := dkimSMTPRequest(server.Port())
	request.DKIM = dkimSigningKey(t, "acme.io")
	require.NoError(t, NewSMTPService(&noopLogger{}).SendEmail(context.Background(), request))

	messages := server.GetMessages()
	require.Len(t, messages, 1)
	message := unstuff(messages[0].data)

	verifications := verifyDKIM(t, message, "acme.io")
	require.Len(t, verifications, 1)
	assert.NoError(t, verifications[0].Err)
	assert.Equal(t, "acme.io", verifications[0].Domain)
	signed := strings.ToLower(strings.Join(verifications[0].HeaderKeys, ":"))
	assert.Contains(t, signed, "list-unsubscribe:list-unsubscribe:list-unsubscribe-post:list-unsubscribe-post")
	assert.NotContains(t, signed, "date")
	assert.NotContains(t, signed, "message-id")

	for i, b := range message {
		require.Less(t, b, byte(0x80), "byte %d of what the server received is not 7-bit", i)
	}
	assert.Regexp(t, regexp.MustCompile(`(?m)^Content-Transfer-Encoding: quoted-printable\r$`), string(message))
	assert.Regexp(t, regexp.MustCompile(`(?m)^Content-Transfer-Encoding: base64\r$`), string(message))
	assert.True(t, bytes.HasPrefix(message, []byte("DKIM-Signature:")), "the signature comes first")
}

func TestSMTPService_SendEmail_UnsignedWithoutKey(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	require.NoError(t, NewSMTPService(&noopLogger{}).SendEmail(context.Background(), dkimSMTPRequest(server.Port())))
	messages := server.GetMessages()
	require.Len(t, messages, 1)
	assert.NotContains(t, string(messages[0].data), "DKIM-Signature")
}

func TestSMTPService_SendEmail_KeyOfAnotherDomainIsASigningError(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	request := dkimSMTPRequest(server.Port())
	request.DKIM = dkimSigningKey(t, "other.io")
	err := NewSMTPService(&noopLogger{}).SendEmail(context.Background(), request)
	assert.True(t, errors.Is(err, dkim.ErrSign))
	assert.Empty(t, server.GetMessages(), "nothing is sent")
}

func TestSMTPService_SendEmail_InternationalizedSenderDomain(t *testing.T) {
	server := newMockSMTPServer(t, true)
	defer server.Close()

	request := dkimSMTPRequest(server.Port())
	request.FromAddress = "news@bücher.de"
	request.DKIM = dkimSigningKey(t, "xn--bcher-kva.de")
	require.NoError(t, NewSMTPService(&noopLogger{}).SendEmail(context.Background(), request))

	messages := server.GetMessages()
	require.Len(t, messages, 1)
	assert.Equal(t, "news@xn--bcher-kva.de", messages[0].from, "the envelope carries the ASCII form")
	assert.Contains(t, server.GetMailFromCommand(), "<news@xn--bcher-kva.de>")
	message := unstuff(messages[0].data)
	assert.Regexp(t, regexp.MustCompile(`(?m)^From: .*<news@xn--bcher-kva\.de>\r$`), string(message))

	verifications := verifyDKIM(t, message, "xn--bcher-kva.de")
	require.Len(t, verifications, 1)
	assert.NoError(t, verifications[0].Err)
}

func TestAsciiEmailDomain(t *testing.T) {
	assert.Equal(t, "news@xn--bcher-kva.de", asciiEmailDomain("news@bücher.de"))
	assert.Equal(t, "news@acme.io", asciiEmailDomain("news@acme.io"))
	assert.Equal(t, "no-at-sign", asciiEmailDomain("no-at-sign"))
}
