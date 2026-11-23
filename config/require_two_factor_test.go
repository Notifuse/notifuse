package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveRequireTwoFactor(t *testing.T) {
	stored := func(v bool) *SystemSettings { return &SystemSettings{RequireTwoFactor: v} }

	cases := []struct {
		name        string
		env         string
		settings    *SystemSettings
		isInstalled bool
		want        bool
	}{
		{"environment true wins over a stored false", "true", stored(false), true, true},
		{"environment false wins over a stored true", "false", stored(true), true, false},
		{"unset environment: the stored value", "", stored(true), true, true},
		{"unset environment, stored false", "", stored(false), true, false},
		{"not installed: only the environment counts", "", stored(true), false, false},
		{"not installed, environment true", "true", nil, false, true},
		{"installed without settings", "", nil, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, resolveRequireTwoFactor(tc.env, tc.settings, tc.isInstalled))
		})
	}
}
