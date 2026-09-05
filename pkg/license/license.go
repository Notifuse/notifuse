// Package license parses and verifies Notifuse licence keys.
//
// A licence key is a single line, self-contained, and verified entirely offline
// against a public key compiled into this binary:
//
//	NFUSE1.<base64url(payload_json)>.<base64url(ed25519_sig)>
//
// Offline verification is the point. An installation never phones home, so
// neither a customer's network outage, nor ours, nor the eventual death of any
// licensing vendor can degrade an installation that has already been paid for.
// The price of that is that there is no revocation: a key is good until its exp,
// and that cost is accepted deliberately rather than paid for with a callback
// that would turn every customer's outage into our outage.
//
// This package is pure. It performs no I/O, reads no configuration, logs
// nothing, and imports nothing outside the standard library, so that policy
// lives in exactly one place — the caller — and this file only ever answers
// whether an envelope is authentic and well formed.
package license

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SchemaVersion is the only payload schema this build understands.
//
// An unknown version is refused rather than best-effort decoded. A key minted in
// 2028 against a schema this binary has never seen must not be interpreted with
// today's field meanings: silently reading a future payload is how a capability
// gets granted that nobody ever sold.
const SchemaVersion = 1

// envelopePrefix identifies the envelope format itself, not the payload schema.
// It changes only if the framing changes (different signature algorithm, a
// fourth segment), which is a harder break than a payload version bump.
const envelopePrefix = "NFUSE1"

// signingPrefix is prepended to the base64url payload before signing and before
// verification — domain separation. Without it, a signature this key produced
// over some other Notifuse artefact could be replayed as a licence. Mint and
// Parse both go through signedBytes rather than assembling these bytes
// independently, so the two halves cannot disagree about what was signed.
const signingPrefix = "notifuse-license-v1:"

// MaxClockSkew is how far into the future a key's iat may sit before Parse
// refuses it.
//
// This is the only clock defence in the whole system, and it is deliberately a
// tolerance rather than a strict comparison: a few seconds of drift between the
// machine that mints a key and the customer's server is normal, and refusing a
// key on the day it was bought is a support ticket, not a defence. There is no
// persisted high-water mark anywhere by design — rolling a clock back far enough
// to outrun an annual exp plus its grace period breaks AWS SigV4 request
// signing, TLS notBefore and SAML assertion windows inside the same process, so
// the evasion is not one this product can actually be operated through, while a
// stateful check would misfire on the ordinary act of restoring a production
// snapshot into staging.
const MaxClockSkew = 5 * time.Minute

// The failures Parse can report. All of them are matchable with errors.Is, and
// callers are expected to treat every one of them identically — degrade to the
// free tier — and to distinguish them only when writing the support log line
// that tells a customer why their key did not take.
var (
	// ErrMalformedEnvelope means the string is not shaped like a licence key at
	// all: wrong prefix, or not exactly three dot-separated segments.
	ErrMalformedEnvelope = errors.New("licence key is not a NFUSE1 envelope")

	// ErrBadEncoding means a segment is not valid unpadded base64url.
	ErrBadEncoding = errors.New("licence key segment is not valid base64url")

	// ErrNoTrustedKey means this binary carries no usable public key, so nothing
	// could have been verified. It is what a build still carrying the
	// pubkey_prod.go placeholder returns for every key ever minted.
	ErrNoTrustedKey = errors.New("no licence signing key is compiled into this binary")

	// ErrBadSignature means no compiled public key verifies the envelope: a
	// forgery, a tampered payload, a truncated signature, or a key minted for a
	// different signing authority.
	ErrBadSignature = errors.New("licence key signature does not verify")

	// ErrMalformedPayload means the signature was good but the payload is not the
	// JSON object this schema describes.
	ErrMalformedPayload = errors.New("licence key payload is not valid json")

	// ErrUnknownVersion means the payload announces a schema this build does not
	// implement. See SchemaVersion for why that is refused rather than tolerated.
	ErrUnknownVersion = errors.New("licence key schema version is not supported")

	// ErrFutureIssuedAt means iat is further ahead than MaxClockSkew allows.
	ErrFutureIssuedAt = errors.New("licence key was issued in the future")

	// ErrBadSigningKey is returned by Mint when handed something that is not an
	// Ed25519 private key. Parse never returns it.
	ErrBadSigningKey = errors.New("ed25519 private key has the wrong length")
)

