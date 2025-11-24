package service

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/wneessen/go-mail"
)

// SMTPService implements the domain.EmailProviderService interface for SMTP
type SMTPService struct {
	logger        logger.Logger
	clientFactory domain.SMTPClientFactory
}

// defaultGoMailFactory implements the domain.SMTPClientFactory interface directly using go-mail
type defaultGoMailFactory struct{}

func (f *defaultGoMailFactory) CreateClient(host string, port int, username, password string, useTLS bool) (*mail.Client, error) {
	var tlsPolicy mail.TLSPolicy
	var clientOptions []mail.Option

	// Configure TLS policy
	if useTLS {
		tlsPolicy = mail.TLSMandatory
	} else {
		// For local development servers like Mailpit, disable TLS completely
		tlsPolicy = mail.NoTLS
	}

	// Basic client options
	clientOptions = append(clientOptions,
		mail.WithPort(port),
		mail.WithTLSPolicy(tlsPolicy),
		mail.WithTimeout(10*time.Second),
	)

	// Only add authentication if username and password are provided
	// This allows for servers like Mailpit that don't require authentication
	if username != "" && password != "" {
		clientOptions = append(clientOptions,
			mail.WithUsername(username),
			mail.WithPassword(password),
			mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
		)
	}

	client, err := mail.NewClient(host, clientOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create mail client: %w", err)
	}

	return client, nil
}

// NewSMTPService creates a new instance of SMTPService
func NewSMTPService(logger logger.Logger) *SMTPService {
	return &SMTPService{
		logger:        logger,
		clientFactory: &defaultGoMailFactory{},
	}
}

// SendEmail sends an email using SMTP
func (s *SMTPService) SendEmail(ctx context.Context, request domain.SendEmailProviderRequest) error {
	// Validate the request
	if err := request.Validate(); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	if request.Provider.SMTP == nil {
		return fmt.Errorf("SMTP settings required")
	}

	// Create a client directly
	client, err := s.clientFactory.CreateClient(
		request.Provider.SMTP.Host,
		request.Provider.SMTP.Port,
		request.Provider.SMTP.Username,
		request.Provider.SMTP.Password,
		request.Provider.SMTP.UseTLS,
	)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	if client == nil {
		return fmt.Errorf("SMTP client factory returned nil client")
	}
	defer func() { _ = client.Close() }()

	// Create and configure the message
	msg := mail.NewMsg(mail.WithNoDefaultUserAgent())

	if err := msg.FromFormat(request.FromName, request.FromAddress); err != nil {
		return fmt.Errorf("invalid sender: %w", err)
	}
	if err := msg.To(request.To); err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}

	// Add CC recipients if specified (filter out empty strings)
	if len(request.EmailOptions.CC) > 0 {
		validCC := make([]string, 0, len(request.EmailOptions.CC))
		for _, ccAddr := range request.EmailOptions.CC {
			if ccAddr != "" {
				validCC = append(validCC, ccAddr)
			}
		}
		if len(validCC) > 0 {
			if err := msg.Cc(validCC...); err != nil {
				return fmt.Errorf("invalid CC recipients: %w", err)
			}
		}
	}

	// Add BCC recipients if specified (filter out empty strings)
	if len(request.EmailOptions.BCC) > 0 {
		validBCC := make([]string, 0, len(request.EmailOptions.BCC))
		for _, bccAddr := range request.EmailOptions.BCC {
			if bccAddr != "" {
				validBCC = append(validBCC, bccAddr)
			}
		}
		if len(validBCC) > 0 {
			if err := msg.Bcc(validBCC...); err != nil {
				return fmt.Errorf("invalid BCC recipients: %w", err)
			}
		}
	}

	// Add Reply-To if specified
	if request.EmailOptions.ReplyTo != "" {
		if err := msg.ReplyTo(request.EmailOptions.ReplyTo); err != nil {
			return fmt.Errorf("invalid reply-to address: %w", err)
		}
	}

	// Add message ID tracking header
	msg.SetGenHeader("X-Message-ID", request.MessageID)

	// Remove User-Agent and X-Mailer headers
	// msg.SetUserAgent("")

	msg.Subject(request.Subject)
	msg.SetBodyString(mail.TypeTextHTML, request.Content)

	// Add attachments if specified
	for i, att := range request.EmailOptions.Attachments {
		// Decode base64 content
		content, err := att.DecodeContent()
		if err != nil {
			return fmt.Errorf("attachment %d: failed to decode content: %w", i, err)
		}

		// Prepare file options for go-mail
		var fileOpts []mail.FileOption

		// Set content type if provided
		if att.ContentType != "" {
			fileOpts = append(fileOpts, mail.WithFileContentType(mail.ContentType(att.ContentType)))
		}

		// Add attachment or embed inline
		if att.Disposition == "inline" {
			// For inline attachments, set Content-ID for HTML references
			// Generate a simple Content-ID from filename (e.g., <logo.png>)
			contentID := att.Filename
			fileOpts = append(fileOpts, mail.WithFileContentID(contentID))
			_ = msg.EmbedReader(att.Filename, bytes.NewReader(content), fileOpts...)
		} else {
			_ = msg.AttachReader(att.Filename, bytes.NewReader(content), fileOpts...)
		}
	}

	// Send the email directly
	if err := client.DialAndSend(msg); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}
