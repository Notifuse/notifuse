package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name        string
		versionStr  string
		expected    float64
		expectError bool
	}{
		{
			name:       "valid version with v prefix",
			versionStr: "v3.14",
			expected:   3.0,
		},
		{
			name:       "valid version without v prefix",
			versionStr: "4.0",
			expected:   4.0,
		},
		{
			name:       "major version only",
			versionStr: "5",
			expected:   5.0,
		},
		{
			name:        "empty string",
			versionStr:  "",
			expectError: true,
		},
		{
			name:        "invalid format",
			versionStr:  "invalid",
			expectError: true,
		},
		{
			name:        "non-numeric major version",
			versionStr:  "abc.1",
			expectError: true,
		},
		{
			name:       "version with multiple dots",
			versionStr: "1.2.3",
			expected:   1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseVersion(tt.versionStr)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestGetCurrentCodeVersion(t *testing.T) {
	// This test assumes VERSION is set to "4.0" in config
	version, err := GetCurrentCodeVersion()

	require.NoError(t, err)
	assert.Equal(t, 6.0, version)
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		{
			name:     "v1 less than v2",
			v1:       "3.0",
			v2:       "4.0",
			expected: -1,
		},
		{
			name:     "v1 greater than v2",
			v1:       "5.0",
			v2:       "4.0",
			expected: 1,
		},
		{
			name:     "v1 equal to v2",
			v1:       "4.0",
			v2:       "4.0",
			expected: 0,
		},
		{
			name:     "v1 and v2 with different minor versions",
			v1:       "4.1",
			v2:       "4.2",
			expected: 0, // Only compares major version
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CompareVersions(tt.v1, tt.v2)

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompareVersionsErrors(t *testing.T) {
	tests := []struct {
		name        string
		v1          string
		v2          string
		expectError bool
	}{
		{
			name:        "invalid v1",
			v1:          "invalid",
			v2:          "4.0",
			expectError: true,
		},
		{
			name:        "invalid v2",
			v1:          "4.0",
			v2:          "invalid",
			expectError: true,
		},
		{
			name:        "both invalid",
			v1:          "invalid1",
			v2:          "invalid2",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CompareVersions(tt.v1, tt.v2)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsVersionSuperior(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		newVersion     string
		expected       bool
	}{
		{
			name:           "new version is superior",
			currentVersion: "3.0",
			newVersion:     "4.0",
			expected:       true,
		},
		{
			name:           "new version is not superior",
			currentVersion: "4.0",
			newVersion:     "3.0",
			expected:       false,
		},
		{
			name:           "versions are equal",
			currentVersion: "4.0",
			newVersion:     "4.0",
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := IsVersionSuperior(tt.currentVersion, tt.newVersion)

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsVersionSuperiorErrors(t *testing.T) {
	_, err := IsVersionSuperior("invalid", "4.0")
	assert.Error(t, err)

	_, err = IsVersionSuperior("4.0", "invalid")
	assert.Error(t, err)
}
