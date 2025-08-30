package mailer

import (
	"fmt"
	"log"
	"time"

	"github.com/wneessen/go-mail"
)

// Mailer is the interface for sending emails
type Mailer interface {
	// SendWorkspaceInvitation sends an invitation email with the given token
	SendWorkspaceInvitation(email, workspaceName, inviterName, token string) error
	// SendMagicCode sends a magic code for authentication purposes
	SendMagicCode(email, code string) error
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
func (m *SMTPMailer) SendWorkspaceInvitation(email, workspaceName, inviterName, token string) error {
	inviteURL := fmt.Sprintf("%s/accept-invitation?token=%s", m.config.APIEndpoint, token)

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
	subject := fmt.Sprintf("You've been invited to join %s on Notifuse", workspaceName)
	msg.Subject(subject)

	// Create HTML content
	htmlBody := fmt.Sprintf(`
	<html>
		<body>
			<h1>You've been invited to join Notifuse!</h1>
			<p>Hello,</p>
			<p>%s has invited you to join the <strong>%s</strong> workspace on Notifuse.</p>
			<p>Click the link below to join:</p>
			<p><a href="%s">Accept invitation</a></p>
			<p>If the link doesn't work, copy and paste this URL into your browser:</p>
			<p>%s</p>
			<p>This invitation will expire in 7 days.</p>
			<p>Thanks,<br>The Notifuse Team</p>
		</body>
	</html>`, inviterName, workspaceName, inviteURL, inviteURL)

	// Set alternative body parts
	plainBody := fmt.Sprintf(
		"Hello,\n\n%s has invited you to join the %s workspace on Notifuse.\n\n"+
			"Use the following link to join: %s\n\n"+
			"This invitation will expire in 7 days.\n\n"+
			"Thanks,\nThe Notifuse Team", inviterName, workspaceName, inviteURL)

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
func (m *SMTPMailer) SendMagicCode(email, code string) error {
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
	subject := "Your Notifuse authentication code"
	msg.Subject(subject)

	// Create HTML content
	htmlBody := fmt.Sprintf(`
	<html>
		<body>
			<h1>Your authentication code</h1>
			<p>Hello,</p>
			<p>Your authentication code for Notifuse is:</p>
			<h2 style="font-size: 24px; letter-spacing: 3px; background-color: #f5f5f5; padding: 15px; display: inline-block; border-radius: 5px;">%s</h2>
			<p>The code will expire in 10 minutes.</p>
			<p>If you did not request this code, please ignore this email.</p>
			<p>Thanks,<br>The Notifuse Team</p>
		</body>
	</html>`, code)

	// Set alternative body parts
	plainBody := fmt.Sprintf(
		"Hello,\n\nYour authentication code for Notifuse is: %s\n\n"+
			"This code will expire in 10 minutes.\n\n"+
			"If you did not request this code, please ignore this email.\n\n"+
			"Thanks,\nThe Notifuse Team", code)

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

// createSMTPClient creates and configures a new SMTP client
func (m *SMTPMailer) createSMTPClient() (*mail.Client, error) {
	// In test mode, return nil client to avoid SMTP connections
	if m.testMode {
		return nil, nil
	}

	client, err := mail.NewClient(m.config.SMTPHost,
		mail.WithPort(m.config.SMTPPort),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(m.config.SMTPUsername),
		mail.WithPassword(m.config.SMTPPassword),
		mail.WithTLSPolicy(mail.TLSOpportunistic),
		mail.WithTimeout(10*time.Second),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create SMTP client: %w", err)
	}

	return client, nil
}

// ConsoleMailer is a development implementation that just logs emails
type ConsoleMailer struct{}

// NewConsoleMailer creates a new console mailer for development
func NewConsoleMailer() *ConsoleMailer {
	return &ConsoleMailer{}
}

// SendWorkspaceInvitation logs the invitation details to console
func (m *ConsoleMailer) SendWorkspaceInvitation(email, workspaceName, inviterName, token string) error {
	fmt.Println("==============================================================")
	fmt.Println("                 WORKSPACE INVITATION EMAIL                   ")
	fmt.Println("==============================================================")
	fmt.Printf("To: %s\n", email)
	fmt.Printf("Subject: You've been invited to join %s\n\n", workspaceName)
	fmt.Println("Email Content:")
	fmt.Printf("Hello,\n\n")
	fmt.Printf("%s has invited you to join the %s workspace on Notifuse.\n\n", inviterName, workspaceName)
	fmt.Printf("Use the following token to join: %s\n\n", token)
	fmt.Println("==============================================================")

	return nil
}

// SendMagicCode logs the magic code details to console
func (m *ConsoleMailer) SendMagicCode(email, code string) error {
	fmt.Println("==============================================================")
	fmt.Println("                 AUTHENTICATION MAGIC CODE                    ")
	fmt.Println("==============================================================")
	fmt.Printf("To: %s\n", email)
	fmt.Printf("Subject: Your authentication code\n\n")
	fmt.Println("Email Content:")
	fmt.Printf("Hello,\n\n")
	fmt.Printf("Your authentication code is: %s\n\n", code)
	fmt.Println("==============================================================")

	return nil
}
