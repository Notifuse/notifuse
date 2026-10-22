package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The startup re-encryption under a new SECRET_KEY and the save guard find the values
// to handle by these paths. A credential left out would stay under the old key, and
// every load of its workspace would then fail.
func TestEncryptedFieldPaths(t *testing.T) {
	settings, integrations := EncryptedFieldPaths()

	t.Run("reach the workspace key and nested credentials", func(t *testing.T) {
		assert.Contains(t, settings, "encrypted_secret_key")
		assert.Contains(t, settings, "file_manager.encrypted_secret_key")
		assert.Contains(t, integrations, "[].email_provider.smtp.encrypted_username")
		assert.Contains(t, integrations, "[].email_provider.smtp.encrypted_password")
		assert.Contains(t, integrations, "[].supabase_settings.auth_email_hook.encrypted_signature_key")
		assert.Contains(t, integrations, "[].llm_provider.anthropic.encrypted_api_key")
	})

	t.Run("never enter free-form maps or values", func(t *testing.T) {
		for _, path := range append(append([]string{}, settings...), integrations...) {
			assert.False(t, strings.HasPrefix(path, "template_blocks."), path)
			assert.False(t, strings.HasPrefix(path, "custom_field_labels."), path)
			assert.False(t, strings.Contains(path, "credential_hints"), path)
		}
	})

	// The paths never enter a map, so a credential held in one would be missed.
	t.Run("no map holds an Encrypted field", func(t *testing.T) {
		for _, root := range []reflect.Type{reflect.TypeOf(WorkspaceSettings{}), reflect.TypeOf(Integrations{})} {
			for _, m := range mapsIn(root, map[reflect.Type]bool{}) {
				assert.False(t, holdsEncryptedField(m.Elem(), map[reflect.Type]bool{}),
					"%s holds an Encrypted field in a map, which the paths do not reach", m)
			}
		}
	})

	// The paths only hold strings and never enter an interface: an Encrypted field of
	// another type, or a credential behind a new interface, would be missed.
	t.Run("every Encrypted field is a string, and the only interface a template block", func(t *testing.T) {
		var interfaces []string
		for _, root := range []reflect.Type{reflect.TypeOf(WorkspaceSettings{}), reflect.TypeOf(Integrations{})} {
			for _, f := range fieldsIn(root, root.Name(), map[reflect.Type]bool{}) {
				if strings.HasPrefix(f.field.Name, "Encrypted") {
					assert.Equal(t, reflect.String, f.field.Type.Kind(), "%s is not a string", f.path)
				}
				if elementType(f.field.Type).Kind() == reflect.Interface {
					interfaces = append(interfaces, f.path)
				}
			}
		}
		assert.Equal(t, []string{"WorkspaceSettings.TemplateBlocks.Block"}, interfaces)
	})

	// Every stored Encrypted field, set to a marker, must sit at one of the paths.
	t.Run("cover every Encrypted field a stored workspace carries", func(t *testing.T) {
		const marker = "marker-value"
		var w Workspace
		fill(reflect.ValueOf(&w).Elem(), marker)

		for name, value := range map[string]interface{}{"settings": w.Settings, "integrations": w.Integrations} {
			raw, err := json.Marshal(value)
			require.NoError(t, err)
			var tree interface{}
			require.NoError(t, json.Unmarshal(raw, &tree))

			want := settings
			if name == "integrations" {
				want = integrations
			}
			found := markerPaths(tree, nil, marker)
			require.NotEmpty(t, found, name)
			for _, found := range found {
				assert.Contains(t, want, found, "%s holds an encrypted value at %s, which the rotation would miss", name, found)
			}
		}
	})
}

// fill sets every Encrypted string field reachable from v to marker, giving each
// pointer, and each slice one element, on the way.
func fill(v reflect.Value, marker string) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		fill(v.Elem(), marker)
	case reflect.Slice:
		if v.Len() == 0 {
			v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		}
		fill(v.Index(0), marker)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if !field.IsExported() {
				continue
			}
			if strings.HasPrefix(field.Name, "Encrypted") && field.Type.Kind() == reflect.String {
				v.Field(i).SetString(marker)
				continue
			}
			fill(v.Field(i), marker)
		}
	}
}

type fieldAt struct {
	path  string
	field reflect.StructField
}

// fieldsIn returns every exported field reachable from t, with its path of Go field names.
func fieldsIn(t reflect.Type, path string, seen map[reflect.Type]bool) []fieldAt {
	t = elementType(t)
	if t.Kind() != reflect.Struct || seen[t] {
		return nil
	}
	seen[t] = true
	var fields []fieldAt
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		fieldPath := path + "." + field.Name
		fields = append(fields, fieldAt{path: fieldPath, field: field})
		fields = append(fields, fieldsIn(field.Type, fieldPath, seen)...)
	}
	return fields
}

// elementType strips pointers, slices, arrays and maps down to the type they hold.
func elementType(t reflect.Type) reflect.Type {
	for {
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			t = t.Elem()
		default:
			return t
		}
	}
}

// mapsIn returns every map type reachable from t through exported fields.
func mapsIn(t reflect.Type, seen map[reflect.Type]bool) []reflect.Type {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return mapsIn(t.Elem(), seen)
	case reflect.Map:
		return append([]reflect.Type{t}, mapsIn(t.Elem(), seen)...)
	case reflect.Struct:
		if seen[t] {
			return nil
		}
		seen[t] = true
		var maps []reflect.Type
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).IsExported() {
				maps = append(maps, mapsIn(t.Field(i).Type, seen)...)
			}
		}
		return maps
	}
	return nil
}

// holdsEncryptedField reports whether a value of type t can carry an Encrypted field.
func holdsEncryptedField(t reflect.Type, seen map[reflect.Type]bool) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return holdsEncryptedField(t.Elem(), seen)
	case reflect.Struct:
		if seen[t] {
			return false
		}
		seen[t] = true
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if strings.HasPrefix(field.Name, "Encrypted") || holdsEncryptedField(field.Type, seen) {
				return true
			}
		}
	}
	return false
}

// markerPaths returns the path of every string equal to marker in a decoded JSON tree.
func markerPaths(node interface{}, path []string, marker string) []string {
	switch n := node.(type) {
	case map[string]interface{}:
		var found []string
		for k, child := range n {
			found = append(found, markerPaths(child, append(append([]string{}, path...), k), marker)...)
		}
		return found
	case []interface{}:
		var found []string
		for _, child := range n {
			found = append(found, markerPaths(child, append(append([]string{}, path...), "[]"), marker)...)
		}
		return found
	case string:
		if n == marker {
			return []string{strings.Join(path, ".")}
		}
	}
	return nil
}
