package mailer

import (
	"reflect"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestGetTranslations_PerLocale(t *testing.T) {
	cases := []struct {
		lang            string
		expectedSubject string // MagicCode.Subject
	}{
		{"en", "Your Notifuse authentication code"},
		{"fr", "Votre code d'authentification Notifuse"},
		{"es", "Tu código de autenticación de Notifuse"},
		{"de", "Ihr Notifuse-Authentifizierungscode"},
		{"ca", "El teu codi d'autenticació de Notifuse"},
		{"pt-BR", "Seu código de autenticação da Notifuse"},
		{"ja", "Notifuse 認証コード"},
		{"it", "Il tuo codice di autenticazione Notifuse"},
	}

	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			got := GetTranslations(tc.lang).MagicCode.Subject
			if got != tc.expectedSubject {
				t.Errorf("GetTranslations(%q).MagicCode.Subject = %q, want %q", tc.lang, got, tc.expectedSubject)
			}
		})
	}
}

func TestGetTranslations_FallbackToEnglish(t *testing.T) {
	english := GetTranslations("en")
	for _, lang := range []string{"", "  ", "xx", "klingon", "zz-ZZ"} {
		t.Run(lang, func(t *testing.T) {
			if got := GetTranslations(lang); !reflect.DeepEqual(got, english) {
				t.Errorf("GetTranslations(%q) should fall back to English", lang)
			}
		})
	}
}

func TestGetTranslations_CaseInsensitive(t *testing.T) {
	want := GetTranslations("pt-br")
	for _, lang := range []string{"pt-BR", "PT-BR", "Pt-Br", "  pt-BR  "} {
		t.Run(lang, func(t *testing.T) {
			if got := GetTranslations(lang); !reflect.DeepEqual(got, want) {
				t.Errorf("GetTranslations(%q) should resolve to the pt-BR translation set", lang)
			}
		})
	}
}

func TestIsSupportedLanguage(t *testing.T) {
	supported := []string{"en", "fr", "es", "de", "ca", "pt-BR", "pt-br", "ja", "it", "  IT  "}
	for _, lang := range supported {
		if !IsSupportedLanguage(lang) {
			t.Errorf("IsSupportedLanguage(%q) = false, want true", lang)
		}
	}

	unsupported := []string{"", "xx", "klingon", "ru", "zh"}
	for _, lang := range unsupported {
		if IsSupportedLanguage(lang) {
			t.Errorf("IsSupportedLanguage(%q) = true, want false", lang)
		}
	}
}

// TestRegistryMatchesDomainUILanguages guards against drift between the mailer's
// translation registry and domain.SupportedUILanguages (used to validate the
// users.language column). Both must describe the same set of locales.
func TestRegistryMatchesDomainUILanguages(t *testing.T) {
	if len(systemEmailTranslations) != len(domain.SupportedUILanguages) {
		t.Fatalf("registry has %d locales, domain.SupportedUILanguages has %d",
			len(systemEmailTranslations), len(domain.SupportedUILanguages))
	}
	for code := range domain.SupportedUILanguages {
		if _, ok := systemEmailTranslations[normalizeLocale(code)]; !ok {
			t.Errorf("domain UI language %q has no translation set in the mailer registry", code)
		}
	}
}

// TestRegistryLangConsistency verifies each registry entry's canonical Lang field
// matches its (lowercased) map key, guarding against key/casing drift.
func TestRegistryLangConsistency(t *testing.T) {
	for key, set := range systemEmailTranslations {
		if set.Lang == "" {
			t.Errorf("locale %q has an empty Lang field", key)
			continue
		}
		if got := normalizeLocale(set.Lang); got != key {
			t.Errorf("registry key %q does not match normalized Lang %q (Lang=%q)", key, got, set.Lang)
		}
	}
}

func TestGetTranslations_LangField(t *testing.T) {
	cases := map[string]string{
		"en":    "en",
		"FR":    "fr",
		"pt-BR": "pt-BR", // canonical casing is preserved
		"  ja ": "ja",
		"xx":    DefaultEmailLanguage, // unsupported falls back to English
		"":      DefaultEmailLanguage,
	}
	for input, want := range cases {
		if got := GetTranslations(input).Lang; got != want {
			t.Errorf("GetTranslations(%q).Lang = %q, want %q", input, got, want)
		}
	}
}

// TestTranslations_Completeness ensures every registered locale provides a
// non-empty value for every translation string, guarding against missing
// translations being silently shipped.
func TestTranslations_Completeness(t *testing.T) {
	for lang, set := range systemEmailTranslations {
		t.Run(lang, func(t *testing.T) {
			assertNoEmptyStrings(t, lang, reflect.ValueOf(set))
		})
	}
}

func assertNoEmptyStrings(t *testing.T, lang string, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			value := v.Field(i)
			if value.Kind() == reflect.String {
				if value.String() == "" {
					t.Errorf("locale %q has an empty translation for field %q", lang, field.Name)
				}
			} else {
				assertNoEmptyStrings(t, lang, value)
			}
		}
	}
}
