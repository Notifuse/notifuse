package totp

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rfcSecret is the SHA-1 seed of RFC 6238, Appendix B.
var rfcSecret = []byte("12345678901234567890")

// TestCodeMatchesRFC6238Vectors checks the SHA-1 test vectors of RFC 6238 Appendix B.
// The RFC prints 8-digit values; a 6-digit code is the same value modulo 10^6.
func TestCodeMatchesRFC6238Vectors(t *testing.T) {
	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},          // 94287082
		{1111111109, "081804"},  // 07081804
		{1111111111, "050471"},  // 14050471
		{1234567890, "005924"},  // 89005924
		{2000000000, "279037"},  // 69279037
		{20000000000, "353130"}, // 65353130
	}
	for _, tc := range cases {
		step := Step(time.Unix(tc.unix, 0))
		assert.Equal(t, tc.want, Code(rfcSecret, step), "unix time %d", tc.unix)
	}
}

func TestMatchAcceptsOneStepEitherSide(t *testing.T) {
	now := time.Unix(1234567890, 0)
	current := Step(now)

	for _, offset := range []int64{-1, 0, 1} {
		step, result := Match(rfcSecret, Code(rfcSecret, current+offset), now, 0)
		assert.Equal(t, MatchOK, result, "offset %d", offset)
		assert.Equal(t, current+offset, step, "offset %d", offset)
	}
	for _, offset := range []int64{-2, 2} {
		_, result := Match(rfcSecret, Code(rfcSecret, current+offset), now, 0)
		assert.Equal(t, MatchInvalid, result, "offset %d", offset)
	}
}

func TestMatchReportsAReplay(t *testing.T) {
	now := time.Unix(1234567890, 0)
	current := Step(now)
	code := Code(rfcSecret, current)

	step, result := Match(rfcSecret, code, now, current-1)
	require.Equal(t, MatchOK, result)
	require.Equal(t, current, step)

	// The same code once its step has been accepted.
	step, result = Match(rfcSecret, code, now, current)
	assert.Equal(t, MatchReplay, result)
	assert.Zero(t, step)

	// An earlier step than the last accepted one is a replay too.
	_, result = Match(rfcSecret, Code(rfcSecret, current-1), now, current)
	assert.Equal(t, MatchReplay, result)
}

func TestMatchAcceptsALaterStepAfterAnAcceptedOne(t *testing.T) {
	now := time.Unix(1234567890, 0)
	current := Step(now)

	step, result := Match(rfcSecret, Code(rfcSecret, current+1), now, current)
	assert.Equal(t, MatchOK, result)
	assert.Equal(t, current+1, step)
}

func TestMatchRefusesMalformedInput(t *testing.T) {
	now := time.Unix(1234567890, 0)
	valid := Code(rfcSecret, Step(now))

	_, result := Match(rfcSecret[:SecretSize-1], valid, now, 0)
	assert.Equal(t, MatchInvalid, result, "secret shorter than SecretSize")

	for _, code := range []string{"", "12345", "1234567", "12a456", " " + valid[1:], "١٢٣٤٥٦", "+12345"} {
		_, result := Match(rfcSecret, code, now, 0)
		assert.Equal(t, MatchInvalid, result, "code %q", code)
	}
}

func TestGenerateSecret(t *testing.T) {
	a, err := GenerateSecret()
	require.NoError(t, err)
	b, err := GenerateSecret()
	require.NoError(t, err)

	assert.Len(t, a, SecretSize)
	assert.NotEqual(t, a, b)
	assert.Len(t, EncodeSecret(a), 32, "20 bytes are 32 base32 characters without padding")
	assert.NotContains(t, EncodeSecret(a), "=")
}

func TestKeyURI(t *testing.T) {
	uri := KeyURI("Acme: Notifuse", "a+b/c@x.io", rfcSecret)

	parsed, err := url.Parse(uri)
	require.NoError(t, err)
	assert.Equal(t, "otpauth", parsed.Scheme)
	assert.Equal(t, "totp", parsed.Host)
	assert.Equal(t, "/Acme Notifuse:a+b/c@x.io", parsed.Path, "colons removed, label decodes back")

	query := parsed.Query()
	assert.Equal(t, "Acme Notifuse", query.Get("issuer"))
	assert.Equal(t, "SHA1", query.Get("algorithm"))
	assert.Equal(t, "6", query.Get("digits"))
	assert.Equal(t, "30", query.Get("period"))
	assert.Equal(t, EncodeSecret(rfcSecret), query.Get("secret"))

	assert.True(t, strings.HasSuffix(uri, "&secret="+EncodeSecret(rfcSecret)), "the secret comes last")
	assert.Contains(t, uri, "otpauth://totp/Acme%20Notifuse:a+b%2Fc@x.io?", "the account's slash is escaped inside the label")
	assert.Contains(t, uri, "?issuer=Acme%20Notifuse&", "a space in the issuer is %20, never +")

	plus := KeyURI("A+B Mail", "u@x.io", rfcSecret)
	assert.Contains(t, plus, "?issuer=A%2BB%20Mail&", "a real plus stays a plus")
	parsedPlus, err := url.Parse(plus)
	require.NoError(t, err)
	assert.Equal(t, "A+B Mail", parsedPlus.Query().Get("issuer"))
}
