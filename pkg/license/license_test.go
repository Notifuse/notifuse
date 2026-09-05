package license

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// devSigningKeyPath is relative to this package's directory, which is the
// working directory of the test binary.
const devSigningKeyPath = "testdata/dev_signing_key.json"

// devPubKeySourcePath is read as text, not compiled: the constant it holds only
// exists under -tags licdev, and the whole point of the check is to catch the
// pair drifting apart in a build where it is not compiled at all.
const devPubKeySourcePath = "pubkey_dev.go"

var devSlotActiveRE = regexp.MustCompile(`slotActiveEncoded\s*=\s*"([^"]*)"`)

type devKeyFixture struct {
	Public  string `json:"public"`
	Private string `json:"private"`
}

// devKeyPair loads the committed dev key pair. It is a fixture, not a secret:
// see the warning in the file itself.
func devKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()

	raw, err := os.ReadFile(devSigningKeyPath)
	require.NoError(t, err, "the dev key fixture is missing; regenerate it with `go run ./licensegen keygen` from the cloud repository and update pubkey_dev.go, rather than deleting the tests that depend on it")

	var fixture devKeyFixture
	require.NoError(t, json.Unmarshal(raw, &fixture))

	pub, err := base64.StdEncoding.DecodeString(fixture.Public)
	require.NoError(t, err)
	priv, err := base64.StdEncoding.DecodeString(fixture.Private)
	require.NoError(t, err)
	require.Len(t, priv, ed25519.PrivateKeySize)

	return ed25519.PublicKey(pub), ed25519.PrivateKey(priv)
}

// strangerKeyPair is a key the verifier has never heard of — a forger, or a
// signing authority from another deployment.
func strangerKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv
}

func validClaims() *Claims {
	return &Claims{
		V:     SchemaVersion,
		LID:   "lic_7f3a9c21",
		Org:   "ACME SAS",
		Sub:   "billing@acme.com",
		Tier:  "agency",
		Feat:  []string{"rbac", "ses_tenant"},
		MaxWS: 15,
		IAT:   1772582400,
		Exp:   1804118400,
	}
}

func mustMint(t *testing.T, priv ed25519.PrivateKey, claims *Claims) string {
	t.Helper()
	raw, err := Mint(priv, claims)
	require.NoError(t, err)
	return raw
}

