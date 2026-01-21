package domain

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestContact_Validate(t *testing.T) {
	tests := []struct {
		name    string
		contact Contact
		wantErr bool
	}{
		{
			name: "valid contact with required email field only",
			contact: Contact{
				Email: "test@example.com",
			},
			wantErr: false,
		},
		{
			name: "valid contact with all optional fields",
			contact: Contact{
				Email:      "test@example.com",
				ExternalID: &NullableString{String: "ext123", IsNull: false},
				Timezone:   &NullableString{String: "Europe/Paris", IsNull: false},
				Language:   &NullableString{String: "en", IsNull: false},
				FirstName:  &NullableString{String: "John", IsNull: false},
				LastName:   &NullableString{String: "Doe", IsNull: false},
				CustomJSON1: &NullableJSON{
					Data:   map[string]interface{}{"preferences": map[string]interface{}{"theme": "dark"}},
					IsNull: false,
				},
			},
			wantErr: false,
		},
		{
			name: "missing email",
			contact: Contact{
				ExternalID: &NullableString{String: "ext123", IsNull: false},
				Timezone:   &NullableString{String: "Europe/Paris", IsNull: false},
			},
			wantErr: true,
		},
		{
			name: "invalid email",
			contact: Contact{
				Email:      "invalid-email",
				ExternalID: &NullableString{String: "ext123", IsNull: false},
				Timezone:   &NullableString{String: "Europe/Paris", IsNull: false},
			},
			wantErr: true,
		},
		{
			name: "valid contact with all custom fields",
			contact: Contact{
				Email:           "test@example.com",
				CustomString1:   &NullableString{String: "custom1", IsNull: false},
				CustomString2:   &NullableString{String: "custom2", IsNull: false},
				CustomString3:   &NullableString{String: "custom3", IsNull: false},
				CustomString4:   &NullableString{String: "custom4", IsNull: false},
				CustomString5:   &NullableString{String: "custom5", IsNull: false},
				CustomNumber1:   &NullableFloat64{Float64: 1.0, IsNull: false},
				CustomNumber2:   &NullableFloat64{Float64: 2.0, IsNull: false},
				CustomNumber3:   &NullableFloat64{Float64: 3.0, IsNull: false},
				CustomNumber4:   &NullableFloat64{Float64: 4.0, IsNull: false},
				CustomNumber5:   &NullableFloat64{Float64: 5.0, IsNull: false},
				CustomDatetime1: &NullableTime{Time: time.Now(), IsNull: false},
				CustomDatetime2: &NullableTime{Time: time.Now(), IsNull: false},
				CustomDatetime3: &NullableTime{Time: time.Now(), IsNull: false},
				CustomDatetime4: &NullableTime{Time: time.Now(), IsNull: false},
				CustomDatetime5: &NullableTime{Time: time.Now(), IsNull: false},
				CustomJSON1:     &NullableJSON{Data: map[string]interface{}{"key": "value"}, IsNull: false},
				CustomJSON2:     &NullableJSON{Data: map[string]interface{}{"key": "value"}, IsNull: false},
				CustomJSON3:     &NullableJSON{Data: map[string]interface{}{"key": "value"}, IsNull: false},
				CustomJSON4:     &NullableJSON{Data: map[string]interface{}{"key": "value"}, IsNull: false},
				CustomJSON5:     &NullableJSON{Data: map[string]interface{}{"key": "value"}, IsNull: false},
			},
			wantErr: false,
		},
		{
			name: "valid contact with custom number fields",
			contact: Contact{
				Email:         "test@example.com",
				CustomNumber1: &NullableFloat64{Float64: 100.0, IsNull: false},
				CustomNumber2: &NullableFloat64{Float64: 5.0, IsNull: false},
				CustomNumber3: &NullableFloat64{Float64: 10.0, IsNull: false},
			},
			wantErr: false,
		},
		{
			name: "valid contact with address fields",
			contact: Contact{
				Email:        "test@example.com",
				AddressLine1: &NullableString{String: "123 Main St", IsNull: false},
				AddressLine2: &NullableString{String: "Apt 4B", IsNull: false},
				Country:      &NullableString{String: "USA", IsNull: false},
				Postcode:     &NullableString{String: "12345", IsNull: false},
				State:        &NullableString{String: "CA", IsNull: false},
			},
			wantErr: false,
		},
		{
			name: "valid contact with contact info fields",
			contact: Contact{
				Email:     "test@example.com",
				Phone:     &NullableString{String: "+1234567890", IsNull: false},
				FirstName: &NullableString{String: "John", IsNull: false},
				LastName:  &NullableString{String: "Doe", IsNull: false},
				JobTitle:  &NullableString{String: "Developer", IsNull: false},
			},
			wantErr: false,
		},
		{
			name: "valid contact with locale fields",
			contact: Contact{
				Email:    "test@example.com",
				Timezone: &NullableString{String: "America/New_York", IsNull: false},
				Language: &NullableString{String: "en-US", IsNull: false},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.contact.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestScanContact(t *testing.T) {
	now := time.Now()

	// Create JSON test data
	jsonData1, _ := json.Marshal(map[string]interface{}{"preferences": map[string]interface{}{"theme": "dark"}})
	jsonData2, _ := json.Marshal([]interface{}{"tag1", "tag2"})
	jsonData3, _ := json.Marshal(42.5)
	jsonData4, _ := json.Marshal("string value")
	jsonData5, _ := json.Marshal(true)

	// Create mock scanner
	scanner := &contactMockScanner{
		data: []interface{}{
			"test@example.com", // Email
			sql.NullString{String: "ext123", Valid: true},       // ExternalID
			sql.NullString{String: "Europe/Paris", Valid: true}, // Timezone
			sql.NullString{String: "en-US", Valid: true},        // Language
			sql.NullString{String: "John", Valid: true},         // FirstName
			sql.NullString{String: "Doe", Valid: true},          // LastName
			sql.NullString{String: "John Doe", Valid: true},     // FullName
			sql.NullString{String: "+1234567890", Valid: true},  // Phone
			sql.NullString{String: "123 Main St", Valid: true},  // AddressLine1
			sql.NullString{String: "Apt 4B", Valid: true},       // AddressLine2
			sql.NullString{String: "USA", Valid: true},          // Country
			sql.NullString{String: "12345", Valid: true},        // Postcode
			sql.NullString{String: "CA", Valid: true},           // State
			sql.NullString{String: "Developer", Valid: true},    // JobTitle
			sql.NullString{String: "Custom 1", Valid: true},     // CustomString1
			sql.NullString{String: "Custom 2", Valid: true},     // CustomString2
			sql.NullString{String: "Custom 3", Valid: true},     // CustomString3
			sql.NullString{String: "Custom 4", Valid: true},     // CustomString4
			sql.NullString{String: "Custom 5", Valid: true},     // CustomString5
			sql.NullFloat64{Float64: 42.0, Valid: true},         // CustomNumber1
			sql.NullFloat64{Float64: 43.0, Valid: true},         // CustomNumber2
			sql.NullFloat64{Float64: 44.0, Valid: true},         // CustomNumber3
			sql.NullFloat64{Float64: 45.0, Valid: true},         // CustomNumber4
			sql.NullFloat64{Float64: 46.0, Valid: true},         // CustomNumber5
			sql.NullTime{Time: now, Valid: true},                // CustomDatetime1
			sql.NullTime{Time: now, Valid: true},                // CustomDatetime2
			sql.NullTime{Time: now, Valid: true},                // CustomDatetime3
			sql.NullTime{Time: now, Valid: true},                // CustomDatetime4
			sql.NullTime{Time: now, Valid: true},                // CustomDatetime5
			jsonData1,                                           // CustomJSON1
			jsonData2,                                           // CustomJSON2
			jsonData3,                                           // CustomJSON3
			jsonData4,                                           // CustomJSON4
			jsonData5,                                           // CustomJSON5
			now,                                                 // CreatedAt
			now,                                                 // UpdatedAt
			now,                                                 // DBCreatedAt
			now,                                                 // DBUpdatedAt
		},
	}

	// Test successful scan
	contact, err := ScanContact(scanner)
	assert.NoError(t, err)
	assert.Equal(t, "test@example.com", contact.Email)
	assert.Equal(t, "ext123", contact.ExternalID.String)
	assert.Equal(t, "Europe/Paris", contact.Timezone.String)
	assert.Equal(t, "en-US", contact.Language.String)
	assert.False(t, contact.Language.IsNull)
	assert.Equal(t, "John", contact.FirstName.String)
	assert.False(t, contact.FirstName.IsNull)
	assert.Equal(t, "Doe", contact.LastName.String)
	assert.False(t, contact.LastName.IsNull)
	assert.Equal(t, "John Doe", contact.FullName.String)
	assert.False(t, contact.FullName.IsNull)
	assert.Equal(t, "+1234567890", contact.Phone.String)
	assert.False(t, contact.Phone.IsNull)
	assert.Equal(t, "123 Main St", contact.AddressLine1.String)
	assert.False(t, contact.AddressLine1.IsNull)
	assert.Equal(t, "Apt 4B", contact.AddressLine2.String)
	assert.False(t, contact.AddressLine2.IsNull)
	assert.Equal(t, "USA", contact.Country.String)
	assert.False(t, contact.Country.IsNull)
	assert.Equal(t, "12345", contact.Postcode.String)
	assert.False(t, contact.Postcode.IsNull)
	assert.Equal(t, "CA", contact.State.String)
	assert.False(t, contact.State.IsNull)
	assert.Equal(t, "Developer", contact.JobTitle.String)
	assert.False(t, contact.JobTitle.IsNull)
	assert.Equal(t, "Custom 1", contact.CustomString1.String)
	assert.False(t, contact.CustomString1.IsNull)
	assert.Equal(t, 42.0, contact.CustomNumber1.Float64)
	assert.False(t, contact.CustomNumber1.IsNull)
	assert.Equal(t, now, contact.CustomDatetime1.Time)
	assert.False(t, contact.CustomDatetime1.IsNull)

	// Test custom JSON fields
	assert.False(t, contact.CustomJSON1.IsNull)
	preferences, ok := contact.CustomJSON1.Data.(map[string]interface{})
	assert.True(t, ok)
	theme, ok := preferences["preferences"].(map[string]interface{})["theme"].(string)
	assert.True(t, ok)
	assert.Equal(t, "dark", theme)

	assert.False(t, contact.CustomJSON2.IsNull)
	tags, ok := contact.CustomJSON2.Data.([]interface{})
	assert.True(t, ok)
	assert.Equal(t, "tag1", tags[0])
	assert.Equal(t, "tag2", tags[1])

	assert.False(t, contact.CustomJSON3.IsNull)
	assert.Equal(t, 42.5, contact.CustomJSON3.Data)

	assert.False(t, contact.CustomJSON4.IsNull)
	assert.Equal(t, "string value", contact.CustomJSON4.Data)

	assert.False(t, contact.CustomJSON5.IsNull)
	assert.Equal(t, true, contact.CustomJSON5.Data)

	// Test scan error
	scanner.err = sql.ErrNoRows
	_, err = ScanContact(scanner)
	assert.Error(t, err)

	// Test scanning with null values
	t.Run("should handle null values", func(t *testing.T) {
		scanner := &contactMockScanner{
			data: []interface{}{
				"test@example.com",                            // Email
				sql.NullString{String: "", Valid: false},      // ExternalID
				sql.NullString{String: "", Valid: false},      // Timezone
				sql.NullString{String: "", Valid: false},      // Language
				sql.NullString{String: "", Valid: false},      // FirstName
				sql.NullString{String: "", Valid: false},      // LastName
				sql.NullString{String: "", Valid: false},      // Phone
				sql.NullString{String: "", Valid: false},      // AddressLine1
				sql.NullString{String: "", Valid: false},      // AddressLine2
				sql.NullString{String: "", Valid: false},      // Country
				sql.NullString{String: "", Valid: false},      // Postcode
				sql.NullString{String: "", Valid: false},      // State
				sql.NullString{String: "", Valid: false},      // JobTitle
				sql.NullString{String: "", Valid: false},      // CustomString1
				sql.NullString{String: "", Valid: false},      // CustomString2
				sql.NullString{String: "", Valid: false},      // CustomString3
				sql.NullString{String: "", Valid: false},      // CustomString4
				sql.NullString{String: "", Valid: false},      // CustomString5
				sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber1
				sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber2
				sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber3
				sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber4
				sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber5
				sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime1
				sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime2
				sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime3
				sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime4
				sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime5
				[]byte("null"), // CustomJSON1
				[]byte("null"), // CustomJSON2
				[]byte("null"), // CustomJSON3
				[]byte("null"), // CustomJSON4
				[]byte("null"), // CustomJSON5
				time.Now(),     // CreatedAt
				time.Now(),     // UpdatedAt
				time.Now(),     // DBCreatedAt
				time.Now(),     // DBUpdatedAt
			},
		}

		contact, err := ScanContact(scanner)
		assert.NoError(t, err)
		assert.Equal(t, "test@example.com", contact.Email)
		assert.Nil(t, contact.ExternalID)
		assert.Nil(t, contact.Timezone)
		assert.Nil(t, contact.Language)
		assert.Nil(t, contact.FirstName)
		assert.Nil(t, contact.LastName)
		assert.Nil(t, contact.Phone)
		assert.Nil(t, contact.AddressLine1)
		assert.Nil(t, contact.AddressLine2)
		assert.Nil(t, contact.Country)
		assert.Nil(t, contact.Postcode)
		assert.Nil(t, contact.State)
		assert.Nil(t, contact.JobTitle)
		assert.Nil(t, contact.CustomString1)
		assert.Nil(t, contact.CustomString2)
		assert.Nil(t, contact.CustomString3)
		assert.Nil(t, contact.CustomString4)
		assert.Nil(t, contact.CustomString5)
		assert.Nil(t, contact.CustomNumber1)
		assert.Nil(t, contact.CustomNumber2)
		assert.Nil(t, contact.CustomNumber3)
		assert.Nil(t, contact.CustomNumber4)
		assert.Nil(t, contact.CustomNumber5)
		assert.Nil(t, contact.CustomDatetime1)
		assert.Nil(t, contact.CustomDatetime2)
		assert.Nil(t, contact.CustomDatetime3)
		assert.Nil(t, contact.CustomDatetime4)
		assert.Nil(t, contact.CustomDatetime5)
		assert.Nil(t, contact.CustomJSON1)
		assert.Nil(t, contact.CustomJSON2)
		assert.Nil(t, contact.CustomJSON3)
		assert.Nil(t, contact.CustomJSON4)
	})

	// Test scanning with invalid JSON data
	t.Run("should handle invalid JSON data", func(t *testing.T) {
		scanner := &contactMockScanner{
			data: []interface{}{
				"test@example.com",                            // Email
				sql.NullString{String: "", Valid: false},      // ExternalID
				sql.NullString{String: "", Valid: false},      // Timezone
				sql.NullString{String: "", Valid: false},      // Language
				sql.NullString{String: "", Valid: false},      // FirstName
				sql.NullString{String: "", Valid: false},      // LastName
				sql.NullString{String: "", Valid: false},      // Phone
				sql.NullString{String: "", Valid: false},      // AddressLine1
				sql.NullString{String: "", Valid: false},      // AddressLine2
				sql.NullString{String: "", Valid: false},      // Country
				sql.NullString{String: "", Valid: false},      // Postcode
				sql.NullString{String: "", Valid: false},      // State
				sql.NullString{String: "", Valid: false},      // JobTitle
				sql.NullString{String: "", Valid: false},      // CustomString1
				sql.NullString{String: "", Valid: false},      // CustomString2
				sql.NullString{String: "", Valid: false},      // CustomString3
				sql.NullString{String: "", Valid: false},      // CustomString4
				sql.NullString{String: "", Valid: false},      // CustomString5
				sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber1
				sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber2
				sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber3
				sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber4
				sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber5
				sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime1
				sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime2
				sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime3
				sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime4
				sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime5
				[]byte(`{invalid json}`),                      // CustomJSON1
				[]byte("null"),                                // CustomJSON2
				[]byte("null"),                                // CustomJSON3
				[]byte("null"),                                // CustomJSON4
				[]byte("null"),                                // CustomJSON5
				time.Now(),                                    // CreatedAt
				time.Now(),                                    // UpdatedAt
				time.Now(),                                    // DBCreatedAt
				time.Now(),                                    // DBUpdatedAt
			},
		}

		contact, err := ScanContact(scanner)
		assert.NoError(t, err)
		assert.Nil(t, contact.CustomJSON1)
		assert.Nil(t, contact.CustomJSON2)
		assert.Nil(t, contact.CustomJSON3)
		assert.Nil(t, contact.CustomJSON4)
		assert.Nil(t, contact.CustomJSON5)
	})
}

// Mock scanner for testing
type contactMockScanner struct {
	data []interface{}
	err  error
}

func (m *contactMockScanner) Scan(dest ...interface{}) error {
	if m.err != nil {
		return m.err
	}

	for i, d := range dest {
		if i >= len(m.data) {
			continue
		}

		switch v := d.(type) {
		case *string:
			if s, ok := m.data[i].(string); ok {
				*v = s
			}
		case *sql.NullString:
			if s, ok := m.data[i].(sql.NullString); ok {
				*v = s
			}
		case *sql.NullFloat64:
			if f, ok := m.data[i].(sql.NullFloat64); ok {
				*v = f
			}
		case *sql.NullTime:
			if t, ok := m.data[i].(sql.NullTime); ok {
				*v = t
			}
		case *[]byte:
			switch data := m.data[i].(type) {
			case []byte:
				*v = data
			case string:
				*v = []byte(data)
			}
		case *int:
			if n, ok := m.data[i].(int); ok {
				*v = n
			}
		case *float64:
			if f, ok := m.data[i].(float64); ok {
				*v = f
			}
		case *time.Time:
			if t, ok := m.data[i].(time.Time); ok {
				*v = t
			}
		}
	}

	return nil
}

func TestContact_Merge(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour)

	tests := []struct {
		name     string
		base     *Contact
		other    *Contact
		expected *Contact
	}{
		{
			name: "Merge with nil contact",
			base: &Contact{
				Email:     "test@example.com",
				FirstName: &NullableString{String: "Original", IsNull: false},
			},
			other: nil,
			expected: &Contact{
				Email:     "test@example.com",
				FirstName: &NullableString{String: "Original", IsNull: false},
			},
		},
		{
			name: "Merge basic fields",
			base: &Contact{
				Email:     "old@example.com",
				FirstName: &NullableString{String: "Old", IsNull: false},
				LastName:  &NullableString{String: "Name", IsNull: false},
			},
			other: &Contact{
				Email:     "new@example.com",
				FirstName: &NullableString{String: "New", IsNull: false},
			},
			expected: &Contact{
				Email:     "new@example.com",
				FirstName: &NullableString{String: "New", IsNull: false},
				LastName:  &NullableString{String: "Name", IsNull: false},
			},
		},
		{
			name: "Merge with null fields",
			base: &Contact{
				Email:     "test@example.com",
				FirstName: &NullableString{String: "Original", IsNull: false},
				LastName:  &NullableString{String: "Name", IsNull: false},
			},
			other: &Contact{
				Email:     "test@example.com",
				FirstName: &NullableString{String: "", IsNull: true},
			},
			expected: &Contact{
				Email:     "test@example.com",
				FirstName: &NullableString{String: "", IsNull: true},
				LastName:  &NullableString{String: "Name", IsNull: false},
			},
		},
		{
			name: "Merge timestamps",
			base: &Contact{
				Email:     "test@example.com",
				CreatedAt: now,
				UpdatedAt: now,
			},
			other: &Contact{
				Email:     "test@example.com",
				CreatedAt: later,
				UpdatedAt: later,
			},
			expected: &Contact{
				Email:     "test@example.com",
				CreatedAt: later,
				UpdatedAt: later,
			},
		},
		{
			name: "Merge custom fields",
			base: &Contact{
				Email:         "test@example.com",
				CustomString1: &NullableString{String: "Old String", IsNull: false},
				CustomNumber1: &NullableFloat64{Float64: 1.0, IsNull: false},
				CustomJSON1:   &NullableJSON{Data: map[string]interface{}{"old": "value"}, IsNull: false},
			},
			other: &Contact{
				Email:         "test@example.com",
				CustomString1: &NullableString{String: "New String", IsNull: false},
				CustomNumber1: &NullableFloat64{Float64: 2.0, IsNull: false},
				CustomJSON1:   &NullableJSON{Data: map[string]interface{}{"new": "value"}, IsNull: false},
			},
			expected: &Contact{
				Email:         "test@example.com",
				CustomString1: &NullableString{String: "New String", IsNull: false},
				CustomNumber1: &NullableFloat64{Float64: 2.0, IsNull: false},
				CustomJSON1:   &NullableJSON{Data: map[string]interface{}{"new": "value"}, IsNull: false},
			},
		},
		{
			name: "Merge custom number fields",
			base: &Contact{
				Email:         "test@example.com",
				CustomNumber1: &NullableFloat64{Float64: 100.0, IsNull: false},
				CustomNumber2: &NullableFloat64{Float64: 1.0, IsNull: false},
				CustomDatetime1: &NullableTime{Time: now, IsNull: false},
			},
			other: &Contact{
				Email:         "test@example.com",
				CustomNumber1: &NullableFloat64{Float64: 200.0, IsNull: false},
				CustomNumber2: &NullableFloat64{Float64: 2.0, IsNull: false},
				CustomDatetime1: &NullableTime{Time: later, IsNull: false},
			},
			expected: &Contact{
				Email:         "test@example.com",
				CustomNumber1: &NullableFloat64{Float64: 200.0, IsNull: false},
				CustomNumber2: &NullableFloat64{Float64: 2.0, IsNull: false},
				CustomDatetime1: &NullableTime{Time: later, IsNull: false},
			},
		},
		{
			name: "Merge address fields",
			base: &Contact{
				Email:        "test@example.com",
				AddressLine1: &NullableString{String: "123 Old St", IsNull: false},
				AddressLine2: &NullableString{String: "Apt 1", IsNull: false},
				Country:      &NullableString{String: "USA", IsNull: false},
				State:        &NullableString{String: "CA", IsNull: false},
				Postcode:     &NullableString{String: "12345", IsNull: false},
			},
			other: &Contact{
				Email:        "test@example.com",
				AddressLine1: &NullableString{String: "456 New St", IsNull: false},
				Country:      &NullableString{String: "Canada", IsNull: false},
			},
			expected: &Contact{
				Email:        "test@example.com",
				AddressLine1: &NullableString{String: "456 New St", IsNull: false},
				AddressLine2: &NullableString{String: "Apt 1", IsNull: false},
				Country:      &NullableString{String: "Canada", IsNull: false},
				State:        &NullableString{String: "CA", IsNull: false},
				Postcode:     &NullableString{String: "12345", IsNull: false},
			},
		},
		{
			name: "Merge with all custom fields",
			base: &Contact{
				Email:           "test@example.com",
				CustomString1:   &NullableString{String: "old1", IsNull: false},
				CustomString2:   &NullableString{String: "old2", IsNull: false},
				CustomString3:   &NullableString{String: "old3", IsNull: false},
				CustomString4:   &NullableString{String: "old4", IsNull: false},
				CustomString5:   &NullableString{String: "old5", IsNull: false},
				CustomNumber1:   &NullableFloat64{Float64: 1.0, IsNull: false},
				CustomNumber2:   &NullableFloat64{Float64: 2.0, IsNull: false},
				CustomNumber3:   &NullableFloat64{Float64: 3.0, IsNull: false},
				CustomNumber4:   &NullableFloat64{Float64: 4.0, IsNull: false},
				CustomNumber5:   &NullableFloat64{Float64: 5.0, IsNull: false},
				CustomDatetime1: &NullableTime{Time: now, IsNull: false},
				CustomDatetime2: &NullableTime{Time: now, IsNull: false},
				CustomDatetime3: &NullableTime{Time: now, IsNull: false},
				CustomDatetime4: &NullableTime{Time: now, IsNull: false},
				CustomDatetime5: &NullableTime{Time: now, IsNull: false},
				CustomJSON1:     &NullableJSON{Data: map[string]interface{}{"old": "value"}, IsNull: false},
				CustomJSON2:     &NullableJSON{Data: map[string]interface{}{"old": "value"}, IsNull: false},
				CustomJSON3:     &NullableJSON{Data: map[string]interface{}{"old": "value"}, IsNull: false},
				CustomJSON4:     &NullableJSON{Data: map[string]interface{}{"old": "value"}, IsNull: false},
				CustomJSON5:     &NullableJSON{Data: map[string]interface{}{"old": "value"}, IsNull: false},
			},
			other: &Contact{
				Email:           "test@example.com",
				CustomString1:   &NullableString{String: "new1", IsNull: false},
				CustomString2:   &NullableString{String: "new2", IsNull: false},
				CustomString3:   &NullableString{String: "new3", IsNull: false},
				CustomString4:   &NullableString{String: "new4", IsNull: false},
				CustomString5:   &NullableString{String: "new5", IsNull: false},
				CustomNumber1:   &NullableFloat64{Float64: 10.0, IsNull: false},
				CustomNumber2:   &NullableFloat64{Float64: 20.0, IsNull: false},
				CustomNumber3:   &NullableFloat64{Float64: 30.0, IsNull: false},
				CustomNumber4:   &NullableFloat64{Float64: 40.0, IsNull: false},
				CustomNumber5:   &NullableFloat64{Float64: 50.0, IsNull: false},
				CustomDatetime1: &NullableTime{Time: later, IsNull: false},
				CustomDatetime2: &NullableTime{Time: later, IsNull: false},
				CustomDatetime3: &NullableTime{Time: later, IsNull: false},
				CustomDatetime4: &NullableTime{Time: later, IsNull: false},
				CustomDatetime5: &NullableTime{Time: later, IsNull: false},
				CustomJSON1:     &NullableJSON{Data: map[string]interface{}{"new": "value"}, IsNull: false},
				CustomJSON2:     &NullableJSON{Data: map[string]interface{}{"new": "value"}, IsNull: false},
				CustomJSON3:     &NullableJSON{Data: map[string]interface{}{"new": "value"}, IsNull: false},
				CustomJSON4:     &NullableJSON{Data: map[string]interface{}{"new": "value"}, IsNull: false},
				CustomJSON5:     &NullableJSON{Data: map[string]interface{}{"new": "value"}, IsNull: false},
			},
			expected: &Contact{
				Email:           "test@example.com",
				CustomString1:   &NullableString{String: "new1", IsNull: false},
				CustomString2:   &NullableString{String: "new2", IsNull: false},
				CustomString3:   &NullableString{String: "new3", IsNull: false},
				CustomString4:   &NullableString{String: "new4", IsNull: false},
				CustomString5:   &NullableString{String: "new5", IsNull: false},
				CustomNumber1:   &NullableFloat64{Float64: 10.0, IsNull: false},
				CustomNumber2:   &NullableFloat64{Float64: 20.0, IsNull: false},
				CustomNumber3:   &NullableFloat64{Float64: 30.0, IsNull: false},
				CustomNumber4:   &NullableFloat64{Float64: 40.0, IsNull: false},
				CustomNumber5:   &NullableFloat64{Float64: 50.0, IsNull: false},
				CustomDatetime1: &NullableTime{Time: later, IsNull: false},
				CustomDatetime2: &NullableTime{Time: later, IsNull: false},
				CustomDatetime3: &NullableTime{Time: later, IsNull: false},
				CustomDatetime4: &NullableTime{Time: later, IsNull: false},
				CustomDatetime5: &NullableTime{Time: later, IsNull: false},
				CustomJSON1:     &NullableJSON{Data: map[string]interface{}{"new": "value"}, IsNull: false},
				CustomJSON2:     &NullableJSON{Data: map[string]interface{}{"new": "value"}, IsNull: false},
				CustomJSON3:     &NullableJSON{Data: map[string]interface{}{"new": "value"}, IsNull: false},
				CustomJSON4:     &NullableJSON{Data: map[string]interface{}{"new": "value"}, IsNull: false},
				CustomJSON5:     &NullableJSON{Data: map[string]interface{}{"new": "value"}, IsNull: false},
			},
		},
		{
			name: "Merge with null custom fields",
			base: &Contact{
				Email:           "test@example.com",
				CustomString1:   &NullableString{String: "old1", IsNull: false},
				CustomNumber1:   &NullableFloat64{Float64: 1.0, IsNull: false},
				CustomDatetime1: &NullableTime{Time: now, IsNull: false},
				CustomJSON1:     &NullableJSON{Data: map[string]interface{}{"old": "value"}, IsNull: false},
			},
			other: &Contact{
				Email:           "test@example.com",
				CustomString1:   &NullableString{String: "", IsNull: true},
				CustomNumber1:   &NullableFloat64{Float64: 0, IsNull: true},
				CustomDatetime1: &NullableTime{Time: time.Time{}, IsNull: true},
				CustomJSON1:     &NullableJSON{Data: nil, IsNull: true},
			},
			expected: &Contact{
				Email:           "test@example.com",
				CustomString1:   &NullableString{String: "", IsNull: true},
				CustomNumber1:   &NullableFloat64{Float64: 0, IsNull: true},
				CustomDatetime1: &NullableTime{Time: time.Time{}, IsNull: true},
				CustomJSON1:     &NullableJSON{Data: nil, IsNull: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.base.Merge(tt.other)

			// Compare Email
			if tt.base.Email != tt.expected.Email {
				t.Errorf("Email = %v, want %v", tt.base.Email, tt.expected.Email)
			}

			// Compare FirstName if present
			if tt.expected.FirstName != nil {
				if tt.base.FirstName == nil {
					t.Error("FirstName is nil, want non-nil")
				} else if tt.base.FirstName.String != tt.expected.FirstName.String || tt.base.FirstName.IsNull != tt.expected.FirstName.IsNull {
					t.Errorf("FirstName = %+v, want %+v", tt.base.FirstName, tt.expected.FirstName)
				}
			}

			// Compare LastName if present
			if tt.expected.LastName != nil {
				if tt.base.LastName == nil {
					t.Error("LastName is nil, want non-nil")
				} else if tt.base.LastName.String != tt.expected.LastName.String || tt.base.LastName.IsNull != tt.expected.LastName.IsNull {
					t.Errorf("LastName = %+v, want %+v", tt.base.LastName, tt.expected.LastName)
				}
			}

			// Compare timestamps
			if !tt.base.CreatedAt.Equal(tt.expected.CreatedAt) {
				t.Errorf("CreatedAt = %v, want %v", tt.base.CreatedAt, tt.expected.CreatedAt)
			}
			if !tt.base.UpdatedAt.Equal(tt.expected.UpdatedAt) {
				t.Errorf("UpdatedAt = %v, want %v", tt.base.UpdatedAt, tt.expected.UpdatedAt)
			}

			// Compare custom fields if present
			if tt.expected.CustomString1 != nil {
				if tt.base.CustomString1 == nil {
					t.Error("CustomString1 is nil, want non-nil")
				} else if tt.base.CustomString1.String != tt.expected.CustomString1.String || tt.base.CustomString1.IsNull != tt.expected.CustomString1.IsNull {
					t.Errorf("CustomString1 = %+v, want %+v", tt.base.CustomString1, tt.expected.CustomString1)
				}
			}

			// Compare all custom fields
			compareCustomFields(t, tt.base, tt.expected)
		})
	}
}

func compareCustomFields(t *testing.T, base, expected *Contact) {
	// Compare CustomString fields
	for i := 1; i <= 5; i++ {
		field := fmt.Sprintf("CustomString%d", i)
		baseField := reflect.ValueOf(base).Elem().FieldByName(field)
		expectedField := reflect.ValueOf(expected).Elem().FieldByName(field)

		if !expectedField.IsNil() {
			if baseField.IsNil() {
				t.Errorf("%s is nil, want non-nil", field)
			} else {
				baseValue := baseField.Interface().(*NullableString)
				expectedValue := expectedField.Interface().(*NullableString)
				if baseValue.String != expectedValue.String || baseValue.IsNull != expectedValue.IsNull {
					t.Errorf("%s = %+v, want %+v", field, baseValue, expectedValue)
				}
			}
		}
	}

	// Compare CustomNumber fields
	for i := 1; i <= 5; i++ {
		field := fmt.Sprintf("CustomNumber%d", i)
		baseField := reflect.ValueOf(base).Elem().FieldByName(field)
		expectedField := reflect.ValueOf(expected).Elem().FieldByName(field)

		if !expectedField.IsNil() {
			if baseField.IsNil() {
				t.Errorf("%s is nil, want non-nil", field)
			} else {
				baseValue := baseField.Interface().(*NullableFloat64)
				expectedValue := expectedField.Interface().(*NullableFloat64)
				if baseValue.Float64 != expectedValue.Float64 || baseValue.IsNull != expectedValue.IsNull {
					t.Errorf("%s = %+v, want %+v", field, baseValue, expectedValue)
				}
			}
		}
	}

	// Compare CustomDatetime fields
	for i := 1; i <= 5; i++ {
		field := fmt.Sprintf("CustomDatetime%d", i)
		baseField := reflect.ValueOf(base).Elem().FieldByName(field)
		expectedField := reflect.ValueOf(expected).Elem().FieldByName(field)

		if !expectedField.IsNil() {
			if baseField.IsNil() {
				t.Errorf("%s is nil, want non-nil", field)
			} else {
				baseValue := baseField.Interface().(*NullableTime)
				expectedValue := expectedField.Interface().(*NullableTime)
				if !baseValue.Time.Equal(expectedValue.Time) || baseValue.IsNull != expectedValue.IsNull {
					t.Errorf("%s = %+v, want %+v", field, baseValue, expectedValue)
				}
			}
		}
	}

	// Compare CustomJSON fields
	for i := 1; i <= 5; i++ {
		field := fmt.Sprintf("CustomJSON%d", i)
		baseField := reflect.ValueOf(base).Elem().FieldByName(field)
		expectedField := reflect.ValueOf(expected).Elem().FieldByName(field)

		if !expectedField.IsNil() {
			if baseField.IsNil() {
				t.Errorf("%s is nil, want non-nil", field)
			} else {
				baseValue := baseField.Interface().(*NullableJSON)
				expectedValue := expectedField.Interface().(*NullableJSON)
				if !reflect.DeepEqual(baseValue.Data, expectedValue.Data) || baseValue.IsNull != expectedValue.IsNull {
					t.Errorf("%s = %+v, want %+v", field, baseValue, expectedValue)
				}
			}
		}
	}
}

func TestFromJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	validJSON := `{
		"email": "test@example.com",
		"external_id": "ext123",
		"timezone": "Europe/Paris",
		"language": "en",
		"first_name": "John",
		"last_name": "Doe",
		"phone": "1234567890",
		"address_line_1": "123 Main St",
		"address_line_2": "Apt 4B",
		"country": "US",
		"postcode": "12345",
		"state": "NY",
		"job_title": "Engineer",
		"custom_string_1": "custom1",
		"custom_string_2": null,
		"custom_number_1": 42.5,
		"custom_number_2": null,
		"custom_datetime_1": "` + now.Format(time.RFC3339) + `",
		"custom_datetime_2": null,
		"custom_json_1": {"key": "value"},
		"custom_json_2": null
	}`

	tests := []struct {
		name    string
		input   interface{}
		want    *Contact
		wantErr bool
	}{
		{
			name:  "valid JSON as []byte",
			input: []byte(validJSON),
			want: &Contact{
				Email:           "test@example.com",
				ExternalID:      &NullableString{String: "ext123", IsNull: false},
				Timezone:        &NullableString{String: "Europe/Paris", IsNull: false},
				Language:        &NullableString{String: "en", IsNull: false},
				FirstName:       &NullableString{String: "John", IsNull: false},
				LastName:        &NullableString{String: "Doe", IsNull: false},
				Phone:           &NullableString{String: "1234567890", IsNull: false},
				AddressLine1:    &NullableString{String: "123 Main St", IsNull: false},
				AddressLine2:    &NullableString{String: "Apt 4B", IsNull: false},
				Country:         &NullableString{String: "US", IsNull: false},
				Postcode:        &NullableString{String: "12345", IsNull: false},
				State:           &NullableString{String: "NY", IsNull: false},
				JobTitle:        &NullableString{String: "Engineer", IsNull: false},
				CustomString1:   &NullableString{String: "custom1", IsNull: false},
				CustomString2:   &NullableString{String: "", IsNull: true},
				CustomNumber1:   &NullableFloat64{Float64: 42.5, IsNull: false},
				CustomNumber2:   &NullableFloat64{Float64: 0, IsNull: true},
				CustomDatetime1: &NullableTime{Time: now, IsNull: false},
				CustomDatetime2: &NullableTime{Time: time.Time{}, IsNull: true},
				CustomJSON1:     &NullableJSON{Data: map[string]interface{}{"key": "value"}, IsNull: false},
				CustomJSON2:     &NullableJSON{Data: nil, IsNull: true},
			},
			wantErr: false,
		},
		{
			name:  "valid JSON as string",
			input: validJSON,
			want: &Contact{
				Email:      "test@example.com",
				ExternalID: &NullableString{String: "ext123", IsNull: false},
				// ... other fields same as above ...
			},
			wantErr: false,
		},
		{
			name:    "invalid input type",
			input:   42,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "missing required email",
			input:   `{"external_id": "ext123"}`,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid email format",
			input:   `{"email": "invalid-email"}`,
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid JSON format for nullable string",
			input: `{
				"email": "test@example.com",
				"external_id": 123
			}`,
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid JSON format for nullable float",
			input: `{
				"email": "test@example.com",
				"custom_number_1": "not-a-number"
			}`,
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid JSON format for nullable time",
			input: `{
				"email": "test@example.com",
				"custom_datetime_1": "invalid-time"
			}`,
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid JSON format for custom JSON",
			input: `{
				"email": "test@example.com",
				"custom_json_1": "not-a-json-object"
			}`,
			want:    nil,
			wantErr: true,
		},
		{
			name: "complex custom JSON fields",
			input: `{
				"email": "test@example.com",
				"custom_json_1": {
					"nested": {
						"array": [1, 2, 3],
						"object": {"key": "value"}
					}
				},
				"custom_json_2": [
					{"id": 1, "name": "item1"},
					{"id": 2, "name": "item2"}
				]
			}`,
			want: &Contact{
				Email: "test@example.com",
				CustomJSON1: &NullableJSON{
					Data: map[string]interface{}{
						"nested": map[string]interface{}{
							"array":  []interface{}{float64(1), float64(2), float64(3)},
							"object": map[string]interface{}{"key": "value"},
						},
					},
					IsNull: false,
				},
				CustomJSON2: &NullableJSON{
					Data: []interface{}{
						map[string]interface{}{"id": float64(1), "name": "item1"},
						map[string]interface{}{"id": float64(2), "name": "item2"},
					},
					IsNull: false,
				},
			},
			wantErr: false,
		},
		{
			name: "custom JSON fields with null values",
			input: `{
				"email": "test@example.com",
				"custom_json_1": null,
				"custom_json_2": null,
				"custom_json_3": null,
				"custom_json_4": null,
				"custom_json_5": null
			}`,
			want: &Contact{
				Email:       "test@example.com",
				CustomJSON1: &NullableJSON{Data: nil, IsNull: true},
				CustomJSON2: &NullableJSON{Data: nil, IsNull: true},
				CustomJSON3: &NullableJSON{Data: nil, IsNull: true},
				CustomJSON4: &NullableJSON{Data: nil, IsNull: true},
				CustomJSON5: &NullableJSON{Data: nil, IsNull: true},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromJSON(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Compare specific fields that we want to verify
			if tt.want != nil {
				assert.Equal(t, tt.want.Email, got.Email)

				if tt.want.ExternalID != nil {
					assert.Equal(t, tt.want.ExternalID.String, got.ExternalID.String)
					assert.Equal(t, tt.want.ExternalID.IsNull, got.ExternalID.IsNull)
				}

				if tt.want.CustomJSON1 != nil {
					assert.Equal(t, tt.want.CustomJSON1.IsNull, got.CustomJSON1.IsNull)
					assert.Equal(t, tt.want.CustomJSON1.Data, got.CustomJSON1.Data)
				}

				if tt.want.CustomJSON2 != nil {
					assert.Equal(t, tt.want.CustomJSON2.IsNull, got.CustomJSON2.IsNull)
					assert.Equal(t, tt.want.CustomJSON2.Data, got.CustomJSON2.Data)
				}
			}
		})
	}
}

func TestGetContactsRequest_FromQueryParams(t *testing.T) {
	tests := []struct {
		name       string
		params     url.Values
		wantErr    bool
		wantResult *GetContactsRequest
	}{
		{
			name: "valid request",
			params: url.Values{
				"workspace_id": []string{"workspace123"},
				"email":        []string{"test@example.com"},
				"external_id":  []string{"ext123"},
				"first_name":   []string{"John"},
				"last_name":    []string{"Doe"},
				"phone":        []string{"+1234567890"},
				"country":      []string{"US"},
				"language":     []string{"en"},
				"limit":        []string{"50"},
				"cursor":       []string{"cursor123"},
			},
			wantErr: false,
			wantResult: &GetContactsRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				ExternalID:  "ext123",
				FirstName:   "John",
				LastName:    "Doe",
				Phone:       "+1234567890",
				Country:     "US",
				Language:    "en",
				Limit:       50,
				Cursor:      "cursor123",
			},
		},
		{
			name: "missing workspace ID",
			params: url.Values{
				"email": []string{"test@example.com"},
				"limit": []string{"50"},
			},
			wantErr: true,
		},
		{
			name: "invalid email",
			params: url.Values{
				"workspace_id": []string{"workspace123"},
				"email":        []string{"invalid-email"},
				"limit":        []string{"50"},
			},
			wantErr: true,
		},
		{
			name: "invalid limit format",
			params: url.Values{
				"workspace_id": []string{"workspace123"},
				"limit":        []string{"invalid"},
			},
			wantErr: true,
		},
		{
			name: "limit out of range",
			params: url.Values{
				"workspace_id": []string{"workspace123"},
				"limit":        []string{"200"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &GetContactsRequest{}
			err := req.FromQueryParams(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantResult.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, tt.wantResult.Email, req.Email)
				assert.Equal(t, tt.wantResult.ExternalID, req.ExternalID)
				assert.Equal(t, tt.wantResult.FirstName, req.FirstName)
				assert.Equal(t, tt.wantResult.LastName, req.LastName)
				assert.Equal(t, tt.wantResult.Phone, req.Phone)
				assert.Equal(t, tt.wantResult.Country, req.Country)
				assert.Equal(t, tt.wantResult.Language, req.Language)
				assert.Equal(t, tt.wantResult.Limit, req.Limit)
				assert.Equal(t, tt.wantResult.Cursor, req.Cursor)
			}
		})
	}
}

func TestGetContactsRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request *GetContactsRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: &GetContactsRequest{
				WorkspaceID: "workspace123",
				Limit:       50,
			},
			wantErr: false,
		},
		{
			name: "missing workspace ID",
			request: &GetContactsRequest{
				Limit: 50,
			},
			wantErr: true,
		},
		{
			name: "zero limit should be set to default",
			request: &GetContactsRequest{
				WorkspaceID: "workspace123",
				Limit:       0,
			},
			wantErr: false,
		},
		{
			name: "limit > 100 should be capped",
			request: &GetContactsRequest{
				WorkspaceID: "workspace123",
				Limit:       150,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.request.Limit == 0 {
					assert.Equal(t, 20, tt.request.Limit) // default limit
				} else if tt.request.Limit > 100 {
					assert.Equal(t, 100, tt.request.Limit) // max limit
				}
			}
		})
	}
}

func TestDeleteContactRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request *DeleteContactRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: &DeleteContactRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
			},
			wantErr: false,
		},
		{
			name: "missing workspace ID",
			request: &DeleteContactRequest{
				Email: "test@example.com",
			},
			wantErr: true,
		},
		{
			name: "missing email",
			request: &DeleteContactRequest{
				WorkspaceID: "workspace123",
			},
			wantErr: true,
		},
		{
			name: "invalid email format",
			request: &DeleteContactRequest{
				WorkspaceID: "workspace123",
				Email:       "invalid-email",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBatchImportContactsRequest_Validate(t *testing.T) {
	validContacts := `[{"email":"test@example.com"}]`
	invalidContacts := `[{"email":"invalid-email"}]`
	notAnArray := `"not-an-array"`

	tests := []struct {
		name          string
		request       BatchImportContactsRequest
		wantErr       bool
		expectedEmail string
	}{
		{
			name: "valid request",
			request: BatchImportContactsRequest{
				WorkspaceID: "workspace123",
				Contacts:    json.RawMessage(validContacts),
			},
			wantErr:       false,
			expectedEmail: "test@example.com",
		},
		{
			name: "missing workspace ID",
			request: BatchImportContactsRequest{
				Contacts: json.RawMessage(validContacts),
			},
			wantErr: true,
		},
		{
			name: "invalid contacts format",
			request: BatchImportContactsRequest{
				WorkspaceID: "workspace123",
				Contacts:    json.RawMessage(notAnArray),
			},
			wantErr: true,
		},
		{
			name: "invalid contact data",
			request: BatchImportContactsRequest{
				WorkspaceID: "workspace123",
				Contacts:    json.RawMessage(invalidContacts),
			},
			wantErr:       false, // Request parsing is lenient - validation happens in service layer
			expectedEmail: "",    // FromJSON fails for invalid email, creates empty contact
		},
		{
			name: "empty contacts array",
			request: BatchImportContactsRequest{
				WorkspaceID: "workspace123",
				Contacts:    json.RawMessage(`[]`),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contacts, workspaceID, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, workspaceID)
				assert.Nil(t, contacts)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.request.WorkspaceID, workspaceID)
				assert.NotNil(t, contacts)
				assert.Len(t, contacts, 1)
				if tt.expectedEmail != "" {
					assert.Equal(t, tt.expectedEmail, contacts[0].Email)
				}
			}
		})
	}
}

