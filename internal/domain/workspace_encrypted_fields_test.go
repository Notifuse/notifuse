package domain

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// encryptedFields walks t and returns the path and JSON name of every field whose Go
// name starts with Encrypted, through pointers, slices, arrays and maps.
func encryptedFields(t reflect.Type, path string, seen map[reflect.Type]bool, found map[string]string) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || seen[t] {
		return
	}
	seen[t] = true
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		fieldPath := path + "." + field.Name
		if strings.HasPrefix(field.Name, "Encrypted") {
			found[fieldPath] = strings.Split(field.Tag.Get("json"), ",")[0]
			continue
		}
		encryptedFields(field.Type, fieldPath, seen, found)
	}
}

// The startup re-encryption under a new SECRET_KEY finds the values to re-encrypt by
// their JSON name. A credential stored under another name would be left under the old
// key, and every load of its workspace would then fail.
func TestWorkspaceEncryptedFieldsAreNamedForTheRotation(t *testing.T) {
	found := map[string]string{}
	seen := map[reflect.Type]bool{}
	encryptedFields(reflect.TypeOf(Workspace{}), "Workspace", seen, found)
	encryptedFields(reflect.TypeOf(Integration{}), "Integration", seen, found)

	// The walk reaches nested credentials, so an empty result cannot pass silently.
	assert.Contains(t, found, "Workspace.Settings.EncryptedSecretKey")
	assert.Contains(t, found, "Workspace.Integrations.EmailProvider.SMTP.EncryptedUsername")

	for path, name := range found {
		assert.True(t, strings.HasPrefix(name, "encrypted_"),
			"%s is stored as %q: it must be named encrypted_ for the SECRET_KEY rotation to find it", path, name)
	}
}
