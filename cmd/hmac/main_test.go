package main

import (
	"bytes"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
)

const (
	hmacTestKey      = "a-secret-key-giving-32-bytes-of-key-material"
	hmacTestPrevious = "a-previous-key-giving-32-bytes-of-material"
)

func TestRun_PrintsTheHMACTheResetVerifies(t *testing.T) {
	var out bytes.Buffer

	code := run([]string{"root@example.com", hmacTestKey}, &out)

	assert.Equal(t, 0, code)
	hmac := domain.ComputeEmailHMAC("root@example.com", hmacTestKey)
	assert.True(t, domain.VerifyEmailHMAC("root@example.com", hmac, hmacTestKey))
	assert.Contains(t, out.String(), "HMAC: "+hmac+"\n")
	assert.Contains(t, out.String(), "Reset URL: /api/demo.reset?hmac="+hmac+"\n")
}

func TestRun_SignsWithTheFirstKeyOfAList(t *testing.T) {
	var out bytes.Buffer

	code := run([]string{"root@example.com", hmacTestKey + "," + hmacTestPrevious}, &out)

	assert.Equal(t, 0, code)
	assert.Contains(t, out.String(), "HMAC: "+domain.ComputeEmailHMAC("root@example.com", hmacTestKey)+"\n")
}

func TestRun_RefusesAKeyTheServerRefuses(t *testing.T) {
	var out bytes.Buffer

	code := run([]string{"root@example.com", "short"}, &out)

	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "too short")
	assert.NotContains(t, out.String(), "HMAC:")
}

func TestRun_MissingArgumentsPrintsUsage(t *testing.T) {
	var out bytes.Buffer

	code := run([]string{"root@example.com"}, &out)

	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "Usage: go run cmd/hmac/main.go <email> <secret_key>")
}