// Claims is the payload of a licence key.
//
// Two rules govern how these fields may be consumed, and neither is enforceable
// here. Tier is for display only — never branch on it, or a Custom deal priced
// by hand unlocks nothing. Feat is an explicit allow-list and never a deny-list,
// so a key minted today cannot silently unlock a capability invented in 2028.
type Claims struct {
	// V is the payload schema version. Parse refuses anything but SchemaVersion.
	V int `json:"v"`

	// LID identifies the licence, for support and for de-duplicating a re-issue
	// against the key it replaces.
	LID string `json:"lid"`

	// Org and Sub are the licensee's name and billing address, shown in the
	// console as "Licensed to: ACME SAS — billing@acme.com". They are a social
	// deterrent against key sharing, not a cryptographic one, and that is the
	// whole of their job.
	Org string `json:"org"`
	Sub string `json:"sub"`

	// Tier is display only. Never branch on it: entitlements are Feat and MaxWS.
	Tier string `json:"tier"`

	// Feat is the allow-list of licensed capabilities.
	Feat []string `json:"feat"`

	// MaxWS is the workspace ceiling; -1 means unlimited.
	MaxWS int `json:"max_ws"`

	// IAT and Exp are Unix seconds. There is no grace field: the grace period is
	// a code constant in the consuming service, so the policy can be changed
	// without re-minting a single key that is already in a customer's hands.
	IAT int64 `json:"iat"`
	Exp int64 `json:"exp"`
}

// Parse verifies a licence envelope against the public keys compiled into this
// binary and returns its claims.
//
// The order of operations is load-bearing. The signature is checked before the
// payload is unmarshalled, so this package never interprets JSON that an
// attacker chose. Only then are the schema version and the issue date examined.
//
// Parse deliberately does not look at Exp. Expiry is a state the caller derives
// against the current time — active, grace, expired — not a parse failure: a
// lapsed key still names its licensee, which is exactly what the console needs
// in order to tell a customer whose key ran out apart from an installation that
// never had one.
func Parse(raw string) (*Claims, error) {
	return parseAt(raw, trustedKeys, time.Now())
}

// parseAt is Parse with the trusted key set and the clock passed in, so that
// tests can exercise rotation and skew without touching package state. Package
// state would have to be mutable, and every test in this repository runs under
// the race detector.
func parseAt(raw string, keys [][]byte, now time.Time) (*Claims, error) {
	prefix, encodedPayload, encodedSig, ok := splitEnvelope(raw)
	if !ok || prefix != envelopePrefix {
		return nil, ErrMalformedEnvelope
	}

	payload, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return nil, fmt.Errorf("%w: payload", ErrBadEncoding)
	}
	sig, err := base64.RawURLEncoding.DecodeString(encodedSig)
	if err != nil {
		return nil, fmt.Errorf("%w: signature", ErrBadEncoding)
	}

	usable := usableKeys(keys)
	if len(usable) == 0 {
		return nil, ErrNoTrustedKey
	}

	// Any slot may verify. Trying them all is what makes a key rotation a
	// non-event: keys minted under the retiring authority and keys minted under
	// the new one are both accepted for as long as both slots are populated.
	verified := false
	signed := signedBytes(encodedPayload)
	for _, key := range usable {
		if ed25519.Verify(ed25519.PublicKey(key), signed, sig) {
			verified = true
			break
		}
	}
	if !verified {
		return nil, ErrBadSignature
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedPayload, err)
	}

	if claims.V != SchemaVersion {
		return nil, fmt.Errorf("%w: v=%d", ErrUnknownVersion, claims.V)
	}

	if time.Unix(claims.IAT, 0).After(now.Add(MaxClockSkew)) {
		return nil, ErrFutureIssuedAt
	}

	return &claims, nil
}

