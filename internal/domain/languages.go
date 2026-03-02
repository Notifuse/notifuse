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
