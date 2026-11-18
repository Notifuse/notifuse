package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSegmentValueError_Error(t *testing.T) {
	t.Run("names the contact, the value and the field", func(t *testing.T) {
		err := &SegmentValueError{
			Column: "custom_json_1", Path: []string{"renewed_at"}, Type: SegmentValueTypeDate,
			Email: "jane@example.com", Value: "2026-02-30", Cause: `date/time field value out of range: "2026-02-30"`,
		}
		assert.Equal(t, `custom_json_1.renewed_at: contact jane@example.com holds "2026-02-30", which is not a valid date`, err.Error())
	})

	t.Run("a number", func(t *testing.T) {
		err := &SegmentValueError{Column: "custom_json_2", Path: []string{"score"}, Type: SegmentValueTypeNumber,
			Email: "joe@example.com", Value: "high"}
		assert.Equal(t, `custom_json_2.score: contact joe@example.com holds "high", which is not a valid number`, err.Error())
	})

	t.Run("a nested path is joined with dots", func(t *testing.T) {
		err := &SegmentValueError{Column: "custom_json_1", Path: []string{"orders", "0", "placed_at"},
			Type: SegmentValueTypeDate, Email: "jane@example.com", Value: "soon"}
		assert.True(t, strings.HasPrefix(err.Error(), "custom_json_1.orders.0.placed_at: "), err.Error())
	})

	t.Run("the whole column", func(t *testing.T) {
		err := &SegmentValueError{Column: "custom_json_1", Type: SegmentValueTypeNumber,
			Email: "jane@example.com", Value: `{"a":1}`}
		assert.True(t, strings.HasPrefix(err.Error(), "custom_json_1: contact"), err.Error())
	})

	t.Run("without a culprit it blames no contact", func(t *testing.T) {
		err := &SegmentValueError{Column: "custom_json_1", Path: []string{"renewed_at"}, Type: SegmentValueTypeDate,
			Cause: `date/time field value out of range: "2026-02-30"`}
		assert.Equal(t, `the segment query could not convert a value: date/time field value out of range: "2026-02-30"`, err.Error())
		assert.NotContains(t, err.Error(), "contact")
	})
}

func TestTruncateSegmentValue(t *testing.T) {
	assert.Equal(t, "2026-02-30", TruncateSegmentValue("2026-02-30"))

	exact := strings.Repeat("a", 120)
	assert.Equal(t, exact, TruncateSegmentValue(exact), "a value at the limit is kept whole")

	long := strings.Repeat("é", 200)
	cut := TruncateSegmentValue(long)
	assert.True(t, utf8.ValidString(cut), "no character is split")
	assert.Equal(t, strings.Repeat("é", 120)+"…", cut)
}

func TestSegmentValueError_MarshalJSON(t *testing.T) {
	raw, err := json.Marshal(&SegmentValueError{Column: "custom_json_1", Path: []string{"renewed_at"},
		Type: SegmentValueTypeDate, Email: "jane@example.com", Value: "2026-02-30", Cause: "kept out"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"column":"custom_json_1","path":["renewed_at"],"type":"date","email":"jane@example.com","value":"2026-02-30"}`, string(raw))

	raw, err = json.Marshal(&SegmentValueError{Cause: "x"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"column":"","path":[],"type":"","email":"","value":""}`, string(raw))
}

func TestSegmentValueError_LastError(t *testing.T) {
	valueErr := &SegmentValueError{Column: "custom_json_1", Path: []string{"renewed_at"}, Type: SegmentValueTypeDate,
		Email: "jane@example.com", Value: "2026-02-30", Cause: "out of range"}
	lastErr := valueErr.LastError(SegmentLastErrorSourceContactQueue)
	assert.Equal(t, &SegmentLastError{
		Kind: SegmentLastErrorKindValueConversion, Column: "custom_json_1", Path: []string{"renewed_at"},
		Type: SegmentValueTypeDate, Email: "jane@example.com", Value: "2026-02-30", Cause: "out of range",
		Source: SegmentLastErrorSourceContactQueue,
	}, lastErr)
	lastErr.Path[0] = "changed"
	assert.Equal(t, "renewed_at", valueErr.Path[0], "the record has a path of its own")
}

func TestSegmentLastError_JSONRoundTrip(t *testing.T) {
	original := &SegmentLastError{
		Kind: SegmentLastErrorKindValueConversion, Column: "custom_json_1", Path: []string{"orders", "0", "total"},
		Type: SegmentValueTypeNumber, Email: "jane@example.com", Value: "high",
		Cause: `invalid input syntax for type numeric: "high"`, Source: SegmentLastErrorSourceBuild,
	}
	raw, err := json.Marshal(original)
	require.NoError(t, err)
	assert.JSONEq(t, `{"kind":"value_conversion","column":"custom_json_1","path":["orders","0","total"],"type":"number","email":"jane@example.com","value":"high","cause":"invalid input syntax for type numeric: \"high\"","source":"build"}`, string(raw))

	var decoded SegmentLastError
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, *original, decoded)

	other, err := json.Marshal(NewOtherSegmentLastError(SegmentLastErrorSourceBuild, "boom"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"kind":"other","column":"","path":[],"type":"","email":"","value":"","cause":"boom","source":"build"}`, string(other))

	nilPath, err := json.Marshal(SegmentLastError{Kind: SegmentLastErrorKindOther})
	require.NoError(t, err)
	assert.Contains(t, string(nilPath), `"path":[]`)
}
