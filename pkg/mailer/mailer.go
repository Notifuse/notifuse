package mailer

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/wneessen/go-mail"
)

//go:generate mockgen -destination=../mocks/mock_mailer.go -package=pkgmocks github.com/Notifuse/notifuse/pkg/mailer Mailer

// Mailer is the interface for sending emails. The trailing language argument
// selects the locale of the email content (see GetTranslations).
type Mailer interface {
	// SendWorkspaceInvitation sends an invitation email with the given token
	SendWorkspaceInvitation(email, workspaceName, inviterName, token, language string) error
	// SendMagicCode sends a magic code for authentication purposes
	SendMagicCode(email, code, language string) error
	// SendCircuitBreakerAlert sends a notification when a broadcast is paused due to circuit breaker
	SendCircuitBreakerAlert(email, workspaceName, broadcastName, reason, language string) error
}

// Config holds the configuration for the mailer
type Config struct {
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	FromEmail    string
	FromName     string
	APIEndpoint  string
	UseTLS       bool
	EHLOHostname string
}

// SMTPMailer implements the Mailer interface using SMTP
type SMTPMailer struct {
	config   *Config
	testMode bool
}

// NewSMTPMailer creates a new SMTP mailer
func NewSMTPMailer(config *Config) *SMTPMailer {
	return &SMTPMailer{
		config:   config,
		testMode: false,
	}
}

// NewTestSMTPMailer creates a new SMTP mailer in test mode (won't connect to SMTP server)
func NewTestSMTPMailer(config *Config) *SMTPMailer {
	return &SMTPMailer{
		config:   config,
		testMode: true,
	}
}

// SendWorkspaceInvitation sends an invitation email with the given token
func (m *SMTPMailer) SendWorkspaceInvitation(email, workspaceName, inviterName, token, language string) error {
	t := GetTranslations(language)

	// Strip trailing slash from API endpoint to avoid double slashes in URL
	endpoint := strings.TrimSuffix(m.config.APIEndpoint, "/")
	inviteURL := fmt.Sprintf("%s/console/accept-invitation?token=%s", endpoint, token)

	// Create a new message
	msg := mail.NewMsg(mail.WithNoDefaultUserAgent())

	// Set sender and recipient
	if err := msg.FromFormat(m.config.FromName, m.config.FromEmail); err != nil {
		return fmt.Errorf("failed to set email from address: %w", err)
	}

	if err := msg.To(email); err != nil {
		return fmt.Errorf("failed to set email recipient: %w", err)
	}

	// Set subject and body
	subject := fmt.Sprintf(t.Invitation.Subject, workspaceName)
	msg.Subject(subject)

	// Create HTML content
	htmlBody := fmt.Sprintf(`
	<html lang="%s">
		<body>
			<h1>%s</h1>
			<p>%s</p>
			<p>%s</p>
			<p>%s</p>
			<p><a href="%s">%s</a></p>
			<p>%s</p>
			<p>%s</p>
			<p>%s</p>
			<p>%s<br>%s</p>
		</body>
	</html>`,
		t.Lang,
		t.Invitation.Heading,
		t.Common.Greeting,
		fmt.Sprintf(t.Invitation.Body, inviterName, "<strong>"+workspaceName+"</strong>"),
		t.Invitation.ClickPrompt,
		inviteURL, t.Invitation.LinkText,
		t.Invitation.FallbackURL,
		inviteURL,
		t.Invitation.Expiry,
		t.Invitation.SignOff, t.Common.TeamName)

	// Set alternative body parts
	plainBody := fmt.Sprintf("%s\n\n%s\n\n%s\n\n%s\n\n%s\n%s",
		t.Common.Greeting,
		fmt.Sprintf(t.Invitation.Body, inviterName, workspaceName),
		fmt.Sprintf(t.Invitation.PlainLink, inviteURL),
		t.Invitation.Expiry,
		t.Invitation.SignOff, t.Common.TeamName)

	msg.SetBodyString(mail.TypeTextHTML, htmlBody)
	msg.AddAlternativeString(mail.TypeTextPlain, plainBody)

	// Create SMTP client
	client, err := m.createSMTPClient()
	if err != nil {
		return err
	}

	// For testing - log information if client is nil
	if client == nil {
		log.Printf("Sending invitation email to: %s", email)
		log.Printf("From: %s <%s>", m.config.FromName, m.config.FromEmail)
		log.Printf("Subject: %s", subject)
		log.Printf("Invitation URL: %s", inviteURL)
		return nil
	}

	// Send the email
	if err := client.DialAndSend(msg); err != nil {
		return fmt.Errorf("failed to send invitation email: %w", err)
	}

	return nil
}

