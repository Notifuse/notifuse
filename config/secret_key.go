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
// published, in any of the spellings that decode to it, quoted or not: an env file
// read by docker --env-file keeps the quotes, so a quoted published key is just as
// easy to sign with.
func isPublishedKey(key string) bool {
	return isPublishedValue(key) || isPublishedValue(unquoted(key))
}

// unquoted returns value trimmed and, when it is wrapped in one pair of matching
// quotes, without them, trimmed again.
func unquoted(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 && (trimmed[0] == '"' || trimmed[0] == '\'') && trimmed[len(trimmed)-1] == trimmed[0] {
		return strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	return trimmed
}

func isPublishedValue(key string) bool {
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

// minSecretKeyBytes is the key material the key that signs must give: what openssl
// rand -base64 32, the documented way to make one, does.
const minSecretKeyBytes = 32

// secretKeys is what SECRET_KEY (or PASETO_PRIVATE_KEY) holds: current encrypts and
// signs; previous only decrypts, so a token signed with an old key, the published one
// included, is refused. isList records that the value had a comma.
type secretKeys struct {
	current  string
	previous []string
	isList   bool
}

// parseSecretKeys reads the value of variable. Without a comma it is one key, byte
// for byte as before 42.0. With one, it is a list: one pair of quotes around the whole
// value is removed (docker --env-file keeps them, and no stored data depends on a list),
// a comma separates two keys, two commas stand for a comma inside a key, and spaces
// around each key are trimmed. Each previous key is also tried as written, spaces
// included, and with and without one pair of quotes: a key read from a file may end
// with a newline, a single key read through docker --env-file kept its quotes, and the
// data was encrypted with them.
//
// It refuses a first key anyone can sign with: a published one, or one giving less key
// material than a generated key, as the documentation's placeholders do.
func parseSecretKeys(raw, variable string) (secretKeys, error) {
	if !strings.Contains(raw, ",") {
		return secretKeys{current: raw}, checkCurrentKey(raw, variable, false)
	}

	items := splitKeyList(unquoted(raw))
	// The keys exactly as written: the spaces and quotes around the whole value belong
	// to the first and last of them. unquoted removes no comma, so both have the same
	// number of keys.
	written := splitKeyList(raw)
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			return secretKeys{}, fmt.Errorf("%s lists an empty key. A comma separates two keys; write a comma "+
				"inside a key twice", variable)
		}
	}
	current := strings.TrimSpace(items[0])
	if strings.ContainsAny(current[:1], `"'`) || strings.ContainsAny(current[len(current)-1:], `"'`) {
		return secretKeys{}, fmt.Errorf("the first key of %s starts or ends with a quote. Quote the whole "+
			"list or none of it", variable)
	}
	if err := checkCurrentKey(current, variable, true); err != nil {
		return secretKeys{}, err
	}

	keys := secretKeys{current: current, isList: true}
	seen := map[string]bool{current: true}
	for i := 1; i < len(items); i++ {
		bare := unquoted(items[i])
		for _, candidate := range []string{
			bare, strings.TrimSpace(items[i]), items[i], written[i], `"` + bare + `"`, "'" + bare + "'",
		} {
			if !seen[candidate] {
				seen[candidate] = true
				keys.previous = append(keys.previous, candidate)
			}
		}
	}
	return keys, nil
}

// NewSecurityConfig reads the value of variable, SECRET_KEY or PASETO_PRIVATE_KEY, into
// the keys the server signs, encrypts and decrypts with, or the error it refuses the
// value with.
func NewSecurityConfig(raw, variable string) (SecurityConfig, error) {
	keys, err := parseSecretKeys(raw, variable)
	if err != nil {
		return SecurityConfig{}, err
	}
	return SecurityConfig{
		JWTSecret:          deriveJWTSecret(keys.current),
		SecretKey:          keys.current,
		PreviousSecretKeys: keys.previous,
		SecretKeyIsList:    keys.isList,
		KeyVariable:        variable,
	}, nil
}

// DecryptionKeys returns every key a stored value may be encrypted with, the current
// one first.
func (s SecurityConfig) DecryptionKeys() []string {
	return append([]string{s.SecretKey}, s.PreviousSecretKeys...)
}

