package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// Key values Notifuse itself has published, recognised by digest so this file
// carries no copy of them:
//   - the SECRET_KEY compose.yaml provided by default before 42.0, by the SHA-256 of
//     the string and of the 64 bytes it base64-decodes to. The second catches the
//     same key written another way: unpadded, in the URL alphabet, wrapped over
//     lines, or as its raw bytes;
//   - the SECRET_KEY placeholder env.example carried before 42.0.
const (
	formerComposeDefaultDigest        = "1a792822151257d08abb381ce0a056f9525c48f87ecd6eda5124e4be116958d1"
	formerComposeDefaultBytesDigest   = "c7bb0efe1226741284609648323b4a4e824ce34a5f2467ab359fec0632c0bf61"
	formerEnvExamplePlaceholderDigest = "55a2aa619c852bbc2a756b83696b60ab1b91ab7163f3e0d8463da815fb93a268"
)

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// isPublishedKey reports whether key is one of the key values Notifuse has
// published, in any of the spellings that decode to it.
func isPublishedKey(key string) bool {
	trimmed := strings.TrimSpace(key)
	switch sha256Hex([]byte(trimmed)) {
	case formerComposeDefaultDigest, formerEnvExamplePlaceholderDigest:
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
// set SIGNING_KEY keeps every signature it has issued. It refuses a published key,
// which anyone can sign with.
func resolveSigningKey(secretKey, signingKeyEnv string) (string, error) {
	if signingKey := strings.TrimSpace(signingKeyEnv); signingKey != "" {
		if isPublishedKey(signingKey) {
			return "", fmt.Errorf("SIGNING_KEY is a value published with Notifuse (the former compose.yaml " +
				"default SECRET_KEY or the env.example placeholder), which Notifuse does not accept for signing. " +
				"Generate a new one with: openssl rand -base64 32")
		}
		return signingKey, nil
	}

	if isPublishedKey(secretKey) {
		return "", fmt.Errorf("SIGNING_KEY must be set: SECRET_KEY is a value published with Notifuse (the former " +
			"compose.yaml default or the env.example placeholder), which Notifuse does not accept for signing. " +
			"Generate a signing key with: openssl rand -base64 32, set it as SIGNING_KEY, and keep SECRET_KEY " +
			"unchanged because it decrypts your stored data. Users will have to sign in again, API keys " +
			"must be recreated, and pending invitations sent again")
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
