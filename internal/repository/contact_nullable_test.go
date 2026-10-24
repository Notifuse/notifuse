package repository

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// The contact Nullable→sql.Null converters decide what actually lands in
// nullable columns: conflating "unset" (nil pointer) with "explicitly null"
// (IsNull) or with a real value would silently erase or invent contact data.

func TestContactNullableConverters(t *testing.T) {
	t.Run("string: nil and IsNull are NULL, a value survives verbatim", func(t *testing.T) {
		assert.Equal(t, "", contactToNullString(nil).String)
		assert.False(t, contactToNullString(nil).Valid)
		assert.False(t, contactToNullString(&domain.NullableString{IsNull: true}).Valid)

		got := contactToNullString(&domain.NullableString{String: "hello", IsNull: false})
		require.True(t, got.Valid)
		assert.Equal(t, "hello", got.String)

		empty := contactToNullString(&domain.NullableString{String: "", IsNull: false})
		assert.True(t, empty.Valid, "an explicitly-set empty string is not NULL")
	})

	t.Run("float: zero is a real value when not null", func(t *testing.T) {
		assert.False(t, contactToNullFloat64(nil).Valid)
		assert.False(t, contactToNullFloat64(&domain.NullableFloat64{IsNull: true}).Valid)

		got := contactToNullFloat64(&domain.NullableFloat64{Float64: 0, IsNull: false})
		require.True(t, got.Valid, "0.0 must not collapse to NULL")
		assert.Equal(t, 0.0, got.Float64)
	})

	t.Run("time: zero time is a real value when not null", func(t *testing.T) {
		assert.False(t, contactToNullTime(nil).Valid)
		assert.False(t, contactToNullTime(&domain.NullableTime{IsNull: true}).Valid)

		when := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
		got := contactToNullTime(&domain.NullableTime{Time: when, IsNull: false})
		require.True(t, got.Valid)
		assert.Equal(t, when, got.Time)
	})

	t.Run("json: marshals data, and unmarshalable data degrades to NULL", func(t *testing.T) {
		assert.False(t, contactToNullJSON(nil).Valid)
		assert.False(t, contactToNullJSON(&domain.NullableJSON{IsNull: true}).Valid)

		got := contactToNullJSON(&domain.NullableJSON{
			Data: map[string]interface{}{"tier": "gold", "score": float64(3)}, IsNull: false,
		})
		require.True(t, got.Valid)
		assert.JSONEq(t, `{"tier":"gold","score":3}`, got.String)

		bad := contactToNullJSON(&domain.NullableJSON{
			Data: map[string]interface{}{"score": math.NaN()}, IsNull: false,
		})
		assert.False(t, bad.Valid, "NaN cannot be marshaled; NULL beats aborting the write")
	})
}