// Mint builds a signed envelope for the given claims.
//
// It lives beside Parse so that exactly one implementation of the wire format
// exists in the tree. The signer — the licensegen CLI and the billing service, both in the private cloud repository, that
// mints a key after a payment — and the verifier that runs on every customer's
// server therefore cannot drift apart: a change to the payload encoding or to
// the domain-separation prefix breaks both halves at once, here, in this
// package's tests, rather than silently locking every installation out.
//
// Minting requires the private key, which lives in a secret store and never
// ships, so carrying this function in the server binary grants a reader nothing.
func Mint(priv ed25519.PrivateKey, claims *Claims) (string, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return "", ErrBadSigningKey
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	sig := ed25519.Sign(priv, signedBytes(encodedPayload))

	return envelopePrefix + "." + encodedPayload + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// signedBytes is the exact byte string that is signed and verified: the
// domain-separation prefix followed by the base64url payload as it appears on
// the wire, not the decoded JSON. Signing the encoded form means a verifier
// never has to re-serialise anything to check a signature, so no JSON
// canonicalisation question can ever arise.
func signedBytes(encodedPayload string) []byte {
	return []byte(signingPrefix + encodedPayload)
}

// splitEnvelope cuts raw into its three segments, rejecting anything with a
// different number of them.
//
// The separator is unambiguous because unpadded base64url's alphabet
// (A-Z a-z 0-9 - _) contains no dot, so no segment can hide one. This is a hand
// rolled split rather than strings.Split to keep the package's import set to the
// crypto and encoding primitives it actually needs.
func splitEnvelope(raw string) (prefix, payload, sig string, ok bool) {
	first, second := -1, -1
	for i := 0; i < len(raw); i++ {
		if raw[i] != '.' {
			continue
		}
		switch {
		case first < 0:
			first = i
		case second < 0:
			second = i
		default:
			// A fourth segment is not a licence key with something appended: it
			// is a different format, and guessing at it is how a parser grows a
			// bug that only fires on someone else's input.
			return "", "", "", false
		}
	}
	if first < 0 || second < 0 {
		return "", "", "", false
	}
	return raw[:first], raw[first+1 : second], raw[second+1:], true
}

// usableKeys returns the slots that can actually verify something.
//
// Three kinds of slot are skipped rather than reported as an error. An empty or
// mis-sized slot is the ordinary state of slotNext between rotations, or a
// half-decoded constant. An all-zero slot is the placeholder committed in
// pubkey_prod.go, which must be replaced before any release. And a slot holding
// a point of small order is refused for the reason spelled out at
// lowOrderEncodings: such a slot does not verify weakly, it verifies
// everything.
//
// Skipping rather than failing is also what keeps this package to its promise of
// never panicking on a customer's server: ed25519.Verify panics on a public key
// of the wrong length, so a mis-typed or half-decoded slot would otherwise take
// the process down at the exact moment a licence is first checked. Every skip
// here moves the build towards refusing licences, never towards accepting them,
// which is the only direction this decision is allowed to fail in.
func usableKeys(keys [][]byte) [][]byte {
	usable := make([][]byte, 0, len(keys))
	for _, key := range keys {
		if len(key) != ed25519.PublicKeySize || isZero(key) || isLowOrder(key) {
			continue
		}
		usable = append(usable, key)
	}
	return usable
}

// isZero reports whether every byte is zero, which identifies the placeholder
// key. No constant-time comparison is wanted or needed: a public key is not a
// secret, and the answer is already visible in the binary.
//
// The all-zero encoding is also in lowOrderEncodings, so this check is not the
// only thing refusing it. It is kept because it names the placeholder rather
// than a curve property, and because the release-day test reads it to tell "the
// ceremony has not happened yet" apart from "the ceremony produced a slot that
// cannot verify anything".
func isZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// lowOrderEncodings is every published encoding of an Ed25519 point of small
// order: the eight points whose order divides eight, plus the non-canonical
// encodings of several of them that the ecosystem's decoders accept.
//
// A public key that is one of these is not a weak key, it is an open door.
// Verification checks [S]B = R + [k]A; when A has small order, [k]A takes one of
// a handful of values whatever k is, so a forger picks R from that same handful,
// sets S to zero, and produces a signature that verifies over a message nobody
// signed. crypto/ed25519 does not reject these — accepting non-canonical
// encodings is a documented compatibility decision in Go's decoder, not a bug —
// so the rejection has to happen here, before a slot is ever handed to Verify.
//
// The one that matters is the first: the neutral element encodes as 0x01
// followed by thirty-one zero bytes, whose base64 is
// AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA= — a single character away from
// the placeholder in pubkey_prod.go, at a position where A and Q are adjacent on
// the keyboard. That constant is hand-edited by a human at release time. A build
// that shipped it would accept every envelope in existence, silently, with no
// revocation possible for any key already issued, and every existing check would
// stay green: it is thirty-two bytes long and it is not zero.
//
// This is a table rather than curve arithmetic on purpose. The set is small,
// closed and published — it cannot grow — and a deny-list of fourteen constants
// a reviewer can diff against the reference is cheaper and more auditable than a
// scalar multiplication, and adds no dependency to a package whose whole promise
// is that it imports nothing but the standard library.
//
// Sources: "Taming the many EdDSAs" (Chalkias, Garillot, Nikolaenko, SSR 2020),
// Table 4; and the blacklist in libsodium's ge25519_has_small_order.
var lowOrderEncodings = decodeHexKeys(
	"0100000000000000000000000000000000000000000000000000000000000000", // order 1: the neutral element
	"0100000000000000000000000000000000000000000000000000000000000080", // order 1, sign bit set
	"ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f", // order 2
	"ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", // order 2, sign bit set
	"0000000000000000000000000000000000000000000000000000000000000000", // order 4: also the committed placeholder
	"0000000000000000000000000000000000000000000000000000000000000080", // order 4, the other sign
	"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05", // order 8
	"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc85", // order 8, sign bit set
	"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a", // order 8
	"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac03fa", // order 8, sign bit set
	"edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f", // p, a non-canonical zero
	"edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", // p, sign bit set
	"eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f", // p+1, a non-canonical neutral element
	"eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", // p+1, sign bit set
)

// isLowOrder reports whether a slot holds one of the small-order encodings.
//
// A linear scan over fourteen entries runs once per Parse, on a code path that
// already computes an Ed25519 verification, so its cost is not measurable. No
// constant-time comparison is wanted: a public key is not a secret, and the
// answer is compiled into the binary.
func isLowOrder(b []byte) bool {
	for _, bad := range lowOrderEncodings {
		if bytes.Equal(b, bad) {
			return true
		}
	}
	return false
}

// decodeHexKeys turns the literals above into bytes.
//
// A literal that does not decode to thirty-two bytes is dropped rather than
// panicked on, for the same reason decodeKey drops an undecodable slot: this
// package must never take a customer's process down. A dropped entry weakens the
// deny-list, so TestLowOrderEncodingsAreWellFormed pins the count and the size
// of every entry, and the behavioural test beside it keeps its own independent
// copy of the list — nothing here is trusted to check itself.
func decodeHexKeys(encoded ...string) [][]byte {
	keys := make([][]byte, 0, len(encoded))
	for _, e := range encoded {
		decoded, err := hex.DecodeString(e)
		if err != nil || len(decoded) != ed25519.PublicKeySize {
			continue
		}
		keys = append(keys, decoded)
	}
	return keys
}

// decodeKey turns a committed base64 public key into bytes for a trustedKeys
// slot. Standard base64 with padding is used for key material because that is
// what every key-printing tool emits, including licensegen; the envelope's
// own base64url flavour applies only to the wire format.
//
// A slot that does not decode becomes nil, which usableKeys then skips. That is
// the fail-safe direction: a typo in a committed key makes this build refuse
// every licence, which costs a paying customer a support ticket, whereas the
// alternative — an init that panics — would take a running installation down.
func decodeKey(encoded string) []byte {
	if encoded == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	return decoded
}

// HasTrustedKey reports whether this binary carries a signing key that could verify
// anything at all.
//
// It exists so that a build shipping the pubkey_prod.go placeholder can say so, once, at
// startup, instead of answering ErrNoTrustedKey to every key in existence in silence. The
// distinction matters enormously in support: "your key is wrong" and "this binary cannot
// accept any key" produce the same refusal and have completely different remedies, and only
// one of them is our fault.
//
// The answer is fixed at compile time — trustedKeys is a package variable initialised from
// constants — so callers may treat it as a build property rather than as state.
func HasTrustedKey() bool {
	return hasUsableKey(trustedKeys)
}

// hasUsableKey is HasTrustedKey with the slot set passed in, so the detection itself can be
// tested against a synthetic placeholder without the test's answer depending on which build
// tag it was compiled under.
func hasUsableKey(keys [][]byte) bool {
	return len(usableKeys(keys)) > 0
}
