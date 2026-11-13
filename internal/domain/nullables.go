package domain

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// NullableString represents a string that can be null
type NullableString struct {
	String string
	IsNull bool
}

// Value implements the driver.Valuer interface for database/sql
func (ns NullableString) Value() (driver.Value, error) {
	if ns.IsNull {
		return nil, nil
	}
	return ns.String, nil
}

// Scan implements the sql.Scanner interface for database/sql
func (ns *NullableString) Scan(value interface{}) error {
	if value == nil {
		ns.String = ""
		ns.IsNull = true
		return nil
	}

	switch v := value.(type) {
	case string:
		ns.String = v
		ns.IsNull = false
		return nil
	case []byte:
		ns.String = string(bytes.Clone(v))
		ns.IsNull = false
		return nil
	default:
		return fmt.Errorf("cannot scan %T into NullableString", value)
	}
}

// MarshalJSON implements json.Marshaler
func (ns NullableString) MarshalJSON() ([]byte, error) {
	if ns.IsNull {
		return []byte("null"), nil
	}
	return json.Marshal(ns.String)
}

// UnmarshalJSON implements json.Unmarshaler
func (ns *NullableString) UnmarshalJSON(data []byte) error {
	// Handle null case
	if string(data) == "null" {
		ns.String = ""
		ns.IsNull = true
		return nil
	}

	// Try to unmarshal as a string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		ns.String = str
		ns.IsNull = false
		return nil
	}

	// If that fails, try to unmarshal as an object
	var obj struct {
		String string `json:"String"`
		IsNull bool   `json:"IsNull"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	ns.String = obj.String
	ns.IsNull = obj.IsNull
	return nil
}

// NullableFloat64 represents a float64 that can be null
type NullableFloat64 struct {
	Float64 float64
	IsNull  bool
}

// Value implements the driver.Valuer interface for database/sql
func (nf NullableFloat64) Value() (driver.Value, error) {
	if nf.IsNull {
		return nil, nil
	}
	return nf.Float64, nil
}

// Scan implements the sql.Scanner interface for database/sql
func (nf *NullableFloat64) Scan(value interface{}) error {
	if value == nil {
		nf.Float64 = 0
		nf.IsNull = true
		return nil
	}

	switch v := value.(type) {
	case float64:
		nf.Float64 = v
		nf.IsNull = false
		return nil
	case int64:
		nf.Float64 = float64(v)
		nf.IsNull = false
		return nil
	case []byte:
		// Try to convert []byte to float64
		cloned := bytes.Clone(v)
		var f sql.NullFloat64
		if err := f.Scan(cloned); err != nil {
			return err
		}
		nf.Float64 = f.Float64
		nf.IsNull = !f.Valid
		return nil
	default:
		return fmt.Errorf("cannot scan %T into NullableFloat64", value)
	}
}

// MarshalJSON implements json.Marshaler
func (nf NullableFloat64) MarshalJSON() ([]byte, error) {
	if nf.IsNull {
		return []byte("null"), nil
	}
	return json.Marshal(nf.Float64)
}

// UnmarshalJSON implements json.Unmarshaler
func (nf *NullableFloat64) UnmarshalJSON(data []byte) error {
	// Handle null case
	if string(data) == "null" {
		nf.Float64 = 0
		nf.IsNull = true
		return nil
	}

	// Try to unmarshal as a float64 first
	var f float64
	if err := json.Unmarshal(data, &f); err == nil {
		nf.Float64 = f
		nf.IsNull = false
		return nil
	}

	// If that fails, try to unmarshal as an object
	var obj struct {
		Float64 float64 `json:"Float64"`
		IsNull  bool    `json:"IsNull"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	nf.Float64 = obj.Float64
	nf.IsNull = obj.IsNull
	return nil
}

// NullableTime represents a time.Time that can be null
type NullableTime struct {
	Time   time.Time
	IsNull bool
}

// Value implements the driver.Valuer interface for database/sql
func (nt NullableTime) Value() (driver.Value, error) {
	if nt.IsNull {
		return nil, nil
	}
	return nt.Time, nil
}

// Scan implements the sql.Scanner interface for database/sql
func (nt *NullableTime) Scan(value interface{}) error {
	if value == nil {
		nt.Time = time.Time{}
		nt.IsNull = true
		return nil
	}

	switch v := value.(type) {
	case time.Time:
		nt.Time = v
		nt.IsNull = false
		return nil
	case []byte:
		return nt.scanText(string(v))
	case string:
		return nt.scanText(v)
	default:
		return fmt.Errorf("cannot scan %T into NullableTime", value)
	}
}

// timestampTextLayouts are the forms a timestamp takes as text: RFC 3339 (JSON),
// PostgreSQL's output with its numeric zone, and both without a zone, read as UTC as
// the TIMESTAMP columns are written. Fractional seconds are optional in each.
var timestampTextLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999Z07",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02",
}

// scanText reads a timestamp given as text. Text in none of the layouts is refused,
// infinity and BC dates included: a time.Time cannot hold them.
func (nt *NullableTime) scanText(text string) error {
	for _, layout := range timestampTextLayouts {
		if t, err := time.Parse(layout, text); err == nil {
			nt.Time = t
			nt.IsNull = false
			return nil
		}
	}
	return fmt.Errorf("cannot scan %q into NullableTime: not a timestamp", text)
}

// MarshalJSON implements json.Marshaler
func (nt NullableTime) MarshalJSON() ([]byte, error) {
	if nt.IsNull {
		return []byte("null"), nil
	}
	return []byte(fmt.Sprintf(`"%s"`, nt.Time.Format(time.RFC3339))), nil
}

// UnmarshalJSON implements json.Unmarshaler
func (nt *NullableTime) UnmarshalJSON(data []byte) error {
	// Handle null case
	if string(data) == "null" {
		nt.Time = time.Time{}
		nt.IsNull = true
		return nil
	}

	// Try to unmarshal as a string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		// Try to parse the time string
		t, err := time.Parse(time.RFC3339, str)
		if err != nil {
			return err
		}
		nt.Time = t
		nt.IsNull = false
		return nil
	}

	// If that fails, try to unmarshal as an object
	var obj struct {
		Time   time.Time `json:"Time"`
		IsNull bool      `json:"IsNull"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	nt.Time = obj.Time
	nt.IsNull = obj.IsNull
	return nil
}
