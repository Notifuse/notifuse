package domain

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
)

// MapOfAny is persisted as JSON in the database
type MapOfAny map[string]any

// Scan implements the sql.Scanner interface
func (m *MapOfAny) Scan(val interface{}) error {

	var data []byte

	if b, ok := val.([]byte); ok {
		// VERY IMPORTANT: we need to clone the bytes here
		// The sql driver will reuse the same bytes RAM slots for future queries
		// Thank you St Antoine De Padoue for helping me find this bug
		data = bytes.Clone(b)
	} else if s, ok := val.(string); ok {
		data = []byte(s)
	} else if val == nil {
		return nil
	}

	return json.Unmarshal(data, m)
}

// Value implements the driver.Valuer interface
func (m MapOfAny) Value() (driver.Value, error) {
	return json.Marshal(m)
}

// JSONArray is persisted as a JSON array in the database
type JSONArray []interface{}

// Scan implements the sql.Scanner interface
func (a *JSONArray) Scan(val interface{}) error {
	var data []byte

	if b, ok := val.([]byte); ok {
		// Clone bytes to avoid reuse issues
		data = bytes.Clone(b)
	} else if s, ok := val.(string); ok {
		data = []byte(s)
	} else if val == nil {
		*a = nil
		return nil
	}

	if err := json.Unmarshal(data, a); err != nil {
		return err
	}

	// Convert float64 values that are actually integers back to int
	// This is necessary because json.Unmarshal converts all numbers to float64,
	// but PostgreSQL parameters need proper int types
	for i, v := range *a {
		if f, ok := v.(float64); ok {
			// Check if the float is actually an integer value
			if f == float64(int(f)) {
				(*a)[i] = int(f)
			}
		}
	}

	return nil
}

// Value implements the driver.Valuer interface
func (a JSONArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	return json.Marshal(a)
}
