package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataFeedHeader_Validate(t *testing.T) {
	tests := []struct {
		name    string
		header  DataFeedHeader
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid header",
			header: DataFeedHeader{
				Name:  "Authorization",
				Value: "Bearer token123",
			},
			wantErr: false,
		},
		{
			name: "valid header with custom name",
			header: DataFeedHeader{
				Name:  "X-Custom-Header",
				Value: "custom-value",
			},
			wantErr: false,
		},
		{
			name: "empty name",
			header: DataFeedHeader{
				Name:  "",
				Value: "some-value",
			},
			wantErr: true,
			errMsg:  "header name is required",
		},
		{
			name: "empty value",
			header: DataFeedHeader{
				Name:  "Authorization",
				Value: "",
			},
			wantErr: true,
			errMsg:  "header value is required",
		},
		{
			name: "both empty",
			header: DataFeedHeader{
				Name:  "",
				Value: "",
			},
			wantErr: true,
			errMsg:  "header name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.header.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGlobalFeedSettings_Validate(t *testing.T) {
	tests := []struct {
		name     string
		settings GlobalFeedSettings
		wantErr  bool
		errMsg   string
	}{
		{
			name: "disabled settings - no validation",
			settings: GlobalFeedSettings{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "disabled settings with invalid URL - no validation",
			settings: GlobalFeedSettings{
				Enabled: false,
				URL:     "not-a-valid-url",
			},
			wantErr: false,
		},
		{
			name: "valid https URL",
			settings: GlobalFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/data",
			},
			wantErr: false,
		},
		{
			name: "valid http URL",
			settings: GlobalFeedSettings{
				Enabled: true,
				URL:     "http://api.example.com/data",
			},
			wantErr: false,
		},
		{
			name: "enabled without URL",
			settings: GlobalFeedSettings{
				Enabled: true,
				URL:     "",
			},
			wantErr: true,
			errMsg:  "URL is required when global feed is enabled",
		},
		{
			name: "invalid URL scheme - ftp",
			settings: GlobalFeedSettings{
				Enabled: true,
				URL:     "ftp://api.example.com/data",
			},
			wantErr: true,
			errMsg:  "URL must use http or https scheme",
		},
		{
			name: "invalid URL scheme - file",
			settings: GlobalFeedSettings{
				Enabled: true,
				URL:     "file:///etc/passwd",
			},
			wantErr: true,
			errMsg:  "URL must use http or https scheme",
		},
		{
			name: "URL with query parameters",
			settings: GlobalFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/data?key=value",
			},
			wantErr: true,
			errMsg:  "URL must not contain query parameters",
		},
		{
			name: "URL with fragment",
			settings: GlobalFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/data#section",
			},
			wantErr: true,
			errMsg:  "URL must not contain fragment",
		},
		{
			name: "valid with headers",
			settings: GlobalFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/data",
				Headers: []DataFeedHeader{
					{Name: "Authorization", Value: "Bearer token123"},
					{Name: "X-Custom", Value: "custom-value"},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid header in list",
			settings: GlobalFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/data",
				Headers: []DataFeedHeader{
					{Name: "Authorization", Value: "Bearer token123"},
					{Name: "", Value: "value"}, // Invalid header
				},
			},
			wantErr: true,
			errMsg:  "header name is required",
		},
		{
			name: "URL with path",
			settings: GlobalFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/v1/broadcasts/data",
			},
			wantErr: false,
		},
		{
			name: "URL with port",
			settings: GlobalFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com:8443/data",
			},
			wantErr: false,
		},
		{
			name: "invalid URL format",
			settings: GlobalFeedSettings{
				Enabled: true,
				URL:     "://invalid-url",
			},
			wantErr: true,
			errMsg:  "invalid URL",
		},
		{
			name: "URL with just scheme",
			settings: GlobalFeedSettings{
				Enabled: true,
				URL:     "https://",
			},
			wantErr: true,
			errMsg:  "URL must have a host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.settings.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGlobalFeedSettings_GetTimeout(t *testing.T) {
	// GetTimeout always returns hardcoded 5 seconds
	settings := GlobalFeedSettings{}
	assert.Equal(t, 5, settings.GetTimeout())

	// Even with other settings, timeout is always 5
	settingsWithURL := GlobalFeedSettings{
		Enabled: true,
		URL:     "https://api.example.com/data",
	}
	assert.Equal(t, 5, settingsWithURL.GetTimeout())
}

