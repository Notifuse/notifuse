//go:build !licdev

package license

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReleaseBuildRefusesEveryKeyWhileThePlaceholderStands checks the compiled
// answer, not the source text: the build tag on this file is what makes it the
// release build's own statement about itself.
//
// Today it asserts the fail-safe state — the placeholder is compiled in, so
// HasTrustedKey is false and Parse refuses even a perfectly well-formed,
// correctly signed envelope. That is deliberate and correct until a human
// generates the real key. The day one is pasted into pubkey_prod.go, this test
// flips on its own to asserting that a release build actually verifies, which is
// the assertion that matters from then on. Either way it fails if the two ever
// disagree.
func TestReleaseBuildRefusesEveryKeyWhileThePlaceholderStands(t *testing.T) {
	_, priv := goldenKeyPair(t)
	raw := mustMint(t, priv, validClaims())

	if !HasTrustedKey() {
		_, err := Parse(raw)
		require.ErrorIs(t, err, ErrNoTrustedKey,
			"a build with no usable signing key must refuse every key with the one error string that names the cause")
		return
	}

	// A real key is compiled in. It is not the golden test key, so this envelope
	// must be refused as a forgery rather than as an absence.
	_, err := Parse(raw)
	assert.ErrorIs(t, err, ErrBadSignature,
		"a release build must not trust the public test key from the golden fixture")
}
