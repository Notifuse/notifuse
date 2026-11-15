package mailer

import (
	"bytes"
	"strings"
	"testing"

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