func TestGlobalFeedSettings_ValueScan(t *testing.T) {
	// Test serialization
	original := GlobalFeedSettings{
		Enabled: true,
		URL:     "https://api.example.com/data",
		Headers: []DataFeedHeader{
			{Name: "Authorization", Value: "Bearer token123"},
		},
	}

	// Test Value method
	value, err := original.Value()
	require.NoError(t, err)
	assert.NotNil(t, value)

	// Test Scan method
	var scanned GlobalFeedSettings
	err = scanned.Scan(value)
	require.NoError(t, err)

	// Verify the scanned value matches the original
	assert.Equal(t, original.Enabled, scanned.Enabled)
	assert.Equal(t, original.URL, scanned.URL)
	require.Len(t, scanned.Headers, 1)
	assert.Equal(t, original.Headers[0].Name, scanned.Headers[0].Name)
	assert.Equal(t, original.Headers[0].Value, scanned.Headers[0].Value)

	// Test scanning nil value
	var nilTarget GlobalFeedSettings
	err = nilTarget.Scan(nil)
	require.NoError(t, err)

	// Test scanning invalid type
	var invalidTarget GlobalFeedSettings
	err = invalidTarget.Scan("not-a-byte-array")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type assertion to []byte failed")

	// Test that null headers becomes empty array after scan
	nullHeadersJSON := []byte(`{"enabled":true,"url":"https://example.com","headers":null}`)
	var nullHeadersTarget GlobalFeedSettings
	err = nullHeadersTarget.Scan(nullHeadersJSON)
	require.NoError(t, err)
	assert.NotNil(t, nullHeadersTarget.Headers, "Headers should not be nil after scanning null")
	assert.Equal(t, 0, len(nullHeadersTarget.Headers), "Headers should be empty array")
}

func TestGlobalFeedSettings_MarshalJSON(t *testing.T) {
	// Test that MarshalJSON produces empty array instead of null for Headers
	emptySettings := GlobalFeedSettings{
		Enabled: true,
		URL:     "https://api.example.com/data",
		Headers: nil, // nil headers
	}
	marshaled, err := emptySettings.MarshalJSON()
	require.NoError(t, err)
	assert.Contains(t, string(marshaled), `"headers":[]`, "Headers should be serialized as empty array, not null")
}

func TestDataFeedHeader_JSON(t *testing.T) {
	// Test JSON marshaling and unmarshaling
	header := DataFeedHeader{
		Name:  "Authorization",
		Value: "Bearer token123",
	}

	// Marshal
	data, err := json.Marshal(header)
	require.NoError(t, err)

	// Unmarshal
	var unmarshaled DataFeedHeader
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, header.Name, unmarshaled.Name)
	assert.Equal(t, header.Value, unmarshaled.Value)
}

func TestGlobalFeedRequestPayload(t *testing.T) {
	// Test the request payload structure
	payload := GlobalFeedRequestPayload{
		Broadcast: GlobalFeedBroadcast{
			ID:   "broadcast123",
			Name: "Test Broadcast",
		},
		List: GlobalFeedList{
			ID:   "list123",
			Name: "Test List",
		},
		Workspace: GlobalFeedWorkspace{
			ID:   "workspace123",
			Name: "Test Workspace",
		},
	}

	// Marshal
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Unmarshal
	var unmarshaled GlobalFeedRequestPayload
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, payload.Broadcast.ID, unmarshaled.Broadcast.ID)
	assert.Equal(t, payload.Broadcast.Name, unmarshaled.Broadcast.Name)
	assert.Equal(t, payload.List.ID, unmarshaled.List.ID)
	assert.Equal(t, payload.List.Name, unmarshaled.List.Name)
	assert.Equal(t, payload.Workspace.ID, unmarshaled.Workspace.ID)
	assert.Equal(t, payload.Workspace.Name, unmarshaled.Workspace.Name)
}

