package mailer

import (
	"bytes"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wneessen/go-mail"
)

// allStrings collects every string of a translation set, nested sets included.
func allStrings(v reflect.Value, out *[]string) {
	switch v.Kind() {
	case reflect.String:
		*out = append(*out, v.String())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			allStrings(v.Field(i), out)
		}
	}
}

func TestTranslations_NameTheBrandThroughTheToken(t *testing.T) {
	for locale, set := range systemEmailTranslations {
		t.Run(locale, func(t *testing.T) {
			var raw []string
			allStrings(reflect.ValueOf(set), &raw)
			for _, s := range raw {
				assert.NotContains(t, s, "Notifuse", "a hard-coded product name in %q", s)
			}
			assert.Contains(t, set.Common.TeamName, brandToken, "the signature names the brand")
			assert.Contains(t, set.TwoFactor.EnabledSubject, brandToken)

			var filled []string
			allStrings(reflect.ValueOf(set.withBrand("Acme")), &filled)
			for _, s := range filled {
				assert.NotContains(t, s, brandToken, "a token left in %q", s)
			}
		})
	}
}

func TestTranslations_DefaultBrandKeepsTheNotifuseWording(t *testing.T) {
	en := GetTranslations("en").withBrand(DefaultBrandName)
	assert.Equal(t, "The Notifuse Team", en.Common.TeamName)
	assert.Equal(t, "Your Notifuse authentication code", en.MagicCode.Subject)
	assert.Equal(t, "You've been invited to join %s on Notifuse", en.Invitation.Subject)
}

func renderedMessage(t *testing.T, msg *mail.Msg) string {
	t.Helper()
	var buf bytes.Buffer
	_, err := msg.WriteTo(&buf)
	require.NoError(t, err)
	decoded, err := decodeQuotedPrintable(buf.String())
	require.NoError(t, err)
	return decoded
}

// The HTML part of a message, from its text/html header on.
func htmlPart(raw string) string {
	i := strings.Index(raw, "Content-Type: text/html")
	if i == -1 {
		return ""
	}
	return raw[i:]
}

func textPart(raw string) string {
	i := strings.Index(raw, "Content-Type: text/plain")
	j := strings.Index(raw, "Content-Type: text/html")
	if i == -1 || j == -1 || j < i {
		return ""
	}
	return raw[i:j]
}

func TestSMTPMailer_SystemEmailsCarryTheBrand(t *testing.T) {
	const brand = "A & B <Labs>"
	m := NewTestSMTPMailer(&Config{
		FromEmail:   "noreply@example.com",
		APIEndpoint: "https://api.example.com",
		BrandName:   func() string { return brand },
	})
	at := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

	for locale := range systemEmailTranslations {
		build := map[string]func() (*mail.Msg, error){
			"invitation": func() (*mail.Msg, error) { return m.invitationMessage("u@example.com", "Team", "Ada", "tok", locale) },
			"magic code": func() (*mail.Msg, error) { return m.magicCodeMessage("u@example.com", "123456", locale) },
			"circuit": func() (*mail.Msg, error) {
				return m.circuitBreakerAlertMessage("u@example.com", "Team", "Launch", "bounces", locale)
			},
			"retention": func() (*mail.Msg, error) {
				return m.dataRetentionPendingMessage("u@example.com", "ws1", "Team", at, locale)
			},
			"2FA enabled": func() (*mail.Msg, error) {
				msg, _, err := m.securityNoticeMessage("u@example.com", SecurityNotice{Kind: SecurityNoticeTwoFactorEnabled, At: at}, locale)
				return msg, err
			},
		}
		for name, newMsg := range build {
			t.Run(locale+"/"+name, func(t *testing.T) {
				msg, err := newMsg()
				require.NoError(t, err)
				raw := renderedMessage(t, msg)
				html, text := htmlPart(raw), textPart(raw)

				assert.NotContains(t, raw, "Notifuse", "the brand replaces Notifuse everywhere")
				assert.NotContains(t, raw, brandToken)
				assert.Contains(t, html, "A &amp; B &lt;Labs&gt;", "escaped in the HTML part")
				assert.NotContains(t, html, "<Labs>", "never raw markup in the HTML part")
				assert.Contains(t, text, brand, "as typed in the text part")
			})
		}
	}
}

func TestSMTPMailer_SubjectsCarryTheBrandAsTyped(t *testing.T) {
	m := NewTestSMTPMailer(&Config{FromEmail: "noreply@example.com", BrandName: func() string { return "A & B" }})
	msg, err := m.magicCodeMessage("u@example.com", "123456", "en")
	require.NoError(t, err)
	assert.Equal(t, []string{"Your A & B authentication code"}, msg.GetGenHeader(mail.HeaderSubject))
}

