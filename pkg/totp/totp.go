// Package totp computes and checks time-based one-time passwords (RFC 6238) with the
// parameters every common authenticator app supports: HMAC-SHA-1, 6 digits, a 30-second
// step and a 20-byte secret.
//
// Match tells which time step a code matched, so a caller can refuse a code it already
// accepted (RFC 6238 §5.2): it stores the last accepted step and passes it back as
// afterStep.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // HMAC-SHA-1 is the HOTP PRF every authenticator app supports (RFC 4226, RFC 6238).
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// SecretSize is the secret length in bytes: 160 bits, as RFC 4226 recommends.
	SecretSize = 20
	// Digits is the code length.
	Digits = 6
	// Period is the length of a time step, in seconds.
	Period = 30
	// Skew is how many steps before and after the current one a code may come from.
	Skew = 1
)

// MatchResult says what Match found.
type MatchResult int

const (
	// MatchInvalid means no step in the window produces the code.
	MatchInvalid MatchResult = iota
	// MatchOK means the code matches a step after afterStep.
	MatchOK
	// MatchReplay means the code matches only a step at or before afterStep: it was
	// already accepted.
	MatchReplay
)

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret returns SecretSize random bytes.
func GenerateSecret() ([]byte, error) {
	secret := make([]byte, SecretSize)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("totp: generate secret: %w", err)
	}
	return secret, nil
}

// EncodeSecret returns the secret as unpadded base32, the form authenticator apps take.
func EncodeSecret(secret []byte) string {
	return encoding.EncodeToString(secret)
}

// Step returns the time step that contains t.
func Step(t time.Time) int64 {
	return t.Unix() / Period
}

// Code returns the code for the given step (HOTP with the step as counter, RFC 4226).
func Code(secret []byte, step int64) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, secret)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", Digits, value%1_000_000)
}

// Match checks code against the steps around now and returns the step it matched.
//
// It refuses a secret shorter than SecretSize and any code that is not exactly Digits
// ASCII digits; the caller strips spaces first. Every step of the window is compared in
// constant time, whatever the outcome. A code that matches a step after afterStep is
// MatchOK with that step (the earliest such step); one that matches only steps at or
// before afterStep is MatchReplay.
func Match(secret []byte, code string, now time.Time, afterStep int64) (int64, MatchResult) {
	if len(secret) < SecretSize || !isDigits(code) {
		return 0, MatchInvalid
	}
	current := Step(now)
	var matched int64
	result := MatchInvalid
	for step := current - Skew; step <= current+Skew; step++ {
		if subtle.ConstantTimeCompare([]byte(Code(secret, step)), []byte(code)) != 1 {
			continue
		}
		switch {
		case step > afterStep && result != MatchOK:
			matched, result = step, MatchOK
		case step <= afterStep && result == MatchInvalid:
			result = MatchReplay
		}
	}
	if result != MatchOK {
		return 0, result
	}
	return matched, MatchOK
}

func isDigits(code string) bool {
	if len(code) != Digits {
		return false
	}
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	return true
}

// KeyURI returns the otpauth:// URI an authenticator app reads from a QR code
// (Google's Key Uri Format). Colons are removed from the issuer and the account, which
// the label uses as its separator. The secret comes last, as some apps expect.
func KeyURI(issuer, account string, secret []byte) string {
	issuer = strings.ReplaceAll(issuer, ":", "")
	account = strings.ReplaceAll(account, ":", "")
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	return "otpauth://totp/" + label +
		"?issuer=" + url.QueryEscape(issuer) +
		"&algorithm=SHA1" +
		fmt.Sprintf("&digits=%d&period=%d", Digits, Period) +
		"&secret=" + EncodeSecret(secret)
}
