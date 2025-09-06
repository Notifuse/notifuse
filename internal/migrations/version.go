package migrations

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Notifuse/notifuse/config"
)

// ParseVersion parses version string like "v3.14" or "3.14" and returns major version
func ParseVersion(versionStr string) (float64, error) {
	// Remove 'v' prefix if present
	cleanVersion := strings.TrimPrefix(versionStr, "v")

	// Split by dot to get major.minor
	parts := strings.Split(cleanVersion, ".")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid version format: %s", versionStr)
	}

	// Parse major version
	major, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid major version: %s", parts[0])
	}

	return major, nil
}

// GetCurrentCodeVersion returns the major version from config.VERSION
func GetCurrentCodeVersion() (float64, error) {
	return ParseVersion(config.VERSION)
}

// CompareVersions compares two version strings
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func CompareVersions(v1, v2 string) (int, error) {
	major1, err := ParseVersion(v1)
	if err != nil {
		return 0, fmt.Errorf("failed to parse version %s: %w", v1, err)
	}

	major2, err := ParseVersion(v2)
	if err != nil {
		return 0, fmt.Errorf("failed to parse version %s: %w", v2, err)
	}

	if major1 < major2 {
		return -1, nil
	} else if major1 > major2 {
		return 1, nil
	}
	return 0, nil
}

// IsVersionSuperior checks if newVersion is superior to currentVersion
func IsVersionSuperior(currentVersion, newVersion string) (bool, error) {
	comparison, err := CompareVersions(currentVersion, newVersion)
	if err != nil {
		return false, err
	}
	return comparison < 0, nil
}
