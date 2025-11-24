package notifuse_mjml

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSlug(t *testing.T) {
	t.Run("normal string", func(t *testing.T) {
		result := GenerateSlug("My Blog Post")
		// Should be "my-blog-post-" followed by 6-char nanoid
		assert.True(t, strings.HasPrefix(result, "my-blog-post-"))
		assert.Len(t, result, len("my-blog-post-")+6)
	})

	t.Run("hello world", func(t *testing.T) {
		result := GenerateSlug("Hello World")
		assert.True(t, strings.HasPrefix(result, "hello-world-"))
		assert.Len(t, result, len("hello-world-")+6)
	})

	t.Run("empty string", func(t *testing.T) {
		result := GenerateSlug("")
		// Should default to "post-" followed by nanoid
		assert.True(t, strings.HasPrefix(result, "post-"))
		assert.Len(t, result, len("post-")+6)
	})

	t.Run("underscores converted", func(t *testing.T) {
		result := GenerateSlug("Test___Post")
		assert.True(t, strings.HasPrefix(result, "test-post-"))
		assert.Len(t, result, len("test-post-")+6)
	})

	t.Run("special characters removed", func(t *testing.T) {
		result := GenerateSlug("Post@#$%Special")
		assert.True(t, strings.HasPrefix(result, "postspecial-"))
		assert.Len(t, result, len("postspecial-")+6)
	})

	t.Run("leading and trailing hyphens trimmed", func(t *testing.T) {
		result := GenerateSlug("---post---")
		assert.True(t, strings.HasPrefix(result, "post-"))
		assert.Len(t, result, len("post-")+6)
	})

	t.Run("consecutive hyphens collapsed", func(t *testing.T) {
		result := GenerateSlug("post--with---hyphens")
		assert.True(t, strings.HasPrefix(result, "post-with-hyphens-"))
		assert.Len(t, result, len("post-with-hyphens-")+6)
	})

	t.Run("generated nanoid always 6 characters", func(t *testing.T) {
		result := GenerateSlug("test")
		parts := strings.Split(result, "-")
		require.Len(t, parts, 2)
		assert.Len(t, parts[1], 6, "NanoID should be exactly 6 characters")
	})

	t.Run("generated nanoid contains only lowercase alphanumeric", func(t *testing.T) {
		result := GenerateSlug("test")
		parts := strings.Split(result, "-")
		require.Len(t, parts, 2)
		nanoid := parts[1]
		matched, _ := regexp.MatchString(`^[a-z0-9]{6}$`, nanoid)
		assert.True(t, matched, "NanoID should contain only lowercase alphanumeric characters")
	})
}

func TestValidateSlug(t *testing.T) {
	t.Run("valid format", func(t *testing.T) {
		assert.True(t, ValidateSlug("my-blog-post-abc123"))
	})

	t.Run("empty string", func(t *testing.T) {
		assert.False(t, ValidateSlug(""))
	})

	t.Run("missing nanoid suffix", func(t *testing.T) {
		assert.False(t, ValidateSlug("my-blog-post"))
	})

	t.Run("nanoid too short", func(t *testing.T) {
		assert.False(t, ValidateSlug("my-post-abc12"))
	})

	t.Run("nanoid too long", func(t *testing.T) {
		assert.False(t, ValidateSlug("my-post-abc1234"))
	})

	t.Run("invalid characters uppercase", func(t *testing.T) {
		assert.False(t, ValidateSlug("my-post-ABC123"))
	})

	t.Run("no hyphen separator", func(t *testing.T) {
		assert.False(t, ValidateSlug("myblogpostabc123"))
	})

	t.Run("valid with numbers in base", func(t *testing.T) {
		assert.True(t, ValidateSlug("post-123-abc123"))
	})

	t.Run("valid with multiple hyphens", func(t *testing.T) {
		assert.True(t, ValidateSlug("my-blog-post-title-abc123"))
	})
}

func TestExtractSlugBase(t *testing.T) {
	t.Run("valid slug extraction", func(t *testing.T) {
		result := ExtractSlugBase("my-blog-post-abc123")
		assert.Equal(t, "my-blog-post", result)
	})

	t.Run("invalid slug format returns original", func(t *testing.T) {
		result := ExtractSlugBase("my-post")
		assert.Equal(t, "my-post", result)
	})

	t.Run("short slug returns original", func(t *testing.T) {
		result := ExtractSlugBase("abc123")
		assert.Equal(t, "abc123", result)
	})

	t.Run("slug exactly 7 chars returns original", func(t *testing.T) {
		result := ExtractSlugBase("a-bc123")
		assert.Equal(t, "a-bc123", result)
	})

	t.Run("valid slug with numbers", func(t *testing.T) {
		result := ExtractSlugBase("post-123-abc123")
		assert.Equal(t, "post-123", result)
	})

	t.Run("valid slug with multiple hyphens", func(t *testing.T) {
		result := ExtractSlugBase("my-blog-post-title-abc123")
		assert.Equal(t, "my-blog-post-title", result)
	})
}

func TestGenerateNanoID(t *testing.T) {
	t.Run("generates ID of specified length", func(t *testing.T) {
		result := GenerateNanoID(10)
		assert.Len(t, result, 10)
	})

	t.Run("length <=0 defaults to 6", func(t *testing.T) {
		result := GenerateNanoID(0)
		assert.Len(t, result, 6)
	})

	t.Run("negative length defaults to 6", func(t *testing.T) {
		result := GenerateNanoID(-5)
		assert.Len(t, result, 6)
	})

	t.Run("all characters from NanoIDAlphabet", func(t *testing.T) {
		result := GenerateNanoID(100)
		alphabetSet := make(map[rune]bool)
		for _, r := range NanoIDAlphabet {
			alphabetSet[r] = true
		}

		for _, char := range result {
			assert.True(t, alphabetSet[char], "Character %c should be in NanoIDAlphabet", char)
		}
	})

	t.Run("generates different IDs", func(t *testing.T) {
		id1 := GenerateNanoID(10)
		id2 := GenerateNanoID(10)
		// Very unlikely to be the same, but possible
		// Just verify they're valid
		assert.Len(t, id1, 10)
		assert.Len(t, id2, 10)
	})

	t.Run("fallback mechanism works", func(t *testing.T) {
		// The fallback uses index-based selection if crypto/rand fails
		// We can't easily test the actual failure, but we can verify
		// the function always returns a valid result
		result := GenerateNanoID(6)
		assert.Len(t, result, 6)
		matched, _ := regexp.MatchString(`^[a-z0-9]{6}$`, result)
		assert.True(t, matched, "Result should be valid even with fallback")
	})
}

