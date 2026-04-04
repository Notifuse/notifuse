package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectRailway_NoVars(t *testing.T) {
	// No RAILWAY_ vars in a clean test environment
	assert.False(t, DetectRailway())
}

func TestDetectRailway_ThreeOrMoreVars(t *testing.T) {
	t.Setenv("RAILWAY_ENVIRONMENT_NAME", "production")
	t.Setenv("RAILWAY_PROJECT_ID", "abc123")
	t.Setenv("RAILWAY_SERVICE_ID", "svc456")

	assert.True(t, DetectRailway())
}

func TestDetectRailway_OnlyTwoVars(t *testing.T) {
	t.Setenv("RAILWAY_ENVIRONMENT_NAME", "production")
	t.Setenv("RAILWAY_PROJECT_ID", "abc123")

	assert.False(t, DetectRailway())
}

func TestDetectRailway_ManyVars(t *testing.T) {
	t.Setenv("RAILWAY_ENVIRONMENT_NAME", "production")
	t.Setenv("RAILWAY_PROJECT_ID", "abc123")
	t.Setenv("RAILWAY_SERVICE_ID", "svc456")
	t.Setenv("RAILWAY_DEPLOYMENT_ID", "dep789")
	t.Setenv("RAILWAY_PUBLIC_DOMAIN", "app.up.railway.app")
	t.Setenv("RAILWAY_GIT_COMMIT_SHA", "deadbeef")

	assert.True(t, DetectRailway())
}

func TestCheckBlockedPlatforms_NotRailway(t *testing.T) {
	err := CheckBlockedPlatforms()
	assert.NoError(t, err)
}

func TestCheckBlockedPlatforms_Railway(t *testing.T) {
	t.Setenv("RAILWAY_ENVIRONMENT_NAME", "production")
	t.Setenv("RAILWAY_PROJECT_ID", "abc123")
	t.Setenv("RAILWAY_SERVICE_ID", "svc456")

	err := CheckBlockedPlatforms()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Railway.com has violated Notifuse's copyright")
	assert.Contains(t, err.Error(), "no longer supported")
}
