package branding

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type urlFixtures struct {
	Valid   []string `json:"valid"`
	Invalid []string `json:"invalid"`
}

func loadURLFixtures(t *testing.T) urlFixtures {
	t.Helper()
	data, err := os.ReadFile("testdata/urls.json")
	require.NoError(t, err)
	var f urlFixtures
	require.NoError(t, json.Unmarshal(data, &f))
	require.NotEmpty(t, f.Valid)
	require.NotEmpty(t, f.Invalid)
	return f
}

func TestValidateURL_SharedFixtures(t *testing.T) {
	f := loadURLFixtures(t)
	for _, u := range f.Valid {
		t.Run("valid "+u, func(t *testing.T) {
			assert.NoError(t, ValidateURL(u))
		})
	}
	for _, u := range f.Invalid {
		t.Run("invalid "+u, func(t *testing.T) {
			assert.Error(t, ValidateURL(u))
		})
	}
}

func TestValidateURL_Empty(t *testing.T) {
	assert.NoError(t, ValidateURL(""))
}

func TestValidateURL_LengthBound(t *testing.T) {
	prefix := "https://cdn.example.com/"
	atLimit := prefix + strings.Repeat("a", MaxURLBytes-len(prefix))
	require.Len(t, atLimit, MaxURLBytes)
	assert.NoError(t, ValidateURL(atLimit))
	assert.Error(t, ValidateURL(atLimit+"a"))
}

func TestValidateURL_ControlCharacters(t *testing.T) {
	assert.Error(t, ValidateURL("https://cdn.example.com/logo\x00.png"))
	assert.Error(t, ValidateURL("https://cdn.example.com/logo\t.png"))
	assert.Error(t, ValidateURL("https://cdn.example.com/logo\u0085.png"))
}

func TestValidateName(t *testing.T) {
	assert.NoError(t, ValidateName(""))
	assert.NoError(t, ValidateName("Acme"))
	assert.NoError(t, ValidateName(strings.Repeat("é", MaxNameCodePoints)), "counts code points, not bytes")
	assert.Error(t, ValidateName(strings.Repeat("a", MaxNameCodePoints+1)))
	assert.Error(t, ValidateName("Acme\nLabs"))
	assert.Error(t, ValidateName("Acme\u0007"))
	assert.Error(t, ValidateName(string([]byte{0xff, 0xfe})))
	assert.NoError(t, ValidateName("A & B <Labs>"), "markup characters are plain text and allowed")
	assert.Error(t, ValidateName("Acme\u202Egnp.exe"), "a right-to-left override reorders the text")
	assert.Error(t, ValidateName("Ac\u200Bme"), "a zero-width space hides in the name")
	assert.Error(t, ValidateName("Acme\u2066"), "a bidirectional isolate")
	assert.NoError(t, ValidateName("👩\u200D💻 Labs"), "the zero-width joiner builds emoji")
}

func TestNormalize(t *testing.T) {
	b := Branding{Name: "  Acme  ", LogoURL: " https://cdn.example.com/l.png\n", IconURL: "   ", HideHelpMenu: true}.Normalize()
	assert.Equal(t, Branding{Name: "Acme", LogoURL: "https://cdn.example.com/l.png", IconURL: "", HideHelpMenu: true}, b)
}

func TestValidate_NameRequiredWithAnImage(t *testing.T) {
	var fe *FieldError

	err := Branding{LogoURL: "https://cdn.example.com/l.png"}.Validate()
	require.ErrorAs(t, err, &fe)
	assert.Equal(t, FieldName, fe.Field)

	err = Branding{IconURL: "https://cdn.example.com/i.png"}.Validate()
	require.ErrorAs(t, err, &fe)
	assert.Equal(t, FieldName, fe.Field)

	assert.NoError(t, Branding{Name: "Acme", LogoURL: "https://cdn.example.com/l.png"}.Validate())
	assert.NoError(t, Branding{HideHelpMenu: true}.Validate(), "the help switch alone needs no name")
}

func TestValidate_ReportsTheFailingField(t *testing.T) {
	var fe *FieldError

	require.ErrorAs(t, Branding{Name: "Acme", LogoURL: "http://cdn.example.com/l.png"}.Validate(), &fe)
	assert.Equal(t, FieldLogoURL, fe.Field)

	require.ErrorAs(t, Branding{Name: "Acme", IconURL: "https://localhost/i.png"}.Validate(), &fe)
	assert.Equal(t, FieldIconURL, fe.Field)

	require.ErrorAs(t, Branding{Name: strings.Repeat("a", MaxNameCodePoints+1)}.Validate(), &fe)
	assert.Equal(t, FieldName, fe.Field)
}

func TestIsCustom(t *testing.T) {
	assert.False(t, Branding{}.IsCustom())
	assert.False(t, Branding{HideHelpMenu: true}.IsCustom(), "hiding the Help menu is not a custom identity")
	assert.False(t, Branding{Name: "   "}.IsCustom())
	assert.True(t, Branding{Name: "Acme"}.IsCustom())
	assert.True(t, Branding{LogoURL: "https://cdn.example.com/l.png"}.IsCustom())
	assert.True(t, Branding{IconURL: "https://cdn.example.com/i.png"}.IsCustom())
}