// SendMagicCode sends an authentication magic code email
func (m *SMTPMailer) SendMagicCode(email, code, language string) error {
	t := GetTranslations(language)

	// Create a new message
	msg := mail.NewMsg(mail.WithNoDefaultUserAgent())

	// Set sender and recipient
	if err := msg.FromFormat(m.config.FromName, m.config.FromEmail); err != nil {
		return fmt.Errorf("failed to set email from address: %w", err)
	}

	if err := msg.To(email); err != nil {
		return fmt.Errorf("failed to set email recipient: %w", err)
	}

	// Set subject
	subject := t.MagicCode.Subject
	msg.Subject(subject)

	// Create HTML content
	htmlBody := fmt.Sprintf(`
	<html lang="%s">
		<body>
			<h1>%s</h1>
			<p>%s</p>
			<p>%s</p>
			<h2 style="font-size: 24px; letter-spacing: 3px; background-color: #f5f5f5; padding: 15px; display: inline-block; border-radius: 5px;">%s</h2>
			<p>%s</p>
			<p>%s</p>
			<p>%s<br>%s</p>
		</body>
	</html>`,
		t.Lang,
		t.MagicCode.Heading,
		t.Common.Greeting,
		t.MagicCode.Intro,
		code,
		t.MagicCode.Expiry,
		t.MagicCode.IgnoreNotice,
		t.MagicCode.SignOff, t.Common.TeamName)

	// Set alternative body parts
	plainBody := fmt.Sprintf("%s\n\n%s %s\n\n%s\n\n%s\n\n%s\n%s",
		t.Common.Greeting,
		t.MagicCode.Intro, code,
		t.MagicCode.Expiry,
		t.MagicCode.IgnoreNotice,
		t.MagicCode.SignOff, t.Common.TeamName)

	msg.SetBodyString(mail.TypeTextHTML, htmlBody)
	msg.AddAlternativeString(mail.TypeTextPlain, plainBody)

	// Create SMTP client
	client, err := m.createSMTPClient()
	if err != nil {
		return err
	}

	// For testing - log information if client is nil
	if client == nil {
		log.Printf("Sending magic code to: %s", email)
		log.Printf("From: %s <%s>", m.config.FromName, m.config.FromEmail)
		log.Printf("Subject: %s", subject)
		log.Printf("Code: %s", code)
		return nil
	}

	// Send the email
	if err := client.DialAndSend(msg); err != nil {
		return fmt.Errorf("failed to send magic code email: %w", err)
	}

	return nil
}

