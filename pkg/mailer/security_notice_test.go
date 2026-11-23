package mailer

import (
	"bytes"
	"fmt"
	"mime/quotedprintable"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wneessen/go-mail"
)

var allSecurityNotices = []SecurityNoticeKind{
	SecurityNoticeTwoFactorEnabled,
	SecurityNoticeTwoFactorDisabled,
	SecurityNoticeTwoFactorReplaced,
	SecurityNoticeRecoveryCodeUsed,
	SecurityNoticeRecoveryCodesNew,
	SecurityNoticeTwoFactorReset,
	SecurityNoticeTwoFactorCodeFailures,
}

// renderSecurityNotice builds a notice and returns its subject and decoded raw message.
func renderSecurityNotice(t *testing.T, notice SecurityNotice, language string) (string, string) {
	t.Helper()
	m := NewTestSMTPMailer(&Config{FromEmail: "noreply@example.com", FromName: "Notifuse"})
	msg, subject, err := m.securityNoticeMessage("user@example.com", notice, language)
	require.NoError(t, err)
	var buf bytes.Buffer
	_, err = msg.WriteTo(&buf)
	require.NoError(t, err)
	decoded, err := decodeQuotedPrintable(buf.String())
	require.NoError(t, err)
	return subject, decoded
}

func decodeQuotedPrintable(raw string) (string, error) {
	var out bytes.Buffer
	_, err := out.ReadFrom(quotedprintable.NewReader(strings.NewReader(raw)))
	return out.String(), err
}

func TestSecurityNotice_EveryKindRendersInEveryLanguage(t *testing.T) {
	at := time.Date(2026, 11, 23, 9, 30, 0, 0, time.FixedZone("CET", 3600))
	for locale := range systemEmailTranslations {
		for _, kind := range allSecurityNotices {
			t.Run(locale+"/"+string(kind), func(t *testing.T) {
				subject, raw := renderSecurityNotice(t, SecurityNotice{
					Kind: kind, At: at, IP: "203.0.113.7", UserAgent: "Firefox/140", RecoveryCodesLeft: 7,
				}, locale)
				assert.NotEmpty(t, subject)
				assert.Contains(t, raw, "2026-11-23 08:30 UTC", "the time is given in UTC")
				assert.Contains(t, raw, "203.0.113.7")
				assert.Contains(t, raw, "Firefox/140")
				assert.NotContains(t, raw, "%!", "no broken format verb")
				if kind == SecurityNoticeRecoveryCodeUsed {
					assert.Contains(t, raw, "7", "the codes left")
				}
			})
		}
	}
}

func TestSecurityNotice_TextPartFirst(t *testing.T) {
	m := NewTestSMTPMailer(&Config{FromEmail: "noreply@example.com", FromName: "Notifuse"})
	msg, _, err := m.securityNoticeMessage("user@example.com", SecurityNotice{Kind: SecurityNoticeTwoFactorEnabled, At: time.Now()}, "en")
	require.NoError(t, err)
	assert.Equal(t, []string{"Two-factor authentication is on for your Notifuse account"}, msg.GetGenHeader(mail.HeaderSubject))
	var buf bytes.Buffer
	_, err = msg.WriteTo(&buf)
	require.NoError(t, err)
	raw := buf.String()
	text := strings.Index(raw, "Content-Type: text/plain")
	html := strings.Index(raw, "Content-Type: text/html")
	require.NotEqual(t, -1, text)
	require.NotEqual(t, -1, html)
	assert.Less(t, text, html)
}

// The User-Agent header is the attacker's choice, and the notice of repeated failures is
// one an attacker can trigger: what it says must reach the HTML part as text.
func TestSecurityNotice_EscapesTheBrowserString(t *testing.T) {
	_, raw := renderSecurityNotice(t, SecurityNotice{
		Kind: SecurityNoticeTwoFactorCodeFailures, At: time.Now(),
		IP: `<b>1.2.3.4</b>`, UserAgent: `<a href="https://evil.example">Reset your password</a>`,
	}, "en")
	htmlStart := strings.Index(raw, "Content-Type: text/html")
	require.NotEqual(t, -1, htmlStart)
	htmlPart := raw[htmlStart:]
	assert.NotContains(t, htmlPart, `<a href="https://evil.example">`)
	assert.Contains(t, htmlPart, `&lt;a href=&#34;https://evil.example&#34;&gt;Reset your password&lt;/a&gt;`)
	assert.NotContains(t, htmlPart, "<b>1.2.3.4</b>")
	// The text part is not HTML: the value stays as sent, on its own line.
	assert.Contains(t, raw[:htmlStart], "Browser: <a href=\"https://evil.example\">Reset your password</a>\r\n")
}

