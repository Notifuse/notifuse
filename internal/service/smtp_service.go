package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/wneessen/go-mail"
)

// getSMTPDialTimeout returns the SMTP dial timeout.
// Can be overridden via SMTP_DIAL_TIMEOUT environment variable for testing.
// Default is 30 seconds (industry standard for SMTP).
func getSMTPDialTimeout() time.Duration {
	if timeout := os.Getenv("SMTP_DIAL_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			return d
		}
	}
	return 30 * time.Second
}

// smtpConnection wraps a connection to an SMTP server and provides low-level
// command sending that avoids the SMTP extension issues (BODY=8BITMIME, SMTPUTF8)
// that both go-mail and net/smtp.Client.Mail() add when the server advertises
// support for them. This causes problems with strict SMTP servers like Sender.net
// (issue #172).
type smtpConnection struct {
	conn   net.Conn
	reader *bufio.Reader
}

func newSMTPConnection(conn net.Conn) *smtpConnection {
	return &smtpConnection{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

func (c *smtpConnection) readResponse() (int, string, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return 0, "", err
	}

	if len(line) < 4 {
		return 0, "", fmt.Errorf("short response: %s", line)
	}

	code := 0
	if _, err := fmt.Sscanf(line[:3], "%d", &code); err != nil {
		return 0, "", fmt.Errorf("invalid response code: %s", line)
	}

	return code, strings.TrimSpace(line[4:]), nil
}

func (c *smtpConnection) readMultilineResponse() (int, error) {
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return 0, err
		}

		if len(line) < 4 {
			return 0, fmt.Errorf("short response: %s", line)
		}

		code := 0
		if _, err := fmt.Sscanf(line[:3], "%d", &code); err != nil {
			return 0, fmt.Errorf("invalid response code: %s", line)
		}

		// If the 4th char is a space, it's the last line
		if line[3] == ' ' {
			return code, nil
		}
		// If it's a dash, continue reading
	}
}

func (c *smtpConnection) sendCommand(cmd string) (int, string, error) {
	if _, err := fmt.Fprintf(c.conn, "%s\r\n", cmd); err != nil {
		return 0, "", err
	}
	return c.readResponse()
}

func (c *smtpConnection) sendCommandMultiline(cmd string) (int, error) {
	if _, err := fmt.Fprintf(c.conn, "%s\r\n", cmd); err != nil {
		return 0, err
	}
	return c.readMultilineResponse()
}

func (c *smtpConnection) Close() error {
	return c.conn.Close()
}