// SendCircuitBreakerAlert sends a notification when a broadcast is paused due to circuit breaker
func (m *SMTPMailer) SendCircuitBreakerAlert(email, workspaceName, broadcastName, reason, language string) error {
	t := GetTranslations(language)

	// Create a new message
	msg := mail.NewMsg(mail.WithNoDefaultUserAgent())

	// Set sender and recipient
	if err := msg.FromFormat(m.config.FromName, m.config.FromEmail); err != nil {
		return fmt.Errorf("failed to set email from address: %w", err)
	}

	if err := msg.To(email); err != nil {
		return fmt.Errorf("failed to set email recipient: %w", err)
	}

	// Set subject
	subject := fmt.Sprintf(t.CircuitBreaker.Subject, broadcastName)
	msg.Subject(subject)

	// Create HTML content
	htmlBody := fmt.Sprintf(`
	<html lang="%s">
		<body>
			<h1 style="color: #d32f2f;">%s</h1>
			<p>%s</p>
			<p>%s</p>

			<div style="background-color: #fff3cd; border: 1px solid #ffeaa7; padding: 15px; border-radius: 5px; margin: 20px 0;">
				<h3 style="color: #856404; margin-top: 0;">%s</h3>
				<p style="margin-bottom: 0; color: #856404;"><strong>%s</strong></p>
			</div>

			<p>%s<br>%s</p>
		</body>
	</html>`,
		t.Lang,
		t.CircuitBreaker.Heading,
		t.Common.Greeting,
		fmt.Sprintf(t.CircuitBreaker.Body, `<strong>"`+broadcastName+`"</strong>`, "<strong>"+workspaceName+"</strong>"),
		t.CircuitBreaker.ReasonLabel,
		reason,
		t.CircuitBreaker.SignOff, t.Common.TeamName)

	// Set alternative body parts
	plainBody := fmt.Sprintf("%s\n\n%s\n\n%s\n\n%s %s\n\n%s\n%s",
		t.CircuitBreaker.Heading,
		t.Common.Greeting,
		fmt.Sprintf(t.CircuitBreaker.Body, `"`+broadcastName+`"`, workspaceName),
		t.CircuitBreaker.ReasonLabel, reason,
		t.CircuitBreaker.SignOff, t.Common.TeamName)

	msg.SetBodyString(mail.TypeTextHTML, htmlBody)
	msg.AddAlternativeString(mail.TypeTextPlain, plainBody)

	// Create SMTP client
	client, err := m.createSMTPClient()
	if err != nil {
		return err
	}

	// For testing - log information if client is nil
	if client == nil {
		log.Printf("Sending circuit breaker alert to: %s", email)
		log.Printf("From: %s <%s>", m.config.FromName, m.config.FromEmail)
		log.Printf("Subject: %s", subject)
		log.Printf("Broadcast: %s", broadcastName)
		log.Printf("Workspace: %s", workspaceName)
		log.Printf("Reason: %s", reason)
		return nil
	}

	// Send the email
	if err := client.DialAndSend(msg); err != nil {
		return fmt.Errorf("failed to send circuit breaker alert email: %w", err)
	}

	return nil
}

// createSMTPClient creates and configures a new SMTP client
func (m *SMTPMailer) createSMTPClient() (*mail.Client, error) {
	// In test mode, return nil client to avoid SMTP connections
	if m.testMode {
		return nil, nil
	}

	// Determine TLS policy based on config
	tlsPolicy := mail.TLSOpportunistic
	if !m.config.UseTLS {
		tlsPolicy = mail.NoTLS
	}

	// Build client options
	clientOptions := []mail.Option{
		mail.WithPort(m.config.SMTPPort),
		mail.WithTLSPolicy(tlsPolicy),
		mail.WithTimeout(10 * time.Second),
	}

	// Only add authentication if username and password are provided
	// This allows for unauthenticated SMTP servers (e.g., local relays, port 25)
	if m.config.SMTPUsername != "" && m.config.SMTPPassword != "" {
		clientOptions = append(clientOptions,
			mail.WithUsername(m.config.SMTPUsername),
			mail.WithPassword(m.config.SMTPPassword),
			mail.WithSMTPAuth(selectSMTPAuthType(m.config.UseTLS)),
		)
	}

	// Set custom EHLO hostname if configured, fall back to from-email domain, then SMTP host
	ehlo := m.config.EHLOHostname
	if ehlo == "" {
		if idx := strings.LastIndex(m.config.FromEmail, "@"); idx >= 0 {
			ehlo = m.config.FromEmail[idx+1:]
		}
	}
	if ehlo == "" {
		ehlo = m.config.SMTPHost
	}
	clientOptions = append(clientOptions, mail.WithHELO(ehlo))

	client, err := mail.NewClient(m.config.SMTPHost, clientOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create SMTP client: %w", err)
	}

	return client, nil
}

