package trackingtoken

import (
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKey(t *testing.T, secret string) []byte {
	t.Helper()
	key, err := DeriveKey(secret)
	require.NoError(t, err)
	return key
}

// reseal builds a token from a plaintext, as anyone can: the outer key is public.
func reseal(t *testing.T, plaintext string) string {
	t.Helper()
	token, err := crypto.EncryptTrackingToken(plaintext)
	require.NoError(t, err)
	return token
}

func open(t *testing.T, token string) string {
	t.Helper()
	plaintext, err := crypto.DecryptTrackingToken(token)
	require.NoError(t, err)
	return plaintext
}

func TestMintClick_RoundTrip(t *testing.T) {
	key := testKey(t, "workspace-secret")
	urls := []string{
		"https://shop.com/x?a=1&b=2",
		"https://shop.com/line\nbreak",
		"https://shop.com/nul\x00byte",
		"https://shop.com/50%off",
		"//shop.com/x",
		"myapp://x",
		"",
	}
	for _, u := range urls {
		token, err := MintClick(key, "msg-1", "ws-1", 1730000000, u)
		require.NoError(t, err)

		parsed, err := Parse(token, PurposeClick)
		require.NoError(t, err)
		assert.Equal(t, "msg-1", parsed.MessageID)
		assert.Equal(t, "ws-1", parsed.WorkspaceID)
		assert.Equal(t, int64(1730000000), parsed.Timestamp)
		assert.Equal(t, u, parsed.URL, "the destination must survive byte for byte")
		assert.NotEmpty(t, parsed.Tag)
		assert.True(t, parsed.Verify(key, PurposeClick), "url %q", u)
	}
}

func TestMintOpen_RoundTrip(t *testing.T) {
	key := testKey(t, "workspace-secret")
	token, err := MintOpen(key, "msg-1", "ws-1", 1730000000)
	require.NoError(t, err)

	parsed, err := Parse(token, PurposeOpen)
	require.NoError(t, err)
	assert.Equal(t, "msg-1", parsed.MessageID)
	assert.Equal(t, "ws-1", parsed.WorkspaceID)
	assert.Equal(t, int64(1730000000), parsed.Timestamp)
	assert.Empty(t, parsed.URL)
	assert.True(t, parsed.Verify(key, PurposeOpen))
}

func TestVerify_PurposeIsBound(t *testing.T) {
	key := testKey(t, "workspace-secret")

	click, err := MintClick(key, "msg-1", "ws-1", 1730000000, "")
	require.NoError(t, err)
	parsedClick, err := Parse(click, PurposeClick)
	require.NoError(t, err)
	assert.False(t, parsedClick.Verify(key, PurposeOpen), "a click tag must not verify as an open")

	openToken, err := MintOpen(key, "msg-1", "ws-1", 1730000000)
	require.NoError(t, err)
	// An open token re-sealed with an empty destination field has the click shape.
	asClick, err := Parse(reseal(t, open(t, openToken)+"\n"), PurposeClick)
	require.NoError(t, err)
	assert.False(t, asClick.Verify(key, PurposeClick), "an open tag must not verify as a click")
}

func TestVerify_RejectsOtherKeysAndChangedFields(t *testing.T) {
	key := testKey(t, "workspace-secret")
	token, err := MintClick(key, "msg-1", "ws-1", 1730000000, "https://shop.com/x")
	require.NoError(t, err)

	parsed, err := Parse(token, PurposeClick)
	require.NoError(t, err)
	assert.False(t, parsed.Verify(testKey(t, "other-secret"), PurposeClick))
	assert.False(t, parsed.Verify(nil, PurposeClick))

	parts := strings.SplitN(open(t, token), "\n", 4)
	_, tag, found := strings.Cut(parts[2], ".")
	require.True(t, found)

	changed := map[string]string{
		"message id":  "msg-2\n" + parts[1] + "\n" + parts[2] + "\n" + parts[3],
		"workspace":   parts[0] + "\nws-2\n" + parts[2] + "\n" + parts[3],
		"timestamp":   parts[0] + "\n" + parts[1] + "\n1730000001." + tag + "\n" + parts[3],
		"destination": parts[0] + "\n" + parts[1] + "\n" + parts[2] + "\nhttps://evil.example/x",
	}
	for name, plaintext := range changed {
		forged, err := Parse(reseal(t, plaintext), PurposeClick)
		require.NoError(t, err, name)
		assert.Equal(t, tag, forged.Tag, name)
		assert.False(t, forged.Verify(key, PurposeClick), "changing the %s must break the tag", name)
	}
}

func TestMint_WithoutKeyIsUnsigned(t *testing.T) {
	click, err := MintClick(nil, "msg-1", "ws-1", 1730000000, "https://shop.com/x")
	require.NoError(t, err)
	assert.Equal(t, "msg-1\nws-1\n1730000000\nhttps://shop.com/x", open(t, click), "unsigned tokens keep the legacy shape")
	parsed, err := Parse(click, PurposeClick)
	require.NoError(t, err)
	assert.Empty(t, parsed.Tag)
	assert.False(t, parsed.Verify(testKey(t, "workspace-secret"), PurposeClick))
	assert.False(t, parsed.Verify(nil, PurposeClick))

	openToken, err := MintOpen([]byte{}, "msg-1", "ws-1", 1730000000)
	require.NoError(t, err)
	assert.Equal(t, "msg-1\nws-1\n1730000000", open(t, openToken))
}

func TestParse_LegacyTokens(t *testing.T) {
	click, err := Parse(reseal(t, "msg-1\nws-1\n1730000000\nhttps://shop.com/x"), PurposeClick)
	require.NoError(t, err)
	assert.Equal(t, Token{MessageID: "msg-1", WorkspaceID: "ws-1", Timestamp: 1730000000, URL: "https://shop.com/x", rawTimestamp: "1730000000"}, click)

	openToken, err := Parse(reseal(t, "msg-1\nws-1\n1730000000"), PurposeOpen)
	require.NoError(t, err)
	assert.Equal(t, "msg-1", openToken.MessageID)
	assert.Equal(t, "ws-1", openToken.WorkspaceID)
	assert.Empty(t, openToken.Tag)
}

// TestMint_ReadableByPreviousRelease pins the field order 42.1 reads: it splits /r/
// tokens with SplitN(…, 4) and /t/ tokens with SplitN(…, 3), and must still find the
// message, workspace and destination where they were during a rolling upgrade.
func TestMint_ReadableByPreviousRelease(t *testing.T) {
	key := testKey(t, "workspace-secret")

	click, err := MintClick(key, "msg-1", "ws-1", 1730000000, "https://shop.com/a\nb")
	require.NoError(t, err)
	parts := strings.SplitN(open(t, click), "\n", 4)
	require.Len(t, parts, 4)
	assert.Equal(t, "msg-1", parts[0])
	assert.Equal(t, "ws-1", parts[1])
	assert.Equal(t, "https://shop.com/a\nb", parts[3])

	openToken, err := MintOpen(key, "msg-1", "ws-1", 1730000000)
	require.NoError(t, err)
	parts = strings.SplitN(open(t, openToken), "\n", 3)
	require.Len(t, parts, 3)
	assert.Equal(t, "msg-1", parts[0])
	assert.Equal(t, "ws-1", parts[1])
}

func TestParse_Timestamp(t *testing.T) {
	cases := map[string]int64{
		"1730000000.tag": 1730000000,
		"1730000000":     1730000000,
		"":               0,
		"abc":            0,
		"abc.tag":        0,
	}
	for field, want := range cases {
		parsed, err := Parse(reseal(t, "msg-1\nws-1\n"+field), PurposeOpen)
		require.NoError(t, err, field)
		assert.Equal(t, want, parsed.Timestamp, field)
	}
}

func TestParse_Errors(t *testing.T) {
	_, err := Parse("", PurposeClick)
	assert.Error(t, err)
	_, err = Parse("not-a-token", PurposeClick)
	assert.Error(t, err)
	_, err = Parse(reseal(t, "msg-1\nws-1\n1730000000"), PurposeClick)
	assert.Error(t, err, "a click token needs four fields")
	_, err = Parse(reseal(t, "msg-1\nws-1"), PurposeOpen)
	assert.Error(t, err, "an open token needs three fields")
}

func TestDeriveKey(t *testing.T) {
	a := testKey(t, "secret-a")
	assert.Len(t, a, 32)
	assert.Equal(t, a, testKey(t, "secret-a"), "deterministic")
	assert.NotEqual(t, a, testKey(t, "secret-b"))

	key, err := DeriveKey("")
	assert.Error(t, err)
	assert.Nil(t, key)
}

func TestSchemeOf(t *testing.T) {
	cases := map[string]string{
		"JavaScript:x":            "javascript",
		" javascript:x":           "javascript",
		"\tjava\nscript:x":        "javascript",
		"\x01data:text/html,x":    "data",
		"https://shop.com/50%off": "https",
		"myapp://open":            "myapp",
		"ms-msdt:/id x":           "ms-msdt",
		"//x":                     "",
		"x":                       "",
		":x":                      "",
		"1http://x":               "",
		"/pricing:x":              "",
		"":                        "",
	}
	for raw, want := range cases {
		assert.Equal(t, want, SchemeOf(raw), "%q", raw)
	}
}

func TestPreprocess(t *testing.T) {
	assert.Equal(t, "https://brand.com/sale", Preprocess(" https://brand.com/sale "))
	assert.Equal(t, "https://brand.com/sale", Preprocess("https://brand.com/sa\r\nle\t"))
	assert.Equal(t, "", Preprocess(" \x00\x1f "))
}

func TestWorkspaceStatus_ZeroValueIsUnknown(t *testing.T) {
	var status WorkspaceStatus
	assert.Equal(t, WorkspaceUnknown, status, "a status never set must fail closed")
}

func TestValidWorkspaceID(t *testing.T) {
	for _, id := range []string{"ws1", "test4a2b9c1d", "ws_1", "ws-1", strings.Repeat("a", 63)} {
		assert.True(t, ValidWorkspaceID(id), id)
	}
	for _, id := range []string{"", strings.Repeat("a", 64), "ws 1", "ws/1", "ws.1", "wś1", "ws1\n"} {
		assert.False(t, ValidWorkspaceID(id), "%q", id)
	}
}

func TestHostOf(t *testing.T) {
	cases := map[string]string{
		"https://Track.Brand.com:8443/base": "track.brand.com",
		"https://api.example.com":           "api.example.com",
		"api.example.com":                   "api.example.com",
		"API.example.com:8080":              "api.example.com",
		" http://localhost:4000/ ":          "localhost",
		"":                                  "",
		"https://[::1":                      "",
	}
	for raw, want := range cases {
		assert.Equal(t, want, HostOf(raw), "%q", raw)
	}
}
