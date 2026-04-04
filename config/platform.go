package config

import (
	"fmt"
	"os"
	"strings"
)

// DetectRailway returns true if 3 or more environment variables with the
// "RAILWAY_" prefix are present, indicating a Railway.com deployment.
func DetectRailway() bool {
	count := 0
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "RAILWAY_") {
			count++
		}
	}
	return count >= 3
}

// CheckBlockedPlatforms returns an error if the application is running on a
// platform that is no longer supported.
func CheckBlockedPlatforms() error {
	if DetectRailway() {
		return fmt.Errorf("Notifuse cannot run on Railway.com. Railway.com has violated Notifuse's copyright and is therefore no longer supported.")
	}
	return nil
}