func TestResolve_TheFiveValidCombinations(t *testing.T) {
	logo := "https://cdn.example.com/logo.png"
	icon := "https://cdn.example.com/icon.png"

	tests := []struct {
		name string
		in   Branding
		want Resolved
	}{
		{
			name: "nothing set",
			in:   Branding{},
			want: Resolved{Custom: false, Name: DefaultName, SplashLogoSrc: DefaultLogoPath, FaviconSrc: DefaultFaviconPath},
		},
		{
			name: "name only",
			in:   Branding{Name: "Acme"},
			want: Resolved{Custom: true, Name: "Acme", SplashLogoSrc: "", FaviconSrc: NeutralIconPath},
		},
		{
			name: "name and logo",
			in:   Branding{Name: "Acme", LogoURL: logo},
			want: Resolved{Custom: true, Name: "Acme", LogoURL: logo, SplashLogoSrc: logo, FaviconSrc: NeutralIconPath},
		},
		{
			name: "name and icon",
			in:   Branding{Name: "Acme", IconURL: icon},
			want: Resolved{Custom: true, Name: "Acme", IconURL: icon, SplashLogoSrc: "", FaviconSrc: NeutralIconPath},
		},
		{
			name: "name, logo and icon",
			in:   Branding{Name: "Acme", LogoURL: logo, IconURL: icon},
			want: Resolved{Custom: true, Name: "Acme", LogoURL: logo, IconURL: icon, SplashLogoSrc: logo, FaviconSrc: NeutralIconPath},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.in.Validate())
			got := Resolve(tt.in)
			tt.want.Revision = tt.in.Revision()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolve_HideHelpMenuIsIndependent(t *testing.T) {
	r := Resolve(Branding{HideHelpMenu: true})
	assert.False(t, r.Custom)
	assert.True(t, r.HideHelpMenu)
	assert.Equal(t, DefaultName, r.Name)
}

func TestRevision(t *testing.T) {
	base := Branding{Name: "Acme", LogoURL: "https://cdn.example.com/l.png"}
	assert.Equal(t, base.Revision(), base.Revision(), "stable")
	assert.Equal(t, base.Revision(), Branding{Name: " Acme ", LogoURL: "https://cdn.example.com/l.png "}.Revision(), "computed on normalised values")
	assert.Len(t, base.Revision(), 64)

	variants := []Branding{
		{Name: "Acme2", LogoURL: base.LogoURL},
		{Name: base.Name, LogoURL: "https://cdn.example.com/m.png"},
		{Name: base.Name, LogoURL: base.LogoURL, IconURL: "https://cdn.example.com/i.png"},
		{Name: base.Name, LogoURL: base.LogoURL, HideHelpMenu: true},
	}
	for _, v := range variants {
		assert.NotEqual(t, base.Revision(), v.Revision(), "%+v", v)
	}
}

func TestEncodeDecode_RoundTrip(t *testing.T) {
	in := Branding{Name: " Acme ", LogoURL: "https://cdn.example.com/l.png", IconURL: "", HideHelpMenu: true}
	data, err := in.Encode()
	require.NoError(t, err)

	out, err := Decode(data)
	require.NoError(t, err)
	assert.Equal(t, in.Normalize(), out)
}

func TestEncodeDecode_Reset(t *testing.T) {
	data, err := Branding{}.Encode()
	require.NoError(t, err)
	out, err := Decode(data)
	require.NoError(t, err)
	assert.Equal(t, Branding{}, out)
	assert.False(t, out.IsCustom())
}

func TestDecode_OnlyChecksTheJSON(t *testing.T) {
	_, err := Decode([]byte("not json"))
	assert.True(t, errors.Is(err, ErrInvalidStored))

	// Values the API would refuse today still read: they may have been saved under older rules.
	got, err := Decode([]byte(`{"name":"","logo_url":"https://10.0.0.1/l.png"}`))
	require.NoError(t, err)
	assert.Equal(t, Branding{LogoURL: "https://10.0.0.1/l.png"}, got)
}

func TestValidateChanged(t *testing.T) {
	stored := Branding{Name: "Acme", LogoURL: "https://10.0.0.1/l.png"} // valid once, refused today

	assert.NoError(t, stored.ValidateChanged(stored), "the stored brand sent back unchanged")
	assert.NoError(t, Branding{Name: "Acme", LogoURL: "https://10.0.0.1/l.png", HideHelpMenu: true}.ValidateChanged(stored),
		"changing another field does not re-check the untouched logo")
	assert.NoError(t, Branding{Name: " Acme ", LogoURL: "https://10.0.0.1/l.png "}.ValidateChanged(stored), "compared normalised")

	var fe *FieldError
	require.ErrorAs(t, Branding{Name: "Acme", LogoURL: "https://192.168.0.1/m.png"}.ValidateChanged(stored), &fe)
	assert.Equal(t, FieldLogoURL, fe.Field, "a changed URL is checked")
	require.ErrorAs(t, Branding{Name: "", LogoURL: "https://10.0.0.1/l.png"}.ValidateChanged(stored), &fe)
	assert.Equal(t, FieldName, fe.Field, "removing the name while a logo stays is refused")
	assert.NoError(t, Branding{}.ValidateChanged(stored), "clearing everything is always valid")
}

func TestResolve_StoredValuesThatBypassedTheAPI(t *testing.T) {
	r := Resolve(Branding{Name: "Acme", LogoURL: "http://cdn.example.com/l.png", IconURL: "javascript:alert(1)"})
	assert.Empty(t, r.LogoURL, "an http URL is not shown")
	assert.Empty(t, r.IconURL)
	assert.Empty(t, r.SplashLogoSrc)

	r = Resolve(Branding{LogoURL: "https://cdn.example.com/l.png"})
	assert.True(t, r.Custom)
	assert.Equal(t, DefaultName, r.Name, "a custom identity without a name shows the default name")
}

func TestDecode_IgnoresUnknownKeys(t *testing.T) {
	out, err := Decode([]byte(`{"name":"Acme","primary_color":"#123456"}`))
	require.NoError(t, err)
	assert.Equal(t, Branding{Name: "Acme"}, out)
}