// splitKeyList splits a list on single commas, keeping the spaces around each key; two
// commas in a row are a comma inside a key.
func splitKeyList(list string) []string {
	var items []string
	var item strings.Builder
	for i := 0; i < len(list); i++ {
		if list[i] != ',' {
			item.WriteByte(list[i])
			continue
		}
		if i+1 < len(list) && list[i+1] == ',' {
			item.WriteByte(',')
			i++
			continue
		}
		items = append(items, item.String())
		item.Reset()
	}
	return append(items, item.String())
}

// checkCurrentKey refuses a key that would encrypt and sign while anyone can sign
// with it, and says how to rotate to a new one without losing the stored data.
func checkCurrentKey(key, variable string, isList bool) error {
	subject := variable
	if isList {
		subject = "the first key of " + variable
	}
	if isPublishedKey(key) {
		return fmt.Errorf("%s is a value published with Notifuse (the former compose.yaml default or the "+
			"env.example placeholder), which anyone can sign with. %s", subject, rotationRemedy(variable))
	}
	if len(deriveJWTSecret(key)) < minSecretKeyBytes {
		return fmt.Errorf("%s is too short to sign with: it must give at least %d bytes of key material. %s",
			subject, minSecretKeyBytes, rotationRemedy(variable))
	}
	return nil
}

// rotationRemedy tells an installation how to move to a new key: the new one first,
// the current one after it, which Notifuse uses to re-encrypt the stored data.
func rotationRemedy(variable string) string {
	return fmt.Sprintf("Generate a new key with: openssl rand -base64 32, and put it first, keeping your "+
		"current key after a comma: %[1]s=<new key>,<current key> (write each comma inside a key twice). "+
		"Stop every instance and start them all with that value: Notifuse re-encrypts your stored data "+
		"with the new key when it starts. Once they all run, set the new key alone in %[1]s, without "+
		"quotes or spaces around it. Users will have to sign in again, API keys must be deleted and "+
		"created again, and pending invitations sent again", variable)
}

// StoredDataRemedy tells an installation whose key opens none of its stored data what
// to set, including the case the upgrade to 42.0 makes common: an installation that
// never set the variable ran on the former compose.yaml default.
func StoredDataRemedy(variable string) string {
	return fmt.Sprintf("Set %[1]s to the key the data is encrypted with, written as it was, or put a new key "+
		"first and that key after a comma to replace it. An installation that never set %[1]s ran on the "+
		"former compose.yaml default, which the installation guide gives", variable)
}

// KeyHint says what to change when a single key opens none of the stored data but
// another form of it does, completing "<variable> opens none of it, but ": the key
// without the quotes or spaces around it, or, for a key without any, the key in quotes,
// which docker --env-file kept before 42.0. opens tries a form. Empty when none opens.
func KeyHint(key, variable string, opens func(candidate string) bool) string {
	if trimmed := unquoted(key); trimmed != key {
		if opens(trimmed) {
			return fmt.Sprintf("does without the quotes or spaces around it: remove them from %s", variable)
		}
		return ""
	}
	// Listed after itself, the key is also tried in quotes, so a start with the key twice
	// re-encrypts the data under it as written.
	for _, quoted := range []string{`"` + key + `"`, "'" + key + "'"} {
		if opens(quoted) {
			return fmt.Sprintf("the same key in quotes does: the data was encrypted with the quotes a tool "+
				"kept around it. Set %[1]s=<key>,<key>, the key twice, and start every instance with it: "+
				"Notifuse re-encrypts the data with the key as written. Once they all run, set the key alone "+
				"again", variable)
		}
	}
	return ""
}

// deriveJWTSecret turns the key that signs into the HMAC secret for JWTs: its decoded
// bytes when it is base64 of at least 32 bytes (PASETO_PRIVATE_KEY compatibility),
// else its raw bytes.
func deriveJWTSecret(secretKey string) []byte {
	decoded, err := base64.StdEncoding.DecodeString(secretKey)
	if err == nil && len(decoded) >= 32 {
		return decoded
	}
	return []byte(secretKey)
}
