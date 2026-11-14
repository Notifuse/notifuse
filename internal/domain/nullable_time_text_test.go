package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A timestamp handed over as text, as PostgreSQL writes it or as JSON carries it, is
// read into the instant it names.
func TestNullableTime_ScanText(t *testing.T) {
	plus0530 := time.FixedZone("", 5*3600+30*60)
	cases := []struct {
		text string
		want time.Time
	}{
		{"2026-03-04T05:06:07Z", time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)},
		{"2026-03-04T05:06:07.123456+05:30", time.Date(2026, 3, 4, 5, 6, 7, 123456000, plus0530)},
		{"2026-03-04T05:06:07.123456", time.Date(2026, 3, 4, 5, 6, 7, 123456000, time.UTC)},
		{"2026-03-04 05:06:07.123456+00", time.Date(2026, 3, 4, 5, 6, 7, 123456000, time.UTC)},
		{"2026-03-04 05:06:07-07:30", time.Date(2026, 3, 4, 12, 36, 7, 0, time.UTC)},
		{"2026-03-04 05:06:07+05:30", time.Date(2026, 3, 4, 5, 6, 7, 0, plus0530)},
		{"2026-03-04 05:06:07", time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)},
		{"2026-03-04", time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		for name, input := range map[string]interface{}{"bytes": []byte(tc.text), "string": tc.text} {
			t.Run(name+" "+tc.text, func(t *testing.T) {
				nt := NullableTime{IsNull: true}
				require.NoError(t, nt.Scan(input))

				assert.False(t, nt.IsNull)
				assert.True(t, tc.want.Equal(nt.Time), "got %s, want %s", nt.Time, tc.want)
			})
		}
	}
}

// Text that names no time is refused rather than read as the zero time, and the value
// is left as it was. Values PostgreSQL can write but Go does not read (infinity, which
// a time.Time cannot hold, and BC dates, which time.Parse does not read) are refused too.
func TestNullableTime_ScanText_Refusals(t *testing.T) {
	for _, text := range []string{"invalid", "2026-13-45", "", "infinity", "2026-03-04 05:06:07 BC"} {
		t.Run(text, func(t *testing.T) {
			before := NullableTime{Time: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
			nt := before

			err := nt.Scan([]byte(text))

			require.Error(t, err)
			assert.Contains(t, err.Error(), "NullableTime")
			assert.Equal(t, before, nt)
		})
	}
}
