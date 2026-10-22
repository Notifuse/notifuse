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

// DecryptWithKeys opens a value produced by EncryptString with the first of keys that
// opens it, and says which one. AES-GCM authenticates, so a wrong key fails instead of
// yielding garbage, and the key that opens a value is never in doubt.
func DecryptWithKeys(value string, keys []string) (string, int, error) {
	for i, key := range keys {
		if plaintext, err := DecryptFromHexString(value, key); err == nil {
			return plaintext, i, nil
		}
	}
	return "", -1, ErrUndecryptable
}

// ReencryptValue makes a value produced by EncryptString open with current. A value
// current already opens comes back unchanged; one that a previous key opens comes back
// encrypted with current.
func ReencryptValue(value, current string, previous []string) (string, bool, error) {
	plaintext, index, err := DecryptWithKeys(value, append([]string{current}, previous...))
	if err != nil {
		return "", false, err
	}
	if index == 0 {
		return value, false, nil
	}
	reencrypted, err := EncryptString(plaintext, current)
	if err != nil {
		return "", false, err
	}
	return reencrypted, true, nil
}

// ReencryptFields applies ReencryptValue to the non-empty string at each of paths in a
// JSON document. A path lists its segments separated by ".", each an object key or "[]"
// for every element of an array: "[].email_provider.smtp.encrypted_password". Values
// elsewhere are left alone, whatever their key is named. With no previous key it only checks that current
// opens them all. It never decrypts into the document.
//
// The document comes back as given when nothing changed; otherwise it is re-encoded,
// numbers as written. An error names the path of the value no key opens.
func ReencryptFields(doc []byte, paths []string, current string, previous []string) ([]byte, bool, error) {
	if len(bytes.TrimSpace(doc)) == 0 {
		return doc, false, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(doc))
	decoder.UseNumber()
	var tree any
	if err := decoder.Decode(&tree); err != nil {
		return nil, false, fmt.Errorf("failed to decode document: %w", err)
	}

	changed, err := reencryptNode(tree, newPathTree(paths), "", current, previous)
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

// pathTree holds paths segment by segment; a leaf marks the end of a path.
type pathTree struct {
	children map[string]*pathTree
	leaf     bool
}

const arraySegment = "[]"

func newPathTree(paths []string) *pathTree {
	root := &pathTree{}
	for _, path := range paths {
		node := root
		for _, segment := range strings.Split(path, ".") {
			if node.children == nil {
				node.children = map[string]*pathTree{}
			}
			child, ok := node.children[segment]
			if !ok {
				child = &pathTree{}
				node.children[segment] = child
			}
			node = child
		}
		node.leaf = true
	}
	return root
}

func reencryptNode(node any, paths *pathTree, path, current string, previous []string) (bool, error) {
	changed := false
	switch n := node.(type) {
	case map[string]any:
		// Sorted, so the path in an error does not depend on map order.
		keys := make([]string, 0, len(n))
		for k := range n {
			if paths.children[k] != nil {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := paths.children[k]
			childPath := k
			if path != "" {
				childPath = path + "." + k
			}
			if value, ok := n[k].(string); ok && child.leaf {
				if value == "" {
					continue
				}
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
			c, err := reencryptNode(n[k], child, childPath, current, previous)
			if err != nil {
				return false, err
			}
			changed = changed || c
		}
	case []any:
		child := paths.children[arraySegment]
		if child == nil {
			return false, nil
		}
		for i, element := range n {
			c, err := reencryptNode(element, child, fmt.Sprintf("%s[%d]", path, i), current, previous)
			if err != nil {
				return false, err
			}
			changed = changed || c
		}
	}
	return changed, nil
}
