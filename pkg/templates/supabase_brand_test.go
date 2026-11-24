package templates

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/pkg/branding"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func treeJSON(t *testing.T, tree notifuse_mjml.EmailBlock) string {
	t.Helper()
	raw, err := json.Marshal(tree)
	require.NoError(t, err)
	return string(raw)
}

func findImage(block notifuse_mjml.EmailBlock, src string) notifuse_mjml.EmailBlock {
	if block.GetType() == notifuse_mjml.MJMLComponentMjImage && block.GetAttributes()["src"] == src {
		return block
	}
	for _, child := range block.GetChildren() {
		if found := findImage(child, src); found != nil {
			return found
		}
	}
	return nil
}

func countType(block notifuse_mjml.EmailBlock, kind notifuse_mjml.MJMLComponentType) int {
	n := 0
	if block.GetType() == kind {
		n++
	}
	for _, child := range block.GetChildren() {
		n += countType(child, kind)
	}
	return n
}

func TestBrandSupabaseTemplate(t *testing.T) {
	custom := func(logo string) branding.Resolved {
		return branding.Resolve(branding.Branding{Name: "Acme & Co", LogoURL: logo})
	}

	for name, build := range AllSupabaseTemplates() {
		t.Run(name+"/default mode leaves the template as it is", func(t *testing.T) {
			before, err := build()
			require.NoError(t, err)
			after, err := build()
			require.NoError(t, err)
			BrandSupabaseTemplate(after, branding.Resolve(branding.Branding{}))
			assert.Equal(t, treeJSON(t, before), treeJSON(t, after))
		})

		t.Run(name+"/the logo replaces the banner", func(t *testing.T) {
			tree, err := build()
			require.NoError(t, err)
			images := countType(tree, notifuse_mjml.MJMLComponentMjImage)
			out := treeJSON(t, BrandSupabaseTemplate(tree, custom("https://cdn.example.com/acme.png")))

			logo := findImage(tree, "https://cdn.example.com/acme.png")
			require.NotNil(t, logo, "the logo is in the template")
			assert.Equal(t, "Acme & Co", logo.GetAttributes()["alt"])
			assert.Equal(t, images, countType(tree, notifuse_mjml.MJMLComponentMjImage), "the banner keeps its place")
			assert.NotContains(t, strings.ToLower(out), "notifuse", "no Notifuse mark is left")
		})

		t.Run(name+"/without a logo the banner goes", func(t *testing.T) {
			tree, err := build()
			require.NoError(t, err)
			images := countType(tree, notifuse_mjml.MJMLComponentMjImage)
			sections := countType(tree, notifuse_mjml.MJMLComponentMjSection)
			out := treeJSON(t, BrandSupabaseTemplate(tree, custom("")))

			assert.Equal(t, images-1, countType(tree, notifuse_mjml.MJMLComponentMjImage))
			assert.Equal(t, sections-1, countType(tree, notifuse_mjml.MJMLComponentMjSection), "its emptied section goes too")
			assert.NotContains(t, strings.ToLower(out), "notifuse")
			assert.NotContains(t, out, "Powered by")
			assert.Equal(t, 0, countType(tree, notifuse_mjml.MJMLComponentMjSocial), "the social block held only Notifuse links")
		})

		t.Run(name+"/the rebranded template still converts and round-trips", func(t *testing.T) {
			tree, err := build()
			require.NoError(t, err)
			BrandSupabaseTemplate(tree, custom(""))
			assert.Contains(t, notifuse_mjml.ConvertJSONToMJML(tree), "<mj-body")
			raw := treeJSON(t, tree)
			back, err := notifuse_mjml.UnmarshalEmailBlock([]byte(raw))
			require.NoError(t, err)
			assert.JSONEq(t, raw, treeJSON(t, back))
		})
	}

	t.Run("a nil tree is returned as it is", func(t *testing.T) {
		assert.Nil(t, BrandSupabaseTemplate(nil, custom("")))
	})
}
