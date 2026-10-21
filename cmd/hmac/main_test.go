package main

import (
	"bytes"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestRun_PrintsTheHMACTheResetVerifies(t *testing.T) {
	var out bytes.Buffer

	code := run([]string{"root@example.com", "signing-key"}, &out)

	assert.Equal(t, 0, code)
	hmac := domain.ComputeEmailHMAC("root@example.com", "signing-key")
	assert.True(t, domain.VerifyEmailHMAC("root@example.com", hmac, "signing-key"))
	assert.Contains(t, out.String(), "HMAC: "+hmac+"\n")
	assert.Contains(t, out.String(), "Reset URL: /api/demo.reset?hmac="+hmac+"\n")
}

func TestRun_MissingArgumentsPrintsUsage(t *testing.T) {
	var out bytes.Buffer

	code := run([]string{"root@example.com"}, &out)

	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "Usage: go run cmd/hmac/main.go <email> <signing_key>")
}
