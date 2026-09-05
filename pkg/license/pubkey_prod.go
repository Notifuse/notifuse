//go:build !licdev

package license

// The signing keys trusted by a release build.
//
// Two slots exist from the first release, not from the first rotation. Without a
// second slot, a compromised signing key forces an emergency release whose only
// possible shape is "trust this new key instead", which rejects every key
// already in a paying customer's hands — under time pressure, during an
// incident, on the day the support queue is already on fire. With a second slot,
// the same incident is a release that adds a key while the old one keeps
// working, and the retiring slot is emptied at leisure once every licence has
// been re-issued. The cost of carrying the remedy in advance is five lines and
// thirty-two bytes.
//
// The keys are committed as base64 constants decoded at init rather than
// //go:embed-ed from a file. Thirty-two bytes are not worth a second file plus a
// directive: a constant puts the key, the comment explaining what it is, and the
// build tag that selects it in one place a reviewer reads at once, and a release
// commit that changes it shows the change in the diff instead of in a binary
// blob.
const (
	// slotActiveEncoded is a PLACEHOLDER and MUST BE REPLACED BEFORE ANY RELEASE.
	//
	// It is thirty-two zero bytes, which is not a valid Ed25519 public key and is
	// explicitly refused by usableKeys, so a build that still carries it rejects
	// every licence key in existence with ErrNoTrustedKey. That is the fail-safe
	// direction — an unreplaced placeholder degrades every installation to the
	// free tier and produces one unmistakable error string, whereas a build that
	// silently accepted anything would ship the product with its licensing
	// switched off and nothing to show for it in the logs.
	//
	// Two wrong values do NOT fail safe, and neither announces itself.
	//
	// The first is the neutral element of the curve — 0x01 followed by thirty-one
	// zeros — whose base64 differs from the placeholder below in a single
	// character, at a position where A and Q are adjacent on the keyboard. Under
	// it, every envelope in existence verifies against a signature anyone can
	// write out by hand: the licensing scheme is switched off, and no key already
	// in a customer's hands can be revoked. usableKeys refuses it and every other
	// small-order encoding; see lowOrderEncodings in license.go.
	//
	// The second is any thirty-two bytes that are simply not the public half of
	// the pair whose private half went to the secret manager — the public key of
	// a second keygen run when the first had scrolled off the screen, a seed from
	// a secret store that keeps the thirty-two-byte form, a paste that lost a
	// character and re-padded to the same length. Nothing about such a slot looks
	// wrong: it is the right length, it is not zero, it is not small-order, so
	// HasTrustedKey reports true and the startup warning stays quiet while every
	// paying customer's key is refused with ErrBadSignature. No property of the
	// bytes can catch that. Only step 4 below can.
	//
	// A rotation is the same ceremony against slotNextEncoded and its own
	// "slot_next" vector, which is why that file has two fields. The retiring slot
	// keeps its vector until the day its key is removed, and then both go together.
	//
	// The release ceremony has been completed: a real pair was generated on a
	// trusted machine, its private half stored in the billing service's secret
	// manager, its public half pasted below, and a self-test envelope minted with
	// that private half pinned in testdata/release_selftest.json.
	//
	// TestReleaseSelfTestVectorMatchesTheCompiledKeys is what keeps the two honest
	// from here on: it verifies that pinned envelope against these exact bytes, so
	// a future edit that mistypes or replaces the key turns `go test ./pkg/license/`
	// red instead of refusing every paying customer's licence in silence. Never
	// re-mint the vector to match a slot you have not verified — that pins the
	// mistake instead of catching it.
	slotActiveEncoded = "9WeH7ppMdiG6MOUKX587oGTR09NfRPZHNM0o5jw6AoU="

	// slotNextEncoded is empty until a rotation is under way. During a rotation
	// both slots are populated and both are accepted; once every outstanding
	// licence has been re-issued under the new key, the retiring key is removed
	// from slotActive and the new one moves into it.
	slotNextEncoded = ""
)

// trustedKeys is the slot set Parse verifies against. Slots that are empty, that
// do not decode, that carry the placeholder, or that hold a point of small order
// are skipped rather than fatal — see usableKeys, which explains why this package
// must not panic on a customer's server.
var trustedKeys = [][]byte{
	decodeKey(slotActiveEncoded),
	decodeKey(slotNextEncoded),
}
