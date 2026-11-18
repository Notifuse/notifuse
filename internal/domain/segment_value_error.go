package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The types a segment converts a custom JSON value to before comparing it.
const (
	SegmentValueTypeDate   = "date"
	SegmentValueTypeNumber = "number"
)

// The kinds of error kept on a segment.
const (
	// SegmentLastErrorKindValueConversion is a contact value the segment query could not convert.
	SegmentLastErrorKindValueConversion = "value_conversion"
	// SegmentLastErrorKindOther is any other failure of the last computation.
	SegmentLastErrorKindOther = "other"
)

// The computations that record an error on a segment.
const (
	SegmentLastErrorSourceBuild        = "build"
	SegmentLastErrorSourceContactQueue = "contact_queue"
)

// segmentValueMaxRunes is the length past which a contact value is cut in an error.
const segmentValueMaxRunes = 120

// TruncateSegmentValue cuts a contact value to segmentValueMaxRunes characters, ending it with
// « … » when it was longer. It counts characters, not bytes, so a multi-byte character is never
// split.
func TruncateSegmentValue(value string) string {
	runes := []rune(value)
	if len(runes) <= segmentValueMaxRunes {
		return value
	}
	return string(runes[:segmentValueMaxRunes]) + "…"
}

// SegmentValueError is a segment query that failed because a value could not be converted to the
// type a filter compares it as. With a culprit, it names the contact and the value; without one
// (no contact could be pointed at), it carries only the database's own message, and Column, Path
// and Type are empty when more than one filter could have raised.
type SegmentValueError struct {
	Column string
	Path   []string
	Type   string
	Email  string
	Value  string
	Cause  string
}

// HasCulprit reports whether the error names the contact holding the value.
func (e *SegmentValueError) HasCulprit() bool {
	return e.Email != ""
}

// Field is the column and the path inside it, joined with dots: custom_json_1.renewed_at.
func (e *SegmentValueError) Field() string {
	return segmentErrorField(e.Column, e.Path)
}

// Error is the sentence logs and API clients read. Without a culprit it makes no claim that a
// contact value is at fault.
func (e *SegmentValueError) Error() string {
	if !e.HasCulprit() {
		return "the segment query could not convert a value: " + e.Cause
	}
	what := "valid value"
	switch e.Type {
	case SegmentValueTypeDate:
		what = "valid date"
	case SegmentValueTypeNumber:
		what = "valid number"
	}
	return fmt.Sprintf("%s: contact %s holds %q, which is not a %s", e.Field(), e.Email, e.Value, what)
}

// segmentValueErrorJSON is the part of a SegmentValueError an API client receives: the cause is
// the database's message, which the sentence already carries.
type segmentValueErrorJSON struct {
	Column string   `json:"column"`
	Path   []string `json:"path"`
	Type   string   `json:"type"`
	Email  string   `json:"email"`
	Value  string   `json:"value"`
}

// MarshalJSON writes the fields a client composes its own sentence from. Path is always an array.
func (e *SegmentValueError) MarshalJSON() ([]byte, error) {
	path := e.Path
	if path == nil {
		path = []string{}
	}
	return json.Marshal(segmentValueErrorJSON{
		Column: e.Column,
		Path:   path,
		Type:   e.Type,
		Email:  e.Email,
		Value:  e.Value,
	})
}

// LastError is the record a segment keeps of this error, for the computation that met it.
func (e *SegmentValueError) LastError(source string) *SegmentLastError {
	path := append([]string{}, e.Path...)
	return &SegmentLastError{
		Kind:   SegmentLastErrorKindValueConversion,
		Column: e.Column,
		Path:   path,
		Type:   e.Type,
		Email:  e.Email,
		Value:  e.Value,
		Cause:  e.Cause,
		Source: source,
	}
}

// SegmentLastError is the last failure of a segment's computation, kept on the segment (as JSONB)
// so that a failure nobody watched happen is still visible.
type SegmentLastError struct {
	Kind   string   `json:"kind"`
	Column string   `json:"column"`
	Path   []string `json:"path"`
	Type   string   `json:"type"`
	Email  string   `json:"email"`
	Value  string   `json:"value"`
	Cause  string   `json:"cause"`
	Source string   `json:"source"`
}

// NewOtherSegmentLastError records a failure that is not a value conversion.
func NewOtherSegmentLastError(source, cause string) *SegmentLastError {
	return &SegmentLastError{
		Kind:   SegmentLastErrorKindOther,
		Path:   []string{},
		Cause:  cause,
		Source: source,
	}
}

// MarshalJSON writes Path as an array even when it is empty.
func (e SegmentLastError) MarshalJSON() ([]byte, error) {
	type plain SegmentLastError
	if e.Path == nil {
		e.Path = []string{}
	}
	return json.Marshal(plain(e))
}

// segmentErrorField joins a column and a path inside it with dots.
func segmentErrorField(column string, path []string) string {
	if column == "" {
		return ""
	}
	if len(path) == 0 {
		return column
	}
	return column + "." + strings.Join(path, ".")
}
