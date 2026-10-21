package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// The SECRET_KEY value compose.yaml shipped as its default before 42.0, recognised
// by digest so this file carries no copy of it: the SHA-256 of the string, and of
// the 64 bytes it base64-decodes to. The second catches the same key written another
// way (unpadded, URL alphabet, wrapped, raw), all of which derive the same JWT secret.
const (
	formerComposeDefaultDigest      = "1a792822151257d08abb381ce0a056f9525c48f87ecd6eda5124e4be116958d1"
	formerComposeDefaultBytesDigest = "c7bb0efe1226741284609648323b4a4e824ce34a5f2467ab359fec0632c0bf61"
)

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// isFormerComposeDefault reports whether key is the SECRET_KEY value compose.yaml
// used to ship as its default, in any of the spellings that decode to it.
func isFormerComposeDefault(key string) bool {
	trimmed := strings.TrimSpace(key)
	if sha256Hex([]byte(trimmed)) == formerComposeDefaultDigest {
		return true
	}

	candidates := [][]byte{[]byte(key), []byte(trimmed)}
	compact := strings.TrimRight(strings.Join(strings.Fields(trimmed), ""), "=")
	for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
		if decoded, err := enc.DecodeString(compact); err == nil {
			candidates = append(candidates, decoded)
		}
	}
	for _, c := range candidates {
		if sha256Hex(c) == formerComposeDefaultBytesDigest {
			return true
		}
	}
	return false
}

// resolveSigningKey returns the key the installation signs with: SIGNING_KEY,
// trimmed, when it is set, else secretKey unchanged, so an installation that never
// set SIGNING_KEY keeps every signature it has issued. It refuses the former compose
// default, which is public.
func resolveSigningKey(secretKey, signingKeyEnv string) (string, error) {
	if signingKey := strings.TrimSpace(signingKeyEnv); signingKey != "" {
		if isFormerComposeDefault(signingKey) {
			return "", fmt.Errorf("SIGNING_KEY is the SECRET_KEY value compose.yaml used to ship as its default, " +
				"which Notifuse no longer accepts for signing. Generate a new one with: openssl rand -base64 32")
		}
		return signingKey, nil
	}

	if isFormerComposeDefault(secretKey) {
		return "", fmt.Errorf("SIGNING_KEY must be set: SECRET_KEY is the value compose.yaml used to ship as its default, " +
			"which Notifuse no longer accepts for signing. Generate a signing key with: openssl rand -base64 32, " +
			"set it as SIGNING_KEY, and keep SECRET_KEY unchanged because it decrypts your stored data. " +
			"Users will have to sign in again, and API keys must be recreated")
	}
	return secretKey, nil
}

// deriveJWTSecret turns the signing key into the HMAC secret for JWTs: its decoded
// bytes when it is base64 of at least 32 bytes (PASETO_PRIVATE_KEY compatibility),
// else its raw bytes.
func deriveJWTSecret(signingKey string) []byte {
	decoded, err := base64.StdEncoding.DecodeString(signingKey)
	if err == nil && len(decoded) >= 32 {
		return decoded
	}
	return []byte(signingKey)
}