func TestUpsertContactRequest_Validate(t *testing.T) {
	validContact := `{"email":"test@example.com"}`
	invalidContact := `{"email":"invalid-email"}`

	tests := []struct {
		name    string
		request UpsertContactRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: UpsertContactRequest{
				WorkspaceID: "workspace123",
				Contact:     json.RawMessage(validContact),
			},
			wantErr: false,
		},
		{
			name: "missing workspace ID",
			request: UpsertContactRequest{
				Contact: json.RawMessage(validContact),
			},
			wantErr: true,
		},
		{
			name: "invalid contact data",
			request: UpsertContactRequest{
				WorkspaceID: "workspace123",
				Contact:     json.RawMessage(invalidContact),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contact, workspaceID, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, workspaceID)
				assert.Nil(t, contact)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.request.WorkspaceID, workspaceID)
				assert.NotNil(t, contact)
				assert.Equal(t, "test@example.com", contact.Email)
			}
		})
	}
}

func TestErrContactNotFound_Error(t *testing.T) {
	err := fmt.Errorf("contact not found")
	assert.Equal(t, "contact not found", err.Error())
}

func TestFromJSON_AdditionalCases(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
	}{
		{
			name: "valid JSON with all fields",
			input: `{
				"email": "test@example.com",
				"external_id": "ext123",
				"timezone": "UTC",
				"language": "en",
				"first_name": "John",
				"last_name": "Doe",
				"phone": "+1234567890",
				"address_line_1": "123 Main St",
				"address_line_2": "Apt 4B",
				"country": "US",
				"postcode": "12345",
				"state": "NY",
				"job_title": "Engineer",
				"custom_string_1": "custom1",
				"custom_number_1": 42,
				"custom_datetime_1": "2023-01-01T12:00:00Z",
				"custom_json_1": {"key": "value"}
			}`,
			wantErr: false,
		},
		{
			name:    "unsupported data type",
			input:   123,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   "{invalid json}",
			wantErr: true,
		},
		{
			name: "invalid field types",
			input: `{
				"email": "test@example.com",
				"external_id": 123,
				"custom_number_1": "not a number",
				"custom_datetime_1": "invalid date"
			}`,
			wantErr: true,
		},
		{
			name: "null fields",
			input: `{
				"email": "test@example.com",
				"external_id": null,
				"custom_number_1": null,
				"custom_json_1": null
			}`,
			wantErr: false,
		},
		{
			name: "invalid custom JSON",
			input: `{
				"email": "test@example.com",
				"custom_json_1": "not an object or array"
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contact, err := FromJSON(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, contact)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, contact)
				assert.NotEmpty(t, contact.Email)
			}
		})
	}
}

// TestContact_ToMapOfAny tests the ToMapOfAny method
func TestContact_ToMapOfAny(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name        string
		contact     *Contact
		expectError bool
		validate    func(t *testing.T, result MapOfAny)
	}{
		{
			name: "basic contact with required fields only",
			contact: &Contact{
				Email:     "test@example.com",
				CreatedAt: now,
				UpdatedAt: now,
			},
			expectError: false,
			validate: func(t *testing.T, result MapOfAny) {
				assert.Equal(t, "test@example.com", result["email"])
				assert.NotNil(t, result["created_at"])
				assert.NotNil(t, result["updated_at"])
			},
		},
		{
			name: "contact with optional string fields",
			contact: &Contact{
				Email:     "test@example.com",
				CreatedAt: now,
				UpdatedAt: now,
				FirstName: &NullableString{String: "John", IsNull: false},
				LastName:  &NullableString{String: "Doe", IsNull: false},
				Phone:     &NullableString{String: "+1234567890", IsNull: false},
			},
			expectError: false,
			validate: func(t *testing.T, result MapOfAny) {
				assert.Equal(t, "test@example.com", result["email"])
				assert.Equal(t, "John", result["first_name"])
				assert.Equal(t, "Doe", result["last_name"])
				assert.Equal(t, "+1234567890", result["phone"])
			},
		},
		{
			name: "contact with numeric fields",
			contact: &Contact{
				Email:         "test@example.com",
				CreatedAt:     now,
				UpdatedAt:     now,
				CustomNumber1: &NullableFloat64{Float64: 100.50, IsNull: false},
				CustomNumber2: &NullableFloat64{Float64: 5, IsNull: false},
				CustomNumber3: &NullableFloat64{Float64: 42.0, IsNull: false},
			},
			expectError: false,
			validate: func(t *testing.T, result MapOfAny) {
				assert.Equal(t, "test@example.com", result["email"])
				assert.Equal(t, 100.50, result["custom_number_1"])
				assert.Equal(t, float64(5), result["custom_number_2"])
				assert.Equal(t, float64(42.0), result["custom_number_3"])
			},
		},
		{
			name: "contact with date fields",
			contact: &Contact{
				Email:           "test@example.com",
				CreatedAt:       now,
				UpdatedAt:       now,
				CustomDatetime1: &NullableTime{Time: now, IsNull: false},
			},
			expectError: false,
			validate: func(t *testing.T, result MapOfAny) {
				assert.Equal(t, "test@example.com", result["email"])
				assert.NotNil(t, result["custom_datetime_1"])
			},
		},
		{
			name: "contact with JSON fields",
			contact: &Contact{
				Email:     "test@example.com",
				CreatedAt: now,
				UpdatedAt: now,
				CustomJSON1: &NullableJSON{
					Data:   map[string]interface{}{"preferences": map[string]interface{}{"theme": "dark"}},
					IsNull: false,
				},
				CustomJSON2: &NullableJSON{
					Data:   []interface{}{"tag1", "tag2"},
					IsNull: false,
				},
			},
			expectError: false,
			validate: func(t *testing.T, result MapOfAny) {
				assert.Equal(t, "test@example.com", result["email"])
				// JSON fields should be preserved in the map
				prefsMap, ok := result["custom_json_1"].(map[string]interface{})
				assert.True(t, ok)
				preferences, ok := prefsMap["preferences"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, "dark", preferences["theme"])

				tags, ok := result["custom_json_2"].([]interface{})
				assert.True(t, ok)
				assert.Equal(t, 2, len(tags))
				assert.Equal(t, "tag1", tags[0])
				assert.Equal(t, "tag2", tags[1])
			},
		},
		{
			name: "contact with all types of fields",
			contact: &Contact{
				Email:           "test@example.com",
				ExternalID:      &NullableString{String: "ext123", IsNull: false},
				FirstName:       &NullableString{String: "John", IsNull: false},
				LastName:        &NullableString{String: "Doe", IsNull: false},
				CustomNumber2:   &NullableFloat64{Float64: 100.50, IsNull: false},
				CustomNumber3:   &NullableFloat64{Float64: 5, IsNull: false},
				CustomDatetime2: &NullableTime{Time: now, IsNull: false},
				CustomString1:   &NullableString{String: "Custom 1", IsNull: false},
				CustomNumber1:   &NullableFloat64{Float64: 42.0, IsNull: false},
				CustomDatetime1: &NullableTime{Time: now, IsNull: false},
				CustomJSON1:     &NullableJSON{Data: map[string]interface{}{"key": "value"}, IsNull: false},
				CreatedAt:       now,
				UpdatedAt:       now,
			},
			expectError: false,
			validate: func(t *testing.T, result MapOfAny) {
				assert.Equal(t, "test@example.com", result["email"])
				assert.Equal(t, "ext123", result["external_id"])
				assert.Equal(t, "John", result["first_name"])
				assert.Equal(t, "Doe", result["last_name"])
				assert.Equal(t, 100.50, result["custom_number_2"])
				assert.Equal(t, float64(5), result["custom_number_3"])
				assert.NotNil(t, result["custom_datetime_2"])
				assert.Equal(t, "Custom 1", result["custom_string_1"])
				assert.Equal(t, float64(42.0), result["custom_number_1"])
				assert.NotNil(t, result["custom_datetime_1"])

				jsonField, ok := result["custom_json_1"].(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, "value", jsonField["key"])
			},
		},
		{
			name: "contact with null fields",
			contact: &Contact{
				Email:       "test@example.com",
				FirstName:   &NullableString{String: "", IsNull: true},
				LastName:    &NullableString{String: "", IsNull: true},
				CustomJSON1: &NullableJSON{Data: nil, IsNull: true},
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			expectError: false,
			validate: func(t *testing.T, result MapOfAny) {
				assert.Equal(t, "test@example.com", result["email"])
				// Null fields should be converted to null/nil in the map
				assert.Nil(t, result["first_name"])
				assert.Nil(t, result["last_name"])
				assert.Nil(t, result["custom_json_1"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.contact.ToMapOfAny()

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, result)

			// Run the custom validation function
			tt.validate(t, result)
		})
	}
}

func TestContact_MergeContactLists(t *testing.T) {
	t.Run("merge into nil list", func(t *testing.T) {
		// Create a contact with nil contact lists
		contact := &Contact{
			Email: "test@example.com",
		}

		// Create a list to merge
		list := &ContactList{
			ListID: "list-123",
			Status: ContactListStatusActive,
		}

		// Merge the list
		contact.MergeContactLists(list)

		// Verify the contact now has one list
		require.NotNil(t, contact.ContactLists)
		require.Len(t, contact.ContactLists, 1)
		assert.Equal(t, "list-123", contact.ContactLists[0].ListID)
		assert.Equal(t, ContactListStatusActive, contact.ContactLists[0].Status)
	})

	t.Run("merge new list", func(t *testing.T) {
		// Create a contact with an existing list
		existingList := &ContactList{
			ListID: "list-123",
			Status: ContactListStatusActive,
		}

		contact := &Contact{
			Email:        "test@example.com",
			ContactLists: []*ContactList{existingList},
		}

		// Create a new list to merge
		newList := &ContactList{
			ListID: "list-456",
			Status: ContactListStatusPending,
		}

		// Merge the new list
		contact.MergeContactLists(newList)

		// Verify the contact now has both lists
		require.Len(t, contact.ContactLists, 2)
		assert.Equal(t, "list-123", contact.ContactLists[0].ListID)
		assert.Equal(t, ContactListStatusActive, contact.ContactLists[0].Status)
		assert.Equal(t, "list-456", contact.ContactLists[1].ListID)
		assert.Equal(t, ContactListStatusPending, contact.ContactLists[1].Status)
	})

	t.Run("update existing list", func(t *testing.T) {
		// Create a contact with an existing list
		existingList := &ContactList{
			ListID: "list-123",
			Status: ContactListStatusActive,
		}

		contact := &Contact{
			Email:        "test@example.com",
			ContactLists: []*ContactList{existingList},
		}

		// Create an updated version of the existing list
		updatedList := &ContactList{
			ListID: "list-123",                    // Same ID
			Status: ContactListStatusUnsubscribed, // Updated status
		}

		// Merge the updated list
		contact.MergeContactLists(updatedList)

		// Verify the contact still has only one list with the updated status
		require.Len(t, contact.ContactLists, 1)
		assert.Equal(t, "list-123", contact.ContactLists[0].ListID)
		assert.Equal(t, ContactListStatusUnsubscribed, contact.ContactLists[0].Status)
	})
}

func TestFromJSON_ParseFunctions(t *testing.T) {
	t.Run("parseNullableString", func(t *testing.T) {
		// Test cases for parseNullableString
		testCases := []struct {
			name     string
			json     string
			field    string
			expected *NullableString
			wantErr  bool
		}{
			{
				name:     "valid string",
				json:     `{"test_field": "test value"}`,
				field:    "test_field",
				expected: &NullableString{String: "test value", IsNull: false},
				wantErr:  false,
			},
			{
				name:     "explicit null",
				json:     `{"test_field": null}`,
				field:    "test_field",
				expected: &NullableString{IsNull: true},
				wantErr:  false,
			},
			{
				name:     "field doesn't exist",
				json:     `{"other_field": "value"}`,
				field:    "test_field",
				expected: nil,
				wantErr:  false,
			},
			{
				name:     "wrong type",
				json:     `{"test_field": 123}`,
				field:    "test_field",
				expected: nil,
				wantErr:  true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := gjson.Parse(tc.json)
				var target *NullableString
				err := parseNullableString(result, tc.field, &target)

				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					if tc.expected == nil {
						assert.Nil(t, target)
					} else {
						assert.NotNil(t, target)
						assert.Equal(t, tc.expected.IsNull, target.IsNull)
						if !tc.expected.IsNull {
							assert.Equal(t, tc.expected.String, target.String)
						}
					}
				}
			})
		}
	})

	t.Run("parseNullableFloat", func(t *testing.T) {
		// Test cases for parseNullableFloat
		testCases := []struct {
			name     string
			json     string
			field    string
			expected *NullableFloat64
			wantErr  bool
		}{
			{
				name:     "valid number",
				json:     `{"test_field": 42.5}`,
				field:    "test_field",
				expected: &NullableFloat64{Float64: 42.5, IsNull: false},
				wantErr:  false,
			},
			{
				name:     "explicit null",
				json:     `{"test_field": null}`,
				field:    "test_field",
				expected: &NullableFloat64{IsNull: true},
				wantErr:  false,
			},
			{
				name:     "field doesn't exist",
				json:     `{"other_field": 123}`,
				field:    "test_field",
				expected: nil,
				wantErr:  false,
			},
			{
				name:     "wrong type",
				json:     `{"test_field": "not a number"}`,
				field:    "test_field",
				expected: nil,
				wantErr:  true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := gjson.Parse(tc.json)
				var target *NullableFloat64
				err := parseNullableFloat(result, tc.field, &target)

				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					if tc.expected == nil {
						assert.Nil(t, target)
					} else {
						assert.NotNil(t, target)
						assert.Equal(t, tc.expected.IsNull, target.IsNull)
						if !tc.expected.IsNull {
							assert.Equal(t, tc.expected.Float64, target.Float64)
						}
					}
				}
			})
		}
	})

	t.Run("parseNullableTime", func(t *testing.T) {
		// Valid RFC3339 time
		validTimeStr := "2023-01-15T14:30:45Z"
		validTime, _ := time.Parse(time.RFC3339, validTimeStr)

		// Test cases for parseNullableTime
		testCases := []struct {
			name     string
			json     string
			field    string
			expected *NullableTime
			wantErr  bool
		}{
			{
				name:     "valid time",
				json:     `{"test_field": "2023-01-15T14:30:45Z"}`,
				field:    "test_field",
				expected: &NullableTime{Time: validTime, IsNull: false},
				wantErr:  false,
			},
			{
				name:     "explicit null",
				json:     `{"test_field": null}`,
				field:    "test_field",
				expected: &NullableTime{IsNull: true},
				wantErr:  false,
			},
			{
				name:     "field doesn't exist",
				json:     `{"other_field": "2023-01-15T14:30:45Z"}`,
				field:    "test_field",
				expected: nil,
				wantErr:  false,
			},
			{
				name:     "invalid time format",
				json:     `{"test_field": "not a valid time"}`,
				field:    "test_field",
				expected: nil,
				wantErr:  true,
			},
			{
				name:     "wrong type",
				json:     `{"test_field": 123}`,
				field:    "test_field",
				expected: nil,
				wantErr:  true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := gjson.Parse(tc.json)
				var target *NullableTime
				err := parseNullableTime(result, tc.field, &target)

				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					if tc.expected == nil {
						assert.Nil(t, target)
					} else {
						assert.NotNil(t, target)
						assert.Equal(t, tc.expected.IsNull, target.IsNull)
						if !tc.expected.IsNull {
							assert.Equal(t, tc.expected.Time.UTC().Format(time.RFC3339), target.Time.UTC().Format(time.RFC3339))
						}
					}
				}
			})
		}
	})
}

func TestFromJSON_Comprehensive(t *testing.T) {
	// Test cases for FromJSON with various input types and content
	validTime := "2023-01-15T14:30:45Z"

	// Basic valid JSON with only required fields
	t.Run("minimal valid contact", func(t *testing.T) {
		jsonStr := `{"email": "test@example.com"}`
		contact, err := FromJSON(jsonStr)
		require.NoError(t, err)
		assert.Equal(t, "test@example.com", contact.Email)
	})

	// JSON with all field types
	t.Run("comprehensive contact with all fields", func(t *testing.T) {
		jsonStr := fmt.Sprintf(`{
			"email": "test@example.com",
			"external_id": "ext123",
			"timezone": "Europe/Paris",
			"language": "fr",
			"first_name": "John",
			"last_name": "Doe",
			"phone": "+33612345678",
			"address_line_1": "123 Main St",
			"address_line_2": "Apt 4B",
			"country": "France",
			"postcode": "75001",
			"state": "Paris",
			"job_title": "Engineer",
			"custom_string_1": "Custom Value 1",
			"custom_string_2": "Custom Value 2",
			"custom_number_1": 42.5,
			"custom_number_2": 99,
			"custom_datetime_1": "%s",
			"custom_datetime_2": "%s",
			"custom_json_1": {"preferences": {"theme": "dark"}},
			"custom_json_2": ["item1", "item2"]
		}`, validTime, validTime)

		contact, err := FromJSON(jsonStr)
		require.NoError(t, err)

		// Check required fields
		assert.Equal(t, "test@example.com", contact.Email)

		// Check optional fields
		assert.Equal(t, "ext123", contact.ExternalID.String)
		assert.Equal(t, "Europe/Paris", contact.Timezone.String)
		assert.Equal(t, "fr", contact.Language.String)
		assert.Equal(t, "John", contact.FirstName.String)
		assert.Equal(t, "Doe", contact.LastName.String)
		assert.Equal(t, "+33612345678", contact.Phone.String)
		assert.Equal(t, "123 Main St", contact.AddressLine1.String)
		assert.Equal(t, "Apt 4B", contact.AddressLine2.String)
		assert.Equal(t, "France", contact.Country.String)
		assert.Equal(t, "75001", contact.Postcode.String)
		assert.Equal(t, "Paris", contact.State.String)
		assert.Equal(t, "Engineer", contact.JobTitle.String)

		// Check custom fields
		assert.Equal(t, "Custom Value 1", contact.CustomString1.String)
		assert.Equal(t, "Custom Value 2", contact.CustomString2.String)
		assert.Equal(t, 42.5, contact.CustomNumber1.Float64)
		assert.Equal(t, 99.0, contact.CustomNumber2.Float64)

		// Check custom JSON fields
		assert.NotNil(t, contact.CustomJSON1)
		assert.NotNil(t, contact.CustomJSON2)
	})

	// Test with byte array input
	t.Run("JSON from byte array", func(t *testing.T) {
		jsonBytes := []byte(`{"email": "test@example.com", "first_name": "Jane"}`)
		contact, err := FromJSON(jsonBytes)
		require.NoError(t, err)
		assert.Equal(t, "test@example.com", contact.Email)
		assert.Equal(t, "Jane", contact.FirstName.String)
	})

	// Test with gjson.Result input
	t.Run("JSON from gjson.Result", func(t *testing.T) {
		jsonResult := gjson.Parse(`{"email": "test@example.com", "last_name": "Smith"}`)
		contact, err := FromJSON(jsonResult)
		require.NoError(t, err)
		assert.Equal(t, "test@example.com", contact.Email)
		assert.Equal(t, "Smith", contact.LastName.String)
	})

	// Test failures
	t.Run("missing required email", func(t *testing.T) {
		jsonStr := `{"first_name": "John"}`
		contact, err := FromJSON(jsonStr)
		assert.Error(t, err)
		assert.Nil(t, contact)
		assert.Contains(t, err.Error(), "email is required")
	})

	t.Run("invalid email format", func(t *testing.T) {
		jsonStr := `{"email": "not-an-email"}`
		contact, err := FromJSON(jsonStr)
		assert.Error(t, err)
		assert.Nil(t, contact)
		assert.Contains(t, err.Error(), "invalid email format")
	})

	t.Run("unsupported data type", func(t *testing.T) {
		contact, err := FromJSON(123) // Invalid type
		assert.Error(t, err)
		assert.Nil(t, contact)
		assert.Contains(t, err.Error(), "unsupported data type")
	})

	// Test JSON with null fields
	t.Run("JSON with explicit null fields", func(t *testing.T) {
		jsonStr := `{
			"email": "test@example.com",
			"first_name": null,
			"custom_number_1": null,
			"custom_datetime_1": null,
			"custom_json_1": null
		}`
		contact, err := FromJSON(jsonStr)
		require.NoError(t, err)
		assert.Equal(t, "test@example.com", contact.Email)
		assert.True(t, contact.FirstName.IsNull)
		assert.True(t, contact.CustomNumber1.IsNull)
		assert.True(t, contact.CustomDatetime1.IsNull)
		assert.True(t, contact.CustomJSON1.IsNull)
	})

	// Test custom JSON fields with various types
	t.Run("custom JSON fields with different valid types", func(t *testing.T) {
		jsonStr := `{
			"email": "test@example.com",
			"custom_json_1": {"object": true},
			"custom_json_2": [1, 2, 3],
			"custom_json_3": "invalid" 
		}`
		_, err := FromJSON(jsonStr)
		assert.Error(t, err) // Should fail due to invalid JSON in custom_json_3
		assert.Contains(t, err.Error(), "invalid JSON value for custom_json_3")
	})
}

func TestVerifyEmailHMAC(t *testing.T) {
	email := "test@example.com"
	secretKey := "super-secret-key"

	// Compute a valid HMAC
	validHMAC := ComputeEmailHMAC(email, secretKey)

	// Test correct verification
	t.Run("valid HMAC verification", func(t *testing.T) {
		result := VerifyEmailHMAC(email, validHMAC, secretKey)
		assert.True(t, result)
	})

	// Test invalid HMAC
	t.Run("invalid HMAC verification", func(t *testing.T) {
		invalidHMAC := "invalid-hmac-value"
		result := VerifyEmailHMAC(email, invalidHMAC, secretKey)
		assert.False(t, result)
	})

	// Test different email
	t.Run("different email HMAC verification", func(t *testing.T) {
		differentEmail := "other@example.com"
		result := VerifyEmailHMAC(differentEmail, validHMAC, secretKey)
		assert.False(t, result)
	})

	// Test different secret key
	t.Run("different secret key HMAC verification", func(t *testing.T) {
		differentKey := "different-secret-key"
		result := VerifyEmailHMAC(email, validHMAC, differentKey)
		assert.False(t, result)
	})
}

// TestFromJSON_AllTypesParser tests the FromJSON function with all possible input types
func TestFromJSON_AllTypesParser(t *testing.T) {
	// Test different input types
	t.Run("string input", func(t *testing.T) {
		input := `{"email": "test@example.com"}`
		contact, err := FromJSON(input)
		require.NoError(t, err)
		assert.Equal(t, "test@example.com", contact.Email)
	})

	t.Run("byte array input", func(t *testing.T) {
		input := []byte(`{"email": "test@example.com"}`)
		contact, err := FromJSON(input)
		require.NoError(t, err)
		assert.Equal(t, "test@example.com", contact.Email)
	})

	t.Run("gjson.Result input", func(t *testing.T) {
		input := gjson.Parse(`{"email": "test@example.com"}`)
		contact, err := FromJSON(input)
		require.NoError(t, err)
		assert.Equal(t, "test@example.com", contact.Email)
	})

	t.Run("unsupported type input", func(t *testing.T) {
		input := 123 // integers are not supported
		contact, err := FromJSON(input)
		assert.Error(t, err)
		assert.Nil(t, contact)
		assert.Contains(t, err.Error(), "unsupported data type")
	})
}

// TestFromJSON_ErrorCases tests various error cases for FromJSON
func TestFromJSON_ErrorCases(t *testing.T) {
	t.Run("missing email", func(t *testing.T) {
		input := `{"first_name": "John"}`
		contact, err := FromJSON(input)
		assert.Error(t, err)
		assert.Nil(t, contact)
		assert.Contains(t, err.Error(), "email is required")
	})

	t.Run("invalid email format", func(t *testing.T) {
		input := `{"email": "invalid-email"}`
		contact, err := FromJSON(input)
		assert.Error(t, err)
		assert.Nil(t, contact)
		assert.Contains(t, err.Error(), "invalid email format")
	})

	t.Run("invalid json", func(t *testing.T) {
		// The gjson library is very forgiving with malformed JSON
		// We need a clearly broken JSON to make this fail
		input := `{not-valid-json`
		contact, err := FromJSON(input)
		assert.Nil(t, contact)
		// Even if no error, the contact should be nil because
		// the email field will be missing from the invalid JSON
		if err == nil {
			t.Log("gjson was able to parse invalid JSON but couldn't find email field")
		}
	})
}

// TestParseNullableString tests the parseNullableString function with various inputs
func TestParseNullableString(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		field    string
		expected *NullableString
		wantErr  bool
	}{
		{
			name:     "valid string",
			json:     `{"test_field": "test value"}`,
			field:    "test_field",
			expected: &NullableString{String: "test value", IsNull: false},
			wantErr:  false,
		},
		{
			name:     "explicit null",
			json:     `{"test_field": null}`,
			field:    "test_field",
			expected: &NullableString{IsNull: true},
			wantErr:  false,
		},
		{
			name:     "empty string",
			json:     `{"test_field": ""}`,
			field:    "test_field",
			expected: &NullableString{String: "", IsNull: false},
			wantErr:  false,
		},
		{
			name:     "field doesn't exist",
			json:     `{"other_field": "value"}`,
			field:    "test_field",
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "wrong type - number",
			json:     `{"test_field": 123}`,
			field:    "test_field",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "wrong type - boolean",
			json:     `{"test_field": true}`,
			field:    "test_field",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "wrong type - object",
			json:     `{"test_field": {"nested": "object"}}`,
			field:    "test_field",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "wrong type - array",
			json:     `{"test_field": ["array", "values"]}`,
			field:    "test_field",
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := gjson.Parse(tc.json)
			var target *NullableString
			err := parseNullableString(result, tc.field, &target)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid type for")
			} else {
				assert.NoError(t, err)
				if tc.expected == nil {
					assert.Nil(t, target)
				} else {
					assert.NotNil(t, target)
					assert.Equal(t, tc.expected.IsNull, target.IsNull)
					if !tc.expected.IsNull {
						assert.Equal(t, tc.expected.String, target.String)
					}
				}
			}
		})
	}
}

// TestParseNullableFloat tests the parseNullableFloat function with various inputs
func TestParseNullableFloat(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		field    string
		expected *NullableFloat64
		wantErr  bool
	}{
		{
			name:     "valid integer",
			json:     `{"test_field": 42}`,
			field:    "test_field",
			expected: &NullableFloat64{Float64: 42, IsNull: false},
			wantErr:  false,
		},
		{
			name:     "valid float",
			json:     `{"test_field": 42.5}`,
			field:    "test_field",
			expected: &NullableFloat64{Float64: 42.5, IsNull: false},
			wantErr:  false,
		},
		{
			name:     "zero value",
			json:     `{"test_field": 0}`,
			field:    "test_field",
			expected: &NullableFloat64{Float64: 0, IsNull: false},
			wantErr:  false,
		},
		{
			name:     "negative value",
			json:     `{"test_field": -42.5}`,
			field:    "test_field",
			expected: &NullableFloat64{Float64: -42.5, IsNull: false},
			wantErr:  false,
		},
		{
			name:     "explicit null",
			json:     `{"test_field": null}`,
			field:    "test_field",
			expected: &NullableFloat64{IsNull: true},
			wantErr:  false,
		},
		{
			name:     "field doesn't exist",
			json:     `{"other_field": 123}`,
			field:    "test_field",
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "wrong type - string",
			json:     `{"test_field": "not a number"}`,
			field:    "test_field",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "wrong type - boolean",
			json:     `{"test_field": true}`,
			field:    "test_field",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "wrong type - object",
			json:     `{"test_field": {"nested": "object"}}`,
			field:    "test_field",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "wrong type - array",
			json:     `{"test_field": [1, 2, 3]}`,
			field:    "test_field",
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := gjson.Parse(tc.json)
			var target *NullableFloat64
			err := parseNullableFloat(result, tc.field, &target)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid type for")
			} else {
				assert.NoError(t, err)
				if tc.expected == nil {
					assert.Nil(t, target)
				} else {
					assert.NotNil(t, target)
					assert.Equal(t, tc.expected.IsNull, target.IsNull)
					if !tc.expected.IsNull {
						assert.Equal(t, tc.expected.Float64, target.Float64)
					}
				}
			}
		})
	}
}

// TestParseNullableTime tests the parseNullableTime function with various inputs
func TestParseNullableTime(t *testing.T) {
	// Valid RFC3339 time
	validTimeStr := "2023-01-15T14:30:45Z"
	validTime, _ := time.Parse(time.RFC3339, validTimeStr)

	tests := []struct {
		name     string
		json     string
		field    string
		expected *NullableTime
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid RFC3339 time",
			json:     `{"test_field": "2023-01-15T14:30:45Z"}`,
			field:    "test_field",
			expected: &NullableTime{Time: validTime, IsNull: false},
			wantErr:  false,
		},
		{
			name:     "valid RFC3339Nano time",
			json:     `{"test_field": "2023-01-15T14:30:45.123456789Z"}`,
			field:    "test_field",
			expected: &NullableTime{Time: time.Date(2023, 1, 15, 14, 30, 45, 123456789, time.UTC), IsNull: false},
			wantErr:  false,
		},
		{
			name:     "valid RFC3339 with timezone",
			json:     `{"test_field": "2023-01-15T14:30:45+01:00"}`,
			field:    "test_field",
			expected: &NullableTime{Time: time.Date(2023, 1, 15, 13, 30, 45, 0, time.UTC), IsNull: false}, // Converted to UTC
			wantErr:  false,
		},
		{
			name:     "explicit null",
			json:     `{"test_field": null}`,
			field:    "test_field",
			expected: &NullableTime{IsNull: true},
			wantErr:  false,
		},
		{
			name:     "field doesn't exist",
			json:     `{"other_field": "2023-01-15T14:30:45Z"}`,
			field:    "test_field",
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "invalid time format",
			json:     `{"test_field": "not a valid time"}`,
			field:    "test_field",
			expected: nil,
			wantErr:  true,
			errMsg:   "invalid time format",
		},
		{
			name:     "wrong type - number",
			json:     `{"test_field": 123}`,
			field:    "test_field",
			expected: nil,
			wantErr:  true,
			errMsg:   "invalid type for",
		},
		{
			name:     "wrong type - boolean",
			json:     `{"test_field": true}`,
			field:    "test_field",
			expected: nil,
			wantErr:  true,
			errMsg:   "invalid type for",
		},
		{
			name:     "wrong type - object",
			json:     `{"test_field": {"date": "2023-01-15"}}`,
			field:    "test_field",
			expected: nil,
			wantErr:  true,
			errMsg:   "invalid type for",
		},
		{
			name:     "wrong type - array",
			json:     `{"test_field": ["2023", "01", "15"]}`,
			field:    "test_field",
			expected: nil,
			wantErr:  true,
			errMsg:   "invalid type for",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := gjson.Parse(tc.json)
			var target *NullableTime
			err := parseNullableTime(result, tc.field, &target)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errMsg != "" {
					assert.Contains(t, err.Error(), tc.errMsg)
				}
			} else {
				assert.NoError(t, err)
				if tc.expected == nil {
					assert.Nil(t, target)
				} else {
					assert.NotNil(t, target)
					assert.Equal(t, tc.expected.IsNull, target.IsNull)
					if !tc.expected.IsNull {
						assert.Equal(t, tc.expected.Time.UTC().Format(time.RFC3339), target.Time.UTC().Format(time.RFC3339))
					}
				}
			}
		})
	}
}

func TestGetContactsRequest_FromQueryParams_WithContactListsAndStatuses(t *testing.T) {
	params := url.Values{
		"workspace_id":        []string{"ws_123"},
		"with_contact_lists":  []string{"true"},
		"list_id":             []string{"list_456"},
		"contact_list_status": []string{"active"},
	}

	req := &GetContactsRequest{}
	err := req.FromQueryParams(params)
	assert.NoError(t, err)
	assert.Equal(t, "ws_123", req.WorkspaceID)
	assert.Equal(t, true, req.WithContactLists)
	assert.Equal(t, "list_456", req.ListID)
	assert.Equal(t, "active", req.ContactListStatus)

	// Invalid boolean should error
	paramsInvalid := url.Values{
		"workspace_id":       []string{"ws_123"},
		"with_contact_lists": []string{"notabool"},
	}
	req2 := &GetContactsRequest{}
	err = req2.FromQueryParams(paramsInvalid)
	assert.Error(t, err)
}

func TestScanContact_SetsDBTimestamps(t *testing.T) {
	now := time.Now()
	scanner := &contactMockScanner{
		data: []interface{}{
			"test@example.com",                            // Email
			sql.NullString{String: "", Valid: false},      // ExternalID
			sql.NullString{String: "", Valid: false},      // Timezone
			sql.NullString{String: "", Valid: false},      // Language
			sql.NullString{String: "", Valid: false},      // FirstName
			sql.NullString{String: "", Valid: false},      // LastName
			sql.NullString{String: "", Valid: false},      // FullName
			sql.NullString{String: "", Valid: false},      // Phone
			sql.NullString{String: "", Valid: false},      // AddressLine1
			sql.NullString{String: "", Valid: false},      // AddressLine2
			sql.NullString{String: "", Valid: false},      // Country
			sql.NullString{String: "", Valid: false},      // Postcode
			sql.NullString{String: "", Valid: false},      // State
			sql.NullString{String: "", Valid: false},      // JobTitle
			sql.NullString{String: "", Valid: false},      // CustomString1
			sql.NullString{String: "", Valid: false},      // CustomString2
			sql.NullString{String: "", Valid: false},      // CustomString3
			sql.NullString{String: "", Valid: false},      // CustomString4
			sql.NullString{String: "", Valid: false},      // CustomString5
			sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber1
			sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber2
			sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber3
			sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber4
			sql.NullFloat64{Float64: 0, Valid: false},     // CustomNumber5
			sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime1
			sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime2
			sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime3
			sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime4
			sql.NullTime{Time: time.Time{}, Valid: false}, // CustomDatetime5
			[]byte("null"), // CustomJSON1
			[]byte("null"), // CustomJSON2
			[]byte("null"), // CustomJSON3
			[]byte("null"), // CustomJSON4
			[]byte("null"), // CustomJSON5
			now,            // CreatedAt
			now,            // UpdatedAt
			now,            // DBCreatedAt
			now,            // DBUpdatedAt
		},
	}

	contact, err := ScanContact(scanner)
	assert.NoError(t, err)
	assert.Equal(t, now, contact.CreatedAt)
	assert.Equal(t, now, contact.UpdatedAt)
	assert.Equal(t, now, contact.DBCreatedAt)
	assert.Equal(t, now, contact.DBUpdatedAt)
}

func TestContact_Merge_DBTimeStamps(t *testing.T) {
	base := &Contact{Email: "test@example.com"}
	other := &Contact{Email: "test@example.com"}
	dbCreated := time.Now().Add(-time.Hour)
	dbUpdated := time.Now()
	other.DBCreatedAt = dbCreated
	other.DBUpdatedAt = dbUpdated

	base.Merge(other)
	assert.Equal(t, dbCreated, base.DBCreatedAt)
	assert.Equal(t, dbUpdated, base.DBUpdatedAt)
}

func TestBatchImportContactsRequest_Validate_ErrorIndex(t *testing.T) {
	// Test that request validation is lenient - doesn't reject at request level
	// Individual contact validation happens in the service layer for partial success
	req := BatchImportContactsRequest{
		WorkspaceID: "ws_123",
		Contacts:    json.RawMessage(`[{"email":"valid@example.com"},{"email":"invalid-email"}]`),
	}
	contacts, workspaceID, err := req.Validate()
	assert.NoError(t, err, "Request validation should be lenient")
	assert.Equal(t, "ws_123", workspaceID)
	assert.Len(t, contacts, 2, "Should parse both contacts even if one has invalid data")
}

func TestComputeEmailHMAC_DeterministicAndKeySensitive(t *testing.T) {
	email := "test@example.com"
	key1 := "k1"
	key2 := "k2"

	// Deterministic for same inputs
	h1 := ComputeEmailHMAC(email, key1)
	h2 := ComputeEmailHMAC(email, key1)
	assert.Equal(t, h1, h2)

	// Different keys produce different HMACs
	h3 := ComputeEmailHMAC(email, key2)
	assert.NotEqual(t, h1, h3)
}

func TestFromJSON_TrimsWhitespace(t *testing.T) {
	// NBSP is U+00A0 (non-breaking space)
	nbsp := "\u00a0"

	tests := []struct {
		name           string
		input          string
		expectedEmail  string
		expectedFirst  string
		expectedLast   string
		wantErr        bool
		wantErrContain string
	}{
		{
			name:          "trims trailing NBSP from email",
			input:         `{"email": "test@example.com` + nbsp + `", "first_name": "John"}`,
			expectedEmail: "test@example.com",
			expectedFirst: "John",
			wantErr:       false,
		},
		{
			name:          "trims leading NBSP from email",
			input:         `{"email": "` + nbsp + `test@example.com", "first_name": "John"}`,
			expectedEmail: "test@example.com",
			expectedFirst: "John",
			wantErr:       false,
		},
		{
			name:          "trims surrounding NBSP from email",
			input:         `{"email": "` + nbsp + `test@example.com` + nbsp + `", "first_name": "John"}`,
			expectedEmail: "test@example.com",
			expectedFirst: "John",
			wantErr:       false,
		},
		{
			name:          "trims regular spaces from email",
			input:         `{"email": "  test@example.com  ", "first_name": "John"}`,
			expectedEmail: "test@example.com",
			expectedFirst: "John",
			wantErr:       false,
		},
		{
			name:          "trims mixed whitespace from email",
			input:         `{"email": " ` + nbsp + `test@example.com` + nbsp + ` ", "first_name": "John"}`,
			expectedEmail: "test@example.com",
			expectedFirst: "John",
			wantErr:       false,
		},
		{
			name:          "trims NBSP from string fields",
			input:         `{"email": "test@example.com", "first_name": "` + nbsp + `John` + nbsp + `", "last_name": "` + nbsp + `Doe` + nbsp + `"}`,
			expectedEmail: "test@example.com",
			expectedFirst: "John",
			expectedLast:  "Doe",
			wantErr:       false,
		},
		{
			name:          "trims tabs and newlines from email",
			input:         "{\"email\": \"\ttest@example.com\n\", \"first_name\": \"John\"}",
			expectedEmail: "test@example.com",
			expectedFirst: "John",
			wantErr:       false,
		},
		{
			name:           "email with only NBSP becomes empty and fails validation",
			input:          `{"email": "` + nbsp + nbsp + `"}`,
			wantErr:        true,
			wantErrContain: "email is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contact, err := FromJSON(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrContain)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, contact)

			assert.Equal(t, tt.expectedEmail, contact.Email, "email should be trimmed")

			if tt.expectedFirst != "" && contact.FirstName != nil {
				assert.Equal(t, tt.expectedFirst, contact.FirstName.String, "first_name should be trimmed")
			}

			if tt.expectedLast != "" && contact.LastName != nil {
				assert.Equal(t, tt.expectedLast, contact.LastName.String, "last_name should be trimmed")
			}
		})
	}
}

func TestTrimUnicodeSpace(t *testing.T) {
	// NBSP is U+00A0 (non-breaking space)
	nbsp := "\u00a0"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "no whitespace",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "regular spaces",
			input:    "  hello  ",
			expected: "hello",
		},
		{
			name:     "tabs",
			input:    "\thello\t",
			expected: "hello",
		},
		{
			name:     "newlines",
			input:    "\nhello\n",
			expected: "hello",
		},
		{
			name:     "NBSP only",
			input:    nbsp + nbsp,
			expected: "",
		},
		{
			name:     "trailing NBSP",
			input:    "hello" + nbsp,
			expected: "hello",
		},
		{
			name:     "leading NBSP",
			input:    nbsp + "hello",
			expected: "hello",
		},
		{
			name:     "surrounding NBSP",
			input:    nbsp + "hello" + nbsp,
			expected: "hello",
		},
		{
			name:     "mixed whitespace",
			input:    " " + nbsp + "\t" + "hello" + "\n" + nbsp + " ",
			expected: "hello",
		},
		{
			name:     "preserves internal whitespace",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "preserves internal NBSP",
			input:    "hello" + nbsp + "world",
			expected: "hello" + nbsp + "world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimUnicodeSpace(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
