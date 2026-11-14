package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The nullables also decode from their struct form, {"String": …, "IsNull": …}, which
// is how a value marshalled without the custom marshaller looks.
func TestNullables_UnmarshalJSON_ObjectForm(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		var ns NullableString
		require.NoError(t, json.Unmarshal([]byte(`{"String": "hello", "IsNull": false}`), &ns))
		assert.Equal(t, NullableString{String: "hello"}, ns)

		require.Error(t, json.Unmarshal([]byte(`[1]`), &ns))
	})

	t.Run("float", func(t *testing.T) {
		var nf NullableFloat64
		require.NoError(t, json.Unmarshal([]byte(`{"Float64": 1.5, "IsNull": false}`), &nf))
		assert.Equal(t, NullableFloat64{Float64: 1.5}, nf)

		require.Error(t, json.Unmarshal([]byte(`[1]`), &nf))
	})

	t.Run("time", func(t *testing.T) {
		var nt NullableTime
		require.NoError(t, json.Unmarshal([]byte(`{"Time": "2026-01-02T03:04:05Z", "IsNull": true}`), &nt))
		assert.True(t, nt.Time.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
		assert.True(t, nt.IsNull)

		require.Error(t, json.Unmarshal([]byte(`[1]`), &nt))
	})
}

// A float column read as text that is not a number is refused.
func TestNullableFloat64_Scan_RefusesNonNumericBytes(t *testing.T) {
	var nf NullableFloat64
	assert.Error(t, nf.Scan([]byte("abc")))
}
