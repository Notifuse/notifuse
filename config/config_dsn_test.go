package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The system DSN is built by string formatting, not a URL builder, so the
// password-omission rule and the sslmode default are the whole contract:
// a passwordless config must produce no "password=" pair at all (an empty
// one changes libpq auth behavior), and sslmode must default to require.

func TestGetSystemDSN(t *testing.T) {
	withPassword := getSystemDSN(&DatabaseConfig{
		Host: "db.internal", Port: 6543, User: "app", Password: "s3cret", DBName: "notifuse_system",
	})
	assert.Equal(t,
		"host=db.internal port=6543 user=app password=s3cret dbname=notifuse_system sslmode=require",
		withPassword, "sslmode defaults to require when unset")

	withoutPassword := getSystemDSN(&DatabaseConfig{
		Host: "db.internal", Port: 5432, User: "app", DBName: "notifuse_system",
	})
	assert.Equal(t,
		"host=db.internal port=5432 user=app dbname=notifuse_system sslmode=require",
		withoutPassword)
	assert.NotContains(t, withoutPassword, "password=")

	explicit := getSystemDSN(&DatabaseConfig{
		Host: "db.internal", Port: 5432, User: "app", DBName: "notifuse_system", SSLMode: "disable",
	})
	assert.Contains(t, explicit, "sslmode=disable", "an explicit sslmode is never overridden")
}