func TestGlobalFeedBroadcast_JSON(t *testing.T) {
	broadcast := GlobalFeedBroadcast{
		ID:   "broadcast123",
		Name: "Test Broadcast",
	}

	data, err := json.Marshal(broadcast)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"id":"broadcast123"`)
	assert.Contains(t, string(data), `"name":"Test Broadcast"`)

	var unmarshaled GlobalFeedBroadcast
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, broadcast.ID, unmarshaled.ID)
	assert.Equal(t, broadcast.Name, unmarshaled.Name)
}

func TestGlobalFeedList_JSON(t *testing.T) {
	list := GlobalFeedList{
		ID:   "list123",
		Name: "Test List",
	}

	data, err := json.Marshal(list)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"id":"list123"`)
	assert.Contains(t, string(data), `"name":"Test List"`)

	var unmarshaled GlobalFeedList
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, list.ID, unmarshaled.ID)
	assert.Equal(t, list.Name, unmarshaled.Name)
}

func TestGlobalFeedWorkspace_JSON(t *testing.T) {
	workspace := GlobalFeedWorkspace{
		ID:   "workspace123",
		Name: "Test Workspace",
	}

	data, err := json.Marshal(workspace)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"id":"workspace123"`)
	assert.Contains(t, string(data), `"name":"Test Workspace"`)

	var unmarshaled GlobalFeedWorkspace
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, workspace.ID, unmarshaled.ID)
	assert.Equal(t, workspace.Name, unmarshaled.Name)
}

func TestGlobalFeedSettings_DefaultValues(t *testing.T) {
	// Test that a new GlobalFeedSettings has expected default values
	settings := GlobalFeedSettings{}

	assert.False(t, settings.Enabled)
	assert.Empty(t, settings.URL)
	assert.Nil(t, settings.Headers)

	// GetTimeout returns hardcoded 5 seconds
	assert.Equal(t, 5, settings.GetTimeout())
}

// Tests for RecipientFeedSettings

func TestRecipientFeedSettings_Validate(t *testing.T) {
	tests := []struct {
		name     string
		settings RecipientFeedSettings
		wantErr  bool
		errMsg   string
	}{
		{
			name: "disabled settings - no validation",
			settings: RecipientFeedSettings{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "valid https URL",
			settings: RecipientFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/recipient",
			},
			wantErr: false,
		},
		{
			name: "enabled without URL",
			settings: RecipientFeedSettings{
				Enabled: true,
				URL:     "",
			},
			wantErr: true,
			errMsg:  "URL is required when recipient feed is enabled",
		},
		{
			name: "invalid URL scheme - ftp",
			settings: RecipientFeedSettings{
				Enabled: true,
				URL:     "ftp://api.example.com/recipient",
			},
			wantErr: true,
			errMsg:  "URL must use http or https scheme",
		},
		{
			name: "invalid URL scheme - http",
			settings: RecipientFeedSettings{
				Enabled: true,
				URL:     "http://api.example.com/recipient",
			},
			wantErr: true,
			errMsg:  "URL must use https scheme",
		},
		{
			name: "URL with query parameters",
			settings: RecipientFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/recipient?key=value",
			},
			wantErr: true,
			errMsg:  "URL must not contain query parameters",
		},
		{
			name: "valid with all options",
			settings: RecipientFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/recipient",
				Headers: []DataFeedHeader{
					{Name: "Authorization", Value: "Bearer token123"},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid header in list",
			settings: RecipientFeedSettings{
				Enabled: true,
				URL:     "https://api.example.com/recipient",
				Headers: []DataFeedHeader{
					{Name: "", Value: "value"},
				},
			},
			wantErr: true,
			errMsg:  "header name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.settings.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRecipientFeedSettings_GetTimeout(t *testing.T) {
	// GetTimeout always returns hardcoded 5 seconds
	settings := RecipientFeedSettings{}
	assert.Equal(t, 5, settings.GetTimeout())

	// Even with other settings, timeout is always 5
	settingsWithURL := RecipientFeedSettings{
		Enabled: true,
		URL:     "https://api.example.com/recipient",
	}
	assert.Equal(t, 5, settingsWithURL.GetTimeout())
}

func TestRecipientFeedSettings_GetMaxRetries(t *testing.T) {
	// GetMaxRetries always returns hardcoded 2 (3 total attempts)
	settings := RecipientFeedSettings{}
	assert.Equal(t, 2, settings.GetMaxRetries())

	// Even with other settings, max retries is always 2
	settingsWithURL := RecipientFeedSettings{
		Enabled: true,
		URL:     "https://api.example.com/recipient",
	}
	assert.Equal(t, 2, settingsWithURL.GetMaxRetries())
}

func TestRecipientFeedSettings_GetRetryDelay(t *testing.T) {
	// GetRetryDelay always returns hardcoded 5000 (5 seconds)
	settings := RecipientFeedSettings{}
	assert.Equal(t, 5000, settings.GetRetryDelay())

	// Even with other settings, retry delay is always 5000
	settingsWithURL := RecipientFeedSettings{
		Enabled: true,
		URL:     "https://api.example.com/recipient",
	}
	assert.Equal(t, 5000, settingsWithURL.GetRetryDelay())
}

func TestNullableStringValue(t *testing.T) {
	tests := []struct {
		name           string
		input          *NullableString
		expectedResult string
	}{
		{
			name:           "nil input returns empty string",
			input:          nil,
			expectedResult: "",
		},
		{
			name:           "IsNull true returns empty string",
			input:          &NullableString{String: "value", IsNull: true},
			expectedResult: "",
		},
		{
			name:           "valid nullable returns value",
			input:          &NullableString{String: "hello", IsNull: false},
			expectedResult: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NullableStringValue(tt.input)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestBuildRecipientFeedContact(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("full contact with all fields", func(t *testing.T) {
		contact := &Contact{
			Email:           "test@example.com",
			ExternalID:      &NullableString{String: "ext123", IsNull: false},
			Timezone:        &NullableString{String: "America/New_York", IsNull: false},
			Language:        &NullableString{String: "en", IsNull: false},
			FirstName:       &NullableString{String: "John", IsNull: false},
			LastName:        &NullableString{String: "Doe", IsNull: false},
			FullName:        &NullableString{String: "John Doe", IsNull: false},
			Phone:           &NullableString{String: "+1234567890", IsNull: false},
			AddressLine1:    &NullableString{String: "123 Main St", IsNull: false},
			AddressLine2:    &NullableString{String: "Apt 4", IsNull: false},
			Country:         &NullableString{String: "USA", IsNull: false},
			Postcode:        &NullableString{String: "12345", IsNull: false},
			State:           &NullableString{String: "NY", IsNull: false},
			JobTitle:        &NullableString{String: "Engineer", IsNull: false},
			CustomString1:   &NullableString{String: "custom1", IsNull: false},
			CustomNumber1:   &NullableFloat64{Float64: 42.5, IsNull: false},
			CustomDatetime1: &NullableTime{Time: now, IsNull: false},
			CreatedAt:       now,
			UpdatedAt:       now,
		}

		result := BuildRecipientFeedContact(contact)

		assert.Equal(t, "test@example.com", result.Email)
		assert.Equal(t, "ext123", result.ExternalID)
		assert.Equal(t, "America/New_York", result.Timezone)
		assert.Equal(t, "en", result.Language)
		assert.Equal(t, "John", result.FirstName)
		assert.Equal(t, "Doe", result.LastName)
		assert.Equal(t, "John Doe", result.FullName)
		assert.Equal(t, "+1234567890", result.Phone)
		assert.Equal(t, "123 Main St", result.AddressLine1)
		assert.Equal(t, "Apt 4", result.AddressLine2)
		assert.Equal(t, "USA", result.Country)
		assert.Equal(t, "12345", result.Postcode)
		assert.Equal(t, "NY", result.State)
		assert.Equal(t, "Engineer", result.JobTitle)
		assert.Equal(t, "custom1", result.CustomString1)
		assert.NotNil(t, result.CustomNumber1)
		assert.Equal(t, 42.5, *result.CustomNumber1)
		assert.NotNil(t, result.CustomDatetime1)
		assert.Equal(t, now.Format(time.RFC3339), *result.CustomDatetime1)
		assert.Equal(t, now.Format(time.RFC3339), result.CreatedAt)
		assert.Equal(t, now.Format(time.RFC3339), result.UpdatedAt)
	})

	t.Run("minimal contact with email only", func(t *testing.T) {
		contact := &Contact{
			Email:     "minimal@example.com",
			CreatedAt: now,
			UpdatedAt: now,
		}

		result := BuildRecipientFeedContact(contact)

		assert.Equal(t, "minimal@example.com", result.Email)
		assert.Empty(t, result.ExternalID)
		assert.Empty(t, result.FirstName)
		assert.Empty(t, result.LastName)
		assert.Nil(t, result.CustomNumber1)
		assert.Nil(t, result.CustomDatetime1)
		assert.Nil(t, result.CustomJSON1)
	})

	t.Run("contact with custom JSON", func(t *testing.T) {
		jsonData := map[string]interface{}{"key": "value", "num": float64(123)}
		contact := &Contact{
			Email:       "json@example.com",
			CustomJSON1: &NullableJSON{Data: jsonData, IsNull: false},
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		result := BuildRecipientFeedContact(contact)

		assert.Equal(t, "json@example.com", result.Email)
		assert.NotNil(t, result.CustomJSON1)
		assert.Equal(t, jsonData, result.CustomJSON1)
	})

	t.Run("contact with null custom JSON", func(t *testing.T) {
		contact := &Contact{
			Email:       "nulljson@example.com",
			CustomJSON1: &NullableJSON{Data: nil, IsNull: true},
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		result := BuildRecipientFeedContact(contact)

		assert.Equal(t, "nulljson@example.com", result.Email)
		assert.Nil(t, result.CustomJSON1)
	})
}

func TestRecipientFeedRequestPayload(t *testing.T) {
	// Test the request payload structure
	contact := RecipientFeedContact{
		Email:     "test@example.com",
		FirstName: "John",
		LastName:  "Doe",
	}

	payload := RecipientFeedRequestPayload{
		Contact: contact,
		Broadcast: RecipientFeedBroadcast{
			ID:   "broadcast123",
			Name: "Test Broadcast",
		},
		List: RecipientFeedList{
			ID:   "list123",
			Name: "Test List",
		},
		Workspace: RecipientFeedWorkspace{
			ID:   "workspace123",
			Name: "Test Workspace",
		},
	}

	// Marshal
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Unmarshal
	var unmarshaled RecipientFeedRequestPayload
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, payload.Contact.Email, unmarshaled.Contact.Email)
	assert.Equal(t, payload.Contact.FirstName, unmarshaled.Contact.FirstName)
	assert.Equal(t, payload.Broadcast.ID, unmarshaled.Broadcast.ID)
	assert.Equal(t, payload.List.ID, unmarshaled.List.ID)
	assert.Equal(t, payload.Workspace.ID, unmarshaled.Workspace.ID)
}

func TestRecipientFeedSettings_ValueScan(t *testing.T) {
	// Test serialization
	original := RecipientFeedSettings{
		Enabled: true,
		URL:     "https://api.example.com/recipient",
		Headers: []DataFeedHeader{
			{Name: "Authorization", Value: "Bearer token123"},
		},
	}

	// Test Value method
	value, err := original.Value()
	require.NoError(t, err)
	assert.NotNil(t, value)

	// Test Scan method
	var scanned RecipientFeedSettings
	err = scanned.Scan(value)
	require.NoError(t, err)

	// Verify the scanned value matches the original
	assert.Equal(t, original.Enabled, scanned.Enabled)
	assert.Equal(t, original.URL, scanned.URL)
	require.Len(t, scanned.Headers, 1)
	assert.Equal(t, original.Headers[0].Name, scanned.Headers[0].Name)
	assert.Equal(t, original.Headers[0].Value, scanned.Headers[0].Value)

	// Test scanning nil value
	var nilTarget RecipientFeedSettings
	err = nilTarget.Scan(nil)
	require.NoError(t, err)

	// Test scanning invalid type
	var invalidTarget RecipientFeedSettings
	err = invalidTarget.Scan("not-a-byte-array")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type assertion to []byte failed")

	// Test that null headers becomes empty array after scan
	nullHeadersJSON := []byte(`{"enabled":true,"url":"https://example.com","headers":null}`)
	var nullHeadersTarget RecipientFeedSettings
	err = nullHeadersTarget.Scan(nullHeadersJSON)
	require.NoError(t, err)
	assert.NotNil(t, nullHeadersTarget.Headers, "Headers should not be nil after scanning null")
	assert.Equal(t, 0, len(nullHeadersTarget.Headers), "Headers should be empty array")
}

func TestRecipientFeedSettings_MarshalJSON(t *testing.T) {
	// Test that MarshalJSON produces empty array instead of null for Headers
	emptySettings := RecipientFeedSettings{
		Enabled: true,
		URL:     "https://api.example.com/recipient",
		Headers: nil, // nil headers
	}
	marshaled, err := emptySettings.MarshalJSON()
	require.NoError(t, err)
	assert.Contains(t, string(marshaled), `"headers":[]`, "Headers should be serialized as empty array, not null")
}

// Tests for DataFeedSettings (consolidated struct)

func TestDataFeedSettings_Validate(t *testing.T) {
	tests := []struct {
		name     string
		settings DataFeedSettings
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "empty settings - valid",
			settings: DataFeedSettings{},
			wantErr:  false,
		},
		{
			name: "nil global feed and recipient feed - valid",
			settings: DataFeedSettings{
				GlobalFeed:    nil,
				RecipientFeed: nil,
			},
			wantErr: false,
		},
		{
			name: "valid global feed only",
			settings: DataFeedSettings{
				GlobalFeed: &GlobalFeedSettings{
					Enabled: true,
					URL:     "https://api.example.com/data",
				},
			},
			wantErr: false,
		},
		{
			name: "valid recipient feed only",
			settings: DataFeedSettings{
				RecipientFeed: &RecipientFeedSettings{
					Enabled: true,
					URL:     "https://api.example.com/recipient",
				},
			},
			wantErr: false,
		},
		{
			name: "valid both feeds",
			settings: DataFeedSettings{
				GlobalFeed: &GlobalFeedSettings{
					Enabled: true,
					URL:     "https://api.example.com/data",
				},
				RecipientFeed: &RecipientFeedSettings{
					Enabled: true,
					URL:     "https://api.example.com/recipient",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid global feed - missing URL",
			settings: DataFeedSettings{
				GlobalFeed: &GlobalFeedSettings{
					Enabled: true,
					URL:     "",
				},
			},
			wantErr: true,
			errMsg:  "global feed: URL is required",
		},
		{
			name: "invalid recipient feed - invalid scheme",
			settings: DataFeedSettings{
				RecipientFeed: &RecipientFeedSettings{
					Enabled: true,
					URL:     "http://api.example.com/recipient", // HTTP not allowed
				},
			},
			wantErr: true,
			errMsg:  "recipient feed: URL must use https scheme",
		},
		{
			name: "disabled feeds - no validation errors",
			settings: DataFeedSettings{
				GlobalFeed: &GlobalFeedSettings{
					Enabled: false,
					URL:     "invalid-url",
				},
				RecipientFeed: &RecipientFeedSettings{
					Enabled: false,
					URL:     "invalid-url",
				},
			},
			wantErr: false,
		},
		{
			name: "with global feed data",
			settings: DataFeedSettings{
				GlobalFeed: &GlobalFeedSettings{
					Enabled: true,
					URL:     "https://api.example.com/data",
				},
				GlobalFeedData: map[string]interface{}{
					"discount_code": "SUMMER2024",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.settings.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDataFeedSettings_ValueScan(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	// Test serialization with all fields
	original := DataFeedSettings{
		GlobalFeed: &GlobalFeedSettings{
			Enabled: true,
			URL:     "https://api.example.com/data",
			Headers: []DataFeedHeader{
				{Name: "Authorization", Value: "Bearer token123"},
			},
		},
		GlobalFeedData: map[string]interface{}{
			"discount_code": "SUMMER2024",
			"featured": map[string]interface{}{
				"title": "Featured Product",
			},
		},
		GlobalFeedFetchedAt: &now,
		RecipientFeed: &RecipientFeedSettings{
			Enabled: true,
			URL:     "https://api.example.com/recipient",
		},
	}

	// Test Value method
	value, err := original.Value()
	require.NoError(t, err)
	assert.NotNil(t, value)

	// Test Scan method
	var scanned DataFeedSettings
	err = scanned.Scan(value)
	require.NoError(t, err)

	// Verify the scanned value matches the original
	require.NotNil(t, scanned.GlobalFeed)
	assert.Equal(t, original.GlobalFeed.Enabled, scanned.GlobalFeed.Enabled)
	assert.Equal(t, original.GlobalFeed.URL, scanned.GlobalFeed.URL)
	require.Len(t, scanned.GlobalFeed.Headers, 1)

	assert.NotNil(t, scanned.GlobalFeedData)
	assert.Equal(t, "SUMMER2024", scanned.GlobalFeedData["discount_code"])

	require.NotNil(t, scanned.GlobalFeedFetchedAt)
	assert.Equal(t, now.Format(time.RFC3339), scanned.GlobalFeedFetchedAt.Format(time.RFC3339))

	require.NotNil(t, scanned.RecipientFeed)
	assert.Equal(t, original.RecipientFeed.Enabled, scanned.RecipientFeed.Enabled)
	assert.Equal(t, original.RecipientFeed.URL, scanned.RecipientFeed.URL)

	// Test scanning nil value
	var nilTarget DataFeedSettings
	err = nilTarget.Scan(nil)
	require.NoError(t, err)
	assert.Nil(t, nilTarget.GlobalFeed)
	assert.Nil(t, nilTarget.RecipientFeed)

	// Test scanning invalid type
	var invalidTarget DataFeedSettings
	err = invalidTarget.Scan("not-a-byte-array")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type assertion to []byte failed")

	// Test empty object
	emptyJSON := []byte(`{}`)
	var emptyTarget DataFeedSettings
	err = emptyTarget.Scan(emptyJSON)
	require.NoError(t, err)
	assert.Nil(t, emptyTarget.GlobalFeed)
	assert.Nil(t, emptyTarget.RecipientFeed)
}

func TestDataFeedSettings_PartialData(t *testing.T) {
	// Test with only global feed
	globalOnly := DataFeedSettings{
		GlobalFeed: &GlobalFeedSettings{
			Enabled: true,
			URL:     "https://api.example.com/data",
		},
	}

	value, err := globalOnly.Value()
	require.NoError(t, err)

	var scanned DataFeedSettings
	err = scanned.Scan(value)
	require.NoError(t, err)

	assert.NotNil(t, scanned.GlobalFeed)
	assert.Nil(t, scanned.RecipientFeed)
	assert.Nil(t, scanned.GlobalFeedData)
	assert.Nil(t, scanned.GlobalFeedFetchedAt)

	// Test with only recipient feed
	recipientOnly := DataFeedSettings{
		RecipientFeed: &RecipientFeedSettings{
			Enabled: true,
			URL:     "https://api.example.com/recipient",
		},
	}

	value, err = recipientOnly.Value()
	require.NoError(t, err)

	var scanned2 DataFeedSettings
	err = scanned2.Scan(value)
	require.NoError(t, err)

	assert.Nil(t, scanned2.GlobalFeed)
	assert.NotNil(t, scanned2.RecipientFeed)
}

func TestDataFeedSettings_JSONSerialization(t *testing.T) {
	now := time.Now().UTC()

	settings := DataFeedSettings{
		GlobalFeed: &GlobalFeedSettings{
			Enabled: true,
			URL:     "https://api.example.com/data",
		},
		GlobalFeedData: map[string]interface{}{
			"key": "value",
		},
		GlobalFeedFetchedAt: &now,
	}

	// Marshal to JSON
	data, err := json.Marshal(settings)
	require.NoError(t, err)

	// Verify JSON structure
	assert.Contains(t, string(data), `"global_feed"`)
	assert.Contains(t, string(data), `"global_feed_data"`)
	assert.Contains(t, string(data), `"global_feed_fetched_at"`)

	// Unmarshal back
	var unmarshaled DataFeedSettings
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.NotNil(t, unmarshaled.GlobalFeed)
	assert.True(t, unmarshaled.GlobalFeed.Enabled)
	assert.Equal(t, "value", unmarshaled.GlobalFeedData["key"])
}
