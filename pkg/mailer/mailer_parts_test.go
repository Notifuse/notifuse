package mailer

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wneessen/go-mail"
)

// Each system email is multipart/alternative with the text part first and the HTML part
// last, as RFC 2046 section 5.1.4 asks: a client shows the last part it can display.
func TestSMTPMailer_SystemEmailsSendTheTextPartFirst(t *testing.T) {
	m := NewTestSMTPMailer(&Config{FromEmail: "noreply@example.com", FromName: "Notifuse", APIEndpoint: "https://api.example.com/"})
	build := map[string]func() (*mail.Msg, error){
		"invitation": func() (*mail.Msg, error) {
			return m.invitationMessage("user@example.com", "Acme", "Ada", "tok123", "en")
		},
		"magic code": func() (*mail.Msg, error) { return m.magicCodeMessage("user@example.com", "123456", "en") },
		"circuit breaker alert": func() (*mail.Msg, error) {
			return m.circuitBreakerAlertMessage("user@example.com", "Acme", "Launch", "bounce rate too high", "en")
		},
		"data retention notice": func() (*mail.Msg, error) {
			return m.dataRetentionPendingMessage("user@example.com", "ws1", "Acme", time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), "en")
		},
	}
	for name, newMsg := range build {
		t.Run(name, func(t *testing.T) {
			msg, err := newMsg()
			require.NoError(t, err)
			var buf bytes.Buffer
			_, err = msg.WriteTo(&buf)
			require.NoError(t, err)
			raw := buf.String()

			assert.Contains(t, raw, "Content-Type: multipart/alternative")
			text := strings.Index(raw, "Content-Type: text/plain")
			html := strings.Index(raw, "Content-Type: text/html")
			require.NotEqual(t, -1, text, "a text part")
			require.NotEqual(t, -1, html, "an HTML part")
			assert.Less(t, text, html, "the text part comes before the HTML part")
			assert.Contains(t, raw, "To: <user@example.com>")
		})
	}
}

func TestSMTPMailer_InvitationMessage_Content(t *testing.T) {
	m := NewTestSMTPMailer(&Config{FromEmail: "noreply@example.com", FromName: "Notifuse", APIEndpoint: "https://api.example.com/"})
	msg, err := m.invitationMessage("user@example.com", "Acme", "Ada", "tok123", "en")
	require.NoError(t, err)
	assert.Equal(t, []string{"You've been invited to join Acme on Notifuse"}, msg.GetGenHeader(mail.HeaderSubject))
	var buf bytes.Buffer
	_, err = msg.WriteTo(&buf)
	require.NoError(t, err)
	// The parts are quoted-printable: "=" is written "=3D".
	assert.Contains(t, buf.String(), "<p>https://api.example.com/console/accept-invitation?token=3Dtok123</p>",
		"no double slash after the endpoint")
}

func TestSMTPMailer_DataRetentionPendingMessage_Content(t *testing.T) {
	m := NewTestSMTPMailer(&Config{FromEmail: "noreply@example.com", FromName: "Notifuse", APIEndpoint: "https://api.example.com/"})
	at := time.Date(2026, 12, 1, 23, 30, 0, 0, time.FixedZone("x", -3*3600)) // 2 December in UTC
	for _, lang := range []string{"en", "fr", "ja"} {
		t.Run(lang, func(t *testing.T) {
			msg, err := m.dataRetentionPendingMessage("owner@example.com", "ws1", "Acme <&> Co", at, lang)
			require.NoError(t, err)
			var buf bytes.Buffer
			_, err = msg.WriteTo(&buf)
			require.NoError(t, err)
			decoded, err := io.ReadAll(quotedprintable.NewReader(&buf))
			require.NoError(t, err)
			raw := string(decoded)
			tr := GetTranslations(lang)
			assert.Contains(t, raw, "2026-12-02", "the UTC day it applies")
			assert.Contains(t, raw, "https://api.example.com/console/workspace/ws1/settings/data-retention")
			assert.Contains(t, raw, "Acme &lt;&amp;&gt; Co", "the workspace name is escaped in the HTML part")
			assert.NotContains(t, raw, "<strong>Acme <&> Co</strong>")
			subject, err := new(mime.WordDecoder).DecodeHeader(strings.Join(msg.GetGenHeader("Subject"), ""))
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf(tr.RetentionPending.Subject, "Acme <&> Co"), subject)
		})
	}
}
