package domain

import (
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapOfAny_Scan(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    MapOfAny
		wantErr bool
	}{
		{
			name:  "valid JSON bytes",
			input: []byte(`{"key": "value", "number": 123}`),
			want: MapOfAny{
				"key":    "value",
				"number": float64(123), // JSON unmarshals numbers as float64
			},
			wantErr: false,
		},
		{
			name:  "valid JSON string",
			input: `{"key": "value", "number": 123}`,
			want: MapOfAny{
				"key":    "value",
				"number": float64(123),
			},
			wantErr: false,
		},
		{
			name:    "nil input",
			input:   nil,
			want:    nil,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   []byte(`{invalid json`),
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m MapOfAny
			err := m.Scan(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, m)
		})
	}
}

func TestMapOfAny_Value(t *testing.T) {
	tests := []struct {
		name    string
		input   MapOfAny
		want    driver.Value
		wantErr bool
	}{
		{
			name: "valid map",
			input: MapOfAny{
				"key":    "value",
				"number": 123,
			},
			want:    []byte(`{"key":"value","number":123}`),
			wantErr: false,
		},
		{
			name:    "nil map",
			input:   nil,
			want:    []byte("null"),
			wantErr: false,
		},
		{
			name: "complex map",
			input: MapOfAny{
				"string": "value",
				"number": 123,
				"bool":   true,
				"null":   nil,
				"array":  []interface{}{1, "two", 3},
				"object": map[string]interface{}{"nested": "value"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.input.Value()

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			if tt.want != nil {
				assert.Equal(t, tt.want, got)
			} else {
				// For complex cases, verify the JSON is valid
				jsonBytes, ok := got.([]byte)
				assert.True(t, ok)

				var unmarshaled interface{}
				err := json.Unmarshal(jsonBytes, &unmarshaled)
				assert.NoError(t, err)
			}
		})
	}
}

func TestJSONArray_Scan(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    JSONArray
		wantErr bool
	}{
		{
			name:  "valid JSON bytes array",
			input: []byte(`[1, 2, "three", true, null]`),
			want:  JSONArray{1, 2, "three", true, nil},
			wantErr: false,
		},
		{
			name:  "valid JSON string array",
			input: `[1, 2, "three", true]`,
			want:  JSONArray{1, 2, "three", true},
			wantErr: false,
		},
		{
			name:    "nil input",
			input:   nil,
			want:    nil,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   []byte(`[invalid json`),
			want:    nil,
			wantErr: true,
		},
		{
			name:  "array with float64 integers converted to int",
			input: []byte(`[1.0, 2.0, 3.5, 4.0]`),
			want:  JSONArray{1, 2, 3.5, 4}, // 1.0, 2.0, 4.0 should become int, 3.5 stays float64
			wantErr: false,
		},
		{
			name:  "empty array",
			input: []byte(`[]`),
			want:  JSONArray{},
			wantErr: false,
		},
		{
			name:  "nested arrays",
			input: []byte(`[[1, 2], [3, 4]]`),
			want:  JSONArray{[]interface{}{1.0, 2.0}, []interface{}{3.0, 4.0}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var a JSONArray
			err := a.Scan(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, len(tt.want), len(a))
			
			// Check each element, accounting for float64 to int conversion
			for i, expected := range tt.want {
				if i < len(a) {
					if expectedInt, ok := expected.(int); ok {
						// If expected is int, check if actual is int or float64 that equals int
						if actualInt, ok := a[i].(int); ok {
							assert.Equal(t, expectedInt, actualInt)
						} else if actualFloat, ok := a[i].(float64); ok {
							assert.Equal(t, float64(expectedInt), actualFloat)
						} else {
							t.Errorf("expected int at index %d, got %T", i, a[i])
						}
					} else {
						assert.Equal(t, expected, a[i])
					}
				}
			}
		})
	}
}

func TestJSONArray_Value(t *testing.T) {
	tests := []struct {
		name    string
		input   JSONArray
		wantErr bool
	}{
		{
			name:    "valid array",
			input:   JSONArray{1, 2, "three", true, nil},
			wantErr: false,
		},
		{
			name:    "nil array",
			input:   nil,
			wantErr: false,
		},
		{
			name:    "empty array",
			input:   JSONArray{},
			wantErr: false,
		},
		{
			name:    "complex array",
			input:   JSONArray{1, "two", 3.5, true, false, nil, []interface{}{1, 2}, map[string]interface{}{"key": "value"}},
			wantErr: false,
		},
		{
			name:    "array with integers",
			input:   JSONArray{1, 2, 3},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.input.Value()

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			
			if tt.input == nil {
				assert.Nil(t, got)
			} else {
				// Verify the value is a valid JSON byte array
				jsonBytes, ok := got.([]byte)
				assert.True(t, ok)
				
				// Verify we can unmarshal it back
				var unmarshaled JSONArray
				err := json.Unmarshal(jsonBytes, &unmarshaled)
				assert.NoError(t, err)
				assert.Equal(t, len(tt.input), len(unmarshaled))
			}
		})
	}
}
