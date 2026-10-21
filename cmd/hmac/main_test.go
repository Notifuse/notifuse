package main

import (
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
)

// The helper must produce exactly what the demo reset endpoint verifies, under the
// key it is given.
func TestEmailHMAC_MatchesServerVerification(t *testing.T) {
	got := emailHMAC("root@example.com", "signing-key")

	assert.Equal(t, domain.ComputeEmailHMAC("root@example.com", "signing-key"), got)
	assert.True(t, domain.VerifyEmailHMAC("root@example.com", got, "signing-key"))
	assert.False(t, domain.VerifyEmailHMAC("root@example.com", got, "secret-key"))
}
