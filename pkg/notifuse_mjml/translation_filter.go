package notifuse_mjml

import (
	"fmt"
	"regexp"
	"strings"
)

// translationPlaceholderRegex matches {{ key }} placeholders in translation values.
var translationPlaceholderRegex = regexp.MustCompile(`\{\{\s*(\w+)\s*\}\}`)

// TranslationFilters provides the Liquid `t` filter for resolving translation keys.
// Register with SecureLiquidEngine.RegisterTranslations().
type TranslationFilters struct {
	translations map[string]interface{}
}

// T is the Liquid filter: {{ "welcome.heading" | t }}
// With placeholders: {{ "welcome.greeting" | t: name: "John" }}
//
// liquidgo calls this method with:
//   - input: the piped value (the translation key string)
//   - args: variadic positional args followed by an optional keyword args map
//
// liquidgo passes keyword args (name: value) as the last element
// in args if it's a map[string]interface{}.
func (tf *TranslationFilters) T(input interface{}, args ...interface{}) interface{} {
	keyStr := fmt.Sprintf("%v", input)

	value := resolveNestedKey(tf.translations, keyStr)
	if value == "" {
		return "[Missing translation: " + keyStr + "]"
	}

	// Check if last arg is a keyword args map
	var kwargs map[string]interface{}
	if len(args) > 0 {
		if m, ok := args[len(args)-1].(map[string]interface{}); ok {
			kwargs = m
		}
	}

	if len(kwargs) > 0 {
		value = interpolatePlaceholders(value, kwargs)
	}

	return value
}

// resolveNestedKey traverses a nested map using a dot-separated key path
// and returns the string value. Returns empty string if key not found or value is not a string.
func resolveNestedKey(data map[string]interface{}, key string) string {
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

// interpolatePlaceholders replaces {{ key }} placeholders in a translation value
// with the corresponding values from the args map.
func interpolatePlaceholders(value string, args map[string]interface{}) string {
	if args == nil || len(args) == 0 {
		return value
	}

	return translationPlaceholderRegex.ReplaceAllStringFunc(value, func(match string) string {
		submatch := translationPlaceholderRegex.FindStringSubmatch(match)
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