// sendRawEmail sends an email using raw SMTP commands without the problematic
// SMTP extensions (BODY=8BITMIME, SMTPUTF8) that cause issues with strict SMTP
// servers like Sender.net (issue #172).
//
// Both go-mail and Go's standard library smtp.Client.Mail() automatically add
// these extensions when the server advertises support, so we need to bypass
// them by sending raw SMTP commands.
func sendRawEmail(host string, port int, username, password string, useTLS bool, from string, to []string, msg []byte) error {
	addr := fmt.Sprintf("%s:%d", host, port)

	// Connect to SMTP server with configurable timeout
	dialer := &net.Dialer{Timeout: getSMTPDialTimeout()}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	smtpConn := newSMTPConnection(conn)
	defer smtpConn.Close()

	// Read greeting (use multiline to handle RFC 5321 multi-line banners - issue #183)
	code, err := smtpConn.readMultilineResponse()
	if err != nil {
		return fmt.Errorf("failed to read greeting: %w", err)
	}
	if code != 220 {
		return fmt.Errorf("unexpected greeting code: %d", code)
	}

	// Send EHLO
	hostname := "localhost"
	code, err = smtpConn.sendCommandMultiline(fmt.Sprintf("EHLO %s", hostname))
	if err != nil {
		return fmt.Errorf("EHLO failed: %w", err)
	}
	if code != 250 {
		return fmt.Errorf("EHLO rejected with code: %d", code)
	}

	// STARTTLS if enabled
	if useTLS {
		code, _, err = smtpConn.sendCommand("STARTTLS")
		if err != nil {
			return fmt.Errorf("STARTTLS command failed: %w", err)
		}
		if code != 220 {
			return fmt.Errorf("STARTTLS rejected with code: %d", code)
		}

		// Upgrade connection to TLS
		tlsConfig := &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		}
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			return fmt.Errorf("TLS handshake failed: %w", err)
		}

		// Replace connection with TLS connection
		smtpConn = newSMTPConnection(tlsConn)
		defer smtpConn.Close()

		// Send EHLO again after TLS
		code, err = smtpConn.sendCommandMultiline(fmt.Sprintf("EHLO %s", hostname))
		if err != nil {
			return fmt.Errorf("EHLO after TLS failed: %w", err)
		}
		if code != 250 {
			return fmt.Errorf("EHLO after TLS rejected with code: %d", code)
		}
	}

	// AUTH if credentials provided
	if username != "" && password != "" {
		// Use AUTH PLAIN
		authString := fmt.Sprintf("\x00%s\x00%s", username, password)
		encoded := base64.StdEncoding.EncodeToString([]byte(authString))
		code, _, err = smtpConn.sendCommand(fmt.Sprintf("AUTH PLAIN %s", encoded))
		if err != nil {
			return fmt.Errorf("AUTH failed: %w", err)
		}
		if code != 235 {
			return fmt.Errorf("authentication failed with code: %d", code)
		}
	}

	// MAIL FROM - without any extensions (this is the key fix for issue #172)
	code, _, err = smtpConn.sendCommand(fmt.Sprintf("MAIL FROM:<%s>", from))
	if err != nil {
		return fmt.Errorf("MAIL FROM failed: %w", err)
	}
	if code != 250 {
		return fmt.Errorf("MAIL FROM rejected with code: %d", code)
	}

	// RCPT TO for each recipient
	for _, recipient := range to {
		if recipient == "" {
			continue
		}
		code, _, err = smtpConn.sendCommand(fmt.Sprintf("RCPT TO:<%s>", recipient))
		if err != nil {
			return fmt.Errorf("RCPT TO failed for %s: %w", recipient, err)
		}
		if code != 250 && code != 251 {
			return fmt.Errorf("RCPT TO rejected for %s with code: %d", recipient, code)
		}
	}

	// DATA
	code, _, err = smtpConn.sendCommand("DATA")
	if err != nil {
		return fmt.Errorf("DATA command failed: %w", err)
	}
	if code != 354 {
		return fmt.Errorf("DATA rejected with code: %d", code)
	}

	// Send message body
	// Ensure proper line endings and dot-stuffing
	if _, err := smtpConn.conn.Write(msg); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	// End message with CRLF.CRLF
	if _, err := fmt.Fprintf(smtpConn.conn, "\r\n.\r\n"); err != nil {
		return fmt.Errorf("failed to write message terminator: %w", err)
	}

	// Read response after DATA
	code, _, err = smtpConn.readResponse()
	if err != nil {
		return fmt.Errorf("failed to read DATA response: %w", err)
	}
	if code != 250 {
		return fmt.Errorf("message rejected with code: %d", code)
	}

	// QUIT
	_, _, _ = smtpConn.sendCommand("QUIT")

	return nil
}

// SMTPService implements the domain.EmailProviderService interface for SMTP
type SMTPService struct {
	logger logger.Logger
}

// NewSMTPService creates a new instance of SMTPService
func NewSMTPService(logger logger.Logger) *SMTPService {
	return &SMTPService{
		logger: logger,
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

	smtpSettings := request.Provider.SMTP

	// Create and configure the message using go-mail for MIME composition
	msg := mail.NewMsg(mail.WithNoDefaultUserAgent())

	if err := msg.FromFormat(request.FromName, request.FromAddress); err != nil {
		return fmt.Errorf("invalid sender: %w", err)
	}
	if err := msg.To(request.To); err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}

	// Collect all recipients for SMTP envelope
	recipients := []string{request.To}

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
			recipients = append(recipients, validCC...)
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
			recipients = append(recipients, validBCC...)
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

	// Add RFC-8058 List-Unsubscribe headers for one-click unsubscribe
	if request.EmailOptions.ListUnsubscribeURL != "" {
		msg.SetGenHeader("List-Unsubscribe", fmt.Sprintf("<%s>", request.EmailOptions.ListUnsubscribeURL))
		msg.SetGenHeader("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	}

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
			contentID := att.Filename
			fileOpts = append(fileOpts, mail.WithFileContentID(contentID))
			if err := msg.EmbedReader(att.Filename, bytes.NewReader(content), fileOpts...); err != nil {
				return fmt.Errorf("attachment %d: failed to embed inline: %w", i, err)
			}
		} else {
			if err := msg.AttachReader(att.Filename, bytes.NewReader(content), fileOpts...); err != nil {
				return fmt.Errorf("attachment %d: failed to attach: %w", i, err)
			}
		}
	}

	// Write the composed message to a buffer
	var buf bytes.Buffer
	if _, err := msg.WriteTo(&buf); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	// Send using native net/smtp (avoids BODY=8BITMIME extension issues - fix for issue #172)
	if err := sendRawEmail(
		smtpSettings.Host,
		smtpSettings.Port,
		smtpSettings.Username,
		smtpSettings.Password,
		smtpSettings.UseTLS,
		request.FromAddress,
		recipients,
		buf.Bytes(),
	); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}
