package mailer

import (
	"fmt"
	"html"
	"log"
	"strings"
	"time"

	"github.com/wneessen/go-mail"
)

// SecurityNoticeKind names a security notice about two-factor authentication.
type SecurityNoticeKind string

const (
	SecurityNoticeTwoFactorEnabled      SecurityNoticeKind = "two_factor_enabled"
	SecurityNoticeTwoFactorDisabled     SecurityNoticeKind = "two_factor_disabled"
	SecurityNoticeRecoveryCodeUsed      SecurityNoticeKind = "recovery_code_used"
	SecurityNoticeRecoveryCodesNew      SecurityNoticeKind = "recovery_codes_regenerated"
	SecurityNoticeTwoFactorReset        SecurityNoticeKind = "two_factor_reset"
	SecurityNoticeTwoFactorCodeFailures SecurityNoticeKind = "two_factor_code_failures"
)

// maxUserAgentLength bounds the browser string shown in a notice. The User-Agent header is
// chosen by whoever sends the request, an attacker included.
const maxUserAgentLength = 200

// SecurityNotice describes one notice. IP is the address the request gave, which a client
// can forge through X-Forwarded-For: the notice says so. RecoveryCodesLeft is read only by
// SecurityNoticeRecoveryCodeUsed.
type SecurityNotice struct {
	Kind              SecurityNoticeKind
	At                time.Time
	IP                string
	UserAgent         string
	RecoveryCodesLeft int
}

// securityNoticeText returns the subject and body of a notice in t's language.
func securityNoticeText(t TwoFactorStrings, n SecurityNotice) (string, string, error) {
	switch n.Kind {
	case SecurityNoticeTwoFactorEnabled:
		return t.EnabledSubject, t.EnabledBody, nil
	case SecurityNoticeTwoFactorDisabled:
		return t.DisabledSubject, t.DisabledBody, nil
	case SecurityNoticeRecoveryCodeUsed:
		return t.RecoveryCodeUsedSubject, fmt.Sprintf(t.RecoveryCodeUsedBody, n.RecoveryCodesLeft), nil
	case SecurityNoticeRecoveryCodesNew:
		return t.RecoveryCodesNewSubject, t.RecoveryCodesNewBody, nil
	case SecurityNoticeTwoFactorReset:
		return t.ResetSubject, t.ResetBody, nil
	case SecurityNoticeTwoFactorCodeFailures:
		return t.FailuresSubject, t.FailuresBody, nil
	}
	return "", "", fmt.Errorf("unknown security notice %q", n.Kind)
}

// cleanUserAgent keeps a browser string printable on one line and bounded in length.
func cleanUserAgent(ua string) string {
	ua = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, ua)
	ua = strings.Join(strings.Fields(ua), " ")
	if runes := []rune(ua); len(runes) > maxUserAgentLength {
		ua = string(runes[:maxUserAgentLength]) + "…"
	}
	return ua
}

// securityNoticeDetails returns the three detail lines (label, value) of a notice.
func securityNoticeDetails(t TwoFactorStrings, n SecurityNotice) [][2]string {
	value := func(s string) string {
		if s == "" {
			return t.UnknownValue
		}
		return s
	}
	return [][2]string{
		{t.WhenLabel, n.At.UTC().Format("2006-01-02 15:04 UTC")},
		{t.IPLabel, value(strings.TrimSpace(n.IP))},
		{t.BrowserLabel, value(cleanUserAgent(n.UserAgent))},
	}
}

// securityNoticeMessage builds a security notice. Every value is escaped in the HTML part,
// the browser string first among them.
func (m *SMTPMailer) securityNoticeMessage(email string, n SecurityNotice, language string) (*mail.Msg, string, error) {
	tr := GetTranslations(language)
	t := tr.TwoFactor
	subject, body, err := securityNoticeText(t, n)
	if err != nil {
		return nil, "", err
	}
	details := securityNoticeDetails(t, n)

	var htmlDetails, plainDetails strings.Builder
	for _, d := range details {
		fmt.Fprintf(&htmlDetails, "<li>%s %s</li>", html.EscapeString(d[0]), html.EscapeString(d[1]))
		fmt.Fprintf(&plainDetails, "%s %s\n", d[0], d[1])
	}

	htmlBody := fmt.Sprintf(`
	<html lang="%s">
		<body>
			<h1>%s</h1>
			<p>%s</p>
			<p>%s</p>
			<ul>%s</ul>
			<p>%s</p>
			<p>%s<br>%s</p>
		</body>
	</html>`,
		html.EscapeString(tr.Lang),
		html.EscapeString(subject),
		html.EscapeString(tr.Common.Greeting),
		html.EscapeString(body),
		htmlDetails.String(),
		html.EscapeString(t.NotYouNotice),
		html.EscapeString(t.SignOff), html.EscapeString(tr.Common.TeamName))

	plainBody := fmt.Sprintf("%s\n\n%s\n\n%s\n%s\n\n%s\n%s",
		tr.Common.Greeting,
		body,
		plainDetails.String(),
		t.NotYouNotice,
		t.SignOff, tr.Common.TeamName)

	msg, err := m.newSystemMessage(email, subject, plainBody, htmlBody)
	return msg, subject, err
}

// SendSecurityNotice sends a security notice about two-factor authentication.
func (m *SMTPMailer) SendSecurityNotice(email string, notice SecurityNotice, language string) error {
	msg, subject, err := m.securityNoticeMessage(email, notice, language)
	if err != nil {
		return err
	}

	client, err := m.createSMTPClient()
	if err != nil {
		return err
	}
	if client == nil {
		log.Printf("Sending security notice %s to: %s", notice.Kind, email)
		log.Printf("Subject: %s", subject)
		return nil
	}
	if err := client.DialAndSend(msg); err != nil {
		return fmt.Errorf("failed to send security notice email: %w", err)
	}
	return nil
}

// SendSecurityNotice logs the security notice to the console.
func (m *ConsoleMailer) SendSecurityNotice(email string, notice SecurityNotice, language string) error {
	tr := GetTranslations(language)
	subject, body, err := securityNoticeText(tr.TwoFactor, notice)
	if err != nil {
		return err
	}
	fmt.Println("==============================================================")
	fmt.Println("                 SECURITY NOTICE EMAIL                        ")
	fmt.Println("==============================================================")
	fmt.Printf("To: %s\n", email)
	fmt.Printf("Subject: %s\n\n", subject)
	fmt.Printf("%s\n\n%s\n\n", tr.Common.Greeting, body)
	for _, d := range securityNoticeDetails(tr.TwoFactor, notice) {
		fmt.Printf("%s %s\n", d[0], d[1])
	}
	fmt.Printf("\n%s\n\n%s\n%s\n", tr.TwoFactor.NotYouNotice, tr.TwoFactor.SignOff, tr.Common.TeamName)
	fmt.Println("==============================================================")
	return nil
}