// envelope signs arbitrary payload bytes, which Mint cannot do because it only
// ever emits well-formed JSON. It is needed to prove that the signature is
// checked before the payload is interpreted.
func envelope(t *testing.T, priv ed25519.PrivateKey, payload []byte) string {
	t.Helper()
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	sig := ed25519.Sign(priv, signedBytes(encoded))
	return envelopePrefix + "." + encoded + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// flipByte changes one character of a string to a different one from the
// base64url alphabet, so the result is still decodable and only the bytes it
// carries have changed.
func flipByte(t *testing.T, s string, i int) string {
	t.Helper()
	require.Greater(t, len(s), i)
	b := []byte(s)
	if b[i] == 'A' {
		b[i] = 'B'
	} else {
		b[i] = 'A'
	}
	return string(b)
}

// referenceTime is a fixed clock, so that no test depends on when it is run.
var referenceTime = time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

func TestParse(t *testing.T) {
	devPub, devPriv := devKeyPair(t)
	_, strangerPriv := strangerKeyPair(t)
	keys := [][]byte{devPub}

	valid := mustMint(t, devPriv, validClaims())

	t.Run("a key signed by a trusted slot round-trips to its claims", func(t *testing.T) {
		claims, err := parseAt(valid, keys, referenceTime)
		require.NoError(t, err)
		assert.Equal(t, validClaims(), claims)
	})

	t.Run("expiry is not a parse failure", func(t *testing.T) {
		// A lapsed key still names its licensee, and the console needs that to
		// tell "your key ran out" apart from "this installation never had one".
		// Whether exp has passed is a state the caller derives, not an error here.
		lapsed := validClaims()
		lapsed.Exp = referenceTime.Add(-365 * 24 * time.Hour).Unix()

		claims, err := parseAt(mustMint(t, devPriv, lapsed), keys, referenceTime)
		require.NoError(t, err)
		assert.Equal(t, lapsed.Exp, claims.Exp)
	})

	t.Run("the signature is checked before the payload is interpreted", func(t *testing.T) {
		// Verifying first is what keeps this package from ever unmarshalling
		// JSON an attacker chose.
		garbage := []byte("this is not json at all")

		_, err := parseAt(envelope(t, strangerPriv, garbage), keys, referenceTime)
		require.ErrorIs(t, err, ErrBadSignature)

		_, err = parseAt(envelope(t, devPriv, garbage), keys, referenceTime)
		require.ErrorIs(t, err, ErrMalformedPayload)
	})

	segments := strings.Split(valid, ".")
	require.Len(t, segments, 3, "the envelope is three dot-separated segments")

	testCases := []struct {
		name    string
		raw     string
		wantErr error
	}{
		{
			name:    "an empty string is not an envelope",
			raw:     "",
			wantErr: ErrMalformedEnvelope,
		},
		{
			name:    "a string with no separators is not an envelope",
			raw:     "NFUSE1",
			wantErr: ErrMalformedEnvelope,
		},
		{
			name:    "two segments are not an envelope",
			raw:     segments[0] + "." + segments[1],
			wantErr: ErrMalformedEnvelope,
		},
		{
			name: "a fourth segment is refused rather than ignored",
			// Trailing data is a different format, not a licence key with
			// something appended; guessing is how a parser grows a bug that only
			// fires on someone else's input.
			raw:     valid + ".extra",
			wantErr: ErrMalformedEnvelope,
		},
		{
			name:    "a different envelope prefix is refused",
			raw:     "NFUSE2." + segments[1] + "." + segments[2],
			wantErr: ErrMalformedEnvelope,
		},
		{
			name:    "the prefix is case sensitive",
			raw:     "nfuse1." + segments[1] + "." + segments[2],
			wantErr: ErrMalformedEnvelope,
		},
		{
			name:    "a payload segment that is not base64url is refused",
			raw:     "NFUSE1.not base64!." + segments[2],
			wantErr: ErrBadEncoding,
		},
		{
			name:    "a signature segment that is not base64url is refused",
			raw:     "NFUSE1." + segments[1] + ".not base64!",
			wantErr: ErrBadEncoding,
		},
		{
			name:    "padded base64 is refused, the wire format is unpadded",
			raw:     "NFUSE1." + segments[1] + "=." + segments[2],
			wantErr: ErrBadEncoding,
		},
		{
			name: "a tampered payload no longer verifies",
			// The single most important rejection: editing max_ws or feat in a
			// real key and re-encoding it must not produce a usable licence.
			raw:     "NFUSE1." + flipByte(t, segments[1], 10) + "." + segments[2],
			wantErr: ErrBadSignature,
		},
		{
			name:    "a tampered signature no longer verifies",
			raw:     "NFUSE1." + segments[1] + "." + flipByte(t, segments[2], 10),
			wantErr: ErrBadSignature,
		},
		{
			name:    "a truncated signature no longer verifies",
			raw:     "NFUSE1." + segments[1] + "." + segments[2][:20],
			wantErr: ErrBadSignature,
		},
		{
			name: "empty segments verify against nothing",
			// Valid base64 for zero bytes, so this gets past decoding and is
			// refused where it should be: at the signature.
			raw:     "NFUSE1..",
			wantErr: ErrBadSignature,
		},
		{
			name:    "a key signed by another authority is refused",
			raw:     mustMint(t, strangerPriv, validClaims()),
			wantErr: ErrBadSignature,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := parseAt(tc.raw, keys, referenceTime)
			assert.Nil(t, claims)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestParseSchemaVersion(t *testing.T) {
	devPub, devPriv := devKeyPair(t)
	keys := [][]byte{devPub}

	// A key minted in 2028 against a schema this binary has never seen must not
	// be read with today's field meanings — that is how a capability nobody sold
	// gets granted — and a v0 key means a field the mint forgot to set.
	for _, version := range []int{0, 2, -1} {
		claims := validClaims()
		claims.V = version

		got, err := parseAt(mustMint(t, devPriv, claims), keys, referenceTime)
		assert.Nil(t, got)
		require.ErrorIs(t, err, ErrUnknownVersion)
	}
}

func TestParseClockSkew(t *testing.T) {
	devPub, devPriv := devKeyPair(t)
	keys := [][]byte{devPub}

	minted := func(t *testing.T, offset time.Duration) (*Claims, error) {
		t.Helper()
		claims := validClaims()
		claims.IAT = referenceTime.Add(offset).Unix()
		return parseAt(mustMint(t, devPriv, claims), keys, referenceTime)
	}

	t.Run("a few minutes of drift is tolerated", func(t *testing.T) {
		// The signing machine and the customer's server are two different
		// clocks. Refusing a key on the day it was bought is a support ticket,
		// not a defence, so this bound is a tolerance and not a comparison.
		claims, err := minted(t, 4*time.Minute)
		require.NoError(t, err)
		assert.Equal(t, referenceTime.Add(4*time.Minute).Unix(), claims.IAT)
	})

	t.Run("a key issued well into the future is refused", func(t *testing.T) {
		claims, err := minted(t, 10*time.Minute)
		assert.Nil(t, claims)
		require.ErrorIs(t, err, ErrFutureIssuedAt)
	})

	t.Run("a key issued in the past is always fine", func(t *testing.T) {
		// There is no high-water mark and no persisted clock state anywhere:
		// restoring a production snapshot into staging must not degrade a paying
		// customer's instance for a reason nobody can diagnose.
		claims, err := minted(t, -10*365*24*time.Hour)
		require.NoError(t, err)
		assert.NotNil(t, claims)
	})
}

func TestParseKeyRotation(t *testing.T) {
	devPub, devPriv := devKeyPair(t)
	nextPub, nextPriv := strangerKeyPair(t)

	slots := [][]byte{devPub, nextPub}

	t.Run("a key signed by the active slot verifies", func(t *testing.T) {
		_, err := parseAt(mustMint(t, devPriv, validClaims()), slots, referenceTime)
		require.NoError(t, err)
	})

	t.Run("a key signed by the second slot verifies too", func(t *testing.T) {
		// This is the whole reason the second slot exists: during a rotation,
		// keys minted under either authority must both be accepted, or the
		// rotation rejects every licence already in a customer's hands.
		_, err := parseAt(mustMint(t, nextPriv, validClaims()), slots, referenceTime)
		require.NoError(t, err)
	})

	t.Run("an unusable slot beside a good one is skipped, not fatal", func(t *testing.T) {
		// ed25519.Verify panics on a public key of the wrong length, so a
		// half-decoded or mistyped slot would otherwise take a customer's
		// process down at the exact moment their licence is first checked.
		ragged := [][]byte{nil, []byte("too short"), devPub}

		require.NotPanics(t, func() {
			_, err := parseAt(mustMint(t, devPriv, validClaims()), ragged, referenceTime)
			require.NoError(t, err)
		})
	})
}

func TestParseWithoutAUsableKey(t *testing.T) {
	_, devPriv := devKeyPair(t)
	raw := mustMint(t, devPriv, validClaims())

	testCases := []struct {
		name string
		keys [][]byte
	}{
		{
			name: "the committed placeholder is not a key",
			// pubkey_prod.go ships thirty-two zero bytes until a release
			// replaces them. A build that still carries the placeholder must say
			// so with one unmistakable error, not accept keys it cannot verify.
			keys: [][]byte{make([]byte, ed25519.PublicKeySize), nil},
		},
		{
			name: "no slots at all",
			keys: nil,
		},
		{
			name: "only slots of the wrong length",
			keys: [][]byte{[]byte("nope"), {}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := parseAt(raw, tc.keys, referenceTime)
			assert.Nil(t, claims)
			require.ErrorIs(t, err, ErrNoTrustedKey)
		})
	}
}

// TestParseUsesTheCompiledSlots exercises the exported Parse, and therefore the
// keys this particular build actually carries. It adapts to the build tag on
// purpose: pinning "the placeholder is present" would turn the release commit
// that replaces it into a failing test, and pinning "the dev key is present"
// would fail on every ordinary build.
func TestParseUsesTheCompiledSlots(t *testing.T) {
	devPub, devPriv := devKeyPair(t)

	claims := validClaims()
	claims.IAT = time.Now().Add(-time.Hour).Unix()
	raw := mustMint(t, devPriv, claims)

	usable := usableKeys(trustedKeys)
	devCompiled := false
	for _, key := range usable {
		if bytes.Equal(key, devPub) {
			devCompiled = true
		}
	}

	got, err := Parse(raw)

	switch {
	case len(usable) == 0:
		require.ErrorIs(t, err, ErrNoTrustedKey,
			"this build carries no usable signing key, so every licence must be refused with that one error")
	case devCompiled:
		require.NoError(t, err, "a -tags licdev build must accept a key minted with the committed dev key")
		assert.Equal(t, claims.LID, got.LID)
	default:
		require.ErrorIs(t, err, ErrBadSignature,
			"the dev signing key is committed in this repository; a build that trusts it would hand every reader a licence")
	}
}

// TestDevPublicKeyMatchesItsPrivateHalf reads pubkey_dev.go as text because the
// constant it declares is only compiled under -tags licdev, and a pair that
// drifted apart would make every dev build reject every dev key with a signature
// error that reads like a bug in the verifier.
func TestDevPublicKeyMatchesItsPrivateHalf(t *testing.T) {
	fixturePub, priv := devKeyPair(t)

	derived, ok := priv.Public().(ed25519.PublicKey)
	require.True(t, ok)
	assert.True(t, bytes.Equal(derived, fixturePub),
		"the public half recorded in the fixture is not the one this private key produces")

	source, err := os.ReadFile(devPubKeySourcePath)
	require.NoError(t, err, "pubkey_dev.go is missing; if it moved, this check must follow it rather than be deleted")

	match := devSlotActiveRE.FindSubmatch(source)
	require.NotNil(t, match, "slotActiveEncoded is no longer a plain string constant in %s — update this parser, do not stop checking", devPubKeySourcePath)

	compiled, err := base64.StdEncoding.DecodeString(string(match[1]))
	require.NoError(t, err)
	assert.True(t, bytes.Equal(compiled, fixturePub),
		"pubkey_dev.go and the testdata private key are no longer a pair")
}

// goldenCase is one pinned envelope from the cross-implementation fixture.
//
// The same three cases are pinned in the billing service, at
// cloud/billing-api/src/licences/licence-golden.fixture.ts, under the same test
// key. Two independent implementations mint these envelopes — that service, and
// Mint below, which is compiled into the product a customer runs — and they must
// agree byte for byte forever. A divergence means either that every key we issue
// is rejected by every installation, or that a working key stops verifying after
// an upgrade, and neither failure is visible until it has already happened to
// somebody. Pinning both sides is what turns such a drift into two red tests
// instead of a silent outage.
type goldenCase struct {
	name     string
	claims   *Claims
	envelope string
}

// goldenSigningKeyBase64 is the 64-byte Go form (seed || public key) of the test
// key the TypeScript fixture calls GOLDEN_PRIVATE_KEY_GO_BASE64.
//
// THIS KEY IS PUBLIC AND EXISTS ONLY FOR TESTS. It has never signed a customer
// key and must never be configured as a signing key anywhere. It is deliberately
// NOT the dev key from testdata/: the point of a cross-implementation vector is
// that both sides sign with the same key, and a vector pinned under a key only
// one side holds proves nothing about the other.
const goldenSigningKeyBase64 = "WNfoQC3VMhjyFmBTsS+OsYYp+cPlJjPUSvN2PV78w1bqHzp7nZwnz868GucAveTX6/kW+9DiWRifVXgBJvlGqw=="

// goldenPublicKeyBase64 is the matching public half, in the standard-base64 form
// a verifier key slot takes. The TypeScript fixture calls it
// GOLDEN_PUBLIC_KEY_BASE64.
const goldenPublicKeyBase64 = "6h86e52cJ8/OvBrnAL3k1+v5FvvQ4lkYn1V4ASb5Rqs="

// goldenCases mirrors GOLDEN_CASES in the TypeScript fixture, in the same order.
// The three are chosen for what actually differs between the two languages
// rather than to look representative.
func goldenCases() []goldenCase {
	return []goldenCase{
		{
			// The ordinary shape from the plan. Pins field order, the envelope
			// framing, the domain-separation prefix and unpadded base64url.
			name: "agency key",
			claims: &Claims{
				V:     1,
				LID:   "lic_7f3a9c21",
				Org:   "ACME SAS",
				Sub:   "billing@acme.com",
				Tier:  "agency",
				Feat:  []string{"rbac", "ses_tenant"},
				MaxWS: 15,
				IAT:   1772582400,
				Exp:   1804118400,
			},
			envelope: "NFUSE1.eyJ2IjoxLCJsaWQiOiJsaWNfN2YzYTljMjEiLCJvcmciOiJBQ01FIFNBUyIsInN1YiI6ImJpbGxpbmdAYWNtZS5jb20iLCJ0aWVyIjoiYWdlbmN5IiwiZmVhdCI6WyJyYmFjIiwic2VzX3RlbmFudCJdLCJtYXhfd3MiOjE1LCJpYXQiOjE3NzI1ODI0MDAsImV4cCI6MTgwNDExODQwMH0.IUQdgFHpGYXlYxwWC1hXdS6FsKCjqlCSJYjmU8rhui0-6dSZ2mJNPMRFuojt-PCzTxw_Tc6p88Wb5c8jPYk3AA",
		},
		{
			// An organisation name containing an ampersand and angle brackets.
			// Go's encoder escapes those three characters to \u0026, \u003c and
			// \u003e and JSON.stringify does not, so this is the case that fails
			// first if the Go-compatible escaping on the TypeScript side is ever
			// removed — and it is exactly the case two JSON serialisers are most
			// likely to disagree about. Also pins a negotiated Custom tier,
			// unlimited workspaces and every feature.
			name: "custom key with characters Go escapes and JavaScript does not",
			claims: &Claims{
				V:     1,
				LID:   "lic_5c0f11ab",
				Org:   "Smith & Sons <Ltd>",
				Sub:   "ops@smith.example",
				Tier:  "custom",
				Feat:  []string{"rbac", "ses_tenant", "sso", "audit_logs"},
				MaxWS: -1,
				IAT:   1772582400,
				Exp:   1804118400,
			},
			envelope: "NFUSE1.eyJ2IjoxLCJsaWQiOiJsaWNfNWMwZjExYWIiLCJvcmciOiJTbWl0aCBcdTAwMjYgU29ucyBcdTAwM2NMdGRcdTAwM2UiLCJzdWIiOiJvcHNAc21pdGguZXhhbXBsZSIsInRpZXIiOiJjdXN0b20iLCJmZWF0IjpbInJiYWMiLCJzZXNfdGVuYW50Iiwic3NvIiwiYXVkaXRfbG9ncyJdLCJtYXhfd3MiOi0xLCJpYXQiOjE3NzI1ODI0MDAsImV4cCI6MTgwNDExODQwMH0.c1yy9X9AA3OWTejsqeQY-DTQMBdTz-HmO4lzoz03dbwi40Ylvr3w1YOLlPCKvsVonLTHNspZ7591A-YjKslbBQ",
		},
		{
			// A key that unlocks nothing. Pins [] rather than null, which is what
			// Go emits for a nil slice and what a nil Feat would silently produce.
			name: "key with no features",
			claims: &Claims{
				V:     1,
				LID:   "lic_00000000",
				Org:   "Solo",
				Sub:   "solo@example.com",
				Tier:  "custom",
				Feat:  []string{},
				MaxWS: 1,
				IAT:   1772582400,
				Exp:   1804118400,
			},
			envelope: "NFUSE1.eyJ2IjoxLCJsaWQiOiJsaWNfMDAwMDAwMDAiLCJvcmciOiJTb2xvIiwic3ViIjoic29sb0BleGFtcGxlLmNvbSIsInRpZXIiOiJjdXN0b20iLCJmZWF0IjpbXSwibWF4X3dzIjoxLCJpYXQiOjE3NzI1ODI0MDAsImV4cCI6MTgwNDExODQwMH0.Ax5V6kJWCNJ_qV3QSR6ZpSTW8E6n71HCv9ZuK5tX-ORmr5UV0-kEApadFdfunaLF2lXDzTvR7PqSWEkbtt8XBQ",
		},
	}
}

// goldenKeyPair decodes the cross-implementation test key. It is public: see
// goldenSigningKeyBase64.
func goldenKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()

	priv, err := base64.StdEncoding.DecodeString(goldenSigningKeyBase64)
	require.NoError(t, err)
	require.Len(t, priv, ed25519.PrivateKeySize)

	pub, err := base64.StdEncoding.DecodeString(goldenPublicKeyBase64)
	require.NoError(t, err)
	require.Len(t, pub, ed25519.PublicKeySize)

	// The two halves of a fixture that drifted apart would make every assertion
	// below fail with a signature error that reads like a bug in the verifier.
	derived, ok := ed25519.PrivateKey(priv).Public().(ed25519.PublicKey)
	require.True(t, ok)
	require.True(t, bytes.Equal(derived, pub),
		"the golden public key is not the one the golden private key produces")

	return ed25519.PublicKey(pub), ed25519.PrivateKey(priv)
}

// TestMintParseGoldenVectors pins the wire format against fixed inputs, in both
// directions and for all three cross-implementation cases.
//
// Nothing else would catch a change here: a re-serialisation, a reordered struct
// field, a different escaping policy or a changed domain-separation prefix would
// leave every other test in this repository green while silently invalidating
// every key the billing service produces. Changing these constants means
// changing the wire format — bump the envelope prefix and keep the old branch
// instead.
//
// If one of these ever goes red, the fix is NOT to re-pin the constant. It is to
// find which of the two implementations moved.
func TestMintParseGoldenVectors(t *testing.T) {
	goldenPub, goldenPriv := goldenKeyPair(t)

	for _, tc := range goldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			// Mint: Ed25519 signing is deterministic, so the envelope is
			// reproducible byte for byte from the same claims and key.
			assert.Equal(t, tc.envelope, mustMint(t, goldenPriv, tc.claims),
				"the Go implementation no longer produces the envelope pinned in licence-golden.fixture.ts")

			// Parse: and the same bytes must still decode back to the claims that
			// produced them, so a verifier and a signer cannot drift apart in
			// opposite directions and cancel out.
			claims, err := parseAt(tc.envelope, [][]byte{goldenPub}, referenceTime)
			require.NoError(t, err)
			assert.Equal(t, tc.claims, claims)
		})
	}
}

