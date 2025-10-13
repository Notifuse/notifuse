package service

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wneessen/go-mail"
)

// TestGoMailRawOutput tests the actual raw SMTP output from go-mail
func TestGoMailRawOutput(t *testing.T) {
	t.Run("Raw message with From name - hello example", func(t *testing.T) {
		msg := mail.NewMsg()
		
		// This is exactly what your code does
		err := msg.FromFormat("hello", "test@notifuse.com")
		require.NoError(t, err)
		
		err = msg.To("recipient@example.com")
		require.NoError(t, err)
		
		msg.Subject("Test Subject")
		msg.SetBodyString(mail.TypeTextHTML, "<h1>Test</h1>")
		
		// Write to buffer to see ACTUAL raw output
		var buf bytes.Buffer
		_, err = msg.WriteTo(&buf)
		require.NoError(t, err)
		
		rawMessage := buf.String()
		
		t.Log("======================================================================")
		t.Log("ACTUAL RAW SMTP MESSAGE OUTPUT:")
		t.Log("======================================================================")
		t.Log(rawMessage)
		t.Log("======================================================================")
		
		// Find the From header
		lines := strings.Split(rawMessage, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "From:") {
				t.Logf("\n✅ From header found: %s\n", line)
				
				// Check if name "hello" is included
				if strings.Contains(line, "hello") {
					t.Logf("✅✅ SUCCESS: From header INCLUDES the name 'hello'")
				} else {
					t.Errorf("❌ FAIL: From header does NOT include 'hello'")
					t.Errorf("   Expected: From header to contain 'hello'")
					t.Errorf("   Actual:   %s", line)
				}
			}
		}
	})
}

func TestGoMailRawOutputComparison(t *testing.T) {
	t.Run("Compare: With name vs Without name", func(t *testing.T) {
		// WITH NAME
		msgWithName := mail.NewMsg()
		msgWithName.FromFormat("hello", "test@notifuse.com")
		msgWithName.To("recipient@example.com")
		msgWithName.Subject("Test")
		msgWithName.SetBodyString(mail.TypeTextPlain, "Test")
		
		var bufWith bytes.Buffer
		msgWithName.WriteTo(&bufWith)
		
		// WITHOUT NAME (empty string)
		msgWithoutName := mail.NewMsg()
		msgWithoutName.FromFormat("", "test@notifuse.com")
		msgWithoutName.To("recipient@example.com")
		msgWithoutName.Subject("Test")
		msgWithoutName.SetBodyString(mail.TypeTextPlain, "Test")
		
		var bufWithout bytes.Buffer
		msgWithoutName.WriteTo(&bufWithout)
		
		// Extract From headers
		var fromWith, fromWithout string
		for _, line := range strings.Split(bufWith.String(), "\n") {
			if strings.HasPrefix(line, "From:") {
				fromWith = line
			}
		}
		for _, line := range strings.Split(bufWithout.String(), "\n") {
			if strings.HasPrefix(line, "From:") {
				fromWithout = line
			}
		}
		
		t.Log("======================================================================")
		t.Log("COMPARISON: With name vs Without name")
		t.Log("======================================================================")
		t.Logf("WITH name 'hello':    %s", fromWith)
		t.Logf("WITHOUT name (empty): %s", fromWithout)
		t.Log("======================================================================")
		
		// Verify
		if strings.Contains(fromWith, "hello") {
			t.Log("✅ WITH name: Correctly includes 'hello'")
		}
		
		if !strings.Contains(fromWithout, "hello") && strings.Contains(fromWithout, "test@notifuse.com") {
			t.Log("✅ WITHOUT name: Correctly shows only email (no name)")
		}
	})
}
