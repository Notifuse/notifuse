package notifuse_mjml

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

const (
	// NanoIDAlphabet is the character set used for nanoid generation
	NanoIDAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
)

// GenerateSlug creates a URL-safe slug from a name with a nanoid suffix
// Returns format: {sanitized-name}-{6-char-nanoid}
// Example: "My Blog Post" -> "my-blog-post-abc123"
func GenerateSlug(name string) string {
	// Convert to lowercase
	slug := strings.ToLower(name)

	// Replace spaces and underscores with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")

	// Remove all characters except alphanumeric and hyphens
	reg := regexp.MustCompile(`[^a-z0-9-]+`)
	slug = reg.ReplaceAllString(slug, "")

	// Remove consecutive hyphens
	slug = regexp.MustCompile(`-+`).ReplaceAllString(slug, "-")

	// Trim hyphens from start and end
	slug = strings.Trim(slug, "-")

	// If slug is empty after sanitization, use a default
	if slug == "" {
		slug = "post"
	}

	// Append 6-character nanoid for uniqueness
	nanoid := GenerateNanoID(6)
	return fmt.Sprintf("%s-%s", slug, nanoid)
}

// ValidateSlug checks if a slug has the correct format
// Expected format: {user-part}-{6-char-nanoid}
// Example: "my-blog-post-abc123"
func ValidateSlug(slug string) bool {
	if slug == "" {
		return false
	}

	// Must match pattern: alphanumeric and hyphens, ending with -{6 chars}
	pattern := regexp.MustCompile(`^[a-z0-9-]+-[a-z0-9]{6}$`)
	return pattern.MatchString(slug)
}

// ExtractSlugBase extracts the user-defined part of the slug (without nanoid)
// Example: "my-blog-post-abc123" -> "my-blog-post"
func ExtractSlugBase(slug string) string {
	if !ValidateSlug(slug) {
		return slug
	}

	// Remove the last 7 characters (-{6chars})
	if len(slug) > 7 {
		return slug[:len(slug)-7]
	}

	return slug
}

// GenerateNanoID generates a cryptographically secure random ID of specified length
// using only lowercase alphanumeric characters (a-z, 0-9)
func GenerateNanoID(length int) string {
	if length <= 0 {
		length = 6
	}

	result := make([]byte, length)
	alphabetLen := big.NewInt(int64(len(NanoIDAlphabet)))

	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			// Fallback to less secure but functional method if crypto/rand fails
			result[i] = NanoIDAlphabet[i%len(NanoIDAlphabet)]
			continue
		}
		result[i] = NanoIDAlphabet[num.Int64()]
	}

	return string(result)
}