// TestGoldenVectorsAreNotAcceptedByTheDevKey guards the fixture against the
// mistake it replaced: a golden vector signed with a key only one implementation
// holds proves nothing about the other, and would keep passing while the two
// sides diverged.
func TestGoldenVectorsAreNotAcceptedByTheDevKey(t *testing.T) {
	devPub, _ := devKeyPair(t)

	for _, tc := range goldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseAt(tc.envelope, [][]byte{devPub}, referenceTime)
			require.ErrorIs(t, err, ErrBadSignature)
		})
	}
}

func TestSignedBytesAreDomainSeparated(t *testing.T) {
	// Without the prefix, a signature this authority produced over some other
	// Notifuse artefact could be replayed as a licence payload.
	assert.Equal(t, []byte("notifuse-license-v1:abc"), signedBytes("abc"))
}

func TestMint(t *testing.T) {
	_, devPriv := devKeyPair(t)

	t.Run("a private key of the wrong length is refused rather than passed to ed25519", func(t *testing.T) {
		// ed25519.Sign panics on a wrong-sized key, and a mint that panics in
		// the billing service is a failed checkout.
		raw, err := Mint(ed25519.PrivateKey("short"), validClaims())
		assert.Empty(t, raw)
		require.ErrorIs(t, err, ErrBadSigningKey)
	})

	t.Run("a display field carrying invalid utf-8 still mints and verifies", func(t *testing.T) {
		// Org and Sub are whatever the billing record holds, so minting must not
		// depend on them being clean. json.Marshal substitutes the replacement
		// character rather than failing; what matters is that the key it
		// produces still verifies, since a key that mints but does not parse
		// would reach the customer before anyone noticed.
		devPub, _ := devKeyPair(t)
		messy := validClaims()
		messy.Org = string([]byte{0xff, 0xfe})

		raw, err := Mint(devPriv, messy)
		require.NoError(t, err)

		claims, err := parseAt(raw, [][]byte{devPub}, referenceTime)
		require.NoError(t, err)
		assert.Equal(t, "\ufffd\ufffd", claims.Org)
	})
}

