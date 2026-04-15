package smtp_bridge

import "fmt"

// TLS posture for the SMTP bridge listener.
const (
	ModeOff      = "off"      // plaintext; no STARTTLS advertised
	ModeSTARTTLS = "starttls" // STARTTLS required on port 587
	ModeImplicit = "implicit" // TLS-from-byte-1 (SMTPS) on port 465
)

// ValidateMode returns an error when s is not one of the supported modes.
func ValidateMode(s string) error {
	switch s {
	case ModeOff, ModeSTARTTLS, ModeImplicit:
		return nil
	default:
		return fmt.Errorf("invalid SMTP bridge TLS mode %q (expected %q, %q, or %q)", s, ModeOff, ModeSTARTTLS, ModeImplicit)
	}
}
