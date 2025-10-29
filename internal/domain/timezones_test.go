package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidTimezone(t *testing.T) {
	t.Run("Valid timezones", func(t *testing.T) {
		validTimezones := []string{
			"UTC",
			"GMT",
			"America/New_York",
			"America/Los_Angeles",
			"Europe/London",
			"Europe/Paris",
			"Asia/Tokyo",
			"Asia/Shanghai",
			"Australia/Sydney",
			"Africa/Cairo",
			"Pacific/Auckland",
		}

		for _, tz := range validTimezones {
			t.Run(tz, func(t *testing.T) {
				assert.True(t, IsValidTimezone(tz), "Timezone %s should be valid", tz)
			})
		}
	})

	t.Run("Invalid timezones", func(t *testing.T) {
		invalidTimezones := []string{
			"",
			"Invalid/Timezone",
			"NotReal/City",
			"America/FakeCity",
			"Europe/NonExistent",
			"Random_String",
			"123/456",
			"UTC+5", // This format is not in the list
		}

		for _, tz := range invalidTimezones {
			t.Run(tz, func(t *testing.T) {
				assert.False(t, IsValidTimezone(tz), "Timezone %s should be invalid", tz)
			})
		}
	})

	t.Run("Case sensitivity", func(t *testing.T) {
		// Timezone names are case-sensitive
		assert.True(t, IsValidTimezone("America/New_York"))
		assert.False(t, IsValidTimezone("america/new_york"))
		assert.False(t, IsValidTimezone("AMERICA/NEW_YORK"))

		assert.True(t, IsValidTimezone("UTC"))
		assert.False(t, IsValidTimezone("utc"))
		assert.False(t, IsValidTimezone("Utc"))
	})

	t.Run("Edge cases", func(t *testing.T) {
		// Test some edge cases
		assert.False(t, IsValidTimezone(" UTC "), "Timezone with spaces should be invalid")
		assert.False(t, IsValidTimezone("UTC "), "Timezone with trailing space should be invalid")
		assert.False(t, IsValidTimezone(" UTC"), "Timezone with leading space should be invalid")
	})

	t.Run("All timezones in list are valid", func(t *testing.T) {
		// Test that all timezones in our Timezones slice are considered valid
		// This is a sanity check to ensure the IsValidTimezone function works correctly
		for _, tz := range Timezones {
			assert.True(t, IsValidTimezone(tz), "Timezone %s from Timezones slice should be valid", tz)
		}
	})

	t.Run("Timezone list properties", func(t *testing.T) {
		// Test properties of the Timezones slice
		assert.Greater(t, len(Timezones), 0, "Timezones slice should not be empty")
		assert.Contains(t, Timezones, "UTC", "Timezones should contain UTC")

		// Check for some major timezones
		majorTimezones := []string{
			"America/New_York",
			"America/Los_Angeles",
			"Europe/London",
			"Asia/Tokyo",
		}

		for _, tz := range majorTimezones {
			assert.Contains(t, Timezones, tz, "Timezones should contain major timezone %s", tz)
		}
	})

	t.Run("Performance test with large input", func(t *testing.T) {
		// Test that the function performs reasonably with the full list
		// This is more of a sanity check than a real performance test
		for i := 0; i < 100; i++ {
			assert.True(t, IsValidTimezone("UTC"))
			assert.False(t, IsValidTimezone("Invalid/Timezone"))
		}
	})
}

func TestTimezonesList(t *testing.T) {
	t.Run("No duplicate timezones", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, tz := range Timezones {
			assert.False(t, seen[tz], "Timezone %s appears more than once in the list", tz)
			seen[tz] = true
		}
	})

	t.Run("All timezones are non-empty strings", func(t *testing.T) {
		for i, tz := range Timezones {
			assert.NotEmpty(t, tz, "Timezone at index %d should not be empty", i)
		}
	})

	t.Run("UTC is first or present", func(t *testing.T) {
		// UTC should be in the list (though not necessarily first)
		found := false
		for _, tz := range Timezones {
			if tz == "UTC" {
				found = true
				break
			}
		}
		assert.True(t, found, "UTC should be present in the timezones list")
	})
}
