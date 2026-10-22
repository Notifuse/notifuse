package crypto

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrUndecryptable is returned for a stored value that none of the given keys opens.
var ErrUndecryptable = errors.New("no key opens the value")

// encryptedFieldPrefix names the JSON fields that hold a value encrypted with the
// instance secret key.
const encryptedFieldPrefix = "encrypted_"

// ReencryptValue makes a value produced by EncryptString open with current. A value
// current already opens comes back unchanged; one that a previous key opens comes back
// encrypted with current. AES-GCM authenticates, so a wrong key fails instead of
// yielding garbage, and the key that opens a value is never in doubt.
func ReencryptValue(value, current string, previous []string) (string, bool, error) {
	if _, err := DecryptFromHexString(value, current); err == nil {
		return value, false, nil
	}
	for _, key := range previous {
		plaintext, err := DecryptFromHexString(value, key)
		if err != nil {
			continue
		}
		reencrypted, err := EncryptString(plaintext, current)
		if err != nil {
			return "", false, err
		}
		return reencrypted, true, nil
	}
	return "", false, ErrUndecryptable
}

// ReencryptFields applies ReencryptValue to every non-empty string held under a key
// starting with "encrypted_", at any depth of a JSON document. With no previous key it
// only checks that current opens them all. It never decrypts into the document.
//
// The document comes back as given when nothing changed; otherwise it is re-encoded,
// numbers as written. An error names the path of the value no key opens.
func ReencryptFields(doc []byte, current string, previous []string) ([]byte, bool, error) {
	if len(bytes.TrimSpace(doc)) == 0 {
		return doc, false, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(doc))
	decoder.UseNumber()
	var tree any
	if err := decoder.Decode(&tree); err != nil {
		return nil, false, fmt.Errorf("failed to decode document: %w", err)
	}

	changed, err := reencryptNode(tree, "", current, previous)
	if err != nil {
		return nil, false, err
	}
	if !changed {
		return doc, false, nil
	}

	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(tree); err != nil {
		return nil, false, fmt.Errorf("failed to encode document: %w", err)
	}
	return bytes.TrimSuffix(out.Bytes(), []byte("\n")), true, nil
}

func reencryptNode(node any, path, current string, previous []string) (bool, error) {
	changed := false
	switch n := node.(type) {
	case map[string]any:
		// Sorted, so the path in an error does not depend on map order.
		keys := make([]string, 0, len(n))
		for k := range n {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			childPath := k
			if path != "" {
				childPath = path + "." + k
			}
			if value, ok := n[k].(string); ok && value != "" && strings.HasPrefix(k, encryptedFieldPrefix) {
				reencrypted, c, err := ReencryptValue(value, current, previous)
				if err != nil {
					return false, fmt.Errorf("%s: %w", childPath, err)
				}
				if c {
					n[k] = reencrypted
					changed = true
				}
				continue
			}
			c, err := reencryptNode(n[k], childPath, current, previous)
			if err != nil {
				return false, err
			}
			changed = changed || c
		}
	case []any:
		for i, child := range n {
			c, err := reencryptNode(child, fmt.Sprintf("%s[%d]", path, i), current, previous)
			if err != nil {
				return false, err
			}
			changed = changed || c
		}
	}
	return changed, nil
}
