package integration

import (
	"testing"
	"time"

	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wneessen/go-mail"
)

// TestGoMailDirectSendToMailhog tests go-mail library directly sending to Mailhog
// This isolates whether the From name issue is in:
// - Our code
// - The go-mail library
// - Mailhog
func TestGoMailDirectSendToMailhog(t *testing.T) {
	testutil.SkipIfShort(t)

	// Clear Mailhog before test
	err := testutil.ClearMailhogMessages(t)
	require.NoError(t, err, "Should clear Mailhog messages")

	t.Run("send email with display name using FromFormat", func(t *testing.T) {
		// Create go-mail client directly
		client, err := mail.NewClient("localhost",
			mail.WithPort(1025),
			mail.WithTLSPolicy(mail.NoTLS),
			mail.WithTimeout(10*time.Second),
		)
		require.NoError(t, err, "Should create mail client")
		defer client.Close()

		// Create message
		msg := mail.NewMsg(mail.WithNoDefaultUserAgent())

		// Use FromFormat to set From header with display name
		fromName := "Test Display Name"
		fromEmail := "test@example.com"
		err = msg.FromFormat(fromName, fromEmail)
		require.NoError(t, err, "FromFormat should succeed")

		// Set other required fields
		err = msg.To("recipient@example.com")
		require.NoError(t, err, "To should succeed")

		msg.Subject("GoMail Direct Test - FromFormat")
		msg.SetBodyString(mail.TypeTextHTML, "<h1>Test</h1><p>Testing FromFormat</p>")

		// Check what the From header contains before sending
		fromAddrs := msg.GetFromString()
		t.Logf("From header in message object: %v", fromAddrs)
		require.NotEmpty(t, fromAddrs, "From header should be set")
		assert.Contains(t, fromAddrs[0], fromName, "From header should contain display name")

		// Send the email
		err = client.DialAndSend(msg)
		require.NoError(t, err, "Should send email successfully")

		// Wait for Mailhog to receive the email
		time.Sleep(1 * time.Second)

		// Retrieve the email from Mailhog
		messageID, err := testutil.WaitForMailhogMessageWithSubject(t, "GoMail Direct Test - FromFormat", 5*time.Second)
		require.NoError(t, err, "Should find email in Mailhog")

		// Get full message with headers
		message, err := testutil.GetMailhogMessageWithHeaders(t, messageID)
		require.NoError(t, err, "Should retrieve message with headers")

		// Verify From header in Mailhog
		fromHeader, ok := message.Content.Headers["From"]
		require.True(t, ok, "From header should exist in Mailhog message")

		t.Logf("From header in Mailhog: %s", fromHeader)
		t.Logf("Raw SMTP data: %s", message.Raw.Data[:min(500, len(message.Raw.Data))]) // First 500 chars

		// Check if display name is present in From header
		assert.Contains(t, fromHeader[0], fromName, 
			"From header should contain display name '%s'", fromName)
		assert.Contains(t, fromHeader[0], fromEmail, 
			"From header should contain email address '%s'", fromEmail)

		// Check raw SMTP data for From header
		assert.Contains(t, message.Raw.Data, fromName, 
			"Raw SMTP data should contain display name")
	})

	t.Run("send email with display name using From", func(t *testing.T) {
		// Clear Mailhog
		err := testutil.ClearMailhogMessages(t)
		require.NoError(t, err)

		// Create go-mail client directly
		client, err := mail.NewClient("localhost",
			mail.WithPort(1025),
			mail.WithTLSPolicy(mail.NoTLS),
			mail.WithTimeout(10*time.Second),
		)
		require.NoError(t, err, "Should create mail client")
		defer client.Close()

		// Create message
		msg := mail.NewMsg(mail.WithNoDefaultUserAgent())

		// Use From with manually formatted address
		fromName := "Another Display Name"
		fromEmail := "another@example.com"
		fromFormatted := "\"" + fromName + "\" <" + fromEmail + ">"
		
		t.Logf("Formatted From address: %s", fromFormatted)
		
		err = msg.From(fromFormatted)
		require.NoError(t, err, "From should succeed")

		// Set other required fields
		err = msg.To("recipient@example.com")
		require.NoError(t, err, "To should succeed")

		msg.Subject("GoMail Direct Test - From")
		msg.SetBodyString(mail.TypeTextHTML, "<h1>Test</h1><p>Testing From</p>")

		// Check what the From header contains before sending
		fromAddrs := msg.GetFromString()
		t.Logf("From header in message object: %v", fromAddrs)
		require.NotEmpty(t, fromAddrs, "From header should be set")
		assert.Contains(t, fromAddrs[0], fromName, "From header should contain display name")

		// Send the email
		err = client.DialAndSend(msg)
		require.NoError(t, err, "Should send email successfully")

		// Wait for Mailhog to receive the email
		time.Sleep(1 * time.Second)

		// Retrieve the email from Mailhog
		messageID, err := testutil.WaitForMailhogMessageWithSubject(t, "GoMail Direct Test - From", 5*time.Second)
		require.NoError(t, err, "Should find email in Mailhog")

		// Get full message with headers
		message, err := testutil.GetMailhogMessageWithHeaders(t, messageID)
		require.NoError(t, err, "Should retrieve message with headers")

		// Verify From header in Mailhog
		fromHeader, ok := message.Content.Headers["From"]
		require.True(t, ok, "From header should exist in Mailhog message")

		t.Logf("From header in Mailhog: %s", fromHeader)
		t.Logf("Raw SMTP data: %s", message.Raw.Data[:min(500, len(message.Raw.Data))]) // First 500 chars

		// Check if display name is present in From header
		assert.Contains(t, fromHeader[0], fromName, 
			"From header should contain display name '%s'", fromName)
		assert.Contains(t, fromHeader[0], fromEmail, 
			"From header should contain email address '%s'", fromEmail)

		// Check raw SMTP data for From header
		assert.Contains(t, message.Raw.Data, fromName, 
			"Raw SMTP data should contain display name")
	})

	t.Run("send email without display name", func(t *testing.T) {
		// Clear Mailhog
		err := testutil.ClearMailhogMessages(t)
		require.NoError(t, err)

		// Create go-mail client directly
		client, err := mail.NewClient("localhost",
			mail.WithPort(1025),
			mail.WithTLSPolicy(mail.NoTLS),
			mail.WithTimeout(10*time.Second),
		)
		require.NoError(t, err, "Should create mail client")
		defer client.Close()

		// Create message
		msg := mail.NewMsg(mail.WithNoDefaultUserAgent())

		// Use From with just email address (no display name)
		fromEmail := "bare@example.com"
		err = msg.From(fromEmail)
		require.NoError(t, err, "From should succeed")

		// Set other required fields
		err = msg.To("recipient@example.com")
		require.NoError(t, err, "To should succeed")

		msg.Subject("GoMail Direct Test - No Display Name")
		msg.SetBodyString(mail.TypeTextHTML, "<h1>Test</h1><p>Testing without display name</p>")

		// Send the email
		err = client.DialAndSend(msg)
		require.NoError(t, err, "Should send email successfully")

		// Wait for Mailhog to receive the email
		time.Sleep(1 * time.Second)

		// Retrieve the email from Mailhog
		messageID, err := testutil.WaitForMailhogMessageWithSubject(t, "GoMail Direct Test - No Display Name", 5*time.Second)
		require.NoError(t, err, "Should find email in Mailhog")

		// Get full message with headers
		message, err := testutil.GetMailhogMessageWithHeaders(t, messageID)
		require.NoError(t, err, "Should retrieve message with headers")

		// Verify From header in Mailhog
		fromHeader, ok := message.Content.Headers["From"]
		require.True(t, ok, "From header should exist in Mailhog message")

		t.Logf("From header in Mailhog: %s", fromHeader)

		// When no display name is provided, From header should just be the email
		assert.Contains(t, fromHeader[0], fromEmail, 
			"From header should contain email address '%s'", fromEmail)
	})
}