func TestSMTPMailer_WithoutABrandTheEmailsSayNotifuse(t *testing.T) {
	m := NewTestSMTPMailer(&Config{FromEmail: "noreply@example.com"})
	msg, err := m.magicCodeMessage("u@example.com", "123456", "en")
	require.NoError(t, err)
	raw := renderedMessage(t, msg)
	assert.Contains(t, raw, "The Notifuse Team")
	assert.Equal(t, []string{"Your Notifuse authentication code"}, msg.GetGenHeader(mail.HeaderSubject))
}

func TestConsoleMailer_PrintsTheBrand(t *testing.T) {
	m := NewConsoleMailerWithBrand(func() string { return "Acme" })
	out := captureOutput(func() {
		require.NoError(t, m.SendMagicCode("u@example.com", "123456", "en"))
	})
	assert.Contains(t, out, "Your Acme authentication code")
	assert.Contains(t, out, "The Acme Team")
	assert.NotContains(t, out, "Notifuse")
}

// withBrand doubles a "%" in the brand only in strings that hold one, which it takes for format
// strings. That holds while no translation carries a literal "%".
func TestTranslations_PercentOnlyInVerbs(t *testing.T) {
	verb := regexp.MustCompile(`%(\[\d+\])?[sdv]`)
	for locale, set := range systemEmailTranslations {
		var raw []string
		allStrings(reflect.ValueOf(set), &raw)
		for _, s := range raw {
			assert.Equal(t, strings.Count(s, "%"), len(verb.FindAllString(s, -1)), "%s: a literal %% in %q", locale, s)
		}
	}
}

func TestSMTPMailer_ABrandWithAPercentStaysWhole(t *testing.T) {
	const brand = "100% Mail"
	m := NewTestSMTPMailer(&Config{FromEmail: "noreply@example.com", APIEndpoint: "https://api.example.com", BrandName: func() string { return brand }})
	at := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	for locale := range systemEmailTranslations {
		build := map[string]func() (*mail.Msg, error){
			"invitation": func() (*mail.Msg, error) { return m.invitationMessage("u@example.com", "Team", "Ada", "tok", locale) },
			"magic code": func() (*mail.Msg, error) { return m.magicCodeMessage("u@example.com", "123456", locale) },
			"circuit": func() (*mail.Msg, error) {
				return m.circuitBreakerAlertMessage("u@example.com", "Team", "Launch", "bounces", locale)
			},
			"retention": func() (*mail.Msg, error) {
				return m.dataRetentionPendingMessage("u@example.com", "ws1", "Team", at, locale)
			},
			"recovery code used": func() (*mail.Msg, error) {
				msg, _, err := m.securityNoticeMessage("u@example.com", SecurityNotice{Kind: SecurityNoticeRecoveryCodeUsed, At: at, RecoveryCodesLeft: 3}, locale)
				return msg, err
			},
		}
		for name, newMsg := range build {
			t.Run(locale+"/"+name, func(t *testing.T) {
				msg, err := newMsg()
				require.NoError(t, err)
				raw := renderedMessage(t, msg)
				assert.NotContains(t, raw, "%!", "no broken verb")
				assert.NotContains(t, raw, "%%")
				assert.Contains(t, raw, brand)
				for _, subject := range msg.GetGenHeader(mail.HeaderSubject) {
					assert.NotContains(t, subject, "%!")
				}
			})
		}
	}
}

func TestSMTPMailer_NamesTypedByMembersAreEscapedInHTML(t *testing.T) {
	m := NewTestSMTPMailer(&Config{FromEmail: "noreply@example.com", APIEndpoint: "https://api.example.com"})
	const hostile = `<a href="https://evil.example">Verify</a>`
	invitation, err := m.invitationMessage("u@example.com", hostile, hostile, "tok", "en")
	require.NoError(t, err)
	alert, err := m.circuitBreakerAlertMessage("u@example.com", hostile, hostile, hostile, "en")
	require.NoError(t, err)
	for name, msg := range map[string]*mail.Msg{"invitation": invitation, "alert": alert} {
		html := htmlPart(renderedMessage(t, msg))
		assert.NotContains(t, html, `<a href="https://evil.example">`, name)
		assert.Contains(t, html, "&lt;a href=&#34;https://evil.example&#34;&gt;", name)
	}
}

func TestSMTPMailer_SenderNameFallsBackToTheBrand(t *testing.T) {
	from := func(cfg *Config) []string {
		msg, err := NewTestSMTPMailer(cfg).magicCodeMessage("u@example.com", "123456", "en")
		require.NoError(t, err)
		return msg.GetFromString()
	}
	assert.Equal(t, []string{`"Notifuse" <noreply@example.com>`}, from(&Config{FromEmail: "noreply@example.com"}))
	assert.Equal(t, []string{`"Acme" <noreply@example.com>`}, from(&Config{FromEmail: "noreply@example.com", BrandName: func() string { return "Acme" }}))
	assert.Equal(t, []string{`"Ops" <noreply@example.com>`}, from(&Config{FromEmail: "noreply@example.com", FromName: "Ops", BrandName: func() string { return "Acme" }}), "a set name wins")
}
