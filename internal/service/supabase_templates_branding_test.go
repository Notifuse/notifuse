package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/pkg/branding"
	"github.com/Notifuse/notifuse/pkg/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupabaseService_StarterTreeFollowsTheBrand(t *testing.T) {
	render := func(s *SupabaseService) string {
		tree, err := s.starterTree(templates.CreateSupabaseSignupEmailStructure)
		require.NoError(t, err)
		raw, err := json.Marshal(tree)
		require.NoError(t, err)
		return strings.ToLower(string(raw))
	}

	assert.Contains(t, render(&SupabaseService{}), "powered by", "without a brand source the template is unchanged")

	s := &SupabaseService{}
	shown := branding.Resolve(branding.Branding{})
	s.SetBranding(func() branding.Resolved { return shown })
	assert.Contains(t, render(s), "supabase-notifuse.png", "default mode keeps the Notifuse banner")

	shown = branding.Resolve(branding.Branding{Name: "Acme", LogoURL: "https://cdn.example.com/acme.png"})
	out := render(s)
	assert.NotContains(t, out, "notifuse", "read at creation, so a new brand needs no restart")
	assert.Contains(t, out, "https://cdn.example.com/acme.png")
}
