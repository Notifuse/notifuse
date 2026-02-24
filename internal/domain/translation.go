package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// ResolveNestedKey traverses a nested map using a dot-separated key path
// and returns the string value. Returns empty string if key not found or value is not a string.
func ResolveNestedKey(data map[string]interface{}, key string) string {
	if data == nil || key == "" {
		return ""
	}

	parts := strings.Split(key, ".")
	var current interface{} = data

	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current, ok = m[part]
		if !ok {
			return ""
		}
	}

	if str, ok := current.(string); ok {
		return str
	}
	return ""
}

var placeholderRegex = regexp.MustCompile(`\{\{\s*(\w+)\s*\}\}`)

// InterpolatePlaceholders replaces {{ key }} placeholders in a translation value
// with the corresponding values from the args map.
func InterpolatePlaceholders(value string, args map[string]interface{}) string {
	if args == nil || len(args) == 0 {
		return value
	}

	return placeholderRegex.ReplaceAllStringFunc(value, func(match string) string {
		submatch := placeholderRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		key := submatch[1]
		if val, ok := args[key]; ok {
			return fmt.Sprintf("%v", val)
		}
		return match // leave unresolved placeholders as-is
	})
}

// ResolveLocale determines the best locale to use given a contact's language preference,
// available translation locales, and fallback defaults.
// Fallback chain: exact match -> base language -> template default -> workspace default.
func ResolveLocale(contactLanguage string, availableLocales []string, templateDefault *string, workspaceDefault string) string {
	if contactLanguage == "" {
		if templateDefault != nil && *templateDefault != "" {
			return *templateDefault
		}
		return workspaceDefault
	}

	contactLang := strings.ToLower(contactLanguage)

	// 1. Exact match (case-insensitive)
	for _, locale := range availableLocales {
		if strings.ToLower(locale) == contactLang {
			return locale
		}
	}

	// 2. Base language match (e.g., "pt-BR" -> "pt")
	if idx := strings.Index(contactLang, "-"); idx > 0 {
		baseLang := contactLang[:idx]
		for _, locale := range availableLocales {
			if strings.ToLower(locale) == baseLang {
				return locale
			}
		}
	}

	// 3. Template default language
	if templateDefault != nil && *templateDefault != "" {
		return *templateDefault
	}

	// 4. Workspace default language
	return workspaceDefault
}

// MergeTranslations deep-merges two translation maps. Values in override take priority.
func MergeTranslations(base, override map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy base
	for k, v := range base {
		result[k] = v
	}

	// Merge override
	for k, v := range override {
		if baseVal, exists := result[k]; exists {
			// If both are maps, deep merge
			baseMap, baseIsMap := baseVal.(map[string]interface{})
			overrideMap, overrideIsMap := v.(map[string]interface{})
			if baseIsMap && overrideIsMap {
				result[k] = MergeTranslations(baseMap, overrideMap)
				continue
			}
		}
		result[k] = v
	}

	return result
}
