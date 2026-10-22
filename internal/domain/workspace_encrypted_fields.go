package domain

import (
	"reflect"
	"strings"
	"sync"
)

var (
	encryptedFieldPathsOnce sync.Once
	settingsEncryptedPaths  []string
	integrationsEncrypted   []string
)

// EncryptedFieldPaths returns where the settings and the integrations of a workspace, as
// stored, hold values encrypted with the instance secret key: the JSON path of every
// field whose name starts with Encrypted, in the form crypto.ReencryptFields reads.
// Derived from the types, so a credential added to them is included, and free-form maps
// and values are never taken for one.
func EncryptedFieldPaths() (settings []string, integrations []string) {
	encryptedFieldPathsOnce.Do(func() {
		settingsEncryptedPaths = encryptedPaths(reflect.TypeOf(WorkspaceSettings{}), nil, map[reflect.Type]bool{})
		integrationsEncrypted = encryptedPaths(reflect.TypeOf(Integrations{}), nil, map[reflect.Type]bool{})
	})
	return settingsEncryptedPaths, integrationsEncrypted
}

func encryptedPaths(t reflect.Type, prefix []string, visiting map[reflect.Type]bool) []string {
	switch t.Kind() {
	case reflect.Pointer:
		return encryptedPaths(t.Elem(), prefix, visiting)
	case reflect.Slice, reflect.Array:
		return encryptedPaths(t.Elem(), append(prefix, "[]"), visiting)
	case reflect.Struct:
	default:
		// Maps and interfaces hold free-form content, never a credential.
		return nil
	}
	if visiting[t] {
		return nil
	}
	visiting[t] = true
	defer delete(visiting, t)

	var paths []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if field.Anonymous && name == "" {
			paths = append(paths, encryptedPaths(field.Type, prefix, visiting)...)
			continue
		}
		if name == "" {
			name = field.Name
		}
		path := append(append([]string{}, prefix...), name)
		if strings.HasPrefix(field.Name, "Encrypted") && field.Type.Kind() == reflect.String {
			paths = append(paths, strings.Join(path, "."))
			continue
		}
		paths = append(paths, encryptedPaths(field.Type, path, visiting)...)
	}
	return paths
}