func TestDecodeKey(t *testing.T) {
	testCases := []struct {
		name    string
		encoded string
		want    []byte
	}{
		{
			name:    "an empty slot decodes to nothing",
			encoded: "",
			want:    nil,
		},
		{
			name: "a slot that does not decode becomes nil rather than panicking",
			// A typo in a committed key costs a support ticket; an init that
			// panicked would take a running installation down instead.
			encoded: "this is not base64",
			want:    nil,
		},
		{
			name:    "a well-formed key decodes to its bytes",
			encoded: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, ed25519.PublicKeySize)),
			want:    bytes.Repeat([]byte{7}, ed25519.PublicKeySize),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, decodeKey(tc.encoded))
		})
	}
}

// prodPubKeySourcePath is read as text for the same reason devPubKeySourcePath
// is: the constant it declares is compiled only under `!licdev`, and the check
// has to hold whichever tag the suite is run with.
const prodPubKeySourcePath = "pubkey_prod.go"

var prodSlotActiveRE = regexp.MustCompile(`slotActiveEncoded\s*=\s*"([^"]*)"`)

func TestHasUsableKey(t *testing.T) {
	realKey := bytes.Repeat([]byte{7}, ed25519.PublicKeySize)

	testCases := []struct {
		name string
		keys [][]byte
		want bool
	}{
		{
			name: "the thirty-two-zero-byte placeholder is not a usable key",
			// This is the state pubkey_prod.go ships in today, and the whole
			// reason HasTrustedKey exists: a build carrying it refuses every
			// licence key ever minted, and has to be able to say so.
			keys: [][]byte{bytes.Repeat([]byte{0}, ed25519.PublicKeySize), nil},
			want: false,
		},
		{
			name: "no slots at all is not a usable key",
			keys: nil,
			want: false,
		},
		{
			name: "two empty slots are not a usable key",
			keys: [][]byte{nil, nil},
			want: false,
		},
		{
			name: "a slot of the wrong length is not a usable key",
			// ed25519.Verify panics on one of these, so it must be refused here
			// rather than reached.
			keys: [][]byte{bytes.Repeat([]byte{7}, 16)},
			want: false,
		},
		{
			name: "one real key in the second slot is enough",
			// The rotation shape: the retiring slot has been emptied and the new
			// key has not been moved up yet.
			keys: [][]byte{nil, realKey},
			want: true,
		},
		{
			name: "a real key beside the placeholder is usable",
			keys: [][]byte{realKey, bytes.Repeat([]byte{0}, ed25519.PublicKeySize)},
			want: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, hasUsableKey(tc.keys))
		})
	}
}

