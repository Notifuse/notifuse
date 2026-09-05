//go:build licdev

package license

// The signing key trusted by a development build (`go build -tags licdev`).
//
// This exists so that anyone working on the licensing code can mint a key and
// watch a real entitlement change, without the production signing key ever
// leaving its secret store and without a debug switch in the release binary that
// could be flipped. The build tag is the only seam licensing is allowed to add
// to this repository: no feature code branches on it, and the two pubkey files
// declare exactly the same symbols so that everything downstream compiles
// identically either way.
//
// The matching private key is committed under testdata/dev_signing_key.json. It
// is a fixture, not a secret: it is useless against a release binary, which
// trusts a different key entirely, and having it in the tree is what lets the
// test suite exercise a genuine end-to-end round-trip rather than a key it
// invented for itself.
const (
	// slotActiveEncoded is the dev public key. Its private half is the fixture
	// under testdata/, and license_test.go checks that the two still match — a
	// pair that silently drifted apart would make every dev build reject every
	// dev key with a signature error that looks like a bug in the verifier.
	slotActiveEncoded = "HN2174FtXW2CpzcNs/QO6xrMbjaQQJdkG/r9q3CZyZA="

	// slotNextEncoded mirrors the production layout: empty until a rotation is
	// under way. It is here so that rotation can be rehearsed under the dev tag
	// against the same code path release builds take.
	slotNextEncoded = ""
)

// trustedKeys is the slot set Parse verifies against, identical in shape to the
// production one.
var trustedKeys = [][]byte{
	decodeKey(slotActiveEncoded),
	decodeKey(slotNextEncoded),
}
