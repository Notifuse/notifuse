package analytics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestQuery_GetDefaultTimezone(t *testing.T) {
	tests := []struct {
		name     string
		timezone *string
		expected string
	}{
		{
			name:     "no timezone set",
			timezone: nil,
			expected: "UTC",
		},
		{
			name:     "timezone set",
			timezone: stringPtr("America/New_York"),
			expected: "America/New_York",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Query{Timezone: tt.timezone}
			assert.Equal(t, tt.expected, q.GetDefaultTimezone())
		})
	}
}

func TestQuery_HasTimeDimensions(t *testing.T) {
	tests := []struct {
		name           string
		timeDimensions []TimeDimension
		expected       bool
	}{
		{
			name:           "no time dimensions",
			timeDimensions: []TimeDimension{},
			expected:       false,
		},
		{
			name: "has time dimensions",
			timeDimensions: []TimeDimension{{
				Dimension:   "created_at",
				Granularity: "day",
			}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Query{TimeDimensions: tt.timeDimensions}
			assert.Equal(t, tt.expected, q.HasTimeDimensions())
		})
	}
}

func TestQuery_GetLimit(t *testing.T) {
	tests := []struct {
		name     string
		limit    *int
		expected int
	}{
		{
			name:     "no limit set",
			limit:    nil,
			expected: 1000,
		},
		{
			name:     "limit set",
			limit:    intPtr(50),
			expected: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Query{Limit: tt.limit}
			assert.Equal(t, tt.expected, q.GetLimit())
		})
	}
}

func TestQuery_GetOffset(t *testing.T) {
	tests := []struct {
		name     string
		offset   *int
		expected int
	}{
		{
			name:     "no offset set",
			offset:   nil,
			expected: 0,
		},
		{
			name:     "offset set",
			offset:   intPtr(100),
			expected: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Query{Offset: tt.offset}
			assert.Equal(t, tt.expected, q.GetOffset())
		})
	}
}

func TestMeta(t *testing.T) {
	meta := Meta{
		Total:         100,
		ExecutionTime: 500 * time.Millisecond,
		Query:         "SELECT COUNT(*) FROM message_history",
		Params:        []interface{}{"workspace-123"},
	}

	assert.Equal(t, 100, meta.Total)
	assert.Equal(t, 500*time.Millisecond, meta.ExecutionTime)
	assert.Equal(t, "SELECT COUNT(*) FROM message_history", meta.Query)
	assert.Equal(t, []interface{}{"workspace-123"}, meta.Params)
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}