// lowOrderTestEncodings is this file's OWN copy of the fourteen small-order
// Ed25519 point encodings, written out again rather than read from the table in
// license.go.
//
// That duplication is the point. A test that iterated the production list would
// go green the instant an entry was deleted from it, which is exactly the
// silent-weakening this package has already shipped twice under a fully green
// build. Two independent copies means removing an entry from the guard fails
// here, and removing it from here fails review against the published list.
//
// The list is the union of the eight points of small order and their
// non-canonical encodings, as published in "Taming the many EdDSAs"
// (Chalkias, Garillot, Nikolaenko) and as blacklisted by libsodium's
// ge25519_has_small_order.
var lowOrderTestEncodings = []struct {
	name string
	hex  string
}{
	{"the identity, one base64 character from the committed placeholder", "0100000000000000000000000000000000000000000000000000000000000000"},
	{"the identity with the sign bit set", "0100000000000000000000000000000000000000000000000000000000000080"},
	{"the order-2 point", "ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f"},
	{"the order-2 point with the sign bit set", "ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
	{"the all-zero order-4 point, which is also the committed placeholder", "0000000000000000000000000000000000000000000000000000000000000000"},
	{"the other order-4 point", "0000000000000000000000000000000000000000000000000000000000000080"},
	{"the first order-8 point", "26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05"},
	{"the first order-8 point with the sign bit set", "26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc85"},
	{"the second order-8 point", "c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a"},
	{"the second order-8 point with the sign bit set", "c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac03fa"},
	{"p, a non-canonical encoding of the order-4 point", "edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f"},
	{"p with the sign bit set", "edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
	{"p+1, a non-canonical encoding of the identity", "eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f"},
	{"p+1 with the sign bit set", "eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
}

func mustDecodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	require.Len(t, b, ed25519.PublicKeySize)
	return b
}

// forgeUnder builds a licence envelope that ed25519.Verify accepts under pub,
// without knowing any private key, and returns false if it cannot.
//
// This is the whole attack, and it is twenty lines. Against a small-order public
// key the verification equation [S]B = R + [k]A collapses: with S = 0 the left
// side is the identity, and [k]A ranges over a handful of points regardless of
// what k is, so an R can simply be picked from that same handful. The forger
// also chooses the payload — they are minting themselves a licence — so they may
// vary it freely until one lands. In practice this takes between one and
// thirty-four attempts.
func forgeUnder(t *testing.T, pub []byte) (string, bool) {
	t.Helper()

	for n := 0; n < 64; n++ {
		claims := &Claims{
			V:     SchemaVersion,
			LID:   fmt.Sprintf("lic_forged_%d", n),
			Org:   "Anyone At All",
			Sub:   "forger@example.com",
			Tier:  "custom",
			Feat:  []string{"rbac", "ses_tenant", "sso", "audit_logs"},
			MaxWS: -1,
			IAT:   1772582400,
			Exp:   4102444800,
		}
		payload, err := json.Marshal(claims)
		require.NoError(t, err)

		encoded := base64.RawURLEncoding.EncodeToString(payload)
		signed := signedBytes(encoded)

		for _, candidate := range lowOrderTestEncodings {
			// R is a small-order point and S is left as thirty-two zero bytes.
			sig := make([]byte, ed25519.SignatureSize)
			copy(sig, mustDecodeHex(t, candidate.hex))

			if ed25519.Verify(ed25519.PublicKey(pub), signed, sig) {
				return envelopePrefix + "." + encoded + "." + base64.RawURLEncoding.EncodeToString(sig), true
			}
		}
	}
	return "", false
}

// TestLowOrderPointsAreForgeable is the evidence for the guard below it: it
// shows, by construction, that Go's ed25519.Verify accepts a signature nobody
// signed under every one of these encodings.
//
// crypto/ed25519 does not reject small-order public keys — that is a documented
// property of the ecosystem's decoding rules, not a bug — so the rejection has
// to happen here. If this test ever fails because the standard library started
// refusing one of these, that is good news: keep the guard, and record the
// change here rather than deleting either.
func TestLowOrderPointsAreForgeable(t *testing.T) {
	for _, tc := range lowOrderTestEncodings {
		t.Run(tc.name, func(t *testing.T) {
			_, found := forgeUnder(t, mustDecodeHex(t, tc.hex))
			assert.True(t, found,
				"ed25519.Verify no longer accepts a forgery under this encoding; keep it in the guard and update this test")
		})
	}
}

// TestUsableKeysRefusesLowOrderPoints is the guard itself.
//
// A slot holding one of these is not a weak key, it is an open door: every
// envelope on earth verifies against it, so the licensing scheme is switched off
// and no revocation is possible for any key already issued. The identity is one
// base64 character away from the placeholder a human hand-edits at release time
// (A -> Q, adjacent on the keyboard), which is how a wrong slot gets there.
func TestUsableKeysRefusesLowOrderPoints(t *testing.T) {
	devPub, devPriv := devKeyPair(t)

	for _, tc := range lowOrderTestEncodings {
		t.Run(tc.name, func(t *testing.T) {
			key := mustDecodeHex(t, tc.hex)

			forged, found := forgeUnder(t, key)
			require.True(t, found, "the forgery construction failed, so this case proves nothing")

			assert.Empty(t, usableKeys([][]byte{key}),
				"a small-order slot must be skipped exactly as the placeholder is")
			assert.False(t, hasUsableKey([][]byte{key}),
				"a build whose only slot is a small-order point carries no signing key and must say so at startup")

			claims, err := parseAt(forged, [][]byte{key}, referenceTime)
			assert.Nil(t, claims)
			require.ErrorIs(t, err, ErrNoTrustedKey,
				"this envelope was minted with no private key at all and must not be accepted")

			t.Run("beside a good key the forgery is refused and real keys still verify", func(t *testing.T) {
				// The realistic shape: one slot is mistyped during a rotation
				// while the other still holds the authority everyone's keys were
				// minted under.
				slots := [][]byte{key, devPub}

				_, err := parseAt(forged, slots, referenceTime)
				require.ErrorIs(t, err, ErrBadSignature)

				genuine, err := parseAt(mustMint(t, devPriv, validClaims()), slots, referenceTime)
				require.NoError(t, err, "the guard must not cost the good slot beside it")
				assert.Equal(t, "lic_7f3a9c21", genuine.LID)
			})
		})
	}
}

// TestLowOrderEncodingsAreWellFormed pins the production table itself.
//
// decodeHexKeys drops a literal that does not decode rather than panicking, so a
// single mistyped hex character would silently shorten the deny-list instead of
// failing anywhere. Ten entries would still look like a guard in review.
func TestLowOrderEncodingsAreWellFormed(t *testing.T) {
	require.Len(t, lowOrderEncodings, 14,
		"an entry was dropped or added; the published small-order set has fourteen encodings")

	seen := map[string]bool{}
	for _, key := range lowOrderEncodings {
		assert.Len(t, key, ed25519.PublicKeySize)
		assert.False(t, seen[string(key)], "duplicate entry %x hides the loss of a real one", key)
		seen[string(key)] = true
	}

	// And every encoding this file independently lists must be in it. The two
	// lists are written out separately on purpose; this is where they meet.
	for _, tc := range lowOrderTestEncodings {
		assert.True(t, seen[string(mustDecodeHex(t, tc.hex))],
			"%s is missing from the guard's table", tc.name)
	}
}

// TestUsableKeysKeepsOrdinaryKeys pins the other direction: the guard is a
// fourteen-entry deny-list, not a heuristic, so no key anyone will ever be
// issued can trip it. A guard that refused real keys would reject a paying
// customer's licence on the day they bought it.
func TestUsableKeysKeepsOrdinaryKeys(t *testing.T) {
	for i := 0; i < 256; i++ {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		require.True(t, hasUsableKey([][]byte{pub}), "an ordinary generated key was refused: %x", pub)
	}
}

// TestProductionKeySlotIsAPlaceholder asserts that pubkey_prod.go and the
// comment above its constant still agree with each other.
//
// While the slot holds the placeholder, the file must carry the TODO naming the
// human step, because a placeholder nobody is reminded to replace is how a
// release ships with licensing switched off. The day a real key is pasted in,
// this test does not need editing: it flips to asserting that the key is a
// well-formed, non-zero, usable Ed25519 public key, which is the check that
// matters from then on.
//
// It reads the file as text rather than the constant so that it runs under both
// build tags — the placeholder must not be able to hide behind `-tags licdev`.
func TestProductionKeySlotIsAPlaceholder(t *testing.T) {
	source, err := os.ReadFile(prodPubKeySourcePath)
	require.NoError(t, err, "pubkey_prod.go is missing; if it moved, this check must follow it rather than be deleted")

	match := prodSlotActiveRE.FindSubmatch(source)
	require.NotNil(t, match, "slotActiveEncoded is no longer a plain string constant in %s — update this parser, do not stop checking", prodPubKeySourcePath)

	decoded := decodeKey(string(match[1]))

	if hasUsableKey([][]byte{decoded}) {
		// A real signing key has landed. From here on, the only thing worth
		// asserting is that it is well formed.
		assert.Len(t, decoded, ed25519.PublicKeySize)
		assert.False(t, isZero(decoded))
		// strings.Contains rather than assert.NotContains: on failure the latter
		// prints the whole of pubkey_prod.go as one escaped line, which buries the
		// message on the one day anybody reads it.
		assert.False(t, strings.Contains(string(source), "TODO(release)"),
			"the signing key has been replaced, so the release TODO is stale and must go")
		return
	}

	assert.True(t, isZero(decoded), "the production slot is unusable for a reason other than being the placeholder — a typo in a committed key rejects every licence just as silently")
	assert.True(t, strings.Contains(string(source), "TODO(release)"),
		"the placeholder signing key is still in place and the TODO naming the human step that replaces it has been removed")
	assert.True(t, strings.Contains(string(source), "licensegen keygen"),
		"the TODO must name the command that generates the real pair, so nobody has to guess at it under release pressure")
}

// releaseSelfTestPath holds the envelopes minted during the key ceremony, one
// per populated slot. See the file itself, and the TODO(release) block in
// pubkey_prod.go, for what goes in it.
const releaseSelfTestPath = "testdata/release_selftest.json"

var prodSlotNextRE = regexp.MustCompile(`slotNextEncoded\s*=\s*"([^"]*)"`)

type releaseSelfTest struct {
	SlotActive string `json:"slot_active"`
	SlotNext   string `json:"slot_next"`
}

// TestReleaseSelfTestVectorMatchesTheCompiledKeys is what tells the operator, on
// release day, that the thirty-two bytes they pasted are the right thirty-two
// bytes.
//
// Every other check in this package is satisfied by any thirty-two-byte value
// that is not zero and not a small-order point. The seed of the pair — printed
// on the line directly above the public half by `licensegen keygen`, and the
// obvious thing to copy by mistake — is exactly such a value. Paste it, and the
// build is green, HasTrustedKey is true, the startup warning stays quiet, and
// every key the billing service ever mints is refused with ErrBadSignature. The
// same is true of the public half of a pair whose private half went to a
// different secret store, or of a paste that lost a character to a terminal.
//
// So the only thing that can distinguish a right paste from a wrong one is an
// envelope the real private key actually signed. That is what this vector is.
// The negative half — a byte-flipped copy that must be refused — is derived here
// rather than committed, so it cannot be pinned wrong or forgotten; it proves the
// verifier ran at all, rather than something upstream returning success.
//
// The test skips cleanly while a slot holds the placeholder, so it costs nothing
// until the day it matters, and fails hard the moment a real key is compiled in
// without a vector to check it against. That asymmetry is the whole design: an
// opt-in check nobody is forced to fill in would be green on the day it was
// needed.
func TestReleaseSelfTestVectorMatchesTheCompiledKeys(t *testing.T) {
	// Read as text, like the placeholder test above it, so the check holds under
	// -tags licdev too: a production slot must not be able to hide behind the
	// dev build tag.
	source, err := os.ReadFile(prodPubKeySourcePath)
	require.NoError(t, err, "pubkey_prod.go is missing; if it moved, this check must follow it rather than be deleted")

	raw, err := os.ReadFile(releaseSelfTestPath)
	require.NoError(t, err, "%s is missing; it is the only thing that can tell a correct key paste from a wrong one, so restore it rather than deleting this test", releaseSelfTestPath)

	var vector releaseSelfTest
	require.NoError(t, json.Unmarshal(raw, &vector), "%s is not valid json", releaseSelfTestPath)

	slots := []struct {
		name     string
		constant string
		pattern  *regexp.Regexp
		envelope string
	}{
		{"the active slot verifies the envelope pinned for it", "slotActiveEncoded", prodSlotActiveRE, vector.SlotActive},
		{"the rotation slot verifies the envelope pinned for it", "slotNextEncoded", prodSlotNextRE, vector.SlotNext},
	}

	for _, slot := range slots {
		t.Run(slot.name, func(t *testing.T) {
			match := slot.pattern.FindSubmatch(source)
			require.NotNil(t, match, "%s is no longer a plain string constant in %s — update this parser, do not stop checking", slot.constant, prodPubKeySourcePath)

			// usableKeys, not decodeKey alone: a slot this build would refuse to
			// verify with is a slot there is nothing to self-test.
			usable := usableKeys([][]byte{decodeKey(string(match[1]))})

			if len(usable) == 0 {
				require.Empty(t, slot.envelope,
					"a self-test envelope is pinned for %s but that slot still holds the placeholder — the ceremony was done out of order, or the key paste did not land", slot.constant)
				t.Skipf("%s holds no usable key yet, so there is nothing to self-test", slot.constant)
			}

			require.NotEmpty(t, slot.envelope,
				"a signing key is compiled into %s but no self-test envelope is pinned for it in %s, so nothing checks that those thirty-two bytes are the public half of the pair the billing service signs with. Mint one — see the TODO(release) block in %s — rather than emptying the slot",
				slot.constant, releaseSelfTestPath, prodPubKeySourcePath)

			claims, err := parseAt(slot.envelope, usable, time.Now())
			require.NoError(t, err,
				"%s does not verify the envelope pinned for it. The overwhelmingly likely cause is that the constant is not the public half of the minting pair: the seed prints directly above it in `licensegen keygen` output, and a paste that lost a character re-pads to the same length. Check the paste before re-minting the vector.", slot.constant)
			require.NotNil(t, claims)

			prefix, payload, sig, ok := splitEnvelope(slot.envelope)
			require.True(t, ok, "the pinned envelope is not a three-segment NFUSE1 key")

			tampered := []struct {
				name string
				raw  string
			}{
				{"a flipped payload byte is refused", prefix + "." + flipByte(t, payload, 10) + "." + sig},
				{"a flipped signature byte is refused", prefix + "." + payload + "." + flipByte(t, sig, 10)},
			}
			for _, tc := range tampered {
				t.Run(tc.name, func(t *testing.T) {
					_, err := parseAt(tc.raw, usable, time.Now())
					require.ErrorIs(t, err, ErrBadSignature,
						"a tampered copy of the self-test envelope was not refused, so the positive half above proves nothing")
				})
			}
		})
	}
}
