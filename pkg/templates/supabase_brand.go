package templates

import (
	"strings"

	"github.com/Notifuse/notifuse/pkg/branding"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// The Notifuse marks the Supabase starter templates carry.
const (
	supabaseBannerSrc  = "https://storage.googleapis.com/readonlydemo/supabase-notifuse.png"
	supabasePoweredBy  = `<br>Powered by <a class="editor-link" href="https://www.notifuse.com">Notifuse</a>`
	supabaseGitHubLink = "https://github.com/Notifuse/notifuse"
	supabaseXLink      = "https://x.com/notifuse"
)

// BrandSupabaseTemplate takes Notifuse's own marks out of a Supabase starter template when the
// instance shows its own brand: the banner becomes the brand's logo, or goes when the brand has
// none, and the "Powered by Notifuse" line and the Notifuse GitHub and X links go. A column,
// section or social block left empty by a removal goes too. In default mode the template is
// returned as it is. The tree is changed in place and returned.
func BrandSupabaseTemplate(tree notifuse_mjml.EmailBlock, brand branding.Resolved) notifuse_mjml.EmailBlock {
	if tree == nil || !brand.Custom {
		return tree
	}
	brandSupabaseBlock(tree, brand)
	return tree
}

// brandSupabaseBlock rebrands a block and its children. It reports whether the block stays.
func brandSupabaseBlock(block notifuse_mjml.EmailBlock, brand branding.Resolved) bool {
	attrs := block.GetAttributes()
	switch block.GetType() {
	case notifuse_mjml.MJMLComponentMjImage:
		if attrString(attrs, "src") == supabaseBannerSrc {
			if brand.LogoURL == "" {
				return false
			}
			attrs["src"] = brand.LogoURL
			attrs["alt"] = brand.Name
			attrs["srcset"] = ""
		}
	case notifuse_mjml.MJMLComponentMjSocialElement:
		if href := attrString(attrs, "href"); href == supabaseGitHubLink || href == supabaseXLink {
			return false
		}
	case notifuse_mjml.MJMLComponentMjText:
		if content := block.GetContent(); content != nil && strings.Contains(*content, supabasePoweredBy) {
			stripped := strings.Replace(*content, supabasePoweredBy, "", 1)
			block.SetContent(&stripped)
		}
	}

	children := block.GetChildren()
	if len(children) == 0 {
		return true
	}
	kept := children[:0:0]
	for _, child := range children {
		if child != nil && brandSupabaseBlock(child, brand) {
			kept = append(kept, child)
		}
	}
	block.SetChildren(kept)
	if len(kept) > 0 {
		return true
	}
	// Only the containers a removal can empty go; anything else keeps its place.
	switch block.GetType() {
	case notifuse_mjml.MJMLComponentMjColumn, notifuse_mjml.MJMLComponentMjSection, notifuse_mjml.MJMLComponentMjSocial:
		return false
	}
	return true
}

func attrString(attrs map[string]interface{}, key string) string {
	if attrs == nil {
		return ""
	}
	value, _ := attrs[key].(string)
	return value
}