// selectSMTPAuthType picks the SMTP authentication mechanism.
//
// When UseTLS is false, go-mail has two security gates that block PLAIN over
// an unencrypted connection: SMTPAuthAutoDiscover skips PLAIN/LOGIN at the
// mechanism-selection step, and SMTPAuthPlain refuses again at the AUTH step
// ("unencrypted connection" error). SMTPAuthPlainNoEnc bypasses both — the
// wire protocol is still standard "AUTH PLAIN", so any server that
// advertises AUTH PLAIN accepts it. The user already accepted plaintext
// transit by setting UseTLS=false, so this is consistent with their intent.
func selectSMTPAuthType(useTLS bool) mail.SMTPAuthType {
	if !useTLS {
		return mail.SMTPAuthPlainNoEnc
	}
	return mail.SMTPAuthAutoDiscover
}

// ConsoleMailer is a development implementation that just logs emails
type ConsoleMailer struct{}

// NewConsoleMailer creates a new console mailer for development
func NewConsoleMailer() *ConsoleMailer {
	return &ConsoleMailer{}
}

// SendWorkspaceInvitation logs the invitation details to console
func (m *ConsoleMailer) SendWorkspaceInvitation(email, workspaceName, inviterName, token, language string) error {
	t := GetTranslations(language)
	fmt.Println("==============================================================")
	fmt.Println("                 WORKSPACE INVITATION EMAIL                   ")
	fmt.Println("==============================================================")
	fmt.Printf("To: %s\n", email)
	fmt.Printf("Subject: %s\n\n", fmt.Sprintf(t.Invitation.Subject, workspaceName))
	fmt.Println("Email Content:")
	fmt.Printf("%s\n\n", t.Common.Greeting)
	fmt.Printf("%s\n\n", fmt.Sprintf(t.Invitation.Body, inviterName, workspaceName))
	fmt.Printf("%s\n\n", fmt.Sprintf(t.Invitation.PlainLink, token))
	fmt.Printf("%s\n\n", t.Invitation.Expiry)
	fmt.Printf("%s\n%s\n", t.Invitation.SignOff, t.Common.TeamName)
	fmt.Println("==============================================================")

	return nil
}

// SendMagicCode logs the magic code details to console
func (m *ConsoleMailer) SendMagicCode(email, code, language string) error {
	t := GetTranslations(language)
	fmt.Println("==============================================================")
	fmt.Println("                 AUTHENTICATION MAGIC CODE                    ")
	fmt.Println("==============================================================")
	fmt.Printf("To: %s\n", email)
	fmt.Printf("Subject: %s\n\n", t.MagicCode.Subject)
	fmt.Println("Email Content:")
	fmt.Printf("%s\n\n", t.Common.Greeting)
	fmt.Printf("%s %s\n\n", t.MagicCode.Intro, code)
	fmt.Printf("%s\n\n", t.MagicCode.Expiry)
	fmt.Printf("%s\n\n", t.MagicCode.IgnoreNotice)
	fmt.Printf("%s\n%s\n", t.MagicCode.SignOff, t.Common.TeamName)
	fmt.Println("==============================================================")

	return nil
}

// SendCircuitBreakerAlert logs the circuit breaker alert details to console
func (m *ConsoleMailer) SendCircuitBreakerAlert(email, workspaceName, broadcastName, reason, language string) error {
	t := GetTranslations(language)
	fmt.Println("==============================================================")
	fmt.Println("                 CIRCUIT BREAKER ALERT EMAIL                  ")
	fmt.Println("==============================================================")
	fmt.Printf("To: %s\n", email)
	fmt.Printf("Subject: %s\n\n", fmt.Sprintf(t.CircuitBreaker.Subject, broadcastName))
	fmt.Println("Email Content:")
	fmt.Printf("%s\n\n", t.CircuitBreaker.Heading)
	fmt.Printf("%s\n\n", t.Common.Greeting)
	fmt.Printf("%s\n\n", fmt.Sprintf(t.CircuitBreaker.Body, `"`+broadcastName+`"`, workspaceName))
	fmt.Printf("%s %s\n\n", t.CircuitBreaker.ReasonLabel, reason)
	fmt.Printf("%s\n%s\n", t.CircuitBreaker.SignOff, t.Common.TeamName)
	fmt.Println("==============================================================")

	return nil
}
