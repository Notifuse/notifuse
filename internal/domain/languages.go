package domain

// DefaultLanguageCode is the fallback language used when none is specified.
const DefaultLanguageCode = "en"

// SupportedLanguages maps language codes to their display names.
// This is the curated list of languages available for workspace content.
var SupportedLanguages = map[string]string{
	"ar":    "Arabic",
	"ca":    "Catalan",
	"cs":    "Czech",
	"da":    "Danish",
	"de":    "German",
	"el":    "Greek",
	"en":    "English",
	"es":    "Spanish",
	"fi":    "Finnish",
	"fr":    "French",
	"he":    "Hebrew",
	"hi":    "Hindi",
	"hu":    "Hungarian",
	"id":    "Indonesian",
	"it":    "Italian",
	"ja":    "Japanese",
	"ko":    "Korean",
	"nl":    "Dutch",
	"nb":    "Norwegian Bokmal",
	"pl":    "Polish",
	"pt":    "Portuguese",
	"pt-BR": "Portuguese (Brazil)",
	"ro":    "Romanian",
	"ru":    "Russian",
	"sv":    "Swedish",
	"th":    "Thai",
	"tr":    "Turkish",
	"uk":    "Ukrainian",
	"vi":    "Vietnamese",
	"zh":    "Chinese",
	"zh-TW": "Chinese (Traditional)",
}

// IsValidLanguage checks if the given code is a supported language.
func IsValidLanguage(code string) bool {
	_, ok := SupportedLanguages[code]
	return ok
}

// SupportedUILanguages are the locales the console UI and the system emails
// (magic code, workspace invitation, circuit-breaker alert) are translated into.
// A user's language preference must be one of these. It is deliberately narrower
// than SupportedLanguages, which lists languages available for workspace content.
var SupportedUILanguages = map[string]string{
	"en":    "English",
	"fr":    "Français",
	"es":    "Español",
	"de":    "Deutsch",
	"ca":    "Català",
	"pt-BR": "Português (Brasil)",
	"ja":    "日本語",
	"it":    "Italiano",
}

// IsSupportedUILanguage checks if the code is a supported UI / system-email locale.
func IsSupportedUILanguage(code string) bool {
	_, ok := SupportedUILanguages[code]
	return ok
}

// ErrUnsupportedLanguage is returned when a language code is not a supported UI locale.
type ErrUnsupportedLanguage struct {
	Language string
}

func (e *ErrUnsupportedLanguage) Error() string {
	return "unsupported language: " + e.Language
}
