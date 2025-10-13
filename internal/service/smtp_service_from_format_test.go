package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wneessen/go-mail"
)

// TestGoMailFromFormat tests the go-mail library's FromFormat behavior
// to ensure it properly formats the From header with name and email
func TestGoMailFromFormat(t *testing.T) {
	t.Run("FromFormat with name sets both name and email", func(t *testing.T) {
		msg := mail.NewMsg()
		require.NotNil(t, msg)

		// Call FromFormat with name and email
		err := msg.FromFormat("Notifuse", "hello@notifuse.com")
		require.NoError(t, err)

		// Get the From addresses
		from := msg.GetFrom()
		require.Len(t, from, 1)

		// Verify the name and address are set correctly
		assert.Equal(t, "Notifuse", from[0].Name)
		assert.Equal(t, "hello@notifuse.com", from[0].Address)

		// Verify the String() output includes the name
		// Expected format: "Notifuse" <hello@notifuse.com>
		assert.Equal(t, `"Notifuse" <hello@notifuse.com>`, from[0].String())
	})

	t.Run("FromFormat with empty name only sets email", func(t *testing.T) {
		msg := mail.NewMsg()
		require.NotNil(t, msg)

		// Call FromFormat with empty name
		err := msg.FromFormat("", "hello@notifuse.com")
		require.NoError(t, err)

		// Get the From addresses
		from := msg.GetFrom()
		require.Len(t, from, 1)

		// Verify name is empty and address is set
		assert.Equal(t, "", from[0].Name)
		assert.Equal(t, "hello@notifuse.com", from[0].Address)

		// Verify the String() output does NOT include the name
		// Expected format: <hello@notifuse.com>
		assert.Equal(t, "<hello@notifuse.com>", from[0].String())
	})

	t.Run("FromFormat with special characters in name", func(t *testing.T) {
		msg := mail.NewMsg()
		require.NotNil(t, msg)

		// Call FromFormat with name containing special characters
		err := msg.FromFormat("Test User, Inc.", "test@example.com")
		require.NoError(t, err)

		// Get the From addresses
		from := msg.GetFrom()
		require.Len(t, from, 1)

		// Verify the name is preserved correctly
		assert.Equal(t, "Test User, Inc.", from[0].Name)
		assert.Equal(t, "test@example.com", from[0].Address)

		// The String() method should properly quote the name
		assert.Equal(t, `"Test User, Inc." <test@example.com>`, from[0].String())
	})

	t.Run("FromFormat with international characters", func(t *testing.T) {
		msg := mail.NewMsg()
		require.NotNil(t, msg)

		// Call FromFormat with name containing non-ASCII characters
		err := msg.FromFormat("Tëst Üser 日本", "test@example.com")
		require.NoError(t, err)

		// Get the From addresses
		from := msg.GetFrom()
		require.Len(t, from, 1)

		// Verify the name is preserved (RFC 2047 encoding happens during sending)
		assert.Equal(t, "Tëst Üser 日本", from[0].Name)
		assert.Equal(t, "test@example.com", from[0].Address)

		// The name should be present in the String() output
		// RFC 2047 encoding is applied automatically by mail.Address.String()
		fromString := from[0].String()
		assert.Contains(t, fromString, "test@example.com")
		// Name will be RFC 2047 encoded, so we just verify it's not empty
		assert.NotEqual(t, "<test@example.com>", fromString)
	})

	t.Run("FromFormat with invalid email returns error", func(t *testing.T) {
		msg := mail.NewMsg()
		require.NotNil(t, msg)

		// Call FromFormat with invalid email
		err := msg.FromFormat("Test User", "invalid-email")
		assert.Error(t, err)
	})

	t.Run("FromFormat with empty email returns error", func(t *testing.T) {
		msg := mail.NewMsg()
		require.NotNil(t, msg)

		// Call FromFormat with empty email
		err := msg.FromFormat("Test User", "")
		assert.Error(t, err)
	})
}

// TestSMTPService_FromNameInEmail tests that the SMTP service correctly
// passes the FromName to the go-mail library
func TestSMTPService_FromNameInEmail(t *testing.T) {
	t.Run("Verify FromName is used in email message", func(t *testing.T) {
		// This test creates a real mail.Msg object and verifies
		// that calling FromFormat with the values from SendEmailProviderRequest
		// produces the expected output

		fromName := "Notifuse Platform"
		fromAddress := "noreply@notifuse.com"

		msg := mail.NewMsg()
		require.NotNil(t, msg)

		// Simulate what the SMTP service does
		err := msg.FromFormat(fromName, fromAddress)
		require.NoError(t, err)

		// Verify the From header is set correctly
		from := msg.GetFrom()
		require.Len(t, from, 1)

		assert.Equal(t, fromName, from[0].Name, "FromName should be set correctly")
		assert.Equal(t, fromAddress, from[0].Address, "FromAddress should be set correctly")

		// Verify the formatted output
		expectedFormat := `"Notifuse Platform" <noreply@notifuse.com>`
		assert.Equal(t, expectedFormat, from[0].String(), 
			"From header should include both name and email in RFC 5322 format")
	})
}