func TestCleanUserAgent(t *testing.T) {
	assert.Equal(t, "Mozilla/5.0 Firefox", cleanUserAgent("  Mozilla/5.0\r\nFirefox\x00 "))
	long := strings.Repeat("x", 500)
	cleaned := cleanUserAgent(long)
	assert.Equal(t, maxUserAgentLength+1, len([]rune(cleaned)), "200 characters and an ellipsis")
	assert.True(t, strings.HasSuffix(cleaned, "…"))
	assert.Equal(t, strings.Repeat("é", maxUserAgentLength)+"…", cleanUserAgent(strings.Repeat("é", 300)), "cut on characters, not bytes")
}

func TestSecurityNotice_UnknownValues(t *testing.T) {
	_, raw := renderSecurityNotice(t, SecurityNotice{Kind: SecurityNoticeTwoFactorDisabled, At: time.Now()}, "fr")
	assert.Contains(t, raw, "Adresse IP indiquée par la requête : inconnu")
	assert.Contains(t, raw, "Navigateur : inconnu")
}

func TestSecurityNotice_UnknownKind(t *testing.T) {
	m := NewTestSMTPMailer(&Config{FromEmail: "noreply@example.com", FromName: "Notifuse"})
	_, _, err := m.securityNoticeMessage("user@example.com", SecurityNotice{Kind: "nope"}, "en")
	assert.Error(t, err)
	assert.Error(t, NewConsoleMailer().SendSecurityNotice("user@example.com", SecurityNotice{Kind: "nope"}, "en"))
}

func TestSecurityNotice_UnsupportedLanguageFallsBackToEnglish(t *testing.T) {
	subject, _ := renderSecurityNotice(t, SecurityNotice{Kind: SecurityNoticeTwoFactorReset, At: time.Now()}, "xx")
	assert.Equal(t, "Two-factor authentication was reset on your Notifuse account", subject)
}

func TestSendSecurityNotice_TestModeAndConsole(t *testing.T) {
	m := NewTestSMTPMailer(&Config{FromEmail: "noreply@example.com", FromName: "Notifuse"})
	out := captureLog(func() {
		require.NoError(t, m.SendSecurityNotice("user@example.com", SecurityNotice{Kind: SecurityNoticeRecoveryCodesNew, At: time.Now()}, "en"))
	})
	assert.Contains(t, out, "recovery_codes_regenerated")
	require.NoError(t, NewConsoleMailer().SendSecurityNotice("user@example.com",
		SecurityNotice{Kind: SecurityNoticeRecoveryCodeUsed, At: time.Now(), RecoveryCodesLeft: 3}, "de"))
}

// Every locale of the system emails has every security notice string, and the recovery
// code notice keeps its argument.
func TestTwoFactorTranslations_Complete(t *testing.T) {
	require.Equal(t, len(systemEmailTranslations), len(twoFactorTranslations), "one set per system email locale")
	for locale, set := range systemEmailTranslations {
		v := reflect.ValueOf(set.TwoFactor)
		for i := 0; i < v.NumField(); i++ {
			assert.NotEmpty(t, v.Field(i).String(), "%s: %s", locale, v.Type().Field(i).Name)
		}
		assert.Contains(t, set.TwoFactor.RecoveryCodeUsedBody, "%[1]d", locale)
		assert.Equal(t, fmt.Sprintf(set.TwoFactor.RecoveryCodeUsedBody, 4), strings.Replace(set.TwoFactor.RecoveryCodeUsedBody, "%[1]d", "4", 1), locale)
	}
}
