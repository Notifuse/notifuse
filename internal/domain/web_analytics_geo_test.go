package domain

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Coordinate precision must respect the place-name toggles.
//
// The geo settings offer three separate switches — store region, store city, and
// a coordinate precision — and it is natural to read them as independent. They
// are not: a coordinate IS a place name, just expressed differently. At the
// default precision of 2, which the console itself labels "City level (~1km)", a
// workspace that switched "Store city" off was still storing a city-accurate
// coordinate on every session and every goal. The setting looked honoured and was
// not.
//
// The rule this encodes: a coordinate is never more precise than the finest place
// name the workspace agreed to store.
//
//	city stored   -> up to 2 decimals (~1 km)
//	region only   -> up to 1 decimal  (~11 km)
//	neither       -> 0 decimals       (~111 km, country-scale)
//
// A clamp rather than a hard gate, deliberately: blanking the coordinates would
// empty the Live map, and an empty Live map reads as "nobody is online" — a more
// confusing bug than a coarse pin.

func TestWebAnalyticsSettings_EffectiveGeoCoordsPrecision(t *testing.T) {
	cases := []struct {
		storeRegion bool
		storeCity   bool
		configured  int
		want        int
		why         string
	}{
		// City stored: the configured value stands, whatever it is.
		{true, true, 2, 2, "city is stored, so a city-accurate coordinate reveals nothing new"},
		{true, true, 1, 1, "a workspace asking for less precision still gets less"},
		{true, true, 0, 0, ""},
		{false, true, 2, 2, "city without region is still city-level"},

		// Region only: a coordinate must not be finer than the region.
		{true, false, 2, 1, "storing a city-accurate coordinate would defeat switching city off"},
		{true, false, 1, 1, ""},
		{true, false, 0, 0, "the configured value is a ceiling too, never raised"},

		// Neither: country is all that was agreed to.
		{false, false, 2, 0, "neither name is stored, so the coordinate must not name a place"},
		{false, false, 1, 0, ""},
		{false, false, 0, 0, ""},
	}

	for _, tc := range cases {
		name := fmt.Sprintf("region=%v/city=%v/configured=%d", tc.storeRegion, tc.storeCity, tc.configured)
		t.Run(name, func(t *testing.T) {
			s := &WebAnalyticsSettings{
				GeoEnabled:         true,
				GeoStoreRegion:     tc.storeRegion,
				GeoStoreCity:       tc.storeCity,
				GeoCoordsPrecision: tc.configured,
			}
			assert.Equal(t, tc.want, s.EffectiveGeoCoordsPrecision(), tc.why)
		})
	}
}

// Absent settings mean "everything on", matching what applyWebGeo already does
// when it has no settings to consult.
func TestWebAnalyticsSettings_EffectiveGeoCoordsPrecisionNilReceiver(t *testing.T) {
	var s *WebAnalyticsSettings
	assert.Equal(t, 2, s.EffectiveGeoCoordsPrecision())
}

// Validate bounds the stored value to 0-2, but the settings can also arrive from
// an older row or a hand-written API call.
func TestWebAnalyticsSettings_EffectiveGeoCoordsPrecisionBoundsJunk(t *testing.T) {
	full := func(configured int) *WebAnalyticsSettings {
		return &WebAnalyticsSettings{
			GeoEnabled: true, GeoStoreRegion: true, GeoStoreCity: true,
			GeoCoordsPrecision: configured,
		}
	}

	assert.Equal(t, 0, full(-5).EffectiveGeoCoordsPrecision(), "a negative precision is not more precise")
	assert.Equal(t, 2, full(9).EffectiveGeoCoordsPrecision(), "and 9 decimals is a street address")
}

// The clamp is a ceiling on precision, never a floor: turning a place name off
// can only ever make coordinates coarser.
func TestWebAnalyticsSettings_EffectiveGeoCoordsPrecisionNeverIncreases(t *testing.T) {
	for _, configured := range []int{0, 1, 2} {
		for _, region := range []bool{false, true} {
			for _, city := range []bool{false, true} {
				s := &WebAnalyticsSettings{
					GeoEnabled: true, GeoStoreRegion: region, GeoStoreCity: city,
					GeoCoordsPrecision: configured,
				}
				assert.LessOrEqual(t, s.EffectiveGeoCoordsPrecision(), configured,
					"region=%v city=%v configured=%d produced a finer coordinate than asked for",
					region, city, configured)
			}
		}
	}
}
